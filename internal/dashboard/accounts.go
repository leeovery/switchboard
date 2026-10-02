package dashboard

import (
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/status"
)

const (
	// recentLines are the most lines RECENT's strip takes.
	recentLines = 4
	// labelGap is the blank cells after the widest of the strips' labels.
	labelGap = 3
)

// strip is a strip of the view under its cards: its label, how many lines
// it takes at most, and what draws n of them from x along row y.
type strip struct {
	label string
	lines int
	draw  func(c *canvas, x, y, n int)
}

// accounts draws the Accounts view from row top until row bottom: the
// accounts' cards, as the dashboard drew them before milestone 5, and under
// them, strips: COMING UP, with one account, whose heading has no room for
// it, and RECENT, each after a blank row, its label at its left, but on a
// phone. The cards take the rows the strips' first lines leave, and the
// strips what the cards leave.
func (f Frame) accounts(c *canvas, doc status.Document, now time.Time, top, bottom int) {
	strips := f.strips(doc, now)
	y := top
	for _, l := range f.cards(doc, now, bottom-top-2*len(strips)) {
		c.line(margin, y, l)
		y++
	}
	x := margin
	for _, s := range strips {
		if !f.phone() {
			x = max(x, margin+ansi.StringWidth(s.label)+labelGap)
		}
	}
	for _, s := range strips {
		n := min(s.lines, bottom-y-1)
		if n < 1 {
			return
		}
		y++
		if !f.phone() {
			c.line(margin, y, line{{s.label, labelInk}})
		}
		s.draw(c, x, y, n)
		y += n
	}
}

// strips are the strips under the cards of doc at now: COMING UP, with one
// account, where anything is; and RECENT, while there's something to say of
// what's happened.
func (f Frame) strips(doc status.Document, now time.Time) []strip {
	var strips []strip
	if len(doc.Accounts) == 1 {
		if things := upcoming(doc, now, f.Policy); len(things) > 0 {
			draw := func(c *canvas, x, y, n int) { f.comingLines(c, things, now, x, y, n) }
			strips = append(strips, strip{label: "COMING UP", lines: min(len(things), comingThings), draw: draw})
		}
	}
	if n := f.recentCount(doc, now, recentLines); n > 0 {
		draw := func(c *canvas, x, y, n int) { f.recentStrip(c, doc, now, x, y, f.edge(), n) }
		strips = append(strips, strip{label: "RECENT", lines: n, draw: draw})
	}
	return strips
}

// cards are the accounts' cards, as the dashboard drew them before milestone
// 5, rows lines at most: as many to a row as fit the width, or where they
// don't fit the rows, a line each.
func (f Frame) cards(doc status.Document, now time.Time, rows int) []line {
	room := f.Width - margin
	if body, _, ok := grid(doc, now, room); ok && len(body) <= rows {
		return body
	}
	body, _ := compact(doc, now, room)
	return body[:min(len(body), max(rows, 0))]
}
