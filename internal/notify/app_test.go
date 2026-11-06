package notify

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
)

const message = "work · Work has room again"

// cantWrite is the line logged where the app can't be written out.
const cantWrite = `msg="can't write out the notifications' app, so notifications are posted through osascript"`

func TestTheAppIsWrittenOutRegisteredAndPostedThrough(t *testing.T) {
	tests := []struct {
		name string
		// before puts what's there before, at the app's path.
		before func(t *testing.T, path string)
	}{
		{name: "none there", before: func(*testing.T, string) {}},
		{name: "another version there", before: func(t *testing.T, path string) {
			put(t, zipOf(t, appEntries("old")...), path)
		}},
		{name: "its sum there but not the app", before: func(t *testing.T, path string) {
			put(t, zipOf(t, appEntries("new")...), path)
			if err := os.RemoveAll(path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "its sum there but an executable that can't be run", before: func(t *testing.T, path string) {
			put(t, zipOf(t, appEntries("new")...), path)
			if err := os.Chmod(filepath.Join(path, executable), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zipped := zipOf(t, appEntries("new")...)
			s := newAppSetup(t, zipped)
			tt.before(t, s.path)

			if err := s.desktop.Notify(message); err != nil {
				t.Fatalf("Notify() error = %v", err)
			}

			want := s.throughApp(message)
			if got := s.programs.ran(); !slices.EqualFunc(got, want, slices.Equal) {
				t.Errorf("ran %q, want %q", got, want)
			}
			s.expectApp(t, "new")
			if got, want := s.read(t, sumPath(s.path)), sumOf(zipped)+"\n"; got != want {
				t.Errorf("the sum beside the app reads %q, want the zip's, %q", got, want)
			}
			if got, want := s.beside(t), []string{lockName, sumName, appName}; !slices.Equal(got, want) {
				t.Errorf("beside the app are %q, want %q alone", got, want)
			}
			s.programs.expectMinimalEnvironment(t)
		})
	}
}

func TestACurrentAppIsntWrittenAgainButIsRegistered(t *testing.T) {
	zipped := zipOf(t, appEntries("new")...)
	s := newAppSetup(t, zipped)
	put(t, zipped, s.path)
	marker := filepath.Join(s.path, "Contents", "marker")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.desktop.Notify(message); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the app was written again (%v), want it left as it was", err)
	}
	want := s.throughApp(message)
	if got := s.programs.ran(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("ran %q, want %q: registered, as Launch Services can lose it, and posted through", got, want)
	}
}

func TestAnAppThatCantBeWrittenOutLeavesItToOsascript(t *testing.T) {
	tests := []struct {
		name   string
		zipped func(t *testing.T) []byte
		why    string
	}{
		{
			name: "an entry outside its folder",
			zipped: func(t *testing.T) []byte {
				return zipOf(t, append(appEntries("new"), entry{name: "../escaped", mode: 0o644})...)
			},
			why: `\"../escaped\" would land outside its folder`,
		},
		{
			name: "an entry at an absolute path",
			zipped: func(t *testing.T) []byte {
				return zipOf(t, append(appEntries("new"), entry{name: "/escaped", mode: 0o644})...)
			},
			why: `\"/escaped\" would land outside its folder`,
		},
		{
			name: "a link",
			zipped: func(t *testing.T) []byte {
				return zipOf(t, append(appEntries("new"), entry{name: "Switchboard.app/Contents/escaped", mode: fs.ModeSymlink | 0o777, body: "../../.."})...)
			},
			why: `\"Switchboard.app/Contents/escaped\" is a link`,
		},
		{
			name: "a named pipe",
			zipped: func(t *testing.T) []byte {
				return zipOf(t, append(appEntries("new"), entry{name: "Switchboard.app/Contents/pipe", mode: fs.ModeNamedPipe | 0o644})...)
			},
			why: `\"Switchboard.app/Contents/pipe\" is neither a plain file nor a folder`,
		},
		{
			name: "no app",
			zipped: func(t *testing.T) []byte {
				return zipOf(t, entry{name: "Other.app/Contents/MacOS/Switchboard", mode: 0o755, body: "#!/bin/sh\n"})
			},
			why: "holds no Switchboard.app/Contents/MacOS/Switchboard that can be run",
		},
		{
			name: "an executable zipped without its execute bit",
			zipped: func(t *testing.T) []byte {
				entries := appEntries("new")
				for i, e := range entries {
					if e.name == appName+"/"+executable {
						entries[i].mode = 0o666
					}
				}
				return zipOf(t, entries...)
			},
			why: "holds no Switchboard.app/Contents/MacOS/Switchboard that can be run",
		},
		{
			name:   "not a zip",
			zipped: func(*testing.T) []byte { return []byte("not a zip") },
			why:    "read the notifications' app's zip",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newAppSetup(t, tt.zipped(t))

			for range 2 {
				if err := s.desktop.Notify(message); err != nil {
					t.Fatalf("Notify() error = %v", err)
				}
			}

			s.expectOsascriptAlone(t, 2)
			if n := strings.Count(log.String(), cantWrite); n != 1 || !log.Has("level=WARN", cantWrite, tt.why) {
				t.Errorf("log reads\n%s\nwant one warning that the app can't be written out, saying %s", log, tt.why)
			}
			if got, want := s.beside(t), []string{lockName}; !slices.Equal(got, want) {
				t.Errorf("beside the app are %q, want %q alone: none written, nothing escaped", got, want)
			}
		})
	}
}

func TestAnAppThatCantBeRegisteredLeavesItToOsascriptForTheRestOfTheRun(t *testing.T) {
	log := logstest.Capture(t)
	s := newAppSetup(t, zipOf(t, appEntries("new")...))
	s.programs.fail("lsregister", exitStatus(1))

	s.desktop.Prepare()
	for range 2 {
		if err := s.desktop.Notify(message); err != nil {
			t.Fatalf("Notify() error = %v", err)
		}
	}

	want := [][]string{{lsregister, "-f", s.path}, osascript(), osascript()}
	if got := s.programs.ran(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("ran %q, want %q: registered once, then osascript", got, want)
	}
	if n := strings.Count(log.String(), cantWrite); n != 1 || !log.Has("level=WARN", cantWrite, "register "+s.path+" with Launch Services: exit status 1") {
		t.Errorf("log reads\n%s\nwant one warning that the app can't be registered", log)
	}
}

func TestASecondWriterWaitsForTheFirstAndOnlyRegistersWhatItWrote(t *testing.T) {
	zipped := zipOf(t, appEntries("new")...)
	s := newAppSetup(t, zipped)
	// The first writer holds the lock, has written the app, and has yet to
	// register it.
	release := holdLock(t, s.path)
	put(t, zipped, s.path)
	marker := filepath.Join(s.path, "Contents", "marker")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	posted := make(chan error, 1)
	go func() { posted <- s.desktop.Notify(message) }()
	select {
	case err := <-posted:
		t.Fatalf("Notify() = %v while another writer held the lock, want it waiting", err)
	case <-time.After(3 * lockPoll):
	}
	release()
	if err := <-posted; err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the app was written again (%v), want the first writer's left as it was", err)
	}
	want := s.throughApp(message)
	if got := s.programs.ran(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("ran %q, want %q: the app registered and posted through", got, want)
	}
}

