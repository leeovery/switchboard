package router

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
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
	restart := status.Moving{Stage: status.MoveAwaitingRestart, Move: status.MovingIDs}
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
			m := newMoving(mover, nil)
			m.retry = time.Millisecond
			m.run(t.Context(), func() idmove.History { return idmove.History{} }, nil)
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
		{name: "the API not answering", err: fmt.Errorf("decide: %w", idmove.ErrUnanswered), want: status.Moving{Stage: status.MoveAwaitingAPI, Move: status.MovingIDs}},
		{name: "the API not naming an account's token's organization", err: &idmove.Unanswered{Account: "work", Why: "HTTP 404 for sk-ant-oat01-abc"},
			want: status.Moving{Stage: status.MoveAwaitingAPI, Move: status.MovingIDs, Account: "work", Why: "HTTP 404 for " + redact.Placeholder}},
		{name: "failing", err: errors.New("read sk-ant-oat01-abc: denied"), want: status.Moving{Stage: status.MoveFailed, Move: status.MovingIDs, Why: "read " + redact.Placeholder + ": denied"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMoving(&answeringMover{answers: []error{tt.err}, wait: time.Hour}, nil)
			m.retry = time.Hour
			if got := m.report(); got != (status.Moving{Stage: status.MoveRunning, Move: status.MovingIDs}) {
				t.Errorf("before the move first ran, it stands %+v, want it running", got)
			}
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			go func() {
				defer close(done)
				m.run(ctx, func() idmove.History { return idmove.History{} }, nil)
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
			m := newMoving(mover, nil)
			m.retry = tc.retry
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			m.run(ctx, func() idmove.History { return idmove.History{} }, nil)
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
	m := newMoving(mover, nil)
	m.retry = time.Millisecond
	m.run(t.Context(), func() idmove.History { return idmove.History{} }, nil)
	if want := []time.Duration{0, time.Millisecond, 2 * time.Millisecond}; !slices.Equal(mover.waited, want) {
		t.Errorf("the move asked its mover how long to wait having waited %v, want %v: none first, then each wait it gave", mover.waited, want)
	}
}

func TestTheMoveStopsWithTheRouter(t *testing.T) {
	mover := &answeringMover{answers: []error{errors.New("the disk is full")}}
	m := newMoving(mover, nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	m.run(ctx, func() idmove.History { return idmove.History{} }, nil)
	if mover.runs != 1 || !m.owed() {
		t.Errorf("the move ran %d times, still owed %t, want it run once, then stopped with the router, still owed", mover.runs, m.owed())
	}
}

func TestNoMoveOwedRunsNothing(t *testing.T) {
	m := newMoving(nil, nil)
	m.settle()
	m.run(t.Context(), func() idmove.History {
		t.Fatal("the history was asked for with no move owed")
		return idmove.History{}
	}, nil)
	if m.owed() || m.report() != (status.Moving{}) {
		t.Errorf("no move is owed, but the router says the move stands %+v", m.report())
	}
}

// answeringPoolMover is a move to pools that answers each run with the next
// of its answers, nil once they've run out, counting its runs, and noting
// whether the router took up the pools as each had it, and the runs of the
// move onto the ids before its first, where it's given that move.
type answeringPoolMover struct {
	answers []error
	runs    int
	tookUp  []bool
	ids     *answeringMover
	idsRuns int
}

func (m *answeringPoolMover) RunBeside(_ context.Context, _ idmove.History, takeUp func() bool) error {
	m.runs++
	m.tookUp = append(m.tookUp, takeUp())
	if m.runs == 1 && m.ids != nil {
		m.idsRuns = m.ids.runs
	}
	if m.runs > len(m.answers) {
		return nil
	}
	return m.answers[m.runs-1]
}

// The move to pools runs once the move onto the ids is done, never while it
// waits for the router to restart onto the new ids; and runs again while it
// waits for the router to take up the first pool, till it's done.
func TestTheMoveToPoolsRunsOnceTheMoveOntoTheIDsIsDone(t *testing.T) {
	awaiting := status.Moving{Stage: status.MoveAwaitingRestart, Move: status.MovingIDs}
	tests := []struct {
		name      string
		ids       []error
		pools     []error
		poolsRuns int
		stands    status.Moving
	}{
		{name: "both done at once", poolsRuns: 1},
		{name: "the move onto the ids failing, then done", ids: []error{errors.New("the disk is full")}, poolsRuns: 1},
		{name: "the move onto the ids waiting for a restart", ids: []error{idmove.ErrAwaitingRestart}, stands: awaiting},
		{name: "the move to pools waiting for the pool taken up, then done", pools: []error{idmove.ErrAwaitingPools}, poolsRuns: 2},
		{name: "the move to pools failing, then done", pools: []error{errors.New("the disk is full")}, poolsRuns: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ids := &answeringMover{answers: tt.ids}
				pools := &answeringPoolMover{answers: tt.pools, ids: ids}
				m := newMoving(ids, pools)
				m.run(t.Context(), func() idmove.History { return idmove.History{} }, func() bool { return true })
				if pools.runs != tt.poolsRuns || m.report() != tt.stands || m.owed() != (tt.stands != status.Moving{}) {
					t.Errorf("the move to pools ran %d times, standing %+v, owed %t; want %d, %+v", pools.runs, m.report(), m.owed(), tt.poolsRuns, tt.stands)
				}
				if pools.runs > 0 && pools.idsRuns != len(tt.ids)+1 {
					t.Errorf("the move to pools first ran after %d runs of the move onto the ids, want it once that was done, after %d", pools.idsRuns, len(tt.ids)+1)
				}
			})
		})
	}
}

// While the move to pools is owed alone, the history waits on it, so
// imports and exports do too, but the move onto the ids isn't owed, so the
// accounts record is kept in step with the config, and tokens held to their
// organizations; the status says the move to pools stands as it does, an
// import is refused in its words, and the move runs again a retry's wait
// later, done once the router routes in pools.
func TestWhileTheMoveToPoolsIsOwedTheHistoryWaitsOnItAlone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pools := &answeringPoolMover{answers: []error{idmove.ErrAwaitingPools}}
		m := newMoving(nil, pools)
		if got := m.report(); got != (status.Moving{Stage: status.MoveRunning, Move: status.MovingPools}) || !m.owed() || m.idsOwed() {
			t.Fatalf("before it ran, the move stands %+v, owed %t, the ids' owed %t; want the move to pools running, owed alone", got, m.owed(), m.idsOwed())
		}
		m.settle()
		var takesUp atomic.Bool
		done := make(chan struct{})
		go func() {
			defer close(done)
			m.run(t.Context(), func() idmove.History { return idmove.History{} }, takesUp.Load)
		}()
		synctest.Wait()
		if want := (status.Moving{Stage: status.MoveAwaitingRestart, Move: status.MovingPools}); m.report() != want || !m.owed() || m.idsOwed() {
			t.Errorf("waiting for the router to take the pool up, the move stands %+v, owed %t, the ids' owed %t; want %+v, owed alone", m.report(), m.owed(), m.idsOwed(), want)
		}
		err := m.historyMoving()
		if !errors.Is(err, errHistoryMoving) || err.Error() != "the history waits on its move to pools: import once it's done" {
			t.Errorf("an import is refused with %v, want it in the words of the move to pools", err)
		}

		takesUp.Store(true)
		time.Sleep(moveRetry)
		<-done
		if m.owed() || !slices.Equal(pools.tookUp, []bool{false, true}) {
			t.Errorf("a retry's wait later, the move is owed %t, told the router routes in pools %v; want it run again as it does, and done", m.owed(), pools.tookUp)
		}
	})
}
