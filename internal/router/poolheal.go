package router

import (
	"fmt"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/idmove"
)

// healRetry is how long the router waits to try again a heal that didn't
// give a config its pools back, while the config stays as it was.
const healRetry = time.Minute

// poolHealer gives the config file its pools back, as idmove's HealPools
// does, and the accounts' ids, as idmove's Heal does, first, so the file it
// leaves is one the router reads as written, while the config the router
// follows names none once the move to pools is done, as one put back by hand
// or from a repository it's kept in. It's a convenience for the owner's
// file, never what keeps the pools apart: the router routes within the pools
// the accounts record keeps meanwhile, as membership's kept says, whether or
// not the heal can be written. It tries once a minute at most while a heal
// of the config as it stands doesn't write it, as a failure may pass, and
// tells of one that fails once for each config, the heal as the router
// started, which serve tries, among them.
type poolHealer struct {
	path, stateDir string
	homeDir        func() (string, error)
	now            func() time.Time
	// tried are the accounts of the config the heal was last tried on
	// without writing it, and triedAt when: nil since a heal written, or a
	// config with pools.
	tried   config.Accounts
	triedAt time.Time
	// told are the accounts of the config whose heal's failure was last told
	// of: nil since a heal written, or a config with pools.
	told config.Accounts
}

// newPoolHealer returns the healer of the config file cfg gives, its home
// directory as cfg's HomeDir returns it, which takes cfg's Unhealed, where
// it's set, as a heal of the config the router started on tried at once,
// and tells of it: none where the router has no config file, or no state
// directory, and so no accounts record to heal it from.
func newPoolHealer(cfg Config) *poolHealer {
	if cfg.ConfigFile.path == "" || cfg.StateDir == "" {
		return nil
	}
	h := &poolHealer{path: cfg.ConfigFile.path, stateDir: cfg.StateDir, homeDir: cfg.HomeDir, now: cfg.Now}
	if cfg.Unhealed != nil {
		h.failed(cfg.Accounts, cfg.Unhealed)
	}
	return h
}

// look heals the config file, as poolHealer says, where the accounts and
// pools given, as the config the router follows makes them, name no pool
// once the move to pools is done. The heals take the accounts record's lock,
// so it's never called holding it.
func (h *poolHealer) look(accounts config.Accounts, pools config.Pools) {
	if h == nil {
		return
	}
	if len(pools) > 0 || !idmove.PoolsMoved(h.stateDir) {
		h.tried, h.told = nil, nil
		return
	}
	if h.tried != nil && slices.Equal(accounts, h.tried) && h.now().Sub(h.triedAt) < healRetry {
		return
	}
	healed, err := h.heal()
	switch {
	case err != nil:
		h.failed(accounts, err)
	case healed:
		h.tried, h.told = nil, nil
		logger.Info("wrote the pools back in the config, which named none, each account in the pool it was in", "path", h.path)
	default:
		h.tried, h.triedAt = slices.Clone(accounts), h.now()
	}
}

// heal writes the accounts' new ids back in the config file, as idmove's
// Heal does, then its pools, as HealPools does, reporting whether the pools
// were written. Where the ids can't be, it fails, writing no pools, so the
// file never holds pools keyed by the ids the accounts had before.
func (h *poolHealer) heal() (bool, error) {
	if _, err := idmove.Heal(h.path, h.stateDir, h.homeDir); err != nil {
		return false, fmt.Errorf("write the accounts' new ids back first: %w", err)
	}
	return idmove.HealPools(h.path, h.stateDir, h.homeDir)
}

// failed notes that a heal of the config of the accounts given failed, for
// the reason given, now, and tells of it, once for each config.
func (h *poolHealer) failed(accounts config.Accounts, err error) {
	// Never nil, as of none configured: nil would try it again at once.
	h.tried, h.triedAt = append(config.Accounts{}, accounts...), h.now()
	if h.told != nil && slices.Equal(accounts, h.told) {
		return
	}
	h.told = h.tried
	logger.Warn("couldn't write the pools back in the config, which names none; it's tried again each minute", "path", h.path, "error", err)
}
