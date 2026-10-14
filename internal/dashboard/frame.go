package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// phoneUnder is the width the frame is a phone's under.
const phoneUnder = 100

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
	// Failed is why the last read failed, and when the next is due, while it's
	// the last: zero otherwise. The status line says it, but where it says
	// the router isn't answering.
	Failed Failure
	// Outdated is set when the router is from before it told of events and
	// gave its history: RECENT asks for it to be restarted.
	Outdated bool
	// Fresh are the events looks saw newly, by id; Changed the accounts whose
	// states looks saw change, by id, and StateChanged those whose states in
	// a word, as Accounts says them, looks saw change; and TurnedOut the
	// windows whose words looks saw turn to out ~, by account and key: each
	// as far as the cue on it has faded, from 0, just seen, to 1, gone.
	Fresh        map[int]float64
	Changed      map[string]float64
	StateChanged map[string]float64
	TurnedOut    map[Ref]float64
	// Trails are how the accounts' windows have been used, for the charts.
	Trails Trails
	// Eased are how much of each bar easing after a read it fills, by its
	// window's account and key, the pool's by its key alone, on its way to
	// filling as the view draws it of the document, as Filled has it: nil
	// once they're there. Everything else is drawn from the document's own.
	Eased map[Ref]float64
	// Brightened are how bright each number a read or the request stream
	// changed is still, as Numbers names them: from 1, just changed, to 0,
	// faded back.
	Brightened map[Number]float64
	// Sections are the Overview's sections shown, as u, g, r, s, c and l
	// show and hide them, and By how its windows are grouped, as b switches
	// it: by window, or by account.
	Sections Sections
	By       By
	// Traces are the requests the Overview's traces draw, by account and by
	// session, as the watch counts them; and TraceScale the requests a
	// minute that fill a trace's height, 0 for the live rule, the busiest
	// column of every trace that shows on the page, never less than 8.
	Traces     views.Traces
	TraceScale float64
	// Span is how far ahead Runway looks, as w switches it.
	Span Span
	// Sessions are the sessions the router listed, the one seen last first,
	// for Sessions' calls, SESSIONS and ROUTING's counts: nil where it listed
	// none, as while probing, and each account's sessions are counted as the
	// document counts them.
	Sessions []status.Session
	// Stream is what the router's request stream has told of the routed
	// requests, as the views read it, by each request's id: zero where the
	// watch hears none.
	Stream views.Stream
	// Traffic is what the router's request stream tells of the sessions'
	// requests, as Sessions' switchboard draws it, while the watch reads it:
	// zero while it doesn't.
	Traffic Traffic
	// Order is the order Sessions' calls run in, as the watch keeps it from
	// look to look: a seat keeps its row while the router lists it there,
	// and one new to its account joins its group's foot.
	Order Order
	// Scroll is how far the view is scrolled down, where it doesn't fit: in
	// rows; on the Overview, in accounts, USAGE's, within its section; and
	// on History, the Year in rows, Weeks in accounts, Days in accounts too,
	// BY ACCOUNT's, within it, and Tokens in periods. It goes no further than
	// Scrolling says it goes.
	Scroll int
	// Focus is the account the Overview's focus is on, by id, which ↑ and ↓
	// move and GRAPHS follows: "" while it's on none.
	Focus string
	// Layout is the Overview's layout as the watch keeps it from look to
	// look, as LayoutOf gave it, so it changes only on a key or a resize, or
	// a section's shape changing: the zero Layout, or one made for another
	// frame or shape, lays it out afresh.
	Layout Layout
	// KeyLine is the keys that work on the page, as its last row lists them,
	// or the note a key left in their place.
	KeyLine KeyLine
	// Card is the card open over the page, which is drawn faded behind it,
	// standing still: nil while none is.
	Card Opened
	// Policy is the provider's say in which windows every model shares, which
	// a session's 5-hour window is, and which its week.
	Policy score.Policy
	// WindowName names a window by its key wherever the Overview names one,
	// as "5-hour" or "Fable week": nil names it by its label in the
	// document, as status.NameOfWindow has it.
	WindowName func(key string) string
	// WeekWindow is the key of the window a week long every model shares,
	// the accounts' own week, as the config names it: the week Accounts'
	// WEEK USED shows, and its page reads the weeks of.
	WeekWindow string
	// Accounts is Accounts' sub-tab shown, its period, what's chosen in it,
	// and its page.
	Accounts AccountsShown
	// History is History's sub-tab shown, what each of its views shows, and
	// their pages.
	History HistoryPage
	// Session is the session's page open, drawn in place of the view: nil
	// while none is.
	Session *SessionShown
	// Log is the Log's sub-tab shown, its tables and how each stands.
	Log LogPage

	// held is the moment of a document as At holds it, and built the
	// Overview's sections as Built built them, each for every look the frame,
	// or a copy of it, takes of their document at their moment to share; and
	// runs the sessions running, for the sections built together to share:
	// nil, where each look works them out afresh.
	held  *status.HeldMoment
	built *sectionsBuilt
	runs  *runningHeld
}

