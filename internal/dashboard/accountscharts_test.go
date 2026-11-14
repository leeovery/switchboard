package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// chartedPeaks are each column's peaks in the eight weeks of History's
// Weeks, 10 August to 28 September, as percentages, this week's so far;
// and the facts Weeks says of the seven whole: limits hit, or all
// accounts' weeks short, and what they left at a reset, on average.
var chartedPeaks = []struct {
	peaks  []float64
	limits int
	left   float64
}{
	{[]float64{72, 88, 100, 94, 100, 81, 100, 34}, 3, 0.09},
	{[]float64{64, 70, 58, 77, 69, 90, 85, 66}, 0, 0.27},
	{[]float64{22, 35, 41, 30, 52, 47, 60, 78}, 0, 0.59},
	{[]float64{53, 64, 66, 67, 74, 73, 82, 59}, 3, 0.32},
}

// chartedWorth is each column's worth this week, Monday to Thursday, in
// dollars, the days after today to come: work's peak Wednesday's $148,
// personal's half of work's each day, side's none, and all accounts' every
// one's together.
var chartedWorth = [][]float64{{40, 90, 148, 60}, {20, 45, 74, 30}, {0, 0, 0, 0}, {60, 135, 222, 90}}

// chartedPage is Compare's page of the auto world this week, as threeCompared
// has it, with its charts: its reach the eight weeks to this one, each
// column's peaks and facts as chartedPeaks has them, and its worth by day,
// as chartedWorth has it.
func chartedPage(t *testing.T) views.AccountsPage {
	page := comparedPage(views.PeriodThisWeek, comparedVersions(t), compareWork, comparePersonal, compareSide, compareAll)
	first := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	for i := range 8 {
		week := first.AddDate(0, 0, 7*i)
		b := views.Bucket{Start: week, Label: week.Format("2 Jan")}
		if week.Day() <= 7 || i == 0 {
			b.Axis = week.Format("Jan")
		}
		page.Reach = append(page.Reach, b)
	}
	rows := []*views.AccountsRow{&page.Rows[0], &page.Rows[1], &page.Rows[2], &page.All}
	for i, row := range rows {
		c := chartedPeaks[i]
		for w, p := range c.peaks {
			row.Reached = append(row.Reached, views.WeekPeak{Share: new(p / 100), Open: w == 7, AtCap: p >= 100})
		}
		row.Reaching = views.AccountVerdict{All: row == &page.All, Weeks: 7, Limits: c.limits, LeftAtReset: new(c.left)}
		row.WorthBy = views.WorthCharts{weekByDay(chartedWorth[i])}
	}
	return page
}

// weekByDay is a week's worth by day, Monday 28 September to Sunday, the
// dollars given of its first days, the rest to come, its peak the first
// worth most.
func weekByDay(dollars []float64) views.WorthBy {
	by := views.WorthBy{Unit: views.ByDay, Peak: -1, ToCome: 7 - len(dollars)}
	for i := range 7 {
		day := time.Date(2026, time.September, 28+i, 0, 0, 0, 0, time.UTC)
		b := views.WorthBucket{Start: day, Label: day.Format("Mon"), Axis: day.Format("Mon")}
		if i < len(dollars) && dollars[i] > 0 {
			b.Amount = usd(dollars[i])
			if by.Peak < 0 || dollars[i] > dollars[by.Peak] {
				by.Peak = i
			}
		}
		by.Buckets = append(by.Buckets, b)
	}
	return by
}

// chartedCompare is Compare of chartedPage, as text alone, at the size
// given, and its rows drawn.
func chartedCompare(t *testing.T, width, height int, page views.AccountsPage) (Frame, []string) {
	f, doc := threeCompared(t, width, height)
	f.Accounts.Page = &page
	return f, textOf(f.Draw(doc, routedAt))
}

// cut is the cells of row from column x, as many as text has, so a test
// reads what's drawn there against it.
func cut(row string, x int, text string) string {
	return ansi.Cut(row, x, x+ansi.StringWidth(text))
}

