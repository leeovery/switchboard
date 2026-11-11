package cli_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// movingDeps are commands' deps with a config of their own, whose history
// waits on its move onto the accounts' ids: the state directory they find
// holds a request ledger's directory, the move not marked done. It returns
// the state directory too.
func movingDeps(t *testing.T) (cli.Deps, string) {
	t.Helper()
	deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": writeConfig(t, "[[account]]\nid = \"work\"\n")}, t.TempDir())
	dir := stateDir(t, deps)
	if err := os.MkdirAll(ledger.Dir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	return deps, dir
}

// markMoved marks the move onto the accounts' ids done in the state
// directory dir, as the router does once it has moved the history: a test
// whose history isn't one waiting on the move has it marked so.
func markMoved(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "accounts.moved"), []byte("2026-09-28T13:12:00Z\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTheVerbsThatReadTheHistoryWaitOnItsMove(t *testing.T) {
	folder := t.TempDir()
	for _, args := range [][]string{
		{"events"},
		{"history"},
		{"history", "--windows"},
		{"requests"},
		{"sessions"},
		{"sessions", "0b5c6f2e"},
		{"export", folder},
		{"import", folder},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			deps, _ := movingDeps(t)
			got := run(t, deps, args...)
			if want := "Error: " + prose.HistoryMoving + ": try again once it's done\n"; got.code != 1 || got.stderr != want || got.stdout != "" {
				t.Errorf("switchboard %s, the history waiting on its move, = %+v, want it refused: %q", strings.Join(args, " "), got, want)
			}
		})
	}
}

func TestTheVerbsReadTheHistoryOnceItsMoved(t *testing.T) {
	deps, dir := movingDeps(t)
	markMoved(t, dir)
	if got := onATerminal.run(t, deps, "events"); got.code != 0 || !strings.HasPrefix(got.stdout, "no events since") {
		t.Errorf("switchboard events, the history moved, = %+v, want it read", got)
	}
}

func TestTheDashboardOpensOnceTheHistoryIsMoved(t *testing.T) {
	deps, dir := movingDeps(t)
	deps.FollowEvery = time.Millisecond
	opened := make(chan struct{})
	deps.Watch = func(context.Context, watch.Config, io.Writer, []string) error {
		close(opened)
		return nil
	}
	root := cli.NewRootCommand(deps)
	root.SetArgs([]string{"dashboard"})
	stdout := &syncBuffer{}
	root.SetOut(stdout)
	root.SetErr(io.Discard)
	code := make(chan int, 1)
	go func() { code <- cli.Execute(root) }()

	waitUntil(t, "the dashboard says it waits on the move", func() bool {
		return stdout.String() == prose.HistoryMoving+": the dashboard opens once it's done\n"
	})
	select {
	case <-opened:
		t.Fatal("the dashboard opened while the history waited on its move")
	case <-time.After(20 * time.Millisecond):
	}
	markMoved(t, dir)
	<-opened
	if got := <-code; got != 0 {
		t.Errorf("switchboard dashboard exited %d, want 0 once it opened", got)
	}
}

func TestServeMovesTheAccountsAndTheirHistoryOntoTheirIDsAcrossTheRestartItCallsFor(t *testing.T) {
	srv := newServeSetup(t, newOrganizationsAPI(t).URL, nil)
	if err := os.Remove(filepath.Join(srv.state, "accounts.moved")); err != nil {
		t.Fatal(err)
	}
	requests := filepath.Join(ledger.Dir(srv.state), "requests-"+dayfile.DateOf(testNow)+".jsonl")
	if err := os.MkdirAll(filepath.Dir(requests), 0o700); err != nil {
		t.Fatal(err)
	}
	line := `{"at":"2026-09-28T12:00:00Z","request":"r1","kind":"messages","account":"work","reason":"moved: side hit its limit","status":200,"attempts":1,"total_ms":5}` + "\n"
	if err := os.WriteFile(requests, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}

	stop := srv.start(t)
	waitUntil(t, "the config holds the accounts' new ids", func() bool {
		data, err := os.ReadFile(srv.config)
		return err == nil && !strings.Contains(string(data), `"work"`)
	})
	doc := srv.waitForStatus(t, func(status.Document) bool { return true })
	if !doc.Moving {
		t.Error("the router doesn't say it's moving the history, want it to till it's restarted onto the new ids")
	}
	if _, ok := doc.Account("work"); !ok {
		t.Errorf("the router routes under %v, want the old ids till it's restarted", doc.Accounts)
	}
	if got := run(t, srv.deps, "requests"); got.code != 1 || !strings.Contains(got.stderr, prose.HistoryMoving) {
		t.Errorf("switchboard requests mid-move = %+v, want it refused, waiting on the move", got)
	}
	if got := stop(); got.code != 0 {
		t.Fatalf("switchboard serve = %+v, want exit status 0", got)
	}

	srv.start(t)
	doc = srv.waitForStatus(t, func(doc status.Document) bool { return !doc.Moving })
	record, err := identity.NewFile(srv.state).Read()
	if err != nil {
		t.Fatal(err)
	}
	work, _ := record.ByWas("work")
	side, _ := record.ByWas("side")
	if work.Organization != "org-work" || side.Organization != "org-side" || !identity.Spells("org-work", work.ID) || !identity.Spells("org-side", side.ID) {
		t.Errorf("the record holds work as %+v and side as %+v, want each on the id its organization derives", work, side)
	}
	if _, ok := doc.Account(work.ID); !ok {
		t.Errorf("the router routes under %v, want work's new id, %s, once restarted", doc.Accounts, work.ID)
	}
	data, err := os.ReadFile(requests)
	if want := `"account":"` + work.ID + `","reason":"moved: ` + side.ID + ` hit its limit"`; err != nil || !strings.Contains(string(data), want) {
		t.Errorf("the ledger's line reads %s (%v), want it moved onto the new ids: %s", data, err, want)
	}
	if got := run(t, srv.deps, "requests"); got.code != 0 {
		t.Errorf("switchboard requests, moved, = %+v, want the ledger read", got)
	}
	for _, id := range []string{work.ID, side.ID} {
		if _, err := os.Stat(filepath.Join(srv.state, "tokens", id)); err != nil {
			t.Errorf("%s's token file: %v, want it there", id, err)
		}
	}
	for _, id := range []string{"work", "side"} {
		if _, err := os.Lstat(filepath.Join(srv.state, "tokens", id)); err == nil {
			t.Errorf("the token file of the former id %s is still there", id)
		}
	}
}
