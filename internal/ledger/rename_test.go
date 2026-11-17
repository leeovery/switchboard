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

// requestsBy counts the requests of each account's day in the summary the
// ledger in dir holds of the rename tests' day, by the account's id.
func requestsBy(t *testing.T, dir string) map[string]int {
	t.Helper()
	var summary ledger.Summary
	if err := json.Unmarshal(summaryIn(t, dir), &summary); err != nil {
		t.Fatal(err)
	}
	requests := make(map[string]int)
	for _, a := range summary.Accounts {
		for _, m := range a.Models {
			requests[a.Account] += m.Requests()
		}
	}
	return requests
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
	if err := openedIn(dir, on(3, 10, 0)).RenameDay(date, renamedLine, ledger.Renaming{Account: renamedID}); err != nil {
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
	if err := openedIn(dir, on(3, 10, 0)).RenameDay(date, renamedLine, ledger.Renaming{Account: renamedID}); err != nil {
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
	if err := l.RenameDay(date, renamedLine, ledger.Renaming{Account: renamedID}); err != nil {
		t.Fatalf("RenameDay() = %v", err)
	}
	summary, plain := summaryIn(t, dir), readDay(t, dir, on(0, 10, 0))
	if err := l.RenameDay(date, renamedLine, ledger.Renaming{Account: renamedID}); err != nil {
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
	if err := openedIn(dir, on(3, 10, 0)).RenameDay(date, renamedLine, ledger.Renaming{Account: renamedID}); err != nil {
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
	if err := openedIn(dir, on(3, 10, 0)).RenameSummary(date, ledger.Renaming{Account: renamedID}); err != nil {
		t.Fatalf("RenameSummary() = %v", err)
	}
	if got := summaryIn(t, dir); !bytes.Contains(got, []byte(`"account":"wk7q"`)) || bytes.Contains(got, []byte(`"work"`)) {
		t.Errorf("the summary renamed alone is %s, want its accounts renamed", got)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, was) {
		t.Errorf("the day's file, its summary renamed alone, is %q (%v), want it as it was", got, err)
	}
}

func TestASummaryThatRenamedNamesAnAccountTwiceMergesItsDaysTheirLinesPrunedOrNot(t *testing.T) {
	inEitherCase := func(id string) string {
		if strings.EqualFold(id, "work") {
			return "wk7q"
		}
		return id
	}
	for _, pruned := range []bool{false, true} {
		t.Run(map[bool]string{false: "lines kept", true: "lines pruned"}[pruned], func(t *testing.T) {
			dir := t.TempDir()
			writeAt(t, dir, on(0, 12, 0), noReadings,
				&ledger.Line{At: on(0, 10, 0), Request: "r1", Kind: ledger.KindMessage, Session: "s1", Model: "claude-opus-5-5", Account: "Work", Reason: "new", Status: 200, Attempts: 1},
				&ledger.Line{At: on(0, 11, 0), Request: "r2", Kind: ledger.KindMessage, Session: "s2", Model: "claude-opus-5-5", Account: "work", Reason: "new", Status: 200, Attempts: 1},
				&ledger.Line{At: on(0, 11, 30), Request: "r3", Kind: ledger.KindMessage, Session: "s1", Model: "claude-haiku-4-5", Account: "work", Reason: "new", Status: 529},
			)
			summariseAt(dir, t.TempDir(), on(3, 10, 0))
			if pruned {
				if err := os.Remove(dayFile(dir, on(0, 10, 0))); err != nil {
					t.Fatal(err)
				}
			}

			if err := openedIn(dir, on(3, 10, 0)).RenameDay(date, func(line []byte) []byte { return line }, ledger.Renaming{Account: inEitherCase}); err != nil {
				t.Fatalf("RenameDay() = %v", err)
			}
			summariseAt(dir, t.TempDir(), on(3, 11, 0))
			var summary ledger.Summary
			if err := json.Unmarshal(summaryIn(t, dir), &summary); err != nil {
				t.Fatal(err)
			}
			if len(summary.Accounts) != 1 || summary.Accounts[0].Account != "wk7q" {
				t.Fatalf("the day's summary is %+v, want one account, wk7q", summary.Accounts)
			}
			day := summary.Accounts[0]
			var requests []int
			for _, m := range day.Models {
				requests = append(requests, m.Requests())
			}
			if day.Sessions != 2 || !slices.Equal(day.SessionIDs, []string{"s1", "s2"}) || !slices.Equal(requests, []int{1, 2}) {
				t.Errorf("wk7q's day is %+v, want both ids' requests, by model, haiku's then opus's, and each session once", day)
			}
		})
	}
}

func TestASummaryOfAnOlderVersionRenamedIsSummarisedAgainFromItsLines(t *testing.T) {
	for _, tt := range []struct {
		name string
		lay  func(t *testing.T, dir string)
	}{
		{name: "its lines plain", lay: plainLines},
		{name: "its lines compressed, on a file system that keeps whole seconds", lay: compressedLines},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.lay(t, dir)
			markOfVersion(t, dir, ofVersion(summaryOf(date, 2, windowsRead{highest: `{"5h":0.9}`}, theTwo...), 5), 5)
			l := openedIn(dir, on(3, 10, 0))
			if err := l.RenameDay(date, renamedLine, ledger.Renaming{Account: renamedID}); err != nil {
				t.Fatalf("RenameDay() = %v", err)
			}
			var renamed ledger.Summary
			if err := json.Unmarshal(summaryIn(t, dir), &renamed); err != nil || renamed.Version != 5 {
				t.Fatalf("renamed, the summary is of version %d, %v, want its own, 5", renamed.Version, err)
			}

			l.SummariseEnded(on(3, 10, 0))
			var summary ledger.Summary
			if err := json.Unmarshal(summaryIn(t, dir), &summary); err != nil {
				t.Fatal(err)
			}
			if summary.Version != 7 || len(summary.Accounts) != 1 || summary.Accounts[0].Account != "wk7q" || summary.Accounts[0].Models[0].Hours == nil {
				t.Errorf("a round after the rename left the summary %s, want it summarised again from its lines renamed, as version 7, its hours counted", summaryIn(t, dir))
			}
		})
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

// Renaming a day's summary's pools gives each account's work the pool its
// own is renamed to, as an import gives another Mac's pool the pool here
// holding its accounts: two renamed to one merged into one, the account's
// whole as it was; one renamed beside one that isn't kept apart.
func TestRenamingASummarysPoolsMergesTwoGivenOne(t *testing.T) {
	tests := []struct {
		name     string
		renamed  map[string]string
		wantWork []string
	}{
		{name: "both of work's pools given one", renamed: map[string]string{"a1a1": "q7mt", "b2b2": "q7mt"}, wantWork: []string{"q7mt"}},
		{name: "one of work's pools given another", renamed: map[string]string{"a1a1": "q7mt"}, wantWork: []string{"b2b2", "q7mt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeAt(t, dir, on(0, 12, 0), noReadings,
				&ledger.Line{At: on(0, 10, 0), Request: "r1", Kind: ledger.KindMessage, Session: "s1", Model: "claude-opus-5-5", Account: "work", Pool: "a1a1", Reason: "new", Status: 200, Attempts: 1},
				&ledger.Line{At: on(0, 11, 0), Request: "r2", Kind: ledger.KindMessage, Session: "s1", Model: "claude-opus-5-5", Account: "work", Pool: "b2b2", Reason: "new", Status: 200, Attempts: 1},
				&ledger.Line{At: on(0, 11, 30), Request: "r3", Kind: ledger.KindMessage, Session: "s2", Model: "claude-opus-5-5", Account: "personal", Pool: "b2b2", Reason: "new", Status: 200, Attempts: 1},
			)
			summariseAt(dir, t.TempDir(), on(3, 10, 0))
			pool := func(id string) string {
				if to, ok := tt.renamed[id]; ok {
					return to
				}
				return id
			}

			if err := openedIn(dir, on(3, 10, 0)).RenameSummary(date, ledger.Renaming{Pool: pool}); err != nil {
				t.Fatalf("RenameSummary() = %v", err)
			}
			var summary ledger.Summary
			if err := json.Unmarshal(summaryIn(t, dir), &summary); err != nil {
				t.Fatal(err)
			}
			pools := make(map[string][]string)
			for _, a := range summary.Accounts {
				pools[a.Account] = append(pools[a.Account], a.Pool)
				for _, p := range a.Pools {
					pools[a.Account] = append(pools[a.Account], p.Pool)
				}
			}
			wantWork := tt.wantWork
			if len(wantWork) > 1 {
				wantWork = append([]string{""}, wantWork...)
			}
			if !slices.Equal(pools["work"], wantWork) || !slices.Equal(pools["personal"], []string{pool("b2b2")}) {
				t.Errorf("the summary's pools are %q, want work's %q and personal's %q", pools, tt.wantWork, pool("b2b2"))
			}
			if got := requestsBy(t, dir); got["work"] != 2 || got["personal"] != 1 {
				t.Errorf("the summary's requests are %v, want work's 2 and personal's 1, as they were", got)
			}
		})
	}
}

// summaryOfLines is the summary of the day of the rename tests whose lines are
// those given.
func summaryOfLines(t *testing.T, lines ...*ledger.Line) []byte {
	t.Helper()
	dir := t.TempDir()
	writeAt(t, dir, on(0, 12, 0), noReadings, lines...)
	summariseAt(dir, t.TempDir(), on(3, 10, 0))
	return summaryIn(t, dir)
}

// A rename changes a summary only where it renames something in it: an
// account or a pool it names, or an account's work in no pool, given the
// pool for it; never a summary whose work is in pools the rename leaves as
// they are, whatever pool it gives work in none; and never one that doesn't
// read.
func TestARenameChangesASummaryOnlyWhereItRenamesSomethingInIt(t *testing.T) {
	pooled := summaryOfLines(t,
		&ledger.Line{At: on(0, 10, 0), Request: "r1", Kind: ledger.KindMessage, Session: "s1", Model: "claude-opus-5-5", Account: "work", Pool: "a1a1", Reason: "new", Status: 200, Attempts: 1},
		&ledger.Line{At: on(0, 11, 0), Request: "r2", Kind: ledger.KindMessage, Session: "s2", Model: "claude-opus-5-5", Account: "personal", Pool: "b2b2", Reason: "new", Status: 200, Attempts: 1},
	)
	unpooled := summaryOfLines(t,
		&ledger.Line{At: on(0, 10, 0), Request: "r1", Kind: ledger.KindMessage, Session: "s1", Model: "claude-opus-5-5", Account: "work", Reason: "new", Status: 200, Attempts: 1},
	)
	same := func(id string) string { return id }
	tests := []struct {
		name    string
		summary []byte
		rename  ledger.Renaming
		want    bool
	}{
		{name: "every account and pool as it is, work in none given one", summary: pooled, rename: ledger.Renaming{Account: same, Pool: same, Unpooled: "z9z9"}},
		{name: "an account renamed", summary: pooled, rename: ledger.Renaming{Account: renamedID}, want: true},
		{name: "a pool renamed", summary: pooled, rename: ledger.Renaming{Pool: strings.ToUpper}, want: true},
		{name: "work in no pool given one", summary: unpooled, rename: ledger.Renaming{Unpooled: "z9z9"}, want: true},
		{name: "one that doesn't read", summary: []byte(`{"accounts":`), rename: ledger.Renaming{Account: renamedID, Unpooled: "z9z9"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rename.Changes(tt.summary); got != tt.want {
				t.Errorf("Changes() = %v, want %v", got, tt.want)
			}
		})
	}
}
