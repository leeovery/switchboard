package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/views"
)

// The day's ticks fall on hours as many apart as the first of hourSteps
// that puts them tickCells apart, at least, and its labels the first that
// puts them labelCells apart: each divides a day, so they fall on the same
// hours every day. A column's length that gives the labels a step gives the
// ticks one that divides it, so every hour labelled is ticked.
var hourSteps = []int{1, 2, 3, 6, 12, 24}

const (
	tickCells  = 3
	labelCells = 10
	// quarterCells is the fewest cells a day spans for the week's axis to
	// tick its quarters.
	quarterCells = 12
)

// runwayAxis draws the axis over the lanes, as laid out, across the
// timeline's columns: on its first row, now, bold, from a cell before now's
// column, where the timeline has a now; on the next, a week's days, as
// weekdays draws them, or the hours of any other, as hours draws them; and
// under them, their ticks along a rule.
func (f Frame) runwayAxis(c *canvas, laid runwayLaid) {
	tl := f.Runway.Timeline
	if tl.Columns <= 0 {
		return
	}
	if col := tl.NowColumn(); col >= 0 && col < tl.Columns {
		c.text(laid.x+col-1, laid.top, "now", titleInk)
	}
	c.text(laid.x, laid.top+2, rule(tl.Columns), borderInk)
	if f.Runway.Window == views.RunwayWeek {
		weekdays(c, tl, laid.x, laid.top+1)
		return
	}
	hours(c, tl, laid.x, laid.top+1)
}

// apart is the first of hourSteps whose hours span cells columns of the
// timeline tl at least.
func apart(tl views.Timeline, cells int) int {
	for _, h := range hourSteps {
		if time.Duration(h)*time.Hour >= time.Duration(cells)*tl.Step() {
			return h
		}
	}
	return hourSteps[len(hourSteps)-1]
}

// hours draws the day's hours over its timeline from x along row y, and
// their ticks along the rule under them: each day's start, as dayNames draws
// it; a tick on the hours as far apart as tickCells asks, as the timeline's
// Hours has them; and over those as far apart as labelCells asks, where it
// fits, the hour, as 16:00, muted, but where it would come within labelCells
// of a day's name.
func hours(c *canvas, tl views.Timeline, x, y int) {
	labels := apart(tl, labelCells)
	named := dayNames(c, tl, x, y)
	for _, at := range tl.Hours(apart(tl, tickCells)) {
		col := tl.Column(at)
		c.text(x+col, y+1, "┬", borderInk)
		label := line{{prose.TimeOfDay(at, at), mutedInk}}
		crowded := slices.ContainsFunc(named, func(name int) bool { return max(col-name, name-col) < labelCells })
		if at.Hour()%labels == 0 && col+label.width() <= tl.Columns && !crowded {
			c.line(x+col, y, label)
		}
	}
}

// dayNames ticks the start of each day in the timeline from x along the rule
// on row y+1, as its Midnights has them, and names it over the tick on row
// y, its weekday, as Fri, picked out, where that fits, and returns the
// columns of the names it drew.
func dayNames(c *canvas, tl views.Timeline, x, y int) []int {
	var named []int
	for _, at := range tl.Midnights() {
		col := tl.Column(at)
		c.text(x+col, y+1, "┬", borderInk)
		if name := (line{{prose.Weekday(at.Weekday()), strongInk}}); col+name.width() <= tl.Columns {
			c.line(x+col, y, name)
			named = append(named, col)
		}
	}
	return named
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
