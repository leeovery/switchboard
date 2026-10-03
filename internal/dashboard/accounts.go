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
// accounts' cards, and under them, strips: COMING UP, with one account,
// whose heading has no room for it, and RECENT, each after a blank row, its
// label at its left, but on a phone. The cards take the rows the strips'
// first lines leave, and the strips what the cards leave. On row bottom, at
// the right, it says which windows hide from every card.
func (f Frame) accounts(c *canvas, doc status.Document, now time.Time, top, bottom int) {
	c.right(f.edge(), bottom, hiddenNote(doc, hiddenWindows(doc, now, f.Policy)).fit(f.edge()-margin))
	strips := f.strips(doc, now)
	y := f.cards(c, doc, now, top, bottom-top-2*len(strips))
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
