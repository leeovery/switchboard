package status

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// Condition is the state an account is in, as its card says it: what holds
// it back, or what it's doing.
type Condition int

// The conditions, in the order of precedence an account's state is said in:
// the first that's so is the one said.
const (
	// Tokenless is an account without a usable token.
	Tokenless Condition = iota + 1
	// Limited is an account held back from every request: at a limit in a
	// window every model shares, or with its token refused.
	Limited
	// PartlyLimited is an account held back from some models' requests
	// alone: at a limit in a model's own window, or refused that model's.
	PartlyLimited
	// Unreadable is an account whose usage couldn't be read.
	Unreadable
	// Unread is an account nothing has been read of yet.
	Unread
	// Reserved is an account a window has brought to its reserve, which holds
	// it back, or the global pin spends.
	Reserved
	// Pressed is an account under pressure.
	Pressed
	// Idle is an account whose window a request starts has lapsed.
	Idle
	// Open is an account with room.
	Open
)

// Near is how close a window comes to where an account runs out before it's
// said to near it: within 10 points, as a week nears its reserve.
const Near = 0.1

// State is what an account's card says of it: its condition, and its words,
// what holds it back or what it's doing, then, where there's more to say,
// what that means.
type State struct {
	Condition Condition
	// Says is what holds the account back or what it's doing, such as
	// "under pressure", and Then what that means, such as "new sessions go
	// elsewhere": "" where there's no more to say.
	Says, Then string
}

// String is the state's words, as in "under pressure · new sessions go
// elsewhere".
func (s State) String() string {
	return dotted(s.Says, s.Then)
}

// dotted runs the parts that aren't empty together, a dot between each.
func dotted(parts ...string) string {
	return strings.Join(slices.DeleteFunc(parts, func(p string) bool { return p == "" }), " · ")
}

// StateOf is the state of doc's account a at now, as its card says it, in
// Condition's order of precedence: without a usable token, how to give it
// one; a limit, or a refusal, holding it back, and until when, as heldBack
// says, even where its last read failed; that its usage couldn't be read, or
// hasn't been, as Unread says; its reserve, as reservedState says; that it's
// under pressure, and whether new sessions go elsewhere, else when it runs
// out, as RunsOut has it; its lapsed window, and when a prime starts it,
// where the router says; and last that it's open, its week nearing its
// reserve, or new sessions coming here. Words from elsewhere show cleaned,
// and times as Dated shows them.
func (d Document) StateOf(a Account, now time.Time, policy score.Policy) State {
	if !a.TokenSet {
		return State{Condition: Tokenless, Says: "no token", Then: "switchboard accounts token " + Clean(a.ID)}
	}
	if held, ok := d.heldBack(a, now, policy); ok {
		return held
	}
	if unread, ok := a.Unread(); ok {
		return unread
	}
	if reserved := d.Reserved(a); reserved != "" {
		return d.reservedState(a, reserved, policy)
	}
	if a.Pressure.Under {
		return d.pressed(a, now)
	}
	if slices.Contains(a.Lapsed, policy.Started) {
		return State{Condition: Idle, Says: "idle", Then: d.StartsAt(a.ID, now)}
	}
	return d.open(a)
}

// Unread is the state of account a while its usage isn't read, reporting
// false once it is: why its last read failed, whatever was read of it
// before; else that nothing has been read of it yet.
func (a Account) Unread() (State, bool) {
	switch {
	case a.Error != "":
		return State{Condition: Unreadable, Says: "can't read it", Then: Clean(a.Error)}, true
	case a.FetchedAt.IsZero():
		return State{Condition: Unread, Says: "not read yet"}, true
	}
	return State{}, false
}

