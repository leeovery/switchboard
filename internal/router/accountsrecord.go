package router

import (
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
)

// accountsRecord keeps the accounts record in step with the config the
// router follows, as identity's KeepUp keeps it, so an account configured or
// removed on another Mac sharing this one's config is one this Mac has had
// too: its token held to its organization, and, once removed, named by its
// label. It keeps it so once the move onto the accounts' ids is done, never
// while it's owed, as the config's ids and the record's differ till then: as
// the router starts, at each look after, where the accounts upkeep found
// configured have changed since, and at the first look once the move is done.
// Upkeep alone keeps it; an import reads the config it follows, as followed
// gives it.
type accountsRecord struct {
	record identity.File
	owed   func() bool
	now    func() time.Time
	// path is the config file's, "" where the router has none.
	path string
	// started are the accounts the router started with: those followed while
	// upkeep has found the config file making no other, and the labels of
	// those that leave it before the record was first kept in step.
	started config.Accounts
	// seen are the accounts upkeep found the config file making as the record
	// was last kept in step, whatever came of it: nil before it first was. mu
	// guards it, as an import reads it.
	mu   sync.Mutex
	seen config.Accounts
	// kept are the accounts the record was last kept in step with: nil
	// before it first was. Upkeep alone reads and writes it.
	kept config.Accounts
}

// newAccountsRecord returns what keeps the accounts record in the state
// directory cfg gives in step with the config, as accountsRecord says, the
// router started with the accounts given: none where it has no state
// directory, and so no record.
func newAccountsRecord(cfg Config, started accounts, owed func() bool) *accountsRecord {
	if cfg.StateDir == "" {
		return nil
	}
	return &accountsRecord{record: identity.NewFile(cfg.StateDir), owed: owed, now: cfg.Now, path: cfg.ConfigFile.path, started: started.configured()}
}

// look keeps the record in step with the config the router follows, as
// followed gives it under the record's lock, where seen, the accounts upkeep
// found the config file making, those the router started with where it's
// none, have changed since it last looked, as accountsRecord says, judging
// again what the record holds then, and logs what that changed. Where it
// can't, as where the record is damaged, it warns, and tries again once the
// accounts upkeep finds configured change, or the router restarts.
func (a *accountsRecord) look(seen config.Accounts) {
	if a == nil || a.owed() || !a.saw(seen) {
		return
	}
	var kept identity.Kept
	var configured config.Accounts
	err := a.record.Change(func(r *identity.Record, _ func() error) error {
		configured = a.followed()
		kept = r.KeepUp(configured, a.labelOf, a.now().UTC())
		return nil
	})
	if err != nil {
		logger.Warn("can't keep the accounts record in step with the config", "error", err)
		return
	}
	a.kept = configured
	logKept(kept)
}

// saw notes seen as the accounts upkeep found the config file making, and
// reports whether they differ from those it found as the record was last
// kept in step: always, before it first was.
func (a *accountsRecord) saw(seen config.Accounts) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.seen != nil && slices.Equal(seen, a.seen) {
		return false
	}
	// Never nil, as of none seen: nil would keep it again at every look.
	a.seen = append(config.Accounts{}, seen...)
	return true
}

// followed returns the accounts the config file makes as it stands, read
// now, under the record's lock where the caller holds it, as accounts add and
// remove save the config with the record under that lock: so the record is
// kept in step with the config as they left it, never as it stood a moment
// before. Where the router has no config file, or it makes no valid config
// now, they're those upkeep last found it making, else those the router
// started with.
func (a *accountsRecord) followed() config.Accounts {
	if a.path != "" {
		if cfg, err := config.Load(a.path); err == nil {
			return cfg.Accounts
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.seen) == 0 {
		return a.started
	}
	return a.seen
}

// labelOf is the label the account with the given id last had in the config,
// as the router knows it: in the accounts the record was last kept in step
// with, else those it started with; "" where it knows none, as of an account
// that left the config while no router ran.
func (a *accountsRecord) labelOf(id string) string {
	for _, as := range []config.Accounts{a.kept, a.started} {
		if i := slices.IndexFunc(as, func(c config.Account) bool { return strings.EqualFold(c.ID, id) }); i >= 0 {
			return as[i].Label
		}
	}
	return ""
}

// logKept logs what keeping the accounts record in step with the config
// changed of it, as kept has it: each account added, marked removed or
// configured again, and, at warn, each left out, as its id clashes.
func logKept(kept identity.Kept) {
	for _, id := range kept.Added {
		logger.Info("account configured here added to the accounts record", "account", id)
	}
	for _, id := range kept.Removed {
		logger.Info("account no longer configured marked removed in the accounts record", "account", id)
	}
	for _, id := range kept.Returned {
		logger.Info("account configured again; the accounts record has it so", "account", id)
	}
	for _, id := range kept.Clashed {
		logger.Warn("account configured under an id the accounts record holds for another, or that switchboard doesn't make; left out of the record", "account", id)
	}
}
