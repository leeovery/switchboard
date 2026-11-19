package dashboard

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

// COMING UP and LATELY, folded, open in their short form, their heading and
// their first two lines in three rows, as the plan's last step: only in the
// rows it leaves blank, where those hold the short form and the blank row
// before it; where they don't, they stay folded. Side by side, LATELY takes
// its short form beside COMING UP's, in the same rows. Stacked, COMING UP
// takes it first, and LATELY only in what's left. USAGE takes 13 rows here,
// then a blank row; neither short form ever grows.
func TestCOMINGUPAndLATELYOpenShortInTheRowsThePlanLeavesBlank(t *testing.T) {
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
		{name: "side by side, a row blank, short of the two more the short form takes: folded", width: 160, rows: 16, coming: folded, lately: folded, last: 15},
		{name: "side by side, exactly two rows blank: their short form", width: 160, rows: 17, coming: short, lately: short, last: 17},
		{name: "side by side, three rows blank: their short form, a row blank", width: 160, rows: 18, coming: short, lately: short, last: 17},
		{name: "side by side, their least fitting: their least, never their short form", width: 160, rows: 19, coming: shown{rows: 5}, lately: shown{rows: 5}, last: 19},
		{name: "stacked, two rows blank, short of COMING UP's short form and the blank rows round it", width: 73, rows: 18, coming: folded, lately: folded, last: 16},
		{name: "stacked, three rows blank: COMING UP's short form first, LATELY folded under it", width: 73, rows: 19, coming: short, lately: folded, last: 19},
		{name: "stacked, four rows blank: COMING UP's short form, a row blank", width: 73, rows: 20, coming: short, lately: folded, last: 19},
		{name: "stacked, COMING UP's least fitting: its least, never its short form", width: 73, rows: 21, coming: shown{rows: 5}, lately: folded, last: 21},
		{name: "stacked, COMING UP grown into the rows LATELY's short form would take: LATELY folded", width: 73, rows: 23, coming: shown{rows: 7}, lately: folded, last: 23},
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

// The short form takes only rows the plan leaves blank, never one it gives
// anything else: at 73x36, COMING UP stays folded and LATELY at its least,
// where both short would take its rows; at 73x40, COMING UP grows to six
// lines, LATELY folded; at 160x31 with every section, SESSIONS shows a
// session and how many more, COMING UP and LATELY folded; and where USAGE
// gives way further, a model's own week used, at 108x35, every other
// section folded, they open short in the rows its step leaves at the foot.
func TestTheShortFormTakesOnlyTheRowsThePlanLeavesBlank(t *testing.T) {
	doc, sessions, stream := overviewWorld()
	tests := []struct {
		name          string
		width, height int
		shown         Sections
		fable         bool
		rows          map[Section]int
		folds, short  []Section
	}{
		{
			name: "73x36", width: 73, height: 36,
			rows:  map[Section]int{SectionUsage: 20, SectionRouting: 1, SectionComing: 1, SectionLately: 5},
			folds: []Section{SectionRouting, SectionComing},
		},
		{
			name: "73x40", width: 73, height: 40,
			rows:  map[Section]int{SectionUsage: 20, SectionRouting: 1, SectionComing: 8, SectionLately: 1},
			folds: []Section{SectionRouting, SectionLately},
		},
		{
			name: "160x31, every section", width: 160, height: 31, shown: every,
			rows:  map[Section]int{SectionUsage: 11, SectionRouting: 6, SectionSessions: 4, SectionComing: 1, SectionLately: 1},
			folds: []Section{SectionComing, SectionLately},
		},
		{
			name: "108x35, a model's own week used", width: 108, height: 35, fable: true,
			rows:  map[Section]int{SectionUsage: 23, SectionRouting: 1, SectionComing: 3, SectionLately: 3},
			folds: []Section{SectionRouting}, short: []Section{SectionComing, SectionLately},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: tt.height, View: Overview, Policy: claudeLike, Sessions: sessions, Stream: stream, Sections: tt.shown}
			laid := f.laidOut(doc, routedAt, 0)
			if tt.fable {
				f = frameOf(tt.width, tt.height)
				laid = f.laidOut(threeFabled(), now, 0)
			}
			if got := rowsGiven(laid); !maps.Equal(got, tt.rows) {
				t.Errorf("the sections take %v, want %v", got, tt.rows)
			}
			if got := folds(laid); !slices.Equal(got, tt.folds) {
				t.Errorf("%v fold, want %v", got, tt.folds)
			}
			if got := shortened(laid); !slices.Equal(got, tt.short) {
				t.Errorf("%v take their short form, want %v", got, tt.short)
			}
		})
	}
}

// The short form draws its heading, its key at its end and, where it holds
// more lines than two, the ellipsis its folded heading has, else nothing;
// then, straight under it, its first two lines as the list draws two. With a
// model's own week used, COMING UP has more than two things and LATELY none,
// and its third thing's time is no wider than the first two's, so its lines
// read as its least's first two: side by side at 116x44 against 116x41, and
// COMING UP stacked at 73x43 against 73x45.
func TestTheShortFormDrawsItsHeadingThenItsFirstTwoLines(t *testing.T) {
	doc := threeFabled()
	drawn := func(width, height int) []string { return textOf(frameOf(width, height).Draw(doc, now)) }
	tests := []struct {
		name         string
		width        int
		short, least int
		holds, lacks []string
	}{
		{name: "side by side", width: 116, short: 44, least: 41, holds: []string{"COMING UP   … ─", "─ c ", "LATELY  ─"}, lacks: []string{"LATELY   …"}},
		{name: "stacked", width: 73, short: 43, least: 45, holds: []string{"COMING UP   … ─", "─ c "}},
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
			heading := short[s] + " "
			for _, holds := range tt.holds {
				if !strings.Contains(heading, holds) {
					t.Errorf("the short form's heading reads %q, want it to hold %q", short[s], holds)
				}
			}
			for _, lacks := range tt.lacks {
				if strings.Contains(heading, lacks) {
					t.Errorf("the short form's heading reads %q, want it without %q", short[s], lacks)
				}
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
			short := listsShort(section{Section: s, lines: func() int { return tt.lines }}).leaned(givenShort)
			if got := f.says(planned{section: short}, f.Width).plain(); got != tt.want {
				t.Errorf("%s in its short form, holding %d lines, says %q after its name, want %q", s.heading(), tt.lines, got, tt.want)
			}
		}
	}
}
