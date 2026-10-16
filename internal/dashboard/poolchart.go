package dashboard

import (
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// poolChart is a window of the pool as GRAPHS charts it beside Σ pool by
// account, in the chart burn names: its key, and its name, in capitals; its
// length; its room over that length back from now, as views.PoolRoom has
// it; and now.
type poolChart struct {
	key, name string
	length    time.Duration
	room      views.PoolRoom
	now       time.Time
	burn      Burn
}

// poolChartsOf are the pool's charts at m's moment, of the windows the
// Overview shows of doc, in USAGE's order, each of its room in the window as
// the frame's PoolRooms have it, as its Burn has it.
func (f Frame) poolChartsOf(doc status.Document, m status.Moment) []poolChart {
	var charts []poolChart
	for _, w := range f.overviewWindows(doc, m.Now) {
		length, _ := quota.Length(w.key)
		charts = append(charts, poolChart{key: w.key, name: strings.ToUpper(w.name), length: length, room: f.PoolRooms[w.key], now: m.Now, burn: f.Burn})
	}
	return charts
}

// draw draws the pool's chart in the bounds given, as a chart's is drawn:
// its top row; its rows, a column a cell, its room's columns ending at the
// last, each the share of all its accounts' room used then, as the column
// gives it, its burn-up filled to that, as fillColumn fills it, or its
// burn-down to what's left, as level draws it, none where its room isn't
// known; and its axis. A chart whose room is known at now alone says it has
// no history across its middle row, over what it crosses; one of a window
// the pool hasn't, its top row alone.
func (pc poolChart) draw(c *canvas, look Look, at bounds) {
	chartTop(c, at, pc.name)
	h, room := at.rows-2, pc.room
	if h < 1 || room.Of == 0 {
		return
	}
	floor, from, known := at.y+h, at.width-len(room.Columns), 0
	for i, column := range room.Columns {
		if !column.Known || from+i < 0 {
			continue
		}
		known++
		x, used := at.x+from+i, min(max(column.Used, 0), 1)
		if pc.burn == BurnDown {
			level(c, look, x, floor, h, 1-used)
		} else {
			fillColumn(c, look, x, floor, h, halfEven(used*float64(8*h)))
		}
	}
	pc.axis(c, at.x, floor+1, at.width)
	if known < 2 {
		across(c, at.x, at.y+1+h/2, at.width, untracedLabel)
	}
}

// axis draws the pool's chart's axis width cells from x along row y: its
// length back from now, faint, at its left, a time of day for a window of a
// day or less, else with its date, as a week back falls on now's weekday;
// and now, in bold, ending at its right.
func (pc poolChart) axis(c *canvas, x, y, width int) {
	when := prose.TimeOfDay
	if pc.length > quota.Day {
		when = prose.Past
	}
	after := c.text(x, y, when(pc.now, pc.now.Add(-pc.length)), faintInk)
	if from := x + width - len("now"); from > after {
		c.text(from, y, "now", labelInk)
	}
}
