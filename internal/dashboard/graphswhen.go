package dashboard

import (
	"strings"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// WHEN YOU WORK's parts, as the design lays them out.
const (
	// hoursGridX is the grid's first column, after a weekday's name and a
	// space, and its key's.
	hoursGridX = 4
	// hoursADay are the grid's columns of hours, as the page counts them.
	hoursADay = len(views.HoursDay{}.Hours)
	// hoursWeekdays are the grid's rows, a weekday each, as the page has
	// them wherever it has a grid.
	hoursWeekdays = 7
	// An hour takes from one cell, a tile alone, to hourMostCells, its
	// natural hourNaturalCells; each but the first, one at least, its tile's,
	// and its last a gap.
	hourNaturalCells = 2
	hourMostCells    = 4
	// hoursLabelEvery is how many hours apart the hours over the grid are
	// labelled, from midnight's.
	hoursLabelEvery = 3
	// hoursKeyApart is what goes between the items of the key.
	hoursKeyApart = 3
	// hoursKeyGap are the blank rows between the grid and its key.
	hoursKeyGap = 1
)

// What WHEN YOU WORK draws: an hour's tile, where the look blends its shades;
// and what its key's shades count.
const (
	hourTile     = "▆"
	hoursKeyUnit = "requests an hour"
)

// hoursGrid is WHEN YOU WORK as Graphs draws it in a look: its page's
// weekdays' hours, each shaded at its level, and the key to the shades.
type hoursGrid struct {
	page views.HoursPage
	look Look
}

// hoursPart is WHEN YOU WORK across the cells flex lays it in: a weekday's
// name and an hour a cell at its least, a tile alone; hourNaturalCells an
// hour at its natural; and growing to hourMostCells an hour at its most.
func hoursPart() flex.Part {
	return flex.Part{Least: hoursWidth(1), Natural: hoursWidth(hourNaturalCells), Most: hoursWidth(hourMostCells), Grows: true}
}

// hoursLaid is the width WHEN YOU WORK takes of room cells, as flex lays
// hoursPart in them: its least where they're fewer.
func hoursLaid(room int) int {
	spans, _ := flex.Lay(room, hoursPart())
	return spans[0].Size
}

// hoursWidth is the width of the grid hourCells cells an hour.
func hoursWidth(hourCells int) int {
	return hoursGridX + hoursADay*hourCells
}

// hourCells are the cells an hour takes of a grid width cells wide:
// ⌊(width − hoursGridX) / hoursADay⌋, from one to hourMostCells.
func hourCells(width int) int {
	return min(max((width-hoursGridX)/hoursADay, 1), hourMostCells)
}

// tileCells are the cells of an hour's tile, of its cells: all but its last,
// a gap, where it has more than one; else its one.
func tileCells(cells int) int {
	return cells - min(cells-1, 1)
}

// rows are the rows WHEN YOU WORK takes width cells wide, whatever its page
// holds, so the page holds still as the reads come: the hours' labels, a row
// a weekday, the blank row and the key, reckoned as reckonedKey reckons it.
func (g hoursGrid) rows(width int) int {
	return 1 + hoursWeekdays + hoursKeyGap + len(g.reckonedKey(width))
}

// draw draws WHEN YOU WORK from x down from row y, width cells wide, and
// nothing outside the rows it takes: where no day is known, that nothing is
// recorded, in text.subtle; else every third hour, 00 to 21, in text.faint,
// over its column; under them, a row a weekday, its name in text.subtle and
// from hoursGridX its hours, each a tile as tile draws it at its level; and
// where any hour is active, under a blank row, the key, from hoursGridX.
func (g hoursGrid) draw(c *canvas, x, y, width int) {
	in := c.within(bounds{x: x, y: y, width: width, rows: g.rows(width)})
	if len(g.page.Days) == 0 {
		in.line(x, y, line{{nothingRecorded, dimInk}}.fit(width))
		return
	}
	cells := hourCells(width)
	at := func(hour int) int { return x + hoursGridX + hour*cells }
	for hour := 0; hour < hoursADay; hour += hoursLabelEvery {
		in.text(at(hour), y, prose.HourNamed(hour), faintInk)
	}
	for i, d := range g.page.Days {
		row := y + 1 + i
		in.text(x, row, d.Name, dimInk)
		for hour, tile := range d.Hours {
			in.line(at(hour), row, line{g.tile(tile.Level, tileCells(cells))})
		}
	}
	for i, l := range g.key(width) {
		in.line(x+hoursGridX, y+1+len(g.page.Days)+hoursKeyGap+i, l)
	}
}

// tile is an hour's tile at its level, cells wide, in its shade's ink: ▆;
// or, in a look that draws no shades of its own, its level's glyph.
func (g hoursGrid) tile(level, cells int) span {
	glyph := hourTile
	if !g.look.blends() {
		glyph = shadeGlyph(level)
	}
	return span{strings.Repeat(glyph, cells), dayInk(level)}
}

// key is the key to the grid's shades, of a grid width cells wide, from
// hoursGridX, of the page's cuts, flowed as keyFlowed flows it onto the rows
// reckonedKey reserves for it: where its cuts are wider than those reckoned
// and it takes more, whole items give way from its end, what they count
// first, so no bound is cut, nor lost to the section's foot. None where no
// hour is active, as the page has no cuts.
func (g hoursGrid) key(width int) []line {
	if len(g.page.KeyCuts) == 0 {
		return nil
	}
	items, reserved := g.keyItems(g.page.KeyCuts), len(g.reckonedKey(width))
	key := keyFlowed(items, width)
	for len(key) > reserved {
		items = items[:len(items)-1]
		key = keyFlowed(items, width)
	}
	return key
}

// reckonedKey is the key, of a grid width cells wide, as keyFlowed flows it,
// its every cut at reckonedCut, the widest it's reckoned at, as the Year's
// is.
func (g hoursGrid) reckonedKey(width int) []line {
	reckoned := keyWide.number(reckonedCut)
	return keyFlowed(g.keyItems([]string{reckoned, reckoned, reckoned}), width)
}

// keyItems are the key's items, of the cuts given, as the page words them:
// each shade's tile and its bounds, as the Year's key words them, in
// text.subtle; then what they count, in text.faint.
func (g hoursGrid) keyItems(cuts []string) []line {
	var items []line
	for level, bound := range keyWide.boundsOf(cuts) {
		items = append(items, line{g.tile(level, 1), {" " + bound, dimInk}})
	}
	return append(items, line{{hoursKeyUnit, faintInk}})
}

// keyFlowed are the key's items flowed onto as many rows as they take of a
// grid width cells wide, from hoursGridX, hoursKeyApart apart, an item never
// broken.
func keyFlowed(items []line, width int) []line {
	return wrapped(spaces(hoursKeyApart), width-hoursGridX, items)
}

// whenDrawn draws WHEN YOU WORK in the section placed, from its left, at the
// width flex lays it at in its columns, as hoursLaid has it, of the page the
// watch built;
// where it built none, as before the ledger's days are read, or with no
// ledger, as a page with no day known, that nothing is recorded yet.
func (f Frame) whenDrawn(c *canvas, placed graphsPlaced) {
	g := hoursGrid{look: f.Look}
	if f.Graphs.Hours != nil {
		g.page = *f.Graphs.Hours
	}
	body := placed.body(hoursLaid(placed.across.Size))
	g.draw(c.within(body), body.x, body.y, body.width)
}
