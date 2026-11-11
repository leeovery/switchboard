package idmove_test

import (
	"cmp"
	"maps"
	"testing"

	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
)

// movedRecord is a record of the fixture's accounts moved onto their ids,
// each with the id it had before, the one removed before ids were made
// among them.
var movedRecord = identity.Record{Accounts: []identity.Entry{
	{ID: workID, Organization: workOrganization, Added: now, Was: work},
	{ID: personalID, Organization: personalOrganization, Added: now, Was: personal},
	{ID: "h7c2", Label: gone, Added: now, Removed: now, Was: gone},
}}

// A folder exported before the move, naming the old ids, is moved onto the
// ids the record made, every day's totals by account the same, as the move
// moves the history; and moved again, as a folder exported after it, naming
// the new ids, it reads as it is.
func TestAFolderExportedBeforeTheMoveIsMovedOntoTheIDs(t *testing.T) {
	f := newFixture(t)
	before := f.tallies(t)
	if err := idmove.MoveFolder(t.Context(), f.state, movedRecord); err != nil {
		t.Fatalf("MoveFolder: %v", err)
	}
	rename := map[string]string{work: workID, personal: personalID, gone: "h7c2"}
	want := make(map[string]int, len(before))
	for key, n := range before {
		kind, date, account := splitKey(key)
		want[kind+"|"+date+"|"+cmp.Or(rename[account], account)] += n
	}
	if got := f.tallies(t); !maps.Equal(got, want) {
		t.Errorf("the folder's days' totals by account are\n%v\nwant\n%v", got, want)
	}

	moved := f.snapshotOf(t, "ledger", "history")
	if err := idmove.MoveFolder(t.Context(), f.state, movedRecord); err != nil {
		t.Fatalf("MoveFolder again: %v", err)
	}
	if got := f.snapshotOf(t, "ledger", "history"); !maps.Equal(got, moved) {
		t.Errorf("a folder naming the new ids changed, moved again:\n%s", differences(moved, got))
	}
}

// A record of no account that had an id before ids were made leaves a folder
// as it is.
func TestAFolderIsLeftAsItIsByARecordOfNoFormerIDs(t *testing.T) {
	f := newFixture(t)
	was := f.snapshotOf(t, "ledger", "history")
	record := identity.Record{Accounts: []identity.Entry{{ID: workID, Organization: workOrganization, Added: now}}}
	if err := idmove.MoveFolder(t.Context(), f.state, record); err != nil {
		t.Fatalf("MoveFolder: %v", err)
	}
	if got := f.snapshotOf(t, "ledger", "history"); !maps.Equal(got, was) {
		t.Errorf("the folder changed:\n%s", differences(was, got))
	}
}
