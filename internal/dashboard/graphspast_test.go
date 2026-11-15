package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/views"
)

// trendsYear is plainYear with limits hit in both windows, as the capture
// world's are, whose totals are too wide for a phone's three columns.
func trendsYear() views.YearPage {
	page := plainYear()
	page.Numbers.Limits = map[string]int{"7d": 3, "5h": 412}
	return page
}

// trendsFrame is a frame of Trends, in nord, of a terminal the size given,
// with the sections named shown, YEAR's and BY MONTH's page trendsYear's,
// MODELS BY DAY's daysWorld's of the frames' accounts, the last 12 months
// read, its windows named as claude names them.
func trendsFrame(t *testing.T, width, height int, sections ...string) Frame {
	t.Helper()
	f := graphsFrame(width, height, GraphsSectionsNamed(sections))
	f.Look = Screen(builtin(t, "nord"))
	year, days := trendsYear(), daysWorld(workDays, personalDays, sideDays)
	f.Graphs.Year, f.Graphs.Days, f.Graphs.Read = &year, &days, true
	f.WindowName = func(key string) string {
		return map[string]string{"5h": "5-hour", "7d": "Week", "7d_oi": "Fable week"}[key]
	}
	return f
}

// everyTrend are every section of Trends' names.
var everyTrend = []string{"week", "pool", "glance", "when", "year", "months", "models"}

// pastPlaced is where Trends lays the section out in the frame.
func pastPlaced(t *testing.T, f Frame, s GraphsSection) graphsPlaced {
	t.Helper()
	laid := f.graphsLaid()
	i := slices.IndexFunc(laid.sections, func(p graphsPlaced) bool { return p.section == s })
	if i < 0 {
		t.Fatalf("Trends lays out no %s", s.heading())
	}
	return laid.sections[i]
}

// figureLabels are the labels of the figures of each column given, as the
// form labels them.
func figureLabels(y yearDrawn) [][]string {
	var labels [][]string
	for _, c := range y.laid.columns {
		var column []string
		for _, figure := range c.figures {
			column = append(column, y.form.label(figure))
		}
		labels = append(labels, column)
	}
	return labels
}

// YEAR's totals sit beside the grid where the width holds both, as at 173;
// else under it, in History's three columns at 108; and at 73, a phone's,
// where History's leaves its third column out, in two, the third's figures
// on a row under the rest, so none goes unsaid, History's own Year left as
// it was.
func TestTrendsYearsTotalsSitBesideTheGridWhereTheyFitElseUnderIt(t *testing.T) {
	tests := []struct {
		width   int
		beside  bool
		columns [][]string
		rows    int
	}{
		{width: 173, beside: true, rows: yearKeyRow + 1, columns: [][]string{
			{"requests", "sessions", "limits hit", "worth", "plans cost", "against it", "busiest day", "longest run"},
		}},
		{width: 108, rows: yearNumbersRow + 4, columns: [][]string{
			{"requests", "sessions", "limits hit"}, {"worth", "plans cost", "against it"}, {"busiest day", "longest run"},
		}},
		{width: 73, rows: yearNumbersRow + 5, columns: [][]string{
			{"requests", "sessions", "limits", "busiest"}, {"worth", "plans", "against", "longest"},
		}},
	}
	for _, tt := range tests {
		f := trendsFrame(t, tt.width, 60, "year")
		y, ok := f.graphsYear()
		if !ok {
			t.Fatalf("at %d columns, YEAR has no Year", tt.width)
		}
		if got := figureLabels(y); y.laid.beside != tt.beside || !slices.EqualFunc(got, tt.columns, slices.Equal) {
			t.Errorf("at %d columns, YEAR's totals are beside the grid %t, in %q; want %t, in %q", tt.width, y.laid.beside, got, tt.beside, tt.columns)
		}
		if got := y.daysRows(); got != tt.rows || pastPlaced(t, f, GraphsYear).rows != sectionHead+tt.rows {
			t.Errorf("at %d columns, YEAR's body takes %d rows, laid out in %d, want %d", tt.width, got, pastPlaced(t, f, GraphsYear).rows-sectionHead, tt.rows)
		}
	}
	history, _ := yearFrame(t, 73, 60, views.YearAsk{}, trendsYear()).yearDrawn()
	if got, want := figureLabels(history), [][]string{{"requests", "sessions", "limits"}, {"worth", "plans", "against"}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("at 73 columns, History's Year's totals are in %q, want %q, its third column left out as it was", got, want)
	}
}

