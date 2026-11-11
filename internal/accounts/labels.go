package accounts

import (
	"errors"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
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
// added to the config file as it stands, writing nothing: its label taken as
// config takes one, failing with a config.WrongLabel that names no account,
// or with ErrNoLabel or ErrLabelTaken; and as config.Edit fails.
func (r Registry) CheckNew(account config.NewAccount) error {
	draft, err := config.Edit(r.ConfigPath)
	if err != nil {
		return err
	}
	_, err = checkNew(draft.Config(), account)
	return err
}

// checkNew returns the label of the account to add, as config takes it,
// failing, as CheckNew says, where it isn't one the account can have, among
// the accounts cfg configures, nil configuring none.
func checkNew(cfg *config.Config, account config.NewAccount) (string, error) {
	label, err := config.TakeLabel(account.Label)
	switch {
	case err != nil:
		return "", err
	case label == "":
		return "", ErrNoLabel
	case labelTaken(cfg, "", label):
		return "", ErrLabelTaken
	}
	return label, nil
}

// labelTaken reports whether an account cfg configures, but the one with the
// given id, shows label, as each shows, ignoring case.
func labelTaken(cfg *config.Config, id, label string) bool {
	if cfg == nil {
		return false
	}
	shown := prose.Clean(label)
	for _, a := range cfg.Accounts {
		if a.ID != id && strings.EqualFold(prose.Clean(a.Label), shown) {
			return true
		}
	}
	return false
}
