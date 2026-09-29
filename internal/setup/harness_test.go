package setup_test

import (
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

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/skill"
	"github.com/leeovery/switchboard/internal/tokens"
)

var update = flag.Bool("update", false, "rewrite the golden files with what the tests show")

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
// home, where the LaunchAgent's plist and switchboard's bin directory go;
// Claude Code's config directory; the directories on PATH; Claude Code; and
// this switchboard, run by a link as Homebrew links it. launchd, the router
// and the API are fakes.
type world struct {
	root   string
	config string
	state  string
	home   string
	tmp    string
	skill  string
	// switchboard is the path switchboard is run by, a link to its version.
	switchboard string
	// bin is switchboard's own directory for PATH, and realClaude is Claude
	// Code, on PATH.
	bin, realClaude string
	path            []string
	installPaths    []string
	launchd         *fakeLaunchd
	api             *fakeAPI
}

func newWorld(t *testing.T) *world {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	at := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }
	w := &world{
		root:    root,
		config:  at("config", "switchboard", "config.toml"),
		state:   at("state", "switchboard"),
		home:    at("home"),
		tmp:     at("tmp"),
		skill:   at("claude", "skills", "switchboard", "SKILL.md"),
		bin:     at("home", ".local", "share", "switchboard", "bin"),
		launchd: &fakeLaunchd{},
		api:     &fakeAPI{},
	}
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

// putBinOnPath puts switchboard's bin directory first on PATH, as the line
// setup says to add does.
func (w *world) putBinOnPath() {
	w.path = append([]string{w.bin}, w.path...)
}

// setup is setup in the world, at the terminal the user is at.
func (w *world) setup(t *testing.T, u *user) *setup.Setup {
	t.Helper()
	env := map[string]string{"TMPDIR": w.tmp}
	svc, err := service.New(service.Config{
		GOOS:      "darwin",
		UID:       os.Getuid(),
		Home:      w.home,
		Getenv:    func(key string) string { return env[key] },
		StateDir:  w.state,
		Launchctl: w.launchd.run,
		Router:    w.launchd,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &setup.Setup{
		Terminal:     u.terminal(),
		Accounts:     accounts.Registry{ConfigPath: w.config, Tokens: w.tokens(), Prober: w.api.proberAt},
		Service:      svc,
		Install:      service.InstallOptions{Executable: w.switchboard},
		Path:         strings.Join(w.path, string(filepath.ListSeparator)),
		InstallPaths: w.installPaths,
		Bin:          w.bin,
		Home:         w.home,
		Skill:        w.skill,
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

// done lays the world out as the first run of setup leaves it, once the user
// has put switchboard's bin directory on PATH as it says: doneConfig, with
// both accounts' tokens; the service installed, and its router up;
// switchboard's claude link; and the skill installed.
func (w *world) done(t *testing.T) {
	t.Helper()
	w.writeConfig(t, doneConfig)
	w.writeToken(t, "work", "test-token-work")
	w.writeToken(t, "side", "test-token-side")
	w.installService(t, w.switchboard)
	claudetest.Link(t, w.switchboard, filepath.Join(w.bin, "claude"))
	w.putBinOnPath()
	if err := skill.Install(w.skill); err != nil {
		t.Fatal(err)
	}
}

// installService installs the LaunchAgent to run the switchboard at binary,
// as service install does, launchd loading it and starting its router, and
// forgets the launchctl runs it took.
func (w *world) installService(t *testing.T, binary string) {
	t.Helper()
	if _, err := w.setup(t, &user{}).Service.Install(t.Context(), service.InstallOptions{Executable: binary}); err != nil {
		t.Fatal(err)
	}
	w.launchd.calls = nil
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
// Bootstrapping the service loads it and starts a router, kickstarting it
// starts another, as does signalling the router, which stops it for launchd
// to start again, and booting it out stops it; printing the service exits
// 113 while it isn't loaded, as launchctl does. Each router started answers
// with a pid of its own, from 4242 on, unless dead is set, when none starts;
// the next unanswered health checks go unanswered all the same, as while a
// router starts.
type fakeLaunchd struct {
	loaded bool
	dead   bool
	// pid is the running router's, 0 when none is.
	pid        int
	started    int
	unanswered int
	calls      [][]string
}

func (f *fakeLaunchd) run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	switch {
	case args[0] == "bootstrap":
		f.loaded = true
		f.start()
	case !f.loaded:
		return []byte("Could not find service \"io.github.leeovery.switchboard\" in domain for user gui: 501\n"), exitStatus(113)
	case args[0] == "bootout":
		f.loaded, f.pid = false, 0
	case args[0] == "kickstart", args[0] == "kill":
		f.start()
	}
	return nil, nil
}

func (f *fakeLaunchd) start() {
	f.pid = 0
	if !f.dead {
		f.pid = 4242 + f.started
		f.started++
	}
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
	return router.Health{OK: true, PID: f.pid, Listen: "127.0.0.1:4747"}, nil
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
