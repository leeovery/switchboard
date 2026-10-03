// Package theme is the dashboard's colour, as Portal's is: the tokens it's
// drawn in, which name what a colour means and how prominent it is, never a
// hue; the themes that give each token a colour, built in or read from
// .theme files in Portal's format, so a Portal theme works as it is; the
// choice of one theme, or of a pair for light and dark terminals; and the
// preferences file that keeps the choice.
package theme

import (
	"image/color"
	"math"
	"time"
)

// Token is a colour by what it means and how prominent it is, never by its
// hue: the key a .theme file gives it under. The zero Token names no colour.
type Token int

// The tokens: Portal's 19, which every theme file gives, then switchboard's
// own, for its charts, each worked out from Portal's where a file leaves it
// out.
const (
	_ Token = iota
	TextPrimary
	TextSecondary
	TextTertiary
	TextMuted
	TextSubtle
	TextFaint
	TextOnSelection
	AccentPrimary
	AccentKey
	AccentMode
	AccentAttention
	StatePositive
	StateDestructive
	// Canvas is painted on every cell, every other colour read against it.
	Canvas
	BgSelection
	BgAttention
	BgSubtle
	Border
	TextOnAttention
	// VizRamp1 to VizRamp4 fill a bar, from its first cell to its last.
	VizRamp1
	VizRamp2
	VizRamp3
	VizRamp4
	// VizTrack is a bar's empty cells.
	VizTrack
	// VizPace marks where even use across a window would be.
	VizPace
	// VizReserve marks where a reserve starts, and its floor.
	VizReserve
	// VizSeries1 to VizSeries6 are the accounts', in order, round again past
	// six, as their cords are drawn.
	VizSeries1
	VizSeries2
	VizSeries3
	VizSeries4
	VizSeries5
	VizSeries6
	tokens
)

// The first and last of Portal's tokens, every one of which a theme file
// gives, and of the ramp's and the series'.
const (
	firstBase   = TextPrimary
	lastBase    = TextOnAttention
	seriesCount = int(VizSeries6-VizSeries1) + 1
)

// names are the tokens' keys in a .theme file.
var names = [tokens]string{
	TextPrimary:      "text.primary",
	TextSecondary:    "text.secondary",
	TextTertiary:     "text.tertiary",
	TextMuted:        "text.muted",
	TextSubtle:       "text.subtle",
	TextFaint:        "text.faint",
	TextOnSelection:  "text.on-selection",
	AccentPrimary:    "accent.primary",
	AccentKey:        "accent.key",
	AccentMode:       "accent.mode",
	AccentAttention:  "accent.attention",
	StatePositive:    "state.positive",
	StateDestructive: "state.destructive",
	Canvas:           "canvas",
	BgSelection:      "bg.selection",
	BgAttention:      "bg.attention",
	BgSubtle:         "bg.subtle",
	Border:           "border",
	TextOnAttention:  "text.on-attention",
	VizRamp1:         "viz.ramp.1",
	VizRamp2:         "viz.ramp.2",
	VizRamp3:         "viz.ramp.3",
	VizRamp4:         "viz.ramp.4",
	VizTrack:         "viz.track",
	VizPace:          "viz.pace",
	VizReserve:       "viz.reserve",
	VizSeries1:       "viz.series.1",
	VizSeries2:       "viz.series.2",
	VizSeries3:       "viz.series.3",
	VizSeries4:       "viz.series.4",
	VizSeries5:       "viz.series.5",
	VizSeries6:       "viz.series.6",
}

// String is the token's key in a .theme file, such as text.primary.
func (t Token) String() string {
	if t <= 0 || t >= tokens {
		return ""
	}
	return names[t]
}

// Theme is a palette: a colour for every token, under the slug that names it.
type Theme struct {
	// Slug names the theme: a built-in's name, or its file's, less .theme.
	Slug    string
	colours [tokens]color.Color
	// faint are the tokens drawn faint, in the terminal's own foreground, and
	// reversed the surfaces drawn by swapping its foreground and background:
	// the terminal theme's, as no colour of the terminal's sixteen does for
	// them on every terminal.
	faint, reversed [tokens]bool
}

