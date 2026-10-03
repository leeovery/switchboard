package dashboard

import (
	"image/color"
	"math"

	"charm.land/lipgloss/v2"

	"github.com/leeovery/switchboard/internal/theme"
)

// Look is how a frame is drawn: in a theme's colours, with its blends worked
// out against the colour beneath them, and its canvas painted on every cell
// where the dashboard fills the screen; or, as NO_COLOR asks, without colour,
// state told by its glyphs and bold. The zero Look draws text alone, without
// escapes.
type Look struct {
	theme theme.Theme
	// ground is what blends are worked out against: the canvas the look
	// paints, or the terminal's own background.
	ground color.Color
	// styled is set for every look but the zero one, coloured for every look
	// but NO_COLOR's, and paints for one that fills the screen with a theme
	// that paints its canvas.
	styled, coloured, paints bool
}

// Screen is the look of the dashboard full screen in a theme: its canvas,
// where it has one, painted on every cell, and its blends worked out against
// it.
func Screen(t theme.Theme) Look {
	return Look{theme: t, ground: t.Colour(theme.Canvas), styled: true, coloured: true, paints: t.Paints()}
}

// Print is the look of the dashboard printed into the scrollback in a theme:
// no canvas painted, its blends worked out against the terminal's background
// as the terminal gave it, or where it gave none, the theme's canvas.
func Print(t theme.Theme, background color.Color) Look {
	if background == nil {
		background = t.Colour(theme.Canvas)
	}
	return Look{theme: t, ground: background, styled: true, coloured: true}
}

// NoColour is the look NO_COLOR asks for: no canvas and no colour, bold alone.
func NoColour() Look {
	return Look{styled: true}
}

// Canvas is the colour the look paints on every cell: nil where it paints
// none.
func (l Look) Canvas() color.Color {
	if !l.paints {
		return nil
	}
	return l.theme.Colour(theme.Canvas)
}

// Blend blends c by amount into the colour beneath it, from 0, all c, to 1,
// all beneath, and reports whether it could: a look without colour, or in the
// terminal's own, which the dashboard can't know, blends nothing, and draws
// shade glyphs instead.
func (l Look) Blend(c color.Color, amount float64) (color.Color, bool) {
	if !l.blends() || c == nil {
		return nil, false
	}
	return theme.Mix(c, l.ground, amount), true
}

// blends reports whether the look can blend its colours: it has colour, and
// its theme's colours are its own, not the terminal's.
func (l Look) blends() bool {
	return l.coloured && l.theme.Paints()
}

// ramp is the colour of a bar's ramp at t, from its first stop at 0 to its
// last at 1: blended between the two stops either side, or, in a look that
// blends nothing, the nearest.
func (l Look) ramp(t float64) color.Color {
	stops := l.theme.Ramp()
	last := len(stops) - 1
	pos := min(max(t, 0), 1) * float64(last)
	if !l.blends() {
		return stops[int(math.Round(pos))]
	}
	i := min(int(pos), last-1)
	return theme.Mix(stops[i], stops[i+1], pos-float64(i))
}

// ink is how a span is drawn: its colour, by what it means, on the canvas or
// a surface of its own, and whether it's bold. The zero ink leaves text as it
// is, but for the canvas it's painted on.
type ink struct {
	token theme.Token
	// fade blends the colour so far into the colour beneath, from 0, all the
	// colour, to 1, all beneath, where the look can blend: a mark drawn
	// faint, as a short bar's empty cells are.
	fade float64
	// glow blends the colour so far toward text.primary, from 0, all the
	// colour, to 1, all text.primary: a cord lit by what travels it.
	glow float64
	// ramp marks a bar's filled cell, coloured by where it sits along the
	// bar, at, from 0 at its first cell to 1 at its last, rather than by a
	// token.
	ramp bool
	at   float64
	// on is the surface it's drawn on, such as a selected row's, or a bar's
	// projection under the cell its fill ends in: the zero hue for the
	// canvas.
	on   hue
	bold bool
}

// hue is a colour as an ink has one: a token's, or a bar's ramp's at a place
// along it, faded so far into the colour beneath. The zero hue is no colour.
type hue struct {
	token theme.Token
	ramp  bool
	at    float64
	fade  float64
}

// hue is the ink's own colour, as a hue.
func (k ink) hue() hue {
	return hue{token: k.token, ramp: k.ramp, at: k.at, fade: k.fade}
}

