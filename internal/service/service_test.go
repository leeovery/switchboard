package service_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
)

// target is the service, as launchctl names it for the user 501.
const target = "gui/501/io.github.leeovery.switchboard"

func TestInstall(t *testing.T) {
	s := newSetup(t, nil, upOnceStarted(4242))

	installed, err := s.svc.Install(t.Context(), service.InstallOptions{Executable: s.binary})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	want := [][]string{{"bootout", target}, {"bootstrap", "gui/501", s.plist}}
	if !reflect.DeepEqual(s.launchctl.calls, want) {
		t.Errorf("ran launchctl %q, want %q", s.launchctl.calls, want)
	}
	s.checkPlist(t, s.binary, "", "")
	if installed.Router == nil || installed.Router.PID != 4242 || installed.Warnings != nil {
		t.Errorf("Install() = %+v, want the router at pid 4242 answering, and no warnings", installed)
	}
	if info, err := os.Stat(filepath.Dir(s.svc.Log())); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Errorf("launchd's log directory: %v, %v; want a directory only its owner can use", info, err)
	}
}

func TestInstallReplacesAPlistThere(t *testing.T) {
	s := newSetup(t, nil, upOnceStarted(4242))
	if err := os.MkdirAll(filepath.Dir(s.plist), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.plist, []byte("<plist/>\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.svc.Install(t.Context(), service.InstallOptions{Executable: s.binary}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	s.checkPlist(t, s.binary, "", "")
}

func TestInstallWhenLaunchdHasntLoadedTheService(t *testing.T) {
	for _, status := range []int{3, 113} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			s := newSetup(t, nil, upOnceStarted(4242))
			s.launchctl.exits = map[string]int{"bootout": status}

			if _, err := s.svc.Install(t.Context(), service.InstallOptions{Executable: s.binary}); err != nil {
				t.Fatalf("Install() error = %v", err)
			}
			if want := [][]string{{"bootout", target}, {"bootstrap", "gui/501", s.plist}}; !reflect.DeepEqual(s.launchctl.calls, want) {
				t.Errorf("ran launchctl %q, want %q", s.launchctl.calls, want)
			}
		})
	}
}

func TestInstallFailsWhenLaunchctlDoes(t *testing.T) {
	notFound := errors.New(`exec: "launchctl": executable file not found in $PATH`)
	tests := []struct {
		name    string
		exits   map[string]int
		cantRun error
		// want returns the error Install fails with, and the runs it makes,
		// given the plist's path.
		want func(plist string) (string, [][]string)
	}{
		{
			name:  "booting the service out",
			exits: map[string]int{"bootout": 5},
			want: func(string) (string, [][]string) {
				return "launchctl bootout " + target + ": bootout failed: 5: Input/output error (exit status 5)", [][]string{{"bootout", target}}
			},
		},
		{
			name:  "bootstrapping the service",
			exits: map[string]int{"bootstrap": 5},
			want: func(plist string) (string, [][]string) {
				return "launchctl bootstrap gui/501 " + plist + ": bootstrap failed: 5: Input/output error (exit status 5)",
					[][]string{{"bootout", target}, {"bootstrap", "gui/501", plist}}
			},
		},
		{
			name:    "without a launchctl to run",
			cantRun: notFound,
			want: func(string) (string, [][]string) {
				return "launchctl bootout " + target + `: exec: "launchctl": executable file not found in $PATH`, [][]string{{"bootout", target}}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t, nil, upOnceStarted(4242))
			s.launchctl.exits, s.launchctl.cantRun = tt.exits, tt.cantRun

			_, err := s.svc.Install(t.Context(), service.InstallOptions{Executable: s.binary})
			wantErr, wantCalls := tt.want(s.plist)
			if err == nil || err.Error() != wantErr {
				t.Errorf("Install() error = %v, want %q", err, wantErr)
			}
			if !reflect.DeepEqual(s.launchctl.calls, wantCalls) {
				t.Errorf("ran launchctl %q, want %q", s.launchctl.calls, wantCalls)
			}
		})
	}
}