// heldBack is the state of doc's account a, held back at now by a limit or
// the upstream's refusal, reporting false where neither holds it back:
// first from every request, a limit, then its token refused; then from some
// models' alone, a limit, then their family refused, which says other models
// still come here, but where a window every model shares has reached the
// reserve that holds the account back, which holds back the rest too, and is
// said as reservedState says it.
func (d Document) heldBack(a Account, now time.Time, policy score.Policy) (State, bool) {
	held, limited := a.Held(now, policy)
	refused := a.Refused.Holds(now)
	switch {
	case limited && held.Every:
		then := back(now, held.Until)
		if then != "" {
			then += ", " + Until(now, held.Until)
		}
		return State{Condition: Limited, Says: "limit reached", Then: then}, true
	case refused && a.Refused.Family == "":
		return State{Condition: Limited, Says: a.Refused.Answer(), Then: "until " + Dated(now, a.Refused.Until)}, true
	case (limited || refused) && d.ReserveHolds(a) && slices.ContainsFunc(a.AtReserve, policy.IsShared):
		return d.reservedState(a, d.Reserved(a), policy), true
	case limited:
		says := Short(a.label(held.Windows[0])) + " limit"
		return State{Condition: PartlyLimited, Says: says, Then: dotted(back(now, held.Until), stillComes)}, true
	case refused:
		return State{Condition: PartlyLimited, Says: a.Refused.Answer(), Then: dotted("until "+Dated(now, a.Refused.Until), stillComes)}, true
	}
	return State{}, false
}

// stillComes is what's said of an account held back from some models'
// requests alone, while another model's can still go there.
const stillComes = "other models still come here"

// reservedState is the state of account a at its reserve, as Reserved says
// it: held back from every request where a window every model shares has
// reached it, or spending it, the global pin naming the account; else from
// some models' requests alone, their own window naming them, which says
// other models still come here, as in "Fable wk at its reserve (90%) ·
// other models still come here".
func (d Document) reservedState(a Account, reserved string, policy score.Policy) State {
	s := State{Condition: Reserved, Says: reserved}
	if d.Pin.Has(a.ID) || slices.ContainsFunc(a.AtReserve, policy.IsShared) {
		return s
	}
	s.Says, s.Then = Short(a.label(a.AtReserve[0]))+" "+reserved, stillComes
	return s
}

// back says when an account held back at now has room again, at t, as in
// "back 15:54": "" where that's unknown.
func back(now, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return "back " + Dated(now, t)
}

// Hold is what holds an account back at its limit: the keys of the windows
// it's at its limit in, as far as they're known, until when it is, and
// whether it's held back from every request.
type Hold struct {
	Windows []string
	// Until is when it has room again: zero where that's unknown.
	Until time.Time
	// Every is set when it's held back from every request: at its limit in a
	// window every model shares, or with a limit naming none.
	Every bool
	// limit is when the router's limit lifts, while one holds, and named the
	// windows it names, none naming every window; spent is when each window
	// read spent resets, by key, zero where that's unknown.
	limit time.Time
	named []string
	spent map[string]time.Time
}

// Holds reports whether the hold holds the window with the given key: one it
// names, or any, for a limit naming none, which holds every window.
func (h Hold) Holds(key string) bool {
	return len(h.Windows) == 0 || slices.Contains(h.Windows, key)
}

// Lifts returns when the hold lifts off the window with the given key: the
// later of when the router's limit lifts, where it holds the window, and when
// the window resets, where it was read spent. It's zero where that isn't
// known, and for a window the hold doesn't hold.
func (h Hold) Lifts(key string) time.Time {
	reset, spent := h.spent[key]
	limited := !h.limit.IsZero() && (len(h.named) == 0 || slices.Contains(h.named, key))
	switch {
	case spent && reset.IsZero():
		return time.Time{}
	case limited:
		return score.Later(h.limit, reset)
	default:
		return reset
	}
}

// Held returns what holds the account back at its limit at now, as policy
// says which windows every model shares, reporting false where nothing does:
// the limit the router saw it reach, while it holds, and every window read
// spent, until it resets, as probing reads them, those every model shares
// first. It's back once the last of what holds it back has lifted, as the
// router reckons it: of what holds back every request, where it's held back
// from every request.
func (a Account) Held(now time.Time, policy score.Policy) (Hold, bool) {
	h := Hold{spent: make(map[string]time.Time)}
	if a.Limit.Holds(now) {
		h.limit, h.named, h.Windows = a.Limit.Until, a.Limit.Windows, slices.Clone(a.Limit.Windows)
	}
	unnamed := !h.limit.IsZero() && len(h.named) == 0
	for _, w := range a.Windows {
		if score.Project(w, now).Kind != score.Exhausted {
			continue
		}
		h.spent[w.Key] = w.ResetsAt
		if !unnamed && !slices.Contains(h.Windows, w.Key) {
			h.Windows = append(h.Windows, w.Key)
		}
	}
	if h.limit.IsZero() && len(h.spent) == 0 {
		return Hold{}, false
	}
	slices.SortStableFunc(h.Windows, func(x, y string) int { return sharedFirst(policy, x, y) })
	h.Every = unnamed || slices.ContainsFunc(h.Windows, policy.IsShared)
	h.Until = h.back(policy)
	return h, true
}

