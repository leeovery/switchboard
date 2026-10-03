package claude_test

import (
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
)

// The events of a streamed answer, as the Messages API streams one.
const (
	messageStart = "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"claude-opus-5-5","stop_reason":null,"usage":{"input_tokens":3,"cache_creation_input_tokens":512,"cache_read_input_tokens":40000,"output_tokens":1}}}` + "\n\n"
	thinkingStart = "event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}` + "\n\n"
	signature = "event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"c2lnbmVk"}}` + "\n\n"
	blockStop = "event: content_block_stop\n" + `data: {"type":"content_block_stop","index":0}` + "\n\n"
	toolInput = "event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"path\": \"a long path\""}}` + "\n\n"
	ping         = "event: ping\n" + `data: {"type": "ping"}` + "\n\n"
	messageDelta = "event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":120}}` + "\n\n"
	messageStop = "event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n"
	overloaded  = "event: error\n" + `data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}` + "\n\n"
)

// counted are the tokens messageStart and messageDelta give together.
var counted = quota.Tokens{Input: 3, Output: 120, CacheRead: 40000, CacheWrite: 512}

// textDelta is the event that adds text to a block.
func textDelta(text string) string {
	return "event: content_block_delta\n" + `data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"` + text + `"}}` + "\n\n"
}

// thinkingDelta is the event that adds thinking to a block.
func thinkingDelta(thinking string) string {
	return "event: content_block_delta\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"` + thinking + `"}}` + "\n\n"
}

// inputDelta is the event that adds a piece of the JSON of a tool's input to
// a block.
func inputDelta(partialJSON string) string {
	quoted, _ := json.Marshal(partialJSON)
	return "event: content_block_delta\n" + `data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":` + string(quoted) + `}}` + "\n\n"
}

func TestProviderCountsAStreamedAnswer(t *testing.T) {
	tests := []struct {
		name   string
		events []string
		// wantChars are the counts chars is called with, in turn.
		wantChars  []int
		wantTokens quota.Tokens
		wantOK     bool
	}{
		{
			name: "its thinking, text and tool's input as each comes, and its closing usage",
			events: []string{messageStart, thinkingStart, thinkingDelta("Let me think."), signature, blockStop,
				textDelta("Hello, "), textDelta("world ☃"), toolInput, ping, messageDelta, messageStop},
			wantChars:  []int{13, 20, 27, 49},
			wantTokens: counted,
			wantOK:     true,
		},
		{
			name:       "a tool's input, a piece of its JSON at a time, as the pieces read decoded",
			events:     []string{messageStart, inputDelta(`{"text": "café`), inputDelta(` au lait\n"}`), messageDelta},
			wantChars:  []int{14, 26},
			wantTokens: counted,
			wantOK:     true,
		},
		{
			name: "a closing usage that gives every count itself",
			events: []string{messageStart, textDelta("Hi"),
				"event: message_delta\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":9,"cache_creation_input_tokens":0,"cache_read_input_tokens":41000,"output_tokens":7}}` + "\n\n",
				messageStop},
			wantChars:  []int{2},
			wantTokens: quota.Tokens{Input: 9, Output: 7, CacheRead: 41000},
			wantOK:     true,
		},
		{
			name:      "an answer cut short, before its closing usage",
			events:    []string{messageStart, textDelta("Hello")},
			wantChars: []int{5},
		},
		{
			name:      "an answer that ends in an error",
			events:    []string{messageStart, textDelta("Hello"), overloaded},
			wantChars: []int{5},
		},
		{
			name: "lines it can't read passed over",
			events: []string{"data: not json\n\n", ": a comment\n\n", "event: content_block_delta\ndata: {\"type\":\"content_block_delta\"\n\n",
				messageStart, "data:" + strings.TrimPrefix(strings.TrimPrefix(textDelta("tight"), "event: content_block_delta\n"), "data: "), messageDelta},
			wantChars:  []int{5},
			wantTokens: counted,
			wantOK:     true,
		},
		{
			name:       "lines that end in CRLF",
			events:     []string{strings.ReplaceAll(messageStart+textDelta("crlf")+messageDelta, "\n", "\r\n")},
			wantChars:  []int{4},
			wantTokens: counted,
			wantOK:     true,
		},
		{
			name:   "a line too long to count, after which nothing is",
			events: []string{messageStart, "data: " + strings.Repeat("x", 1<<20) + "\n\n", textDelta("late"), messageDelta},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var told []int
			body := iotest.HalfReader(strings.NewReader(strings.Join(tt.events, "")))
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
			body:        `{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"Hi"}],"usage":{"input_tokens":3,"cache_creation_input_tokens":512,"cache_read_input_tokens":40000,"output_tokens":120}}`,
			wantTokens:  counted,
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

func TestProviderCountsAsFarAsItCanReadAnAnswer(t *testing.T) {
	broken := io.MultiReader(strings.NewReader(messageStart+textDelta("Hello")), iotest.ErrReader(io.ErrUnexpectedEOF))
	var told []int
	tokens, ok := (claude.Provider{}).Count("text/event-stream", broken, func(n int) { told = append(told, n) })
	if ok || tokens != (quota.Tokens{}) || !slices.Equal(told, []int{5}) {
		t.Errorf("Count() = %+v, %v, telling chars %v, want no tokens, having told of the 5 characters read", tokens, ok, told)
	}
}
