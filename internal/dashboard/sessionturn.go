package dashboard

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
// with it as they scroll: of the turn open, its heads and its first request
// shown (call 57); of any other, none.
func (s *SessionShown) Kept() int {
	if s.Open == 0 || s.Open != s.Turn {
		return 0
	}
	return 2
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
// the Log's, from turnSpread's left, wide; else a phone's.
func (d sessionDrawn) requestsLayout() logLayout {
	if d.l.wide {
		return turnSpread.wide(d.Width, turnTicks)
	}
	return turnSpread.phoned(d.Width, turnTicks)
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
// the Log's row of it, as requestRow draws it in the page's columns, its
// tree's │ before it in border; lit across the width where it's chosen.
func (d sessionDrawn) request(c *canvas, y, i int) {
	c.text(treeAt, y, "│", borderInk)
	requestRow(c, y, d.shown.Requests[i].Row, d.requestsLayout(), func(id string) string { return named(d.m, id) }, d.now)
	if i == d.shown.Request {
		c.surface(0, y, d.Width, litInk)
	}
}

// foot draws the turn open's foot along row y: its tree's └ in border, and
// how many of its requests are out of view either way, and how to go
// through them.
func (d sessionDrawn) foot(c *canvas, y int) {
	c.text(treeAt, y, "└", borderInk)
	from, to := d.shown.requestsShown(d.l)
	c.line(turnSpread.left, y, outOfView(from, len(d.shown.Requests)-to, d.Width-turnSpread.left, requestsOut))
}

// turnsIn counts the turns' own rows among rows.
func turnsIn(rows []pageRow) int {
	n := 0
	for _, r := range rows {
		if r.kind == aTurn {
			n++
		}
	}
	return n
}
