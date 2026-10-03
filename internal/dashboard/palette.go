package dashboard

import (
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
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

// Blank is a blank line width cells wide, of the canvas where the look
// paints it: "" where it doesn't.
func (l Look) Blank(width int) string {
	if !l.paints {
		return ""
	}
	return l.render(strings.Repeat(" ", max(width, 0)), ink{})
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
	// on is the surface it's drawn on, such as a selected row's: the zero
	// Token for the canvas.
	on theme.Token
	// ramp marks a bar's filled cell, coloured by where it sits along the
	// bar, at, from 0 at its first cell to 1 at its last, rather than by a
	// token.
	ramp bool
	at   float64
	bold bool
}

var (
	textInk       = ink{token: theme.TextTertiary}
	dimInk        = ink{token: theme.TextSubtle}
	nameInk       = ink{token: theme.AccentMode, bold: true}
	accentInk     = ink{token: theme.AccentMode}
	borderInk     = ink{token: theme.Border}
	bestBorderInk = ink{token: theme.AccentMode}
	titleInk      = ink{token: theme.TextPrimary, bold: true}
	badgeInk      = ink{token: theme.AccentMode, bold: true}
	pinInk        = ink{token: theme.AccentPrimary}
	pinBadgeInk   = ink{token: theme.AccentPrimary, bold: true}
	primaryInk    = ink{token: theme.TextTertiary}
	trackInk      = ink{token: theme.VizTrack}
	paceInk       = ink{token: theme.VizPace}
	reserveInk    = ink{token: theme.VizReserve}
	warningInk    = ink{token: theme.AccentAttention}
	errorInk      = ink{token: theme.StateDestructive}
	exhaustedInk  = ink{token: theme.StateDestructive, bold: true}
)

// render draws text in the ink, as the look draws it.
func (l Look) render(text string, k ink) string {
	fg, bg := l.colour(k), l.surface(k)
	if !l.styled || text == "" || (fg == nil && bg == nil && !k.bold) {
		return text
	}
	style := lipgloss.NewStyle().Bold(k.bold)
	if fg != nil {
		style = style.Foreground(fg)
	}
	if bg != nil {
		style = style.Background(bg)
	}
	return style.Render(text)
}

// colour is the ink's colour in the look: nil without colour.
func (l Look) colour(k ink) color.Color {
	switch {
	case !l.coloured:
		return nil
	case k.ramp:
		return l.ramp(k.at)
	default:
		return l.theme.Colour(k.token)
	}
}

// surface is the colour the ink is drawn on in the look: its own surface's,
// else the canvas, where the look paints it.
func (l Look) surface(k ink) color.Color {
	if k.on != 0 && l.coloured {
		return l.theme.Colour(k.on)
	}
	return l.Canvas()
}

// A window's tone turns to attention, then to destructive, at these
// fractions used.
const (
	attentionFrom   = 0.7
	destructiveFrom = 0.9
)

// tone is a window's colour by how much of it is used: positive, then from
// 70% attention, and from 90% or once it's exhausted, destructive.
func tone(w quota.Window, p score.Projection) theme.Token {
	switch {
	case p.Kind == score.Exhausted || w.Utilization >= destructiveFrom:
		return theme.StateDestructive
	case w.Utilization >= attentionFrom:
		return theme.AccentAttention
	default:
		return theme.StatePositive
	}
}
