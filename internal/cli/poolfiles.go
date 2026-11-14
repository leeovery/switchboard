package cli

import (
	"context"
	"iter"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// What a verb reads of the recorded files in its pool is built here alone,
// from the pool's read, filedPool's: the request ledger's lines, days and
// sessions, the readings history's windows and the router's events. So no
// verb reads the whole ledger, or every day of it, once its pool is known,
// and a verb given a session or an account, and no pool, reads the pool it's
// in, never every pool's work together. poolfiles_guard_test.go holds it.

// lines are the lines of the requests routed in the pool that arrived from
// from on, as the ledger's reader r reads them: every pool's of the zero
// pool, as of a config with no pools.
func (p filedPool) lines(r *ledger.Reader, from time.Time) iter.Seq[ledger.Held] {
	return p.read.Lines(r.Lines(from))
}

// days reads the request ledger's days in the pool, by the ledger's reader
// r, as its read's Days has them, and tells where its work begins, at now,
// as its read's Start tells it and ledger.Begins takes it: history counts
// its days from there, as the dashboard does. The start is zero where the
// ledger fails to say, its days counted then from the first it gives.
func (p filedPool) days(r *ledger.Reader, now time.Time) (views.LedgerDays, time.Time) {
	first, err := p.read.Start(r).FirstDay(now)
	began, _, err := ledger.Begins(first, err, now)
	if err != nil {
		logger.Debug("can't tell where the pool's work begins, so its days are counted from the first read", "error", err)
	}
	return p.read.Days(r), began
}

// sessions reads the request ledger's sessions in the pool, through the
// readers r, as its read's Ledger has them.
func (p filedPool) sessions(r stateReaders) views.Ledger {
	return p.read.Ledger(r.ledger)
}

// windows are those of w of the accounts in the pool now, as its read's
// Windows has them.
func (p filedPool) windows(w views.Windows) views.Windows {
	return p.read.Windows(w)
}

// events are those of held of the pool, as its read's Events has them.
func (p filedPool) events(held iter.Seq[events.Held]) iter.Seq[events.Held] {
	return p.read.Events(held)
}

// shows reports whether the event a line of the router's events holds is of
// the pool, as its read's ShowsHeld has it.
func (p filedPool) shows(h events.Held) bool {
	return p.read.ShowsHeld(h)
}

// held are those of a session's lines routed in the pool, as its read's Held
// has them.
func (p filedPool) held(lines []ledger.Held) []ledger.Held {
	return p.read.Held(lines)
}

// sessionIn returns the session with the given id, as read has it of every
// pool's work, through the readers r, read in the pool it's in now, as
// views' SessionsRead.PoolNow has it, where its work is split between
// pools, with that pool, as readPool has it of cfg as its pools stand; else
// read as it is, a session of one pool, with none. It fails where the pool
// it's in can't be read.
func (a *app) sessionIn(ctx context.Context, cfg *config.Config, r stateReaders, read views.SessionsRead, id string, now time.Time) (views.SessionsRead, filedPool, error) {
	pooled := a.standing(ctx, cfg)
	in, split := read.PoolNow(r.ledger, id, a.firstPool(r.dir, pooled.Pools))
	if !split {
		return read, filedPool{}, nil
	}
	pool, err := a.readPool(pooled, r.dir, in)
	if err != nil {
		return views.SessionsRead{}, filedPool{}, err
	}
	return views.ReadSession(pool.sessions(r), now, id), pool, nil
}

// linesIn returns held, a session's lines of every pool's work, oldest
// first, those routed in the pool it's in now, as views.PoolNow has it, with
// that pool, as readPool has it of cfg, its pools as they stand, of the
// files in the state directory dir: held as they are, with none, where
// they're none, and in a config with no pools. It fails where that pool
// can't be read.
func (a *app) linesIn(cfg *config.Config, dir string, held []ledger.Held) ([]ledger.Held, filedPool, error) {
	if len(held) == 0 {
		return held, filedPool{}, nil
	}
	pool, err := a.readPool(cfg, dir, views.PoolNow(held, a.firstPool(dir, cfg.Pools)))
	if err != nil {
		return nil, filedPool{}, err
	}
	return pool.held(held), pool, nil
}

// accountIn returns the pool the account with the given id is in now, as
// status.AccountPools places it, of cfg's accounts, its pools as they stand,
// or of the accounts record in the state directory dir, of one removed: else
// the one the verbs act on without --pool, as readPool has it, as of an
// account neither places. It fails as readPool does.
func (a *app) accountIn(cfg *config.Config, dir, account string) (filedPool, error) {
	in, _ := status.AccountPools(cfg.Accounts, recordIn(dir))(account)
	return a.readPool(cfg, dir, in)
}
