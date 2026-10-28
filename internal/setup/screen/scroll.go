package screen

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// wheeled scrolls the page dashboard.WheelRows rows the way the wheel
// turned.
func (m Model) wheeled(msg tea.MouseWheelMsg) Model {
	switch msg.Button {
	case tea.MouseWheelDown:
		return m.scrolled(dashboard.WheelRows, 0)
	case tea.MouseWheelUp:
		return m.scrolled(-dashboard.WheelRows, 0)
	}
	return m
}

// scrolled scrolls the page rows on, or pages on, as far as it scrolls at
// the size drawn at, as Scrolling.ScrolledTo has it: where none of it
// shows, as below setup's least, it's kept where it is, for the terminal
// made larger again.
func (m Model) scrolled(rows, pages int) Model {
	s := m.page().Scrolling(m.size.Width, m.size.Height)
	if s.Page == 0 {
		return m
	}
	m.scroll = s.ScrolledTo(m.scroll, rows, pages)
	return m
}
