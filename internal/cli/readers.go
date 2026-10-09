package cli

import (
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// caps are the caps of the accounts cfg configures, as the request ledger
// summarises its days with them.
func caps(cfg *config.Config) ledger.Caps {
	return ledger.CapsOf(cfg.Accounts, claude.SharedWindows)
}

// readers has wc read the request ledger, the readings history and the
// router's events where they lie, in the state directory, by the commands'
// clock, the ledger's days summarised with the caps of the accounts cfg
// configures. Without a state directory to find, they read nothing.
func (a *app) readers(wc *watch.Config, cfg *config.Config) {
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		logger.Debug("can't find the ledger, the readings history or the router's events", "error", err)
		wc.Ledger, wc.Readings, wc.Events = ledger.NewEmpty(a.Now, caps(cfg)), readings.Empty{}, events.Empty{}
		return
	}
	wc.Ledger = ledger.NewFollower(dir, a.Now, caps(cfg), logger)
	wc.Readings = readings.NewReader(dir, logger)
	wc.Events = events.NewReader(dir, a.Now, logger)
}
