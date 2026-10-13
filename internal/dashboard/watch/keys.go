package watch

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// pageKey acts on a key of the page, as pressed hands it on: j, k, PgDn and
// PgUp scroll the view where it doesn't fit, as scrollView has it; and the
// rest each as acting has the page act: on the Overview, u, g, r, s, c and l
// show and hide its sections, b switches how its windows are grouped, and ↑
// and ↓ move the focus, as overviewKey says; on Accounts, its keys act as
// accountsKey says; on History, [ ], < >, v and ← → act as historyKey says;
// tab and shift-tab show the next tab and the one before; t opens the theme
// picker; and w, in Runway, switches between the day and the week. The rest
// do nothing: the number keys and m among them, which act in the routing
// card alone.
func (m Model) pageKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := key.String()
	if by, ok := scrollKey(k); ok {
		return m.scrollView(by), nil
	}
	afresh := m.acting()
	if next, ok := afresh.overviewKey(k); ok {
		return next, nil
	}
	if next, cmd, ok := afresh.accountsKey(k); ok {
		return next, cmd
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
// key line leaves it to help; whether it leads the key line, in tab views'
// place, as esc back leads a session's page's; whether the key line sets it
// with p, ? and q, rather than among the page's own; whether it sets it
// apart from the keys before it, in a group of its own; whether a phone's
// line leaves it to help, keeping it to a line wider than a phone's; and the
// word a phone's line gives it, where that's another, as enter's open.
type keyListing struct {
	dashboard.Key
	works, helpful, leads, withEnds, apart, wide bool
	phoneDoes                                    string
}

// pageKeys is what lists the page's keys, as it stands, but for whether its
// view scrolls, which follows the size it's drawn at: its own keys but those
// that scroll it, as keys has them; whether it's the Overview, which lists
// those always; p, and whether it's listed, as pinKey has it; and whether the
// dashboard is drawn in colour.
type pageKeys struct {
	own      []keyListing
	overview bool
	pin      dashboard.Key
	pinned   bool
	coloured bool
}

// keys is what lists the page's keys as m stands, its own but those that
// scroll it in the design's order: on the Overview, its toggles, each word
// faint where its section is hidden; b, its word saying how the windows are
// grouped; and with more than one account, ↑↓, which the key line sets with
// p, ? and q; w, in Runway, switching between the day and the week, its word
// saying which it shows; Accounts', as accountsListings has them; and
// History's, as historyListings has them. A session's page lists its own, as
// sessionKeys has them, in place of its tab's.
func (m Model) keys() pageKeys {
	if m.session != nil {
		return m.sessionKeys()
	}
	overview := m.view == dashboard.Overview
	var own []keyListing
	for _, s := range dashboard.EverySection() {
		own = append(own, keyListing{Key: dashboard.Key{Key: s.Key(), Does: s.Word(), Hidden: !m.sections.Has(s)}, works: overview})
	}
	own = append(own,
		keyListing{Key: dashboard.Key{Key: "b", Does: "by", State: string(layoutOf(m.by))}, works: overview},
		keyListing{Key: dashboard.Key{Key: "↑↓", Does: "focus"}, works: overview && m.asRead.accounts > 1, withEnds: true},
		keyListing{Key: dashboard.Key{Key: "w", Does: "window: " + m.span.Name()}, works: m.view == dashboard.Runway},
	)
	pin, pinned := m.pinKey()
	return pageKeys{own: slices.Concat(own, m.accountsListings(), m.historyListings()), overview: overview, pin: pin, pinned: pinned, coloured: m.coloured()}
}

// accountsListings are Accounts' own keys: none yet.
func (m Model) accountsListings() []keyListing {
	return nil
}

// listings are the page's own keys, where its view scrolls or not, in the
// design's order: those that don't scroll it, as keys has them; then j and k,
// PgUp and PgDn, and the wheel, which the key line leaves to help, where the
// view scrolls, and on the Overview always, as USAGE scrolls once its
// accounts outgrow it.
func (p pageKeys) listings(scrolls bool) []keyListing {
	scrolls = scrolls || p.overview
	return slices.Concat(p.own, []keyListing{
		{Key: dashboard.Key{Key: "j k", Does: "scroll"}, works: scrolls, helpful: true},
		{Key: dashboard.Key{Key: "PgUp PgDn", Does: "page"}, works: scrolls, helpful: true},
		{Key: dashboard.Key{Key: "wheel", Does: "scroll"}, works: scrolls, helpful: true},
	})
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
// as a wide one; else tab views, or the page's own keys that lead in its
// place, KeysAfter cells after them, as esc back does on a session's page,
// then the page's own keys that work, in their groups, those it leaves to
// help aside, then p pin where a pin can be set, and ? help and q quit last;
// and the note a key left in their place, while that's news. Most pages set
// p pin apart, a phone's line keeping the page's own keys between those that
// lead and ? and q quit, but those it leaves to a wider line; the Overview
// sets ↑↓ focus, p pin, ? help and q quit as a group of their own,
// overviewApart after its toggles, which a phone's line leaves to help.
func (m Model) keyLine(f dashboard.Frame, now time.Time) dashboard.KeyLine {
	if m.picker.open {
		return dashboard.KeyLine{Groups: [][]dashboard.Key{m.picker.keys(), {dashboard.Quit}}, Lead: dashboard.KeysAfter, Note: m.noted(now)}
	}
	keys := m.keys()
	page := grouped(keys.listings(f.Scrolling(m.doc, now).Most > 0))
	lead, apart := []dashboard.Key{tabViews}, 3
	if page.lead != nil {
		lead, apart = page.lead, dashboard.KeysAfter
	}
	l := dashboard.KeyLine{Groups: append([][]dashboard.Key{lead}, page.groups...), Lead: apart, PhoneHelp: "keys", Note: m.noted(now)}
	if keys.overview {
		ends := page.ends
		if keys.pinned {
			ends = append(ends, keys.pin)
		}
		l.Lead, l.After, l.PhoneHelp = overviewApart, overviewApart, "keys and toggles"
		l.Groups = append(l.Groups, append(ends, helpKey, dashboard.Quit))
		return l
	}
	l.Phone = page.phone
	if keys.pinned {
		l.Groups = append(l.Groups, []dashboard.Key{keys.pin})
	}
	l.Groups = append(l.Groups, []dashboard.Key{helpKey, dashboard.Quit})
	return l
}

// lineKeys are the page's own keys as the key line sets them: in their
// groups; those that lead it, in tab views' place; those a phone's line
// keeps, as it words them; and those it sets with p, ? and q.
type lineKeys struct {
	groups            [][]dashboard.Key
	lead, phone, ends []dashboard.Key
}

// grouped are the keys listed that work, as the key line sets them, those it
// leaves to help aside: each with those before it, but one set apart, which
// starts a group of its own, and those that lead it.
func grouped(listed []keyListing) lineKeys {
	var keys lineKeys
	for _, k := range listed {
		switch {
		case !k.works || k.helpful:
		case k.leads:
			keys.lead = append(keys.lead, k.Key)
		case k.withEnds:
			keys.ends = append(keys.ends, k.Key)
		default:
			if k.apart || len(keys.groups) == 0 {
				keys.groups = append(keys.groups, nil)
			}
			keys.groups[len(keys.groups)-1] = append(keys.groups[len(keys.groups)-1], k.Key)
			if !k.wide {
				keys.phone = append(keys.phone, k.phoned())
			}
		}
	}
	return keys
}

// phoned is the key as a phone's line words it.
func (k keyListing) phoned() dashboard.Key {
	if k.phoneDoes != "" {
		k.Does = k.phoneDoes
	}
	return k.Key
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

// helpKeys are the keys help lists, where the page's view scrolls or not,
// worded as the key line words them: tab; every key of the page's own that
// works, those the key line leaves to help among them; p, where a pin can be
// set; t, in colour; and R, ? and q.
func (p pageKeys) helpKeys(scrolls bool) []dashboard.Key {
	keys := []dashboard.Key{tabViews}
	for _, k := range p.listings(scrolls) {
		if k.works {
			keys = append(keys, k.Key)
		}
	}
	if p.pinned {
		keys = append(keys, p.pin)
	}
	if p.coloured {
		keys = append(keys, dashboard.Key{Key: "t", Does: "themes"})
	}
	return append(keys, dashboard.Key{Key: "R", Does: "refresh"}, helpKey, dashboard.Quit)
}
