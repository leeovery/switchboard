package setup

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/interrupts"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/setuptoken"
	"github.com/leeovery/switchboard/internal/tokens"
)

// TokenMaker makes an account's setup token, as setuptoken.Maker does: its
// Signing's Report hears every step before Make returns.
type TokenMaker interface {
	Make(ctx context.Context, s setuptoken.Signing) (tokens.Token, error)
}

// What making a token fails with, given nothing to make it with or to open
// its sign-in address with, and reopening an address that never opened.
var (
	errNoTokenMaker    = errors.New("make a token: setup has no TokenMaker")
	errNoBrowser       = errors.New("make a token: setup has no Browser to open the sign-in address in")
	errNoPrivateWindow = errors.New("make a token: setup has no PrivateWindow to open the sign-in address in")
	errNeverOpened     = errors.New("reopen the sign-in address: it never opened, so it's there to show")
)

// TokenFor is whose token MakeToken or PasteToken saves, and the accounts
// it's compared with.
type TokenFor struct {
	// Account is the account: its ID, and its Label to name it by, where
	// it's configured, and the token replaces its own; all of it but its ID
	// where Add is set, and it's added with the token, under the id the
	// token's organization decides.
	Account config.NewAccount
	Add     bool
	// Organizations are the organizations of the accounts this run of setup
	// has read, which the token's is compared with.
	Organizations Organizations
}

// Organizations are accounts' organizations, by account id, as accounts
// has them.
type Organizations = accounts.Organizations

// Organizations are the organizations of the review's accounts, as the API
// named them checking each token: those known alone, which a token made or
// pasted is compared with.
func (r Review) Organizations() Organizations {
	o := Organizations{}
	for _, a := range r.Accounts {
		if a.Token.Organization != "" {
			o[a.ID] = a.Token.Organization
		}
	}
	return o
}

