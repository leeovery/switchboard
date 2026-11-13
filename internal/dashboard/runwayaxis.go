package dashboard

import (
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/views"
)

// quarterCells is the fewest cells a day spans for the week's axis to tick
// its quarters.
const quarterCells = 12

// runwayAxis draws the axis over the lanes, as laid out, across the
// timeline's columns, as timeAxis draws it, where the fit draws it.
func (f Frame) runwayAxis(c *canvas, laid runwayLaid) {
	if laid.fit.draws(laid.fit.axis) {
		timeAxis(c, f.Runway.Timeline, laid.x, laid.fit.axis)
	}
}

// timeAxis draws Runway's axis of the week over the timeline tl, from x down
// from row top: on its first row, where the timeline has a now, now, bold,
// from a cell before now's column but never before the canvas's first, so it
// reads whole where now's column is the first at x 0; on the next, the
// week's days, as weekdays draws them; and under them, their ticks along a
// rule.
func timeAxis(c *canvas, tl views.Timeline, x, top int) {
	if tl.Columns <= 0 {
		return
	}
	if col := tl.NowColumn(); col >= 0 && col < tl.Columns {
		c.text(max(x+col-1, 0), top, "now", titleInk)
	}
	c.text(x, top+2, rule(tl.Columns), borderInk)
	weekdays(c, tl, x, top+1)
}

// weekdays draws the week's days over its timeline from x along row y, and
// their ticks along the rule under them: where a day spans quarterCells
// columns, a faint ╵ on each of its quarters, as the timeline's Hours has
// them; and a tick at each day's start, as its Midnights has them, and over
// it, where it fits and clears the name before by dayCells, the day's name,
// muted, but today's, where the timeline has a now, picked out.
func weekdays(c *canvas, tl views.Timeline, x, y int) {
	if quota.Day >= quarterCells*tl.Step() {
		for _, at := range tl.Hours(6) {
			c.text(x+tl.Column(at), y+1, "╵", faintInk)
		}
	}
	last := -dayCells
	for _, at := range tl.Midnights() {
		col := tl.Column(at)
		c.text(x+col, y+1, "┬", borderInk)
		name := line{{prose.Weekday(at.Weekday()), mutedInk}}
		if col-last < dayCells || col+name.width() > tl.Columns {
			continue
		}
		if !tl.Now.IsZero() && prose.SameDay(at, at, tl.Now) {
			name[0].ink = strongInk
		}
		c.line(x+col, y, name)
		last = col
	}
}
