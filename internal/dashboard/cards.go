package dashboard

import (
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// padding is the blank cells either side of a card's content.
const padding = 2

// fullness is how full a card is drawn: in full, its featured window as a
// readout of big digits, a blank row between its parts; mid, as a one-line
// header; or compact, as the header, without its chart's axis.
type fullness int

const (
	fullCard fullness = iota
	midCard
	compactCard
)

// density is how much a card shows: how full it is, and how many rows its
// chart takes.
type density struct {
	fullness fullness
	chart    int
}

// densities are a card's densities, the richest first: full, with a 4-row
// chart; mid, with one of 6, 5, 4 or 3 rows; and compact, with one of 3 or 2.
var densities = []density{{fullCard, 4}, {midCard, 6}, {midCard, 5}, {midCard, 4}, {midCard, 3}, {compactCard, 3}, {compactCard, 2}}

// part is a part of a card, between its edges.
type part int

const (
	gap part = iota
	stateRow
	readoutRows
	headerRow
	chartRows
	axisRow
	barRows
)

// parts are a card's parts at the density, top to bottom.
func (d density) parts() []part {
	switch d.fullness {
	case fullCard:
		return []part{gap, stateRow, gap, readoutRows, gap, chartRows, axisRow, gap, barRows, gap}
	case midCard:
		return []part{stateRow, gap, headerRow, chartRows, axisRow, barRows}
	default:
		return []part{stateRow, headerRow, chartRows, barRows}
	}
}

// height is how many rows a part takes on a card of the density whose
// windows' bars take bars rows.
func (d density) height(p part, bars int) int {
	switch p {
	case readoutRows:
		return bigRows
	case chartRows:
		return d.chart
	case barRows:
		return bars
	default:
		return 1
	}
}

// rows is how many rows a card of the density takes, its edges included,
// whose windows' bars take bars rows: 12 more than its chart's and its bars'
// in full, 6 more mid, and 4 more compact.
func (d density) rows(bars int) int {
	rows := 2
	for _, p := range d.parts() {
		rows += d.height(p, bars)
	}
	return rows
}

// offset is how many rows under its top edge a card of the density, whose
// windows' bars take bars rows, has its part p: under its last part, for
// one it doesn't have.
func (d density) offset(p part, bars int) int {
	rows := 1
	for _, q := range d.parts() {
		if q == p {
			return rows
		}
		rows += d.height(q, bars)
	}
	return rows
}

// barsPerRow is how many of a card's bar lines sit side by side inside a
// card width cells wide: one, but on a card wide enough for more.
func barsPerRow(width int) int {
	return max(1, (width+barLinesGap)/(barLine+barLinesGap))
}

// barRowsFor is how many rows the bars of a card whose inside is width cells
// wide take, of the shown windows but the one it features.
func barRowsFor(shown, width int) int {
	perRow := barsPerRow(width)
	return (max(shown-1, 0) + perRow - 1) / perRow
}

// labelWidth is how many cells the bar lines' labels take, with the gap
// after them, as the windows shown name them: labelColumn at the least, so
// they line up as the frames have them, and more for a longer label.
func labelWidth(doc status.Document, shown []string) int {
	width := labelColumn
	for _, key := range shown {
		width = max(width, ansi.StringWidth(status.Short(labelOf(doc, key)))+1)
	}
	return width
}

// card draws an account's card, its front fc, width cells wide from x along
// row y, at the density d, its windows' bars taking bars rows, their labels
// labelled cells wide: its edges, in the border's colour, or on the account
// new sessions go to, accent.mode's, and its parts between.
func (f Frame) card(c *canvas, fc face, now time.Time, x, y, width int, d density, bars, labelled int) {
	border := borderInk
	if fc.next {
		border = bestBorderInk
	}
	topEdge(c, fc, x, y, width, border)
	row := y + 1
	for _, p := range d.parts() {
		height := d.height(p, bars)
		for i := range height {
			sides(c, x, row+i, width, border)
		}
		f.part(c, fc, now, p, x+padding+1, row, width-2*(padding+1), height, labelled)
		row += height
	}
	bottomEdge(c, fc, x, row, width, border)
}

// part draws a card's part p, width cells wide and rows tall from x along
// row y: its state, its row picked out from side to side while a look saw it
// change, fading back; its featured window's readout, header, chart or axis,
// where it has one; or its other windows' bars.
func (f Frame) part(c *canvas, fc face, now time.Time, p part, x, y, width, rows, labelled int) {
	switch {
	case p == stateRow:
		c.line(x, y, fc.stateLine().fit(width))
		if fade, ok := f.Changed[fc.account.ID]; ok {
			c.surface(x-padding, y, width+2*padding, hue{token: theme.BgAttention, fade: fade})
		}
	case p == barRows:
		f.bars(c, fc, now, x, y, width, rows, labelled)
	case !fc.hasFeatured:
	case p == readoutRows:
		f.readout(c, fc, now, x, y, x+width)
	case p == headerRow:
		f.header(c, fc, now, x, y, width, labelled)
	case p == chartRows:
		f.chart(c, fc.chart, x, y, width, rows)
	case p == axisRow:
		f.axis(c, fc.chart, x, y, width)
	}
}

// bars draws the bar lines of a card's windows but the one it features, in
// the order shown, as many to a row as fit, barLinesGap apart, rows rows of
// them, width cells wide from x along row y. A window the account hasn't
// read leaves its place blank, so the rows line up across the grid.
func (f Frame) bars(c *canvas, fc face, now time.Time, x, y, width, rows, labelled int) {
	perRow := barsPerRow(width)
	each := (width - barLinesGap*(perRow-1)) / perRow
	i := 0
	for _, key := range fc.windows {
		if fc.hasFeatured && key == fc.featured.window.Key {
			continue
		}
		row, col := i/perRow, i%perRow
		if row >= rows {
			return
		}
		if s, ok := fc.bars[key]; ok {
			f.meter(c, s, fc.account.Reserve, now, x+col*(each+barLinesGap), y+row, each, labelled)
		}
		i++
	}
}