// On a phone, the row under YEAR's totals has the busiest day under the
// first column's figures and the longest run under the second's, each
// labelled short, as the rest are.
func TestTrendsYearsRowUnderItsTotalsLinesUpWithTheirColumns(t *testing.T) {
	f := trendsFrame(t, 73, 61, "year")
	y, _ := f.graphsYear()
	c := newCanvas(f.Width, y.daysRows())
	y.days(c, 0)
	text := textOf(c.rows(Look{}))
	limits, last := text[yearNumbersRow+3], text[yearNumbersRow+4]
	column := func(row, word string) int { return ansi.StringWidth(row[:strings.Index(row, word)]) }
	if !strings.HasPrefix(last, "busiest   Tue 22 Sep") || column(last, "longest") != column(limits, "against") {
		t.Errorf("the totals' last rows read\n%q\n%q\nwant busiest day under limits, and longest run under against", limits, last)
	}
}

// Under the grid, YEAR's totals spread to end where the grid's last week
// ends, on a phone as at 108 and 138, the room between their columns shared;
// where one column holds them, it's left as it is; and History's Year,
// laid out alike at 108, keeps its columns where they were, short of it.
func TestTrendsYearsTotalsUnderTheGridEndWhereItEnds(t *testing.T) {
	ends := func(y yearDrawn) (grid, totals int) {
		c := newCanvas(y.Width, y.daysRows())
		y.days(c, 0)
		rows := textOf(c.rows(Look{}))
		for i, row := range rows {
			end := ansi.StringWidth(strings.TrimRight(row, " "))
			if i < yearKeyRow && strings.Contains(row, "■") {
				grid = max(grid, end)
			} else if i > yearNumbersRow {
				totals = max(totals, end)
			}
		}
		return grid, totals
	}
	for _, width := range []int{73, 108, 138} {
		y, _ := trendsFrame(t, width, 60, "year").graphsYear()
		if grid, totals := ends(y); totals != grid {
			t.Errorf("at %d columns, YEAR's totals end at %d, want where its grid ends, %d", width, totals, grid)
		}
	}
	one := trendsFrame(t, 108, 60, "year").yearFor(yearDrawn{page: trendsYear(), apart: true})
	if spread := one.spread(one.laid.columns[:1], one.Width); spread[0].x != 0 {
		t.Errorf("one column of totals spread moves to %d, want it left at 0", spread[0].x)
	}
	history, _ := yearFrame(t, 108, 60, views.YearAsk{}, trendsYear()).yearDrawn()
	if grid, totals := ends(history); totals >= grid {
		t.Errorf("at 108 columns, History's Year's totals end at %d, want them short of its grid's end, %d, unspread", totals, grid)
	}
}