// WEEKS draws each column's weeks of the reach as columns five rows tall,
// a week's peak filling them from the foot to it, a full week all five,
// this week's so far as far as it has got, with … under it on the axis in
// place of a name, the months named where they start; under them its
// facts, Weeks' over its whole weeks, as fully as its column holds: an
// account's limits hit, all accounts' weeks short, and what they left.
func TestWEEKSChartsEachColumnsWeeksOfTheReach(t *testing.T) {
	_, rows := chartedCompare(t, 173, 60, chartedPage(t))
	head := rowWith(rows, "WEEKS  each week at its peak")
	if head < 0 {
		t.Fatalf("no WEEKS heading in\n%s", strings.Join(rows, "\n"))
	}
	work, all := columnOf(rows[6], "work"), columnOf(rows[6], "all accounts")
	column := func(x, week int) string {
		var cells []string
		for r := 1; r <= weeksChartRows; r++ {
			cells = append(cells, ansi.Cut(rows[head+r], x+4*week, x+4*week+3))
		}
		return strings.Join(cells, "|")
	}
	tests := []struct {
		name string
		x    int
		week int
		want string
	}{
		{"work's 100%, full", work, 2, "███|███|███|███|███"},
		{"work's 72%, its top in eighths", work, 0, "┄┄┄|▅▅▅|███|███|███"},
		{"work's this week, so far", work, 7, "┄┄┄|   |   |▆▆▆|███"},
		{"all accounts' 53%", all, 0, "   |   |▅▅▅|███|███"},
	}
	for _, tt := range tests {
		if got := column(tt.x, tt.week); got != tt.want {
			t.Errorf("%s reads %q, want %q", tt.name, got, tt.want)
		}
	}
	axis, facts := rows[head+weeksChartRows+1], rows[head+weeksChartRows+2]
	for _, want := range []struct {
		x    int
		text string
	}{{work, "Aug"}, {work + 4*4, "Sep"}, {work + 4*7, "…"}, {all + 4*7, "…"}} {
		if got := cut(axis, want.x, want.text); got != want.text {
			t.Errorf("the axis reads %q from column %d, want %q", got, want.x, want.text)
		}
	}
	if got, want := strings.TrimSpace(ansi.Cut(facts, work, columnOf(rows[6], "personal"))), "limit hit 3 of 7 · 9% left"; got != want {
		t.Errorf("work's facts read %q, want %q", got, want)
	}
	if got, want := strings.TrimSpace(ansi.Cut(facts, all, 173)), "short on 3 of 7 · 32% left at a reset"; got != want {
		t.Errorf("all accounts' facts read %q, want %q", got, want)
	}
}

// The cap's level is marked across the chart of an account that keeps a
// reserve, along the row its cap falls in, on the cells no column fills;
// across no other's, nor all accounts'.
func TestWEEKSMarksTheCapWhereAReserveIsKept(t *testing.T) {
	_, rows := chartedCompare(t, 173, 60, chartedPage(t))
	top := rowWith(rows, "WEEKS  each week at its peak") + 1
	work, personal, all := columnOf(rows[6], "work"), columnOf(rows[6], "personal"), columnOf(rows[6], "all accounts")
	if got, want := ansi.Cut(rows[top], work, work+31), "┄┄┄┄▃▃▃┄███┄▆▆▆┄███┄┄┄┄┄███┄┄┄┄"; got != want {
		t.Errorf("work's chart's top row reads %q, want its cap's level marked between the columns, %q", got, want)
	}
	for _, x := range []int{personal, all} {
		if got := ansi.Cut(rows[top], x, x+31); strings.Contains(got, capLevel) {
			t.Errorf("the chart from column %d marks a cap, %q, where no reserve is kept", x, got)
		}
	}
}

// At 12 months WEEKS' heading says a month's column is its fullest week,
// and its axis marks no column this week's, so far, as a month's isn't,
// though its fullest week may be.
func TestWEEKSSaysAMonthIsItsFullestAt12Months(t *testing.T) {
	page := chartedPage(t)
	page.Period = views.PeriodTwelveMonths
	_, rows := chartedCompare(t, 173, 60, page)
	head := rowWith(rows, "WEEKS  each week at its peak, a month its fullest")
	if head < 0 {
		t.Fatalf("no heading naming a month's fullest week in\n%s", strings.Join(rows, "\n"))
	}
	if axis := rows[head+weeksChartRows+1]; strings.Contains(axis, views.PeakSoFar) {
		t.Errorf("the axis reads %q, want no column marked this week's", axis)
	}
}

