package setup_test

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/setuptoken"
	"github.com/leeovery/switchboard/internal/tokens"
)

// signIn is the sign-in address the fake setup-token hands over, and
// madeToken the token it makes, neither shaped as a token is, so that
// nothing redacts them by their shape alone.
const (
	signIn    = "https://claude.ai/oauth/authorize?state=test-state-personal"
	madeToken = "test-token-personal"
)

// fakeMaker is setup-token, as a TokenMaker. It has the Opener it's given
// open the sign-in address, reporting it, then reports its wait on the
// approval, then gives what it's set to: madeToken, or token where that's
// set, reporting it made; or it fails with err, before it says it made the
// token, or after, where madeFirst is set. Blocking, it waits till its
// context ends before it says it made the token, and fails with the cancel;
// holding, once it has said so, it waits till held is closed, noting in
// ended whether its context ends within endsWithin, then gives what it's
// set to, as setup-token reads on for a token made. It reports on a
// goroutine of its own, which it waits for before it returns, as setup-token
// does, and counts the times it's asked.
type fakeMaker struct {
	token     string
	err       error
	madeFirst bool
	block     bool
	held      chan struct{}
	ended     bool
	asked     int
}

func (m *fakeMaker) Make(ctx context.Context, s setuptoken.Signing) (tokens.Token, error) {
	m.asked++
	reports := make(chan setuptoken.Event, 3)
	var told sync.WaitGroup
	told.Go(func() {
		for e := range reports {
			s.Report(e)
		}
	})
	defer told.Wait()
	defer close(reports)
	address, _ := setuptoken.ParseAddress(signIn)
	opening, err := s.Opener.Open(ctx, address)
	reports <- setuptoken.Event{Step: setuptoken.StepAddress, Address: address, Opening: opening, Unopened: err}
	reports <- setuptoken.Event{Step: setuptoken.StepWaiting}
	switch {
	case m.block:
		<-ctx.Done()
		return tokens.Token{}, fmt.Errorf("make a setup token: %w", context.Cause(ctx))
	case m.err != nil && !m.madeFirst:
		return tokens.Token{}, m.err
	}
	reports <- setuptoken.Event{Step: setuptoken.StepMade}
	if m.held != nil {
		<-m.held
		m.ended = endsSoon(ctx)
	}
	if m.err != nil {
		return tokens.Token{}, m.err
	}
	return tokens.Parse(cmp.Or(m.token, madeToken))
}

// endsWithin is how long a test waits on a context it holds to end, where it
// wrongly would: a cancel can reach it on a goroutine of its own, as
// context.AfterFunc runs one, a moment after what cancelled it returns.
const endsWithin = 500 * time.Millisecond

// endsSoon reports whether ctx ends within endsWithin.
func endsSoon(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	case <-time.After(endsWithin):
		return false
	}
}

// fakeOpener opens nothing, noting each address it's asked to open, and
// answering as it's set to.
type fakeOpener struct {
	opening setuptoken.Opening
	err     error
	asked   []string
}

func (o *fakeOpener) Open(_ context.Context, address setuptoken.Address) (setuptoken.Opening, error) {
	o.asked = append(o.asked, address.Reveal())
	return o.opening, o.err
}

// screenFailure is setup-token's own failure, as a *setuptoken.ScreenError
// is, showing screen.
type screenFailure struct{ screen string }

func (screenFailure) Error() string { return "setup-token showed an error" }

func (e screenFailure) Screen() string { return e.screen }

// tokenRig is setup in a world, making tokens with maker, and opening
// sign-in addresses with browser, as the default browser opens them as it
// is, and private, in a private Firefox window.
type tokenRig struct {
	s                *setup.Setup
	maker            *fakeMaker
	browser, private *fakeOpener
}

func (w *world) tokenRig(t *testing.T, maker *fakeMaker) tokenRig {
	t.Helper()
	r := tokenRig{
		s:       w.setup(t, &user{}),
		maker:   maker,
		browser: &fakeOpener{opening: setuptoken.Opening{Opened: setuptoken.OpenedInBrowser}},
		private: &fakeOpener{opening: setuptoken.Opening{Opened: setuptoken.OpenedPrivately, Browser: "Firefox"}},
	}
	r.s.TokenMaker, r.s.Browser, r.s.PrivateWindow = maker, r.browser, r.private
	return r
}

// heard notes each step MakeToken reports.
type heard struct{ steps []setup.Making }

func (h *heard) report(m setup.Making) {
	h.steps = append(h.steps, m)
}

// address returns the step that has the sign-in address ready.
func (h *heard) address(t *testing.T) setup.Making {
	t.Helper()
	i := slices.IndexFunc(h.steps, func(m setup.Making) bool { return m.Step == setup.AddressReady })
	if i < 0 {
		t.Fatalf("MakeToken reported %+v, want the address ready", h.steps)
	}
	return h.steps[i]
}

// adding is personal, added with its token, and replacing the account with
// the given id, configured, its token replaced.
var adding = setup.TokenFor{Account: config.NewAccount{ID: "personal", Label: "Personal", Plan: "max5x"}, Add: true}

func replacing(id string) setup.TokenFor {
	return setup.TokenFor{Account: config.NewAccount{ID: id}}
}

// checkTokened checks the outcome is want.
func checkTokened(t *testing.T, got setup.Tokened, err error, want setup.Tokened) {
	t.Helper()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("the token's outcome = %+v, %v; want %+v", got, err, want)
	}
}

// checkTakenToday checks the account's token file holds want, taken today,
// by setup's clock, so it lasts a year from today.
func (w *world) checkTakenToday(t *testing.T, id, want string) {
	t.Helper()
	w.checkToken(t, id, want)
	token, _ := tokens.Parse(want)
	if taken, err := w.tokens().Taken(id, token); err != nil || !taken.Equal(tokenTaken) {
		t.Errorf("%s's token was taken %v (%v), want %v", id, taken, err, tokenTaken)
	}
}

