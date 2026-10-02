package dashboard

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/theme"
)

// canvas is a frame of the dashboard as a grid of cells, each a glyph in its
// ink, which the views draw into at the cells they choose, as the frames
// signed off for it were drawn, and which draws itself in a look a row at a
// time. What's drawn past its edges isn't drawn.
type canvas struct {
	width, height int
	cells         []cell
}

// cell is a glyph and the ink it's drawn in. A glyph two cells wide covers
// the cell after it too, whose glyph is "".
type cell struct {
	glyph string
	ink   ink
}

// blank is a cell with nothing drawn in it.
var blank = cell{glyph: " "}

// newCanvas is a canvas width cells wide and height tall, every cell blank.
func newCanvas(width, height int) *canvas {
	width, height = max(width, 0), max(height, 0)
	c := &canvas{width: width, height: height, cells: make([]cell, width*height)}
	for i := range c.cells {
		c.cells[i] = blank
	}
	return c
}

// at is the cell at x along row y, or nil off the canvas.
func (c *canvas) at(x, y int) *cell {
	if x < 0 || x >= c.width || y < 0 || y >= c.height {
		return nil
	}
	return &c.cells[y*c.width+x]
}

// text draws text in the ink k from x along row y, a grapheme a cell, or two
// for a wide one, and returns the column after it. A grapheme that takes no
// cell, such as a stray combining mark, isn't drawn.
func (c *canvas) text(x, y int, text string, k ink) int {
	for text != "" {
		glyph, width := ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
		text = text[len(glyph):]
		if width > 0 {
			c.put(x, y, glyph, min(width, 2), k)
			x += min(width, 2)
		}
	}
	return x
}

// put draws a glyph width cells wide at x along row y. Half a wide glyph it
// covers is left blank, as is a wide glyph the canvas's edge cuts.
func (c *canvas) put(x, y int, glyph string, width int, k ink) {
	for i := range width {
		c.clear(x+i, y)
	}
	first, last := c.at(x, y), c.at(x+width-1, y)
	if first == nil || last == nil {
		return
	}
	*first = cell{glyph: glyph, ink: k}
	if width == 2 {
		*last = cell{ink: k}
	}
}

// clear blanks the cell at x along row y, and the other half of a wide
// glyph it's part of.
func (c *canvas) clear(x, y int) {
	here := c.at(x, y)
	if here == nil {
		return
	}
	if here.glyph == "" {
		if before := c.at(x-1, y); before != nil {
			*before = blank
		}
	} else if after := c.at(x+1, y); after != nil && after.glyph == "" {
		*after = blank
	}
	*here = blank
}

// line draws the line from x along row y, and returns the column after it.
func (c *canvas) line(x, y int, l line) int {
	for _, s := range l {
		x = c.text(x, y, s.text, s.ink)
	}
	return x
}

// right draws the line along row y to end before the column end, and returns
// the column it starts at.
func (c *canvas) right(end, y int, l line) int {
	start := end - l.width()
	c.line(start, y, l)
	return start
}

// surface lays a surface under the width cells from x along row y, as a
// highlighted row has, their glyphs and colours as they were: on is the
// surface's token, faded so far into the canvas.
func (c *canvas) surface(x, y, width int, on theme.Token, fade float64) {
	for i := range width {
		if here := c.at(x+i, y); here != nil {
			here.ink.on, here.ink.onFade = on, fade
		}
	}
}

// rows draws the canvas as look draws it, a string a row. Where the look
// paints its canvas, every row is the canvas's whole width; where it
// doesn't, a row ends at its last glyph or surface.
func (c *canvas) rows(look Look) []string {
	rows := make([]string, c.height)
	for y := range rows {
		rows[y] = c.row(y, look)
	}
	return rows
}

// row draws row y as look draws it: each run of cells in one ink at once, a
// blank, of which nothing shows but its surface, running on in the ink
// before it on the same surface.
func (c *canvas) row(y int, look Look) string {
	cells := c.cells[y*c.width : (y+1)*c.width]
	end := len(cells)
	for !look.paints && end > 0 && cells[end-1].glyph == " " && cells[end-1].ink.on == 0 {
		end--
	}
	var b, run strings.Builder
	var pen ink
	for _, here := range cells[:end] {
		k := here.ink
		switch {
		case here.glyph == "":
			continue
		case here.glyph == " " && k.on == pen.on && k.onFade == pen.onFade:
			k = pen
		case here.glyph == " ":
			k = ink{on: k.on, onFade: k.onFade}
		}
		if k != pen {
			b.WriteString(look.render(run.String(), pen))
			run.Reset()
			pen = k
		}
		run.WriteString(here.glyph)
	}
	b.WriteString(look.render(run.String(), pen))
	return b.String()
}
