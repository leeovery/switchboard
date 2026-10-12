package status_test

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// routed is an account as the router knows it as it builds its document:
// read, as the router last read it, before it's judged as it stands; and how
// fast its session has been used over the last half hour, where the router
// has a recent rate of it, 0 where it has none.
type routed struct {
	status.Account
	rate float64
}

// trails are how the router has read the account's session lately, as
// score.Noted keeps them, where it has a recent rate of it: from where rate
// puts its use the half hour before gen, never below none, to where it was
// last read.
func (r routed) trails(gen time.Time) map[string][]score.Reading {
	session, ok := r.Window(policy.Pressure)
	if !ok || r.rate == 0 {
		return nil
	}
	from := gen.Add(-score.Recent)
	return map[string][]score.Reading{session.Key: {
		{At: from, Utilization: max(session.Utilization-r.rate*score.Recent.Hours(), 0)},
		{At: r.FetchedAt, Utilization: session.Utilization},
	}}
}

// asTheRouterBuilds is the router's document at gen of the accounts given,
// before it's worked out, the global pin naming those pinned, built as the
// router builds its own: each account as it stands at gen, as AsOf has it;
// with a usable token, its pressure, as status.PressureFrom has it, and its
// recent rates, as status.RatesOf has them, over its trails; the best of
// those not shut; and the primary.
func asTheRouterBuilds(gen time.Time, pinned []string, accounts ...routed) status.Document {
	doc := status.Document{GeneratedAt: gen, Source: status.SourceRouter, Router: status.Health{Healthy: true}}
	if len(pinned) > 0 {
		doc.Pin = status.Pin{Accounts: pinned, Since: gen.Add(-time.Hour)}
	}
	var open []status.Account
	for _, r := range accounts {
		a := r.AsOf(policy, gen)
		if a.TokenSet {
			trails := r.trails(gen)
			a.Pressure = status.PressureFrom(policy, status.Pressing{
				ID: a.ID, Reserve: a.Reserve, Latest: r.Windows, Trails: trails, Spent: slices.Contains(pinned, a.ID), Shut: a.Shut(gen, policy),
			}, gen)
			a.Rates = status.RatesOf(r.Windows, trails, gen)
		}
		doc.Accounts = append(doc.Accounts, a)
		if !a.Shut(gen, policy) {
			open = append(open, a)
		}
	}
	doc.Best, doc.Primary = status.Best(policy, open, pinned, gen), status.PrimaryOf(doc.Accounts)
	return doc
}

// asProbed is the document probing builds at gen of the accounts given, each
// read as given, before it's worked out, as Collect builds it: each account
// as it stands at gen, the best of them, and the primary.
func asProbed(gen time.Time, accounts ...status.Account) status.Document {
	doc := status.Document{GeneratedAt: gen, Source: status.SourceProbe}
	for _, a := range accounts {
		doc.Accounts = append(doc.Accounts, a.AsOf(policy, gen))
	}
	doc.Best, doc.Primary = status.Best(policy, doc.Accounts, nil, gen), status.PrimaryOf(doc.Accounts)
	return doc
}

// readEarlier has the account read at the time given, rather than as the
// document was built.
func readEarlier(at time.Time) func(*status.Account) {
	return func(a *status.Account) {
		a.FetchedAt = at
	}
}

// hardMoment is a document the router, or probing, builds, judged at a
// moment a view has got it wrong at before, and what's wanted of it then.
type hardMoment struct {
	name string
	// doc is the document as built, before it's worked out.
	doc status.Document
	now time.Time
	// next is the account ▲ marks.
	next string
	// accounts are what's wanted of each of the document's accounts, by id.
	accounts map[string]accountWant
}

// accountWant is what's wanted of an account at a hard moment.
type accountWant struct {
	routing                                   status.RoutingKind
	held, capHolds, idle, outOfQuota, canRise bool
	underPressure                             bool
	limitSince                                time.Time
	// windows are what's wanted of each of its windows, by key.
	windows map[string]windowWant
}

// String says what's wanted of the account, its times as sinceBuilt has them.
func (a accountWant) String() string {
	return fmt.Sprintf("{routing:%d held:%v capHolds:%v idle:%v outOfQuota:%v canRise:%v underPressure:%v limitSince:%s}",
		a.routing, a.held, a.capHolds, a.idle, a.outOfQuota, a.canRise, a.underPressure, sinceBuilt(a.limitSince))
}

// windowWant is what's wanted of a window at a hard moment.
type windowWant struct {
	kind                   status.WindowKind
	at                     time.Time
	used                   float64
	next                   time.Time
	resetSinceRead, lapsed bool
	held                   bool
	since                  time.Time
	capped, cappedAsRead   bool
	runsOut                bool
	out                    time.Time
}

// String says what's wanted of the window, its times as sinceBuilt has them.
func (w windowWant) String() string {
	return fmt.Sprintf("{kind:%d at:%s used:%v next:%s resetSinceRead:%v lapsed:%v held:%v since:%s capped:%v cappedAsRead:%v runsOut:%v out:%s}",
		w.kind, sinceBuilt(w.at), w.used, sinceBuilt(w.next), w.resetSinceRead, w.lapsed, w.held, sinceBuilt(w.since),
		w.capped, w.cappedAsRead, w.runsOut, sinceBuilt(w.out))
}

// sinceBuilt shows t as how long after the documents were built it is, as
// "built+112h0m0s", or "none" where it's zero.
func sinceBuilt(t time.Time) string {
	if t.IsZero() {
		return "none"
	}
	return "built" + fmt.Sprintf("%+v", t.Sub(built))
}

