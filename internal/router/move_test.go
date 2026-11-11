package router_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// fakeMover is a move onto the accounts' ids as the router runs it: Settle
// writes the state file it's given, where it's given one, and each run hands
// the history's files it's given to history, then answers as the test sends
// on runs; the API is asked again only after the test's end.
type fakeMover struct {
	statePath, settled string
	history            chan idmove.History
	runs               chan error
}

func newFakeMover() *fakeMover {
	return &fakeMover{history: make(chan idmove.History, 8), runs: make(chan error)}
}

func (m *fakeMover) Settle() error {
	if m.settled == "" {
		return nil
	}
	return os.WriteFile(m.statePath, []byte(m.settled), 0o600)
}

func (m *fakeMover) RunBeside(ctx context.Context, history idmove.History) error {
	m.history <- history
	select {
	case err := <-m.runs:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *fakeMover) AskAgain(time.Duration) time.Duration {
	return time.Hour
}

// moving reports whether the router's status says the move stands as want
// does: zero for none owed.
func moving(want status.Moving) func(status.Document) bool {
	return func(doc status.Document) bool { return doc.Moving == want }
}

// running is the move as the router's status says it stands as it runs.
var running = status.Moving{Stage: status.MoveRunning}

func TestTheRouterSettlesTheMoveBeforeItTakesUpItsState(t *testing.T) {
	cfg := runConfig(t, "http://127.0.0.1:1")
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	mover := newFakeMover()
	mover.statePath = filepath.Join(cfg.StateDir, "state.json")
	mover.settled = `{"version": 1, "pin": {"accounts": ["side"], "since": "2026-09-28T12:00:00Z", "move": false}, "sessions": []}`
	if err := os.WriteFile(mover.statePath, []byte(`{"version": 1, "pin": {"accounts": ["gone"], "since": "2026-09-28T12:00:00Z", "move": false}, "sessions": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.Move = mover
	runRouter(t, cfg)

	doc := waitForStatus(t, router.SocketPath(cfg.StateDir), moving(running))
	if !slices.Equal(doc.Pin.Accounts, []string{"side"}) {
		t.Errorf("the router took up the pin %v, want [side], as the move settled the state file before it took it up", doc.Pin.Accounts)
	}
}

func TestTheRouterSaysItsMovingTheHistoryTillTheMoveIsDone(t *testing.T) {
	cfg := runConfig(t, "http://127.0.0.1:1")
	mover := newFakeMover()
	cfg.Move = mover
	runRouter(t, cfg)
	path := router.SocketPath(cfg.StateDir)

	h := <-mover.history
	if h.Ledger == nil || h.Readings == nil || h.Events == nil {
		t.Fatalf("the move was given the history %+v, want the router's ledger, readings and events", h)
	}
	waitForStatus(t, path, moving(running))
	mover.runs <- nil
	waitForStatus(t, path, moving(status.Moving{}))
}

func TestTheRoutersStatusSaysWhatHoldsTheMoveUp(t *testing.T) {
	cfg := runConfig(t, "http://127.0.0.1:1")
	mover := newFakeMover()
	cfg.Move = mover
	runRouter(t, cfg)
	path := router.SocketPath(cfg.StateDir)

	<-mover.history
	mover.runs <- idmove.ErrUnanswered
	waitForStatus(t, path, moving(status.Moving{Stage: status.MoveAwaitingAPI}))
}

func TestTheRoutersStatusSaysTheMoveWaitsForItToRestart(t *testing.T) {
	cfg := runConfig(t, "http://127.0.0.1:1")
	mover := newFakeMover()
	cfg.Move = mover
	runRouter(t, cfg)
	path := router.SocketPath(cfg.StateDir)

	<-mover.history
	mover.runs <- idmove.ErrAwaitingRestart
	waitForStatus(t, path, moving(status.Moving{Stage: status.MoveAwaitingRestart}))
}

func TestARouterWithNoMoveOwedSaysNothingOfOne(t *testing.T) {
	cfg := runConfig(t, "http://127.0.0.1:1")
	runRouter(t, cfg)
	if doc := waitForStatus(t, router.SocketPath(cfg.StateDir), func(status.Document) bool { return true }); doc.Moving != (status.Moving{}) {
		t.Errorf("a router with no move owed says the move stands %+v", doc.Moving)
	}
}
