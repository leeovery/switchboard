package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// pageKey acts on a key of the page, as pressed hands it on: j, k, PgDn and
// PgUp scroll the view where it doesn't fit; on the Overview, u, g, r, s, c
// and l show and hide its sections, b switches how its windows are grouped,
// and ↑ and ↓ move the focus, as overviewKey says; tab and shift-tab show the
// next tab and the one before; t opens the theme picker; and w, in Runway,
// switches between the day and the week. The rest do nothing: the number
// keys and m among them, which act in the routing card alone.
func (m Model) pageKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := key.String()
	if by, ok := scrollKey(k); ok {
		return m.scrollView(by), nil
	}
	if next, ok := m.overviewKey(k); ok {
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
		return m.nextSpan(), nil
	}
	return m, nil
}

// overviewKey acts on a key of the Overview's, reporting false for a key it
// doesn't take, or on another tab: a section's key shows or hides it, as
// toggle has it; b switches how its windows are grouped, as regroup has it;
// and ↑ and ↓ move the focus, as moveFocus has it.
func (m Model) overviewKey(key string) (Model, bool) {
	if m.view != dashboard.Overview {
		return m, false
	}
	if section, ok := dashboard.SectionKeyed(key); ok {
		return m.toggle(section), true
	}
	switch key {
	case "b":
		return m.regroup(), true
	case "up":
		return m.moveFocus(-1), true
	case "down":
		return m.moveFocus(1), true
	}
	return m, false
}

// keyListing is a key of the page's own, as the key line and help list it:
// the key, its word, whether it works where it is, and whether the key line
// leaves it to help.
type keyListing struct {
	key, word      string
	works, helpful bool
}

// listings are the page's own keys, as m stands, in the design's order: w,
// in Runway, switching between the day and the week, its word saying which
// it shows; and j and k, PgUp and PgDn, and the wheel, where the view
// scrolls, which the key line leaves to help.
func (m Model) listings() []keyListing {
	scrolls := m.scrolling().Most > 0
	return []keyListing{
		{key: "w", word: "window: " + m.span.Name(), works: m.view == dashboard.Runway},
		{key: "j k", word: "scroll", works: scrolls, helpful: true},
		{key: "PgUp PgDn", word: "page", works: scrolls, helpful: true},
		{key: "wheel", word: "scroll", works: scrolls, helpful: true},
	}
}

// keyLine is the key line as m stands at now: while the theme picker is
// open, its keys alone, then q quit, a phone's line as a wide one; else tab
// views, then the page's own keys that work, those it leaves to help aside,
// then p pin where a pin can be set, and ? help and q quit last, a phone's
// line keeping the page's own keys between them; and the note a key left in
// their place, while that's news.
func (m Model) keyLine(now time.Time) dashboard.KeyLine {
	if m.picker.open {
		return dashboard.KeyLine{Groups: [][]dashboard.Key{m.picker.keys(), {quit}}, Lead: keysAfter, Note: m.noted(now)}
	}
	l := dashboard.KeyLine{Lead: 3, PhoneHelp: "keys", Note: m.noted(now)}
	if m.view == dashboard.Overview {
		l.Lead, l.PhoneHelp = 5, "keys and toggles"
	}
	l.Groups = [][]dashboard.Key{{{Key: "tab", Does: "views"}}}
	var page []dashboard.Key
	for _, k := range m.listings() {
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
	l.Groups = append(l.Groups, []dashboard.Key{{Key: "?", Does: "help"}, quit})
	return l
}

// quit is q, which quits, as the key line and help list it.
var quit = dashboard.Key{Key: "q", Does: "quit"}

// keysAfter is how many blank cells the key line sets a group of keys apart
// from the next by.
const keysAfter = 4

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
	return append(keys, dashboard.Key{Key: "R", Does: "refresh"}, dashboard.Key{Key: "?", Does: "help"}, quit)
}
