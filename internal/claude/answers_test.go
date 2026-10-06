package claude_test

import (
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/claude/claudetest"
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
		wantTokens quota.Tokens
		wantOK     bool
	}{
		{
			name: "its thinking, text and tool's input as each comes, and its closing usage",
			events: []string{start, claudetest.ThinkingStart, claudetest.ThinkingDelta("Let me think."), claudetest.Signature, claudetest.BlockStop,
				text("Hello, "), text("world ☃"), claudetest.InputDelta(`{"path": "a long path"`), claudetest.Ping, usage, stop},
			wantChars:  []int{13, 20, 27, 49},
			wantTokens: claudetest.AnswerTokens,
			wantOK:     true,
		},
		{
			name:       "a tool's input, a piece of its JSON at a time, as the pieces read decoded",
			events:     []string{start, claudetest.InputDelta(`{"text": "café`), claudetest.InputDelta(` au lait\n"}`), usage},
			wantChars:  []int{14, 26},
			wantTokens: claudetest.AnswerTokens,
			wantOK:     true,
		},
		{
			name: "a closing usage that gives every count itself",
			events: []string{start, text("Hi"),
				"event: message_delta\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":9,"cache_creation_input_tokens":0,"cache_read_input_tokens":41000,"output_tokens":7}}` + "\n\n",
				stop},
			wantChars:  []int{2},
			wantTokens: quota.Tokens{Input: 9, Output: 7, CacheRead: 41000},
			wantOK:     true,
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
			wantTokens: claudetest.AnswerTokens,
			wantOK:     true,
		},
		{
			name:       "lines that end in CRLF",
			events:     []string{strings.ReplaceAll(start+text("crlf")+usage, "\n", "\r\n")},
			wantChars:  []int{4},
			wantTokens: claudetest.AnswerTokens,
			wantOK:     true,
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
			tokens, ok := (claude.Provider{}).Count("text/event-stream; charset=utf-8", body, func(n int) { told = append(told, n) })
			if tokens != tt.wantTokens || ok != tt.wantOK {
				t.Errorf("Count() = %+v, %v, want %+v, %v", tokens, ok, tt.wantTokens, tt.wantOK)
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
		wantTokens  quota.Tokens
		wantOK      bool
	}{
		{
			name:        "a message",
			contentType: "application/json",
			body:        claudetest.Message,
			wantTokens:  claudetest.AnswerTokens,
			wantOK:      true,
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
			tokens, ok := (claude.Provider{}).Count(tt.contentType, strings.NewReader(tt.body), func(n int) {
				t.Errorf("chars told %d, want nothing told of an answer that doesn't stream", n)
			})
			if tokens != tt.wantTokens || ok != tt.wantOK {
				t.Errorf("Count() = %+v, %v, want %+v, %v", tokens, ok, tt.wantTokens, tt.wantOK)
			}
		})
	}
}
