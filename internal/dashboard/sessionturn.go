package dashboard

import (
	"cmp"
	"strconv"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// treeAt is the column a turn opened draws its requests' tree down: │ on
// each request's row, and └ on its foot's.
const treeAt = 2

// turnTicks are the columns a turn opened shows its requests in: the Log's
// shown at first, but session, which a session's own page doesn't need.
var turnTicks = LogTicks{}.Toggled(LogSession)

// requestsOut is how a turn opened says how many of its requests are out of
// view, and how to go through them: "▼ 32 more of its requests   ·   j/k
// through them".
var requestsOut = outForm{
	above: " of its requests", below: " of its requests", gap: "   ·   ", keys: "j/k", does: " through them",
	inks: turnsOut.inks,
}

// turnsRowKind is what a row of a session's turns is: a turn's own; or under
// the turn open's, its heads, a request of it, or its foot.
type turnsRowKind int

const (
	aTurn turnsRowKind = iota
	openHeads
	openRequest
	openFoot
)

// turnsRow is a row of a session's turns as its page lays them out: of the
// kind given, a turn's by its index among the page's turns, newest first,
// and a request's by its index among the turn open's.
type turnsRow struct {
	kind  turnsRowKind
	index int
}

// turnsRows are the rows of the session's turns, newest first, as the
// layout l lays them out: a row a turn, and under the turn open's own, its
// heads, its requests shown, as requestsShown has them, and its foot.
func (s *SessionShown) turnsRows(l sessionLayout) []turnsRow {
	rows := make([]turnsRow, 0, len(s.Page.Turns))
	for i, t := range s.Page.Turns {
		rows = append(rows, turnsRow{kind: aTurn, index: i})
		if s.Open == 0 || t.Turn != s.Open {
			continue
		}
		rows = append(rows, turnsRow{kind: openHeads})
		from, to := s.requestsShown(l)
		for r := from; r < to; r++ {
			rows = append(rows, turnsRow{kind: openRequest, index: r})
		}
		rows = append(rows, turnsRow{kind: openFoot})
	}
	return rows
}

// turnsInView are the rows of the session's turns, as turnsRows lays them
// out, and the first in view and the one after the last, as they're
// scrolled: no further than leaves the rows the layout gives them full.
func (s *SessionShown) turnsInView(l sessionLayout) (rows []turnsRow, from, to int) {
	rows = s.turnsRows(l)
	from = min(max(s.Scroll, 0), max(len(rows)-l.turns, 0))
	return rows, from, min(from+l.turns, len(rows))
}

// requestsShown are the turn open's requests shown, the first and the one
// after the last, as far as they're scrolled: as many as the layout l shows
// at a time, no further than leaves them full.
func (s *SessionShown) requestsShown(l sessionLayout) (from, to int) {
	shown := min(len(s.Requests), l.requests)
	from = min(max(s.RequestsScroll, 0), len(s.Requests)-shown)
	return from, from + shown
}

// Kept is how many rows under the turn chosen's own its turns keep in view
// with it as they scroll: of the turn open, its heads and its requests shown
// down to the one chosen, as it opens its first (call 57); of any other,
// none.
func (s *SessionShown) Kept() int {
	if s.Open == 0 || s.Open != s.Turn {
		return 0
	}
	return 2 + max(s.Request-s.RequestsScroll, 0)
}

// RequestsScrolling is how far the requests of the turn open on the
// session's page scroll within the rows its layout gives them: none where
// no page or turn is open.
func (f Frame) RequestsScrolling() Scrolling {
	if f.Session == nil || f.Session.Open == 0 {
		return Scrolling{}
	}
	return byRows(len(f.Session.Requests), f.sessionLayout(len(f.Session.Page.Accounts)).requests)
}

// turnLitRow is the row the session's page draws the request chosen of its
// turn open on: false where none is open, or it's out of view, or the page
// has room for its chrome alone.
func (f Frame) turnLitRow() (int, bool) {
	l := f.sessionLayout(len(f.Session.Page.Accounts))
	if l.chrome {
		return 0, false
	}
	rows, from, to := f.Session.turnsInView(l)
	for i := from; i < to; i++ {
		if r := rows[i]; r.kind == openRequest && r.index == f.Session.Request {
			return l.turnsAt + i - from, true
		}
	}
	return 0, false
}

// turnCardRows are the rows a details card placed by a request of the
// session's turn open sits within: from the page's heading to its turns'
// last row.
func (f Frame) turnCardRows() (first, last int) {
	l := f.sessionLayout(len(f.Session.Page.Accounts))
	return l.heading, l.moreTurns - 1
}

// requestsLayout is how the page lays out its turn open's requests' rows:
// the Log's, from turnSpread's left, at every width.
func (d sessionDrawn) requestsLayout() logLayout {
	return turnSpread.wide(d.Width, turnTicks)
}

// pageRow is the row r of the session's turns: a turn's, which a click
// chooses; the turn open's heads; a request of it, which a click chooses;
// or its foot.
func (d sessionDrawn) pageRow(r turnsRow) pageRow {
	row := pageRow{turnsRow: r}
	switch r.kind {
	case aTurn:
		t := d.page.Turns[r.index]
		row.click = Target{On: OnTurn, Turn: t.Turn}
		row.draw = func(c *canvas, y int) { d.turn(c, y, t, r.index == 0) }
	case openHeads:
		row.draw = func(c *canvas, y int) { logHeads(c, y, d.requestsLayout(), faintInk) }
	case openRequest:
		row.click = Target{On: OnRequest, Row: r.index}
		row.draw = func(c *canvas, y int) { d.request(c, y, r.index) }
	case openFoot:
		row.draw = d.foot
	}
	return row
}

// request draws the row of the turn open's request at index i along row y:
// the Log's row of it, as requestRow draws it in the page's columns, as it
// stands at now, the frame's stream telling what it holds of it, as the
// Log's rows are drawn; its tree's │ before it in border; lit across the
// width where it's chosen.
func (d sessionDrawn) request(c *canvas, y, i int) {
	c.text(treeAt, y, "│", borderInk)
	requestRow(c, y, d.shown.Requests[i].Row.Live(d.Stream, d.now), d.requestsLayout(), func(id string) string { return named(d.m, id) }, d.now)
	if i == d.shown.Request {
		c.surface(0, y, d.Width, litInk)
	}
}

// foot draws the turn open's foot along row y: its tree's └ in border, and
// how many of its requests are out of view either way, and how to go
// through them; or, while they're read, loading…, in text.subtle.
func (d sessionDrawn) foot(c *canvas, y int) {
	c.text(treeAt, y, "└", borderInk)
	if d.shown.RequestsLoading {
		c.line(turnSpread.left, y, line{{loadingWord, dimInk}}.fit(d.Width-turnSpread.left))
		return
	}
	from, to := d.shown.requestsShown(d.l)
	c.line(turnSpread.left, y, outOfView(from, len(d.shown.Requests)-to, d.Width-turnSpread.left, requestsOut))
}

// turnsIn counts the turns' own rows among rows.
func turnsIn(rows []turnsRow) int {
	n := 0
	for _, r := range rows {
		if r.kind == aTurn {
			n++
		}
	}
	return n
}

// turnPart is one of the columns of a session's turns.
type turnPart int

// The columns of a session's turns, in the row's order: its number, its
// start, how long it took, its requests, its tools, the tokens it read,
// wrote and put out, its worth, the accounts it went to, and the newest's ↑
// asking while a request of it waits.
const (
	turnNumber turnPart = iota
	turnStarted
	turnTook
	turnRequests
	turnTools
	turnRead
	turnWritten
	turnOut
	turnWorth
	turnOn
	turnAsking
)

// The turns the columns of a session's turns give way in as it narrows, in
// this order, so a turn keeps its number, start, length, requests, worth and
// accounts: tools first, the column that grows, so none comes back to narrow
// it as the frame widens; ↑ asking, which row 6 says too; read, written and
// out; the start, length and requests to their short heads; and last, on to
// its least.
const (
	toolsLeaves = iota + 1
	askingLeaves
	readLeaves
	writtenLeaves
	outLeaves
	figuresNarrow
	turnOnNarrows
)

// turnGap is the blanks between the columns of a session's turns.
const turnGap = 2

// turnPartFacts are each column's heads, the fullest first, as firstFit
// tries them in its cells; its least and its natural, 0 for its least, the
// number's, on's and ↑ asking's as turnParts gives them; whether its words
// are right-aligned; and the turns it narrows and leaves in, 0 never.
var turnPartFacts = [...]struct {
	heads           []string
	least, natural  int
	right           bool
	narrows, leaves int
}{
	turnNumber:   {heads: []string{"#"}, right: true},
	turnStarted:  {heads: []string{"started", "start"}, least: 5, natural: 7, narrows: figuresNarrow},
	turnTook:     {heads: []string{"took"}, least: 4, natural: 5, narrows: figuresNarrow},
	turnRequests: {heads: []string{"requests", "req"}, least: 3, natural: 8, right: true, narrows: figuresNarrow},
	turnTools:    {heads: []string{"tools"}, least: 12, natural: 30, leaves: toolsLeaves},
	turnRead:     {heads: []string{"read"}, least: 7, right: true, leaves: readLeaves},
	turnWritten:  {heads: []string{"written"}, least: 7, right: true, leaves: writtenLeaves},
	turnOut:      {heads: []string{"out"}, least: 6, right: true, leaves: outLeaves},
	turnWorth:    {heads: []string{"worth"}, least: 7, right: true},
	turnOn:       {heads: []string{"on"}, narrows: turnOnNarrows},
	turnAsking:   {leaves: askingLeaves},
}

// leastNumber is the fewest cells a turn's number takes: to turn 999.
const leastNumber = 3

// turnParts are the columns of a session's turns as flex lays them,
// turnGap apart, each from its least to its natural, holding its natural
// until it narrows in its turn: the number in the cells given; on from a
// label's least to the natural given; ↑ asking in its words' cells; and
// tools taking what's left.
//
// No natural comes from the turns shown, so the columns hold still as they
// scroll and as turns come and go, but the number's, which moves on a cell
// only as the session reaches its thousandth turn.
func turnParts(number, on int) []flex.Part {
	parts := make([]flex.Part, len(turnPartFacts))
	for p, facts := range turnPartFacts {
		parts[p] = flex.Part{
			Least: facts.least, Natural: cmp.Or(facts.natural, facts.least),
			Narrows: facts.narrows, Leaves: facts.leaves, Gap: turnGap,
		}
	}
	asking := ansi.StringWidth(views.Activity{Doing: views.Asking}.WaitingSaid())
	parts[turnNumber].Least, parts[turnNumber].Natural = number, number
	parts[turnOn].Least, parts[turnOn].Natural = labelLeast, on
	parts[turnAsking].Least, parts[turnAsking].Natural = asking, asking
	parts[turnTools].Grows = true
	return parts
}

// turnColumn is a column of a session's turns as it's laid out: which it
// is, from column x, width cells wide.
type turnColumn struct {
	part     turnPart
	x, width int
}

// turnColumnsOf are the columns of a session's turns shown across a frame
// width cells wide, the number's cells and on's natural those given, as
// turnParts lays them out. Where even their leasts don't fit, under 40
// columns, they lie as the turns left them, each cut at the frame's edge,
// those past it left out: a turn's row has no other form, as none keeping
// its figures would fit there either.
func turnColumnsOf(width, number, on int) []turnColumn {
	spans, _ := flex.Lay(width, turnParts(number, on)...)
	var columns []turnColumn
	for p, span := range spans {
		if cells := min(span.Size, width-span.At); !span.Left && cells > 0 {
			columns = append(columns, turnColumn{part: turnPart(p), x: span.At, width: cells})
		}
	}
	return columns
}

// head is the column's head, in text.subtle: the fullest of its heads that
// fits its cells; none for ↑ asking.
func (col turnColumn) head() line {
	heads := turnPartFacts[col.part].heads
	if len(heads) == 0 {
		return nil
	}
	forms := make([]line, len(heads))
	for i, h := range heads {
		forms[i] = line{{h, dimInk}}
	}
	return firstFit(col.width, forms...)
}

// draw draws l in the column along row y, cut to its cells.
func (col turnColumn) draw(c *canvas, y int, l line) {
	l = l.fit(col.width)
	if turnPartFacts[col.part].right {
		c.right(col.end(), y, l)
		return
	}
	c.line(col.x, y, l)
}

// end is the column after the column's last.
func (col turnColumn) end() int {
	return col.x + col.width
}

// turnNumberCells are the cells the number of the newest of the turns,
// newest first, takes, leastNumber at the least.
func turnNumberCells(turns []views.Turn) int {
	if len(turns) == 0 {
		return leastNumber
	}
	return max(leastNumber, len(strconv.Itoa(turns[0].Turn)))
}

// turnOnCells is the natural of a turn's on: the most a turn says of the
// accounts the moment m knows, a move between the two widest of their names,
// each a label's, as labelsCells counts them, as turnOn says it; or the one,
// where it knows one; labelLeast at the least. It counts the accounts
// configured, never those the session ran on.
func turnOnCells(m status.Moment) int {
	cells := labelsCells(m)
	widest := 0
	switch {
	case len(cells) > 1:
		widest = cells[0] + ansi.StringWidth(moveArrow) + cells[1]
	case len(cells) == 1:
		widest = cells[0]
	}
	return max(widest, labelLeast)
}
