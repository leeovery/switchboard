package cli

import (
	"context"
	"fmt"
	"sync"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/filestate"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// caps are the caps of the accounts cfg configures, as the request ledger
// summarises its days with them.
func caps(cfg *config.Config) ledger.Caps {
	return ledger.CapsOf(cfg.Accounts, claude.SharedWindows())
}

// stateReaders read the request ledger, the readings history and the
// router's events where they lie, with no router, and removed are the
// accounts removed they may name, as removedIn gives them: the dashboard
// reads through them, and a verb that reads those files gets its readers
// here.
type stateReaders struct {
	ledger   views.Ledger
	readings views.Readings
	events   views.Events
	removed  status.RemovedAccounts
	// dir is the state directory they read in.
	dir string
}

// notedFirst returns the id of the first pool the accounts record in the
// state directory dir notes, as idmove.FirstPoolID reads it, read once a
// run, a record that can't be read warned of once, as a command reads one
// state directory: a pool's read of the files and the ledger's go by it.
func (a *app) notedFirst(dir string) string {
	a.notedOnce.Do(func() { a.noted = sync.OnceValue(func() string { return idmove.FirstPoolID(dir) }) })
	return a.noted()
}

// firstPool returns the id of the first pool, as idmove.FirstOf has it of
// pools and the one the accounts record in the state directory dir notes,
// as notedFirst reads it: as the router has it, so work in no pool is the
// same pool's in both.
func (a *app) firstPool(dir string, pools config.Pools) string {
	return idmove.FirstOf(pools, a.notedFirst(dir))
}

// ledgerFirst is the ledger's option that has it know the first pool, as
// firstPool has it of the pools pools gives, asked once, and only where a
// day has work in no pool.
func (a *app) ledgerFirst(dir string, pools func() config.Pools) ledger.Option {
	return ledger.FirstPool(sync.OnceValue(func() string { return a.firstPool(dir, pools()) }))
}

// newStateReaders returns a verb's readers, as stateReadersKnowing has them,
// the ledger knowing the first pool as ledgerFirst has it, read once.
func (a *app) newStateReaders(ctx context.Context, cfg *config.Config) (stateReaders, error) {
	return a.stateReadersKnowing(ctx, cfg, a.ledgerFirst)
}

// stateReadersKnowing returns readers of the request ledger, the readings
// history and the router's events in the state directory, by the commands'
// clock, the ledger's days summarised with the caps of the accounts cfg
// configures, and its sessions resting on the states its digests hold, of
// the version the views build them in, knowing the first pool as first has
// it of cfg's pools as they stand, as standing has them, asked once. It fails
// where there's no state directory to find, and while the history waits on
// its move onto the accounts' ids.
func (a *app) stateReadersKnowing(ctx context.Context, cfg *config.Config, first func(dir string, pools func() config.Pools) ledger.Option) (stateReaders, error) {
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return stateReaders{}, err
	}
	if err := historyMoved(dir); err != nil {
		return stateReaders{}, err
	}
	pools := sync.OnceValue(func() config.Pools { return a.standing(ctx, cfg).Pools })
	return stateReaders{
		ledger:   a.followerIn(dir, caps(cfg), first(dir, pools)),
		readings: readings.NewReader(dir, logger),
		events:   events.NewReader(dir, a.Now, logger),
		removed:  removedIn(dir, cfg),
		dir:      dir,
	}, nil
}

// followerIn follows the request ledger in the state directory dir, by the
// commands' clock, its days summarised with caps, knowing the first pool as
// first has it, and its sessions resting on the states its digests hold, of
// the version the views build them in.
func (a *app) followerIn(dir string, caps ledger.Caps, first ledger.Option) *ledger.Follower {
	return ledger.NewFollower(dir, a.Now, caps, views.DigestVersion, logger, first)
}

// removedIn are the accounts removed that the accounts record in the state
// directory dir keeps, as status.RemovedOf gives them of the accounts cfg
// configures: none, warning, where the record can't be read, so the history
// reads all the same, each account removed named by its id.
func removedIn(dir string, cfg *config.Config) status.RemovedAccounts {
	record, err := identity.NewFile(dir).Read()
	if err != nil {
		logger.Warn("can't read the accounts record, so the accounts removed are named by their ids", "error", err)
		return nil
	}
	return status.RemovedOf(record, cfg.Accounts)
}

// recordIn reads the accounts record in the state directory dir once, as
// it's first asked for: an empty one, warning, where it can't be read, so
// the accounts and pools removed are known by their ids alone.
func recordIn(dir string) func() identity.Record {
	return sync.OnceValue(func() identity.Record {
		record, err := identity.NewFile(dir).Read()
		if err != nil {
			logger.Warn("can't read the accounts record, so the accounts and pools removed are known by their ids alone", "error", err)
		}
		return record
	})
}