// hardMoments are the moments views have got the document wrong at: the
// documents the router, or probing, builds then, through AsOf, and what's
// wanted of each fact of them. A row lists every account and every window.
func hardMoments() []hardMoment {
	limited := asTheRouterBuilds(built, nil,
		routed{Account: with(readAt("work", spentIn("5h", 1, 40*time.Minute), windowIn("7d", 0.6, 3*day)), func(a *status.Account) {
			a.Limit = status.Limit{ID: 4, Windows: []string{"5h"}, Until: built.Add(40 * time.Minute)}
		})},
		routed{Account: readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.2, 5*day))},
	)
	limited.Events = []status.Event{{
		ID: 9, At: built.Add(-20 * time.Minute), Kind: status.EventLimit, Account: "work",
		Windows: []string{"5h"}, Until: built.Add(40 * time.Minute), Limit: 4,
	}}
	capped := asTheRouterBuilds(built, nil,
		routed{Account: with(readAt("work", windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.92, time.Hour)), reserving)},
		routed{Account: readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.2, 5*day))},
	)
	lapsed := asTheRouterBuilds(built, nil,
		routed{Account: with(readAt("work", windowIn("5h", 0.5, -time.Hour), windowIn("7d", 0.3, 3*day)), readEarlier(built.Add(-6*time.Hour)))},
		routed{Account: readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.2, 5*day))},
	)
	lapsed.Prime = status.Prime{Day: "08:00-22:00", Window: "5h", Slots: []status.Slot{
		{Account: "work", At: "13:30", Next: built.Add(18 * time.Minute)},
		{Account: "side", At: "16:00", Next: built.Add(5 * time.Hour)},
	}}
	stillSide := map[string]windowWant{
		"5h": {kind: status.WindowOpen, at: built.Add(4 * time.Hour), used: 0.1, next: built.Add(4 * time.Hour)},
		"7d": {kind: status.WindowOpen, at: built.Add(5 * day), used: 0.2, next: built.Add(5 * day)},
	}
	return []hardMoment{
		{
			name: "just after a reset, before the reread",
			doc: asTheRouterBuilds(built, nil,
				routed{Account: readAt("work", windowIn("5h", 0.8, time.Hour), windowIn("7d", 0.5, 2*day))},
				routed{Account: readAt("side", windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.7, 30*time.Minute))},
			),
			now:  built.Add(time.Hour + 5*time.Second),
			next: "side",
			accounts: map[string]accountWant{
				"work": {routing: status.RoutingIdle, idle: true, canRise: true, windows: map[string]windowWant{
					"5h": {kind: status.WindowNotStarted, resetSinceRead: true, lapsed: true},
					"7d": {kind: status.WindowOpen, at: built.Add(2 * day), used: 0.5, next: built.Add(2 * day)},
				}},
				"side": {routing: status.RoutingNext, canRise: true, windows: map[string]windowWant{
					"5h": {kind: status.WindowOpen, at: built.Add(3 * time.Hour), used: 0.3, next: built.Add(3 * time.Hour)},
					"7d": {kind: status.WindowOpen, at: built.Add(30*time.Minute + 7*day), next: built.Add(30*time.Minute + 7*day), resetSinceRead: true},
				}},
			},
		},
		{
			name: "a failed read after an earlier one",
			doc: asTheRouterBuilds(built, nil,
				routed{Account: with(readAt("work", windowIn("5h", 0.7, 2*time.Hour), windowIn("7d", 0.4, 3*day)), func(a *status.Account) {
					a.FetchedAt, a.Error = built.Add(-20*time.Minute), "HTTP 529 · Overloaded"
				}), rate: 0.3},
				routed{Account: readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.2, 5*day))},
			),
			now:  built.Add(5 * time.Minute),
			next: "side",
			accounts: map[string]accountWant{
				"work": {routing: status.RoutingPressed, canRise: true, underPressure: true, windows: map[string]windowWant{
					"5h": {kind: status.WindowStale, at: built.Add(-20 * time.Minute), used: 0.7, next: built.Add(2 * time.Hour), out: built.Add(time.Hour)},
					"7d": {kind: status.WindowStale, at: built.Add(-20 * time.Minute), used: 0.4, next: built.Add(3 * day)},
				}},
				"side": {routing: status.RoutingNext, canRise: true, windows: stillSide},
			},
		},
		{
			name: "its token refused",
			doc: asTheRouterBuilds(built, nil,
				routed{Account: with(readAt("work", windowIn("5h", 0.4, 2*time.Hour), windowIn("7d", 0.5, 2*day)), func(a *status.Account) {
					a.Refused = status.Refusal{Until: built.Add(time.Hour), Status: 401}
				}), rate: 0.4},
				routed{Account: readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.2, 5*day))},
			),
			now:  built.Add(10 * time.Minute),
			next: "side",
			accounts: map[string]accountWant{
				"work": {routing: status.RoutingRefused, windows: map[string]windowWant{
					"5h": {kind: status.WindowStale, at: built, used: 0.4, next: built.Add(2 * time.Hour), out: built.Add(90 * time.Minute)},
					"7d": {kind: status.WindowStale, at: built, used: 0.5, next: built.Add(2 * day)},
				}},
				"side": {routing: status.RoutingNext, canRise: true, windows: stillSide},
			},
		},
		{
			name: "a stale document, built three hours before now",
			doc: asTheRouterBuilds(built, nil,
				routed{Account: readAt("work", windowIn("5h", 0.4, 2*time.Hour), windowIn("7d", 0.6, 4*day))},
				routed{Account: readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.3, 5*day))},
			),
			now:  built.Add(3 * time.Hour),
			next: "side",
			accounts: map[string]accountWant{
				"work": {routing: status.RoutingIdle, idle: true, canRise: true, windows: map[string]windowWant{
					"5h": {kind: status.WindowNotStarted, resetSinceRead: true, lapsed: true},
					"7d": {kind: status.WindowRunsOut, at: built.Add(2 * day), used: 0.6, next: built.Add(4 * day), runsOut: true, out: built.Add(2 * day)},
				}},
				"side": {routing: status.RoutingNext, canRise: true, windows: map[string]windowWant{
					"5h": {kind: status.WindowOpen, at: built.Add(4 * time.Hour), used: 0.1, next: built.Add(4 * time.Hour)},
					"7d": {kind: status.WindowRunsOut, at: built.Add(112 * time.Hour), used: 0.3, next: built.Add(5 * day), runsOut: true, out: built.Add(112 * time.Hour)},
				}},
			},
		},
		{
			name: "its session lapsed",
			doc:  lapsed,
			now:  built.Add(10 * time.Minute),
			next: "work",
			accounts: map[string]accountWant{
				"work": {routing: status.RoutingIdle, idle: true, canRise: true, windows: map[string]windowWant{
					"5h": {kind: status.WindowNotStarted, lapsed: true},
					"7d": {kind: status.WindowOpen, at: built.Add(3 * day), used: 0.3, next: built.Add(3 * day)},
				}},
				"side": {routing: status.RoutingOpen, canRise: true, windows: stillSide},
			},
		},
		{
			name: "a week at its reserve when built",
			doc:  capped,
			now:  built.Add(10 * time.Minute),
			next: "side",
			accounts: map[string]accountWant{
				"work": {routing: status.RoutingCapped, capHolds: true, outOfQuota: true, windows: map[string]windowWant{
					"5h": {kind: status.WindowOpen, at: built.Add(3 * time.Hour), used: 0.3, next: built.Add(3 * time.Hour)},
					"7d": {kind: status.WindowCapped, at: built.Add(time.Hour), used: 0.92, next: built.Add(time.Hour), capped: true, cappedAsRead: true},
				}},
				"side": {routing: status.RoutingNext, canRise: true, windows: stillSide},
			},
		},
		{
			name: "a week at its reserve when built, then reset",
			doc:  capped,
			now:  built.Add(time.Hour + time.Minute),
			next: "side",
			accounts: map[string]accountWant{
				"work": {routing: status.RoutingOpen, canRise: true, windows: map[string]windowWant{
					"5h": {kind: status.WindowOpen, at: built.Add(3 * time.Hour), used: 0.3, next: built.Add(3 * time.Hour)},
					"7d": {kind: status.WindowOpen, at: built.Add(time.Hour + 7*day), next: built.Add(time.Hour + 7*day), resetSinceRead: true, cappedAsRead: true},
				}},
				"side": {routing: status.RoutingNext, canRise: true, windows: stillSide},
			},
		},
		{
			name: "a limit holding",
			doc:  limited,
			now:  built.Add(10 * time.Minute),
			next: "side",
			accounts: map[string]accountWant{
				"work": {routing: status.RoutingLimited, held: true, outOfQuota: true, limitSince: built.Add(-20 * time.Minute), windows: map[string]windowWant{
					"5h": {
						kind: status.WindowLimited, at: built.Add(40 * time.Minute), used: 1, next: built.Add(40 * time.Minute),
						held: true, since: built.Add(-20 * time.Minute),
					},
					"7d": {kind: status.WindowRunsOut, at: built.Add(64 * time.Hour), used: 0.6, next: built.Add(3 * day), runsOut: true, out: built.Add(64 * time.Hour)},
				}},
				"side": {routing: status.RoutingNext, canRise: true, windows: stillSide},
			},
		},
		{
			name: "a limit holding, then lifted",
			doc:  limited,
			now:  built.Add(41 * time.Minute),
			next: "side",
			accounts: map[string]accountWant{
				"work": {routing: status.RoutingIdle, idle: true, canRise: true, windows: map[string]windowWant{
					"5h": {kind: status.WindowNotStarted, resetSinceRead: true, lapsed: true},
					"7d": {kind: status.WindowRunsOut, at: built.Add(64 * time.Hour), used: 0.6, next: built.Add(3 * day), runsOut: true, out: built.Add(64 * time.Hour)},
				}},
				"side": {routing: status.RoutingNext, canRise: true, windows: stillSide},
			},
		},
		{
			name: "probed, of several accounts",
			doc: asProbed(built,
				readAt("work", windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.5, 2*day)),
				readAt("side", windowIn("5h", 0.6, time.Hour), windowIn("7d", 0.05, 6*day)),
			),
			now:  built.Add(time.Minute),
			next: "",
			accounts: map[string]accountWant{
				"work": {routing: status.RoutingOpen, canRise: true, windows: map[string]windowWant{
					"5h": {kind: status.WindowOpen, at: built.Add(3 * time.Hour), used: 0.3, next: built.Add(3 * time.Hour)},
					"7d": {kind: status.WindowOpen, at: built.Add(2 * day), used: 0.5, next: built.Add(2 * day)},
				}},
				"side": {routing: status.RoutingOpen, canRise: true, windows: map[string]windowWant{
					"5h": {kind: status.WindowOpen, at: built.Add(time.Hour), used: 0.6, next: built.Add(time.Hour)},
					"7d": {kind: status.WindowOpen, at: built.Add(6 * day), used: 0.05, next: built.Add(6 * day)},
				}},
			},
		},
		{
			name: "probed, of one account, with no room",
			doc:  asProbed(built, readAt("work", windowIn("5h", 0.3, 3*time.Hour), spentIn("7d", 1, 2*day))),
			now:  built.Add(time.Minute),
			next: "work",
			accounts: map[string]accountWant{
				"work": {routing: status.RoutingLimited, held: true, outOfQuota: true, windows: map[string]windowWant{
					"5h": {kind: status.WindowOpen, at: built.Add(3 * time.Hour), used: 0.3, next: built.Add(3 * time.Hour)},
					"7d": {kind: status.WindowLimited, at: built.Add(2 * day), used: 1, next: built.Add(2 * day), held: true},
				}},
			},
		},
	}
}

