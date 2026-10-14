package status_test

import (
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
)

func TestEveryFactAtEachHardMoment(t *testing.T) {
	for _, hm := range statustest.HardMoments(policy) {
		t.Run(hm.Name, func(t *testing.T) {
			m := hm.Doc.At(hm.Now, policy)
			checkMomentAgainstStatus(t, hm.Doc, m)
			checkWanted(t, hm.Want, m)
		})
	}
}

// checkWanted checks the moment m against what's wanted of it, which lists
// every account and every window.
func checkWanted(t *testing.T, want statustest.Facts, m status.Moment) {
	t.Helper()
	got := statustest.FactsOf(m)
	if got.Next != want.Next {
		t.Errorf("Next = %q, want %q", got.Next, want.Next)
	}
	if len(got.Accounts) != len(want.Accounts) {
		t.Fatalf("%d accounts, want %d", len(got.Accounts), len(want.Accounts))
	}
	for _, a := range m.Accounts {
		got, want := got.Accounts[a.ID], want.Accounts[a.ID]
		if gotFacts, wantFacts := withoutWindows(got), withoutWindows(want); !reflect.DeepEqual(gotFacts, wantFacts) {
			t.Errorf("%s stands as %v, want %v", a.ID, gotFacts, wantFacts)
		}
		if len(got.Windows) != len(want.Windows) {
			t.Errorf("%s has %d windows, want %d", a.ID, len(got.Windows), len(want.Windows))
		}
		for _, w := range a.Windows {
			if got, want := got.Windows[w.Key], want.Windows[w.Key]; !reflect.DeepEqual(got, want) {
				t.Errorf("%s's %s stands as\n%v\nwant\n%v", a.ID, w.Key, got, want)
			}
		}
	}
}

// withoutWindows is how the account a stands, but for its windows.
func withoutWindows(a statustest.AccountFacts) statustest.AccountFacts {
	a.Windows = nil
	return a
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
	var pressureOut status.RunOut
	if session, ok := a.Window(a.Pressure.Window); ok {
		pressureOut, _ = doc.RunsOut(a, session, now)
	}
	pool := m.Pool()
	outOfQuota := held && hold.Every || doc.CapHolds(a, now, policy)
	var lifts []time.Time
	if held && hold.Every {
		lifts = append(lifts, hold.Until)
	}
	if doc.CapHolds(a, now, policy) {
		lifts = append(lifts, capBack)
	}
	if a.TokenRefused(now) {
		lifts = append(lifts, a.Refused.Until)
	}
	limit, _ := doc.LimitEvent(a, now)
	want := status.AccountNow{
		ID: a.ID, Label: a.Label, Place: place, Primary: a.Primary,
		Reserve: a.Reserve, Cap: 1 - a.Reserve, Floor: doc.Floor(a), ReserveHolds: doc.ReserveHolds(a),
		Pinned: doc.Pin.Has(a.ID), Next: a.ID == m.Next, Elsewhere: m.Next != "" && a.ID != m.Next,
		InPool: slices.Contains(pool.Accounts, a.ID), OutOfPool: len(pool.Accounts) > 0 && !slices.Contains(pool.Accounts, a.ID),
		TokenSet: a.TokenSet, Read: !a.FetchedAt.IsZero(), ReadAt: a.FetchedAt, Error: prose.Clean(a.Error), Failures: a.Failures, Sessions: a.Sessions, Extra: a.Extra,
		Routing: got.Routing, Hold: hold, Held: held, LimitHolds: a.Limit.Holds(now), LimitSince: got.LimitSince, LimitMoved: limit.Count,
		ReachedReserve: a.ReachedReserve(now), CapHolds: doc.CapHolds(a, now, policy), CapSince: got.CapSince, CapBack: capBack,
		Refusal: refusal, RefusedSince: got.RefusedSince, TokenRefused: a.TokenRefused(now),
		Pressure: status.PressureNow{Under: a.UnderPressure(now), Window: a.Pressure.Window, Out: pressureOut},
		Idle:     a.HasLapsed(policy.Started, now, policy), NextPrime: primeOf(doc, a.ID),
		OutOfQuota: outOfQuota, CanRise: a.TokenSet && !a.FetchedAt.IsZero() && !outOfQuota && !a.TokenRefused(now),
		Back:    lastOf(lifts),
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
		Pinned: a.Pinned, Next: a.Next, Elsewhere: a.Elsewhere, InPool: a.InPool, OutOfPool: a.OutOfPool,
		TokenSet: a.TokenSet, Read: a.Read, ReadAt: a.ReadAt, Error: a.Error, Failures: a.Failures, Sessions: a.Sessions, Extra: a.Extra,
		Routing: a.Routing, Hold: a.Hold, Held: a.Held, LimitHolds: a.LimitHolds, LimitSince: a.LimitSince, LimitMoved: a.LimitMoved,
		ReachedReserve: a.ReachedReserve, CapHolds: a.CapHolds, CapSince: a.CapSince, CapBack: a.CapBack,
		Refusal: a.Refusal, RefusedSince: a.RefusedSince, TokenRefused: a.TokenRefused, Pressure: a.Pressure,
		Idle: a.Idle, NextPrime: a.NextPrime, OutOfQuota: a.OutOfQuota, CanRise: a.CanRise, Back: a.Back, Windows: a.Windows,
	}
}

