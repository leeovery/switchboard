package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

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

const (
	// idleFade dims an idle call's cord halfway into the canvas.
	idleFade = 0.5
	// plugGlyph is a cord's plug, at its call's end.
	plugGlyph = "●"
	// refusedJack is a jack whose request the upstream refused.
	refusedJack = "✕"
	// hangingGlyph is a cord hanging loose from a jack.
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

// cord is a cord from a call to a jack of its line's panel: the seat plugged
// in, the place of its line, whose colour it takes, and whether its call is
// busy; from the call's row, along it, and, where the jack's row is lower,
// down at its bend, then along that to the jack.
type cord struct {
	plug     Plug
	place    int
	busy     bool
	from, to int
	bend     int
}

// point is a cell: x along row y.
type point struct {
	x, y int
}

// bend sets the column each of the bay's bent cords turns down at, so none
// crosses another: the one that starts highest furthest right, bendClear
// cells clear of a cord hanging loose from a jack, the rest in turn
// bendsApart cells to the left, or closer where that leaves them short of
// the plugs. It reports false where they don't fit, a cell apart at least.
func (b *bay) bend() bool {
	var bent []int
	for i, k := range b.cords {
		if k.to != k.from {
			bent = append(bent, i)
		}
	}
	if len(bent) == 0 {
		return true
	}
	right, left := b.x-looseCells-bendClear, plugAt+1
	apart := bendsApart
	if n := len(bent) - 1; n > 0 {
		apart = min(apart, (right-left)/n)
	}
	if right < left || apart < 1 {
		return false
	}
	for j, i := range bent {
		b.cords[i].bend = right - j*apart
	}
	return true
}

// path is the cells the cord runs through, from its plug to its jack, in
// column jack: along the call's row, and where it bends, down its bend and
// along the jack's row.
func (k cord) path(jack int) []point {
	if k.from == k.to {
		return rowCells(plugAt, jack+1, k.from)
	}
	path := rowCells(plugAt, k.bend, k.from)
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

// cordInk is the ink of a cord to the line in place n: its viz.series
// colour, dimmed halfway into the canvas while its call is idle.
func cordInk(n int, busy bool) ink {
	k := ink{token: theme.SeriesOf(n)}
	if !busy {
		k.fade = idleFade
	}
	return k
}

// drawCord draws the cord k, its jack in column jack: its plug, the cord, and
// the jack, in its line's colour, dimmed while its call is idle, or, where
// the look can't dim it, drawn light; then over it what travels it, as the
// request stream tells.
func (f Frame) drawCord(c *canvas, k cord, jack int) {
	glyphs := heavyCord
	if !k.busy && !f.Look.blends() {
		glyphs = lightCord
	}
	path, base := k.path(jack), cordInk(k.place, k.busy)
	for i, p := range path {
		c.text(p.x, p.y, k.glyph(p, i, len(path)-1, glyphs), base)
	}
	if call, ok := f.Traffic.call(k.plug); ok {
		travel(c, path, theme.SeriesOf(k.place), call)
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

// relight draws the cell at p in the ink k, its glyph as it is.
func relight(c *canvas, p point, k ink) {
	if here := c.at(p.x, p.y); here != nil && here.glyph != "" {
		here.ink = k
	}
}

// shows reports whether any of the cord's cells, its jack in column jack,
// shows on screen, as s has it.
func (k cord) shows(s sight, jack int) bool {
	return slices.ContainsFunc(k.path(jack), func(p point) bool { return s.shows(p.x, p.y, 1, 1) })
}

// looseCord is a cord hanging loose from the jack on row y, cells long,
// faded so far, from 0 to 1, and fading where a move held by no limit let
// it go.
type looseCord struct {
	y, cells int
	fade     float64
	fades    bool
}

// looseCords are the cords hanging loose from the free jacks of the bay b of
// doc at now: from each placeholder's jack, the cord its move let go,
// looseCells long, fading as the move says, but where a limit moved it; then
// from each account's other free jacks, in turn, a stub for each session its
// limit moved off it, while the limit holds, the first stubCells long and
// each a cell shorter, and the cords moves let go once a listing of the
// router's sessions showed them done, fading.
func (f Frame) looseCords(doc status.Document, b bay, now time.Time) []looseCord {
	moves := latestMoves(f.Traffic.repatching())
	var loose []looseCord
	for i, p := range b.panels {
		plugged := make([]bool, p.rows)
		for _, r := range b.calls {
			if place(doc, r.assignment.Account) != i+1 {
				continue
			}
			plugged[r.jack] = true
			if m, ok := moves[r.seat()]; ok && r.gone {
				loose = append(loose, looseCord{y: p.top + 1 + r.jack, cells: looseCells, fade: m.Fade, fades: !m.Held})
			}
		}
		var free []int
		for j, taken := range plugged {
			if !taken {
				free = append(free, j)
			}
		}
		stubs := stubbed(doc, p.account, now)
		for k := 0; k < stubs && len(free) > 0; k++ {
			loose = append(loose, looseCord{y: p.top + 1 + free[0], cells: max(stubCells-k, 1)})
			free = free[1:]
		}
		for _, m := range f.Traffic.Moves {
			if m.Listed && m.From == p.account.ID && len(free) > 0 {
				loose = append(loose, looseCord{y: p.top + 1 + free[0], cells: looseCells, fade: m.Fade, fades: !m.Held})
				free = free[1:]
			}
		}
	}
	return loose
}

// hang draws the cords hanging loose from the jacks of the bay b of doc at
// now, as looseCords has them, ╌, dim, to their jacks' left, each faded as
// far as it has; and once it's gone, as gone says, not at all.
func (f Frame) hang(c *canvas, doc status.Document, b bay, now time.Time) {
	for _, l := range f.looseCords(doc, b, now) {
		if f.gone(l.fade) {
			continue
		}
		k := dimInk
		k.fade = l.fade
		c.text(b.x-l.cells, l.y, strings.Repeat(hangingGlyph, l.cells), k)
	}
}

// gone reports whether what's faded so far, from 0 to 1, shows no more: once
// it has faded all the way, or, in a look that can't fade it, halfway.
func (f Frame) gone(fade float64) bool {
	return fade >= 1 || (fade >= 0.5 && !f.Look.blends())
}

// stubbed is how many sessions doc's account a's limit moved off it, while
// the limit holds at now, as the router told of it: none otherwise.
func stubbed(doc status.Document, a status.Account, now time.Time) int {
	if e, ok := limitEvent(doc, a, now); ok {
		return e.Count
	}
	return 0
}
