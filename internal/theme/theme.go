// Package theme is the dashboard's colour: the tokens it's drawn in, which
// name what a colour means and how prominent it is, never a hue; the themes
// that give each token a colour, built in or read from .theme files; the
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

// The tokens: the 19 base tokens, which every theme file gives, then the
// charts' own, each worked out from the base tokens where a file leaves it
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
	// VizFamily1 to VizFamily4 are the model families' hues, in the version
	// table's order: Opus, Sonnet, Fable, Haiku.
	VizFamily1
	VizFamily2
	VizFamily3
	VizFamily4
	// VizVersion1 to VizVersion6 are the model versions' shades, as the
	// version table assigns them, a family's newest first.
	VizVersion1
	VizVersion2
	VizVersion3
	VizVersion4
	VizVersion5
	VizVersion6
	// VizDay0 to VizDay4 are a day's shades by how busy it was: VizDay0 a day
	// with nothing on it, then the active days' quarters, the busiest last.
	VizDay0
	VizDay1
	VizDay2
	VizDay3
	VizDay4
	tokens
)

// The first and last of the base tokens, every one of which a theme file
// gives, and how many series there are.
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
	VizFamily1:       "viz.family.1",
	VizFamily2:       "viz.family.2",
	VizFamily3:       "viz.family.3",
	VizFamily4:       "viz.family.4",
	VizVersion1:      "viz.version.1",
	VizVersion2:      "viz.version.2",
	VizVersion3:      "viz.version.3",
	VizVersion4:      "viz.version.4",
	VizVersion5:      "viz.version.5",
	VizVersion6:      "viz.version.6",
	VizDay0:          "viz.day.0",
	VizDay1:          "viz.day.1",
	VizDay2:          "viz.day.2",
	VizDay3:          "viz.day.3",
	VizDay4:          "viz.day.4",
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
// text in the terminal's own foreground, and the dim text and surfaces it
// draws faint or reversed.
func (t Theme) Colour(tok Token) color.Color {
	if tok <= 0 || tok >= tokens {
		return nil
	}
	return t.colours[tok]
}

// Faint reports whether the theme draws tok faint: the terminal theme's dim
// text, borders and tracks, in the terminal's own foreground, as of the
// terminal's sixteen colours none is dim on every terminal, bright black
// being Solarized Dark's background; and its older versions, in their
// family's colour, as it mixes no colour toward a canvas.
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

// derive works out each of the charts' tokens the theme leaves out from the
// base tokens, so a theme without them still draws every chart. The series
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
	or(VizTrack, c[BgSubtle])
	or(VizPace, c[TextPrimary])
	or(VizReserve, c[AccentAttention])
	for i, from := range []Token{AccentKey, StatePositive, AccentPrimary, AccentMode, TextSecondary, AccentAttention} {
		or(VizSeries1+Token(i), c[from])
	}
	for i, from := range []Token{AccentKey, AccentMode, AccentPrimary, TextTertiary} {
		or(VizFamily1+Token(i), c[from])
	}
	for i, slot := range versionSlots {
		or(VizVersion1+Token(i), t.version(c[slot.family], slot.older))
	}
	or(VizDay0, c[BgSubtle])
	for i, step := range t.Steps(c[AccentKey]) {
		or(VizDay1+Token(i), step)
	}
}

// versionSlots are the families viz.version.1 to viz.version.6 take their
// hues from, as the version table assigns them, and which are older than
// their family's newest: Opus 5.5 and Opus 5, Sonnet 5.5 and Sonnet 5, Fable
// 5.1, and Haiku 5.5.
var versionSlots = [...]struct {
	family Token
	older  bool
}{
	{family: VizFamily1},
	{family: VizFamily1, older: true},
	{family: VizFamily2},
	{family: VizFamily2, older: true},
	{family: VizFamily3},
	{family: VizFamily4},
}

// OlderFade is how far a version older than its family's newest is mixed
// toward the canvas from its family's hue, where the theme gives it no shade
// of its own.
const OlderFade = 0.3

// version is the shade of a version of the family whose hue is given: the hue
// itself, for the family's newest; for an older one, the hue mixed OlderFade
// toward the canvas, where the theme paints one, else the hue, as the
// terminal theme draws an older version faint instead.
func (t Theme) version(family color.Color, older bool) color.Color {
	if !older || !t.Paints() {
		return family
	}
	return Mix(family, t.colours[Canvas], OlderFade)
}

// Steps are the four steps a ramp of shades takes toward hue, from the
// theme's viz.day.0, as Shades works them out on its canvas; or, in a theme
// that blends nothing, as the terminal's doesn't, hue at every step, a glyph's
// density telling them apart.
func (t Theme) Steps(hue color.Color) [4]color.Color {
	if !t.Paints() {
		return [4]color.Color{hue, hue, hue, hue}
	}
	return Shades(t.colours[VizDay0], hue, !Dark(t.colours[Canvas]))
}

// Shades is the rule every ramp of shades is drawn by: from base, 40% and
// then 70% of the way to hue, then hue itself, then hue mixed 55% toward
// white, or toward black on a light canvas.
func Shades(base, hue color.Color, light bool) [4]color.Color {
	var far color.Color = color.White
	if light {
		far = color.Black
	}
	return [4]color.Color{Mix(base, hue, 0.4), Mix(base, hue, 0.7), hue, Mix(hue, far, 0.55)}
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
