package dashboard

import (
	"image/color"
	"math"

	"charm.land/lipgloss/v2"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// The palette is Nord's (nordtheme.com), whose muted colours suit the dark
// terminals a dashboard spends its day on.
var (
	textColor   = lipgloss.Color("#D8DEE9")
	brightColor = lipgloss.Color("#ECEFF4")
	dimColor    = lipgloss.Color("#616E88")
	borderColor = lipgloss.Color("#4C566A")
	accentColor = lipgloss.Color("#88C0D0")
	green       = lipgloss.Color("#A3BE8C")
	yellow      = lipgloss.Color("#EBCB8B")
	orange      = lipgloss.Color("#D08770")
	red         = lipgloss.Color("#BF616A")
)

// ink is how a span is drawn: its colour, and whether it's bold. The zero ink
// leaves text as it is.
type ink struct {
	color color.Color
	bold  bool
}

var (
	textInk       = ink{color: textColor}
	dimInk        = ink{color: dimColor}
	nameInk       = ink{color: accentColor, bold: true}
	accentInk     = ink{color: accentColor}
	borderInk     = ink{color: borderColor}
	bestBorderInk = ink{color: accentColor}
	titleInk      = ink{color: brightColor, bold: true}
	badgeInk      = ink{color: accentColor, bold: true}
	trackInk      = ink{color: borderColor}
	markerInk     = ink{color: brightColor}
	offlineInk    = ink{color: yellow}
	errorInk      = ink{color: red}
	exhaustedInk  = ink{color: red, bold: true}
)

// render draws text in the ink.
func (k ink) render(text string) string {
	if k == (ink{}) {
		return text
	}
	return lipgloss.NewStyle().Foreground(k.color).Bold(k.bold).Render(text)
}

// ramp is the gradient a bar is filled along, from its first cell to its last.
var ramp = []color.Color{green, yellow, orange, red}

// A window's tone turns yellow, then red, at these fractions used.
const (
	yellowFrom = 0.7
	redFrom    = 0.9
)

// tone colours a window by how much of it is used: green, then yellow from
// 70%, and red from 90% or once it's exhausted.
func tone(w quota.Window, p score.Projection) color.Color {
	switch {
	case p.Kind == score.Exhausted || w.Utilization >= redFrom:
		return red
	case w.Utilization >= yellowFrom:
		return yellow
	default:
		return green
	}
}

// rampAt is the ramp's colour at t, from 0 at its start to 1 at its end.
func rampAt(t float64) color.Color {
	last := len(ramp) - 1
	pos := min(max(t, 0), 1) * float64(last)
	i := min(int(pos), last-1)
	return mix(ramp[i], ramp[i+1], pos-float64(i))
}

// mix blends from a to b by t, from 0 (all a) to 1 (all b).
func mix(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	channel := func(from, to uint32) uint8 {
		// RGBA scales each 8-bit channel by 0x101 to 16 bits.
		return uint8(math.Round((float64(from) + (float64(to)-float64(from))*t) / 0x101))
	}
	return color.RGBA{R: channel(ar, br), G: channel(ag, bg), B: channel(ab, bb), A: 0xff}
}
