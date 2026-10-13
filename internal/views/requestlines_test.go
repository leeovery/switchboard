package views_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

func TestARequestsLineIsItsRowAsItsLineAloneTellsItWithTheTokensItTookIn(t *testing.T) {
	now := time.Date(2026, 10, 7, 13, 12, 0, 0, time.Local)
	first := ledger.Line{At: now.Add(-time.Hour), Request: "r1", Kind: ledger.KindMessage, Session: "s1", Model: "claude-opus-5-5", Account: "work",
		Reason: "new", Status: 200, TotalMS: 14230,
		Usage: json.RawMessage(`{"input_tokens":12,"cache_creation_input_tokens":3120,"cache_read_input_tokens":182340,"output_tokens":845}`)}
	moved := ledger.Line{At: now.Add(-time.Minute), Request: "r2", Kind: ledger.KindMessage, Session: "s1", Model: "claude-opus-5-5", Account: "side",
		Reason: "moved: work hit its limit", From: "work", Status: 200, Canceled: true, TotalMS: 2000}
	var rows []views.RequestLine
	for r := range views.RequestLines(slices.Values([]ledger.Held{{Line: first}, {Line: moved}})) {
		rows = append(rows, r)
	}
	if len(rows) != 2 {
		t.Fatalf("RequestLines gave %d rows, want one a line", len(rows))
	}

	r := rows[0]
	if r.In != 12+3120+182340 || r.Out != 845 || !r.Metered || r.Request != "r1" || r.Session != "s1" || r.Status != 200 || r.Took == nil || *r.Took != 14230*time.Millisecond {
		t.Errorf("the first line's row is %+v, want its tokens in, its input and the cache's read and written, and the rest as its line has them", r)
	}
	if r.NewSession || r.Mark != views.NoMark || r.Worth.Amount != nil || r.Worth.Unpriced != nil {
		t.Errorf("the first line's row is %+v, want no story told, so not its session's first, and no worth worked out", r)
	}

	r = rows[1]
	if r.Metered || r.In != 0 || !r.Canceled || r.Mark != views.MarkMoved || wordsSaid(r.Routing.Words) != "from [work], at its limit" || r.Routing.Cost != nil {
		t.Errorf("the move's row is %+v, want no usage, canceled, and its move from its line alone, its cost untold", r)
	}
}
