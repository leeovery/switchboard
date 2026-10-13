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
// allowance, the pool, and what's coming up, each as the document stands at
// that moment, as At has it, by the rules a view reads it by then. It goes by
// the document's own fields alone, so one that has them already is given
// back as it was, and one from a router from before it gave them gains them
// as that router's own would have.
func (d Document) WorkedOut(policy score.Policy) Document {
	m := d.At(d.GeneratedAt, policy)
	d.Accounts = slices.Clone(d.Accounts)
	for i, j := range m.judged {
		d.Accounts[i].Windows = j.measured()
	}
	d.Pool = m.pool()
	d.ComingUp = m.comingUp()
	return d
}

// AccountWorkedOut is the document's account with the given id, its windows
// with their even pace and allowance, as WorkedOut works them out, without
// the pool or what's coming up; reporting false where the document has no
// such account.
func (d Document) AccountWorkedOut(id string, policy score.Policy) (Account, bool) {
	a, ok := d.Account(id)
	if ok {
		a.Windows = d.judgeAccount(a, d.GeneratedAt, policy).measured()
	}
	return a, ok
}

// standing is how an account stands at a moment, as WindowStatesOf works it
// out: which of its windows have lapsed, and aren't running, as lapsedAt has
// them; what limit holds it, as held says, limited set where any does;
// whether its cap holds it back from every request, a window every model
// shares having reached its reserve, which holds it back, as reserveHolds
// says; and whether its token is refused.
type standing struct {
	lapsed  []string
	held    Hold
	limited bool
	capped  bool
	refused bool
}

// hasLapsed reports whether the account's window with the given key has
// lapsed at the moment given, as policy judges it: the account notes it has,
// or its own window by that key has lapsed by then, as policy's HasLapsed
// judges it. It isn't running, and reads empty, until a request starts it.
func (a Account) hasLapsed(key string, at time.Time, policy score.Policy) bool {
	w, ok := a.Window(key)
	if !ok {
		w = quota.Window{Key: key}
	}
	return a.lapsed(w, at, policy)
}

// lapsed reports whether the account's window w has lapsed at the moment
// given, as hasLapsed judges it.
func (a Account) lapsed(w quota.Window, at time.Time, policy score.Policy) bool {
	return slices.Contains(a.Lapsed, w.Key) || policy.HasLapsed(w, at)
}

// lapsedAt are the keys of the account's windows that have lapsed at the
// moment given, in Usage's order, as hasLapsed judges each.
func (a Account) lapsedAt(at time.Time, policy score.Policy) []string {
	var keys []string
	for _, w := range a.Windows {
		if a.lapsed(w, at, policy) {
			keys = append(keys, w.Key)
		}
	}
	return keys
}

// WindowAt is the account's window with the given key as it stands at the
// moment given, as policy judges it: lapsed, as hasLapsed judges it, empty,
// with no reset, as the router's AsOf has it, as it isn't running until a
// request starts it; else as it runs then, as score.Current has it, so once
// it has reset since it was read, empty, its next reset a length on. It
// reports false where the account has no such window.
func (a Account) WindowAt(key string, at time.Time, policy score.Policy) (quota.Window, bool) {
	w, ok := a.Window(key)
	if !ok {
		return quota.Window{}, false
	}
	return a.asItStands(w, at, policy), true
}

// asItStands is the account's window w as it stands at the moment given, as
// WindowAt has it.
func (a Account) asItStands(w quota.Window, at time.Time, policy score.Policy) quota.Window {
	return windowAsOf(w, a.lapsed(w, at, policy), at)
}

// windowAsOf is window w as it stands at at, as WindowAt has it, lapsed or
// not as given.
func windowAsOf(w quota.Window, lapsed bool, at time.Time) quota.Window {
	if lapsed {
		return quota.Window{Key: w.Key, Label: w.Label}
	}
	return score.Current(w, at)
}

