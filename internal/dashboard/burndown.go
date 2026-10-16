package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/theme"
)

// The marks a burn-down draws besides its lines: its floor where no room is
// left, as at Claude's limit, and on along it till the limit lifts; and ✕,
// where its window runs out.
const (
	floorLine = "▁"
	outMark   = "✕"
)

// The glyphs the burn-down's key gives its braille lines: where its window's
// heading, and its account's cap.
const (
	headingKey = "⠢"
	capKey     = "⠤"
)

// capFade is how far a burn-down's cap fades into the canvas, drawn faint.
const capFade = 0.4

var (
	// outInk is a burn-down's ✕ where its window runs out.
	outInk = ink{token: theme.StateDestructive, bold: true}
	// capDotInk is a burn-down's cap: viz.reserve, drawn faint.
	capDotInk = ink{token: theme.VizReserve, fade: capFade}
)

// The ranks of a burn-down's braille lines, where they share a cell: where
// its window's heading is the more telling.
const (
	capRank = iota
	headingRank
)

// burnDown draws the chart's burn-down on p: each column to now filled to the
// room left at its time, as roomAt has it, a level falling from full, as
// level draws it, and none past now; while Claude's limit holds the window,
// a line along the floor on till it lifts, as heldAhead draws it; then on the
// cells left empty, in turn, so where they meet, the first drawn shows: its
// account's cap, where it has one, a faint braille line along its floor, as
// capDots draws it, and where it's heading, a braille line from the level
// now, as headingDots draws it; now's line, down its column; and ✕ where it
// runs out, over what it falls on. What it's heading for is drawn only where
// the room left now is known.
func (ch chart) burnDown(c *canvas, look Look, p plot) {
	for col := range p.now + 1 {
		if room, ok := ch.roomAt(ch.readAt(col, p.width)); ok {
			level(c, look, p.x+col, p.floor, p.h, room)
		}
	}
	if ch.course.held {
		ch.heldAhead(c, p)
	}
	d := newDots(p.h)
	if ch.capped {
		ch.capDots(d, p)
	}
	from, known := ch.roomAt(ch.readAt(p.now, p.width))
	if known {
		ch.headingDots(d, p, from)
	}
	d.draw(c, p)
	for r := range p.h {
		if here := c.at(p.x+p.now, p.floor-r); here != nil && *here == blank {
			c.text(p.x+p.now, p.floor-r, nowLine, faintInk)
		}
	}
	if col, ok := ch.outColumn(p); ok && known {
		c.text(p.x+col, p.floor-p.h+1+d.rowOf(ch.floorRoom())/4, outMark, outInk)
	}
}

// roomAt is the room the window had left at t, as the last reading at or
// before t left it, reporting false where none was read by then; but none
// from when Claude's limit was reached in it, while that holds it, where
// that's known.
func (ch chart) roomAt(t time.Time) (float64, bool) {
	if h := ch.course; h.held && !h.since.IsZero() && !t.Before(h.since) {
		return 0, true
	}
	used, ok := ch.usedAt(t)
	return 1 - used, ok
}

// level draws a burn-down's column at x, h rows tall up from row floor,
// filled to room, a share of the whole, in eighths of a cell, as fillUp fills
// it, at least its lowest eighth where any room is left: each row in the
// ramp's colour at its height read from the top, 1 − (r + 0.5) ÷ h for row r
// from the floor, chartFade into the canvas, so a level runs from the ramp's
// first stop to its last as it falls. Where none is left, as at Claude's
// limit, it's a line along the floor in state.destructive.
func level(c *canvas, look Look, x, floor, h int, room float64) {
	if room <= 0 {
		c.text(x, floor, floorLine, errorInk)
		return
	}
	fillUp(c, look, x, floor, max(halfEven(min(room, 1)*float64(8*h)), 1), func(r int) ink {
		return ink{ramp: true, at: 1 - (float64(r)+0.5)/float64(h), fade: chartFade}
	})
}

// heldAhead draws a line along p's floor in state.destructive from the
// column after now's, while Claude's limit holds the window, until it lifts,
// each column whose time starts before then, or to the chart's end where
// that isn't known.
func (ch chart) heldAhead(c *canvas, p plot) {
	back := ch.course.back
	for col := p.now + 1; col < p.width && (back.IsZero() || ch.at(float64(col)/float64(p.width)).Before(back)); col++ {
		c.text(p.x+col, p.floor, floorLine, errorInk)
	}
}

