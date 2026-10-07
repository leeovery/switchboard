package claudetest

import (
	"encoding/json"
	"strings"

	"github.com/leeovery/switchboard/internal/quota"
)

// The events of a streamed answer, as the Messages API streams one: it starts
// with MessageStart, giving the usage the answer starts with, and ends with
// MessageEnd, MessageDelta's closing usage and MessageStop, which give
// AnswerTokens together.
const (
	MessageStart = "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"claude-opus-5-5","stop_reason":null,"usage":{"input_tokens":3,"cache_creation_input_tokens":512,"cache_read_input_tokens":40000,"output_tokens":1}}}` + "\n\n"
	ThinkingStart = "event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}` + "\n\n"
	Signature = "event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"c2lnbmVk"}}` + "\n\n"
	BlockStop    = "event: content_block_stop\n" + `data: {"type":"content_block_stop","index":0}` + "\n\n"
	Ping         = "event: ping\n" + `data: {"type": "ping"}` + "\n\n"
	MessageDelta = "event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":120}}` + "\n\n"
	MessageStop = "event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n"
	MessageEnd  = MessageDelta + MessageStop
	// Overloaded is the error an answer can end in partway.
	Overloaded = "event: error\n" + `data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}` + "\n\n"
)

// Message is an answer the Messages API gives whole, not streamed: a message,
// whose usage gives AnswerTokens.
const Message = `{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"Hello, world"}],"model":"claude-opus-5-5","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":3,"cache_creation_input_tokens":512,"cache_read_input_tokens":40000,"output_tokens":120}}`

// AnswerTokens are the tokens MessageStart and MessageDelta give together, and
// Message gives.
var AnswerTokens = quota.Tokens{Input: 3, Output: 120, CacheRead: 40000, CacheWrite: 512}

// AnswerUsage is the usage MessageStart and MessageDelta give together, and
// Message gives, as the request ledger keeps it: field for field, in the
// order of the fields' names.
var AnswerUsage = json.RawMessage(`{"cache_creation_input_tokens":512,"cache_read_input_tokens":40000,"input_tokens":3,"output_tokens":120}`)

// The thinking and the tool's input of the answer AnswerPieces streams.
const (
	answerThinking = "Let me think."
	answerInput    = `{"path": "notes.txt", "content": "Hello"}`
)

// BlockStart is the event that starts a block of a streamed answer, of the
// kind given, calling the tool named, where it's a call of one. The kind and
// the tool's name go in as they are, so they mustn't need escaping.
func BlockStart(kind, tool string) string {
	block := `{"type":"` + kind + `"`
	if tool != "" {
		block += `,"id":"toolu_test","name":"` + tool + `","input":{}`
	}
	return BlockStarting(block + "}")
}

// BlockStarting is the event that starts a block of a streamed answer, the
// block given as its JSON, as it is.
func BlockStarting(block string) string {
	return "event: content_block_start\n" + `data: {"type":"content_block_start","index":0,"content_block":` + block + "}\n\n"
}

// TextDelta is the event that adds text to a streamed answer. The text goes
// in as it is, so it mustn't need escaping.
func TextDelta(text string) string {
	return "event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"` + text + `"}}` + "\n\n"
}

// ThinkingDelta is the event that adds thinking to a streamed answer. The
// thinking goes in as it is, so it mustn't need escaping.
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
