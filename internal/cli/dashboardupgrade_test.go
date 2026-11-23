package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/theme"
)

// shownVariable is the variable a dashboard restarted onto an upgrade is
// given what the one before showed in.
const shownVariable = "SWITCHBOARD_DASHBOARD_SHOWN"

// ran is a program run, by its path, command line and environment.
type ran struct {
	path      string
	argv, env []string
}

// upgradable has deps' dashboard run as a switchboard binary of its own at
// version 1.3.1, by argv, answering 1.4.0 when asked its version, each ask
// and exec noted in the ran returned, an exec failing; and returns the
// binary's path.
func upgradable(t *testing.T, deps *cli.Deps, argv ...string) (string, *ran, *ran) {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "switchboard")
	putBinary(t, exe)
	var asked, execed ran
	deps.Version = "1.3.1"
	deps.Executable = func() (string, error) { return exe, nil }
	deps.Args = argv
	deps.Check = func(_ context.Context, path string, argv, env []string) ([]byte, error) {
		asked = ran{path, argv, env}
		return []byte("switchboard version 1.4.0\n"), nil
	}
	deps.Exec = func(path string, argv, env []string) error {
		execed = ran{path, argv, env}
		return errors.New("exec format error")
	}
	return exe, &asked, &execed
}

// valueIn is the value of the variable called name in env: "" for none.
func valueIn(env []string, name string) string {
	for _, kv := range env {
		if value, ok := strings.CutPrefix(kv, name+"="); ok {
			return value
		}
	}
	return ""
}

// The dashboard restarts onto an upgrade of the binary it was run as: it
// watches that binary's path from as it starts, asks the router's health,
// asks the binary its version as --version prints it, run as the dashboard
// was, and replaces the process with it, run by the dashboard's command line
// without --pool, in its environment, carrying what it shows.
func TestTheDashboardRestartsOntoAnUpgradeOfItsBinary(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	exe, asked, execed := upgradable(t, &srv.deps, "switchboard", "dashboard", "--pool", "b2n8", "15m")
	cfg := recordWatch(t, &srv.deps)
	run(t, srv.deps, "dashboard")
	u := cfg.Upgrade

	if u.Version != "1.3.1" || u.Binary.MovedTo(u.Look()) {
		t.Errorf("Upgrade is of version %q, its binary moved %v; want the dashboard's, 1.3.1, unmoved", u.Version, u.Binary.MovedTo(u.Look()))
	}
	putBinary(t, exe)
	if !u.Binary.MovedTo(u.Look()) {
		t.Error("with the binary's path leading to another file, Upgrade finds it unmoved, want it upgraded")
	}

	if version, err := u.Ask(t.Context()); err != nil || version != "1.4.0" {
		t.Errorf("Ask() = %q, %v; want 1.4.0", version, err)
	}
	environ := slices.Sorted(slices.Values(srv.deps.Environ()))
	if asked.path != exe || !slices.Equal(asked.argv, []string{"switchboard", "--version"}) || !slices.Equal(slices.Sorted(slices.Values(asked.env)), environ) {
		t.Errorf("asked %s as %q, want the binary, run as switchboard --version, in the dashboard's environment", asked.path, asked.argv)
	}

	if err := u.Exec(watch.Shown{Prefs: theme.Prefs{Tab: "runway"}, Pool: "b2n8"}); err == nil {
		t.Error("Exec() error = nil, want the exec's")
	}
	if want := []string{"switchboard", "dashboard", "15m"}; execed.path != exe || !slices.Equal(execed.argv, want) {
		t.Errorf("replaced the process with %s as %q, want the binary, as %q", execed.path, execed.argv, want)
	}
	carried := valueIn(execed.env, shownVariable)
	if got := slices.Sorted(slices.Values(withoutShown(execed.env))); carried == "" || !slices.Equal(got, environ) {
		t.Errorf("replaced the process in the environment %q, want the dashboard's, and what it shows in %s", execed.env, shownVariable)
	}

	var pid atomic.Int64
	pid.Store(4242)
	serveRouter(t, srv.socket(), &pid, map[string]http.HandlerFunc{
		"GET /health": func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(router.Health{OK: true, PID: 4242, Version: "1.4.0"})
		},
	})
	if health := u.Health(t.Context()); health.Version != "1.4.0" {
		t.Errorf("Health() = %+v, want the router's answer, of 1.4.0", health)
	}
}

