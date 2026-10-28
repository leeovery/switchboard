package setup

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync/atomic"
	"unicode"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
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
	// Account is the account: its ID alone where it's configured, and the
	// token replaces its own; all of it where Add is set, and it's added with
	// the token.
	Account config.NewAccount
	Add     bool
	// Organizations are the organizations of the accounts this run of setup
	// has read, which the token's is compared with.
	Organizations Organizations
}

// Organizations are accounts' organizations, by account id: "" where one
// isn't known.
type Organizations map[string]string

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
	// TokenNotMade is setup-token giving no token: Tokened's Shown is what it
	// showed.
	TokenNotMade
	// TokenUnread is setup-token saying it made the token, which lasts a
	// year on the account, without showing it whole: Tokened's Shown is what
	// it showed, the token in it, to copy and paste.
	TokenUnread
	// NoClaudeCode is there being no Claude Code to run setup-token with.
	NoClaudeCode
	// TokenNotGiven is what was pasted being no token.
	TokenNotGiven
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
	// SameAs is the id of the account the token's subscription is already,
	// at TokenSameAccount.
	SameAs string
	// Shown is what setup-token showed, at TokenNotMade and TokenUnread, a
	// line each, the blank lines left out: for the person alone, never a
	// log, as at TokenUnread it holds the token.
	Shown []string
}

