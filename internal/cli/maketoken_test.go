package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/setuptoken"
	"github.com/leeovery/switchboard/internal/tokens"
)

// The tokens the organizations API answers for, each in its organization:
// personal's own; side's new one, its own subscription's; another of
// personal's subscription; one it doesn't answer for; a stale one of side's,
// which it refuses; work's, of a subscription of its own; and one it answers
// for naming no organization.
const (
	personalsToken = "test-token-personal"
	madeSide       = "test-token-side"
	madeTwice      = "test-token-personal-again"
	madeUnanswered = "test-token-down"
	staleSide      = "test-token-stale"
	madeWork       = "test-token-work"
	madeUnnamed    = "test-token-unnamed"
)

// signIn is the sign-in address the fake setup-token hands over.
const signIn = "https://claude.ai/oauth/authorize?code=test-sign-in"

// What accounts token says of each step, as a making goes: starting is the
// other accounts' tokens checked, then setup-token started, two lines.
const (
	starting = "Checking the other accounts' tokens, to tell their subscriptions from this one's.\n" +
		"Starting claude setup-token: its sign-in page opens in the browser once it's ready."
	waits   = "Waiting for you to approve it in the browser: the token's saved automatically, so there's nothing to copy."
	made    = "claude setup-token created the token: an interrupt now waits until it's checked, and what came of it is said."
	checks  = "Checking the token and saving it: the API checks it, and it's good for a year from today."
	private = "Close any other private window first: they all share one sign-in."
)

// organizationsAPI takes the tokens it names an organization for, naming
// it, refuses every other, and doesn't answer for madeUnanswered, noting
// each token it's asked about.
type organizationsAPI struct {
	URL string

	mu    sync.Mutex
	asked []string
}

