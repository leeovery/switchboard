package claude

import (
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/linescan"
	"github.com/leeovery/switchboard/internal/quota"
)

const (
	// maxEventLine is how much of a line of an answer's event stream is read
	// at most, its line ending included: a line that runs to it without its
	// ending, as a server tool's result given whole as its block starts can,
	// is passed over without being held, and the lines after it read.
	maxEventLine = 1 << 20
	// maxMessage is the most of an answer that isn't streamed that's read for
	// its usage.
	maxMessage = 16 << 20
	// requestIDHeader gives Anthropic's id for an answer.
	requestIDHeader = "Request-Id"
)

// toolCalls are the kinds of block that call a tool, by its name: the
// client's tools, those the API runs itself, and those of the MCP servers its
// connector calls.
var toolCalls = []string{"tool_use", "server_tool_use", "mcp_tool_use"}

// Count reads an answer of the Messages API's, by its header, and its body,
// decoded, as far as it needs, by its content type: a stream of events,
// calling chars with how many characters of text, thinking and tools' input
// it has streamed so far as each part of them comes, or a message whole. It
// returns the tokens the answer's closing usage gives, nil when none came, as
// for an answer cut short, an error, or an answer of another kind, such as a
// count of tokens; and what the request ledger keeps of the answer: from its
// header, Anthropic's id for it and its usage headers, and from its body, the
// model that served it, why it stopped, its blocks, the tools it called, its
// error and its closing usage, as the API gave it.
func (Provider) Count(h http.Header, body io.Reader, chars func(int)) (*quota.Tokens, ledger.Reply) {
	var t tally
	media, _, _ := mime.ParseMediaType(h.Get("Content-Type"))
	switch media {
	case "text/event-stream":
		t.events(body, chars)
	case "application/json":
		t.message(body)
	}
	reply := t.reply()
	reply.Answer.ID, reply.Limits = h.Get(requestIDHeader), limitsOf(h)
	return t.closing(), reply
}

// event is what counting needs of one of the Messages API's stream events:
// its type; the model its message names, and the usage its message starts
// with, or brings up to date; the block it starts; what it adds to a block,
// or why the message stopped; and the error it ends in.
type event struct {
	Type    string `json:"type"`
	Message struct {
		Model string `json:"model"`
		Usage usage  `json:"usage"`
	} `json:"message"`
	ContentBlock block        `json:"content_block"`
	Delta        delta        `json:"delta"`
	Usage        usage        `json:"usage"`
	Error        ledger.Error `json:"error"`
}

// block is a block of an answer: its kind; the tool it calls, by name, where
// it calls one; and, of a fallback, what it hands the answer off to.
type block struct {
	Type string  `json:"type"`
	Name string  `json:"name"`
	To   handoff `json:"to"`
}

// handoff is what a fallback hands an answer off to: the model that goes on
// with it.
type handoff struct {
	Model string `json:"model"`
}

// delta is what an event adds to a block of the answer: text, thinking, or a
// piece of the JSON of a tool's input; or, of the message, why it stopped.
type delta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Thinking    string `json:"thinking"`
	PartialJSON string `json:"partial_json"`
	StopReason  string `json:"stop_reason"`
}

// chars is how many characters d adds to what the answer writes: of its text,
// its thinking or a tool's input. A signature or a citation adds none.
func (d delta) chars() int {
	switch d.Type {
	case "text_delta":
		return utf8.RuneCountInString(d.Text)
	case "thinking_delta":
		return utf8.RuneCountInString(d.Thinking)
	case "input_json_delta":
		return utf8.RuneCountInString(d.PartialJSON)
	}
	return 0
}

// usage is the usage an answer gives, as the API gives it, field for field,
// so whatever it counts is kept as it is.
type usage map[string]json.RawMessage

// update brings u up to date with later, whose counts are its message's so
// far: each field it gives replaces u's, but for one it gives as null, which
// keeps a count u has.
func (u *usage) update(later usage) {
	for field, value := range later {
		if _, has := (*u)[field]; has && string(value) == "null" {
			continue
		}
		if *u == nil {
			*u = make(usage, len(later))
		}
		(*u)[field] = value
	}
}