// checkWindowAgainstStatus checks each fact of doc's account a's window w,
// read, standing at now as got, against the function it takes the place of:
// all but its kind, when what that says comes, its even pace and its
// allowance, which the window tests hold through the moment, and each fact
// here is read against.
func checkWindowAgainstStatus(t *testing.T, doc status.Document, a status.Account, w quota.Window, got status.WindowNow, now time.Time) {
	t.Helper()
	want := status.WindowNow{
		WindowName: got.WindowName,
		Shared:     policy.IsShared(w.Key), Started: w.Key == policy.Started, Pressure: w.Key == policy.Pressure,
		Kind: got.Kind, At: got.At,
		Read: status.AsRead{Used: w.Utilization, ResetsAt: w.ResetsAt, At: a.FetchedAt},
	}
	if want.Kind != status.WindowTokenless && want.Kind != status.WindowUnread {
		stands, _ := a.WindowAt(w.Key, now, policy)
		out, runsOut := doc.RunsOut(a, w, now)
		want.Used, want.Pace, want.Next = stands.Utilization, got.Pace, stands.ResetsAt
		want.Allowance, want.Room = got.Allowance, got.Room
		want.Start, want.Length, _ = stands.Span()
		want.ResetSinceRead, want.Lapsed = w.ResetBy(now), a.HasLapsed(w.Key, now, policy)
		want.Held = want.Kind == status.WindowLimited
		if want.Held {
			want.Back, want.Since = want.At, got.Since
		}
		want.Capped = doc.ReserveHolds(a) && slices.Contains(a.ReachedReserve(now), w.Key)
		want.CappedAsRead = doc.ReserveHolds(a) && slices.Contains(a.AtReserve, w.Key)
		want.RunsOut, want.Out = want.Kind == status.WindowRunsOut, out
		unheld := !a.TokenRefused(now) && !want.Lapsed && a.Error == "" && !want.Capped
		want.RunsOutAhead = want.RunsOut || want.Held && unheld && runsOut
		if share, ok := dashboardProjected(want.Kind, out.Projection); ok {
			want.Projected = &share
		}
		want.Nearing = want.Projected != nil && !runsOut && *want.Projected >= doc.Floor(a)-status.Near-score.Tolerance
		want.InUse = dashboardInUse(stands, doc.Project(a, w, now).Projection)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s's %s stands as\n%+v\nwant\n%+v", a.ID, w.Key, got, want)
	}
}

