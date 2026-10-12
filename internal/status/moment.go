package status

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// Moment is a document as it stands at a moment, as a policy judges it: each
// fact a view reads of it, worked out once, by the one function that owns
// it, for the frame, the printout or the tick that draws it. Each field says
// whether it's as the document was read, or as it stands at Now.
type Moment struct {
	// Now is the moment, and ReadAt when the document was read, which its
	// projections, even paces and allowances go from: when it was built,
	// else Now.
	Now, ReadAt time.Time
	// Routed is set on the router's document, and Probed on one built by
	// probing, as read: neither on a document of neither, as before any is
	// read.
	Routed, Probed bool
	// Fallback, Health, Restart, Pin, Events and Sessions are the document's
	// records, as read: why it was probed rather than the router's, the
	// router's health, a restart it has due, its global pin, what has
	// happened lately, and how many sessions it routed in the last hour.
	Fallback Fallback
	Health   Health
	Restart  Restart
	Pin      Pin
	Events   []Event
	Sessions int
	// ComingUp is what's coming to the accounts after Now, of what the
	// document had coming as it was built, soonest first.
	ComingUp []Upcoming
	// Next is the id of the account new sessions go to, which ▲ marks, as
	// read, as next has it.
	Next string
	// Accounts are its accounts, in the config's order, each as it stands at
	// Now.
	Accounts []AccountNow
	// Shown are the windows of the accounts worth showing at Now, in
	// quota's order: those every model shares, always, and any other an
	// account has used this period, or is heading to, as its InUse says, or
	// a probe expected but couldn't read. Hidden are the rest, unused on
	// every account.
	Shown, Hidden []WindowName
	// Coming is what comes next of priming at Now: of the windows a prime
	// starts, the next to reset, as they stand then, and the router's next
	// prime.
	Coming []Coming

	doc    Document
	policy score.Policy
	judged []judgedAccount
	pool   func() Pool
}

// AccountNow is an account of a document as it stands at a moment.
type AccountNow struct {
	// ID and Label are as configured, Place is its place in the config's
	// order, from 1, and Primary is set on the primary.
	ID, Label string
	Place     int
	Primary   bool
	// Reserve is the share of every window it leaves unused, as configured,
	// and Cap where its reserve starts, 1 − Reserve. ReserveHolds is set
	// where its reserve holds it back once its use reaches it, as read: it
	// has one, and the global pin doesn't spend it. Floor is the share of a
	// window used where it runs out: its cap where its reserve holds, else
	// its limit.
	Reserve, Cap, Floor float64
	ReserveHolds        bool
	// Pinned is set where the global pin names it, Next where new sessions go
	// to it, which ▲ marks, InPool where it's one of the pool's accounts, as
	// Pool has them, and OutOfPool where the pool leaves it out, as the
	// global pin names others, each as read.
	Pinned, Next, InPool, OutOfPool bool
	// TokenSet is set where its token is usable, Read where its usage was
	// read, whatever windows it reported, and ReadAt is when: zero where it
	// never was. Error says why its last read failed, cleaned, Failures are
	// the windows its last probe expected but couldn't read, and Sessions how
	// many sessions the router sent it in the last hour, each as read.
	TokenSet, Read bool
	ReadAt         time.Time
	Error          string
	Failures       []quota.Failure
	Sessions       int
	// Routing is how it stands with the router at Now, as RoutingOf says it.
	Routing Routing
	// Hold is what holds it back at its limit at Now, as Held has it, and
	// Held is set where anything does. LimitSince is when it reached the
	// router's limit holding it at Now, as that limit's event tells: zero
	// where none tells.
	Hold       Hold
	Held       bool
	LimitSince time.Time
	// ReachedReserve are the keys of its windows at its reserve at Now,
	// whether or not the global pin spends it, as ReachedReserve has them.
	// CapHolds is set where its cap holds it back from every request at Now,
	// as CapHolds says, and CapBack is when it lets go: once the last of
	// those windows resets, zero where its cap doesn't hold or that isn't
	// known.
	ReachedReserve []string
	CapHolds       bool
	CapBack        time.Time
	// Refusal is the upstream's refusal of its requests holding at Now: zero
	// where none does. TokenRefused is set where the refusal is of its
	// token, which holds back every request.
	Refusal      Refusal
	TokenRefused bool
	// Pressure is how it stands under pressure at Now.
	Pressure PressureNow
	// Idle is set where its window a request starts has lapsed at Now, as
	// HasLapsed judges it, and NextPrime is when the router next primes it,
	// starting that window, as read: zero where the document doesn't say,
	// and it starts with its next request.
	Idle      bool
	NextPrime time.Time
	// CanRise is set where it can take a request of some model at Now, so
	// its use can rise: no limit or cap holds it back from every request,
	// and its token isn't refused.
	CanRise bool
	// Windows are its windows, in its order, each as it stands at Now.
	Windows []WindowNow

	// unreported is the kind of a window it hasn't reported, as Window gives
	// it.
	unreported WindowKind
}