func TestMakeTokenOpensTheSignInAddressByTheOpenerRule(t *testing.T) {
	noPrivate := &setuptoken.NoPrivateWindowError{Browser: "Safari"}
	tests := []struct {
		name     string
		runAgain bool
		f        setup.TokenFor
		// private is the private window's opener, where it's not one that
		// opens Firefox's.
		private *fakeOpener
		// asItIs says the browser as it is opens the address, rather than a
		// private window.
		asItIs bool
		want   setup.Making
	}{
		{
			name:   "the first account, on a first run: in the browser as it is",
			f:      setup.TokenFor{Account: config.NewAccount{ID: "work", Label: "Work"}, Add: true},
			asItIs: true,
			want:   setup.Making{Step: setup.AddressReady, Opened: setuptoken.OpenedInBrowser, For: "Work"},
		},
		{
			name:     "the primary, on a run again: in the browser as it is, named by its label",
			runAgain: true,
			f:        replacing("work"),
			asItIs:   true,
			want:     setup.Making{Step: setup.AddressReady, Opened: setuptoken.OpenedInBrowser, For: "Work"},
		},
		{
			name:     "another account: in a private window, the browser named, and the primary it's signed in to",
			runAgain: true,
			f:        replacing("side"),
			want:     setup.Making{Step: setup.AddressReady, Opened: setuptoken.OpenedPrivately, Browser: "Firefox", For: "Side", SignedIn: "Work"},
		},
		{
			name:     "an account added once there's one: in a private window",
			runAgain: true,
			f:        adding,
			want:     setup.Making{Step: setup.AddressReady, Opened: setuptoken.OpenedPrivately, Browser: "Firefox", For: "Personal", SignedIn: "Work"},
		},
		{
			name:     "an account added with no label: named by its id",
			runAgain: true,
			f:        setup.TokenFor{Account: config.NewAccount{ID: "team"}, Add: true},
			want:     setup.Making{Step: setup.AddressReady, Opened: setuptoken.OpenedPrivately, Browser: "Firefox", For: "team", SignedIn: "Work"},
		},
		{
			name:     "another account, the browser opening no private window: the address to show, saying why",
			runAgain: true,
			f:        replacing("side"),
			private:  &fakeOpener{err: noPrivate},
			want:     setup.Making{Step: setup.AddressReady, Opened: setuptoken.NotOpened, Browser: "Safari", Why: noPrivate.Error(), For: "Side", SignedIn: "Work"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			if tt.runAgain {
				w.done(t)
			}
			r := w.tokenRig(t, &fakeMaker{})
			if tt.private != nil {
				r.private = tt.private
				r.s.PrivateWindow = tt.private
			}
			var h heard

			got, err := r.s.MakeToken(t.Context(), tt.f, h.report)

			if err != nil || got.Result != setup.TokenSaved {
				t.Fatalf("MakeToken() = %+v, %v; want the token saved", got, err)
			}
			opened, unasked := r.private, r.browser
			if tt.asItIs {
				opened, unasked = r.browser, r.private
			}
			if !slices.Equal(opened.asked, []string{signIn}) || len(unasked.asked) > 0 {
				t.Errorf("the browser as it is opened %q, the private window %q; want the address opened as it is: %t", r.browser.asked, r.private.asked, tt.asItIs)
			}
			at := h.address(t)
			if at.Address.Reveal() != signIn {
				t.Errorf("the address ready is %q, want %q", at.Address.Reveal(), signIn)
			}
			at.Address = setuptoken.Address{}
			if at != tt.want {
				t.Errorf("the address ready was told as %+v, want %+v", at, tt.want)
			}
		})
	}
}

func TestMakeTokenReportsEachStepInTurn(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	r := w.tokenRig(t, &fakeMaker{})
	var h heard

	got, err := r.s.MakeToken(t.Context(), replacing("side"), h.report)

	checkTokened(t, got, err, setup.Tokened{Result: setup.TokenSaved})
	var steps []setup.MakingStep
	for _, m := range h.steps {
		steps = append(steps, m.Step)
	}
	if want := []setup.MakingStep{setup.AddressReady, setup.AwaitingApproval, setup.TokenMade, setup.CheckingToken}; !slices.Equal(steps, want) {
		t.Errorf("MakeToken reported %v, want %v", steps, want)
	}
}

func TestMakeTokenSavesTheTokenWithItsYear(t *testing.T) {
	down := errors.New("dial tcp: no route to the API for " + madeToken)
	tests := []struct {
		name string
		f    setup.TokenFor
		api  fakeAPI
		want accounts.Checked
	}{
		{
			name: "an account added, the token checked, its organization named",
			f:    adding,
			api:  fakeAPI{organizations: map[string]string{madeToken: "org-personal"}},
			want: accounts.Checked{Organization: "org-personal"},
		},
		{
			name: "an account added, the API not answering: unchecked, why said without the token",
			f:    adding,
			api:  fakeAPI{down: down},
			want: accounts.Checked{Unchecked: true, Why: "dial tcp: no route to the API for " + redact.Placeholder},
		},
		{
			name: "a token replaced, checked",
			f:    replacing("side"),
			want: accounts.Checked{},
		},
		{
			name: "a token replaced, the API not answering: unchecked",
			f:    replacing("side"),
			api:  fakeAPI{down: down},
			want: accounts.Checked{Unchecked: true, Why: "dial tcp: no route to the API for " + redact.Placeholder},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			*w.api = tt.api
			r := w.tokenRig(t, &fakeMaker{})

			got, err := r.s.MakeToken(t.Context(), tt.f, nil)

			checkTokened(t, got, err, setup.Tokened{Result: setup.TokenSaved, Checked: tt.want})
			w.checkTakenToday(t, tt.f.Account.ID, madeToken)
			if tt.f.Add && !strings.Contains(w.readConfig(t), `id    = "personal"`) {
				t.Errorf("the config reads\n%s\nwant personal added", w.readConfig(t))
			}
		})
	}
}

