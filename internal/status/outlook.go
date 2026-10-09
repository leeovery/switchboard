package status

import (
	"cmp"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// Pool is the accounts new sessions can go to now, taken as one: those the
// global pin names, else every account.
type Pool struct {
	// Accounts are the ids of its accounts, in the config's order.
	Accounts []string `json:"accounts"`
	// Pinned is set when they're the accounts the global pin names.
	Pinned bool `json:"pinned,omitempty"`
	// Windows are a window each that any of them has, shortest first.
	Windows []PoolWindow `json:"windows"`
}

// PoolWindow is a window of the pool, its accounts' taken as one.
type PoolWindow struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Room is the room its accounts have left in it, summed in accounts'
	// worth, 1.5 being an account and a half.
	Room float64 `json:"room"`
	// Used is what its bar fills to: 1 − Room ÷ its accounts.
	Used float64 `json:"used"`
	// Pace is the even paces of its accounts' windows that are running,
	// averaged: nil where none is.
	Pace *float64 `json:"pace,omitempty"`
}

// The kinds of thing coming up, as an Upcoming's Kind says.
const (
	// UpcomingBack is an account having room again: its limit lifting, or,
	// with Cap, the window at its cap resetting.
	UpcomingBack = "back"
	// UpcomingRunsOut is a window running out before it resets: at its
	// account's cap, with Cap, else at its limit.
	UpcomingRunsOut = "runs_out"
	// UpcomingReset is a window resetting.
	UpcomingReset = "reset"
	// UpcomingPrime is the router priming an account.
	UpcomingPrime = "prime"
)

// Upcoming is something coming to an account after the document was built,
// as COMING UP lists it.
type Upcoming struct {
	At      time.Time `json:"at"`
	Account string    `json:"account"`
	// Kind says what comes, such as UpcomingBack.
	Kind string `json:"kind"`
	// Window is the key of the window that runs out or resets, or, of an
	// account back, the one that held back some models' requests alone, as a
	// model's own week does: "" for one held back from every request, and
	// for a prime.
	Window string `json:"window,omitempty"`
	// Cap is set where it's the account's cap, rather than its limit, that
	// it's back from or runs out at.
	Cap bool `json:"cap,omitempty"`
}

// WorkedOut is the document with what's worked out of it as of when it was
// built, as policy judges its windows: each window's even pace and
// allowance, the pool, and what's coming up. It goes by the document's own
// fields alone, so one that has them already is given back as it was, and
// one from a router from before it gave them gains them as that router's
// own would have.
func (d Document) WorkedOut(policy score.Policy) Document {
	d.Accounts = slices.Clone(d.Accounts)
	standings := make([]standing, len(d.Accounts))
	for i, a := range d.Accounts {
		standings[i] = d.standingOf(a, policy)
		d.Accounts[i].Windows = d.measured(a, standings[i])
	}
	d.Pool = d.pool(standings, policy)
	d.ComingUp = d.comingUp(standings, policy)
	return d
}

// AccountWorkedOut is the document's account with the given id, its windows
// with their even pace and allowance, as WorkedOut works them out, without
// the pool or what's coming up; reporting false where the document has no
// such account.
func (d Document) AccountWorkedOut(id string, policy score.Policy) (Account, bool) {
	a, ok := d.Account(id)
	if ok {
		a.Windows = d.measured(a, d.standingOf(a, policy))
	}
	return a, ok
}

// standing is how an account stands as of when the document was built, as
// what's worked out of it reads it: the keys of its windows that have lapsed,
// and aren't running; what limit holds it, as Held says; whether its cap
// holds it back from every request, a window every model shares having
// reached its reserve, which holds it back, as ReserveHolds says; and whether
// its token is refused.
type standing struct {
	lapsed  []string
	held    Hold
	limited bool
	capped  bool
	refused bool
}

