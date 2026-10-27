//go:build darwin

package setuptoken_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/setuptoken"
)

func TestMakeTakesTheTokenOnceTheBrowserApprovesTheSignIn(t *testing.T) {
	f := newFake(t, "", "FAKE_EXIT_AFTER=0.5")
	leaks := watchLeaks(t)
	open := &opener{opened: setuptoken.OpenedInBrowser}
	var events []setuptoken.Event

	token, err := f.maker.Make(t.Context(), setuptoken.Signing{Opener: open, Report: approving(t, f, &events)})

	if err != nil || token.Reveal() != fakeToken {
		t.Fatalf("Make() = %q, %v; want the token the fake made", token.Reveal(), err)
	}
	if !slices.Equal(open.asked, []string{fakeAddress}) {
		t.Errorf("the Opener was asked to open %q, want the address setup-token handed its browser alone", open.asked)
	}
	want := []setuptoken.Step{setuptoken.StepAddress, setuptoken.StepWaiting, setuptoken.StepMade}
	if got := steps(events); !slices.Equal(got, want) {
		t.Fatalf("Make reported steps %v, want %v", got, want)
	}
	if e := events[0]; e.Address.Reveal() != fakeAddress || e.Opened != setuptoken.OpenedInBrowser || e.Unopened != nil {
		t.Errorf("Make reported the address as %q, opened %v (%v); want the one handed over, opened in the browser", e.Address.Reveal(), e.Opened, e.Unopened)
	}
	if got := f.noted(t, "args"); got != "setup-token" {
		t.Errorf("claude was run with %q, want setup-token", got)
	}
	leaks.check(t, f)
}

func TestMakeRunsSetupTokenOnATerminalWideEnoughToDrawTheTokenWhole(t *testing.T) {
	f := newFake(t, "")
	var events []setuptoken.Event

	if _, err := f.maker.Make(t.Context(), setuptoken.Signing{Opener: &opener{}, Report: approving(t, f, &events)}); err != nil {
		t.Fatalf("Make() failed: %v", err)
	}

	if got := f.noted(t, "tty"); !strings.HasPrefix(got, "/dev/ttys") {
		t.Errorf("setup-token ran on %q, want a pseudo-terminal", got)
	}
	if got := f.noted(t, "controlling"); got != "yes" {
		t.Errorf("setup-token could open /dev/tty: %q, want its terminal its controlling terminal", got)
	}
	if got := f.noted(t, "size"); got != "50 1000" {
		t.Errorf("setup-token's terminal is %q rows and columns, want 50 by 1000", got)
	}
}

func TestMakeReportsHowTheAddressOpened(t *testing.T) {
	refused := &setuptoken.NoPrivateWindowError{Browser: "Safari"}
	failed := errors.New("open the sign-in address: exit status 1")
	tests := []struct {
		name   string
		opened setuptoken.Opened
		err    error
	}{
		{name: "in the browser", opened: setuptoken.OpenedInBrowser},
		{name: "in a private window", opened: setuptoken.OpenedPrivately},
		{name: "not, the browser having no private window", opened: setuptoken.NotOpened, err: refused},
		{name: "not, open failing", opened: setuptoken.NotOpened, err: failed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(t, "")
			open := &opener{opened: tt.opened, err: tt.err}
			var events []setuptoken.Event

			token, err := f.maker.Make(t.Context(), setuptoken.Signing{Opener: open, Report: approving(t, f, &events)})

			if err != nil || token.Reveal() != fakeToken {
				t.Fatalf("Make() = %q, %v; want the token, approved however the address opened", token.Reveal(), err)
			}
			if len(events) == 0 || events[0].Step != setuptoken.StepAddress {
				t.Fatalf("Make reported %v first, want the address", steps(events))
			}
			if e := events[0]; e.Opened != tt.opened || !errors.Is(e.Unopened, tt.err) || e.Address.Reveal() != fakeAddress {
				t.Errorf("Make reported the address opened %v (%v), want %v (%v), with the address to show", e.Opened, e.Unopened, tt.opened, tt.err)
			}
		})
	}
}