// withoutShown is env without shownVariable.
func withoutShown(env []string) []string {
	return slices.DeleteFunc(slices.Clone(env), func(kv string) bool { return strings.HasPrefix(kv, shownVariable+"=") })
}

// A dashboard closes its log before it replaces itself, so the log ends with
// its exit, and starts it again where it can't.
func TestTheDashboardsLogEndsWithItsExitAsItRestarts(t *testing.T) {
	deps := pooledDeps(t, false)
	deps.Getenv, deps.Environ = withVariable(deps, "SWITCHBOARD_LOG_LEVEL", "debug")
	upgradable(t, &deps, "switchboard", "dashboard")
	var atExec, after string
	exec := deps.Exec
	deps.Exec = func(path string, argv, env []string) error {
		atExec = readLog(t, deps, "cli.log")
		return exec(path, argv, env)
	}
	deps.Watch = func(_ context.Context, cfg watch.Config, _ io.Writer, _ []string) error {
		_ = cfg.Upgrade.Exec(watch.Shown{})
		after = readLog(t, deps, "cli.log")
		return nil
	}
	run(t, deps, "dashboard")

	if lines := strings.Split(strings.TrimSpace(atExec), "\n"); !strings.Contains(lines[len(lines)-1], "msg=exit") {
		t.Errorf("as the process was replaced, the log ended %q, want its exit", lines[len(lines)-1])
	}
	if lines := strings.Split(strings.TrimSpace(after), "\n"); !strings.Contains(lines[len(lines)-1], "msg=start") {
		t.Errorf("once it wasn't, the log ended %q, want it started again", lines[len(lines)-1])
	}
}

// withVariable is deps' environment with the variable called name set to
// value, as Getenv and Environ give it.
func withVariable(deps cli.Deps, name, value string) (func(string) string, func() []string) {
	getenv, environ := deps.Getenv, deps.Environ
	return func(key string) string {
			if key == name {
				return value
			}
			return getenv(key)
		}, func() []string {
			return append(environ(), name+"="+value)
		}
}

// A dashboard restarted onto an upgrade opens on what the one before
// carried, whatever the preferences keep, and on the pool it showed where the
// config still has it, else on the one the verbs act on, never refusing it;
// passing over a field it doesn't know, and what it can't read, as the log
// says. Nothing it runs, nor the dashboard it restarts into, inherits it.
func TestARestartedDashboardOpensOnWhatTheOneBeforeShowed(t *testing.T) {
	before := theme.Prefs{Tab: "runway", By: "account", Log: theme.LogPrefs{Columns: []string{"model"}}}
	tests := []struct {
		name string
		// carried is what the dashboard before carried: a Shown, in the form
		// a restart writes it, or as written.
		shown   *watch.Shown
		written string
		want    theme.Prefs
		pool    string
		logged  string
	}{
		{name: "what it showed", shown: &watch.Shown{Prefs: before, Pool: "q7mt"}, want: before, pool: "q7mt"},
		{name: "fields of a later version", written: `{"prefs": {"tab": "runway", "a-later-key": 1}, "pool": "q7mt", "a-later-field": true}`, want: theme.Prefs{Tab: "runway"}, pool: "q7mt"},
		{name: "its pool gone", shown: &watch.Shown{Prefs: before, Pool: "x9z9"}, want: before, pool: "b2n8", logged: `msg="the pool the dashboard showed before it restarted is gone; opening on the one the verbs act on"`},
		{name: "showing no pool", shown: &watch.Shown{Prefs: before}, want: before, pool: "b2n8"},
		{name: "nothing it can read", written: "runway", want: theme.Prefs{Tab: "log"}, pool: "b2n8", logged: `msg="can't read what the dashboard showed before it restarted; opening as the preferences keep it"`},
		{name: "preferences it can't read", written: `{"prefs": "runway", "pool": "q7mt"}`, want: theme.Prefs{Tab: "log"}, pool: "b2n8", logged: "level=WARN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := tt.written
			if tt.shown != nil {
				value = carriedBy(t, *tt.shown)
			}
			deps := pooledDeps(t, false)
			if err := theme.NewPrefsFile(stateDir(t, deps), deps.Now).Update(func(p *theme.Prefs) { p.Tab = "log" }); err != nil {
				t.Fatal(err)
			}
			deps.Getenv, deps.Environ = withVariable(deps, shownVariable, value)
			_, asked, _ := upgradable(t, &deps, "switchboard", "dashboard")
			var cfg watch.Config
			var environ []string
			deps.Watch = func(_ context.Context, given watch.Config, _ io.Writer, env []string) error {
				cfg, environ = given, env
				_, _ = given.Upgrade.Ask(t.Context())
				return nil
			}
			if got := run(t, deps, "dashboard"); got != (result{}) {
				t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
			}

			if !reflect.DeepEqual(cfg.Opened, tt.want) || cfg.View != dashboard.ViewKept(tt.want.Tab) || cfg.Pool.ID != tt.pool {
				t.Errorf("the dashboard opened on %+v, its tab %s, pool %q; want %+v, and %q", cfg.Opened, cfg.View, cfg.Pool.ID, tt.want, tt.pool)
			}
			if valueIn(environ, shownVariable) != "" || valueIn(asked.env, shownVariable) != "" {
				t.Errorf("%s was passed on, to the terminal %v, to the binary asked %v; want it taken once", shownVariable, valueIn(environ, shownVariable) != "", valueIn(asked.env, shownVariable) != "")
			}
			if log := readLog(t, deps, "cli.log"); tt.logged != "" && !strings.Contains(log, tt.logged) {
				t.Errorf("the log reads\n%s\nwant %q", log, tt.logged)
			}
		})
	}
}

