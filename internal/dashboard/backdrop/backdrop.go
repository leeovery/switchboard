// Package backdrop is the terminal's background, for a screen drawn full in
// the user's theme: asked of the terminal as the screen starts, the screen
// blank till it answers or has had its time to, the half of the pair in
// force picked by how dark it is, set to the canvas of a theme that paints
// one, and put back as the screen stops.
package backdrop

import (
	"fmt"
	"image/color"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/theme"
)

var logger = logs.For("backdrop")

// Unanswered says the terminal has had its time to say what its background
// is.
type Unanswered struct{}

// Ask asks the terminal what its background is (OSC 11), and has after
// deliver Unanswered once theme.AnswerWithin has passed, for the screen to
// stay blank till one or the other, so as not to flash one theme then the
// other.
func Ask(after func(time.Duration, tea.Msg) tea.Cmd) tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, after(theme.AnswerWithin, Unanswered{}))
}

// Backdrop is the terminal's background: whether it's dark, as the terminal
// said, what it was before the screen set its own, and whether it has. The
// zero Backdrop is a terminal not yet asked, its background never set.
type Backdrop struct {
	// settled is set once the terminal has said what its background is, or
	// had its time to: the screen is drawn from then on.
	settled bool
	// heard is set once the terminal has answered the one question the
	// screen asks it, which it may do after its time.
	heard bool
	// dark is whether the terminal's background is dark: a terminal that
	// didn't say is taken for dark.
	dark bool
	// original is the terminal's background as it said before the screen set
	// its own, to set back: nil where it hasn't said, or said a canvas of the
	// screen's own.
	original color.Color
	// set is set once the screen has set the terminal's background, which is
	// put back as the screen stops.
	set bool
}

// Settled reports whether the terminal has said what its background is, or
// had its time to: till then the screen is blank.
func (b Backdrop) Settled() bool {
	return b.settled
}

// Answered takes in the terminal's answer to the one question the screen
// asks it, its background, whenever it comes, the screen drawn in pair: as
// the screen asks before it sets its own, it's the background the screen
// found, and the half of the pair goes by how dark it is. One that's a
// canvas the screen paints may be one an exit it couldn't catch left, so
// it's never set back: OSC 111 resets the terminal's instead. Only the first
// answer counts. It reports whether the screen is to be drawn in the theme
// in force afresh: once settled, or should a late answer turn the half of
// the pair.
func (b Backdrop) Answered(background color.Color, pair theme.Pair) (Backdrop, bool) {
	if b.heard {
		return b, false
	}
	painted := paintedBy(background, pair)
	b.heard, b.original = true, background
	if painted {
		b.original = nil
	}
	logger.Debug("terminal background", "answered", background != nil, "painted", painted, "late", b.settled)
	return b.settle(theme.Dark(background))
}

// NoAnswer takes the terminal for dark, having had its time to say what its
// background is, unless it has said. It reports whether the screen is to be
// drawn in the theme in force afresh.
func (b Backdrop) NoAnswer() (Backdrop, bool) {
	if b.settled {
		return b, false
	}
	return b.settle(theme.Dark(nil))
}

// settle has the screen drawn, from now on, as on a terminal whose
// background is dark, or light, reporting whether that's news.
func (b Backdrop) settle(dark bool) (Backdrop, bool) {
	if b.settled && b.dark == dark {
		return b, false
	}
	b.settled, b.dark = true, dark
	return b, true
}

// InForce is the theme of pair the screen is drawn in, by the terminal's
// background.
func (b Backdrop) InForce(pair theme.Pair) theme.Theme {
	return pair.For(b.dark)
}

// DrawIn is b once the screen is drawn in t: should t paint its canvas, the
// terminal's background is set to it, and so is to be put back.
func (b Backdrop) DrawIn(t theme.Theme) Backdrop {
	b.set = b.set || t.Paints()
	return b
}

// DrawInForce is b once the screen is drawn in the theme of pair in force,
// as DrawIn has it, and that theme.
func (b Backdrop) DrawInForce(pair theme.Pair) (Backdrop, theme.Theme) {
	t := b.InForce(pair)
	return b.DrawIn(t), t
}

// Background is what the screen sets the terminal's background to, drawn in
// look: the look's canvas, where it paints one; else, once the screen has
// set its own, the background the terminal said it found, as the reset nil
// has Bubble Tea write (OSC 111) restores the terminal profile's own, not one
// set since; and nil where the terminal didn't say, or nothing was set.
func (b Backdrop) Background(look dashboard.Look) color.Color {
	if canvas := look.Canvas(); canvas != nil || !b.set {
		return canvas
	}
	return b.original
}

// PutBack puts back the terminal's background on w as the screen found it,
// once the screen has set its own: the colour the terminal said it was, or,
// where it didn't say, the terminal's own default, which OSC 111 restores.
func (b Backdrop) PutBack(w io.Writer) {
	switch {
	case !b.set:
		return
	case b.original == nil:
		_, _ = io.WriteString(w, ansi.ResetBackgroundColor)
	default:
		_, _ = io.WriteString(w, ansi.SetBackgroundColor(rgbHex(b.original)))
	}
}

// paintedBy reports whether background is a canvas the screen paints: a
// built-in theme's, or one of pair's.
func paintedBy(background color.Color, pair theme.Pair) bool {
	if background == nil {
		return false
	}
	for _, t := range append(theme.Builtins(), pair.Light, pair.Dark) {
		if canvas := t.Colour(theme.Canvas); canvas != nil && rgbHex(canvas) == rgbHex(background) {
			return true
		}
	}
	return false
}

// rgbHex is c written #rrggbb.
func rgbHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}