// paceOf is the even pace of a window standing at now as stands, as WindowAt
// has a window: where even use across it would have put its use as of when
// doc was read, as readAt has it, so it stays put while the document does,
// as its projection does, and 0 where it reset after then; reporting false
// where it isn't running, or its length or reset isn't known.
func (d Document) paceOf(stands quota.Window, now time.Time) (float64, bool) {
	return score.Elapsed(stands, d.readAt(now))
}

// allowanceOf is what can be spent of account a's window standing at now as
// stands, as WindowAt has a window, and still last to its reset, its use
// running to where the account runs out, as Floor says, as of when doc was
// read, as readAt has it, as its pace is; or, where it stands in a period
// that started after then, from that period's start, so it's never spread
// over more than the window's length. It's zero where nothing can be.
func (d Document) allowanceOf(a Account, stands quota.Window, now time.Time) quota.Allowance {
	from := d.readAt(now)
	if start, _, ok := stands.Span(); ok {
		from = score.Later(from, start)
	}
	allowance, _ := score.AllowanceOf(stands, d.Floor(a), from)
	return allowance
}

// idleAt reports whether the account is idle at the moment given: its window
// a request starts, as policy names it, has lapsed, as hasLapsed judges it.
func (a Account) idleAt(at time.Time, policy score.Policy) bool {
	return a.hasLapsed(policy.Started, at, policy)
}

// running reports whether the account's window with the given key is
// running: it hasn't lapsed, as lapsedAt judged it.
func (s standing) running(key string) bool {
	return !slices.Contains(s.lapsed, key)
}

// every reports whether a limit holds the account back from every request.
func (s standing) every() bool {
	return s.limited && s.held.Every
}

// outOfQuota reports whether the account's quota for a request of any model
// has run out: a limit or its cap holds it back from every one.
func (s standing) outOfQuota() bool {
	return s.every() || s.capped
}

// shut reports whether the account can take no request: it's out of quota,
// as outOfQuota says, or its token is refused.
func (s standing) shut() bool {
	return s.outOfQuota() || s.refused
}

// holds reports whether a limit holds the account's window with the given
// key as its own: one the router's limit names, or one read spent. A limit
// that names no window holds the account at its limit, as every says, but
// takes no window's room: what each has left is what its reading says.
func (s standing) holds(key string) bool {
	return s.limited && slices.Contains(s.held.Windows, key)
}

// measure is the even pace and the allowance of account a's window with the
// given key, standing at at as stands, as WindowAt has a window, the account
// standing as s says: as paceOf and allowanceOf have them, the pace nil where
// it isn't running, or its length or reset isn't known; and no allowance
// where a limit holds the window as its own, as holds says, as it has no
// room.
func (d Document) measure(a Account, s standing, key string, stands quota.Window, at time.Time) (*float64, quota.Allowance) {
	var pace *float64
	if p, ok := d.paceOf(stands, at); ok {
		pace = &p
	}
	if s.holds(key) {
		return pace, quota.Allowance{}
	}
	return pace, d.allowanceOf(a, stands, at)
}

// Floor is the share of a window of account a used where the account runs
// out: where its reserve starts, where that holds it back, as reserveHolds
// says, else its limit.
func (d Document) Floor(a Account) float64 {
	if d.reserveHolds(a) {
		return 1 - a.Reserve
	}
	return 1
}

// pool is the accounts new sessions can go to, as one, each judged at a
// moment, in the accounts' order: zero with one account, whose own windows
// say the same.
func (d Document) pool(accounts []judgedAccount) Pool {
	if !d.pooled() {
		return Pool{}
	}
	var members []judgedAccount
	for _, j := range accounts {
		if d.inPool(j.states.account.ID) {
			members = append(members, j)
		}
	}
	p := Pool{Pinned: !d.Pin.IsZero(), Windows: []PoolWindow{}}
	var keys []string
	for _, m := range members {
		p.Accounts = append(p.Accounts, m.states.account.ID)
		for _, w := range m.windows {
			if !slices.Contains(keys, w.window.Key) {
				keys = append(keys, w.window.Key)
			}
		}
	}
	slices.SortFunc(keys, quota.CompareKeys)
	for _, key := range keys {
		p.Windows = append(p.Windows, poolWindow(members, key))
	}
	return p
}