func TestInstallRefusesATemporaryBuild(t *testing.T) {
	tests := []struct {
		name string
		// build places a switchboard binary under root, whose temporary
		// directory is tmp, and returns the path it's run by, and the path
		// Install finds it at.
		build func(t *testing.T, root, tmp string) (exe, found string)
	}{
		{
			name: "in the temporary directory",
			build: func(t *testing.T, _, tmp string) (string, string) {
				exe := writeBinary(t, filepath.Join(tmp, "switchboard"))
				return exe, exe
			},
		},
		{
			name: "built by go run",
			build: func(t *testing.T, root, _ string) (string, string) {
				exe := writeBinary(t, filepath.Join(root, "cache", "go-build2718281828", "b001", "exe", "switchboard"))
				return exe, exe
			},
		},
		{
			name: "linked to from elsewhere",
			build: func(t *testing.T, root, tmp string) (string, string) {
				built := writeBinary(t, filepath.Join(tmp, "switchboard"))
				link := filepath.Join(root, "bin", "sb")
				if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(built, link); err != nil {
					t.Fatal(err)
				}
				return link, built
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t, nil, nil)
			s.launchctl.refuse = true
			exe, found := tt.build(t, s.root, s.tmp)

			_, err := s.svc.Install(t.Context(), service.InstallOptions{Executable: exe})
			want := "this switchboard is a temporary build, " + found + ", which won't be there for launchd to start: install it with go install, and install the service with that one"
			if err == nil || err.Error() != want {
				t.Errorf("Install() error = %v, want %q", err, want)
			}
			if _, err := os.Stat(s.plist); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the plist: %v, want none written", err)
			}
		})
	}
}

func TestInstallChecksTheEnvFile(t *testing.T) {
	tests := []struct {
		name string
		// mode is the env file's; zero leaves none there.
		mode os.FileMode
		// directory puts a directory where the env file is.
		directory   bool
		wantWarning bool
		wantErr     string
	}{
		{name: "only its owner's to read", mode: 0o600},
		{name: "readable by its group", mode: 0o640, wantWarning: true},
		{name: "readable by anyone", mode: 0o604, wantWarning: true},
		{name: "missing", wantErr: "read the env file: open %s: no such file or directory"},
		{name: "a directory", directory: true, wantErr: "the env file %s isn't a file"},
		{name: "unreadable", mode: 0o200, wantErr: "read the env file: open %s: permission denied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t, nil, upOnceStarted(4242))
			dir := filepath.Join(s.root, "secrets")
			envFile := filepath.Join(dir, "tokens.env")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			switch {
			case tt.directory:
				if err := os.Mkdir(envFile, 0o700); err != nil {
					t.Fatal(err)
				}
			case tt.mode != 0:
				if err := os.WriteFile(envFile, []byte("export CLAUDE_TOKEN_WORK=test-token-work\n"), tt.mode); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(envFile, tt.mode); err != nil {
					t.Fatal(err)
				}
			}
			if tt.mode != 0 && tt.mode&0o400 == 0 && os.Getuid() == 0 {
				t.Skip("root reads any file")
			}
			s.launchctl.refuse = tt.wantErr != ""
			// The env file is given as the user would give it, by its path
			// from where they are.
			t.Chdir(dir)

			installed, err := s.svc.Install(t.Context(), service.InstallOptions{Executable: s.binary, EnvFile: "tokens.env"})
			if tt.wantErr != "" {
				if want := fmt.Sprintf(tt.wantErr, envFile); err == nil || err.Error() != want {
					t.Errorf("Install() error = %v, want %q", err, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("Install() error = %v", err)
			}
			var want []string
			if tt.wantWarning {
				want = []string{fmt.Sprintf("other users can read the env file %s (mode %04o), and it holds tokens: chmod 600 it", envFile, tt.mode)}
			}
			if !reflect.DeepEqual(installed.Warnings, want) {
				t.Errorf("warned %q, want %q", installed.Warnings, want)
			}
			s.checkPlist(t, s.binary, envFile, "")
		})
	}
}

func TestInstallServesTheConfigGiven(t *testing.T) {
	env := map[string]string{"XDG_CONFIG_HOME": "/elsewhere/config", "XDG_STATE_HOME": "/elsewhere/state"}
	s := newSetup(t, env, upOnceStarted(4242))
	t.Chdir(s.root)

	if _, err := s.svc.Install(t.Context(), service.InstallOptions{Executable: s.binary, Config: "work.toml"}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	s.checkPlist(t, s.binary, "", filepath.Join(s.root, "work.toml"))
}

func TestInstallWaitsForTheRouter(t *testing.T) {
	tests := []struct {
		name string
		// answer answers a health check made since after Install began,
		// when asked checks came before it.
		answer func(since time.Duration, asked int) (router.Health, error)
		// wantPID is the answering router's, or 0 for none.
		wantPID    int
		wantWaited time.Duration
	}{
		{
			name:       "up at once",
			answer:     func(_ time.Duration, asked int) (router.Health, error) { return upAfter(asked, 1, 4242) },
			wantPID:    4242,
			wantWaited: 0,
		},
		{
			name: "up in two seconds",
			answer: func(since time.Duration, _ int) (router.Health, error) {
				if since < 2*time.Second {
					return router.Health{}, errNotRunning
				}
				return up(4242)
			},
			wantPID:    4242,
			wantWaited: 2 * time.Second,
		},
		{
			name:       "never up",
			answer:     func(time.Duration, int) (router.Health, error) { return router.Health{}, errNotRunning },
			wantWaited: service.StartWait,
		},
		{
			name: "up once the router before it has stopped",
			answer: func(since time.Duration, _ int) (router.Health, error) {
				if since < time.Second {
					return up(100)
				}
				return up(4242)
			},
			wantPID:    4242,
			wantWaited: time.Second,
		},
		{
			name:       "not up, as a router launchd didn't start still is",
			answer:     func(time.Duration, int) (router.Health, error) { return up(100) },
			wantWaited: service.StartWait,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				began := time.Now()
				s := newSetup(t, nil, func(asked int) (router.Health, error) { return tt.answer(time.Since(began), asked) })

				installed, err := s.svc.Install(t.Context(), service.InstallOptions{Executable: s.binary})
				if err != nil {
					t.Fatalf("Install() error = %v", err)
				}
				if waited := time.Since(began); waited != tt.wantWaited {
					t.Errorf("waited %v for the router, want %v", waited, tt.wantWaited)
				}
				switch {
				case tt.wantPID == 0 && installed.Router != nil:
					t.Errorf("Install() found the router %+v, want none answering", *installed.Router)
				case tt.wantPID != 0 && (installed.Router == nil || installed.Router.PID != tt.wantPID):
					t.Errorf("Install() found the router %v, want pid %d", installed.Router, tt.wantPID)
				}
			})
		})
	}
}