// carriedBy is shown as a dashboard restarting writes it in shownVariable,
// for the one it restarts into.
func carriedBy(t *testing.T, shown watch.Shown) string {
	t.Helper()
	deps := pooledDeps(t, false)
	_, _, execed := upgradable(t, &deps, "switchboard", "dashboard")
	deps.Watch = func(_ context.Context, cfg watch.Config, _ io.Writer, _ []string) error {
		_ = cfg.Upgrade.Exec(shown)
		return nil
	}
	run(t, deps, "dashboard")
	return valueIn(execed.env, shownVariable)
}

// The binary asked its version that doesn't say one isn't restarted onto.
func TestTheDashboardsUpgradeFailsWhereTheBinaryDoesntSayItsVersion(t *testing.T) {
	tests := []struct {
		name string
		out  string
		err  error
	}{
		{name: "failing", out: "Error: unknown flag: --version\n", err: errors.New("exit status 1")},
		{name: "saying nothing", out: ""},
		{name: "saying something else", out: "Error: something went wrong\n"},
		{name: "saying more than a version", out: "switchboard version 1.4.0 (built today)\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := statusDeps(t, fakeClaudeAPI(t), nil)
			upgradable(t, &deps, "switchboard", "dashboard")
			deps.Check = func(context.Context, string, []string, []string) ([]byte, error) {
				return []byte(tt.out), tt.err
			}
			cfg := recordWatch(t, &deps)
			run(t, deps, "dashboard")

			if version, err := cfg.Upgrade.Ask(t.Context()); err == nil {
				t.Errorf("Ask() = %q, want an error", version)
			}
		})
	}
}

// A dashboard that can't find its binary, or the command line it was run
// by, never restarts.
func TestTheDashboardWithoutItsBinaryNeverRestarts(t *testing.T) {
	tests := []struct {
		name       string
		executable func() (string, error)
		args       []string
	}{
		{name: "its binary not found", executable: func() (string, error) { return "", errors.New("not found") }, args: []string{"switchboard", "dashboard"}},
		{name: "its command line not known", executable: func() (string, error) { return "/test/bin/switchboard", nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := statusDeps(t, fakeClaudeAPI(t), nil)
			deps.Executable, deps.Args = tt.executable, tt.args
			cfg := recordWatch(t, &deps)
			run(t, deps, "dashboard")

			if cfg.Upgrade.Look != nil || cfg.Upgrade.Exec != nil {
				t.Error("the dashboard's Upgrade looks at a binary, or replaces the process; want the zero Upgrade")
			}
		})
	}
}

// putBinary puts a binary at path: another file than any there before.
func putBinary(t *testing.T, path string) {
	t.Helper()
	next := path + ".next"
	if err := os.WriteFile(next, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(next, path); err != nil {
		t.Fatal(err)
	}
}
