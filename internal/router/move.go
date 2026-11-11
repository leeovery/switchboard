package router

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
)

// moveRetry is how long the router waits to run the move onto the accounts'
// ids again where it failed.
const moveRetry = time.Minute

// errHistoryMoving is what an import fails with while the history waits on
// its move onto the accounts' ids.
var errHistoryMoving = errors.New(prose.HistoryMoving + ": import once it's done")

// Mover moves the accounts and their history onto the ids switchboard makes,
// once, as idmove.Mover does: Settle moves the files the router takes up as
// it starts, before it does, and RunBeside the rest, beside the router, given
// the history's files as it keeps them, failing with idmove.ErrUnanswered,
// having changed nothing, where the API doesn't answer, to run again once
// AskAgain's wait is up, given the wait before, zero the first time.
type Mover interface {
	Settle() error
	RunBeside(ctx context.Context, history idmove.History) error
	AskAgain(wait time.Duration) time.Duration
}

// moving runs the move onto the accounts' ids beside the router, where it has
// one, and says how it stands.
type moving struct {
	mover Mover
	// retry is how long the move waits to run again once it fails otherwise
	// than for the API not answering.
	retry time.Duration
	// stands is how the move stands while it's owed or running, as the
	// status says it: from the router's start, where it has a mover, until
	// the move is done. Nil where none is owed.
	stands atomic.Pointer[status.Moving]
}

// newMoving returns the moving of mover's move, none where it's nil.
func newMoving(mover Mover) *moving {
	m := &moving{mover: mover, retry: moveRetry}
	if mover != nil {
		m.standing(status.Moving{Stage: status.MoveRunning})
	}
	return m
}

// owed reports whether the move is owed: not done yet.
func (m *moving) owed() bool {
	return m.stands.Load() != nil
}

// report is how the move stands, for the status document: zero where none is
// owed.
func (m *moving) report() status.Moving {
	if s := m.stands.Load(); s != nil {
		return *s
	}
	return status.Moving{}
}

// standing notes how the move stands.
func (m *moving) standing(s status.Moving) {
	m.stands.Store(&s)
}

// settle has the mover move the files the router takes up as it starts,
// where a move's owed. Where it can't, the router starts all the same, the
// state those files kept of the accounts moved let go of.
func (m *moving) settle() {
	if !m.owed() {
		return
	}
	if err := m.mover.Settle(); err != nil {
		logger.Warn("couldn't move the state file and the newest readings onto the accounts' ids, so what they kept of the accounts moved is let go of", "error", err)
	}
}

// run runs the move, given the history's files as history gives them, until
// it's done, ctx ends, or it waits for the router to restart onto the
// accounts' new ids, which its config holds by then: where the API doesn't
// answer, it runs again after the wait the mover's AskAgain gives; where it
// fails otherwise, after the retry's wait. It notes how the move stands
// throughout, as the status says it.
func (m *moving) run(ctx context.Context, history func() idmove.History) {
	if !m.owed() {
		return
	}
	var asked time.Duration
	for {
		m.standing(status.Moving{Stage: status.MoveRunning})
		err := m.mover.RunBeside(ctx, history())
		wait := m.retry
		switch {
		case err == nil:
			m.stands.Store(nil)
			return
		case ctx.Err() != nil:
			return
		case errors.Is(err, idmove.ErrAwaitingRestart):
			m.standing(status.Moving{Stage: status.MoveAwaitingRestart})
			logger.Info("the move onto the accounts' ids waits for the router to restart onto them")
			return
		case errors.Is(err, idmove.ErrUnanswered):
			asked = m.mover.AskAgain(asked)
			wait = asked
			m.standing(awaitingAPI(err))
			logger.Info("the move onto the accounts' ids waits to ask the API again for the organizations of their tokens", "in", wait, "why", err)
		default:
			m.standing(status.Moving{Stage: status.MoveFailed, Why: redact.Text(err.Error())})
			logger.Warn("couldn't move the accounts and their history onto their ids, so it's run again", "error", err, "in", wait)
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// awaitingAPI is the move waiting on the API, as err, the move's failure,
// says: the account whose token's organization it waits on, and why, where
// err says.
func awaitingAPI(err error) status.Moving {
	waiting := status.Moving{Stage: status.MoveAwaitingAPI}
	if u, ok := errors.AsType[*idmove.Unanswered](err); ok {
		waiting.Account, waiting.Why = u.Account, redact.Text(u.Why)
	}
	return waiting
}

// historyFiles are the history's files as the router keeps them, as the move
// rewrites them beside it.
func (r *Router) historyFiles() idmove.History {
	return idmove.History{Ledger: r.ledger.opened.Load(), Readings: r.history.files, Events: r.eventFiles.Files()}
}
