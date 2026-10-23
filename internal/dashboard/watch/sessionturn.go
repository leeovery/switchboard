package watch

import (
	"time"

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

// turnOpened is a turn's open as the watch knows it from another: the open
// of the page it opened on, as sessionShown's asked has it, which no page
// opened since shares, and how many turns that page had opened, it the
// last.
type turnOpened struct {
	asked, turns int
}

// opened is the open of the turn s has open last.
func (s *sessionShown) opened() turnOpened {
	return turnOpened{asked: s.asked, turns: s.turns}
}

// turnOpen reports whether the page open has the turn of the open given
// open still, as the last it opened.
func (m Model) turnOpen(o turnOpened) bool {
	return m.session != nil && m.session.opened() == o
}

// turnReadMsg is a read of the lines of the requests of the turn of the
// open given, as views.HeldPage's ReadTurn reads them.
type turnReadMsg struct {
	opened turnOpened
	turn   views.HeldTurn
}

// turnLoadingMsg is the turn of the open given, having waited LoadingAfter
// on its read.
type turnLoadingMsg struct {
	opened turnOpened
}

// openTurn opens the turn chosen into its requests, in place, as enter does,
// its first chosen, as many shown at a time as RequestsFit has it at the
// size m draws at, and reads the lines of its requests, as readTurn reads
// them: once it has waited LoadingAfter on them, it shows they're loading,
// as turnLoading has it; once they're read, relived builds their rows, and
// keeps them in view as choosing has it. Nothing opens where no turn is
// chosen.
func (m Model) openTurn() (Model, tea.Cmd) {
	s := *m.session
	if s.shown.Turn == 0 {
		return m, nil
	}
	s.shown.Open, s.shown.Request, s.shown.RequestsScroll = s.shown.Turn, 0, 0
	s.shown.RequestsRows = m.pageFrame(s.shown).RequestsFit()
	s.held, s.turns, s.turnWaited = s.held.Opening(s.shown.Turn, m.pageLive(s.id, m.now())), s.turns+1, false
	m.session = &s
	return m, tea.Batch(m.readTurn(), m.after(LoadingAfter, turnLoadingMsg{opened: s.opened()}))
}

// turnLoading is m with the turn open shown loading, its read under way
// LoadingAfter after it opened: not where the page it opened on is closed,
// or another page or turn opened since, as turnOpen has it; and, once its
// read has landed, loading no longer, as relived builds it.
func (m Model) turnLoading(msg turnLoadingMsg) Model {
	if !m.turnOpen(msg.opened) {
		return m
	}
	s := *m.session
	s.turnWaited, s.built = true, nil
	m.session = &s
	return m
}

// readTurn reads, as a command, the lines of the requests of the turn open
// on the session's page, through the ledger, as the page is built at now,
// as views.HeldPage's ReadTurn reads them: none, where the watch reads no
// ledger.
func (m Model) readTurn() tea.Cmd {
	s := m.session
	held, live, opened := s.held, m.pageLive(s.id, m.now()), s.opened()
	var l views.LedgerSession
	if m.cfg.Ledger != nil {
		l = m.cfg.Ledger
	}
	return func() tea.Msg {
		return turnReadMsg{opened: opened, turn: held.ReadTurn(l, live)}
	}
}

// turnRead takes in the read of a turn's lines, where the page it was read
// for is open still, and the turn it opened last, as turnOpen has it: the
// page holds them, as views.HeldPage's Turned has it, and builds their rows
// afresh.
func (m Model) turnRead(msg turnReadMsg) Model {
	if !m.turnOpen(msg.opened) {
		return m
	}
	s := *m.session
	s.held, s.taken = s.held.Turned(msg.turn), s.taken+1
	m.session = &s
	return m
}

// closeTurn closes the turn open, as esc does, the turn staying chosen, the
// page holding the lines of its requests no longer.
func (m Model) closeTurn() Model {
	s := *m.session
	s.shown.Open, s.shown.Requests, s.shown.Request, s.shown.RequestsScroll, s.shown.RequestsLoading, s.shown.RequestsRows = 0, nil, 0, 0, false, 0
	s.held, s.turnWaited = s.held.Closing(), false
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
	s.shown.Request, s.shown.RequestsScroll = i, scrolling.Keeping(s.shown.RequestsScroll, i, 0)
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
// the turn chosen in view, as choosing has it, with the rows under it
// TurnsKept keeps with it: of the turn open, those through its foot.
func (m Model) requestInView() Model {
	s := *m.session
	s.shown = choosing(s.shown, turnAt(s.shown, s.shown.Turn), m.turnsView(s.shown))
	m.session = &s
	return m
}

// sessionResized is m with the turn open on its page, where it has one,
// showing as many requests at a time as RequestsFit has it at the size m
// now draws at, its request chosen kept in view, as are the turn open and
// its foot among its turns.
func (m Model) sessionResized() Model {
	if m.session == nil || m.session.shown.Open == 0 {
		return m
	}
	s := *m.session
	s.shown.RequestsRows = m.pageFrame(s.shown).RequestsFit()
	s.shown.RequestsScroll = m.pageFrame(s.shown).RequestsScrolling().Keeping(s.shown.RequestsScroll, s.shown.Request, 0)
	m.session = &s
	return m.requestInView()
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

// turnCardAt is the Log's details card over the session's page as it's drawn
// at now: a request in flight's as the stream the page's rows are drawn by
// tells it then, as the Log's card is, while the page behind stands still.
func (m Model) turnCardAt(now time.Time) dashboard.LogCard {
	c := m.turnCard()
	c.Stream = m.traffic.stream(now)
	return c
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