func TestMakeTokenSavesNothingWhereItFails(t *testing.T) {
	switchboard := claudetest.Program(t, filepath.Join(t.TempDir(), "switchboard"))
	_, noClaude := claude.Find(t.TempDir(), nil, func() (string, error) { return switchboard, nil })
	if _, ok := errors.AsType[*claude.NotFoundError](noClaude); !ok {
		t.Fatalf("claude.Find() error = %v, want no claude found", noClaude)
	}
	tests := []struct {
		name  string
		maker *fakeMaker
		// organizations are the other accounts' setup has read, and api the
		// organizations the API names tokens in.
		organizations setup.Organizations
		api           map[string]string
		// asked says the API is asked about the token.
		asked bool
		want  setup.Tokened
	}{
		{
			name:  "refused by the API",
			maker: &fakeMaker{token: refusedToken},
			asked: true,
			want:  setup.Tokened{Result: setup.TokenRefused, Why: "the API refused the token (HTTP 401 · Invalid bearer token), so nothing is saved: make another with claude setup-token, run while signed in to that subscription"},
		},
		{
			name:          "the same subscription as another account, which it names",
			maker:         &fakeMaker{},
			organizations: setup.Organizations{"work": "org-work", "side": "org-side"},
			api:           map[string]string{madeToken: "org-side"},
			asked:         true,
			want:          setup.Tokened{Result: setup.TokenSameAccount, SameAs: "side"},
		},
		{
			name:  "the same subscription as several other accounts: the first by id",
			maker: &fakeMaker{},
			organizations: setup.Organizations{
				"work": "org-work", "side": "org-work", "team-1": "org-work", "team-2": "org-work",
				"team-3": "org-work", "team-4": "org-work", "team-5": "org-work", "team-6": "org-work",
			},
			api:   map[string]string{madeToken: "org-work"},
			asked: true,
			want:  setup.Tokened{Result: setup.TokenSameAccount, SameAs: "side"},
		},
		{
			name:  "setup-token failing, with what it showed but its blank lines",
			maker: &fakeMaker{err: screenFailure{screen: "Claude Code\n\nOAuth error: Request failed with status code 400   \n\n  Press Enter to retry.\n\n"}},
			want: setup.Tokened{
				Result: setup.TokenNotMade,
				Why:    "setup-token showed an error",
				Shown:  []string{"Claude Code", "OAuth error: Request failed with status code 400", "  Press Enter to retry."},
			},
		},
		{
			name:  "no Claude Code",
			maker: &fakeMaker{err: fmt.Errorf("make it: %w", noClaude)},
			want:  setup.Tokened{Result: setup.NoClaudeCode, Why: "make it: " + noClaude.Error()},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			w.api.organizations = tt.api
			r := w.tokenRig(t, tt.maker)
			before := w.snapshot(t)
			f := adding
			f.Organizations = tt.organizations

			got, err := r.s.MakeToken(t.Context(), f, nil)

			checkTokened(t, got, err, tt.want)
			w.checkUnchanged(t, before)
			if asked := len(w.api.asked) > 0; asked != tt.asked {
				t.Errorf("the API was asked about %q, want it asked: %t", w.api.asked, tt.asked)
			}
		})
	}
}

func TestMakeTokenComparesOnlyOrganizationsKnown(t *testing.T) {
	tests := []struct {
		name          string
		f             setup.TokenFor
		organizations setup.Organizations
		api           map[string]string
	}{
		{name: "the token's organization unknown", f: adding, organizations: setup.Organizations{"work": "org-work", "side": "org-side"}},
		{name: "the other accounts' unknown", f: adding, organizations: setup.Organizations{"work": "", "side": ""}, api: map[string]string{madeToken: "org-work"}},
		{name: "no other account read", f: adding, api: map[string]string{madeToken: "org-work"}},
		{name: "the account's own, its token replaced", f: replacing("work"), organizations: setup.Organizations{"work": "org-work", "side": "org-side"}, api: map[string]string{madeToken: "org-work"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			w.api.organizations = tt.api
			r := w.tokenRig(t, &fakeMaker{})
			f := tt.f
			f.Organizations = tt.organizations

			got, err := r.s.MakeToken(t.Context(), f, nil)

			checkTokened(t, got, err, setup.Tokened{Result: setup.TokenSaved, Checked: accounts.Checked{Organization: tt.api[madeToken]}})
			w.checkToken(t, f.Account.ID, madeToken)
		})
	}
}

// stops are the ways the making is stopped: the cancel, and the world's
// interrupt.
var stops = []struct {
	name string
	stop func(w *world, cancel context.CancelFunc)
	want setup.TokenResult
}{
	{name: "cancelled", stop: func(_ *world, cancel context.CancelFunc) { cancel() }, want: setup.TokenCancelled},
	{name: "interrupted", stop: func(w *world, _ context.CancelFunc) { w.interrupt() }, want: setup.TokenInterrupted},
}

func TestMakeTokenStoppedBeforeItsMadeSavesNothing(t *testing.T) {
	for _, tt := range stops {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			r := w.tokenRig(t, &fakeMaker{block: true})
			before := w.snapshot(t)
			report := func(m setup.Making) {
				if m.Step == setup.AwaitingApproval {
					tt.stop(w, cancel)
				}
			}

			got, err := r.s.MakeToken(ctx, adding, report)

			checkTokened(t, got, err, setup.Tokened{Result: tt.want})
			w.checkUnchanged(t, before)
			if len(w.api.asked) > 0 {
				t.Errorf("the API was asked about %q, want nothing asked", w.api.asked)
			}
		})
	}
}

