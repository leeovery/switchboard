package accounts

import (
	"errors"

	"github.com/leeovery/switchboard/internal/config"
)

// What giving an account a label fails with, where the label is none, or
// another configured account's. Every account has a label of its own, as it
// shows, ignoring case; a config that doesn't, edited by hand, still loads,
// but nothing that writes it gives one so.
var (
	ErrNoLabel    = errors.New("a label is required: every account has one")
	ErrLabelTaken = errors.New("another account has that label already, ignoring case: give each its own")
)

// CheckNew checks an account to add, its id not yet known, as it would be
// added to the config file as it stands, writing nothing, and returns it as
// it would be added, as checkNew has it, its NewPool, where it has one,
// checked as CheckNewPool checks one and its Pool left as given, and its
// Primary as given: whether it's marked its pool's primary, as the first in
// it, is worked out as it's added, as AddChecked says. It fails
// with a NewPoolError where the new pool's refused; with a config.WrongLabel
// that names no account, or with ErrNoLabel or ErrLabelTaken; with a
// PoolNeededError as inPool does; and as config.Edit fails.
func (r Registry) CheckNew(adding Adding) (Adding, error) {
	draft, err := config.Edit(r.ConfigPath, r.HomeDir)
	if err != nil {
		return Adding{}, err
	}
	checked, _, err := checkNew(draft, adding, checkedID)
	if err != nil {
		return Adding{}, err
	}
	if adding.NewPool != nil {
		checked.Account.Pool = adding.Account.Pool
	}
	return checked, nil
}

// checkNew returns the account to add as it would be added to the draft,
// the pool it's to go in made there first, where it's a new one, under the
// id given, and its label as config takes it: its label checked against the
// accounts the draft configures; and in the pool inPool gives it, its
// Primary as given. It fails, as CheckNew says, where the new pool's refused, the label isn't
// one the account can have, or there's no pool to add it to.
func checkNew(draft *config.Draft, adding Adding, id poolID) (Adding, string, error) {
	label, err := config.TakeLabel(adding.Account.Label)
	switch {
	case err != nil:
		return Adding{}, "", err
	case label == "":
		return Adding{}, "", ErrNoLabel
	case labelTaken(draft.Config(), "", label):
		return Adding{}, "", ErrLabelTaken
	}
	if adding.NewPool != nil {
		if adding.Account.Pool, err = madeIn(draft, *adding.NewPool, id); err != nil {
			return Adding{}, "", err
		}
	}
	if adding.Account, err = inPool(draft.Config(), adding.Account); err != nil {
		return Adding{}, "", err
	}
	return adding, label, nil
}

// firstIn reports whether an account going in the pool with the given id
// that cfg configures is the first in it, so its primary, which it's marked,
// added or moved there: where cfg has pools, and none of its accounts is in
// that one.
func firstIn(cfg *config.Config, pool string) bool {
	return cfg != nil && len(cfg.Pools) > 0 && len(cfg.Accounts.InPool(pool)) == 0
}

// labelTaken reports whether an account cfg configures, but the one with the
// given id, shows label, as config.Accounts' LabelTaken says: none where cfg
// is nil.
func labelTaken(cfg *config.Config, id, label string) bool {
	return cfg != nil && cfg.Accounts.LabelTaken(id, label)
}
