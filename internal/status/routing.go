package status

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/score"
)

// RoutingKind is how an account stands with the router, as the Overview's
// ROUTING says it, and Accounts' Detail and Sessions' lines with it.
type RoutingKind int

// The kinds, in the order of precedence an account's is said in: the first
// that holds is the one said. A hold on some models' requests alone holds
// only where nothing holds the account back from every request, as its words
// say other models still come here.
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
	// RoutingModelCapped is an account at its cap in a model's own window
	// alone, as Fable's week, which holds back that model's requests alone.
	RoutingModelCapped
	// RoutingPressed is an account under pressure.
	RoutingPressed
	// RoutingIdle is an account whose 5-hour window has lapsed.
	RoutingIdle
	// RoutingNext is an account with room that new sessions go to.
	RoutingNext
	// RoutingOpen is an account with room.
	RoutingOpen
)

// Glyph is the glyph an account's words lead with, of its kind: ✕ without a
// usable token; ■ held back by a limit or a refusal; ! unreadable; ○ not read
// yet, or idle; and ● otherwise.
func (k RoutingKind) Glyph() string {
	switch k {
	case RoutingTokenless:
		return "✕"
	case RoutingLimited, RoutingModelLimited, RoutingRefused, RoutingModelRefused:
		return "■"
	case RoutingUnreadable:
		return "!"
	case RoutingUnread, RoutingIdle:
		return "○"
	default:
		return "●"
	}
}

// Routing is how an account stands with the router, as ROUTING says it: its
// kind, which its glyph and colour follow; its first words, as "at its cap";
// the clauses after them, each cut whole from the end where they don't fit,
// as "back 17:10" and "new sessions go elsewhere"; and its short form, as a
// phone has it, in parts, its first words first, as "at its cap" and
// "17:10".
type Routing struct {
	Kind    RoutingKind
	Says    string
	Clauses []string
	Short   []string
}

// String is the words in full, a dot between each part, as in "at its cap ·
// back 17:10 · new sessions go elsewhere".
func (r Routing) String() string {
	return strings.Join(slices.Concat([]string{r.Says}, r.Clauses), " · ")
}

// What's said of an account at its cap, which holds it back, or the global
// pin spends; and of the account new sessions go to.
const (
	goElsewhere = "new sessions go elsewhere"
	stillPinned = "pinned, so new sessions still come here"
	comeHere    = "new sessions come here"
)

// RoutingOf is how doc's account a stands with the router at now, as policy
// says which windows every model shares: the first of the kinds that holds.
// A limit or a refusal is said even where its last read failed, as
// heldRouting says; its usage unreadable only where nothing was read of it
// before, as otherwise its windows as last read say how it stands; its cap
// as cappedRouting says; under pressure, when its pressure window runs out;
// idle at now, as idleAt judges it, when a prime starts it; and open, where new sessions go, of several
// accounts. Words from elsewhere show cleaned, and times as Dated shows them.
func (d Document) RoutingOf(a Account, now time.Time, policy score.Policy) Routing {
	if !a.TokenSet {
		return Routing{Kind: RoutingTokenless, Says: "no token", Clauses: []string{"switchboard accounts token " + Clean(a.ID)}, Short: []string{"no token"}}
	}
	if held, ok := d.heldRouting(a, now, policy); ok {
		return held
	}
	switch {
	case a.FetchedAt.IsZero() && a.Error != "":
		return Routing{Kind: RoutingUnreadable, Says: "can't read it", Clauses: []string{Clean(a.Error)}, Short: []string{"can't read it"}}
	case a.FetchedAt.IsZero():
		return Routing{Kind: RoutingUnread, Says: "not read yet", Short: []string{"not read yet"}}
	}
	if capped, ok := d.cappedRouting(a, now, policy); ok {
		return capped
	}
	switch {
	case a.Pressure.Under:
		return d.pressedRouting(a, now)
	case a.idleAt(now, policy):
		return Routing{Kind: RoutingIdle, Says: "idle", Clauses: []string{d.StartsAt(a.ID, now)}, Short: []string{"idle"}}
	case len(d.Accounts) > 1 && d.Best == a.ID:
		return Routing{Kind: RoutingNext, Says: "open", Clauses: []string{comeHere}, Short: []string{"open", "next"}}
	}
	return Routing{Kind: RoutingOpen, Says: "open", Short: []string{"open"}}
}

