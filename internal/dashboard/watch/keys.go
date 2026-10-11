package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// pageKey acts on a key of the page, as pressed hands it on: j, k, PgDn and
// PgUp scroll the view where it doesn't fit; the arrows, space and s act on
// the Overview's cards, as cardKey says; tab and shift-tab show the next tab
// and the one before; t opens the theme picker; w has the cards feature the
// next window, or in Runway, switches between the day and the week; and g
// has the cards draw their charts in the next style. The rest do nothing:
// the number keys and m among them, which act in the routing card alone.
func (m Model) pageKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := key.String()
	if rows, ok := m.scrollKey(k); ok {
		return m.scrollBy(rows), nil
	}
	if next, ok := m.cardKey(k); ok {
		return next, nil
	}
	switch k {
	case "tab":
		return m.turn(1)
	case "shift+tab":
		return m.turn(-1)
	case "t":
		return m.openPicker()
	case "w":
		return m.nextWindow()
	case "g":
		return m.nextChart(), nil
	}
	return m, nil
}

// keyListing is a key of the page's own, as the key line and help list it:
// the key, its word, whether it works where it is, and whether the key line
// leaves it to help.
type keyListing struct {
	key, word      string
	works, helpful bool
}

// listings are the page's own keys, as m stands, in the design's order: w,
// as wKey says; on the Overview, the arrows and space, s, and g; and j and
// k, PgUp and PgDn, and the wheel, where the view scrolls, which the key
// line leaves to help, as it does s.
func (m Model) listings() []keyListing {
	cards, scrolls := m.view == dashboard.Overview, m.scrolling().Most > 0
	return []keyListing{
		m.wKey(),
		{key: "←→", word: "focus", works: cards},
		{key: "space", word: "flip", works: cards},
		{key: "s", word: "flip all", works: cards, helpful: true},
		{key: "g", word: "chart", works: cards},
		{key: "j k", word: "scroll", works: scrolls, helpful: true},
		{key: "PgUp PgDn", word: "page", works: scrolls, helpful: true},
		{key: "wheel", word: "scroll", works: scrolls, helpful: true},
	}
}

// wKey is w as the tab shown has it: in Runway, switching between the day
// and the week, its word saying which it shows; on the Overview, once a
// document has been read, as it does nothing before, cycling the window
// every card features, its word saying which; and elsewhere, nothing.
func (m Model) wKey() keyListing {
	if m.view == dashboard.Runway {
		return keyListing{key: "w", word: "window: " + m.span.Name(), works: true}
	}
	window := m.featured.Name(m.doc, m.now(), m.cfg.Policy)
	return keyListing{key: "w", word: "window: " + window, works: m.view == dashboard.Overview && !m.updated.IsZero()}
}

// keyLine is the key line as m stands at now: while the theme picker is
// open, its keys alone; else tab views, then the page's own keys that work,
// those it leaves to help aside, then p pin where a pin can be set, and ?
// help and q quit last; and the note a key left in their place, while
// that's news.
func (m Model) keyLine(now time.Time) dashboard.KeyLine {
	l := dashboard.KeyLine{Lead: 3, PhoneHelp: "keys", Note: m.noted(now)}
	if m.picker.open {
		l.Groups = [][]dashboard.Key{m.picker.keys()}
		return l
	}
	if m.view == dashboard.Overview {
		l.Lead = 5
	}
	l.Groups = [][]dashboard.Key{{{Key: "tab", Does: "views"}}}
	var page []dashboard.Key
	for _, k := range m.listings() {
		if k.works && !k.helpful {
			page = append(page, dashboard.Key{Key: k.key, Does: k.word})
		}
	}
	if len(page) > 0 {
		l.Groups = append(l.Groups, page)
	}
	if pin, ok := m.pinKey(); ok {
		l.Groups = append(l.Groups, []dashboard.Key{pin})
	}
	l.Groups = append(l.Groups, []dashboard.Key{{Key: "?", Does: "help"}, {Key: "q", Does: "quit"}})
	return l
}

// helpKeys are the keys help lists, as m stands, worded as the key line
// words them: tab; every key of the page's own that works, those the key
// line leaves to help among them; p, where a pin can be set; t, in colour;
// and R, ? and q.
func (m Model) helpKeys() []dashboard.Key {
	keys := []dashboard.Key{{Key: "tab", Does: "views"}}
	for _, k := range m.listings() {
		if k.works {
			keys = append(keys, dashboard.Key{Key: k.key, Does: k.word})
		}
	}
	if pin, ok := m.pinKey(); ok {
		keys = append(keys, pin)
	}
	if m.coloured() {
		keys = append(keys, dashboard.Key{Key: "t", Does: "themes"})
	}
	return append(keys, dashboard.Key{Key: "R", Does: "refresh"}, dashboard.Key{Key: "?", Does: "help"}, dashboard.Key{Key: "q", Does: "quit"})
}