// pooled reports whether the document has a pool: it has several accounts,
// and new sessions can go to one of them, as inPool says.
func (d Document) pooled() bool {
	return len(d.Accounts) > 1 && slices.ContainsFunc(d.Accounts, func(a Account) bool { return d.inPool(a.ID) })
}

// inPool reports whether new sessions can go to the account with the given
// id: the global pin names it, or there's none.
func (d Document) inPool(id string) bool {
	return d.Pin.IsZero() || d.Pin.Has(id)
}

// poolWindow is the window with the given key of the members given, as one:
// the room each has left in it, as poolRoom has it, summed; what that leaves
// its bar filled to; and their even paces in it, as it stands, where it's
// running, as paceOf has each, averaged.
func poolWindow(members []judgedAccount, key string) PoolWindow {
	pw := PoolWindow{Key: key}
	var paces float64
	var running int
	for _, m := range members {
		w, ok := m.window(key)
		if !ok {
			continue
		}
		pw.Label = cmp.Or(pw.Label, w.window.Label)
		pw.Room += w.room
		if w.pace != nil {
			paces, running = paces+*w.pace, running+1
		}
	}
	pw.Used = 1 - pw.Room/float64(len(members))
	if running > 0 {
		mean := paces / float64(running)
		pw.Pace = &mean
	}
	return pw
}

// poolRoom is the room account a has left in its window w at at, as the pool
// counts it, standing as s says then, to where the account runs out, as
// Floor says: none in any window of one nothing has been read of, or whose
// token is refused; none in the window a request starts, as policy names it,
// of one at its limit or its cap; and none in a window a limit holds as its
// own, as holds says.
func (d Document) poolRoom(a Account, s standing, w quota.Window, at time.Time, policy score.Policy) float64 {
	switch {
	case a.FetchedAt.IsZero() || s.refused:
		return 0
	case w.Key == policy.Started && s.outOfQuota():
		return 0
	case s.holds(w.Key):
		return 0
	}
	return score.Room(w, d.Floor(a), at)
}

// comingUp is what's coming to the accounts after the moment, as it's worked
// out of a document as of when it was built, soonest first, each account as
// judged then, in the accounts' order: each account's, as comingTo says,
// those at one time in the order of the accounts, then the router's next
// prime of each. It's nil when nothing is, as a document read back without
// any has it.
func (m Moment) comingUp() []Upcoming {
	var all []Upcoming
	for _, j := range m.judged {
		all = append(all, m.doc.comingTo(j)...)
	}
	for _, s := range m.doc.Prime.Slots {
		all = append(all, Upcoming{At: s.Next, Account: s.Account, Kind: UpcomingPrime})
	}
	var after []Upcoming
	for _, u := range all {
		if u.At.After(m.Now) {
			u.At = u.At.UTC()
			after = append(after, u)
		}
	}
	slices.SortStableFunc(after, func(x, y Upcoming) int { return x.At.Compare(y.At) })
	return after
}

// comingTo is what's coming to the account judged as j: its having room
// again, as backs says; its windows running out, as runningOut says; and
// their resets, as resets says.
func (d Document) comingTo(j judgedAccount) []Upcoming {
	backs := d.backs(j)
	return slices.Concat(backs, j.runningOut(), j.resets(backs))
}

// backs are when the account judged as j has room again: held back from
// every request, by a limit or its cap, once each has lifted, as
// backFromEvery says; and held back from some models' requests, as each
// window holding back theirs alone lifts, as backsOfModels says.
func (d Document) backs(j judgedAccount) []Upcoming {
	var backs []Upcoming
	if j.states.stood.outOfQuota() {
		backs = j.backFromEvery()
	}
	return append(backs, d.backsOfModels(j)...)
}

