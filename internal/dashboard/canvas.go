package dashboard

import (
	"slices"
	"strings"
	"unique"

	"github.com/charmbracelet/x/ansi"
)

// canvas is a frame of the dashboard as a grid of cells, each a glyph in its
// ink, which the views draw into at the cells they choose, as the frames
// signed off for it were drawn, and which draws itself in a look a row at a
// time. What's drawn past its edges isn't drawn, but for one that grows,
// which takes in the rows drawn past its foot, as usage's printout has no
// height to fit; nor what's drawn outside its clip, where it has one.
type canvas struct {
	width int
	cells [][]cell
	grows bool
	clip  *bounds
	// traces are the traces drawn on the canvas, and on those within it,
	// where it holds them to be painted at the scale they share once every
	// one is drawn, as holdingTraces has it: nil where each is painted as
	// it's drawn.
	traces *[]tracing
	// heads are the headings drawn on the canvas, and on those within it,
	// marked as heading has them.
	heads *[]headed
}

// cell is a glyph, the ink it's drawn in, and what of the page it draws
// that moves as it changes, where it draws any. A glyph two cells wide
// covers the cell after it too, whose glyph is "".
type cell struct {
	glyph string
	ink   ink
	mark  unique.Handle[mark]
}

// mark is what of the page a cell draws that moves as it changes: a number,
// which brightens, or a window's bar, which eases to a new reading; whether
// it's cued, picked out by a cue fading back, as it's drawn only while it
// fades; and a cord, plugged in at cord, what travels it moving along it, or
// where loose, a cord a move let go from the jack of cord's account, fading.
type mark struct {
	number Number
	bar    Ref
	cued   bool
	cord   Plug
	loose  bool
}

// blank is a cell with nothing drawn in it.
var blank = cell{glyph: " "}

// newCanvas is a canvas width cells wide and height tall, every cell blank.
func newCanvas(width, height int) *canvas {
	c := &canvas{width: max(width, 0), heads: new([]headed)}
	c.grow(height)
	return c
}

// growing is a canvas width cells wide that grows down as it's drawn on.
func growing(width int) *canvas {
	c := newCanvas(width, 0)
	c.grows = true
	return c
}

// heading marks the row at as a heading of the rows from first under it,
// which a view drawn whole on the canvas and shown a row at a time shows
// together, as rowsPaged has them.
func (c *canvas) heading(at, first int) {
	*c.heads = append(*c.heads, headed{at: at, first: first})
}

// headings are the headings marked on the canvas, in the order of their
// rows.
func (c *canvas) headings() []headed {
	return slices.SortedFunc(slices.Values(*c.heads), func(a, b headed) int { return a.at - b.at })
}

// height is how many rows the canvas has.
func (c *canvas) height() int {
	return len(c.cells)
}

// grow adds blank rows to the canvas until it's height rows tall.
func (c *canvas) grow(height int) {
	for len(c.cells) < height {
		row := make([]cell, c.width)
		for i := range row {
			row[i] = blank
		}
		c.cells = append(c.cells, row)
	}
}

// within is the canvas, one that doesn't grow, drawn on only in the bounds
// given, as a section's body is in those the planner gives it, and in its
// own clip, where it has one: what's drawn outside either isn't drawn.
func (c *canvas) within(b bounds) *canvas {
	in := *c
	if c.clip != nil {
		b = b.and(*c.clip)
	}
	in.clip = &b
	return &in
}

// at is the cell at x along row y, or nil off the canvas, or outside its
// clip. A canvas that grows takes in the row first, where it's past its foot.
func (c *canvas) at(x, y int) *cell {
	if c.grows && x >= 0 && x < c.width {
		c.grow(y + 1)
	}
	if x < 0 || x >= c.width || y < 0 || y >= len(c.cells) || c.clip != nil && !c.clip.holds(x, y) {
		return nil
	}
	return &c.cells[y][x]
}

