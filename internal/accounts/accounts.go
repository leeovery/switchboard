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
	// ErrInterrupted is switchboard's interrupt, wrapped or not, which the
	// CLI exits 130 for: what Input.Hidden fails with when the user
	// interrupts it, having put the terminal back as it was, and so taking
	// a token; and installing the service, interrupted.
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
	// user gave, which is saved all the same, as Checked.Why says: nil when
	// it did.
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

// taken is how an account came by the token the user gave, which the API
// answered for as checked says.
func taken(checked Checked) Taken {
	if !checked.Unchecked {
		return Taken{}
	}
	return Taken{Unchecked: errors.New(checked.Why)}
}

// Check asks the API at the config's upstream whether it takes the token, as
// Add and SetToken do, saving nothing. It fails, wrapping ErrRefused, when
// the API refuses the token, and with ctx's error once ctx is done; and says
// why when the API doesn't answer.
func (r Registry) Check(ctx context.Context, token tokens.Token) (Checked, error) {
	draft, err := config.Edit(r.ConfigPath)
	if err != nil {
		return Checked{}, err
	}
	upstream, err := draft.Upstream()
	if err != nil {
		return Checked{}, err
	}
	return r.check(ctx, upstream, token)
}

// Add registers an account: its [[account]] table in the config file, made
// when there's none, and, when its token file holds no usable token, the
// token the user gives, unless the API refuses it, when nothing is saved.
func (r Registry) Add(ctx context.Context, account config.NewAccount) (Taken, error) {
	var t Taken
	err := r.add(account, func(upstream string) (err error) {
		t, err = r.keepOrTake(ctx, account.ID, upstream)
		return err
	})
	if err != nil {
		return Taken{}, err
	}
	logAdded(account, t.Kept)
	return t, nil
}

// AddGiven registers an account as AddChecked does, the token given checked
// first, as Check checks it, once the account is shown to be one the config
// can add: nothing is saved where the API refuses the token.
func (r Registry) AddGiven(ctx context.Context, account config.NewAccount, token tokens.Token) (Checked, error) {
	var checked Checked
	err := r.add(account, func(upstream string) (err error) {
		if checked, err = r.check(ctx, upstream, token); err != nil {
			return err
		}
		return r.write(ctx, account.ID, token, checked)
	})
	if err != nil {
		return Checked{}, err
	}
	logAdded(account, false)
	return checked, nil
}

// AddChecked registers an account as Add does, with the token given, which
// Check has checked, as checked says, saving it whatever its token file
// holds. It doesn't ask the API, and saves nothing once ctx is done.
// checked must be what Check gave for this token: nothing ties the two.
func (r Registry) AddChecked(ctx context.Context, account config.NewAccount, token tokens.Token, checked Checked) error {
	err := r.add(account, func(string) error {
		return r.write(ctx, account.ID, token, checked)
	})
	if err != nil {
		return err
	}
	logAdded(account, false)
	return nil
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
	upstream, err := draft.Upstream()
	if err != nil {
		return err
	}
	if err := give(upstream); err != nil {
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

// SetTokenGiven replaces the token of the account with the given id as
// SetTokenChecked does, the token given checked first, as Check checks it,
// once the account is shown to be configured: nothing is saved where the API
// refuses the token.
func (r Registry) SetTokenGiven(ctx context.Context, id string, token tokens.Token) (Checked, error) {
	draft, err := config.Edit(r.ConfigPath)
	if err != nil {
		return Checked{}, err
	}
	if _, err := configured(draft.Config(), id); err != nil {
		return Checked{}, err
	}
	upstream, err := draft.Upstream()
	if err != nil {
		return Checked{}, err
	}
	checked, err := r.check(ctx, upstream, token)
	if err != nil {
		return Checked{}, err
	}
	if err := r.write(ctx, id, token, checked); err != nil {
		return Checked{}, err
	}
	return checked, nil
}

// SetTokenChecked replaces the token of the account with the given id, which
// the config file configures, with the one given, which Check has checked,
// as checked says. It doesn't ask the API, and saves nothing once ctx is
// done. checked must be what Check gave for this token: nothing ties the two.
func (r Registry) SetTokenChecked(ctx context.Context, id string, token tokens.Token, checked Checked) error {
	if err := r.Configured(id); err != nil {
		return err
	}
	return r.write(ctx, id, token, checked)
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

// take has the user give the account's token, and saves it to the account's
// token file, with its record, unless the API at upstream refuses it.
func (r Registry) take(ctx context.Context, id, upstream string) (Taken, error) {
	token, err := r.Input.read(id)
	if err != nil {
		return Taken{}, err
	}
	checked, err := r.check(ctx, upstream, token)
	if err != nil {
		return Taken{}, err
	}
	if err := r.write(ctx, id, token, checked); err != nil {
		return Taken{}, err
	}
	return taken(checked), nil
}

// write writes the token to the account's token file, with its record,
// warning where checked says the API didn't answer for it. It writes nothing
// once ctx is done.
func (r Registry) write(ctx context.Context, id string, token tokens.Token, checked Checked) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("save the token: %w", err)
	}
	if checked.Unchecked {
		logger.Warn("couldn't check a token with the API", "account", id, "error", checked.Why)
	}
	if err := r.Tokens.Write(id, token, r.now()); err != nil {
		return err
	}
	logger.Info("saved a token", "account", id, "checked", !checked.Unchecked)
	return nil
}

// now reads the registry's clock, else the system's.
func (r Registry) now() time.Time {
	if r.Now == nil {
		return time.Now()
	}
	return r.Now()
}

// check asks the API at upstream whether it takes the token, by probing with
// it. It fails, wrapping ErrRefused, when the API refuses the token, and with
// ctx's error once ctx is done, whatever the probe made of it; and says why
// when the API doesn't answer.
func (r Registry) check(ctx context.Context, upstream string, token tokens.Token) (Checked, error) {
	probed, err := r.Prober(upstream).Probe(ctx, token.Reveal())
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Checked{}, fmt.Errorf("check the token with the API: %w", ctxErr)
	}
	if err == nil {
		return Checked{Organization: probed.Organization}, nil
	}
	why := redact.Text(err.Error(), token.Reveal())
	if refused, ok := errors.AsType[refusal](err); ok && refused.Refused() {
		return Checked{}, fmt.Errorf("%w (%s), so nothing is saved: make another with claude setup-token, run while signed in to that subscription", ErrRefused, why)
	}
	return Checked{Unchecked: true, Why: why}, nil
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