// lastOf is the latest of the times given: zero where there are none, or one
// is zero, as not known.
func lastOf(times []time.Time) time.Time {
	var last time.Time
	for _, t := range times {
		if t.IsZero() {
			return time.Time{}
		}
		if t.After(last) {
			last = t
		}
	}
	return last
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

// dashboardProjected is the share a window of the given kind, heading as p
// says, is heading for by its reset, as the dashboard's standing.projected
// has it: all of it where it runs out; none while it's held at its limit or
// not started, or where p says nothing.
func dashboardProjected(kind status.WindowKind, p score.Projection) (float64, bool) {
	switch {
	case kind == status.WindowLimited || kind == status.WindowNotStarted:
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
	for _, hm := range statustest.HardMoments(policy) {
		t.Run(hm.Name, func(t *testing.T) {
			worked := hm.Doc.WorkedOut(policy)
			m := hm.Doc.At(hm.Doc.GeneratedAt, policy)
			if got, want := poolNowOf(worked.Pool), m.Pool(); !reflect.DeepEqual(got, want) {
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

func TestAWindowNoDocumentLabelsIsNamedByTheLabelClaudeGivesItsKey(t *testing.T) {
	tests := []struct{ key, want string }{
		{key: "5h", want: "5-hour"},
		{key: "1h", want: "1-hour"},
		{key: "7d", want: "Week"},
		{key: "7d_oi", want: "Fable week"},
		{key: "7d_opus", want: "Opus week"},
		{key: "90m", want: "90m"},
		{key: "7d_new", want: "7d_new"},
	}
	for _, tt := range tests {
		if got := status.NameOfKey(tt.key); got != tt.want {
			t.Errorf("NameOfKey(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestAMomentNamesAWindowAsItsAccountsDoElseByItsKey(t *testing.T) {
	doc := routerDocOf(readAt("work", windowIn("5h", 0.2, 3*time.Hour), quota.Window{Key: "7d", Label: "Seven days", Utilization: 0.4, ResetsAt: built.Add(2 * day)}))
	m := doc.At(built, policy)
	for key, want := range map[string]string{"5h": "5-hour", "7d": "Seven days", "7d_opus": "Opus week", "7d_new": "7d_new"} {
		if got := m.NameOf(key); got != want {
			t.Errorf("NameOf(%q) = %q, want %q", key, got, want)
		}
	}
	if w, ok := m.Window("7d"); !ok || w.Label != "Seven days" {
		t.Errorf("Window(7d) = %+v, %v; want the week as its accounts label it", w, ok)
	}
	if _, ok := m.Window("7d_opus"); ok {
		t.Error("Window(7d_opus) found a window none of the accounts has")
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

func TestHowManySessionsALimitMovedWhileItHolds(t *testing.T) {
	limited := with(readAt("work", spentIn("5h", 1, time.Hour)), func(a *status.Account) {
		a.Limit = status.Limit{ID: 4, Windows: []string{"5h"}, Until: built.Add(time.Hour)}
	})
	reached := func(limit, count int) status.Event {
		return status.Event{ID: 9, At: built.Add(-time.Minute), Kind: status.EventLimit, Account: "work", Windows: []string{"5h"}, Limit: limit, Count: count}
	}
	// newer are the router's newest 50 events, each newer than the limit's,
	// which has aged out of them.
	newer := make([]status.Event, 50)
	for i := range newer {
		newer[i] = status.Event{ID: 60 - i, At: built.Add(-time.Duration(i) * time.Second), Kind: status.EventMoved, Session: "c61b0e4f", From: "side", To: "personal"}
	}
	tests := []struct {
		name   string
		events []status.Event
		now    time.Time
		want   int
	}{
		{name: "one moved at once", events: []status.Event{reached(4, 1)}, now: built, want: 1},
		{name: "a second moved at its next request", events: []status.Event{reached(4, 2)}, now: built, want: 2},
		{name: "the limit lifted", events: []status.Event{reached(4, 2)}, now: built.Add(time.Hour + time.Second)},
		{name: "its event carrying no count", events: []status.Event{reached(4, 0)}, now: built},
		{name: "an older limit's event, the one holding untold", events: []status.Event{reached(3, 2)}, now: built},
		{name: "its event aged out of the newest 50", events: newer, now: built},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := routerDocOf(limited)
			doc.Events = tt.events
			if a, _ := doc.At(tt.now, policy).Account("work"); a.LimitMoved != tt.want {
				t.Errorf("LimitMoved = %d, want %d", a.LimitMoved, tt.want)
			}
		})
	}
}

func TestWhenTheCapHoldingAnAccountBegan(t *testing.T) {
	capped := routerDocOf(with(readAt("work", windowIn("5h", 0.95, 3*time.Hour), windowIn("7d", 0.4, 3*day)), reserving))
	reached := func(at, until time.Duration) status.Event {
		return status.Event{At: built.Add(at), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Until: built.Add(until), Reserve: 0.1}
	}
	untilUntold := reached(-10*time.Minute, 0)
	untilUntold.Until = time.Time{}
	tests := []struct {
		name   string
		doc    status.Document
		events []status.Event
		now    time.Time
		want   time.Time
	}{
		{name: "as its event tells", doc: capped, events: []status.Event{reached(-10*time.Minute, 3*time.Hour)}, now: built, want: built.Add(-10 * time.Minute)},
		{
			name: "as the newest of its events tells", doc: capped,
			events: []status.Event{reached(-10*time.Minute, 3*time.Hour), reached(-4*time.Hour, -2*time.Hour)}, now: built, want: built.Add(-10 * time.Minute),
		},
		{name: "as its event tells, telling no until", doc: capped, events: []status.Event{untilUntold}, now: built, want: built.Add(-10 * time.Minute)},
		{name: "its event missing", doc: capped, now: built},
		{name: "its event missing, an earlier cap's told, lifted since", doc: capped, events: []status.Event{reached(-4*time.Hour, -2*time.Hour)}, now: built},
		{
			name: "another account's event", doc: capped, now: built,
			events: []status.Event{{At: built.Add(-10 * time.Minute), Kind: status.EventCap, Account: "side", Windows: []string{"5h"}, Until: built.Add(3 * time.Hour)}},
		},
		{name: "its event told after now", doc: capped, events: []status.Event{reached(5*time.Minute, 3*time.Hour)}, now: built},
		{name: "its window reset since read, its cap gone", doc: capped, events: []status.Event{reached(-10*time.Minute, 3*time.Hour)}, now: built.Add(3*time.Hour + time.Second)},
		{name: "the global pin spending its cap", doc: pinnedTo(capped, "work"), events: []status.Event{reached(-10*time.Minute, 3*time.Hour)}, now: built},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.doc
			doc.Events = tt.events
			if a, _ := doc.At(tt.now, policy).Account("work"); !a.CapSince.Equal(tt.want) {
				t.Errorf("CapSince = %v, want %v", a.CapSince, tt.want)
			}
		})
	}
}

func TestWhenTheRefusalHoldingAnAccountBegan(t *testing.T) {
	refusing := func(r status.Refusal) status.Account {
		return with(readAt("work", windowIn("5h", 0.3, 3*time.Hour)), func(a *status.Account) { a.Refused = r })
	}
	token := refusing(status.Refusal{Until: built.Add(time.Hour), Status: 401})
	family := refusing(status.Refusal{Until: built.Add(30 * time.Minute), Status: 403, Family: "opus"})
	refused := func(at, until time.Duration, code int, family string) status.Event {
		return status.Event{At: built.Add(at), Kind: status.EventRefused, Account: "work", Until: built.Add(until), Status: code, Family: family}
	}
	// liftedEarly is a refusal of work's token, its until brought forward to
	// when it was lifted, as the token was replaced.
	liftedEarly := refused(-50*time.Minute, -35*time.Minute, 401, "")
	tests := []struct {
		name    string
		account status.Account
		events  []status.Event
		want    time.Time
	}{
		{name: "its token's, as its event tells", account: token, events: []status.Event{refused(-20*time.Minute, time.Hour, 401, "")}, want: built.Add(-20 * time.Minute)},
		{name: "a model family's, as its event tells", account: family, events: []status.Event{refused(-15*time.Minute, 30*time.Minute, 403, "opus")}, want: built.Add(-15 * time.Minute)},
		{name: "its event missing", account: token},
		{name: "its event missing, an earlier refusal's told, lifted early", account: token, events: []status.Event{liftedEarly}},
		{
			name: "its token refused again, after a refusal lifted early", account: token,
			events: []status.Event{refused(-20*time.Minute, time.Hour, 401, ""), liftedEarly}, want: built.Add(-20 * time.Minute),
		},
		{name: "a refusal lifted early, none holding", account: refusing(status.Refusal{}), events: []status.Event{liftedEarly}},
		{name: "another answer's event", account: token, events: []status.Event{refused(-20*time.Minute, time.Hour, 403, "")}},
		{name: "another model family's event", account: family, events: []status.Event{refused(-15*time.Minute, 30*time.Minute, 403, "sonnet")}},
		{name: "its event told after now", account: token, events: []status.Event{refused(5*time.Minute, time.Hour, 401, "")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := routerDocOf(tt.account)
			doc.Events = tt.events
			if a, _ := doc.At(built, policy).Account("work"); !a.RefusedSince.Equal(tt.want) {
				t.Errorf("RefusedSince = %v, want %v", a.RefusedSince, tt.want)
			}
		})
	}
}

// A window reset by hand runs from then till its reset, kept, and its even
// pace goes by that: at its hard moment, work's session, restarted an hour
// before its document was built, runs four hours, a quarter of them gone as
// it was read.
func TestAWindowRestartedByHandRunsFromItsRestart(t *testing.T) {
	for _, hm := range statustest.HardMoments(policy) {
		if hm.Name != "a 5-hour window restarted by hand" {
			continue
		}
		a, _ := hm.Doc.At(hm.Now, policy).Account("work")
		w, _ := a.Window("5h")
		if !w.Start.Equal(statustest.Built.Add(-time.Hour)) || w.Length != 4*time.Hour || w.Pace == nil || *w.Pace != 0.25 {
			t.Errorf("the session runs from %v for %v, its pace %s; want from an hour before %v, for 4h, its pace 0.25", w.Start, w.Length, paceOf(w.Pace), statustest.Built)
		}
		return
	}
	t.Fatal("no hard moment of a window restarted by hand")
}

func TestAWindowNotHeldBeganNoLimit(t *testing.T) {
	doc := routerDocOf(readAt("work", windowIn("5h", 0.5, time.Hour)))
	doc.Events = []status.Event{{At: built.Add(-time.Minute), Kind: status.EventLimit, Account: "work", Windows: []string{"5h"}}}
	a, _ := doc.At(built, policy).Account("work")
	if w, _ := a.Window("5h"); w.Held || !w.Since.IsZero() || !w.Back.IsZero() {
		t.Errorf("the session, with room, stands held %v, back %v, since %v; want none of it", w.Held, w.Back, w.Since)
	}
}

func TestAMomentsPoolIsWorkedOutOnceForEveryCopy(t *testing.T) {
	m := fullDocument().At(built.Add(time.Hour), policy)
	first := m.Pool()
	if len(first.Accounts) == 0 || len(first.Windows) == 0 {
		t.Fatalf("Pool() = %+v, want a pool of the document's accounts", first)
	}
	copied := m
	if again := copied.Pool(); !reflect.DeepEqual(again, first) {
		t.Errorf("Pool() asked again = %+v, want %+v", again, first)
	}
	if got := (status.Moment{}).Pool(); !reflect.DeepEqual(got, status.PoolNow{}) {
		t.Errorf("the zero moment's Pool() = %+v, want none", got)
	}
}

// A window runs out where a view says so only where nothing before running
// out holds, the first of the kinds that holds winning: a limit, a refused
// token or a failed read says more than its projection, which it keeps for
// the bar's projected fill. It runs out ahead, as a timeline draws it, where
// it runs out once a limit holding it lifts, judged by the same kinds: a
// stale window runs out nowhere ahead either.
func TestAWindowRunsOutWhereNothingBeforeItHolds(t *testing.T) {
	// The week, 60% used three days in, runs out two days on at its pace.
	running := readAt("work", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.6, 4*day))
	out := built.Add(2 * day)
	failed := func(a *status.Account) { a.Error = "HTTP 529 · Overloaded" }
	refused := func(a *status.Account) { a.Refused = status.Refusal{Until: built.Add(time.Hour), Status: 401} }
	unnamed := func(a *status.Account) { a.Limit = status.Limit{Until: built.Add(time.Hour)} }
	tokenless := func(a *status.Account) { a.TokenSet = false }
	unread := func(a *status.Account) { a.FetchedAt = time.Time{} }
	tests := []struct {
		name           string
		account        status.Account
		kind           status.WindowKind
		runsOut, ahead bool
		// unknown is set where nothing is known of the window, which then
		// has no projection.
		unknown bool
	}{
		{name: "with nothing before it", account: running, kind: status.WindowRunsOut, runsOut: true, ahead: true},
		{name: "its account without a usable token", account: with(running, tokenless), kind: status.WindowTokenless, unknown: true},
		{name: "nothing read of its account", account: with(running, unread), kind: status.WindowUnread, unknown: true},
		{name: "a limit naming no window holding it, its account without a usable token", account: with(with(running, unnamed), tokenless), kind: status.WindowTokenless, unknown: true},
		{name: "its last read failed", account: with(running, failed), kind: status.WindowStale},
		{name: "its token refused", account: with(running, refused), kind: status.WindowStale},
		{name: "a limit naming no window holding it", account: with(running, unnamed), kind: status.WindowLimited, ahead: true},
		{name: "a limit naming no window holding it, its token refused", account: with(with(running, unnamed), refused), kind: status.WindowLimited},
		{name: "a limit naming no window holding it, its last read failed", account: with(with(running, unnamed), failed), kind: status.WindowLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := statustest.Router(policy, nil, statustest.Routed{Account: tt.account}).WorkedOut(policy).At(built.Add(time.Minute), policy).Account("work")
			w, _ := a.Window("7d")
			wantOut := out
			if tt.unknown {
				wantOut = time.Time{}
			}
			if w.Kind != tt.kind || w.RunsOut != tt.runsOut || w.RunsOutAhead != tt.ahead || !w.Out.At.Equal(wantOut) {
				t.Errorf("the week stands as kind %d, running out %v, ahead %v, its projection out at %v; want kind %d, %v, %v, and out at %v",
					w.Kind, w.RunsOut, w.RunsOutAhead, w.Out.At, tt.kind, tt.runsOut, tt.ahead, wantOut)
			}
			if tt.runsOut && !w.At.Equal(w.Out.At) {
				t.Errorf("the week runs out at %v, want %v, as its projection has it", w.At, w.Out.At)
			}
		})
	}
}

// A document stays on screen while the router doesn't answer, its
// projections put from when it was read. Once the clock reaches where one has
// a window run out, that's past, not to come: the window runs out nowhere, in
// USAGE's kind, the pressure clause and COMING UP alike, and heads nowhere.
func TestARunOutTheClockHasReachedIsNone(t *testing.T) {
	// Side's session, used at 1.2 an hour lately: 85% used, it runs out 7m30s
	// after it was read; 70% used, a fifth kept back, it reaches its reserve
	// 5 minutes after, and its limit 15.
	running := statustest.Routed{Account: readAt("side", windowIn("5h", 0.85, 2*time.Hour)), Rate: 1.2}
	reserved := statustest.Routed{Account: with(readAt("side", windowIn("5h", 0.7, 2*time.Hour)), func(a *status.Account) { a.Reserve = 0.2 }), Rate: 1.2}
	open := statustest.Routed{Account: readAt("work", windowIn("5h", 0.2, 2*time.Hour))}
	tests := []struct {
		name string
		side statustest.Routed
		now  time.Time
		// out is where side's session runs out at now, zero where it doesn't.
		out time.Time
	}{
		{name: "a minute before its run-out", side: running, now: built.Add(6*time.Minute + 30*time.Second), out: built.Add(7*time.Minute + 30*time.Second)},
		{name: "as the clock reaches its run-out", side: running, now: built.Add(7*time.Minute + 30*time.Second)},
		{name: "once the clock has passed its run-out", side: running, now: built.Add(10 * time.Minute)},
		{name: "a minute before its reserve", side: reserved, now: built.Add(4 * time.Minute), out: built.Add(5 * time.Minute)},
		{name: "past its reserve, its limit still ahead", side: reserved, now: built.Add(10 * time.Minute)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := statustest.Router(policy, nil, open, tt.side).WorkedOut(policy).At(tt.now, policy)
			a, _ := m.Account("side")
			w, _ := a.Window("5h")
			runsOut := !tt.out.IsZero()
			if !w.Out.At.Equal(tt.out) || !a.Pressure.Out.At.Equal(tt.out) {
				t.Errorf("the session runs out at %v, by its pressure at %v; want %v", w.Out.At, a.Pressure.Out.At, tt.out)
			}
			if w.RunsOut != runsOut || w.RunsOutAhead != runsOut || (w.Kind == status.WindowRunsOut) != runsOut {
				t.Errorf("the session stands as kind %d, running out %v, ahead %v; want running out %v", w.Kind, w.RunsOut, w.RunsOutAhead, runsOut)
			}
			coming := slices.ContainsFunc(m.ComingUp, func(u status.Upcoming) bool { return u.Account == "side" && u.Kind == status.UpcomingRunsOut })
			if coming != runsOut {
				t.Errorf("COMING UP lists the session's run-out %v, want %v, as USAGE says it", coming, runsOut)
			}
			var clauses []string
			if runsOut {
				clauses = []string{"out ~" + prose.Dated(tt.now, tt.out)}
			}
			if a.Routing.Kind != status.RoutingPressed || !slices.Equal(a.Routing.Clauses, clauses) {
				t.Errorf("Routing = %+v, want under pressure, saying %q", a.Routing, clauses)
			}
			if !runsOut && (w.Out != status.RunOut{} || w.Projected != nil || w.Nearing) {
				t.Errorf("the session heads to %+v, projected %v, nearing %v; want nowhere", w.Out, w.Projected, w.Nearing)
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
		account := statustest.Routed{Account: with(readAt("work", session, windowIn("7d", r.Float64(), 3*day)), func(a *status.Account) {
			a.Reserve = reserves[r.IntN(len(reserves))]
		})}
		if r.IntN(3) > 0 {
			account.Rate = r.Float64() * 0.6
		}
		var pinned []string
		if r.IntN(2) == 0 {
			pinned = []string{"work"}
		}
		doc := statustest.Router(policy, pinned, account, statustest.Routed{Account: readAt("side", windowIn("5h", 0.1, 4*time.Hour))}).WorkedOut(policy)
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
		if got, want := doc.Pressed(doc.Accounts[0], built, policy), "under pressure: "+verb+" ~"+prose.TimeOfDay(built, stored.RunsOut); got != want {
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
			if got := routingOf(doc, tt.account, stateNow).String(); got != tt.routing {
				t.Errorf("Routing = %q, want %q", got, tt.routing)
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

// An account is out of quota where a limit or its cap holds it back from
// every request; and it can take a request, its use rising, where it has a
// usable token, its usage was read, its quota hasn't run out and its token
// isn't refused.
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
		{name: "without a usable token", account: status.Account{ID: "work", Label: "work"}},
		{name: "nothing read of it yet", account: status.Account{ID: "work", Label: "work", TokenSet: true}},
		{name: "its first read failed, nothing read of it before", account: status.Account{ID: "work", Label: "work", TokenSet: true, Error: "HTTP 529 · Overloaded"}},
		{name: "its last read failed, after an earlier one", account: with(readAt("work", session, week), func(a *status.Account) { a.Error = "HTTP 529 · Overloaded" }), canRise: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pinned []string
			if tt.pinned {
				pinned = []string{tt.account.ID}
			}
			m := statustest.Router(policy, pinned, statustest.Routed{Account: tt.account}).WorkedOut(policy).At(built.Add(time.Minute), policy)
			if a := m.Accounts[0]; a.OutOfQuota != tt.outOfQuota || a.CanRise != tt.canRise {
				t.Errorf("OutOfQuota = %v, CanRise = %v; want %v, %v", a.OutOfQuota, a.CanRise, tt.outOfQuota, tt.canRise)
			}
		})
	}
}

func TestAMomentAtAnotherNowIsItsDocumentAtThatNow(t *testing.T) {
	for _, hm := range statustest.HardMoments(policy) {
		t.Run(hm.Name, func(t *testing.T) {
			got, want := hm.Doc.At(built, policy).At(hm.Now), hm.Doc.At(hm.Now, policy)
			if !reflect.DeepEqual(factsOf(got), factsOf(want)) {
				t.Errorf("as built, then at now, the moment is\n%+v\nwant\n%+v", factsOf(got), factsOf(want))
			}
			if !reflect.DeepEqual(got.Pool(), want.Pool()) {
				t.Errorf("as built, then at now, the pool is %+v, want %+v", got.Pool(), want.Pool())
			}
		})
	}
}

// A held moment is its document's at its now, as its policy judges it,
// worked out once, and of that document at that now by that policy alone:
// another document, another now, or another policy is another moment's.
func TestAHeldMomentIsItsDocumentsAtItsNow(t *testing.T) {
	doc, at := fullDocument(), built.Add(time.Hour)
	held := doc.HoldAt(at, policy)
	if got, want := held.Moment(), doc.At(at, policy); !reflect.DeepEqual(factsOf(got), factsOf(want)) || !reflect.DeepEqual(got.Pool(), want.Pool()) {
		t.Errorf("the moment held is\n%+v\nwant\n%+v", factsOf(got), factsOf(want))
	}
	other := doc
	other.GeneratedAt = doc.GeneratedAt.Add(time.Second)
	another := policy
	another.Started = ""
	tests := []struct {
		name   string
		held   *status.HeldMoment
		doc    status.Document
		now    time.Time
		policy score.Policy
		want   bool
	}{
		{name: "its document at its now", held: held, doc: doc, now: at, policy: policy, want: true},
		{name: "its document at its now, as worked out already", held: doc.At(at, policy).Held(), doc: doc, now: at, policy: policy, want: true},
		{name: "its document at another now", held: held, doc: doc, now: at.Add(time.Second), policy: policy},
		{name: "another document at its now", held: held, doc: other, now: at, policy: policy},
		{name: "its document at its now, by another policy", held: held, doc: doc, now: at, policy: another},
		{name: "its document at its now, worked out by another policy", held: doc.At(at, another).Held(), doc: doc, now: at, policy: policy},
		{name: "none held", doc: doc, now: at, policy: policy},
	}
	for _, tt := range tests {
		if got := tt.held.Of(tt.doc, tt.now, tt.policy); got != tt.want {
			t.Errorf("%s: Of() = %v, want %v", tt.name, got, tt.want)
		}
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
	a := statustest.Probed(policy, work).At(built, policy).Accounts[0]
	if got, want := a.Title(), "work · Work place"; got != want || got != work.Title() {
		t.Errorf("Title() = %q, want %q, as the account's Title has it, %q", got, want, work.Title())
	}
}
