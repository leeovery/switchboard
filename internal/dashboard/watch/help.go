package watch

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// helping is help while it's open over the page: how many rows it's scrolled
// down by.
type helping struct {
	scroll int
}

// openHelp opens the page's help over it, as ? does, from its top.
func (m Model) openHelp() Model {
	m.help = helping{}
	return m.cover(helpOpen)
}

// helpKey acts on a key while help is open, which takes every key but
// ctrl+c: ? and esc close it; j and k scroll it a row, and PgDn and PgUp a
// page; and the rest do nothing.
func (m Model) helpKey(key string) (tea.Model, tea.Cmd) {
	if key == "?" || key == "esc" {
		return m.uncover()
	}
	if by, ok := scrollKey(key); ok {
		return m.scrollHelp(by), nil
	}
	return m, nil
}

// scrollHelp scrolls help by, within the card it's drawn in on screen.
func (m Model) scrollHelp(by scrollStep) Model {
	f, _, _ := m.screen(m.now())
	m.help.scroll = by.from(m.help.scroll, f.CardScrolling())
	return m
}

// helpCard is the page's help as m stands: its keys, and how far it's
// scrolled.
func (m Model) helpCard() dashboard.Help {
	return dashboard.Help{Keys: m.helpKeys(), Scroll: m.help.scroll}
}
