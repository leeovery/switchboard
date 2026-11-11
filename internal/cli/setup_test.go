package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/setup/screen"
	"github.com/leeovery/switchboard/internal/setuptoken"
	"github.com/leeovery/switchboard/internal/shell"
)

func TestSetupNeedsATerminal(t *testing.T) {
	tests := []struct {
		name          string
		input, output bool
	}{
		{name: "neither stdin nor stdout a terminal"},
		{name: "stdin alone a terminal", input: true},
		{name: "stdout alone a terminal", output: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			claudeConfig := filepath.Join(t.TempDir(), "claude")
			deps := testDeps(map[string]string{"CLAUDE_CONFIG_DIR": claudeConfig}, home)
			deps.TerminalInput = func(io.Reader) bool { return tt.input }
			deps.Terminal = func(io.Writer) bool { return tt.output }
			deps.SetupScreen = noScreen(t)

			got := run(t, deps, "setup")
			if want := (result{stderr: "Error: " + setup.ErrNoTerminal.Error() + "\n", code: 1}); got != want {
				t.Errorf("switchboard setup = %+v, want %+v", got, want)
			}
			for _, path := range []string{
				filepath.Join(home, ".config", "switchboard", "config.toml"),
				filepath.Join(home, "Library", "LaunchAgents", "io.github.leeovery.switchboard.plist"),
				claudeConfig,
			} {
				if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("%s: %v, want nothing written", path, err)
				}
			}
		})
	}
}

func TestSetupSaysItNeedsATerminalWhereTheScreenFindsNone(t *testing.T) {
	deps := setupDeps(t)
	deps.SetupScreen = func(context.Context, screen.Config, io.Writer, []string) (screen.Left, error) {
		return screen.Left{}, screen.ErrNotTerminal
	}

	got := run(t, deps, "setup")
	if want := (result{stderr: "Error: " + setup.ErrNoTerminal.Error() + "\n", code: 1}); got != want {
		t.Errorf("switchboard setup = %+v, want %+v", got, want)
	}
}

