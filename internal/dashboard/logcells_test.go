package dashboard

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// The inks the Requests' cells no frame holds are held to.
var (
	mutedCell     = ink{token: theme.TextMuted}
	faintCell     = ink{token: theme.TextFaint}
	secondaryCell = ink{token: theme.TextSecondary}
)

// columnCell is a cell of a request's row drawn: what the column holds, and the
// ink its first cell is drawn in, as a test holds the row to it.
type columnCell struct {
	text string
	ink  ink
}

// drawnCells are what the columns of layout hold along row y of the canvas
// c, by the column: each its cells' text, trimmed, and the ink of the first
// of them drawn, the zero ink where none is.
func drawnCells(c *canvas, y int, layout logLayout) map[LogColumn]columnCell {
	cells := make(map[LogColumn]columnCell, len(layout.places))
	for _, p := range layout.places {
		var text strings.Builder
		var first ink
		for x := p.x; x < p.x+p.width; x++ {
			at := c.at(x, y)
			if at == nil || at.glyph == "" {
				continue
			}
			if at.glyph != " " && first == (ink{}) {
				first = at.ink
			}
			text.WriteString(at.glyph)
		}
		cells[p.column] = columnCell{strings.TrimSpace(text.String()), first}
	}
	return cells
}

// everyExtra are the Requests' columns as they open, with every extra f
// shows ticked.
func everyExtra() LogTicks {
	var ticks LogTicks
	for _, c := range EveryLogColumn() {
		if c.extra() {
			ticks = ticks.Toggled(c)
		}
	}
	return ticks
}

// rowsHold holds the rows of the Requests drawn on the canvas c from row top,
// laid out as layout, to want, a row each: each cell want names, its text and
// its ink.
func rowsHold(t *testing.T, c *canvas, top int, layout logLayout, want []map[LogColumn]columnCell) {
	t.Helper()
	for i, cells := range want {
		drawn := drawnCells(c, top+i, layout)
		for column, w := range cells {
			if got := drawn[column]; got != w {
				t.Errorf("row %d's %s reads %q in %+v, want %q in %+v", i+1, column.Name(), got.text, got.ink, w.text, w.ink)
			}
		}
	}
}

// The extras f shows, each in text.muted: the tokens it took in uncached;
// how many times it went upstream; its size, in KB; its messages and its
// tools; its budget for thinking, as tokens; its agent, cut to its name and
// version; and its id. And its worth at API prices: to the cent, under half
// a cent <$0.01, and + where part of it is unpriced, as a request whose
// answer gave no usage.
func TestTheRequestsDrawTheExtrasAndWorthAsTheDesignSays(t *testing.T) {
	at := time.Date(2026, 10, 1, 14, 31, 13, 0, now.Location())
	budget := int64(16000)
	full := asked("0a000001", at)
	full.Usage, full.Attempts, full.Agent = json.RawMessage(aTurnsUsage), 2, "claude-cli/2.1.0 (external, cli)"
	full.Shape = ledger.Shape{Bytes: 482 * 1024, Messages: 40, Tools: 12, Thinking: ledger.Thinking{Type: "enabled", BudgetTokens: &budget}}
	tiny := asked("0a000002", at.Add(time.Second))
	tiny.Model, tiny.Usage = "claude-haiku-4-5", json.RawMessage(`{"input_tokens":3,"output_tokens":1}`)
	unmetered := asked("0a000003", at.Add(2*time.Second))
	p := requestsOf(views.LogSources{From: at, Now: at.Add(time.Minute)}, []ledger.Line{full, tiny, unmetered})
	p.Columns = everyExtra()

	f := logFrameOf(p, 300, 24)
	layout := logColumns(f.Width, p.Columns)
	rowsHold(t, f.page(routerDoc("side", 0), at.Add(time.Minute)), f.top()+1, layout, []map[LogColumn]columnCell{
		{
			LogWorth: {"$0.07", mutedCell}, LogInput: {"3", mutedCell}, LogAttempts: {"2", mutedCell}, LogSize: {"482 KB", mutedCell},
			LogMessages: {"40", mutedCell}, LogTools: {"12", mutedCell}, LogThinking: {"16k", mutedCell},
			LogAgent: {"claude-cli/2.1.0", mutedCell}, LogRequestID: {"0a000001", mutedCell},
		},
		{LogWorth: {"<$0.01", mutedCell}},
		{LogWorth: {"$0.00+", mutedCell}},
	})
}