// standingOf is how doc's account a stands as of when doc was built, as
// policy judges its windows: those the document notes have lapsed have, and
// so has any other policy judges lapsed by then.
func (d Document) standingOf(a Account, policy score.Policy) standing {
	at := d.GeneratedAt
	s := standing{lapsed: slices.Concat(a.Lapsed, policy.Lapsed(a.Windows, at)), refused: a.TokenRefused(at)}
	s.held, s.limited = a.Held(at, policy)
	s.capped = d.ReserveHolds(a) && slices.ContainsFunc(score.AtReserve(a.Windows, a.Reserve, at), policy.IsShared)
	return s
}

// running reports whether the account's window with the given key is
// running: it hasn't lapsed.
func (s standing) running(key string) bool {
	return !slices.Contains(s.lapsed, key)
}

// every reports whether a limit holds the account back from every request.
func (s standing) every() bool {
	return s.limited && s.held.Every
}

// shut reports whether the account can take no request: a limit or its cap
// holds it back from every one, or its token is refused.
func (s standing) shut() bool {
	return s.every() || s.capped || s.refused
}

// holds reports whether a limit holds the account's window with the given
// key as its own: one the router's limit names, or one read spent. A limit
// that names no window holds the account at its limit, as every says, but
// takes no window's room: what each has left is what its reading says.
func (s standing) holds(key string) bool {
	return s.limited && slices.Contains(s.held.Windows, key)
}

// measured are account a's windows, standing as s says, each with its even
// pace and its allowance as of when the document was built, its use running
// to where the account runs out, as Floor says: neither where it has lapsed,
// as it isn't running, and no allowance where a limit holds it as its own,
// as holds says, as it has no room.
func (d Document) measured(a Account, s standing) []quota.Window {
	at := d.GeneratedAt
	windows := slices.Clone(a.Windows)
	for i, w := range windows {
		windows[i].Pace, windows[i].Allowance = nil, quota.Allowance{}
		if !s.running(w.Key) {
			continue
		}
		if pace, ok := score.EvenPace(w, at); ok {
			windows[i].Pace = &pace
		}
		if !s.holds(w.Key) {
			windows[i].Allowance, _ = score.AllowanceOf(w, d.Floor(a), at)
		}
	}
	return windows
}

// Floor is the share of a window of account a used where the account runs
// out: where its reserve starts, where that holds it back, as ReserveHolds
// says, else its limit.
func (d Document) Floor(a Account) float64 {
	if d.ReserveHolds(a) {
		return 1 - a.Reserve
	}
	return 1
}

// member is an account of the pool, its windows measured, and how it stands.
type member struct {
	account  Account
	standing standing
}

// pool is the accounts new sessions can go to as of when the document was
// built, as one, each standing as standings says, in the accounts' order,
// their windows' even paces as measured says: zero with one account, whose
// own windows say the same.
func (d Document) pool(standings []standing, policy score.Policy) Pool {
	members := d.poolMembers(standings)
	if len(d.Accounts) < 2 || len(members) == 0 {
		return Pool{}
	}
	p := Pool{Pinned: !d.Pin.IsZero(), Windows: []PoolWindow{}}
	var keys []string
	for _, m := range members {
		p.Accounts = append(p.Accounts, m.account.ID)
		for _, w := range m.account.Windows {
			if !slices.Contains(keys, w.Key) {
				keys = append(keys, w.Key)
			}
		}
	}
	slices.SortFunc(keys, quota.CompareKeys)
	for _, key := range keys {
		p.Windows = append(p.Windows, d.poolWindow(members, key, policy))
	}
	return p
}

// poolMembers are the accounts new sessions can go to, in the config's
// order, each standing as standings says: those the global pin names, else
// every one.
func (d Document) poolMembers(standings []standing) []member {
	var members []member
	for i, a := range d.Accounts {
		if d.Pin.IsZero() || d.Pin.Has(a.ID) {
			members = append(members, member{account: a, standing: standings[i]})
		}
	}
	return members
}