// PressureNow is how an account stands under pressure at a moment.
type PressureNow struct {
	// Under is set while it's under pressure at Now: the router found it so
	// as it built the document, and its pressure window hasn't reset since,
	// as underPressure judges it.
	Under bool
	// Window is the key of the window its pressure is judged in, as the
	// router named it, as read, and Out where that window runs out at Now,
	// as its own Out has it: At zero where it doesn't before it resets.
	Window string
	Out    RunOut
}

// WindowName is a window of a document's accounts, by its key, as a moment
// names it.
type WindowName struct {
	Key string
	// Label is its label, as the first of the accounts with it labels it:
	// its key where none does.
	Label string
	// Name is what it's called: of a window of whole hours shorter than a
	// day, its length, as "5-hour"; else its label, cleaned.
	Name string
}

// WindowNow is a window of an account as it stands at a moment. Of a window
// of an account without a usable token, or nothing read, its Kind says so,
// and of the rest, its name, its roles and Read alone are given.
type WindowNow struct {
	WindowName
	// Shared, Started and Pressure are its roles, as policy names them: a
	// window every model shares, the window a request starts, and the window
	// pressure is judged in.
	Shared, Started, Pressure bool
	// Kind is how it stands at Now, the first that holds of the kinds in
	// their order, and At when what Kind says comes, or came, as Of has them.
	Kind WindowKind
	At   time.Time
	// Used is how much of it is used as it stands at Now, as WindowAt has it:
	// none once it has reset since it was read, or of one not started. Pace
	// is its even pace, and Allowance what can be spent of it and still last
	// to its reset, as of when the document was read, of the window so
	// standing, as measure has them: nil where it isn't running or its reset
	// isn't known, and no allowance where a limit holds it as its own. Next
	// is its next reset as it stands at Now: zero where it has lapsed, or
	// that isn't known.
	Used      float64
	Pace      *float64
	Allowance quota.Allowance
	Next      time.Time
	// Room is the room it has left at Now, in accounts' worth, as the pool
	// counts it, as RoomOf has it.
	Room float64
	// Start is when it started, as it stands at Now, and Length how long it
	// runs from then to its reset: zero where that isn't known, as of one
	// not started.
	Start  time.Time
	Length time.Duration
	// ResetSinceRead is set where its reset, as read, has passed by Now, and
	// Lapsed where it has lapsed at Now, as HasLapsed judges it, and isn't
	// running.
	ResetSinceRead, Lapsed bool
	// Held is set where Claude's limit holds it at Now, as Kind says. Back is
	// when room returns, as At has it, zero where that isn't known, and
	// Since when the account reached the limit, as the router's event of it
	// tells, zero where none tells.
	Held        bool
	Back, Since time.Time
	// Capped is set where its use has reached its account's cap at Now, which
	// holds the account back, as ReachedReserve judges it. CappedAsRead is
	// set where the router noted it at its reserve as it built the document,
	// as AtReserve has it, which holds the account back: history, which
	// stands as noted after the window resets.
	Capped, CappedAsRead bool
	// RunsOut is set where it runs out at Now, as a view says it does: its
	// Kind is WindowRunsOut, the first of the kinds that holds, and At says
	// when. A window a limit holds, at its cap, not started, or of an account
	// whose token is refused or whose last read failed doesn't, whatever its
	// projection says.
	RunsOut bool
	// Out is where it runs out at Now at the pace it's heading at, as
	// RunsOut says, projected from when the document was read, whatever
	// holds it: At zero where it doesn't before it resets, as once it has
	// reset since it was read. Its Heading is where it's heading, as the
	// document's Project says. A bar's projection goes by it.
	Out RunOut
	// Projected is the share of it it's heading for by its reset at Now, as
	// a bar's projection fills to: all of it where it runs out at its limit;
	// nil where nothing says, and while it's held or not started.
	Projected *float64
	// Nearing is set where what it's heading for at Now, short of running
	// out, comes within Near of where its account runs out.
	Nearing bool
	// InUse is set where its account has used it this period, or is heading
	// to: some of it used as it stands at Now, or its heading has some used
	// by its reset.
	InUse bool
	// Read is the window as it was read.
	Read AsRead
}

