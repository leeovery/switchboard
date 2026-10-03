package dashboard

import (
	"image/color"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/theme"
)

// now is the clock frames are drawn at: a Monday, 13:12 an hour east of UTC.
var now = time.Date(2026, 9, 28, 13, 12, 0, 0, time.FixedZone("UTC+1", 60*60))

// barOf is a bar ten cells long, as fill draws it, used to the share given
// and heading nowhere further, its reserve's mark and even pace's at the
// cells given, noMarker for neither.
func barOf(t *testing.T, used float64, reserve, pace int) string {
	t.Helper()
	c := newCanvas(10, 1)
	Frame{Look: Screen(builtin(t, "nord"))}.fill(c, 0, 0, 10, used, used, reserve, pace)
	return c.rows(Look{})[0]
}

func TestABarFillsInEighthsOfACell(t *testing.T) {
	tests := []struct {
		name string
		used float64
		want string
	}{
		{name: "empty", used: 0, want: "░░░░░░░░░░"},
		{name: "below zero", used: -0.2, want: "░░░░░░░░░░"},
		{name: "too little for an eighth", used: 0.006, want: "░░░░░░░░░░"},
		{name: "rounds up to an eighth", used: 0.007, want: "▏░░░░░░░░░"},
		{name: "an eighth", used: 0.0125, want: "▏░░░░░░░░░"},
		{name: "half a cell", used: 0.05, want: "▌░░░░░░░░░"},
		{name: "seven eighths", used: 0.0875, want: "▉░░░░░░░░░"},
		{name: "a cell", used: 0.1, want: "█░░░░░░░░░"},
		{name: "cells and three eighths", used: 0.3375, want: "███▍░░░░░░"},
		{name: "one eighth short of full", used: 0.9875, want: "█████████▉"},
		{name: "full", used: 1, want: "██████████"},
		{name: "over the limit", used: 1.3, want: "██████████"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := barOf(t, tt.used, noMarker, noMarker); got != tt.want {
				t.Errorf("a bar used %v = %q, want %q", tt.used, got, tt.want)
			}
		})
	}
}

func TestCellAt(t *testing.T) {
	tests := []struct {
		name     string
		fraction float64
		want     int
	}{
		{name: "at the start", fraction: 0, want: 0},
		{name: "inside the first cell", fraction: 0.09, want: 0},
		{name: "the middle", fraction: 0.5, want: 5},
		{name: "inside the last cell", fraction: 0.95, want: 9},
		{name: "at the end", fraction: 1, want: 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cellAt(tt.fraction, 10); got != tt.want {
				t.Errorf("cellAt(%v, 10) = %d, want %d", tt.fraction, got, tt.want)
			}
		})
	}
}

func TestReserveCell(t *testing.T) {
	tests := []struct {
		name    string
		reserve float64
		width   int
		want    int
	}{
		{name: "a tenth, where 90% of the bar ends", reserve: 0.1, width: 40, want: 36},
		{name: "a quarter", reserve: 0.25, width: 40, want: 30},
		{name: "a tenth of a short bar, in the cell 90% falls in", reserve: 0.1, width: 8, want: 7},
		{name: "a sliver, in the last cell", reserve: 0.01, width: 40, want: 39},
		{name: "none", reserve: 0, width: 40, want: noMarker},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reserveCell(tt.reserve, tt.width); got != tt.want {
				t.Errorf("reserveCell(%v, %d) = %d, want %d", tt.reserve, tt.width, got, tt.want)
			}
		})
	}
}

func TestEvenPaceIsMarkedOverABar(t *testing.T) {
	tests := []struct {
		name          string
		used, elapsed float64
		want          string
	}{
		{name: "at the start of an empty bar", used: 0, elapsed: 0, want: "┃░░░░░░░░░"},
		{name: "ahead of the fill", used: 0.3, elapsed: 0.5, want: "███░░┃░░░░"},
		{name: "just past the fill, keeping pace", used: 0.5, elapsed: 0.5, want: "█████┃░░░░"},
		{name: "over the fill's edge", used: 0.55, elapsed: 0.5, want: "█████┃░░░░"},
		{name: "behind the fill", used: 0.8, elapsed: 0.5, want: "█████┃██░░"},
		{name: "at the end of a full bar", used: 1, elapsed: 1, want: "█████████┃"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := barOf(t, tt.used, noMarker, cellAt(tt.elapsed, 10)); got != tt.want {
				t.Errorf("a bar used %v, marked at %v, = %q, want %q", tt.used, tt.elapsed, got, tt.want)
			}
		})
	}
}