func TestSetupRefusesASwitchboardThatWontLast(t *testing.T) {
	deps := setupDeps(t)
	tmp := t.TempDir()
	deps.Getenv = withEnv(deps.Getenv, "TMPDIR", tmp)
	built := filepath.Join(tmp, "go-build", "switchboard")
	if err := os.MkdirAll(filepath.Dir(built), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(built, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	deps.Executable = func() (string, error) { return built, nil }
	deps.SetupScreen = noScreen(t)

	got := run(t, deps, "setup")
	if want := "which won't last: use one that does"; got.code != 1 || got.stdout != "" || !strings.Contains(got.stderr, want) {
		t.Errorf("switchboard setup = %+v, want exit status 1, an error saying %q, and nothing else", got, want)
	}
}

func TestSetupLeavesAsTheScreenWasLeft(t *testing.T) {
	const saved = "switchboard: interrupted once the token was saved, as it lasts a year on the account\n"
	stepFailed := errors.New("write the skill: permission denied")
	screenFailed := errors.New("run setup's screen: read the terminal: input/output error")
	tests := []struct {
		name      string
		left      screen.Left
		screenErr error
		// want is all setup printed once the screen had gone, stdout and
		// stderr as one, as a terminal shows them.
		want     string
		wantCode int
		// wantDashboard is set where the dashboard opens.
		wantDashboard bool
	}{
		{name: "q before Done", left: screen.Left{Ending: screen.Paused}, want: "run setup again to carry on\n"},
		{name: "q at Done", left: screen.Left{Ending: screen.Finished}},
		{name: "enter at Done: the dashboard", left: screen.Left{Ending: screen.ToDashboard}, want: "the dashboard\n", wantDashboard: true},
		{name: "a step that failed", left: screen.Left{Ending: screen.Failed, Err: stepFailed}, want: "Error: " + stepFailed.Error() + "\n", wantCode: 1},
		{name: "the screen failing", left: screen.Left{Ending: screen.Paused}, screenErr: screenFailed, want: "Error: " + screenFailed.Error() + "\n", wantCode: 1},
		{name: "the screen failing at Done by enter: no dashboard", left: screen.Left{Ending: screen.ToDashboard}, screenErr: screenFailed, want: "Error: " + screenFailed.Error() + "\n", wantCode: 1},
		{
			name:      "the screen failing at a step that failed: both",
			left:      screen.Left{Ending: screen.Failed, Err: stepFailed},
			screenErr: screenFailed,
			want:      "Error: " + screenFailed.Error() + "\n" + stepFailed.Error() + "\n",
			wantCode:  1,
		},
		{name: "an interrupt held before Done: the token saved first", left: screen.Left{Ending: screen.Paused, Interrupted: true}, want: saved + "Error: interrupted\n", wantCode: 130},
		{name: "an interrupt held at Done", left: screen.Left{Ending: screen.ToDashboard, Interrupted: true}, want: saved + "Error: interrupted\n", wantCode: 130},
		{
			name:     "an interrupt held at a step that failed",
			left:     screen.Left{Ending: screen.Failed, Err: stepFailed, Interrupted: true},
			want:     saved + "Error: interrupted: " + stepFailed.Error() + "\n",
			wantCode: 130,
		},
		{
			name:      "an interrupt held as the screen failed",
			left:      screen.Left{Ending: screen.Paused, Interrupted: true},
			screenErr: screenFailed,
			want:      saved + "Error: interrupted: " + screenFailed.Error() + "\n",
			wantCode:  130,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := setupDeps(t)
			var happened []string
			deps.SetupScreen = func(context.Context, screen.Config, io.Writer, []string) (screen.Left, error) {
				happened = append(happened, "screen")
				return tt.left, tt.screenErr
			}
			deps.Watch = func(_ context.Context, _ watch.Config, out io.Writer, _ []string) error {
				happened = append(happened, "dashboard")
				_, _ = io.WriteString(out, "the dashboard\n")
				return nil
			}

			printed, code := runSeen(t, deps, "setup")
			if printed != tt.want || code != tt.wantCode {
				t.Errorf("switchboard setup printed %q, exit status %d; want %q, %d", printed, code, tt.want, tt.wantCode)
			}
			want := []string{"screen"}
			if tt.wantDashboard {
				want = append(want, "dashboard")
			}
			if !slices.Equal(happened, want) {
				t.Errorf("setup ran %q, want %q", happened, want)
			}
		})
	}
}

// stepRow is a step's line as setup prints it: its mark at the third
// column, its label at the sixth, and what it says at the 27th, as Done
// draws it.
func stepRow(mark, label, detail string) string {
	return strings.TrimRight(fmt.Sprintf("  %s  %-21s%s", mark, label, detail), " ")
}

// underRow is what a step's line leaves to add by hand, as setup prints it
// under the line, whole, from the label's column.
func underRow(add string) string {
	return strings.Repeat(" ", 5) + add
}

func TestSetupPrintsEachStepsLineAsItLeaves(t *testing.T) {
	const (
		saved = "switchboard: interrupted once the token was saved, as it lasts a year on the account\n"
		line  = `if [ -r "$HOME/.local/share/switchboard/shell/init.zsh" ]; then . "$HOME/.local/share/switchboard/shell/init.zsh"; fi  # switchboard`
		dir   = "$HOME/.local/share/switchboard/bin"
		why   = "it failed, as setup says"
	)
	working := setup.TokenStanding{State: setup.TokenWorking}
	review := &setup.Review{Accounts: []setup.Reviewed{{ID: "work", Primary: true, Token: working}, {ID: "personal", Token: working}}}
	primed := &setup.Primed{Day: config.Day{Start: 8 * time.Hour, End: 23 * time.Hour}}
	serviced := &setup.Serviced{Said: []setup.Said{{Mark: setup.Done, Label: "Installed", Detail: "~/Library/LaunchAgents/io.github.leeovery.switchboard.plist"}}, Router: &router.Health{OK: true}}
	byHand := &setup.LinkFound{Made: setup.LinkCreated, Shell: setup.ShellFound{Situation: setup.OfferLine, Where: "~/.zshrc", Line: line, Dir: dir}}
	unchecked := &setup.LinkFound{Made: setup.LinkCreated, Shell: setup.ShellFound{Situation: setup.UnknownShell, Dir: dir}}
	skill := &setup.Said{Mark: setup.Done, Label: "Installed", Detail: "~/.claude/skills/switchboard/SKILL.md"}
	var (
		accountsRow = stepRow("✓", "Accounts", "2, all working · primary: work")
		primingRow  = stepRow("✓", "Priming", "on, 08:00–23:00")
		serviceRow  = stepRow("✓", "Service", "installed · router healthy")
		linkRows    = []string{stepRow("!", "claude link", "created, but it isn't first on your PATH. Add this line to ~/.zshrc:"), underRow(line)}
		dirRows     = []string{stepRow("!", "claude link", "created, but setup can't check it for your shell. Put this directory first on your PATH:"), underRow(dir)}
		skillRow    = stepRow("✓", "Skill", "installed")
		everyRow    = slices.Concat([]string{accountsRow, primingRow, serviceRow}, linkRows, []string{skillRow})
	)
	stepFailed := errors.New(why)
	screenFailed := errors.New("run setup's screen: read the terminal: input/output error")
	tests := []struct {
		name      string
		left      screen.Left
		screenErr error
		// rows are the steps' lines setup prints first, then leaving what it
		// prints after them, and wantCode its exit status.
		rows     []string
		leaving  string
		wantCode int
		// wantDashboard is set where the dashboard opens, after.
		wantDashboard bool
	}{
		{
			name:    "q mid-way",
			left:    screen.Left{Ending: screen.Paused, Summary: setup.Summary{At: setup.StepService, Review: review, Primed: primed, Service: serviced}},
			rows:    []string{accountsRow, primingRow, serviceRow},
			leaving: "run setup again to carry on\n",
		},
		{
			name: "q at Done: the line left to add by hand under the claude link's",
			left: screen.Left{Ending: screen.Finished, Summary: setup.Summary{At: setup.StepDone, Review: review, Primed: primed, Service: serviced, Link: byHand, Skill: skill}},
			rows: everyRow,
		},
		{
			name:          "enter at Done: before the dashboard",
			left:          screen.Left{Ending: screen.ToDashboard, Summary: setup.Summary{At: setup.StepDone, Review: review, Primed: primed, Service: serviced, Link: byHand, Skill: skill}},
			rows:          everyRow,
			leaving:       "the dashboard\n",
			wantDashboard: true,
		},
		{
			name:     "Accounts failed",
			left:     screen.Left{Ending: screen.Failed, Err: stepFailed, Summary: setup.Summary{At: setup.StepAccounts, Failure: why}},
			rows:     []string{stepRow("✗", "Accounts", why)},
			leaving:  "Error: " + why + "\n",
			wantCode: 1,
		},
		{
			name:     "Priming failed",
			left:     screen.Left{Ending: screen.Failed, Err: stepFailed, Summary: setup.Summary{At: setup.StepPriming, Failure: why, Review: review}},
			rows:     []string{accountsRow, stepRow("✗", "Priming", why)},
			leaving:  "Error: " + why + "\n",
			wantCode: 1,
		},
		{
			name:     "Service failed",
			left:     screen.Left{Ending: screen.Failed, Err: stepFailed, Summary: setup.Summary{At: setup.StepService, Failure: why, Review: review, Primed: primed}},
			rows:     []string{accountsRow, primingRow, stepRow("✗", "Service", why)},
			leaving:  "Error: " + why + "\n",
			wantCode: 1,
		},
		{
			name:     "the claude link failed",
			left:     screen.Left{Ending: screen.Failed, Err: stepFailed, Summary: setup.Summary{At: setup.StepLink, Failure: why, Review: review, Primed: primed, Service: serviced}},
			rows:     []string{accountsRow, primingRow, serviceRow, stepRow("✗", "claude link", why)},
			leaving:  "Error: " + why + "\n",
			wantCode: 1,
		},
		{
			name:     "Skill failed: the directory left to put first on PATH under the claude link's",
			left:     screen.Left{Ending: screen.Failed, Err: stepFailed, Summary: setup.Summary{At: setup.StepSkill, Failure: why, Review: review, Primed: primed, Service: serviced, Link: unchecked}},
			rows:     slices.Concat([]string{accountsRow, primingRow, serviceRow}, dirRows, []string{stepRow("✗", "Skill", why)}),
			leaving:  "Error: " + why + "\n",
			wantCode: 1,
		},
		{
			name:     "an interrupt held as a token was saved: the summary, then the token saved first",
			left:     screen.Left{Ending: screen.Paused, Interrupted: true, Summary: setup.Summary{At: setup.StepAccounts, Review: review}},
			rows:     []string{accountsRow},
			leaving:  saved + "Error: interrupted\n",
			wantCode: 130,
		},
		{
			name: "a token the screen gave back: the summary, then the token",
			left: screen.Left{Ending: screen.Paused, Unsaved: []screen.TokenLeft{{For: "personal", Tokened: setup.Tokened{Result: setup.TokenRefused}}}, Summary: setup.Summary{At: setup.StepAccounts, Review: review}},
			rows: []string{accountsRow},
			leaving: "The token was refused\nThe API refused the token, so it isn't saved.\n" +
				"Run switchboard setup again to try again, or to paste a token you made with claude setup-token.\n" +
				"run setup again to carry on\n",
		},
		{
			name: "tokens the screen gave back: the summary, then each, in order, led by its account",
			left: screen.Left{Ending: screen.Paused, Unsaved: []screen.TokenLeft{
				{For: "personal", Tokened: setup.Tokened{Result: setup.TokenNotSaved, Why: "the config changed", Shown: []string{"test-token-personal"}}},
				{For: "team", Tokened: setup.Tokened{Result: setup.TokenRefused}},
			}, Summary: setup.Summary{At: setup.StepAccounts, Review: review}},
			rows: []string{accountsRow},
			leaving: "For personal:\nThe token was created, but setup couldn't save it\nthe config changed\n  test-token-personal\n" +
				"It lasts a year on the account, so copy it, then once that's put right, run switchboard setup again, and press p on its token card to paste it.\n" +
				"\nFor team:\nThe token was refused\nThe API refused the token, so it isn't saved.\n" +
				"Run switchboard setup again to try again, or to paste a token you made with claude setup-token.\n" +
				"run setup again to carry on\n",
		},
		{
			name:      "the screen failing: the summary, then why",
			left:      screen.Left{Ending: screen.Paused, Summary: setup.Summary{At: setup.StepService, Review: review, Primed: primed, Service: serviced}},
			screenErr: screenFailed,
			rows:      []string{accountsRow, primingRow, serviceRow},
			leaving:   "Error: " + screenFailed.Error() + "\n",
			wantCode:  1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := setupDeps(t)
			deps.Getenv = withEnv(deps.Getenv, "COLUMNS", "160")
			var happened []string
			deps.SetupScreen = func(_ context.Context, _ screen.Config, out io.Writer, _ []string) (screen.Left, error) {
				happened = append(happened, "screen")
				_, _ = io.WriteString(out, "the screen\n")
				return tt.left, tt.screenErr
			}
			deps.Watch = func(_ context.Context, _ watch.Config, out io.Writer, _ []string) error {
				happened = append(happened, "dashboard")
				_, _ = io.WriteString(out, "the dashboard\n")
				return nil
			}

			printed, code := runSeen(t, deps, "setup")
			want := "the screen\n" + strings.Join(tt.rows, "\n") + "\n" + tt.leaving
			if printed != want || code != tt.wantCode {
				t.Errorf("switchboard setup printed\n%s\nexit status %d; want\n%s\nexit status %d", printed, code, want, tt.wantCode)
			}
			wantRan := []string{"screen"}
			if tt.wantDashboard {
				wantRan = append(wantRan, "dashboard")
			}
			if !slices.Equal(happened, wantRan) {
				t.Errorf("setup ran %q, want %q", happened, wantRan)
			}
		})
	}
}

