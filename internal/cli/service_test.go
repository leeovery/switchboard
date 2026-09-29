package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
)

// target is the service, as launchctl names it for the user 501.
const target = "gui/501/io.github.leeovery.switchboard"

func TestServiceInstall(t *testing.T) {
	s := newServiceSetup(t)

	got := run(t, s.srv.deps, "service", "install")
	want := result{stdout: "installed " + s.plist + "\nthe router is up: healthy, pid " + strconv.Itoa(os.Getpid()) + "\n"}
	if got != want {
		t.Errorf("switchboard service install = %+v, want %+v", got, want)
	}
	if want := [][]string{{"bootout", target}, {"bootstrap", "gui/501", s.plist}}; !reflect.DeepEqual(s.launchd.calls, want) {
		t.Errorf("ran launchctl %q, want %q", s.launchd.calls, want)
	}
	s.checkPlist(t,
		"<string>"+s.binary+"</string>\n\t\t<string>serve</string>\n\t</array>",
		"<key>SWITCHBOARD_CONFIG</key>\n\t\t<string>"+s.srv.config+"</string>",
		"<key>XDG_STATE_HOME</key>\n\t\t<string>"+filepath.Dir(s.srv.state)+"</string>",
	)
}

func TestServiceInstallWithAnEnvFileAndAConfig(t *testing.T) {
	s := newServiceSetup(t)
	dir := t.TempDir()
	envFile := filepath.Join(dir, "tokens.env")
	if err := os.WriteFile(envFile, []byte("export CLAUDE_TOKEN_WORK=test-token-work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(envFile, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	got := run(t, s.srv.deps, "service", "install", "--env-file", "tokens.env", "--config", "work.toml")
	wantWarning := "warning: other users can read the env file " + envFile + " (mode 0644), and it holds tokens: chmod 600 it\n"
	if got.code != 0 || got.stderr != wantWarning {
		t.Errorf("switchboard service install = %+v, want exit status 0, and on stderr\n%s", got, wantWarning)
	}
	s.checkPlist(t, "\t\t<string>switchboard</string>\n"+
		"\t\t<string>"+envFile+"</string>\n"+
		"\t\t<string>"+s.binary+"</string>\n"+
		"\t\t<string>--config</string>\n"+
		"\t\t<string>"+filepath.Join(dir, "work.toml")+"</string>\n"+
		"\t</array>")
}

func TestServiceInstallWhenTheRouterDoesntAnswer(t *testing.T) {
	// Set up outside the bubble: the fake API's server waits on the network,
	// which would keep the bubble's clock from moving.
	s := newServiceSetup(t)
	s.launchd.starts = false
	synctest.Test(t, func(t *testing.T) {
		got := run(t, s.srv.deps, "service", "install")
		want := result{
			stdout: "installed " + s.plist + "\n",
			stderr: "Error: the router didn't answer within 5s of starting: see why with switchboard logs router, and in " +
				filepath.Join(s.srv.state, "logs", "launchd.log") + "\n",
			code: 1,
		}
		if got != want {
			t.Errorf("switchboard service install = %+v, want %+v", got, want)
		}
	})
}

func TestServiceUninstall(t *testing.T) {
	s := newServiceSetup(t)
	if got := run(t, s.srv.deps, "service", "install"); got.code != 0 {
		t.Fatalf("switchboard service install = %+v, want exit status 0", got)
	}
	s.launchd.calls = nil

	if got, want := run(t, s.srv.deps, "service", "uninstall"), (result{stdout: "uninstalled: removed " + s.plist + "\n"}); got != want {
		t.Errorf("switchboard service uninstall = %+v, want %+v", got, want)
	}
	if got, want := run(t, s.srv.deps, "service", "uninstall"), (result{stdout: "the service isn't installed\n"}); got != want {
		t.Errorf("switchboard service uninstall, again = %+v, want %+v", got, want)
	}
	if want := [][]string{{"bootout", target}, {"bootout", target}}; !reflect.DeepEqual(s.launchd.calls, want) {
		t.Errorf("ran launchctl %q, want %q", s.launchd.calls, want)
	}
	if _, err := os.Stat(s.plist); !os.IsNotExist(err) {
		t.Errorf("the plist: %v, want it gone", err)
	}
}

func TestServiceRestart(t *testing.T) {
	s := newServiceSetup(t)
	s.launchd.loaded = true

	got := run(t, s.srv.deps, "service", "restart")
	if want := (result{stdout: "restarted\nthe router is up: healthy, pid " + strconv.Itoa(os.Getpid()) + "\n"}); got != want {
		t.Errorf("switchboard service restart = %+v, want %+v", got, want)
	}
	if want := [][]string{{"kickstart", "-k", target}}; !reflect.DeepEqual(s.launchd.calls, want) {
		t.Errorf("ran launchctl %q, want %q", s.launchd.calls, want)
	}
}

func TestServiceRestartWhenLaunchdHasntLoadedIt(t *testing.T) {
	s := newServiceSetup(t)

	got := run(t, s.srv.deps, "service", "restart")
	want := result{stderr: "Error: launchd hasn't loaded the service: install it with switchboard service install\n", code: 1}
	if got != want {
		t.Errorf("switchboard service restart = %+v, want %+v", got, want)
	}
}

func TestServiceStatus(t *testing.T) {
	s := newServiceSetup(t)

	got := run(t, s.srv.deps, "service", "status")
	want := result{stdout: "LaunchAgent  not installed\nlaunchd      not loaded\nrouter       not running\n"}
	if got != want {
		t.Errorf("before installing, switchboard service status = %+v, want %+v", got, want)
	}

	if got := run(t, s.srv.deps, "service", "install"); got.code != 0 {
		t.Fatalf("switchboard service install = %+v, want exit status 0", got)
	}
	got = run(t, s.srv.deps, "service", "status")
	want = result{stdout: "LaunchAgent  " + s.plist + "\nlaunchd      loaded\nrouter       healthy, pid " + strconv.Itoa(os.Getpid()) + "\n"}
	if got != want {
		t.Errorf("once installed, switchboard service status = %+v, want %+v", got, want)
	}
}

func TestServiceIsMacOSOnly(t *testing.T) {
	for _, args := range [][]string{{"install"}, {"install", "--env-file", "tokens.env"}, {"uninstall"}, {"restart"}, {"status"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			s := newServiceSetup(t)
			s.srv.deps.GOOS = "linux"

			got := run(t, s.srv.deps, append([]string{"service"}, args...)...)
			want := result{stderr: "Error: the service is macOS only for now: elsewhere, run switchboard serve under your system's service manager\n", code: 1}
			if got != want {
				t.Errorf("switchboard service %s = %+v, want %+v", strings.Join(args, " "), got, want)
			}
			if s.launchd.calls != nil {
				t.Errorf("ran launchctl %q, want it left alone", s.launchd.calls)
			}
		})
	}
}

// serviceSetup is serve's setup with a switchboard binary to install, and
// launchd faked, so that bootstrapping the service starts the router.
type serviceSetup struct {
	srv     *serveSetup
	launchd *fakeLaunchd
	// binary is the switchboard installed, its symlinks resolved, and plist
	// where the service's plist goes.
	binary, plist string
}

func newServiceSetup(t *testing.T) *serviceSetup {
	t.Helper()
	srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"TMPDIR": t.TempDir()})
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "switchboard")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	launchd := &fakeLaunchd{t: t, srv: srv, starts: true}
	srv.deps.Executable = func() (string, error) { return binary, nil }
	srv.deps.Launchctl = launchd.run
	home, err := srv.deps.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	plist := filepath.Join(home, "Library", "LaunchAgents", "io.github.leeovery.switchboard.plist")
	return &serviceSetup{srv: srv, launchd: launchd, binary: binary, plist: plist}
}

