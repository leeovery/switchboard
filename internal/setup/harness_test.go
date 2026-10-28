package setup_test

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/sessionhook"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/shell"
	"github.com/leeovery/switchboard/internal/skill"
	"github.com/leeovery/switchboard/internal/tokens"
)

var update = flag.Bool("update", false, "rewrite the golden files with what the tests show")

// tokenTaken is when setup takes a token the user gives, by its clock.
var tokenTaken = time.Date(2026, 10, 27, 9, 30, 0, 0, time.UTC)

// doneConfig is the config a first run of setup leaves, as the user answers
// firstRun: work and side, work the primary, and priming over 08:00-23:00.
const doneConfig = `[[account]]
id    = "work"
label = "Work"
primary = true

[[account]]
id    = "side"
label = "Side"

[prime]
day = "08:00-23:00"
`

// world is what setup sets up, every part of it under a directory of the
// test's own: the config file; the state directory, with the token files; a
// home, where the LaunchAgent's plist, switchboard's bin and shell
// directories and the shell's startup file go; Claude Code's config
// directory; the directories on PATH; Claude Code; and this switchboard, run
// by a link as Homebrew links it. launchd, the router, the API and the
// user's login shell are fakes.
type world struct {
	root   string
	config string
	state  string
	home   string
	tmp    string
	skill  string
	// settings is Claude Code's settings file, where the session hook goes.
	settings string
	// switchboard is the path switchboard is run by, a link to its version.
	switchboard string
	// bin is switchboard's own directory for PATH, and realClaude is Claude
	// Code, on PATH.
	bin, realClaude string
	path            []string
	installPaths    []string
	// configFlag is the config file setup is given, as --config gives it,
	// "" for none.
	configFlag string
	// env is the environment setup is run in, over the world's own, as
	// getenv reads it; a variable set to "" is unset.
	env map[string]string
	// unstartable is why a router couldn't start, nil when it could, and
	// checked what setup checked one would start from, each the state
	// directory's XDG_STATE_HOME, the config file and any log level.
	unstartable error
	checked     []string
	// binary is the XML a binary plist in the service's place reads as,
	// through plutil, "" for none, and plutilSays what plutil fails with
	// reading one it doesn't know, "" for its own words.
	binary, plutilSays string
	// shellDir is switchboard's shell directory.
	shellDir string
	login    *fakeLogin
	// interrupt is the user's interrupting setup as it asks the login shell.
	interrupt context.CancelFunc
	launchd   *fakeLaunchd
	api       *fakeAPI
}

func newWorld(t *testing.T) *world {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	at := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }
	w := &world{
		root:     root,
		config:   at("config", "switchboard", "config.toml"),
		state:    at("state", "switchboard"),
		home:     at("home"),
		tmp:      at("tmp"),
		skill:    at("claude", "skills", "switchboard", "SKILL.md"),
		settings: at("claude", "settings.json"),
		bin:      at("home", ".local", "share", "switchboard", "bin"),
		shellDir: at("home", ".local", "share", "switchboard", "shell"),
		env:      map[string]string{},
		launchd:  &fakeLaunchd{home: at("home")},
		api:      &fakeAPI{},
	}
	w.login = &fakeLogin{w: w}
	mkdir(t, w.tmp)
	version := claudetest.Program(t, at("Cellar", "switchboard", "1.2.3", "bin", "switchboard"))
	w.switchboard = claudetest.Link(t, version, at("brew", "bin", "switchboard"))
	w.realClaude = claudetest.Program(t, at("claude-code", "bin", "claude"))
	w.path = []string{filepath.Dir(w.realClaude)}
	return w
}

// goInstalled places another switchboard, as go install builds one, and
// returns its path.
func (w *world) goInstalled(t *testing.T) string {
	t.Helper()
	return claudetest.Program(t, filepath.Join(w.root, "go", "bin", "switchboard"))
}

