package watch

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// turnCarding is what the watch holds of the Log's details card while it's
// open over a session's page, of its turn open's request chosen: how far its
// body is scrolled, and how many rows it took as it was placed, as
// dashboard.LogCard's Placed has it. Its details are the request chosen's as
// the page holds it now, so they follow each read.
type turnCarding struct {
	scroll, placed int
}

// turnKey acts on a key while the session's page has a turn open, as
// sessionKey hands it on, reporting false for one it leaves to the page: esc
// closes the turn; ↑ and ↓ choose the request before and after; and enter
// opens the request chosen's details.
func (m Model) turnKey(key string) (Model, tea.Cmd, bool) {
	switch key {
	case "esc":
		return m.closeTurn(), nil, true
	case "up":
		return m.moveRequest(-1), nil, true
	case "down":
		return m.moveRequest(1), nil, true
	case "enter":
		next, cmd := m.openTurnCard()
		return next, cmd, true
	}
	return m, nil, false
}

// openTurn opens the turn chosen into its requests, in place, as enter does,
// its first chosen: relived builds its requests' rows, and keeps them in
// view as choosing has it. Nothing opens where no turn is chosen.
func (m Model) openTurn() Model {
	s := *m.session
	if s.shown.Turn == 0 {
		return m
	}
	s.shown.Open, s.shown.Request, s.shown.RequestsScroll = s.shown.Turn, 0, 0
	m.session = &s
	return m
}

// closeTurn closes the turn open, as esc does, the turn staying chosen.
func (m Model) closeTurn() Model {
	s := *m.session
	s.shown.Open, s.shown.Requests, s.shown.Request, s.shown.RequestsScroll = 0, nil, 0, 0
	m.session = &s
	return m
}

// moveRequest chooses the turn open's request by requests on from the one
// chosen, oldest first, as ↑ and ↓ do, no further than the first and the
// last, scrolling its requests, and its turns, no further than keeps it in
// view, as requestInView has it.
func (m Model) moveRequest(by int) Model {
	s := *m.session
	i := min(max(s.shown.Request+by, 0), max(len(s.shown.Requests)-1, 0))
	scrolling := m.requestsScrolling()
	s.shown.Request, s.shown.RequestsScroll = i, keptInView(s.shown.RequestsScroll, i, 0, scrolling)
	m.session = &s
	return m.requestInView()
}

// scrollRequests scrolls the turn open's requests by, as j and k do with a
// turn open, no further than they go, the request chosen kept in view: the
// one nearest it in view, where it scrolls out, its turns scrolling as
// requestInView has it.
func (m Model) scrollRequests(by scrollStep) Model {
	s := *m.session
	scrolling := m.requestsScrolling()
	s.shown.RequestsScroll = by.from(s.shown.RequestsScroll, scrolling)
	s.shown.Request = inViewAt(s.shown.Request, s.shown.RequestsScroll, scrolling)
	m.session = &s
	return m.requestInView()
}

// requestInView is m with its page's turns scrolled no further than keeps
// the turn chosen in view, as choosing has it, with the rows under it Kept
// keeps with it: of the turn open, those down to its request chosen.
func (m Model) requestInView() Model {
	s := *m.session
	s.shown = choosing(s.shown, turnAt(s.shown, s.shown.Turn), m.turnsScrolling(s.shown))
	m.session = &s
	return m
}

// scrollSession scrolls the session's page by, as j, k, PgDn, PgUp and the
// wheel over its turns do: the turn open's requests, where one is, as
// scrollRequests has it; else its turns, as scrollTurns has it.
func (m Model) scrollSession(by scrollStep) Model {
	if m.session.shown.Open != 0 {
		return m.scrollRequests(by)
	}
	return m.scrollTurns(by)
}

