package claude

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"mime"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/quota"
)

const (
	// maxEventLine is the longest line of an answer's event stream that's
	// counted: none the Messages API streams comes near it, and counting
	// stops at one longer.
	maxEventLine = 1 << 20
	// maxMessage is the most of an answer that isn't streamed that's read for
	// its usage.
	maxMessage = 16 << 20
)

// Count reads an answer of the Messages API's, decoded, as far as it needs, by
// its content type: a stream of events, calling chars with how many characters
// of text, thinking and tools' input it has streamed so far as each part of
// them comes, or a message whole. It returns the tokens the answer's closing
// usage gives, reporting false when none came, as for an answer cut short, an
// error, or an answer of another kind, such as a count of tokens.
func (Provider) Count(contentType string, body io.Reader, chars func(int)) (quota.Tokens, bool) {
	media, _, _ := mime.ParseMediaType(contentType)
	switch media {
	case "text/event-stream":
		return countEvents(body, chars)
	case "application/json":
		return countMessage(body)
	}
	return quota.Tokens{}, false
}

// countEvents reads a stream of the Messages API's events, as Count does.
func countEvents(body io.Reader, chars func(int)) (quota.Tokens, bool) {
	lines := bufio.NewScanner(body)
	lines.Buffer(nil, maxEventLine)
	var t tally
	for lines.Scan() {
		data, ok := bytes.CutPrefix(lines.Bytes(), []byte("data:"))
		if !ok {
			continue
		}
		if t.take(bytes.TrimPrefix(data, []byte(" "))) {
			chars(t.chars)
		}
	}
	return t.closing()
}

// countMessage reads a message the Messages API answered with whole, as Count
// does.
func countMessage(body io.Reader) (quota.Tokens, bool) {
	var message struct {
		Type  string `json:"type"`
		Usage *usage `json:"usage"`
	}
	if err := json.NewDecoder(io.LimitReader(body, maxMessage)).Decode(&message); err != nil {
		return quota.Tokens{}, false
	}
	if message.Type != "message" || message.Usage == nil {
		return quota.Tokens{}, false
	}
	return message.Usage.tokens(), true
}

// event is what counting needs of one of the Messages API's stream events:
// its type; the usage its message starts with, or brings up to date; and
// what it adds to a block.
type event struct {
	Type    string `json:"type"`
	Message struct {
		Usage *usage `json:"usage"`
	} `json:"message"`
	Usage *usage `json:"usage"`
	Delta delta  `json:"delta"`
}

// delta is what an event adds to a block of the answer: text, thinking, or a
// piece of the JSON of a tool's input.
type delta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Thinking    string `json:"thinking"`
	PartialJSON string `json:"partial_json"`
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

// usage is the tokens an event gives, each nil where it gives none.
type usage struct {
	Input      *int `json:"input_tokens"`
	Output     *int `json:"output_tokens"`
	CacheRead  *int `json:"cache_read_input_tokens"`
	CacheWrite *int `json:"cache_creation_input_tokens"`
}

// update brings u up to date with later, whose counts are its message's so
// far: each it gives replaces u's.
func (u *usage) update(later *usage) {
	if later == nil {
		return
	}
	u.Input = cmp.Or(later.Input, u.Input)
	u.Output = cmp.Or(later.Output, u.Output)
	u.CacheRead = cmp.Or(later.CacheRead, u.CacheRead)
	u.CacheWrite = cmp.Or(later.CacheWrite, u.CacheWrite)
}

// tokens are the tokens u gives, none where it gives none.
func (u *usage) tokens() quota.Tokens {
	return quota.Tokens{Input: given(u.Input), Output: given(u.Output), CacheRead: given(u.CacheRead), CacheWrite: given(u.CacheWrite)}
}

// given is the count n gives, 0 where it gives none.
func given(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

// tally is what an answer's events have told so far: the characters of its
// text, thinking and tools' input, and its usage, as its start gives it and
// each delta of the message brings it up to date, the last of which closes
// it.
type tally struct {
	chars  int
	usage  usage
	closed bool
}

// take takes in an event's data, and reports whether it brought characters
// of the answer's text, thinking or tools' input. Data that isn't an event is
// passed over.
func (t *tally) take(data []byte) bool {
	var e event
	if json.Unmarshal(data, &e) != nil {
		return false
	}
	switch e.Type {
	case "message_start":
		t.usage.update(e.Message.Usage)
	case "message_delta":
		t.usage.update(e.Usage)
		t.closed = t.closed || e.Usage != nil
	case "content_block_delta":
		n := e.Delta.chars()
		t.chars += n
		return n > 0
	}
	return false
}

// closing returns the tokens the answer's closing usage gave, reporting false
// when none came.
func (t *tally) closing() (quota.Tokens, bool) {
	if !t.closed {
		return quota.Tokens{}, false
	}
	return t.usage.tokens(), true
}
