// Package accounts adds switchboard's accounts, edits them, replaces their
// tokens and removes them: an account's [[account]] table in the config file,
// its token file and its place in the accounts record together. A token the
// user gives, typed unseen at a terminal or piped in, is checked with the
// API before it's saved, and the organization the API names it in decides
// the id of an account added with it, and must be the account's for one
// replacing its token.
package accounts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/prose"
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

// RefusedError is what taking a token the API refuses fails with: why it
// refused, as the probe said it, any token in it hidden. It wraps
// ErrRefused.
type RefusedError struct {
	Why string
}

// Error says the API refused the token, why, and to make another.
func (e *RefusedError) Error() string {
	return fmt.Sprintf("%v (%s), so nothing is saved: make another with claude setup-token, run while signed in to that subscription", ErrRefused, e.Why)
}

// Unwrap is ErrRefused.
func (e *RefusedError) Unwrap() error {
	return ErrRefused
}

// SameSubscriptionError is what taking a token fails with where its
// subscription is another account's already, as the organization the API
// names it in says: Other names the account.
type SameSubscriptionError struct {
	Other string
}

// Error says which account has the token's subscription, and to make the
// token signed in to the other.
func (e *SameSubscriptionError) Error() string {
	return HasThatSubscription(e.Other) + ", so nothing is saved: make another with claude setup-token, run while signed in to the other subscription"
}

// HasThatSubscription says the account with the given id has a token's
// subscription already, as each refusal of the token begins.
func HasThatSubscription(id string) string {
	return id + " has that subscription already"
}

// Organizations are accounts' organizations, by account id: "" where one
// isn't known.
type Organizations map[string]string

// SameSubscription refuses a token given the account with the given id,
// whose organization is organization, where another account has it,
// logging the refusal: nil where none has, or organization isn't known.
func (o Organizations) SameSubscription(organization, id string) *SameSubscriptionError {
	other := o.holder(organization, id)
	if other == "" {
		return nil
	}
	logger.Info("a token is another account's subscription's, so it isn't saved", "account", id, "other", other)
	return &SameSubscriptionError{Other: other}
}

