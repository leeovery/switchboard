package status

import (
	"time"

	"github.com/leeovery/switchboard/internal/score"
)

// RoutingKind is how an account stands with the router, as the Overview's
// ROUTING says it, and Accounts' Detail and Sessions' lines with it.
type RoutingKind int

// The kinds, in the order of precedence an account's is said in: the first
// that holds is the one said.
const (
	// RoutingTokenless is an account without a usable token.
	RoutingTokenless RoutingKind = iota + 1
	// RoutingLimited is an account a limit holds back from every request.
	RoutingLimited
	// RoutingModelLimited is an account a limit holds back from some models'
	// requests alone.
	RoutingModelLimited
	// RoutingRefused is an account whose token was refused.
	RoutingRefused
	// RoutingModelRefused is an account one model's requests were refused on
	// alone.
	RoutingModelRefused
	// RoutingUnreadable is an account whose usage can't be read, nothing
	// read of it before.
	RoutingUnreadable
	// RoutingUnread is an account nothing has been read of yet.
	RoutingUnread
	// RoutingCapped is an account at its cap, which holds it back.
	RoutingCapped
	// RoutingCappedPinned is an account at its cap that the global pin
	// names, so new sessions still go to it.
	RoutingCappedPinned
	// RoutingPressed is an account under pressure.
	RoutingPressed
	// RoutingIdle is an account whose 5-hour window has lapsed.
	RoutingIdle
	// RoutingNext is an account with room that new sessions go to.
	RoutingNext
	// RoutingOpen is an account with room.
	RoutingOpen
)

// Routing is how an account stands with the router, as ROUTING says it: its
// kind, which its glyph and colour follow; its first words, as "at its cap";
// the clauses after them, each cut whole from the end where they don't fit,
// as "back 17:10" and "new sessions go elsewhere"; and its short form, as a
// phone has it, as "at its cap · 17:10".
type Routing struct {
	Kind    RoutingKind
	Says    string
	Clauses []string
	Short   string
}

// RoutingOf is how doc's account a stands with the router at now, as policy
// says which windows every model shares: the first of the kinds that holds.
func (d Document) RoutingOf(a Account, now time.Time, policy score.Policy) Routing {
	return Routing{}
}
