package router_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
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
	data, err := json.Marshal(struct {
		Version  int              `json:"version"`
		Accounts []identity.Entry `json:"accounts"`
	}{1, accounts})
	if err != nil {
		t.Fatal(err)
	}
	put(t, identity.NewFile(root).Path(), append(data, '\n'), stagedOn)
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

// asMoved is lines with the account work named by workID, as the move writes
// them.
func asMoved(lines []string) []string {
	moved := make([]string, len(lines))
	for i, line := range lines {
		moved[i] = strings.ReplaceAll(line, `"account":"work"`, `"account":"`+workID+`"`)
	}
	return moved
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
	before, after := dateOf(20), dateOf(10)
	putRecord(t, folder(cfg, 1), movedWork)
	stageLines(t, folder(cfg, 1), requestLines, before, requestLines.linesOf(before, 1)...)
	stageLines(t, folder(cfg, 1), requestLines, after, asMoved(requestLines.linesOf(after, 2))...)

	want := router.Imported{Folders: 1, Days: router.ImportedDays{Added: 2}}
	if got := importNow(t, cfg); got != want {
		t.Errorf("Import() = %+v, want %+v", got, want)
	}
	for date, n := range map[string]int{before: 1, after: 2} {
		if got, want := held(t, requestLines.files(cfg.StateDir), date), asMoved(requestLines.linesOf(date, n)); !slices.Equal(got, want) {
			t.Errorf("once imported, the request ledger of %s holds\n%s\nwant\n%s", date, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
	if got := readRecord(t, cfg.StateDir); !slices.Equal(got.Accounts, []identity.Entry{movedWork}) {
		t.Errorf("once imported, the Mac's accounts record is %+v, want the folder's account joined to it", got.Accounts)
	}
}

// A folder whose accounts record disagrees with the Mac's is refused, saying
// so: nothing taken in, the Mac's record as it was, and what was staged
// removed.
func TestAFolderWhoseAccountsRecordDisagreesIsRefused(t *testing.T) {
	cfg := importConfig(t)
	putRecord(t, cfg.StateDir, movedWork)
	was := stateOf(t, identity.NewFile(cfg.StateDir).Path())
	date := dateOf(10)
	putRecord(t, folder(cfg, 1), identity.Entry{ID: "k3f9", Organization: movedWork.Organization, Added: stagedOn})
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