// capDots draws the account's cap on d, over p, as a faint braille line along
// its floor, where 1 − cap of the window is left: a dot at the right of
// every other cell, from the first.
func (ch chart) capDots(d dots, p plot) {
	row := d.rowOf(1 - ch.capAt)
	for col := 0; col < p.width; col += 2 {
		d.dot(2*col+1, row, capDotInk, capRank)
	}
}

// headingDots draws where the window's heading on d, over p, from, the room
// left now: a braille line in text.subtle, a dot at the left of each cell
// from the column after now's, on a straight line from the level now to
// where headingTo has it end, in the column before.
func (ch chart) headingDots(d dots, p plot, from float64) {
	end, to, ok := ch.headingTo(p)
	if !ok {
		return
	}
	pace, endShare := ch.pace(), ch.share(ch.headingEnd())
	for col := p.now + 1; col < end; col++ {
		at := (float64(col) + 0.25) / float64(p.width)
		progress := 1.0
		if endShare > pace {
			progress = min(max((at-pace)/(endShare-pace), 0), 1)
		}
		d.dot(2*col, d.rowOf(from+(to-from)*progress), dimInk, headingRank)
	}
}

// headingTo is where the window's heading ends on p: where it runs out, the
// column of ✕, as outColumn has it, and the room on its floor; else, where
// its projection keeps it short of running out, the chart's end, and the
// room it has left at its reset. It reports false where it's heading nowhere
// the chart draws.
func (ch chart) headingTo(p plot) (end int, room float64, ok bool) {
	if col, out := ch.outColumn(p); out {
		return col, ch.floorRoom(), true
	}
	if left := ch.course.left; left != nil {
		return p.width, *left, true
	}
	return 0, 0, false
}

// headingEnd is when the window's heading ends: when it runs out, or else at
// its reset.
func (ch chart) headingEnd() time.Time {
	if !ch.course.out.IsZero() {
		return ch.course.out
	}
	return ch.start.Add(ch.length)
}

// outColumn is the column of p the window runs out in, as its projection has
// it, now's column at the soonest, reporting false where it doesn't run out
// before its reset.
func (ch chart) outColumn(p plot) (int, bool) {
	if ch.course.out.IsZero() {
		return 0, false
	}
	return max(min(int(ch.share(ch.course.out)*float64(p.width)), p.width-1), p.now), true
}

// floorRoom is the room left on the burn-down's floor where its window runs
// out: at its account's cap, where that's where it runs out, the reserve,
// 1 − cap; else none.
func (ch chart) floorRoom() float64 {
	if ch.course.atCap {
		return 1 - ch.capAt
	}
	return 0
}

// share is how far through the window t is, from 0 at its start to 1 at its
// reset.
func (ch chart) share(t time.Time) float64 {
	return float64(t.Sub(ch.start)) / float64(ch.length)
}

// at is the time a share of the way through the window.
func (ch chart) at(share float64) time.Time {
	return ch.start.Add(time.Duration(share * float64(ch.length)))
}

// dots are braille drawn over a chart h rows tall: each cell's dots, two
// across and four down, and the ink and rank of the most telling line
// through it.
type dots struct {
	h     int
	cells map[[2]int]brailleCell
}

// brailleCell is a cell of braille: its dots, and the ink and rank of the
// most telling line through it.
type brailleCell struct {
	bits rune
	ink  ink
	rank int
}

// dotBits are a braille cell's dots, by column and row.
var dotBits = [2][4]rune{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}}

// newDots is braille over a chart h rows tall, no dot drawn.
func newDots(h int) dots {
	return dots{h: h, cells: make(map[[2]int]brailleCell)}
}

// rowOf is the row of dots for room, a share of the chart from its floor:
// the top row for all of it, the bottom for none.
func (d dots) rowOf(room float64) int {
	return halfEven((1 - min(max(room, 0), 1)) * float64(4*d.h-1))
}

// dot draws a dot at column px and row py of the dots, in the ink k where
// rank is at least that of what's been drawn in its cell already.
func (d dots) dot(px, py int, k ink, rank int) {
	if px < 0 || py < 0 || py >= 4*d.h {
		return
	}
	at := [2]int{px / 2, py / 4}
	cell, ok := d.cells[at]
	if !ok || rank >= cell.rank {
		cell.ink, cell.rank = k, rank
	}
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