func TestUninstall(t *testing.T) {
	tests := []struct {
		name string
		// installed puts the plist in place first.
		installed   bool
		exits       map[string]int
		wantRemoved bool
		wantErr     string
	}{
		{name: "installed and loaded", installed: true, wantRemoved: true},
		{name: "installed, not loaded", installed: true, exits: map[string]int{"bootout": 113}, wantRemoved: true},
		{name: "not installed", exits: map[string]int{"bootout": 113}},
		{
			name:      "when launchd can't boot it out",
			installed: true,
			exits:     map[string]int{"bootout": 5},
			wantErr:   "launchctl bootout " + target + ": bootout failed: 5: Input/output error (exit status 5)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t, nil, nil)
			s.launchctl.exits = tt.exits
			if tt.installed {
				s.putPlist(t)
			}

			removed, err := s.svc.Uninstall(t.Context())
			if (err == nil) != (tt.wantErr == "") || (err != nil && err.Error() != tt.wantErr) || removed != tt.wantRemoved {
				t.Errorf("Uninstall() = %v, %v; want %v, and the error %q", removed, err, tt.wantRemoved, tt.wantErr)
			}
			if want := [][]string{{"bootout", target}}; !reflect.DeepEqual(s.launchctl.calls, want) {
				t.Errorf("ran launchctl %q, want %q", s.launchctl.calls, want)
			}
			_, statErr := os.Stat(s.plist)
			if kept := statErr == nil; kept != (tt.installed && tt.wantErr != "") {
				t.Errorf("the plist is still there: %v, want %v", kept, tt.installed && tt.wantErr != "")
			}
		})
	}
}

