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
var here = idmove.Here{Install: func() (string, error) { return thisInstall, nil }}

// The export.json of the folders of the tests: this Mac's from before ids,
// another's then, and another's from this build, holding its record.
var (
	thisBefore  = export.Manifest{Install: thisInstall}
	otherBefore = export.Manifest{Install: otherInstall}
	otherWithID = export.Manifest{Install: otherInstall, IDs: true, Accounts: true}
)

// newFolder is the fixture's install, its state directory laid out as a
// folder staged to import, its export.json as m says: every day's files
// compressed, as export writes them.
func newFolder(t *testing.T, m export.Manifest) fixture {
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
	m.Version, m.ExportedAt = export.Version, now
	data, err := json.Marshal(m)
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
	f := newFolder(t, thisBefore)
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

// A folder this build exported from a Mac with no accounts record joins
// nothing and has nothing renamed.
func TestAFolderExportedWithIDsWithoutARecordJoinsNothing(t *testing.T) {
	f := newFolder(t, export.Manifest{Install: otherInstall, IDs: true})
	before := f.snapshotOf(t, "ledger", "history")
	record := thisMac()
	f.joined(t, &record)

	if !slices.Equal(record.Accounts, thisMac().Accounts) {
		t.Errorf("joined, the record is %+v, want it as it was", record.Accounts)
	}
	if got := f.snapshotOf(t, "ledger", "history"); !maps.Equal(got, before) {
		t.Errorf("joined, the folder changed:\n%s", differences(before, got))
	}
}

// A folder that can't be renamed exactly is refused, saying which folder,
// the record as it was, as Refusal says it is: one another Mac exported
// before switchboard made ids, whose ids are that Mac's own names, and one
// whose export.json says it holds an accounts record it doesn't, as one a
// sync hasn't carried whole.
func TestAFolderThatCantBeRenamedExactlyIsRefused(t *testing.T) {
	tests := []struct {
		name string
		m    export.Manifest
		want error
	}{
		{name: "another Mac's from before ids", m: otherBefore, want: idmove.ErrBeforeIDs},
		{name: "one whose record hasn't synced", m: otherWithID, want: idmove.ErrUnsynced},
		{name: "this Mac's whose record hasn't synced", m: export.Manifest{Install: thisInstall, IDs: true, Accounts: true}, want: idmove.ErrUnsynced},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFolder(t, tt.m)
			if err := idmove.Refusal(f.state, here); !errors.Is(err, tt.want) {
				t.Errorf("Refusal = %v, want %v", err, tt.want)
			}
			record := thisMac()
			_, err := idmove.JoinFolder(f.state, &record, here)
			if !errors.Is(err, tt.want) || !strings.Contains(err.Error(), "the folder mac-"+tt.m.Install+" can't be taken in") {
				t.Errorf("JoinFolder = %v, want %v, saying which folder", err, tt.want)
			}
			if !slices.Equal(record.Accounts, thisMac().Accounts) {
				t.Errorf("refused, the record is %+v, want it as it was", record.Accounts)
			}
		})
	}
}

// This Mac's install is asked for only where a folder needs it, so one that
// can't be had refuses only this Mac's folder from before ids, and another
// Mac's is joined all the same, taking no label.
func TestThisMacsInstallIsAskedForOnlyWhereAFolderNeedsIt(t *testing.T) {
	broken := idmove.Here{Install: func() (string, error) { return "", fs.ErrPermission }}
	f := newFolder(t, otherWithID)
	putRecord(t, f.state, []identity.Entry{
		{ID: workID, Organization: workOrganization, Added: now, Was: work},
		{ID: "p4m2", Organization: "test-org-home", Label: "family", Added: now, Configured: true},
	})
	record := identity.Record{Accounts: append(thisMac().Accounts, identity.Entry{ID: "p4m2", Organization: "test-org-home", Label: "home", Added: now})}
	if _, err := idmove.JoinFolder(f.state, &record, broken); err != nil {
		t.Errorf("JoinFolder of another Mac's folder with its record, this Mac's install not to be had = %v, want it joined", err)
	}
	if e, _ := record.ByID("p4m2"); e.Label != "home" {
		t.Errorf("joined, this Mac's install not to be had, home is labelled %q, want its label here, home", e.Label)
	}
	if _, err := idmove.JoinFolder(newFolder(t, thisBefore).state, &record, broken); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("JoinFolder of this Mac's folder from before ids, its install not to be had = %v, want why", err)
	}
}

