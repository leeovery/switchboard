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

func TestGRAPHSByAccountChartsTheWindowsThatFitBesideEachBlock(t *testing.T) {
	windows := []string{"5-HOUR", "WEEK", "FABLE WEEK"}
	tests := []struct {
		width     int
		fit, wide int
	}{
		{width: 150, fit: 1, wide: 42},
		{width: 157, fit: 1, wide: 49},
		{width: 158, fit: 2, wide: 24},
		{width: 160, fit: 2, wide: 25},
		{width: 183, fit: 2, wide: 36},
		{width: 184, fit: 3, wide: 24},
		{width: 220, fit: 3, wide: 36},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d columns", tt.width), func(t *testing.T) {
			f := besideOf(tt.width)
			rows, _ := usageBody(f, trio(), printNow, 40)
			want := titlesRow(tt.wide, besideGap, windows[:tt.fit]...)
			for _, y := range []int{0, 7, 14} {
				if got := colAt(rows[y], 107, tt.width-107); got != want {
					t.Errorf("beside the block on row %d, the charts' top row reads\n%q\nwant\n%q", y, got, want)
				}
			}
		})
	}
	if f := besideOf(149); f.graphsBeside() || f.graphsSection(trio(), printNow).least == 0 {
		t.Errorf("at 149 columns, GRAPHS is beside USAGE's blocks, want it a section of its own, as by window")
	}
}

func TestTheChartsBesideABlockTakeItsRowsAndTheRowAfterForTheirAxis(t *testing.T) {
	f := besideOf(220)
	f.Burn = BurnDown
	rows, _ := usageBody(f, trio(), printNow, 40)
	// work's block takes rows 0 to 4: the charts' top row on its first, four
	// rows of chart, and their axis on row 5, under its last.
	if got := colAt(rows[5], 107, 5); got != "12:10" {
		t.Errorf("row 5 beside work's block reads %q, want the 5-hour's axis, from its start, 12:10", got)
	}
	if got := colAt(rows[6], 107, 113); strings.TrimSpace(got) != "" {
		t.Errorf("row 6, between work's block and personal's, reads %q beside them, want nothing", got)
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
		{accounts: 4, rows: 40, shown: map[string]int{"a1": 0, "a2": 5, "a3": 10, "a4": 15}},
		{accounts: 10, scroll: 2, rows: 22, shown: map[string]int{"a3": 1, "a4": 6, "a5": 11}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d accounts", tt.accounts), func(t *testing.T) {
			f := besideOf(160)
			f.Scroll = tt.scroll
			doc := manyRead(tt.accounts)
			rows, _ := usageBody(f, doc, now, tt.rows)
			want := titlesRow(25, besideGap, "5-HOUR", "WEEK")
			for id, y := range tt.shown {
				if !strings.Contains(rows[y], id) || colAt(rows[y], 107, 53) != want {
					t.Errorf("row %d reads %q, want %s's block, its charts beside it from column 107", y, rows[y], id)
				}
			}
		})
	}
}

func TestBesideAreAsManyChartsAsTakeTheirLeastSideBySideSharingTheRoom(t *testing.T) {
	for _, tt := range []struct{ k, room, want, width int }{
		{k: 3, room: 23, want: 0, width: 0}, {k: 3, room: 24, want: 1, width: 24}, {k: 3, room: 49, want: 1, width: 49},
		{k: 3, room: 50, want: 2, width: 24}, {k: 3, room: 75, want: 2, width: 36}, {k: 3, room: 76, want: 3, width: 24},
		{k: 3, room: 200, want: 3, width: 65}, {k: 0, room: 200, want: 0, width: 0},
	} {
		if got, width := besideLayout(tt.k, tt.room); got != tt.want || width != tt.width {
			t.Errorf("of %d charts, %d fit in %d cells, %d wide, want %d, %d wide", tt.k, got, tt.room, width, tt.want, tt.width)
		}
	}
}
