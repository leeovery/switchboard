package cli

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// What a verb reads of the recorded files is read here alone, through the
// pool's read, filedPool's, and the readers a verb reads them by are made
// here, or in readers.go for the verbs' and the dashboard's together: the
// request ledger's lines, days and sessions, the readings history's
// windows, and the router's events, listed and followed. So no verb reads
// the whole ledger, or every day of it, once its pool is known, and a verb
// given a session or an account, and no pool, reads the pool it's in, never
// every pool's work together. poolfiles_guard_test.go holds it.

// ledgerReader returns a reader of the request ledger in the state directory
// dir, by the clock now, which summarises days with the caps of the accounts
// cfg configures, knowing the first pool as ledgerFirst has it of cfg's
// pools, cfg as they stand.
func (a *app) ledgerReader(dir string, cfg *config.Config, now func() time.Time) *ledger.Reader {
	return ledger.NewReader(dir, now, caps(cfg), logger, a.ledgerFirst(dir, func() config.Pools { return cfg.Pools }))
}

// requestLines are the lines of the requests routed in the pool p that
// arrived from from on, of the request ledger in the state directory dir,
// as ledgerReader reads it of cfg by the commands' clock: every pool's of
// the zero pool, as of a config with no pools.
func (a *app) requestLines(p filedPool, dir string, cfg *config.Config, from time.Time) iter.Seq[ledger.Held] {
	return p.read.Lines(a.ledgerReader(dir, cfg, a.Now).Lines(from))
}

// requestDays reads the days of the request ledger in the state directory
// dir in the pool p, as its read's Days has them, by ledgerReader's reader
// of cfg at now, and tells where its work begins, as its read's Start tells
// it and ledger.Begins takes it: history counts its days from there, as the
// dashboard does. The start is zero where the ledger fails to say, its days
// counted then from the first it gives.
func (a *app) requestDays(p filedPool, dir string, cfg *config.Config, now time.Time) (views.LedgerDays, time.Time) {
	r := a.ledgerReader(dir, cfg, func() time.Time { return now })
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

// windows are the windows of the accounts given in the pool now, as its
// read's Windows has them, read from the readings history in the state
// directory dir over the days from from to now, as views.NewWindows reads
// them.
func (p filedPool) windows(dir string, accounts config.Accounts, from, now time.Time) views.Windows {
	return p.read.Windows(views.NewWindows(readings.NewReader(dir, logger), accounts, from, now))
}

// eventsHeld are the router's events of the pool p in the state directory
// dir, as its read's Events has them, held from from on, as events' Reader
// holds them by the commands' clock to the day to, and today's read through
// today, as HeldTailing reads them.
func (a *app) eventsHeld(p filedPool, dir string, from, to time.Time, today events.Tailed) iter.Seq[events.Held] {
	return p.read.Events(events.NewReader(dir, a.Now, logger).HeldTailing(from, to, today))
}

// followed hands print each event of the pool, as its read's ShowsHeld has
// it, that f finds filed after those it began from, every interval given,
// as f's run does, until ctx ends or print fails.
func (p filedPool) followed(ctx context.Context, f *eventsFollower, every time.Duration, print func(events.Held) error) error {
	return f.run(ctx, every, func(h events.Held) error {
		if !p.read.ShowsHeld(h) {
			return nil
		}
		return print(h)
	})
}

// sessionIn returns the session with the given id, as read has it of every
// pool's work, through the readers r, read in the pool it's in now, as
// views' SessionsRead.PoolNow has it, with that pool, of cfg as its pools
// stand, as pooledConfig has them: where its work is split between pools,
// read again in that pool, as poolNow has it; else read as it is, in the
// one pool it's of, as onePool names it. It fails where the pools can't be
// told, and where a split session's pool can't be read.
func (a *app) sessionIn(ctx context.Context, cfg *config.Config, r stateReaders, read views.SessionsRead, id string, now time.Time) (views.SessionsRead, filedPool, error) {
	pooled, err := a.pooledConfig(ctx, cfg)
	if err != nil {
		return views.SessionsRead{}, filedPool{}, err
	}
	in, split := read.PoolNow(r.ledger, id, a.firstPool(r.dir, pooled.Pools))
	if !split {
		return read, a.onePool(pooled, r.dir, in), nil
	}
	pool, err := a.poolNow(pooled, r.dir, in)
	if err != nil {
		return views.SessionsRead{}, filedPool{}, err
	}
	return views.ReadSession(pool.sessions(r), now, id), pool, nil
}

// linesIn returns held, those of every pool's work that tell the pool a
// session or an account given is in now, oldest first, those routed in that
// pool, as views.PoolNow has it, with that pool, as poolNow has it of cfg,
// its pools as they stand, of the files in the state directory dir: held as
// they are, with none, where they're none, and in a config with no pools.
// It fails where that pool can't be read.
func (a *app) linesIn(cfg *config.Config, dir string, held []ledger.Held) ([]ledger.Held, filedPool, error) {
	if len(held) == 0 {
		return held, filedPool{}, nil
	}
	pool, err := a.poolNow(cfg, dir, views.PoolNow(held, a.firstPool(dir, cfg.Pools)))
	if err != nil {
		return nil, filedPool{}, err
	}
	return pool.read.Held(held), pool, nil
}

// accountIn returns the pool the account with the given id is in now, as
// status.AccountPools places it, of cfg's accounts, its pools as they stand,
// or of the accounts record in the state directory dir, of one removed, as
// readPool has it, failing as it does; and whether either places it: the
// zero pool where neither does, as of an account an import brought, or one
// removed before pools, for the pool its lines tell, as linesIn has it.
func (a *app) accountIn(cfg *config.Config, dir, account string) (filedPool, bool, error) {
	in, placed := status.AccountPools(cfg.Accounts, recordIn(dir))(account)
	if !placed {
		return filedPool{}, false, nil
	}
	pool, err := a.readPool(cfg, dir, in)
	return pool, true, err
}

// poolNow returns the pool with the id in, as readPool has it of cfg, of
// the files in the state directory dir, that the newest of the lines of a
// session or an account given, and no pool, recorded: failing, where
// neither the config nor the accounts record keeps it, saying so, as no
// --pool was given to name another.
func (a *app) poolNow(cfg *config.Config, dir, in string) (filedPool, error) {
	pool, err := a.readPool(cfg, dir, in)
	if _, unknown := errors.AsType[router.PoolNeeded](err); unknown {
		return filedPool{}, fmt.Errorf("can't tell which pool to read: the newest request names pool %q, which neither the config nor the accounts record keeps", redact.Text(in))
	}
	return pool, err
}

// onePool returns the pool with the id in, as readPool has it of cfg, of
// the files in the state directory dir, that a session of one pool's work
// is in, for its page to name: none where in is none, or where it can't be
// read, as the session's work reads alike in every pool's read, so its
// page is shown all the same, naming none.
func (a *app) onePool(cfg *config.Config, dir, in string) filedPool {
	if in == "" {
		return filedPool{}
	}
	pool, err := a.readPool(cfg, dir, in)
	if err != nil {
		logger.Debug("can't read the pool a session's work is in, so its page names none", "error", err)
		return filedPool{}
	}
	return pool
}
