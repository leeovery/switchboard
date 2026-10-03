package dashboard

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/theme"
)

const (
	// keyLabel is the key line's label.
	keyLabel = "KEY"
	// glyphsGap is the blank cells between the key line's glyphs, and
	// groupRule what runs between their groups.
	glyphsGap = 3
	groupRule = "   │   "
)

// The help's panel, as Portal's: its rows inset helpInset cells inside its
// border, its keys in a column, helpGap cells before what each does.
const (
	helpInset = 2
	helpGap   = 3
)

// forTheKey says the key to the glyphs is behind ?, where the view has no
// room to show it.
var forTheKey = line{{"?", keyInk}, {" for the key", dimInk}}

// glyph is a glyph of the view, drawn as the view draws it, and what it
// means.
type glyph struct {
	drawn line
	means string
}

// glyphs are the glyphs the key explains of the view shown. Of Runway, its
// legend's. Of Sessions, its switchboard's and its requests', as
// switchboardGlyphs has them. Of the cards, two groups: a chart's, in the
// style they draw, as chartGlyphs has them; then a bar's, its use, where
// it's heading, even pace and the reserve, and a session's dot on a card's
// edge. Each is drawn as the look draws it on a card: a bar's projection,
// where the look can't fade it, in shade.
func (f Frame) glyphs() [][]glyph {
	switch f.View {
	case Runway:
		return [][]glyph{f.legendGlyphs()}
	case Sessions:
		return switchboardGlyphs()
	}
	heading := line{{"██", ink{ramp: true, at: 1.0 / 3, fade: projectionFade}}}
	if !f.Look.blends() {
		heading = line{{strings.Repeat(shadeCell, 2), ink{ramp: true, at: 1.0 / 3}}}
	}
	var used line
	for i := range rampCells {
		used = append(used, span{blocks[8], ink{ramp: true, at: along(i, rampCells)}})
	}
	return [][]glyph{
		chartGlyphs(f.Chart),
		{
			{drawn: used, means: "used"},
			{drawn: heading, means: "heading"},
			{drawn: line{{paceMarker.text, ink{token: theme.VizPace, bold: true}}}, means: "even pace"},
			{drawn: line{reserveMarker}, means: "reserve"},
			{drawn: line{sessionDot(true)}, means: "session, lit while busy"},
		},
	}
}

// chartGlyphs are the glyphs of a card's chart in the style given, drawn in
// an open account's colour. A burn-down's: its room left, the dotted line
// where it's heading, ✕ where it runs out, and its reserve. A burn rate's:
// its use per 10 minutes, a bar used too fast to last, and the dotted line
// at the fastest that lasts. An hourglass's: its sand above, the room left,
// and below, what's used; and its stream, as thick as its recent rate,
// falling while its account is busy.
func chartGlyphs(style Chart) []glyph {
	open := ink{token: theme.StatePositive, fade: projectionFade}
	switch style {
	case BurnRate:
		return []glyph{
			{drawn: line{{"▃▅", open}}, means: "use per 10 min"},
			{drawn: line{{"▆▆", fastInk}}, means: "too fast to last"},
			{drawn: line{{"⠂⠂⠂", lastingInk}}, means: "fastest that lasts"},
		}
	case Hourglass:
		return []glyph{
			{drawn: line{{"▀▀", open}}, means: "room left"},
			{drawn: line{{"▄▄", open}}, means: "used"},
			{drawn: line{{"▐▌", ink{token: theme.StatePositive}}}, means: "its recent rate, falling while busy"},
		}
	default:
		return []glyph{
			{drawn: line{{"▆▆", open}}, means: "room left"},
			{drawn: line{{"⠂⠄⡀", ink{token: theme.StatePositive}}}, means: "heading"},
			{drawn: line{{outMark, outInk}}, means: "runs out"},
			{drawn: line{{"⠠ ⠠", ink{token: theme.VizReserve, fade: reserveFade}}}, means: "reserve"},
		}
	}
}

