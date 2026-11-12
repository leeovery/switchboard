package idmove_test

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/status"
)

// thisFirst is the id of this Mac's first pool in the folder tests.
const thisFirst = "b2n8"

// thisMacPooled is this Mac's accounts record, as its moves made it: its
// accounts as thisMac has them, and its first pool.
func thisMacPooled() identity.Record {
	r := thisMac()
	r.Pools = []identity.PoolEntry{{ID: thisFirst, Added: now, First: true}}
	return r
}

// putPooledRecord writes the accounts record of the accounts and pools
// given in dir.
func putPooledRecord(t *testing.T, dir string, accounts []identity.Entry, pools []identity.PoolEntry) {
	t.Helper()
	data, err := identity.Record{Accounts: accounts, Pools: pools}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, identity.NewFile(dir).Path(), string(data))
}

// requestOf is a request line's request.
func requestOf(l []byte) string {
	var line ledger.Line
	_ = json.Unmarshal(l, &line)
	return line.Request
}

// kindOf is an event's kind.
func kindOf(l []byte) string {
	var e status.Event
	_ = json.Unmarshal(l, &e)
	return e.Kind
}

// A folder from before pools, of version 1 or 2, has every request line, pin
// event and summary given this Mac's first pool as it's taken in, its
// accounts renamed as ever, every total kept.
func TestAFolderFromBeforePoolsIsGivenThisMacsFirstPool(t *testing.T) {
	tests := []struct {
		name   string
		m      export.Manifest
		record []identity.Entry
		rename map[string]string
	}{
		{
			name:   "this Mac's, from before ids, of version 1",
			m:      export.Manifest{Install: thisInstall, Version: 1},
			rename: map[string]string{work: workID, personal: personalID, gone: goneID},
		},
		{
			name: "another Mac's, with its record, of version 2",
			m:    export.Manifest{Install: otherInstall, IDs: true, Accounts: true, Version: 2},
			record: []identity.Entry{
				{ID: workID, Organization: workOrganization, Added: now, Was: work},
				{ID: personalID, Organization: personalOrganization, Added: now, Was: personal},
				{ID: "r7kq", Label: gone, Added: now, Removed: now, Was: gone},
			},
			rename: map[string]string{work: workID, personal: personalID, gone: "r7kq"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFolder(t, tt.m)
			if tt.record != nil {
				putRecord(t, f.state, tt.record)
			}
			before := f.tallies(t)
			record := thisMacPooled()
			f.joined(t, &record)

			for line, pool := range pooledLines(t, ledgerFiles(f.state), requestOf) {
				if pool != thisFirst {
					t.Errorf("request line %s names pool %q, want this Mac's first, %q", line, pool, thisFirst)
				}
			}
			pins := pooledLines(t, events.Files(ledger.Dir(f.state), logs.For("test")), kindOf)
			want := map[string]string{longAgo + " " + status.EventLimit: "", longAgo + " " + status.EventMoved: "", today + " " + status.EventPin: thisFirst}
			if !maps.Equal(pins, want) {
				t.Errorf("the events name the pools %v, want %v", pins, want)
			}
			for _, date := range []string{longAgo, yesterday} {
				var s ledger.Summary
				if err := json.Unmarshal([]byte(readFile(t, ledger.SummaryFile(ledger.Dir(f.state), date))), &s); err != nil {
					t.Fatal(err)
				}
				for _, a := range s.Accounts {
					if len(a.Models) > 0 && a.Pool != thisFirst {
						t.Errorf("the summary of %s has %q's work in pool %q, want this Mac's first", date, a.Account, a.Pool)
					}
				}
			}
			f.wantTallies(t, before, tt.rename)
		})
	}
}

func TestAFolderFromBeforePoolsIsRefusedWhereThisMacNotesNoFirstPool(t *testing.T) {
	f := newFolder(t, export.Manifest{Install: thisInstall, Version: 2, IDs: true})
	record := thisMac()
	if _, err := idmove.JoinFolder(f.state, &record, here); !errors.Is(err, idmove.ErrNoFirstPool) {
		t.Errorf("JoinFolder = %v, want %v", err, idmove.ErrNoFirstPool)
	}
	if !slices.Equal(record.Accounts, thisMac().Accounts) || record.Pools != nil {
		t.Errorf("refused, the record is %+v, want it as it was", record)
	}
}