// startable checks a router could start from start, as unstartable says,
// noting what it checked.
func (w *world) startable(start service.Start) error {
	w.checked = append(w.checked, strings.TrimSpace(start.StateDir+" "+start.ConfigFile+" "+start.LogLevel))
	return w.unstartable
}

// plutil reads the binary plist in the service's place as binary has it.
func (w *world) plutil(_ context.Context, args ...string) ([]byte, error) {
	if w.binary == "" || args[len(args)-1] != w.plist() {
		return nil, errors.New(cmp.Or(w.plutilSays, "plutil: not a binary plist the world knows"))
	}
	return []byte(w.binary), nil
}

// getenv reads the environment setup is run in: the world's own, TMPDIR,
// XDG_CONFIG_HOME and XDG_STATE_HOME finding config and state, Claude Code's
// config directory and zsh as $SHELL, with env over it.
func (w *world) getenv(key string) string {
	env := map[string]string{
		"TMPDIR":            w.tmp,
		"XDG_CONFIG_HOME":   filepath.Join(w.root, "config"),
		"XDG_STATE_HOME":    filepath.Join(w.root, "state"),
		"CLAUDE_CONFIG_DIR": filepath.Join(w.root, "claude"),
		"SHELL":             "/bin/zsh",
	}
	maps.Copy(env, w.env)
	return env[key]
}

// shellFiles are switchboard's files for the shells in the world, each with
// a completion script naming its shell.
func (w *world) shellFiles() shell.Files {
	return shell.Files{Dir: w.shellDir, Bin: w.bin, Completion: func(sh shell.Shell) ([]byte, error) {
		return []byte("# switchboard's completions for " + string(sh) + "\n"), nil
	}}
}

// startupFile is the startup file of the user's shell, as $SHELL names it,
// where the shell reads it.
func (w *world) startupFile() string {
	sh, _ := shell.Of(w.getenv("SHELL"))
	return shell.StartupFile(sh, w.home, w.login.reads())
}

// line is switchboard's line for the user's shell, as $SHELL names it.
func (w *world) line() string {
	sh, _ := shell.Of(w.getenv("SHELL"))
	return shell.Line(sh, shell.InitFile(w.shellDir, sh), w.home)
}

// addLine adds switchboard's line to the user's shell's startup file, as
// setup does.
func (w *world) addLine(t *testing.T) {
	t.Helper()
	if err := shell.AddLine(w.startupFile(), w.line()); err != nil {
		t.Fatal(err)
	}
}

