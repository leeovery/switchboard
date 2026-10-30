package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
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
				return screen.Left{Ending: screen.Paused, Unsaved: &unsaved, Interrupted: tt.interrupted}, tt.screenErr
			}

			printed, code := runSeen(t, deps, "setup")
			want := "the screen\n" + strings.Join(tt.want, "\n") + "\n" + tt.leaving
			if printed != want || code != tt.wantCode {
				t.Errorf("switchboard setup printed\n%s\nexit status %d; want\n%s\nexit status %d", printed, code, want, tt.wantCode)
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
			if s.Hold != nil || s.Saving != nil || s.Usage != nil {
				t.Error("setup holds interrupts of its own, tells Saving of one held, or prints usage; want the process's held, the screen's Left saying so, and no usage")
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
		"Installed " + s.plist + ": ",
		"The router is up: healthy, pid " + strconv.Itoa(os.Getpid()) + ".",
		"Linked " + link + " to " + s.binary + ".",
		"Wrote switchboard's files for the shells, in " + shellDir + ": init.zsh, init.bash, completion.bash and init.fish.",
		"claude goes through switchboard in a new terminal: zsh runs " + link + ".",
		"Installed the session hook, which tells a session that doesn't go through switchboard so: " + settings,
		"Installed the skill, which tells Claude what switchboard does under claude: " + skill,
	}
	for _, w := range want {
		if !slices.ContainsFunc(said, func(s string) bool { return strings.HasPrefix(s, w) }) {
			t.Errorf("setup's steps said\n%s\nwant one saying\n%s", strings.Join(said, "\n"), w)
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
	day, err := steps.Priming()
	if err != nil || day != (config.Day{}) {
		t.Fatalf("Priming() = %v, %v; want priming off", day, err)
	}
	if _, err := steps.SetDay("08:00-23:00"); err != nil {
		t.Fatalf("setting priming's day: %v", err)
	}
	return reviewed
}

// takeSteps takes the steps the screen takes past Accounts, as a user
// pressing enter through them does, adding switchboard's line where the
// claude link step offers it, and returns the sentences they said.
func takeSteps(t *testing.T, ctx context.Context, steps screen.Steps) []string {
	t.Helper()
	var said []string
	tell := func(all ...setup.Said) {
		for _, s := range all {
			said = append(said, s.Sentence)
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
