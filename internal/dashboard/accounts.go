package dashboard

import (
	"strconv"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

const (
	// recentLines are the most lines RECENT takes.
	recentLines = 4
	// labelGap is the blank cells after the widest of the labels under the
	// cards.
	labelGap = 3
	// recentLabel is RECENT's label, which the labels under the cards line
	// up with wherever RECENT is.
	recentLabel = "RECENT"
)

var (
	// thumbInk is the part of a scrollbar its view shows, and railInk its
	// track.
	thumbInk = ink{token: theme.TextMuted}
	railInk  = ink{token: theme.TextFaint}
)

// strip is a strip of the view under its cards: its label, how many lines
// it takes, at most until it's laid out, then as many as it shows, and what
// draws n of them from x along row y.
type strip struct {
	label string
	lines int
	draw  func(c *canvas, x, y, n int)
}

// accounts draws the Accounts view from row top, laid out as the layout
// rules have it: its accounts' cards, scrolled where they don't fit, a
// scrollbar beside them, and RECENT in their grid's empty cell, or with one
// account, COMING UP and RECENT in a column beside its card; under them,
// their strips, each after a blank row, its label at its left, but on a
// phone; then, where there are cards for it to explain, the key line, where
// it shows; and the line over the footer.
func (f Frame) accounts(c *canvas, doc status.Document, now time.Time, top int) {
	faces, shown := f.faces(doc, now)
	l := f.layOut(doc, now, len(shown), top)
	offset := min(max(f.Scroll, 0), l.content-l.view)
	area := newCanvas(f.Width, l.content)
	f.grid(area, doc, now, faces, l, labelWidth(doc, shown))
	c.paste(area, offset, top, l.view)
	key, over := f.foot(f.stripsUnder(c, l.strips, top+l.view), l.key)
	if len(faces) == 0 || over < top {
		return
	}
	if l.key {
		c.line(margin, key, f.keyLine(l.strips))
	}
	if l.scrolls() {
		above, below := l.hidden(len(faces), offset)
		says := scrolled(above, below).fit(f.edge() - margin)
		c.line((f.Width-says.width())/2, over, says)
		scrollbar(c, f.Width-1, top, l.view, l.content, offset)
		return
	}
	c.right(f.edge(), over, f.unseen(doc, now, l.key).fit(f.edge()-margin))
}

// foot is where the key line goes, and the line over the footer, under
// strips that end before row y, the key line showing as key says: full
// screen, the key line over the line over the footer, over the footer;
// printed once, without a footer, each after a blank row, the key line
// first.
func (f Frame) foot(y int, key bool) (keyRow, over int) {
	switch {
	case !f.printed():
		return f.Height - 3, f.Height - 2
	case key:
		return y + 1, y + 3
	default:
		return y + 1, y + 1
	}
}

// unseen is what the line over the footer says of what the cards don't show,
// where they fit: which windows hide from every card, and, full screen,
// where the key line doesn't show as key says, that the key's behind ?.
func (f Frame) unseen(doc status.Document, now time.Time, key bool) line {
	note := hiddenNote(doc, hiddenWindows(doc, now, f.Policy))
	if key || f.printed() {
		return note
	}
	return together([]chunk{{text: note}, {text: line{{"?", keyInk}, {" for the key", dimInk}}}}, "   ·   ")
}

// grid draws the faces' cards on c, laid out as l has them, from its top:
// their labels labelled cells wide, RECENT in the first empty cell of the
// last row, and with one account, COMING UP and RECENT in a column beside
// its card.
func (f Frame) grid(c *canvas, doc status.Document, now time.Time, faces []face, l layout, labelled int) {
	height := l.density.rows(l.bars)
	for i, fc := range faces {
		x, y := l.at(i, l.density)
		f.card(c, fc, now, x, y, l.width, l.density, l.bars, labelled)
	}
	switch {
	case l.beside:
		f.column(c, doc, now, margin+l.width+cardGutter+1, 0, height, l.density.offset(chartRows, l.bars))
	case l.cell:
		x, y := l.at(len(faces), l.density)
		f.recentCell(c, doc, now, x, y, l.width, height)
	}
}

// stripsUnder draws the strips under the cards, from row y, each after a
// blank row, as many lines of each as it shows, and as fit over the line
// over the footer, and returns the row after the last drawn. Their lines
// start in a column, after the widest of their labels, but on a phone,
// where they have none.
func (f Frame) stripsUnder(c *canvas, strips []strip, y int) int {
	x := margin
	if !f.phone() {
		x = f.labelled(strips)
	}
	for _, s := range strips {
		n := s.lines
		if !f.printed() {
			n = min(n, f.Height-2-y-1)
		}
		if n < 1 {
			break
		}
		y++
		if !f.phone() {
			c.line(margin, y, line{{s.label, labelInk}})
		}
		s.draw(c, x, y, n)
		y += n
	}
	return y
}

// labelled is the column the lines under the cards start in: labelGap cells
// after the widest of the strips' labels, and RECENT's, so they line up with
// RECENT wherever it is.
func (f Frame) labelled(strips []strip) int {
	widest := ansi.StringWidth(recentLabel)
	for _, s := range strips {
		widest = max(widest, ansi.StringWidth(s.label))
	}
	return margin + widest + labelGap
}

// strips are the strips under the cards of doc at now, in the grid g: with
// one account, but one with a column beside its card, COMING UP, where
// anything is; and RECENT, while there's something to say of what's
// happened, where it has neither that column nor an empty cell of the grid.
func (f Frame) strips(doc status.Document, now time.Time, g grid) []strip {
	if g.beside {
		return nil
	}
	var strips []strip
	if len(doc.Accounts) == 1 {
		if things := upcoming(doc, now, f.Policy); len(things) > 0 {
			draw := func(c *canvas, x, y, n int) { f.comingLines(c, things, now, x, y, n) }
			strips = append(strips, strip{label: "COMING UP", lines: min(len(things), comingThings), draw: draw})
		}
	}
	if n := f.recentCount(doc, now, recentLines); n > 0 && !g.cell {
		draw := func(c *canvas, x, y, n int) { f.recentStrip(c, doc, now, x, y, f.edge(), n) }
		strips = append(strips, strip{label: recentLabel, lines: n, draw: draw})
	}
	return strips
}

// recentCell draws RECENT in an empty cell of the grid, width cells wide and
// height tall from x along row y: its label a row down and a cell in, and
// under it as many of its lines as fit over the cell's foot, recentLines at
// most.
func (f Frame) recentCell(c *canvas, doc status.Document, now time.Time, x, y, width, height int) {
	c.line(x+1, y+1, line{{recentLabel, labelInk}})
	f.recentStrip(c, doc, now, x+1, y+2, x+width, min(recentLines, height-3))
}

// column draws COMING UP and RECENT in a column beside one account's card,
// from x along row y to the frame's edge, as tall as the card, height rows,
// whose chart starts chart rows down: COMING UP from the card's top, and
// under it, a blank row after it, RECENT, its lines level with the card's
// chart where COMING UP leaves room.
func (f Frame) column(c *canvas, doc status.Document, now time.Time, x, y, height, chart int) {
	c.line(x, y, line{{"COMING UP", labelInk}})
	things := upcoming(doc, now, f.Policy)
	n := min(len(things), comingThings, height-1)
	f.comingLines(c, things, now, x, y+1, n)
	at := max(y+n+2, y+chart-1)
	if at >= y+height {
		return
	}
	c.line(x, at, line{{recentLabel, labelInk}})
	f.recentColumn(c, doc, now, x, at+1, y+height)
}

// scrolled says what's out of view where the cards scroll: how many more
// accounts there are above, and below, and how to scroll to them, as in "▼ 2
// more accounts below · j/k or wheel to scroll".
func scrolled(above, below int) line {
	var l line
	for _, out := range []struct {
		n           int
		mark, where string
	}{{above, "▲ ", " above"}, {below, "▼ ", " below"}} {
		if out.n == 0 {
			continue
		}
		if len(l) > 0 {
			l = append(l, span{" · ", dimInk})
		}
		l = append(l, span{out.mark, keyInk}, span{moreAccounts(out.n) + out.where, mutedInk})
	}
	return append(l, span{" · j/k or wheel to scroll", dimInk})
}

// moreAccounts counts n more accounts, as in "2 more accounts".
func moreAccounts(n int) string {
	if n == 1 {
		return "1 more account"
	}
	return strconv.Itoa(n) + " more accounts"
}

// scrollbar draws a scrollbar down column x from row y, rows tall, for
// content rows that scroll, shown from the row offset: ┃ the part shown,
// as long as the share it shows, on a │ track, the whole of it.
func scrollbar(c *canvas, x, y, rows, content, offset int) {
	if rows < 1 || content <= rows {
		return
	}
	thumb := max(1, rows*rows/content)
	most := content - rows
	start := (offset*(rows-thumb) + most/2) / most
	for i := range rows {
		if i >= start && i < start+thumb {
			c.text(x, y+i, "┃", thumbInk)
		} else {
			c.text(x, y+i, "│", railInk)
		}
	}
}