// checkPlist checks the service's plist holds each of parts.
func (s *serviceSetup) checkPlist(t *testing.T, parts ...string) {
	t.Helper()
	data, err := os.ReadFile(s.plist)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range parts {
		if !strings.Contains(string(data), part) {
			t.Errorf("the plist reads\n%s\nwant it to hold\n%s", data, part)
		}
	}
}

// fakeLaunchd stands in for launchctl, and for launchd behind it, noting each
// run: bootstrapping the service loads it, and starts srv's router when
// starts is set; kickstarting a loaded service starts the router; booting
// one out forgets it; and printing one says it's loaded. A service that isn't
// loaded fails each but bootstrap as launchctl does.
type fakeLaunchd struct {
	t      *testing.T
	srv    *serveSetup
	starts bool
	loaded bool
	calls  [][]string
}

func (f *fakeLaunchd) run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	switch {
	case args[0] == "bootstrap":
		f.loaded = true
		if f.starts {
			f.srv.start(f.t)
		}
	case !f.loaded:
		return []byte("Could not find service \"io.github.leeovery.switchboard\" in domain for user gui: 501\n"), exitStatus(113)
	case args[0] == "bootout":
		f.loaded = false
	case args[0] == "kickstart":
		f.srv.start(f.t)
	}
	return nil, nil
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
