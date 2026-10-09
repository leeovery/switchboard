package cli

import (
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/views"
)

// caps are the caps of the accounts cfg configures, as the request ledger
// summarises its days with them.
func caps(cfg *config.Config) ledger.Caps {
	return ledger.CapsOf(cfg.Accounts, claude.SharedWindows)
}

// stateReaders read the request ledger, the readings history and the
// router's events where they lie, with no router: the dashboard reads
// through them, and a verb that reads those files gets its readers here.
type stateReaders struct {
	ledger   views.Ledger
	readings views.Readings
	events   views.Events
}

// newStateReaders returns readers of the request ledger, the readings history
// and the router's events in the state directory, by the commands' clock, the
// ledger's days summarised with the caps of the accounts cfg configures. It
// fails where there's no state directory to find.
func (a *app) newStateReaders(cfg *config.Config) (stateReaders, error) {
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return stateReaders{}, err
	}
	return stateReaders{
		ledger:   ledger.NewFollower(dir, a.Now, caps(cfg), logger),
		readings: readings.NewReader(dir, logger),
		events:   events.NewReader(dir, a.Now, logger),
	}, nil
}

// readers has wc read the request ledger, the readings history and the
// router's events where they lie, through stateReadersOrNone's readers.
func (a *app) readers(wc *watch.Config, cfg *config.Config) {
	r := a.stateReadersOrNone(cfg)
	wc.Ledger, wc.Readings, wc.Events = r.ledger, r.readings, r.events
}

// stateReadersOrNone returns newStateReaders' readers, or, without a state
// directory to find, readers that read nothing.
func (a *app) stateReadersOrNone(cfg *config.Config) stateReaders {
	r, err := a.newStateReaders(cfg)
	if err != nil {
		logger.Debug("can't find the ledger, the readings history or the router's events", "error", err)
		r = stateReaders{ledger: ledger.NewEmpty(a.Now, caps(cfg)), readings: readings.Empty{}, events: events.Empty{}}
	}
	return r
}
