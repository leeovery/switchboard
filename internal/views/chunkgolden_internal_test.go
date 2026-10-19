package views

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// goldenRows are lines of a day, written out in full so no change to a
// generator moves them, so the golden of a chunk of their records holds a
// value of every key of a stored row and of a chunk's meta: a subagent's
// request of session s, moved off the account that hit its limit, with all
// a router's line says, arriving to the nanosecond, its usage writing to the
// cache for five minutes and for an hour, reading it and searching the web,
// its answer reading its account's windows near its limit; a quota check
// and a count of tokens of the same session; a request of session o without
// an id, cut off as it went upstream, so it gave no usage; and its next, of
// a model no price knows, its client gone and its answer an overload, its
// usage counting its input under a name in capitals, a server tool no price
// knows, and writes to the cache named beside those it breaks down.
func goldenRows() []ledger.Line {
	at := func(hour, minute int) time.Time {
		return time.Date(2026, 10, 8, hour, minute, 0, 0, time.UTC)
	}
	subagent := ledger.Line{
		At: at(9, 0).Add(123_456_789 * time.Nanosecond), Request: "r1", Kind: ledger.KindMessage, Session: "s", Dir: "/code/app", Model: foldOpus,
		Account: "side", Reason: "moved: work hit its limit", From: "work", Tried: []ledger.Tried{{Account: "work", Why: "hit its limit"}}, Status: 200,
		Attempts: 2, FirstMS: new(int64(800)), TotalMS: 11_800, Agent: "claude-cli/2.1.300 (external, cli)",
		Betas: []string{"claude-code-20250219", "context-1m-2025-08-07"}, Prompt: "p1", Class: "subagent", AgentID: "a1",
		Shape: ledger.Shape{
			Bytes: 482_000, Messages: 12, System: 3, Tools: 9, MaxTokens: new(int64(32_000)), Thinking: ledger.Thinking{BudgetTokens: new(int64(16_000))},
			Stream: new(true), OutputConfig: ledger.OutputConfig{Effort: "high"}, InferenceGeo: "us",
		},
		Answer: ledger.Answer{
			ID: "msg_01", Model: foldOpus, Stop: "tool_use", Tools: []string{"Read", "Edit"}, Blocks: map[string]int{"thinking": 1, "text": 1, "tool_use": 2},
		},
		Usage: json.RawMessage(`{"input_tokens":10,"cache_creation_input_tokens":3120,"cache_read_input_tokens":90000,` +
			`"cache_creation":{"ephemeral_5m_input_tokens":120,"ephemeral_1h_input_tokens":3000},"output_tokens":300,"server_tool_use":{"web_search_requests":2}}`),
		Limits: map[string]string{
			"status": "allowed_warning", "5h-utilization": "0.92", "5h-reset": "1791378000", "5h-status": "allowed_warning", "7d-utilization": "0.4",
		},
	}
	check := ledger.Line{
		At: at(9, 1), Request: "r2", Kind: ledger.KindCheck, Session: "s", Model: foldHaiku, Account: "side", Reason: "sticky", Status: 200, Attempts: 1,
		TotalMS: 300, Usage: json.RawMessage(`{"input_tokens":8,"output_tokens":1}`),
	}
	count := ledger.Line{At: at(9, 2), Request: "r3", Kind: ledger.KindCount, Session: "s", Model: foldOpus, Account: "side", Reason: "sticky", Status: 200,
		Attempts: 1, TotalMS: 90}
	cut := ledger.Line{At: at(11, 0), Kind: ledger.KindMessage, Session: "o", Model: foldSonnet, Account: "work", Reason: "new", CutOff: true, Attempts: 1,
		TotalMS: 4000, Class: ledger.ClassMain}
	overloaded := ledger.Line{
		At: at(11, 5), Request: "o2", Kind: ledger.KindMessage, Session: "o", Model: unpricedModel, Account: "work", Reason: "sticky", Status: 529,
		Canceled: true, Attempts: 3, TotalMS: 9000, Class: ledger.ClassMain,
		Usage: json.RawMessage(`{"Input_Tokens":12,"output_tokens":10,"cache_read_input_tokens":40,"cache_creation_input_tokens":100,` +
			`"cache_creation.ephemeral_5m_input_tokens":40,"cache_creation":{"ephemeral_1h_input_tokens":60},"cache_creation.unknown":3,` +
			`"server_tool_use":{"code_execution_requests":1}}`),
	}
	return []ledger.Line{subagent, check, count, cut, overloaded}
}

// chunkGolden is the golden of a chunk of a day's rows: each of its rows,
// the JSON of a line of them, in order; and its meta's.
type chunkGolden struct {
	Rows []json.RawMessage `json:"rows"`
	Meta json.RawMessage   `json:"meta"`
}

// The chunk of the records of goldenRows' lines, as rowsChunk encodes it,
// is what the golden of the version of the days' rows it's written in holds,
// as goldenChunk has it: testdata/rows-chunk-v<RowsVersion>.json. A change
// to how a row or a chunk's meta is stored, or to the working behind them,
// that changes it without a new version fails, as rows already written would
// be read as they no longer are. A new version has its golden written with
// -update, in place of the one before, which is removed; one already kept
// isn't written again. A chunk is the views' alone, in no envelope of the
// ledger's.
func TestAChunkIsEncodedAsTheGoldenOfItsVersionHoldsIt(t *testing.T) {
	keptGolden{
		file: "rows-chunk", what: "a chunk of a day's rows", version: RowsVersion, test: "TestAChunkIsEncodedAsTheGoldenOfItsVersionHoldsIt",
		envelope: func(*testing.T, []byte) int { return 0 },
		bump:     "RowsVersion where how a row or a chunk's meta is stored, or the working behind them, changed",
	}.check(t, goldenChunk(t))
}

// goldenChunk is the chunk of the records of goldenRows' lines, as it's
// built, as chunkGolden has it, indented, ending its last line.
func goldenChunk(t *testing.T) []byte {
	t.Helper()
	built, _ := encodedChunk(t, recordsOfLines(goldenRows()))
	g := chunkGolden{Meta: built.meta}
	for line := range bytes.Lines(built.rows) {
		g.Rows = append(g.Rows, bytes.TrimSuffix(line, []byte{'\n'}))
	}
	plain, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	return indented(t, plain)
}
