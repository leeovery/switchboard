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
	// Unreadable is an account whose usage couldn't be read.
	Unreadable
	// Unread is an account nothing has been read of yet.
	Unread
	// Limited is an account held back from every request: at a limit in a
	// window every model shares, or with its token refused.
	Limited
	// PartlyLimited is an account held back from some models' requests
	// alone: at a limit in a model's own window, or refused that model's.
	PartlyLimited
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
// one; why its usage couldn't be read; that nothing has been; a limit, or a
// refusal, holding it back, and until when, from every request, then from
// some models' alone, which says other models still come here; its reserve;
// that it's under pressure, and whether new sessions go elsewhere, else when
// it runs out, as RunsOut has it; its lapsed window, and when a prime starts
// it, where the router says; and last that it's open, its week nearing its
// reserve, or new sessions coming here. Words from elsewhere show cleaned,
// and times as Dated shows them.
func (d Document) StateOf(a Account, now time.Time, policy score.Policy) State {
	switch {
	case !a.TokenSet:
		return State{Condition: Tokenless, Says: "no token", Then: "switchboard accounts token " + Clean(a.ID)}
	case a.Error != "" && len(a.Windows) == 0:
		return State{Condition: Unreadable, Says: "can't read it", Then: Clean(a.Error)}
	case a.FetchedAt.IsZero():
		return State{Condition: Unread, Says: "not read yet"}
	}
	if held, ok := heldBack(a, now, policy); ok {
		return held
	}
	if reserved := d.Reserved(a); reserved != "" {
		return State{Condition: Reserved, Says: reserved}
	}
	if a.Pressure.Under {
		return d.pressed(a, now)
	}
	if slices.Contains(a.Lapsed, policy.Started) {
		return State{Condition: Idle, Says: "idle", Then: d.StartsAt(a.ID, now)}
	}
	return d.open(a)
}

// heldBack is the state of account a, held back at now by a limit or the
// upstream's refusal, reporting false where neither holds it back: first
// from every request, a limit, then its token refused; then from some
// models' alone, a limit, then their family refused.
func heldBack(a Account, now time.Time, policy score.Policy) (State, bool) {
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
	case limited:
		says := Short(a.label(held.Windows[0])) + " limit"
		return State{Condition: PartlyLimited, Says: says, Then: dotted(back(now, held.Until), stillComes)}, true
	case refused:
		return State{Condition: PartlyLimited, Says: a.Refused.Answer(), Then: dotted("until "+Dated(now, a.Refused.Until), stillComes)}, true
	}
	return State{}, false
}

// stillComes is what's said of an account held back from some models'
// requests alone.
const stillComes = "other models still come here"

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
}

// Holds reports whether the hold holds the window with the given key: one it
// names, or any, for a limit naming none, which holds every window.
func (h Hold) Holds(key string) bool {
	return len(h.Windows) == 0 || slices.Contains(h.Windows, key)
}

// Held returns what holds the account back at its limit at now, as policy
// says which windows every model shares, reporting false where nothing does:
// the limit the router saw it reach, while it holds; else the windows read
// spent, until they reset, as probing reads them. Those every model shares
// come first.
func (a Account) Held(now time.Time, policy score.Policy) (Hold, bool) {
	if a.Limit.Holds(now) {
		windows := slices.Clone(a.Limit.Windows)
		slices.SortStableFunc(windows, func(x, y string) int { return sharedFirst(policy, x, y) })
		return Hold{Windows: windows, Until: a.Limit.Until, Every: len(windows) == 0 || policy.IsShared(windows[0])}, true
	}
	var spent []quota.Window
	for _, w := range a.Windows {
		if score.Project(w, now).Kind == score.Exhausted {
			spent = append(spent, w)
		}
	}
	if len(spent) == 0 {
		return Hold{}, false
	}
	slices.SortStableFunc(spent, func(x, y quota.Window) int { return sharedFirst(policy, x.Key, y.Key) })
	return Hold{Windows: []string{spent[0].Key}, Until: spent[0].ResetsAt, Every: policy.IsShared(spent[0].Key)}, true
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
		length, ok := quota.Length(w.Key)
		return ok && length > day && w.Utilization >= 1-a.Reserve-Near-score.Tolerance
	})
	if i < 0 {
		return quota.Window{}, false
	}
	return a.Windows[i], true
}
