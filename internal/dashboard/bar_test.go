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
			if got := bar(tt.fraction, noMarker, 10).plain(); got != tt.want {
				t.Errorf("bar(%v) = %q, want %q", tt.fraction, got, tt.want)
			}
		})
	}
}

func TestPaceCell(t *testing.T) {
	tests := []struct {
		name    string
		elapsed float64
		want    int
	}{
		{name: "at the start", elapsed: 0, want: 0},
		{name: "inside the first cell", elapsed: 0.09, want: 0},
		{name: "the middle", elapsed: 0.5, want: 5},
		{name: "inside the last cell", elapsed: 0.95, want: 9},
		{name: "at the end", elapsed: 1, want: 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := paceCell(tt.elapsed, 10); got != tt.want {
				t.Errorf("paceCell(%v, 10) = %d, want %d", tt.elapsed, got, tt.want)
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
			if got := bar(tt.fraction, paceCell(tt.elapsed, 10), 10).plain(); got != tt.want {
				t.Errorf("bar(%v) with the marker at %v = %q, want %q", tt.fraction, tt.elapsed, got, tt.want)
			}
		})
	}
}

func TestBarColors(t *testing.T) {
	cells := bar(1, noMarker, 10)
	if got := cells[0].ink.color; !sameColor(got, green) {
		t.Errorf("first cell's color = %v, want green %v", got, green)
	}
	if got := cells[9].ink.color; !sameColor(got, red) {
		t.Errorf("last cell's color = %v, want red %v", got, red)
	}
	short := bar(0.2, noMarker, 10)
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
