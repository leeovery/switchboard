package cli_test

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
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

// The marks, in the state directory, of the history's moves done.
const (
	idsMark   = "accounts.moved"
	poolsMark = "pools.moved"
)

// markMoved marks the moves of the history done in the state directory dir,
// onto the accounts' ids and to pools, as the router does once it has made
// them: a test whose history isn't one waiting on a move has it marked so.
func markMoved(t *testing.T, dir string) {
	t.Helper()
	markDone(t, dir, idsMark, poolsMark)
}

// markDone marks the moves of the history done in the state directory dir
// that the marks given mark.
func markDone(t *testing.T, dir string, marks ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, mark := range marks {
		if err := os.WriteFile(filepath.Join(dir, mark), []byte("2026-09-28T13:12:00Z\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// noRouter is what a command says holds the move onto the accounts' ids up
// where no router answers, and noRouterPools the move to pools.
var (
	noRouter      = status.Moving{Stage: status.MoveNoRouter}.Said()
	noRouterPools = status.Moving{Stage: status.MoveNoRouter, Move: status.MovingPools}.Said()
)

func TestTheVerbsThatReadTheHistoryWaitOnItsMoves(t *testing.T) {
	folder := t.TempDir()
	moves := []struct {
		name string
		// done marks the moves done before it.
		done []string
		said string
	}{
		{name: "onto the accounts' ids", said: noRouter},
		{name: "to pools", done: []string{idsMark}, said: noRouterPools},
	}
	for _, move := range moves {
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
			t.Run(move.name+", "+strings.Join(args, " "), func(t *testing.T) {
				deps, dir := movingDeps(t)
				markDone(t, dir, move.done...)
				got := run(t, deps, args...)
				if want := "Error: " + move.said + "; try again once it's done\n"; got.code != 1 || got.stderr != want || got.stdout != "" {
					t.Errorf("switchboard %s, the history waiting on its move, = %+v, want it refused: %q", strings.Join(args, " "), got, want)
				}
			})
		}
	}
}

// A router that's there but doesn't say how the move stands, as one busy or
// restarting, is told apart from none.
func TestAVerbWaitingOnTheMoveSaysWhereTheRouterDidntSayHowItStands(t *testing.T) {
	deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": writeConfig(t, "[[account]]\nid = \"work\"\n"), "XDG_STATE_HOME": shortTempDir(t)}, t.TempDir())
	dir := stateDir(t, deps)
	if err := os.MkdirAll(ledger.Dir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", router.SocketPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	got := run(t, deps, "events")
	untold := status.Moving{Stage: status.MoveUntold}
	if want := "Error: " + untold.Said() + "; try again once it's done\n"; got.code != 1 || got.stderr != want {
		t.Errorf("switchboard events, the router not saying, = %+v, want it refused: %q", got, want)
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
		return stdout.String() == noRouter+"; the dashboard opens once it's done\n"
	})
	select {
	case <-opened:
		t.Fatal("the dashboard opened while the history waited on its move")
	case <-time.After(20 * time.Millisecond):
	}
	markDone(t, dir, idsMark)
	waitUntil(t, "the dashboard says it waits on the move to pools", func() bool {
		return strings.HasSuffix(stdout.String(), "\n"+noRouterPools+"; the dashboard opens once it's done\n")
	})
	select {
	case <-opened:
		t.Fatal("the dashboard opened while the history waited on its move to pools")
	case <-time.After(20 * time.Millisecond):
	}
	markDone(t, dir, poolsMark)
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
	doc := srv.waitForStatus(t, func(doc status.Document) bool { return doc.Moving.Stage == status.MoveAwaitingRestart })
	if _, ok := doc.Account("work"); !ok {
		t.Errorf("the router routes under %v, want the old ids till it's restarted", doc.Accounts)
	}
	if got := run(t, srv.deps, "requests"); got.code != 1 || !strings.Contains(got.stderr, prose.HistoryMoving+": "+prose.MoveWaitsRestart) {
		t.Errorf("switchboard requests mid-move = %+v, want it refused, waiting on the router's restart", got)
	}
	if got := stop(); got.code != 0 {
		t.Fatalf("switchboard serve = %+v, want exit status 0", got)
	}

	stop = srv.start(t)
	doc = srv.waitForStatus(t, func(doc status.Document) bool { return doc.Moving == status.Moving{} })
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

	// The config put back as it was, as from a repository it's kept in, a
	// router starting heals it.
	if got := stop(); got.code != 0 {
		t.Fatalf("switchboard serve = %+v, want exit status 0", got)
	}
	srv.writeConfig(t)
	srv.start(t)
	doc = srv.waitForStatus(t, func(status.Document) bool { return true })
	if _, ok := doc.Account(work.ID); !ok {
		t.Errorf("started on a config put back to the old ids, the router routes under %v, want work's new id, %s", doc.Accounts, work.ID)
	}
	if data, err := os.ReadFile(srv.config); err != nil || strings.Contains(string(data), `"work"`) || !strings.Contains(string(data), `"`+work.ID+`"`) {
		t.Errorf("the config put back reads %s (%v), want it healed onto the new ids", data, err)
	}
}