// WEEKS' facts take the fullest form their column holds: what was left at
// a reset said short, then not at all; and where no whole week is judged, a
// dash.
func TestWEEKSFactsTakeTheFullestFormTheyHold(t *testing.T) {
	v := views.AccountVerdict{Weeks: 51, Limits: 3, LeftAtReset: new(0.21)}
	for _, tt := range []struct {
		cells int
		want  string
	}{{40, "limit hit 3 of 51 · 21% left at a reset"}, {35, "limit hit 3 of 51 · 21% left"}, {20, "limit hit 3 of 51"}} {
		if got := fitParts(tt.cells, weeksFacts(v).fitting(tt.cells)...).plain(); got != tt.want {
			t.Errorf("in %d cells the facts read %q, want %q", tt.cells, got, tt.want)
		}
	}
	if got := fitParts(40, weeksFacts(views.AccountVerdict{}).fitting(40)...).plain(); got != "—" {
		t.Errorf("with no whole week the facts read %q, want a dash", got)
	}
}

// WORTH charts each column's worth four rows tall, every account's against
// the highest bucket of any, so work's Wednesday, the highest, fills all
// four and personal's, half of it, two; all accounts' against its own, so
// its Wednesday fills all four too, and its peak says so. The axis names
// each day; the line under it names the peak.
func TestWORTHChartsTheAccountsOnOneScaleAllAccountsOnItsOwn(t *testing.T) {
	_, rows := chartedCompare(t, 173, 60, chartedPage(t))
	head := rowWith(rows, "WORTH  at API prices, by day")
	if head < 0 {
		t.Fatalf("no WORTH heading by day in\n%s", strings.Join(rows, "\n"))
	}
	work, personal, all := columnOf(rows[6], "work"), columnOf(rows[6], "personal"), columnOf(rows[6], "all accounts")
	day := func(x, i int) string {
		var cells []string
		for r := 1; r <= worthChartRows; r++ {
			cells = append(cells, ansi.Cut(rows[head+r], x+5*i, x+5*i+4))
		}
		return strings.Join(cells, "|")
	}
	tests := []struct {
		name string
		x, i int
		want string
	}{
		{"work's Wednesday, the highest of any account's", work, 2, "████|████|████|████"},
		{"personal's Wednesday, half of work's", personal, 2, "    |    |████|████"},
		{"work's Tuesday", work, 1, "    |▃▃▃▃|████|████"},
		{"personal's Tuesday, half of work's", personal, 1, "    |    |▂▂▂▂|████"},
		{"all accounts' Wednesday, its own highest", all, 2, "████|████|████|████"},
		{"all accounts' Tuesday, on its own scale", all, 1, "    |▃▃▃▃|████|████"},
	}
	for _, tt := range tests {
		if got := day(tt.x, tt.i); got != tt.want {
			t.Errorf("%s reads %q, want %q", tt.name, got, tt.want)
		}
	}
	if got, want := strings.TrimSpace(ansi.Cut(rows[head+worthChartRows+1], work, personal)), "Mon  Tue  Wed  Thu  Fri  Sat  Sun"; got != want {
		t.Errorf("work's axis reads %q, want %q", got, want)
	}
	peaks := rows[head+worthChartRows+2]
	if got, want := strings.TrimSpace(ansi.Cut(peaks, work, personal)), "peak Wed $148"; got != want {
		t.Errorf("work's peak reads %q, want %q", got, want)
	}
	if got, want := strings.TrimSpace(ansi.Cut(peaks, all, 173)), "peak Wed $222 · its own scale"; got != want {
		t.Errorf("all accounts' peak reads %q, want %q", got, want)
	}
}

// WORTH's peak takes the fullest form its column holds, its words giving
// way and never its figure: all accounts' saying its scale is its own, then
// not; then its worth alone. Nothing is said where nothing was worth
// anything.
func TestWORTHsPeakTakesTheFullestFormItHolds(t *testing.T) {
	by := views.WorthBy{Unit: views.ByWeek, Peak: 0, Buckets: []views.WorthBucket{{Label: "24 Aug", Amount: usd(710)}}}
	for _, tt := range []struct {
		cells int
		all   bool
		want  string
	}{
		{40, true, "peak week of 24 Aug $710 · its own scale"},
		{37, true, "peak week of 24 Aug $710"},
		{37, false, "peak week of 24 Aug $710"},
		{18, false, "$710"},
	} {
		if got := fitParts(tt.cells, peakForms(by, tt.all).fitting(tt.cells)...).plain(); got != tt.want {
			t.Errorf("in %d cells, all accounts' %v, the peak reads %q, want %q", tt.cells, tt.all, got, tt.want)
		}
	}
	if got := fitParts(40, peakForms(views.WorthBy{Peak: -1}, true).fitting(40)...).plain(); got != "" {
		t.Errorf("with nothing worth anything the peak reads %q, want nothing", got)
	}
}

