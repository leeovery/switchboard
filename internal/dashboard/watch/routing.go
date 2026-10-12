package watch

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// routing is the routing card's picker while it's open: card numbers the
// routing cards opened, this one the last; the ids of the accounts picked,
// in the config's order, none picking auto, and those picked as it opened;
// the row the cursor is on; whether m is ticked; and why its last apply sent
// nothing, or the router refused it, until the picks change. listed are the
// sessions the router lists, as views listed them to price their moves, for
// the document that arrived at costed.
type routing struct {
	card           int
	picked, opened []string
	cursor         int
	move           bool
	refused        error
	listed         []views.ListedSession
	costed         time.Time
}

// costedMsg is the sessions the router lists, as views listed them to price
// their moves, for the document that arrived at at.
type costedMsg struct {
	at     time.Time
	listed []views.ListedSession
}

// openRouting opens the routing card over the page, as p does, where the key
// line lists p: the accounts the standing pin names picked, else auto, the
// cursor on the first picked, and m clear. While the router doesn't answer,
// it opens nothing, the key line saying so in its place.
func (m Model) openRouting() (tea.Model, tea.Cmd) {
	switch key, listed := m.pinKey(); {
	case !listed:
		return m, nil
	case key.Off:
		return m.noting(status.ErrNoRouter.Error())
	}
	picked := m.standing()
	rows := m.routingRows(m.now())
	cursor := max(slices.IndexFunc(rows, func(row dashboard.RoutingRow) bool { return slices.Contains(picked, row.Account) }), 0)
	m.routing = routing{card: m.routing.card + 1, picked: picked, opened: picked, cursor: cursor}
	return m.cover(routingOpen), nil
}

