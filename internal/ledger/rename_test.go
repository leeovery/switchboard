package ledger_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
)

// renamedTo are the ids the rename tests' accounts are renamed to, by the
// ids they had.
var renamedTo = map[string]string{"work": "wk7q", "personal": "ab3k"}

// renamedID renames an id as renamedTo does, leaving any other as it is.
func renamedID(id string) string {
	if to, ok := renamedTo[id]; ok {
		return to
	}
	return id
}

// renamedLine renames each account a line names in quotes, as renamedTo
// does.
func renamedLine(line []byte) []byte {
	for from, to := range renamedTo {
		line = bytes.ReplaceAll(line, []byte(`"`+from+`"`), []byte(`"`+to+`"`))
	}
	return line
}

// summarisedDay has a ledger in a directory of its own write lines of the
// rename tests' day, and summarise the day, and returns the directory.
func summarisedDay(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeAt(t, dir, on(0, 12, 0), noReadings,
		&ledger.Line{At: on(0, 10, 0), Request: "r1", Kind: ledger.KindMessage, Account: "work", Reason: "new", Status: 200, Attempts: 1},
		&ledger.Line{At: on(0, 11, 0), Request: "r2", Kind: ledger.KindMessage, Account: "personal", Reason: "new", Status: 200, Attempts: 1},
		&ledger.Line{At: on(0, 11, 30), Request: "r3", Kind: ledger.KindMessage, Reason: "no account has room", Status: 529},
	)
	summariseAt(dir, t.TempDir(), on(3, 10, 0))
	return dir
}

// openedIn is the ledger in dir, as a router started at at opens it.
func openedIn(dir string, at time.Time) *ledger.Ledger {
	return ledger.Open(dir, 400*24*time.Hour, func() time.Time { return at }, noReadings, caps, logs.For("router"))
}