// poolLabelsIn labels a pool by its id, as status.PoolLabels does, of the
// pools cfg configures and those the accounts record in the state directory
// dir keeps, as recordIn reads it.
func poolLabelsIn(dir string, cfg *config.Config) func(id string) string {
	return status.PoolLabels(cfg.Pools, recordIn(dir))
}

// readers has wc read the request ledger and the readings history where
// they lie, through stateReadersKnowing's readers, the ledger reading the
// first pool afresh at each ask, as idmove.FirstPool does, as a dashboard
// may stay open across the move to pools; and returns those readers, for
// the router's events to be read of the pool the dashboard shows, as
// shownPools reads them. Without a state directory to find, they read
// nothing, and it reports false.
func (a *app) readers(ctx context.Context, wc *watch.Config, cfg *config.Config) (stateReaders, bool) {
	r, err := a.stateReadersKnowing(ctx, cfg, idmove.FirstPool)
	if err != nil {
		logger.Debug("can't find the ledger, the readings history or the router's events", "error", err)
		wc.Ledger, wc.Readings = ledger.NewEmpty(a.Now, caps(cfg)), readings.Empty{}
		return stateReaders{}, false
	}
	wc.Ledger, wc.Readings, wc.Removed = r.ledger, r.readings, r.removed
	return r, true
}

// dashboardPools is how the dashboard reads the pool it shows, as
// shownPools has it, probing alone where probe says, the config file
// standing as stamp says: wc reading the request ledger and the readings
// history as readers has it, and the router's events read through it, of
// the pool shown alone.
func (a *app) dashboardPools(ctx context.Context, wc *watch.Config, cfg *config.Config, probe bool, stamp filestate.State) shownPools {
	r, read := a.readers(ctx, wc, cfg)
	return shownPools{a: a, probe: probe, dir: r.dir, events: r.events, read: read, stamp: stamp}
}

// poolsNamed labels a pool by its id, as status.PoolLabels does, where the
// recorded files may hold more than one pool's work, as status.ManyPoolsFiled
// has it of pools, as they stand, and the accounts record record returns,
// read once for both: so the words of the router's events name the pool
// whose sessions' own pins a forced pin cleared, one from the days of
// several pools included. It's nil, naming none, where they hold one pool's
// alone.
func poolsNamed(pools config.Pools, record func() identity.Record) func(id string) string {
	if !status.ManyPoolsFiled(pools, record) {
		return nil
	}
	return status.PoolLabels(pools, record)
}

// namesIn are the names of the accounts cfg configures and of those removed
// that the accounts record in the state directory dir keeps, as removedIn
// gives them.
func namesIn(dir string, cfg *config.Config) status.Names {
	return status.NamesOf(cfg.Accounts, removedIn(dir, cfg))
}

// accountNames are the names of the accounts the config configures and of
// those removed that the accounts record keeps, for a command that names
// accounts without reading the history: each named by its id, warning, where
// the config can't be loaded, so what names them prints all the same.
func (a *app) accountNames() status.Names {
	cfg, err := a.loadConfig()
	if err != nil {
		logger.Warn("can't load the config, so each account is named by its id", "error", err)
		return status.Names{}
	}
	return a.namesOf(cfg)
}

// namesOf are the names of the accounts cfg configures and of those removed
// that the accounts record keeps, as removedHere gives them.
func (a *app) namesOf(cfg *config.Config) status.Names {
	return status.NamesOf(cfg.Accounts, a.removedHere(cfg))
}

// idGiven fails where the account given to a command that takes an id is
// some account's label, as notALabel says, the accounts named as
// accountNames names them.
func (a *app) idGiven(given string) error {
	return notALabel(a.accountNames(), given)
}

// notALabel fails where the account given, as a command takes one by its id,
// is no account's id but some account's label, ignoring case, as names know
// them, naming the id of each account labelled so, as a label removed may
// have been given again: "work is a label: give its id, b6s7". Any other is
// taken as an id, as one names can't know, where the accounts record can't
// be read.
func notALabel(names status.Names, given string) error {
	ids := names.Labelled(given)
	if len(ids) == 0 || names.Knows(given) {
		return nil
	}
	for i, id := range ids {
		ids[i] = prose.Clean(id)
	}
	return fmt.Errorf("%s is a label: give its id, %s", prose.Clean(redact.Text(given)), prose.ListOr(ids))
}

// removedHere are the accounts removed that the accounts record in the state
// directory keeps, as removedIn gives them of the accounts cfg configures:
// none where there's no state directory to find.
func (a *app) removedHere(cfg *config.Config) status.RemovedAccounts {
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		logger.Debug("can't find the state directory, so the accounts removed are named by their ids", "error", err)
		return nil
	}
	return removedIn(dir, cfg)
}
