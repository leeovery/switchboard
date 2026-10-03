package watch

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// wheelRows is how many rows a turn of the wheel scrolls the cards.
const wheelRows = 3

// scrollKey is how many rows the key scrolls the cards, down where more than
// zero, up where less, reporting false for a key that doesn't scroll them: j
// and k a row, PgDn and PgUp a page, as many rows as show at once.
func (m Model) scrollKey(key string) (int, bool) {
	switch key {
	case "j", "J":
		return 1, true
	case "k", "K":
		return -1, true
	case "pgdown":
		return m.scrolling().Page, true
	case "pgup":
		return -m.scrolling().Page, true
	}
	return 0, false
}

// wheeled scrolls the cards as the wheel turns, wheelRows a turn, but while
// the theme picker or the help is open over them.
func (m Model) wheeled(msg tea.MouseWheelMsg) Model {
	if m.picker.open || m.helping {
		return m
	}
	switch msg.Button {
	case tea.MouseWheelDown:
		return m.scrollBy(wheelRows)
	case tea.MouseWheelUp:
		return m.scrollBy(-wheelRows)
	}
	return m
}

// scrollBy scrolls the cards by rows, down where more than zero, up where
// less, no further either way than they go as the screen stands now, which
// may be less than when they were last scrolled.
func (m Model) scrollBy(rows int) Model {
	most := m.scrolling().Most
	m.scroll = min(max(min(m.scroll, most)+rows, 0), most)
	return m
}

// scrolling is how far the cards on screen scroll.
func (m Model) scrolling() dashboard.Scrolling {
	now := m.now()
	return m.frame(now).Scrolling(m.shown(now), now)
}
