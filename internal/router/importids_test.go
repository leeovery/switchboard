package router_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/router"
)

// workID is the id switchboard made for the account the tests' lines name as
// work, its id before switchboard made ids.
const workID = "b6s7"

// movedWork is the account work as an accounts record keeps it once it's
// moved onto workID.
var movedWork = identity.Entry{ID: workID, Organization: "test-org-work", Added: stagedOn, Was: "work"}

// putRecord writes, in root, a state directory or a folder staged, an
// accounts record of the accounts given.
func putRecord(t *testing.T, root string, accounts ...identity.Entry) {
	t.Helper()
	data, err := identity.Record{Accounts: accounts}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	put(t, identity.NewFile(root).Path(), data, stagedOn)
}

// readRecord is the accounts record in root, failing the test where it can't
// be read.
func readRecord(t *testing.T, root string) identity.Record {
	t.Helper()
	record, err := identity.NewFile(root).Read()
	if err != nil {
		t.Fatal(err)
	}
	return record
}

// named is lines with the account work named by id.
func named(lines []string, id string) []string {
	renamed := make([]string, len(lines))
	for i, line := range lines {
		renamed[i] = strings.ReplaceAll(line, `"account":"work"`, `"account":"`+id+`"`)
	}
	return renamed
}

// asMoved is lines with the account work named by workID, as the move writes
// them.
func asMoved(lines []string) []string {
	return named(lines, workID)
}

