package status_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// The order of care: the windows every model shares, the longest first,
// then each model's own, as any window the policy doesn't share is, in
// quota's order.
func TestWindowsListInTheOrderOfCare(t *testing.T) {
	tests := []struct {
		name   string
		shared []string
		keys   []string
		want   []string
	}{
		{name: "Claude's: the week, the 5-hour, then Fable's week", shared: []string{"5h", "7d"}, keys: []string{"7d_oi", "5h", "7d"}, want: []string{"7d", "5h", "7d_oi"}},
		{
			name: "a model's window the policy doesn't know falls in with the models'", shared: []string{"5h", "7d"},
			keys: []string{"7d_om", "5h", "7d_oi", "7d"}, want: []string{"7d", "5h", "7d_oi", "7d_om"},
		},
		{name: "a policy of one window: it first, the rest as models'", shared: []string{"5h"}, keys: []string{"7d_oi", "7d", "5h"}, want: []string{"5h", "7d", "7d_oi"}},
		{name: "three shared, the longest first", shared: []string{"5h", "1d", "7d"}, keys: []string{"5h", "7d_oi", "1d", "7d"}, want: []string{"7d", "1d", "5h", "7d_oi"}},
		{name: "a shared key whose length can't be read: last of the shared", shared: []string{"odd", "5h"}, keys: []string{"7d_oi", "odd", "5h"}, want: []string{"5h", "odd", "7d_oi"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slices.SortedFunc(slices.Values(tt.keys), status.CompareWindows(score.Policy{Shared: tt.shared}))
			if !slices.Equal(got, tt.want) {
				t.Errorf("listed %q, want %q", got, tt.want)
			}
		})
	}
}

// The windows every model shares are listed where some account has or may
// have them: one is left out only where there's an account, and every one
// lacks it, read with a usable token, reporting another such window but not
// it.
func TestTheMomentListsItsWindowsInTheOrderOfCare(t *testing.T) {
	unread := func(id string) status.Account { return status.Account{ID: id, Label: id, TokenSet: true} }
	weekOnly := func(id string) status.Account { return readAt(id, windowIn("7d", 0.3, 3*day)) }
	every := routerDocOf(
		readAt("work", windowIn("5h", 0.4, 2*time.Hour), windowIn("7d", 0.3, 3*day), windowIn("7d_oi", 0.2, 3*day)),
		readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.5, 3*day)),
	)
	tests := []struct {
		name   string
		doc    status.Document
		shared []string
		want   []string
		listed []string
	}{
		{name: "every one reported, once", doc: every, shared: []string{"5h", "7d"}, want: []string{"7d", "5h"}, listed: []string{"7d", "5h", "7d_oi"}},
		{name: "one shared that every account lacks", doc: every, shared: []string{"5h", "1d"}, want: []string{"5h"}, listed: []string{"5h", "7d", "7d_oi"}},
		{name: "the shared alone, before any is read", doc: routerDocOf(unread("work"), unread("side")), shared: []string{"5h", "7d"}, want: []string{"7d", "5h"}, listed: []string{"7d", "5h"}},
		{name: "every account lacking the 5-hour", doc: routerDocOf(weekOnly("work"), weekOnly("side")), shared: []string{"5h", "7d"}, want: []string{"7d"}, listed: []string{"7d"}},
		{
			name: "one account lacking the 5-hour, another with it", doc: routerDocOf(weekOnly("work"), readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.5, 3*day))),
			shared: []string{"5h", "7d"}, want: []string{"7d", "5h"}, listed: []string{"7d", "5h"},
		},
		{
			name: "one account lacking the 5-hour, another not read, which may have it", doc: routerDocOf(weekOnly("work"), unread("side")),
			shared: []string{"5h", "7d"}, want: []string{"7d", "5h"}, listed: []string{"7d", "5h"},
		},
		{name: "no accounts", doc: routerDocOf(), shared: []string{"5h", "7d"}, want: []string{"7d", "5h"}, listed: []string{"7d", "5h"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := policy
			p.Shared = tt.shared
			m := tt.doc.At(built.Add(time.Minute), p)
			if got := m.SharedWindows(); !slices.Equal(got, tt.want) {
				t.Errorf("SharedWindows() = %q, want %q", got, tt.want)
			}
			if got := m.Listed(); !slices.Equal(got, tt.listed) {
				t.Errorf("Listed() = %q, want %q", got, tt.listed)
			}
			if got := m.Reported(); !slices.Equal(got, tt.listed) {
				t.Errorf("Reported() = %q, want %q", got, tt.listed)
			}
		})
	}
}

