package cli

import (
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
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
		ledger:   ledger.NewFollower(dir, a.Now, caps(cfg), views.DigestVersion, logger),
		readings: readings.NewReader(dir, logger),
		events:   events.NewReader(dir, a.Now, logger),
		removed:  removedIn(dir, cfg),
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

// readers has wc read the request ledger, the readings history and the
// router's events where they lie, through newStateReaders' readers. Without a
// state directory to find, they read nothing.
func (a *app) readers(wc *watch.Config, cfg *config.Config) {
	r, err := a.newStateReaders(cfg)
	if err != nil {
		logger.Debug("can't find the ledger, the readings history or the router's events", "error", err)
		r = stateReaders{ledger: ledger.NewEmpty(a.Now, caps(cfg)), readings: readings.Empty{}, events: events.Empty{}}
	}
	wc.Ledger, wc.Readings, wc.Events, wc.Removed = r.ledger, r.readings, r.events, r.removed
}

// namesIn are the names of the accounts cfg configures and of those removed
// that the accounts record in the state directory dir keeps, as removedIn
// gives them.
func namesIn(dir string, cfg *config.Config) status.Names {
	return status.NamesOf(cfg.Accounts, removedIn(dir, cfg))
}