func TestEveryFactAtEachHardMoment(t *testing.T) {
	for _, tt := range hardMoments() {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.doc.WorkedOut(policy)
			m := doc.At(tt.now, policy)
			checkMomentAgainstStatus(t, doc, m)
			checkWanted(t, tt, m)
		})
	}
}

// checkWanted checks the moment m against what the hard moment wants of
// each of its accounts and windows, which lists every one.
func checkWanted(t *testing.T, tt hardMoment, m status.Moment) {
	t.Helper()
	if m.Next != tt.next {
		t.Errorf("Next = %q, want %q", m.Next, tt.next)
	}
	if len(m.Accounts) != len(tt.accounts) {
		t.Fatalf("%d accounts, want %d", len(m.Accounts), len(tt.accounts))
	}
	for _, a := range m.Accounts {
		want := tt.accounts[a.ID]
		got := accountWant{
			routing: a.Routing.Kind, held: a.Held, capHolds: a.CapHolds, idle: a.Idle, outOfQuota: a.OutOfQuota, canRise: a.CanRise,
			underPressure: a.Pressure.Under, limitSince: a.LimitSince,
		}
		wantFacts := want
		wantFacts.windows = nil
		if !reflect.DeepEqual(got, wantFacts) {
			t.Errorf("%s stands as %+v, want %+v", a.ID, got, wantFacts)
		}
		if len(a.Windows) != len(want.windows) {
			t.Errorf("%s has %d windows, want %d", a.ID, len(a.Windows), len(want.windows))
		}
		for _, w := range a.Windows {
			got := windowWant{
				kind: w.Kind, at: w.At, used: w.Used, next: w.Next, resetSinceRead: w.ResetSinceRead, lapsed: w.Lapsed,
				held: w.Held, since: w.Since, capped: w.Capped, cappedAsRead: w.CappedAsRead, runsOut: w.RunsOut, out: w.Out.At,
			}
			if want := want.windows[w.Key]; !reflect.DeepEqual(got, want) {
				t.Errorf("%s's %s stands as\n%+v\nwant\n%+v", a.ID, w.Key, got, want)
			}
		}
	}
}