// Where YEAR's totals leave a column, its figures go under the columns that
// stay, each laid out as wide as every figure it then holds, so none is cut
// short: in a quiet year, whose short figures size narrow columns, and in
// one whose moved figures are the widest of all.
func TestTrendsYearsFiguresMovedUnderTheRestAreWhole(t *testing.T) {
	quiet, slight := trendsYear(), trendsYear()
	quiet.Numbers = views.YearNumbers{Requests: 1234, Sessions: 56, Amount: usd(140), Cost: usd(7200), Ratio: ratioOf(140, 7200), BusiestDay: "2026-09-22", LongestRun: 120}
	slight.Numbers = views.YearNumbers{Requests: 12, Sessions: 3, Amount: usd(4), Cost: usd(7200), Ratio: ratioOf(4, 7200), BusiestDay: "2026-09-22", LongestRun: 1200}
	for _, tt := range []struct {
		name    string
		page    views.YearPage
		longest string
	}{{"a quiet year", quiet, "120 days"}, {"the moved figures the widest", slight, "1,200 days"}} {
		for _, width := range []int{46, 52, 60, 73} {
			f := trendsFrame(t, width, 60, "year")
			f.Graphs.Year = &tt.page
			y, _ := f.graphsYear()
			c := newCanvas(f.Width, y.daysRows())
			y.days(c, 0)
			totals := strings.Join(textOf(c.rows(Look{}))[yearNumbersRow+1:], "\n")
			if strings.Contains(totals, "…") || !strings.Contains(totals, "Tue 22 Sep") || !strings.Contains(totals, tt.longest) {
				t.Errorf("%s at %d columns, YEAR's totals read\n%s\nwant every figure whole, the busiest day and the longest run, %s, among them", tt.name, width, totals, tt.longest)
			}
		}
	}
}

// BY MONTH is History's, of the same page: each month's row the same to its
// bars at every width, but its bars take the width left, the largest
// month's ending as far short of the edge as a month's name starts in from
// the left, where History's stop at the house's most.
func TestTrendsByMonthIsHistorysItsBarsTakingTheWidthLeft(t *testing.T) {
	for _, width := range []int{173, 108, 73} {
		trends, _ := trendsFrame(t, width, 60, "months").graphsYear()
		history, _ := yearFrame(t, width, 60, views.YearAsk{}, trendsYear()).yearDrawn()
		ours, theirs := newCanvas(width, trends.height()), newCanvas(width, history.height())
		trends.months(ours, 0)
		history.months(theirs, 0)
		a, b := textOf(ours.rows(Look{})), textOf(theirs.rows(Look{}))
		longest := 0
		for i := range trends.page.ByMonth {
			row, was := a[trends.laid.byMonth.first+i], b[history.laid.byMonth.first+i]
			barX := trends.form.barX
			if row[:barX] != was[:barX] {
				t.Errorf("at %d columns, BY MONTH's row reads %q, want History's %q to its bars", width, row[:barX], was[:barX])
			}
			if !strings.Contains(row, "so far") {
				longest = max(longest, ansi.StringWidth(row))
			}
		}
		if want := width - byMonthNameX; longest != want || history.laid.bar > byMonthBarMost {
			t.Errorf("at %d columns, BY MONTH's longest bar ends at %d, History's taking %d cells; want it to end at %d, History's at %d at the most",
				width, longest, history.laid.bar, want, byMonthBarMost)
		}
	}
}

// MODELS BY DAY is Days' first part, of the same page: each family's days in
// the cells History's Days draws them in, its names' column its families'
// alone, BY ACCOUNT's names left out; its key Days' but for BY ACCOUNT's
// mark, a row, wide or narrow.
func TestTrendsModelsByDayIsDaysFamiliesStrips(t *testing.T) {
	tests := []struct {
		width int
		key   string
	}{
		{width: 173, key: "less                more  each shade a quarter of the active days    ·· days to come"},
		{width: 73, key: "less                more   ·· days to come"},
	}
	for _, tt := range tests {
		f := trendsFrame(t, tt.width, 61, "models")
		history := daysFrame(t, *f.Graphs.Days, tt.width, 61)
		ours, ok := f.modelsLaid(0)
		if !ok {
			t.Fatalf("at %d columns, MODELS BY DAY lays nothing out", tt.width)
		}
		theirs := history.layOutDays(*history.History.Days, 0)
		c, h := newCanvas(f.Width, ours.keyRow+1), newCanvas(f.Width, history.viewRows())
		f.modelsDrawn(c, f.pastLaid(), 0)
		history.historyDays(h, 0)
		for i := range ours.page.ByModel {
			for day := max(ours.first, theirs.first); day < len(ours.page.ByModel[i].Days); day++ {
				got := c.at(ours.dayAt(day), ours.stripRow(ours.familyRow(i)))
				want := h.at(theirs.dayAt(day), theirs.stripRow(theirs.familyRow(i)))
				if got.glyph != want.glyph || got.ink != want.ink {
					t.Fatalf("at %d columns, family %d's day %d is %q in %+v, want History's %q in %+v", tt.width, i, day, got.glyph, got.ink, want.glyph, want.ink)
				}
			}
		}
		if ours.narrow != (tt.width < 100) {
			t.Errorf("at %d columns, MODELS BY DAY is narrow %t", tt.width, ours.narrow)
		}
		if names := familyDot("sonnet", secondaryInk).width() + nameGap; !ours.narrow && ours.row.labels.Size != names {
			t.Errorf("at %d columns, the names' column is %d cells, want %d, the families' alone", tt.width, ours.row.labels.Size, names)
		}
		if got := textOf(c.rows(Look{}))[ours.keyRow]; got != tt.key {
			t.Errorf("at %d columns, the key reads %q, want %q", tt.width, got, tt.key)
		}
	}
}

