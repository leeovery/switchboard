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
// the key, worded as they list it; whether it works where it is; whether the
// key line leaves it to help; and whether the key line sets it with p, ? and
// q, rather than among the page's own.
type keyListing struct {
	dashboard.Key
	works, helpful, withEnds bool
}

// listings are the page's own keys, as m stands, in the design's order: on
// the Overview, its toggles, each word faint where its section is hidden; b,
// its word saying how the windows are grouped; and with more than one
// account, ↑↓, which the key line sets with p, ? and q; w, in Runway,
// switching between the day and the week, its word saying which it shows;
// and j and k, PgUp and PgDn, and the wheel, which the key line leaves to
// help, where the view scrolls, and on the Overview always, as USAGE scrolls
// once its accounts outgrow it.
func (m Model) listings() []keyListing {
	overview := m.view == dashboard.Overview
	scrolls := overview || m.scrolling().Most > 0
	var listed []keyListing
	for _, s := range dashboard.EverySection() {
		listed = append(listed, keyListing{Key: dashboard.Key{Key: s.Key(), Does: s.Word(), Hidden: !m.sections.Has(s)}, works: overview})
	}
	return append(listed,
		keyListing{Key: dashboard.Key{Key: "b", Does: "by", State: string(layoutOf(m.by))}, works: overview},
		keyListing{Key: dashboard.Key{Key: "↑↓", Does: "focus"}, works: overview && len(m.doc.Accounts) > 1, withEnds: true},
		keyListing{Key: dashboard.Key{Key: "w", Does: "window: " + m.span.Name()}, works: m.view == dashboard.Runway},
		keyListing{Key: dashboard.Key{Key: "j k", Does: "scroll"}, works: scrolls, helpful: true},
		keyListing{Key: dashboard.Key{Key: "PgUp PgDn", Does: "page"}, works: scrolls, helpful: true},
		keyListing{Key: dashboard.Key{Key: "wheel", Does: "scroll"}, works: scrolls, helpful: true},
	)
}

// keyLine is the key line as m stands at now: while the theme picker is
// open, its keys alone, then q quit, a phone's line as a wide one; else tab
// views, then the page's own keys that work, those it leaves to help aside,
// then p pin where a pin can be set, and ? help and q quit last; and the
// note a key left in their place, while that's news. Most pages set p pin
// apart, a phone's line keeping the page's own keys between tab views and ?
// and q quit; the Overview sets ↑↓ focus, p pin, ? help and q quit as a
// group of their own, overviewApart after its toggles, which a phone's line
// leaves to help.
func (m Model) keyLine(now time.Time) dashboard.KeyLine {
	if m.picker.open {
		return dashboard.KeyLine{Groups: [][]dashboard.Key{m.picker.keys(), {quit}}, Lead: keysAfter, Note: m.noted(now)}
	}
	var page, ends []dashboard.Key
	for _, k := range m.listings() {
		switch {
		case !k.works || k.helpful:
		case k.withEnds:
			ends = append(ends, k.Key)
		default:
			page = append(page, k.Key)
		}
	}
	pin, pins := m.pinKey()
	l := dashboard.KeyLine{Groups: [][]dashboard.Key{{tabViews}}, Lead: 3, PhoneHelp: "keys", Note: m.noted(now)}
	if len(page) > 0 {
		l.Groups = append(l.Groups, page)
	}
	if m.view == dashboard.Overview {
		if pins {
			ends = append(ends, pin)
		}
		l.Lead, l.After, l.PhoneHelp = overviewApart, overviewApart, "keys and toggles"
		l.Groups = append(l.Groups, append(ends, helpKey, quit))
		return l
	}
	l.Phone = page
	if pins {
		l.Groups = append(l.Groups, []dashboard.Key{pin})
	}
	l.Groups = append(l.Groups, []dashboard.Key{helpKey, quit})
	return l
}

// tabViews, helpKey and quit are tab, ? and q, which show the next tab, open
// the page's help and quit, as the key line and help list them.
var (
	tabViews = dashboard.Key{Key: "tab", Does: "views"}
	helpKey  = dashboard.Key{Key: "?", Does: "help"}
	quit     = dashboard.Key{Key: "q", Does: "quit"}
)

// keysAfter is how many blank cells the key line sets a group of keys apart
// from the next by; and overviewApart how many the Overview's sets each of
// its groups apart by, tab views among them.
const (
	keysAfter     = 4
	overviewApart = 5
)

// helpKeys are the keys help lists, as m stands, worded as the key line
// words them: tab; every key of the page's own that works, those the key
// line leaves to help among them; p, where a pin can be set; t, in colour;
// and R, ? and q.
func (m Model) helpKeys() []dashboard.Key {
	keys := []dashboard.Key{tabViews}
	for _, k := range m.listings() {
		if k.works {
			keys = append(keys, k.Key)
		}
	}
	if pin, ok := m.pinKey(); ok {
		keys = append(keys, pin)
	}
	if m.coloured() {
		keys = append(keys, dashboard.Key{Key: "t", Does: "themes"})
	}
	return append(keys, dashboard.Key{Key: "R", Does: "refresh"}, helpKey, quit)
}