// checkMomentAgainstStatus checks each fact of the moment m of doc against the
// function of status, or of the dashboard, it takes the place of.
func checkMomentAgainstStatus(t *testing.T, doc status.Document, m status.Moment) {
	t.Helper()
	now := m.Now
	if want := doc.GeneratedAt; !m.ReadAt.Equal(want) {
		t.Errorf("ReadAt = %v, want %v: when the document was built", m.ReadAt, want)
	}
	records := status.Moment{
		Now: now, ReadAt: m.ReadAt, Routed: doc.Source == status.SourceRouter, Probed: doc.Source == status.SourceProbe, Fallback: doc.Fallback, Health: doc.Router,
		Restart: doc.Restart, Pin: doc.Pin, Events: doc.Events, Sessions: doc.Sessions,
	}
	got := status.Moment{
		Now: m.Now, ReadAt: m.ReadAt, Routed: m.Routed, Probed: m.Probed, Fallback: m.Fallback, Health: m.Health,
		Restart: m.Restart, Pin: m.Pin, Events: m.Events, Sessions: m.Sessions,
	}
	if !reflect.DeepEqual(got, records) {
		t.Errorf("the records are %+v, want the document's %+v", got, records)
	}
	if want := slices.DeleteFunc(slices.Clone(doc.ComingUp), func(u status.Upcoming) bool { return !u.At.After(now) }); !reflect.DeepEqual(m.ComingUp, want) {
		t.Errorf("ComingUp = %+v, want the document's after now, %+v", m.ComingUp, want)
	}
	if want := dashboardBestOf(doc); m.Next != want {
		t.Errorf("Next = %q, want %q, as the dashboard's bestOf marks ▲", m.Next, want)
	}
	if got, want := m.Pool(), doc.PoolAt(now, policy); !reflect.DeepEqual(got, want) {
		t.Errorf("Pool() = %+v, want %+v, as PoolAt has it", got, want)
	}
	if got, want := m.Coming, doc.Coming(now, policy); !reflect.DeepEqual(got, want) {
		t.Errorf("Coming = %+v, want %+v, as Coming has it", got, want)
	}
	if got, want := m.Text(), doc.Text(now, policy); got != want {
		t.Errorf("Text() =\n%s\nwant\n%s", got, want)
	}
	ids := make([]string, len(doc.Accounts))
	for i, a := range doc.Accounts {
		ids[i] = a.ID
	}
	if got, want := m.Destination(ids), doc.Destination(ids); got != want {
		t.Errorf("Destination(%q) = %q, want %q", ids, got, want)
	}
	shown, hidden := dashboardWindowsOf(doc, now)
	if got := keysOf(m.Shown); !slices.Equal(got, shown) {
		t.Errorf("Shown = %q, want %q", got, shown)
	}
	if got := keysOf(m.Hidden); !slices.Equal(got, hidden) {
		t.Errorf("Hidden = %q, want %q", got, hidden)
	}
	for i, a := range doc.Accounts {
		checkAccountAgainstStatus(t, doc, a, i+1, m)
	}
}

// checkAccountAgainstStatus checks each fact of doc's account a, in the place
// given, of the moment m, against the function it takes the place of.
func checkAccountAgainstStatus(t *testing.T, doc status.Document, a status.Account, place int, m status.Moment) {
	t.Helper()
	now := m.Now
	got, ok := m.Account(a.ID)
	if !ok {
		t.Fatalf("Account(%q) reports none", a.ID)
	}
	hold, held := a.Held(now, policy)
	var refusal status.Refusal
	if a.Refused.Holds(now) {
		refusal = a.Refused
	}
	var capBack time.Time
	if doc.CapHolds(a, now, policy) {
		capBack, _ = score.LetGo(a.Windows, a.Reserve, policy.IsShared, now)
	}
	session, _ := a.Window(a.Pressure.Window)
	pressureOut, _ := doc.RunsOut(a, session, now)
	pool := m.Pool()
	outOfQuota := held && hold.Every || doc.CapHolds(a, now, policy)
	want := status.AccountNow{
		ID: a.ID, Label: a.Label, Place: place, Primary: a.Primary,
		Reserve: a.Reserve, Cap: 1 - a.Reserve, Floor: doc.Floor(a), ReserveHolds: doc.ReserveHolds(a),
		Pinned: doc.Pin.Has(a.ID), Next: a.ID == m.Next,
		InPool: slices.Contains(pool.Accounts, a.ID), OutOfPool: len(pool.Accounts) > 0 && !slices.Contains(pool.Accounts, a.ID),
		TokenSet: a.TokenSet, Read: !a.FetchedAt.IsZero(), ReadAt: a.FetchedAt, Error: status.Clean(a.Error), Failures: a.Failures, Sessions: a.Sessions,
		Routing: doc.RoutingOf(a, now, policy), Hold: hold, Held: held, LimitSince: got.LimitSince,
		ReachedReserve: a.ReachedReserve(now), CapHolds: doc.CapHolds(a, now, policy), CapBack: capBack,
		Refusal: refusal, TokenRefused: a.TokenRefused(now),
		Pressure: status.PressureNow{Under: doc.Pressed(a, now) != "", Window: a.Pressure.Window, Out: pressureOut},
		Idle:     a.HasLapsed(policy.Started, now, policy), NextPrime: primeOf(doc, a.ID),
		OutOfQuota: outOfQuota, CanRise: !outOfQuota && !a.TokenRefused(now),
		Windows: got.Windows,
	}
	if gotFacts := withoutUnreported(got); !reflect.DeepEqual(gotFacts, want) {
		t.Errorf("%s stands as\n%+v\nwant\n%+v", a.ID, gotFacts, want)
	}
	for i, w := range a.Windows {
		checkWindowAgainstStatus(t, doc, a, w, got.Windows[i], now)
	}
}

// withoutUnreported is the account a as it stands, as its exported fields
// alone have it.
func withoutUnreported(a status.AccountNow) status.AccountNow {
	return status.AccountNow{
		ID: a.ID, Label: a.Label, Place: a.Place, Primary: a.Primary,
		Reserve: a.Reserve, Cap: a.Cap, Floor: a.Floor, ReserveHolds: a.ReserveHolds,
		Pinned: a.Pinned, Next: a.Next, InPool: a.InPool, OutOfPool: a.OutOfPool,
		TokenSet: a.TokenSet, Read: a.Read, ReadAt: a.ReadAt, Error: a.Error, Failures: a.Failures, Sessions: a.Sessions,
		Routing: a.Routing, Hold: a.Hold, Held: a.Held, LimitSince: a.LimitSince,
		ReachedReserve: a.ReachedReserve, CapHolds: a.CapHolds, CapBack: a.CapBack,
		Refusal: a.Refusal, TokenRefused: a.TokenRefused, Pressure: a.Pressure,
		Idle: a.Idle, NextPrime: a.NextPrime, OutOfQuota: a.OutOfQuota, CanRise: a.CanRise, Windows: a.Windows,
	}
}

// checkWindowAgainstStatus checks each fact of doc's account a's window w,
// read, standing at now as got, against the function it takes the place of.
func checkWindowAgainstStatus(t *testing.T, doc status.Document, a status.Account, w quota.Window, got status.WindowNow, now time.Time) {
	t.Helper()
	st := doc.WindowStatesOf(a, now, policy).Of(w.Key)
	want := status.WindowNow{
		WindowName: got.WindowName,
		Shared:     policy.IsShared(w.Key), Started: w.Key == policy.Started, Pressure: w.Key == policy.Pressure,
		Kind: st.Kind, At: st.At,
		Read: status.AsRead{Used: w.Utilization, ResetsAt: w.ResetsAt, At: a.FetchedAt},
	}
	if st.Kind != status.WindowTokenless && st.Kind != status.WindowUnread {
		stands, _ := a.WindowAt(w.Key, now, policy)
		out, runsOut := doc.RunsOut(a, w, now)
		want.Used, want.Pace, want.Next = st.Used, st.Pace, stands.ResetsAt
		want.Allowance = got.Allowance
		if st.Kind == status.WindowOpen {
			want.Allowance = st.Allowance
		}
		want.Room, _ = doc.RoomOf(a, w.Key, now, policy)
		want.Start, want.Length, _ = stands.Span()
		want.ResetSinceRead, want.Lapsed = w.ResetBy(now), a.HasLapsed(w.Key, now, policy)
		want.Held = st.Kind == status.WindowLimited
		if want.Held {
			want.Back, want.Since = st.At, got.Since
		}
		want.Capped = doc.ReserveHolds(a) && slices.Contains(a.ReachedReserve(now), w.Key)
		want.CappedAsRead = doc.ReserveHolds(a) && slices.Contains(a.AtReserve, w.Key)
		want.RunsOut, want.Out = st.Kind == status.WindowRunsOut, out
		if share, ok := dashboardProjected(st, out.Projection); ok {
			want.Projected = &share
		}
		want.Nearing = want.Projected != nil && !runsOut && *want.Projected >= doc.Floor(a)-status.Near-score.Tolerance
		want.InUse = dashboardInUse(stands, doc.Project(a, w, now).Projection)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s's %s stands as\n%+v\nwant\n%+v", a.ID, w.Key, got, want)
	}
}