// requestsScrolling is how far the turn open's requests scroll, laid out at
// the size m draws the frame at.
func (m Model) requestsScrolling() dashboard.Scrolling {
	f := dashboard.Frame{Width: m.size.Width, Height: m.size.Height, Session: m.session.frame()}
	return f.RequestsScrolling()
}

// clickRequest acts on a click on the row of the turn open's request at
// index i: on the one chosen, it opens its details, as enter does; on
// another, it chooses it.
func (m Model) clickRequest(i int) (tea.Model, tea.Cmd) {
	if i == m.session.shown.Request {
		return m.openTurnCard()
	}
	return m.moveRequest(i - m.session.shown.Request), nil
}

// keptInView is scroll moved no further than keeps the rows from i to
// i+under in view of what scrolls as s says, none past its ends.
func keptInView(scroll, i, under int, s dashboard.Scrolling) int {
	if s.Page > 0 {
		scroll = min(max(scroll, i+under-s.Page+1), i)
	}
	return min(max(scroll, 0), s.Most)
}

// inViewAt is i moved no further than keeps it among the rows in view of
// what scrolls as s says, scrolled to scroll.
func inViewAt(i, scroll int, s dashboard.Scrolling) int {
	if s.Page > 0 {
		i = min(max(i, scroll), scroll+s.Page-1)
	}
	return i
}

// chosenRequest is the Log's line of the turn open's request chosen: false
// where no page shows one.
func (m Model) chosenRequest() (views.LogLine, bool) {
	if m.session == nil {
		return views.LogLine{}, false
	}
	shown := m.session.shown
	if shown.Open == 0 || shown.Request < 0 || shown.Request >= len(shown.Requests) {
		return views.LogLine{}, false
	}
	return shown.Requests[shown.Request], true
}

// openTurnCard opens the Log's details card of the turn open's request
// chosen, as enter does, over the page, the turn staying open behind it:
// nothing where none is chosen.
func (m Model) openTurnCard() (Model, tea.Cmd) {
	if _, ok := m.chosenRequest(); !ok {
		return m, nil
	}
	return m.placeTurnCard(), nil
}

// placeTurnCard has the card open over the session's page, unscrolled,
// placed by its request's row, as LogFitted has it, the page standing still
// behind it as it stands now, till a key steps the card on.
func (m Model) placeTurnCard() Model {
	s := *m.session
	s.card = turnCarding{}
	m.session = &s
	_, placed := m.frame(m.now()).LogFitted(m.turnCard())
	s.card.placed = placed.Placed
	return m.cover(turnCardOpen)
}

// turnCard is the Log's details card over the session's page, as the watch
// holds it: of its turn open's request chosen, as the page holds it now.
func (m Model) turnCard() dashboard.LogCard {
	line, ok := m.chosenRequest()
	if !ok {
		return dashboard.LogCard{}
	}
	details, _ := line.Details()
	return dashboard.LogCard{Request: &details, Scroll: m.session.card.scroll, Placed: m.session.card.placed}
}

// turnCardKey acts on a key while the Log's details card is open over a
// session's page, which takes every key but ctrl+c: ↑ and ↓ step to the
// request before or after in the turn open, its details in the card, the
// lit row following and the card placed afresh; esc closes the card, the
// turn left open at the request chosen; j and k scroll the card a row, and
// PgDn and PgUp a page, where it scrolls within; and the rest do nothing.
func (m Model) turnCardKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		return m.uncover()
	case "up":
		return m.moveRequest(-1).placeTurnCard(), nil
	case "down":
		return m.moveRequest(1).placeTurnCard(), nil
	}
	if by, ok := scrollKey(key); ok {
		f, _, _ := m.screen(m.now())
		return m.turnCardScrolled(by.from(m.session.card.scroll, f.CardScrolling())), nil
	}
	return m, nil
}

// turnCardScrolled is m with the details card over the session's page
// scrolled to scroll.
func (m Model) turnCardScrolled(scroll int) Model {
	s := *m.session
	s.card.scroll = scroll
	m.session = &s
	return m
}