// summaryIn returns the summary the ledger in dir holds of the rename
// tests' day, as written.
func summaryIn(t *testing.T, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(ledger.SummaryFile(dir, date))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRenamingADayRenamesItsLinesAndItsSummarysAccountsInOrder(t *testing.T) {
	dir := summarisedDay(t)
	if err := openedIn(dir, on(3, 10, 0)).RenameDay(date, renamedLine, renamedID); err != nil {
		t.Fatalf("RenameDay() = %v", err)
	}
	var summary ledger.Summary
	if err := json.Unmarshal(summaryIn(t, dir), &summary); err != nil {
		t.Fatal(err)
	}
	var accounts []string
	for _, a := range summary.Accounts {
		accounts = append(accounts, a.Account)
	}
	if want := []string{"", "ab3k", "wk7q"}; !slices.Equal(accounts, want) {
		t.Errorf("the summary's accounts are %q, want %q, renamed, in the order of their ids", accounts, want)
	}
	var named []string
	for text := range strings.Lines(readDay(t, dir, on(0, 10, 0))) {
		var l ledger.Line
		if err := json.Unmarshal([]byte(text), &l); err != nil {
			t.Fatal(err)
		}
		named = append(named, l.Account)
	}
	if want := []string{"wk7q", "ab3k", ""}; !slices.Equal(named, want) {
		t.Errorf("the day's lines are of %q, want %q", named, want)
	}
}

func TestARenamedSummaryStandsWithoutItsLinesCountedAgain(t *testing.T) {
	dir := summarisedDay(t)
	if err := openedIn(dir, on(3, 10, 0)).RenameDay(date, renamedLine, renamedID); err != nil {
		t.Fatalf("RenameDay() = %v", err)
	}
	renamed := summaryIn(t, dir)
	info, err := os.Stat(ledger.SummaryFile(dir, date))
	if err != nil {
		t.Fatal(err)
	}
	openedIn(dir, on(3, 11, 0)).SummariseEnded(on(3, 11, 0))
	again, err := os.Stat(ledger.SummaryFile(dir, date))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(summaryIn(t, dir), renamed) || !again.ModTime().Equal(info.ModTime()) {
		t.Errorf("a round wrote the renamed summary again, want it to stand as it was marked:\nwas %s\nnow %s", renamed, summaryIn(t, dir))
	}
}

func TestRenamingADayAlreadyRenamedChangesNothing(t *testing.T) {
	dir := summarisedDay(t)
	l := openedIn(dir, on(3, 10, 0))
	if err := l.RenameDay(date, renamedLine, renamedID); err != nil {
		t.Fatalf("RenameDay() = %v", err)
	}
	summary, plain := summaryIn(t, dir), readDay(t, dir, on(0, 10, 0))
	if err := l.RenameDay(date, renamedLine, renamedID); err != nil {
		t.Fatalf("RenameDay() again = %v", err)
	}
	if !bytes.Equal(summaryIn(t, dir), summary) || readDay(t, dir, on(0, 10, 0)) != plain {
		t.Error("renaming the day again changed it, want it as the first rename left it")
	}
}

func TestRenamingADayOfASummaryAloneRenamesItsSummary(t *testing.T) {
	dir := summarisedDay(t)
	if err := os.Remove(dayFile(dir, on(0, 10, 0))); err != nil {
		t.Fatal(err)
	}
	if err := openedIn(dir, on(3, 10, 0)).RenameDay(date, renamedLine, renamedID); err != nil {
		t.Fatalf("RenameDay() = %v", err)
	}
	if got := summaryIn(t, dir); !bytes.Contains(got, []byte(`"account":"wk7q"`)) || bytes.Contains(got, []byte(`"work"`)) {
		t.Errorf("the summary of a day of no lines is %s, want its accounts renamed", got)
	}
}

// A day's summary renamed alone has its accounts renamed, its day's files as
// they were.
func TestRenamingADaysSummaryAloneLeavesItsFiles(t *testing.T) {
	dir := summarisedDay(t)
	path := dayFile(dir, on(0, 10, 0))
	was, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := openedIn(dir, on(3, 10, 0)).RenameSummary(date, renamedID); err != nil {
		t.Fatalf("RenameSummary() = %v", err)
	}
	if got := summaryIn(t, dir); !bytes.Contains(got, []byte(`"account":"wk7q"`)) || bytes.Contains(got, []byte(`"work"`)) {
		t.Errorf("the summary renamed alone is %s, want its accounts renamed", got)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, was) {
		t.Errorf("the day's file, its summary renamed alone, is %q (%v), want it as it was", got, err)
	}
}

func TestASummaryThatRenamedNamesAnAccountTwiceIsLeftForARoundToMakeAfresh(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, dir, on(0, 12, 0), noReadings,
		&ledger.Line{At: on(0, 10, 0), Request: "r1", Kind: ledger.KindMessage, Account: "work", Reason: "new", Status: 200, Attempts: 1},
		&ledger.Line{At: on(0, 11, 0), Request: "r2", Kind: ledger.KindMessage, Account: "wk7q", Reason: "new", Status: 200, Attempts: 1},
	)
	summariseAt(dir, t.TempDir(), on(3, 10, 0))

	if err := openedIn(dir, on(3, 10, 0)).RenameDay(date, renamedLine, renamedID); err != nil {
		t.Fatalf("RenameDay() = %v", err)
	}
	summariseAt(dir, t.TempDir(), on(3, 11, 0))
	var summary ledger.Summary
	if err := json.Unmarshal(summaryIn(t, dir), &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Accounts) != 1 || summary.Accounts[0].Account != "wk7q" || summary.Accounts[0].Models[0].Upstream != 2 {
		t.Errorf("the day's summary, made afresh, is %+v, want one account, wk7q, of both requests", summary.Accounts)
	}
}

func TestRebuildingDeletesWhatsBuiltOfTheDaysAlone(t *testing.T) {
	dir := summarisedDay(t)
	built := []string{"digest-" + date + ".json.gz", "parts-" + date + ".gz", "logday-" + date + ".json.gz", "rows-" + date + ".gz"}
	for _, name := range built {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("built"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := openedIn(dir, on(3, 10, 0)).Rebuild(); err != nil {
		t.Fatalf("Rebuild() = %v", err)
	}
	for _, name := range built {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s is still there, want it deleted", name)
		}
	}
	for _, kept := range []string{dayFile(dir, on(0, 10, 0)), ledger.SummaryFile(dir, date)} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s = %v, want it kept", kept, err)
		}
	}
}