func TestSetupShowsATokenItCouldntSaveOnceTheScreenHasGone(t *testing.T) {
	screenFailed := errors.New("run setup's screen: read the terminal: input/output error")
	tests := []struct {
		name    string
		unsaved setup.Tokened
		// interrupted has an interrupt held as the token was saved, and
		// screenErr the screen failing.
		interrupted bool
		screenErr   error
		want        []string
		// leaving is what setup says last, as it leaves, and wantCode its
		// exit status.
		leaving  string
		wantCode int
	}{
		{
			name:    "made but unread",
			unsaved: setup.Tokened{Result: setup.TokenUnread, Shown: []string{" Your token:", " test-token-personal"}},
			want: []string{
				"The token was created, but setup couldn't read it",
				"It lasts a year on the account, so copy it from what claude setup-token showed:",
				"  Your token:",
				"  test-token-personal",
				"Then run switchboard setup again, and press p on its token card to paste it.",
			},
			leaving: "run setup again to carry on\n",
		},
		{
			name:        "made but not saved, an interrupt held: nothing said of its being saved",
			unsaved:     setup.Tokened{Result: setup.TokenNotSaved, Why: "the config changed", Shown: []string{"test-token-personal"}, InterruptHeld: true},
			interrupted: true,
			want: []string{
				"The token was created, but setup couldn't save it",
				"the config changed",
				"  test-token-personal",
				"It lasts a year on the account, so copy it, then once that's put right, run switchboard setup again, and press p on its token card to paste it.",
			},
			leaving:  "Error: interrupted\n",
			wantCode: 130,
		},
		{
			name:        "refused, an interrupt held as the screen failed",
			unsaved:     setup.Tokened{Result: setup.TokenRefused, InterruptHeld: true},
			interrupted: true,
			screenErr:   screenFailed,
			want: []string{
				"The token was refused",
				"The API refused the token, so it isn't saved.",
				"Run switchboard setup again to try again, or to paste a token you made with claude setup-token.",
			},
			leaving:  "Error: interrupted: " + screenFailed.Error() + "\n",
			wantCode: 130,
		},
		{
			name:    "the same account's",
			unsaved: setup.Tokened{Result: setup.TokenSameAccount, SameAs: "work"},
			want: []string{
				"The token is for work's subscription",
				"work has that subscription already, so the token isn't saved.",
				"Run switchboard setup again to try again, signed in to the other subscription, or to paste a token you made with claude setup-token.",
			},
			leaving: "run setup again to carry on\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := setupDeps(t)
			deps.SetupScreen = func(_ context.Context, _ screen.Config, out io.Writer, _ []string) (screen.Left, error) {
				_, _ = io.WriteString(out, "the screen\n")
				unsaved := tt.unsaved
				return screen.Left{Ending: screen.Paused, Unsaved: []screen.TokenLeft{{For: "personal", Tokened: unsaved}}, Interrupted: tt.interrupted}, tt.screenErr
			}

			printed, code := runSeen(t, deps, "setup")
			want := "the screen\n" + strings.Join(tt.want, "\n") + "\n" + tt.leaving
			if printed != want || code != tt.wantCode {
				t.Errorf("switchboard setup printed\n%s\nexit status %d; want\n%s\nexit status %d", printed, code, want, tt.wantCode)
			}
		})
	}
}