// Colour is the theme's colour for a token. It's nil for the zero Token, and
// for what the terminal theme leaves as the terminal has it: its canvas, its
// text in the terminal's own foreground, and what it draws faint or reversed.
func (t Theme) Colour(tok Token) color.Color {
	if tok <= 0 || tok >= tokens {
		return nil
	}
	return t.colours[tok]
}

// Faint reports whether the theme draws tok faint, in the terminal's own
// foreground, rather than in a colour of its own: the terminal theme's dim
// text, borders and tracks, as of the terminal's sixteen colours none is dim
// on every terminal, bright black being Solarized Dark's background.
func (t Theme) Faint(tok Token) bool {
	return tok > 0 && tok < tokens && t.faint[tok]
}

// Reversed reports whether the theme draws the surface tok by swapping the
// terminal's own foreground and background, rather than in a colour of its
// own: the terminal theme's selection and attention, as of the terminal's
// sixteen colours none shows behind its own foreground on every terminal.
func (t Theme) Reversed(tok Token) bool {
	return tok > 0 && tok < tokens && t.reversed[tok]
}

// Ramp is the colours a bar fills along, from its first cell to its last.
func (t Theme) Ramp() []color.Color {
	return []color.Color{t.colours[VizRamp1], t.colours[VizRamp2], t.colours[VizRamp3], t.colours[VizRamp4]}
}

// Series is the colour of the account in place n, counting from 1, round
// again past six.
func (t Theme) Series(n int) color.Color {
	return t.colours[SeriesOf(n)]
}

// SeriesOf is the token of the account in place n, counting from 1, round
// again past six: viz.series.1 to viz.series.6, which its cords are drawn in.
func SeriesOf(n int) Token {
	return VizSeries1 + Token((max(n, 1)-1)%seriesCount)
}

// Paints reports whether the theme paints its canvas: every theme but the
// terminal's, which leaves the terminal's own background, transparent or an
// image as it may be, and so blends nothing into it.
func (t Theme) Paints() bool {
	return t.colours[Canvas] != nil
}

// Suits reports whether the theme reads on a terminal whose background is
// dark, or light, as printed there with none of its own painted: its canvas
// is as dark, or it paints none, its colours the terminal's own.
func (t Theme) Suits(dark bool) bool {
	return !t.Paints() || Dark(t.colours[Canvas]) == dark
}

// derive works out each of switchboard's tokens the theme leaves out from
// Portal's, so a theme without them still draws every chart. The series
// never take state.destructive, which a limit or a refusal is drawn in.
func (t *Theme) derive() {
	c := &t.colours
	or := func(tok Token, from color.Color) {
		if c[tok] == nil {
			c[tok] = from
		}
	}
	or(VizRamp1, c[StatePositive])
	or(VizRamp2, c[AccentAttention])
	if c[VizRamp3] == nil {
		c[VizRamp3] = Mix(c[AccentAttention], c[StateDestructive], 0.5)
	}
	or(VizRamp4, c[StateDestructive])
	or(VizTrack, c[Border])
	or(VizPace, c[TextPrimary])
	or(VizReserve, c[AccentKey])
	for i, from := range []Token{AccentKey, StatePositive, AccentPrimary, AccentMode, TextSecondary, AccentAttention} {
		or(VizSeries1+Token(i), c[from])
	}
}

// Mix blends from a to b by t, from 0, all a, to 1, all b.
func Mix(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	channel := func(from, to uint32) uint8 {
		// RGBA scales each 8-bit channel by 0x101 to 16 bits.
		return uint8(math.Round((float64(from) + (float64(to)-float64(from))*t) / 0x101))
	}
	return color.RGBA{R: channel(ar, br), G: channel(ag, bg), B: channel(ab, bb), A: 0xff}
}

// AnswerWithin is how long a terminal is given to say what its background is
// (OSC 11) before it's taken for dark: one answers in a few milliseconds, or
// over SSH in a few more.
const AnswerWithin = 150 * time.Millisecond

// Dark reports whether a terminal's background, as the terminal gave it, is
// dark: its lightness under half. One the terminal didn't give, nil, is taken
// for dark.
func Dark(background color.Color) bool {
	if background == nil {
		return true
	}
	r, g, b, _ := background.RGBA()
	lightest, darkest := max(r, g, b), min(r, g, b)
	return float64(lightest)+float64(darkest) < 0xffff
}
