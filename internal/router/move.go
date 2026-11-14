package router

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
)

// moveRetry is how long the router waits to run a move again where it
// failed, or waits for the router to take up the first pool.
const moveRetry = time.Minute

// errHistoryMoving is what an import fails with while the history waits on
// a move, wrapped in the words of the move, as historyMoving says.
var errHistoryMoving = errors.New("import once it's done")

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

// PoolMover moves the history to pools, once, after the move onto the ids,
// as idmove.PoolMover does: RunBeside runs it beside the router, given the
// history's files as it keeps them, and what has it take up the pools its
// config holds, reporting whether it routes in them, failing with
// idmove.ErrAwaitingPools, having touched no file of the history, while it
// doesn't.
type PoolMover interface {
	RunBeside(ctx context.Context, history idmove.History, takeUp func() bool) error
}

// moving runs the moves the history makes once beside the router, where
// it has them, the move onto the accounts' ids, then the move to pools, and
// says how they stand.
type moving struct {
	mover Mover
	pools PoolMover
	// retry is how long a move waits to run again once it fails otherwise
	// than for the API not answering, and to look again whether the router
	// has taken up the first pool.
	retry time.Duration
	// stands is how the move owed stands, as the status says it: from the
	// router's start, where it has a move to make, until every move is done.
	// Nil where none is owed.
	stands atomic.Pointer[status.Moving]
}

// newMoving returns the moving of mover's move onto the ids, then pools'
// move to pools, each none where it's nil.
func newMoving(mover Mover, pools PoolMover) *moving {
	m := &moving{mover: mover, pools: pools, retry: moveRetry}
	switch {
	case mover != nil:
		m.standing(status.Moving{Stage: status.MoveRunning, Move: status.MovingIDs})
	case pools != nil:
		m.standing(status.Moving{Stage: status.MoveRunning, Move: status.MovingPools})
	}
	return m
}

// owed reports whether a move is owed: not done yet, so the history waits on
// it.
func (m *moving) owed() bool {
	return m.stands.Load() != nil
}

// idsOwed reports whether the move onto the accounts' ids is owed, the
// config's ids and the record's differing till it's done.
func (m *moving) idsOwed() bool {
	s := m.stands.Load()
	return s != nil && s.Move == status.MovingIDs
}

// report is how the move owed stands, for the status document: zero where
// none is owed.
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

// historyMoving is the error of an import while the history waits on the
// move owed, in its words, matching errHistoryMoving.
func (m *moving) historyMoving() error {
	return fmt.Errorf("%s: %w", m.report().Waits(), errHistoryMoving)
}

// settle has the mover move the files the router takes up as it starts,
// where the move onto the ids is owed. Where it can't, the router starts all
// the same, the state those files kept of the accounts moved let go of.
func (m *moving) settle() {
	if !m.idsOwed() {
		return
	}
	if err := m.mover.Settle(); err != nil {
		logger.Warn("couldn't move the state file and the newest readings onto the accounts' ids, so what they kept of the accounts moved is let go of", "error", err)
	}
}

// run runs the moves owed, given the history's files as history gives them,
// the move onto the ids first, then the move to pools, which has the router
// take up the pools of its config as takeUp does, each as until runs it,
// until every one's done, ctx ends, or the move onto the ids waits for the
// router to restart onto the accounts' new ids. It notes how they stand
// throughout, as the status says it.
func (m *moving) run(ctx context.Context, history func() idmove.History, takeUp func() bool) {
	if m.idsOwed() && !m.until(ctx, status.MovingIDs, func() error { return m.mover.RunBeside(ctx, history()) }) {
		return
	}
	if m.pools != nil && !m.until(ctx, status.MovingPools, func() error { return m.pools.RunBeside(ctx, history(), takeUp) }) {
		return
	}
	m.stands.Store(nil)
}

// until runs attempt, the move named move, until it's done, reporting
// whether it is: where it waits for the router to restart onto the
// accounts' new ids, it stops there; where it waits for the router to take
// up the first pool, it runs again after the retry's wait; where the API
// doesn't answer, after the wait the mover's AskAgain gives; where it fails
// otherwise, after the retry's wait. It stops once ctx ends.
func (m *moving) until(ctx context.Context, move string, attempt func() error) bool {
	var asked time.Duration
	told := false
	for {
		m.standing(status.Moving{Stage: status.MoveRunning, Move: move})
		err := attempt()
		wait := m.retry
		switch {
		case err == nil:
			return true
		case ctx.Err() != nil:
			return false
		case errors.Is(err, idmove.ErrAwaitingRestart):
			m.standing(status.Moving{Stage: status.MoveAwaitingRestart, Move: move})
			logger.Info("the move onto the accounts' ids waits for the router to restart onto them")
			return false
		case errors.Is(err, idmove.ErrAwaitingPools):
			m.standing(status.Moving{Stage: status.MoveAwaitingRestart, Move: move})
			if !told {
				logger.Info("the move to pools waits for the router to take up the first pool, which its config now holds")
				told = true
			}
		case errors.Is(err, idmove.ErrUnanswered):
			asked = m.mover.AskAgain(asked)
			wait = asked
			m.standing(awaitingAPI(err))
			logger.Info("the move onto the accounts' ids waits to ask the API again for the organizations of their tokens", "in", wait, "why", err)
		default:
			m.standing(status.Moving{Stage: status.MoveFailed, Move: move, Why: redact.Text(err.Error())})
			logger.Warn("couldn't make a move of the history, so it's run again", "move", move, "error", err, "in", wait)
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}

// awaitingAPI is the move onto the ids waiting on the API, as err, the
// move's failure, says: the account whose token's organization it waits on,
// and why, where err says.
func awaitingAPI(err error) status.Moving {
	waiting := status.Moving{Stage: status.MoveAwaitingAPI, Move: status.MovingIDs}
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

// takeUpPools has the router take up the pools its config file holds, as the
// move to pools has written the first there, at once, as upkeep's look takes
// them up, reporting whether it routes in them: not where it has no config
// file, or the file makes no valid config, or one naming no pools, which
// upkeep heals. Routing in them, it gives the events it keeps from before
// them the first pool, as the move gives those filed.
func (r *Router) takeUpPools() bool {
	if r.cfg.ConfigFile.path == "" {
		return false
	}
	cfg, err := config.Load(r.cfg.ConfigFile.path, r.cfg.HomeDir)
	if err != nil {
		logger.Debug("can't read the config to take up its pools", "error", err)
		return false
	}
	r.state.pools.follow(cfg.Accounts, cfg.Pools)
	if pools := r.state.pools.current(); pools.pooled() {
		r.recent.pooled(pools.of(""))
	}
	return len(cfg.Pools) > 0
}
