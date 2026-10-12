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
// routing cards opened, this one the last; the ids of the accounts a key
// picked, in the config's order, none picking auto, once chosen says one
// has, as picks says; the row the cursor is on; whether m is ticked; why its
// last apply sent nothing, or the router refused it, until the picks change;
// and what it knows of what its moves cost.
type routing struct {
	card    int
	picked  []string
	chosen  bool
	cursor  int
	move    bool
	refused error
	priced  pricing
}

// pricing is what the routing card knows of what its moves cost: the
// sessions views listed to price them, and the sessions the router listed
// that they were listed of; and whether a listing runs, which a card opened
// while it does waits on too.
type pricing struct {
	listed  []views.ListedSession
	of      []status.Session
	listing bool
}

// costedMsg is the sessions views listed to price their moves, of the
// sessions the router listed given.
type costedMsg struct {
	of     []status.Session
	listed []views.ListedSession
}

// openRouting opens the routing card over the page, as p does, where the key
// line lists p: picking what the pin standing names, as picks says, the
// cursor on the first picked, and m clear. While the router doesn't answer,
// it opens nothing, the key line saying so in its place.
func (m Model) openRouting() (tea.Model, tea.Cmd) {
	switch key, listed := m.pinKey(); {
	case !listed:
		return m, nil
	case key.Off:
		return m.noting(status.ErrNoRouter.Error())
	}
	picked, _ := m.standing()
	rows := m.routingRows(m.now())
	cursor := max(slices.IndexFunc(rows, func(row dashboard.RoutingRow) bool { return slices.Contains(picked, row.Account) }), 0)
	m.routing = routing{card: m.routing.card + 1, cursor: cursor, priced: pricing{listing: m.routing.priced.listing}}
	return m.cover(routingOpen), nil
}

// standing are the ids of the accounts the pin standing now names, in the
// config's order: those it can be sent with, sendable, which have a usable
// token, as the router refuses a pin naming any without, as one that lost
// its token since it was pinned; and every one, named. One held at its
// limit, or its token refused, can be sent, as the router takes it.
func (m Model) standing() (sendable, named []string) {
	for _, a := range m.doc.Accounts {
		if !m.doc.Pin.Has(a.ID) {
			continue
		}
		named = append(named, a.ID)
		if a.TokenSet {
			sendable = append(sendable, a.ID)
		}
	}
	return sendable, named
}

// picks are the ids of the accounts the routing card picks, in the config's
// order, none picking auto: those a key picked, once one has; till then,
// those the pin standing now can be sent with, as standing says, so a card
// opened before an order lands, or before a pin's set from the command
// line, follows the pin.
func (m Model) picks() []string {
	if m.routing.chosen {
		return m.routing.picked
	}
	picked, _ := m.standing()
	return picked
}

// routingKey acts on a key while the routing card is open, which takes
// every key but ctrl+c: ↑ and ↓ move between its rows; space picks the row
// the cursor is on, and a number the row it numbers, the cursor going there;
// m ticks or clears moving the sessions running; enter applies the picks;
// and esc closes the card, changing nothing. The rest do nothing.
func (m Model) routingKey(key string) (tea.Model, tea.Cmd) {
	rows := m.routingRows(m.now())
	r := m.routing
	r.cursor, r.picked = min(r.cursor, len(rows)-1), m.picks()
	switch key {
	case "esc":
		return m.uncover()
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
// there, where the row takes it, as takes says: a number past the accounts
// there are, or an account's that can't be picked and isn't, does nothing.
func (r routing) numbered(rows []dashboard.RoutingRow, key string) routing {
	i := slices.IndexFunc(rows, func(row dashboard.RoutingRow) bool { return row.Key == key })
	if i < 0 || !r.takes(rows[i]) {
		return r
	}
	r.cursor = i
	return r.picking(rows, i)
}

// picking is r with the row with the given index picked, where it takes it,
// as takes says: auto, clearing every account, those that can't be picked
// among them; or an account, ticked or cleared, which clears auto.
func (r routing) picking(rows []dashboard.RoutingRow, i int) routing {
	switch row := rows[i]; {
	case !r.takes(row):
		return r
	case row.Account == "":
		r.picked = nil
	default:
		r.picked = toggled(r.picked, row.Account, rows)
	}
	r.chosen, r.refused = true, nil
	return r
}

// takes reports whether the row takes space and its number: where it can be
// picked; or where it can't, while it's picked, as a pin from the command
// line can pick it, to clear it, never to pick it again.
func (r routing) takes(row dashboard.RoutingRow) bool {
	return row.Pickable || slices.Contains(r.picked, row.Account)
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
	picked, move := m.picks(), m.routing.move
	_, named := m.standing()
	switch {
	case !alters(picked, named, move):
		return m.uncover()
	case len(picked) == 0:
		return m.command(order{})
	}
	return m.command(order{accounts: picked, move: move})
}

// alters reports whether applying the accounts picked, with m ticked as
// move says, changes anything of the pin standing now, which names those
// named: they're not those, or m is ticked with accounts picked, to move the
// sessions running elsewhere to them. Auto moves none, so with none picked,
// m alone changes nothing.
func alters(picked, named []string, move bool) bool {
	return !slices.Equal(picked, named) || move && len(picked) > 0
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
		Rows: rows, Picked: m.picks(), Cursor: min(r.cursor, len(rows)-1), Move: r.move,
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
	return views.MovingTo(m.picks(), m.sessions, m.routing.priced.listed)
}

// costing has views list the sessions the router lists, pricing their
// moves, while the routing card wants them: while it's open, m ticked, and
// some session would move, where the sessions listed aren't those it last
// priced, as sameSession compares them, whatever document they came with.
// It starts none while one runs: the sessions are listed again once it
// lands, where they've changed since.
func (m Model) costing() (Model, tea.Cmd) {
	p := m.routing.priced
	if m.open != routingOpen || p.listing || m.moving().Sessions == 0 || slices.EqualFunc(m.sessions, p.of, sameSession) {
		return m, nil
	}
	m.routing.priced.listing = true
	of := m.sessions
	src := views.SessionSources{Running: of, Ledger: m.cfg.Ledger, Prices: m.cfg.Prices, Now: m.now()}
	return m, func() tea.Msg {
		return costedMsg{of: of, listed: views.ListSessions(src).Sessions}
	}
}

// costed takes in the sessions listed to price the routing card's moves:
// the listing runs no more, and while the card is open, it prices them,
// however many looks have come since it started.
func (m Model) costed(msg costedMsg) Model {
	m.routing.priced.listing = false
	if m.open == routingOpen {
		m.routing.priced.listed, m.routing.priced.of = msg.listed, msg.of
	}
	return m
}

// sameSession reports whether two sessions the router listed are alike as
// pricing a move reads them: the same session, pinned alike, with the same
// models, each going to the same account, as busy, and seen last at the same
// time, which a request's line in the ledger follows.
func sameSession(a, b status.Session) bool {
	return a.ID == b.ID && a.Pin == b.Pin && slices.EqualFunc(a.Assignments, b.Assignments, func(x, y status.Assignment) bool {
		return x.Model == y.Model && x.Account == y.Account && x.InFlight == y.InFlight && x.LastSeen.Equal(y.LastSeen)
	})
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
