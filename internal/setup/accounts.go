package setup

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
)

// accounts lists the accounts, each with whether its token is usable, taking
// a token for each without one; offers to add accounts; and settles which is
// the primary.
func (r *run) accounts(ctx context.Context) error {
	draft, err := config.Edit(r.registry.ConfigPath)
	if err != nil {
		return err
	}
	cfg := draft.Config()
	if cfg != nil {
		for _, a := range cfg.Accounts {
			if err := r.token(ctx, cfg, a); err != nil {
				return err
			}
		}
	}
	if err := r.addAccounts(ctx, cfg == nil); err != nil {
		return err
	}
	return r.primary()
}

// token says whether the account's token is usable, and takes one the user
// gives when it isn't. A token the user doesn't give, or the API refuses, is
// told of, and leaves the account without one; an interrupt stops setup.
func (r *run) token(ctx context.Context, cfg *config.Config, a config.Account) error {
	_, err := r.registry.Tokens.Read(a.ID)
	if err == nil {
		r.Terminal.sayf("%s: its token is usable.", title(a))
		return nil
	}
	r.Terminal.sayf("%s has no usable token: %v", title(a), err)
	taken, err := r.registry.SetToken(ctx, cfg, a.ID)
	switch {
	case errors.Is(err, accounts.ErrInterrupted):
		return err
	case err != nil:
		r.Terminal.sayf("%s has none still: %v. Run setup again, or switchboard accounts token %s, to give it one.", a.ID, err, a.ID)
		return nil
	}
	r.changed = true
	r.Terminal.sayf("Saved %s's token.%s", a.ID, unchecked(taken))
	return nil
}

// addAccounts offers to add accounts, one at a time, until the user has none
// to add. With none configured, it asks for the first straight away, as
// there's nothing to set up without one.
func (r *run) addAccounts(ctx context.Context, none bool) error {
	if none {
		r.Terminal.sayf("No accounts yet. Each is a Claude subscription, whose token claude setup-token makes, run while signed in to it: add the first.")
	}
	for {
		if !none {
			more, err := r.Terminal.confirm("Add another account?", false)
			if err != nil || !more {
				return err
			}
		}
		id, err := r.askID()
		if err != nil || id == "" {
			return err
		}
		added, err := r.addAccount(ctx, id)
		if err != nil {
			return err
		}
		none = none && !added
	}
}

// askID asks for a new account's id until it's one an account can have, or
// the user gives none.
func (r *run) askID() (string, error) {
	for {
		id, err := r.Terminal.ask("Its id, for good: letters, digits, - and _, such as work: ")
		if err != nil || id == "" {
			return "", err
		}
		if err := config.CheckID(id); err != nil {
			r.Terminal.sayf("%v.", err)
			continue
		}
		return id, nil
	}
}

// addAccount adds the account with the given id, asking for its label, and
// for its token unless its token file holds a usable one, and reports
// whether it did. What stops it, such as a token the API refuses, is told
// of, and adds nothing; an interrupt stops setup.
func (r *run) addAccount(ctx context.Context, id string) (bool, error) {
	label, err := r.Terminal.ask(fmt.Sprintf("Its label, to show it by (Enter for %s): ", id))
	if err != nil {
		return false, err
	}
	taken, err := r.registry.Add(ctx, config.NewAccount{ID: id, Label: label})
	switch {
	case errors.Is(err, accounts.ErrInterrupted):
		return false, err
	case err != nil:
		r.Terminal.sayf("%s isn't added: %v.", id, err)
		return false, nil
	}
	r.changed = true
	kept := ""
	if taken.Kept {
		kept = ", keeping the token in " + r.registry.Tokens.Path(id)
	}
	r.Terminal.sayf("Added %s%s.%s", id, kept, unchecked(taken))
	return true, nil
}

// primary says which account is the primary. With more than one and none
// marked, it asks which the browser and the Claude apps are signed into,
// and marks that one.
func (r *run) primary() error {
	draft, err := config.Edit(r.registry.ConfigPath)
	if err != nil {
		return err
	}
	if r.cfg = draft.Config(); r.cfg == nil {
		return errNoAccount
	}
	configured := r.cfg.Accounts
	if len(configured) == 1 || draft.MarksPrimary() {
		r.Terminal.sayf("The primary, the account the browser and the Claude apps are signed into, is %s.", title(configured.Primary()))
		return nil
	}
	r.Terminal.sayf("Which account are the browser and the Claude apps signed into? It's the primary: Claude Code's own token is its, and the router leaves a share of its quota for the apps.")
	id, err := r.choose(configured.IDs())
	if err != nil {
		return err
	}
	if err := draft.SetPrimary(id); err != nil {
		return err
	}
	if err := draft.Save(); err != nil {
		return err
	}
	r.cfg, r.changed = draft.Config(), true
	logger.Info("marked the primary", "account", id)
	r.Terminal.sayf("The primary is %s.", title(r.cfg.Accounts.Primary()))
	return nil
}

// choose asks which of the accounts with the given ids is the primary, until
// the answer is one of them: Enter gives the first.
func (r *run) choose(ids []string) (string, error) {
	question := fmt.Sprintf("The primary, of %s (Enter for %s): ", strings.Join(ids, ", "), ids[0])
	for {
		id, err := r.Terminal.ask(question)
		switch {
		case err != nil:
			return "", err
		case id == "":
			return ids[0], nil
		case slices.Contains(ids, id):
			return id, nil
		}
		r.Terminal.sayf("There's no account %q.", id)
	}
}

// unchecked says, when the API didn't answer whether it takes the token
// taken, that it was saved all the same.
func unchecked(taken accounts.Taken) string {
	if taken.Unchecked == nil {
		return ""
	}
	return fmt.Sprintf(" The API didn't answer to check the token (%v), so it's saved unchecked.", taken.Unchecked)
}