// emboldened makes width cells from x along row y bold, as they're drawn,
// none off the canvas.
func (c *canvas) emboldened(x, y, width int) {
	for i := range width {
		if here := c.at(x+i, y); here != nil {
			here.ink.bold = true
		}
	}
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

// put draws a glyph width cells wide at x along row y, the mark its ink
// carries noted on its cells rather than in their ink, as it changes nothing
// of how they're drawn. Half a wide glyph it covers is left blank, as is a
// wide glyph the canvas's edge cuts.
func (c *canvas) put(x, y int, glyph string, width int, k ink) {
	for i := range width {
		c.clear(x+i, y)
	}
	first, last := c.at(x, y), c.at(x+width-1, y)
	if first == nil || last == nil {
		return
	}
	marked := k.mark
	k.mark = unique.Handle[mark]{}
	*first = cell{glyph: glyph, ink: k, mark: marked}
	if width == 2 {
		*last = cell{ink: k, mark: marked}
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

// rightFit draws the parts along row y to end before the column end, as
// right does, but where they'd run past the column edge, cut as fitParts
// cuts them in the cells from where they'd start, and ending at the edge;
// and returns the column they start at.
func (c *canvas) rightFit(end, edge, y int, parts ...part) int {
	fitted := fitParts(edge-(end-partsWidth(parts)), parts...)
	return c.right(min(end, edge), y, fitted)
}

// surface lays a surface under the width cells from x along row y, as a
// highlighted row has, their glyphs and colours as they were.
func (c *canvas) surface(x, y, width int, on hue) {
	for i := range width {
		if here := c.at(x+i, y); here != nil {
			here.ink.on = on
		}
	}
}

// barred marks the cells n long from x along row y, as far as they're drawn,
// as the bar of the window ref names, which eases to a new reading.
func (c *canvas) barred(x, y, n int, ref Ref) {
	bar := unique.Make(mark{bar: ref})
	for i := range n {
		if here := c.at(x+i, y); here != nil {
			here.mark = bar
		}
	}
}

// marked marks the cells given, as far as they're drawn, as m says.
func (c *canvas) marked(cells []point, m mark) {
	handle := unique.Make(m)
	for _, p := range cells {
		if here := c.at(p.x, p.y); here != nil {
			here.mark = handle
		}
	}
}

// cued marks the cells n long from x along row y, as far as they're drawn,
// as cued, keeping the number or bar they're marked as.
func (c *canvas) cued(x, y, n int) {
	plain := unique.Make(mark{cued: true})
	for i := range n {
		switch here := c.at(x+i, y); {
		case here == nil:
		case here.mark == (unique.Handle[mark]{}):
			here.mark = plain
		default:
			m := here.mark.Value()
			m.cued = true
			here.mark = unique.Make(m)
		}
	}
}

// paste draws rows rows of from, from its row first, onto the canvas from row
// y, as they are: a scrolled part of a view, cut to the rows it shows.
func (c *canvas) paste(from *canvas, first, y, rows int) {
	for i := range rows {
		if first+i < 0 || first+i >= from.height() {
			continue
		}
		for x, here := range from.cells[first+i] {
			if there := c.at(x, y+i); there != nil {
				*there = here
			}
		}
	}
}

// fade fades everything drawn on the canvas but the rows kept, as the page
// behind a card is: each cell's colours, as finally drawn, its own fade and
// glow included, mixed behindFade of the way to the canvas, and nothing bold.
// A row kept, as the line the Log's details are of, stays as it's drawn,
// its colours, its surface and its bold. What's drawn after it, as the card,
// isn't faded.
func (c *canvas) fade(kept ...int) {
	for y, row := range c.cells {
		if slices.Contains(kept, y) {
			continue
		}
		for x := range row {
			row[x].ink.behind, row[x].ink.bold = true, false
		}
	}
}

// rows draws the canvas as look draws it, a string a row. Where the look
// paints its canvas, every row is the canvas's whole width; where it
// doesn't, a row ends at its last glyph or surface.
func (c *canvas) rows(look Look) []string {
	rows := make([]string, len(c.cells))
	for y := range rows {
		rows[y] = c.row(y, look)
	}
	return rows
}

// row draws row y as look draws it: each run of cells in one ink at once, a
// blank, of which nothing shows but its surface, running on in the ink
// before it on the same surface, as faded behind a card.
func (c *canvas) row(y int, look Look) string {
	cells := c.cells[y]
	end := len(cells)
	for !look.paints && end > 0 && cells[end-1].glyph == " " && cells[end-1].ink.on == (hue{}) {
		end--
	}
	var b, run strings.Builder
	var pen ink
	for _, here := range cells[:end] {
		k := here.ink
		switch {
		case here.glyph == "":
			continue
		case here.glyph == " " && k.on == pen.on:
			k = pen
		case here.glyph == " ":
			k = ink{on: k.on, behind: k.behind}
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