// Each model's own window is listed as the 5-hour is, only where some
// account has it to show: where some account has used it, some of it used
// as it stands now, however near or far its limit, or where a limit naming
// it holds it, as one may once it has reset to 0%; never where a limit
// naming no window holds it alone, which says nothing of its use. An
// account known not to have used
// it goes without it, as an account without a 5-hour window does; one with
// no usable token, nothing read, its read failed since, or a limit holding
// it, may have used it, so keeps it. Reported
// lists every model's window any account reports, used or not. Those every
// model shares are listed always.
func TestAModelsWindowIsListedWhereSomeAccountHasUsedIt(t *testing.T) {
	work := func(fable float64, change func(*status.Account)) status.Account {
		a := readAt("work", windowIn("5h", 0.4, 2*time.Hour), windowIn("7d", 0.3, 3*day), windowIn("7d_oi", fable, 3*day))
		if change != nil {
			a = with(a, change)
		}
		return a
	}
	side := func(w quota.Window) status.Account {
		return readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.5, 3*day), w)
	}
	reserveAll := func(a *status.Account) { a.Reserve = 0.9999 }
	failed := func(a *status.Account) { a.Error = "HTTP 529 · Overloaded" }
	holdingAll := func(a *status.Account) { a.Limit = status.Limit{Until: built.Add(time.Hour)} }
	holdingFable := func(a *status.Account) {
		a.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: built.Add(time.Hour)}
	}
	lapsed := windowIn("7d_oi", 0.3, -time.Hour)
	tests := []struct {
		name   string
		doc    status.Document
		listed []string
		// without are the ids of the accounts that go without the model's
		// window, as its Without says.
		without []string
	}{
		{
			name: "every window at 0%, a 99.99% reserve putting the floor at 0.01%: not listed", listed: []string{"7d", "5h"},
			doc: routerDocOf(
				with(readAt("work", windowIn("5h", 0, 2*time.Hour), windowIn("7d", 0, 3*day), windowIn("7d_oi", 0, 3*day)), reserveAll),
				with(readAt("side", windowIn("5h", 0, 4*time.Hour), windowIn("7d", 0, 3*day), windowIn("7d_oi", 0, 3*day)), reserveAll),
			),
			without: []string{"work", "side"},
		},
		{
			name: "every one at 0%, one's cap holding it back from every request: not listed, that one keeping it, held with its account", listed: []string{"7d", "5h"},
			doc:     routerDocOf(work(0, reserveAll), side(windowIn("7d_oi", 0, 3*day))),
			without: []string{"side"},
		},
		{
			name: "every one at 0%, a limit naming the session holding one back from every request: not listed, that one keeping it, held with its account", listed: []string{"7d", "5h"},
			doc: routerDocOf(work(0, func(a *status.Account) {
				a.Limit = status.Limit{Windows: []string{"5h"}, Until: built.Add(time.Hour)}
			}), side(windowIn("7d_oi", 0, 3*day))),
			without: []string{"side"},
		},
		{
			name: "one account at 3%: listed, the other without it", listed: []string{"7d", "5h", "7d_oi"},
			doc:     routerDocOf(work(0, nil), side(windowIn("7d_oi", 0.03, 3*day))),
			without: []string{"work"},
		},
		{
			name: "a new model's, used: listed as any model's is", listed: []string{"7d", "5h", "7d_om"},
			doc:     routerDocOf(work(0, nil), side(windowIn("7d_om", 0.03, 3*day))),
			without: []string{"work"},
		},
		{
			name: "used, but reset since it was read: not listed", listed: []string{"7d", "5h"},
			doc:     routerDocOf(work(0, nil), side(lapsed)),
			without: []string{"work", "side"},
		},
		{
			name: "used by every account: listed, none without it", listed: []string{"7d", "5h", "7d_oi"},
			doc: routerDocOf(work(0.2, nil), side(windowIn("7d_oi", 0.5, 3*day))),
		},
		{
			name: "at 0% as last read, its read failed since, which may have used it: not listed, that one keeping it", listed: []string{"7d", "5h"},
			doc:     routerDocOf(work(0, failed), side(windowIn("7d_oi", 0, 3*day))),
			without: []string{"side"},
		},
		{
			name: "at 0% as last read, its read failed since, beside one used: listed, that one keeping it, stale", listed: []string{"7d", "5h", "7d_oi"},
			doc: routerDocOf(work(0, failed), side(windowIn("7d_oi", 0.03, 3*day))),
		},
		{
			name: "read 0% again once reset, a limit naming it still holding it: listed, that one keeping it, held", listed: []string{"7d", "5h", "7d_oi"},
			doc:     routerDocOf(work(0, holdingFable), side(windowIn("7d_oi", 0, 3*day))),
			without: []string{"side"},
		},
		{
			name: "every one at 0%, a limit naming no window holding one, which says nothing of its use: not listed, that one keeping it", listed: []string{"7d", "5h"},
			doc:     routerDocOf(work(0, holdingAll), side(windowIn("7d_oi", 0, 3*day))),
			without: []string{"side"},
		},
		{
			name: "at 0% beside an account with no usable token, which may have used it: not listed, that one keeping it", listed: []string{"7d", "5h"},
			doc:     routerDocOf(work(0, func(a *status.Account) { a.TokenSet = false }), side(windowIn("7d_oi", 0, 3*day))),
			without: []string{"side"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.doc.At(built.Add(time.Minute), policy)
			if got := m.Listed(); !slices.Equal(got, tt.listed) {
				t.Errorf("Listed() = %q, want %q", got, tt.listed)
			}
			reported := []string{"7d", "5h"}
			var without []string
			for _, a := range m.Accounts {
				w := a.Windows[2]
				if w.Shared {
					t.Fatalf("%s's %s is shared, want a model's own", a.ID, w.Key)
				}
				if w.Without() {
					without = append(without, a.ID)
				}
				reported = append(reported, w.Key)
			}
			if !slices.Equal(without, tt.without) {
				t.Errorf("the accounts without their model's window are %q, want %q", without, tt.without)
			}
			if got, want := m.Reported(), slices.Compact(slices.SortedFunc(slices.Values(reported), m.CompareWindows)); !slices.Equal(got, want) {
				t.Errorf("Reported() = %q, want %q", got, want)
			}
		})
	}
}

// An account lacking a window every model shares goes without it, and an
// account with it, at 0% or not, doesn't: unused, a window every model
// shares still shows, as the 5-hour's rule asks only whether the account has
// it.
func TestAWindowEveryModelSharesGoesWithoutOnlyWhereItsAccountLacksIt(t *testing.T) {
	m := routerDocOf(
		readAt("work", windowIn("7d", 0, 3*day)),
		readAt("side", windowIn("5h", 0, 4*time.Hour), windowIn("7d", 0.5, 3*day)),
	).At(built.Add(time.Minute), policy)
	for _, tt := range []struct {
		id, key string
		without bool
	}{
		{id: "work", key: "5h", without: true},
		{id: "work", key: "7d"},
		{id: "side", key: "5h"},
		{id: "side", key: "7d"},
	} {
		a, _ := m.Account(tt.id)
		if w, _ := a.Window(tt.key); w.Without() != tt.without {
			t.Errorf("%s's %s goes without it: %v, want %v", tt.id, tt.key, w.Without(), tt.without)
		}
	}
}