// tokens are the tokens u gives, none where it gives none.
func (u usage) tokens() quota.Tokens {
	return quota.Tokens{Input: u.count("input_tokens"), Output: u.count("output_tokens"),
		CacheRead: u.count("cache_read_input_tokens"), CacheWrite: u.count("cache_creation_input_tokens")}
}

// count is the count u gives as field, 0 where it gives none.
func (u usage) count(field string) int {
	n, _ := strconv.Atoi(string(u[field]))
	return n
}

// tally is what an answer has told so far: the characters of its text,
// thinking and tools' input; its usage, as its start gives it and each delta
// of the message brings it up to date, the last of which closes it; and what
// it tells of itself.
type tally struct {
	chars  int
	usage  usage
	closed bool
	answer ledger.Answer
}

// events reads a stream of the Messages API's events, as Count does.
func (t *tally) events(body io.Reader, chars func(int)) {
	lines := linescan.New(body, maxEventLine)
	for lines.Scan() {
		data, ok := bytes.CutPrefix(lines.Bytes(), []byte("data:"))
		if !ok {
			continue
		}
		if t.take(bytes.TrimPrefix(data, []byte(" "))) {
			chars(t.chars)
		}
	}
}

// take takes in an event's data, and reports whether it brought characters
// of the answer's text, thinking or tools' input. Data that isn't an event is
// passed over, and a field of an event of another type than the API gives is
// left unread, the rest of the event read all the same.
func (t *tally) take(data []byte) bool {
	var e event
	if !objectRead(json.Unmarshal(data, &e)) {
		return false
	}
	switch e.Type {
	case "message_start":
		t.answer.Model = e.Message.Model
		t.usage.update(e.Message.Usage)
	case "content_block_start":
		t.held(e.ContentBlock)
	case "content_block_delta":
		n := e.Delta.chars()
		t.chars += n
		return n > 0
	case "message_delta":
		t.answer.Stop = cmp.Or(e.Delta.StopReason, t.answer.Stop)
		t.usage.update(e.Usage)
		t.closed = t.closed || e.Usage != nil
	case "error":
		t.answer.Error = e.Error
	}
	return false
}

// message reads a message the Messages API answered with whole, as Count
// does, or the error it answered with: a field of another type than the API
// gives is left unread, the rest read all the same.
func (t *tally) message(body io.Reader) {
	var message struct {
		Type       string       `json:"type"`
		Model      string       `json:"model"`
		StopReason string       `json:"stop_reason"`
		Content    []block      `json:"content"`
		Usage      usage        `json:"usage"`
		Error      ledger.Error `json:"error"`
	}
	if !objectRead(json.NewDecoder(io.LimitReader(body, maxMessage)).Decode(&message)) {
		return
	}
	switch message.Type {
	case "message":
		t.answer.Model, t.answer.Stop = message.Model, message.StopReason
		for _, b := range message.Content {
			t.held(b)
		}
		t.usage.update(message.Usage)
		t.closed = message.Usage != nil
	case "error":
		t.answer.Error = message.Error
	}
}

// held notes a block the answer holds, and the tool it calls, by name, where
// it calls one. The model a fallback hands the answer off to, where it gives
// one, is the answer's from then on: the one a stream's start named is the
// one that handed it off.
func (t *tally) held(b block) {
	if b.Type == "" {
		return
	}
	if t.answer.Blocks == nil {
		t.answer.Blocks = make(map[string]int)
	}
	t.answer.Blocks[b.Type]++
	switch {
	case slices.Contains(toolCalls, b.Type):
		t.answer.Tools = append(t.answer.Tools, b.Name)
	case b.Type == "fallback":
		t.answer.Model = cmp.Or(b.To.Model, t.answer.Model)
	}
}

// closing returns the tokens the answer's closing usage gave, nil when none
// came.
func (t *tally) closing() *quota.Tokens {
	if !t.closed {
		return nil
	}
	return new(t.usage.tokens())
}

// reply is what the request ledger keeps of what the answer's body told: of
// the answer, and its closing usage, where it came.
func (t *tally) reply() ledger.Reply {
	reply := ledger.Reply{Answer: t.answer}
	if t.closed && len(t.usage) > 0 {
		reply.Usage, _ = json.Marshal(t.usage)
	}
	return reply
}
