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
// routing cards opened, this one the last; session is the id of the session
// a session's card routes, "" on new sessions'; the ids of the accounts a
// key picked, in the config's order, none picking auto, once chosen says one
// has, as picks says; the row the cursor is on; whether m is ticked; why its
// last apply sent nothing, or the router refused it, until the picks change;
// and what it knows of what its moves cost.
type routing struct {
	card    int
	session string
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
// line lists p: on a session's page, the session's card, else new sessions';
// picking what the pin standing names, as picks says, the cursor on the
// first picked, and m clear. While the router doesn't answer, it opens
// nothing, the key line saying so in its place; nor on a session's page not
// yet built, which names no session to route.
func (m Model) openRouting() (tea.Model, tea.Cmd) {
	switch key, listed := m.pinKey(); {
	case !listed:
		return m, nil
	case key.Off:
		return m.noting(status.ErrNoRouter.Error())
	}
	var session string
	if m.session != nil {
		if session = m.session.shown.Page.Session; session == "" {
			return m, nil
		}
	}
	now := m.now()
	mo := m.moment(now)
	m.routing = routing{card: m.routing.card + 1, session: session, priced: pricing{listing: m.routing.priced.listing}}
	picked := m.picks(mo)
	rows := dashboard.RoutingRows(mo, m.sessions, m.traffic.stream(now))
	m.routing.cursor = max(slices.IndexFunc(rows, func(row dashboard.RoutingRow) bool { return slices.Contains(picked, row.Account) }), 0)
	return m.cover(routingOpen), nil
}

// routedPage is the page of the session a session's card routes, reporting
// false where none is open, or the card is new sessions'.
func (m Model) routedPage() (views.SessionPage, bool) {
	if m.session == nil || m.routing.session == "" || m.session.shown.Page.Session != m.routing.session {
		return views.SessionPage{}, false
	}
	return m.session.shown.Page, true
}

// moment is the document on screen as it stands at now, as the policy
// judges it: the moment of the message Update is taking in, where now is
// when it came, as taking has it, else worked out afresh.
func (m Model) moment(now time.Time) status.Moment {
	if held := m.taking.moment; held.Of(m.doc, now, m.cfg.Policy) {
		return held.Moment()
	}
	return m.doc.At(now, m.cfg.Policy)
}

// standing are the ids of the accounts the pin the card routes names as it
// stands at the moment mo, whether they can be picked or not: on a session's
// card, its own pin's, as its page has it; else the global pin's, as pinned
// has them.
func (m Model) standing(mo status.Moment) []string {
	if m.routing.session == "" {
		return pinned(mo)
	}
	if page, ok := m.routedPage(); ok && page.Pin != "" {
		return []string{page.Pin}
	}
	return nil
}

// pinned are the ids of the accounts the global pin standing at the moment
// mo names, in the config's order, whether they can be picked or not.
func pinned(mo status.Moment) []string {
	var ids []string
	for _, a := range mo.Accounts {
		if a.Pinned {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// sendable are the ids given that a pin can be sent with at the moment mo,
// in the config's order: those of the accounts with a usable token, as the
// router refuses a pin naming any without, as one that lost its token since
// it was pinned. One held at its limit, or its token refused, can be sent,
// as the router takes it.
func sendable(mo status.Moment, ids []string) []string {
	var sent []string
	for _, a := range mo.Accounts {
		if a.TokenSet && slices.Contains(ids, a.ID) {
			sent = append(sent, a.ID)
		}
	}
	return sent
}

// picks are the ids of the accounts the routing card picks at the moment
// mo, in the config's order, none picking auto: those a key picked, once one
// has; till then, those the pin standing names, as standing has them, so a
// card opened before an order lands, or before a pin's set from the command
// line, follows the pin.
func (m Model) picks(mo status.Moment) []string {
	if m.routing.chosen {
		return m.routing.picked
	}
	return m.standing(mo)
}

// routingKey acts on a key while the routing card is open, which takes
// every key but ctrl+c: ↑ and ↓ move between its rows; space picks the row
// the cursor is on, and a number the row it numbers, the cursor going there;
// m ticks or clears moving the sessions running, on new sessions' card;
// enter applies the picks; and esc closes the card, changing nothing. The
// rest do nothing.
func (m Model) routingKey(key string) (tea.Model, tea.Cmd) {
	now := m.now()
	mo := m.moment(now)
	rows := dashboard.RoutingRows(mo, m.sessions, m.traffic.stream(now))
	r := m.routing
	r.cursor, r.picked = min(r.cursor, len(rows)-1), m.picks(mo)
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
		r = r.ticking()
	default:
		r = r.numbered(rows, key)
	}
	m.routing = r
	return m, nil
}

// ticking is r with m ticked or cleared, on new sessions' card: a session's
// has no m.
func (r routing) ticking() routing {
	if r.session == "" {
		r.move, r.refused = !r.move, nil
	}
	return r
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
// among them; or an account, which clears auto: on a session's card, it
// alone, picked; on new sessions', ticked or cleared.
func (r routing) picking(rows []dashboard.RoutingRow, i int) routing {
	switch row := rows[i]; {
	case !r.takes(row):
		return r
	case row.Account == "":
		r.picked = nil
	case r.session != "":
		r.picked = []string{row.Account}
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

// apply gives the router the picks, as enter does, for the session a
// session's card routes, or new sessions: the accounts picked that can be
// sent, as sendable says, in the config's order, moving the sessions running
// as m is ticked; or, with none, auto, which moves none, m ticked or not.
// Where it changes nothing of the pin standing, an account it names without
// a token and all, it sends nothing, and closes the card.
func (m Model) apply() (Model, tea.Cmd) {
	mo := m.moment(m.now())
	picked, move, session := m.picks(mo), m.routing.move, m.routing.session
	sent := sendable(mo, picked)
	if !alters(picked, m.standing(mo), sent, move) {
		return m.uncover()
	}
	if len(sent) > 0 {
		return m.command(order{session: session, accounts: sent, move: move})
	}
	return m.command(order{session: session})
}

// alters reports whether applying the accounts picked, of which those sent
// can be sent, with m ticked as move says, changes anything of the pin
// standing now, which names those named: they're not those, or m is ticked
// with accounts to send, to move the sessions running elsewhere to them.
// With none to send, the order moves none, so m alone changes nothing.
func alters(picked, named, sent []string, move bool) bool {
	return !slices.Equal(picked, named) || move && len(sent) > 0
}

// routingCard is the routing card as the picker stands, its rows read afresh
// from the document on screen at now, with the sessions listed, as the
// request stream has moved and seen them since: those picked, those of them
// enter leaves out of the order, as sendable says, and what the order moves;
// on a session's card, the session, as its page has it, its directory shown
// under the home the watch is given, and what moving it costs.
func (m Model) routingCard(now time.Time) dashboard.RoutingCard {
	r, mo := m.routing, m.moment(now)
	rows := dashboard.RoutingRows(mo, m.sessions, m.traffic.stream(now))
	picked := m.picks(mo)
	sent := sendable(mo, picked)
	moving := m.moving(sent)
	card := dashboard.RoutingCard{
		Rows: rows, Picked: picked, Unsent: slices.DeleteFunc(slices.Clone(picked), func(id string) bool { return slices.Contains(sent, id) }),
		Cursor: min(r.cursor, len(rows)-1), Move: r.move, Moving: moving.Sessions, Cost: moving.Cost,
	}
	if page, ok := m.routedPage(); ok {
		card.Mode, card.Cost = dashboard.ThisSession, page.MoveCost
		card.Session = dashboard.RoutingSession{ID: page.Session, Dir: page.Dir, Home: m.cfg.Home, On: page.Account, Auto: page.Auto}
	}
	if r.refused != nil {
		card.Refused = r.refused.Error()
	}
	return card
}

// closing closes a session's card once its page lists no p, as pinKey has
// it, as once the session has ended, or once no page of its session shows.
func (m Model) closing() (Model, tea.Cmd) {
	if m.open != routingOpen || m.routing.session == "" {
		return m, nil
	}
	if _, ok := m.routedPage(); ok {
		if _, listed := m.pinKey(); listed {
			return m, nil
		}
	}
	return m.uncover()
}

// moving is what the order enter sends moves, of the sessions listed,
// priced as views listed them last: to the picks sent, those it can be sent
// with, as sendable says; nothing while m is clear.
func (m Model) moving(sent []string) views.Moving {
	if !m.routing.move {
		return views.Moving{}
	}
	return views.MovingTo(sent, m.sessions, m.routing.priced.listed)
}

// costing has views list the sessions the router lists, pricing their
// moves, while the routing card wants them: while it's open, m ticked, and
// some session would move, where the sessions listed aren't placed as those
// it last priced were, as views.SamePlaces compares them, whatever document
// they came with. It starts none while one runs: the sessions are listed
// again once it lands, where they've changed since.
func (m Model) costing() (Model, tea.Cmd) {
	p := m.routing.priced
	if m.open != routingOpen || p.listing || views.SamePlaces(m.sessions, p.of) {
		return m, nil
	}
	if mo := m.moment(m.now()); m.moving(sendable(mo, m.picks(mo))).Sessions == 0 {
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

// pinKey is p as the key line and help list it, and whether they list it:
// where there's more than one account to pin to, or a pin stands, to clear
// it, as the document on screen was read; on a session's page, while the
// session runs, as its page draws it, where there's more than one account,
// or its own pin stands; but never while the source probes as asked,
// reading no router to pin through, whether one answers or not. It's faint
// while the router doesn't answer, as p then opens nothing.
func (m Model) pinKey() (dashboard.Key, bool) {
	key := dashboard.Key{Key: "p", Does: "pin", Off: !m.answering()}
	switch {
	case m.cfg.ProbeOnly:
		return key, false
	case m.session != nil:
		page := m.session.shown.Page
		return key, page.Runs() && (m.asRead.accounts > 1 || page.Pin != "")
	}
	return key, m.asRead.accounts > 1 || m.asRead.pinned
}
