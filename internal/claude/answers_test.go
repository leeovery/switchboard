package claude_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
)

func TestProviderCountsAStreamedAnswer(t *testing.T) {
	const (
		start = claudetest.MessageStart
		usage = claudetest.MessageDelta
		stop  = claudetest.MessageStop
	)
	text := claudetest.TextDelta
	// longest is a text whose delta's line runs to 1 MiB, its line ending
	// included, the longest held.
	longest := textOfLine(1 << 20)
	tests := []struct {
		name   string
		events []string
		// broken is what the body fails with once its events are read, if it
		// does.
		broken error
		// wantChars are the counts chars is called with, in turn.
		wantChars  []int
		wantTokens *quota.Tokens
	}{
		{
			name: "its thinking, text and tool's input as each comes, and its closing usage",
			events: []string{start, claudetest.ThinkingStart, claudetest.ThinkingDelta("Let me think."), claudetest.Signature, claudetest.BlockStop,
				text("Hello, "), text("world ☃"), claudetest.InputDelta(`{"path": "a long path"`), claudetest.Ping, usage, stop},
			wantChars:  []int{13, 20, 27, 49},
			wantTokens: &claudetest.AnswerTokens,
		},
		{
			name:       "a tool's input, a piece of its JSON at a time, as the pieces read decoded",
			events:     []string{start, claudetest.InputDelta(`{"text": "café`), claudetest.InputDelta(` au lait\n"}`), usage},
			wantChars:  []int{14, 26},
			wantTokens: &claudetest.AnswerTokens,
		},
		{
			name: "a closing usage that gives every count itself",
			events: []string{start, text("Hi"),
				"event: message_delta\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":9,"cache_creation_input_tokens":0,"cache_read_input_tokens":41000,"output_tokens":7}}` + "\n\n",
				stop},
			wantChars:  []int{2},
			wantTokens: &quota.Tokens{Input: 9, Output: 7, CacheRead: 41000},
		},
		{
			name:      "an answer cut short, before its closing usage",
			events:    []string{start, text("Hello")},
			wantChars: []int{5},
		},
		{
			name:      "an answer that ends in an error",
			events:    []string{start, text("Hello"), claudetest.Overloaded},
			wantChars: []int{5},
		},
		{
			name:      "an answer read as far as it can be, failing then",
			events:    []string{start, text("Hello")},
			broken:    io.ErrUnexpectedEOF,
			wantChars: []int{5},
		},
		{
			name: "lines it can't read passed over",
			events: []string{"data: not json\n\n", ": a comment\n\n", "event: content_block_delta\ndata: {\"type\":\"content_block_delta\"\n\n",
				start, "data:" + strings.TrimPrefix(strings.TrimPrefix(text("tight"), "event: content_block_delta\n"), "data: "), usage},
			wantChars:  []int{5},
			wantTokens: &claudetest.AnswerTokens,
		},
		{
			name:       "lines that end in CRLF",
			events:     []string{strings.ReplaceAll(start+text("crlf")+usage, "\n", "\r\n")},
			wantChars:  []int{4},
			wantTokens: &claudetest.AnswerTokens,
		},
		{
			name:       "a line as long as can be held, its line ending included, read",
			events:     []string{start, text(longest), usage},
			wantChars:  []int{len(longest)},
			wantTokens: &claudetest.AnswerTokens,
		},
		{
			name:       "a line too long to hold passed over, and those after it read",
			events:     []string{start, text(longest + "x"), text("late"), usage},
			wantChars:  []int{4},
			wantTokens: &claudetest.AnswerTokens,
		},
		{
			name: "a model and a stop reason of other types than the API gives, which cost the counting nothing",
			events: []string{swapped(t, start, `"model":"claude-opus-5-5"`, `"model":{"id":"claude-opus-5-5"}`), text("Hi"),
				swapped(t, usage, `"stop_reason":"end_turn"`, `"stop_reason":{"type":"end_turn"}`), stop},
			wantChars:  []int{2},
			wantTokens: &claudetest.AnswerTokens,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var told []int
			body := iotest.HalfReader(gunzipped(t, strings.Join(tt.events, "")))
			if tt.broken != nil {
				body = io.MultiReader(body, iotest.ErrReader(tt.broken))
			}
			tokens, _ := (claude.Provider{}).Count(typed("text/event-stream; charset=utf-8"), body, func(n int) { told = append(told, n) })
			if !reflect.DeepEqual(tokens, tt.wantTokens) {
				t.Errorf("Count() tokens = %+v, want %+v", tokens, tt.wantTokens)
			}
			if !slices.Equal(told, tt.wantChars) {
				t.Errorf("chars was told %v, want %v", told, tt.wantChars)
			}
		})
	}
}

func TestProviderCountsAMessageAnsweredWhole(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		wantTokens  *quota.Tokens
	}{
		{
			name:        "a message",
			contentType: "application/json",
			body:        claudetest.Message,
			wantTokens:  &claudetest.AnswerTokens,
		},
		{
			name:        "an error",
			contentType: "application/json",
			body:        `{"type":"error","error":{"type":"rate_limit_error","message":"Error"}}`,
		},
		{name: "a count of tokens, which spends none", contentType: "application/json", body: `{"input_tokens":1234}`},
		{name: "a message without its usage", contentType: "application/json", body: `{"type":"message","content":[]}`},
		{name: "a body that isn't JSON", contentType: "application/json", body: `<html>Bad gateway</html>`},
		{name: "a body of another type", contentType: "text/html", body: `{"type":"message","usage":{"output_tokens":1}}`},
		{name: "a body of no type", body: `{"type":"message","usage":{"output_tokens":1}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, _ := (claude.Provider{}).Count(typed(tt.contentType), strings.NewReader(tt.body), func(n int) {
				t.Errorf("chars told %d, want nothing told of an answer that doesn't stream", n)
			})
			if !reflect.DeepEqual(tokens, tt.wantTokens) {
				t.Errorf("Count() tokens = %+v, want %+v", tokens, tt.wantTokens)
			}
		})
	}
}