// Once setup-token says it has made the token, which lasts a year on the
// account, the token is kept however the making's stopped, setup-token read
// on under a context the stop never ends: checked and saved where it shows
// the token whole, and given back to copy where it doesn't.
func TestMakeTokenKeepsATokenMadeThroughAStop(t *testing.T) {
	unread := screenFailure{screen: "Your OAuth token (valid for 1 year):\n\n" + madeToken + "\n"}
	wantUnread := setup.Tokened{
		Result: setup.TokenUnread,
		Why:    "setup-token showed an error",
		Shown:  []string{"Your OAuth token (valid for 1 year):", madeToken},
	}
	for _, stop := range stops {
		tests := []struct {
			name  string
			maker *fakeMaker
			want  setup.Tokened
		}{
			{name: "shown whole: saved", maker: &fakeMaker{held: make(chan struct{})}, want: setup.Tokened{Result: setup.TokenSaved}},
			{name: "unread: given back", maker: &fakeMaker{held: make(chan struct{}), madeFirst: true, err: unread}, want: wantUnread},
		}
		for _, tt := range tests {
			t.Run(stop.name+" once it's made, "+tt.name, func(t *testing.T) {
				w := newWorld(t)
				w.done(t)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				r := w.tokenRig(t, tt.maker)
				before := w.snapshot(t)
				report := func(m setup.Making) {
					if m.Step == setup.TokenMade {
						stop.stop(w, cancel)
						close(tt.maker.held)
					}
				}

				got, err := r.s.MakeToken(ctx, adding, report)

				checkTokened(t, got, err, tt.want)
				if tt.maker.ended {
					t.Error("the stop ended what setup-token was read under, want it read on within its own time limits")
				}
				if tt.want.Result == setup.TokenSaved {
					w.checkTakenToday(t, "personal", madeToken)
					return
				}
				w.checkUnchanged(t, before)
			})
		}
	}
	t.Run("unread, unstopped: given back", func(t *testing.T) {
		w := newWorld(t)
		w.done(t)
		r := w.tokenRig(t, &fakeMaker{madeFirst: true, err: unread})

		got, err := r.s.MakeToken(t.Context(), adding, nil)

		checkTokened(t, got, err, wantUnread)
	})
	t.Run("cancelled as the API is asked: saved", func(t *testing.T) {
		w := newWorld(t)
		w.done(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		w.api.probing = cancel
		r := w.tokenRig(t, &fakeMaker{})

		got, err := r.s.MakeToken(ctx, adding, nil)

		checkTokened(t, got, err, setup.Tokened{Result: setup.TokenSaved})
		w.checkTakenToday(t, "personal", madeToken)
	})
}

// From the moment setup-token says it has made the token till it's saved,
// or given back, the process's interrupts are held, and let go however the
// making ends; before then, they're left alone, as an interrupt then stops
// setup-token.
func TestMakeTokenHoldsTheInterruptsFromTheTokenMadeThroughItsSave(t *testing.T) {
	unread := screenFailure{screen: "Your OAuth token (valid for 1 year):\n\n" + madeToken + "\n"}
	before := map[setup.MakingStep]bool{setup.AddressReady: false, setup.AwaitingApproval: false}
	made := map[setup.MakingStep]bool{setup.AddressReady: false, setup.AwaitingApproval: false, setup.TokenMade: true}
	saving := map[setup.MakingStep]bool{setup.AddressReady: false, setup.AwaitingApproval: false, setup.TokenMade: true, setup.CheckingToken: true}
	tests := []struct {
		name  string
		maker *fakeMaker
		// heldAt is whether they're held as each step is told, and holds how
		// many times they're held.
		heldAt map[setup.MakingStep]bool
		holds  int
	}{
		{name: "saved", maker: &fakeMaker{}, heldAt: saving, holds: 1},
		{name: "refused", maker: &fakeMaker{token: refusedToken}, heldAt: saving, holds: 1},
		{name: "unread", maker: &fakeMaker{madeFirst: true, err: unread}, heldAt: made, holds: 1},
		{name: "not made", maker: &fakeMaker{err: screenFailure{}}, heldAt: before},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			r := w.tokenRig(t, tt.maker)
			heldAt := map[setup.MakingStep]bool{}
			report := func(m setup.Making) { heldAt[m.Step], _ = w.interrupts.held() }
			var heldAsked bool
			w.api.probing = func() { heldAsked, _ = w.interrupts.held() }

			if _, err := r.s.MakeToken(t.Context(), adding, report); err != nil {
				t.Fatalf("MakeToken() error = %v", err)
			}

			if !maps.Equal(heldAt, tt.heldAt) {
				t.Errorf("the interrupts held as each step was told: %v, want %v", heldAt, tt.heldAt)
			}
			if asked := len(w.api.asked) > 0; asked && !heldAsked {
				t.Error("the interrupts weren't held as the API was asked, want them held through the save")
			}
			if held, holds := w.interrupts.held(); held || holds != tt.holds {
				t.Errorf("the interrupts held %d times, held still: %t; want held %d times, then let go", holds, held, tt.holds)
			}
		})
	}
}

// An interrupt as a token made is saved, as a second Ctrl-C once setup's
// screen has gone, is told to Saving, the first alone, and the token's saved
// all the same, the outcome saying one was held; a SIGPIPE, which isn't one,
// tells nothing, and nor does one as the interrupts are let go, the save
// done, though the outcome says it was held.
func TestAnInterruptAsATokenMadeIsSavedWaitsForTheSave(t *testing.T) {
	tests := []struct {
		name string
		// asked are sent as the API is asked, and atRelease as the
		// interrupts are let go; told is how many times Saving is told, and
		// held whether the outcome says one was held.
		asked     []os.Signal
		atRelease os.Signal
		told      int
		held      bool
	}{
		{name: "an interrupt, then another", asked: []os.Signal{os.Interrupt, syscall.SIGTERM}, told: 1, held: true},
		{name: "a SIGPIPE", asked: []os.Signal{syscall.SIGPIPE}},
		{name: "a SIGHUP as they're let go", atRelease: syscall.SIGHUP, held: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			r := w.tokenRig(t, &fakeMaker{})
			saving := make(chan struct{}, 2)
			r.s.Saving = func() { saving <- struct{}{} }
			w.interrupts.atRelease = tt.atRelease
			told := 0
			w.api.probing = func() {
				for _, sig := range tt.asked {
					w.interrupts.send(sig)
				}
				if tt.told == 0 {
					return
				}
				select {
				case <-saving:
					told++
				case <-time.After(time.Minute):
					t.Error("a minute on, Saving hadn't been told of the interrupt")
				}
			}

			got, err := r.s.MakeToken(t.Context(), adding, nil)

			checkTokened(t, got, err, setup.Tokened{Result: setup.TokenSaved, InterruptHeld: tt.held})
			w.checkTakenToday(t, "personal", madeToken)
			if told += len(saving); told != tt.told {
				t.Errorf("Saving was told %d times, want %d", told, tt.told)
			}
		})
	}
}

