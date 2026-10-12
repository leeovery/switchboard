package dashboard

import (
	"slices"
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/status"
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

// scaled is the frame with the scale of every trace its page draws of doc at
// now worked out, once a frame, as its TraceScale, where it pins none, so
// each trace scales alike: the busiest column of them all, as busiestColumn
// says.
func (f Frame) scaled(doc status.Document, now time.Time) Frame {
	if f.TraceScale <= 0 {
		f.TraceScale = f.busiestColumn(doc, now)
	}
	return f
}

// traceScale is the requests a minute that fill a trace's height: the
// frame's TraceScale, as it pins it, or scaled works it out; leastScale
// before either.
func (f Frame) traceScale() float64 {
	if f.TraceScale > 0 {
		return f.TraceScale
	}
	return leastScale
}

// busiestColumn is the most requests a minute in any column of every trace
// the page draws of doc at now, each as many cells wide as an account's trace
// is, as traceCells says, as busiestOf says.
func (f Frame) busiestColumn(doc status.Document, now time.Time) float64 {
	cells := f.traceCells(doc)
	return f.busiestOf(doc, func(t views.Trace) []float64 { return t.Cells(cells, now) })
}

// busiestOf is the most requests a minute in any column of every trace the
// page draws of doc, each's columns as columnsOf gives them, never less than
// leastScale: ROUTING's accounts', and the pool's, theirs summed, beside
// USAGE's blocks by account, while ROUTING shows; and the sessions', while
// SESSIONS shows.
func (f Frame) busiestOf(doc status.Document, columnsOf func(views.Trace) []float64) float64 {
	var columns [][]float64
	if f.Sections.Has(SectionRouting) {
		for _, t := range f.accountTraces(doc.Accounts) {
			columns = append(columns, columnsOf(t))
		}
		if f.routingBesideUsage() {
			columns = append(columns, summed(f.accountTraces(poolAccounts(doc)), columnsOf))
		}
	}
	if f.Sections.Has(SectionSessions) {
		for _, t := range f.Traces.Sessions {
			columns = append(columns, columnsOf(t))
		}
	}
	most := float64(leastScale)
	for _, c := range columns {
		if len(c) > 0 {
			most = max(most, slices.Max(c))
		}
	}
	return most
}

// traceCells are the cells an account's trace takes in ROUTING by window, as
// wide as the frame is: from traceFrom to traceTail short of its edge, from
// routingRows columns; narrower, from the name column of doc's accounts to
// narrowTail short of it. Never fewer than one.
func (f Frame) traceCells(doc status.Document) int {
	if f.Width >= routingRows {
		return max(f.Width-traceTail-traceFrom, 1)
	}
	return max(f.Width-narrowTail-nameColumn(namesOf(doc), f.Width), 1)
}

// accountTraces are the traces of the accounts given, in their order.
func (f Frame) accountTraces(accounts []status.Account) []views.Trace {
	traces := make([]views.Trace, len(accounts))
	for i, a := range accounts {
		traces[i] = f.Traces.Accounts[a.ID]
	}
	return traces
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

// drawTrace draws the trace t at now in the bounds given, a column a cell,
// the newest at the right, scale requests a minute filling its rows; faded
// where its account is out of the pool.
func (f Frame) drawTrace(c *canvas, t views.Trace, now time.Time, at bounds, scale float64, faded bool) {
	f.drawCells(c, t.Cells(at.width, now), at, scale, faded)
}

// drawCells draws a trace's cells, each its requests a minute, in the bounds
// given, a column a cell: each filled round(min(v ÷ scale, 1) × rows × 8)
// eighths from its bottom row up, its full cells as background, where the
// look blends, else █, and its top cell the eighth block of what's left, in
// accent.key mixed with the canvas so 80% of it shows; a column filled none,
// its bottom row ▁ in text.faint. Faded, each keeps 35% of the colour it
// had, as a stale reading's bar does.
func (f Frame) drawCells(c *canvas, cells []float64, at bounds, scale float64, faded bool) {
	fill, lane := traceInks(faded)
	floor := at.y + at.rows - 1
	for i, v := range cells[:min(len(cells), max(at.width, 0))] {
		eighths := halfEven(min(v/scale, 1) * float64(at.rows*8))
		if eighths <= 0 {
			c.text(at.x+i, floor, traceLane, lane)
			continue
		}
		for r := 0; r < at.rows && eighths > 8*r; r++ {
			glyph, k := f.traceCell(min(eighths-8*r, 8), fill)
			c.text(at.x+i, floor-r, glyph, k)
		}
	}
}

// traceCell is a cell of a trace filled eighths of it, from 1 to 8, in the
// fill's ink: full, a blank on the fill's colour where the look blends, else
// █; else its eighth block.
func (f Frame) traceCell(eighths int, fill ink) (string, ink) {
	if eighths == 8 && f.Look.blends() {
		return " ", ink{on: fill.hue()}
	}
	return levels[eighths], fill
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
