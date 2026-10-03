package router

import (
	"encoding/json"
	"strings"

	"github.com/leeovery/switchboard/internal/quota"
)

// The events a streamed answer starts and ends with, as the Messages API
// streams one, for the tests of the request stream, inside the package and
// out: MessageStart with the usage the answer starts with, and MessageEnd
// with its closing usage, which give AnswerTokens together.
const (
	MessageStart = "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"claude-opus-5-5","usage":{"input_tokens":3,"cache_creation_input_tokens":512,"cache_read_input_tokens":40000,"output_tokens":1}}}` + "\n\n"
	MessageEnd = "event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":120}}` + "\n\n" +
		"event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n"
)

// AnswerTokens are the tokens MessageStart and MessageEnd give together.
var AnswerTokens = quota.Tokens{Input: 3, Output: 120, CacheRead: 40000, CacheWrite: 512}

// The thinking and the tool's input of the answer AnswerPieces streams.
const (
	answerThinking = "Let me think."
	answerInput    = `{"path": "notes.txt", "content": "Hello"}`
)

// TextDelta is the event that adds text to a streamed answer.
func TextDelta(text string) string {
	return "event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"` + text + `"}}` + "\n\n"
}

// ThinkingDelta is the event that adds thinking to a streamed answer.
func ThinkingDelta(thinking string) string {
	return "event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"` + thinking + `"}}` + "\n\n"
}

// InputDelta is the event that adds a piece of the JSON of a tool's input to
// a streamed answer.
func InputDelta(partialJSON string) string {
	quoted, _ := json.Marshal(partialJSON)
	return "event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":` + string(quoted) + `}}` + "\n\n"
}

// AnswerPieces are the pieces of a streamed answer, an event each: its
// thinking, the texts given, and a tool's input, in two pieces, which stream
// AnswerChars characters, closing with AnswerTokens.
func AnswerPieces(texts ...string) []string {
	pieces := []string{MessageStart, ThinkingDelta(answerThinking)}
	for _, text := range texts {
		pieces = append(pieces, TextDelta(text))
	}
	half := len(answerInput) / 2
	return append(pieces, InputDelta(answerInput[:half]), InputDelta(answerInput[half:]), MessageEnd)
}

// AnswerChars is how many characters of thinking, text and a tool's input
// AnswerPieces stream with the texts given.
func AnswerChars(texts ...string) int {
	return len([]rune(answerThinking + strings.Join(texts, "") + answerInput))
}
