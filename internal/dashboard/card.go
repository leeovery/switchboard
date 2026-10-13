package dashboard

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/theme"
)

// The widths a card wants, its edges included: the routing card's, and
// help's.
const (
	routingWidth = 82
	helpWidth    = 128
)

// cardInset is how many cells in from a card's edges its content runs: its
// line and two blank cells, either side, so its keys row ends where its
// content does.
const cardInset = 3

// Opened is a card open over the page, as its data: the routing card's, or
// help's. The frame builds the card it draws of it.
type Opened interface {
	// card is the card f draws of it.
	card(f Frame) Card
}

// Card is a card drawn over the page, centred across it and down it, or
// where it's of a line of the page, by that line, the page faded behind it:
// a rounded line, its title in its top edge; a blank row; its body, from
// cardInset cells in, the rows the cursor's on lit; a blank row; and its
// keys, where it has any, right-aligned cardInset cells in on its last row
// but one.
type Card struct {
	// title is in its top edge, after ╭─, as ROUTING · NEW SESSIONS, in
	// text.subtle; or, where titled is set, as titled draws it, an account
	// it names in text.primary, as the Log's details name one.
	title  string
	titled line
	// width is the cells it takes across, its edges included, as cardWidth
	// gives it.
	width int
	// body are the lines it shows, from its top: where they scroll, those in
	// view.
	body []line
	// keys are its keys row: none where it has none.
	keys []Key
	// scroll is how far its body is scrolled, where it's taller than the
	// card has room for.
	scroll cardScroll
	// lit are the rows of its body the cursor's on: none where it has no
	// cursor.
	lit bodyRows
	// of is the line of the page it's of, as the Log's details are of the
	// line chosen: nil where it's of none.
	of *ofLine
}

// ofLine is the line of the page a card is of: its row, which stays as the
// page draws it, bright, as the page fades behind the card; and, where the
// card sits by it, under it or over it, the row its top edge is on.
type ofLine struct {
	row    int
	top    int
	placed bool
}

// kept are the rows of the page the card keeps bright as the page fades
// behind it: the row of the line it's of, where it's of one.
func (k Card) kept() []int {
	if k.of == nil {
		return nil
	}
	return []int{k.of.row}
}

// bodyRows are rows of a card's body: n of them from its row first.
type bodyRows struct {
	first, n int
}

// cardScroll is how far a card's body is scrolled: how many of its rows are
// out of view above those it shows, and below them.
type cardScroll struct {
	above, below int
}

// cardWidth is how wide a card that wants to be wanted cells wide is drawn
// on the frame: no wider than the frame less a cell either side, and on a
// phone, that wide.
func (f Frame) cardWidth(wanted int) int {
	if f.phone() {
		return f.Width - 2
	}
	return min(wanted, f.Width-2)
}

// bodyRoom is how many rows of body a card with keys, or without, has room
// for on the frame, where the card takes all but spare of the frame's rows.
func (f Frame) bodyRoom(spare int, keys bool) int {
	return f.Height - spare - chrome(keys)
}

// chrome is how many of a card's rows aren't its body: its top edge and the
// blank row under it, the blank row over its keys row, or over its foot,
// its keys row, where it has keys, and its foot.
func chrome(keys bool) int {
	if keys {
		return 5
	}
	return 4
}

// CardScrolling is how far the body of the card open over the frame
// scrolls: Most, the furthest, in rows, zero where it fits; and Page, how
// many rows of it show at once. It's zero while no card is open.
func (f Frame) CardScrolling() Scrolling {
	if f.Card == nil {
		return Scrolling{}
	}
	card := f.Card.card(f)
	return Scrolling{Most: card.scroll.above + card.scroll.below, Page: len(card.body)}
}

// box is the cells the card takes on a page width × height, its edges
// included: from x along row y, w cells across and h down, centred across
// the page; down it, where it sits by the line it's of, from the row it's
// placed at; else centred down it, or, taller than the page, from its top,
// so its title shows and its foot is cut.
func (k Card) box(width, height int) (x, y, w, h int) {
	h = len(k.body) + chrome(len(k.keys) > 0)
	y = max((height-h)/2, 0)
	if k.of != nil && k.of.placed {
		y = k.of.top
	}
	return (width - k.width) / 2, y, k.width, h
}

// draw draws the card over what's drawn on c, in its box, the cells inside
// its line blank. Its lit rows are on bg.selection from a cell before its
// content to a cell after it, a cell clear of its line either side, as the
// Log's f panel lights a row a cell either side of what it says.
func (k Card) draw(c *canvas) {
	keys := len(k.keys) > 0
	x, y, _, height := k.box(c.width, c.height())
	inside, content := max(k.width-2, 0), max(k.width-2*cardInset, 0)
	title := k.titled
	if title == nil {
		title = line{{k.title, dimInk}}
	}
	title = slices.Concat(line{{"─ ", dimInk}}, title, line{{" ", dimInk}}).fit(inside)
	c.text(x, y, "╭", dimInk)
	c.text(c.line(x+1, y, title), y, rule(inside-title.width())+"╮", dimInk)
	for row := 1; row < height-1; row++ {
		c.text(x, y+row, "│"+strings.Repeat(" ", inside)+"│", dimInk)
	}
	for i, l := range k.body {
		c.line(x+cardInset, y+2+i, l.fit(content))
	}
	for i := range k.lit.n {
		c.surface(x+cardInset-1, y+2+k.lit.first+i, content+2, hue{token: theme.BgSelection})
	}
	if keys {
		c.right(x+k.width-cardInset, y+height-2, keyRow(k.keys).fit(content))
	}
	c.text(x, y+height-1, "╰"+rule(inside)+"╯", dimInk)
}

// keyRow is a card's keys as its keys row lists them, three cells apart.
func keyRow(keys []Key) line {
	var l line
	for i, k := range keys {
		if i > 0 {
			l = append(l, spaces(keysApart))
		}
		l = append(l, k.words()...)
	}
	return l
}