func TestABarMarksWhereTheReserveStarts(t *testing.T) {
	tests := []struct {
		name string
		used float64
		// pace is the cell even pace's mark takes, or noMarker for none.
		pace int
		want string
	}{
		{name: "short of it", used: 0.5, pace: noMarker, want: "█████░░░░╎"},
		{name: "past it", used: 0.95, pace: noMarker, want: "█████████╎"},
		{name: "beside even pace", used: 0.3, pace: 5, want: "███░░┃░░░╎"},
		{name: "under even pace, which shows", used: 0.8, pace: 9, want: "████████░┃"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := barOf(t, tt.used, reserveCell(0.1, 10), tt.pace); got != tt.want {
				t.Errorf("a bar used %v, keeping a tenth back, = %q, want %q", tt.used, got, tt.want)
			}
		})
	}
}

func TestABarsCellsAreColouredByWhereTheySitAlongIt(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	full, short := newCanvas(10, 1), newCanvas(10, 1)
	Frame{Look: look}.fill(full, 0, 0, 10, 1, 1, noMarker, noMarker)
	Frame{Look: look}.fill(short, 0, 0, 10, 0.2, 0.2, noMarker, noMarker)
	if got := look.colour(full.at(0, 0).ink); !sameColor(got, rgb(0xA3BE8C)) {
		t.Errorf("a full bar's first cell is %v, want the ramp's first stop, #A3BE8C", got)
	}
	if got := look.colour(full.at(9, 0).ink); !sameColor(got, rgb(0xBF616A)) {
		t.Errorf("a full bar's last cell is %v, want the ramp's last stop, #BF616A", got)
	}
	if got, want := look.colour(short.at(1, 0).ink), look.colour(full.at(1, 0).ink); !sameColor(got, want) {
		t.Errorf("a short bar's second cell is %v, want %v, as on a full bar", got, want)
	}
	if got := short.at(2, 0).ink; got != trackInk {
		t.Errorf("the track is drawn in %+v, want viz.track's ink, %+v", got, trackInk)
	}
}

func TestRamp(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	tests := []struct {
		name string
		t    float64
		want color.Color
	}{
		{name: "start", t: 0, want: rgb(0xA3BE8C)},
		{name: "before the start", t: -1, want: rgb(0xA3BE8C)},
		{name: "a third", t: 1.0 / 3, want: rgb(0xEBCB8B)},
		{name: "two thirds", t: 2.0 / 3, want: rgb(0xD08770)},
		{name: "end", t: 1, want: rgb(0xBF616A)},
		{name: "past the end", t: 2, want: rgb(0xBF616A)},
		{name: "three quarters of the way from green to yellow", t: 0.25, want: rgb(0xD9C88B)},
		{name: "two thirds of the way from yellow to orange", t: 5.0 / 9, want: rgb(0xD99E79)},
		{name: "seven tenths of the way from orange to red", t: 0.9, want: rgb(0xC46C6C)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nord.ramp(tt.t); !sameColor(got, tt.want) {
				t.Errorf("ramp(%v) = %v, want %v", tt.t, got, tt.want)
			}
		})
	}
}

func TestTheTerminalsRampStepsRatherThanBlends(t *testing.T) {
	term := Screen(builtin(t, "terminal"))
	stops := term.theme.Ramp()
	for at, want := range map[float64]color.Color{0: stops[0], 0.15: stops[0], 0.2: stops[1], 0.45: stops[1], 0.55: stops[2], 0.8: stops[2], 0.9: stops[3], 1: stops[3]} {
		if got := term.ramp(at); got != want {
			t.Errorf("ramp(%v) = %v, want the nearest stop, %v, never a blend the terminal's colours can't name", at, got, want)
		}
	}
}

// builtin is the built-in theme slug names.
func builtin(t *testing.T, slug string) theme.Theme {
	t.Helper()
	b, ok := theme.Builtin(slug)
	if !ok {
		t.Fatalf("no built-in theme %s", slug)
	}
	return b
}

// rgb is the colour 0xRRGGBB.
func rgb(hex uint32) color.Color {
	return color.RGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 0xff}
}

func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}