// At is the frame holding the moment of doc at now, as its Policy judges it:
// worked out once, the first time a look asks, and shared by every look the
// frame, or a copy of it, takes of doc at now after, each view's and the bars
// eased to it, as Filled has them. A frame holding the moment of doc at now,
// as its Policy judges it, already is as it was; a look at another moment, of
// another document, or one another policy judged, works it out afresh. Each
// look the frame takes, as Draw, Numbers and Scrolling do, holds the moment
// first, so none works out two.
func (f Frame) At(doc status.Document, now time.Time) Frame {
	if f.held.Of(doc, now, f.Policy) {
		return f
	}
	f.held = doc.HoldAt(now, f.Policy)
	return f
}

// Holding is the frame holding h, a moment of a document at a time held
// already, as one a read worked out: At keeps it for a look at that document
// at that time, where the frame's Policy judged it, and works one out afresh
// for any other.
func (f Frame) Holding(h *status.HeldMoment) Frame {
	f.held = h
	return f
}

// Moment is doc at now, as every view reads it: as At holds it, where the
// frame holds the moment of doc at now as its Policy judges it, else worked
// out afresh, as its Policy judges it.
func (f Frame) Moment(doc status.Document, now time.Time) status.Moment {
	if f.held.Of(doc, now, f.Policy) {
		return f.held.Moment()
	}
	return doc.At(now, f.Policy)
}

// Draw draws the frame of doc at now: a string for each of its rows, each,
// where the look paints its canvas, the terminal's whole width. It draws the
// page, as page has it, its numbers brightened as Brightened says; then,
// where a card is open, fades the page and draws the card over it.
// Countdowns run from now, and times show in now's time zone. Every view
// reads the one moment of doc at now, as At holds it.
func (f Frame) Draw(doc status.Document, now time.Time) []string {
	f = f.At(doc, now)
	return f.finish(f.page(doc, now)).rows(f.Look)
}

// Rendered is the frame of doc at now drawn once: its rows, as Draw draws
// them, and what moves on screen in it, as moving reads it off the same page,
// of the one moment, as At holds it.
func (f Frame) Rendered(doc status.Document, now time.Time) ([]string, Motion) {
	f = f.At(doc, now)
	c := f.page(doc, now)
	moving := f.moving(c, doc, now)
	return f.finish(c).rows(f.Look), moving
}

// finish finishes the page drawn on c: its numbers brightened as Brightened
// says; then, where a card is open, the page faded, but for the line the
// card is of, where it's of one, and the card drawn over it.
func (f Frame) finish(c *canvas) *canvas {
	c.brighten(f.Brightened, f.Look)
	if f.Card != nil {
		card := f.Card.card(f)
		c.fade(card.kept()...)
		card.draw(c)
	}
	return c
}

// page draws the frame's page of doc at now from the top: its title row, its
// status line, its heading row, the view, the view's state line and the key
// line, but for the heading row and the view where the terminal is too short
// for them, as cramped says.
func (f Frame) page(doc status.Document, now time.Time) *canvas {
	c := newCanvas(f.Width, f.Height)
	f.title(c, now)
	f.routerStatus(c, doc, now)
	f.headingRow(c)
	f.view(c, doc, now)
	f.stateLine(c, now)
	f.keyLine(c)
	return c
}

