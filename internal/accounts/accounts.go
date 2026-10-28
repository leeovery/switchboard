// Package accounts adds switchboard's accounts, edits them, replaces their
// tokens and removes them: an account's [[account]] table in the config file
// and its token file together. A token the user gives, typed unseen at a
// terminal or piped in, is checked with the API before it's saved.
package accounts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/redact"
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
	// Now reads the clock, which says when a token the user gives is taken:
	// nil reads the system's.
	Now func() time.Time
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

// Checked is what the API answered of a token it didn't refuse.
type Checked struct {
	// Unchecked is set when the API didn't answer whether it takes the token.
	Unchecked bool
	// Why is why it didn't, with nothing shaped like a token in it: "" when
	// it did.
	Why string
	// Organization is the id of the organization the API's answer named the
	// token's account in: "" where it named none.
	Organization string
}

// answer is what the API answered of a token it didn't refuse, as Checked
// has it, with the error that says why it didn't answer, when it didn't.
type answer struct {
	Checked
	unanswered error
}

// Check asks the API at the config's upstream whether it takes the token, as
// Add and SetToken do. It fails, wrapping ErrRefused, when the API refuses
// the token, and says why when the API doesn't answer.
func (r Registry) Check(ctx context.Context, token tokens.Token) (Checked, error) {
	draft, err := config.Edit(r.ConfigPath)
	if err != nil {
		return Checked{}, err
	}
	upstream, err := draft.Upstream()
	if err != nil {
		return Checked{}, err
	}
	a, err := r.check(ctx, upstream, token)
	return a.Checked, err
}

// Add registers an account: its [[account]] table in the config file, made
// when there's none, and, when its token file holds no usable token, the
// token the user gives, unless the API refuses it, when nothing is saved.
func (r Registry) Add(ctx context.Context, account config.NewAccount) (Taken, error) {
	var taken Taken
	err := r.add(account, func(upstream string) (err error) {
		taken, err = r.keepOrTake(ctx, account.ID, upstream)
		return err
	})
	if err != nil {
		return Taken{}, err
	}
	logAdded(account, taken.Kept)
	return taken, nil
}

// AddGiven registers an account as Add does, with the token given, which is
// checked, as Check checks it, then saved, whatever its token file holds.
func (r Registry) AddGiven(ctx context.Context, account config.NewAccount, token tokens.Token) (Checked, error) {
	var checked Checked
	err := r.add(account, func(upstream string) error {
		a, err := r.save(ctx, account.ID, upstream, token)
		checked = a.Checked
		return err
	})
	if err != nil {
		return Checked{}, err
	}
	logAdded(account, false)
	return checked, nil
}

// add adds the account's [[account]] table to the config file once give has
// given it its token, checked with the API at the config's upstream: nothing
// is saved where give fails.
func (r Registry) add(account config.NewAccount, give func(upstream string) error) error {
	draft, err := config.Edit(r.ConfigPath)
	if err != nil {
		return err
	}
	if err := draft.AddAccount(account); err != nil {
		return err
	}
	if err := give(draft.Config().Upstream); err != nil {
		return err
	}
	return draft.Save()
}

func logAdded(account config.NewAccount, kept bool) {
	logger.Info("added an account", "account", account.ID, "primary", account.Primary, "token_kept", kept)
}

// SetToken replaces the token of the account with the given id, which cfg
// configures, with the one the user gives, unless the API refuses it.
func (r Registry) SetToken(ctx context.Context, cfg *config.Config, id string) (Taken, error) {
	if _, err := configured(cfg, id); err != nil {
		return Taken{}, err
	}
	return r.take(ctx, id, cfg.Upstream)
}

