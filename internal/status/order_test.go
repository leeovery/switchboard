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

func TestTheMomentListsItsWindowsInTheOrderOfCare(t *testing.T) {
	unread := func(id string) status.Account { return status.Account{ID: id, Label: id, TokenSet: true} }
	every := routerDocOf(
		readAt("work", windowIn("5h", 0.4, 2*time.Hour), windowIn("7d", 0.3, 3*day), windowIn("7d_oi", 0.2, 3*day)),
		readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.5, 3*day)),
	)
	tests := []struct {
		name   string
		doc    status.Document
		shared []string
		want   []string
	}{
		{name: "every one reported, once", doc: every, shared: []string{"5h", "7d"}, want: []string{"7d", "5h", "7d_oi"}},
		{name: "one shared that no account reports", doc: every, shared: []string{"5h", "1d"}, want: []string{"1d", "5h", "7d", "7d_oi"}},
		{name: "the shared alone, before any is read", doc: routerDocOf(unread("work"), unread("side")), shared: []string{"5h", "7d"}, want: []string{"7d", "5h"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := policy
			p.Shared = tt.shared
			m := tt.doc.At(built.Add(time.Minute), p)
			if got := m.Listed(true); !slices.Equal(got, tt.want) {
				t.Errorf("Listed(true) = %q, want %q", got, tt.want)
			}
			if got, want := m.SharedWindows(), tt.want[:len(tt.shared)]; !slices.Equal(got, want) {
				t.Errorf("SharedWindows() = %q, want %q", got, want)
			}
		})
	}
}

// Each model's own window is listed where models are asked for, and else
// only while it nears or reaches its limit on some account: held at it, at
// the account's cap, running out, or nearing where it runs out. Those every
// model shares are listed always.
func TestAModelsWindowIsListedWhereAskedForOrNearingItsLimit(t *testing.T) {
	limitedOn := func(key string) func(*status.Account) {
		return func(a *status.Account) {
			a.Limit = status.Limit{ID: 1, Windows: []string{key}, Until: built.Add(time.Hour)}
		}
	}
	sideWith := func(w quota.Window, change func(*status.Account)) status.Document {
		side := readAt("side", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.5, 3*day), w)
		if change != nil {
			side = with(side, change)
		}
		return routerDocOf(readAt("work", windowIn("5h", 0.4, 2*time.Hour), windowIn("7d", 0.3, 3*day), windowIn("7d_oi", 0.2, 3*day)), side)
	}
	shared := []string{"7d", "5h"}
	tests := []struct {
		name string
		doc  status.Document
		// stands is how side's model window stands, checked so the case
		// lists it, or doesn't, for the reason it names.
		stands func(status.WindowNow) bool
		hidden []string
	}{
		{name: "quiet on every account: hidden", doc: sideWith(windowIn("7d_oi", 0.3, 3*day), nil), stands: quiet, hidden: shared},
		{name: "a new model's, quiet: hidden as any model's is", doc: sideWith(windowIn("7d_om", 0.3, 3*day), nil), stands: quiet, hidden: shared},
		{
			name: "held at its limit", doc: sideWith(windowIn("7d_oi", 0.3, 3*day), limitedOn("7d_oi")),
			stands: func(w status.WindowNow) bool { return w.Held }, hidden: []string{"7d", "5h", "7d_oi"},
		},
		{
			name: "at the account's cap", doc: sideWith(windowIn("7d_oi", 0.92, 3*day), reserving),
			stands: func(w status.WindowNow) bool { return w.Capped && !w.Held }, hidden: []string{"7d", "5h", "7d_oi"},
		},
		{
			name: "running out", doc: sideWith(windowIn("7d_oi", 0.5, 6*day), nil),
			stands: func(w status.WindowNow) bool { return w.RunsOut && !w.Capped }, hidden: []string{"7d", "5h", "7d_oi"},
		},
		{
			name: "nearing where it runs out", doc: sideWith(windowIn("7d_oi", 0.8, day), nil),
			stands: func(w status.WindowNow) bool { return w.Nearing && !w.RunsOut && !w.Capped }, hidden: []string{"7d", "5h", "7d_oi"},
		},
		{
			name: "a new model's, nearing", doc: sideWith(windowIn("7d_om", 0.8, day), nil),
			stands: func(w status.WindowNow) bool { return w.Nearing && !w.RunsOut }, hidden: []string{"7d", "5h", "7d_om"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.doc.At(built.Add(time.Minute), policy)
			side, _ := m.Account("side")
			model := side.Windows[2]
			if model.Shared || !tt.stands(model) {
				t.Fatalf("side's %s stands as %+v, not as the case says", model.Key, model)
			}
			if got := m.Listed(false); !slices.Equal(got, tt.hidden) {
				t.Errorf("Listed(false) = %q, want %q", got, tt.hidden)
			}
			every := slices.Compact(slices.SortedFunc(slices.Values([]string{"7d", "5h", "7d_oi", model.Key}), m.CompareWindows))
			if got := m.Listed(true); !slices.Equal(got, every) {
				t.Errorf("Listed(true) = %q, want %q", got, every)
			}
		})
	}
}

// quiet reports whether the window neither nears nor reaches its limit.
func quiet(w status.WindowNow) bool {
	return !w.Held && !w.Capped && !w.RunsOut && !w.Nearing
}
