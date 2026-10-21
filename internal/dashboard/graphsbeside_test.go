package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
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

func TestTheChartsBesideABlockTake5RowsWhereTheRowsAllowElseTodays2StackedOrItsRowsAndTheRowAfter(t *testing.T) {
	tests := []struct {
		name           string
		width, windows int
		roomy          bool
		// axis is the week chart's axis row; under, where the 5-hour's chart
		// is stacked under it, true; blank, the row parting work's charts
		// from personal's block, on the row after.
		axis, blank int
		under       bool
	}{
		{name: "220, two windows, rows to spare: 5 rows of chart", width: 220, windows: 2, roomy: true, axis: 6, blank: 7},
		{name: "220, two windows, rows short: 2 rows of chart", width: 220, windows: 2, axis: 3, blank: 4},
		{name: "220, three windows, rows to spare: 5 rows of chart", width: 220, windows: 3, roomy: true, axis: 6, blank: 7},
		{name: "220, three windows, rows short: 4 rows of chart, its block's rows and one", width: 220, windows: 3, axis: 5, blank: 6},
		{name: "160, two windows stacked, rows to spare: 5 rows a chart", width: 160, windows: 2, roomy: true, axis: 6, under: true, blank: 14},
		{name: "160, two windows stacked, rows short: 2 rows a chart", width: 160, windows: 2, axis: 3, under: true, blank: 8},
		{name: "160, three windows stacked, rows short: 2 rows a chart", width: 160, windows: 3, axis: 3, under: true, blank: 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := besideOf(tt.width)
			f.Burn, f.Models = BurnDown, tt.windows == 3
			usage := f.usageSection(trio(), printNow)
			rows := usage.least - sectionHead
			if tt.roomy {
				rows = usage.most - sectionHead
			}
			drawn, _ := usageBody(f, trio(), printNow, rows)
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
		{accounts: 10, scroll: 2, rows: 22, shown: map[string]int{"a3": 1, "a4": 6, "a5": 11}},
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

func TestBesideAreAsManyChartsAsTakeTheirLeastSideBySideSharingTheRoom(t *testing.T) {
	for _, tt := range []struct{ k, room, want, width int }{
		{k: 3, room: 29, want: 0}, {k: 3, room: 30, want: 1, width: 30}, {k: 3, room: 61, want: 1, width: 61},
		{k: 3, room: 62, want: 2, width: 30}, {k: 3, room: 93, want: 2, width: 45}, {k: 3, room: 94, want: 3, width: 30},
		{k: 3, room: 200, want: 3, width: 65}, {k: 0, room: 200, want: 0},
	} {
		spans := chartsFitting(tt.k, tt.room, besideGap)
		for i, s := range spans {
			if s.Size != tt.width || s.At != i*(tt.width+besideGap) {
				t.Errorf("of %d charts in %d cells, chart %d lies %+v, want %d wide, %d apart", tt.k, tt.room, i, s, tt.width, besideGap)
			}
		}
		if len(spans) != tt.want {
			t.Errorf("of %d charts, %d fit in %d cells, want %d", tt.k, len(spans), tt.room, tt.want)
		}
	}
}
