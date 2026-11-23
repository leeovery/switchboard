package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/leeovery/switchboard/internal/router"
)

// The dashboard restarts onto an upgrade of the binary it was run as: it
// watches that binary's path from as it starts, asks the router's health,
// asks the binary its version as --version prints it, run as the dashboard
// was, and replaces the process with it, run by the dashboard's command line
// and in its environment, showing the pool shown.
func TestTheDashboardRestartsOntoAnUpgradeOfItsBinary(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	exe := filepath.Join(t.TempDir(), "switchboard")
	putBinary(t, exe)
	srv.deps.Version = "1.3.1"
	srv.deps.Executable = func() (string, error) { return exe, nil }
	srv.deps.Args = []string{"switchboard", "dashboard", "--pool", "b2n8", "15m"}
	type ran struct {
		path      string
		argv, env []string
	}
	var asked, execed ran
	srv.deps.Check = func(_ context.Context, path string, argv, env []string) ([]byte, error) {
		asked = ran{path, argv, env}
		return []byte("switchboard version 1.4.0\n"), nil
	}
	srv.deps.Exec = func(path string, argv, env []string) error {
		execed = ran{path, argv, env}
		return errors.New("exec format error")
	}
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

	if err := u.Exec("q7mt"); err == nil {
		t.Error("Exec() error = nil, want the exec's")
	}
	if want := []string{"switchboard", "dashboard", "15m", "--pool=q7mt"}; execed.path != exe || !slices.Equal(execed.argv, want) {
		t.Errorf("replaced the process with %s as %q, want the binary, as %q", execed.path, execed.argv, want)
	}
	if !slices.Equal(slices.Sorted(slices.Values(execed.env)), environ) {
		t.Errorf("replaced the process in the environment %q, want the dashboard's", execed.env)
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
			exe := filepath.Join(t.TempDir(), "switchboard")
			putBinary(t, exe)
			deps.Executable = func() (string, error) { return exe, nil }
			deps.Args = []string{"switchboard", "dashboard"}
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