// primeOf is when doc says the router next primes the account with the given
// id, as its daily prime has it.
func primeOf(doc status.Document, id string) time.Time {
	i := slices.IndexFunc(doc.Prime.Slots, func(s status.Slot) bool { return s.Account == id })
	if i < 0 {
		return time.Time{}
	}
	return doc.Prime.Slots[i].Next
}

// dashboardProjected is the share a window standing as st, heading as p
// says, is heading for by its reset, as the dashboard's standing.projected
// has it: all of it where it runs out; none while it's held at its limit or
// not started, or where p says nothing.
func dashboardProjected(st status.WindowState, p score.Projection) (float64, bool) {
	switch {
	case st.Kind == status.WindowLimited || st.Kind == status.WindowNotStarted:
		return 0, false
	case p.Kind == score.RunsOut:
		return 1, true
	case p.Kind == score.OnPace:
		return p.AtReset, true
	default:
		return 0, false
	}
}

// dashboardInUse reports whether a window standing as stands, heading as p
// says, is in use, as the dashboard's inUse has it.
func dashboardInUse(stands quota.Window, p score.Projection) bool {
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

// dashboardWindowsOf are the keys of doc's accounts' windows shown at now,
// and those hidden, as the dashboard's windowsOf has them.
func dashboardWindowsOf(doc status.Document, now time.Time) (shown, hidden []string) {
	used := make(map[string]bool)
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			stands, _ := a.WindowAt(w.Key, now, policy)
			used[w.Key] = used[w.Key] || policy.IsShared(w.Key) || dashboardInUse(stands, doc.Project(a, w, now).Projection)
		}
		for _, f := range a.Failures {
			used[f.Window] = true
		}
	}
	for key, inUse := range used {
		if inUse {
			shown = append(shown, key)
		} else {
			hidden = append(hidden, key)
		}
	}
	slices.SortFunc(shown, quota.CompareKeys)
	slices.SortFunc(hidden, quota.CompareKeys)
	return shown, hidden
}

// dashboardBestOf is the account ▲ marks, as the dashboard's bestOf has it:
// with one account, that one; of several, the router's best, and none in a
// document built by probing.
func dashboardBestOf(doc status.Document) string {
	switch {
	case len(doc.Accounts) == 1:
		return doc.Accounts[0].ID
	case doc.Source != status.SourceRouter:
		return ""
	}
	return doc.Best
}

// keysOf are the keys of the windows named.
func keysOf(names []status.WindowName) []string {
	var keys []string
	for _, n := range names {
		keys = append(keys, n.Key)
	}
	return keys
}

func TestADocumentWorkedOutIsAsItStoodWhenItWasBuilt(t *testing.T) {
	for _, tt := range hardMoments() {
		t.Run(tt.name, func(t *testing.T) {
			worked := tt.doc.WorkedOut(policy)
			m := tt.doc.At(tt.doc.GeneratedAt, policy)
			if got, want := worked.Pool, m.Pool(); !reflect.DeepEqual(got, want) {
				t.Errorf("worked out, the pool is %+v, want %+v, as it stood as the document was built", got, want)
			}
			for i, a := range worked.Accounts {
				for j, w := range a.Windows {
					if now := m.Accounts[i].Windows[j]; !reflect.DeepEqual(w.Pace, now.Pace) || w.Allowance != now.Allowance {
						t.Errorf("worked out, %s's %s has pace %s and allowance %+v, want %s and %+v, as it stood as the document was built",
							a.ID, w.Key, paceOf(w.Pace), w.Allowance, paceOf(now.Pace), now.Allowance)
					}
				}
			}
		})
	}
}

func TestAWindowIsNamedByItsHoursElseByItsLabelElseByItsKey(t *testing.T) {
	tests := []struct {
		name, key, label, want string
	}{
		{name: "whole hours under a day, by them", key: "5h", label: "Session", want: "5-hour"},
		{name: "whole hours under a day, unlabelled", key: "5h", want: "5-hour"},
		{name: "a week, by its label", key: "7d", label: "Week", want: "Week"},
		{name: "a model's week, by its label", key: "7d_oi", label: "Fable week", want: "Fable week"},
		{name: "a day, by its label, as it's no shorter than one", key: "1d", label: "Day", want: "Day"},
		{name: "unlabelled, by its key", key: "7d", want: "7d"},
		{name: "its label cleaned", key: "7d", label: "\x1bWeek\a", want: "Week"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.NameOfWindow(tt.key, tt.label); got != tt.want {
				t.Errorf("NameOfWindow(%q, %q) = %q, want %q", tt.key, tt.label, got, tt.want)
			}
		})
	}
}

func TestTheWindowsShownAndHiddenAreNamed(t *testing.T) {
	doc := routerDocOf(
		with(readAt("work", windowIn("5h", 0.2, 3*time.Hour), windowIn("7d", 0.4, 2*day), windowIn("7d_oi", 0, 2*day)), func(a *status.Account) {
			a.Failures = []quota.Failure{{Label: "Sonnet", Window: "7d_sonnet", Error: "HTTP 529 · Overloaded"}}
		}),
		readAt("side", quota.Window{Key: "5h", Utilization: 0.1, ResetsAt: built.Add(time.Hour)}, windowIn("7d_oi", 0, 2*day)),
	)
	m := doc.At(built, policy)
	wantShown := []status.WindowName{
		{Key: "5h", Label: "Session", Name: "5-hour"},
		{Key: "7d", Label: "Week", Name: "Week"},
		{Key: "7d_sonnet", Label: "7d_sonnet", Name: "7d_sonnet"},
	}
	if !reflect.DeepEqual(m.Shown, wantShown) {
		t.Errorf("Shown = %+v, want %+v: every model's windows, and one a probe couldn't read, named by their first label", m.Shown, wantShown)
	}
	if want := []status.WindowName{{Key: "7d_oi", Label: "Fable week", Name: "Fable week"}}; !reflect.DeepEqual(m.Hidden, want) {
		t.Errorf("Hidden = %+v, want %+v: a model's own window unused on every account", m.Hidden, want)
	}
	work, _ := m.Account("work")
	if w, _ := work.Window("7d_oi"); w.Name != "Fable week" || w.Label != "Fable week" {
		t.Errorf("work's Fable week is named %q, labelled %q; want it named as the moment names it", w.Name, w.Label)
	}
}

