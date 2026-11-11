package router

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/idmove"
)

// moveRetry is how long the router waits to run the move onto the accounts'
// ids again where it failed.
const moveRetry = time.Minute

// Mover moves the accounts and their history onto the ids switchboard makes,
// once, as idmove.Mover does: Settle moves the files the router takes up as
// it starts, before it does, and RunBeside the rest, beside the router, given
// the history's files as it keeps them.
type Mover interface {
	Settle() error
	RunBeside(ctx context.Context, history idmove.History) error
}

// moving runs the move onto the accounts' ids beside the router, where it has
// one, and says whether it's owed.
type moving struct {
	mover Mover
	retry time.Duration
	// owed is set while the move is owed or running: from the router's start,
	// where it has a mover, until the move is done.
	owed atomic.Bool
}

// newMoving returns the moving of mover's move, none where it's nil.
func newMoving(mover Mover) *moving {
	m := &moving{mover: mover, retry: moveRetry}
	m.owed.Store(mover != nil)
	return m
}

// settle has the mover move the files the router takes up as it starts,
// where a move's owed. Where it can't, the router starts all the same, the
// state those files kept of the accounts moved let go of.
func (m *moving) settle() {
	if !m.owed.Load() {
		return
	}
	if err := m.mover.Settle(); err != nil {
		logger.Warn("couldn't move the state file and the newest readings onto the accounts' ids, so what they kept of the accounts moved is let go of", "error", err)
	}
}

// run runs the move, given the history's files as history gives them, until
// it's done, ctx ends, or it waits for the router to restart onto the
// accounts' new ids, which its config holds by then: where it fails
// otherwise, it runs again after the retry's wait.
func (m *moving) run(ctx context.Context, history func() idmove.History) {
	if !m.owed.Load() {
		return
	}
	for {
		err := m.mover.RunBeside(ctx, history())
		switch {
		case err == nil:
			m.owed.Store(false)
			return
		case errors.Is(err, idmove.ErrAwaitingRestart):
			logger.Info("the move onto the accounts' ids waits for the router to restart onto them")
			return
		case ctx.Err() != nil:
			return
		}
		logger.Warn("couldn't move the accounts and their history onto their ids, so it's run again", "error", err, "in", m.retry)
		timer := time.NewTimer(m.retry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// historyFiles are the history's files as the router keeps them, as the move
// rewrites them beside it.
func (r *Router) historyFiles() idmove.History {
	return idmove.History{Ledger: r.ledger.opened.Load(), Readings: r.history.files, Events: r.eventFiles.Files()}
}