// Each of the new sections' headings carries its key at the right of its
// rule; BY MONTH's and MODELS BY DAY's say what they show where it fits
// whole, and nothing where it doesn't, as BY MONTH's at 73.
func TestTrendsNewSectionsHeadingsCarryTheirKeys(t *testing.T) {
	tests := []struct {
		width int
		want  map[GraphsSection]string
	}{
		{width: 173, want: map[GraphsSection]string{
			GraphsYear:   "YEAR  " + strings.Repeat("─", 165) + " y",
			GraphsMonths: "BY MONTH   worth at API prices; against what the plans cost that month " + strings.Repeat("─", 100) + " m",
			GraphsModels: "MODELS BY DAY   every account, weighed at API prices, on one scale " + strings.Repeat("─", 104) + " d",
		}},
		{width: 73, want: map[GraphsSection]string{
			GraphsYear:   "YEAR  " + strings.Repeat("─", 65) + " y",
			GraphsMonths: "BY MONTH  " + strings.Repeat("─", 61) + " m",
			GraphsModels: "MODELS BY DAY   every account, weighed at API prices, on one scale ──── d",
		}},
	}
	for _, tt := range tests {
		f := trendsFrame(t, tt.width, 61, everyTrend...)
		whole := newCanvas(f.Width, f.graphsLaid().rows)
		f.graphsDrawn(whole, f.Moment(threeRouted(), now), f.graphsLaid())
		text := textOf(whole.rows(Look{}))
		for s, want := range tt.want {
			if got := text[pastPlaced(t, f, s).at]; got != want {
				t.Errorf("at %d columns, %s's heading reads\n%q\nwant\n%q", tt.width, s.heading(), got, want)
			}
		}
	}
}