// From the screen going with a token's making in hand, as it tells Holding,
// setup holds the interrupts through the printing of a making it gave back,
// one coming then ending setup as interrupted once it's printed whole,
// saying nothing of a save; and with none given back, lets them go before
// it prints anything, an interrupt as it prints ending it as one always has.
func TestSetupHoldsTheInterruptsTillATokenGivenBackIsShown(t *testing.T) {
	const carryOn = "run setup again to carry on"
	unread := &setup.Tokened{Result: setup.TokenUnread, Shown: []string{" Your token:", " test-token-personal"}}
	accountsRow := stepRow("✓", "Accounts", "2, all working · primary: work")
	shown := []string{
		"The token was created, but setup couldn't read it",
		"It lasts a year on the account, so copy it from what claude setup-token showed:",
		"  Your token:",
		"  test-token-personal",
		"Then run switchboard setup again, and press p on its token card to paste it.",
	}
	tests := []struct {
		name string
		// holding has the screen tell Holding, as it does going with a
		// token's making in hand, and goneInterrupt then sends an interrupt,
		// as one comes while its jobs are waited for; unsaved is the making
		// it gives back.
		holding, goneInterrupt bool
		unsaved                []screen.TokenLeft
		// interruptAt is the line printed as an interrupt is sent.
		interruptAt string
		want        result
		// heldAt is how many holds held the interrupts as it was sent,
		// holds how many held them in all.
		heldAt, holds int
	}{
		{
			name:        "a token given back, an interrupt as its last line prints",
			holding:     true,
			unsaved:     []screen.TokenLeft{{For: "personal", Tokened: *unread}},
			interruptAt: shown[len(shown)-1],
			want:        result{stdout: lines(slices.Concat([]string{"the screen", accountsRow}, shown)...), stderr: "Error: interrupted\n", code: 130},
			heldAt:      1,
			holds:       1,
		},
		{
			name:        "a token given back, an interrupt as the summary prints",
			holding:     true,
			unsaved:     []screen.TokenLeft{{For: "personal", Tokened: *unread}},
			interruptAt: accountsRow,
			want:        result{stdout: lines(slices.Concat([]string{"the screen", accountsRow}, shown)...), stderr: "Error: interrupted\n", code: 130},
			heldAt:      1,
			holds:       1,
		},
		{
			name:    "a token given back, no interrupt",
			holding: true,
			unsaved: []screen.TokenLeft{{For: "personal", Tokened: *unread}},
			want:    result{stdout: lines(slices.Concat([]string{"the screen", accountsRow}, shown, []string{carryOn})...)},
			holds:   1,
		},
		{
			// Ended as interrupted, saying nothing of a token saved.
			name:          "none given back, an interrupt as the screen's jobs end",
			holding:       true,
			goneInterrupt: true,
			want:          result{stdout: lines("the screen", accountsRow), stderr: "Error: interrupted\n", code: 130},
			holds:         1,
		},
		{
			name:        "none given back, an interrupt as the summary prints",
			holding:     true,
			interruptAt: accountsRow,
			want:        result{stdout: lines("the screen", accountsRow, carryOn)},
			holds:       1,
		},
		{
			name:        "no token's making in hand, an interrupt as the summary prints",
			interruptAt: accountsRow,
			want:        result{stdout: lines("the screen", accountsRow, carryOn)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			held := &heldInterrupts{}
			deps := setupDeps(t)
			deps.Getenv = withEnv(deps.Getenv, "COLUMNS", "160")
			deps.Hold = held.hold
			working := setup.TokenStanding{State: setup.TokenWorking}
			review := &setup.Review{Accounts: []setup.Reviewed{{ID: "work", Primary: true, Token: working}, {ID: "personal", Token: working}}}
			deps.SetupScreen = func(_ context.Context, cfg screen.Config, out io.Writer, _ []string) (screen.Left, error) {
				_, _ = io.WriteString(out, "the screen\n")
				if tt.holding {
					cfg.Holding()
					cfg.Holding()
				}
				if tt.goneInterrupt {
					held.send(os.Interrupt)
				}
				return screen.Left{Ending: screen.Paused, Summary: setup.Summary{Review: review}, Unsaved: tt.unsaved}, nil
			}

			got, stdout := runWriting(t, deps, &holdingWriter{held: held, interruptAt: tt.interruptAt}, "setup")
			if got != tt.want {
				t.Errorf("switchboard setup =\n%+v\nwant\n%+v", got, tt.want)
			}
			if stdout.heldAt != tt.heldAt {
				t.Errorf("%d holds held the interrupts as one was sent, want %d", stdout.heldAt, tt.heldAt)
			}
			if active, holds := held.state(); active != 0 || holds != tt.holds {
				t.Errorf("the interrupts were held %d times, %d holding them still; want %d, none still", holds, active, tt.holds)
			}
			for _, line := range stdout.unheld {
				if slices.Contains(shown, line) {
					t.Errorf("%q, of the token given back, was printed once the interrupts were let go, want every line of it printed while they're held", line)
				}
			}
		})
	}
}

