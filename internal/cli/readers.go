package cli

import (
	"context"
	"fmt"
	"sync"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/events"
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

// newStateReaders returns readers of the request ledger, the readings history
// and the router's events in the state directory, by the commands' clock, the
// ledger's days summarised with the caps of the accounts cfg configures, and
// its sessions resting on the states its digests hold, of the version the
// views build them in. It fails where there's no state directory to find, and
// while the history waits on its move onto the accounts' ids.
func (a *app) newStateReaders(cfg *config.Config) (stateReaders, error) {
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return stateReaders{}, err
	}
	if err := historyMoved(dir); err != nil {
		return stateReaders{}, err
	}
	return stateReaders{
		ledger:   ledger.NewFollower(dir, a.Now, caps(cfg), views.DigestVersion, logger, idmove.FirstPool(dir)),
		readings: readings.NewReader(dir, logger),
		events:   events.NewReader(dir, a.Now, logger),
		removed:  removedIn(dir, cfg),
		dir:      dir,
	}, nil
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

// readers has wc read the request ledger, the readings history and the
// router's events where they lie, through newStateReaders' readers, the
// events those of the pool the dashboard shows, as shownEvents has them.
// Without a state directory to find, they read nothing.
func (a *app) readers(ctx context.Context, wc *watch.Config, cfg *config.Config) {
	r, err := a.newStateReaders(cfg)
	if err != nil {
		logger.Debug("can't find the ledger, the readings history or the router's events", "error", err)
		wc.Ledger, wc.Readings, wc.Events = ledger.NewEmpty(a.Now, caps(cfg)), readings.Empty{}, events.Empty{}
		return
	}
	wc.Ledger, wc.Readings, wc.Events, wc.Removed = r.ledger, r.readings, a.shownEvents(ctx, cfg, r), r.removed
}

// shownEvents reads the router's events through r's reader as the dashboard
// shows them: those of the pool it shows, the one the verbs read with none
// named, as filedPool has it, as the router gives its document of that pool.
// Where that can't be told, it reads none, warning, so the dashboard never
// shows every pool's together.
func (a *app) shownEvents(ctx context.Context, cfg *config.Config, r stateReaders) views.Events {
	pool, _, err := a.filedPool(ctx, cfg, r.dir, "")
	if err != nil {
		logger.Warn("can't tell which pool the dashboard shows, so it reads none of the router's files of events", "error", err)
		return events.Empty{}
	}
	return pool.read.Filed(r.events)
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