// poolWindow is the window with the given key of the members given, as one:
// the room each has left in it, as poolRoom says, summed; what that leaves
// its bar filled to; and their even paces in it, where it's running,
// averaged.
func (d Document) poolWindow(members []member, key string, policy score.Policy) PoolWindow {
	pw := PoolWindow{Key: key}
	var paces float64
	var running int
	for _, m := range members {
		w, ok := m.account.Window(key)
		if !ok {
			continue
		}
		pw.Label = cmp.Or(pw.Label, w.Label)
		pw.Room += d.poolRoom(m, w, policy)
		if w.Pace != nil {
			paces, running = paces+*w.Pace, running+1
		}
	}
	pw.Used = 1 - pw.Room/float64(len(members))
	if running > 0 {
		mean := paces / float64(running)
		pw.Pace = &mean
	}
	return pw
}

// poolRoom is the room member m has left in its window w as the pool counts
// it, as of when the document was built, to where the account runs out, as
// Floor says: none in any window of one nothing has been read of, or whose
// token is refused; none in the window a request starts, as policy names it,
// of one at its limit or its cap; and none in a window a limit holds as its
// own, as holds says.
func (d Document) poolRoom(m member, w quota.Window, policy score.Policy) float64 {
	s := m.standing
	switch {
	case m.account.FetchedAt.IsZero() || s.refused:
		return 0
	case w.Key == policy.Started && (s.every() || s.capped):
		return 0
	case s.holds(w.Key):
		return 0
	}
	return score.Room(w, d.Floor(m.account), d.GeneratedAt)
}

// comingUp is what's coming to the accounts after the document was built,
// soonest first, each standing as standings says, in the accounts' order,
// as policy judges their windows: each account's, as comingTo says, those at
// one time in the order of the accounts, then the router's next prime of
// each. It's nil when nothing is, as a document read back without any has
// it.
func (d Document) comingUp(standings []standing, policy score.Policy) []Upcoming {
	var all []Upcoming
	for i, a := range d.Accounts {
		all = append(all, d.comingTo(a, standings[i], policy)...)
	}
	for _, s := range d.Prime.Slots {
		all = append(all, Upcoming{At: s.Next, Account: s.Account, Kind: UpcomingPrime})
	}
	var after []Upcoming
	for _, u := range all {
		if u.At.After(d.GeneratedAt) {
			u.At = u.At.UTC()
			after = append(after, u)
		}
	}
	slices.SortStableFunc(after, func(x, y Upcoming) int { return x.At.Compare(y.At) })
	return after
}

// comingTo is what's coming to account a, standing as s says, as of when the
// document was built: its having room again, as backs says; its windows
// running out, as runningOut says; and their resets, as resets says.
func (d Document) comingTo(a Account, s standing, policy score.Policy) []Upcoming {
	backs := d.backs(a, s, policy)
	return slices.Concat(backs, d.runningOut(a, s), d.resets(a, s, backs, policy))
}

// backs are when account a, standing as s says, has room again: held back
// from every request, by a limit or its cap, once each has lifted, as
// backFromEvery says; and held back from some models' requests, as each
// window holding back theirs alone lifts, as backsOfModels says.
func (d Document) backs(a Account, s standing, policy score.Policy) []Upcoming {
	var backs []Upcoming
	if s.every() || s.capped {
		backs = d.backFromEvery(a, s, policy)
	}
	return append(backs, d.backsOfModels(a, s, policy)...)
}