func TestRestart(t *testing.T) {
	s := newSetup(t, nil, func(asked int) (router.Health, error) {
		if asked == 0 {
			return up(100)
		}
		return up(4242)
	})

	h, err := s.svc.Restart(t.Context())
	if err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	if h == nil || h.PID != 4242 {
		t.Errorf("Restart() found the router %v, want the one at pid 4242", h)
	}
	if want := [][]string{{"kickstart", "-k", target}}; !reflect.DeepEqual(s.launchctl.calls, want) {
		t.Errorf("ran launchctl %q, want %q", s.launchctl.calls, want)
	}
}

func TestRestartFailsWhenLaunchctlDoes(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   func(err error) bool
	}{
		{name: "not loaded", status: 113, want: func(err error) bool { return errors.Is(err, service.ErrNotLoaded) }},
		{
			name:   "otherwise",
			status: 5,
			want: func(err error) bool {
				return err != nil && !errors.Is(err, service.ErrNotLoaded) &&
					err.Error() == "launchctl kickstart -k "+target+": kickstart failed: 5: Input/output error (exit status 5)"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t, nil, func(int) (router.Health, error) { return router.Health{}, errNotRunning })
			s.launchctl.exits = map[string]int{"kickstart": tt.status}

			if h, err := s.svc.Restart(t.Context()); h != nil || !tt.want(err) {
				t.Errorf("Restart() = %v, %v", h, err)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	tests := []struct {
		name      string
		installed bool
		exits     map[string]int
		answer    func(asked int) (router.Health, error)
		want      service.Status
	}{
		{
			name:      "installed, loaded and up",
			installed: true,
			answer:    func(int) (router.Health, error) { return up(4242) },
			want:      service.Status{Installed: true, Loaded: true, Router: &router.Health{OK: true, Version: "1.2.3", PID: 4242}},
		},
		{
			name:   "none of them",
			exits:  map[string]int{"print": 113},
			answer: func(int) (router.Health, error) { return router.Health{}, errNotRunning },
			want:   service.Status{RouterErr: errNotRunning},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t, nil, tt.answer)
			s.launchctl.exits = tt.exits
			if tt.installed {
				s.putPlist(t)
			}

			st, err := s.svc.Status(t.Context())
			if err != nil {
				t.Fatalf("Status() error = %v", err)
			}
			if !reflect.DeepEqual(st, tt.want) {
				t.Errorf("Status() = %+v, want %+v", st, tt.want)
			}
			if want := [][]string{{"print", target}}; !reflect.DeepEqual(s.launchctl.calls, want) {
				t.Errorf("ran launchctl %q, want %q", s.launchctl.calls, want)
			}
		})
	}
}

func TestStatusWithoutALaunchctlToRun(t *testing.T) {
	s := newSetup(t, nil, nil)
	s.launchctl.cantRun = errors.New(`exec: "launchctl": executable file not found in $PATH`)

	if _, err := s.svc.Status(t.Context()); err == nil {
		t.Error("Status() error = nil, want launchctl's failure to run")
	}
}

func TestNewOffMacOS(t *testing.T) {
	for _, goos := range []string{"linux", "windows", "freebsd"} {
		t.Run(goos, func(t *testing.T) {
			if _, err := service.New(service.Config{GOOS: goos}); !errors.Is(err, service.ErrUnsupported) {
				t.Errorf("New() error = %v, want %v", err, service.ErrUnsupported)
			}
		})
	}
}

// errNotRunning is how the router's client fails without a router to ask.
var errNotRunning = fmt.Errorf("%w: dial unix /tmp/sb/control.sock: connect: no such file or directory", router.ErrNotRunning)

// up is a healthy router's answer, running as pid.
func up(pid int) (router.Health, error) {
	return router.Health{OK: true, Version: "1.2.3", PID: pid}, nil
}

// upAfter answers as a router running as pid, once checks have been asked
// before, and as no router until then.
func upAfter(asked, checks, pid int) (router.Health, error) {
	if asked < checks {
		return router.Health{}, errNotRunning
	}
	return up(pid)
}

// upOnceStarted answers as no router, before the service is installed, and
// then as one running as pid.
func upOnceStarted(pid int) func(asked int) (router.Health, error) {
	return func(asked int) (router.Health, error) { return upAfter(asked, 1, pid) }
}