func TestALockHeldPastTheWaitLeavesItToOsascript(t *testing.T) {
	log := logstest.Capture(t)
	s := newAppSetup(t, zipOf(t, appEntries("new")...))
	s.desktop.app.wait = 2 * lockPoll
	defer holdLock(t, s.path)()

	if err := s.desktop.Notify(message); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	s.expectOsascriptAlone(t, 1)
	if !log.Has("level=WARN", cantWrite, "another switchboard held the notifications' app's lock") {
		t.Errorf("log reads\n%s\nwant a warning that the lock wasn't had in time", log)
	}
	if _, err := os.Stat(s.path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the app is there (%v), want none written without the lock", err)
	}
}

func TestAnAppThatCantBePutInPlaceLeavesTheOneThereInPlace(t *testing.T) {
	log := logstest.Capture(t)
	s := newAppSetup(t, zipOf(t, appEntries("new")...))
	put(t, zipOf(t, appEntries("old")...), s.path)
	s.desktop.app.rename = func(oldpath, newpath string) error {
		if newpath == s.path && filepath.Base(oldpath) == appName {
			return errors.New("rename refused")
		}
		return os.Rename(oldpath, newpath)
	}

	if err := s.desktop.Notify(message); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	s.expectApp(t, "old")
	s.expectOsascriptAlone(t, 1)
	if !log.Has("level=WARN", cantWrite, "put the notifications' app in place: rename refused") {
		t.Errorf("log reads\n%s\nwant a warning that the app couldn't be put in place", log)
	}
}