// SetTokenGiven replaces the token of the account with the given id, which
// the config file configures, with the one given, checked, as Check checks
// it, then saved.
func (r Registry) SetTokenGiven(ctx context.Context, id string, token tokens.Token) (Checked, error) {
	draft, err := config.Edit(r.ConfigPath)
	if err != nil {
		return Checked{}, err
	}
	cfg := draft.Config()
	if _, err := configured(cfg, id); err != nil {
		return Checked{}, err
	}
	a, err := r.save(ctx, id, cfg.Upstream, token)
	return a.Checked, err
}

// Removed is what removing an account removed of its token file, as
// tokens.Store's Remove says, and, when it was the primary, the account
// that's the primary now.
type Removed struct {
	tokens.Removed
	// Primary is the id of the account that's the primary now, when the one
	// removed was: "" when it wasn't.
	Primary string
}

// Remove removes the account with the given id: its [[account]] table from
// the config file, then its token file, saying what it removed of that, and
// which account is the primary now, when it removed the primary.
func (r Registry) Remove(id string) (Removed, error) {
	draft, err := config.Edit(r.ConfigPath)
	if err != nil {
		return Removed{}, err
	}
	before := draft.Config()
	if err := draft.RemoveAccount(id); err != nil {
		return Removed{}, err
	}
	if err := draft.Save(); err != nil {
		return Removed{}, err
	}
	file, err := r.Tokens.Remove(id)
	if err != nil {
		return Removed{}, err
	}
	removed := Removed{Removed: file}
	if before.Accounts.Primary().ID == id {
		removed.Primary = draft.Config().Accounts.Primary().ID
	}
	logger.Info("removed an account", "account", id, "token_file", file.File, "linked_to", file.LinkedTo, "linked_to_gone", file.LinkedToGone, "primary_now", removed.Primary)
	return removed, nil
}

// keepOrTake keeps the account's token when its token file holds a usable
// one, and otherwise takes the one the user gives.
func (r Registry) keepOrTake(ctx context.Context, id, upstream string) (Taken, error) {
	if _, err := r.Tokens.Read(id); err == nil {
		return Taken{Kept: true}, nil
	}
	return r.take(ctx, id, upstream)
}

// take has the user give the account's token, and saves it as save does.
func (r Registry) take(ctx context.Context, id, upstream string) (Taken, error) {
	token, err := r.Input.read(id)
	if err != nil {
		return Taken{}, err
	}
	a, err := r.save(ctx, id, upstream, token)
	if err != nil {
		return Taken{}, err
	}
	return Taken{Unchecked: a.unanswered}, nil
}

// save saves the token to the account's token file, with its record, unless
// the API at upstream refuses it.
func (r Registry) save(ctx context.Context, id, upstream string, token tokens.Token) (answer, error) {
	a, err := r.check(ctx, upstream, token, "account", id)
	if err != nil {
		return answer{}, err
	}
	if err := r.Tokens.Write(id, token, r.now()); err != nil {
		return answer{}, err
	}
	logger.Info("saved a token", "account", id, "checked", !a.Unchecked)
	return a, nil
}

// now reads the registry's clock, else the system's.
func (r Registry) now() time.Time {
	if r.Now == nil {
		return time.Now()
	}
	return r.Now()
}

// check asks the API at upstream whether it takes the token, by probing with
// it. It fails, wrapping ErrRefused, when the API refuses the token, and says
// why when the API doesn't answer, warning of it with the log attributes
// given.
func (r Registry) check(ctx context.Context, upstream string, token tokens.Token, logged ...any) (answer, error) {
	probed, err := r.Prober(upstream).Probe(ctx, token.Reveal())
	if err == nil {
		return answer{Organization: probed.Organization}, nil
	}
	why := redact.Text(err.Error(), token.Reveal())
	if refused, ok := errors.AsType[refusal](err); ok && refused.Refused() {
		return answer{}, fmt.Errorf("%w (%s), so nothing is saved: make another with claude setup-token, run while signed in to that subscription", ErrRefused, why)
	}
	logger.Warn("couldn't check a token with the API", append(logged, "error", why)...)
	return answer{Unchecked: true, Why: why, unanswered: err}, nil
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
