package accounts

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/tokens"
)

// Question is what's asked where the organization a token's answer named
// doesn't settle which account it's for: whether it's a new account, or one
// of Choices, the accounts the accounts record has, configured no longer,
// that it may be.
type Question struct {
	// Named is set where the answer named an organization, which none of
	// Choices is known to have; else it named none, and any of them may be
	// the token's.
	Named   bool
	Choices []identity.Entry
}

// Answer is the user's answer to a Question: New for a new account, else ID,
// the id of the one of its choices the token is for. The zero Answer is
// none.
type Answer struct {
	New bool
	ID  string
}

// Ask answers a Question, as the user does, failing where they can't be
// asked or don't answer.
type Ask func(Question) (Answer, error)

// Added is an account added: the id it's known by, and Returning, set where
// it's one this Mac has had, back under its id, so its history carries on.
type Added struct {
	ID        string
	Returning bool
}

// QuestionError is what adding an account fails with where the token's
// organization doesn't settle which account it's for, and no Answer says.
type QuestionError struct {
	Question Question
}

// Error says why which account the token is for is the user's to say.
func (e *QuestionError) Error() string {
	if e.Question.Named {
		return "the token's subscription is none this Mac's accounts are known to have, but may be one removed before switchboard made ids, so which it is is yours to say"
	}
	return "the API named no organization for the token, so whether it's a new account or one this Mac had is yours to say"
}

// NotAChoiceError is what adding an account fails with where its Answer
// names an account the token can't be: ID, none of Question's choices.
type NotAChoiceError struct {
	ID       string
	Question Question
}

// Error says the id answered isn't one of the accounts the token may be.
func (e *NotAChoiceError) Error() string {
	return prose.Quoted(e.ID) + " isn't one of the accounts the token may be"
}

// ConfiguredError is what adding an account fails with where the token's
// subscription is a configured account's: ID and Label name it.
type ConfiguredError struct {
	ID, Label string
}

// Error names the account the token's subscription is, configured already.
func (e *ConfiguredError) Error() string {
	return "the token is for " + prose.Quoted(e.Label) + " (" + e.ID + "), configured already, so nothing is saved"
}

// UnansweredError is what adding an account fails with where the API didn't
// answer for its token, as Why says: an account isn't added until its
// organization is known.
type UnansweredError struct {
	Why string
}

// Error says the API didn't answer, so nothing is saved, and to try again.
func (e *UnansweredError) Error() string {
	return "the API didn't answer for the token (" + e.Why + "), so nothing is saved: an account is added only once it has, to know its subscription, so try again"
}

// Add registers an account, with the token the user gives, as AddChecked
// does: the account, its id not yet known, checked first, as CheckNew checks
// it, the token taken and checked with the API at the config's upstream, and
// compared with the other accounts', as Others gives theirs, where the API
// names its organization. Where that organization doesn't settle which
// account it is, ask answers, asked once nothing is held, and the account is
// added as it answers. Nothing is saved where the API refuses the token, or
// doesn't answer for it.
func (r Registry) Add(ctx context.Context, account config.NewAccount, ask Ask) (Added, error) {
	draft, err := config.Edit(r.ConfigPath)
	if err != nil {
		return Added{}, err
	}
	label, err := checkNew(draft.Config(), account)
	if err != nil {
		return Added{}, err
	}
	upstream, err := draft.Upstream()
	if err != nil {
		return Added{}, err
	}
	token, err := r.Input.read(prose.Clean(label))
	if err != nil {
		return Added{}, err
	}
	checked, err := r.check(ctx, upstream, token)
	if err != nil {
		return Added{}, err
	}
	known, err := r.known(ctx, checked)
	if err != nil {
		return Added{}, err
	}
	added, err := r.AddChecked(ctx, account, token, checked, known, Answer{})
	question, asking := errors.AsType[*QuestionError](err)
	if !asking || ask == nil {
		return added, err
	}
	answer, err := ask(question.Question)
	if err != nil {
		return Added{}, err
	}
	return r.AddChecked(ctx, account, token, checked, known, answer)
}

// known are the organizations of the accounts configured, as Others gives
// them, which the token's is compared with: none asked where the API named
// no organization, or there's no Others.
func (r Registry) known(ctx context.Context, checked Checked) (Organizations, error) {
	if checked.Organization == "" || r.Others == nil {
		return nil, nil
	}
	known, err := r.Others(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("compare the token with the other accounts': %w", err)
	}
	return known, nil
}