// Saving is told on a goroutine of its own, so MakeToken returns while a
// Saving that takes its time runs on.
func TestMakeTokenNeverWaitsOnSaving(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	r := w.tokenRig(t, &fakeMaker{})
	told, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	r.s.Saving = func() {
		close(told)
		<-release
	}
	w.api.probing = func() {
		w.interrupts.send(os.Interrupt)
		<-told
	}
	type made struct {
		tokened setup.Tokened
		err     error
	}
	returned := make(chan made, 1)
	go func() {
		got, err := r.s.MakeToken(t.Context(), adding, nil)
		returned <- made{got, err}
	}()
	select {
	case got := <-returned:
		checkTokened(t, got.tokened, got.err, setup.Tokened{Result: setup.TokenSaved, InterruptHeld: true})
	case <-time.After(time.Minute):
		t.Fatal("a minute on, MakeToken waited on Saving still, want it returned")
	}
}

// A token made that setup can't save, which lasts a year on the account, is
// given back to copy, why with it, never logged: where the config can't be
// read to check it, where the account it's added for was added meanwhile, or
// where its token file can't be written; and where setup-token fails
// otherwise once it's made, why alone, as setup never read the token.
func TestMakeTokenGivesBackATokenMadeItCantSave(t *testing.T) {
	tests := []struct {
		name  string
		f     setup.TokenFor
		maker *fakeMaker
		// atMade breaks what the save needs as the token's made; why is in
		// the outcome's Why, and shown is what it gives back.
		atMade func(t *testing.T, w *world)
		why    string
		shown  []string
	}{
		{
			name: "the config unreadable", f: replacing("side"), maker: &fakeMaker{},
			atMade: func(t *testing.T, w *world) { w.writeConfig(t, "[[account]\n") },
			why:    "config", shown: []string{madeToken},
		},
		{
			name: "the account added meanwhile", f: adding, maker: &fakeMaker{},
			atMade: func(t *testing.T, w *world) {
				w.writeConfig(t, doneConfig+"\n[[account]]\nid = \"personal\"\n")
			},
			why: "personal", shown: []string{madeToken},
		},
		{
			name: "its token file unwritable", f: replacing("side"), maker: &fakeMaker{},
			atMade: func(t *testing.T, w *world) {
				w.removeToken(t, "side")
				mkdir(t, filepath.Join(w.tokens().Path("side"), "kept"))
			},
			why: "side", shown: []string{madeToken},
		},
		{
			name: "setup-token failing otherwise once it's made", f: replacing("side"),
			maker:  &fakeMaker{madeFirst: true, err: errors.New("read setup-token's terminal: input/output error")},
			atMade: func(*testing.T, *world) {},
			why:    "input/output error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logged bytes.Buffer
			logs.Init(logs.Options{Level: slog.LevelDebug, Mirror: &logged})
			defer logs.Close(0)
			w := newWorld(t)
			w.done(t)
			r := w.tokenRig(t, tt.maker)
			report := func(m setup.Making) {
				if m.Step == setup.TokenMade {
					tt.atMade(t, w)
				}
			}

			got, err := r.s.MakeToken(t.Context(), tt.f, report)

			if err != nil || got.Result != setup.TokenNotSaved || !strings.Contains(got.Why, tt.why) || !slices.Equal(got.Shown, tt.shown) {
				t.Errorf("MakeToken() = %v, %v, why %q, giving back the token made: %t; want it not saved, why naming %q, giving back the token made: %t",
					got.Result, err, got.Why, slices.Contains(got.Shown, madeToken), tt.why, tt.shown != nil)
			}
			if strings.Contains(got.Why, madeToken) || strings.Contains(logged.String(), madeToken) {
				t.Error("the token made is in why it isn't saved, or the log, want it given back alone")
			}
		})
	}
}

// A cancel that lands once the save has passed its own look at the context
// saves all the same, so the outcome says it's saved.
func TestPasteTokenCancelledAsItSavesIsSaved(t *testing.T) {
	const pasted = "test-token-pasted"
	for _, f := range []setup.TokenFor{adding, replacing("side")} {
		t.Run(f.Account.ID, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			r := w.tokenRig(t, &fakeMaker{})
			r.s.Accounts.Now = func() time.Time {
				cancel()
				return tokenTaken
			}

			got, err := r.s.PasteToken(ctx, f, pasted)

			checkTokened(t, got, err, setup.Tokened{Result: setup.TokenSaved})
			if ctx.Err() == nil {
				t.Fatal("the save took no cancel, want one as it saves")
			}
			w.checkTakenToday(t, f.Account.ID, pasted)
		})
	}
}

// A save that fails as ctx ends, for a reason of its own, fails with it,
// rather than saying it's cancelled.
func TestPasteTokenFailingToSaveAsItsCancelledFails(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	writeFile(t, filepath.Join(w.tokens().Path("personal"), "in-the-way"), "", 0o600)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r := w.tokenRig(t, &fakeMaker{})
	r.s.Accounts.Now = func() time.Time {
		cancel()
		return tokenTaken
	}

	got, err := r.s.PasteToken(ctx, adding, "test-token-pasted")

	if err == nil || errors.Is(err, context.Canceled) {
		t.Errorf("PasteToken() = %+v, %v; want the token file's failure", got, err)
	}
}

