package dashboard

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"unique"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// stripOf is a strip's eight weeks' days at the levels the digits give, a
// digit a day from the first, those past them at level 0.
func stripOf(levels string) []views.StripDay {
	days := make([]views.StripDay, 56)
	for i, level := range levels {
		days[i].Level = int(level - '0')
	}
	return days
}

// busy are a strip's days but the three to come, each active.
var busy = strings.Repeat("1234", 13) + "1"

// daysWorld is MODELS BY DAY as trends-models draws it: the eight weeks
// from 10 August, Friday to Sunday to come, and the four families' shares.
func daysWorld() views.DaysPage {
	return daysShared([4]float64{0.59, 0.37, 0.02, 0.02})
}

// daysShared is daysWorld with the four families' shares given, Opus's to
// Haiku's.
func daysShared(shares [4]float64) views.DaysPage {
	families := make([]views.DaysStrip, len(shares))
	for i, family := range []string{"opus", "sonnet", "fable", "haiku"} {
		families[i] = views.DaysStrip{Of: family, Days: stripOf(busy), Share: &shares[i]}
	}
	weeks := []string{"2026-08-10", "2026-08-17", "2026-08-24", "2026-08-31", "2026-09-07", "2026-09-14", "2026-09-21", "2026-09-28"}
	return views.DaysPage{Weeks: weeks, ToCome: 3, ByModel: families}
}

// modelsFrame is a frame of Trends, MODELS BY DAY shown, of the page
// given, in a terminal of the size given, in nord.
func modelsFrame(t *testing.T, page views.DaysPage, width, height int) Frame {
	t.Helper()
	f := graphsFrame(width, height, GraphsSectionsNamed([]string{"models"}))
	f.Look = Screen(builtin(t, "nord"))
	f.Graphs.Days, f.Graphs.Read = &page, true
	return f
}

// modelsShown is MODELS BY DAY as the frame lays it out from row 0, and
// drawn there on a canvas of its own, failing the test where it lays out
// nothing.
func modelsShown(t *testing.T, f Frame) (daysLayout, *canvas) {
	t.Helper()
	l, ok := f.modelsLaid(0)
	if !ok {
		t.Fatal("MODELS BY DAY lays nothing out")
	}
	c := newCanvas(f.Width, l.keyRow+l.keyRows)
	f.modelsDrawn(c, f.pastLaid(), 0)
	return l, c
}

// A family's days take its own colour in four steps, as the frames draw
// them in nord.
func TestDaysShadesEachFamilysDaysInItsSteps(t *testing.T) {
	f := modelsFrame(t, daysWorld(), 160, 37)
	l, c := modelsShown(t, f)
	drawn := screenOf(t, c.rows(f.Look))
	row := l.stripRow(l.familyRow(0))
	for level, want := range []string{"#495B76", "#546E91", "#5E81AC", "#B7C6DA"} {
		if got := seenAt(drawn, l.dayAt(level)+1, row).bg.String(); got != want {
			t.Errorf("Opus's level %d is on %s, want %s", level+1, got, want)
		}
	}
}

// Each part of MODELS BY DAY is in the ink the design gives it.
func TestDaysDrawsEachPartInItsInk(t *testing.T) {
	f := modelsFrame(t, daysWorld(), 160, 37)
	l, c := modelsShown(t, f)
	rows := textOf(c.rows(Look{}))
	opus, row := familyHue("opus"), l.stripRow(l.familyRow(0))
	at := func(y int, text string) int {
		before, _, _ := strings.Cut(rows[y], text)
		return ansi.StringWidth(before)
	}
	this := l.row.weeks[len(l.row.weeks)-1].At
	tests := []struct {
		name string
		x, y int
		want ink
	}{
		{name: "a week's first day", x: l.row.weeks[0].At, y: l.top, want: faintInk},
		{name: "this week's", x: this, y: l.top, want: strongInk},
		{name: "this week's days' initials", x: this + 2, y: l.top + 1, want: faintInk},
		{name: "a family's dot", x: 0, y: row, want: ink{token: theme.VizFamily1}},
		{name: "a family's name", x: 2, y: row, want: secondaryInk},
		{name: "a family's day, its first cell, in its steps", x: l.dayAt(0), y: row, want: ink{on: stepInk(opus, 1).hue()}},
		{name: "its second cell", x: l.dayAt(0) + 1, y: row, want: ink{on: stepInk(opus, 1).hue()}},
		{name: "its busiest", x: l.dayAt(3), y: row, want: ink{on: stepInk(opus, 4).hue()}},
		{name: "a day to come", x: l.dayAt(54), y: row, want: faintInk},
		{name: "a family's share", x: at(row, "59%"), y: row, want: strongInk.numbered(Number{Kind: NumberShareOfUse, Of: "opus"})},
		{name: "of use", x: at(row, "of use"), y: row, want: dimInk},
		{name: "the key's less", x: 0, y: l.keyRow, want: dimInk},
		{name: "the key's first shade", x: 5, y: l.keyRow, want: ink{on: dayInk(0).hue()}},
		{name: "its last", x: 18, y: l.keyRow, want: ink{on: dayInk(4).hue()}},
		{name: "what the shades are", x: at(l.keyRow, "each shade"), y: l.keyRow, want: faintInk},
		{name: "the days to come", x: at(l.keyRow, "·· days"), y: l.keyRow, want: faintInk},
		{name: "what they are", x: at(l.keyRow, "days to come"), y: l.keyRow, want: dimInk},
	}
	for _, tt := range tests {
		want, marked := tt.want, tt.want.mark
		want.mark = unique.Handle[mark]{}
		if got := c.at(tt.x, tt.y); got.ink != want || got.mark != marked {
			t.Errorf("%s, column %d of row %d, is %q in %+v, want %+v", tt.name, tt.x, tt.y+1, got.glyph, got.ink, tt.want)
		}
	}
}

