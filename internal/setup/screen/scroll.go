package screen

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// wheelRows is how many rows a turn of the wheel scrolls the page, as it
// scrolls the dashboard's.
const wheelRows = 3

// scrollStep is how far a key or a turn of the wheel scrolls, down where more
// than zero, up where less: rows, and pages, as many rows as show at once.
type scrollStep struct {
	rows, pages int
}

// scrollKey is how far the key scrolls, reporting false for a key that
// doesn't: j and k a row, PgDn and PgUp a page, as the dashboard's keys
// scroll.
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

// from is where the page, scrolled at and scrolling as s says, is scrolled
// to: its rows on, or its pages, as s pages them; no further either way than
// it goes as it stands now, which may be less than when it was last
// scrolled.
func (by scrollStep) from(at int, s dashboard.Scrolling) int {
	at = min(at, s.Most)
	if by.pages != 0 {
		return s.Paged(at, by.pages)
	}
	return min(max(at+by.rows, 0), s.Most)
}

// wheeled scrolls the page wheelRows rows the way the wheel turned.
func (m Model) wheeled(msg tea.MouseWheelMsg) Model {
	switch msg.Button {
	case tea.MouseWheelDown:
		return m.scrolled(scrollStep{rows: wheelRows})
	case tea.MouseWheelUp:
		return m.scrolled(scrollStep{rows: -wheelRows})
	}
	return m
}

// scrolled scrolls the page by, as far as it scrolls at the size drawn at:
// where none of it shows, as below setup's least, it's kept where it is, for
// the terminal made larger again.
func (m Model) scrolled(by scrollStep) Model {
	s := m.page().Scrolling(m.size.Width, m.size.Height)
	if s.Page == 0 {
		return m
	}
	m.scroll = by.from(m.scroll, s)
	return m
}