// AsRead is a window as it was read, before it's judged at a moment: how much
// of it was used, when it was to reset, and when it was read.
type AsRead struct {
	Used     float64
	ResetsAt time.Time
	At       time.Time
}

// At is the document as it stands at now, as policy judges its windows: each
// fact a view reads of it, worked out once, as Moment has them. Its pool is
// worked out the first time it's asked for.
func (d Document) At(now time.Time, policy score.Policy) Moment {
	judged := d.judgeAll(now, policy)
	m := Moment{
		Now: now, ReadAt: d.readAt(now), Routed: d.Source == SourceRouter, Probed: d.Source == SourceProbe,
		Fallback: d.Fallback, Health: d.Router, Restart: d.Restart, Pin: d.Pin, Events: d.Events, Sessions: d.Sessions,
		ComingUp: comingAfter(d.ComingUp, now), Next: d.next(),
		doc: d, policy: policy, judged: judged,
		pool: sync.OnceValue(func() Pool { return d.pool(judged) }),
	}
	names := d.windowNames()
	m.Accounts = make([]AccountNow, len(judged))
	for i, j := range judged {
		m.Accounts[i] = m.accountNow(i+1, j, names)
	}
	m.Shown, m.Hidden = m.windowsShown(names)
	m.Coming = m.coming()
	return m
}

// Pool is the accounts new sessions can go to at Now, as one, as PoolAt has
// it: zero with one account, whose own windows say the same.
func (m Moment) Pool() Pool {
	if m.pool == nil {
		return Pool{}
	}
	return m.pool()
}

// Text is the document as status prints it at Now, as Text has it, with the
// sessions given.
func (m Moment) Text(sessions ...Session) string {
	return m.doc.Text(m.Now, m.policy, sessions...)
}

// Destination says where a pin to the accounts with the given ids sends
// sessions, as the document's Destination says it.
func (m Moment) Destination(ids []string) string {
	return m.doc.Destination(ids)
}

// Account is the account with the given id, reporting false where the
// document has none.
func (m Moment) Account(id string) (AccountNow, bool) {
	i := slices.IndexFunc(m.Accounts, func(a AccountNow) bool { return a.ID == id })
	if i < 0 {
		return AccountNow{}, false
	}
	return m.Accounts[i], true
}

// Window is the account's window with the given key; or, reporting false,
// one it hasn't reported, which stands unread, or without a usable token,
// its key and kind alone given.
func (a AccountNow) Window(key string) (WindowNow, bool) {
	i := slices.IndexFunc(a.Windows, func(w WindowNow) bool { return w.Key == key })
	if i < 0 {
		return WindowNow{Key: key, Kind: a.unreported}, false
	}
	return a.Windows[i], true
}

// next is the id of the account new sessions go to, which ▲ marks: of one
// account, that one, whatever its room, as there's nowhere else for them to
// go; of several, the router's best, "" where none has room; and none of
// several in a document built by probing, as where new sessions go is the
// router's to say, and its best is worked out without the router's pin.
func (d Document) next() string {
	switch {
	case len(d.Accounts) == 1:
		return d.Accounts[0].ID
	case d.Source != SourceRouter:
		return ""
	}
	return d.Best
}

// comingAfter are the things coming up given that come after now, in their
// order.
func comingAfter(upcoming []Upcoming, now time.Time) []Upcoming {
	return slices.DeleteFunc(slices.Clone(upcoming), func(u Upcoming) bool { return !u.At.After(now) })
}

// judgedAccount is an account of a document as it stands at a moment, worked
// out once: how it stands, as its WindowStates have it, and each of its
// windows, in its order, as judge has it.
type judgedAccount struct {
	states  WindowStates
	windows []judged
}

// judgeAll is how each of doc's accounts stands at now, in their order, as
// policy judges their windows.
func (d Document) judgeAll(now time.Time, policy score.Policy) []judgedAccount {
	all := make([]judgedAccount, len(d.Accounts))
	for i, a := range d.Accounts {
		all[i] = d.judgeAccount(a, now, policy)
	}
	return all
}

// judgeAccount is how doc's account a stands at now, as policy judges its
// windows.
func (d Document) judgeAccount(a Account, now time.Time, policy score.Policy) judgedAccount {
	j := judgedAccount{states: d.WindowStatesOf(a, now, policy), windows: make([]judged, len(a.Windows))}
	for i, w := range a.Windows {
		j.windows[i] = j.states.judge(w)
	}
	return j
}