// backsOfModels are when the account judged as j has room again for some
// models' requests, as each window that holds back theirs alone, as policy
// says which, lifts, named: one a limit holds, as Lifts says, and one at its
// cap, where its reserve holds the account back, as it resets.
func (d Document) backsOfModels(j judgedAccount) []Upcoming {
	a, s, policy := j.states.account, j.states.stood, j.states.policy
	var backs []Upcoming
	for _, key := range s.held.Windows {
		if s.limited && !policy.IsShared(key) {
			backs = append(backs, Upcoming{At: s.held.Lifts(key), Account: a.ID, Kind: UpcomingBack, Window: key})
		}
	}
	if !d.reserveHolds(a) {
		return backs
	}
	for _, key := range j.states.reached {
		if w, ok := j.window(key); ok && !policy.IsShared(key) && !s.holds(key) {
			backs = append(backs, Upcoming{At: w.window.ResetsAt, Account: a.ID, Kind: UpcomingBack, Window: key, Cap: true})
		}
	}
	return backs
}

// backFromEvery is when the account judged as j, held back from every
// request by a limit or its cap, has room again: once each has lifted, Cap
// set where its cap lifts last; none where either lifts at no known time.
func (j judgedAccount) backFromEvery() []Upcoming {
	s, capBack := j.states.stood, j.states.capBack
	back := Upcoming{Account: j.states.account.ID, Kind: UpcomingBack}
	if s.every() {
		if s.held.Until.IsZero() {
			return nil
		}
		back.At = s.held.Until
	}
	if s.capped {
		if capBack.IsZero() {
			return nil
		}
		if capBack.After(back.At) {
			back.At, back.Cap = capBack, true
		}
	}
	return []Upcoming{back}
}

// runningOut is the windows of the account judged as j running out before
// they reset, as a moment's windows' RunsOut has them, of the kind that runs
// out, as kindOf judges it: so none that a limit holds, that has lapsed, that
// was last read before a read that failed, or at the account's cap. None runs
// out while the account can take no request, as its use can't rise.
func (j judgedAccount) runningOut() []Upcoming {
	if j.states.stood.shut() {
		return nil
	}
	var outs []Upcoming
	for _, w := range j.windows {
		if kind, _ := j.states.kindOf(w); kind == WindowRunsOut {
			outs = append(outs, Upcoming{At: w.out.At, Account: j.states.account.ID, Kind: UpcomingRunsOut, Window: w.window.Key, Cap: w.out.Reserve})
		}
	}
	return outs
}

// resets are the windows of the account judged as j resetting, each at its
// next reset, as it stands: none of a window that has lapsed; none that
// brings the account back, as one of backs does at the time, of every
// request, or of the requests the window holds back; and none of a window
// some models' requests alone count, as a model's own week, where it resets
// with its week, as resetsWithItsWeek says.
func (j judgedAccount) resets(backs []Upcoming) []Upcoming {
	var resets []Upcoming
	for _, w := range j.windows {
		key, next := w.window.Key, w.stands.ResetsAt
		back := slices.ContainsFunc(backs, func(b Upcoming) bool {
			return b.At.Equal(next) && (b.Window == "" || b.Window == key)
		})
		if !j.states.stood.running(key) || next.IsZero() || back || j.resetsWithItsWeek(w, next) {
			continue
		}
		resets = append(resets, Upcoming{At: next, Account: j.states.account.ID, Kind: UpcomingReset, Window: key})
	}
	return resets
}

// resetsWithItsWeek reports whether the account judged as j's window w, one
// some models' requests alone count, as policy says, resets at next with a
// window of its length every model shares, as that stands, as a model's own
// week resets with the week.
func (j judgedAccount) resetsWithItsWeek(w judged, next time.Time) bool {
	policy := j.states.policy
	length, ok := quota.Length(w.window.Key)
	if !ok || policy.IsShared(w.window.Key) {
		return false
	}
	return slices.ContainsFunc(j.windows, func(shared judged) bool {
		l, ok := quota.Length(shared.window.Key)
		return ok && l == length && policy.IsShared(shared.window.Key) && shared.stands.ResetsAt.Equal(next)
	})
}
