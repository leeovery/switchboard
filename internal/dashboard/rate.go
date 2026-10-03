package dashboard

import (
	"math"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/theme"
)

// rateSpan is what a burn rate measures use over: its bars are the use per
// 10 minutes.
const rateSpan = 10 * time.Minute

var (
	// fastInk is a burn rate's bar where the window was used faster than it
	// could be and still last to its reset.
	fastInk = ink{token: theme.AccentAttention}
	// lastingInk is the dotted line at the fastest the window could be used
	// and still last to its reset.
	lastingInk = ink{token: theme.VizPace}
)

// burnRate draws the window's burn rate width cells wide and rows tall from
// x along row y. Its use per 10 minutes is a bar in eighths of a cell, a
// column for each stretch of the window up to now, as rateAt has it, in the
// state's colour faded halfway, or in attention where it's faster than the
// window could be used and still last to its reset: without history, its
// use since it started, spread evenly, dim, marked "no history yet". A
// dotted line runs along it at that fastest rate, as lasting has it, where
// it's clear of the bars and the line for now. The bars and the line share
// a scale, whichever is the higher at its top. Held at its limit, a line
// runs along the floor from when that was reached until the limit lifts, as
// a burn-down's does. A lapsed window has nothing used, and says when it
// starts; and one whose reset isn't known has no length to draw it over.
func (f Frame) burnRate(c *canvas, p plot, x, y, width, rows int) {
	switch {
	case width < 1 || rows < 1:
		return
	case p.lapsed:
		across(c, x, y+rows/2, width, "nothing used · "+p.starts)
		return
	case !p.spanned:
		return
	}
	ahead := p.nowColumn(width)
	lasting, lasts := p.lasting()
	rates, top := p.rates(width, ahead, lasting)
	for col, r := range rates {
		switch {
		case p.heldAt(p.middleOf(col, width)):
			c.text(x+col, y+rows-1, floorLine, errorInk)
		case r.ok:
			column(c, x+col, y, rows, barEighths(r.rate, top, rows), p.barInk(r.rate, lasting, lasts))
		}
	}
	if !p.traced {
		untraced(c, x, y, width, rows, ahead)
	}
	if p.held {
		limitLine(c, p, x, y, width, rows, ahead)
	}
	nowRule(c, x, y, width, rows, ahead)
	if lasts {
		lastingLine(c, x, y, width, rows, share(lasting, top))
	}
}

// rated is a burn rate's bar: the use per 10 minutes over its stretch,
// where the readings say.
type rated struct {
	rate float64
	ok   bool
}

// rates are the bars of the columns of a chart width cells wide before its
// column ahead, as rateAt has them, and the top of their scale: the highest
// of them, or lasting, where that's higher.
func (p plot) rates(width, ahead int, lasting float64) ([]rated, float64) {
	rates := make([]rated, ahead)
	top := lasting
	for col := range rates {
		rates[col].rate, rates[col].ok = p.rateAt(col, width)
		if rates[col].ok {
			top = max(top, rates[col].rate)
		}
	}
	return rates, top
}

// rateAt is the window's use per 10 minutes over the stretch of its bar
// that column col of a chart width cells wide falls in, reporting false
// where its readings don't say: the rise its readings make across the
// stretch, from its last before the stretch starts, or its first within it,
// to its last as the stretch ends, or by now while it runs, spread over the
// whole stretch. A bar's stretch is 10 minutes, or the column's, where that
// covers more, as a week's does. Without history, it's the use since the
// window started, spread evenly over it.
func (p plot) rateAt(col, width int) (float64, bool) {
	if !p.traced {
		passed := p.now.Sub(p.start)
		if passed <= 0 {
			return 0, false
		}
		return p.window.Utilization * float64(rateSpan) / float64(passed), true
	}
	stretch := max(rateSpan, p.length/time.Duration(width))
	middle := p.at((float64(col) + 0.5) / float64(width))
	from := p.start.Add(middle.Sub(p.start) / stretch * stretch)
	to := earliest(from.Add(stretch), p.now)
	before, ok := p.usedFrom(from, to)
	after, read := p.usedAt(to)
	if !ok || !read {
		return 0, false
	}
	return max(after-before, 0) * float64(rateSpan) / float64(stretch), true
}

// usedFrom is how much of the window was used as the stretch from from to to
// started: as its last reading at or before from left it, or, where it has
// none, its first within the stretch; reporting false where it has neither.
func (p plot) usedFrom(from, to time.Time) (float64, bool) {
	if used, ok := p.usedAt(from); ok {
		return used, true
	}
	if readings := p.trail.Readings; len(readings) > 0 && !readings[0].At.After(to) {
		return readings[0].Utilization, true
	}
	return 0, false
}

// lasting is the fastest the window could be used from now, per 10 minutes,
// and still last to its reset: what's left of it short of where its account
// runs out, as its floor says, spread over the time left; reporting false
// while it's held at its limit, and once its reset has passed.
func (p plot) lasting() (float64, bool) {
	left := p.window.ResetsAt.Sub(p.now)
	if p.held || left <= 0 {
		return 0, false
	}
	return max(p.floor-p.window.Utilization, 0) * float64(rateSpan) / float64(left), true
}

// barInk is the ink of a burn rate's bar of rate, against lasting, the
// fastest that lasts, where lasts says there is one: in attention where
// it's faster, else in the state's colour faded halfway; and without
// history, dim.
func (p plot) barInk(rate, lasting float64, lasts bool) ink {
	switch {
	case !p.traced:
		return faintLevelInk
	case lasts && rate > lasting+score.Tolerance:
		return fastInk
	default:
		return ink{token: p.tone, fade: projectionFade}
	}
}

// barEighths is how many eighths of a cell a burn rate's bar of rate fills
// of a chart rows tall, on a scale whose top is top: one at the least, for
// any use at all, so none passes unseen.
func barEighths(rate, top float64, rows int) int {
	eighths := int(math.Round(share(rate, top) * float64(8*rows)))
	if rate > score.Tolerance {
		return max(eighths, 1)
	}
	return eighths
}

// share is how much of top v is, from 0 to 1: none, where top is none.
func share(v, top float64) float64 {
	if top <= 0 {
		return 0
	}
	return min(max(v/top, 0), 1)
}

// lastingLine draws a dotted line along a chart width cells wide and rows
// tall from x along row y, at the fastest the window could be used and still
// last to its reset, a share of the chart from its floor: a dot a cell, in
// each cell nothing's drawn in, so it passes behind the bars, the line for
// now and any words across the chart.
func lastingLine(c *canvas, x, y, width, rows int, at float64) {
	d := newDots(rows)
	py := d.rowOf(at)
	for col := range width {
		if here := c.at(x+col, y+py/4); here != nil && *here == blank {
			d.dot(2*col, py, lastingInk, 0)
		}
	}
	d.draw(c, x, y)
}