// Before their pages are built, YEAR, BY MONTH and MODELS BY DAY are each a
// heading over a blank row, and BY MONTH's says nothing yet.
func TestTrendsNewSectionsWaitForTheirPages(t *testing.T) {
	f := graphsFrame(173, 53, GraphsSectionsNamed([]string{"year", "months", "models"}))
	rows := textOf(f.Draw(threeRouted(), now))
	top := f.top()
	want := []string{
		"YEAR  " + strings.Repeat("─", 165) + " y", "", "",
		"",
		"BY MONTH  " + strings.Repeat("─", 161) + " m", "", "",
		"",
		"MODELS BY DAY   every account, weighed at API prices, on one scale " + strings.Repeat("─", 104) + " d", "", "",
	}
	if got := rows[top : top+len(want)]; !slices.Equal(got, want) {
		t.Errorf("before the pages are built, the view reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Scrolled to each section's heading, the view shows it on its first row, no
// further than shows the last, over the legend, held at the view's foot as
// Graphs holds it, the state row saying what's out of view; and a page never
// ends on a heading without the first row under it, as MODELS BY DAY's at
// 108, whose row over the legend is blank in its place.
func TestTrendsScrollsToEachSectionOverTheLegend(t *testing.T) {
	legend := "▆▆ room left   ⠂⠄⡀ where it's heading   ✕ runs out"
	tests := []struct {
		name          string
		width, height int
		to            GraphsSection
		top, state    string
	}{
		{name: "at its top", width: 173, height: 53, to: GraphsWeek, top: "WEEK ", state: "▼ 46 more below"},
		{name: "scrolled to POOL", width: 173, height: 53, to: GraphsPool, top: "POOL ", state: "▲ 20 more above · ▼ 26 more below"},
		{name: "scrolled to YEAR, past the page's foot", width: 173, height: 53, to: GraphsYear, top: "", state: "▲ 46 more above"},
		{name: "scrolled to YEAR at 108", width: 108, height: 44, to: GraphsYear, top: "YEAR ", state: "▲ 59 more above · ▼ 14 more below"},
		{name: "scrolled to BY MONTH on a phone, past the page's foot", width: 73, height: 61, to: GraphsMonths, top: "", state: "▲ 69 more above"},
	}
	for _, tt := range tests {
		f := trendsFrame(t, tt.width, tt.height, everyTrend...)
		f.Scroll = pastPlaced(t, f, tt.to).at
		text, laid := textOf(f.Draw(threeRouted(), now)), f.graphsLaid()
		foot := f.stateRow()
		if !strings.HasPrefix(text[f.top()], tt.top) || !strings.HasPrefix(text[f.top()+laid.room], legend) || text[foot] != tt.state {
			t.Errorf("%s: the view's top reads %q, the row under its sections %q, the state row %q; want %q first, the legend, then %q",
				tt.name, text[f.top()], text[f.top()+laid.room], text[foot], tt.top, tt.state)
		}
	}
}

// YEAR, BY MONTH and MODELS BY DAY are laid out only while they show, YEAR's
// Year while YEAR or BY MONTH does; and a frame Built lays them out once,
// every look at it, or at a copy alike, sharing what it laid, where a copy
// of another width lays them afresh.
func TestTrendsLaysOutItsPastOnlyWhileItShowsAndOnceAFrame(t *testing.T) {
	for _, tt := range []struct {
		sections     []string
		year, models bool
	}{
		{sections: []string{"week", "when"}},
		{sections: []string{"months"}, year: true},
		{sections: []string{"models"}, models: true},
		{sections: everyTrend, year: true, models: true},
	} {
		if p := trendsFrame(t, 173, 53, tt.sections...).layPast(); p.yearOK != tt.year || p.modelsOK != tt.models {
			t.Errorf("with %q shown, YEAR's Year is laid out %t and MODELS BY DAY %t, want %t and %t", tt.sections, p.yearOK, p.modelsOK, tt.year, tt.models)
		}
	}
	columns := func(f Frame) *figureColumn { return &f.pastLaid().year.laid.columns[0] }
	built := trendsFrame(t, 173, 53, everyTrend...).Built(threeRouted(), now)
	alike, wider := built, built
	alike.Scroll, wider.Width = 3, 180
	first := columns(built)
	again, copied, widened := columns(built) == first, columns(alike) == first, columns(wider) == first
	if !again || !copied || widened {
		t.Errorf("a frame Built shares its Year's layout across its looks %t, with a copy alike %t, with one wider %t; want true, true and false", again, copied, widened)
	}
}

// Trends is the tab's name, in the title row, its key and its place as
// Graphs' were.
func TestTrendsIsTheTabsName(t *testing.T) {
	f := graphsFrame(173, 53, GraphsSections{})
	if Graphs.Title() != "Trends" || !strings.Contains(textOf(f.Draw(threeRouted(), now))[0], "   Runway   Trends   Log") || Views()[6] != Graphs {
		t.Errorf("the tab is titled %q, the title row %q; want Trends, after Runway", Graphs.Title(), textOf(f.Draw(threeRouted(), now))[0])
	}
}