// OtherOrganizations are the organizations of every account but the one with
// the given id, which a token made or pasted for it is compared with: each
// other account's token checked with the API, all at once, as ReviewOf
// checks it, and its answer folded into the review as Review.Checked has it,
// so one the API doesn't answer for, or whose check fails, which is logged,
// has none known. It fails as Review does, and with ctx's error once ctx is
// done.
func (s *Setup) OtherOrganizations(ctx context.Context, id string) (Organizations, error) {
	review, err := s.Review()
	if err != nil {
		return nil, err
	}
	var others []string
	for _, a := range review.Accounts {
		if a.ID != id && a.Token.State == TokenRead {
			others = append(others, a.ID)
		}
	}
	rows, errs := make([]Reviewed, len(others)), make([]error, len(others))
	var wg sync.WaitGroup
	for i, other := range others {
		wg.Go(func() { rows[i], errs[i] = s.ReviewOf(ctx, other) })
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for i, other := range others {
		if errs[i] != nil {
			logger.Warn("couldn't check an account's token, so its organization isn't known", "account", other, "error", errs[i])
		}
		review = review.Checked(other, rows[i], errs[i])
	}
	return review.Organizations(), nil
}

// MakingStep is a step of a token's making, as MakeToken reports it.
type MakingStep int

// The steps MakeToken reports, in the order they come.
const (
	// AddressReady is the sign-in address ready, opened as Making says, or
	// to show.
	AddressReady MakingStep = iota + 1
	// AwaitingApproval is setup-token waiting on the sign-in's approval in
	// the browser.
	AwaitingApproval
	// TokenMade is setup-token saying it has made the token.
	TokenMade
	// CheckingToken is the token being checked with the API, then compared
	// with the other accounts' and saved.
	CheckingToken
)

// makingSteps are setuptoken's steps, as MakeToken reports them.
var makingSteps = map[setuptoken.Step]MakingStep{
	setuptoken.StepAddress: AddressReady,
	setuptoken.StepWaiting: AwaitingApproval,
	setuptoken.StepMade:    TokenMade,
}

// Making is where a token's making has got, as MakeToken reports it.
type Making struct {
	Step MakingStep
	// Address is the sign-in address, at AddressReady, to show where it
	// didn't open, and to open again: printed or logged, it's a placeholder.
	Address setuptoken.Address
	// Opened is how Address opened, at AddressReady: in the browser as it
	// is, in a private window of the default browser, or not, Why saying
	// why.
	Opened setuptoken.Opened
	// Browser is the default browser's name, where it's known: the one whose
	// private window opened, or the one that has none setup can open, such
	// as Safari.
	Browser string
	Why     string
	// For is the account whose token it is, and SignedIn the account the
	// browser is signed in to, where Address is to open in a private window,
	// as the opener rule has it, "" where it's to open in the browser as it
	// is: each by its label, else its id, at AddressReady, as Opening says
	// them.
	For, SignedIn string
}

// making is setuptoken's event e, as MakeToken reports it.
func making(e setuptoken.Event) Making {
	m := Making{Step: makingSteps[e.Step], Address: e.Address, Opened: e.Opened, Browser: e.Browser}
	if e.Unopened != nil {
		m.Why = e.Unopened.Error()
	}
	if none, ok := errors.AsType[*setuptoken.NoPrivateWindowError](e.Unopened); ok {
		m.Browser = none.Browser
	}
	return m
}

// TokenResult is how making or pasting an account's token ended.
type TokenResult int

// The ways making or pasting a token ends. Only TokenSaved saves anything.
const (
	// TokenSaved is the token saved, with its year from today, the API
	// having answered for it as Tokened's Checked says.
	TokenSaved TokenResult = iota + 1
	// TokenRefused is the API refusing the token.
	TokenRefused
	// TokenSameAccount is the token being another account's subscription's,
	// as the organization the API names it in says: Tokened's SameAs names
	// the account.
	TokenSameAccount
	// TokenOtherSubscription is the token, replacing an account's, being for
	// another subscription than the account's, as the accounts record has
	// it: Tokened's SameAs names, by its label, the account the record has
	// of the token's, where there's one to name.
	TokenOtherSubscription
	// TokenAsked is the token, to add an account with, whose organization
	// doesn't settle which account it is: Tokened's Asked holds it, for
	// Answer to add the account as the user says.
	TokenAsked
	// TokenNotMade is setup-token giving no token: Tokened's Shown is what it
	// showed.
	TokenNotMade
	// TokenUnread is setup-token saying it made the token, which lasts a
	// year on the account, without showing it whole: Tokened's Shown is what
	// it showed, the token in it, to copy and paste.
	TokenUnread
	// TokenNotSaved is setup-token saying it made the token, which lasts a
	// year on the account, and setup failing to save it, as Tokened's Why
	// says: Shown is the token, where setup read it, to copy and paste.
	TokenNotSaved
	// NoClaudeCode is there being no Claude Code to run setup-token with.
	NoClaudeCode
	// TokenNotGiven is what was pasted being no token.
	TokenNotGiven
	// TokenUnanswered is the API not answering for a pasted token, which is
	// saved only once it's checked, as Why says.
	TokenUnanswered
	// TokenCancelled is the caller's context ending first, and
	// TokenInterrupted an interrupt.
	TokenCancelled
	TokenInterrupted
)

// Tokened is how making or pasting an account's token ended.
type Tokened struct {
	Result TokenResult
	// Checked is what the API answered of the token, where it's saved.
	Checked accounts.Checked
	// Why is why it isn't saved, with nothing of the token or its sign-in
	// address in it: "" where it is, or was cancelled or interrupted.
	Why string
	// SameAs names the account the token's subscription is already, at
	// TokenSameAccount: by its id, where the token's to replace an account's,
	// and by its label where it's to add one, as no id was given. At
	// TokenOtherSubscription, it's the label of the account the record has
	// of the token's organization, "" for none.
	SameAs string
	// ID is the id of the account the token was saved for, at TokenSaved:
	// for one added, the id the token's organization decided.
	ID string
	// Asked is the token held, at TokenAsked, and the question of which
	// account it's for.
	Asked *Asked
	// Shown is what setup-token showed, at TokenNotMade and TokenUnread, a
	// line each, the blank lines left out; at TokenNotSaved, the token, or,
	// where a paste of a token made but unread couldn't be saved, what
	// setup-token showed, the token in it; and at TokenAsked, the token, where
	// it was made, not pasted: for the person alone, never a log, as it can
	// hold the token.
	Shown []string
	// InterruptHeld says an interrupt came while the token made was held
	// through its save, as Setup.Hold has it, for the caller to end once it
	// has ended, as the interrupt asked.
	InterruptHeld bool
}

// MakeToken makes the account's token with claude setup-token, its sign-in
// address opened by the opener rule (see opensAsItIs), and saves it as
// PasteToken saves a pasted one. It tells report each step, in order and one
// at a time, setup-token's on a goroutine of their own (see
// setuptoken.Signing's Report), so report must return without waiting on
// MakeToken: nil hears nothing. An interrupt, taken as setup's Interrupt
// says, or the end of ctx, stops the making until setup-token says it has
// made the token; from then on the token is kept, as it lasts a year on the
// account: setup-token is read within its own time limits, however ctx
// ends, then the token's checked and saved, or, unread or not saved, given
// back to copy, the process's interrupts held throughout, as setup's Hold
// has them, and Tokened's InterruptHeld saying where one came. It makes
// nothing where the token couldn't be saved for the account, and fails as
// PasteToken does, but for a token made.
func (s *Setup) MakeToken(ctx context.Context, f TokenFor, report func(Making)) (tokened Tokened, err error) {
	if s.TokenMaker == nil {
		return Tokened{}, errNoTokenMaker
	}
	cfg, err := s.takes(f)
	if err != nil {
		return Tokened{}, err
	}
	asItIs := opensAsItIs(cfg, f)
	opener, err := s.opener(asItIs)
	if err != nil {
		return Tokened{}, err
	}
	forName, signedIn := named(cfg, f, asItIs)
	tell := func(m Making) {
		if m.Step == AddressReady {
			m.For, m.SignedIn = forName, signedIn
		}
		if report != nil {
			report(m)
		}
	}
	kept := &madeHold{Holder: s.Hold, saving: s.Saving}
	defer func() { tokened.InterruptHeld = kept.Let() }()
	a := s.make(ctx, opener, tell, kept)
	if a.err != nil {
		return notMade(ctx, f, a)
	}
	kept.begin()
	tell(Making{Step: CheckingToken})
	// The prober bounds its own wait on the API, which, not answering, leaves
	// the token saved unchecked.
	saved, err := s.save(context.WithoutCancel(ctx), f, a.token, false)
	if err != nil {
		return notSaved(f, err, a.token), nil
	}
	return saved, nil
}

// notSaved is the token made for f, which lasts a year on the account, not
// saved, as err says, given back to copy.
func notSaved(f TokenFor, err error, token tokens.Token) Tokened {
	why := redact.Text(err.Error(), token.Reveal())
	logger.Warn("claude setup-token made a token switchboard couldn't save", f.Logged("error", why)...)
	return Tokened{Result: TokenNotSaved, Why: why, Shown: []string{token.Reveal()}}
}

// attempt is how a run of setup-token ended: its token, or err saying why
// there's none. made says it said it made the token, and interrupted that
// an interrupt came before it ended.
type attempt struct {
	token             tokens.Token
	err               error
	made, interrupted bool
}

// make makes a token with the TokenMaker, its sign-in address opened by
// opener, telling tell each step, while ctx lasts, and until an interrupt,
// till setup-token says it has made the token: from then on, neither ends
// what the TokenMaker reads under, and kept holds the interrupts.
func (s *Setup) make(ctx context.Context, opener setuptoken.Opener, tell func(Making), kept *madeHold) attempt {
	asked, stop := s.interruptible(ctx)
	defer stop()
	reading, cancel := context.WithCancel(context.WithoutCancel(asked))
	defer cancel()
	var made atomic.Bool
	defer context.AfterFunc(asked, func() {
		if !made.Load() {
			cancel()
		}
	})()
	token, err := s.TokenMaker.Make(reading, setuptoken.Signing{
		Opener: opener,
		Report: func(e setuptoken.Event) {
			if e.Step == setuptoken.StepMade {
				made.Store(true)
				kept.begin()
			}
			tell(making(e))
		},
	})
	return attempt{token: token, err: err, made: made.Load(), interrupted: asked.Err() != nil && ctx.Err() == nil}
}

// madeHold is the process's interrupts kept for a token made, from begin
// till Let, as interrupts.Kept keeps them, the first told to saving, on a
// goroutine of its own, so nothing waits on it.
type madeHold struct {
	interrupts.Kept
	saving func()
}

// begin keeps the interrupts, where they aren't kept already.
func (k *madeHold) begin() {
	k.Keep(k.told)
}

// told tells saving, where it's set, of an interrupt held.
func (k *madeHold) told() {
	if k.saving != nil {
		go k.saving()
	}
}

// screened is setup-token's own failure, which says what it showed, as a
// *setuptoken.ScreenError does.
type screened interface {
	error
	Screen() string
}

var _ screened = (*setuptoken.ScreenError)(nil)

// notMade is how making the token for f ended without one, as a says. A
// token made, but unread, is given back however it ended, and one made that
// failed otherwise says why; otherwise, a failure once ctx has ended, or at
// an interrupt, is theirs.
func notMade(ctx context.Context, f TokenFor, a attempt) (Tokened, error) {
	screen, isScreen := errors.AsType[screened](a.err)
	_, noClaude := errors.AsType[*claude.NotFoundError](a.err)
	switch {
	case isScreen && a.made:
		logger.Warn("claude setup-token made a token switchboard couldn't read", f.Logged("error", a.err)...)
		return Tokened{Result: TokenUnread, Why: a.err.Error(), Shown: shownLines(screen.Screen())}, nil
	case a.made:
		logger.Warn("claude setup-token made a token switchboard couldn't read", f.Logged("error", a.err)...)
		return Tokened{Result: TokenNotSaved, Why: redact.Text(a.err.Error())}, nil
	case ctx.Err() != nil:
		return Tokened{Result: TokenCancelled}, nil
	case a.interrupted:
		return Tokened{Result: TokenInterrupted}, nil
	case isScreen:
		logger.Warn("claude setup-token made no token", f.Logged("error", a.err)...)
		return Tokened{Result: TokenNotMade, Why: a.err.Error(), Shown: shownLines(screen.Screen())}, nil
	case noClaude:
		return Tokened{Result: NoClaudeCode, Why: a.err.Error()}, nil
	}
	return Tokened{}, a.err
}

// PasteToken checks the token pasted, as pastedToken takes it, with the
// API, compares the organization it names it in with the other accounts',
// and saves it, with its year from today, as nothing says when it was made:
// in the account's place, or with the account, added. It saves nothing where
// what's pasted is no token, the API refuses it, or doesn't answer for it,
// as a token's saved only once it's checked, it's another account's, or for
// another subscription than the account's, or ctx ends before it's saved,
// saying why in words of the paste's own; and, to add an account its
// organization doesn't settle, holds it for the user to say which it is,
// as add has it. It fails, asking the API nothing, where the
// token couldn't be saved for the account, not configured, or one the config
// can't add; and with what's no outcome of the check, as where the config
// can't be read or the token written.
func (s *Setup) PasteToken(ctx context.Context, f TokenFor, pasted string) (Tokened, error) {
	if _, err := s.takes(f); err != nil {
		return Tokened{}, err
	}
	token, err := pastedToken(pasted)
	if err != nil {
		return Tokened{Result: TokenNotGiven, Why: err.Error()}, nil
	}
	return s.save(ctx, f, token, true)
}

// save checks token with the API, runs the duplicate check, and saves it for
// f's account, as PasteToken says, where pasted, else as MakeToken does, a
// token made saved unchecked where the API doesn't answer, as setup-token
// made it, but where it's to add an account, as add has it. A save done as
// ctx ends is a save.
func (s *Setup) save(ctx context.Context, f TokenFor, token tokens.Token, pasted bool) (Tokened, error) {
	checked, err := s.Accounts.Check(ctx, token)
	switch {
	case errors.Is(err, accounts.ErrRefused) && pasted:
		return Tokened{Result: TokenRefused, Why: pastedRefused(err)}, nil
	case errors.Is(err, accounts.ErrRefused):
		return Tokened{Result: TokenRefused, Why: err.Error()}, nil
	case cancelledBy(ctx, err):
		return Tokened{Result: TokenCancelled}, nil
	case err != nil:
		return Tokened{}, err
	}
	id := f.Account.ID
	if pasted && checked.Unchecked {
		logger.Info("the API didn't answer for a token pasted, so it isn't saved", f.Logged("error", checked.Why)...)
		return Tokened{Result: TokenUnanswered, Why: pastedUnanswered(checked.Why)}, nil
	}
	if f.Add {
		return s.add(ctx, &Asked{f: f, token: token, checked: checked, pasted: pasted}, accounts.Answer{})
	}
	if same := f.Organizations.SameSubscription(checked.Organization, id); same != nil {
		return Tokened{Result: TokenSameAccount, SameAs: same.Other}, nil
	}
	err = s.Accounts.SetTokenChecked(ctx, id, token, checked)
	if other, ok := errors.AsType[*accounts.OtherSubscriptionError](err); ok {
		return Tokened{Result: TokenOtherSubscription, SameAs: other.OtherLabel}, nil
	}
	return s.saved(ctx, err, Tokened{Result: TokenSaved, Checked: checked, ID: id})
}

// Asked is a token checked, to add an account with, held while the user
// says which account it's for, as Question asks: Answer adds the account
// with it.
type Asked struct {
	Question accounts.Question
	f        TokenFor
	token    tokens.Token
	checked  accounts.Checked
	pasted   bool
}

// Answer adds the account the asked token is for, as answer says which it
// is, saving it as add does: so asked again, where what the accounts record
// holds has changed since it was asked. A save begun is a save, however ctx
// ends, as a made token's is. A token made, which lasts a year on the
// account, that isn't saved, as where it's a configured account's, or can't
// be saved, is given back to copy, as notSaved has it.
func (s *Setup) Answer(ctx context.Context, asked *Asked, answer accounts.Answer) (Tokened, error) {
	ctx = context.WithoutCancel(ctx)
	if asked.pasted {
		return s.add(ctx, asked, answer)
	}
	tokened, err := s.adding(ctx, asked, answer)
	if err != nil {
		return notSaved(asked.f, err, asked.token), nil
	}
	return tokened, nil
}

// add adds the account with the token a holds, which the API answered for as
// a's checked says, compared with the organizations of the accounts f has
// read, as accounts.Registry.AddChecked adds it, under the id the token's
// organization decides, or answer, where that doesn't settle it, saying
// which account it is: TokenSameAccount where it's a configured account's,
// named by its label; TokenAsked where it's the user's to say which it is,
// holding the token, or where answer says an account it can't be, as one
// configured since it was asked, asked afresh; and failing, saving nothing,
// where the API didn't answer for the token, as an account isn't added until
// its organization is known.
func (s *Setup) add(ctx context.Context, a *Asked, answer accounts.Answer) (Tokened, error) {
	tokened, err := s.adding(ctx, a, answer)
	if configured, ok := errors.AsType[*accounts.ConfiguredError](err); ok {
		return Tokened{Result: TokenSameAccount, SameAs: configured.Label}, nil
	}
	return tokened, err
}

// adding is add, but failing with a ConfiguredError where the token's a
// configured account's.
func (s *Setup) adding(ctx context.Context, a *Asked, answer accounts.Answer) (Tokened, error) {
	added, err := s.Accounts.AddChecked(ctx, a.f.Account, a.token, a.checked, a.f.Organizations, answer)
	if question, ok := errors.AsType[*accounts.QuestionError](err); ok {
		return a.asking(question.Question), nil
	}
	if notAChoice, ok := errors.AsType[*accounts.NotAChoiceError](err); ok {
		return a.asking(notAChoice.Question), nil
	}
	return s.saved(ctx, err, Tokened{Result: TokenSaved, Checked: a.checked, ID: added.ID})
}

// asking is the token a holds, asked which account it's for, as question
// asks: shown, where it was made, not pasted, as it lasts a year on the
// account.
func (a *Asked) asking(question accounts.Question) Tokened {
	asked := *a
	asked.Question = question
	t := Tokened{Result: TokenAsked, Asked: &asked}
	if !a.pasted {
		t.Shown = []string{a.token.Reveal()}
	}
	return t
}

// saved is how a token's save ended, as err says: as saved has it, where it
// saved, which a router that answers takes up; cancelled, where err is ctx's
// ending; else failing with err.
func (s *Setup) saved(ctx context.Context, err error, saved Tokened) (Tokened, error) {
	switch {
	case cancelledBy(ctx, err):
		return Tokened{Result: TokenCancelled}, nil
	case err != nil:
		return Tokened{}, err
	}
	s.wrote()
	return saved, nil
}

// cancelledBy reports whether err is ctx's ending.
func cancelledBy(ctx context.Context, err error) bool {
	return ctx.Err() != nil && errors.Is(err, ctx.Err())
}

// takes returns the config, nil where it has no account, failing where a
// token couldn't be saved for f's account: one not configured, or, to add,
// one the config can't add, as accounts.Registry.CheckNew has it.
func (s *Setup) takes(f TokenFor) (*config.Config, error) {
	draft, err := config.Edit(s.Accounts.ConfigPath)
	if err != nil {
		return nil, err
	}
	cfg, id := draft.Config(), f.Account.ID
	switch {
	case f.Add:
		return cfg, s.Accounts.CheckNew(f.Account)
	case cfg == nil || !slices.Contains(cfg.Accounts.IDs(), id):
		return nil, config.NotConfigured(id)
	}
	return cfg, nil
}

// opensAsItIs is the opener rule: it reports whether the sign-in address for
// f's token opens in the browser as it is, cfg configuring the accounts, as
// for the account it's signed in to, the first, and the primary on a run
// again; every other's opens in a private window.
func opensAsItIs(cfg *config.Config, f TokenFor) bool {
	if f.Add {
		return cfg == nil
	}
	return cfg.Accounts.Primary().ID == f.Account.ID
}

// named is the account the token for f is for, as f gives it, or cfg
// configures it; and, where its sign-in address isn't to open in the browser
// as it is, the account the browser is signed in to, the primary, as the
// opener rule has it: each by its label, else its id.
func named(cfg *config.Config, f TokenFor, asItIs bool) (forName, signedIn string) {
	forName = cmp.Or(f.Account.Label, f.Account.ID)
	if cfg == nil {
		return forName, ""
	}
	if i := slices.IndexFunc(cfg.Accounts, func(a config.Account) bool { return a.ID == f.Account.ID }); i >= 0 {
		forName = cmp.Or(cfg.Accounts[i].Label, f.Account.ID)
	}
	if !asItIs {
		primary := cfg.Accounts.Primary()
		signedIn = cmp.Or(primary.Label, primary.ID)
	}
	return forName, signedIn
}

// opener returns what opens a sign-in address: the browser as it is, where
// asItIs, else a private window.
func (s *Setup) opener(asItIs bool) (setuptoken.Opener, error) {
	switch {
	case asItIs && s.Browser == nil:
		return nil, errNoBrowser
	case asItIs:
		return s.Browser, nil
	case s.PrivateWindow == nil:
		return nil, errNoPrivateWindow
	}
	return s.PrivateWindow, nil
}

// Reopen opens the sign-in address at again, at its AddressReady, the way it
// opened first. One that never opened is there to show, and Reopen fails.
func (s *Setup) Reopen(ctx context.Context, at Making) error {
	if at.Opened == setuptoken.NotOpened {
		return errNeverOpened
	}
	opener, err := s.opener(at.Opened == setuptoken.OpenedInBrowser)
	if err != nil {
		return err
	}
	_, err = opener.Open(ctx, at.Address)
	return err
}

// shownLines are the lines of what setup-token showed that hold anything.
func shownLines(screen string) []string {
	var shown []string
	for line := range strings.Lines(screen) {
		if line = strings.TrimRightFunc(line, unicode.IsSpace); line != "" {
			shown = append(shown, line)
		}
	}
	return shown
}
