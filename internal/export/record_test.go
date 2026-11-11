package export_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/identity"
)

// accountsRecord is where the accounts record is in root, a state directory
// or a Mac's folder.
func accountsRecord(root string) string {
	return filepath.Join(root, "accounts.json")
}

// putRecord writes, in the state directory state, an accounts record of the
// accounts given, last modified at modified.
func putRecord(t *testing.T, state string, modified time.Time, accounts ...identity.Entry) {
	t.Helper()
	data, err := identity.Record{Accounts: accounts}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, accountsRecord(state), data, modified)
}

// exportedRecord is the accounts record in the folder of state's install in
// dir, failing t where it can't be read.
func exportedRecord(t *testing.T, state, dir string) []identity.Entry {
	t.Helper()
	record, err := identity.NewFile(folderOf(t, state, dir)).Read()
	if err != nil {
		t.Fatal(err)
	}
	return record.Accounts
}

var (
	recordAdded = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	work        = identity.Entry{ID: "b6s7", Organization: "test-org-work", Added: recordAdded, Was: "work"}
	side        = identity.Entry{ID: "m2x7", Label: "side", Added: recordAdded, Removed: recordAdded, Was: "side"}
)

// The accounts record is exported with the days, with its time, counting for
// no day, each account configured labelled as it's configured, so a Mac that
// doesn't configure it names it, and each removed as the record labels it;
// passed over while it's as it was, and written again once it changes.
func TestTheAccountsRecordIsExportedLabelledCountingForNoDay(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	putRecord(t, state, summarisedAt, work, side)
	writeFile(t, summary(state, day1), []byte("{}\n"), summarisedAt)
	x := exporter(state)
	x.Accounts = config.Accounts{{ID: "B6S7", Label: "Work"}}

	if _, err := x.Export(dir); err != nil {
		t.Fatal(err)
	}
	labelledWork := work
	labelledWork.Label = "Work"
	if got := exportedRecord(t, state, dir); !slices.Equal(got, []identity.Entry{labelledWork, side}) {
		t.Errorf("the folder's accounts record is %+v, want the state directory's, work labelled as configured", got)
	}
	copied := accountsRecord(folderOf(t, state, dir))
	if info := look(t, copied); !info.ModTime().Equal(summarisedAt) || info.Mode().Perm() != 0o600 {
		t.Errorf("the folder's accounts record is modified at %v, %v; want %v, 0600", info.ModTime(), info.Mode().Perm(), summarisedAt)
	}
	was := look(t, copied)

	r, err := x.Export(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantResult(t, r, state, dir, 0, 1)
	wantUnwritten(t, copied, was)

	changed := work
	changed.Organization = "test-org-other"
	putRecord(t, state, summarisedAt.Add(time.Hour), changed)
	if _, err := x.Export(dir); err != nil {
		t.Fatal(err)
	}
	changed.Label = "Work"
	if got := exportedRecord(t, state, dir); !slices.Equal(got, []identity.Entry{changed}) {
		t.Errorf("once the record changed, the folder's is %+v, want it as the state directory's now is", got)
	}
}

// The accounts record is written before any day's lines, so a sync carrying
// the export in its order never has the days ahead of the record naming their
// accounts; and export.json says the folder holds it.
func TestTheAccountsRecordIsWrittenBeforeTheDaysAndSaidHeld(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	putRecord(t, state, summarisedAt, work)
	writeFile(t, filepath.Join(state, requests.compressed("", day1)), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
	var folder string
	looked := 0
	x := export.WithLooked(exporter(state), func(string) {
		looked++
		if _, err := os.Stat(accountsRecord(folder)); err != nil {
			t.Errorf("as the day's lines are looked at, the folder's accounts record: %v, want it written", err)
		}
	})
	folder = folderOf(t, state, dir)

	if _, err := x.Export(dir); err != nil || looked != 1 {
		t.Fatalf("Export() = %v, looking at %d days, want the day exported", err, looked)
	}
	if m, err := export.ReadManifest(folder); err != nil || !m.Accounts {
		t.Errorf("the folder's export.json reads %+v (%v), want it to say the folder holds the accounts record", m, err)
	}
}

// A state directory without an accounts record exports none, and removes the
// one exported before, so the folder reads as exported before switchboard
// made ids, as its days then are.
func TestNoAccountsRecordExportsNone(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	writeFile(t, summary(state, day1), []byte("{}\n"), summarisedAt)
	putRecord(t, state, summarisedAt, work)
	exportInto(t, state, dir)
	if err := os.Remove(accountsRecord(state)); err != nil {
		t.Fatal(err)
	}

	wantResult(t, exportInto(t, state, dir), state, dir, 0, 1)
	if _, err := os.Lstat(accountsRecord(folderOf(t, state, dir))); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the folder holds an accounts record (%v), want none", err)
	}
	if m, err := export.ReadManifest(folderOf(t, state, dir)); err != nil || m.Accounts {
		t.Errorf("the folder's export.json reads %+v (%v), want it not to say the folder holds the accounts record", m, err)
	}
}

// An accounts record that can't be read fails the export, saying so.
func TestAnAccountsRecordThatCantBeReadFailsTheExport(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	writeFile(t, accountsRecord(state), []byte("{}\n"), summarisedAt)
	if err := os.Chmod(accountsRecord(state), 0o000); err != nil {
		t.Fatal(err)
	}

	_, err := exporter(state).Export(dir)
	if !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "export the accounts record") {
		t.Errorf("Export() of an accounts record that can't be read: %v, want it to fail, saying so", err)
	}
}