func TestMakeTokenTellsTheServiceOfASaveAlone(t *testing.T) {
	const takesUp = " · takes up setup's changes on its own"
	tests := []struct {
		name  string
		maker *fakeMaker
		// organizations are the other accounts' setup has read, and api the
		// organizations the API names tokens in.
		organizations setup.Organizations
		api           map[string]string
		// stopped cancels the making before it's made.
		stopped     bool
		wantTakesUp bool
	}{
		{name: "saved", maker: &fakeMaker{}, wantTakesUp: true},
		{name: "refused", maker: &fakeMaker{token: refusedToken}},
		{name: "not made", maker: &fakeMaker{err: screenFailure{}}},
		{
			name:          "the same subscription as another account",
			maker:         &fakeMaker{},
			organizations: setup.Organizations{"work": "org-work"},
			api:           map[string]string{madeToken: "org-work"},
		},
		{name: "cancelled before it's made", maker: &fakeMaker{block: true}, stopped: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			w.api.organizations = tt.api
			r := w.tokenRig(t, tt.maker)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			report := func(m setup.Making) {
				if tt.stopped && m.Step == setup.AwaitingApproval {
					cancel()
				}
			}
			f := replacing("side")
			f.Organizations = tt.organizations

			if _, err := r.s.MakeToken(ctx, f, report); err != nil {
				t.Fatalf("MakeToken() error = %v", err)
			}
			got, err := r.s.Service(t.Context())
			if err != nil || len(got.Said) != 1 {
				t.Fatalf("Service() = %+v, %v; want the router alone", got, err)
			}
			if takes := strings.HasSuffix(got.Said[0].Detail, takesUp); takes != tt.wantTakesUp {
				t.Errorf("Service() said %q, want the router taking up setup's changes: %t", got.Said[0].Detail, tt.wantTakesUp)
			}
		})
	}
}

func TestMakeTokenMakesNothingForAnAccountThatCantTakeIt(t *testing.T) {
	tests := []struct {
		name string
		f    setup.TokenFor
		want error
	}{
		{name: "one not configured", f: replacing("personal"), want: config.ErrNotConfigured},
		{name: "one to add that's configured", f: setup.TokenFor{Account: config.NewAccount{ID: "side"}, Add: true}, want: config.ErrConfigured},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			r := w.tokenRig(t, &fakeMaker{})
			before := w.snapshot(t)

			_, err := r.s.MakeToken(t.Context(), tt.f, nil)

			if !errors.Is(err, tt.want) || r.maker.asked > 0 {
				t.Errorf("MakeToken() error = %v, made %d; want %v, with nothing made", err, r.maker.asked, tt.want)
			}
			w.checkUnchanged(t, before)
		})
	}
}

func TestMakeTokenWithoutItsSeamsMakesNothing(t *testing.T) {
	tests := []struct {
		name string
		f    setup.TokenFor
		// unset unsets the seam the test is without.
		unset func(*setup.Setup)
		want  string
	}{
		{name: "no TokenMaker", f: replacing("side"), unset: func(s *setup.Setup) { s.TokenMaker = nil }, want: "make a token: setup has no TokenMaker"},
		{name: "no Browser, for the primary", f: replacing("work"), unset: func(s *setup.Setup) { s.Browser = nil }, want: "make a token: setup has no Browser to open the sign-in address in"},
		{name: "no PrivateWindow, for another", f: replacing("side"), unset: func(s *setup.Setup) { s.PrivateWindow = nil }, want: "make a token: setup has no PrivateWindow to open the sign-in address in"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			r := w.tokenRig(t, &fakeMaker{})
			tt.unset(r.s)

			_, err := r.s.MakeToken(t.Context(), tt.f, nil)

			if err == nil || err.Error() != tt.want || r.maker.asked > 0 {
				t.Errorf("MakeToken() error = %v, made %d; want %q, with nothing made", err, r.maker.asked, tt.want)
			}
		})
	}
}

func TestPasteTokenSavesItAsAMadeOne(t *testing.T) {
	const pasted = "test-token-pasted"
	tests := []struct {
		name          string
		paste         string
		organizations setup.Organizations
		api           map[string]string
		// cancelled cancels as the API is asked.
		cancelled bool
		want      setup.Tokened
	}{
		{name: "saved, with its year from today", paste: "  " + pasted + "\n", want: setup.Tokened{Result: setup.TokenSaved}},
		{name: "cancelled during the check", paste: pasted, cancelled: true, want: setup.Tokened{Result: setup.TokenCancelled}},
		{name: "nothing pasted", paste: " \n", want: setup.Tokened{Result: setup.TokenNotGiven, Why: nothingPasted}},
		{name: "more than a token pasted", paste: pasted + " " + pasted, want: setup.Tokened{Result: setup.TokenNotGiven, Why: moreThanToken}},
		{name: "refused", paste: refusedToken, want: setup.Tokened{Result: setup.TokenRefused, Why: "the API refused the token (HTTP 401 · Invalid bearer token), so it isn't saved: check it was copied whole"}},
		{
			name:          "another account's",
			paste:         pasted,
			organizations: setup.Organizations{"work": "org-work"},
			api:           map[string]string{pasted: "org-work"},
			want:          setup.Tokened{Result: setup.TokenSameAccount, SameAs: "work"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			w.api.organizations = tt.api
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.cancelled {
				w.api.probing = cancel
			}
			r := w.tokenRig(t, &fakeMaker{})
			before := w.snapshot(t)
			f := replacing("side")
			f.Organizations = tt.organizations

			got, err := r.s.PasteToken(ctx, f, tt.paste)

			checkTokened(t, got, err, tt.want)
			if r.maker.asked > 0 {
				t.Errorf("setup-token was run %d times, want none for a pasted token", r.maker.asked)
			}
			if tt.want.Result == setup.TokenSaved {
				w.checkTakenToday(t, "side", pasted)
				return
			}
			w.checkUnchanged(t, before)
		})
	}
}

// What PasteToken says of a paste that holds no token it takes.
const (
	nothingPasted = "nothing's pasted: paste the token claude setup-token created"
	moreThanToken = "what's pasted holds more than the token: paste the token alone, as claude setup-token showed it"
)

