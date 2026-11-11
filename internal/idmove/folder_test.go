package idmove_test

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"encoding/json"
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
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
)

// The installs of the folder tests: this Mac's, and another's.
const (
	thisInstall  = "0123456789abcdef"
	otherInstall = "fedcba9876543210"
)

// goneID is the id this Mac's move made for the account removed before ids
// were made.
const goneID = "h7c2"

// thisMac is this Mac's accounts record, as its move made it: the fixture's
// two configured accounts and the one removed before, each with the id it
// had before.
func thisMac() identity.Record {
	return identity.Record{Accounts: []identity.Entry{
		{ID: workID, Organization: workOrganization, Added: now, Was: work},
		{ID: personalID, Organization: personalOrganization, Added: now, Was: personal},
		{ID: goneID, Label: gone, Added: now, Removed: now, Was: gone},
	}}
}

// here is this Mac, as a folder is joined to it.
var here = idmove.Here{Install: thisInstall}

// newFolder is the fixture's install, its state directory laid out as a
// folder staged to import, exported by the install given, its export.json
// saying it holds the accounts record where accounts says so: every day's
// files compressed, as export writes them.
func newFolder(t *testing.T, install string, accounts bool) fixture {
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
	data, err := json.Marshal(export.Manifest{Version: export.Version, Install: install, ExportedAt: now, Accounts: accounts})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(f.state, "export.json"), string(data)+"\n")
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
	rename, err := idmove.JoinFolder(f.state, record, here)
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

// A folder this Mac exported before switchboard made ids, holding no
// accounts record, names its accounts by the ids they had before, which this
// Mac's record knows: it's renamed by them, the record as it was. Joined
// again, the folder reads as it is.
func TestThisMacsFolderFromBeforeIDsIsRenamedByItsRecord(t *testing.T) {
	f := newFolder(t, thisInstall, false)
	before := f.tallies(t)
	record := thisMac()
	f.joined(t, &record)

	if !slices.Equal(record.Accounts, thisMac().Accounts) {
		t.Errorf("joined, the record is %+v, want it as it was", record.Accounts)
	}
	f.wantTallies(t, before, map[string]string{work: workID, personal: personalID, gone: goneID})
	moved := f.snapshotOf(t, "ledger", "history")
	f.joined(t, &record)
	if got := f.snapshotOf(t, "ledger", "history"); !maps.Equal(got, moved) {
		t.Errorf("joined again, the folder changed:\n%s", differences(moved, got))
	}
}

// A folder that can't be renamed exactly is refused, the record as it was:
// one another Mac exported before switchboard made ids, whose ids are that
// Mac's own names, and one whose export.json says it holds an accounts
// record it doesn't, as one a sync hasn't carried whole.
func TestAFolderThatCantBeRenamedExactlyIsRefused(t *testing.T) {
	tests := []struct {
		name     string
		install  string
		accounts bool
		want     error
	}{
		{name: "another Mac's from before ids", install: otherInstall, want: idmove.ErrBeforeIDs},
		{name: "one whose record hasn't synced", install: otherInstall, accounts: true, want: idmove.ErrUnsynced},
		{name: "this Mac's whose record hasn't synced", install: thisInstall, accounts: true, want: idmove.ErrUnsynced},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFolder(t, tt.install, tt.accounts)
			record := thisMac()
			if _, err := idmove.JoinFolder(f.state, &record, here); !errors.Is(err, tt.want) {
				t.Errorf("JoinFolder = %v, want %v", err, tt.want)
			}
			if !slices.Equal(record.Accounts, thisMac().Accounts) {
				t.Errorf("refused, the record is %+v, want it as it was", record.Accounts)
			}
		})
	}
}