func TestMakeStartsSetupTokenInTheUsersEnvironmentWithoutWhatWouldSendItElsewhere(t *testing.T) {
	f := newFake(t, "",
		"ANTHROPIC_BASE_URL=http://127.0.0.1:4747",
		"CLAUDE_CODE_OAUTH_TOKEN=test-token-work",
		"ANTHROPIC_API_KEY=test-key-work",
		"ANTHROPIC_AUTH_TOKEN=test-token-personal",
		"BROWSER=/usr/bin/false",
		"EDITOR=vi",
	)
	var events []setuptoken.Event

	if _, err := f.maker.Make(t.Context(), setuptoken.Signing{Opener: &opener{}, Report: approving(t, f, &events)}); err != nil {
		t.Fatalf("Make() failed: %v", err)
	}

	env := strings.Split(f.noted(t, "env"), "\n")
	for _, name := range []string{"ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"} {
		if slices.ContainsFunc(env, func(v string) bool { return strings.HasPrefix(v, name+"=") }) {
			t.Errorf("setup-token started with %s set, want it unset", name)
		}
	}
	if !slices.Contains(env, "EDITOR=vi") {
		t.Errorf("setup-token started without the user's EDITOR, want the rest of their environment")
	}
	browsers := slices.DeleteFunc(slices.Clone(env), func(v string) bool { return !strings.HasPrefix(v, "BROWSER=") })
	if len(browsers) != 1 || browsers[0] == "BROWSER=/usr/bin/false" || !strings.HasPrefix(browsers[0], "BROWSER="+f.shims) {
		t.Errorf("setup-token started with %q, want BROWSER the shim, in the temporary directory, alone", browsers)
	}
}

func TestMakeRunsTheRealClaudeNotSwitchboardsLink(t *testing.T) {
	f := newFake(t, "")
	switchboard, err := f.maker.Executable()
	if err != nil {
		t.Fatal(err)
	}
	link := claudetest.Link(t, switchboard, filepath.Join(t.TempDir(), "links", "claude"))
	path := strings.TrimPrefix(f.maker.Environ[0], "PATH=")
	f.maker.Environ = with(f.maker.Environ, "PATH="+filepath.Dir(link)+":"+path)
	var events []setuptoken.Event

	token, err := f.maker.Make(t.Context(), setuptoken.Signing{Opener: &opener{}, Report: approving(t, f, &events)})

	if err != nil || token.Reveal() != fakeToken {
		t.Errorf("Make() = %q, %v; want the token from the claude past switchboard's link", token.Reveal(), err)
	}
}

func TestMakeFailsWithoutAClaude(t *testing.T) {
	f := newFake(t, "", "PATH=/usr/bin:/bin")

	_, err := f.maker.Make(t.Context(), setuptoken.Signing{Opener: &opener{}})

	if err == nil || !strings.Contains(err.Error(), "can't find claude") {
		t.Errorf("Make() = %v, want it failing to find claude", err)
	}
	if left, _ := os.ReadDir(f.shims); len(left) > 0 {
		t.Errorf("Make left %d in the temporary directory, want nothing", len(left))
	}
}

func TestMakeTakesATokenDrawnInPiecesWhole(t *testing.T) {
	f := newFake(t, "slow")
	var events []setuptoken.Event

	token, err := f.maker.Make(t.Context(), setuptoken.Signing{Opener: &opener{}, Report: approving(t, f, &events)})

	if err != nil || token.Reveal() != fakeToken {
		t.Errorf("Make() = %q, %v; want the token whole", token.Reveal(), err)
	}
}