// heldRouting is how doc's account a stands, held back at now by a limit or
// the upstream's refusal, reporting false where neither holds it back: from
// every request, a limit, then its token refused; else, where its cap holds
// it back from every request, as cappedRouting says that; else from some
// models' alone, a limit, then their family refused.
func (d Document) heldRouting(a Account, now time.Time, policy score.Policy) (Routing, bool) {
	held, limited := a.Held(now, policy)
	switch {
	case limited && held.Every:
		return Routing{Kind: RoutingLimited, Says: "limit reached", Clauses: backAt(now, held.Until), Short: shortBack("limit reached", now, held.Until)}, true
	case a.TokenRefused(now):
		answer := a.Refused.Answer()
		return Routing{Kind: RoutingRefused, Says: answer, Clauses: []string{"its token needs renewing"}, Short: []string{answer}}, true
	case !limited && !a.Refused.Holds(now):
		return Routing{}, false
	case d.CapHolds(a, now, policy):
		return d.cappedRouting(a, now, policy)
	case limited:
		says := Short(a.label(held.Windows[0])) + " limit"
		return Routing{Kind: RoutingModelLimited, Says: says, Clauses: append(backAt(now, held.Until), stillComes), Short: []string{says}}, true
	}
	short := "refused (" + strconv.Itoa(a.Refused.Status) + ")"
	return Routing{Kind: RoutingModelRefused, Says: a.Refused.Answer(), Clauses: []string{stillComes}, Short: []string{short}}, true
}

// cappedRouting is how account a stands at its cap at now, as policy says
// which windows every model shares, reporting false while no window of it
// has reached its cap, as ReachedReserve judges: at it in a window every model
// shares, which holds it back from every request, as in "at its cap · back
// 17:10 · new sessions go elsewhere", back as the last of those resets; else
// in a model's own, as Fable's week, which holds back that model's requests
// alone, as in "Fable wk at its cap · back Mon 21:00 · other models still
// come here", of its own kind, pinned or not. The global pin naming it spends
// its cap, so new sessions still come here.
func (d Document) cappedRouting(a Account, now time.Time, policy score.Policy) (Routing, bool) {
	keys := slices.SortedStableFunc(slices.Values(a.ReachedReserve(now)), func(x, y string) int { return sharedFirst(policy, x, y) })
	if len(keys) == 0 {
		return Routing{}, false
	}
	shared := policy.IsShared(keys[0])
	kind, says, then := RoutingCapped, "at its cap", goElsewhere
	if !shared {
		kind, says, then = RoutingModelCapped, Short(a.label(keys[0]))+" at its cap", stillComes
	}
	if d.Pin.Has(a.ID) {
		if shared {
			kind = RoutingCappedPinned
		}
		return Routing{Kind: kind, Says: says, Clauses: []string{stillPinned}, Short: []string{says, "pinned"}}, true
	}
	back, _ := score.LetGo(a.Windows, a.Reserve, func(key string) bool { return policy.IsShared(key) == shared }, now)
	return Routing{Kind: kind, Says: says, Clauses: append(backAt(now, back), then), Short: shortBack(says, now, back)}, true
}

// pressedRouting is how account a stands under pressure at now: when its
// pressure window runs out, as RunsOut has it, as in "out ~16:05", where it
// does before it resets.
func (d Document) pressedRouting(a Account, now time.Time) Routing {
	r := Routing{Kind: RoutingPressed, Says: "under pressure", Short: []string{"under pressure"}}
	if w, ok := a.Window(a.Pressure.Window); ok {
		if out, ok := d.RunsOut(a, w, now); ok {
			r.Clauses = []string{"out ~" + Dated(now, out.At)}
		}
	}
	return r
}

// backAt is the clause that says when an account held back at now has room
// again, at t, as in "back 15:54": none where that's unknown.
func backAt(now, t time.Time) []string {
	if t.IsZero() {
		return nil
	}
	return []string{"back " + Dated(now, t)}
}

// shortBack is the short form of what holds an account back, says, and when
// it has room again, at t, as in "limit reached · 15:54": says alone where
// that's unknown.
func shortBack(says string, now, t time.Time) []string {
	if t.IsZero() {
		return []string{says}
	}
	return []string{says, Dated(now, t)}
}
