package router

import (
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/idmove"
)

// healRetry is how long the router waits to try a heal again that didn't
// give the config its pools back.
const healRetry = time.Minute

// poolHealer gives the config file its pools back, the accounts' ids first,
// as idmove's HealConfig does, so the file it leaves is one the router reads
// as written, while the config the router follows names none once the move
// to pools is done, as one put back by hand or from a repository it's kept
// in. It's a convenience for the owner's file, never what keeps the pools
// apart: the router routes within the pools the accounts record keeps
// meanwhile, as membership's kept says, whether or not the heal can be
// written. While a heal doesn't write it, it's tried once a minute at most,
// however the config changes meanwhile; and a failure is told of once, the
// heal as the router started, which serve tries, among them, till a heal is
// written or the config has its pools again.
type poolHealer struct {
	path, stateDir string
	homeDir        func() (string, error)
	now            func() time.Time
	// triedAt is when a heal that didn't write the pools was last tried:
	// zero since a heal written, or a config with pools.
	triedAt time.Time
	// failing is set once a heal's failure is told of, till a heal is
	// written, or a config has pools.
	failing bool
}

// newPoolHealer returns the healer of the config file cfg gives, its home
// directory as cfg's HomeDir returns it, which tells of cfg's Unhealed, where
// it's set: as a heal of the config the router started on tried at once,
// to be tried again, where the move to pools is done and that config names
// no pools, as look tries one; else as no more than a heal that failed, as
// no look tries one till it's needed. It's none where the router has no
// config file, or no state directory, and so no accounts record to heal it
// from.
func newPoolHealer(cfg Config) *poolHealer {
	if cfg.ConfigFile.path == "" || cfg.StateDir == "" {
		return nil
	}
	h := &poolHealer{path: cfg.ConfigFile.path, stateDir: cfg.StateDir, homeDir: cfg.HomeDir, now: cfg.Now}
	switch {
	case cfg.Unhealed == nil:
	case len(cfg.Pools) == 0 && idmove.PoolsMoved(cfg.StateDir):
		h.failed(cfg.Unhealed)
	default:
		logger.Warn("couldn't heal the config", "path", h.path, "error", cfg.Unhealed)
	}
	return h
}

// look heals the config file, as poolHealer says, where pools, those of the
// config the router follows, are none once the move to pools is done. The
// heal takes the accounts record's lock, so it's never called holding it.
func (h *poolHealer) look(pools config.Pools) {
	if h == nil {
		return
	}
	if len(pools) > 0 || !idmove.PoolsMoved(h.stateDir) {
		h.triedAt, h.failing = time.Time{}, false
		return
	}
	if !h.triedAt.IsZero() && h.now().Sub(h.triedAt) < healRetry {
		return
	}
	ids, healed, err := idmove.HealConfig(h.path, h.stateDir, h.homeDir)
	if ids {
		logger.Info("wrote the accounts' new ids back in the config, which named them by those they had before", "path", h.path)
	}
	switch {
	case err != nil:
		h.failed(err)
	case healed:
		h.triedAt, h.failing = time.Time{}, false
		logger.Info("wrote the pools back in the config, which named none, each account in the pool it was in", "path", h.path)
	default:
		h.triedAt = h.now()
	}
}

// failed notes that a heal failed, for the reason given, now, and tells of
// it, where it hasn't told of a failure since a heal was written, or the
// config had pools.
func (h *poolHealer) failed(err error) {
	h.triedAt = h.now()
	if h.failing {
		return
	}
	h.failing = true
	logger.Warn("couldn't heal the config; while it names no pools, it's tried again each minute", "path", h.path, "error", err)
}
