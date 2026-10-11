package dashboard

import "strings"

// The widths a card wants, its edges included: the routing card's, and
// help's.
const (
	routingWidth = 82
	helpWidth    = 128
)

// The cells a card's content starts in from its left edge, and its keys row
// ends short of its right edge.
const (
	cardInset     = 3
	cardKeysInset = 2
)

// Opened is a card open over the page, as its data: the routing card's, or
// help's. The frame builds the card it draws of it.
type Opened interface {
	// card is the card f draws of it.
	card(f Frame) Card
}

// Card is a card drawn over the page, centred across it and down it, the
// page faded behind it: a rounded line, its title in its top edge; its body,
// from cardInset cells in; and its keys, a blank row over them,
// right-aligned cardKeysInset cells in on its last row but one.
type Card struct {
	// title is in its top edge, after ╭─, as ROUTING · NEW SESSIONS.
	title string
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

// draw draws the card over what's drawn on c, centred across it and down it,
// the cells inside its line blank.
func (k Card) draw(c *canvas) {
	inside := max(k.width-2, 0)
	rows := len(k.body)
	if len(k.keys) > 0 {
		rows += 2
	}
	x, y := (c.width-k.width)/2, (c.height()-rows-2)/2
	title := line{{"─ " + k.title + " ", dimInk}}.fit(inside)
	c.text(x, y, "╭", dimInk)
	c.text(c.line(x+1, y, title), y, rule(inside-title.width())+"╮", dimInk)
	for row := range rows {
		c.text(x, y+1+row, "│"+strings.Repeat(" ", inside)+"│", dimInk)
	}
	for i, l := range k.body {
		c.line(x+cardInset, y+1+i, l.fit(k.width-2*cardInset))
	}
	if len(k.keys) > 0 {
		c.right(x+k.width-cardKeysInset, y+rows, keyRow(k.keys).fit(k.width-2*cardKeysInset))
	}
	c.text(x, y+rows+1, "╰"+rule(inside)+"╯", dimInk)
}

// keyRow is a card's keys as its keys row lists them, three cells apart.
func keyRow(keys []Key) line {
	var l line
	for i, k := range keys {
		if i > 0 {
			l = append(l, spaces(3))
		}
		l = append(l, k.words()...)
	}
	return l
}