// setup is a service with a home, a temporary directory and a switchboard
// binary of its own, all under root, with fakes for launchctl and the
// router.
type setup struct {
	svc *service.Service
	// root holds the rest, its symlinks resolved.
	root, tmp string
	// binary is the switchboard to install, and plist where its plist goes.
	binary, plist string
	launchctl     *fakeLaunchctl
}

// newSetup sets the service up with env added to its environment, and a
// router that answers as answer does, or refuses to be asked for nil.
func newSetup(t *testing.T, env map[string]string, answer func(asked int) (router.Health, error)) *setup {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &setup{root: root, tmp: filepath.Join(root, "tmp"), launchctl: &fakeLaunchctl{t: t}}
	if err := os.Mkdir(s.tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	s.binary = writeBinary(t, filepath.Join(root, "bin", "switchboard"))
	vars := map[string]string{"TMPDIR": s.tmp}
	maps.Copy(vars, env)
	home := filepath.Join(root, "home")
	s.svc = newService(t, service.Config{
		Home:      home,
		StateDir:  filepath.Join(home, ".local", "state", "switchboard"),
		Getenv:    func(key string) string { return vars[key] },
		Launchctl: s.launchctl.run,
		Router:    &fakeRouter{t: t, answer: answer, refuse: answer == nil},
	})
	s.plist = filepath.Join(home, "Library", "LaunchAgents", "io.github.leeovery.switchboard.plist")
	if got := s.svc.Plist(); got != s.plist {
		t.Fatalf("Plist() = %s, want %s", got, s.plist)
	}
	return s
}

// checkPlist checks the plist in place is the one serving with the
// switchboard at binary, sourcing envFile and serving config, readable by
// all.
func (s *setup) checkPlist(t *testing.T, binary, envFile, config string) {
	t.Helper()
	want, err := s.svc.PlistOf(binary, envFile, config)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(s.plist)
	if err != nil || string(got) != string(want) {
		t.Errorf("the plist reads\n%s(%v)\nwant\n%s", got, err, want)
	}
	if info, err := os.Stat(s.plist); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("the plist: %v, %v; want it readable by all, writable by its owner alone", info, err)
	}
}

// putPlist puts a plist where the service's goes, as an install leaves one.
func (s *setup) putPlist(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(s.plist), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.plist, []byte("<plist/>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeBinary writes a program at path, and returns path.
func writeBinary(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// fakeLaunchctl stands in for launchctl, noting each run: a subcommand exits
// with the status exits gives it, else 0, saying why as launchctl does.
// cantRun fails every run, as when there's no launchctl to run, and refuse
// fails the test on any.
type fakeLaunchctl struct {
	t       *testing.T
	refuse  bool
	exits   map[string]int
	cantRun error
	calls   [][]string
}

// meanings are what launchctl's exit statuses mean.
var meanings = map[int]string{3: "No such process", 5: "Input/output error", 113: "Could not find specified service"}

func (f *fakeLaunchctl) run(_ context.Context, args ...string) ([]byte, error) {
	if f.refuse {
		f.t.Errorf("ran launchctl %q, want launchctl left alone", args)
		return nil, errors.New("launchctl mustn't run here")
	}
	f.calls = append(f.calls, args)
	if f.cantRun != nil {
		return nil, f.cantRun
	}
	status := f.exits[args[0]]
	if status == 0 {
		return nil, nil
	}
	return fmt.Appendf(nil, "%s failed: %d: %s\n", args[0], status, meanings[status]), exitStatus(status)
}

// exitStatus is a program's exit with a status other than 0, as
// *exec.ExitError reports it.
type exitStatus int

func (e exitStatus) Error() string {
	return "exit status " + strconv.Itoa(int(e))
}

func (e exitStatus) ExitCode() int {
	return int(e)
}

// fakeRouter answers health checks as answer says, given how many came
// before, or fails the test when it's asked while refuse is set.
type fakeRouter struct {
	t      *testing.T
	refuse bool
	answer func(asked int) (router.Health, error)
	asked  int
}

func (r *fakeRouter) Health(context.Context) (router.Health, error) {
	if r.refuse {
		r.t.Error("asked the router how it is, want it left alone")
		return router.Health{}, errNotRunning
	}
	defer func() { r.asked++ }()
	return r.answer(r.asked)
}
