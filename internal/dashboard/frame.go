package dashboard

import (
	"slices"
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// The widths the frame's arrangement changes at: from besideFrom columns,
// one account's card has a column beside it; under phoneUnder, the frame is
// a phone's.
const (
	besideFrom = 150
	phoneUnder = 100
)

// Frame is the dashboard drawn from the document, the clock, and what the
// watch knows besides: full screen, as a watch draws it, the page of the tab
// shown, and the card open over it, if any.
type Frame struct {
	// Width and Height are the terminal's, in cells.
	Width, Height int
	Look          Look
	// View is the tab shown.
	View View
	// Lost is when the router stopped answering, while its last document
	// stays on screen, or before any has been read: zero otherwise.
	Lost time.Time
	// Outdated is set when the router is from before it told of events and
	// gave its history: RECENT asks for it to be restarted.
	Outdated bool
	// Fresh are the events looks saw newly, by id, and Changed the accounts
	// whose cards' states looks saw change, by id, each as far as the
	// highlight on its row has faded, from 0, just seen, to 1, gone.
	Fresh   map[int]float64
	Changed map[string]float64
	// Trails are how the accounts' windows have been used, for the charts.
	Trails Trails
	// Eased are how much of each window the bars fill, by its account and
	// key, while they ease to the document's readings after a read: nil once
	// they're there. Everything else is drawn from the document's own.
	Eased map[Ref]float64
	// Featured is which window every card features, as w sets it on the
	// Overview, and Chart the style every card draws it in, as g sets it;
	// and Span how far ahead Runway looks, as w switches it there.
	Featured Feature
	Chart    Chart
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
	// KeyLine is the keys that work on the page, as its last row lists them,
	// or the note a key left in their place.
	KeyLine KeyLine
	// Card is the card open over the page, which is drawn faded behind it,
	// standing still: nil while none is.
	Card Opened
	// Policy is the provider's say in which windows every model shares, which
	// a session's 5-hour window is, and which its week.
	Policy score.Policy
}

// Draw draws the frame of doc at now: a string for each of its rows, each,
// where the look paints its canvas, the terminal's whole width. It draws the
// page from the top, its title row, its status line, its heading row, the
// view, the view's state line and the key line, but for the heading row and
// the view where the terminal is too short for them, as cramped says; then,
// where a card is open, fades the page and draws the card over it.
// Countdowns run from now, and times show in now's time zone.
func (f Frame) Draw(doc status.Document, now time.Time) []string {
	c := newCanvas(f.Width, f.Height)
	f.title(c, now)
	f.routerStatus(c, doc, now)
	f.headingRow(c)
	f.view(c, doc, now)
	f.stateLine(c)
	f.keyLine(c)
	if f.Card != nil {
		c.fade()
		f.Card.card(f).draw(c)
	}
	return c.rows(f.Look)
}

// view draws the tab shown of doc at now from row top, where the page has
// room for it, as cramped says: the Overview, milestone 5's cards, as a list
// where the view is too short for them, as short says; Sessions and Runway;
// and nothing of a tab not built yet.
func (f Frame) view(c *canvas, doc status.Document, now time.Time) {
	if f.cramped() {
		return
	}
	top := f.top()
	switch {
	case f.View == Overview && f.short(doc, now):
		f.list(c, doc, now)
	case f.View == Overview:
		f.accounts(c, doc, now, top)
	case f.View == Sessions:
		f.sessions(c, doc, now, top)
	case f.View == Runway:
		f.runway(c, doc, now, top)
	}
}

// stateLine draws the view's state line, over the key line. The views
// milestone 5 built draw their own line there.
func (f Frame) stateLine(*canvas) {}

// short reports whether the frame of doc at now, showing the Overview, is
// too short for a row of cards at their sparest in the view's rows. Sessions
// and Runway keep to the rows they have.
func (f Frame) short(doc status.Document, now time.Time) bool {
	if f.View != Overview {
		return false
	}
	g := f.gridOf(len(doc.Accounts), len(shownWindows(doc, now, f.Policy)))
	return f.viewRows() < densities[len(densities)-1].rows(g.bars)
}

// list draws the Overview of doc at now as a view too short for its cards
// has it: a line for each account, as many as fit in the view's rows, its
// place, its name and its state in words, cut to fit.
func (f Frame) list(c *canvas, doc status.Document, now time.Time) {
	top := f.top()
	for i, a := range doc.Accounts[:min(len(doc.Accounts), f.viewRows())] {
		state := face{state: doc.StateOf(a, now, f.Policy)}.stateLine()
		said := slices.Concat(line{{strconv.Itoa(i+1) + " ", dimInk}, {name(a), titleInk}, spaces(2)}, state)
		c.line(margin, top+i, said.fit(f.edge()-margin))
	}
}

// The page's rows under its view: the state line, or the blank row that
// keeps the view off the key line, and the key line. leastView is the fewest
// rows the view is drawn in.
const (
	underView = 2
	leastView = 3
)

// statusRow is the row the status line is on: the one after the blank row
// under the title row, its tabs' on a phone.
func (f Frame) statusRow() int {
	return f.titleRows() + 1
}

// headingY is the row the heading row is on: the one after the blank row
// under the status line.
func (f Frame) headingY() int {
	return f.statusRow() + 2
}

// top is the row the view starts at: the one after the blank row under the
// heading row, and on a phone, its period's, where that takes a row of its
// own; or on the Overview, whose sections head themselves, the heading row's.
func (f Frame) top() int {
	if f.View == Overview {
		return f.headingY()
	}
	return f.headingY() + f.heading().rows(f.phone()) + 1
}

// viewRows is how many rows the view has: from its top to the row over the
// key line.
func (f Frame) viewRows() int {
	return f.Height - underView - f.top()
}

// cramped reports whether the terminal is too short for the page's fixed
// rows and leastView of the view's: the page is then its title row, its
// status line and its key line alone.
func (f Frame) cramped() bool {
	return f.viewRows() < leastView
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
