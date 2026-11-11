package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/idmove"
)

// answeringMover is a move onto the accounts' ids that answers each run with
// the next of its answers, nil once they've run out, counting its runs.
type answeringMover struct {
	answers []error
	runs    int
}

func (m *answeringMover) Settle() error {
	return nil
}

func (m *answeringMover) RunBeside(context.Context, idmove.History) error {
	m.runs++
	if m.runs > len(m.answers) {
		return nil
	}
	return m.answers[m.runs-1]
}

func TestTheMoveRunsAgainWhereItFailsTillItsDone(t *testing.T) {
	tests := []struct {
		name      string
		answers   []error
		runs      int
		stillOwed bool
	}{
		{name: "done at once", runs: 1},
		{name: "failing, then done", answers: []error{errors.New("the disk is full"), errors.New("the disk is full")}, runs: 3},
		{name: "waiting for the router to restart onto the new ids", answers: []error{idmove.ErrAwaitingRestart}, runs: 1, stillOwed: true},
		{name: "failing, then waiting for the restart", answers: []error{errors.New("the disk is full"), idmove.ErrAwaitingRestart}, runs: 2, stillOwed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mover := &answeringMover{answers: tt.answers}
			m := newMoving(mover)
			m.retry = time.Millisecond
			m.run(t.Context(), func() idmove.History { return idmove.History{} })
			if mover.runs != tt.runs || m.owed.Load() != tt.stillOwed {
				t.Errorf("the move ran %d times, still owed %t, want %d, %t", mover.runs, m.owed.Load(), tt.runs, tt.stillOwed)
			}
		})
	}
}

func TestTheMoveStopsWithTheRouter(t *testing.T) {
	mover := &answeringMover{answers: []error{errors.New("the disk is full")}}
	m := newMoving(mover)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	m.run(ctx, func() idmove.History { return idmove.History{} })
	if mover.runs != 1 || !m.owed.Load() {
		t.Errorf("the move ran %d times, still owed %t, want it run once, then stopped with the router, still owed", mover.runs, m.owed.Load())
	}
}

func TestNoMoveOwedRunsNothing(t *testing.T) {
	m := newMoving(nil)
	m.settle()
	m.run(t.Context(), func() idmove.History {
		t.Fatal("the history was asked for with no move owed")
		return idmove.History{}
	})
	if m.owed.Load() {
		t.Error("no move is owed, but the router says it's moving the history")
	}
}
