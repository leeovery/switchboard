package dashboard

import "github.com/leeovery/switchboard/internal/theme"

// What travels a cord: a pulse, its head bright and pulseLength cells in
// all, each behind it glowing glowStep less than the one before, from
// trailGlow; and, while an answer streams, a shimmer, every shimmerEvery-th
// cell lit, glowing shimmerGlow.
const (
	pulseLength  = 5
	trailGlow    = 0.7
	glowStep     = 0.12
	shimmerEvery = 4
	shimmerGlow  = 0.75
)

// The glyphs at a cord's ends: its plug, at its call's end; a jack a cord
// ends at, one nothing is plugged into, and one whose request the upstream
// refused; and a cord hanging loose from a jack.
const (
	plugGlyph    = "●"
	pluggedJack  = "◉"
	freeJack     = "○"
	refusedJack  = "✕"
	hangingGlyph = "╌"
)

// cordGlyphs are how a cord is drawn: along a row, turning down from it, down
// a column, and turning along again.
type cordGlyphs struct {
	along, turnDown, down, turnAlong string
}

// heavyCord is how a cord is drawn, and lightCord an idle call's where the
// look can't dim it.
var (
	heavyCord = cordGlyphs{along: "━", turnDown: "┓", down: "┃", turnAlong: "┗"}
	lightCord = cordGlyphs{along: "─", turnDown: "┐", down: "│", turnAlong: "└"}
)

// restingCord is how far a cord at rest fades toward the canvas: keeping 45%
// of accent.key, mix(canvas, accent.key, 0.45).
const restingCord = 0.55

// cordInk is a call's cord's ink, which says what's happening, never which
// account: accent.key while its session works, and at rest, faded
// restingCord toward the canvas.
func cordInk(works bool) ink {
	k := ink{token: theme.AccentKey}
	if !works {
		k.fade = restingCord
	}
	return k
}

// cordDrawn are the glyphs a call's cord is drawn in, in the look: heavy, but
// light at rest where the look can't dim it.
func (l Look) cordDrawn(works bool) cordGlyphs {
	if !works && !l.blends() {
		return lightCord
	}
	return heavyCord
}

// cord is a cord from a call's plug, in column plug of the call's row from,
// to a jack on row to: along the call's row, and where the jack's row is
// lower, down at its bend, then along that to the jack.
type cord struct {
	plug, from, to, bend int
}

// point is a cell: x along row y.
type point struct {
	x, y int
}

// path is the cells the cord runs through, from its plug to its jack, in
// column jack: along the call's row, and where it bends, down its bend and
// along the jack's row.
func (k cord) path(jack int) []point {
	if k.from == k.to {
		return rowCells(k.plug, jack+1, k.from)
	}
	path := rowCells(k.plug, k.bend, k.from)
	for y := k.from; y <= k.to; y++ {
		path = append(path, point{k.bend, y})
	}
	return append(path, rowCells(k.bend+1, jack+1, k.to)...)
}

// rowCells are the cells of row y from column from to before column to.
func rowCells(from, to, y int) []point {
	var cells []point
	for x := from; x < to; x++ {
		cells = append(cells, point{x, y})
	}
	return cells
}

// glyph is how the cord draws its cell p, the i-th of its path, of last+1,
// in the glyphs g: its plug, its jack, along a row, or at its bend, turning
// down, down, or turning along to its jack.
func (k cord) glyph(p point, i, last int, g cordGlyphs) string {
	switch {
	case i == 0:
		return plugGlyph
	case i == last:
		return pluggedJack
	case k.from == k.to || p.x != k.bend:
		return g.along
	case p.y == k.from:
		return g.turnDown
	case p.y == k.to:
		return g.turnAlong
	default:
		return g.down
	}
}

// travel draws what travels a cord along path, in the colour of token, as
// the call c tells: while its answer streams, every shimmerEvery-th cell
// lit, as far along as its shimmer has moved, and its jack lit; refused, ✕
// on its jack, red; and over them, the pulse travelling it.
func travel(c *canvas, path []point, token theme.Token, call Call) {
	last := len(path) - 1
	switch call.Doing {
	case Streaming:
		for i := 1; i < last; i++ {
			if (i+call.Shimmer)%shimmerEvery == 0 {
				relight(c, path[i], ink{token: token, glow: shimmerGlow, bold: true})
			}
		}
		relight(c, path[last], titleInk)
	case Refused:
		c.text(path[last].x, path[last].y, refusedJack, exhaustedInk)
	}
	if call.Pulsing {
		pulse(c, path, call.Pulse, token)
	}
}

// pulse draws the pulse p along path: its head bright, and the cells behind
// it glowing less, in the colour of token, or, a refusal's, red.
func pulse(c *canvas, path []point, p Pulse, token theme.Token) {
	if p.Red {
		token = theme.StateDestructive
	}
	last := len(path) - 1
	head, behind := int(p.Along*float64(last)), -1
	if p.Back {
		head, behind = last-head, 1
	}
	for k := range pulseLength {
		i := head + k*behind
		if i < 0 || i > last {
			continue
		}
		lit := titleInk
		if k > 0 {
			lit = ink{token: token, glow: trailGlow - glowStep*float64(k)}
		}
		relight(c, path[i], lit)
	}
}

// corded marks the cells of path as the cord of the call plugged in at p,
// for what travels it to move frames on: drawn, it's on screen.
func (c *canvas) corded(path []point, p Plug) {
	c.marked(path, mark{cord: p})
}

// hung marks the cells of path as the cord l hangs loose, for its fading to
// move frames on: drawn, it's on screen.
func (c *canvas) hung(path []point, l Loose) {
	c.marked(path, mark{cord: Plug{Session: l.Session, Account: l.From}, loose: true})
}

// relight draws the cell at p in the ink k, its glyph as it is.
func relight(c *canvas, p point, k ink) {
	if here := c.at(p.x, p.y); here != nil && here.glyph != "" {
		here.ink = k
	}
}