// pastedWhole is a setup token, as setup-token makes one, pasted whole, and
// pastedStart and pastedRest the two parts the unread card breaks it into,
// wrapping it at its width.
const (
	pastedWhole = pastedStart + pastedRest
	pastedStart = "sk-ant-oat01-test-token-pasted-"
	pastedRest  = "side_0123456789"
)

// A token is taken from what's pasted where it's unambiguous, whether its
// line breaks reach setup or not, as the screen's field leaves them out.
// Text with no box-drawing character in it is the token, one word as it
// is, or words broken by whitespace, joined where they make a setup token
// whole. Text with one was copied off the token card: its segments between
// box-drawing characters with whitespace inside are left out; copied a line
// at a time, only the segment the token starts in and those right after
// the blank before the card's gutter are its parts, the words beside the
// card left out; and the parts are taken only where they make a setup token
// whole. Anything else is refused, never quoted, the API asked nothing,
// nothing saved: nothing left; more than one token; or parts that make no
// setup token, as words, punctuation or text round the token do.
func TestAPastedTokenIsTakenOnlyWhereItsUnambiguous(t *testing.T) {
	tests := []struct {
		name, paste string
		// want is the token saved: "" where the paste is refused, as why says.
		want, why string
	}{
		{name: "whole, blanks round it", paste: "\t " + pastedWhole + " \r\n", want: pastedWhole},
		{name: "across lines", paste: pastedStart + "\n" + pastedRest + "\n", want: pastedWhole},
		{name: "with spaces between its parts", paste: pastedStart + "   " + pastedRest, want: pastedWhole},
		{name: "a no-break space between its parts", paste: pastedStart + " " + pastedRest, want: pastedWhole},
		{name: "one word of another shape, as it is", paste: "  test-token-pasted \n", want: "test-token-pasted"},
		{name: "a block copied from the card's gutter", paste: "│ " + pastedStart + "\n│ " + pastedRest + "\n", want: pastedWhole},
		{
			name:  "a line at a time, from its start to its end, the page beside the card blank",
			paste: pastedStart + "      │\n│       │ " + pastedRest,
			want:  pastedWhole,
		},
		{
			name:  "a line at a time, from its start to its end, a word of the page each side of the card",
			paste: pastedStart + "      │ Personal\n  personal│       │ " + pastedRest,
			want:  pastedWhole,
		},
		{
			name:  "a line at a time, whole rows, the page's words beside the card, its blanks kept",
			paste: "  personal│       │ " + pastedStart + "      │ sidehasnoworking   \n  side has│       │ " + pastedRest + "                   │ Personal   ",
			want:  pastedWhole,
		},
		{
			name: "a line at a time, the card's own words above and below it",
			paste: "  work│       │ Your OAuth token (valid for 1 year):       │\n" +
				"  personal│       │ " + pastedStart + "      │ token\n" +
				"  side│       │ " + pastedRest + "                   │\n" +
				"  personal│       │ Store this token securely.       │",
			want: pastedWhole,
		},
		{name: "nothing", paste: "", why: nothingPasted},
		{name: "nothing but blanks and edges", paste: " \n│ ─ │\t", why: nothingPasted},
		{name: "two tokens", paste: pastedWhole + "\n" + pastedWhole, why: moreThanToken},
		{name: "two tokens off the card", paste: "│ " + pastedWhole + "\n│ " + pastedWhole, why: moreThanToken},
		{name: "words before the token", paste: "personal " + pastedWhole, why: moreThanToken},
		{name: "punctuation after it", paste: pastedStart + " " + pastedRest + ".", why: moreThanToken},
		{name: "two words of another shape", paste: "test-token-pasted test-token-pasted", why: moreThanToken},
		{name: "one word of another shape, off the card", paste: "│ test-token-pasted │", why: moreThanToken},
		{name: "the card's words alone", paste: "│       │ Your OAuth token (valid for 1 year):       │", why: moreThanToken},
		{name: "a control in it", paste: "test-token-\x07pasted", why: moreThanToken},
	}
	for _, tt := range tests {
		for _, through := range []struct{ name, paste string }{
			{name: "as pasted", paste: tt.paste},
			{name: "its line breaks left out", paste: strings.NewReplacer("\r", "", "\n", "").Replace(tt.paste)},
		} {
			t.Run(tt.name+", "+through.name, func(t *testing.T) {
				w := newWorld(t)
				w.done(t)
				r := w.tokenRig(t, &fakeMaker{})
				before := w.snapshot(t)

				got, err := r.s.PasteToken(t.Context(), replacing("side"), through.paste)

				if tt.want != "" {
					checkTokened(t, got, err, setup.Tokened{Result: setup.TokenSaved})
					w.checkTakenToday(t, "side", tt.want)
					return
				}
				checkTokened(t, got, err, setup.Tokened{Result: setup.TokenNotGiven, Why: tt.why})
				if len(w.api.asked) > 0 {
					t.Errorf("the API was asked about %d tokens, want nothing asked of a paste refused", len(w.api.asked))
				}
				for _, part := range []string{pastedStart, pastedRest, "test-token-pasted"} {
					if strings.Contains(got.Why, part) {
						t.Errorf("why reads %q, want nothing of what's pasted", got.Why)
					}
				}
				w.checkUnchanged(t, before)
			})
		}
	}
}

// Where the API doesn't answer for a pasted token, it isn't saved, whatever
// its shape, setup saying why: a token cut short, as a copy of its first
// row alone, has a setup token's shape, and saved unchecked would take a
// working token's place, or add an account with none. Adding and replacing
// alike.
func TestAPasteTheAPIDoesntAnswerForIsntSaved(t *testing.T) {
	for _, f := range []setup.TokenFor{adding, replacing("side")} {
		for _, tt := range []struct{ name, paste string }{
			{name: "a setup token whole", paste: pastedWhole},
			{name: "a setup token cut short", paste: pastedWhole[:len(pastedWhole)-5]},
			{name: "a word of another shape", paste: "test-token-pasted"},
		} {
			t.Run(f.Account.ID+", "+tt.name, func(t *testing.T) {
				w := newWorld(t)
				w.done(t)
				w.api.down = errors.New("HTTP 529 · overloaded")
				r := w.tokenRig(t, &fakeMaker{})
				before := w.snapshot(t)

				got, err := r.s.PasteToken(t.Context(), f, tt.paste)

				want := setup.Tokened{Result: setup.TokenUnanswered, Why: "the API didn't answer (HTTP 529 · overloaded), so it isn't saved: paste it again in a moment"}
				checkTokened(t, got, err, want)
				w.checkUnchanged(t, before)
			})
		}
	}
}

