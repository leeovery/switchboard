package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/views"
)

// The sessions moved take forms as the limits hit do: said in full, then
// how many moves, an account's in and out summed and every account's moves
// counted once, in the column's ink; none moved is its one form.
func TestTheSessionsMovedSayEachForm(t *testing.T) {
	k := secondaryInk
	tests := []struct {
		name   string
		totals views.AccountTotals
		ink    ink
		want   []line
	}{
		{name: "on and off", totals: views.AccountTotals{MovedOn: 1, MovedOff: 20}, ink: k, want: []line{{{"1 in · 20 out", k}}, {{"21", k}}}},
		{name: "off alone", totals: views.AccountTotals{MovedOff: 3}, ink: k, want: []line{{{"3 out", k}}, {{"3", k}}}},
		{name: "every account's", totals: views.AccountTotals{All: true, MovedOn: 5, MovedOff: 5}, ink: strongInk, want: []line{{{"5 moves", strongInk}}, {{"5", strongInk}}}},
		{name: "a thousand, grouped", totals: views.AccountTotals{MovedOn: 999, MovedOff: 999}, ink: k, want: []line{{{"999 in · 999 out", k}}, {{"1,998", k}}}},
		{name: "none moved", totals: views.AccountTotals{}, ink: k, want: []line{{{"—", faintInk}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			forms := movedForms(tt.totals, tt.ink)
			if len(forms) != len(tt.want) {
				t.Fatalf("%d forms, want %d", len(forms), len(tt.want))
			}
			for i, form := range forms {
				if got := fitParts(200, form...).inked(); !slices.Equal(got, tt.want[i]) {
					t.Errorf("form %d said %+v, want %+v", i, got, tt.want[i])
				}
			}
		})
	}
}

// Compare drawn wide holds the sessions moved whole at its column's least,
// at their bound, so it never falls to turned Compare's count.
func TestWideComparesLeastHoldsTheSessionsMovedWhole(t *testing.T) {
	whole := partsWidth(movedParts(compareBound(1, false).Totals, secondaryInk))
	if !slices.ContainsFunc(compareFigures(3), func(figure []part) bool { return partsWidth(figure) == whole }) {
		t.Errorf("the figures wide Compare's least holds have none %d cells, the sessions moved whole at their bound", whole)
	}
}

// Turned Compare's ACTIVITY says the sessions moved in full where they fit a
// column, else how many: work's 3 out and 1 in.
func TestTurnedComparesMovedFallsToHowMany(t *testing.T) {
	page := phonePage(t)
	page.Rows[0].Totals.MovedOn = 1
	for _, tt := range []struct {
		width int
		want  string
	}{{52, "1 in · 3 out"}, {44, "4"}, {40, "4"}} {
		f := phoneText(t, tt.width, 60, AccountsCompare, "", page, nil)
		rows := textOf(f.Draw(phoneDoc(), routedAt))[f.top():]
		if got := strings.TrimSpace(ansi.Cut(rows[21], columnOf(rows[20], "moved"), tt.width)); got != tt.want {
			t.Errorf("at %d, work's moved reads %q, want %q, in %q", tt.width, got, tt.want, rows[21])
		}
	}
}

// Turned Compare's BY MODEL never cuts a share: where the names, at their
// least, leave too little, the figures' column narrows first, a version's
// name cut, the bars holding 20; then the bars give way, to 10 at the
// floor, so a share of 100% still shows whole.
func TestTurnedComparesBarsGiveWayBeforeAShareIsCut(t *testing.T) {
	tests := []struct {
		name  string
		width int
		share float64
		bar   int
		end   string
	}{
		{name: "at 52, whole", width: 52, share: 0.55, bar: 20, end: "● Opus 5.5 55%"},
		{name: "at 46, the name cut, the bars held", width: 46, share: 0.55, bar: 20, end: "● Opus… 55%"},
		{name: "at 42, the figures at their least, the bars held", width: 42, share: 0.55, bar: 20, end: "● O… 55%"},
		{name: "at 41, the bars give way", width: 41, share: 0.55, bar: 19, end: "● O… 55%"},
		{name: "at 40, the bars give way", width: 40, share: 0.55, bar: 18, end: "● O… 55%"},
		{name: "at 40, a share of 100% whole", width: 40, share: 1, bar: 18, end: "● … 100%"},
		{name: "at 32, the bars at their floor", width: 32, share: 1, bar: turnedBarFloor, end: "● … 100%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := phonePage(t)
			page.Rows[0].Totals.ByModel = []views.ModelShare{{Model: "claude-opus-5-5", Share: new(tt.share), Amount: usd(100 * tt.share)}}
			f := phoneText(t, tt.width, 60, AccountsCompare, "", page, nil)
			row := strings.TrimRight(textOf(f.Draw(phoneDoc(), routedAt))[f.top()+12], " ")
			if got := strings.Count(row, "█"); got != tt.bar {
				t.Errorf("work's bar is %d cells, want %d, in %q", got, tt.bar, row)
			}
			if !strings.HasSuffix(row, tt.end) {
				t.Errorf("work's BY MODEL row is %q, want it to end %q", row, tt.end)
			}
		})
	}
}

// Turned Compare's groups share their room alike, as the design's columns
// are, but a column is never narrower than its least: its head, and, where
// what it says is the config's, as the plans are, the widest it says; each
// a cell short of the next but the last. What the data says, as worth, never
// widens a column.
func TestTurnedComparesColumnsHoldTheirLeasts(t *testing.T) {
	saying := func(said string) func(compareColumn) partForms {
		return func(compareColumn) partForms { return partForms{{figureIn(said, secondaryInk)}} }
	}
	column := func(head, said string, configured bool) turnedColumn {
		return turnedColumn{head: head, said: saying(said), configured: configured}
	}
	tests := []struct {
		name    string
		room    int
		columns []turnedColumn
		at      []int
	}{
		{name: "alike at 52", room: 38, columns: []turnedColumn{column("reqs", "1", false), column("sessions", "1", false), column("moved", "1", false)}, at: []int{0, 13, 26}},
		{name: "alike at 73", room: 59, columns: []turnedColumn{column("plan", "$200/mo", true), column("worth", "$1", false), column("against it", "1×", false)}, at: []int{0, 20, 40}},
		{name: "the last's head held at 40", room: 28, columns: []turnedColumn{column("plan", "$600/mo", true), column("worth", "$1", false), column("against it", "1×", false)}, at: []int{0, 9, 18}},
		{name: "a plan's sum held at 40", room: 28, columns: []turnedColumn{column("plan", "$1,700/mo", true), column("worth", "$1", false), column("against it", "1×", false)}, at: []int{0, 10, 18}},
		{name: "worth's data never held", room: 28, columns: []turnedColumn{column("plan", "$600/mo", true), column("worth", "$16,024,000", false), column("against it", "1×", false)}, at: []int{0, 9, 18}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spans := turnedGroup{columns: tt.columns}.spans(tt.room, []compareColumn{{}})
			var at []int
			for _, s := range spans {
				at = append(at, s.At)
			}
			if !slices.Equal(at, tt.at) {
				t.Errorf("the columns are at %v, want %v", at, tt.at)
			}
		})
	}
}
