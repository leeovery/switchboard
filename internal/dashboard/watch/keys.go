package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// pageKey acts on a key of the page, as pressed hands it on: j, k, PgDn and
// PgUp scroll the view where it doesn't fit, as scrollView has it; and the
// rest each as acting has the page act: on the Overview, u, g, r, s, c and l
// show and hide its sections, b switches how its windows are grouped, and ↑
// and ↓ move the focus, as overviewKey says; on History, [ ], < >, v and ←
// → act as historyKey says; tab and shift-tab show the next tab and the one
// before; t opens the theme picker; and w, in Runway, switches between the
// day and the week. The rest do nothing: the number keys and m among them,
// which act in the routing card alone.
func (m Model) pageKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := key.String()
	if by, ok := scrollKey(k); ok {
		return m.scrollView(by), nil
	}
	afresh := m.acting()
	if next, ok := afresh.overviewKey(k); ok {
		return next, nil
	}
	if next, ok := afresh.historyKey(k); ok {
		return next, nil
	}
	switch k {
	case "tab":
		return afresh.turn(1)
	case "shift+tab":
		return afresh.turn(-1)
	case "t":
		return afresh.openPicker()
	case "w":
		if m.view == dashboard.Runway {
			return afresh.nextSpan(), nil
		}
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
// key line leaves it to help; whether the key line sets it with p, ? and q,
// rather than among the page's own; whether it sets it apart from the keys
// before it, in a group of its own; and whether a phone's line leaves it to
// help, keeping it to a line wider than a phone's.
type keyListing struct {
	dashboard.Key
	works, helpful, withEnds, apart, wide bool
}

// listings are the page's own keys, as m stands at now, its page drawn as f,
// in the design's order: on the Overview, its toggles, each word faint where
// its section is hidden; b, its word saying how the windows are grouped; and
// with more than one account, ↑↓, which the key line sets with p, ? and q;
// w, in Runway, switching between the day and the week, its word saying
// which it shows; History's, as historyListings has them; and j and k, PgUp
// and PgDn, and the wheel, which the key line leaves to help, where the view
// scrolls, and on the Overview always, as USAGE scrolls once its accounts
// outgrow it.
func (m Model) listings(f dashboard.Frame, now time.Time) []keyListing {
	overview := m.view == dashboard.Overview
	scrolls := overview || f.Scrolling(m.doc, now).Most > 0
	var listed []keyListing
	for _, s := range dashboard.EverySection() {
		listed = append(listed, keyListing{Key: dashboard.Key{Key: s.Key(), Does: s.Word(), Hidden: !m.sections.Has(s)}, works: overview})
	}
	listed = append(listed,
		keyListing{Key: dashboard.Key{Key: "b", Does: "by", State: string(layoutOf(m.by))}, works: overview},
		keyListing{Key: dashboard.Key{Key: "↑↓", Does: "focus"}, works: overview && len(m.doc.Accounts) > 1, withEnds: true},
		keyListing{Key: dashboard.Key{Key: "w", Does: "window: " + m.span.Name()}, works: m.view == dashboard.Runway},
	)
	return append(append(listed, m.historyListings()...),
		keyListing{Key: dashboard.Key{Key: "j k", Does: "scroll"}, works: scrolls, helpful: true},
		keyListing{Key: dashboard.Key{Key: "PgUp PgDn", Does: "page"}, works: scrolls, helpful: true},
		keyListing{Key: dashboard.Key{Key: "wheel", Does: "scroll"}, works: scrolls, helpful: true},
	)
}

// historyListings are History's own keys, each in a group of its own: [ ]
// view; < > and the period, where the sub-tab shown has one; v and what it
// has the sub-tab show, where it cycles anything; and with v, while the
// Year shows one account alone, ←→ stepping between the accounts. A phone's
// line keeps [ ] view alone.
func (m Model) historyListings() []keyListing {
	history, p := m.view == dashboard.History, m.past.page
	period, split := p.Period(), p.Split()
	return []keyListing{
		{Key: dashboard.Key{Key: "[ ]", Does: "view"}, works: history, apart: true},
		{Key: dashboard.Key{Key: "< >", Does: "period", State: period}, works: history && period != "", apart: true, wide: true},
		{Key: dashboard.Key{Key: "v", Does: "split", State: split}, works: history && split != "", apart: true, wide: true},
		{Key: dashboard.Key{Key: "←→", Does: "account"}, works: history && p.Shown == dashboard.HistoryYear && p.YearAsk.Account != "", wide: true},
	}
}

// keyLine is the key line as m stands at now, under its page drawn as f:
// while the theme picker is open, its keys alone, then q quit, a phone's line
// as a wide one; else tab views, then the page's own keys that work, in
// their groups, those it leaves to help aside, then p pin where a pin can be
// set, and ? help and q quit last; and the note a key left in their place,
// while that's news. Most pages set p pin apart, a phone's line keeping the
// page's own keys between tab views and ? and q quit, but those it leaves to
// a wider line; the Overview sets ↑↓ focus, p pin, ? help and q quit as a
// group of their own, overviewApart after its toggles, which a phone's line
// leaves to help.
func (m Model) keyLine(f dashboard.Frame, now time.Time) dashboard.KeyLine {
	if m.picker.open {
		return dashboard.KeyLine{Groups: [][]dashboard.Key{m.picker.keys(), {dashboard.Quit}}, Lead: dashboard.KeysAfter, Note: m.noted(now)}
	}
	page := grouped(m.listings(f, now))
	pin, pins := m.pinKey()
	l := dashboard.KeyLine{Groups: append([][]dashboard.Key{{tabViews}}, page.groups...), Lead: 3, PhoneHelp: "keys", Note: m.noted(now)}
	if m.view == dashboard.Overview {
		ends := page.ends
		if pins {
			ends = append(ends, pin)
		}
		l.Lead, l.After, l.PhoneHelp = overviewApart, overviewApart, "keys and toggles"
		l.Groups = append(l.Groups, append(ends, helpKey, dashboard.Quit))
		return l
	}
	l.Phone = page.phone
	if pins {
		l.Groups = append(l.Groups, []dashboard.Key{pin})
	}
	l.Groups = append(l.Groups, []dashboard.Key{helpKey, dashboard.Quit})
	return l
}

// pageKeys are the page's own keys as the key line sets them: in their
// groups; those a phone's line keeps; and those it sets with p, ? and q.
type pageKeys struct {
	groups      [][]dashboard.Key
	phone, ends []dashboard.Key
}

// grouped are the keys listed that work, as the key line sets them, those it
// leaves to help aside: each with those before it, but one set apart, which
// starts a group of its own.
func grouped(listed []keyListing) pageKeys {
	var keys pageKeys
	for _, k := range listed {
		switch {
		case !k.works || k.helpful:
		case k.withEnds:
			keys.ends = append(keys.ends, k.Key)
		default:
			if k.apart || len(keys.groups) == 0 {
				keys.groups = append(keys.groups, nil)
			}
			keys.groups[len(keys.groups)-1] = append(keys.groups[len(keys.groups)-1], k.Key)
			if !k.wide {
				keys.phone = append(keys.phone, k.Key)
			}
		}
	}
	return keys
}

// tabViews and helpKey are tab and ?, which show the next tab and open the
// page's help, as the key line and help list them.
var (
	tabViews = dashboard.Key{Key: "tab", Does: "views"}
	helpKey  = dashboard.Key{Key: "?", Does: "help"}
)

// overviewApart is how many blank cells the Overview's key line sets each of
// its groups apart by, tab views among them.
const overviewApart = 5

// helpKeys are the keys help lists, as m stands at now, its page drawn as f,
// worded as the key line words them: tab; every key of the page's own that
// works, those the key line leaves to help among them; p, where a pin can be
// set; t, in colour; and R, ? and q.
func (m Model) helpKeys(f dashboard.Frame, now time.Time) []dashboard.Key {
	keys := []dashboard.Key{tabViews}
	for _, k := range m.listings(f, now) {
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
	return append(keys, dashboard.Key{Key: "R", Does: "refresh"}, helpKey, dashboard.Quit)
}