// wantHeld fails the test where the request ledger of the Mac cfg configures
// doesn't hold want on the day with the given date.
func wantHeld(t *testing.T, cfg router.Config, date string, want []string) {
	t.Helper()
	if got := held(t, requestLines.files(cfg.StateDir), date); !slices.Equal(got, want) {
		t.Errorf("once imported, the request ledger of %s holds\n%s\nwant\n%s", date, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A folder exported before the move names the accounts by their old ids: its
// days are taken in under the ids the Mac's record made, a line the Mac holds
// under the new id the same line as it, staged under the old, and a day's
// summary counting the account under its new id.
func TestAFolderExportedBeforeTheMoveIsTakenInUnderTheNewIDs(t *testing.T) {
	cfg := importConfig(t)
	putRecord(t, cfg.StateDir, movedWork)
	date, pastKeep := dateOf(10), dateOf(60)
	for _, kind := range lineKinds {
		put(t, kind.files(cfg.StateDir).CompressedPath(date), gzipOf(t, asMoved(kind.linesOf(date, 1))...), now)
		stageLines(t, folder(cfg, 1), kind, date, kind.linesOf(date, 1, 2)...)
	}
	summary := `{"version":6,"day":"` + pastKeep + `","lines":1,"bytes":{"plain":0,"compressed":0},"accounts":[{"account":"work"}]}` + "\n"
	put(t, ledger.SummaryFile(ledger.Dir(folder(cfg, 1)), pastKeep), []byte(summary), stamped(stagedOn))

	want := router.Imported{Folders: 1, Days: router.ImportedDays{Merged: 1}, Lines: 3, Summaries: 1}
	if got := importNow(t, cfg); got != want {
		t.Errorf("Import() = %+v, want %+v", got, want)
	}
	for _, kind := range lineKinds {
		if got, want := held(t, kind.files(cfg.StateDir), date), asMoved(kind.linesOf(date, 1, 2)); !slices.Equal(got, want) {
			t.Errorf("once imported, the %s of %s holds\n%s\nwant\n%s", kind.name, date, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
	var copied ledger.Summary
	if err := json.Unmarshal([]byte(stateOf(t, ledger.SummaryFile(ledger.Dir(cfg.StateDir), pastKeep)).data), &copied); err != nil {
		t.Fatal(err)
	}
	if len(copied.Accounts) != 1 || copied.Accounts[0].Account != workID {
		t.Errorf("once imported, the summary of %s counts %+v, want the account under %s", pastKeep, copied.Accounts, workID)
	}
	if got := readRecord(t, cfg.StateDir); !slices.Equal(got.Accounts, []identity.Entry{movedWork}) {
		t.Errorf("once imported, the Mac's accounts record is %+v, want it as it was", got.Accounts)
	}
	checkUnstaged(t, cfg)
}

// A folder exported after the move carries its accounts record, which joins
// the Mac's: its days under the new ids are taken in as they are, and those
// it kept from before the move, under the old ids, moved onto the new by the
// record it carries, as on a Mac whose own record never knew them.
func TestAFolderExportedAfterTheMoveIsTakenInByTheRecordItCarries(t *testing.T) {
	cfg := importConfig(t)
	putRecord(t, cfg.StateDir)
	before, after := dateOf(20), dateOf(10)
	putRecord(t, folder(cfg, 1), movedWork)
	stageLines(t, folder(cfg, 1), requestLines, before, requestLines.linesOf(before, 1)...)
	stageLines(t, folder(cfg, 1), requestLines, after, asMoved(requestLines.linesOf(after, 2))...)

	want := router.Imported{Folders: 1, Days: router.ImportedDays{Added: 2}}
	if got := importNow(t, cfg); got != want {
		t.Errorf("Import() = %+v, want %+v", got, want)
	}
	wantHeld(t, cfg, before, asMoved(requestLines.linesOf(before, 1)))
	wantHeld(t, cfg, after, asMoved(requestLines.linesOf(after, 2)))
	if got := readRecord(t, cfg.StateDir); !slices.Equal(got.Accounts, []identity.Entry{movedWork}) {
		t.Errorf("once imported, the Mac's accounts record is %+v, want the folder's account joined to it", got.Accounts)
	}
}

// A folder another Mac exported before the move, naming an account this one
// never had, takes it in as removed, under an id of its own; the folder that
// Mac exports after the move, which names the account under the id it made
// for it there, has it taken in under the same id here, so a day the two
// folders both hold counts once.
func TestAnAccountTakenInBeforeTheMoveIsTheOneTheFolderAfterNames(t *testing.T) {
	cfg := importConfig(t)
	putRecord(t, cfg.StateDir, movedWork)
	date := dateOf(10)
	lines := requestLines.linesOf(date, 1, 2)
	runRouter(t, cfg)
	client := router.NewClient(router.SocketPath(cfg.StateDir))

	stageLines(t, folder(cfg, 1), requestLines, date, named(lines, "team")...)
	if _, err := client.Import(t.Context()); err != nil {
		t.Fatalf("Import() of the folder from before the move: %v", err)
	}
	team, ok := readRecord(t, cfg.StateDir).ByWas("team")
	if !ok || team.Label != "team" || team.Removed.IsZero() {
		t.Fatalf("once the folder from before the move is imported, the Mac's record holds team as %+v, want it removed, labelled team", team)
	}

	putRecord(t, folder(cfg, 1), identity.Entry{ID: "r7kq", Label: "team", Added: stagedOn, Removed: stagedOn, Was: "team"})
	stageLines(t, folder(cfg, 1), requestLines, date, named(lines, "r7kq")...)
	if _, err := client.Import(t.Context()); err != nil {
		t.Fatalf("Import() of the folder from after the move: %v", err)
	}
	wantHeld(t, cfg, date, named(lines, team.ID))
	if got := readRecord(t, cfg.StateDir).Accounts; !slices.Equal(got, []identity.Entry{movedWork, team}) {
		t.Errorf("once both folders are imported, the Mac's record is %+v, want team in it once", got)
	}
}

// One account the Mac knows under another id than the Mac a folder comes
// from has it under is taken in under the Mac's id: a removed account each
// gave a random id of its own, matched by the id it had before, and a
// subscription each had under another id before, matched by its
// organization.
func TestAnAccountUnderAnotherIDOnTheMacExportedFromIsTakenInUnderThisMacs(t *testing.T) {
	side := identity.Entry{ID: "m2x7", Label: "side", Added: stagedOn, Removed: stagedOn, Was: "side"}
	personal := identity.Entry{ID: "qgr2", Organization: "test-org-personal", Added: stagedOn, Was: "personal"}
	tests := []struct {
		name   string
		own    identity.Entry
		staged identity.Entry
		// was is the id the folder's days from before the move name it by.
		was string
	}{
		{
			name:   "removed on both Macs",
			own:    side,
			staged: identity.Entry{ID: "r7kq", Label: "side", Added: stagedOn, Removed: stagedOn, Was: "side"},
			was:    "side",
		},
		{
			name:   "another id before on each Mac",
			own:    personal,
			staged: identity.Entry{ID: "z9k4", Organization: "test-org-personal", Added: stagedOn, Was: "home"},
			was:    "home",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := importConfig(t)
			putRecord(t, cfg.StateDir, tt.own)
			before, after := dateOf(20), dateOf(10)
			put(t, requestLines.files(cfg.StateDir).CompressedPath(after), gzipOf(t, named(requestLines.linesOf(after, 1), tt.own.ID)...), now)
			putRecord(t, folder(cfg, 1), tt.staged)
			stageLines(t, folder(cfg, 1), requestLines, before, named(requestLines.linesOf(before, 1), tt.was)...)
			stageLines(t, folder(cfg, 1), requestLines, after, named(requestLines.linesOf(after, 1, 2), tt.staged.ID)...)

			importNow(t, cfg)
			wantHeld(t, cfg, before, named(requestLines.linesOf(before, 1), tt.own.ID))
			wantHeld(t, cfg, after, named(requestLines.linesOf(after, 1, 2), tt.own.ID))
			if got := readRecord(t, cfg.StateDir).Accounts; len(got) != 1 || got[0].ID != tt.own.ID {
				t.Errorf("once imported, the Mac's record is %+v, want the account in it once, under %s", got, tt.own.ID)
			}
		})
	}
}

// A day staged cut short, as one synced part of the way, is taken in of its
// lines up to the damage, under the new ids, warning of it, rather than
// failing the import.
func TestADayStagedCutShortIsTakenInUnderTheNewIDsUpToTheDamage(t *testing.T) {
	log := logstest.Capture(t)
	cfg := importConfig(t)
	putRecord(t, cfg.StateDir, movedWork)
	date := dateOf(10)
	whole := gzipOf(t, requestLines.linesOf(date, 1, 2)...)
	put(t, requestLines.files(folder(cfg, 1)).CompressedPath(date), whole[:len(whole)-4], stagedOn)

	importNow(t, cfg)
	wantHeld(t, cfg, date, asMoved(requestLines.linesOf(date, 1, 2)))
	if !log.Has("level=WARN", "staged to import read short") {
		t.Errorf("log reads\n%s\nwant the day cut short warned of", log)
	}
	checkUnstaged(t, cfg)
}

// A folder whose accounts record contradicts the Mac's, one of its accounts
// two of the Mac's, one by its id and the other by its id before, is
// refused, saying so: nothing taken in, the Mac's record as it was, and what
// was staged removed.
func TestAFolderWhoseAccountsRecordDisagreesIsRefused(t *testing.T) {
	cfg := importConfig(t)
	putRecord(t, cfg.StateDir, movedWork, identity.Entry{ID: "qgr2", Organization: "test-org-personal", Added: stagedOn, Was: "personal"})
	was := stateOf(t, identity.NewFile(cfg.StateDir).Path())
	date := dateOf(10)
	putRecord(t, folder(cfg, 1), identity.Entry{ID: workID, Added: stagedOn, Was: "personal"})
	stageLines(t, folder(cfg, 1), requestLines, date, requestLines.linesOf(date, 1)...)
	runRouter(t, cfg)

	_, err := router.NewClient(router.SocketPath(cfg.StateDir)).Import(t.Context())
	if err == nil || !strings.Contains(err.Error(), "nothing was imported, as the accounts records disagree") {
		t.Errorf("Import() error = %v, want it refused, saying the accounts records disagree", err)
	}
	if got := held(t, requestLines.files(cfg.StateDir), date); len(got) > 0 {
		t.Errorf("refused, the request ledger holds %q, want nothing taken in", got)
	}
	if got := stateOf(t, identity.NewFile(cfg.StateDir).Path()); got != was {
		t.Errorf("refused, the Mac's accounts record is\n%s\nwant it as it was\n%s", got.data, was.data)
	}
	checkUnstaged(t, cfg)
}

// A folder whose accounts record doesn't read is refused, saying so, nothing
// taken in.
func TestAFolderWhoseAccountsRecordDoesntReadIsRefused(t *testing.T) {
	cfg := importConfig(t)
	date := dateOf(10)
	put(t, identity.NewFile(folder(cfg, 1)).Path(), []byte("not a record\n"), stagedOn)
	stageLines(t, folder(cfg, 1), requestLines, date, requestLines.linesOf(date, 1)...)
	runRouter(t, cfg)

	_, err := router.NewClient(router.SocketPath(cfg.StateDir)).Import(t.Context())
	if err == nil || !strings.Contains(err.Error(), "nothing was imported, as the accounts record of a folder staged to import doesn't read") {
		t.Errorf("Import() error = %v, want it refused, saying the folder's accounts record doesn't read", err)
	}
	if got := held(t, requestLines.files(cfg.StateDir), date); len(got) > 0 {
		t.Errorf("refused, the request ledger holds %q, want nothing taken in", got)
	}
	checkUnstaged(t, cfg)
}
