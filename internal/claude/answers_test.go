package claude_test

import (
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
			name:   "a line too long to count, after which nothing is",
			events: []string{start, "data: " + strings.Repeat("x", 1<<20) + "\n\n", text("late"), usage},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var told []int
			body := iotest.HalfReader(strings.NewReader(strings.Join(tt.events, "")))
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
				Usage: json.RawMessage(`{"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":512},"cache_creation_input_tokens":512,` +
					`"cache_read_input_tokens":40000,"input_tokens":3,"iterations_seen":7,"output_tokens":845,"server_tool_use":{"web_search_requests":1}}`),
				Limits: limits,
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