// standing are the ids of the accounts the standing pin names, in the
// config's order, whether they can be picked or not.
func (m Model) standing() []string {
	var ids []string
	for _, a := range m.doc.Accounts {
		if m.doc.Pin.Has(a.ID) {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// routingKey acts on a key while the routing card is open, which takes
// every key but ctrl+c: ↑ and ↓ move between its rows; space picks the row
// the cursor is on, and a number the row it numbers, the cursor going there;
// m ticks or clears moving the sessions running; enter applies the picks;
// and esc closes the card, changing nothing. The rest do nothing.
func (m Model) routingKey(key string) (tea.Model, tea.Cmd) {
	rows := m.routingRows(m.now())
	r := m.routing
	r.cursor = min(r.cursor, len(rows)-1)
	switch key {
	case "esc":
		return m.uncover(), nil
	case "enter":
		return m.apply()
	case "up":
		r.cursor = max(r.cursor-1, 0)
	case "down":
		r.cursor = min(r.cursor+1, len(rows)-1)
	case "space":
		r = r.picking(rows, r.cursor)
	case "m":
		r.move, r.refused = !r.move, nil
	default:
		r = r.numbered(rows, key)
	}
	m.routing = r
	return m, nil
}

// numbered is r with the row the key numbers picked, the cursor going
// there, where the row can be picked: a number past the accounts there are,
// or an account's that can't be picked, does nothing.
func (r routing) numbered(rows []dashboard.RoutingRow, key string) routing {
	i := slices.IndexFunc(rows, func(row dashboard.RoutingRow) bool { return row.Key == key })
	if i < 0 || !rows[i].Pickable {
		return r
	}
	r.cursor = i
	return r.picking(rows, i)
}

// picking is r with the row with the given index picked: auto, clearing
// every account, those that can't be picked among them; or an account,
// ticked or cleared, which clears auto. An account that can't be picked
// takes nothing.
func (r routing) picking(rows []dashboard.RoutingRow, i int) routing {
	switch row := rows[i]; {
	case !row.Pickable:
		return r
	case row.Account == "":
		r.picked = nil
	default:
		r.picked = toggled(r.picked, row.Account, rows)
	}
	r.refused = nil
	return r
}

// toggled are the ids of the accounts picked with the account with the
// given id ticked or cleared, in the order of the rows.
func toggled(picked []string, id string, rows []dashboard.RoutingRow) []string {
	var ids []string
	for _, row := range rows {
		if row.Account != "" && slices.Contains(picked, row.Account) != (row.Account == id) {
			ids = append(ids, row.Account)
		}
	}
	return ids
}

// apply gives the router the picks, as enter does: the accounts picked, in
// the config's order, moving the sessions running as m is ticked; or, with
// none picked, auto, which moves none, m ticked or not. Where it changes
// nothing, it sends nothing, and closes the card.
func (m Model) apply() (Model, tea.Cmd) {
	r := m.routing
	switch {
	case !r.changes():
		return m.uncover(), nil
	case len(r.picked) == 0:
		return m.command(order{})
	}
	return m.command(order{accounts: r.picked, move: r.move})
}

// changes reports whether applying the picks changes anything: they're not
// as the card opened, or m is ticked with accounts picked, to move the
// sessions running elsewhere to them. Auto moves none, so with none picked,
// m alone changes nothing.
func (r routing) changes() bool {
	return !slices.Equal(r.picked, r.opened) || r.move && len(r.picked) > 0
}

// routingRows are the routing card's rows, read afresh from the document on
// screen at now, with the sessions listed.
func (m Model) routingRows(now time.Time) []dashboard.RoutingRow {
	return dashboard.RoutingRows(m.doc, m.sessions, now, m.cfg.Policy)
}

// routingCard is the routing card as the picker stands, its rows read afresh
// from the document on screen at now, with what the picks move.
func (m Model) routingCard(now time.Time) dashboard.RoutingCard {
	r := m.routing
	rows := m.routingRows(now)
	moving := m.moving()
	card := dashboard.RoutingCard{
		Rows: rows, Picked: r.picked, Cursor: min(r.cursor, len(rows)-1), Move: r.move,
		Moving: moving.Sessions, Cost: moving.Cost,
	}
	if r.refused != nil {
		card.Refused = r.refused.Error()
	}
	return card
}

// moving is what the picks move, of the sessions listed, priced as views
// listed them last: nothing while m is clear.
func (m Model) moving() views.Moving {
	if !m.routing.move {
		return views.Moving{}
	}
	return views.MovingTo(m.routing.picked, m.sessions, m.routing.listed)
}

// costing has views list the sessions the router lists, pricing their
// moves, while the routing card wants them: while it's open, m ticked, and
// some session would move; once for each document read.
func (m Model) costing() (Model, tea.Cmd) {
	if m.open != routingOpen || m.routing.costed.Equal(m.updated) || m.moving().Sessions == 0 {
		return m, nil
	}
	m.routing.costed = m.updated
	at := m.updated
	src := views.SessionSources{Running: m.sessions, Ledger: m.cfg.Ledger, Prices: m.cfg.Prices, Now: m.now()}
	return m, func() tea.Msg {
		return costedMsg{at: at, listed: views.ListSessions(src).Sessions}
	}
}

// costed takes in the sessions listed to price their moves, while the
// routing card is open over the document they were listed for.
func (m Model) costed(msg costedMsg) Model {
	if m.open == routingOpen && msg.at.Equal(m.updated) {
		m.routing.listed = msg.listed
	}
	return m
}

// pinKey is p as the key line and help list it, and whether they list it:
// where there's more than one account to pin to, or a pin stands, to clear
// it; but never while the source probes as asked, reading no router to pin
// through, whether one answers or not. It's faint while the router doesn't
// answer, as p then opens nothing.
func (m Model) pinKey() (dashboard.Key, bool) {
	key := dashboard.Key{Key: "p", Does: "pin", Off: !m.answering()}
	return key, !m.cfg.ProbeOnly && (len(m.doc.Accounts) > 1 || !m.doc.Pin.IsZero())
}
