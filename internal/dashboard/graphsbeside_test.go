package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// besideOf is a frame width columns wide, tall enough for USAGE by account
// to show every account, GRAPHS shown.
func besideOf(width int) Frame {
	f := frameOf(width, 120)
	f.By, f.Sections = ByAccount, Sections{}.Toggled(SectionGraphs)
	return f
}

// titlesRow is the top row of charts of the windows named, each width cells
// wide, gap cells apart: each's name in capitals and the limit's line after
// it.
func titlesRow(width, gap int, names ...string) string {
	var titles []string
	for _, name := range names {
		titles = append(titles, name+" "+strings.Repeat(limitLine, width-len(name)-1))
	}
	return strings.Join(titles, strings.Repeat(" ", gap))
}

func TestGRAPHSByAccountChartsEveryWindowBesideEachBlockAsManyToARowAsFitTheRestUnder(t *testing.T) {
	windows := []string{"WEEK", "5-HOUR", "FABLE WEEK"}
	tests := []struct {
		width, windows int
		perRow, wide   int
	}{
		{width: 150, windows: 2, perRow: 1, wide: 42},
		{width: 160, windows: 2, perRow: 1, wide: 52},
		{width: 170, windows: 2, perRow: 2, wide: 30},
		{width: 173, windows: 2, perRow: 2, wide: 31},
		{width: 202, windows: 2, perRow: 2, wide: 46},
		{width: 220, windows: 2, perRow: 2, wide: 55},
		{width: 150, windows: 3, perRow: 1, wide: 42},
		{width: 160, windows: 3, perRow: 1, wide: 52},
		{width: 170, windows: 3, perRow: 2, wide: 30},
		{width: 173, windows: 3, perRow: 2, wide: 31},
		{width: 202, windows: 3, perRow: 3, wide: 30},
		{width: 220, windows: 3, perRow: 3, wide: 36},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d columns, %d windows", tt.width, tt.windows), func(t *testing.T) {
			f := besideOf(tt.width)
			f.Models = tt.windows == 3
			rows, _ := usageBody(f, trio(), printNow, f.usageSection(trio(), printNow).most-sectionHead)
			// At USAGE's most, each row of charts takes 7 rows, and a blank row
			// parts one block's charts from the next block.
			chartRows := (tt.windows + tt.perRow - 1) / tt.perRow
			block := chartRows*(leastChart+chartEnds) + 1
			for b := range 3 {
				for r := range chartRows {
					y := b*block + r*(leastChart+chartEnds)
					want := titlesRow(tt.wide, besideGap, windows[r*tt.perRow:min((r+1)*tt.perRow, tt.windows)]...)
					if got := strings.TrimRight(colAt(rows[y], 107, tt.width-107), " "); got != want {
						t.Errorf("beside block %d, its charts' row %d, on row %d, reads\n%q\nwant\n%q", b+1, r+1, y, got, want)
					}
				}
			}
		})
	}
	if f := besideOf(149); f.graphsBeside() || f.graphsSection(trio(), printNow).least == 0 {
		t.Errorf("at 149 columns, GRAPHS is beside USAGE's blocks, want it a section of its own, as by window")
	}
}

// usageRowsIn are the rows USAGE takes by account, f's charts beside its
// blocks in the form given: at its least, stacked, or at its most.
func usageRowsIn(f Frame, form besideForm) int {
	v := f.overviewBlock(trio(), printNow).views()
	return [...]int{besideToday: v.all, besideStacked: v.stacked, besideTall: v.big}[form]
}