// setup is setup in the world, at the terminal the user is at.
func (w *world) setup(t *testing.T, u *user) *setup.Setup {
	t.Helper()
	// The CLI's state directory, as it finds it.
	stateDir, err := config.StateDir(w.getenv, func() (string, error) { return w.home, nil })
	if err != nil {
		t.Fatal(err)
	}
	svc, err := service.New(service.Config{
		GOOS:      "darwin",
		UID:       os.Getuid(),
		Home:      w.home,
		Getenv:    w.getenv,
		StateDir:  stateDir,
		Launchctl: w.launchd.run,
		Plutil:    w.plutil,
		Router:    w.launchd.at(stateDir),
		RouterAt:  func(dir string) service.Router { return w.launchd.at(dir) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return &setup.Setup{
		Terminal:     u.terminal(),
		Accounts:     accounts.Registry{ConfigPath: w.config, Tokens: w.tokens(), Prober: w.api.proberAt, Now: func() time.Time { return tokenTaken }},
		LaunchAgent:  svc,
		Install:      service.InstallOptions{Executable: w.switchboard, Config: w.configFlag},
		Startable:    w.startable,
		Path:         strings.Join(w.path, string(filepath.ListSeparator)),
		InstallPaths: w.installPaths,
		Bin:          w.bin,
		Home:         w.home,
		Shells:       w.shellFiles(),
		Login:        w.login,
		Getenv:       w.getenv,
		Interrupt:    w.interruptible,
		SkillFile:    w.skill,
		Settings:     w.settings,
		Usage: func(_ context.Context, out io.Writer) error {
			_, err := io.WriteString(out, "(every account's usage)\n")
			return err
		},
	}
}

// run runs setup in the world, the user giving answers in turn, a line
// each, typed unseen or not as setup asks, and returns what the terminal
// showed, the world's root in it written <root>, and what setup returned.
// It fails the test when setup asked for fewer answers than were given.
func (w *world) run(t *testing.T, answers ...string) (string, error) {
	t.Helper()
	u := &user{answers: answers}
	err := w.setup(t, u).Run(t.Context())
	if len(u.answers) > 0 {
		t.Errorf("setup asked for fewer answers than were given, leaving %q", u.answers)
	}
	return strings.ReplaceAll(u.shown.String(), w.root, "<root>"), err
}

// runs runs setup as run does, failing the test when setup fails.
func (w *world) runs(t *testing.T, answers ...string) string {
	t.Helper()
	shown, err := w.run(t, answers...)
	if err != nil {
		t.Fatalf("Run() error = %v; the terminal showed\n%s", err, shown)
	}
	return shown
}

// done lays the world out as the first run of setup leaves it: doneConfig,
// with both accounts' tokens; the service installed, and its router up;
// switchboard's claude link, its files for the shells, and its line in
// .zshrc; the session hook; and the skill installed.
func (w *world) done(t *testing.T) {
	t.Helper()
	w.writeConfig(t, doneConfig)
	w.writeToken(t, "work", "test-token-work")
	w.writeToken(t, "side", "test-token-side")
	w.installService(t, w.switchboard)
	claudetest.Link(t, w.switchboard, filepath.Join(w.bin, "claude"))
	if _, err := w.shellFiles().Write(); err != nil {
		t.Fatal(err)
	}
	w.addLine(t)
	if _, err := sessionhook.Install(w.settings, w.switchboard); err != nil {
		t.Fatal(err)
	}
	if err := skill.Install(w.skill); err != nil {
		t.Fatal(err)
	}
}

// fakeLogin is the user's login shell, as a new terminal starts it. Its PATH
// finds switchboard's claude link where switchboard's line is in its
// startup file, reading this switchboard's file for it, which is there,
// unless overridden is set, as something after the line changes PATH; and
// otherwise the real claude. It reads its startup file from configDir, where
// that's set, and otherwise as the shell does, which it says; and front,
// where set, stands in front of PATH's claude. Its answers, while it has
// some, come first, in turn, as the user puts switchboard's bin directory on
// PATH by hand meanwhile; and where interrupted is set, the user interrupts
// its asking. A shell switchboard doesn't know, it doesn't ask.
type fakeLogin struct {
	w           *world
	overridden  bool
	configDir   string
	front       shell.Front
	answers     []shell.Answer
	interrupted bool
	// asked are the links it was asked about, in turn.
	asked []string
}

func (f *fakeLogin) Which(ctx context.Context, link string) shell.Answer {
	f.asked = append(f.asked, link)
	if f.interrupted {
		f.w.interrupt()
		return shell.Answer{Why: context.Cause(ctx).Error()}
	}
	sh, known := shell.Of(f.w.getenv("SHELL"))
	switch {
	case f.w.getenv("SHELL") == "":
		return shell.Answer{Why: "$SHELL isn't set"}
	case !known:
		return shell.Answer{Why: "switchboard asks zsh, bash and fish alone"}
	case len(f.answers) > 0:
		answer := f.answers[0]
		f.answers = f.answers[1:]
		return answer
	}
	answer := shell.Answer{Kind: shell.Other, Claude: f.w.realClaude, Front: f.front, ConfigDir: f.reads()}
	init := shell.InitFile(f.w.shellDir, sh)
	_, err := os.Stat(init)
	if found, _ := shell.FindLine(sh, f.w.startupFile(), f.w.home); found.Sources == init && err == nil && !f.overridden {
		answer.Kind, answer.Claude = shell.Link, ""
	}
	return answer
}

// reads is where the shell reads its startup file, as it says: configDir,
// where that's set, and otherwise, for zsh, $ZDOTDIR, else the home, and for
// fish, fish in $XDG_CONFIG_HOME, else in ~/.config; bash says nothing.
func (f *fakeLogin) reads() string {
	getenv := f.w.getenv
	switch sh, _ := shell.Of(getenv("SHELL")); {
	case f.configDir != "":
		return f.configDir
	case sh == shell.Zsh:
		return cmp.Or(getenv("ZDOTDIR"), f.w.home)
	case sh == shell.Fish:
		return filepath.Join(cmp.Or(getenv("XDG_CONFIG_HOME"), filepath.Join(f.w.home, ".config")), "fish")
	}
	return ""
}

// interruptible is setup's way of taking an interrupt in the world: the
// user's interrupting, as interrupt does.
func (w *world) interruptible(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	w.interrupt = cancel
	return ctx, cancel
}

// installService installs the LaunchAgent to run the switchboard at binary,
// as service install does, launchd loading it and starting its router, and
// forgets the launchctl runs it took.
func (w *world) installService(t *testing.T, binary string) {
	t.Helper()
	w.installServiceWith(t, service.InstallOptions{Executable: binary})
}

// installServiceWith installs the LaunchAgent as installService does, as
// opts says.
func (w *world) installServiceWith(t *testing.T, opts service.InstallOptions) {
	t.Helper()
	if _, err := w.setup(t, &user{}).LaunchAgent.Install(t.Context(), opts); err != nil {
		t.Fatal(err)
	}
	w.launchd.calls = nil
}

// editPlist rewrites the LaunchAgent's plist in place as edit says.
func (w *world) editPlist(t *testing.T, edit func(plist string) string) {
	t.Helper()
	plist, err := os.ReadFile(w.plist())
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, w.plist(), edit(string(plist)), 0o644)
}

func (w *world) plist() string {
	return filepath.Join(w.home, "Library", "LaunchAgents", "io.github.leeovery.switchboard.plist")
}

func (w *world) tokens() tokens.Store {
	return tokens.NewStore(w.state, os.Getuid())
}

func (w *world) writeConfig(t *testing.T, content string) {
	t.Helper()
	writeFile(t, w.config, content, 0o644)
}

func (w *world) readConfig(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(w.config)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// writeToken writes secret to the account's token file, only its owner able
// to read it.
func (w *world) writeToken(t *testing.T, id, secret string) {
	t.Helper()
	mkdir(t, filepath.Dir(w.tokens().Path(id)))
	writeFile(t, w.tokens().Path(id), secret+"\n", 0o600)
}

func (w *world) removeToken(t *testing.T, id string) {
	t.Helper()
	if err := os.Remove(w.tokens().Path(id)); err != nil {
		t.Fatal(err)
	}
}

// untouched are the launchctl subcommands a run of setup runs that leaves the
// service as it is: it asks whether the service is loaded.
var untouched = []string{"print"}

// checkLeftToTheRouter checks setup, having shown shown, left the service as
// it was, its router up, saying the router takes up the changes to the
// config and the tokens on its own when changed says setup made some.
func (w *world) checkLeftToTheRouter(t *testing.T, shown string, changed bool) {
	t.Helper()
	const up = "The service is installed, and the router is up: healthy, pid 4242."
	want := up + "\n"
	if changed {
		want = up + " It takes up setup's changes on its own.\n"
	}
	if got := section(t, shown, "3. The service"); got != want {
		t.Errorf("the service step showed\n%s\nwant\n%s", got, want)
	}
	if ran := w.launchd.ran(); !slices.Equal(ran, untouched) {
		t.Errorf("ran launchctl %q, want %q: the service left as it is", ran, untouched)
	}
}

// checkToken checks the account's token file holds a usable token, want.
func (w *world) checkToken(t *testing.T, id, want string) {
	t.Helper()
	token, err := w.tokens().Read(id)
	if err != nil || token.Reveal() != want {
		t.Errorf("%s's token file holds %q (%v), want %q", id, token.Reveal(), err, want)
	}
}

// checkNoToken checks the account has no token file.
func (w *world) checkNoToken(t *testing.T, id string) {
	t.Helper()
	if _, err := os.Lstat(w.tokens().Path(id)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s's token file: %v, want none", id, err)
	}
}

// snapshot notes all the world holds, by path: each file's mode, time and
// content, each link's target, and each directory.
func (w *world) snapshot(t *testing.T) map[string]string {
	t.Helper()
	s := make(map[string]string)
	err := filepath.WalkDir(w.root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			s[path] = "a link to " + target
			return err
		case info.IsDir():
			s[path] = "a directory, " + info.Mode().String()
		default:
			data, err := os.ReadFile(path)
			s[path] = fmt.Sprintf("%v, %d: %q", info.Mode(), info.ModTime().UnixNano(), data)
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// checkUnchanged checks the world holds what before noted, and nothing else.
func (w *world) checkUnchanged(t *testing.T, before map[string]string) {
	t.Helper()
	after := w.snapshot(t)
	for _, path := range slices.Sorted(maps.Keys(before)) {
		if after[path] != before[path] {
			t.Errorf("%s was %s; is %s", path, before[path], after[path])
		}
	}
	for _, path := range slices.Sorted(maps.Keys(after)) {
		if _, ok := before[path]; !ok {
			t.Errorf("%s appeared: %s", path, after[path])
		}
	}
}

// section returns what the terminal showed of the step headed heading, such
// as "4. The claude link": the lines after its heading, up to the next step's.
func section(t *testing.T, shown, heading string) string {
	t.Helper()
	_, after, ok := strings.Cut(shown, "\n"+heading+"\n")
	if !ok {
		t.Fatalf("the terminal showed\n%s\nwant a step headed %q", shown, heading)
	}
	if next := nextHeading.FindStringIndex(after); next != nil {
		after = after[:next[0]]
	}
	return after
}

// nextHeading is where the next step's heading starts: the blank line
// before it.
var nextHeading = regexp.MustCompile(`(?m)^\n\d\. `)

// checkGolden checks what the terminal showed against the golden file name.
func checkGolden(t *testing.T, name, shown string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, []byte(shown), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the golden file (run with -update to make it): %v", err)
	}
	if shown != string(want) {
		t.Errorf("the terminal showed\n%s\nwant (%s; run with -update to accept it)\n%s", shown, path, want)
	}
}

// user is the user at the terminal, giving answers in turn, a line each,
// typed unseen when setup reads a token. The terminal shows what they type
// but for a token, as a terminal does.
type user struct {
	answers []string
	shown   strings.Builder
	// typing is what's left of the line being typed.
	typing string
}

func (u *user) terminal() setup.Terminal {
	return setup.Terminal{In: u, Out: &u.shown, Hidden: u.typeUnseen}
}

// Read types the next answer, which the terminal shows.
func (u *user) Read(p []byte) (int, error) {
	if u.typing == "" {
		if len(u.answers) == 0 {
			return 0, io.EOF
		}
		u.typing = u.answers[0] + "\n"
		u.answers = u.answers[1:]
		u.shown.WriteString(u.typing)
	}
	n := copy(p, u.typing)
	u.typing = u.typing[n:]
	return n, nil
}

// interrupt is an answer that's the user interrupting the typing of a token.
const interrupt = "^C"

// typeUnseen types the next answer, which the terminal doesn't show, nor the
// newline that ends it: or interrupts the typing, as interrupt says to.
func (u *user) typeUnseen() ([]byte, error) {
	if len(u.answers) == 0 {
		return nil, io.EOF
	}
	answer := u.answers[0]
	u.answers = u.answers[1:]
	if answer == interrupt {
		return nil, accounts.ErrInterrupted
	}
	return []byte(answer), nil
}

// fakeLaunchd stands in for launchctl, and launchd and the router behind it.
// Printing the service names its router's pid while one runs. Bootstrapping the service loads it and starts a router, kickstarting it
// starts another, as does signalling the router, which stops it for launchd
// to start again, and booting it out stops it; printing the service exits
// 113 while it isn't loaded, as launchctl does. Each router started answers
// with a pid of its own, from 4242 on, unless dead is set, when none starts;
// the next unanswered health checks go unanswered all the same, as while a
// router starts. Asked to restart, the router restarts in place, keeping its
// pid, unless old is set, when it can't be asked, as one from before routers
// restarted when asked can't.
type fakeLaunchd struct {
	// home is the user's, and stateDir the state directory of the router
	// the plist launchd loaded runs, as its XDG_STATE_HOME and its HOME find
	// it, else as setenv's do, as launchctl setenv gives launchd them.
	home, stateDir string
	setenv         map[string]string
	loaded         bool
	dead           bool
	// byHand is the pid of a router run by hand, answering with its state
	// in byHandAt, else where the router launchd runs would have it, in its
	// place, which print doesn't name; 0 for none. anonymous has the
	// routers answer without their pids. pidless has print say launchd runs
	// one without naming its pid, and stateUnread say neither its state nor
	// its pid, as formats setup can't read wholly might; printFails has
	// print fail, as launchctl erring does.
	byHand                           int
	byHandAt                         string
	anonymous                        bool
	pidless, stateUnread, printFails bool
	old                              bool
	// version is the version the routers answer with.
	version string
	// pid is the running router's, 0 when none is, and restarts how many
	// times it has restarted in place.
	pid        int
	restarts   int
	started    int
	unanswered int
	calls      [][]string
}

func (f *fakeLaunchd) run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	switch {
	case args[0] == "bootstrap":
		f.loaded = true
		f.stateDir = f.stateDirOf(args[2])
		f.start()
	case !f.loaded:
		return []byte("Could not find service \"io.github.leeovery.switchboard\" in domain for user gui: 501\n"), exitStatus(113)
	case args[0] == "bootout":
		f.loaded, f.pid = false, 0
	case args[0] == "kickstart", args[0] == "kill":
		f.start()
	case args[0] == "print" && f.printFails:
		return []byte("Could not print: Input/output error\n"), exitStatus(5)
	case args[0] == "print":
		return f.printed(args[1]), nil
	}
	return nil, nil
}

// printed is what launchctl print prints of the job target, trimmed to
// what matters: whether the router runs, and its pid, as the formats set.
func (f *fakeLaunchd) printed(target string) []byte {
	state := "\tstate = not running\n"
	switch {
	case f.stateUnread:
		state = ""
	case f.pid != 0 && f.pidless:
		state = "\tstate = running\n"
	case f.pid != 0:
		state = fmt.Sprintf("\tstate = running\n\tpid = %d\n", f.pid)
	}
	return []byte(target + " = {\n" + state + "}\n")
}

func (f *fakeLaunchd) start() {
	f.pid = 0
	if !f.dead {
		f.pid = 4242 + f.started
		f.started++
	}
}

// stateHome is how a plist names its router's XDG_STATE_HOME.
var stateHome = regexp.MustCompile(`<key>XDG_STATE_HOME</key>\s*<string>([^<]*)</string>`)

// homeOf is how a plist names its router's HOME, where it names one.
var homeOf = regexp.MustCompile(`<key>HOME</key>\s*<string>([^<]*)</string>`)

// stateDirOf is the state directory of the router the plist at path runs.
func (f *fakeLaunchd) stateDirOf(path string) string {
	plist, _ := os.ReadFile(path)
	value, home := f.setenv["XDG_STATE_HOME"], cmp.Or(f.setenv["HOME"], f.home)
	if m := stateHome.FindSubmatch(plist); m != nil {
		value = string(m[1])
	}
	if m := homeOf.FindSubmatch(plist); m != nil {
		home = string(m[1])
	}
	dir, _ := config.StateDir(func(string) string { return value }, func() (string, error) { return home, nil })
	return dir
}

// at is the router a client of the control socket in dir reaches:
// launchd's, when its router's state is there, else none.
func (f *fakeLaunchd) at(dir string) service.Router {
	return routerAt{f, dir}
}

// routerAt is the router a client of the control socket in dir reaches.
type routerAt struct {
	launchd *fakeLaunchd
	dir     string
}

// errNoRouterThere is how a client fails with no router's socket where it
// dials.
var errNoRouterThere = fmt.Errorf("%w: dial unix control.sock: connect: no such file or directory", router.ErrNotRunning)

func (r routerAt) Health(ctx context.Context) (router.Health, error) {
	f := r.launchd
	if f.byHand != 0 && r.dir == cmp.Or(f.byHandAt, f.stateDir) {
		h := f.router()
		if !f.anonymous {
			h.PID = f.byHand
		}
		return h, nil
	}
	if r.dir != f.stateDir {
		return router.Health{}, errNoRouterThere
	}
	return f.Health(ctx)
}

func (r routerAt) Restart(ctx context.Context) (router.Restart, error) {
	if r.dir != r.launchd.stateDir {
		return router.Restart{}, errNoRouterThere
	}
	return r.launchd.Restart(ctx)
}

// ran returns the launchctl subcommands run, in turn.
func (f *fakeLaunchd) ran() []string {
	var subcommands []string
	for _, call := range f.calls {
		subcommands = append(subcommands, call[0])
	}
	return subcommands
}

func (f *fakeLaunchd) Health(context.Context) (router.Health, error) {
	unanswered := f.unanswered > 0
	f.unanswered = max(f.unanswered-1, 0)
	if f.pid == 0 || unanswered {
		return router.Health{}, fmt.Errorf("%w: dial unix control.sock: connect: no such file or directory", router.ErrNotRunning)
	}
	return f.router(), nil
}

// router is how the running router answers.
func (f *fakeLaunchd) router() router.Health {
	startedAt := time.Date(2026, 9, 28, 13, f.started, f.restarts, 0, time.UTC)
	h := router.Health{OK: true, PID: f.pid, Listen: "127.0.0.1:4747", Version: f.version, StartedAt: startedAt}
	if f.anonymous {
		h.PID = 0
	}
	return h
}

func (f *fakeLaunchd) Restart(context.Context) (router.Restart, error) {
	if f.old {
		return router.Restart{}, fmt.Errorf("%w: the router answered POST /restart with 404 Not Found", router.ErrNoRestart)
	}
	took := f.router()
	f.restarts++
	return router.Restart{Health: took, InPlace: true}, nil
}

// exitStatus is a program's exit with a status other than 0, as
// *exec.ExitError reports it.
type exitStatus int

func (e exitStatus) Error() string { return "exit status " + strconv.Itoa(int(e)) }

func (e exitStatus) ExitCode() int { return int(e) }

// refusedToken is the one token the fake API refuses.
const refusedToken = "test-token-refused"

// fakeAPI takes every token but refusedToken, noting each it's asked about.
type fakeAPI struct {
	asked []string
}

func (a *fakeAPI) proberAt(string) accounts.Prober {
	return a
}

func (a *fakeAPI) Probe(_ context.Context, token string) (quota.Probe, error) {
	a.asked = append(a.asked, token)
	if token == refusedToken {
		return quota.Probe{}, refusal{}
	}
	return quota.Probe{}, nil
}

// refusal is a probe's failure as the API refuses the token.
type refusal struct{}

func (refusal) Error() string { return "HTTP 401 · Invalid bearer token" }

func (refusal) Refused() bool { return true }

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}

// writeFile writes content to a file at path, with perm, making its
// directory.
func writeFile(t *testing.T, path, content string, perm fs.FileMode) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
}
