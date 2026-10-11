package dashboard

import (
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// numbered is how many accounts have a number that picks them on the routing
// card: the first nine.
const numbered = 9

// RoutingCard is the routing card's data, which the watch builds afresh at
// each read while the card is open, so its figures follow them: what it
// routes; its rows; what's picked, the row the cursor is on and whether m is
// ticked; the sessions m would move and what rebuilding their caches costs;
// and why the router refused the last apply.
type RoutingCard struct {
	// Mode is what the card routes.
	Mode RoutingMode
	// Rows are 0 auto's row, then an account's each, in the config's
	// order, as RoutingRows builds them.
	Rows []RoutingRow
	// Picked are the ids of the accounts picked, in the config's order: none
	// picks auto.
	Picked []string
	// Cursor is the row the cursor is on, its index in Rows.
	Cursor int
	// Move is set while m is ticked, to move the sessions running elsewhere
	// to the accounts picked too.
	Move bool
	// Moving counts the sessions that would move: those running on an
	// account not picked, without a pin of their own. Cost is what
	// rebuilding their caches costs: nil where none is known.
	Moving int
	Cost   *ledger.Picodollars
	// Refused is the router's reason for refusing the last apply: "" while
	// it hasn't.
	Refused string
}

// RoutingMode is what a routing card routes: new sessions, its zero value.
// A session's card, on its own page, joins it with that page.
type RoutingMode int

// NewSessions is new sessions' card, ROUTING · NEW SESSIONS, which every
// page opens.
const NewSessions RoutingMode = 0

// RoutingRow is a row of the routing card: auto's, or an account's.
type RoutingRow struct {
	// Account is the account's id: "" on auto's row.
	Account string
	// Key is the number key that picks the row, "0" on auto's, an account's
	// place in the config: "" for an account past the ninth.
	Key string
	// Name is the account's name.
	Name string
	// Open is set where the account takes new sessions now; else Reason says
	// why it's closed, as "at its cap till 17:10", and Capped whether its own
	// cap closed it, in amber, rather than Anthropic, in red.
	Open   bool
	Reason string
	Capped bool
	// Session and Week are its 5-hour window's use and its week's.
	Session, Week Figure
	// Running counts its sessions running.
	Running int
	// Pickable is set where it can be picked: not where it can take no
	// session, held at its limit, its token refused, or with no token.
	Pickable bool
}

// Figure is a window's use as the routing card shows it, from 0 to 1, its
// level coloured as usage colours it: none where Read says the window
// hasn't been read.
type Figure struct {
	Used float64
	Read bool
}

// RoutingRows are the routing card's rows of doc at now, by policy, with
// the sessions the router listed: 0 auto's, then an account's each, in the
// config's order. Until they're built, an account's row is its id, number,
// name and whether it can be picked.
func RoutingRows(doc status.Document, _ []status.Session, now time.Time, policy score.Policy) []RoutingRow {
	rows := []RoutingRow{{Key: "0", Name: "auto", Open: true, Pickable: true}}
	for i, a := range doc.Accounts {
		row := RoutingRow{Account: a.ID, Name: name(a), Pickable: pickable(a, now, policy)}
		if i < numbered {
			row.Key = strconv.Itoa(i + 1)
		}
		rows = append(rows, row)
	}
	return rows
}

// pickable reports whether the account can be picked at now: it has a
// usable token, not refused, and no limit holds back its every request.
func pickable(a status.Account, now time.Time, policy score.Policy) bool {
	return a.TokenSet && !a.TokenRefused(now) && !limited(a, now, policy)
}

// card is the routing card. Until it's built, it's its title alone.
func (r RoutingCard) card(f Frame) Card {
	return Card{title: "ROUTING · NEW SESSIONS", width: f.cardWidth(routingWidth)}
}
