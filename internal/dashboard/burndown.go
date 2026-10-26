package dashboard

import (
	"github.com/leeovery/switchboard/internal/theme"
)

// outMark is ✕, where a burn-down's window runs out.
const outMark = "✕"

// capFade is how far a burn-down's cap fades into the canvas, drawn faint.
const capFade = 0.4

var (
	// outInk is a burn-down's ✕ where its window runs out.
	outInk = ink{token: theme.StateDestructive, bold: true}
	// capDotInk is a burn-down's cap: viz.reserve, drawn faint.
	capDotInk = ink{token: theme.VizReserve, fade: capFade}
)

// dots are braille drawn over a chart h rows tall: each cell's dots, two
// across and four down, and the ink of the last drawn through it.
type dots struct {
	h     int
	cells map[[2]int]brailleCell
}

// brailleCell is a cell of braille: its dots, and the ink of the last drawn
// through it.
type brailleCell struct {
	bits rune
	ink  ink
}

// dotBits are a braille cell's dots, by column and row.
var dotBits = [2][4]rune{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}}

// newDots is braille over a chart h rows tall, no dot drawn.
func newDots(h int) dots {
	return dots{h: h, cells: make(map[[2]int]brailleCell)}
}

// dot draws a dot at column px and row py of the dots, its cell in the ink
// k.
func (d dots) dot(px, py int, k ink) {
	if px < 0 || py < 0 || py >= 4*d.h {
		return
	}
	at := [2]int{px / 2, py / 4}
	cell := d.cells[at]
	cell.ink = k
	cell.bits |= dotBits[px%2][py%4]
	d.cells[at] = cell
}

// draw draws the braille on p, each cell on a blank cell alone, never over
// what's drawn there already.
func (d dots) draw(c *canvas, p plot) {
	top := p.floor - p.h + 1
	for at, cell := range d.cells {
		if here := c.at(p.x+at[0], top+at[1]); here != nil && *here == blank {
			c.text(p.x+at[0], top+at[1], string(0x2800+cell.bits), cell.ink)
		}
	}
}
