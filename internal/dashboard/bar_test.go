package dashboard

import (
	"image/color"
	"testing"
)

func TestBarCells(t *testing.T) {
	tests := []struct {
		name     string
		fraction float64
		want     string
	}{
		{name: "empty", fraction: 0, want: "░░░░░░░░░░"},
		{name: "below zero", fraction: -0.2, want: "░░░░░░░░░░"},
		{name: "too little for an eighth", fraction: 0.006, want: "░░░░░░░░░░"},
		{name: "rounds up to an eighth", fraction: 0.007, want: "▏░░░░░░░░░"},
		{name: "an eighth", fraction: 0.0125, want: "▏░░░░░░░░░"},
		{name: "half a cell", fraction: 0.05, want: "▌░░░░░░░░░"},
		{name: "seven eighths", fraction: 0.0875, want: "▉░░░░░░░░░"},
		{name: "a cell", fraction: 0.1, want: "█░░░░░░░░░"},
		{name: "cells and three eighths", fraction: 0.3375, want: "███▍░░░░░░"},
		{name: "one eighth short of full", fraction: 0.9875, want: "█████████▉"},
		{name: "full", fraction: 1, want: "██████████"},
		{name: "over the limit", fraction: 1.3, want: "██████████"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bar(tt.fraction, 10).plain(); got != tt.want {
				t.Errorf("bar(%v) = %q, want %q", tt.fraction, got, tt.want)
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

func TestBarMarker(t *testing.T) {
	tests := []struct {
		name     string
		fraction float64
		elapsed  float64
		want     string
	}{
		{name: "at the start of an empty bar", fraction: 0, elapsed: 0, want: "┃░░░░░░░░░"},
		{name: "ahead of the fill", fraction: 0.3, elapsed: 0.5, want: "███░░┃░░░░"},
		{name: "just past the fill, keeping pace", fraction: 0.5, elapsed: 0.5, want: "█████┃░░░░"},
		{name: "over the fill's edge", fraction: 0.55, elapsed: 0.5, want: "█████┃░░░░"},
		{name: "behind the fill", fraction: 0.8, elapsed: 0.5, want: "█████┃██░░"},
		{name: "at the end of a full bar", fraction: 1, elapsed: 1, want: "█████████┃"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bar(tt.fraction, 10).mark(cellAt(tt.elapsed, 10), paceMarker).plain(); got != tt.want {
				t.Errorf("bar(%v) with the marker at %v = %q, want %q", tt.fraction, tt.elapsed, got, tt.want)
			}
		})
	}
}

func TestBarMarksWhereTheReserveStarts(t *testing.T) {
	tests := []struct {
		name     string
		fraction float64
		// pace is the cell the pace marker takes, or noMarker for none.
		pace int
		want string
	}{
		{name: "short of it", fraction: 0.5, pace: noMarker, want: "█████░░░░╎"},
		{name: "past it", fraction: 0.95, pace: noMarker, want: "█████████╎"},
		{name: "beside the pace marker", fraction: 0.3, pace: 5, want: "███░░┃░░░╎"},
		{name: "under the pace marker, which shows", fraction: 0.8, pace: 9, want: "████████░┃"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bar(tt.fraction, 10).mark(reserveCell(0.1, 10), reserveMarker).mark(tt.pace, paceMarker)
			if got.plain() != tt.want {
				t.Errorf("bar(%v) with a reserve of a tenth = %q, want %q", tt.fraction, got.plain(), tt.want)
			}
		})
	}
	if got := bar(0.5, 10).mark(reserveCell(0.1, 10), reserveMarker)[9].ink; got != warningInk {
		t.Errorf("the reserve's mark is drawn in %v, want the warning colour, %v", got, warningInk)
	}
}

func TestBarColors(t *testing.T) {
	cells := bar(1, 10)
	if got := cells[0].ink.color; !sameColor(got, green) {
		t.Errorf("first cell's color = %v, want green %v", got, green)
	}
	if got := cells[9].ink.color; !sameColor(got, red) {
		t.Errorf("last cell's color = %v, want red %v", got, red)
	}
	short := bar(0.2, 10)
	if got, want := short[1].ink.color, rampAt(1.0/9); !sameColor(got, want) {
		t.Errorf("second cell of a short bar's color = %v, want the ramp's %v, as on a full bar", got, want)
	}
	if got := short[2].ink.color; !sameColor(got, trackInk.color) {
		t.Errorf("track color = %v, want %v", got, trackInk.color)
	}
}

func TestRampAt(t *testing.T) {
	tests := []struct {
		name string
		t    float64
		want color.Color
	}{
		{name: "start", t: 0, want: green},
		{name: "before the start", t: -1, want: green},
		{name: "a third", t: 1.0 / 3, want: yellow},
		{name: "two thirds", t: 2.0 / 3, want: orange},
		{name: "end", t: 1, want: red},
		{name: "past the end", t: 2, want: red},
		{name: "three quarters of the way from green to yellow", t: 0.25, want: color.RGBA{R: 0xD9, G: 0xC8, B: 0x8B, A: 0xff}},
		{name: "two thirds of the way from yellow to orange", t: 5.0 / 9, want: color.RGBA{R: 0xD9, G: 0x9E, B: 0x79, A: 0xff}},
		{name: "seven tenths of the way from orange to red", t: 0.9, want: color.RGBA{R: 0xC4, G: 0x6C, B: 0x6C, A: 0xff}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rampAt(tt.t); !sameColor(got, tt.want) {
				t.Errorf("rampAt(%v) = %v, want %v", tt.t, got, tt.want)
			}
		})
	}
}

func sameColor(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}