func TestProviderReadsAnAnswerForTheLedger(t *testing.T) {
	// limited is an answer's header as the API gives it: its id, and its
	// usage headers, beside one of another kind.
	limited := http.Header{
		"Request-Id":                                       {"req_011CTest"},
		"Anthropic-Ratelimit-Unified-Status":               {"allowed"},
		"Anthropic-Ratelimit-Unified-Representative-Claim": {"five_hour"},
		"Anthropic-Ratelimit-Unified-5h-Utilization":       {"0.23"},
		"Anthropic-Ratelimit-Unified-5h-Reset":             {"1791320400"},
		"Anthropic-Ratelimit-Unified-7d-Utilization":       {"0.41"},
		"X-Should-Retry":                                   {"false"},
	}
	limits := map[string]string{"status": "allowed", "representative-claim": "five_hour", "5h-utilization": "0.23", "5h-reset": "1791320400", "7d-utilization": "0.41"}
	// closing is the delta that closes a streamed answer that called tools:
	// its usage gives no input of its own, as null, and counts it hasn't
	// before, one of a kind switchboard has never heard of.
	const closing = "event: message_delta\n" + `data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},` +
		`"usage":{"input_tokens":null,"output_tokens":845,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":512},` +
		`"server_tool_use":{"web_search_requests":1},"iterations_seen":7}}` + "\n\n"
	// closed is the usage an answer that closing closes ends with.
	closed := json.RawMessage(`{"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":512},"cache_creation_input_tokens":512,` +
		`"cache_read_input_tokens":40000,"input_tokens":3,"iterations_seen":7,"output_tokens":845,"server_tool_use":{"web_search_requests":1}}`)
	// fetched starts the block of a web fetch's result, given whole, as the
	// API gives one: a PDF, base64-encoded, in a line over 1 MiB.
	fetched := claudetest.BlockStarting(`{"type":"web_fetch_tool_result","tool_use_id":"srvtoolu_test","content":{"type":"web_fetch_result",` +
		`"url":"https://example.com/paper.pdf","content":{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"` +
		strings.Repeat("JVBERi0xLjcK", 100_000) + `"}}}}`)
	// fallback starts the block that marks where the API's server-side
	// fallback hands a streamed answer off from one model to the next.
	fallback := func(from, to string) string {
		return claudetest.BlockStarting(`{"type":"fallback","from":{"model":"` + from + `"},"to":{"model":"` + to + `"}}`)
	}
	tests := []struct {
		name       string
		header     http.Header
		body       string
		wantTokens *quota.Tokens
		want       ledger.Reply
	}{
		{
			name:   "a streamed answer that called tools",
			header: with(limited, "Content-Type", "text/event-stream"),
			body: claudetest.MessageStart + claudetest.ThinkingStart + claudetest.ThinkingDelta("Let me think.") + claudetest.BlockStop +
				claudetest.BlockStart("text", "") + claudetest.TextDelta("Looking.") + claudetest.BlockStart("tool_use", "Bash") +
				claudetest.InputDelta(`{"command":"ls"}`) + claudetest.BlockStart("server_tool_use", "web_search") + claudetest.BlockStart("tool_use", "Read") +
				closing + claudetest.MessageStop,
			wantTokens: &quota.Tokens{Input: 3, Output: 845, CacheRead: 40000, CacheWrite: 512},
			want: ledger.Reply{
				Answer: ledger.Answer{ID: "req_011CTest", Model: "claude-opus-5-5", Stop: "tool_use",
					Blocks: map[string]int{"thinking": 1, "text": 1, "tool_use": 2, "server_tool_use": 1}, Tools: []string{"Bash", "web_search", "Read"}},
				Usage:  closed,
				Limits: limits,
			},
		},
		{
			name:   "a streamed answer read on past a block too long to hold, given whole as it starts",
			header: with(limited, "Content-Type", "text/event-stream"),
			body: claudetest.MessageStart + claudetest.BlockStart("server_tool_use", "web_fetch") + claudetest.InputDelta(`{"url":"https://example.com/paper.pdf"}`) +
				fetched + claudetest.BlockStart("text", "") + claudetest.TextDelta("Read it.") + claudetest.BlockStart("tool_use", "Write") +
				closing + claudetest.MessageStop,
			wantTokens: &quota.Tokens{Input: 3, Output: 845, CacheRead: 40000, CacheWrite: 512},
			want: ledger.Reply{
				Answer: ledger.Answer{ID: "req_011CTest", Model: "claude-opus-5-5", Stop: "tool_use",
					Blocks: map[string]int{"server_tool_use": 1, "text": 1, "tool_use": 1}, Tools: []string{"web_fetch", "Write"}},
				Usage:  closed,
				Limits: limits,
			},
		},
		{
			name:   "a streamed answer that called a tool through the API's MCP connector, by the tool's name",
			header: typed("text/event-stream"),
			body: claudetest.MessageStart + claudetest.BlockStart("mcp_tool_use", "search_docs") + claudetest.InputDelta(`{"query":"limits"}`) +
				claudetest.BlockStart("mcp_tool_result", "") + claudetest.BlockStart("text", "") + claudetest.TextDelta("Found it.") + claudetest.MessageEnd,
			wantTokens: &claudetest.AnswerTokens,
			want: ledger.Reply{
				Answer: ledger.Answer{Model: "claude-opus-5-5", Stop: "end_turn",
					Blocks: map[string]int{"mcp_tool_use": 1, "mcp_tool_result": 1, "text": 1}, Tools: []string{"search_docs"}},
				Usage: claudetest.AnswerUsage,
			},
		},
		{
			name:   "a streamed answer the API fell back to other models on partway, as the last served it",
			header: typed("text/event-stream"),
			body: claudetest.MessageStart + claudetest.BlockStart("text", "") + claudetest.TextDelta("Sure, ") + claudetest.BlockStop +
				fallback("claude-opus-5-5", "claude-opus-5") + claudetest.BlockStop + claudetest.BlockStart("text", "") + claudetest.TextDelta("here ") +
				claudetest.BlockStop + fallback("claude-opus-5", "claude-sonnet-5-5") + claudetest.BlockStop + claudetest.BlockStart("text", "") +
				claudetest.TextDelta("it is.") + claudetest.MessageEnd,
			wantTokens: &claudetest.AnswerTokens,
			want: ledger.Reply{
				Answer: ledger.Answer{Model: "claude-sonnet-5-5", Stop: "end_turn", Blocks: map[string]int{"text": 3, "fallback": 2}},
				Usage:  claudetest.AnswerUsage,
			},
		},
		{
			name:   "a streamed answer whose model isn't text, read but for it",
			header: typed("text/event-stream"),
			body: swapped(t, claudetest.MessageStart, `"model":"claude-opus-5-5"`, `"model":{"id":"claude-opus-5-5"}`) +
				claudetest.BlockStart("text", "") + claudetest.TextDelta("Hi") + claudetest.MessageEnd,
			wantTokens: &claudetest.AnswerTokens,
			want:       ledger.Reply{Answer: ledger.Answer{Stop: "end_turn", Blocks: map[string]int{"text": 1}}, Usage: claudetest.AnswerUsage},
		},
		{
			name:   "a streamed answer whose blocks and stop reason aren't what the API gives, read but for them",
			header: typed("text/event-stream"),
			body: claudetest.MessageStart + claudetest.BlockStarting(`"text"`) +
				claudetest.BlockStarting(`{"type":"tool_use","id":"toolu_test","name":{"tool":"Read"},"input":{}}`) +
				claudetest.BlockStarting(`{"type":"fallback","from":{"model":"claude-opus-5-5"},"to":"claude-opus-5"}`) +
				swapped(t, claudetest.MessageDelta, `"stop_reason":"end_turn"`, `"stop_reason":{"type":"end_turn"}`),
			wantTokens: &claudetest.AnswerTokens,
			want: ledger.Reply{
				Answer: ledger.Answer{Model: "claude-opus-5-5", Blocks: map[string]int{"tool_use": 1, "fallback": 1}, Tools: []string{""}},
				Usage:  claudetest.AnswerUsage,
			},
		},
		{
			name:   "a message answered whole",
			header: with(limited, "Content-Type", "application/json"),
			body: `{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"Looking."},` +
				`{"type":"tool_use","id":"toolu_test","name":"Read","input":{"path":"notes.txt"}}],"model":"claude-opus-5-5","stop_reason":"tool_use",` +
				`"usage":{"input_tokens":3,"cache_read_input_tokens":40000,"output_tokens":120,"service_tier":"standard","iterations_seen":{"count":[1,2]}}}`,
			wantTokens: &quota.Tokens{Input: 3, Output: 120, CacheRead: 40000},
			want: ledger.Reply{
				Answer: ledger.Answer{ID: "req_011CTest", Model: "claude-opus-5-5", Stop: "tool_use", Blocks: map[string]int{"text": 1, "tool_use": 1}, Tools: []string{"Read"}},
				Usage:  json.RawMessage(`{"cache_read_input_tokens":40000,"input_tokens":3,"iterations_seen":{"count":[1,2]},"output_tokens":120,"service_tier":"standard"}`),
				Limits: limits,
			},
		},
		{
			name:   "a message answered whole whose model, stop reason and blocks aren't what the API gives, read but for them",
			header: typed("application/json"),
			body: `{"type":"message","model":{"id":"claude-opus-5-5"},"content":["Hello",{"type":"tool_use","id":"toolu_test","name":{"tool":"Read"},"input":{}},` +
				`{"type":"text","text":"Hi"}],"stop_reason":{"type":"end_turn"},"usage":{"input_tokens":3,"output_tokens":120}}`,
			wantTokens: &quota.Tokens{Input: 3, Output: 120},
			want: ledger.Reply{
				Answer: ledger.Answer{Blocks: map[string]int{"tool_use": 1, "text": 1}, Tools: []string{""}},
				Usage:  json.RawMessage(`{"input_tokens":3,"output_tokens":120}`),
			},
		},
		{
			name:       "a message answered whole whose content isn't a list, read but for it",
			header:     typed("application/json"),
			body:       swapped(t, claudetest.Message, `"content":[{"type":"text","text":"Hello, world"}]`, `"content":"Hello, world"`),
			wantTokens: &claudetest.AnswerTokens,
			want:       ledger.Reply{Answer: ledger.Answer{Model: "claude-opus-5-5", Stop: "end_turn"}, Usage: claudetest.AnswerUsage},
		},
		{
			name:   "an error answered whole",
			header: typed("application/json"),
			body:   `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
			want:   ledger.Reply{Answer: ledger.Answer{Error: ledger.Error{Type: "overloaded_error", Message: "Overloaded"}}},
		},
		{
			name:   "a streamed answer that ends in an error",
			header: typed("text/event-stream"),
			body:   claudetest.MessageStart + claudetest.BlockStart("text", "") + claudetest.TextDelta("Hello") + claudetest.Overloaded,
			want: ledger.Reply{Answer: ledger.Answer{Model: "claude-opus-5-5", Blocks: map[string]int{"text": 1},
				Error: ledger.Error{Type: "overloaded_error", Message: "Overloaded"}}},
		},
		{
			name:   "a streamed answer cut short, before its closing usage",
			header: typed("text/event-stream"),
			body:   claudetest.MessageStart + claudetest.BlockStart("text", "") + claudetest.TextDelta("Hello"),
			want:   ledger.Reply{Answer: ledger.Answer{Model: "claude-opus-5-5", Blocks: map[string]int{"text": 1}}},
		},
		{
			name:   "a count of tokens, which gives no usage",
			header: with(limited, "Content-Type", "application/json"),
			body:   `{"input_tokens":1234}`,
			want:   ledger.Reply{Answer: ledger.Answer{ID: "req_011CTest"}, Limits: limits},
		},
		{
			name:   "an answer of another type, its header alone read",
			header: with(limited, "Content-Type", "text/html"),
			body:   `<html>Bad gateway</html>`,
			want:   ledger.Reply{Answer: ledger.Answer{ID: "req_011CTest"}, Limits: limits},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, got := (claude.Provider{}).Count(tt.header, strings.NewReader(tt.body), func(int) {})
			if !reflect.DeepEqual(tokens, tt.wantTokens) {
				t.Errorf("Count() tokens = %+v, want %+v", tokens, tt.wantTokens)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Count() reads the answer as\n%+v\nwant\n%+v\nits usage %s, want %s", got, tt.want, got.Usage, tt.want.Usage)
			}
		})
	}
}

// gunzipped returns a reader of text as the router decodes an answer it asked
// for in gzip: compressed, then decompressed, its last data coming with its
// end.
func gunzipped(t *testing.T, text string) io.Reader {
	t.Helper()
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	if _, err := io.WriteString(w, text); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	decoded, err := gzip.NewReader(&compressed)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

// swapped is s with old swapped for with, failing t where s doesn't hold old,
// so a fixture's change can't leave a test swapping nothing.
func swapped(t *testing.T, s, old, with string) string {
	t.Helper()
	if !strings.Contains(s, old) {
		t.Fatalf("%q holds no %q to swap", s, old)
	}
	return strings.Replace(s, old, with, 1)
}

// textOfLine is the text whose delta, as claudetest.TextDelta streams it, has
// a data line of size bytes, its line ending included.
func textOfLine(size int) string {
	none := strings.TrimPrefix(claudetest.TextDelta(""), "event: content_block_delta\n")
	return strings.Repeat("x", size-len(strings.TrimSuffix(none, "\n")))
}

// typed is the header of an answer of the content type given.
func typed(contentType string) http.Header {
	return http.Header{"Content-Type": {contentType}}
}

// with returns h with the header name set to value.
func with(h http.Header, name, value string) http.Header {
	h = h.Clone()
	h.Set(name, value)
	return h
}
