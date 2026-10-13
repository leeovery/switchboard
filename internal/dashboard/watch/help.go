package watch

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// helping is help while it's open over the page: how many rows it's
// scrolled down by; and over, what lists the keys of the page as it stood
// when help opened.
type helping struct {
	scroll int
	over   pageKeys
}

// openHelp opens the page's help over it, as ? does, from its top, listing
// the keys of the page it stands still over, so a document that lands while
// it's open moves nothing in it.
func (m Model) openHelp() Model {
	m = m.cover(helpOpen)
	m.help = helping{over: m.keys()}
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
	return dashboard.Help{Keys: m.helpKeys(), Scroll: m.help.scroll}
}

// sessionKeys is what lists a session's page's keys, as m stands, in the
// design's order: esc, back where the page was opened from, or with a turn
// open, closing it; ↑↓ through its turns, or the open turn's requests; enter
// opening the turn chosen into its requests, or the request chosen; and p,
// as pinKey has it. Its scroll keys follow, where its turns scroll, as
// listings has them.
func (m Model) sessionKeys() pageKeys {
	esc, move, enter := dashboard.Key{Key: "esc", Does: "back"}, dashboard.Key{Key: "↑↓", Does: "turn"}, dashboard.Key{Key: "enter", Does: "its requests"}
	if m.session.shown.Open != 0 {
		esc.Does, move.Does, enter.Does = "close", "request", "open"
	}
	pin, pinned := m.pinKey()
	return pageKeys{
		own:      []keyListing{{Key: esc, works: true, apart: true}, {Key: move, works: true, apart: true}, {Key: enter, works: true}},
		pin:      pin,
		pinned:   pinned,
		coloured: m.coloured(),
	}
}

// helpKeys are the keys help lists, those of the page it stands still over,
// drawn at the size drawn at now, which says whether the page scrolls.
func (m Model) helpKeys() []dashboard.Key {
	f := m.page.frame
	f.Width, f.Height = m.size.Width, m.size.Height
	return m.help.over.helpKeys(f.Scrolling(m.page.doc, m.page.at).Most > 0)
}