func TestTheChartsBesideABlockAreTodaysAtUSAGEsLeastThenStackedThen5RowsAChartAsTheRowsAllow(t *testing.T) {
	tests := []struct {
		name           string
		width, windows int
		form           besideForm
		// axis is the week chart's axis row; under, where the 5-hour's chart
		// is stacked under it, true; blank, the row parting work's charts
		// from personal's block, on the row after.
		axis, blank int
		under       bool
	}{
		{name: "220, two windows, least: 2 rows of chart", width: 220, windows: 2, axis: 3, blank: 4},
		{name: "220, two windows, most: 5 rows of chart", width: 220, windows: 2, form: besideTall, axis: 6, blank: 7},
		{name: "220, three windows, least: 4 rows of chart, its block's rows and one", width: 220, windows: 3, axis: 5, blank: 6},
		{name: "220, three windows, most: 5 rows of chart", width: 220, windows: 3, form: besideTall, axis: 6, blank: 7},
		{name: "160, two windows, least: as before, side by side, 2 rows of chart", width: 160, windows: 2, axis: 3, blank: 4},
		{name: "160, two windows, stacked: 2 rows a chart", width: 160, windows: 2, form: besideStacked, axis: 3, under: true, blank: 8},
		{name: "160, two windows, most: stacked, 5 rows a chart", width: 160, windows: 2, form: besideTall, axis: 6, under: true, blank: 14},
		{name: "160, three windows, least: as before, two side by side, 4 rows of chart", width: 160, windows: 3, axis: 5, blank: 6},
		{name: "160, three windows, stacked: 2 rows a chart", width: 160, windows: 3, form: besideStacked, axis: 3, under: true, blank: 12},
		{name: "160, three windows, most: stacked, 5 rows a chart", width: 160, windows: 3, form: besideTall, axis: 6, under: true, blank: 21},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := besideOf(tt.width)
			f.Burn, f.Models = BurnDown, tt.windows == 3
			drawn, _ := usageBody(f, trio(), printNow, usageRowsIn(f, tt.form))
			if got := colAt(drawn[tt.axis], 107, 9); got != "Sun 21:00" {
				t.Errorf("row %d beside work's block reads %q, want the week's axis, from its start, Sun 21:00", tt.axis, got)
			}
			if got := colAt(drawn[tt.axis+1], 107, 6); tt.under && got != "5-HOUR" {
				t.Errorf("row %d beside work's block reads %q, want the 5-hour's chart stacked under the week's", tt.axis+1, got)
			}
			if got := colAt(drawn[tt.blank], 107, 113); strings.TrimSpace(got) != "" {
				t.Errorf("row %d, between work's block and personal's, reads %q beside them, want nothing", tt.blank, got)
			}
			if !strings.HasPrefix(drawn[tt.blank+1], "2 personal") {
				t.Errorf("row %d reads %q, want personal's block", tt.blank+1, drawn[tt.blank+1])
			}
		})
	}
}

func TestByAccountAt160USAGEsLeastIsAsBeforeAndItGrowsWholeToTheChartsStackedThenTo5RowsAChart(t *testing.T) {
	tests := []struct {
		name    string
		windows int
		height  int
		form    besideForm
		folds   []Section
		pool    []PoolChart
	}{
		{name: "two windows, 37 rows: as before the slice, every account and ROUTING", windows: 2, height: 37,
			folds: []Section{SectionComing, SectionLately}, pool: []PoolChart{{Key: "7d", Columns: 25}, {Key: "5h", Columns: 25}}},
		{name: "two windows, 60 rows: stacked", windows: 2, height: 60, form: besideStacked,
			pool: []PoolChart{{Key: "7d", Columns: 52}, {Key: "5h", Columns: 52}}},
		{name: "two windows, 90 rows: 5 rows a chart", windows: 2, height: 90, form: besideTall,
			pool: []PoolChart{{Key: "7d", Columns: 52}, {Key: "5h", Columns: 52}}},
		{name: "three windows, 50 rows: as before, two side by side", windows: 3, height: 50,
			pool: []PoolChart{{Key: "7d", Columns: 25}, {Key: "5h", Columns: 25}}},
		{name: "three windows, 75 rows: stacked, every window", windows: 3, height: 75, form: besideStacked,
			pool: []PoolChart{{Key: "7d", Columns: 52}, {Key: "5h", Columns: 52}, {Key: "7d_oi", Columns: 52}}},
		{name: "three windows, 110 rows: 5 rows a chart", windows: 3, height: 110, form: besideTall,
			pool: []PoolChart{{Key: "7d", Columns: 52}, {Key: "5h", Columns: 52}, {Key: "7d_oi", Columns: 52}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := besideOf(160)
			f.Height, f.Models = tt.height, tt.windows == 3
			laid := f.laidOut(trio(), printNow, f.top())
			if got := folds(laid); !slices.Equal(got, tt.folds) {
				t.Errorf("%v fold, want %v", got, tt.folds)
			}
			if got, want := rowsGiven(laid)[SectionUsage], sectionHead+usageRowsIn(f, tt.form); got != want {
				t.Errorf("USAGE takes %d rows, want %d, whole, its form %d", got, want, tt.form)
			}
			if got := f.usageDrawn(trio(), printNow).beside; got != tt.form {
				t.Errorf("the charts beside USAGE's blocks are drawn in form %d, want %d", got, tt.form)
			}
			if got := f.PoolCharts(trio(), printNow); !slices.Equal(got, tt.pool) {
				t.Errorf("the pool's charts are %v, want %v", got, tt.pool)
			}
		})
	}
}

