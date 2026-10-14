package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/views"
)

// The day's ticks fall on hours as many apart as the first of hourSteps
// that puts them tickCells apart, at least, and its labels the first that
// puts them labelCells apart: each divides a day, so they fall on the same
// hours every day.
var hourSteps = []int{1, 2, 3, 6, 12, 24}

const (
	tickCells  = 3
	labelCells = 10
	// quarterCells is the fewest cells a day spans for the week's axis to
	// tick its quarters.
	quarterCells = 12
)

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
// their ticks along the rule under them: on each day's start, as dayStarts
// gives it, its midnight or wherever the clocks put it, a tick, and over it,
// where it fits, the day it starts, as Fri, picked out; a tick on the hours
// as far apart as tickCells asks, and on the hours as far apart as
// labelCells asks, a tick too, and over it, where it fits, the hour, as
// 16:00, muted, but where it would come within labelCells of a day's name.
// An hour the clocks never read, as they went forward over it, has neither.
func hours(c *canvas, tl views.Timeline, x, y int) {
	ticks, labels := apart(tl, tickCells), apart(tl, labelCells)
	named := dayNames(c, tl, x, y)
	from := tl.Start
	for i := 0; ; i++ {
		hour := (from.Hour() + i) % 24
		t := time.Date(from.Year(), from.Month(), from.Day(), from.Hour()+i, 0, 0, 0, from.Location())
		col := tl.Column(t)
		switch {
		case col >= tl.Columns:
			return
		case col < 0 || t.Hour() != hour || t.Minute() != 0 || t.Equal(dayfile.DayStart(t, 0)) || hour%ticks != 0 && hour%labels != 0:
			continue
		}
		c.text(x+col, y+1, "┬", borderInk)
		label := line{{prose.TimeOfDay(tl.Now, t), mutedInk}}
		crowded := slices.ContainsFunc(named, func(name int) bool { return max(col-name, name-col) < labelCells })
		if hour%labels == 0 && col+label.width() <= tl.Columns && !crowded {
			c.line(x+col, y, label)
		}
	}
}

// dayNames ticks the start of each day over the timeline from x along the
// rule on row y+1, as hours says, and names it over the tick on row y, where
// that fits, and returns the columns of the names it drew.
func dayNames(c *canvas, tl views.Timeline, x, y int) []int {
	var named []int
	for at := range dayStarts(tl.Start, 0) {
		col := tl.Column(at)
		switch {
		case col >= tl.Columns:
			return named
		case col < 0:
			continue
		}
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
// columns, its quarters, as quarters ticks them; and a tick at each
// midnight, and over it, the day's name, where it clears the one before and
// fits, as midnights has them, today's picked out.
func weekdays(c *canvas, tl views.Timeline, x, y int) {
	if quota.Day >= quarterCells*tl.Step() {
		quarters(c, tl, x, y+1)
	}
	for _, m := range midnights(tl.Start, tl.End, tl.Now, tl.Column, tl.Columns, len(prose.Weekday(time.Monday))) {
		c.text(x+m.col, y+1, "┬", borderInk)
		if !m.named {
			continue
		}
		name := line{{prose.Weekday(m.at.Weekday()), mutedInk}}
		if m.today {
			name[0].ink = strongInk
		}
		c.line(x+m.col, y, name)
	}
}

// quarters ticks the quarters of the days over the timeline from x along
// row y, faint, the first day's among them, which starts before the
// timeline, but each fourth, the midnight, which its day's own tick marks:
// time.Date puts one the clocks went forward over in the hour before.
func quarters(c *canvas, tl views.Timeline, x, y int) {
	from := tl.Start
	for i := 1; ; i++ {
		at := time.Date(from.Year(), from.Month(), from.Day(), 6*i, 0, 0, 0, from.Location())
		col := tl.Column(at)
		switch {
		case col >= tl.Columns:
			return
		case col >= 0 && i%4 != 0:
			c.text(x+col, y, "╵", faintInk)
		}
	}
}

// runwayAxis draws the axis over the lanes as laid out: nothing yet.
func (f Frame) runwayAxis(*canvas, runwayLaid) {}
