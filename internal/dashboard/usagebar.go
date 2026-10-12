package dashboard

import (
	"math"

	"github.com/leeovery/switchboard/internal/theme"
)

// staleFade is how far a stale reading's bar fades into the colour beneath:
// it keeps 35% of its colour. The Overview fades an account out of the pool
// as far, and one both stale and out of the pool keeps 35% of 35%.
const staleFade = 0.65

// usageBar is a window's bar as the usage block draws it: filled to used,
// from 0 to 1, more filling it, or on the Overview, while it eases to used
// after a read, to eased; its cap's mark where its account's cap is, at capAt
// of it, off the bar at 1 or more; and even pace's mark at pace of it, none
// where that's nil. An empty bar is its track alone, as of an account
// nothing is read of; a stale one is faded, nothing in it bold, and so, on
// the Overview, is one of an account out of the pool.
type usageBar struct {
	used, capAt float64
	pace        *float64
	eased       *float64
	empty       bool
	stale       bool
	out         bool
}

// draw draws the bar cells long from x along row y, as look draws its cells:
// its fill, eighths of a cell at a time, each cell in the ramp's colour where
// it sits; its track; its cap's mark on the cell's own background; and even
// pace's mark last, over the rest, on no background. A bar of no cells draws
// nothing, no mark either.
func (b usageBar) draw(c *canvas, look Look, x, y, cells int) {
	filled := 0
	if !b.empty {
		filled = halfEven(min(max(b.fills(), 0), 1) * float64(cells) * 8)
	}
	for i := range cells {
		glyph, k := barCell(look, min(max(filled-8*i, 0), 8), rampStep(i, cells))
		c.text(x+i, y, glyph, b.faded(k))
	}
	if b.empty || cells <= 0 {
		return
	}
	if at, ok := b.capCell(cells); ok {
		k := ink{token: theme.VizReserve}
		if look.blends() {
			_, under := barCell(look, min(max(filled-8*at, 0), 8), rampStep(at, cells))
			k.on = under.on
		}
		c.text(x+at, y, capMark, b.faded(k))
	}
	if b.pace != nil {
		c.text(x+min(halfEven(*b.pace*float64(cells)), cells-1), y, paceMark, b.faded(ink{token: theme.VizPace, bold: true}))
	}
}

// capCell is the cell of the bar, cells long, its cap's mark is drawn at,
// round(cap × cells), reporting false where it draws none: where that's off
// the bar, or the bar is empty or has no cells.
func (b usageBar) capCell(cells int) (int, bool) {
	if b.empty || cells <= 0 {
		return 0, false
	}
	at := halfEven(b.capAt * float64(cells))
	return at, at < cells
}

// barCell is a cell of a bar filled eighths of it, from 0 to 8, at a step t
// along the ramp, as look draws it: where it blends its colours, a filled
// cell is a space on the ramp's colour, a part-filled one its eighth block in
// that colour on the track, and an empty one a space on the track; where it
// doesn't, the fill is in blocks, █ and the eighths, in the ramp's colours,
// and the track ░, faint, or without colour, spaces.
func barCell(look Look, eighths int, t float64) (string, ink) {
	ramp := hue{ramp: true, at: t}
	track := hue{token: theme.VizTrack}
	switch {
	case look.blends() && eighths == 8:
		return " ", ink{on: ramp}
	case look.blends() && eighths > 0:
		return blocks[eighths], ink{ramp: true, at: t, on: track}
	case look.blends():
		return " ", ink{on: track}
	case eighths > 0:
		return blocks[eighths], ink{ramp: true, at: t}
	case look.coloured:
		return trackCell, trackInk
	default:
		return " ", ink{}
	}
}

// fills is how much of the bar is filled: as far as it has eased, while it
// eases, else as used.
func (b usageBar) fills() float64 {
	if b.eased != nil {
		return *b.eased
	}
	return b.used
}

// faded is the ink as the bar draws it: as it is, or, for a stale reading or
// an account out of the pool, faded as fade says, its surface too, and not
// bold.
func (b usageBar) faded(k ink) ink {
	fade := b.fade()
	if fade == 0 {
		return k
	}
	k.fade, k.bold, k.dim = fade, false, true
	if k.on != (hue{}) {
		k.on.fade = fade
	}
	return k
}

// fade is how far the bar fades into the colour beneath: staleFade, keeping
// 35% of its colour, where its reading is stale or its account out of the
// pool; the two compounded, keeping 35% of 35%, where both hold; else none.
func (b usageBar) fade() float64 {
	switch {
	case b.stale && b.out:
		return 1 - (1-staleFade)*(1-staleFade)
	case b.stale || b.out:
		return staleFade
	}
	return 0
}

// rampStep is where cell i of a bar cells long sits along the ramp, to the
// nearest of its nine steps, a half to the even one, as the frames are drawn.
func rampStep(i, cells int) float64 {
	return float64(halfEven(along(i, cells)*rampSteps)) / rampSteps
}

// halfEven is x rounded to the nearest whole number, a half to the even one,
// as the frames are drawn: 85.5 is 86, and 84.5 is 84.
func halfEven(x float64) int {
	return int(math.RoundToEven(x))
}