func TestAPostThroughTheApp(t *testing.T) {
	failed := errors.New("signal: killed")
	tests := []struct {
		name       string
		exit       error
		wantErr    error
		notAllowed bool
	}{
		{name: "posted"},
		{name: "not allowed", exit: exitStatus(2), wantErr: ErrNotAllowed, notAllowed: true},
		{name: "failing otherwise", exit: exitStatus(1), wantErr: exitStatus(1)},
		{name: "failing without an exit status", exit: failed, wantErr: failed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newAppSetup(t, zipOf(t, appEntries("new")...))
			s.programs.fail("--post", tt.exit)

			err := s.desktop.Notify(message)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Notify() error = %v, want %v", err, tt.wantErr)
			}
			if got := errors.Is(err, ErrNotAllowed); got != tt.notAllowed {
				t.Errorf("Notify() error = %v, not allowed %v, want %v", err, got, tt.notAllowed)
			}
		})
	}
}

func TestTheFirstPostWaitsForMacOSToBeAskedWhetherItMayPost(t *testing.T) {
	s := newAppSetup(t, zipOf(t, appEntries("new")...))
	answer := s.programs.hold("--authorize")

	posted := make(chan error, 1)
	go func() { posted <- s.desktop.Notify(message) }()
	s.programs.waitFor(t, "--authorize")
	select {
	case err := <-posted:
		t.Fatalf("Notify() = %v before macOS was answered, want it waiting", err)
	case <-time.After(3 * lockPoll):
	}
	close(answer)
	if err := <-posted; err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	want := s.throughApp(message)
	if got := s.programs.ran(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("ran %q, want %q: written out, registered, asked once, then posted through", got, want)
	}
	s.programs.expectMinimalEnvironment(t)
}

func TestMacOSIsAskedWithTheLockLetGo(t *testing.T) {
	s := newAppSetup(t, zipOf(t, appEntries("new")...))
	answer := s.programs.hold("--authorize")
	defer close(answer)

	go s.desktop.Prepare()
	s.programs.waitFor(t, "--authorize")

	unlock, err := lockBeside(s.path, 0)
	if err != nil {
		t.Fatalf("another writer couldn't take the lock while macOS was asked (%v), want it let go", err)
	}
	unlock()
}

func TestAPostMacOSDoesntAllowOnceAskedFailsAsNotAllowed(t *testing.T) {
	log := logstest.Capture(t)
	s := newAppSetup(t, zipOf(t, appEntries("new")...))
	s.programs.fail("--authorize", exitStatus(2))
	s.programs.fail("--post", exitStatus(2))

	if err := s.desktop.Notify(message); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("Notify() error = %v, want %v", err, ErrNotAllowed)
	}
	want := s.throughApp(message)
	if got := s.programs.ran(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("ran %q, want %q: posted through the app all the same", got, want)
	}
	if log.Has("level=WARN") {
		t.Errorf("log reads\n%s\nwant no warning: an ask answered no is answered", log)
	}
}

func TestAnAskThatFailsIsWarnedOfOnceAndThePostsGoThroughTheApp(t *testing.T) {
	const cantAsk = `msg="can't ask macOS whether Switchboard may post notifications; posting through its app all the same"`
	tests := []struct {
		name string
		// fail has the ask fail.
		fail func(s *appSetup)
		why  string
	}{
		{
			name: "failing",
			fail: func(s *appSetup) {
				s.programs.fail("--authorize", fmt.Errorf("%w: The operation couldn't be completed.", exitStatus(1)))
			},
			why: "exit status 1: The operation couldn't be completed.",
		},
		{
			name: "unanswered in time",
			fail: func(s *appSetup) {
				s.desktop.app.ask = 2 * lockPoll
				s.programs.hold("--authorize")
			},
			why: "not answered within 200ms",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newAppSetup(t, zipOf(t, appEntries("new")...))
			tt.fail(s)

			for range 2 {
				if err := s.desktop.Notify(message); err != nil {
					t.Fatalf("Notify() error = %v", err)
				}
			}

			want := s.throughApp(message, message)
			if got := s.programs.ran(); !slices.EqualFunc(got, want, slices.Equal) {
				t.Errorf("ran %q, want %q: asked once, then posted through the app", got, want)
			}
			if n := strings.Count(log.String(), cantAsk); n != 1 || !log.Has("level=WARN", cantAsk, tt.why) {
				t.Errorf("log reads\n%s\nwant one warning that macOS couldn't be asked, saying %s", log, tt.why)
			}
		})
	}
}