// A folder since pools has its record's pools joined to this Mac's, matched
// by id, else added as the other Mac keeps them, and its lines' pools left
// as they are.
func TestAFolderSincePoolsJoinsItsPoolsAndGivesNoLineOne(t *testing.T) {
	f := newFolder(t, export.Manifest{Install: otherInstall, IDs: true, Accounts: true})
	putPooledRecord(t, f.state, thisMac().Accounts, []identity.PoolEntry{
		{ID: thisFirst, Label: "Everything", Added: now, Configured: true},
		{ID: "z4z4", Label: "Default", Added: now, First: true, Configured: true},
		{ID: "c1nt", Label: "Client", Added: now, Removed: now},
	})
	record := thisMacPooled()
	rename, err := idmove.JoinFolder(f.state, &record, idmove.Here{Install: here.Install, Pools: config.Pools{{ID: thisFirst, Label: "Default"}}})
	if err != nil {
		t.Fatalf("JoinFolder: %v", err)
	}
	if err := idmove.RenameFolder(t.Context(), f.state, rename); err != nil {
		t.Fatalf("RenameFolder: %v", err)
	}

	want := []identity.PoolEntry{
		{ID: thisFirst, Added: now, First: true},
		{ID: "z4z4", Label: "Default", Added: now},
		{ID: "c1nt", Label: "Client", Added: now, Removed: now},
	}
	if !slices.EqualFunc(record.Pools, want, identity.PoolEntry.Equal) {
		t.Errorf("joined, the record's pools are\n%+v\nwant\n%+v: this Mac's first as it was, the other's added, never first", record.Pools, want)
	}
	for line, pool := range pooledLines(t, ledgerFiles(f.state), requestOf) {
		if pool != "" {
			t.Errorf("request line %s names pool %q, want it as it was, naming none", line, pool)
		}
	}
}

// A day of this Mac's own, moved to pools, reads as the same day of a folder
// it exported before pools reads given its first pool as it's taken in, so
// an import of that folder finds every line held already.
func TestALineMovedToPoolsReadsAsTheSameLineImportedFromBeforePools(t *testing.T) {
	mac := newIDsFixture(t)
	folder := t.TempDir()
	for _, name := range []string{"ledger", "history", "accounts.json"} {
		copyTree(t, filepath.Join(mac.state, name), filepath.Join(folder, name))
	}
	asFolder(t, folder, export.Manifest{Install: thisInstall, IDs: true, Accounts: true, Version: 2})
	mac.pooled(t)

	record := mac.record(t)
	rename, err := idmove.JoinFolder(folder, &record, idmove.Here{Install: here.Install, Accounts: mac.configured(t), Pools: config.Pools{{ID: mac.firstPool(t)}}})
	if err != nil {
		t.Fatalf("JoinFolder: %v", err)
	}
	if err := idmove.RenameFolder(t.Context(), folder, rename); err != nil {
		t.Fatalf("RenameFolder: %v", err)
	}
	for _, kind := range []func(state string) *dayfile.Files{ledgerFiles, func(state string) *dayfile.Files { return events.Files(ledger.Dir(state), logs.For("test")) }} {
		if got, want := linesOf(t, kind(folder)), linesOf(t, kind(mac.state)); !slices.Equal(got, want) {
			t.Errorf("the folder's lines taken in read\n%q\nwant this Mac's moved\n%q", got, want)
		}
	}
}

// linesOf are the lines of every day of files, each as it's held, in order.
func linesOf(t *testing.T, files *dayfile.Files) []string {
	t.Helper()
	dates, err := files.All()
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, date := range dates {
		if _, _, err := dayfile.ReadDay(files, date, func(l []byte) ([]byte, bool) { return l, true }, func(l []byte) bool {
			lines = append(lines, string(l))
			return true
		}); err != nil {
			t.Fatal(err)
		}
	}
	slices.Sort(lines)
	return lines
}

// copyTree copies the file or directory at from to to, every file in it.
func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		writeFile(t, filepath.Join(to, rel), string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
