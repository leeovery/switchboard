package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

// wheelRows is how many rows a turn of the wheel scrolls help, and a view
// that scrolls by rows.
const wheelRows = dashboard.WheelRows

// scrollStep is how far a key or a turn of the wheel scrolls, down where more
// than zero, up where less: rows, and pages, as many rows as show at once.
type scrollStep struct {
	rows, pages int
}

// scrollKey is how far the key scrolls, as dashboard.ScrollKey has it.
func scrollKey(key string) (scrollStep, bool) {
	rows, pages, ok := dashboard.ScrollKey(key)
	return scrollStep{rows: rows, pages: pages}, ok
}

// wheelTurn is the way the wheel turned, 1 down and -1 up, reporting false
// for a button that isn't the wheel's.
func wheelTurn(button tea.MouseButton) (int, bool) {
	switch button {
	case tea.MouseWheelDown:
		return 1, true
	case tea.MouseWheelUp:
		return -1, true
	}
	return 0, false
}

// from is where what's scrolled at, and scrolls as s says, is scrolled to,
// as Scrolling.ScrolledTo has it.
func (by scrollStep) from(at int, s dashboard.Scrolling) int {
	return s.ScrolledTo(at, by.rows, by.pages)
}

// wheeled scrolls what the wheel turns under the pointer, as the screen
// draws it: help, or the Log's details, over the Log or a session's page,
// over its card; the view, over what of it scrolls, while no card is open,
// as far as notch says, as scrollView scrolls it, as on j and k; and nothing
// else, the routing card among it, nor anything while the theme picker is
// open. A turn scrolls a card wheelRows rows. What's under the pointer with
// no card open is found from the page on screen alone, as onScreen has it,
// which the key line doesn't move.
func (m Model) wheeled(msg tea.MouseWheelMsg) Model {
	way, ok := wheelTurn(msg.Button)
	if !ok || m.picker.open {
		return m
	}
	switch m.open {
	case helpOpen:
		m.help.scroll = m.wheeledCard(m.help.scroll, way, msg)
	case logCardOpen:
		m.log.card.scroll = m.wheeledCard(m.log.card.scroll, way, msg)
	case turnCardOpen:
		return m.turnCardScrolled(m.wheeledCard(m.session.card.scroll, way, msg))
	case nothingOpen:
		if f, at := m.onScreen(); f.Under(m.doc, at, msg.X, msg.Y) == dashboard.UnderView {
			return m.scrollView(notch(f, m.doc, at, way))
		}
	}
	return m
}

// wheeledCard is how far the card open is scrolled, from scroll, as the
// wheel turns the way given at the pointer: wheelRows rows, where it's over
// the card.
func (m Model) wheeledCard(scroll, way int, msg tea.MouseWheelMsg) int {
	if f, doc, at := m.screen(m.now()); f.Under(doc, at, msg.X, msg.Y) == dashboard.UnderCard {
		return scrollStep{rows: way * wheelRows}.from(scroll, f.CardScrolling())
	}
	return scroll
}

// notch is how far a turn of the wheel the way given scrolls the view f
// shows of doc at at: wheelRows rows, where it scrolls a row at a time; else
// a thing, as j and k do, as History's Weeks scroll an account at a time,
// and its Tokens a period.
func notch(f dashboard.Frame, doc status.Document, at time.Time, way int) scrollStep {
	if f.ScrollsRows(doc, at) {
		return scrollStep{rows: way * wheelRows}
	}
	return scrollStep{rows: way}
}

// scrollView scrolls the view shown by, the page acting as acting has it
// only where that moves it from where it's drawn: a scroll that moves
// nothing leaves the Overview's layout as it's kept, but the view scrolled
// no further than it's drawn, so an offset past where it now goes doesn't
// come back once it goes further again. On a session's page, it scrolls its
// turns, or its turn open's requests, as scrollSession has it; and the Log's
// table scrolls as scrollLog has it.
func (m Model) scrollView(by scrollStep) Model {
	switch m.shownView() {
	case dashboard.SessionPage:
		return m.scrollSession(by)
	case dashboard.Log:
		return m.scrollLog(by)
	}
	s := m.scrolling()
	to := by.from(m.scroll, s)
	if to == min(m.scroll, s.Most) {
		m.scroll = to
		return m
	}
	m = m.acting()
	m.scroll = to
	return m
}

// scrolling is how far the view on screen scrolls, as onScreen has it.
func (m Model) scrolling() dashboard.Scrolling {
	f, at := m.onScreen()
	return f.Scrolling(m.doc, at)
}

// onScreen is the page on screen, but for its key line, and the moment it's
// of: the page the last message left shown, its sections as Update built
// them then, where it left one; else, as on a screen blank till the terminal
// says what its background is, the page as m stands at the clock's time.
func (m Model) onScreen() (dashboard.Frame, time.Time) {
	if m.shown.page != nil {
		return *m.shown.page, m.shown.at
	}
	now := m.now()
	return m.frame(now), now
}
