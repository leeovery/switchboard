package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// The widths the frame's arrangement changes at: from slotsInRowFrom
// columns, the heading's four slots sit in a row, and from phoneUnder in two
// rows of two; under it, the frame is a phone's.
const (
	slotsInRowFrom = 150
	phoneUnder     = 100
)

// Frame is the dashboard drawn from the document, the clock, and what the
// watch knows besides: full screen, as a watch draws it, its title row, its
// heading, the view shown and its footer; or printed once, as usage prints
// it, without its footer.
type Frame struct {
	// Width and Height are the terminal's, in cells. A frame printed once
	// has no height to fit, zero: it's as tall as it needs, its cards at
	// their richest, and never scrolls.
	Width, Height int
	Look          Look
	// Views are the views there are, in the order tab moves through them,
	// each a tab, and View the one shown.
	Views []View
	View  View
	// Lost is when the router stopped answering, while its last document
	// stays on screen: zero otherwise.
	Lost time.Time
	// Outdated is set when the router is from before it told of events and
	// gave its history: RECENT asks for it to be restarted.
	Outdated bool
	// Fresh are the events looks saw newly, by id, and Changed the accounts
	// whose cards' states looks saw change, by id, each as far as the
	// highlight on its row has faded, from 0, just seen, to 1, gone.
	Fresh   map[int]float64
	Changed map[string]float64
	// History is how the accounts' windows have been used, for the charts.
	History History
	// Featured is which window every card features, as w sets it in
	// Accounts, and Span how far ahead Runway looks, as w switches it there.
	Featured Feature
	Span     Span
	// Sessions are the sessions the router listed, the one seen last first,
	// for the cards' dots and Sessions' calls: nil where it listed none, as
	// while probing, and each card counts its account's sessions as the
	// document does.
	Sessions []status.Session
	// Traffic is what the router's request stream tells of the sessions'
	// requests, while the watch reads it: zero while it doesn't.
	Traffic Traffic
	// Order is the order Sessions' calls run in, as the watch keeps it from
	// look to look: a seat keeps its row while the router lists it there,
	// and one new to its account joins its group's foot.
	Order Order
	// Scroll is how many rows the view's cards, or its lanes, are scrolled
	// down by, where they don't fit: no further than Scrolling says they go.
	Scroll int
	// Focus is the account whose card has the focus, which the arrow keys
	// move and space flips, its edges heavy: "" while no card has it.
	Focus string
	// Flipped are the accounts whose cards are flipped to show their
	// sessions, by id.
	Flipped map[string]bool
	// Selected is the session picked out on the back of the card with the
	// focus, to be moved: zero while none is.
	Selected Seat
	// Patch are the keys that work on the sessions on a card's back, as the
	// back lists them where it has any, before the key that flips it back.
	Patch []Key
	// Keys are the keys that work, the most used first: the footer lists as
	// many as fit.
	Keys []Key
	// Help are every key there is, as the help lists them while it's open
	// over the view, with the key to the glyphs: nil while it's closed.
	Help []Key
	// Note says what the last key did, or why it couldn't, while that's news:
	// the footer says it in place of the keys.
	Note string
	// Status says how reading goes, at the footer's right: when the document
	// was read, as "read 4s ago".
	Status string
	// Policy is the provider's say in which windows every model shares, which
	// a session's 5-hour window is, and which its week.
	Policy score.Policy
}

// Draw draws the frame of doc at now: a string for each of its rows, each,
// where the look paints its canvas, the terminal's whole width. Countdowns
// run from now, and times show in now's time zone.
func (f Frame) Draw(doc status.Document, now time.Time) []string {
	c := newCanvas(f.Width, f.Height)
	if f.printed() {
		c = growing(f.Width)
	}
	top := f.above(c, doc, now)
	switch f.View {
	case Accounts:
		f.accounts(c, doc, now, top)
	case Sessions:
		f.sessions(c, doc, now, top)
	case Runway:
		f.runway(c, doc, now, top)
	}
	if !f.printed() {
		f.footer(c, doc, f.Height-1)
	}
	if f.Help != nil {
		f.help(c)
	}
	return c.rows(f.Look)
}

// above draws what's above the view, the title row and the heading, a blank
// row after each, and returns the row the view starts at.
func (f Frame) above(c *canvas, doc status.Document, now time.Time) int {
	top := f.title(c, now) + 1
	return f.heading(c, doc, now, top) + 1
}

// printed reports whether the frame is printed once, with no height to fit.
func (f Frame) printed() bool {
	return f.Height == 0
}

// phone reports whether the frame is a phone's: under phoneUnder columns.
func (f Frame) phone() bool {
	return f.Width < phoneUnder
}

// edge is the column the frame's content ends before at the right, a margin
// short of the terminal's.
func (f Frame) edge() int {
	return f.Width - margin
}
