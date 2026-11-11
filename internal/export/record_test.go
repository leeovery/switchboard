package export_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/identity"
)

// accountsRecord is where the accounts record is in root, a state directory
// or a Mac's folder.
func accountsRecord(root string) string {
	return filepath.Join(root, "accounts.json")
}

// The accounts record is exported with the days, byte for byte with its time,
// counting for no day; passed over while it's as it was, and copied again
// once it changes.
func TestTheAccountsRecordIsCopiedCountingForNoDay(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	data := []byte(`{"version":1,"accounts":[{"id":"b6s7","added":"2026-09-01T12:00:00Z"}]}` + "\n")
	writeFile(t, accountsRecord(state), data, summarisedAt)
	writeFile(t, summary(state, day1), []byte("{}\n"), summarisedAt)

	wantResult(t, exportInto(t, state, dir), state, dir, 1, 0)
	copied := accountsRecord(folderOf(t, state, dir))
	wantCopy(t, copied, data, summarisedAt)
	was := look(t, copied)

	wantResult(t, exportInto(t, state, dir), state, dir, 0, 1)
	wantUnwritten(t, copied, was)

	changed := []byte(`{"version":1,"accounts":[{"id":"b6s7","organization":"test-org-work","added":"2026-09-01T12:00:00Z"}]}` + "\n")
	writeFile(t, accountsRecord(state), changed, summarisedAt.Add(time.Hour))
	wantResult(t, exportInto(t, state, dir), state, dir, 0, 1)
	wantCopy(t, copied, changed, summarisedAt.Add(time.Hour))

	record, err := identity.NewFile(folderOf(t, state, dir)).Read()
	if err != nil || len(record.Accounts) != 1 || record.Accounts[0].Organization != "test-org-work" {
		t.Errorf("the folder's accounts record reads %+v (%v), want the state directory's", record, err)
	}
}

// A state directory without an accounts record exports none.
func TestNoAccountsRecordExportsNone(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	writeFile(t, summary(state, day1), []byte("{}\n"), summarisedAt)

	wantResult(t, exportInto(t, state, dir), state, dir, 1, 0)
	if _, err := os.Lstat(accountsRecord(folderOf(t, state, dir))); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the folder holds an accounts record (%v), want none", err)
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
