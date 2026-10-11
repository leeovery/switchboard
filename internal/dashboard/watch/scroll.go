package watch

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// wheelRows is how many rows a turn of the wheel scrolls.
const wheelRows = 3

// scrollStep is how far a key or a turn of the wheel scrolls, down where more
// than zero, up where less: rows, and pages, as many rows as show at once.
type scrollStep struct {
	rows, pages int
}

// scrollKey is how far the key scrolls, reporting false for a key that
// doesn't: j and k a row, PgDn and PgUp a page.
func scrollKey(key string) (scrollStep, bool) {
	switch key {
	case "j":
		return scrollStep{rows: 1}, true
	case "k":
		return scrollStep{rows: -1}, true
	case "pgdown":
		return scrollStep{pages: 1}, true
	case "pgup":
		return scrollStep{pages: -1}, true
	}
	return scrollStep{}, false
}

// wheelTurn is how far the wheel turned scrolls, reporting false for a
// button that isn't the wheel's.
func wheelTurn(button tea.MouseButton) (scrollStep, bool) {
	switch button {
	case tea.MouseWheelDown:
		return scrollStep{rows: wheelRows}, true
	case tea.MouseWheelUp:
		return scrollStep{rows: -wheelRows}, true
	}
	return scrollStep{}, false
}

// from is where what's scrolled at, and scrolls as s says, is scrolled to:
// no further either way than it goes as it stands now, which may be less
// than when it was last scrolled.
func (by scrollStep) from(at int, s dashboard.Scrolling) int {
	return min(max(min(at, s.Most)+by.rows+by.pages*s.Page, 0), s.Most)
}

// wheeled scrolls what the wheel turns under the pointer, as the screen
// draws it: help, over its card; the view, over its rows, while no card is
// open; and nothing else, the routing card among it, nor anything while the
// theme picker is open.
func (m Model) wheeled(msg tea.MouseWheelMsg) Model {
	by, ok := wheelTurn(msg.Button)
	if !ok || m.picker.open {
		return m
	}
	f, _, _ := m.screen(m.now())
	switch under := f.Under(msg.X, msg.Y); {
	case under == dashboard.UnderCard && m.open == helpOpen:
		return m.scrollHelp(by)
	case under == dashboard.UnderView:
		return m.scrollView(by)
	}
	return m
}

// scrollView scrolls the view shown by.
func (m Model) scrollView(by scrollStep) Model {
	m.scroll = by.from(m.scroll, m.scrolling())
	return m
}

// scrolling is how far the view on screen scrolls.
func (m Model) scrolling() dashboard.Scrolling {
	now := m.now()
	return m.frame(now).Scrolling(m.doc, now)
}
