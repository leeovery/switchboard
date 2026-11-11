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
// the next of its answers, nil once they've run out, counting its runs; and
// has the API asked again after wait more than the wait before, noting each
// wait before it's handed.
type answeringMover struct {
	answers []error
	runs    int
	ranAt   []time.Time
	wait    time.Duration
	waited  []time.Duration
}

func (m *answeringMover) Settle() error {
	return nil
}

func (m *answeringMover) RunBeside(context.Context, idmove.History) error {
	m.runs++
	m.ranAt = append(m.ranAt, time.Now())
	if m.runs > len(m.answers) {
		return nil
	}
	return m.answers[m.runs-1]
}

func (m *answeringMover) AskAgain(wait time.Duration) time.Duration {
	m.waited = append(m.waited, wait)
	return wait + m.wait
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
			mover := &answeringMover{answers: tt.answers, wait: time.Millisecond}
			m := newMoving(mover)
			m.retry = time.Millisecond
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
		{name: "the API not naming an account's token's organization", err: &idmove.Unanswered{Account: "work", Why: "HTTP 404 for sk-ant-oat01-abc"},
			want: status.Moving{Stage: status.MoveAwaitingAPI, Account: "work", Why: "HTTP 404 for " + redact.Placeholder}},
		{name: "failing", err: errors.New("read sk-ant-oat01-abc: denied"), want: status.Moving{Stage: status.MoveFailed, Why: "read " + redact.Placeholder + ": denied"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMoving(&answeringMover{answers: []error{tt.err}, wait: time.Hour})
			m.retry = time.Hour
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

// The move waits as its mover says where the API didn't answer, and the
// retry's wait where it failed otherwise: a run waiting the other's wait, an
// hour, wouldn't run again before the test's deadline, nor one waiting the
// retry's in place of its mover's longer one as long as that.
func TestTheMoveWaitsAsItsMoverSaysOnlyWhereTheAPIDidntAnswer(t *testing.T) {
	for _, tc := range []struct {
		name        string
		err         error
		retry, wait time.Duration
		// waits is the wait between the two runs at the least.
		waits time.Duration
	}{
		{name: "the API not answering", err: idmove.ErrUnanswered, retry: time.Hour, wait: time.Millisecond},
		{name: "the API not answering, its mover's wait the longer", err: idmove.ErrUnanswered, retry: time.Millisecond, wait: 100 * time.Millisecond, waits: 100 * time.Millisecond},
		{name: "failing", err: errors.New("the disk is full"), retry: time.Millisecond, wait: time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mover := &answeringMover{answers: []error{tc.err}, wait: tc.wait}
			m := newMoving(mover)
			m.retry = tc.retry
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			m.run(ctx, func() idmove.History { return idmove.History{} })
			want := tc.retry
			if errors.Is(tc.err, idmove.ErrUnanswered) {
				want = tc.wait
			}
			if mover.runs != 2 || m.owed() {
				t.Fatalf("the move ran %d times, still owed %t, want it run again after the %v wait, and done", mover.runs, m.owed(), want)
			}
			if waited := mover.ranAt[1].Sub(mover.ranAt[0]); waited < tc.waits {
				t.Errorf("the move ran again %v after it ran, want %v at the least, its mover's wait", waited, tc.waits)
			}
		})
	}
}

func TestTheMoveAsksTheAPIAgainAfterTheWaitsItsMoverGives(t *testing.T) {
	mover := &answeringMover{answers: []error{idmove.ErrUnanswered, errors.New("the disk is full"), idmove.ErrUnanswered, idmove.ErrUnanswered}, wait: time.Millisecond}
	m := newMoving(mover)
	m.retry = time.Millisecond
	m.run(t.Context(), func() idmove.History { return idmove.History{} })
	if want := []time.Duration{0, time.Millisecond, 2 * time.Millisecond}; !slices.Equal(mover.waited, want) {
		t.Errorf("the move asked its mover how long to wait having waited %v, want %v: none first, then each wait it gave", mover.waited, want)
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
