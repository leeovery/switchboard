package dashboard

import (
	"math"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
)

func TestABarFillsItsUseAndItsProjectionBeyond(t *testing.T) {
	tests := []struct {
		name          string
		used, heading float64
		reserve, pace int
		want          string
	}{
		{name: "used, heading further, its reserve and even pace marked", used: 0.34, heading: 0.87, reserve: reserveCell(0.1, 18), pace: cellAt(0.39, 18), want: "██████▏┃███████▋╎░"},
		{name: "used alone, heading nowhere further", used: 0.12, heading: 0.12, reserve: noMarker, pace: noMarker, want: "██▏░░░░░░░░░░░░░░░"},
		{name: "full, past its limit", used: 1.04, heading: 0, reserve: noMarker, pace: noMarker, want: "██████████████████"},
		{name: "empty", used: 0, heading: 0, reserve: noMarker, pace: 8, want: "░░░░░░░░┃░░░░░░░░░"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCanvas(18, 1)
			Frame{Look: Screen(builtin(t, "nord"))}.fill(c, 0, 0, 18, tt.used, tt.heading, tt.reserve, tt.pace)
			if got := c.rows(Look{})[0]; got != tt.want {
				t.Errorf("drew %q, want %q", got, tt.want)
			}
		})
	}
}

func TestABarsCellsTakeTheRampsColoursItsProjectionFadedHalfway(t *testing.T) {
	c := newCanvas(18, 1)
	Frame{Look: Screen(builtin(t, "nord"))}.fill(c, 0, 0, 18, 0.34, 0.87, reserveCell(0.1, 18), cellAt(0.39, 18))
	at := func(i int) float64 { return math.Round(along(i, 18)*rampSteps) / rampSteps }
	faint := func(i int) hue { return hue{ramp: true, at: at(i), fade: projectionFade} }
	tests := []struct {
		name string
		cell int
		want ink
	}{
		{name: "a full cell, at its place along the ramp, to the nearest eighth", cell: 2, want: ink{ramp: true, at: 0.125}},
		{name: "the cell its use ends in, on its projection", cell: 6, want: ink{ramp: true, at: at(6), on: faint(6)}},
		{name: "even pace, bold, on the projection under it", cell: 7, want: ink{token: theme.VizPace, bold: true, on: faint(7)}},
		{name: "its projection, faded halfway", cell: 8, want: ink{ramp: true, at: at(8), fade: projectionFade}},
		{name: "its reserve, on the track", cell: 16, want: reserveInk},
		{name: "the track", cell: 17, want: trackInk},
	}
	for _, tt := range tests {
		if got := c.at(tt.cell, 0).ink; got != tt.want {
			t.Errorf("%s: cell %d is in %+v, want %+v", tt.name, tt.cell, got, tt.want)
		}
	}
}

func TestAReserveOverABarsFillIsANotch(t *testing.T) {
	c := newCanvas(10, 1)
	Frame{Look: Screen(builtin(t, "nord"))}.fill(c, 0, 0, 10, 0.95, 0.95, reserveCell(0.2, 10), noMarker)
	if got, want := c.at(8, 0).ink, (ink{token: theme.Canvas, on: hue{ramp: true, at: 0.875}}); got != want || c.at(8, 0).glyph != "╎" {
		t.Errorf("the reserve's cell is %q in %+v, want ╎ in %+v, notched in the fill", c.at(8, 0).glyph, got, want)
	}
}

func TestALookThatCantFadeDrawsABarsProjectionInShade(t *testing.T) {
	for _, look := range []Look{Screen(builtin(t, theme.Terminal)), NoColour()} {
		c := newCanvas(18, 1)
		Frame{Look: look}.fill(c, 0, 0, 18, 0.34, 0.87, noMarker, noMarker)
		if got, want := c.rows(Look{})[0], "██████▏▒▒▒▒▒▒▒▒▒░░"; got != want {
			t.Errorf("drew %q, want %q", got, want)
		}
		if on := c.at(6, 0).ink.on; on != (hue{}) {
			t.Errorf("the cell its use ends in is on %+v, want the canvas", on)
		}
	}
}
