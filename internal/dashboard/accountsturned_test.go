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

// Turned Compare's ACTIVITY says the sessions moved in full where they fit a
// column, else how many: work's 3 out and 1 in.
func TestTurnedComparesMovedFallsToHowMany(t *testing.T) {
	page := phonePage(t)
	page.Rows[0].Totals.MovedOn = 1
	for _, tt := range []struct {
		width int
		want  string
	}{{52, "1 in · 3 out"}, {44, "4"}, {40, "4"}} {
		f := phoneText(t, tt.width, turnedRows, AccountsCompare, "", page, nil)
		rows := textOf(f.Draw(phoneDoc(), routedAt))
		heads := slices.Index(rows, rowStarting(t, rows, "ACTIVITY"))
		if got := strings.TrimSpace(ansi.Cut(rows[heads+1], columnOf(rows[heads], "moved"), tt.width)); got != tt.want {
			t.Errorf("at %d, work's moved reads %q, want %q, in %q", tt.width, got, tt.want, rows[heads+1])
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
		accounts[i].Plan = "ultra"
	}
	table := ledger.Table{Plans: []ledger.Plan{{ID: "ultra", Name: "Ultra", Size: 10, Prices: []ledger.PlanPrice{
		{From: "2025-04-09", Monthly: 200 * ledger.Dollar}, {From: "2026-09-21", Monthly: 4000 * ledger.Dollar},
	}}}}
	var heads []string
	for _, monthly := range []float64{200, 4000} {
		page := phonePage(t)
		for i := range page.Rows {
			page.Rows[i].Plan.Price = usd(monthly)
		}
		page.All.Plan.Price = usd(3 * monthly)
		f := phoneText(t, 40, turnedRows, AccountsCompare, "", page, nil)
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

// Turned Compare at 40 columns, with side's plan not known, keeps its
// against it whole, head and figure: the last column holds its head's
// cells, the others sharing what's left, the plans' bound, of the plans
// known, within its share.
func TestTurnedComparesAgainstItIsWholeAt40(t *testing.T) {
	accounts := slices.Clone(threeFramed)
	accounts[0].Plan, accounts[1].Plan = "ultra", "ultra"
	page := phonePage(t)
	page.Rows[2].Plan.Price, page.All.Plan.Price = nil, usd(400)
	page.All.Plan.Plans = views.PlansKnown{Known: 2, Of: 3}
	f := phoneText(t, 40, turnedRows, AccountsCompare, "", page, nil)
	f.Accounts.Accounts, f.PlansAtMost = accounts, views.PlansAtMost(accounts, ledger.Pricing)
	rows := textOf(f.Draw(phoneDoc(), routedAt))
	for _, want := range []string{"VALUE      plan     worth    against it", " side      —        $239     5.2×", " all       $400/mo  $635     4.6× · 2/3"} {
		if !slices.ContainsFunc(rows, func(row string) bool { return strings.TrimRight(row, " ") == want }) {
			t.Errorf("Compare reads\n%s\nwant a row %q", strings.Join(rows, "\n"), want)
		}
	}
}

// Turned Compare gains WEEKS and WORTH after BY MODEL, a band each column,
// the accounts' then all accounts': WEEKS' bands back to back under its
// heading, each its name, the latest of the reach's peaks under their
// columns, how many more the first band's peaks' row says, and Weeks'
// facts at the right; WORTH's a blank row before each, its name on its
// chart's top row, its chart from where the bars start to the frame's edge,
// its axis and its peak under it, all accounts' on its own scale.
func TestTurnedComparesWEEKSAndWORTHAreBands(t *testing.T) {
	const at = 13
	_, rows := chartedCompare(t, 73, turnedRows, chartedPage(t))
	weeks := slices.Index(rows, rowStarting(t, rows, "WEEKS  each week at its peak"))
	if byModel := slices.Index(rows, rowStarting(t, rows, "BY MODEL")); byModel > weeks {
		t.Errorf("WEEKS is on row %d, want it after BY MODEL, on %d", weeks, byModel)
	}
	for i, want := range []struct{ name, facts, left string }{
		{" work", "limit hit 3 of 7", "9% left at a reset"},
		{" personal  ◆", "limit hit 0 of 7", "27% left at a reset"},
		{" side", "limit hit 0 of 7", "59% left at a reset"},
		{" all", "short on 3 of 7", "32% left at a reset"},
	} {
		top := weeks + 2 + i*bandHeight
		if !strings.HasPrefix(rows[top], want.name+" ") || !strings.HasSuffix(rows[top], want.facts) || !strings.HasSuffix(rows[top+1], want.left) {
			t.Errorf("band %d reads %q, %q, want %s, its facts %q, %q", i, rows[top], rows[top+1], want.name, want.facts, want.left)
		}
	}
	if peaks := rows[weeks+2+bandRows]; !strings.HasPrefix(peaks, " ‹ 4 more ") || !strings.HasSuffix(peaks, "100%    81%     100%    34%…") {
		t.Errorf("work's peaks read %q, want the reach's last four, and 4 more", peaks)
	}
	worth := slices.Index(rows, rowStarting(t, rows, "WORTH  at API prices, by day"))
	activity := slices.Index(rows, rowStarting(t, rows, "ACTIVITY"))
	if worth < weeks || activity < worth {
		t.Errorf("WEEKS, WORTH and ACTIVITY are on rows %d, %d and %d, want them in that order", weeks, worth, activity)
	}
	for i, want := range []struct{ name, peak string }{{" work", "peak Wed $148"}, {" all", "peak Wed $222 · its own scale"}} {
		top := worth + 2
		if i > 0 {
			top = activity - 1 - (worthChartRows + 2)
		}
		if !strings.HasPrefix(rows[top], want.name) || rows[top-1] != "" {
			t.Errorf("WORTH's band reads %q, a row before %q, want %s after a blank row", rows[top], rows[top-1], want.name)
		}
		if axis := rows[top+worthChartRows]; columnOf(axis, "Mon") != at || !strings.HasSuffix(strings.TrimRight(axis, " "), "Sun") {
			t.Errorf("%s's axis reads %q, want Mon at %d to Sun", want.name, axis, at)
		}
		if peak := rows[top+worthChartRows+1]; columnOf(peak, want.peak) != at {
			t.Errorf("%s's peak reads %q, want %q at %d", want.name, peak, want.peak, at)
		}
	}
}
