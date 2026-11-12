package router

import (
	"slices"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/idmove"
)

// poolHealer gives the config file its pools back, as idmove's HealPools
// does, where a config upkeep finds it making names none once the move to
// pools is done, as one put back by hand or from a repository it's kept in:
// so the router, which keeps the pools it had meanwhile, as membership's kept
// says, takes them up again at the look after, rather than at its restart,
// which heals the config as it starts. It tries once for each such config, by
// the accounts it configures, never at every look, so one it can't heal is
// told of once.
type poolHealer struct {
	path, stateDir string
	homeDir        func() (string, error)
	// tried are the accounts of the config naming no pools it last tried to
	// heal: nil before it has, and since a config with pools.
	tried config.Accounts
}

// newPoolHealer returns the healer of the config file cfg gives, its home
// directory as cfg's HomeDir returns it: none where the router has no config
// file, or no state directory, and so no accounts record to heal it from.
func newPoolHealer(cfg Config) *poolHealer {
	if cfg.ConfigFile.path == "" || cfg.StateDir == "" {
		return nil
	}
	return &poolHealer{path: cfg.ConfigFile.path, stateDir: cfg.StateDir, homeDir: cfg.HomeDir}
}

// look heals the config file, as poolHealer says, where the accounts and
// pools given, the last valid config upkeep found it making, name no pool.
// HealPools takes the accounts record's lock, so it's never called holding
// it.
func (h *poolHealer) look(accounts config.Accounts, pools config.Pools) {
	switch {
	case h == nil:
		return
	case len(pools) > 0:
		h.tried = nil
		return
	case h.tried != nil && slices.Equal(accounts, h.tried), !idmove.PoolsMoved(h.stateDir):
		return
	}
	// Never nil, as of none configured: nil would try again at every look.
	h.tried = append(config.Accounts{}, accounts...)
	healed, err := idmove.HealPools(h.path, h.stateDir, h.homeDir)
	switch {
	case err != nil:
		logger.Warn("couldn't write the pools back in the config, which names none; the pools stand as they were", "path", h.path, "error", err)
	case healed:
		logger.Info("wrote the pools back in the config, which named none, each account in the pool it was in", "path", h.path)
	}
}