// backsOfModels are when account a, standing as s says, has room again for
// some models' requests, as each window that holds back theirs alone, as
// policy says which, lifts, named: one a limit holds, as Lifts says, and one
// at its cap, where its reserve holds the account back, as it resets.
func (d Document) backsOfModels(a Account, s standing, policy score.Policy) []Upcoming {
	var backs []Upcoming
	for _, key := range s.held.Windows {
		if s.limited && !policy.IsShared(key) {
			backs = append(backs, Upcoming{At: s.held.Lifts(key), Account: a.ID, Kind: UpcomingBack, Window: key})
		}
	}
	if !d.ReserveHolds(a) {
		return backs
	}
	for _, key := range score.AtReserve(a.Windows, a.Reserve, d.GeneratedAt) {
		if w, ok := a.Window(key); ok && !policy.IsShared(key) && !s.holds(key) {
			backs = append(backs, Upcoming{At: w.ResetsAt, Account: a.ID, Kind: UpcomingBack, Window: key, Cap: true})
		}
	}
	return backs
}

// backFromEvery is when account a, held back from every request by a limit
// or its cap, as s says, has room again: once each has lifted, Cap set where
// its cap lifts last; none where either lifts at no known time.
func (d Document) backFromEvery(a Account, s standing, policy score.Policy) []Upcoming {
	back := Upcoming{Account: a.ID, Kind: UpcomingBack}
	if s.every() {
		if s.held.Until.IsZero() {
			return nil
		}
		back.At = s.held.Until
	}
	if s.capped {
		lifts, ok := score.LetGo(a.Windows, a.Reserve, policy.IsShared, d.GeneratedAt)
		if !ok {
			return nil
		}
		if lifts.After(back.At) {
			back.At, back.Cap = lifts, true
		}
	}
	return []Upcoming{back}
}

// runningOut is account a's windows running out before they reset, standing
// as s says, as RunsOut has it as of when the document was built: none while
// it can take no request, as its use can't rise; and none of a window that
// has lapsed, or that a limit holds as its own, as holds says.
func (d Document) runningOut(a Account, s standing) []Upcoming {
	if s.shut() {
		return nil
	}
	var outs []Upcoming
	for _, w := range a.Windows {
		if !s.running(w.Key) || s.holds(w.Key) {
			continue
		}
		if out, ok := d.RunsOut(a, w, d.GeneratedAt); ok {
			outs = append(outs, Upcoming{At: out.At, Account: a.ID, Kind: UpcomingRunsOut, Window: w.Key, Cap: out.Reserve})
		}
	}
	return outs
}

// resets are account a's windows resetting, standing as s says, each at its
// next reset after the document was built, as Current has it: none of a
// window that has lapsed; none that brings the account back, as one of backs
// does at the time, of every request, or of the requests the window holds
// back; and none of a window some models' requests alone count, as a model's
// own week, where it resets with its week, as resetsWithItsWeek says.
func (d Document) resets(a Account, s standing, backs []Upcoming, policy score.Policy) []Upcoming {
	at := d.GeneratedAt
	var resets []Upcoming
	for _, w := range a.Windows {
		next := score.Current(w, at).ResetsAt
		back := slices.ContainsFunc(backs, func(b Upcoming) bool {
			return b.At.Equal(next) && (b.Window == "" || b.Window == w.Key)
		})
		if !s.running(w.Key) || next.IsZero() || back || resetsWithItsWeek(a, w, next, at, policy) {
			continue
		}
		resets = append(resets, Upcoming{At: next, Account: a.ID, Kind: UpcomingReset, Window: w.Key})
	}
	return resets
}

// resetsWithItsWeek reports whether account a's window w, one some models'
// requests alone count, as policy says, resets at next with a window of its
// length every model shares, as of at, as a model's own week resets with the
// week.
func resetsWithItsWeek(a Account, w quota.Window, next, at time.Time, policy score.Policy) bool {
	length, ok := quota.Length(w.Key)
	if !ok || policy.IsShared(w.Key) {
		return false
	}
	return slices.ContainsFunc(a.Windows, func(shared quota.Window) bool {
		l, ok := quota.Length(shared.Key)
		return ok && l == length && policy.IsShared(shared.Key) && score.Current(shared, at).ResetsAt.Equal(next)
	})
}
