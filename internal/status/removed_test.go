package status_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/status"
)

// The accounts removed are those the accounts record keeps removed, by the
// label each showed last, but for one the config configures again, as by an
// edit by hand, in the record's order; and each is found by its id, and none
// else is.
func TestTheAccountsRemovedAreTheRecordsTheConfigDoesntConfigure(t *testing.T) {
	added := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	gone := time.Date(2026, 9, 4, 16, 30, 0, 0, time.UTC)
	record := identity.Record{Accounts: []identity.Entry{
		{ID: "b6s7", Added: added},
		{ID: "h7c2", Label: "team", Added: added, Removed: gone},
		{ID: "m2x7", Label: "side", Added: added, Removed: gone},
		{ID: "k3f9", Label: "spare", Added: added, Removed: gone},
	}}
	configured := config.Accounts{{ID: "b6s7", Label: "work"}, {ID: "M2X7", Label: "side"}, {ID: "qgr2", Label: "team"}}

	removed := status.RemovedOf(record, configured)
	want := status.RemovedAccounts{{ID: "h7c2", Label: "team", Removed: gone}, {ID: "k3f9", Label: "spare", Removed: gone}}
	if !slices.Equal(removed, want) {
		t.Errorf("RemovedOf() = %+v, want %+v", removed, want)
	}
	if got, ok := removed.Of("h7c2"); !ok || got != want[0] {
		t.Errorf("Of(h7c2) = %+v, %v; want %+v", got, ok, want[0])
	}
	for _, id := range []string{"qgr2", "m2x7", ""} {
		if got, ok := removed.Of(id); ok {
			t.Errorf("Of(%q) = %+v, want none", id, got)
		}
	}
}

// A removed account says when it was removed, by its local day, however the
// record keeps the time.
func TestARemovedAccountSaysWhenItWasRemoved(t *testing.T) {
	a := status.RemovedAccount{ID: "h7c2", Label: "team", Removed: time.Date(2026, 9, 4, 12, 0, 0, 0, time.Local).UTC()}
	if got := a.Said(); got != "removed 4 Sep" {
		t.Errorf("Said() = %q, want removed 4 Sep", got)
	}
}
