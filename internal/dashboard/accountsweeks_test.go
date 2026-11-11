package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// WEEKS says a week's peak as a whole percentage without its sign, in
// text.muted, at its cap bold state.destructive, as views marks it, never by
// its figure alone, and a dash where it isn't known, its bar then empty.
func TestWEEKSSaysEachWeeksPeak(t *testing.T) {
	tests := []struct {
		name  string
		share *float64
		atCap bool
		want  line
		empty bool
	}{
		{name: "under its limit", share: new(0.344), want: line{{"34", mutedInk}}},
		{name: "rounding to its limit, short of it", share: new(0.996), want: line{{"100", mutedInk}}},
		{name: "at its limit", share: new(1.0), atCap: true, want: line{{"100", exhaustedInk}}},
		{name: "a reserved account's 96%, at its cap", share: new(0.96), atCap: true, want: line{{"96", exhaustedInk}}},
		{name: "a reserved account's 94%, short of its cap", share: new(0.94), want: line{{"94", mutedInk}}},
		{name: "not known", want: line{noFigure}, empty: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := views.WeekPeak{Share: tt.share, AtCap: tt.atCap}
			if got := peakSaid(p); !slices.Equal(got, tt.want) {
				t.Errorf("peakSaid = %v, want %v", got, tt.want)
			}
			if bar := peakBar(p); bar.empty != tt.empty || !tt.empty && (bar.used != *tt.share || bar.capAt != 1) {
				t.Errorf("peakBar = %+v, want empty %t, or filled to the peak, no cap", bar, tt.empty)
			}
		})
	}
}

// A day of BY DAY is its cells in its shade, a red ▪ in its middle where it
// hit a 5-hour limit: of two cells, the first; of three, the second.
func TestBYDAYMarksADaysLimitInItsMiddle(t *testing.T) {
	f := Frame{Look: NoColour()}
	k := dayInk(3)
	tests := []struct {
		cells int
		want  string
	}{
		{cells: 2, want: "▪" + shadeGlyph(3)},
		{cells: 3, want: shadeGlyph(3) + "▪" + shadeGlyph(3)},
		{cells: 4, want: shadeGlyph(3) + "▪" + strings.Repeat(shadeGlyph(3), 2)},
	}
	for _, tt := range tests {
		got := f.daySwatch(k, 3, true, tt.cells)
		if got.plain() != tt.want {
			t.Errorf("%d cells: %q, want %q", tt.cells, got.plain(), tt.want)
		}
		for _, s := range got {
			if s.text == limitMark && s.ink.token != theme.StateDestructive {
				t.Errorf("%d cells: ▪ in %+v, want state.destructive", tt.cells, s.ink)
			}
		}
		if plain := f.daySwatch(k, 3, false, tt.cells).plain(); plain != strings.Repeat(shadeGlyph(3), tt.cells) {
			t.Errorf("%d cells, no limit: %q", tt.cells, plain)
		}
	}
}

// BY DAY draws the days still to come of this week as · in text.faint, a
// cell each of the day's.
func TestBYDAYDotsTheDaysToCome(t *testing.T) {
	doc := usageWorld(extraOff)
	page := detailsPage(t)
	f := detailFrame(Look{}, framedAccounts, "work", "", page, nil)
	d := f.detailDrawn(doc, routedAt, laidIn(f, doc))
	c := d.panel()
	y := d.underModels() + byDayUnder
	for day := range 7 {
		toCome := day >= 7-page.ToCome
		for cell := range d.l.dw {
			got := c.at(d.l.cx()+5*d.l.step()+day*d.l.dw+cell, y)
			if dotted := got.glyph == "·" && got.ink == faintInk; dotted != toCome {
				t.Errorf("this week's day %d, cell %d, is %q in %+v, want · in text.faint only where it's to come", day+1, cell, got.glyph, got.ink)
			}
		}
	}
}
