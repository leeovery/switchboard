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
	if by, ok := scrollKey(k); ok {
		return m.scrollView(by), nil
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

// pageKeys is what lists the page's keys, as it stands, but for whether its
// view scrolls, which follows the size it's drawn at: w, as wKey has it;
// whether the Overview's cards are shown; p, and whether it's listed, as
// pinKey has it; and whether the dashboard is drawn in colour.
type pageKeys struct {
	w        keyListing
	cards    bool
	pin      dashboard.Key
	pinned   bool
	coloured bool
}

// keysAt is what lists the page's keys as m stands at now.
func (m Model) keysAt(now time.Time) pageKeys {
	pin, pinned := m.pinKey()
	return pageKeys{w: m.wKey(now), cards: m.view == dashboard.Overview, pin: pin, pinned: pinned, coloured: m.coloured()}
}

// listings are the page's own keys, as m stands at now, its page drawn as f,
// as pageKeys' listings has them.
func (m Model) listings(f dashboard.Frame, now time.Time) []keyListing {
	return m.keysAt(now).listings(f.Scrolling(m.doc, now).Most > 0)
}

// listings are the page's own keys, where its view scrolls or not, in the
// design's order: w; on the Overview, the arrows and space, s, and g; and j
// and k, PgUp and PgDn, and the wheel, where the view scrolls, which the key
// line leaves to help, as it does s.
func (p pageKeys) listings(scrolls bool) []keyListing {
	return []keyListing{
		p.w,
		{key: "←→", word: "focus", works: p.cards},
		{key: "space", word: "flip", works: p.cards},
		{key: "s", word: "flip all", works: p.cards, helpful: true},
		{key: "g", word: "chart", works: p.cards},
		{key: "j k", word: "scroll", works: scrolls, helpful: true},
		{key: "PgUp PgDn", word: "page", works: scrolls, helpful: true},
		{key: "wheel", word: "scroll", works: scrolls, helpful: true},
	}
}

// wKey is w as the tab shown has it at now: in Runway, switching between the
// day and the week, its word saying which it shows; on the Overview, once a
// document has been read, as it does nothing before, cycling the window
// every card features, its word saying which; and elsewhere, nothing.
func (m Model) wKey(now time.Time) keyListing {
	if m.view == dashboard.Runway {
		return keyListing{key: "w", word: "window: " + m.span.Name(), works: true}
	}
	window := m.featured.Name(m.doc, now, m.cfg.Policy)
	return keyListing{key: "w", word: "window: " + window, works: m.view == dashboard.Overview && !m.updated.IsZero()}
}

// keyLine is the key line as m stands at now, under its page drawn as f:
// while the theme picker is open, its keys alone, then q quit, a phone's line
// as a wide one; else tab views, then the page's own keys that work, those it
// leaves to help aside, then p pin where a pin can be set, and ? help and q
// quit last, a phone's line keeping the page's own keys between them; and the
// note a key left in their place, while that's news.
func (m Model) keyLine(f dashboard.Frame, now time.Time) dashboard.KeyLine {
	if m.picker.open {
		return dashboard.KeyLine{Groups: [][]dashboard.Key{m.picker.keys(), {dashboard.Quit}}, Lead: dashboard.KeysAfter, Note: m.noted(now)}
	}
	l := dashboard.KeyLine{Lead: 3, PhoneHelp: "keys", Note: m.noted(now)}
	if m.view == dashboard.Overview {
		l.Lead, l.PhoneHelp = 5, "keys and toggles"
	}
	l.Groups = [][]dashboard.Key{{{Key: "tab", Does: "views"}}}
	var page []dashboard.Key
	for _, k := range m.listings(f, now) {
		if k.works && !k.helpful {
			page = append(page, dashboard.Key{Key: k.key, Does: k.word})
		}
	}
	if len(page) > 0 {
		l.Groups, l.Phone = append(l.Groups, page), page
	}
	if pin, ok := m.pinKey(); ok {
		l.Groups = append(l.Groups, []dashboard.Key{pin})
	}
	l.Groups = append(l.Groups, []dashboard.Key{{Key: "?", Does: "help"}, dashboard.Quit})
	return l
}

// helpKeys are the keys help lists, where the page's view scrolls or not,
// worded as the key line words them: tab; every key of the page's own that
// works, those the key line leaves to help among them; p, where a pin can be
// set; t, in colour; and R, ? and q.
func (p pageKeys) helpKeys(scrolls bool) []dashboard.Key {
	keys := []dashboard.Key{{Key: "tab", Does: "views"}}
	for _, k := range p.listings(scrolls) {
		if k.works {
			keys = append(keys, dashboard.Key{Key: k.key, Does: k.word})
		}
	}
	if p.pinned {
		keys = append(keys, p.pin)
	}
	if p.coloured {
		keys = append(keys, dashboard.Key{Key: "t", Does: "themes"})
	}
	return append(keys, dashboard.Key{Key: "R", Does: "refresh"}, dashboard.Key{Key: "?", Does: "help"}, dashboard.Quit)
}