// MakeToken makes the account's token with claude setup-token, its sign-in
// address opened by the opener rule (see opensAsItIs), and saves it as
// PasteToken saves a pasted one. It tells report each step, in order and one
// at a time, setup-token's on a goroutine of their own (see
// setuptoken.Signing's Report), so report must return without waiting on
// MakeToken: nil hears nothing. An interrupt, taken as setup's Interrupt
// says, or the end of ctx, stops the making until setup-token says it has
// made the token; from then on the token is kept, as it lasts a year on the
// account: it's checked and saved however ctx ends, or, unread, given back
// to copy. It makes nothing where the token couldn't be saved for the
// account, and fails as PasteToken does.
func (s *Setup) MakeToken(ctx context.Context, f TokenFor, report func(Making)) (Tokened, error) {
	if s.TokenMaker == nil {
		return Tokened{}, errNoTokenMaker
	}
	cfg, err := s.takes(f)
	if err != nil {
		return Tokened{}, err
	}
	opener, err := s.opener(opensAsItIs(cfg, f))
	if err != nil {
		return Tokened{}, err
	}
	tell := func(m Making) {
		if report != nil {
			report(m)
		}
	}
	a := s.make(ctx, opener, tell)
	if a.err != nil {
		return notMade(ctx, f, a)
	}
	tell(Making{Step: CheckingToken})
	// The prober bounds its own wait on the API, which, not answering, leaves
	// the token saved unchecked.
	return s.save(context.WithoutCancel(ctx), f, a.token)
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
// opener, telling tell each step, while ctx lasts, and until an interrupt.
func (s *Setup) make(ctx context.Context, opener setuptoken.Opener, tell func(Making)) attempt {
	asked, stop := s.interruptible(ctx)
	defer stop()
	var made atomic.Bool
	token, err := s.TokenMaker.Make(asked, setuptoken.Signing{
		Opener: opener,
		Report: func(e setuptoken.Event) {
			if e.Step == setuptoken.StepMade {
				made.Store(true)
			}
			tell(making(e))
		},
	})
	return attempt{token: token, err: err, made: made.Load(), interrupted: asked.Err() != nil && ctx.Err() == nil}
}

// screened is setup-token's own failure, which says what it showed, as a
// *setuptoken.ScreenError does.
type screened interface {
	error
	Screen() string
}

var _ screened = (*setuptoken.ScreenError)(nil)

// notMade is how making the token for f ended without one, as a says. A
// token made, but unread, is given back however it ended; otherwise, a
// failure once ctx has ended, or at an interrupt, is theirs.
func notMade(ctx context.Context, f TokenFor, a attempt) (Tokened, error) {
	screen, isScreen := errors.AsType[screened](a.err)
	_, noClaude := errors.AsType[*claude.NotFoundError](a.err)
	switch {
	case isScreen && a.made:
		logger.Warn("claude setup-token made a token switchboard couldn't read", "account", f.Account.ID, "error", a.err)
		return Tokened{Result: TokenUnread, Why: a.err.Error(), Shown: shownLines(screen.Screen())}, nil
	case ctx.Err() != nil:
		return Tokened{Result: TokenCancelled}, nil
	case a.interrupted:
		return Tokened{Result: TokenInterrupted}, nil
	case isScreen:
		logger.Warn("claude setup-token made no token", "account", f.Account.ID, "error", a.err)
		return Tokened{Result: TokenNotMade, Why: a.err.Error(), Shown: shownLines(screen.Screen())}, nil
	case noClaude:
		return Tokened{Result: NoClaudeCode, Why: a.err.Error()}, nil
	}
	return Tokened{}, a.err
}

// PasteToken checks the token pasted with the API, compares the organization
// it names it in with the other accounts', and saves it, with its year from
// today, as nothing says when it was made: in the account's place, or with
// the account, added. It saves nothing where the API refuses it, it's
// another account's, or ctx ends before it's saved. It fails, asking the API
// nothing, where the token couldn't be saved for the account, not
// configured, or one the config can't add; and with what's no outcome of the
// check, as where the config can't be read or the token written.
func (s *Setup) PasteToken(ctx context.Context, f TokenFor, pasted string) (Tokened, error) {
	if _, err := s.takes(f); err != nil {
		return Tokened{}, err
	}
	token, err := tokens.Parse(pasted)
	if err != nil {
		return Tokened{Result: TokenNotGiven, Why: err.Error()}, nil
	}
	return s.save(ctx, f, token)
}

// save checks token with the API, runs the duplicate check, and saves it for
// f's account, as PasteToken says. A save done as ctx ends is a save.
func (s *Setup) save(ctx context.Context, f TokenFor, token tokens.Token) (Tokened, error) {
	checked, err := s.Accounts.Check(ctx, token)
	switch {
	case errors.Is(err, accounts.ErrRefused):
		return Tokened{Result: TokenRefused, Why: err.Error()}, nil
	case cancelledBy(ctx, err):
		return Tokened{Result: TokenCancelled}, nil
	case err != nil:
		return Tokened{}, err
	}
	id := f.Account.ID
	if other := f.Organizations.holder(checked.Organization, id); other != "" {
		logger.Info("a token is another account's subscription's, so it isn't saved", "account", id, "other", other)
		return Tokened{Result: TokenSameAccount, SameAs: other}, nil
	}
	if f.Add {
		err = s.Accounts.AddChecked(ctx, f.Account, token, checked)
	} else {
		err = s.Accounts.SetTokenChecked(ctx, id, token, checked)
	}
	switch {
	case cancelledBy(ctx, err):
		return Tokened{Result: TokenCancelled}, nil
	case err != nil:
		return Tokened{}, err
	}
	s.wrote()
	return Tokened{Result: TokenSaved, Checked: checked}, nil
}

// cancelledBy reports whether err is ctx's ending.
func cancelledBy(ctx context.Context, err error) bool {
	return ctx.Err() != nil && errors.Is(err, ctx.Err())
}

// takes returns the config, nil where it has no account, failing where a
// token couldn't be saved for f's account: one not configured, or, to add,
// one the config can't add.
func (s *Setup) takes(f TokenFor) (*config.Config, error) {
	draft, err := config.Edit(s.Accounts.ConfigPath)
	if err != nil {
		return nil, err
	}
	cfg, id := draft.Config(), f.Account.ID
	switch {
	case f.Add:
		return cfg, draft.AddAccount(f.Account)
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
