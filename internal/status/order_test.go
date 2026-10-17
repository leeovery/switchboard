package status_test

import (
	"slices"
	"testing"
	"time"

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
			if got := m.Listed(); !slices.Equal(got, tt.want) {
				t.Errorf("Listed() = %q, want %q", got, tt.want)
			}
			if got, want := m.SharedWindows(), tt.want[:len(tt.shared)]; !slices.Equal(got, want) {
				t.Errorf("SharedWindows() = %q, want %q", got, want)
			}
		})
	}
}
