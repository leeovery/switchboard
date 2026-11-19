package dashboard

import (
	"slices"
	"strings"
	"testing"
)

// COMING UP and LATELY take their short form, their heading and their first
// two lines in three rows, in the rows a fold leaves where those hold it and
// their least doesn't fit; where even three rows aren't there, they fold.
// Side by side, LATELY takes its short form beside COMING UP's, in the same
// rows. Stacked, COMING UP takes it first, and LATELY opens only in what's
// left, a blank row between them. USAGE takes 13 rows here, then a blank
// row; neither short form ever grows.
func TestCOMINGUPAndLATELYTakeTheirShortFormWhereTheRowsAFoldLeavesHoldIt(t *testing.T) {
	sections := []section{sized(SectionUsage, 13, 13), sized(SectionComing, 5, 9), sized(SectionLately, 5, 14)}
	type shown struct {
		rows        int
		fold, short bool
	}
	folded, short := shown{rows: 1, fold: true}, shown{rows: 3, short: true}
	tests := []struct {
		name           string
		width, rows    int
		coming, lately shown
		last           int
	}{
		{name: "side by side, a row for them: folded", width: 160, rows: 15, coming: folded, lately: folded, last: 15},
		{name: "side by side, two rows spare, short of three: folded, a row blank", width: 160, rows: 16, coming: folded, lately: folded, last: 15},
		{name: "side by side, exactly three rows spare: their short form", width: 160, rows: 17, coming: short, lately: short, last: 17},
		{name: "side by side, four rows spare: their short form, a row blank", width: 160, rows: 18, coming: short, lately: short, last: 17},
		{name: "side by side, five rows spare: their least, never their short form", width: 160, rows: 19, coming: shown{rows: 5}, lately: shown{rows: 5}, last: 19},
		{name: "stacked, both folded, no room for COMING UP's short form and the blank rows round it", width: 73, rows: 18, coming: folded, lately: folded, last: 16},
		{name: "stacked, COMING UP's short form first, LATELY folded under it", width: 73, rows: 19, coming: short, lately: folded, last: 19},
		{name: "stacked, COMING UP's short form, a row blank", width: 73, rows: 20, coming: short, lately: folded, last: 19},
		{name: "stacked, COMING UP's least, never its short form", width: 73, rows: 21, coming: shown{rows: 5}, lately: folded, last: 21},
		{name: "stacked, COMING UP's least, LATELY's short form in what's left", width: 73, rows: 23, coming: shown{rows: 5}, lately: short, last: 23},
		{name: "stacked, each at its least", width: 73, rows: 25, coming: shown{rows: 5}, lately: shown{rows: 5}, last: 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			laid := planOf(frameOf(tt.width, 60), sections, 0, tt.rows)
			given, shorts := rowsGiven(laid), shortened(laid)
			for _, s := range []struct {
				section Section
				want    shown
			}{{SectionComing, tt.coming}, {SectionLately, tt.lately}} {
				p := laid[slices.IndexFunc(laid, func(p planned) bool { return p.Section == s.section })]
				got := shown{rows: given[s.section], fold: p.fold, short: slices.Contains(shorts, s.section)}
				if got != s.want {
					t.Errorf("in %d rows, %s takes %+v, want %+v", tt.rows, s.section.heading(), got, s.want)
				}
			}
			if got := slices.MaxFunc(laid, func(a, b planned) int { return a.lastRow() - b.lastRow() }).lastRow() + 1; got != tt.last {
				t.Errorf("in %d rows, the last section ends on row %d, want %d", tt.rows, got, tt.last)
			}
		})
	}
}

// The short form draws its heading, its key at its end and, where it holds
// more lines than two, the ellipsis its folded heading has; then, straight
// under it, its first two lines, each as the full list draws it at its
// least, in its columns: so, with LATELY's twelve lines on auto, at 160
// columns, COMING UP and LATELY side by
// side, at 160x29 against 160x30, and at 73 columns COMING UP stacked, at
// 73x36 against 73x37.
func TestTheShortFormDrawsItsHeadingThenItsFirstTwoLinesAsItsLeastDoes(t *testing.T) {
	doc, sessions, stream := overviewWorld()
	doc = withEvents(doc, onAuto()...)
	drawn := func(width, height int) []string {
		f := Frame{Width: width, Height: height, View: Overview, Policy: claudeLike, Sessions: sessions, Stream: stream}
		return textOf(f.Draw(doc, routedAt))
	}
	tests := []struct {
		name         string
		width        int
		short, least int
		headings     []string
	}{
		{name: "side by side", width: 160, short: 29, least: 30, headings: []string{"COMING UP   … ─", "LATELY   … ─"}},
		{name: "stacked", width: 73, short: 36, least: 37, headings: []string{"COMING UP   … ─"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			short, least := drawn(tt.width, tt.short), drawn(tt.width, tt.least)
			at := func(rows []string) int {
				return slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, "COMING UP ") })
			}
			s, l := at(short), at(least)
			if s < 0 || l < 0 {
				t.Fatalf("COMING UP's heading isn't drawn: at row %d short, %d at its least", s, l)
			}
			for _, heading := range tt.headings {
				if !strings.Contains(short[s], heading) {
					t.Errorf("the short form's heading reads %q, want it to hold %q", short[s], heading)
				}
			}
			if !strings.Contains(short[s]+" ", "─ c ") {
				t.Errorf("the short form's heading reads %q, want COMING UP's key, c, at its end", short[s])
			}
			if got, want := short[s+1:s+1+shortLines], least[l+sectionHead:l+sectionHead+shortLines]; !slices.Equal(got, want) {
				t.Errorf("the short form's lines read\n  %q\nwant the least's first two\n  %q", got, want)
			}
		})
	}
}

// The short form's heading says it holds more than its two lines where it
// does, with the folded heading's ellipsis, and nothing where it holds two
// or fewer.
func TestTheShortFormsHeadingSaysItHoldsMoreOnlyWhereItDoes(t *testing.T) {
	f := frameOf(160, 39)
	for _, s := range []Section{SectionComing, SectionLately} {
		for _, tt := range []struct {
			lines int
			want  string
		}{{lines: 0}, {lines: 2}, {lines: 3, want: "…"}, {lines: 12, want: "…"}} {
			short := listsShort(section{Section: s, lines: func() int { return tt.lines }}).leaned(givenNone + 1)
			if got := f.says(planned{section: short}, f.Width).plain(); got != tt.want {
				t.Errorf("%s in its short form, holding %d lines, says %q after its name, want %q", s.heading(), tt.lines, got, tt.want)
			}
		}
	}
}