func TestPasteTokenTakesNothingForAnAccountThatCantTakeIt(t *testing.T) {
	tests := []struct {
		name string
		f    setup.TokenFor
		want error
	}{
		{name: "one not configured", f: replacing("personal"), want: config.ErrNotConfigured},
		{name: "one to add that's configured", f: setup.TokenFor{Account: config.NewAccount{ID: "side"}, Add: true}, want: config.ErrConfigured},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			r := w.tokenRig(t, &fakeMaker{})
			before := w.snapshot(t)

			_, err := r.s.PasteToken(t.Context(), tt.f, "test-token-pasted")

			if !errors.Is(err, tt.want) || len(w.api.asked) > 0 {
				t.Errorf("PasteToken() error = %v, the API asked about %q; want %v, with nothing asked", err, w.api.asked, tt.want)
			}
			w.checkUnchanged(t, before)
		})
	}
}

func TestReopenOpensTheAddressAsItOpenedFirst(t *testing.T) {
	tests := []struct {
		name string
		f    setup.TokenFor
		// private is the private window's opener, where it's not one that
		// opens Firefox's.
		private *fakeOpener
		// wantBrowser and wantPrivate are the times each opener is asked.
		wantBrowser, wantPrivate int
		wantErr                  bool
	}{
		{name: "in the browser as it is", f: replacing("work"), wantBrowser: 2},
		{name: "in a private window", f: replacing("side"), wantPrivate: 2},
		{name: "never opened: there to show", f: replacing("side"), private: &fakeOpener{err: &setuptoken.NoPrivateWindowError{Browser: "Safari"}}, wantPrivate: 1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			r := w.tokenRig(t, &fakeMaker{})
			if tt.private != nil {
				r.private = tt.private
				r.s.PrivateWindow = tt.private
			}
			var h heard
			if _, err := r.s.MakeToken(t.Context(), tt.f, h.report); err != nil {
				t.Fatalf("MakeToken() error = %v", err)
			}

			err := r.s.Reopen(t.Context(), h.address(t))

			if (err != nil) != tt.wantErr {
				t.Errorf("Reopen() error = %v, want an error: %t", err, tt.wantErr)
			}
			if len(r.browser.asked) != tt.wantBrowser || len(r.private.asked) != tt.wantPrivate {
				t.Errorf("the browser as it is opened %q, the private window %q; want %d and %d", r.browser.asked, r.private.asked, tt.wantBrowser, tt.wantPrivate)
			}
			for _, asked := range slices.Concat(r.browser.asked, r.private.asked) {
				if asked != signIn {
					t.Errorf("an opener was asked to open %q, want %q", asked, signIn)
				}
			}
		})
	}
}

// Neither the token nor the sign-in address is ever in an error MakeToken or
// PasteToken gives, what they tell, or a log line, though the API's failure,
// setup-token's screen and the openers' see them.
func TestMakingATokenNeverTellsTheTokenOrTheAddress(t *testing.T) {
	var logged bytes.Buffer
	logs.Init(logs.Options{Level: slog.LevelDebug, Mirror: &logged})
	defer logs.Close(0)
	down := errors.New("dial tcp: no route to the API for " + madeToken)
	shown := "Your OAuth token (valid for 1 year):\n\n" + madeToken + "\n\nStore this token securely.\n"
	w := newWorld(t)
	w.done(t)
	var told []string
	report := func(m setup.Making) { told = append(told, fmt.Sprintf("%+v %v", m, m.Address)) }
	outcome := func(got setup.Tokened, err error) {
		got.Shown = nil
		told = append(told, fmt.Sprintf("%+v %v", got, err))
	}

	w.api.down = down
	r := w.tokenRig(t, &fakeMaker{})
	outcome(r.s.MakeToken(t.Context(), adding, report))
	w.api.down = nil
	r.private.err = &setuptoken.NoPrivateWindowError{Browser: "Safari"}
	r.private.opening = setuptoken.Opening{}
	outcome(r.s.MakeToken(t.Context(), replacing("side"), report))
	r.maker.err = screenFailure{screen: shown}
	for _, madeFirst := range []bool{false, true} {
		r.maker.madeFirst = madeFirst
		got, err := r.s.MakeToken(t.Context(), replacing("side"), report)
		if !slices.Contains(got.Shown, madeToken) {
			t.Errorf("the outcome showed %q, want what setup-token showed, the token in it, to copy", got.Shown)
		}
		outcome(got, err)
	}
	outcome(r.s.PasteToken(t.Context(), replacing("side"), refusedToken))
	outcome(r.s.PasteToken(t.Context(), replacing("side"), madeToken+" "+madeToken))

	for _, logLine := range []string{"saved a token", "couldn't check a token with the API", "claude setup-token made no token", "claude setup-token made a token switchboard couldn't read"} {
		if !strings.Contains(logged.String(), logLine) {
			t.Errorf("the log reads\n%s\nwant a line saying %q", logged.String(), logLine)
		}
	}
	for _, secret := range []string{madeToken, refusedToken, signIn} {
		if strings.Contains(logged.String(), secret) {
			t.Errorf("the log reads\n%s\nwant nothing of %q", logged.String(), secret)
		}
		for _, said := range told {
			if strings.Contains(said, secret) {
				t.Errorf("setup told %q, want nothing of %q", said, secret)
			}
		}
	}
}