// A folder holding the record of the Mac it was exported from has each id
// renamed by that record's former ids onto its own, then onto this Mac's id
// for the account, by id or organization, never by the id it had before:
// work by its id, personal, under another id there, by its organization;
// and the account removed there before ids were made, its organization
// unknown, taken in under its id there, apart from the one removed here.
func TestAFolderIsMovedOntoThisMacsIDsByTheRecordItCarries(t *testing.T) {
	f := newFolder(t, otherInstall, true)
	before := f.tallies(t)
	putRecord(t, f.state, []identity.Entry{
		{ID: workID, Organization: workOrganization, Added: now, Was: work},
		{ID: "z9k4", Organization: personalOrganization, Added: now, Was: personal},
		{ID: "r7kq", Label: gone, Added: now, Removed: now, Was: gone},
	})
	record := thisMac()
	f.joined(t, &record)

	f.wantTallies(t, before, map[string]string{work: workID, personal: personalID, gone: "r7kq"})
	got := record.Accounts
	if len(got) != 4 || !slices.Equal(got[:3], thisMac().Accounts) {
		t.Fatalf("joined, the record is\n%+v\nwant this Mac's accounts as they were, and the one removed there", got)
	}
	if e := got[3]; e.ID != "r7kq" || e.Label != gone || !e.Removed.Equal(now) || e.Was != "" {
		t.Errorf("joined, the account removed there is %+v, want it under r7kq, labelled %s, removed, no id before", e, gone)
	}
}

// A folder whose record doesn't read is refused, the record as it was.
func TestAFolderWhoseRecordDoesntReadIsRefused(t *testing.T) {
	f := newFolder(t, otherInstall, true)
	writeFile(t, identity.NewFile(f.state).Path(), "not a record\n")
	record := thisMac()
	if _, err := idmove.JoinFolder(f.state, &record, here); !errors.Is(err, idmove.ErrStagedRecord) {
		t.Errorf("JoinFolder = %v, want ErrStagedRecord", err)
	}
	if !slices.Equal(record.Accounts, thisMac().Accounts) {
		t.Errorf("refused, the record is %+v, want it as it was", record.Accounts)
	}
}

// A day none of whose files names an account renamed is left as it is.
func TestADayNamingNoAccountRenamedIsLeftAsItIs(t *testing.T) {
	f := newFolder(t, thisInstall, false)
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

// gzipOf is lines, each ended, as one gzip member.
func gzipOf(t *testing.T, lines ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	for _, l := range lines {
		if _, err := w.Write([]byte(l + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A day's file cut short, as one synced part of the way, is renamed of every
// line that reads before the damage, and none after it, the rest of the
// folder moved, rather than failing the import.
func TestADayCutShortIsRenamedUpToTheDamage(t *testing.T) {
	f := newFolder(t, thisInstall, false)
	date := "2026-11-01"
	kept := []string{line(date, "r1", work, "new", "", nil), line(date, "r2", personal, "new", "", nil)}
	after := gzipOf(t, line(date, "r3", work, "new", "", nil))
	path := ledgerFiles(f.state).CompressedPath(date)
	writeFile(t, path, string(append(gzipOf(t, kept...), after[:12]...)))
	record := thisMac()
	f.joined(t, &record)

	text := readFile(t, path)
	want := strings.Join([]string{
		strings.Replace(kept[0], `"account":"work"`, `"account":"`+workID+`"`, 1),
		strings.Replace(kept[1], `"account":"personal"`, `"account":"`+personalID+`"`, 1),
	}, "\n") + "\n"
	if text != want {
		t.Errorf("the day cut short reads\n%s\nwant its two lines before the damage alone, under the new ids\n%s", text, want)
	}
	if got := readFile(t, ledgerFiles(f.state).CompressedPath(yesterday)); strings.Contains(got, `"account":"work"`) {
		t.Errorf("the day after the one cut short reads\n%s\nwant it moved too", got)
	}
}

// A request ledger's day whose file reads none of its lines, its start
// damaged, is left as it is, its summary too, though the summary names an
// account renamed, so the import passes it over as any it can't read.
func TestADayThatReadsNoLineIsLeftAsItIs(t *testing.T) {
	f := newFolder(t, thisInstall, false)
	date := "2026-11-01"
	path := ledgerFiles(f.state).CompressedPath(date)
	writeFile(t, path, "not gzip at all")
	summary := ledger.SummaryFile(ledger.Dir(f.state), date)
	writeFile(t, summary, `{"version":6,"day":"`+date+`","lines":1,"accounts":[{"account":"work"}]}`+"\n")
	record := thisMac()
	f.joined(t, &record)

	if got := readFile(t, path); got != "not gzip at all" {
		t.Errorf("the day that reads no line holds %q, want it as it was", got)
	}
	if got := readFile(t, summary); !strings.Contains(got, `"account":"work"`) {
		t.Errorf("the day's summary reads %s, want it as it was", got)
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