func TestAWindowAnAccountHasntReportedStandsUnread(t *testing.T) {
	doc := routerDocOf(readAt("work", windowIn("5h", 0.2, 3*time.Hour)), status.Account{ID: "personal", Label: "personal"})
	m := doc.At(built, policy)
	tests := []struct {
		account string
		want    status.WindowKind
	}{
		{account: "work", want: status.WindowUnread},
		{account: "personal", want: status.WindowTokenless},
	}
	for _, tt := range tests {
		a, _ := m.Account(tt.account)
		if w, ok := a.Window("7d"); ok || w.Key != "7d" || w.Kind != tt.want {
			t.Errorf("%s's week, unreported, stands as %+v, %v; want of kind %d alone, and false", tt.account, w, ok, tt.want)
		}
	}
	if _, ok := m.Account("spare"); ok {
		t.Errorf("Account(\"spare\") reports one, want none: the document has no such account")
	}
}

func TestAnAccountIsReadWhereItsUsageWasReadWhateverWindowsItReported(t *testing.T) {
	windowsUnread := readAt("work", windowIn("5h", 0.2, 3*time.Hour))
	windowsUnread.FetchedAt = time.Time{}
	failedFirst := status.Account{ID: "spare", Label: "spare", TokenSet: true, Error: "usage read failed"}
	failedAfter := readAt("other", windowIn("5h", 0.2, 3*time.Hour))
	failedAfter.Error = "usage read failed"
	doc := routerDocOf(windowsUnread, readAt("personal"), failedFirst, failedAfter)
	m := doc.At(built, policy)
	tests := []struct {
		account string
		read    bool
		routing status.RoutingKind
	}{
		{account: "work", read: false, routing: status.RoutingUnread},
		{account: "personal", read: true, routing: status.RoutingOpen},
		{account: "spare", read: false, routing: status.RoutingUnreadable},
		{account: "other", read: true, routing: status.RoutingOpen},
	}
	for _, tt := range tests {
		a, _ := m.Account(tt.account)
		if a.Read != tt.read || a.Routing.Kind != tt.routing {
			t.Errorf("%s: Read = %v, routing of kind %d; want %v, of kind %d", tt.account, a.Read, a.Routing.Kind, tt.read, tt.routing)
		}
	}
	work, _ := m.Account("work")
	if w := work.Windows[0]; w.Kind != status.WindowUnread || w.Room != 0 {
		t.Errorf("work's session, its usage not read, stands as %+v; want unread, with no room", w)
	}
}

func TestWhenALimitHoldingAWindowBegan(t *testing.T) {
	limit := func(id int, windows ...string) func(*status.Account) {
		return func(a *status.Account) {
			a.Limit = status.Limit{ID: id, Windows: windows, Until: built.Add(time.Hour)}
		}
	}
	reached := func(at time.Duration, limit int, windows ...string) status.Event {
		return status.Event{At: built.Add(at), Kind: status.EventLimit, Account: "work", Windows: windows, Limit: limit}
	}
	spent := readAt("work", spentIn("5h", 1, time.Hour), windowIn("7d", 0.5, 2*day))
	tests := []struct {
		name    string
		account status.Account
		events  []status.Event
		// since is when the session reached its limit, and limitSince when
		// the account reached the router's.
		since, limitSince time.Time
	}{
		{
			name:    "the router's limit, as its event tells, though a newer one names the window",
			account: with(spent, limit(4, "5h")),
			events:  []status.Event{reached(-5*time.Minute, 5, "5h"), reached(-20*time.Minute, 4, "5h")},
			since:   built.Add(-20 * time.Minute), limitSince: built.Add(-20 * time.Minute),
		},
		{
			name:    "the router's limit, of which no event tells",
			account: with(spent, limit(4, "5h")),
			events:  []status.Event{reached(-5*time.Minute, 5, "5h")},
		},
		{
			name:    "a limit from a router from before limits had an identity: the newest event naming the window",
			account: with(spent, limit(0, "5h")),
			events:  []status.Event{reached(-5*time.Minute, 0, "5h"), reached(-20*time.Minute, 0, "5h")},
			since:   built.Add(-5 * time.Minute),
		},
		{
			name:    "a window read spent, probed: the newest event naming no window",
			account: spent,
			events:  []status.Event{reached(-10*time.Minute, 0), reached(-20*time.Minute, 0, "5h")},
			since:   built.Add(-10 * time.Minute),
		},
		{
			name:    "an event from before the window started",
			account: spent,
			events:  []status.Event{reached(-5*time.Hour, 0, "5h")},
		},
		{
			name:    "an event of another account",
			account: spent,
			events:  []status.Event{{At: built.Add(-time.Minute), Kind: status.EventLimit, Account: "side", Windows: []string{"5h"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := routerDocOf(tt.account)
			doc.Events = tt.events
			a, _ := doc.At(built, policy).Account("work")
			w, _ := a.Window("5h")
			if !w.Held || !w.Since.Equal(tt.since) || !a.LimitSince.Equal(tt.limitSince) {
				t.Errorf("the session, held %v, reached its limit at %v, the account the router's at %v; want held, at %v and %v",
					w.Held, w.Since, a.LimitSince, tt.since, tt.limitSince)
			}
		})
	}
}

func TestAWindowNotHeldBeganNoLimit(t *testing.T) {
	doc := routerDocOf(readAt("work", windowIn("5h", 0.5, time.Hour)))
	doc.Events = []status.Event{{At: built.Add(-time.Minute), Kind: status.EventLimit, Account: "work", Windows: []string{"5h"}}}
	a, _ := doc.At(built, policy).Account("work")
	if w, _ := a.Window("5h"); w.Held || !w.Since.IsZero() || !w.Back.IsZero() {
		t.Errorf("the session, with room, stands held %v, back %v, since %v; want none of it", w.Held, w.Back, w.Since)
	}
}

func TestAMomentsPoolIsThePoolAtItsMomentOnceWorkedOut(t *testing.T) {
	doc := fullDocument()
	m := doc.At(built.Add(time.Hour), policy)
	first := m.Pool()
	if want := doc.PoolAt(built.Add(time.Hour), policy); !reflect.DeepEqual(first, want) {
		t.Errorf("Pool() = %+v, want %+v, as PoolAt has it", first, want)
	}
	copied := m
	if again := copied.Pool(); !reflect.DeepEqual(again, first) {
		t.Errorf("Pool() asked again = %+v, want %+v", again, first)
	}
	if got := (status.Moment{}).Pool(); !reflect.DeepEqual(got, status.Pool{}) {
		t.Errorf("the zero moment's Pool() = %+v, want none", got)
	}
}

// A window runs out where a view says so only where nothing before running
// out holds, the first of the kinds that holds winning: a limit, a refused
// token or a failed read says more than its projection, which it keeps for
// the bar's projected fill.
func TestAWindowRunsOutWhereNothingBeforeItHolds(t *testing.T) {
	// The week, 60% used three days in, runs out two days on at its pace.
	running := readAt("work", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.6, 4*day))
	out := built.Add(2 * day)
	tests := []struct {
		name    string
		account status.Account
		kind    status.WindowKind
		runsOut bool
	}{
		{name: "with nothing before it", account: running, kind: status.WindowRunsOut, runsOut: true},
		{
			name:    "its last read failed",
			account: with(running, func(a *status.Account) { a.Error = "HTTP 529 · Overloaded" }),
			kind:    status.WindowStale,
		},
		{
			name:    "its token refused",
			account: with(running, func(a *status.Account) { a.Refused = status.Refusal{Until: built.Add(time.Hour), Status: 401} }),
			kind:    status.WindowStale,
		},
		{
			name:    "a limit naming no window holding it",
			account: with(running, func(a *status.Account) { a.Limit = status.Limit{Until: built.Add(time.Hour)} }),
			kind:    status.WindowLimited,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := asTheRouterBuilds(built, nil, routed{Account: tt.account}).WorkedOut(policy).At(built.Add(time.Minute), policy).Account("work")
			w, _ := a.Window("7d")
			if w.Kind != tt.kind || w.RunsOut != tt.runsOut || !w.Out.At.Equal(out) {
				t.Errorf("the week stands as kind %d, running out %v, its projection out at %v; want kind %d, %v, and out at %v",
					w.Kind, w.RunsOut, w.Out.At, tt.kind, tt.runsOut, out)
			}
			if tt.runsOut && !w.At.Equal(w.Out.At) {
				t.Errorf("the week runs out at %v, want %v, as its projection has it", w.At, w.Out.At)
			}
		})
	}
}