// window is the account's window with the given key, as judged, reporting
// false where it has none.
func (j judgedAccount) window(key string) (judged, bool) {
	i := slices.IndexFunc(j.windows, func(w judged) bool { return w.window.Key == key })
	if i < 0 {
		return judged{}, false
	}
	return j.windows[i], true
}

// measured are the account's windows as read, each with its even pace and
// its allowance as judged, as the document gives them.
func (j judgedAccount) measured() []quota.Window {
	windows := slices.Clone(j.states.account.Windows)
	for i, w := range j.windows {
		windows[i].Pace, windows[i].Allowance = w.pace, w.allowance
	}
	return windows
}

// accountNow is the account judged as j, in the given place, as it stands at
// the moment, its windows named as names has them.
func (m Moment) accountNow(place int, j judgedAccount, names map[string]WindowName) AccountNow {
	d, s, a := m.doc, j.states, j.states.account
	an := AccountNow{
		ID: a.ID, Label: a.Label, Place: place, Primary: a.Primary,
		Reserve: a.Reserve, Cap: 1 - a.Reserve, Floor: d.Floor(a), ReserveHolds: d.ReserveHolds(a),
		Pinned: d.Pin.Has(a.ID), Next: a.ID == m.Next, InPool: d.pooled() && d.inPool(a.ID), OutOfPool: d.pooled() && !d.inPool(a.ID),
		TokenSet: a.TokenSet, Read: !a.FetchedAt.IsZero(), ReadAt: a.FetchedAt, Error: Clean(a.Error), Failures: a.Failures, Sessions: a.Sessions,
		Hold: s.stood.held, Held: s.stood.limited, LimitSince: eventAt(d.limitEvent(a, m.Now)),
		ReachedReserve: s.reached, CapHolds: s.stood.capped, CapBack: s.capBack, TokenRefused: s.stood.refused,
		Idle: s.idle, NextPrime: d.primeOf(a.ID), CanRise: !s.stood.shut(),
		Pressure: PressureNow{Under: s.pressed, Window: a.Pressure.Window},
	}
	an.unreported, _ = s.unknown(false)
	if s.refusing {
		an.Refusal = a.Refused
	}
	if w, ok := j.window(a.Pressure.Window); ok {
		an.Pressure.Out = w.out
	}
	an.Routing = s.routing(an.Pressure.Out)
	an.Windows = make([]WindowNow, len(j.windows))
	for i, w := range j.windows {
		an.Windows[i] = m.windowNow(s, w, names[w.window.Key])
	}
	return an
}

// windowNow is the window judged as w, of the account whose windows stand as
// s says, as it stands at the moment, named as given.
func (m Moment) windowNow(s WindowStates, w judged, name WindowName) WindowNow {
	a, key, policy := s.account, w.window.Key, m.policy
	wn := WindowNow{
		WindowName: name, Shared: policy.IsShared(key), Started: key == policy.Started, Pressure: key == policy.Pressure,
		Read: AsRead{Used: w.window.Utilization, ResetsAt: w.window.ResetsAt, At: a.FetchedAt},
	}
	if kind, ok := s.unknown(true); ok {
		wn.Kind = kind
		return wn
	}
	wn.Kind, wn.At = s.kind(w.window, w)
	wn.Used, wn.Pace, wn.Allowance, wn.Next, wn.Room = w.stands.Utilization, w.pace, w.allowance, w.stands.ResetsAt, w.room
	wn.Start, wn.Length, _ = w.stands.Span()
	wn.ResetSinceRead, wn.Lapsed = w.window.ResetBy(m.Now), w.lapsed
	wn.Held = wn.Kind == WindowLimited
	if wn.Held {
		wn.Back, wn.Since = wn.At, eventAt(m.doc.reachedIn(a, w.window, m.Now))
	}
	wn.Capped, wn.CappedAsRead = s.capped(key), m.doc.ReserveHolds(a) && slices.Contains(a.AtReserve, key)
	wn.RunsOut, wn.Out, wn.Projected = wn.Kind == WindowRunsOut, w.out, projected(wn.Kind, w.out.Projection)
	wn.Nearing = wn.Projected != nil && !w.runsOut && *wn.Projected >= m.doc.Floor(a)-Near-score.Tolerance
	wn.InUse = inUse(w.stands, w.out.Projection)
	return wn
}

// projected is the share of a window of the given kind, heading as p says,
// that it's heading for by its reset, as a bar's projection fills to: all of
// it where it runs out; nil where p says nothing, and while it's held at its
// limit or not started.
func projected(kind WindowKind, p score.Projection) *float64 {
	if kind == WindowLimited || kind == WindowNotStarted {
		return nil
	}
	switch p.Kind {
	case score.RunsOut:
		return new(1.0)
	case score.OnPace:
		return new(p.AtReset)
	default:
		return nil
	}
}

