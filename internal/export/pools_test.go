package export_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/identity"
)

// The accounts record is exported with its pools: each one the exporting Mac
// configures marked so and labelled as it's configured, one the record has as
// this Mac's that the config the router started with lacks marked so and
// labelled as the config last wrote it, and each removed as the record labels
// it; and with nothing of what this Mac's config alone makes of them, the
// pool each account is in, nor how it writes a pool.
func TestTheAccountsRecordIsExportedWithItsPoolsLabelled(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	first := identity.PoolEntry{ID: "b2n8", Added: recordAdded, First: true, Here: true, Written: identity.PoolWritten{Label: "Default"}}
	since := identity.PoolEntry{ID: "c1nt", Added: recordAdded, Here: true, Written: identity.PoolWritten{Label: "Client", Dirs: []string{"~/Code/client"}}}
	gone := identity.PoolEntry{ID: "q7mt", Label: "Acme", Added: recordAdded, Removed: recordAdded, Written: identity.PoolWritten{Label: "Acme"}}
	pooled := work
	pooled.Pool = "b2n8"
	data, err := identity.Record{Accounts: []identity.Entry{pooled}, Pools: []identity.PoolEntry{first, since, gone}}.Encode()
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
	want := []identity.PoolEntry{
		{ID: "b2n8", Label: "Everything", Added: recordAdded, First: true, Configured: true},
		{ID: "c1nt", Label: "Client", Added: recordAdded, Configured: true},
		{ID: "q7mt", Label: "Acme", Added: recordAdded, Removed: recordAdded},
	}
	if !slices.EqualFunc(record.Pools, want, identity.PoolEntry.Equal) {
		t.Errorf("the folder's record's pools are %+v, want %+v", record.Pools, want)
	}
	if got := record.Accounts[0].Pool; got != "" {
		t.Errorf("the folder's record has work in pool %q, want none", got)
	}
}

func TestAFolderIsFromBeforePoolsWhereItsExportJSONIsOfAnOlderVersion(t *testing.T) {
	for version, want := range map[int]bool{1: true, 2: true, 3: false, export.Version + 1: false} {
		if got := (export.Manifest{Version: version}).BeforePools(); got != want {
			t.Errorf("a folder of version %d is from before pools: %v, want %v", version, got, want)
		}
	}
}
