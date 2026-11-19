package theme

import (
	"image/color"
	"math"
	"testing"
)

// labOf is c as OKLab.
func labOf(c color.Color) (l, a, b float64) {
	r, g, bl, _ := c.RGBA()
	return toOKLab(linear(r), linear(g), linear(bl))
}

// A colour turned keeps its lightness, so text that reads on it reads on the
// turned one, and its chroma where sRGB holds it; turned all the way round,
// or not at all, it's the colour it was; a grey has no hue to turn.
func TestTurnedKeepsTheColoursLightness(t *testing.T) {
	tests := []struct {
		name    string
		colour  color.RGBA
		degrees float64
	}{
		{name: "a muted lilac, a third of the way round", colour: color.RGBA{R: 0xB4, G: 0x8E, B: 0xAD, A: 0xff}, degrees: 120},
		{name: "a muted lilac, back the other way", colour: color.RGBA{R: 0xB4, G: 0x8E, B: 0xAD, A: 0xff}, degrees: -90},
		{name: "a vivid violet, past halfway, its chroma eased to stay in sRGB", colour: color.RGBA{R: 0xBB, G: 0x9A, B: 0xF7, A: 0xff}, degrees: 200},
		{name: "a dark purple, as a light theme's", colour: color.RGBA{R: 0x7C, G: 0x43, B: 0xDB, A: 0xff}, degrees: 150},
		{name: "a grey", colour: color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff}, degrees: 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			turned := Turned(tt.colour, tt.degrees)
			wantL, wantA, wantB := labOf(tt.colour)
			gotL, gotA, gotB := labOf(turned)
			if math.Abs(gotL-wantL) > 0.01 {
				t.Errorf("Turned(%v, %v) = %v, lightness %.3f, want %.3f, as the colour's", tt.colour, tt.degrees, turned, gotL, wantL)
			}
			if chroma, was := math.Hypot(gotA, gotB), math.Hypot(wantA, wantB); chroma > was+0.01 {
				t.Errorf("Turned(%v, %v) = %v, chroma %.3f, want no more than the colour's %.3f", tt.colour, tt.degrees, turned, chroma, was)
			}
			for _, round := range []float64{0, 360} {
				if back := Turned(tt.colour, round); !near(back, tt.colour) {
					t.Errorf("Turned(%v, %v) = %v, want the colour as it was", tt.colour, round, back)
				}
			}
		})
	}
}

// A colour turned is another hue, as far round as asked, where the colour
// has one.
func TestTurnedMovesTheHue(t *testing.T) {
	lilac := color.RGBA{R: 0xB4, G: 0x8E, B: 0xAD, A: 0xff}
	_, a, b := labOf(lilac)
	_, ta, tb := labOf(Turned(lilac, 120))
	got := math.Mod(math.Atan2(tb, ta)-math.Atan2(b, a)+4*math.Pi, 2*math.Pi) * 180 / math.Pi
	if math.Abs(got-120) > 3 {
		t.Errorf("Turned(%v, 120) moves the hue %.1f degrees, want 120", lilac, got)
	}
}

// near reports whether two colours differ by a step of a channel at most.
func near(a, b color.Color) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	for _, d := range []int{int(ar>>8) - int(br>>8), int(ag>>8) - int(bg>>8), int(ab>>8) - int(bb>>8)} {
		if d < -1 || d > 1 {
			return false
		}
	}
	return true
}
