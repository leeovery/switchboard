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
// router's events where they lie, with no router: what the dashboard and the
// data verbs read them through.
type stateReaders struct {
	ledger   views.Ledger
	readings views.Readings
	events   views.Events
}

// stateReaders returns readers of the request ledger, the readings history
// and the router's events in the state directory, by the commands' clock, the
// ledger's days summarised with the caps of the accounts cfg configures.
// Without a state directory to find, they read nothing.
func (a *app) stateReaders(cfg *config.Config) stateReaders {
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		logger.Debug("can't find the ledger, the readings history or the router's events", "error", err)
		return stateReaders{ledger: ledger.NewEmpty(a.Now, caps(cfg)), readings: readings.Empty{}, events: events.Empty{}}
	}
	return stateReaders{
		ledger:   ledger.NewFollower(dir, a.Now, caps(cfg), logger),
		readings: readings.NewReader(dir, logger),
		events:   events.NewReader(dir, a.Now, logger),
	}
}

// readers has wc read the request ledger, the readings history and the
// router's events through the readers stateReaders returns.
func (a *app) readers(wc *watch.Config, cfg *config.Config) {
	r := a.stateReaders(cfg)
	wc.Ledger, wc.Readings, wc.Events = r.ledger, r.readings, r.events
}