// inUse reports whether a window standing as stands, heading as p says, is
// in use this period, or is heading to be: some of it is used, or p has it
// run out, spent already, or with some used by its reset.
func inUse(stands quota.Window, p score.Projection) bool {
	if stands.Utilization > 0 {
		return true
	}
	switch p.Kind {
	case score.RunsOut, score.Exhausted:
		return true
	case score.OnPace:
		return p.AtReset > score.Tolerance
	default:
		return false
	}
}

// windowNames are the windows of doc's accounts, and those a probe of one
// expected but couldn't read, by key, each named as WindowName has it.
func (d Document) windowNames() map[string]WindowName {
	labels := make(map[string]string)
	for _, a := range d.Accounts {
		for _, w := range a.Windows {
			labels[w.Key] = cmp.Or(labels[w.Key], w.Label)
		}
		for _, f := range a.Failures {
			if _, ok := labels[f.Window]; !ok {
				labels[f.Window] = ""
			}
		}
	}
	names := make(map[string]WindowName, len(labels))
	for key, label := range labels {
		label = cmp.Or(label, key)
		names[key] = WindowName{Key: key, Label: label, Name: nameOf(key, label)}
	}
	return names
}

// nameOf is what the window with the given key and label is called: of a
// window of whole hours shorter than a day, its length, as "5-hour"; else its
// label, cleaned.
func nameOf(key, label string) string {
	if length, ok := quota.Length(key); ok && length%time.Hour == 0 && length < day {
		return fmt.Sprintf("%d-hour", length/time.Hour)
	}
	return Clean(label)
}

// windowsShown are the windows of the accounts at the moment, in quota's
// order, named as names has them: those shown, every model's, those in use
// on an account, and those a probe expected but couldn't read; and those
// hidden, the rest.
func (m Moment) windowsShown(names map[string]WindowName) (shown, hidden []WindowName) {
	used := make(map[string]bool)
	for _, a := range m.Accounts {
		for _, w := range a.Windows {
			used[w.Key] = used[w.Key] || w.Shared || w.InUse
		}
		for _, f := range a.Failures {
			used[f.Window] = true
		}
	}
	for _, key := range slices.SortedFunc(maps.Keys(used), quota.CompareKeys) {
		if used[key] {
			shown = append(shown, names[key])
		} else {
			hidden = append(hidden, names[key])
		}
	}
	return shown, hidden
}

// eventAt is when the event given happened, where there is one: zero where
// ok says there isn't.
func eventAt(e Event, ok bool) time.Time {
	if !ok {
		return time.Time{}
	}
	return e.At
}

// reachedIn is the router's event of account a reaching the limit that holds
// its window w at now, reporting false where none tells of it. Where the
// account's limit holds w, it's that limit's event, as limitEvent finds it.
// Where the account has no such limit, as a probed document has none, or one
// without an identity, as a router from before limits had them gives, it's
// the newest event of the account reaching a limit naming w, or naming no
// window, as a limit naming none holds every window, that came while w runs
// as it does now, since it last started: an older limit's event says nothing
// of when w reached it.
func (d Document) reachedIn(a Account, w quota.Window, now time.Time) (Event, bool) {
	if l := a.Limit; l.ID != 0 && l.Holds(now) && (len(l.Windows) == 0 || slices.Contains(l.Windows, w.Key)) {
		return d.limitEvent(a, now)
	}
	start, _, spanned := w.Span()
	if !spanned {
		return Event{}, false
	}
	i := slices.IndexFunc(d.Events, func(e Event) bool {
		named := len(e.Windows) == 0 || slices.Contains(e.Windows, w.Key)
		return e.Kind == EventLimit && e.Account == a.ID && named && !e.At.Before(start) && !e.At.After(now)
	})
	if i < 0 {
		return Event{}, false
	}
	return d.Events[i], true
}

// limitEvent is the router's event of the limit holding account a at now: the
// one whose identity is its limit's. It reports false where no router limit
// holds the account, as none holds one probed, where its limit has no
// identity, as a router from before limits had them gives none, or where no
// event tells of it.
func (d Document) limitEvent(a Account, now time.Time) (Event, bool) {
	if a.Limit.ID == 0 || !a.Limit.Holds(now) {
		return Event{}, false
	}
	i := slices.IndexFunc(d.Events, func(e Event) bool {
		return e.Kind == EventLimit && e.Account == a.ID && e.Limit == a.Limit.ID
	})
	if i < 0 {
		return Event{}, false
	}
	return d.Events[i], true
}
