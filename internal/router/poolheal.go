package router

import (
	"slices"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/idmove"
)

// poolHealer gives the config file its pools back, as idmove's HealPools
// does, while the config the router follows names none once the move to
// pools is done, as one put back by hand or from a repository it's kept in:
// so the router, which keeps the pools the heal writes meanwhile, as
// membership's kept says, takes them up at the look after, rather than at
// its restart, which heals the config as it starts. It tries at every look
// till the heal is written, as a failure may pass, as a config changed as
// it's edited, and tells of one that fails once for each config.
type poolHealer struct {
	path, stateDir string
	homeDir        func() (string, error)
	// started and startedPools are the accounts and pools of the config the
	// router started on, which it follows till the config file makes
	// another.
	started      config.Accounts
	startedPools config.Pools
	// told are the accounts of the config whose heal last failed, told of:
	// nil since a config with pools, or a heal written.
	told config.Accounts
}

// newPoolHealer returns the healer of the config file cfg gives, its home
// directory as cfg's HomeDir returns it: none where the router has no config
// file, or no state directory, and so no accounts record to heal it from.
func newPoolHealer(cfg Config) *poolHealer {
	if cfg.ConfigFile.path == "" || cfg.StateDir == "" {
		return nil
	}
	return &poolHealer{path: cfg.ConfigFile.path, stateDir: cfg.StateDir, homeDir: cfg.HomeDir, started: cfg.Accounts, startedPools: cfg.Pools}
}

// look heals the config file, as poolHealer says, where the accounts and
// pools given, the last valid config upkeep found it making, name no pool,
// or, where changed says it has made none since the router started, those
// the router started on do. HealPools takes the accounts record's lock, so
// it's never called holding it.
func (h *poolHealer) look(accounts config.Accounts, pools config.Pools, changed bool) {
	if h == nil {
		return
	}
	if !changed {
		accounts, pools = h.started, h.startedPools
	}
	if len(pools) > 0 {
		h.told = nil
		return
	}
	healed, err := idmove.HealPools(h.path, h.stateDir, h.homeDir)
	switch {
	case err != nil && (h.told == nil || !slices.Equal(accounts, h.told)):
		// Never nil, as of none configured: nil would tell of it again.
		h.told = append(config.Accounts{}, accounts...)
		logger.Warn("couldn't write the pools back in the config, which names none; the pools stand as they were, and it's tried again at each look", "path", h.path, "error", err)
	case healed:
		h.told = nil
		logger.Info("wrote the pools back in the config, which named none, each account in the pool it was in", "path", h.path)
	}
}