// WORTH charts every column in the finest bucket that fits the narrowest,
// as Projects' WORTH BY takes its, and its heading names it: 12 weeks by
// week where 84 days don't fit a column, by day where they do.
func TestWORTHTakesTheFinestBucketTheNarrowestColumnHolds(t *testing.T) {
	page := chartedPage(t)
	page.Period = views.PeriodTwelveWeeks
	days, weeks := views.WorthBy{Unit: views.ByDay, Peak: -1}, views.WorthBy{Unit: views.ByWeek, Peak: -1}
	days.Buckets, weeks.Buckets = make([]views.WorthBucket, 84), make([]views.WorthBucket, 12)
	for _, row := range []*views.AccountsRow{&page.Rows[0], &page.Rows[1], &page.Rows[2], &page.All} {
		row.WorthBy = views.WorthCharts{days, weeks}
	}
	for _, tt := range []struct {
		width int
		said  string
	}{{173, "WORTH  at API prices, by week"}, {400, "WORTH  at API prices, by day"}} {
		if _, rows := chartedCompare(t, tt.width, 60, page); rowWith(rows, tt.said) < 0 {
			t.Errorf("at %d columns no heading reads %q", tt.width, tt.said)
		}
	}
}

// At 12 months each column holds the 12 months' totals, and against its
// price names their unit, as every period names its own.
func TestAt12MonthsAgainstItsPriceNamesThe12Months(t *testing.T) {
	page := chartedPage(t)
	page.Period = views.PeriodTwelveMonths
	for _, row := range []*views.AccountsRow{&page.Rows[0], &page.Rows[1], &page.Rows[2]} {
		row.Plan.Cost, row.Plan.Ratio = usd(2400), new(3.4)
	}
	page.All.Plan.Cost, page.All.Plan.Ratio = usd(7200), new(3.4)
	_, rows := chartedCompare(t, 173, 60, page)
	said := rows[rowWith(rows, "against its price")]
	for _, want := range []string{"3.4× its $2,400 12 months", "3.4× its $7,200 12 months"} {
		if !strings.Contains(said, want) {
			t.Errorf("against its price reads %q, want it to say %q", said, want)
		}
	}
}

// Where WEEKS and WORTH don't fit the rows, as on a short screen, they take
// the rows they need and the view scrolls to them under the heads held:
// WORTH's peaks out of view at the top, in view scrolled down to them.
func TestWEEKSAndWORTHScrollWhereTheyDontFit(t *testing.T) {
	page := chartedPage(t)
	f, doc := threeCompared(t, 160, 29)
	f.Accounts.Page = &page
	if rows := textOf(f.Draw(doc, routedAt)); rowWith(rows, "peak Wed $148") >= 0 {
		t.Errorf("at the top WORTH's peaks show, want them out of view:\n%s", strings.Join(rows, "\n"))
	}
	f.Scroll = 12
	rows := textOf(f.Draw(doc, routedAt))
	if rowWith(rows, "peak Wed $148") < 0 || !strings.HasPrefix(rows[6], "                          work") {
		t.Errorf("scrolled down, WORTH's peaks don't show under the heads held:\n%s", strings.Join(rows, "\n"))
	}
}

// The cap's level falls on the row of the chart its cap is in, a fifth of a
// week each: a 30% reserve's cap of 70% on the fourth row up, never the top.
func TestWEEKSMarksTheCapOnTheRowItFallsIn(t *testing.T) {
	page := chartedPage(t)
	f, doc := threeCompared(t, 173, 60)
	account("work", func(a *status.Account) { a.Reserve = 0.3 })(&doc)
	f.Accounts.Page = &page
	rows := textOf(f.Draw(doc, routedAt))
	head, work := rowWith(rows, "WEEKS  each week at its peak"), columnOf(rows[6], "work")
	for r := 1; r <= weeksChartRows; r++ {
		marked := strings.Contains(ansi.Cut(rows[head+r], work, work+31), capLevel)
		if want := r == weeksChartRows-3; marked != want {
			t.Errorf("work's chart's row %d up marks the cap %v, want %v", weeksChartRows+1-r, marked, want)
		}
	}
}