var (
	textInk       = ink{token: theme.TextTertiary}
	dimInk        = ink{token: theme.TextSubtle}
	nameInk       = ink{token: theme.AccentMode, bold: true}
	accentInk     = ink{token: theme.AccentMode}
	borderInk     = ink{token: theme.Border}
	bestBorderInk = ink{token: theme.AccentMode}
	focusInk      = ink{token: theme.AccentKey}
	titleInk      = ink{token: theme.TextPrimary, bold: true}
	badgeInk      = ink{token: theme.AccentMode, bold: true}
	pinBadgeInk   = ink{token: theme.AccentPrimary, bold: true}
	primaryInk    = ink{token: theme.TextTertiary}
	trackInk      = ink{token: theme.VizTrack}
	paceInk       = ink{token: theme.VizPace}
	reserveInk    = ink{token: theme.VizReserve}
	warningInk    = ink{token: theme.AccentAttention}
	alertInk      = ink{token: theme.AccentAttention, bold: true}
	errorInk      = ink{token: theme.StateDestructive}
	exhaustedInk  = ink{token: theme.StateDestructive, bold: true}
	labelInk      = ink{token: theme.TextSubtle, bold: true}
	mutedInk      = ink{token: theme.TextMuted}
	secondaryInk  = ink{token: theme.TextSecondary}
	strongInk     = ink{token: theme.TextSecondary, bold: true}
	faintInk      = ink{token: theme.TextFaint}
	positiveInk   = ink{token: theme.StatePositive}
	primedInk     = ink{token: theme.AccentPrimary}
	keyInk        = ink{token: theme.AccentKey, bold: true}
	noteInk       = ink{token: theme.TextSecondary}
)

// render draws text in the ink, as the look draws it.
func (l Look) render(text string, k ink) string {
	fg, bg := l.colour(k), l.surface(k)
	faint, reversed := l.faint(k), l.reversed(k)
	if !l.styled || text == "" || (fg == nil && bg == nil && !k.bold && !faint && !reversed) {
		return text
	}
	style := lipgloss.NewStyle().Bold(k.bold).Faint(faint).Reverse(reversed)
	if fg != nil {
		style = style.Foreground(fg)
	}
	if bg != nil {
		style = style.Background(bg)
	}
	return style.Render(text)
}

// faint reports whether the look draws the ink faint, in the terminal's own
// foreground, as its theme draws the ink's token.
func (l Look) faint(k ink) bool {
	return l.coloured && !k.ramp && l.theme.Faint(k.token)
}

// reversed reports whether the look draws the ink's surface by swapping the
// terminal's own foreground and background, as its theme draws the
// surface's token: until it's faded halfway, as a look that blends nothing
// shows a surface.
func (l Look) reversed(k ink) bool {
	return l.coloured && !k.on.ramp && l.theme.Reversed(k.on.token) && k.on.fade < 0.5
}

// colour is the ink's colour in the look, glowing and faded as the ink says:
// nil without colour.
func (l Look) colour(k ink) color.Color {
	if !l.coloured {
		return nil
	}
	h := k.hue()
	return l.faded(l.glowing(l.unfaded(h), k.glow), h.fade)
}

// glowing is c blended amount toward text.primary, as a lit cord glows: in a
// look that blends nothing, text.primary from halfway, else c as it is.
func (l Look) glowing(c color.Color, amount float64) color.Color {
	switch {
	case amount <= 0 || c == nil:
		return c
	case l.blends():
		return theme.Mix(c, l.theme.Colour(theme.TextPrimary), amount)
	case amount >= 0.5:
		return l.theme.Colour(theme.TextPrimary)
	default:
		return c
	}
}

// unfaded is the hue's colour in the look before it's faded: its token's, or
// its ramp's where it sits.
func (l Look) unfaded(h hue) color.Color {
	if h.ramp {
		return l.ramp(h.at)
	}
	return l.theme.Colour(h.token)
}

// faded is c blended amount into the colour beneath it, or, in a look that
// blends nothing, c as it is.
func (l Look) faded(c color.Color, amount float64) color.Color {
	if blended, ok := l.Blend(c, amount); ok && amount > 0 {
		return blended
	}
	return c
}

// surface is the colour the ink is drawn on in the look: its own surface's,
// faded as the ink says, else the canvas, where the look paints it. In a
// look that blends nothing, a surface shows until it's faded halfway.
func (l Look) surface(k ink) color.Color {
	if !l.coloured {
		return l.Canvas()
	}
	on := l.unfaded(k.on)
	if on == nil || (k.on.fade >= 0.5 && !l.blends()) {
		return l.Canvas()
	}
	return l.faded(on, k.on.fade)
}
