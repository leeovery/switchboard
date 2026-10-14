package dashboard

import (
	"strings"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// legendApart is the blank cells between the legend's parts.
const legendApart = 4

// cellsMean are what the legend says each cell a lane draws means, after its
// glyph.
var cellsMean = map[views.Cell]string{
	views.CellRoom:      " room",
	views.CellRunsOut:   " room, but it runs out at its pace",
	views.CellCapped:    " at its cap",
	views.CellLimited:   " at its limit",
	views.CellRefused:   " token refused",
	views.CellTokenless: " no token",
}

// runwayLegend draws the legend, as laid out, from column 0, a line a row
// from its row down, as runwayLegendLines has them.
func (f Frame) runwayLegend(c *canvas, laid runwayLaid) {
	for i, l := range f.runwayLegendLines() {
		c.line(0, laid.legend+i, l)
	}
}

// runwayLegendLines are the lines of Runway's legend: the keys of what the
// lanes draw, as legendKeys has them, legendApart apart, then, legendApart
// on, what a column is, as columnSaid says it, on one line, where it fits
// across the frame; else the keys on as many lines as they take, wrapping
// whole keys, and what a column is on a line of its own. There are none
// before the watch builds the lanes.
func (f Frame) runwayLegendLines() []line {
	if f.Runway.Lanes == nil {
		return nil
	}
	keys, column := f.legendKeys(), f.columnSaid()
	apart := spaces(legendApart)
	if one := joined(apart, append(keys, column)...); one.width() <= f.Width {
		return []line{one}
	}
	var lines []line
	for _, key := range keys {
		n := len(lines)
		if n == 0 || lines[n-1].width()+legendApart+key.width() > f.Width {
			lines = append(lines, key)
			continue
		}
		lines[n-1] = joined(apart, lines[n-1], key)
	}
	return append(lines, column)
}

// legendKeys are the keys of what the lanes draw, in the legend's order:
// each cell, marked as laneMark marks it, then what it means, muted, as
// cellsMean says it; then, where a lane marks one, │ in text.subtle and the
// view's window's resets, as " its 5-hour resets".
func (f Frame) legendKeys() []line {
	drawn := f.Runway.Lanes.Drawn
	var keys []line
	for _, cell := range drawn.Cells {
		keys = append(keys, line{f.Look.laneMark(cell), {cellsMean[cell], mutedInk}})
	}
	if drawn.Resets {
		keys = append(keys, line{{resetCell, dimInk}, {" its " + runwayNames[f.Runway.Window] + " resets", mutedInk}})
	}
	return keys
}

// columnSaid is what the legend says a column is, in text.subtle: how long,
// as prose.ColumnLength says it, as in "a column 10 minutes"; that the past
// is dimmed, where the timeline has a now and the look can dim it; and,
// stepped back, that under each name is the day's highest use, or the
// week's.
func (f Frame) columnSaid() line {
	tl := f.Runway.Timeline
	said := []string{"a column " + prose.ColumnLength(tl.Step())}
	if !tl.Now.IsZero() && f.Look.blends() {
		said = append(said, "the past dimmed")
	}
	if _, _, named := tl.Named(); named {
		said = append(said, "under each name, the "+f.Runway.period()+"'s highest")
	}
	return line{{strings.Join(said, " · "), dimInk}}
}