// view draws the tab shown of doc at now from row top, where the page has
// room for it, as cramped says: a session's page, where one is open, in its
// place; else the tab shown: the Overview, Accounts, History, Sessions,
// Runway or the Log.
func (f Frame) view(c *canvas, doc status.Document, now time.Time) {
	if f.cramped() {
		return
	}
	top := f.top()
	switch {
	case f.Session != nil:
		f.sessionPage(c, doc, now)
	case f.View == Overview:
		f.overview(c, doc, now, top)
	case f.View == Accounts:
		f.accounts(c, doc, now, top)
	case f.View == History:
		f.history(c, top)
	case f.View == Sessions:
		f.sessions(c, doc, now, top)
	case f.View == Runway:
		f.runway(c, doc, now, top)
	case f.View == Log:
		f.log(c, doc, now, top)
	}
}

// stateLine draws the view's state line, over the key line, as told at now,
// where the page has room for the view, as cramped says: the Log's, once
// its first read lands; or while its find is open, read or not, the find's
// line in its place; but none under a session's page, drawn in the view's
// place. The views milestone 5 built draw their own line there.
func (f Frame) stateLine(c *canvas, now time.Time) {
	if f.cramped() || f.Session != nil || f.View != Log {
		return
	}
	y := f.Height - underView
	if found, ok := f.Log.findLine(f.Width); ok {
		c.line(0, y, found)
		return
	}
	if f.Log.Read {
		c.line(0, y, f.Log.state(f.LogRows()).line(f.Width, now))
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

// Under is what of the frame is under a cell, for the wheel to scroll.
type Under int

const (
	// UnderNothing is a cell of nothing that scrolls: the title row, the
	// status line, the heading row or the key line, or the page faded
	// behind a card.
	UnderNothing Under = iota
	// UnderCard is a cell of the card open over the page, its edges
	// included.
	UnderCard
	// UnderView is a cell of what of the view scrolls, while no card is
	// open.
	UnderView
)

// Under is what's under the cell at x along row y, as the frame draws doc at
// now: while a card is open, the card, in its box, and nothing outside it;
// else what of the view scrolls, as scrolls has it, where the page has room
// for the view, as cramped says.
func (f Frame) Under(doc status.Document, now time.Time, x, y int) Under {
	f = f.At(doc, now)
	inside := func(left, top, width, height int) bool {
		return left <= x && x < left+width && top <= y && y < top+height
	}
	switch {
	case f.Card != nil && inside(f.Card.card(f).box(f.Width, f.Height)):
		return UnderCard
	case f.Card == nil && !f.cramped() && inside(f.scrolls(doc, now)):
		return UnderView
	}
	return UnderNothing
}

// scrolls is what of the view of doc at now scrolls, from column x along row
// y, width cells wide and rows tall: on a session's page, where one is open,
// its turns' rows, across the frame; on the Overview, USAGE's body, as
// overviewScrolls has it; on Accounts, as accountsScrolls has it; on
// History, what of its sub-tab shown does, as historyScrolls has it, across
// the frame; on the Log, its table's lines, under its heads; elsewhere, the
// view's rows.
func (f Frame) scrolls(doc status.Document, now time.Time) (x, y, width, rows int) {
	if f.Session != nil {
		l := f.sessionLayout(len(f.Session.Page.Accounts))
		return 0, l.turnsAt, f.Width, l.turns
	}
	switch f.View {
	case Overview:
		return f.overviewScrolls(doc, now)
	case Accounts:
		return f.accountsScrolls(doc, now)
	case History:
		y, rows = f.historyScrolls()
		return 0, y, f.Width, rows
	case Log:
		return 0, f.top() + 1, f.Width, f.LogRows()
	}
	return 0, f.top(), f.Width, f.viewRows()
}

// cramped reports whether the terminal is too short for the page's fixed
// rows and leastView of the view's, or for a session's page, where one is
// open, as its layout says: the page is then its title row, its status line
// and its key line alone.
func (f Frame) cramped() bool {
	if f.Session != nil {
		return f.sessionLayout(len(f.Session.Page.Accounts)).chrome
	}
	return f.viewRows() < leastView
}

// phone reports whether the frame is a phone's, as Phone has it.
func (f Frame) phone() bool {
	return Phone(f.Width)
}

// Phone reports whether a frame width cells wide is a phone's: under
// phoneUnder columns.
func Phone(width int) bool {
	return width < phoneUnder
}

// edge is the column the frame's content ends before at the right, a margin
// short of the terminal's.
func (f Frame) edge() int {
	return f.Width - margin
}