func newOrganizationsAPI(t *testing.T) *organizationsAPI {
	t.Helper()
	organizations := map[string]string{personalsToken: "org-personal", madeSide: "org-side", madeTwice: "org-personal", madeWork: "org-work", madeUnnamed: ""}
	api := &organizationsAPI{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		api.mu.Lock()
		api.asked = append(api.asked, token)
		api.mu.Unlock()
		organization, ok := organizations[token]
		switch {
		case token == madeUnanswered:
			w.WriteHeader(529)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
			return
		case !ok:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"Invalid bearer token"}}`)
			return
		}
		for name, value := range map[string]string{
			"anthropic-ratelimit-unified-5h-utilization": "0.23",
			"anthropic-ratelimit-unified-5h-reset":       "1790619000",
			"anthropic-ratelimit-unified-5h-status":      "allowed",
			"anthropic-ratelimit-unified-7d-utilization": "0.4",
			"anthropic-ratelimit-unified-7d-reset":       "1790974800",
			"anthropic-ratelimit-unified-7d-status":      "allowed",
			"anthropic-organization-id":                  organization,
		} {
			w.Header().Set(name, value)
		}
		_, _ = io.WriteString(w, `{"type":"message"}`)
	}))
	t.Cleanup(srv.Close)
	api.URL = srv.URL
	return api
}

// askedAbout reports whether the API was asked about token.
func (a *organizationsAPI) askedAbout(token string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Contains(a.asked, token)
}

// fakeTokenMaker stands in for claude setup-token: it opens the sign-in
// address with the opener it's given, reports each step, and makes token,
// or fails with err, before it says it made one, or, where made is set,
// after. waiting runs as it waits for the approval, and stop has it wait
// then till it's stopped; madeIt runs once it says it made the token.
type fakeTokenMaker struct {
	token   string
	err     error
	made    bool
	stop    bool
	waiting func()
	madeIt  func()
	asked   int
}

func (m *fakeTokenMaker) Make(ctx context.Context, s setuptoken.Signing) (tokens.Token, error) {
	m.asked++
	address, _ := setuptoken.ParseAddress(signIn)
	opening, err := s.Opener.Open(ctx, address)
	s.Report(setuptoken.Event{Step: setuptoken.StepAddress, Address: address, Opening: opening, Unopened: err})
	s.Report(setuptoken.Event{Step: setuptoken.StepWaiting})
	if m.waiting != nil {
		m.waiting()
	}
	switch {
	case m.stop:
		<-ctx.Done()
		return tokens.Token{}, ctx.Err()
	case m.err != nil && !m.made:
		return tokens.Token{}, m.err
	}
	s.Report(setuptoken.Event{Step: setuptoken.StepMade})
	if m.madeIt != nil {
		m.madeIt()
	}
	if m.err != nil {
		return tokens.Token{}, m.err
	}
	return tokens.Parse(m.token)
}

// opener opens a sign-in address as opening says, or fails with err, noting
// each it's asked to open.
type opener struct {
	opening setuptoken.Opening
	err     error
	opened  []string
}

func (o *opener) Open(_ context.Context, address setuptoken.Address) (setuptoken.Opening, error) {
	o.opened = append(o.opened, address.Reveal())
	return o.opening, o.err
}

// screenFailure is setup-token's failure, as a *setuptoken.ScreenError is,
// having shown screen.
type screenFailure struct {
	screen string
}

func (screenFailure) Error() string {
	return "setup-token said it made a token, but its screen didn't show one"
}
func (e screenFailure) Screen() string { return e.screen }

// makingDeps are commands' deps at a terminal, personal, the primary, with
// its token, and side, with a stale one, configured, checked by api, the
// token made by maker, its sign-in opened by browser, and by private in a
// private window.
func makingDeps(t *testing.T, api *organizationsAPI, maker *fakeTokenMaker, browser, private *opener) cli.Deps {
	t.Helper()
	deps, _ := accountsDeps(t, api.URL, personalAndSide)
	writeToken(t, deps, "personal", personalsToken)
	writeToken(t, deps, "side", staleSide)
	atATerminal(&deps)
	deps.TokenMaker, deps.Browser, deps.PrivateWindow = maker, browser, private
	deps.Hidden = func(io.Reader) (func() ([]byte, error), bool) {
		t.Error("the command asked for the token, want it made")
		return nil, false
	}
	return deps
}

// lines are each of the lines given, ended.
func lines(each ...string) string {
	return strings.Join(each, "\n") + "\n"
}

func TestAccountsTokenMakesTheTokenAtATerminal(t *testing.T) {
	firefox := &opener{opening: setuptoken.Opening{Opened: setuptoken.OpenedPrivately, Browser: "Firefox"}}
	tests := []struct {
		name             string
		id               string
		browser, private *opener
		want             string
	}{
		{
			name: "another account's, in a private window",
			id:   "side", browser: &opener{}, private: firefox,
			want: lines(starting,
				"Your browser is signed in to Personal, so setup opened a private Firefox window. Sign in to Side there, then approve.",
				private, waits, made, checks, "saved side's token"),
		},
		{
			name: "another account's, no private window opened: the address to open by hand",
			id:   "side", browser: &opener{}, private: &opener{err: &setuptoken.NoPrivateWindowError{Browser: "Safari"}},
			want: lines(starting,
				"Your browser is signed in to Personal, and setup couldn't open a private Safari window. Open this address in one, sign in to Side there, then approve:",
				"  "+signIn, waits, made, checks, "saved side's token"),
		},
		{
			name: "the primary's, in the browser as it is",
			id:   "personal", browser: &opener{opening: setuptoken.Opening{Opened: setuptoken.OpenedInBrowser}}, private: &opener{},
			want: lines(starting, "Its sign-in page is open in the browser.", waits, made, checks, "saved personal's token"),
		},
		{
			name: "the primary's, the browser not opened: the address to open by hand",
			id:   "personal", browser: &opener{err: errors.New("open: exit status 1")}, private: &opener{},
			want: lines(starting,
				"Setup couldn't open your browser. Open this address in it, signed in to Personal, then approve:",
				"  "+signIn, waits, made, checks, "saved personal's token"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newOrganizationsAPI(t)
			maker := &fakeTokenMaker{token: madeSide}
			deps := makingDeps(t, api, maker, tt.browser, tt.private)

			got := run(t, deps, "accounts", "token", tt.id)

			if want := (result{stdout: tt.want}); got != want {
				t.Errorf("switchboard accounts token %s =\n%+v\nwant\n%+v", tt.id, got, want)
			}
			checkTokenFile(t, deps, tt.id, madeSide+"\n")
			if opened := len(tt.browser.opened) + len(tt.private.opened); opened != 1 {
				t.Errorf("the sign-in address was opened %d times, want once", opened)
			}
		})
	}
}

// The other accounts' tokens are checked before the token's made, so its
// organization is compared with theirs; the account's own token isn't asked
// about.
func TestAccountsTokenRefusesAnotherAccountsSubscription(t *testing.T) {
	api := newOrganizationsAPI(t)
	maker := &fakeTokenMaker{token: madeTwice}
	deps := makingDeps(t, api, maker, &opener{}, &opener{opening: setuptoken.Opening{Opened: setuptoken.OpenedPrivately}})

	got := run(t, deps, "accounts", "token", "side")

	want := result{stdout: lines(starting,
		"Your browser is signed in to Personal, so setup opened a private window. Sign in to Side there, then approve.",
		private, waits, made, checks,
		"The token is for personal's subscription",
		"personal has that subscription already, so the token isn't saved.",
		"Run switchboard accounts token side to try again, signed in to the other subscription, or switchboard accounts token side --paste to paste a token you made with claude setup-token.",
	), code: 1}
	if got != want {
		t.Errorf("switchboard accounts token side =\n%+v\nwant\n%+v", got, want)
	}
	checkTokenFile(t, deps, "side", staleSide+"\n")
	if !api.askedAbout(personalsToken) || api.askedAbout(staleSide) {
		t.Error("the API wasn't asked about personal's token, or was asked about side's own; want personal's checked, to know its organization, and side's left")
	}
}

// A making that saves nothing says so in setup's words, what to do now
// being to run the command again, or with --paste, and fails.
func TestAccountsTokenSaysAMakingThatSavedNothing(t *testing.T) {
	switchboard := filepath.Join(t.TempDir(), "switchboard")
	if err := os.WriteFile(switchboard, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(t.TempDir(), ".local", "bin", "claude")
	_, noClaude := claude.Find(t.TempDir(), []string{installed}, func() (string, error) { return switchboard, nil })
	if _, ok := errors.AsType[*claude.NotFoundError](noClaude); !ok {
		t.Fatalf("claude.Find() error = %v, want no claude found", noClaude)
	}
	const tryAgain = "Run switchboard accounts token side to try again, or switchboard accounts token side --paste to paste a token you made with claude setup-token."
	tests := []struct {
		name  string
		maker *fakeTokenMaker
		want  []string
	}{
		{
			name:  "the API refusing it",
			maker: &fakeTokenMaker{token: "test-token-unknown"},
			want:  []string{made, checks, "The token was refused", "The API refused the token, so it isn't saved.", tryAgain},
		},
		{
			name:  "setup-token stopping with an error",
			maker: &fakeTokenMaker{err: screenFailure{screen: " Opening the browser…\n\n The sign-in timed out.\n"}},
			want: []string{
				"The token wasn't created", "claude setup-token stopped with an error. What it showed:",
				"  Opening the browser…", "  The sign-in timed out.", tryAgain,
			},
		},
		{
			name:  "no Claude Code",
			maker: &fakeTokenMaker{err: noClaude},
			want: []string{
				"There's no Claude Code to create the token with", noClaude.Error(),
				"Install Claude Code, then run switchboard accounts token side to try again, or switchboard accounts token side --paste to paste a token you made with claude setup-token.",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := makingDeps(t, newOrganizationsAPI(t), tt.maker, &opener{}, &opener{opening: setuptoken.Opening{Opened: setuptoken.OpenedPrivately}})

			got := run(t, deps, "accounts", "token", "side")

			opened := "Your browser is signed in to Personal, so setup opened a private window. Sign in to Side there, then approve."
			want := result{stdout: lines(slices.Concat([]string{starting, opened, private, waits}, tt.want)...), code: 1}
			if got != want {
				t.Errorf("switchboard accounts token side =\n%+v\nwant\n%+v", got, want)
			}
			checkTokenFile(t, deps, "side", staleSide+"\n")
		})
	}
}

// A token made, but unread or not saved, lasts a year on the account, so
// it's shown, to copy, with why, and how to paste it.
func TestAccountsTokenShowsATokenMadeButNotSaved(t *testing.T) {
	tests := []struct {
		name   string
		maker  func(deps cli.Deps) *fakeTokenMaker
		saying []string
	}{
		{
			name: "unread",
			maker: func(cli.Deps) *fakeTokenMaker {
				return &fakeTokenMaker{made: true, err: screenFailure{screen: " Your OAuth token (valid for 1 year):\n\n " + madeSide + "\n"}}
			},
			saying: []string{
				made,
				"The token was created, but setup couldn't read it",
				"It lasts a year on the account, so copy it from what claude setup-token showed:",
				"  Your OAuth token (valid for 1 year):",
				"  " + madeSide,
				"Then run switchboard accounts token side --paste, and paste it.",
			},
		},
		{
			name: "not saved, as side was removed from the config meanwhile",
			maker: func(deps cli.Deps) *fakeTokenMaker {
				return &fakeTokenMaker{token: madeSide, madeIt: func() {
					path := deps.Getenv("SWITCHBOARD_CONFIG")
					withoutSide := strings.Replace(readFile(t, path), "[[account]]\nid    = \"side\"\nlabel = \"Side\"\n", "", 1)
					if err := os.WriteFile(path, []byte(withoutSide), 0o600); err != nil {
						t.Error(err)
					}
				}}
			},
			saying: []string{
				made, checks,
				"The token was created, but setup couldn't save it",
				`account "side" is not configured`,
				"  " + madeSide,
				"It lasts a year on the account, so copy it, then once that's put right, run switchboard accounts token side --paste, and paste it.",
			},
		},
	}
	for _, tt := range tests {
		for _, interrupted := range []bool{false, true} {
			name := tt.name
			if interrupted {
				name += ", an interrupt held"
			}
			t.Run(name, func(t *testing.T) {
				held := &heldInterrupts{}
				deps := makingDeps(t, newOrganizationsAPI(t), nil, &opener{}, &opener{opening: setuptoken.Opening{Opened: setuptoken.OpenedPrivately}})
				deps.Hold = held.hold
				maker := tt.maker(deps)
				madeIt := maker.madeIt
				maker.madeIt = func() {
					if madeIt != nil {
						madeIt()
					}
					if interrupted {
						held.send(os.Interrupt)
					}
				}
				deps.TokenMaker = maker

				got, unheld := runHolding(t, deps, held, "accounts", "token", "side")

				opened := "Your browser is signed in to Personal, so setup opened a private window. Sign in to Side there, then approve."
				want := result{stdout: lines(slices.Concat([]string{starting, opened, private, waits}, tt.saying)...), code: 1}
				if interrupted {
					want.stderr, want.code = "switchboard: interrupted: saving the token first, as it lasts a year on the account\n", 130
				}
				if got != want {
					t.Errorf("switchboard accounts token side =\n%+v\nwant\n%+v", got, want)
				}
				if ending := tt.saying[len(tt.saying)-4:]; slices.ContainsFunc(ending, func(line string) bool { return slices.Contains(unheld, line) }) {
					t.Errorf("printed %q with the interrupts let go, want them held till the token's printed and what to do with it", unheld)
				}
				checkTokenFile(t, deps, "side", staleSide+"\n")
				if log := readLog(t, deps, "cli.log"); strings.Contains(log, madeSide) {
					t.Errorf("cli.log shows the token made:\n%s", log)
				}
			})
		}
	}
}

// runHolding runs the command line as run does, at a terminal, and returns
// too the lines it printed on stdout once a token was made while held held
// no interrupts.
func runHolding(t *testing.T, deps cli.Deps, held *heldInterrupts, args ...string) (result, []string) {
	t.Helper()
	got, stdout := runWriting(t, deps, &holdingWriter{held: held}, args...)
	return got, stdout.unheld
}

// runWriting runs the command line as run does, at a terminal, writing its
// stdout to stdout.
func runWriting(t *testing.T, deps cli.Deps, stdout *holdingWriter, args ...string) (result, *holdingWriter) {
	t.Helper()
	root := cli.NewRootCommand(deps)
	var stderr bytes.Buffer
	root.SetArgs(args)
	root.SetIn(strings.NewReader(""))
	root.SetOut(stdout)
	root.SetErr(&stderr)
	code := cli.Execute(root)
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}, stdout
}

// An interrupt once setup has let the interrupts go, as the ending prints,
// reaches the command's own hold alone, which says it saves the token first
// and ends the command as interrupted, the ending printed in full.
func TestAccountsTokenInterruptedAsItsEndingPrintsEndsInterrupted(t *testing.T) {
	tests := []struct {
		name   string
		maker  *fakeTokenMaker
		ending []string
		at     string
	}{
		{
			name:   "saved",
			maker:  &fakeTokenMaker{token: madeSide},
			ending: []string{made, checks, "saved side's token"},
			at:     "saved side's token",
		},
		{
			name:  "unread",
			maker: &fakeTokenMaker{made: true, err: screenFailure{screen: " " + madeSide + "\n"}},
			ending: []string{
				made,
				"The token was created, but setup couldn't read it",
				"It lasts a year on the account, so copy it from what claude setup-token showed:",
				"  " + madeSide,
				"Then run switchboard accounts token side --paste, and paste it.",
			},
			at: "Then run switchboard accounts token side --paste, and paste it.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			held := &heldInterrupts{}
			deps := makingDeps(t, newOrganizationsAPI(t), tt.maker, &opener{}, &opener{opening: setuptoken.Opening{Opened: setuptoken.OpenedPrivately}})
			deps.Hold = held.hold

			got, stdout := runWriting(t, deps, &holdingWriter{held: held, interruptAt: tt.at}, "accounts", "token", "side")

			opened := "Your browser is signed in to Personal, so setup opened a private window. Sign in to Side there, then approve."
			want := result{
				stdout: lines(slices.Concat([]string{starting, opened, private, waits}, tt.ending)...),
				stderr: "switchboard: interrupted: saving the token first, as it lasts a year on the account\n",
				code:   130,
			}
			if got != want {
				t.Errorf("switchboard accounts token side =\n%+v\nwant\n%+v", got, want)
			}
			if stdout.heldAt != 1 {
				t.Errorf("%d holds held the interrupts as the ending printed, want the command's alone", stdout.heldAt)
			}
		})
	}
}

// With one account there's no other token to check, so none is, and
// nothing says so.
func TestAccountsTokenOfTheOnlyAccountChecksNoOther(t *testing.T) {
	api := newOrganizationsAPI(t)
	deps, _ := accountsDeps(t, api.URL, "[[account]]\nid    = \"personal\"\nlabel = \"Personal\"\n")
	writeToken(t, deps, "personal", personalsToken)
	atATerminal(&deps)
	deps.TokenMaker = &fakeTokenMaker{token: madeSide}
	deps.Browser = &opener{opening: setuptoken.Opening{Opened: setuptoken.OpenedInBrowser}}

	got := run(t, deps, "accounts", "token", "personal")

	starts := "Starting claude setup-token: its sign-in page opens in the browser once it's ready."
	if want := (result{stdout: lines(starts, "Its sign-in page is open in the browser.", waits, made, checks, "saved personal's token")}); got != want {
		t.Errorf("switchboard accounts token personal =\n%+v\nwant\n%+v", got, want)
	}
	if api.askedAbout(personalsToken) {
		t.Error("personal's own token was checked, want no token but the one made")
	}
}

// holdingWriter keeps what's written, noting each line written, once the
// interrupts were first held, while none held them.
// Where interruptAt is set, it sends an interrupt as that line is
// written, noting how many holds held the interrupts then.
type holdingWriter struct {
	bytes.Buffer
	held        *heldInterrupts
	unheld      []string
	interruptAt string
	heldAt      int
}

func (w *holdingWriter) Write(p []byte) (int, error) {
	line := strings.TrimSuffix(string(p), "\n")
	if active, holds := w.held.state(); holds > 0 && active == 0 {
		w.unheld = append(w.unheld, strings.Split(line, "\n")...)
	}
	if w.interruptAt != "" && line == w.interruptAt {
		w.heldAt, _ = w.held.state()
		w.held.send(os.Interrupt)
	}
	return w.Buffer.Write(p)
}

// Where an other account's token can't be checked, its organization isn't
// known, so a token made for its subscription is saved, as the design
// accepts.
func TestAccountsTokenSavesATokenWhereAnotherAccountsOrganizationIsntKnown(t *testing.T) {
	api := newOrganizationsAPI(t)
	deps := makingDeps(t, api, &fakeTokenMaker{token: madeTwice}, &opener{}, &opener{opening: setuptoken.Opening{Opened: setuptoken.OpenedPrivately}})
	writeToken(t, deps, "personal", madeUnanswered)

	got := run(t, deps, "accounts", "token", "side")

	if !strings.HasSuffix(got.stdout, lines(checks, "saved side's token")) || got.code != 0 {
		t.Errorf("switchboard accounts token side = %+v, want it saved", got)
	}
	checkTokenFile(t, deps, "side", madeTwice+"\n")
	if !api.askedAbout(madeUnanswered) {
		t.Error("personal's token wasn't checked, want it asked about")
	}
}

func TestAccountsTokenSavesATokenMadeTheAPIDoesntAnswerForWithAWarning(t *testing.T) {
	deps := makingDeps(t, newOrganizationsAPI(t), &fakeTokenMaker{token: madeUnanswered}, &opener{}, &opener{opening: setuptoken.Opening{Opened: setuptoken.OpenedPrivately}})

	got := run(t, deps, "accounts", "token", "side")

	if !strings.HasSuffix(got.stdout, lines(checks, "saved side's token")) || got.code != 0 ||
		!strings.HasPrefix(got.stderr, "switchboard: couldn't check the token with the API (") || !strings.HasSuffix(got.stderr, ") — saved it all the same\n") {
		t.Errorf("switchboard accounts token side = %+v, want it saved, warning the API didn't answer for it", got)
	}
	checkTokenFile(t, deps, "side", madeUnanswered+"\n")
}

func TestAccountsTokenInterruptedBeforeTheTokensMadeSavesNothing(t *testing.T) {
	var interrupt context.CancelFunc
	maker := &fakeTokenMaker{token: madeSide, stop: true, waiting: func() { interrupt() }}
	deps := makingDeps(t, newOrganizationsAPI(t), maker, &opener{}, &opener{opening: setuptoken.Opening{Opened: setuptoken.OpenedPrivately}})
	deps.Interrupt = func(ctx context.Context) (context.Context, context.CancelFunc) {
		ctx, interrupt = context.WithCancel(ctx)
		return ctx, interrupt
	}

	got := run(t, deps, "accounts", "token", "side")

	opened := "Your browser is signed in to Personal, so setup opened a private window. Sign in to Side there, then approve."
	want := result{
		stdout: lines(starting, opened, private, waits),
		stderr: "Error: interrupted: claude setup-token is stopped, and nothing is saved\n",
		code:   130,
	}
	if got != want {
		t.Errorf("switchboard accounts token side =\n%+v\nwant\n%+v", got, want)
	}
	checkTokenFile(t, deps, "side", staleSide+"\n")
}

func TestAccountsTokenInterruptedOnceTheTokensMadeSavesItFirst(t *testing.T) {
	held := &heldInterrupts{}
	maker := &fakeTokenMaker{token: madeSide, madeIt: func() { held.send(os.Interrupt) }}
	deps := makingDeps(t, newOrganizationsAPI(t), maker, &opener{}, &opener{opening: setuptoken.Opening{Opened: setuptoken.OpenedPrivately}})
	deps.Hold = held.hold

	got := run(t, deps, "accounts", "token", "side")

	opened := "Your browser is signed in to Personal, so setup opened a private window. Sign in to Side there, then approve."
	want := result{
		stdout: lines(starting, opened, private, waits, made, checks, "saved side's token"),
		stderr: "switchboard: interrupted: saving the token first, as it lasts a year on the account\n",
		code:   130,
	}
	if got != want {
		t.Errorf("switchboard accounts token side =\n%+v\nwant\n%+v", got, want)
	}
	checkTokenFile(t, deps, "side", madeSide+"\n")
	if _, holds := held.state(); holds != 2 {
		t.Errorf("the interrupts were held %d times, want twice from the token made: through its save, and till its end is said", holds)
	}
}

// heldInterrupts stands in for the process's interrupts, held as a token
// made is saved, by any number of holds at once: one sent reaches each hold
// while it's held, and none is ever a real signal.
type heldInterrupts struct {
	mu     sync.Mutex
	caught []chan<- os.Signal
	holds  int
}

func (h *heldInterrupts) hold(caught chan<- os.Signal) func() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.caught = append(h.caught, caught)
	h.holds++
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.caught = slices.DeleteFunc(h.caught, func(c chan<- os.Signal) bool { return c == caught })
	}
}

func (h *heldInterrupts) send(sig os.Signal) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, caught := range h.caught {
		select {
		case caught <- sig:
		default:
		}
	}
}

// state reports how many holds hold the interrupts now, and how many have
// held them.
func (h *heldInterrupts) state() (active, holds int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.caught), h.holds
}

func TestAccountsTokenMakesNoTokenForAnAccountNotConfigured(t *testing.T) {
	maker := &fakeTokenMaker{token: madeSide}
	deps := makingDeps(t, newOrganizationsAPI(t), maker, &opener{}, &opener{})

	got := run(t, deps, "accounts", "token", "work")

	if want := (result{stderr: "Error: account \"work\" is not configured: switchboard accounts lists each account's id\n", code: 1}); got != want {
		t.Errorf("switchboard accounts token work = %+v, want %+v", got, want)
	}
	if maker.asked != 0 {
		t.Error("a token was made, want none for an account not configured")
	}
}

// --paste, and a stdin or stdout that isn't a terminal, take a token given,
// as the command took one before it made them.
func TestAccountsTokenTakesATokenGiven(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		in, out  bool
		input    string
		hidden   string
		wantErr  string
		wantFile string
	}{
		{name: "--paste at a terminal: typed unseen", args: []string{"--paste"}, in: true, out: true, hidden: madeSide, wantFile: madeSide},
		{name: "--paste, stdin piped", args: []string{"--paste"}, out: true, input: madeSide + "\n", wantFile: madeSide},
		{name: "stdin piped", out: true, input: madeSide + "\n", wantFile: madeSide},
		{name: "stdout not a terminal: typed unseen", in: true, hidden: madeSide, wantFile: madeSide},
		{name: "neither a terminal, nothing on stdin", wantErr: "Error: no token given\n", wantFile: staleSide},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maker := &fakeTokenMaker{token: madeSide}
			deps := makingDeps(t, newOrganizationsAPI(t), maker, &opener{}, &opener{})
			deps.TerminalInput = func(io.Reader) bool { return tt.in }
			deps.Terminal = func(io.Writer) bool { return tt.out }
			deps.Hidden = func(io.Reader) (func() ([]byte, error), bool) {
				return func() ([]byte, error) { return []byte(tt.hidden), nil }, tt.in
			}

			got := runWithInput(t, deps, tt.input, append([]string{"accounts", "token", "side"}, tt.args...)...)

			want := result{stdout: "saved side's token\n"}
			switch {
			case tt.wantErr != "":
				want = result{stderr: tt.wantErr, code: 1}
			case tt.in:
				want.stderr = "Paste side's token, from claude setup-token run while signed in to that subscription (it won't show): \n"
			}
			if got != want {
				t.Errorf("switchboard accounts token side = %+v, want %+v", got, want)
			}
			checkTokenFile(t, deps, "side", tt.wantFile+"\n")
			if maker.asked != 0 {
				t.Error("a token was made, want the one given")
			}
		})
	}
}

// sameAsPersonal is what accounts add and token fail with, given a token of
// personal's subscription.
const sameAsPersonal = "Error: personal has that subscription already, so nothing is saved: " +
	"make another with claude setup-token, run while signed in to the other subscription\n"

// A token given, however it's given, is compared with the other accounts',
// as a token made is: one of another account's subscription is refused,
// saving nothing.
func TestAccountsTokenRefusesAGivenTokenOfAnotherAccountsSubscription(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		in     bool
		input  string
		hidden string
	}{
		{name: "--paste at a terminal: typed unseen", args: []string{"--paste"}, in: true, hidden: madeTwice},
		{name: "--paste, stdin piped", args: []string{"--paste"}, input: madeTwice + "\n"},
		{name: "stdin piped", input: madeTwice + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newOrganizationsAPI(t)
			maker := &fakeTokenMaker{token: madeSide}
			deps := makingDeps(t, api, maker, &opener{}, &opener{})
			deps.TerminalInput = func(io.Reader) bool { return tt.in }
			deps.Hidden = func(io.Reader) (func() ([]byte, error), bool) {
				return func() ([]byte, error) { return []byte(tt.hidden), nil }, tt.in
			}

			got := runWithInput(t, deps, tt.input, append([]string{"accounts", "token", "side"}, tt.args...)...)

			want := result{stderr: sameAsPersonal, code: 1}
			if tt.in {
				want.stderr = "Paste side's token, from claude setup-token run while signed in to that subscription (it won't show): \n" + want.stderr
			}
			if got != want {
				t.Errorf("switchboard accounts token side = %+v, want %+v", got, want)
			}
			checkTokenFile(t, deps, "side", staleSide+"\n")
			if !api.askedAbout(personalsToken) || api.askedAbout(staleSide) || maker.asked != 0 {
				t.Error("personal's token wasn't checked, side's own was, or a token was made; want personal's checked, to know its organization, and the token given taken")
			}
		})
	}
}

// configuredAsPersonal is what accounts add fails with, given a token of
// personal's subscription.
const configuredAsPersonal = `Error: the token is for "Personal" (personal), configured already, so nothing is saved: ` +
	"give it a new token with switchboard accounts token personal\n"

// accounts add compares the token given with the other accounts', adding
// nothing where its subscription is another's, and the account where it's
// its own.
func TestAccountsAddComparesTheTokenGivenWithTheOtherAccounts(t *testing.T) {
	tests := []struct {
		name   string
		in     bool
		token  string
		want   result
		wantIn bool
	}{
		{name: "piped, another account's subscription", token: madeTwice, want: result{stderr: configuredAsPersonal, code: 1}},
		{name: "typed unseen, another account's subscription", in: true, token: madeTwice, want: result{stderr: configuredAsPersonal, code: 1}},
		{name: "piped, a subscription of its own", token: madeWork, want: result{stdout: `added "work" as ` + workID + "\n"}, wantIn: true},
		{name: "typed unseen, a subscription of its own", in: true, token: madeWork, want: result{stdout: `added "work" as ` + workID + "\n"}, wantIn: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newOrganizationsAPI(t)
			deps, path := accountsDeps(t, api.URL, personalAndSide)
			writeToken(t, deps, "personal", personalsToken)
			before := readFile(t, path)
			deps.Hidden = func(io.Reader) (func() ([]byte, error), bool) {
				return func() ([]byte, error) { return []byte(tt.token), nil }, tt.in
			}
			if tt.in {
				tt.want.stderr = "Paste work's token, from claude setup-token run while signed in to that subscription (it won't show): \n" + tt.want.stderr
			}

			got := runWithInput(t, deps, tt.token+"\n", "accounts", "add", "work")

			if got != tt.want {
				t.Errorf("switchboard accounts add work = %+v, want %+v", got, tt.want)
			}
			if !api.askedAbout(personalsToken) {
				t.Error("personal's token wasn't checked, want it checked, to know its organization")
			}
			if !tt.wantIn {
				if readFile(t, path) != before {
					t.Errorf("the config reads\n%s\nwant it as it was\n%s", readFile(t, path), before)
				}
				checkNoTokenFile(t, deps, workID)
				return
			}
			if want := before + "\n[[account]]\nid    = \"" + workID + "\"\nlabel = \"work\"\n"; readFile(t, path) != want {
				t.Errorf("the config reads\n%s\nwant\n%s", readFile(t, path), want)
			}
			checkTokenFile(t, deps, workID, tt.token+"\n")
		})
	}
}

func TestAccountsTokensHelpDescribesMakingPastingAndStdin(t *testing.T) {
	if got := run(t, testDeps(nil, t.TempDir()), "accounts", "--help"); !strings.Contains(strings.Join(strings.Fields(got.stdout), " "), "token Replace an account's token, making it with claude setup-token") {
		t.Errorf("switchboard accounts --help reads\n%s\nwant token's making said", got.stdout)
	}
	got := run(t, testDeps(nil, t.TempDir()), "accounts", "token", "--help")
	help := strings.Join(strings.Fields(got.stdout), " ")
	for _, want := range []string{
		"At a terminal, it makes the token as setup does",
		"in a private window for any other account",
		"else shows the address to open in one",
		"An interrupt before the token's made stops claude setup-token, saving nothing.",
		"--paste asks for the token instead",
		"Away from a terminal, it reads the token from stdin.",
		"--paste take a token made with claude setup-token, pasted, in place of making one",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("switchboard accounts token --help reads\n%s\nwant it to say %q", got.stdout, want)
		}
	}
}