// Scrolled, the charts beside USAGE's blocks by account are drawn in the
// rows it shows alone: a block shown in part has its charts in its rows
// shown, and the row saying what's out of view under them has none beside
// it. Ten accounts' blocks, five rows apart from row 0, in 22 rows,
// scrolled 2: a5's first row on the page's 18th, its charts' titles beside
// it, the page's last two rows its own, cut there.
func TestByAccountScrolledTheChartsBesideAreDrawnInTheRowsShownAlone(t *testing.T) {
	f := besideOf(160)
	f.Scroll = 2
	rows, _ := usageBody(f, manyRead(10), now, 22)
	if !strings.Contains(rows[18], "5 a5") || strings.TrimSpace(colAt(rows[18], 107, 53)) == "" {
		t.Errorf("row 18 reads %q, want a5's first row, its charts beside it:\n%s", rows[18], strings.Join(rows, "\n"))
	}
	if !strings.HasPrefix(rows[21], "▲ 2 more above") || strings.TrimSpace(colAt(rows[21], 107, 53)) != "" {
		t.Errorf("the last row reads %q, want what's out of view said, and nothing beside it:\n%s", rows[21], strings.Join(rows, "\n"))
	}
}

func TestByAccountUSAGEGrowsItsBlocksApartForTheChartsBesideThemAheadOfCOMINGUPAndLATELY(t *testing.T) {
	f := besideOf(173)
	f.Models, f.Height = false, 53
	given := rowsGiven(f.laidOut(trio(), printNow, f.top()))
	usage := f.usageSection(trio(), printNow)
	if given[SectionUsage] != usage.most || given[SectionComing] != 5 {
		t.Errorf("at 173 × 53, USAGE takes %d rows and COMING UP %d, want USAGE at its most, %d, its blocks apart for the charts, and COMING UP at its least, 5",
			given[SectionUsage], given[SectionComing], usage.most)
	}
}

func TestWhileGRAPHSIsBesideUSAGEsBlocksROUTINGIsASectionOfItsOwnAndThereIsNoFocus(t *testing.T) {
	tests := []struct {
		name    string
		f       Frame
		beside  bool
		routing int
	}{
		{name: "by account from 150 columns", f: besideOf(160), beside: true, routing: 3 + 3},
		{name: "by account, GRAPHS hidden: ROUTING beside", f: func() Frame { f := besideOf(160); f.Sections = Sections{}; return f }()},
		{name: "by account under 150 columns", f: besideOf(149), routing: 3 + 3},
		{name: "by window", f: func() Frame { f := besideOf(160); f.By = ByWindow; return f }(), routing: 3 + 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.graphsBeside(); got != tt.beside {
				t.Errorf("GRAPHS beside USAGE's blocks: %v, want %v", got, tt.beside)
			}
			if got := tt.f.Focuses(); got == tt.beside {
				t.Errorf("the Overview focuses: %v, want %v", got, !tt.beside)
			}
			if got := tt.f.routingSection(trio(), printNow).least; got != tt.routing {
				t.Errorf("ROUTING takes %d rows at its least, want %d", got, tt.routing)
			}
		})
	}
	g := besideOf(160).graphsSection(trio(), printNow)
	if got, want := g.description(), burnKey(BurnUp); g.least != 0 || !slices.Equal(got, want) {
		t.Errorf("beside, GRAPHS takes %d rows, its heading %+v; want none, and the key alone, as no account is focused", g.least, got)
	}
}

