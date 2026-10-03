package dashboard

import (
	"cmp"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// The Accounts view's grid: cards at least leastCard cells wide, cardGutter
// cells apart, or tightGutter where that fits another across; and one
// account's card, wideCard cells wide where a column fits beside it, its
// chart wideChart rows tall.
const (
	leastCard   = 50
	cardGutter  = 4
	tightGutter = 2
	wideCard    = 104
	wideChart   = 6
)

// grid is how the Accounts view lays its accounts' cards out across the
// frame: as many across as fit, gutter cells apart, each width cells wide,
// in down rows, gap blank rows apart, their windows' bars taking bars rows.
// With one account, beside is set where COMING UP and RECENT take a column
// beside its card; with more, cell is set where the last row of cards has an
// empty cell, which RECENT takes.
type grid struct {
	across, down, width, gutter, gap, bars int
	beside, cell                           bool
}

// gridOf is the grid of n accounts' cards, whose windows shown of them the
// cards show: as many across as fit at least leastCard cells wide, never
// more than there are, sharing the spare width, a card taking the whole
// width where not even one fits; one account's card wideCard cells wide
// with a column beside it, from slotsInRowFrom columns, else the whole
// width; and on a phone, the cards stacked without a blank between.
func (f Frame) gridOf(n, shown int) grid {
	room := f.Width - 2*margin
	g := grid{across: 1, width: room, gap: 1}
	switch {
	case n == 1 && f.Width >= slotsInRowFrom:
		g.width, g.beside = min(wideCard, room), true
	case n > 1:
		g.across, g.gutter = columns(room, n)
		g.width = (room - g.gutter*(g.across-1)) / g.across
	}
	if f.phone() {
		g.gap = 0
	}
	g.down = (n + g.across - 1) / g.across
	g.cell = n%g.across != 0
	g.bars = barRowsFor(shown, g.width-2*(padding+1))
	return g
}

// columns is how many of n cards fit across room cells, at least leastCard
// cells wide, never more than n and never fewer than one, and the gutter
// between them: cardGutter cells, or tightGutter where that fits another.
func columns(room, n int) (across, gutter int) {
	fit := func(gutter int) int {
		return min(n, max(1, (room+gutter)/(leastCard+gutter)))
	}
	if fit(tightGutter) > fit(cardGutter) {
		return fit(tightGutter), tightGutter
	}
	return fit(cardGutter), cardGutter
}

// rows is how many rows the grid's rows of cards take at the density d.
func (g grid) rows(d density) int {
	return g.down*d.rows(g.bars) + max(g.down-1, 0)*g.gap
}

// at is where the grid's cell i is, its cards at the density d: the column
// and the row of its top left corner.
func (g grid) at(i int, d density) (x, y int) {
	return margin + i%g.across*(g.width+g.gutter), i / g.across * (d.rows(g.bars) + g.gap)
}

// layout is the Accounts view laid out in the rows it has: its grid; the
// density its cards are drawn at; the rows they take, content, and the rows
// of them shown at once, view, fewer where they scroll; the strips under
// them, each with the lines it shows; and whether the key line shows.
type layout struct {
	grid
	density       density
	content, view int
	strips        []strip
	key           bool
}

// scrolls reports whether the cards scroll, more of them than the view
// shows.
func (l layout) scrolls() bool {
	return l.content > l.view
}

// layOut lays the Accounts view of doc at now out from row top, its cards
// showing shown windows each, as the layout rules have it. Its cards take
// the space first: the richest density whose rows of cards fit, with a blank
// and a line of each strip under them, wins, one account's card trying a
// full one with a wideChart-row chart first. Then the extras: the rows left
// over grow the strips, in turn, to their every line, then show the key
// line, with the blank over it, where its line fits the width. Where not even
// the sparest cards fit, they scroll between the heading and the strips, a
// line each. Printed, with no height to fit, the cards are at their richest,
// the strips whole, and the key shows where its line fits.
func (f Frame) layOut(doc status.Document, now time.Time, shown, top int) layout {
	l := layout{grid: f.gridOf(len(doc.Accounts), shown)}
	l.strips = f.strips(doc, now, l.grid)
	tries := densities
	if len(doc.Accounts) == 1 {
		tries = slices.Concat([]density{{fullCard, wideChart}}, densities)
	}
	if f.printed() {
		l.density, l.key = tries[0], f.keyFits(l.strips)
		l.content = l.rows(l.density)
		l.view = l.content
		return l
	}
	room := f.Height - 2 - top
	under := 2 * len(l.strips)
	l.density = tries[len(tries)-1]
	for _, d := range tries {
		if l.rows(d)+under <= room {
			l.density = d
			break
		}
	}
	l.content = l.rows(l.density)
	l.view = min(l.content, max(room-under, 0))
	left := room - under - l.content
	for i, s := range l.strips {
		grown := min(max(left, 0), s.lines-1)
		l.strips[i].lines, left = 1+grown, left-grown
	}
	l.key = left >= 2 && f.keyFits(l.strips)
	return l
}

// hidden counts the accounts of a layout that scrolls whose cards are out of
// view, wholly or partly, as the view shows its content from the row
// offset: those above, and those below.
func (l layout) hidden(n, offset int) (above, below int) {
	for i := range n {
		_, y := l.at(i, l.density)
		if y < offset {
			above++
		}
		if y+l.density.rows(l.bars) > offset+l.view {
			below++
		}
	}
	return above, below
}

// Scrolling is how far a frame's view scrolls: Most, the furthest, in rows,
// zero where it fits; and Page, how many rows of it show at once.
type Scrolling struct {
	Most, Page int
}

// Scrolling is how far the frame's view of doc scrolls at now: the Accounts
// view, where not even its sparest cards fit, and Runway, where its lanes
// don't. A frame printed once never scrolls, and nor does a view not built
// yet.
func (f Frame) Scrolling(doc status.Document, now time.Time) Scrolling {
	if l, ok := f.cardsOf(doc, now); ok {
		return Scrolling{Most: l.content - l.view, Page: l.view}
	}
	if f.View != Runway {
		return Scrolling{}
	}
	top := f.above(newCanvas(f.Width, f.Height), doc, now)
	l := f.layOutRunway(doc, now, top)
	return Scrolling{Most: l.content - l.view, Page: l.view}
}

// cardsOf is the Accounts view of doc at now as the frame lays it out full
// screen, under its title and heading, reporting false for a frame that
// shows no cards that move, scroll or take the focus: one printed once, or
// of another view.
func (f Frame) cardsOf(doc status.Document, now time.Time) (layout, bool) {
	if f.View != Accounts || f.printed() {
		return layout{}, false
	}
	top := f.above(newCanvas(f.Width, f.Height), doc, now)
	return f.layOut(doc, now, len(shownWindows(doc, now, f.Policy)), top), true
}

// scrolled is how far the layout's cards are scrolled as the frame has them:
// no further than they go either way.
func (f Frame) scrolled(l layout) int {
	return min(max(f.Scroll, 0), l.content-l.view)
}

// Neighbour is the account whose card is beside, above or below the card of
// doc's account with the given id, as the Accounts view lays them out at
// now: across cards along its row, a card to the left where less than zero,
// or down rows of cards, a row up where less than zero, in its column, or
// the last of a row too short to reach it. It reports false where there's
// no card there.
func (f Frame) Neighbour(doc status.Document, now time.Time, id string, across, down int) (string, bool) {
	l, ok := f.cardsOf(doc, now)
	i := place(doc, id) - 1
	if !ok || i < 0 {
		return "", false
	}
	row, col := i/l.across+down, i%l.across+across
	if row < 0 || row >= l.down || col < 0 || col >= l.across {
		return "", false
	}
	j := row*l.across + col
	if j >= len(doc.Accounts) {
		if across != 0 {
			return "", false
		}
		j = len(doc.Accounts) - 1
	}
	return doc.Accounts[j].ID, true
}

// Reveal is how far the frame's view of doc is scrolled at now once it shows
// the card of the account with the given id: no further than it must go
// for the card to show whole, so a card in view leaves it where it is, or
// to the card's top, where the card is taller than the view.
func (f Frame) Reveal(doc status.Document, now time.Time, id string) int {
	l, ok := f.cardsOf(doc, now)
	i := place(doc, id) - 1
	if !ok || i < 0 {
		return f.Scroll
	}
	_, top := l.at(i, l.density)
	bottom, scroll := top+l.density.rows(l.bars), f.scrolled(l)
	switch {
	case top < scroll:
		return top
	case bottom > scroll+l.view:
		return min(top, bottom-l.view)
	default:
		return scroll
	}
}

// InView is the account whose card is the first of doc's at now that the
// frame's view shows whole, scrolled as it is, or, where it shows none
// whole, the first it shows: "" where there are no cards.
func (f Frame) InView(doc status.Document, now time.Time) string {
	if len(doc.Accounts) == 0 {
		return ""
	}
	l, ok := f.cardsOf(doc, now)
	if !ok {
		return doc.Accounts[0].ID
	}
	scroll, rows, partly := f.scrolled(l), l.density.rows(l.bars), ""
	for i, a := range doc.Accounts {
		_, top := l.at(i, l.density)
		switch {
		case top >= scroll && top+rows <= scroll+l.view:
			return a.ID
		case partly == "" && top < scroll+l.view && top+rows > scroll:
			partly = a.ID
		}
	}
	return cmp.Or(partly, doc.Accounts[0].ID)
}
