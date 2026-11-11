package router

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
)

// accountsRecord keeps the accounts record in step with the config the
// router finds, as identity's KeepUp keeps it, so an account configured or
// removed on another Mac sharing this one's config is one this Mac has had
// too: its token held to its organization, and, once removed, named by its
// label. It keeps it so once the move onto the accounts' ids is done, never
// while it's owed, as the config's ids and the record's differ till then: as
// the router starts, at each look after, where the accounts configured have
// changed since, and at the first look once the move is done. Upkeep alone
// uses it.
type accountsRecord struct {
	record identity.File
	owed   func() bool
	now    func() time.Time
	// started are the accounts the router started with: those configured
	// while the config file has made no other, and the labels of those that
	// leave it before the record was first kept in step.
	started config.Accounts
	// kept are the accounts configured as the record was last kept in step
	// with them, whatever came of it: nil before it first was.
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
	return &accountsRecord{record: identity.NewFile(cfg.StateDir), owed: owed, now: cfg.Now, started: started.configured()}
}

// look keeps the record in step with the accounts configured, those the
// router started with where none's given, as accountsRecord says, under the
// record's lock, judging again what the record holds then, and logs what that
// changed. Where it can't, as where the record is damaged, it warns, and
// tries again once the accounts configured change, or the router restarts.
func (a *accountsRecord) look(configured config.Accounts) {
	if a == nil || a.owed() {
		return
	}
	if len(configured) == 0 {
		configured = a.started
	}
	if a.kept != nil && slices.Equal(configured, a.kept) {
		return
	}
	var kept identity.Kept
	err := a.record.Change(func(r *identity.Record, _ func() error) error {
		kept = r.KeepUp(configured, a.labelOf, a.now().UTC())
		return nil
	})
	// Never nil, as of none configured: nil would keep it again at every look.
	a.kept = append(config.Accounts{}, configured...)
	if err != nil {
		logger.Warn("can't keep the accounts record in step with the config", "error", err)
		return
	}
	logKept(kept)
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