func TestTheAppIsWrittenOutOnceAProcess(t *testing.T) {
	s := newAppSetup(t, zipOf(t, appEntries("new")...))

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(s.desktop.Prepare)
		wg.Go(func() { _ = s.desktop.Notify(message) })
	}
	wg.Wait()

	registered := 0
	for _, ran := range s.programs.ran() {
		if ran[0] == lsregister {
			registered++
		}
	}
	if registered != 1 {
		t.Errorf("ran %q, want the app registered once", s.programs.ran())
	}
	s.expectApp(t, "new")
}

func TestNothingIsRunOffMacOS(t *testing.T) {
	s := newAppSetup(t, zipOf(t, appEntries("new")...))
	s.desktop.goos = "linux"

	s.desktop.Prepare()
	if err := s.desktop.Notify(message); err != nil {
		t.Errorf("Notify() error = %v, want none", err)
	}
	put(t, zipOf(t, appEntries("new")...), s.path)
	if err := s.desktop.Uninstall(); err != nil {
		t.Errorf("Uninstall() error = %v, want none", err)
	}

	if ran := s.programs.ran(); len(ran) > 0 {
		t.Errorf("ran %q off macOS, want nothing", ran)
	}
	s.expectApp(t, "new")
}

func TestNothingIsWrittenOffMacOS(t *testing.T) {
	s := newAppSetup(t, zipOf(t, appEntries("new")...))
	s.desktop.goos = "linux"

	s.desktop.Prepare()
	if err := s.desktop.Notify(message); err != nil {
		t.Errorf("Notify() error = %v, want none", err)
	}
	if _, err := os.Stat(filepath.Dir(s.path)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the app's directory is there (%v), want nothing written", err)
	}
}

func TestWithoutAnAppNothingIsReadiedOrAsked(t *testing.T) {
	s := newAppSetup(t, nil)

	s.desktop.Prepare()
	if err := s.desktop.Notify(message); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	s.expectOsascriptAlone(t, 1)
	if _, err := os.Stat(filepath.Dir(s.path)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the app's directory is there (%v), want nothing written", err)
	}
}

// appSetup is a Desktop on macOS writing out the app zipped in a home of its
// own, running a stand-in for every program.
type appSetup struct {
	path     string
	programs *programs
	desktop  Desktop
}

// put writes the app zipped out at path, as another switchboard would.
func put(t *testing.T, zipped []byte, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeApp(zipped, path, os.Rename); err != nil {
		t.Fatal(err)
	}
}

// holdLock takes the lock beside the app at path, as another switchboard
// writing it out would, until release is called.
func holdLock(t *testing.T, path string) (release func()) {
	t.Helper()
	unlock, err := lockBeside(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	return unlock
}

func newAppSetup(t *testing.T, zipped []byte) *appSetup {
	t.Helper()
	home := t.TempDir()
	vars := map[string]string{"PATH": "/usr/bin:/bin", "HOME": home, "CLAUDE_TOKEN_WORK": "test-token-work"}
	getenv := func(key string) string { return vars[key] }
	homeDir := func() (string, error) { return home, nil }
	p := &programs{fails: map[string]error{}, holds: map[string]chan struct{}{}}
	desktop := NewDesktop(getenv, homeDir)
	desktop.goos, desktop.run = "darwin", p.run
	desktop.app = newApp(zipped)
	return &appSetup{
		path:     filepath.Join(home, ".local", "share", "switchboard", appName),
		programs: p,
		desktop:  desktop,
	}
}

// exe is the app's executable.
func (s *appSetup) exe() string {
	return filepath.Join(s.path, executable)
}

// throughApp is what's run posting the messages through the app once a
// process: registering it, asking macOS whether it may post, and posting.
func (s *appSetup) throughApp(messages ...string) [][]string {
	ran := [][]string{{lsregister, "-f", s.path}, {s.exe(), "--authorize"}}
	for _, m := range messages {
		ran = append(ran, []string{s.exe(), "--post", m})
	}
	return ran
}

// expectApp fails the test unless the app in place is the version given,
// each file with the permissions it was zipped with.
func (s *appSetup) expectApp(t *testing.T, version string) {
	t.Helper()
	for _, e := range appEntries(version) {
		path := filepath.Join(filepath.Dir(s.path), filepath.FromSlash(e.name))
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("the app holds no %s: %v", e.name, err)
			continue
		}
		if e.mode.IsDir() {
			continue
		}
		if info.Mode() != e.mode {
			t.Errorf("%s has mode %v, want %v", e.name, info.Mode(), e.mode)
		}
		if got := s.read(t, path); got != e.body {
			t.Errorf("%s holds %q, want %q", e.name, got, e.body)
		}
	}
}

