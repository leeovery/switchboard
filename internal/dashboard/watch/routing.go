package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// routing is the routing card's picker while it's open: what's picked, the
// row the cursor is on, whether m is ticked, and why the router refused the
// last apply. Until it's built, it holds nothing.
type routing struct{}

// openRouting opens the routing card over the page, as p does. Until the
// picker's built, it opens on nothing picked.
func (m Model) openRouting() (tea.Model, tea.Cmd) {
	m.routing = routing{}
	return m.cover(routingOpen), nil
}

// routingKey acts on a key while the routing card is open, which takes
// every key but ctrl+c: esc closes it, changing nothing. Until the picker's
// built, the rest do nothing.
func (m Model) routingKey(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		return m.uncover(), nil
	}
	return m, nil
}

// routingCard is the routing card as the picker stands, its rows read afresh
// from the document on screen at now, with the sessions listed.
func (m Model) routingCard(now time.Time) dashboard.RoutingCard {
	return dashboard.RoutingCard{Rows: dashboard.RoutingRows(m.doc, m.sessions, now, m.cfg.Policy)}
}

// pinKey is p as the key line and help list it, and whether they do: where
// there's more than one account to pin to.
func (m Model) pinKey() (dashboard.Key, bool) {
	return dashboard.Key{Key: "p", Does: "pin"}, len(m.doc.Accounts) > 1 || !m.doc.Pin.IsZero()
}