// back is when the account is back from the hold, as policy says which
// windows every model shares: once the last of what holds it back has
// lifted, of what holds back every request where the hold does; zero where
// one of those lifts at no known time.
func (h Hold) back(policy score.Policy) time.Time {
	var last time.Time
	if !h.limit.IsZero() && (!h.Every || len(h.named) == 0 || slices.ContainsFunc(h.named, policy.IsShared)) {
		last = h.limit
	}
	for key, reset := range h.spent {
		if h.Every && !policy.IsShared(key) {
			continue
		}
		if reset.IsZero() {
			return time.Time{}
		}
		last = score.Later(last, reset)
	}
	return last
}

// sharedFirst orders the windows with keys x and y as Held lists them: one
// every model shares, as policy says, before one that isn't.
func sharedFirst(policy score.Policy, x, y string) int {
	switch shared := policy.IsShared(x); {
	case shared == policy.IsShared(y):
		return 0
	case shared:
		return -1
	default:
		return 1
	}
}

// label is the label of the account's window with the given key, or the key
// where it has none.
func (a Account) label(key string) string {
	if w, ok := a.Window(key); ok {
		return w.Label
	}
	return key
}

// pressed is the state of account a under pressure at now: while new sessions
// go to another account, that they go elsewhere; else when its pressure
// window runs out, as RunsOut has it, as in "runs out ~16:05 at this pace",
// or, its reserve holding it back there, "at its reserve ~18:21".
func (d Document) pressed(a Account, now time.Time) State {
	s := State{Condition: Pressed, Says: "under pressure"}
	if d.Best != "" && d.Best != a.ID {
		s.Then = "new sessions go elsewhere"
		return s
	}
	w, ok := a.Window(a.Pressure.Window)
	if !ok {
		return s
	}
	if out, ok := d.RunsOut(a, w, now); ok && out.Reserve {
		s.Then = "at its reserve ~" + Dated(now, out.At)
	} else if ok {
		s.Then = "runs out ~" + Dated(now, out.At) + " at this pace"
	}
	return s
}

// StartsAt says when the lapsed window of the account with the given id
// starts again: at its prime, where the router says when that is, as in
// "window starts at its prime, 16:20"; else with its next request. The words
// fit a 50-column card's state line beside "○ idle · ", as the frames have
// them.
func (d Document) StartsAt(id string, now time.Time) string {
	i := slices.IndexFunc(d.Prime.Slots, func(s Slot) bool { return s.Account == id && !s.Next.IsZero() })
	if i < 0 {
		return "window starts with its next request"
	}
	return "window starts at its prime, " + Dated(now, d.Prime.Slots[i].Next)
}

// open is the state of account a with room: once a week of it comes within
// 10 points of where its reserve starts, that it nears its reserve, as in
// "its week nears its reserve"; on the account new sessions go to, of
// several, that they come here.
func (d Document) open(a Account) State {
	s := State{Condition: Open, Says: "open"}
	if week, ok := d.nearing(a); ok {
		s.Then = "its " + InProse(week.Label) + " nears its reserve"
	} else if len(d.Accounts) > 1 && d.Best == a.ID {
		s.Then = "new sessions come here"
	}
	return s
}

// nearing returns the first of account a's weeks, its windows longer than a
// day, within 10 points of where its reserve starts, where the reserve holds
// it back, reporting false where none is.
func (d Document) nearing(a Account) (quota.Window, bool) {
	if !d.ReserveHolds(a) {
		return quota.Window{}, false
	}
	i := slices.IndexFunc(a.Windows, func(w quota.Window) bool {
		return quota.MultiDay(w.Key) && w.Utilization >= 1-a.Reserve-Near-score.Tolerance
	})
	if i < 0 {
		return quota.Window{}, false
	}
	return a.Windows[i], true
}
