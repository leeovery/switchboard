package router_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/router"
)

// workID is the id switchboard made for the account the tests' lines name as
// work, its id before switchboard made ids.
const workID = "b6s7"

// otherInstall is the install of another Mac than the tests' routers'.
const otherInstall = "fedcba9876543210"

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

// configuring has cfg configure the accounts with the ids given, each
// labelled by its id, as a Mac does whose accounts record holds them live:
// the router keeps its record in step with its config as it starts.
func configuring(cfg *router.Config, ids ...string) {
	cfg.Accounts = make([]config.Account, len(ids))
	for i, id := range ids {
		cfg.Accounts[i] = config.Account{ID: id, Label: id}
	}
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

// wantRecord fails the test where the accounts record in root doesn't hold
// the accounts given, each by its id, organization, label, removal and id
// before.
func wantRecord(t *testing.T, root string, want ...identity.Entry) {
	t.Helper()
	got := readRecord(t, root).Accounts
	same := len(got) == len(want)
	for i := 0; same && i < len(got); i++ {
		g, w := got[i], want[i]
		same = g.ID == w.ID && g.Organization == w.Organization && g.Label == w.Label && g.Removed.Equal(w.Removed) && g.Was == w.Was
	}
	if !same {
		t.Errorf("the Mac's accounts record is\n%+v\nwant\n%+v", got, want)
	}
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

// stageOthers marks the first folder staged in cfg's state directory another
// Mac's export: of this build, holding its accounts record, where accounts
// says so, else from before switchboard made ids.
func stageOthers(t *testing.T, cfg router.Config, accounts bool) {
	t.Helper()
	stageManifest(t, folder(cfg, 1), export.Manifest{Install: otherInstall, IDs: accounts, Accounts: accounts})
}

// A folder this Mac exported before the move names the accounts by their old
// ids: its days are taken in under the ids the Mac's record made, a line the
// Mac holds under the new id the same line as it, staged under the old, and
// a day's summary counting the account under its new id.
func TestThisMacsFolderExportedBeforeTheMoveIsTakenInUnderTheNewIDs(t *testing.T) {
	cfg := importConfig(t)
	putRecord(t, cfg.StateDir, movedWork)
	configuring(&cfg, workID)
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
	wantRecord(t, cfg.StateDir, movedWork)
	checkUnstaged(t, cfg)
}

// A folder another Mac exported after the move carries its accounts record,
// which joins the Mac's: its days under the new ids are taken in as they
// are, and those it kept from before the move, under the old ids, moved onto
// the new by the record it carries; the account joins without its id before,
// that Mac's own name for it.
func TestAFolderExportedAfterTheMoveIsTakenInByTheRecordItCarries(t *testing.T) {
	cfg := importConfig(t)
	putRecord(t, cfg.StateDir)
	configuring(&cfg)
	before, after := dateOf(20), dateOf(10)
	stageOthers(t, cfg, true)
	putRecord(t, folder(cfg, 1), movedWork)
	stageLines(t, folder(cfg, 1), requestLines, before, requestLines.linesOf(before, 1)...)
	stageLines(t, folder(cfg, 1), requestLines, after, asMoved(requestLines.linesOf(after, 2))...)

	want := router.Imported{Folders: 1, Days: router.ImportedDays{Added: 2}}
	if got := importNow(t, cfg); got != want {
		t.Errorf("Import() = %+v, want %+v", got, want)
	}
	wantHeld(t, cfg, before, asMoved(requestLines.linesOf(before, 1)))
	wantHeld(t, cfg, after, asMoved(requestLines.linesOf(after, 2)))
	wantRecord(t, cfg.StateDir, identity.Entry{ID: workID, Organization: "test-org-work"})
}

// A folder another Mac exported before the move is refused, saying to export
// it again from that Mac, nothing taken in: its ids are that Mac's own names.
// Exported again once that Mac has moved, it's taken in by the record it
// carries.
func TestAnotherMacsFolderFromBeforeTheMoveIsRefusedUntilExportedAgain(t *testing.T) {
	cfg := importConfig(t)
	putRecord(t, cfg.StateDir, movedWork)
	configuring(&cfg, workID)
	date := dateOf(10)
	lines := named(requestLines.linesOf(date, 1, 2), "team")
	runRouter(t, cfg)
	client := router.NewClient(router.SocketPath(cfg.StateDir))

	stageOthers(t, cfg, false)
	stageLines(t, folder(cfg, 1), requestLines, date, lines...)
	_, err := client.Import(t.Context())
	if err == nil || !strings.Contains(err.Error(), "nothing was imported, as the folder mac-"+otherInstall+" can't be taken in: another Mac exported it before switchboard made account ids") {
		t.Errorf("Import() of another Mac's folder from before the move = %v, want it refused, saying to export it again", err)
	}
	wantHeld(t, cfg, date, nil)
	wantRecord(t, cfg.StateDir, movedWork)

	stageOthers(t, cfg, true)
	putRecord(t, folder(cfg, 1), identity.Entry{ID: "r7kq", Label: "team", Added: stagedOn, Removed: stagedOn, Was: "team"})
	stageLines(t, folder(cfg, 1), requestLines, date, lines...)
	if _, err := client.Import(t.Context()); err != nil {
		t.Fatalf("Import() of the folder exported again: %v", err)
	}
	wantHeld(t, cfg, date, named(requestLines.linesOf(date, 1, 2), "r7kq"))
	wantRecord(t, cfg.StateDir, movedWork, identity.Entry{ID: "r7kq", Label: "team", Removed: stagedOn})
}

// This Mac's folder exported before the move, then again after it, holding
// a day of both, has the day taken in once, under the ids the move made, the
// record as it was.
func TestThisMacsFolderBeforeAndAfterTheMoveCountsADayOnce(t *testing.T) {
	cfg := importConfig(t)
	team := identity.Entry{ID: "h7c2", Label: "team", Added: stagedOn, Removed: stagedOn, Was: "team"}
	putRecord(t, cfg.StateDir, movedWork, team)
	configuring(&cfg, workID)
	date := dateOf(10)
	worked, teamed := requestLines.linesOf(date, 1), requestLines.linesOf(date, 2)
	runRouter(t, cfg)
	client := router.NewClient(router.SocketPath(cfg.StateDir))

	stageLines(t, folder(cfg, 1), requestLines, date, slices.Concat(worked, named(teamed, "team"))...)
	if _, err := client.Import(t.Context()); err != nil {
		t.Fatalf("Import() of the folder from before the move: %v", err)
	}
	install, err := export.InstallID(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	stageManifest(t, folder(cfg, 1), export.Manifest{Install: install, Accounts: true})
	putRecord(t, folder(cfg, 1), movedWork, team)
	stageLines(t, folder(cfg, 1), requestLines, date, slices.Concat(asMoved(worked), named(teamed, "h7c2"))...)
	if _, err := client.Import(t.Context()); err != nil {
		t.Fatalf("Import() of the folder from after the move: %v", err)
	}
	wantHeld(t, cfg, date, slices.Concat(asMoved(worked), named(teamed, "h7c2")))
	wantRecord(t, cfg.StateDir, movedWork, team)
}

// A folder whose export.json says it holds its accounts record, which a sync
// hasn't carried yet, is refused, saying so, nothing taken in; once the
// record arrives, it's taken in.
func TestAFolderWhoseRecordHasntSyncedIsRefusedUntilItHas(t *testing.T) {
	cfg := importConfig(t)
	putRecord(t, cfg.StateDir, movedWork)
	date := dateOf(10)
	runRouter(t, cfg)
	client := router.NewClient(router.SocketPath(cfg.StateDir))

	stageOthers(t, cfg, true)
	stageLines(t, folder(cfg, 1), requestLines, date, asMoved(requestLines.linesOf(date, 1))...)
	_, err := client.Import(t.Context())
	if err == nil || !strings.Contains(err.Error(), "nothing was imported, as the folder mac-"+otherInstall+" can't be taken in: its accounts record hasn't synced yet") {
		t.Errorf("Import() of a folder whose record hasn't synced = %v, want it refused, saying so", err)
	}
	wantHeld(t, cfg, date, nil)

	stageOthers(t, cfg, true)
	putRecord(t, folder(cfg, 1), movedWork)
	stageLines(t, folder(cfg, 1), requestLines, date, asMoved(requestLines.linesOf(date, 1))...)
	if _, err := client.Import(t.Context()); err != nil {
		t.Fatalf("Import() once the record synced: %v", err)
	}
	wantHeld(t, cfg, date, asMoved(requestLines.linesOf(date, 1)))
}

// This Mac's install id is read only where a folder needs it, so one that
// doesn't read fails no import of a folder carrying its record.
func TestAnInstallIDThatDoesntReadFailsNoImportThatDoesntNeedIt(t *testing.T) {
	cfg := importConfig(t)
	putRecord(t, cfg.StateDir, movedWork)
	put(t, filepath.Join(cfg.StateDir, "install-id"), []byte("not an id\n"), stagedOn)
	date := dateOf(10)
	stageOthers(t, cfg, true)
	putRecord(t, folder(cfg, 1), movedWork)
	stageLines(t, folder(cfg, 1), requestLines, date, asMoved(requestLines.linesOf(date, 1))...)

	importNow(t, cfg)
	wantHeld(t, cfg, date, asMoved(requestLines.linesOf(date, 1)))
}

// An import judges the accounts this Mac configures by its config file as it
// stands, as the record is kept in step with, never by the config the router
// started with: an account added to the config since, which the record holds
// as configured here, takes no label or organization from another Mac.
func TestAnImportJudgesTheAccountsConfiguredByTheConfigTheRecordFollows(t *testing.T) {
	cfg := importConfig(t)
	putRecord(t, cfg.StateDir, movedWork)
	configuring(&cfg, workID)
	path := filepath.Join(t.TempDir(), "config.toml")
	put(t, path, []byte("[[account]]\nid = \""+workID+"\"\n\n[[account]]\nid = \"h7c2\"\n"), now)
	cfg.ConfigFile = router.Watch(path)
	stageOthers(t, cfg, true)
	putRecord(t, folder(cfg, 1), identity.Entry{ID: "h7c2", Organization: "test-org-team", Label: "Team", Added: stagedOn, Configured: true})

	importNow(t, cfg)
	wantRecord(t, cfg.StateDir, movedWork, identity.Entry{ID: "h7c2"})
}

// A subscription the other Mac has under another id, and had under another
// before, is taken in under this Mac's id, matched by its organization.
func TestAnAccountUnderAnotherIDThereIsTakenInUnderThisMacsByItsOrganization(t *testing.T) {
	cfg := importConfig(t)
	personal := identity.Entry{ID: "qgr2", Organization: "test-org-personal", Added: stagedOn, Was: "personal"}
	putRecord(t, cfg.StateDir, personal)
	configuring(&cfg, personal.ID)
	before, after := dateOf(20), dateOf(10)
	put(t, requestLines.files(cfg.StateDir).CompressedPath(after), gzipOf(t, named(requestLines.linesOf(after, 1), "qgr2")...), now)
	stageOthers(t, cfg, true)
	putRecord(t, folder(cfg, 1), identity.Entry{ID: "z9k4", Organization: "test-org-personal", Added: stagedOn, Was: "home"})
	stageLines(t, folder(cfg, 1), requestLines, before, named(requestLines.linesOf(before, 1), "home")...)
	stageLines(t, folder(cfg, 1), requestLines, after, named(requestLines.linesOf(after, 1, 2), "z9k4")...)

	importNow(t, cfg)
	wantHeld(t, cfg, before, named(requestLines.linesOf(before, 1), "qgr2"))
	wantHeld(t, cfg, after, named(requestLines.linesOf(after, 1, 2), "qgr2"))
	wantRecord(t, cfg.StateDir, personal)
}

// A day staged cut short, as one synced part of the way, is taken in of
// every line that reads before the damage, under the new ids, and none
// after it, warning of it, rather than failing the import.
func TestADayStagedCutShortIsTakenInUnderTheNewIDsUpToTheDamage(t *testing.T) {
	log := logstest.Capture(t)
	cfg := importConfig(t)
	putRecord(t, cfg.StateDir, movedWork)
	date := dateOf(10)
	after := gzipOf(t, requestLines.linesOf(date, 3)...)
	staged := append(gzipOf(t, requestLines.linesOf(date, 1, 2)...), after[:12]...)
	put(t, requestLines.files(folder(cfg, 1)).CompressedPath(date), staged, stagedOn)
	markOwn(t, folder(cfg, 1))

	importNow(t, cfg)
	wantHeld(t, cfg, date, asMoved(requestLines.linesOf(date, 1, 2)))
	if !log.Has("level=WARN", "staged to import read short") {
		t.Errorf("log reads\n%s\nwant the day cut short warned of", log)
	}
	checkUnstaged(t, cfg)
}

// A folder whose accounts record doesn't read is refused, saying so, nothing
// taken in.
func TestAFolderWhoseAccountsRecordDoesntReadIsRefused(t *testing.T) {
	cfg := importConfig(t)
	date := dateOf(10)
	stageOthers(t, cfg, true)
	put(t, identity.NewFile(folder(cfg, 1)).Path(), []byte("not a record\n"), stagedOn)
	stageLines(t, folder(cfg, 1), requestLines, date, requestLines.linesOf(date, 1)...)
	runRouter(t, cfg)

	_, err := router.NewClient(router.SocketPath(cfg.StateDir)).Import(t.Context())
	if err == nil || !strings.Contains(err.Error(), "nothing was imported, as the folder mac-"+otherInstall+" can't be taken in: its accounts record doesn't read") {
		t.Errorf("Import() error = %v, want it refused, saying the folder's accounts record doesn't read", err)
	}
	wantHeld(t, cfg, date, nil)
	checkUnstaged(t, cfg)
}