// The pressure line's time comes from where the pressure window runs out,
// as RunsOut has it, not from the time the router gave: for any document
// the router builds, they're one time, as the router's pressure goes by the
// same projection (design, how the numbers are worked out). One time to the
// nanosecond's rounding: the router goes to where the account runs out at
// once, and RunsOut to the window's limit, then back to it, rounding twice.
func TestThePressureLinesTimeIsTheRoutersForAnyDocumentItBuilds(t *testing.T) {
	const rounding = time.Nanosecond
	r := rand.New(rand.NewPCG(1, 2))
	reserves := []float64{0, 0.05, 0.1, 0.2}
	for i := range 2000 {
		session := windowIn("5h", r.Float64()*0.99, time.Duration(r.Int64N(int64(6*time.Hour)))-time.Hour)
		account := routed{Account: with(readAt("work", session, windowIn("7d", r.Float64(), 3*day)), func(a *status.Account) {
			a.Reserve = reserves[r.IntN(len(reserves))]
		})}
		if r.IntN(3) > 0 {
			account.rate = r.Float64() * 0.6
		}
		var pinned []string
		if r.IntN(2) == 0 {
			pinned = []string{"work"}
		}
		doc := asTheRouterBuilds(built, pinned, account, routed{Account: readAt("side", windowIn("5h", 0.1, 4*time.Hour))}).WorkedOut(policy)
		stored := doc.Accounts[0].Pressure
		want := stored.RunsOut
		if !want.Before(session.ResetsAt) {
			want = time.Time{}
		}
		a, _ := doc.At(built, policy).Account("work")
		if got := a.Pressure.Out.At; got.IsZero() != want.IsZero() || got.Sub(want).Abs() > rounding {
			t.Fatalf("case %d, %+v, rate %v, reserve %v, pinned %v: the pressure window runs out at %v, want %v, as the router gave it",
				i, session, stored.Rate, a.Reserve, pinned != nil, got, want)
		}
		if !stored.Under {
			continue
		}
		verb := "runs out"
		if a.Pressure.Out.Reserve {
			verb = "at its reserve"
		}
		if got, want := doc.Pressed(doc.Accounts[0], built), "under pressure: "+verb+" ~"+status.TimeOfDay(built, stored.RunsOut); got != want {
			t.Fatalf("case %d: Pressed() = %q, want %q", i, got, want)
		}
	}
}

// One rule says which account new sessions go to, which ▲ marks, and the
// words that say so follow it: a probed document of several accounts marks
// none, as where new sessions go is the router's to say; a lone account
// always carries it, whatever its room.
func TestNewSessionsGoWhereTheRouterSendsThem(t *testing.T) {
	work := with(readNow(0.58, 0.34, 0.12), pressing)
	side := status.Account{ID: "side", Label: "side", TokenSet: true, FetchedAt: stateNow.UTC()}
	tests := []struct {
		name   string
		source string
		best   string
		others []status.Account
		// account is the account whose words are said, the document's first.
		account status.Account
		next    string
		// routing and state are what ROUTING and a card say of the account.
		routing, state string
	}{
		{
			name: "the router's, of several, to another", source: status.SourceRouter, best: "side", others: []status.Account{side},
			account: work, next: "side", routing: "under pressure · out ~16:05", state: "under pressure · new sessions go elsewhere",
		},
		{
			name: "the router's, of several, here", source: status.SourceRouter, best: "work", others: []status.Account{side},
			account: readNow(0.12, 0.33, 0.04), next: "work", routing: "open · new sessions come here", state: "open · new sessions come here",
		},
		{
			name: "probed, of several: none", source: status.SourceProbe, best: "work", others: []status.Account{side},
			account: readNow(0.12, 0.33, 0.04), routing: "open", state: "open",
		},
		{
			name: "probed, of several, another its best: none, so nothing says new sessions go elsewhere", source: status.SourceProbe, best: "side",
			others: []status.Account{side}, account: work, routing: "under pressure · out ~16:05", state: "under pressure · runs out ~16:05 at this pace",
		},
		{
			name: "the router's, of one with no room: that one", source: status.SourceRouter,
			account: readNow(1, 0.34, 0.12), next: "work", routing: "limit reached · back 17:10", state: "limit reached · back 17:10, in 2h 28m",
		},
		{
			name: "probed, of one: that one", source: status.SourceProbe, best: "work",
			account: readNow(0.12, 0.33, 0.04), next: "work", routing: "open", state: "open",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{GeneratedAt: stateNow.UTC(), Source: tt.source, Best: tt.best, Accounts: append([]status.Account{tt.account}, tt.others...)}
			m := doc.At(stateNow, policy)
			if m.Next != tt.next {
				t.Errorf("Next = %q, want %q", m.Next, tt.next)
			}
			for _, a := range m.Accounts {
				if a.Next != (a.ID == tt.next) {
					t.Errorf("%s's Next = %v, want %v", a.ID, a.Next, a.ID == tt.next)
				}
			}
			if got := doc.RoutingOf(tt.account, stateNow, policy).String(); got != tt.routing {
				t.Errorf("RoutingOf() = %q, want %q", got, tt.routing)
			}
			if got := doc.StateOf(tt.account, stateNow, policy).String(); got != tt.state {
				t.Errorf("StateOf() = %q, want %q", got, tt.state)
			}
		})
	}
}

