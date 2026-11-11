package router

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
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
	restart := status.Moving{Stage: status.MoveAwaitingRestart}
	tests := []struct {
		name    string
		answers []error
		runs    int
		stands  status.Moving
	}{
		{name: "done at once", runs: 1},
		{name: "failing, then done", answers: []error{errors.New("the disk is full"), errors.New("the disk is full")}, runs: 3},
		{name: "the API not answering, then done", answers: []error{idmove.ErrUnanswered, fmt.Errorf("decide: %w", idmove.ErrUnanswered)}, runs: 3},
		{name: "waiting for the router to restart onto the new ids", answers: []error{idmove.ErrAwaitingRestart}, runs: 1, stands: restart},
		{name: "failing, then waiting for the restart", answers: []error{errors.New("the disk is full"), idmove.ErrAwaitingRestart}, runs: 2, stands: restart},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mover := &answeringMover{answers: tt.answers}
			m := newMoving(mover)
			m.retry, m.ask = time.Millisecond, time.Millisecond
			m.run(t.Context(), func() idmove.History { return idmove.History{} })
			if mover.runs != tt.runs || m.report() != tt.stands || m.owed() != (tt.stands != status.Moving{}) {
				t.Errorf("the move ran %d times, standing %+v, owed %t, want %d, %+v", mover.runs, m.report(), m.owed(), tt.runs, tt.stands)
			}
		})
	}
}

func TestTheMoveSaysWhatHoldsItUpWhileItWaitsToRunAgain(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want status.Moving
	}{
		{name: "the API not answering", err: fmt.Errorf("decide: %w", idmove.ErrUnanswered), want: status.Moving{Stage: status.MoveAwaitingAPI}},
		{name: "failing", err: errors.New("read sk-ant-oat01-abc: denied"), want: status.Moving{Stage: status.MoveFailed, Why: "read " + redact.Placeholder + ": denied"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMoving(&answeringMover{answers: []error{tt.err}})
			m.retry, m.ask = time.Hour, time.Hour
			if got := m.report(); got != (status.Moving{Stage: status.MoveRunning}) {
				t.Errorf("before the move first ran, it stands %+v, want it running", got)
			}
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			go func() {
				defer close(done)
				m.run(ctx, func() idmove.History { return idmove.History{} })
			}()
			deadline := time.Now().Add(5 * time.Second)
			for m.report().Stage == status.MoveRunning && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			got := m.report()
			cancel()
			<-done
			if got != tt.want {
				t.Errorf("waiting to run again, the move stands %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestTheMoveAsksTheAPIAgainAfterWaitsThatDouble(t *testing.T) {
	m := newMoving(&answeringMover{})
	m.ask = time.Second
	var waits []time.Duration
	for last := time.Duration(0); len(waits) < 3; {
		last = m.askAgain(last)
		waits = append(waits, last)
	}
	if want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}; !slices.Equal(waits, want) {
		t.Errorf("the move waits %v to ask the API again, want %v", waits, want)
	}
	if newMoving(&answeringMover{}).ask != idmove.AskAgain(0) {
		t.Error("a router's move first waits other than as idmove's AskAgain has it")
	}
}

func TestTheMoveStopsWithTheRouter(t *testing.T) {
	mover := &answeringMover{answers: []error{errors.New("the disk is full")}}
	m := newMoving(mover)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	m.run(ctx, func() idmove.History { return idmove.History{} })
	if mover.runs != 1 || !m.owed() {
		t.Errorf("the move ran %d times, still owed %t, want it run once, then stopped with the router, still owed", mover.runs, m.owed())
	}
}

func TestNoMoveOwedRunsNothing(t *testing.T) {
	m := newMoving(nil)
	m.settle()
	m.run(t.Context(), func() idmove.History {
		t.Fatal("the history was asked for with no move owed")
		return idmove.History{}
	})
	if m.owed() || m.report() != (status.Moving{}) {
		t.Errorf("no move is owed, but the router says the move stands %+v", m.report())
	}
}