func TestMakeStopsSetupTokenThatDoesntExitOnceTheTokensMade(t *testing.T) {
	f := newFake(t, "", "FAKE_EXIT_AFTER=60")
	maker := setuptoken.Waiting(f.maker, 100*time.Millisecond, time.Minute)
	leaks := watchLeaks(t)
	var events []setuptoken.Event

	token, err := maker.Make(t.Context(), setuptoken.Signing{Opener: &opener{}, Report: approving(t, f, &events)})

	if err != nil || token.Reveal() != fakeToken {
		t.Errorf("Make() = %q, %v; want the token it showed", token.Reveal(), err)
	}
	leaks.check(t, f)
}

func TestMakeFailsWithTheScreenWhereSetupTokenGivesNoToken(t *testing.T) {
	tests := []struct {
		name string
		mode string
		// want is what the error says, and shown what the screen holds.
		want, shown string
	}{
		{name: "an OAuth error, where it stops setup-token", mode: "oauth-error", want: "setup-token showed an error: Request failed with status code 400", shown: "OAuth error: Request failed with status code 400\n\n Press Enter to retry."},
		{name: "an exit without a token", mode: "on-hold", want: "setup-token exited without making a token (exit status 1)", shown: "This account is on hold."},
		{name: "a token it can't read", mode: "broken", want: "setup-token said it made a token, but its screen didn't show one", shown: fakeToken[:40] + "\n " + fakeToken[40:]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(t, tt.mode)
			leaks := watchLeaks(t)
			var events []setuptoken.Event

			_, err := f.maker.Make(t.Context(), setuptoken.Signing{Opener: &opener{}, Report: approving(t, f, &events)})

			screen, ok := errors.AsType[*setuptoken.ScreenError](err)
			if !ok {
				t.Fatalf("Make() = %v, want a *ScreenError", err)
			}
			if err.Error() != tt.want {
				t.Errorf("Make() = %q, want %q", err, tt.want)
			}
			if !strings.Contains(screen.Screen(), tt.shown) {
				t.Errorf("the error's Screen() = %q, want what setup-token drew last, holding %q", screen.Screen(), tt.shown)
			}
			if strings.Contains(screen.Screen(), "\x1b") {
				t.Errorf("the error's Screen() = %q, want its escapes gone", screen.Screen())
			}
			leaks.check(t, f)
		})
	}
}

func TestMakeKeepsAnUnreadableTokenOnTheScreenAndOutOfTheError(t *testing.T) {
	f := newFake(t, "broken")
	var events []setuptoken.Event

	tok, err := f.maker.Make(t.Context(), setuptoken.Signing{Opener: &opener{}, Report: approving(t, f, &events)})

	screen, ok := errors.AsType[*setuptoken.ScreenError](err)
	if !ok {
		t.Fatalf("Make() = %q, %v, want a *ScreenError", tok.Reveal(), err)
	}
	if redact.HoldsToken(err.Error()) {
		t.Errorf("Make() = %q, want nothing token-shaped in the error", err)
	}
	if !redact.HoldsToken(screen.Screen()) {
		t.Errorf("the error's Screen() = %q, want the token setup-token drew, to copy", screen.Screen())
	}
	if got := steps(events); !slices.Contains(got, setuptoken.StepMade) {
		t.Errorf("Make reported %v, want the token made", got)
	}
}

func TestMakeFailsWhereSetupTokenDoesntHandItsAddressOver(t *testing.T) {
	f := newFake(t, "no-browser")
	maker := setuptoken.Waiting(f.maker, time.Minute, 2*time.Second)
	leaks := watchLeaks(t)
	open := &opener{}

	_, err := maker.Make(t.Context(), setuptoken.Signing{Opener: open})

	screen, ok := errors.AsType[*setuptoken.ScreenError](err)
	if !ok || !strings.Contains(screen.Screen(), "Opening browser to sign in…") {
		t.Fatalf("Make() = %v, want a *ScreenError with what setup-token drew", err)
	}
	if len(open.asked) > 0 {
		t.Errorf("the Opener was asked to open %q, want nothing", open.asked)
	}
	leaks.check(t, f)
}

