package dashboard

import (
	"image/color"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/theme"
)

// now is the clock frames are drawn at: a Monday, 13:12 an hour east of UTC.
var now = time.Date(2026, 9, 28, 13, 12, 0, 0, time.FixedZone("UTC+1", 60*60))

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
