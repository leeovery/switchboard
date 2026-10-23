package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
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
// are, but a column is never narrower than its least: its head, and the
// widest of its bound, the config's, as the plans' prices are; each a cell
// short of the next but the last.
func TestTurnedComparesColumnsHoldTheirLeasts(t *testing.T) {
	column := func(head string, bound ...string) turnedColumn {
		c := turnedColumn{head: head}
		for _, said := range bound {
			c.bound = append(c.bound, []part{figureIn(said, secondaryInk)})
		}
		return c
	}
	tests := []struct {
		name    string
		room    int
		columns []turnedColumn
		at      []int
	}{
		{name: "alike at 52", room: 38, columns: []turnedColumn{column("reqs"), column("sessions"), column("moved")}, at: []int{0, 13, 26}},
		{name: "alike at 73", room: 59, columns: []turnedColumn{column("plan", "$200/mo", "$600/mo"), column("worth"), column("against it")}, at: []int{0, 20, 40}},
		{name: "the last's head held at 40", room: 28, columns: []turnedColumn{column("plan", "$200/mo", "$600/mo"), column("worth"), column("against it")}, at: []int{0, 9, 18}},
		{name: "a plan's sum held at 40", room: 28, columns: []turnedColumn{column("plan", "$200/mo", "$1,700/mo"), column("worth"), column("against it")}, at: []int{0, 10, 18}},
		{name: "a middle column's head held", room: 20, columns: []turnedColumn{column("reqs"), column("sessions"), column("moved")}, at: []int{0, 6, 15}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spans := turnedGroup{columns: tt.columns}.spans(tt.room)
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

// Turned Compare's VALUE holds its columns where they are as the period
// changes, though a plan's price changed between the periods: the plans'
// column is sized by the most each plan has cost, as the config prices it,
// never by the prices of the period's page.
func TestTurnedComparesPlansHoldStillAcrossADatedPriceChange(t *testing.T) {
	accounts := slices.Clone(threeFramed)
	for i := range accounts {
		accounts[i].Plan = "max20x"
	}
	table := ledger.Table{Plans: []ledger.Plan{{ID: "max20x", Name: "Max 20x", Size: 20, Prices: []ledger.PlanPrice{
		{From: "2025-04-09", Monthly: 200 * ledger.Dollar}, {From: "2026-09-21", Monthly: 4000 * ledger.Dollar},
	}}}}
	var heads []string
	for _, monthly := range []float64{200, 4000} {
		page := phonePage(t)
		for i := range page.Rows {
			page.Rows[i].Plan.Price = usd(monthly)
		}
		page.All.Plan.Price = usd(3 * monthly)
		f := phoneText(t, 40, 60, AccountsCompare, "", page, nil)
		f.Accounts.Accounts, f.PlansAtMost = accounts, views.PlansAtMost(accounts, table)
		rows := textOf(f.Draw(phoneDoc(), routedAt))
		i := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(row, "VALUE") })
		if i < 0 {
			t.Fatalf("at $%v a month, no VALUE in\n%s", monthly, strings.Join(rows, "\n"))
		}
		heads = append(heads, rows[i])
		if all := rows[i+4]; !strings.Contains(all, prose.Grouped(int(3*monthly))) {
			t.Errorf("at $%v a month, all accounts' plan is cut: %q", monthly, all)
		}
	}
	if heads[0] != heads[1] {
		t.Errorf("VALUE's heads read %q at one period's prices and %q at the other's", heads[0], heads[1])
	}
}

// Turned Compare at 40 columns, with a plan not known, keeps its against it
// whole, head and figure: the last column holds its head's cells, the
// others sharing what's left.
func TestTurnedComparesAgainstItIsWholeAt40(t *testing.T) {
	page := phonePage(t)
	page.All.Plan.Plans = views.PlansKnown{Known: 2, Of: 3}
	f := phoneText(t, 40, 60, AccountsCompare, "", page, nil)
	rows := textOf(f.Draw(phoneDoc(), routedAt))
	for _, want := range []string{"VALUE      plan     worth    against it", " all       $600/mo  $635     4.6× · 2/3"} {
		if !slices.ContainsFunc(rows, func(row string) bool { return strings.TrimRight(row, " ") == want }) {
			t.Errorf("Compare reads\n%s\nwant a row %q", strings.Join(rows, "\n"), want)
		}
	}
}