func TestSetupsScreenIsSetupsOwn(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		// wantPair is set where the screen is drawn in colour.
		wantPair bool
		wantSize screen.Size
	}{
		{name: "in colour, at the default size", wantPair: true, wantSize: screen.Size{Width: 80, Height: 24}},
		{name: "without colour, at the environment's size", env: map[string]string{"NO_COLOR": "1", "COLUMNS": "120", "LINES": "40"}, wantSize: screen.Size{Width: 120, Height: 40}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := setupDeps(t)
			for key, value := range tt.env {
				deps.Getenv = withEnv(deps.Getenv, key, value)
			}
			writePrefs(t, deps, `{"theme_light": "lake", "theme_dark": "amber"}`)
			writeTheme(t, themesDir(t, deps), "lake", lakeTheme)
			browser, private := &fakeOpener{name: "browser"}, &fakeOpener{name: "private window"}
			deps.Browser, deps.PrivateWindow = browser, private
			var given screen.Config
			deps.SetupScreen = func(_ context.Context, cfg screen.Config, _ io.Writer, environ []string) (screen.Left, error) {
				if !sameEnviron(environ, deps.Environ()) {
					t.Errorf("the screen given the environment %q, want the command's %q", environ, deps.Environ())
				}
				given = cfg
				return screen.Left{Ending: screen.Finished}, nil
			}

			if got := run(t, deps, "setup"); got.code != 0 {
				t.Fatalf("switchboard setup = %+v, want exit status 0", got)
			}
			s, ok := given.Steps.(*setup.Setup)
			if !ok {
				t.Fatalf("the screen's Steps are %T, want setup's own", given.Steps)
			}
			if given.Step != 0 || given.Size != tt.wantSize || given.Now == nil || !given.Now().Equal(testNow) {
				t.Errorf("the screen opened on step %d, at %+v; want the first step, at %+v, by the command's clock", given.Step, given.Size, tt.wantSize)
			}
			switch {
			case !tt.wantPair && given.Pair != nil:
				t.Errorf("the screen is drawn in %s and %s, want it without colour", given.Pair.Light.Slug, given.Pair.Dark.Slug)
			case tt.wantPair && (given.Pair == nil || given.Pair.Light.Slug != "lake" || given.Pair.Dark.Slug != "amber"):
				t.Errorf("the screen is drawn in %+v, want lake, from the themes directory, and amber, as the dashboard's choice keeps them", given.Pair)
			}
			if s.Hold != nil || s.Saving != nil {
				t.Error("setup holds interrupts of its own, or tells Saving of one held; want the process's held, the screen's Left saying so")
			}
			switch finishing := reflect.ValueOf(s.LaunchAgent).Elem().FieldByName("cfg").FieldByName("Finishing"); {
			case !finishing.IsValid():
				t.Error("the service's config has no Finishing to look at, want it found")
			case !finishing.IsNil():
				t.Error("the service tells of an interrupt it waits for, which would print over the screen; want it telling none")
			}
			maker, ok := s.TokenMaker.(setuptoken.Maker)
			if !ok || maker.Executable == nil || !sameEnviron(maker.Environ, deps.Environ()) || !slices.Equal(maker.InstallPaths, s.InstallPaths) {
				t.Errorf("setup makes tokens with %#v, want setuptoken's Maker, in the command's environment, looking where Claude Code installs", s.TokenMaker)
			}
			if s.Browser != setuptoken.Opener(browser) || s.PrivateWindow != setuptoken.Opener(private) {
				t.Errorf("setup opens sign-ins with %#v and %#v, want the command's own, in turn", s.Browser, s.PrivateWindow)
			}
		})
	}
}