// expectOsascriptAlone fails the test unless all it ran was osascript, posting
// the message n times.
func (s *appSetup) expectOsascriptAlone(t *testing.T, n int) {
	t.Helper()
	want := slices.Repeat([][]string{osascript()}, n)
	if got := s.programs.ran(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("ran %q, want osascript alone, %d times", got, n)
	}
}

// beside returns the names in the app's directory.
func (s *appSetup) beside(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(s.path))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func (s *appSetup) read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// osascript is osascript posting the message.
func osascript() []string {
	return []string{"osascript", "-e", displayNotification(message)}
}

// programs stands in for the programs a Desktop runs, noting each, failing
// as it's told, and waiting where it's held.
type programs struct {
	mu    sync.Mutex
	runs  [][]string
	envs  [][]string
	fails map[string]error
	holds map[string]chan struct{}
}

// fail has the program known as key fail with err: lsregister, osascript, or
// the app run with the flag key names.
func (p *programs) fail(key string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fails[key] = err
}

// hold has the program known as key, as fail knows it, wait until the
// channel it returns is closed, or until it's killed as its context ends.
func (p *programs) hold(key string) chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	held := make(chan struct{})
	p.holds[key] = held
	return held
}

func (p *programs) run(ctx context.Context, env []string, name string, args ...string) error {
	p.mu.Lock()
	p.runs = append(p.runs, append([]string{name}, args...))
	p.envs = append(p.envs, env)
	k := key(name, args)
	err, held := p.fails[k], p.holds[k]
	p.mu.Unlock()
	if held != nil {
		select {
		case <-held:
		case <-ctx.Done():
			return errors.New("signal: killed")
		}
	}
	return err
}

func (p *programs) ran() [][]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.runs)
}

// waitFor waits until the program known as key, as fail knows it, has run.
func (p *programs) waitFor(t *testing.T, key string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, ran := range p.ran() {
			if slices.Contains(ran, key) {
				return
			}
		}
		time.Sleep(lockPoll / 10)
	}
	t.Fatalf("%s never ran", key)
}

// expectMinimalEnvironment fails the test unless every program ran in the
// minimal environment, without the token the process's holds.
func (p *programs) expectMinimalEnvironment(t *testing.T) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, env := range p.envs {
		if len(env) != 2 || !strings.HasPrefix(env[0], "PATH=") || !strings.HasPrefix(env[1], "HOME=") {
			t.Errorf("program %d ran with the environment %q, want PATH and HOME alone", i, env)
		}
	}
}

// key is what fail knows a program run as name with args by.
func key(name string, args []string) string {
	switch {
	case name == lsregister:
		return "lsregister"
	case name == "osascript":
		return name
	case len(args) > 0:
		return args[0]
	}
	return name
}

// exitStatus is a program exiting with a status, as exec.ExitError says.
type exitStatus int

func (e exitStatus) Error() string { return "exit status " + strconv.Itoa(int(e)) }
func (e exitStatus) ExitCode() int { return int(e) }

// entry is a file or folder in a zip.
type entry struct {
	name string
	mode fs.FileMode
	body string
}

// appEntries are the entries of the app's zip, as a release makes it, of the
// version given.
func appEntries(version string) []entry {
	return []entry{
		{name: "Switchboard.app/", mode: fs.ModeDir | 0o755},
		{name: "Switchboard.app/Contents/", mode: fs.ModeDir | 0o755},
		{name: "Switchboard.app/Contents/Info.plist", mode: 0o644, body: "<plist>" + version + "</plist>"},
		{name: "Switchboard.app/Contents/MacOS/", mode: fs.ModeDir | 0o755},
		{name: "Switchboard.app/Contents/MacOS/Switchboard", mode: 0o755, body: "#!/bin/sh\n# " + version + "\n"},
		{name: "Switchboard.app/Contents/Resources/AppIcon.icns", mode: 0o640, body: "icon " + version},
	}
}

// zipOf zips the entries given, in their order.
func zipOf(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		header := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		header.SetMode(e.mode)
		f, err := w.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if e.body != "" {
			if _, err := f.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
