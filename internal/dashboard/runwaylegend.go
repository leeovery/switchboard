package dashboard

import (
	"slices"

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

// poolLimitMeans is what the legend says the pool's lane at its limit means,
// after its glyph.
const poolLimitMeans = " the pool at its limit"

// runwayLegend draws the legend, as laid out, from column 0, a line a row
// from its row down, as runwayLegendLines has them.
func (f Frame) runwayLegend(c *canvas, laid runwayLaid) {
	if !laid.fit.draws(laid.fit.legend) {
		return
	}
	for i, l := range f.runwayLegendLines() {
		c.line(0, laid.fit.legend+i, l)
	}
}

// runwayLegendRows is how many rows the legend takes, as runwayLegend draws
// it, for Runway's compaction to count.
func (f Frame) runwayLegendRows() int {
	return len(f.runwayLegendLines())
}

// runwayLegendLines are the lines of Runway's legend: the keys of what the
// lanes draw, as legendKeys has them, legendApart apart, then, legendApart
// on, what a column is, as columnSaid says it, on one line, where it fits
// across the frame; else the keys on as many lines as they take, and what a
// column is on as many after them, as wrapped wraps each. There are none
// before the watch builds the lanes.
func (f Frame) runwayLegendLines() []line {
	if f.Runway.Lanes == nil {
		return nil
	}
	keys, column := f.legendKeys(), f.columnSaid()
	apart, said := spaces(legendApart), span{columnApart, dimInk}
	if one := joined(apart, append(keys, joined(said, column...))...); one.width() <= f.Width {
		return []line{one}
	}
	return append(wrapped(apart, f.Width, keys), wrapped(said, f.Width, column)...)
}

// wrapped are the parts given on as many lines width cells wide as they
// take, apart on a line, a part never broken.
func wrapped(apart span, width int, parts []line) []line {
	var lines []line
	for _, p := range parts {
		n := len(lines)
		if n > 0 {
			if on := joined(apart, lines[n-1], p); on.width() <= width {
				lines[n-1] = on
				continue
			}
		}
		lines = append(lines, p)
	}
	return lines
}

// legendKeys are the keys of what the lanes draw, in the legend's order:
// each cell, marked as laneMark marks it, then what it means, muted, as
// cellsMean says it, and after a limit's, where the pool's lane draws one,
// its own, as poolLimitMark marks it; then, where a lane marks one, │ in
// text.subtle and the view's window's resets, as " its 5-hour resets".
func (f Frame) legendKeys() []line {
	drawn := f.Runway.Lanes.Drawn
	var keys []line
	for _, cell := range drawn.Cells {
		keys = append(keys, line{f.Look.laneMark(cell), {cellsMean[cell], mutedInk}})
	}
	if f.Runway.Lanes.Pool.Limited() {
		after := slices.IndexFunc(drawn.Cells, func(c views.Cell) bool { return c > views.CellLimited })
		if after < 0 {
			after = len(keys)
		}
		keys = slices.Insert(keys, after, line{f.Look.poolLimitMark(), {poolLimitMeans, mutedInk}})
	}
	if drawn.Resets {
		keys = append(keys, line{{resetCell, dimInk}, {" its " + runwayNames[f.Runway.Window] + " resets", mutedInk}})
	}
	return keys
}

// columnApart is what goes between the parts of what the legend says a
// column is.
const columnApart = " · "

// columnSaid are the parts of what the legend says a column is, each in
// text.subtle, columnApart between them: how long, as prose.ColumnLength
// says it, as in "a column 10 minutes"; that the past is dimmed, where the
// timeline has a now and the look can dim it; and, stepped back, that under
// each name is the day's highest use, or the week's.
func (f Frame) columnSaid() []line {
	tl := f.Runway.Timeline
	said := []line{{{"a column " + prose.ColumnLength(tl.Step()), dimInk}}}
	if !tl.Now.IsZero() && f.Look.blends() {
		said = append(said, line{{"the past dimmed", dimInk}})
	}
	if _, _, named := tl.Named(); named {
		said = append(said, line{{"under each name, the " + f.Runway.period() + "'s highest", dimInk}})
	}
	return said
}