// fakeOpener stands in for an opener of a sign-in address, which no test
// opens, told apart from another by its name.
type fakeOpener struct {
	name string
}

func (*fakeOpener) Open(context.Context, setuptoken.Address) (setuptoken.Opening, error) {
	return setuptoken.Opening{}, errors.New("no test opens a sign-in address")
}

func TestSetupsHelpDescribesTheScreen(t *testing.T) {
	got := run(t, testDeps(nil, t.TempDir()), "setup", "--help")
	help := strings.Join(strings.Fields(got.stdout), " ")
	for _, want := range []string{
		"Walk through setting switchboard up, full screen, a page a step",
		"At Done, the last, enter opens the dashboard, and q leaves without it; q at any step before leaves too, and setup run again carries on.",
		"It needs a terminal: without one, it says which command takes each step alone.",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("switchboard setup --help printed\n%s\nwant it to say %q", got.stdout, want)
		}
	}
	if strings.Contains(help, "every account's windows") {
		t.Errorf("switchboard setup --help printed\n%s\nwant nothing said of printing usage, which the screen doesn't", got.stdout)
	}
}

func TestSetupsStepsAreSetUpAsTheCLIFindsThem(t *testing.T) {
	s := newServiceSetup(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "data")
	bin := filepath.Join(data, "switchboard", "bin")
	realClaude := claudetest.Program(t, filepath.Join(root, "claude-code", "claude"))
	claudeConfig := filepath.Join(root, "claude")
	s.setenv("XDG_DATA_HOME", data)
	s.setenv("PATH", filepath.Dir(realClaude))
	s.setenv("CLAUDE_CONFIG_DIR", claudeConfig)
	s.setenv("SHELL", "/bin/zsh")
	s.setenv("LANG", "en_GB.UTF-8")
	home, err := s.srv.deps.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	// The login shell finds the real claude, then, once switchboard's line is
	// added, the link; and says it reads .zshrc in the home.
	var asked [][]string
	s.srv.deps.LoginShell = func(_ context.Context, path string, args, env []string) ([]byte, error) {
		asked = append(asked, slices.Concat([]string{path}, args, env))
		found := realClaude
		if len(asked) > 1 {
			found = filepath.Join(bin, "claude")
		}
		return []byte("switchboard-asks-which-claude\nis:" + found + "\npath:" + found + "\nconfig:" + home + "\nswitchboard-asked-which-claude\n"), nil
	}
	atATerminal(&s.srv.deps)
	var said []string
	var reviewed map[string]setup.TokenState
	s.srv.deps.SetupScreen = func(ctx context.Context, cfg screen.Config, _ io.Writer, _ []string) (screen.Left, error) {
		reviewed = takeAccountsAndPriming(t, ctx, cfg.Steps)
		said = takeSteps(t, ctx, cfg.Steps)
		return screen.Left{Ending: screen.Finished}, nil
	}

	if got := run(t, s.srv.deps, "setup"); got != (result{}) {
		t.Fatalf("switchboard setup = %+v, want exit status 0, and nothing printed", got)
	}
	link := filepath.Join(bin, "claude")
	skill := filepath.Join(claudeConfig, "skills", "switchboard", "SKILL.md")
	settings := filepath.Join(claudeConfig, "settings.json")
	shellDir := filepath.Join(data, "switchboard", "shell")
	line := `if [ -r "` + shellDir + `/init.zsh" ]; then . "` + shellDir + `/init.zsh"; fi  # switchboard`
	want := []string{
		"Installed: ~/Library/LaunchAgents/io.github.leeovery.switchboard.plist",
		"Router: healthy",
		"Link: ",
		"Your shell: ",
		"Session hook: ",
		"Installed: ",
	}
	if len(said) != len(want) {
		t.Errorf("setup's steps said\n%s\nwant a row each of\n%s", strings.Join(said, "\n"), strings.Join(want, "\n"))
	}
	for i, w := range want {
		if i >= len(said) || !strings.HasPrefix(said[i], w) {
			t.Errorf("setup's steps said\n%s\nwant row %d to begin\n%s", strings.Join(said, "\n"), i, w)
		}
	}
	// The prober asks the CLI's API of the tokens in the CLI's token files:
	// it takes work's, and refuses side's, and personal has none.
	if want := map[string]setup.TokenState{"work": setup.TokenWorking, "personal": setup.NoToken, "side": setup.TokenNotTaken}; !maps.Equal(reviewed, want) {
		t.Errorf("the accounts were reviewed as %v, want %v", reviewed, want)
	}
	written, err := config.Load(s.srv.config)
	if err != nil {
		t.Fatal(err)
	}
	if primary, day := written.Accounts.Primary().ID, written.Prime.Day.String(); primary != "work" || day != "08:00-23:00" {
		t.Errorf("the config the CLI finds marks %q the primary, its day %s; want work, and 08:00-23:00", primary, day)
	}
	s.checkPlist(t, "<key>CLAUDE_CONFIG_DIR</key>\n\t\t<string>"+claudeConfig+"</string>")
	if target, err := os.Readlink(link); err != nil || target != s.binary {
		t.Errorf("%s leads to %q (%v), want %s", link, target, err, s.binary)
	}
	for _, path := range []string{skill, settings} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s: %v, want it written", path, err)
		}
	}
	for _, sh := range shell.Shells {
		path := shell.InitFile(shellDir, sh)
		if got, want := readFile(t, path), string(shellFile(t, s.srv.deps, sh, shellDir, bin)); got != want {
			t.Errorf("%s reads\n%s\nwant this switchboard's, its completions loaded\n%s", path, got, want)
		}
	}
	if path := shell.CompletionFile(shellDir); readFile(t, path) != string(completion(t, s.srv.deps, shell.Bash)) {
		t.Errorf("%s reads\n%s\nwant this switchboard's completions for bash", path, readFile(t, path))
	}
	if rc := readFile(t, filepath.Join(home, ".zshrc")); rc != line+"\n" {
		t.Errorf("~/.zshrc reads %q, want switchboard's line alone", rc)
	}
	// The question itself is the shell package's to pin: here, that it's
	// asked of an interactive login zsh, in the environment a new terminal
	// gives it.
	env := []string{"LANG=en_GB.UTF-8", "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "SHELL=/bin/zsh", "TMPDIR=" + s.srv.deps.Getenv("TMPDIR")}
	wantAsked := slices.Concat([]string{"/bin/zsh", "-l", "-i", "-c", "<question>"}, env)
	for i := range asked {
		if len(asked[i]) > 4 && strings.HasPrefix(asked[i][4], "printf '%s\\n' switchboard-asks-which-claude; ") {
			asked[i][4] = "<question>"
		}
		slices.Sort(asked[i][min(5, len(asked[i])):])
	}
	if len(asked) != 2 || !slices.Equal(asked[0], wantAsked) || !slices.Equal(asked[1], wantAsked) {
		t.Errorf("the login shell was asked %q, want twice, as %q: first, and once the line's added", asked, wantAsked)
	}
}

