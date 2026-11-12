package accounts

import (
	"strings"

	"github.com/leeovery/switchboard/internal/config"
)

// Change is what editing an account changes of it: each field left nil, or
// Primary false, leaves what it sets as it is.
type Change struct {
	// Label is the name it's shown by, which every account has, its own.
	Label *string
	// Plan is the plan its subscription is on, one of config.Plans: ""
	// leaves it unknown.
	Plan *string
	// Primary makes it its pool's primary, in place of the one that is.
	Primary bool
	// Reserve is the share of every window the router leaves unused on it: 0
	// for none.
	Reserve *float64
	// Pool is the id of the pool it's moved into: moved, it's that pool's
	// primary only as Primary makes it.
	Pool *string
}

// Asks reports whether the change asks for anything.
func (c Change) Asks() bool {
	return c.Label != nil || c.Plan != nil || c.Primary || c.Reserve != nil || c.Pool != nil
}

// Edited is an account as it was and as an edit left it, and the account
// the edit made no longer the primary, when it made this one the primary.
type Edited struct {
	Was, Now config.Account
	// WasLabelled and NowLabelled are set where the config wrote a label for
	// the account before the edit, and where it does after: where it doesn't,
	// the account's shown by its id.
	WasLabelled, NowLabelled bool
	// FormerPrimary is the id of the account that was the primary before the
	// edit made this one so: "" when it didn't.
	FormerPrimary string
	// Marked is set where the edit marked the account the primary in the
	// config, which didn't before: so too where it was the primary already,
	// only as the first with none marked, where FormerPrimary is "".
	Marked bool
	// Unaliased is the old name the config gave the account's plan, which
	// the edit wrote by its new one: "" where it didn't.
	Unaliased string
	// Before is the config the edit read, as it was before the edit.
	Before *config.Config
}

// Changed reports whether the edit changed the account, as the config reads
// it, marks it or names its plan.
func (e Edited) Changed() bool {
	return e.Was != e.Now || e.Marked || e.Unaliased != ""
}

// Configured fails, matching config.ErrNotConfigured, when no account in the
// config file has the given id, and as config.Edit does for a config file it
// can't read.
func (r Registry) Configured(id string) error {
	draft, err := config.Edit(r.ConfigPath, r.HomeDir)
	if err != nil {
		return err
	}
	_, err = configured(draft.Config(), id)
	return err
}

// Edit changes the account with the given id as change says, in its
// [[account]] table in the config file. A change that leaves the account as
// it was, as the config reads it and marks it, writes nothing. It fails,
// matching config.ErrNotConfigured, when no account has the id; and with
// ErrNoLabel or ErrLabelTaken for a label that's none, or another configured
// account's, as CheckNew has them.
func (r Registry) Edit(id string, change Change) (Edited, error) {
	draft, err := config.Edit(r.ConfigPath, r.HomeDir)
	if err != nil {
		return Edited{}, err
	}
	before := draft.Config()
	was, err := configured(before, id)
	if err != nil {
		return Edited{}, err
	}
	if change.Label != nil && *change.Label == "" {
		return Edited{}, ErrNoLabel
	}
	wasLabelled, wasMarked, wasAlias := draft.Labelled(id), draft.MarksPrimaryOf(id), draft.PlanAlias(id)
	if err := change.apply(draft, was); err != nil {
		return Edited{}, err
	}
	now, err := configured(draft.Config(), id)
	if err != nil {
		return Edited{}, err
	}
	if change.Label != nil && labelTaken(draft.Config(), id, now.Label) {
		return Edited{}, ErrLabelTaken
	}
	edited := Edited{
		Was: was, Now: now,
		WasLabelled: wasLabelled, NowLabelled: draft.Labelled(id),
		Marked: !wasMarked && draft.MarksPrimaryOf(id),
		Before: before,
	}
	if draft.PlanAlias(id) != wasAlias {
		edited.Unaliased = wasAlias
	}
	if !edited.Changed() {
		return edited, nil
	}
	if now.Primary && (!was.Primary || !strings.EqualFold(was.Pool, now.Pool)) {
		edited.FormerPrimary = primaryOf(before, now.Pool)
	}
	if err := draft.Save(); err != nil {
		return Edited{}, err
	}
	logger.Info("edited an account", "account", id, "primary", now.Primary, "plan", now.Plan, "reserve", now.Reserve)
	return edited, nil
}

// primaryOf returns the id of the primary of the pool with the given id that
// cfg configures, the primary of every account in a config with no pools:
// "" where the pool has no account.
func primaryOf(cfg *config.Config, pool string) string {
	if len(cfg.Pools) == 0 {
		return cfg.Accounts.Primary().ID
	}
	primary, _ := cfg.Accounts.PrimaryOf(pool)
	return primary.ID
}

// apply makes the change to the account in the draft, as it was before.
func (c Change) apply(draft *config.Draft, was config.Account) error {
	if c.Label != nil {
		if err := draft.SetLabel(was.ID, *c.Label); err != nil {
			return err
		}
	}
	if c.Plan != nil {
		if err := draft.SetPlan(was.ID, *c.Plan); err != nil {
			return err
		}
	}
	if c.Pool != nil {
		if err := draft.SetPool(was.ID, *c.Pool); err != nil {
			return err
		}
	}
	if c.Primary {
		if err := draft.SetPrimary(was.ID); err != nil {
			return err
		}
	}
	if c.Reserve != nil {
		return draft.SetReserve(was.ID, *c.Reserve)
	}
	return nil
}

// configured returns the account with the given id that cfg configures,
// failing, matching config.ErrNotConfigured, when none has it, as when cfg
// is nil, configuring none.
func configured(cfg *config.Config, id string) (config.Account, error) {
	if cfg != nil {
		for _, a := range cfg.Accounts {
			if a.ID == id {
				return a, nil
			}
		}
	}
	return config.Account{}, config.NotConfigured(id)
}
