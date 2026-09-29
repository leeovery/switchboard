// Package accounts adds switchboard's accounts, replaces their tokens and
// removes them: an account's [[account]] table in the config file and its
// token file together. A token the user gives, typed unseen at a terminal or
// piped in, is checked with the API before it's saved.
package accounts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/tokens"
)

var logger = logs.For("accounts")

var (
	// ErrRefused is what taking a token the API refuses fails with, wrapped.
	ErrRefused = errors.New("the API refused the token")
	// ErrInterrupted is what Input.Hidden fails with, wrapped or not, when
	// the user interrupts it, having put the terminal back as it was, and so
	// what taking the token fails with.
	ErrInterrupted = errors.New("interrupted")
)

// prompt asks the user at a terminal for the token of the account it names.
const prompt = "Paste %s's token, from claude setup-token run while signed in to that subscription (it won't show): "

// Prober reads an account's usage with its token, which asks the API whether
// it takes the token. A failure whose Refused method reports true, as the
// Claude provider's do, is the API refusing it; any other, the API not
// answering whether it does.
type Prober interface {
	Probe(ctx context.Context, token string) (quota.Probe, error)
}

// refusal is a probe's failure that says whether the API refused the token.
type refusal interface {
	error
	Refused() bool
}

// Input is where the user gives a token.
type Input struct {
	// Hidden reads a line typed at the terminal the user is at, without
	// showing it: nil when they aren't at one. It fails with ErrInterrupted
	// when the user interrupts it.
	Hidden func() ([]byte, error)
	// Prompt is where the user at a terminal is asked for the token.
	Prompt io.Writer
	// Piped holds the token alone, when the user isn't at a terminal.
	Piped io.Reader
}

// Registry is switchboard's accounts: their [[account]] tables in the config
// file, and their token files.
type Registry struct {
	ConfigPath string
	Tokens     tokens.Store
	// Prober returns what checks a token with the API at upstream, the
	// config's.
	Prober func(upstream string) Prober
	Input  Input
}

// Taken is how an account came by its token.
type Taken struct {
	// Kept is set when its token file held a usable token already, which it
	// keeps.
	Kept bool
	// Unchecked is why the API didn't answer whether it takes the token the
	// user gave, which is saved all the same: nil when it did.
	Unchecked error
}

// Add registers an account: its [[account]] table in the config file, made
// when there's none, and, when its token file holds no usable token, the
// token the user gives, unless the API refuses it, when nothing is saved.
func (r Registry) Add(ctx context.Context, account config.NewAccount) (Taken, error) {
	draft, err := config.Edit(r.ConfigPath)
	if err != nil {
		return Taken{}, err
	}
	if err := draft.AddAccount(account); err != nil {
		return Taken{}, err
	}
	taken, err := r.keepOrTake(ctx, account.ID, draft.Config().Upstream)
	if err != nil {
		return Taken{}, err
	}
	if err := draft.Save(); err != nil {
		return Taken{}, err
	}
	logger.Info("added an account", "account", account.ID, "primary", account.Primary, "token_kept", taken.Kept)
	return taken, nil
}

// SetToken replaces the token of the account with the given id, which cfg
// configures, with the one the user gives, unless the API refuses it.
func (r Registry) SetToken(ctx context.Context, cfg *config.Config, id string) (Taken, error) {
	if !slices.Contains(cfg.Accounts.IDs(), id) {
		return Taken{}, config.NotConfigured(id)
	}
	return r.take(ctx, id, cfg.Upstream)
}

// Remove removes the account with the given id: its [[account]] table from
// the config file, then its token file, reporting whether it had one.
func (r Registry) Remove(id string) (bool, error) {
	draft, err := config.Edit(r.ConfigPath)
	if err != nil {
		return false, err
	}
	if err := draft.RemoveAccount(id); err != nil {
		return false, err
	}
	if err := draft.Save(); err != nil {
		return false, err
	}
	had := r.Tokens.Has(id)
	if err := r.Tokens.Remove(id); err != nil {
		return false, err
	}
	logger.Info("removed an account", "account", id, "token_file", had)
	return had, nil
}

// keepOrTake keeps the account's token when its token file holds a usable
// one, and otherwise takes the one the user gives.
func (r Registry) keepOrTake(ctx context.Context, id, upstream string) (Taken, error) {
	if _, err := r.Tokens.Read(id); err == nil {
		return Taken{Kept: true}, nil
	}
	return r.take(ctx, id, upstream)
}

// take has the user give the account's token, and saves it to the account's
// token file unless the API at upstream refuses it.
func (r Registry) take(ctx context.Context, id, upstream string) (Taken, error) {
	token, err := r.Input.read(id)
	if err != nil {
		return Taken{}, err
	}
	taken, err := r.check(ctx, id, upstream, token)
	if err != nil {
		return Taken{}, err
	}
	if err := r.Tokens.Write(id, token); err != nil {
		return Taken{}, err
	}
	logger.Info("saved a token", "account", id, "checked", taken.Unchecked == nil)
	return taken, nil
}

// check asks the API at upstream whether it takes the account's token, by
// probing with it. It fails, wrapping ErrRefused, when the API refuses the
// token, and says why when the API doesn't answer.
func (r Registry) check(ctx context.Context, id, upstream string, token tokens.Token) (Taken, error) {
	_, err := r.Prober(upstream).Probe(ctx, token.Reveal())
	if err == nil {
		return Taken{}, nil
	}
	if refused, ok := errors.AsType[refusal](err); ok && refused.Refused() {
		return Taken{}, fmt.Errorf("%w (%v), so nothing is saved: make another with claude setup-token, run while signed in to that subscription", ErrRefused, err)
	}
	logger.Warn("couldn't check a token with the API", "account", id, "error", err)
	return Taken{Unchecked: err}, nil
}

// read takes the account's token from the user: typed at their terminal,
// unseen, once they're asked for it, or piped in.
func (in Input) read(id string) (tokens.Token, error) {
	if in.Hidden == nil {
		return given(tokens.ParseFrom(in.Piped))
	}
	_, _ = fmt.Fprintf(in.Prompt, prompt, id)
	typed, err := in.Hidden()
	// The newline that ended the token didn't show either.
	_, _ = fmt.Fprintln(in.Prompt)
	if err != nil && !errors.Is(err, io.EOF) {
		return tokens.Token{}, fmt.Errorf("read the token: %w", err)
	}
	return given(tokens.Parse(string(typed)))
}

// given returns the token the user gave, or says what's wrong with what they
// gave, never quoting it.
func given(token tokens.Token, err error) (tokens.Token, error) {
	switch {
	case errors.Is(err, tokens.ErrMissing):
		return tokens.Token{}, errors.New("no token given")
	case errors.Is(err, tokens.ErrNotAToken):
		return tokens.Token{}, errors.New("what was given is more than a token: give the token alone")
	case err != nil:
		return tokens.Token{}, fmt.Errorf("read the token: %w", err)
	}
	return token, nil
}
