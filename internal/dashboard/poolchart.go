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
// account, in the chart burn names: its name, in capitals; its room over its
// length back from now, as views.PoolRoomFrom works it out; and now.
type poolChart struct {
	name   string
	length time.Duration
	room   views.PoolRoomFrom
	now    time.Time
	burn   Burn
}

// poolChartsOf are the pool's charts at m's moment, of the windows the
// Overview shows of doc, in USAGE's order, each drawn from the frame's pool
// trails, as its Burn has it.
func (f Frame) poolChartsOf(doc status.Document, m status.Moment) []poolChart {
	var charts []poolChart
	for _, w := range f.overviewWindows(doc, m.Now) {
		length, _ := quota.Length(w.key)
		read := make(map[string][]views.WindowReading)
		for ref, r := range f.PoolTrails {
			if ref.Window == w.key {
				read[ref.Account] = r
			}
		}
		charts = append(charts, poolChart{
			name: strings.ToUpper(w.name), length: length, now: m.Now, burn: f.Burn,
			room: views.PoolRoomFrom{Moment: m, Key: w.key, Read: read},
		})
	}
	return charts
}

// draw draws the pool's chart in the bounds given, as a chart's is drawn:
// its top row; its rows, a column a cell over its length back from now,
// each its room then, a share of all its accounts', its burn-up filled to
// what's used of it, as fillColumn fills it, or its burn-down to what's left,
// as level draws it, none where its room isn't known; and its axis. A chart
// whose room is known at now alone says it has no history across its middle
// row, over what it crosses; one of a window the pool hasn't, its top row
// alone.
func (pc poolChart) draw(c *canvas, look Look, at bounds) {
	chartTop(c, at, pc.name)
	h := at.rows - 2
	room := pc.room.Room(at.width)
	if h < 1 || room.Of == 0 {
		return
	}
	floor, known := at.y+h, 0
	for col, column := range room.Columns {
		if !column.Known {
			continue
		}
		known++
		share := min(max(column.Room/float64(room.Of), 0), 1)
		if pc.burn == BurnDown {
			level(c, look, at.x+col, floor, h, share)
		} else {
			fillColumn(c, look, at.x+col, floor, h, halfEven((1-share)*float64(8*h)))
		}
	}
	pc.axis(c, at.x, floor+1, at.width)
	if known < 2 {
		across(c, at.x, at.y+1+h/2, at.width, untracedLabel)
	}
}

// axis draws the pool's chart's axis width cells from x along row y: its
// length back from now, faint, at its left, a time of day for a window of a
// day or less, else its day too; and now, in bold, ending at its right.
func (pc poolChart) axis(c *canvas, x, y, width int) {
	when := prose.TimeOfDay
	if pc.length > quota.Day {
		when = prose.Clock
	}
	after := c.text(x, y, when(pc.now, pc.now.Add(-pc.length)), faintInk)
	if from := x + width - len("now"); from > after {
		c.text(from, y, "now", labelInk)
	}
}