// Where the look blends nothing, as the terminal theme and NO_COLOR, a day's
// level is told by its glyph's density, ·░▒▓█, in its family's colour.
func TestDaysToldByGlyphsWhereTheLookBlendsNothing(t *testing.T) {
	page := daysWorld()
	page.ByModel[0].Days = stripOf("01234")
	for _, look := range []Look{NoColour(), Screen(builtin(t, "terminal"))} {
		f := modelsFrame(t, page, 160, 37)
		f.Look = look
		l, c := modelsShown(t, f)
		rows := textOf(c.rows(Look{}))
		row := l.stripRow(l.familyRow(0))
		if got, want := string([]rune(rows[row])[l.dayAt(0):l.dayAt(5)]), "··░░▒▒▓▓██"; got != want {
			t.Errorf("Opus's first days read %q, want %q", got, want)
		}
		if got, want := string([]rune(rows[l.keyRow])[:24]), "less ·· ░░ ▒▒ ▓▓ ██ more"; got != want {
			t.Errorf("the key reads %q, want %q", got, want)
		}
	}
}

// In its narrow form, MODELS BY DAY is the last of its weeks that fit the
// width, three at 52 columns, each strip under its name and its count, the
// count ending where the last week's days end, so it reads with its strip,
// the first a blank row under the weeks; and the key on a row under the
// last strip.
func TestDaysInItsNarrowForm(t *testing.T) {
	page := daysWorld()
	page.ByModel = []views.DaysStrip{{Of: "opus", Days: stripOf(strings.Repeat("3", 53)), Share: new(0.8)}, {Of: "sonnet", Days: stripOf(strings.Repeat("01", 26)), Share: new(0.2)}}
	f := modelsFrame(t, page, 52, 35)
	f.Look = NoColour()
	l, c := modelsShown(t, f)
	rows := textOf(c.rows(Look{}))
	want := []string{
		"14 Sep         21 Sep         28 Sep",
		"                              M T W T F S S",
		"",
		textsAt(52, map[int]string{0: "● Opus", 27: "80% of use"}),
		"▓▓▓▓▓▓▓▓▓▓▓▓▓▓ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓ ▓▓▓▓▓▓▓▓······",
		textsAt(52, map[int]string{0: "● Sonnet", 27: "20% of use"}),
		"░░··░░··░░··░░ ··░░··░░··░░·· ░░··░░········",
		"",
		"less ·· ░░ ▒▒ ▓▓ ██ more   ·· days to come",
	}
	if !l.narrow || !slices.Equal(rows, want) {
		t.Errorf("narrow %t, MODELS BY DAY reads\n%s\nwant it narrow, reading\n%s", l.narrow, strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
}

// Till anything is recorded in its eight weeks, MODELS BY DAY says so, in
// text.subtle, where it starts.
func TestDaysSaysNothingIsRecordedYet(t *testing.T) {
	f := modelsFrame(t, views.DaysPage{Weeks: daysWorld().Weeks}, 160, 37)
	if _, ok := f.modelsLaid(0); ok {
		t.Fatal("MODELS BY DAY lays out strips of nothing recorded")
	}
	c := newCanvas(f.Width, 3)
	f.modelsDrawn(c, f.pastLaid(), 0)
	rows := textOf(c.rows(Look{}))
	if rows[0] != "nothing recorded yet" || c.at(0, 0).ink != dimInk {
		t.Errorf("its first row reads %q in %+v, want nothing recorded yet in text.subtle", rows[0], c.at(0, 0).ink)
	}
	if under := strings.Join(rows[1:], ""); under != "" {
		t.Errorf("it drew more under it: %q", under)
	}
}

// The families' shares change as today's use comes in, so a look brightens
// each as it changes.
func TestDaysSharesBrighten(t *testing.T) {
	f := modelsFrame(t, daysWorld(), 160, 37)
	_, c := modelsShown(t, f)
	want := map[Number]string{
		{Kind: NumberShareOfUse, Of: "opus"}: "59%", {Kind: NumberShareOfUse, Of: "sonnet"}: "37%",
		{Kind: NumberShareOfUse, Of: "fable"}: "2%", {Kind: NumberShareOfUse, Of: "haiku"}: "2%",
	}
	if got := c.numbers(); !maps.Equal(got, want) {
		t.Errorf("MODELS BY DAY's numbers are %v, want %v", got, want)
	}
}

// A family's share not priced is a faint dash, still numbered as its share.
func TestDaysSaysAShareNotPricedAsADash(t *testing.T) {
	unpriced := fitParts(80, shareOfUse(views.DaysStrip{Of: "fable"})...)
	if want := (line{{"—", faintInk.numbered(Number{Kind: NumberShareOfUse, Of: "fable"})}, {" of use", dimInk}}); !slices.Equal(unpriced, want) {
		t.Errorf("an unpriced share is %+v, want %+v", unpriced, want)
	}
}

// Narrower than a phone, MODELS BY DAY shows the latest of its weeks that
// fit, each headed by its first day, and none where even the last's days
// don't end short of the edge; and a strip's share, after its name's two
// cells, is cut at the edge, a figure, numbered only where it's drawn whole.
func TestDaysNarrowerStillFitsItsWeeksAndShares(t *testing.T) {
	for width, want := range map[int]string{40: "21 Sep         28 Sep", 15: "28 Sep", 14: ""} {
		_, c := modelsShown(t, modelsFrame(t, daysWorld(), width, 40))
		if rows := textOf(c.rows(Look{})); rows[0] != want {
			t.Errorf("at %d columns, the weeks read %q, want %q", width, rows[0], want)
		}
	}
	opus := Number{Kind: NumberShareOfUse, Of: "opus"}
	for width, want := range map[int]string{8: "59%", 7: ""} {
		if got := modelsFrame(t, daysWorld(), width, 40).Numbers(threeRouted(), now)[opus]; got != want {
			t.Errorf("at %d columns, Opus's share is numbered %q, want %q", width, got, want)
		}
	}
}

// MODELS BY DAY shows as many of its weeks as fit, up to eight, the latest,
// in either form, and each strip's share with them, never at the edge: wide,
// after the families' names, where its key fits on a row, the share four
// cells after the last week's last day; else each strip under its name, the
// weeks the whole width's, and the share beside the name, ending where the
// last week's days end.
func TestDaysShowsTheWeeksThatFitWithEachShareAfterThem(t *testing.T) {
	tests := []struct {
		width, weeks int
		narrow       bool
		from, counts int
		first        string
	}{
		{width: 30, weeks: 2, narrow: true, counts: 11, first: "21 Sep"},
		{width: 45, weeks: 3, narrow: true, counts: 26, first: "14 Sep"},
		{width: 52, weeks: 3, narrow: true, counts: 26, first: "14 Sep"},
		{width: 60, weeks: 4, narrow: true, counts: 41, first: "7 Sep"},
		{width: 73, weeks: 4, narrow: true, counts: 41, first: "7 Sep"},
		{width: 84, weeks: 5, narrow: true, counts: 56, first: "31 Aug"},
		{width: 85, weeks: 3, from: 10, counts: 57, first: "14 Sep"},
		{width: 105, weeks: 4, from: 10, counts: 72, first: "7 Sep"},
		{width: 138, weeks: 7, from: 10, counts: 117, first: "17 Aug"},
		{width: 152, weeks: 8, from: 10, counts: 132, first: "10 Aug"},
		{width: 160, weeks: 8, from: 10, counts: 132, first: "10 Aug"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.width), func(t *testing.T) {
			l, c := modelsShown(t, modelsFrame(t, daysWorld(), tt.width, 40))
			r := l.row
			if l.narrow != tt.narrow || len(r.weeks) != tt.weeks || r.weeks[0].At != tt.from || r.count.At != tt.counts {
				t.Fatalf("narrow: %t, %d weeks from column %d, the shares at %d; want narrow: %t, %d weeks from %d, the shares at %d",
					l.narrow, len(r.weeks), r.weeks[0].At, r.count.At, tt.narrow, tt.weeks, tt.from, tt.counts)
			}
			lastDay, ends := r.weeks[len(r.weeks)-1].At+7*swatchCells-1, r.count.At+r.count.Size
			if gap := r.count.At - lastDay; !tt.narrow && gap != 4 {
				t.Errorf("the shares start %d cells after the last week's last day, want 4", gap)
			}
			if tt.narrow && ends != lastDay+1 {
				t.Errorf("the shares end before column %d, want where the last week's days end, %d", ends, lastDay+1)
			}
			rows := textOf(c.rows(Look{}))
			if heads := strings.Fields(rows[0]); len(heads) != 2*tt.weeks || heads[0]+" "+heads[1] != tt.first {
				t.Errorf("the weeks read %q, want %d from %s", rows[0], tt.weeks, tt.first)
			}
		})
	}
}