func TestServeMovesTheHistoryToPoolsAcrossTheRestartItCallsFor(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	if err := os.Remove(filepath.Join(srv.state, poolsMark)); err != nil {
		t.Fatal(err)
	}
	requests := filepath.Join(ledger.Dir(srv.state), "requests-"+dayfile.DateOf(testNow)+".jsonl")
	if err := os.MkdirAll(filepath.Dir(requests), 0o700); err != nil {
		t.Fatal(err)
	}
	line := `{"at":"2026-09-28T12:00:00Z","request":"r1","kind":"messages","account":"work","status":200,"attempts":1,"total_ms":5}` + "\n"
	if err := os.WriteFile(requests, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}

	stop := srv.start(t)
	waitUntil(t, "the config holds the first pool", func() bool {
		data, err := os.ReadFile(srv.config)
		return err == nil && strings.Contains(string(data), "[[pool]]")
	})
	awaiting := status.Moving{Stage: status.MoveAwaitingRestart, Move: status.MovingPools}
	srv.waitForStatus(t, func(doc status.Document) bool { return doc.Moving == awaiting })
	if got := run(t, srv.deps, "requests"); got.code != 1 || !strings.Contains(got.stderr, prose.HistoryMovingPools+": "+prose.MoveWaitsPools) {
		t.Errorf("switchboard requests mid-move = %+v, want it refused, waiting on the router taking the first pool up", got)
	}
	if data, err := os.ReadFile(requests); err != nil || string(data) != line {
		t.Errorf("the ledger's line reads %s (%v), want it as it was till the router takes the first pool up", data, err)
	}
	if got := stop(); got.code != 0 {
		t.Fatalf("switchboard serve = %+v, want exit status 0", got)
	}

	stop = srv.start(t)
	srv.waitForStatus(t, func(doc status.Document) bool { return doc.Moving == status.Moving{} })
	record, err := identity.NewFile(srv.state).Read()
	if err != nil {
		t.Fatal(err)
	}
	first, ok := record.FirstPool()
	if !ok {
		t.Fatal("the accounts record notes no first pool")
	}
	if data, err := os.ReadFile(requests); err != nil || !strings.Contains(string(data), `"kind":"messages","pool":"`+first.ID+`","account":"work"`) {
		t.Errorf("the ledger's line reads %s (%v), want it given the first pool, %s", data, err, first.ID)
	}
	if _, err := os.Stat(filepath.Join(srv.state, poolsMark)); err != nil {
		t.Errorf("the move to pools' mark: %v, want it marked done", err)
	}
	if got := run(t, srv.deps, "requests"); got.code != 0 {
		t.Errorf("switchboard requests, moved, = %+v, want the ledger read", got)
	}

	// The config put back as it was, as from a repository it's kept in, a
	// router starting heals it.
	if got := stop(); got.code != 0 {
		t.Fatalf("switchboard serve = %+v, want exit status 0", got)
	}
	srv.writeConfig(t)
	srv.start(t)
	srv.waitForStatus(t, func(status.Document) bool { return true })
	cfg, err := config.Load(srv.config, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pool, ok := cfg.Pools.ByID(first.ID); !ok || pool.Label != "Default" || len(cfg.Pools) != 1 {
		t.Errorf("the config put back holds the pools %+v, want it healed with the first, %s, labelled Default", cfg.Pools, first.ID)
	}
}
