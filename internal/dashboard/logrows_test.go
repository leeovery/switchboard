package dashboard

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// The usages of the rows of each kind's answers: a turn's, reading its
// context from the cache; and a move's, writing a million tokens of it again
// for an hour, worth $8 of Claude Opus 5.5.
const (
	aTurnsUsage = `{"input_tokens":3,"cache_read_input_tokens":180000,"cache_creation_input_tokens":2000,"cache_creation":{"ephemeral_1h_input_tokens":2000},"output_tokens":900}`
	aMovesUsage = `{"input_tokens":3,"cache_creation_input_tokens":1000000,"cache_creation":{"ephemeral_1h_input_tokens":1000000},"output_tokens":1200}`
)

// rowsOfEachKind are the lines and the stream of d28c's requests of each
// kind the Requests draw, at 14:42:00 on 1 October: its first, on work, at
// 14:30:00; a move to side after work's limit and then personal's, writing
// its context again, at 14:35:00; a 429 every account refused, passed on, at
// 14:36:00; and two in flight, answering since 14:41:40 and asking since
// 14:41:56.
func rowsOfEachKind() ([]ledger.Line, views.Stream, time.Time) {
	opened := time.Date(2026, 10, 1, 14, 30, 0, 0, now.Location())
	at := opened.Add(12 * time.Minute)
	first := asked("0a000001", opened)
	first.Account, first.Reason, first.Usage = "work", status.ReasonNew, json.RawMessage(aTurnsUsage)
	moved := asked("0a000002", opened.Add(5*time.Minute))
	moved.From, moved.Reason, moved.Attempts, moved.Usage = "work", status.ReasonMovedOff+"work "+status.WhyLimit, 3, json.RawMessage(aMovesUsage)
	moved.Tried = []ledger.Tried{{Account: "work", Why: status.WhyLimit}, {Account: "personal", Why: status.WhyLimit}}
	refused := asked("0a000003", opened.Add(6*time.Minute))
	refused.Status, refused.Attempts = 429, 3
	refused.Tried = []ledger.Tried{{Account: "side", Why: status.WhyLimit}, {Account: "work", Why: status.WhyLimit}}
	inFlight := func(id string, sent time.Time) views.Told {
		return views.Told{Request: id, Session: first.Session, Model: first.Model, Account: "side", Sent: sent, Heard: at}
	}
	answering := inFlight("0a000004", at.Add(-20*time.Second))
	answering.First, answering.Chars = at.Add(-19*time.Second), 5100
	stream := views.Stream{Live: true, Requests: map[string]views.Told{
		answering.Request: answering, "0a000005": inFlight("0a000005", at.Add(-4*time.Second)),
	}}
	return []ledger.Line{first, moved, refused}, stream, at
}

// inked is a cell a test holds a row to: the first of the text given in the
// row, drawn in the ink given.
type inked struct {
	text string
	ink  ink
}

// The inks a row's cells are held to.
var (
	modeMark      = ink{token: theme.AccentMode, bold: true}
	keyMark       = ink{token: theme.AccentKey, bold: true}
	destroyedMark = ink{token: theme.StateDestructive, bold: true}
	tookCounting  = ink{token: theme.AccentKey}
	attention     = ink{token: theme.AccentAttention}
)

