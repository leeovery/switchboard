package export_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/identity"
)

// The accounts record is exported with its pools, each one the exporting Mac
// configures marked so and labelled as it's configured, and each removed as
// the record labels it.
func TestTheAccountsRecordIsExportedWithItsPoolsLabelled(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	first := identity.PoolEntry{ID: "b2n8", Added: recordAdded, First: true}
	gone := identity.PoolEntry{ID: "q7mt", Label: "Acme", Added: recordAdded, Removed: recordAdded}
	data, err := identity.Record{Accounts: []identity.Entry{work}, Pools: []identity.PoolEntry{first, gone}}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, accountsRecord(state), data, summarisedAt)
	x := exporter(state)
	x.Pools = config.Pools{{ID: "B2N8", Label: "Everything"}}

	if _, err := x.Export(dir); err != nil {
		t.Fatal(err)
	}
	record, err := identity.NewFile(folderOf(t, state, dir)).Read()
	if err != nil {
		t.Fatal(err)
	}
	labelled := first
	labelled.Label, labelled.Configured = "Everything", true
	if want := []identity.PoolEntry{labelled, gone}; !slices.Equal(record.Pools, want) {
		t.Errorf("the folder's record's pools are %+v, want %+v: the first labelled as configured, the removed one as the record has it", record.Pools, want)
	}
}

func TestAFolderIsFromBeforePoolsWhereItsExportJSONIsOfAnOlderVersion(t *testing.T) {
	for version, want := range map[int]bool{1: true, 2: true, 3: false, export.Version + 1: false} {
		if got := (export.Manifest{Version: version}).BeforePools(); got != want {
			t.Errorf("a folder of version %d is from before pools: %v, want %v", version, got, want)
		}
	}
}