func TestMakeOpensNothingButAnHTTPSAddress(t *testing.T) {
	f := newFake(t, "", "FAKE_ADDRESS=/System/Applications/Calculator.app")
	leaks := watchLeaks(t)
	open := &opener{}

	_, err := f.maker.Make(t.Context(), setuptoken.Signing{Opener: open})

	if _, ok := errors.AsType[*setuptoken.ScreenError](err); !ok {
		t.Errorf("Make() = %v, want a *ScreenError", err)
	}
	if len(open.asked) > 0 {
		t.Errorf("the Opener was asked to open %q, want nothing", open.asked)
	}
	leaks.check(t, f)
}

func TestMakeStopsSetupTokenWhenItsContextEndsWaitingOnTheApproval(t *testing.T) {
	f := newFake(t, "")
	leaks := watchLeaks(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	report := func(e setuptoken.Event) {
		if e.Step == setuptoken.StepWaiting {
			cancel()
		}
	}

	_, err := f.maker.Make(ctx, setuptoken.Signing{Opener: &opener{}, Report: report})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Make() = %v, want it cancelled", err)
	}
	leaks.check(t, f)
}

func TestMakeStopsSetupTokenWhenItsContextEndsOnceTheTokensMade(t *testing.T) {
	f := newFake(t, "", "FAKE_EXIT_AFTER=60")
	leaks := watchLeaks(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var events []setuptoken.Event
	approve := approving(t, f, &events)
	report := func(e setuptoken.Event) {
		approve(e)
		if e.Step == setuptoken.StepMade {
			cancel()
		}
	}

	_, err := f.maker.Make(ctx, setuptoken.Signing{Opener: &opener{}, Report: report})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Make() = %v, want it cancelled", err)
	}
	leaks.check(t, f)
}

func TestMakeStopsSetupTokenWhenItsContextEndsBeforeItHandsItsAddressOver(t *testing.T) {
	f := newFake(t, "stall")
	leaks := watchLeaks(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)

	go func() {
		_, err := f.maker.Make(ctx, setuptoken.Signing{Opener: &opener{}})
		result <- err
	}()
	f.noted(t, "pid")
	cancel()

	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Errorf("Make() = %v, want it cancelled", err)
	}
	leaks.check(t, f)
}

func TestMakeStopsSetupTokenWhenItsContextEndsWhileTheAddressOpens(t *testing.T) {
	f := newFake(t, "")
	leaks := watchLeaks(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var events []setuptoken.Event

	_, err := f.maker.Make(ctx, setuptoken.Signing{Opener: cancelling{cancel}, Report: approving(t, f, &events)})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Make() = %v, want it cancelled", err)
	}
	if len(events) > 0 {
		t.Errorf("Make reported %v, want nothing once cancelled", steps(events))
	}
	leaks.check(t, f)
}

func TestMakeStartsNothingWhenItsContextHasEnded(t *testing.T) {
	f := newFake(t, "")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := f.maker.Make(ctx, setuptoken.Signing{Opener: &opener{}})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Make() = %v, want it cancelled", err)
	}
	if f.ran() {
		t.Error("setup-token ran, want it never started")
	}
	if left, _ := os.ReadDir(f.shims); len(left) > 0 {
		t.Errorf("Make left %d in the temporary directory, want nothing", len(left))
	}
}

// cancelling is an Opener that cancels the context Make was given as it's
// asked to open the address.
type cancelling struct{ cancel context.CancelFunc }

func (c cancelling) Open(ctx context.Context, _ setuptoken.Address) (setuptoken.Opened, error) {
	c.cancel()
	return setuptoken.NotOpened, ctx.Err()
}

// The exit status a ScreenError carries is setup-token's own.
func TestMakeCarriesHowSetupTokenExited(t *testing.T) {
	f := newFake(t, "on-hold")

	_, err := f.maker.Make(t.Context(), setuptoken.Signing{Opener: &opener{}})

	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 1 {
		t.Errorf("Make() = %v, want it carrying setup-token's exit status, 1", err)
	}
}