// The Requests draw each kind of row in its columns at each width the design
// checks: a session's first, ▲ in accent.mode; a move, ▸ in accent.key, its
// routing cut where the column's short of it, the cost of writing its
// context again kept whole, in accent.attention; a request every account
// refused, ■ and its status in bold state.destructive; and two in flight, ↓
// answering and ↑ asking, each mark bold in accent.key, its took counting in
// accent.key, and what it's doing bold in accent.key. Each is the design's
// at its width, cut with an ellipsis where it's cut, so a row reaching the
// last column ends there, whole.
func TestTheRequestsDrawEachKindOfRowAtEachWidth(t *testing.T) {
	lines, stream, at := rowsOfEachKind()
	p := requestsOf(views.LogSources{From: at, Stream: stream, Now: at}, lines)
	marks := []inked{{"▲", modeMark}, {"▸", keyMark}, {"■", destroyedMark}, {"↓", keyMark}, {"↑", keyMark}}
	flying := [][]inked{3: {{"20.0s", tookCounting}, {"answering", keyMark}}, 4: {{"4.0s", tookCounting}, {"asking", keyMark}}}
	tests := []struct {
		width int
		want  []string
		// inks are cells of the rows, by row, beyond the marks and those in
		// flight; and last the last column's cell of the row that reaches it.
		inks [][]inked
		last string
	}{
		{
			width: 52,
			want: []string{
				"▲ 14:30:00 d28c     work      1.2s   new session",
				"▸ 14:35:00 d28c     side      1.2s   from work",
				"■ 14:36:00 d28c     side      1.2s   after side's l…",
				"↓ 14:41:40 d28c     side     20.0s   answering",
				"↑ 14:41:56 d28c     side      4.0s   asking",
			},
			last: "…",
		},
		{
			width: 100,
			want: []string{
				"▲ 14:30:00  d28c               opus-5-5   work      200       1.2s  new session",
				"▸ 14:35:00  d28c               opus-5-5   side      200       1.2s  after work's limit, pers…  $8.00",
				"■ 14:36:00  d28c               opus-5-5   side      429       1.2s  after side's limit, work's limit",
				"↓ 14:41:40  d28c               opus-5-5   side               20.0s  answering · 5.1k characters",
				"↑ 14:41:56  d28c               opus-5-5   side                4.0s  asking",
			},
			inks: [][]inked{1: {{"$8.00", attention}}, 2: {{"429", destroyedMark}}},
			last: "0",
		},
		{
			width: 120,
			want: []string{
				"▲ 14:30:00  d28c               opus-5-5   work      200       1.2s    180k     2.0k    900  new session",
				"▸ 14:35:00  d28c               opus-5-5   side      200       1.2s       —     1.0M   1.2k  after work's limit, …  $8.00",
				"■ 14:36:00  d28c               opus-5-5   side      429       1.2s                          after side's limit, work's…",
				"↓ 14:41:40  d28c               opus-5-5   side               20.0s                          answering · 5.1k characters",
				"↑ 14:41:56  d28c               opus-5-5   side                4.0s                          asking",
			},
			inks: [][]inked{1: {{"$8.00", attention}}, 2: {{"429", destroyedMark}}},
			last: "0",
		},
		{
			width: 160,
			want: []string{
				"▲ 14:30:00  d28c               opus-5-5   work      200              1.2s    180k     2.0k    900                               new session",
				"▸ 14:35:00  d28c               opus-5-5   side      200              1.2s       —     1.0M   1.2k                               after work's limit, pe…  $8.00",
				"■ 14:36:00  d28c               opus-5-5   side      429              1.2s                                                       after side's limit, work's li…",
				"↓ 14:41:40  d28c               opus-5-5   side               1.0s   20.0s                          answering · 5.1k characters",
				"↑ 14:41:56  d28c               opus-5-5   side                       4.0s                          asking",
			},
			inks: [][]inked{1: {{"$8.00", attention}}, 2: {{"429", destroyedMark}}},
		},
	}
	for _, tt := range tests {
		f := logFrameOf(p, tt.width, 24)
		c := f.page(routerDoc("side", 0), at)
		rows := textOf(f.Draw(routerDoc("side", 0), at))[f.top()+1 : f.top()+1+len(tt.want)]
		if !slices.Equal(rows, tt.want) {
			t.Errorf("at %d columns, the rows read\n%s\nwant\n%s", tt.width, strings.Join(rows, "\n"), strings.Join(tt.want, "\n"))
			continue
		}
		for i, row := range rows {
			y := f.top() + 1 + i
			cells := slices.Concat([]inked{marks[i]}, inkedOn(flying, i), inkedOn(tt.inks, i))
			for _, want := range cells {
				if got := inkOfText(c, y, row, want.text); got != want.ink {
					t.Errorf("at %d columns, row %d's %q is inked %+v, want %+v", tt.width, i+1, want.text, got, want.ink)
				}
			}
		}
		if tt.last != "" && !endsAtTheLast(c, rows, f.top()+1, tt.width, tt.last) {
			t.Errorf("at %d columns, no row ends at the last column with %q", tt.width, tt.last)
		}
	}
}

// endsAtTheLast reports whether one of rows, drawn on the canvas c from row
// top, reaches the last of width columns, its cell there last.
func endsAtTheLast(c *canvas, rows []string, top, width int, last string) bool {
	for i, r := range rows {
		if utf8.RuneCountInString(r) == width && c.at(width-1, top+i).glyph == last {
			return true
		}
	}
	return false
}

// inkedOn are the cells of row i the inks given hold, none where they hold none
// of it.
func inkedOn(inks [][]inked, i int) []inked {
	if i < len(inks) {
		return inks[i]
	}
	return nil
}

// inkOfText is the ink of the first cell of the first of text in the row y
// of the canvas c, whose text is row: the zero ink where it holds none.
func inkOfText(c *canvas, y int, row, text string) ink {
	before, _, found := strings.Cut(row, text)
	if !found {
		return ink{}
	}
	return c.at(utf8.RuneCountInString(before), y).ink
}
