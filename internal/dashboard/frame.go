package dashboard

import (
	"sync"
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
	// states looks saw change, by id; and TurnedOut the windows whose words
	// looks saw turn to out ~, by account and key: each as far as the cue on
	// it has faded, from 0, just seen, to 1, gone.
	Fresh     map[int]float64
	Changed   map[string]float64
	TurnedOut map[Ref]float64
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
	// Traffic is what the router's request stream tells of the sessions'
	// requests, while the watch reads it: zero while it doesn't.
	Traffic Traffic
	// Order is the order Sessions' calls run in, as the watch keeps it from
	// look to look: a seat keeps its row while the router lists it there,
	// and one new to its account joins its group's foot.
	Order Order
	// Scroll is how many rows the view is scrolled down by, where it doesn't
	// fit, or on the Overview, how many accounts USAGE is scrolled down by
	// within its section: no further than Scrolling says it goes.
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
	// document, as nameOfWindow has it.
	WindowName func(key string) string
	// History is History's sub-tab shown, what each of its views shows, and
	// their pages.
	History HistoryPage

	// judged is the pool as Judged judged it, and built the Overview's
	// sections as Built built them, each for every look the frame takes of
	// their document at their moment to share: nil, where each look works
	// them out afresh.
	judged *poolJudged
	built  *sectionsBuilt
}

// docAt is a document at a moment, as a frame's pool is judged and its
// sections built of: the document known by when it was generated.
type docAt struct {
	generated, at time.Time
}

// docAtOf is doc at now, as docAt has it.
func docAtOf(doc status.Document, now time.Time) docAt {
	return docAt{generated: doc.GeneratedAt, at: now}
}

// is reports whether d is doc at now.
func (d docAt) is(doc status.Document, now time.Time) bool {
	return d.generated.Equal(doc.GeneratedAt) && d.at.Equal(now)
}

// poolJudged is the pool of a document as Judged judges it at a moment:
// judged once, the first time it's asked for.
type poolJudged struct {
	of   docAt
	pool func() status.Pool
}

// Judged is the frame with the pool of doc judged at now once, as its Policy
// judges it, the first time a look asks, and shared by every look it, or a
// copy of it, takes of doc at now after: wherever the Overview reads it, and
// the bars eased to it, as Filled has them. A frame judged of doc at now
// already is as it was; a look at another moment, or of another document,
// judges it afresh.
func (f Frame) Judged(doc status.Document, now time.Time) Frame {
	if f.judges(doc, now) {
		return f
	}
	policy := f.Policy
	f.judged = &poolJudged{of: docAtOf(doc, now), pool: sync.OnceValue(func() status.Pool { return doc.PoolAt(now, policy) })}
	return f
}

// judges reports whether the frame has judged the pool of doc at now.
func (f Frame) judges(doc status.Document, now time.Time) bool {
	return f.judged != nil && f.judged.of.is(doc, now)
}

// pool is the pool of doc at now: as Judged judged it, where it judged it of
// doc at now, else judged afresh, as the frame's Policy judges it.
func (f Frame) pool(doc status.Document, now time.Time) status.Pool {
	if f.judges(doc, now) {
		return f.judged.pool()
	}
	return doc.PoolAt(now, f.Policy)
}

// Draw draws the frame of doc at now: a string for each of its rows, each,
// where the look paints its canvas, the terminal's whole width. It draws the
// page, as page has it, its numbers brightened as Brightened says; then,
// where a card is open, fades the page and draws the card over it.
// Countdowns run from now, and times show in now's time zone.
func (f Frame) Draw(doc status.Document, now time.Time) []string {
	return f.finish(f.page(doc, now)).rows(f.Look)
}

// Rendered is the frame of doc at now drawn once: its rows, as Draw draws
// them, and what moves on screen in it, as moving reads it off the same page.
func (f Frame) Rendered(doc status.Document, now time.Time) ([]string, Motion) {
	c := f.page(doc, now)
	moving := f.moving(c, doc, now)
	return f.finish(c).rows(f.Look), moving
}

// finish finishes the page drawn on c: its numbers brightened as Brightened
// says; then, where a card is open, the page faded and the card drawn over
// it.
func (f Frame) finish(c *canvas) *canvas {
	c.brighten(f.Brightened, f.Look)
	if f.Card != nil {
		c.fade()
		f.Card.card(f).draw(c)
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
	f.stateLine(c)
	f.keyLine(c)
	return c
}

// view draws the tab shown of doc at now from row top, where the page has
// room for it, as cramped says: the Overview, History, Sessions and Runway;
// and nothing of a tab not built yet.
func (f Frame) view(c *canvas, doc status.Document, now time.Time) {
	if f.cramped() {
		return
	}
	top := f.top()
	switch f.View {
	case Overview:
		f.overview(c, doc, now, top)
	case History:
		f.history(c, top)
	case Sessions:
		f.sessions(c, doc, now, top)
	case Runway:
		f.runway(c, doc, now, top)
	}
}

// stateLine draws the view's state line, over the key line. The views
// milestone 5 built draw their own line there.
func (f Frame) stateLine(*canvas) {}

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
// y, width cells wide and rows tall: on the Overview, USAGE's body, as
// overviewScrolls has it; elsewhere, the view's rows.
func (f Frame) scrolls(doc status.Document, now time.Time) (x, y, width, rows int) {
	if f.View == Overview {
		return f.overviewScrolls(doc, now)
	}
	return 0, f.top(), f.Width, f.viewRows()
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