// A request that failed with no answer, cut off as the router stopped or
// given up as its client went away, reads as failed: ■, and its status, 0,
// bold state.destructive. One the stream told of in flight that ended
// untold is lost: ■, no answer: the router went away, nothing of its answer
// known. One of no session is — in text.faint; one whose line names no
// directory, its session's id alone.
func TestTheRequestsDrawEachFailureAsTheDesignSays(t *testing.T) {
	at := time.Date(2026, 10, 1, 14, 31, 13, 0, now.Location())
	cut := asked("0a000001", at)
	cut.Status, cut.CutOff = 0, true
	gaveUp := asked("0a000002", at.Add(time.Second))
	gaveUp.Status, gaveUp.Canceled = 0, true
	sessionless := asked("0a000003", at.Add(2*time.Second))
	sessionless.Session = ""
	lost := views.Told{Request: "0a000004", Session: cut.Session, Model: cut.Model, Account: "side", Sent: at.Add(3 * time.Second), Heard: at.Add(3 * time.Second), Lost: true}
	stream := views.Stream{Live: true, Requests: map[string]views.Told{lost.Request: lost}}
	p := requestsOf(views.LogSources{From: at, Now: at.Add(time.Minute), Stream: stream}, []ledger.Line{cut, gaveUp, sessionless})
	p.Stream = stream

	f := logFrameOf(p, 200, 24)
	c := f.page(routerDoc("side", 0), at.Add(time.Minute))
	failed := columnCell{"0", ink{token: theme.StateDestructive, bold: true}}
	rowsHold(t, c, f.top()+1, logColumns(f.Width, p.Columns), []map[LogColumn]columnCell{
		{LogStatus: failed, LogSession: {"d28c", secondaryCell}},
		{LogStatus: failed},
		{LogSession: {"—", faintCell}, LogStatus: {"200", mutedCell}},
		{LogStatus: {}, LogTook: {}, LogStopped: {"no answer: the router went away", mutedCell}},
	})
	for i, want := range []string{"■", "■", "", "■"} {
		if mark := c.at(0, f.top()+1+i); strings.TrimSpace(mark.glyph) != want || want != "" && mark.ink != (ink{token: theme.StateDestructive, bold: true}) {
			t.Errorf("row %d is marked %q in %+v, want %q", i+1, mark.glyph, mark.ink, want)
		}
	}
}

// Claude Code's quota checks and its counts of tokens, shown: each marked ·,
// bold text.faint; stopped blank, whatever its answer said; and its tokens
// as they are.
func TestChecksAndCountsShownAreMarkedAndCountedAsTheyAre(t *testing.T) {
	at := time.Date(2026, 10, 1, 14, 31, 13, 0, now.Location())
	check := asked("0a000001", at)
	check.Kind, check.Usage, check.Answer = ledger.KindCheck, json.RawMessage(`{"input_tokens":8,"output_tokens":1}`), ledger.Answer{Stop: "max_tokens"}
	count := asked("0a000002", at.Add(time.Second))
	count.Kind, count.Usage = ledger.KindCount, json.RawMessage(`{"input_tokens":5400}`)
	p := requestsOf(views.LogSources{From: at, Now: at.Add(time.Minute)}, []ledger.Line{check, count})
	p.Columns = LogTicks{}.Toggled(LogInput)

	f := logFrameOf(p, 160, 24)
	c := f.page(routerDoc("side", 0), at.Add(time.Minute))
	none := columnCell{"—", faintCell}
	rowsHold(t, c, f.top()+1, logColumns(f.Width, p.Columns), []map[LogColumn]columnCell{
		{LogRead: none, LogWritten: none, LogOut: {"1", mutedCell}, LogInput: {"8", mutedCell}, LogStopped: {}},
		{LogRead: none, LogWritten: none, LogOut: none, LogInput: {"5.4k", mutedCell}, LogStopped: {}},
	})
	for i := range 2 {
		if mark := c.at(0, f.top()+1+i); mark.glyph != "·" || mark.ink != (ink{token: theme.TextFaint, bold: true}) {
			t.Errorf("row %d is marked %q in %+v, want · bold in text.faint", i+1, mark.glyph, mark.ink)
		}
	}
}

// On a phone, stopped says the tool a request called alone, in
// text.secondary, for tool_use; else why its answer stopped, in text.muted.
func TestAPhonesStoppedNamesTheToolAlone(t *testing.T) {
	at := time.Date(2026, 10, 1, 14, 31, 13, 0, now.Location())
	first := asked("0a000001", at)
	tool := asked("0a000002", at.Add(time.Second))
	tool.Answer = ledger.Answer{Stop: "tool_use", Tools: []string{"Bash"}}
	ended := asked("0a000003", at.Add(2*time.Second))
	ended.Answer = ledger.Answer{Stop: "end_turn"}
	p := requestsOf(views.LogSources{From: at, Now: at.Add(time.Minute)}, []ledger.Line{first, tool, ended})

	f := logFrameOf(p, 52, 33)
	rowsHold(t, f.page(routerDoc("side", 0), at.Add(time.Minute)), f.top()+2, logColumns(f.Width, p.Columns), []map[LogColumn]columnCell{
		{LogStopped: {"Bash", secondaryCell}},
		{LogStopped: {"end_turn", mutedCell}},
	})
}