// A folder holding the record of the Mac it was exported from has each id
// renamed by that record's former ids onto its own, then onto this Mac's id
// for the account, by id or organization, never by the id it had before:
// work by its id, personal, under another id there, by its organization;
// and the account removed there before ids were made, its organization
// unknown, taken in under its id there, apart from the one removed here.
func TestAFolderIsMovedOntoThisMacsIDsByTheRecordItCarries(t *testing.T) {
	f := newFolder(t, otherWithID)
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

// An account only another Mac configures takes the label that Mac's folder
// gives it, but never one this Mac's own folder gives, which may be older.
func TestALabelIsTakenFromAnotherMacsFolderAlone(t *testing.T) {
	home := identity.Entry{ID: "p4m2", Organization: "test-org-home", Label: "home", Added: now}
	for _, tt := range []struct {
		name    string
		install string
		want    string
	}{
		{name: "another Mac's", install: otherInstall, want: "family"},
		{name: "this Mac's", install: thisInstall, want: "home"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFolder(t, export.Manifest{Install: tt.install, IDs: true, Accounts: true})
			putRecord(t, f.state, []identity.Entry{{ID: "p4m2", Organization: "test-org-home", Label: "family", Added: now, Configured: true}})
			record := identity.Record{Accounts: append(thisMac().Accounts, home)}
			f.joined(t, &record)
			if e, _ := record.ByID("p4m2"); e.Label != tt.want {
				t.Errorf("joined, home is labelled %q, want %q", e.Label, tt.want)
			}
		})
	}
}

// In this Mac's own folder, carrying its record, an account of an id this
// Mac holds is that account, though only this Mac knows its organization,
// as one a token told it since it exported: none of its lines is renamed.
// In another Mac's, it's another, taken in under another id.
func TestThisMacsOwnFolderIsJoinedByItsIDs(t *testing.T) {
	for _, tt := range []struct {
		name    string
		install string
		same    bool
	}{
		{name: "this Mac's", install: thisInstall, same: true},
		{name: "another Mac's", install: otherInstall, same: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFolder(t, export.Manifest{Install: tt.install, IDs: true, Accounts: true})
			putRecord(t, f.state, []identity.Entry{{ID: "k3f9", Added: now}})
			spare := identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: now}
			record := identity.Record{Accounts: append(thisMac().Accounts, spare)}
			rename, err := idmove.JoinFolder(f.state, &record, here)
			if err != nil {
				t.Fatalf("JoinFolder: %v", err)
			}
			if got := rename.Of("k3f9") == "k3f9" && len(record.Accounts) == len(thisMac().Accounts)+1; got != tt.same {
				t.Errorf("joined, k3f9 is renamed %s, the record %+v; want it the account here: %t", rename.Of("k3f9"), record.Accounts, tt.same)
			}
		})
	}
}

// A folder whose record doesn't read is refused, saying which folder, the
// record as it was, as Refusal says it is.
func TestAFolderWhoseRecordDoesntReadIsRefused(t *testing.T) {
	f := newFolder(t, otherWithID)
	writeFile(t, identity.NewFile(f.state).Path(), "not a record\n")
	if err := idmove.Refusal(f.state, here); !errors.Is(err, idmove.ErrStagedRecord) || !idmove.Refused(err) {
		t.Errorf("Refusal = %v, want ErrStagedRecord", err)
	}
	record := thisMac()
	if _, err := idmove.JoinFolder(f.state, &record, here); !errors.Is(err, idmove.ErrStagedRecord) || !strings.Contains(err.Error(), "the folder mac-"+otherInstall+" can't be taken in") {
		t.Errorf("JoinFolder = %v, want ErrStagedRecord, saying which folder", err)
	}
	if !slices.Equal(record.Accounts, thisMac().Accounts) {
		t.Errorf("refused, the record is %+v, want it as it was", record.Accounts)
	}
}

// A day none of whose files names an account renamed is left as it is.
func TestADayNamingNoAccountRenamedIsLeftAsItIs(t *testing.T) {
	f := newFolder(t, thisBefore)
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
	f := newFolder(t, thisBefore)
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
// damaged, is left as it is, for the import to pass over as any it can't
// read; but its summary, which names an account renamed, is renamed alone,
// so what the import copies in of it counts the account under its new id.
func TestADayThatReadsNoLineHasItsSummaryRenamedAlone(t *testing.T) {
	f := newFolder(t, thisBefore)
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
	if got := readFile(t, summary); strings.Contains(got, `"account":"work"`) || !strings.Contains(got, `"account":"`+workID+`"`) {
		t.Errorf("the day's summary reads %s, want its account renamed", got)
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