// takeAccountsAndPriming takes the Accounts and Priming steps as a user
// would: the accounts reviewed, each token checked, work made the primary,
// and priming's day set where none is. It returns how each account's token
// stood, by its id.
func takeAccountsAndPriming(t *testing.T, ctx context.Context, steps screen.Steps) map[string]setup.TokenState {
	t.Helper()
	review, err := steps.Review()
	if err != nil {
		t.Fatalf("reviewing the accounts: %v", err)
	}
	if !review.Unmarked {
		t.Error("the review finds a primary, want three accounts and none marked")
	}
	reviewed := make(map[string]setup.TokenState)
	for _, a := range review.Accounts {
		row, err := steps.ReviewOf(ctx, a.ID)
		if err != nil {
			t.Fatalf("checking %s's token: %v", a.ID, err)
		}
		reviewed[a.ID] = row.Token.State
	}
	if _, err := steps.EditAccount("work", setup.Edit{Primary: true}); err != nil {
		t.Fatalf("making work the primary: %v", err)
	}
	primed, err := steps.Priming()
	if err != nil || primed != (setup.Primed{}) {
		t.Fatalf("Priming() = %v, %v; want priming off", primed, err)
	}
	if _, err := steps.SetDay("08:00-23:00"); err != nil {
		t.Fatalf("setting priming's day: %v", err)
	}
	return reviewed
}