// AddChecked registers an account, with the token given, which Check has
// checked, as checked says: its [[account]] table in the config file, made
// where there's none, its token file and its place in the accounts record,
// under the id the token's organization decides, the configured accounts'
// organizations known as known has them, else as the record has them:
// identity's Adding decides. A new account is given the id its organization
// derives, or, where none was named, a random one; one the record has comes
// back under its id. It fails, saving nothing: with an UnansweredError where
// the API didn't answer for the token; with a ConfiguredError where it's a
// configured account's; with a QuestionError where the organization doesn't
// settle which it is, and answer doesn't say, or a NotAChoiceError where it
// says one it can't be; as CheckNew does for the account; and once ctx is
// done. It doesn't ask the API. checked must be what Check gave for this
// token: nothing ties the two.
//
// The token is written first, then the record, then the config, all under
// the record's lock. Where the record or the config can't be written, the
// token written is removed, and the record put back as it was, as
// identity.File's Change puts it back; should that fail, the record's left
// ahead of the config, which a later add or remove puts right, as an account
// the record has and the config hasn't comes back when it's added again.
func (r Registry) AddChecked(ctx context.Context, account config.NewAccount, token tokens.Token, checked Checked, known Organizations, answer Answer) (Added, error) {
	if checked.Unchecked {
		return Added{}, &UnansweredError{Why: checked.Why}
	}
	record, err := r.Record.Read()
	if err != nil {
		return Added{}, err
	}
	// Decided first as the files stand, so a refusal changes nothing, not
	// even the record's lock; then again under it, so what's decided holds.
	if _, _, err := r.decided(record, account, known, checked.Organization, answer); err != nil {
		return Added{}, err
	}
	var added Added
	err = r.Record.Change(func(record *identity.Record, commit func() error) error {
		draft, d, err := r.decided(*record, account, known, checked.Organization, answer)
		if err != nil {
			return err
		}
		if d.random {
			if d.ID, err = record.Random(r.random(), configuredIn(draft.Config()).IDs()); err != nil {
				return err
			}
		}
		account.ID = d.ID
		if err := draft.AddAccount(account); err != nil {
			return err
		}
		if err := r.write(ctx, account.ID, token, checked); err != nil {
			return err
		}
		if d.Returning {
			record.Return(account.ID, checked.Organization)
		} else {
			record.Add(account.ID, checked.Organization, r.now())
		}
		if err := saved(commit, draft.Save); err != nil {
			return errors.Join(err, r.unwrite(account.ID))
		}
		added = d.Added
		return nil
	})
	if err != nil {
		return Added{}, err
	}
	logger.Info("added an account", "account", account.ID, "primary", account.Primary, "returning", added.Returning)
	return added, nil
}

// saved writes the record, with commit, then the config, with save, which
// isn't saved where the record isn't written.
func saved(commit, save func() error) error {
	if err := commit(); err != nil {
		return err
	}
	return save()
}

// unwrite removes the token file written for the account with the given id,
// which the config didn't take.
func (r Registry) unwrite(id string) error {
	if _, err := r.Tokens.Remove(id); err != nil {
		return fmt.Errorf("remove the token written for %s: %w", id, err)
	}
	return nil
}

// decision is the account a token is added as, as decide has it: random
// where it's a new account under a random id, which is drawn as it's added.
type decision struct {
	Added
	random bool
}

// decided is the config file, as Edit reads it, and the account a token
// whose organization is organization is added as, as decide has it, the
// account checked first as CheckNew checks it.
func (r Registry) decided(record identity.Record, account config.NewAccount, known Organizations, organization string, answer Answer) (*config.Draft, decision, error) {
	draft, err := config.Edit(r.ConfigPath)
	if err != nil {
		return nil, decision{}, err
	}
	if _, err := checkNew(draft.Config(), account); err != nil {
		return nil, decision{}, err
	}
	d, err := decide(record, configuredIn(draft.Config()), known, organization, answer)
	return draft, d, err
}

// decide is the account a token whose organization is organization, "" where
// its answer named none, is added as, the accounts configured having the
// organizations known gives them, as identity's Adding decides it with
// record, and as answer says where that doesn't settle it, failing as
// AddChecked says.
func decide(record identity.Record, configured config.Accounts, known Organizations, organization string, answer Answer) (decision, error) {
	organizations := make(map[string]string, len(configured))
	for _, a := range configured {
		organizations[a.ID] = known[a.ID]
	}
	adding := record.Adding(organizations, organization)
	switch adding.Outcome {
	case identity.Configured:
		return decision{}, configuredError(configured, adding.ID)
	case identity.Returning:
		return decision{ID: adding.ID, Returning: true}, nil
	}
	question := Question{Named: organization != "", Choices: adding.Candidates}
	switch {
	case answer.New || answer.ID == "" && len(question.Choices) == 0:
		return decision{ID: adding.ID, random: adding.Outcome == identity.OrganizationUnknown}, nil
	case answer.ID == "":
		return decision{}, &QuestionError{Question: question}
	}
	i := slices.IndexFunc(question.Choices, func(e identity.Entry) bool { return strings.EqualFold(e.ID, answer.ID) })
	if i < 0 {
		return decision{}, &NotAChoiceError{ID: answer.ID, Question: question}
	}
	return decision{ID: question.Choices[i].ID, Returning: true}, nil
}

// configuredIn are the accounts cfg configures: none where it's nil, as a
// config file without accounts reads.
func configuredIn(cfg *config.Config) config.Accounts {
	if cfg == nil {
		return nil
	}
	return cfg.Accounts
}

// random is what a random id is drawn from: the registry's, else
// crypto/rand's.
func (r Registry) random() io.Reader {
	if r.Random == nil {
		return rand.Reader
	}
	return r.Random
}

// configuredError is the ConfiguredError of the account among configured
// whose id is id, in any case, named by its label.
func configuredError(configured config.Accounts, id string) *ConfiguredError {
	label := id
	if i := slices.IndexFunc(configured, func(a config.Account) bool { return strings.EqualFold(a.ID, id) }); i >= 0 {
		id, label = configured[i].ID, configured[i].Label
	}
	return &ConfiguredError{ID: id, Label: label}
}