func TestGRAPHSBesideUSAGEChartsEveryAccountShownAsItsBlocksScroll(t *testing.T) {
	tests := []struct {
		accounts, scroll, rows int
		// shown are the rows each account's block starts on, by its id.
		shown map[string]int
	}{
		{accounts: 1, rows: 40, shown: map[string]int{"a1": 0}},
		{accounts: 4, rows: 40, shown: map[string]int{"a1": 0, "a2": 8, "a3": 16, "a4": 24}},
		{accounts: 10, scroll: 10, rows: 22, shown: map[string]int{"a3": 0, "a4": 5, "a5": 10, "a6": 15}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d accounts", tt.accounts), func(t *testing.T) {
			f := besideOf(173)
			f.Scroll = tt.scroll
			doc := manyRead(tt.accounts)
			rows, _ := usageBody(f, doc, now, tt.rows)
			want := titlesRow(31, besideGap, "WEEK", "5-HOUR")
			for id, y := range tt.shown {
				if !strings.Contains(rows[y], id) || colAt(rows[y], 107, 64) != want {
					t.Errorf("row %d reads %q, want %s's block, its charts beside it from column 107", y, rows[y], id)
				}
			}
		})
	}
}

func TestBesideAreAsManyChartsAsTakeTheirLeastSideBySideSharingTheRoomTheRestUnderOrLeftOffAtTheLeast(t *testing.T) {
	type fit struct{ k, room, perRow, width, shown int }
	tests := []struct {
		form besideForm
		fits []fit
	}{
		{form: besideToday, fits: []fit{
			{3, 23, 0, 0, 0}, {3, 24, 1, 24, 1}, {3, 49, 1, 49, 1}, {3, 50, 2, 24, 2}, {3, 75, 2, 36, 2}, {3, 76, 3, 24, 3}, {3, 200, 3, 65, 3}, {0, 200, 0, 0, 0},
		}},
		{form: besideStacked, fits: []fit{
			{3, 30, 1, 30, 3}, {3, 61, 1, 61, 3}, {3, 62, 2, 30, 3}, {3, 93, 2, 45, 3}, {3, 94, 3, 30, 3}, {3, 200, 3, 65, 3}, {0, 200, 0, 0, 0},
		}},
		{form: besideTall, fits: []fit{{3, 62, 2, 30, 3}}},
	}
	for _, tt := range tests {
		for _, f := range tt.fits {
			across := tt.form.across(f.k, f.room)
			for i, s := range across.spans {
				if s.Size != f.width || s.At != i*(f.width+besideGap) {
					t.Errorf("form %d: of %d charts in %d cells, chart %d lies %+v, want %d wide, %d apart", tt.form, f.k, f.room, i, s, f.width, besideGap)
				}
			}
			if len(across.spans) != f.perRow || across.shown(f.k) != f.shown {
				t.Errorf("form %d: of %d charts in %d cells, %d to a row and %d shown, want %d and %d", tt.form, f.k, f.room, len(across.spans), across.shown(f.k), f.perRow, f.shown)
			}
		}
	}
}

// Where the charts stacked take no more rows than today's, the stacked form
// is no step of its own, so USAGE at its least draws today's: with four
// windows, side by side at 202 and 220 columns, as many rows of chart as the
// block's 7 rows and the row after give, 6, not the stacked form's 2.
func TestWithFourWindowsUSAGEsLeastDrawsTodaysChartsNotTheStackedForm(t *testing.T) {
	sonnet := func(a *status.Account) {
		a.Windows = append(a.Windows, quota.Window{Key: "7d_sonnet", Label: "Sonnet week", Utilization: 0.1, ResetsAt: on(4, 21, 0).UTC()})
	}
	four := trio(account("work", sonnet), account("personal", sonnet), account("side", sonnet))
	for _, width := range []int{202, 220} {
		f := besideOf(width)
		f.Height, f.Burn = 52, BurnDown
		if got := f.usageDrawn(four, printNow).beside; got != besideToday {
			t.Errorf("at %d × 52, the charts beside USAGE's blocks are drawn in form %d, want today's, %d", width, got, besideToday)
		}
		rows, _ := usageBody(f, four, printNow, f.usageSection(four, printNow).least-sectionHead)
		if got := colAt(rows[3], 107, 9); got == "Sun 21:00" {
			t.Errorf("at %d columns, row 3 beside work's block reads %q, the stacked form's axis, want the week's chart", width, got)
		}
		if got := colAt(rows[7], 107, 9); got != "Sun 21:00" {
			t.Errorf("at %d columns, row 7 beside work's block reads %q, want the week's axis under 6 rows of chart", width, got)
		}
	}
}
