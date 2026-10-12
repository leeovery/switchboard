package watch

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// helping is help while it's open over the page: the keys it lists, those of
// the page as it stood when help opened, at the size it's drawn at now; how
// many rows it's scrolled down by; and of, which lists the keys of that page
// drawn as the frame given.
type helping struct {
	keys   []dashboard.Key
	scroll int
	of     func(f dashboard.Frame) []dashboard.Key
}

// openHelp opens the page's help over it, as ? does, from its top, listing
// the keys of the page it stands still over, so a document that lands while
// it's open moves nothing in it.
func (m Model) openHelp() Model {
	m = m.cover(helpOpen)
	still, at := m, m.page.at
	m.help = helping{of: func(f dashboard.Frame) []dashboard.Key { return still.helpKeys(f, at) }}
	return m.relistHelp()
}

// relistHelp lists help's keys of the page it stands still over, drawn at
// the size drawn at, which says whether the page scrolls.
func (m Model) relistHelp() Model {
	f := m.page.frame
	f.Width, f.Height = m.size.Width, m.size.Height
	m.help.keys = m.help.of(f)
	return m
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

// helpCard is the page's help: its keys, and how far it's scrolled.
func (m Model) helpCard() dashboard.Help {
	return dashboard.Help{Keys: m.help.keys, Scroll: m.help.scroll}
}