func TestAMomentSaysWhereItsDocumentCameFrom(t *testing.T) {
	tests := []struct {
		source         string
		routed, probed bool
	}{
		{source: status.SourceRouter, routed: true},
		{source: status.SourceProbe, probed: true},
		{source: ""},
	}
	for _, tt := range tests {
		m := status.Document{Source: tt.source}.At(built, policy)
		if m.Routed != tt.routed || m.Probed != tt.probed {
			t.Errorf("of a document from %q, Routed = %v and Probed = %v; want %v and %v", tt.source, m.Routed, m.Probed, tt.routed, tt.probed)
		}
	}
}

func TestAnAccountIsInThePoolWhereThePoolTakesItIn(t *testing.T) {
	work, side := readAt("work", windowIn("5h", 0.2, 3*time.Hour)), readAt("side", windowIn("5h", 0.1, 4*time.Hour))
	tests := []struct {
		name     string
		accounts []status.Account
		pin      []string
		in, out  []string
	}{
		{name: "several, nothing pinned: every one", accounts: []status.Account{work, side}, in: []string{"work", "side"}},
		{name: "several, one pinned: that one", accounts: []status.Account{work, side}, pin: []string{"side"}, in: []string{"side"}, out: []string{"work"}},
		{name: "several, pinned to none of them: no pool", accounts: []status.Account{work, side}, pin: []string{"spare"}},
		{name: "one: no pool", accounts: []status.Account{work}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := routerDocOf(tt.accounts...)
			if tt.pin != nil {
				doc.Pin = status.Pin{Accounts: tt.pin}
			}
			for _, a := range doc.At(built, policy).Accounts {
				in, out := slices.Contains(tt.in, a.ID), slices.Contains(tt.out, a.ID)
				if a.InPool != in || a.OutOfPool != out {
					t.Errorf("%s: InPool = %v, OutOfPool = %v; want %v and %v", a.ID, a.InPool, a.OutOfPool, in, out)
				}
			}
		})
	}
}

func TestAnAccountIsOutOfQuotaWhereALimitOrItsCapHoldsItBackFromEveryRequest(t *testing.T) {
	limit := func(windows ...string) func(*status.Account) {
		return func(a *status.Account) {
			a.Limit = status.Limit{ID: 1, Windows: windows, Until: built.Add(time.Hour)}
		}
	}
	refused := func(a *status.Account) {
		a.Refused = status.Refusal{Until: built.Add(time.Hour), Status: 401}
	}
	session, week, fableWeek := windowIn("5h", 0.3, 3*time.Hour), windowIn("7d", 0.5, 2*day), windowIn("7d_oi", 0.2, 4*day)
	atItsReserve := with(readAt("work", session, windowIn("7d", 0.92, 2*day)), reserving)
	tests := []struct {
		name    string
		account status.Account
		pinned  bool
		// outOfQuota is whether its quota has run out a minute after the
		// document was built, and canRise whether it can take a request then.
		outOfQuota, canRise bool
	}{
		{name: "nothing holding it back", account: readAt("work", session, week), canRise: true},
		{name: "a limit naming no window, its windows read with room", account: with(readAt("work", session, week), limit()), outOfQuota: true},
		{name: "a limit naming the week, which every model shares", account: with(readAt("work", session, week), limit("7d")), outOfQuota: true},
		{name: "a limit naming a model's own week", account: with(readAt("work", session, week, fableWeek), limit("7d_oi")), canRise: true},
		{name: "a window every model shares read spent", account: readAt("work", spentIn("5h", 1, 3*time.Hour), week), outOfQuota: true},
		{name: "a window every model shares read spent, reset since", account: readAt("work", spentIn("5h", 1, 30*time.Second), week), canRise: true},
		{name: "a model's own window read spent", account: readAt("work", session, week, spentIn("7d_oi", 1, 4*day)), canRise: true},
		{name: "at its reserve", account: atItsReserve, outOfQuota: true},
		{name: "at its reserve, pinned, which spends it", account: atItsReserve, pinned: true, canRise: true},
		{name: "its token refused alone, which isn't quota", account: with(readAt("work", session, week), refused)},
		{name: "its token refused, and a window every model shares read spent", account: with(readAt("work", spentIn("5h", 1, 3*time.Hour), week), refused), outOfQuota: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pinned []string
			if tt.pinned {
				pinned = []string{tt.account.ID}
			}
			m := asTheRouterBuilds(built, pinned, routed{Account: tt.account}).WorkedOut(policy).At(built.Add(time.Minute), policy)
			if a := m.Accounts[0]; a.OutOfQuota != tt.outOfQuota || a.CanRise != tt.canRise {
				t.Errorf("OutOfQuota = %v, CanRise = %v; want %v, %v", a.OutOfQuota, a.CanRise, tt.outOfQuota, tt.canRise)
			}
		})
	}
}

func TestAMomentAtAnotherNowIsItsDocumentAtThatNow(t *testing.T) {
	for _, tt := range hardMoments() {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.doc.WorkedOut(policy)
			got, want := doc.At(built, policy).At(tt.now), doc.At(tt.now, policy)
			if !reflect.DeepEqual(factsOf(got), factsOf(want)) {
				t.Errorf("as built, then at now, the moment is\n%+v\nwant\n%+v", factsOf(got), factsOf(want))
			}
			if !reflect.DeepEqual(got.Pool(), want.Pool()) {
				t.Errorf("as built, then at now, the pool is %+v, want %+v", got.Pool(), want.Pool())
			}
		})
	}
}

// factsOf is the moment m as its fields alone have it.
func factsOf(m status.Moment) status.Moment {
	return status.Moment{
		Now: m.Now, ReadAt: m.ReadAt, Routed: m.Routed, Probed: m.Probed, Fallback: m.Fallback, Health: m.Health, Restart: m.Restart, Pin: m.Pin,
		Events: m.Events, Sessions: m.Sessions, ComingUp: m.ComingUp, Next: m.Next, Accounts: m.Accounts,
		Shown: m.Shown, Hidden: m.Hidden, Coming: m.Coming,
	}
}

func TestAnAccountAtAMomentIsTitledAsItsAccountIs(t *testing.T) {
	work := readAt("work", windowIn("5h", 0.3, 3*time.Hour))
	work.Label = "Work\tplace"
	a := asProbed(built, work).At(built, policy).Accounts[0]
	if got, want := a.Title(), "work · Work place"; got != want || got != work.Title() {
		t.Errorf("Title() = %q, want %q, as the account's Title has it, %q", got, want, work.Title())
	}
}
