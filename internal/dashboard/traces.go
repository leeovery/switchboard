package dashboard

import (
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// The columns of a trace a row an account, or a session, has from 110
// columns: from traceFrom to traceTail cells short of the frame's edge, its
// rate right-aligned in them.
const (
	traceFrom = 92
	traceTail = 8
)

// leastScale is the fewest requests a minute that fill a trace's height, so
// a quiet page isn't magnified.
const leastScale = 8

// traceFade is how far a trace's colour is mixed into the canvas: 80% of it
// shows.
const traceFade = 0.2

// traceLane is drawn on the bottom row of a trace's column that's empty, so
// the lane always shows.
const traceLane = "▁"

// tracing is a trace drawn on a canvas, which clips it: its columns, each
// its requests a minute, as many as it's drawn cells wide; where they lie;
// and whether its account is out of the pool, which fades it.
type tracing struct {
	on    *canvas
	cells []float64
	at    bounds
	faded bool
}

// holdingTraces is c holding the traces drawn on it, and on the canvases
// within it, till paintTraces paints them at the scale they share.
func holdingTraces(c *canvas) *canvas {
	c.traces = new([]tracing)
	return c
}

// paintTraces paints the traces c holds at the scale they share, as
// traceScale has it of them.
func (f Frame) paintTraces(c *canvas) {
	scale := f.traceScale(*c.traces)
	for _, t := range *c.traces {
		f.paintTrace(t, scale)
	}
}

// traceScale is the requests a minute that fill a trace's height, where
// the traces given are drawn on a page: the frame's TraceScale, where it pins
// one; else the busiest column of them all, at the width each is drawn, of
// those that show on screen, never less than leastScale.
func (f Frame) traceScale(traces []tracing) float64 {
	if f.TraceScale > 0 {
		return f.TraceScale
	}
	most := float64(leastScale)
	for _, t := range traces {
		for i, v := range t.cells {
			if t.shows(i) {
				most = max(most, v)
			}
		}
	}
	return most
}

// shows reports whether the trace's column i shows: a cell of it is on its
// canvas, within its clip.
func (t tracing) shows(i int) bool {
	for y := t.at.y; y < t.at.y+t.at.rows; y++ {
		if t.on.at(t.at.x+i, y) != nil {
			return true
		}
	}
	return false
}

// wideTraceCells are the cells a trace takes on its row from routingRows
// columns, an account's in ROUTING or a session's in SESSIONS: from
// traceFrom to traceTail short of the frame's edge.
func (f Frame) wideTraceCells() int {
	return f.Width - traceTail - traceFrom
}

// accountTraceCells are the cells an account's trace takes in ROUTING as a
// section of its own, the name column given: as wideTraceCells says, from
// routingRows columns; narrower, from the name column to narrowTail short
// of the frame's edge.
func (f Frame) accountTraceCells(names int) int {
	if f.Width >= routingRows {
		return f.wideTraceCells()
	}
	return f.Width - narrowTail - names
}

// summed are the traces' columns, as columnsOf gives them, summed column by
// column: their requests a minute together, as many columns as the widest
// has.
func summed(traces []views.Trace, columnsOf func(views.Trace) []float64) []float64 {
	var sum []float64
	for _, t := range traces {
		for i, v := range columnsOf(t) {
			if i == len(sum) {
				sum = append(sum, 0)
			}
			sum[i] += v
		}
	}
	return sum
}

// drawTrace draws the trace t at now in the bounds given, as drawCells
// draws its cells, as many as it's cells wide.
func (f Frame) drawTrace(c *canvas, t views.Trace, now time.Time, at bounds, faded bool) {
	f.drawCells(c, t.Cells(at.width, now), at, faded)
}

// drawCells draws a trace's cells, each its requests a minute, in the bounds
// given, a column a cell, the newest at the right, as many as show beside
// what a slide-over covers, as uncovered has them; faded where its account
// is out of the pool. Where c holds its traces, it's painted once every
// trace on it is drawn, at the scale they share; else it's painted now, at
// the frame's TraceScale, or leastScale where it pins none.
func (f Frame) drawCells(c *canvas, cells []float64, at bounds, faded bool) {
	shown, at := f.uncovered(cells[:min(len(cells), max(at.width, 0))], at)
	t := tracing{on: c, cells: shown, at: at, faded: faded}
	if c.traces == nil {
		f.paintTrace(t, f.traceScale(nil))
		return
	}
	*c.traces = append(*c.traces, t)
}

// uncovered are a trace's cells, a column each in the bounds given, as many
// as show beside what a slide-over covers of the frame, and the bounds they
// take: as many of its oldest dropped as it covers, so it ends as far short
// of the slide-over as it would of the edge, now's end in view.
func (f Frame) uncovered(cells []float64, at bounds) ([]float64, bounds) {
	drop := min(max(f.Covered, 0), len(cells))
	at.width -= drop
	return cells[drop:], at
}

// uncoveredEdge is the column after the last of the frame's that shows
// beside what a slide-over covers, where a trace's rate ends.
func (f Frame) uncoveredEdge() int {
	return f.Width - max(f.Covered, 0)
}

// paintTrace paints the trace t, scale requests a minute filling its rows:
// each column filled round(min(v ÷ scale, 1) × rows × 8) eighths from its
// bottom row up, its full cells as background, where the look blends, else
// █, and its top cell the eighth block of what's left, in accent.key mixed
// with the canvas so 80% of it shows; a column filled none, its bottom row ▁
// in text.faint. Faded, each keeps 35% of the colour it had, as a stale
// reading's bar does.
func (f Frame) paintTrace(t tracing, scale float64) {
	fill, lane := traceInks(t.faded)
	for i, v := range t.cells {
		f.paintColumn(t.on, t.at.x+i, t.at, traceEighths(v, scale, t.at.rows), fill, lane)
	}
}

// traceEighths are the eighths a trace's column of v requests a minute fills
// of its rows, scale requests a minute filling them all: round(min(v ÷ scale,
// 1) × rows × 8), halves to even.
func traceEighths(v, scale float64, rows int) int {
	return halfEven(min(v/scale, 1) * float64(rows*8))
}

// paintColumn paints a trace's column at x, in the rows of the bounds given,
// eighths filled from its bottom row up: its full cells as background, where
// the look blends, else █, and its top cell the eighth block of what's left,
// in fill; none filled, its bottom row ▁ in lane.
func (f Frame) paintColumn(c *canvas, x int, in bounds, eighths int, fill, lane ink) {
	floor := in.y + in.rows - 1
	if eighths <= 0 {
		c.text(x, floor, traceLane, lane)
		return
	}
	for r := 0; r < in.rows && eighths > 8*r; r++ {
		glyph, k := filledCell(f.Look, min(eighths-8*r, 8), fill)
		c.text(x, floor-r, glyph, k)
	}
}

// traceInks are the inks a trace is drawn in: its fill's, accent.key mixed
// with the canvas so 80% of it shows, and its empty lane's, text.faint; each
// faded for an account out of the pool, keeping 35% of its colour, drawn
// faint where the look can't fade it.
func traceInks(faded bool) (fill, lane ink) {
	fill, lane = ink{token: theme.AccentKey, fade: traceFade}, faintInk
	if faded {
		fill.fade, fill.dim = 1-(1-traceFade)*(1-staleFade), true
		lane.fade, lane.dim = staleFade, true
	}
	return fill, lane
}

// traceRate is the trace t's rate at now, its requests of the last minute,
// as in "15/min"; faint where its account is out of the pool.
func traceRate(t views.Trace, now time.Time, faded bool) span {
	return rateOf(t.Rate(now), faded)
}

// rateOf is a rate of n requests a minute, as in "15/min": bold in
// text.secondary, but none in text.subtle, and faint where its account is out
// of the pool.
func rateOf(n int, faded bool) span {
	text := strconv.Itoa(n) + "/min"
	switch {
	case faded:
		return span{text, faintInk}
	case n == 0:
		return span{text, dimInk}
	}
	return span{text, strongInk}
}