// switchboardGlyphs are the glyphs of Sessions, as it draws them, in two
// groups: its switchboard's, a cord from a call to its account's line, a free
// jack, and a cord hanging loose from one, as a move lets it go; then what a
// call's row says of its request: asking, its answer streaming back, refused,
// throttled, and moved here.
func switchboardGlyphs() [][]glyph {
	return [][]glyph{
		{
			{drawn: line{{plugGlyph + strings.Repeat(heavyCord.along, 2) + pluggedJack, cordInk(1, true)}}, means: "a session's cord to its account"},
			{drawn: line{{freeJack, freeJackInk}}, means: "a free jack"},
			{drawn: line{{strings.Repeat(hangingGlyph, 2), dimInk}, {freeJack, freeJackInk}}, means: "a cord a move let go"},
		},
		{
			{drawn: line{{"↑", nameInk}}, means: "asking"},
			{drawn: line{{"↓", titleInk}}, means: "streaming back, ~ its tokens so far"},
			{drawn: line{{refusedJack, exhaustedInk}}, means: "a limit or a refusal"},
			{drawn: line{{"…", dimInk}}, means: "throttled"},
			{drawn: line{{"↪", nameInk}}, means: "moved here"},
		},
	}
}

// rampCells is how many cells the key's bar of use takes: one a stop of the
// ramp.
const rampCells = 4

// keyLine is the key as a line under the cards of the strips given: its
// label, in their labels' column, then each glyph and what it means, its
// groups ruled apart.
func (f Frame) keyLine(strips []strip) line {
	l := line{{keyLabel, labelInk}, spaces(f.labelled(strips) - margin - ansi.StringWidth(keyLabel))}
	for i, group := range f.glyphs() {
		if i > 0 {
			l = append(l, span{groupRule, faintInk})
		}
		for j, g := range group {
			if j > 0 {
				l = append(l, spaces(glyphsGap))
			}
			l = append(append(l, g.drawn...), span{" " + g.means, mutedInk})
		}
	}
	return l
}

// keyFits reports whether the key line, under the strips given, fits the
// frame's width.
func (f Frame) keyFits(strips []strip) bool {
	return margin+f.keyLine(strips).width() <= f.edge()
}

// help draws the help over the middle of the frame, as Portal's is drawn: in
// a border, its title and how it closes; then every key there is and what it
// does; then the key to the glyphs, a chart's, then a bar's, the keys and
// the glyphs in one column; a rule between each. Where the frame is too
// small for it, it's cut to fit.
func (f Frame) help(c *canvas) {
	keys := make([]glyph, len(f.Help))
	for i, k := range f.Help {
		keys[i] = glyph{drawn: line{{k.Key, keyInk}}, means: k.Does}
	}
	groups := slices.Concat([][]glyph{keys}, f.glyphs())
	column := 0
	for _, group := range groups {
		for _, g := range group {
			column = max(column, g.drawn.width())
		}
	}
	title := line{{"?", ink{token: theme.AccentPrimary, bold: true}}, {" Keys", titleInk}}
	closes := line{{"esc close", mutedInk}}
	compartments := [][]line{{slices.Concat(title, line{spaces(1)}, closes)}}
	inner := compartments[0][0].width()
	for _, group := range groups {
		rows := make([]line, len(group))
		for i, g := range group {
			rows[i] = slices.Concat(g.drawn, line{spaces(column - g.drawn.width() + helpGap), {g.means, secondaryInk}})
			inner = max(inner, rows[i].width())
		}
		compartments = append(compartments, rows)
	}
	inner = min(inner, f.Width-2-2*helpInset)
	compartments[0][0] = spread(title, closes, inner)
	panel(c, compartments, inner, f.Width, f.Height)
}

// panel draws compartments of lines, inner cells wide, in a rounded border
// with a rule between each compartment, centred on a canvas width cells wide
// and height tall, each line inset helpInset cells, over whatever's drawn
// there: from its top, where it's taller than the canvas.
func panel(c *canvas, compartments [][]line, inner, width, height int) {
	if inner < 1 {
		return
	}
	rows := 1 + len(compartments)
	for _, lines := range compartments {
		rows += len(lines)
	}
	inside := inner + 2*helpInset
	x, y := (width-inside-2)/2, max((height-rows)/2, 0)
	c.text(x, y, "╭"+rule(inside)+"╮", borderInk)
	for i, lines := range compartments {
		if i > 0 {
			y++
			c.text(x, y, "├"+rule(inside)+"┤", borderInk)
		}
		for _, l := range lines {
			y++
			c.text(x, y, "│", borderInk)
			c.text(x+1, y, strings.Repeat(" ", inside), ink{})
			c.line(x+1+helpInset, y, l.fit(inner))
			c.text(x+inside+1, y, "│", borderInk)
		}
	}
	c.text(x, y+1, "╰"+rule(inside)+"╯", borderInk)
}