// takeSteps takes the steps the screen takes past Accounts, as a user
// pressing enter through them does, adding switchboard's line where the
// claude link step offers it, and returns the rows they said, each its
// label and detail.
func takeSteps(t *testing.T, ctx context.Context, steps screen.Steps) []string {
	t.Helper()
	var said []string
	tell := func(all ...setup.Said) {
		for _, s := range all {
			said = append(said, s.Label+": "+s.Detail)
		}
	}
	serviced, err := steps.Service(ctx)
	if err != nil {
		t.Fatalf("the service step: %v", err)
	}
	tell(serviced.Said...)
	found, err := steps.Link(ctx)
	if err != nil {
		t.Fatalf("the claude link step: %v", err)
	}
	tell(found.Said...)
	if found.Shell.Situation == setup.OfferLine {
		if found, err = steps.AddLine(found); err != nil {
			t.Fatalf("adding switchboard's line: %v", err)
		}
		if found, err = steps.AskAgain(ctx, found); err != nil {
			t.Fatalf("asking the shell again: %v", err)
		}
	}
	tell(found.Shell.Said, steps.SessionHook())
	skillSaid, err := steps.Skill()
	if err != nil {
		t.Fatalf("the skill step: %v", err)
	}
	tell(skillSaid)
	return said
}

// setupDeps are commands' deps for setup at a terminal, with accounts to
// set up, and a switchboard that lasts, whose screen fails the test where
// it's run: a test that has it run gives its own.
func setupDeps(t *testing.T) cli.Deps {
	t.Helper()
	deps, _ := accountsDeps(t, fakeClaudeAPI(t), personalAndSide)
	binary := filepath.Join(t.TempDir(), "switchboard")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	deps.Executable = func() (string, error) { return binary, nil }
	atATerminal(&deps)
	deps.SetupScreen = noScreen(t)
	return deps
}

// noScreen stands in for setup's screen where it mustn't run.
func noScreen(t *testing.T) func(context.Context, screen.Config, io.Writer, []string) (screen.Left, error) {
	return func(context.Context, screen.Config, io.Writer, []string) (screen.Left, error) {
		t.Error("setup's screen ran")
		return screen.Left{}, nil
	}
}

// withEnv is getenv, but for key, which it gives as value.
func withEnv(getenv func(string) string, key, value string) func(string) string {
	return func(k string) string {
		if k == key {
			return value
		}
		return getenv(k)
	}
}

// runSeen executes the command line in-process with deps, as runWithInput
// does, but with stdout and stderr as one, as a terminal shows them, so what
// each printed is in the order it was printed.
func runSeen(t *testing.T, deps cli.Deps, args ...string) (string, int) {
	t.Helper()
	root := cli.NewRootCommand(deps)
	var seen bytes.Buffer
	root.SetArgs(args)
	root.SetIn(strings.NewReader(""))
	root.SetOut(&seen)
	root.SetErr(&seen)
	code := cli.Execute(root)
	return seen.String(), code
}

// atATerminal has commands run with deps take stdin and stdout for a
// terminal.
func atATerminal(deps *cli.Deps) {
	deps.TerminalInput = func(io.Reader) bool { return true }
	deps.Terminal = func(io.Writer) bool { return true }
}

// sameEnviron reports whether two environments hold the same variables, in
// whatever order.
func sameEnviron(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}