// Every account's worth is charted against the highest bucket of any
// configured account's, one scrolled out of view among them: side's
// Wednesday, twice work's, out of view at 100 columns, so work's fills half
// the chart.
func TestWORTHsScaleIsEveryAccountsShownOrNot(t *testing.T) {
	page := chartedPage(t)
	page.Rows[2].WorthBy = views.WorthCharts{weekByDay([]float64{0, 0, 296, 0})}
	_, rows := chartedCompare(t, 100, 60, page)
	if x := columnOf(rows[6], "side"); x >= 0 {
		t.Fatalf("side shows at column %d at 100 columns, want it scrolled out of view", x)
	}
	head, work := rowWith(rows, "WORTH  at API prices, by day"), columnOf(rows[6], "work")
	var cells []string
	for r := 1; r <= worthChartRows; r++ {
		cells = append(cells, ansi.Cut(rows[head+r], work+3*2, work+3*2+2))
	}
	if got, want := strings.Join(cells, "|"), "  |  |██|██"; got != want {
		t.Errorf("work's Wednesday reads %q, want half the chart, against side's, %q", got, want)
	}
}

// WORTH holds every period's chart in the narrowest column Compare draws, a
// cell a bucket at the fewest, never a chart crushed into a cell: where the
// period's finest buckets don't fit, coarser ones do. At 83 columns, the
// narrowest Compare is wide at, its columns at their least, today is by two
// hours and 30 days by week; at 173 with four accounts, a MacBook's, today
// is by the hour and 30 days still by week. Every head holds as many
// buckets as any period's coarsest chart has.
func TestWORTHFitsEveryPeriodInTheNarrowestColumn(t *testing.T) {
	in := views.HistoryInput{Ledger: ledger.Empty{}, Now: routedAt, Prices: ledger.Pricing, Accounts: tenFramed[:4], WeekStarts: time.Monday, WeekWindow: "7d"}
	fed := views.ReadSessionDays(ledger.Empty{}, routedAt, routedAt).Tailed(nil, true, nil, nil, routedAt)
	ten := tenRows()
	tests := []struct {
		width int
		units map[views.Period]views.BucketUnit
	}{
		{83, map[views.Period]views.BucketUnit{
			views.PeriodToday: views.ByTwoHours, views.PeriodThisWeek: views.ByDay, views.PeriodThirtyDays: views.ByWeek,
			views.PeriodTwelveWeeks: views.ByWeek, views.PeriodTwelveMonths: views.ByMonth,
		}},
		{173, map[views.Period]views.BucketUnit{
			views.PeriodToday: views.ByHour, views.PeriodThisWeek: views.ByDay, views.PeriodThirtyDays: views.ByWeek,
			views.PeriodTwelveWeeks: views.ByWeek, views.PeriodTwelveMonths: views.ByMonth,
		}},
	}
	for _, tt := range tests {
		for period, unit := range tt.units {
			charted := views.NewPast(in, nil).Accounts(period).Hourly(fed)
			page := comparedPage(period, comparedVersions(t), append(slices.Clone(ten[:4]), ten[len(ten)-1])...)
			for x := range page.Rows {
				page.Rows[x].WorthBy = charted.Rows[x].WorthBy
			}
			page.All.WorthBy = charted.All.WorthBy
			f := compareFrame(t, tenFramed[:4], page)
			f.Width, f.Height, f.Look = tt.width, 60, Look{}
			head := f.compareLaid().narrowest()
			if tt.width == 83 && head != f.leastCompared()-1 {
				t.Fatalf("at 83 columns the narrowest head is %d cells, want the least, %d", head, f.leastCompared()-1)
			}
			if !chartFits(head)(views.BucketsMost) {
				t.Errorf("at %d columns a head of %d cells doesn't hold %d buckets, the most a period's coarsest chart has", tt.width, head, views.BucketsMost)
			}
			by := page.All.WorthBy.Fitting(chartFits(head))
			if by.Unit != unit || !chartFits(head)(len(by.Buckets)) {
				t.Errorf("%s at %d columns, its head %d cells, WORTH is by %s in %d buckets, want by %s, fitting", period.Name(), tt.width, head, bucketWord[by.Unit], len(by.Buckets), bucketWord[unit])
			}
			rows := textOf(f.Draw(usageWorld(tenCompared, extraOff, heldFirst(4)), routedAt))
			if said := "WORTH  at API prices, by " + bucketWord[unit]; rowWith(rows, said) < 0 {
				t.Errorf("%s at %d columns, no heading reads %q", period.Name(), tt.width, said)
			}
		}
	}
}