// holder returns the id of the account other than except whose organization
// is organization, "" where there's none, or organization isn't known.
func (o Organizations) holder(organization, except string) string {
	if organization == "" {
		return ""
	}
	for _, id := range slices.Sorted(maps.Keys(o)) {
		if id != except && o[id] == organization {
			return id
		}
	}
	return ""
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
// file, their token files, and the accounts record.
type Registry struct {
	ConfigPath string
	Tokens     tokens.Store
	// Record is the accounts record, which adding an account, replacing its
	// token and removing it read and write: the zero File fails each.
	Record identity.File
	// Prober returns what checks a token with the API at upstream, the
	// config's.
	Prober func(upstream string) Prober
	Input  Input
	// Others returns the organizations of every account but the one with the
	// given id, which the organization of a token the user gives is compared
	// with, asked only where the API names one: nil compares it with none.
	Others func(ctx context.Context, id string) (Organizations, error)
	// Now reads the clock, which says when a token the user gives is taken,
	// and when an account is added or removed: nil reads the system's.
	Now func() time.Time
	// Random is what the id of an account added is drawn from, where its
	// organization isn't known: nil draws from crypto/rand.
	Random io.Reader
}

// Taken is how an account came by its token.
type Taken struct {
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
// Add and SetToken do, saving nothing. It fails with a *RefusedError when
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

// SetToken replaces the token of the account with the given id, which cfg
// configures, with the one the user gives, unless the API refuses it, or
// it's another account's subscription's, or another subscription's than the
// account's, as replace has it.
func (r Registry) SetToken(ctx context.Context, cfg *config.Config, id string) (Taken, error) {
	if _, err := configured(cfg, id); err != nil {
		return Taken{}, err
	}
	return r.take(ctx, id, cfg.Upstream)
}

// SetTokenChecked replaces the token of the account with the given id, which
// the config file configures, with the one given, which Check has checked,
// as checked says, unless it's another subscription's than the account's,
// as replace has it. It doesn't ask the API, and saves nothing once ctx is
// done. checked must be what Check gave for this token: nothing ties the two.
func (r Registry) SetTokenChecked(ctx context.Context, id string, token tokens.Token, checked Checked) error {
	if err := r.Configured(id); err != nil {
		return err
	}
	return r.replace(ctx, id, token, checked)
}

// OtherSubscriptionError is what replacing an account's token fails with
// where the token's organization isn't the account's, as the accounts
// record has it: it's another subscription's, to add as an account of its
// own. OtherID and OtherLabel name the account the record has of the token's
// organization, where the account's isn't known yet: "" where there's none
// to name.
type OtherSubscriptionError struct {
	OtherID, OtherLabel string
}

// Error says the token is another subscription's, naming the account the
// record has of it where there's one to name, and to add it as an account
// of its own.
func (e *OtherSubscriptionError) Error() string {
	of := ""
	if e.OtherID != "" {
		of = ", that of " + prose.Quoted(e.OtherLabel) + " (" + e.OtherID + ")"
	}
	return "the token is for another subscription" + of + ", so nothing is saved: add it as an account of its own"
}

// replace writes the token to the token file of the account with the given
// id, as write does, where its organization, as checked names it, is the
// account's, as the accounts record has it, and records it as the account's
// where the record doesn't know the account's. It fails with an
// OtherSubscriptionError where it's another's, or where the record doesn't
// know the account's, and has another of the token's. Where the API named
// none, or the record hasn't the account, as one from before switchboard
// made ids, there's nothing to hold it to, and nothing is recorded. It's
// checked, and the token written, then the record, under the record's lock,
// so what the record has can't change in between; the record is written
// only where there's something to record, so a refusal, or a token of the
// account's own, leaves it as it is. Where the record can't be written once
// the token is, it fails saying the token's saved and the record isn't.
func (r Registry) replace(ctx context.Context, id string, token tokens.Token, checked Checked) error {
	return r.Record.Change(func(record *identity.Record, commit func() error) error {
		if err := r.owned(record, id, checked.Organization); err != nil {
			return err
		}
		if err := r.write(ctx, id, token, checked); err != nil {
			return err
		}
		if err := commit(); err != nil {
			return fmt.Errorf("%s's token is saved, but its organization isn't recorded: %w", id, err)
		}
		return nil
	})
}

// owned fails, as replace says, where organization, a token's, is another
// subscription's than the account's with the given id, as record has it,
// recording it in record as the account's where it doesn't know the
// account's.
func (r Registry) owned(record *identity.Record, id, organization string) error {
	e, ok := record.ByID(id)
	switch {
	case organization == "" || !ok || e.Organization == organization:
		return nil
	case e.Organization != "":
		return &OtherSubscriptionError{}
	}
	if other, held := record.ByOrganization(organization); held {
		return &OtherSubscriptionError{OtherID: other.ID, OtherLabel: r.labelOf(other)}
	}
	record.Organize(id, organization)
	return nil
}

// labelOf is the label of the account the record has, e: the one it kept,
// where it's removed, else the config's, else its id.
func (r Registry) labelOf(e identity.Entry) string {
	if e.Label != "" {
		return e.Label
	}
	if draft, err := config.Edit(r.ConfigPath); err == nil {
		if a, err := configured(draft.Config(), e.ID); err == nil {
			return a.Label
		}
	}
	return e.ID
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

// TokenFileLeft is Remove's error where it took the account out of the
// config file, but couldn't delete its token file.
type TokenFileLeft struct {
	ID  string
	err error
}

// Error says the account is taken out of the config, and why its token file
// isn't deleted: its path and why, where that's the failure's.
func (e *TokenFileLeft) Error() string {
	left := e.ID + " is taken out of the config, but its token file"
	if path, ok := errors.AsType[*fs.PathError](e.err); ok {
		return left + ", " + path.Path + ", isn't deleted: " + path.Err.Error()
	}
	return left + " isn't deleted: " + e.err.Error()
}

// Unwrap is why the token file isn't deleted.
func (e *TokenFileLeft) Unwrap() error {
	return e.err
}

// Remove removes the account with the given id: marking it removed in the
// accounts record, with the label it showed, where the record has it, then
// its [[account]] table from the config file, then its token file, all under
// the record's lock, saying what it removed of that, and which account is
// the primary now, when it removed the primary. It fails with a
// TokenFileLeft where it removed the table but not the token file. The
// record is written only where it has the account, and is put back as it
// was where the config can't be saved, as identity.File's Change puts it
// back; should that fail, the account's left marked removed but still
// configured, which removing it again puts right.
func (r Registry) Remove(id string) (Removed, error) {
	var before, after *config.Config
	var file tokens.Removed
	var left error
	err := r.Record.Change(func(record *identity.Record, commit func() error) error {
		draft, err := config.Edit(r.ConfigPath)
		if err != nil {
			return err
		}
		before = draft.Config()
		if err := draft.RemoveAccount(id); err != nil {
			return err
		}
		was, _ := configured(before, id)
		record.Remove(id, was.Label, r.now())
		after = draft.Config()
		if err := saved(commit, draft.Save); err != nil {
			return err
		}
		file, left = r.Tokens.Remove(id)
		return nil
	})
	if err != nil {
		return Removed{}, err
	}
	if left != nil {
		return Removed{}, &TokenFileLeft{ID: id, err: left}
	}
	removed := Removed{Removed: file}
	if before.Accounts.Primary().ID == id {
		removed.Primary = after.Accounts.Primary().ID
	}
	logger.Info("removed an account", "account", id, "token_file", file.File, "linked_to", file.LinkedTo, "linked_to_gone", file.LinkedToGone, "primary_now", removed.Primary)
	return removed, nil
}

// take has the user give the account's token, and saves it to the account's
// token file, with its record, unless the API at upstream refuses it, or
// it's another account's subscription's, or another subscription's than the
// account's, as replace has it.
func (r Registry) take(ctx context.Context, id, upstream string) (Taken, error) {
	token, err := r.Input.read(id)
	if err != nil {
		return Taken{}, err
	}
	checked, err := r.check(ctx, upstream, token)
	if err != nil {
		return Taken{}, err
	}
	if err := r.unheld(ctx, id, checked.Organization); err != nil {
		return Taken{}, err
	}
	if err := r.replace(ctx, id, token, checked); err != nil {
		return Taken{}, err
	}
	return taken(checked), nil
}

// unheld fails with a *SameSubscriptionError where an account other than the
// one with the given id has organization, a token's, as Others says: one not
// known has no holder.
func (r Registry) unheld(ctx context.Context, id, organization string) error {
	if organization == "" || r.Others == nil {
		return nil
	}
	others, err := r.Others(ctx, id)
	if err != nil {
		return fmt.Errorf("compare the token with the other accounts': %w", err)
	}
	if same := others.SameSubscription(organization, id); same != nil {
		return same
	}
	return nil
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
// it. It fails with a *RefusedError when the API refuses the token, and with
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
		return Checked{}, &RefusedError{Why: why}
	}
	return Checked{Unchecked: true, Why: why}, nil
}

// read takes the token of the account named from the user: typed at their
// terminal, unseen, once they're asked for it, or piped in.
func (in Input) read(name string) (tokens.Token, error) {
	if in.Hidden == nil {
		return given(tokens.ParseFrom(in.Piped))
	}
	_, _ = fmt.Fprintf(in.Prompt, prompt, name)
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
