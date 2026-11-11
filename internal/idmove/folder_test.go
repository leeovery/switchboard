package idmove_test

import (
	"cmp"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
)

// thisMac is this Mac's accounts record, of the fixture's two configured
// accounts moved onto their ids, each with the id it had before; it knows
// nothing of the one removed before ids were made.
func thisMac() identity.Record {
	return identity.Record{Accounts: []identity.Entry{
		{ID: workID, Organization: workOrganization, Added: now, Was: work},
		{ID: personalID, Organization: personalOrganization, Added: now, Was: personal},
	}}
}

// newFolder is the fixture's install, its state directory laid out as a
// folder staged to import: every day's files compressed, as export writes
// them.
func newFolder(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	kinds := []*dayfile.Files{readingsFiles(f.state), ledgerFiles(f.state), events.Files(ledger.Dir(f.state), logs.For("test"))}
	for _, files := range kinds {
		for _, date := range []string{yesterday, today} {
			plain := filepath.Join(files.Dir, files.Prefix+"-"+date+".jsonl")
			data, err := os.ReadFile(plain)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			putDay(t, files, date, true, strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")...)
			if err := os.Remove(plain); err != nil {
				t.Fatal(err)
			}
		}
	}
	return f
}

// ledgerFiles are the request ledger's files in the state directory.
func ledgerFiles(state string) *dayfile.Files {
	return ledger.Files(ledger.Dir(state), logs.For("test"))
}

// joined joins the fixture's state directory, as a folder staged to import,
// into record, and renames it, failing the test where either fails.
func (f fixture) joined(t *testing.T, record *identity.Record) {
	t.Helper()
	rename, err := idmove.JoinFolder(f.state, record, nil, now)
	if err != nil {
		t.Fatalf("JoinFolder: %v", err)
	}
	if err := idmove.RenameFolder(t.Context(), f.state, rename); err != nil {
		t.Fatalf("RenameFolder: %v", err)
	}
}

// wantTallies fails the test where the fixture's days' totals by account
// aren't before's, each account renamed as rename has it.
func (f fixture) wantTallies(t *testing.T, before map[string]int, rename map[string]string) {
	t.Helper()
	want := make(map[string]int, len(before))
	for key, n := range before {
		kind, date, account := splitKey(key)
		want[kind+"|"+date+"|"+cmp.Or(rename[account], account)] += n
	}
	if got := f.tallies(t); !maps.Equal(got, want) {
		t.Errorf("the folder's days' totals by account are\n%v\nwant\n%v", got, want)
	}
}

// A folder with no accounts record was exported before the move, so every id
// it names is an old one: each this Mac's record doesn't know goes into it
// as removed, under a random id, its old id its label, and the folder is
// moved onto the ids, every day's totals by account the same. Joined again,
// the record gains nothing and the folder reads as it is.
func TestAFolderWithoutARecordIsMovedOntoTheIDsItsOldIDsHave(t *testing.T) {
	f := newFolder(t)
	before := f.tallies(t)
	record := thisMac()
	f.joined(t, &record)

	removed, ok := record.ByWas(gone)
	if !ok || removed.Label != gone || removed.Removed.IsZero() || removed.Organization != "" || !identity.WellFormed(removed.ID) {
		t.Fatalf("the record holds the account removed before as %+v, want it removed, labelled %s, under an id switchboard makes", removed, gone)
	}
	f.wantTallies(t, before, map[string]string{work: workID, personal: personalID, gone: removed.ID})

	moved := f.snapshotOf(t, "ledger", "history")
	again := slices.Clone(record.Accounts)
	f.joined(t, &record)
	if !slices.Equal(record.Accounts, again) {
		t.Errorf("joined again, the record is\n%+v\nwant it as it was\n%+v", record.Accounts, again)
	}
	if got := f.snapshotOf(t, "ledger", "history"); !maps.Equal(got, moved) {
		t.Errorf("joined again, the folder changed:\n%s", differences(moved, got))
	}
}

// A folder holding the record of the Mac it was exported from has each id
// renamed by that record's former ids, then onto this Mac's id for the
// account: work by its id before, which both records know, personal by its
// organization, and the account removed there, which this Mac never had,
// taken in under its id there.
func TestAFolderIsMovedOntoThisMacsIDsByTheRecordItCarries(t *testing.T) {
	f := newFolder(t)
	before := f.tallies(t)
	putRecord(t, f.state, []identity.Entry{
		{ID: "r7kq", Added: now, Was: work},
		{ID: "p4m2", Organization: personalOrganization, Added: now, Was: personal},
		{ID: "h7c2", Label: gone, Added: now, Removed: now, Was: gone},
	})
	record := thisMac()
	f.joined(t, &record)

	f.wantTallies(t, before, map[string]string{work: workID, personal: personalID, gone: "h7c2"})
	if e, ok := record.ByID("h7c2"); !ok || e.Label != gone || e.Removed.IsZero() {
		t.Errorf("the record holds the account removed there as %+v, want it taken in, removed", e)
	}
}

// A day none of whose files names an account renamed is left as it is.
func TestADayNamingNoAccountRenamedIsLeftAsItIs(t *testing.T) {
	f := newFolder(t)
	date := "2026-11-01"
	putDay(t, readingsFiles(f.state), date, true, reading(date, workID, 0.1))
	path := readingsFiles(f.state).CompressedPath(date)
	was, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	record := thisMac()
	f.joined(t, &record)

	if got, err := os.Stat(path); err != nil || !got.ModTime().Equal(was.ModTime()) || got.Size() != was.Size() {
		t.Errorf("the day naming no account renamed was written anew (%v), want it left as it is", err)
	}
}

// A day's file cut short, as one synced part of the way, is renamed of its
// lines up to the damage, and the rest of the folder moved, rather than
// failing the import.
func TestADayCutShortIsRenamedUpToTheDamage(t *testing.T) {
	f := newFolder(t)
	path := ledgerFiles(f.state).CompressedPath(longAgo)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data[:len(data)-12]))
	record := thisMac()
	f.joined(t, &record)

	text := readFile(t, path)
	if strings.Contains(text, `"account":"work"`) || !strings.Contains(text, `"account":"`+workID+`"`) {
		t.Errorf("the day cut short reads\n%s\nwant its lines up to the damage under the new ids", text)
	}
	if got := readFile(t, ledgerFiles(f.state).CompressedPath(yesterday)); strings.Contains(got, `"account":"work"`) {
		t.Errorf("the day after the one cut short reads\n%s\nwant it moved too", got)
	}
}

// A folder whose record doesn't read is refused, the record as it was.
func TestAFolderWhoseRecordDoesntReadIsRefused(t *testing.T) {
	f := newFolder(t)
	writeFile(t, identity.NewFile(f.state).Path(), "not a record\n")
	record := thisMac()
	if _, err := idmove.JoinFolder(f.state, &record, nil, now); !errors.Is(err, idmove.ErrStagedRecord) {
		t.Errorf("JoinFolder = %v, want ErrStagedRecord", err)
	}
	if !slices.Equal(record.Accounts, thisMac().Accounts) {
		t.Errorf("refused, the record is %+v, want it as it was", record.Accounts)
	}
}

// putRecord writes, in dir, an accounts record of the accounts given.
func putRecord(t *testing.T, dir string, accounts []identity.Entry) {
	t.Helper()
	data, err := identity.Record{Accounts: accounts}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, identity.NewFile(dir).Path(), string(data))
}
