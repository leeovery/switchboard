package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
)

func TestInstalledCLIVersion(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing", "claude")
	directory := filepath.Join(dir, "directory", "claude")
	notRunnable := filepath.Join(dir, "not-runnable", "claude")
	first := filepath.Join(dir, "first", "claude")
	second := filepath.Join(dir, "second", "claude")
	onPath := filepath.Join(dir, "path", "claude")
	switchboard := filepath.Join(dir, "switchboard")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for path, mode := range map[string]os.FileMode{notRunnable: 0o600, first: 0o700, second: 0o700, onPath: 0o700, switchboard: 0o700} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, mode); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name      string
		paths     []string
		pathHasIt bool
		output    string
		outputErr error
		want      string
		wantErr   bool
		wantRun   string
	}{
		{
			name:      "from PATH, before any install path, as run finds it",
			paths:     []string{first, second},
			pathHasIt: true,
			output:    "2.1.290 (Claude Code)\n",
			want:      "2.1.290",
			wantRun:   onPath,
		},
		{
			name:    "from the first install path holding a program, as PATH has none, as a LaunchAgent's minimal PATH doesn't",
			paths:   []string{missing, directory, notRunnable, first, second},
			output:  "2.1.290 (Claude Code)\n",
			want:    "2.1.290",
			wantRun: first,
		},
		{
			name:    "the first version in the output",
			paths:   []string{first},
			output:  "claude 10.20.30-beta.1 (build 4.5.6)\n",
			want:    "10.20.30",
			wantRun: first,
		},
		{
			name:    "none without a CLI",
			paths:   []string{missing},
			wantErr: true,
		},
		{
			name:      "none when the CLI fails",
			paths:     []string{first},
			output:    "2.1.290 (Claude Code)\n",
			outputErr: errors.New("signal: killed"),
			wantErr:   true,
			wantRun:   first,
		},
		{
			name:    "none when the output has no version",
			paths:   []string{first},
			output:  "unknown option '--version'\n",
			wantErr: true,
			wantRun: first,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ran string
			cli := installedCLI{
				paths:      tt.paths,
				executable: func() (string, error) { return switchboard, nil },
				output: func(ctx context.Context, _ []string, path string, args ...string) ([]byte, error) {
					ran = path
					if !slices.Equal(args, []string{"--version"}) {
						t.Errorf("ran %s with %q, want --version", path, args)
					}
					if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > versionTimeout {
						t.Errorf("ran %s without a deadline within %v", path, versionTimeout)
					}
					return []byte(tt.output), tt.outputErr
				},
			}
			if tt.pathHasIt {
				cli.pathList = filepath.Dir(onPath)
			}

			got, err := cli.version(t.Context())
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Errorf("version() = %q, %v; want %q, and an error %v", got, err, tt.want, tt.wantErr)
			}
			if ran != tt.wantRun {
				t.Errorf("ran %q, want %q", ran, tt.wantRun)
			}
		})
	}
}

func TestTheCLIRunsWithoutTheTokens(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	switchboard := filepath.Join(t.TempDir(), "switchboard")
	for _, program := range []string{filepath.Join(tmp, "claude"), switchboard} {
		if err := os.WriteFile(program, nil, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	vars := map[string]string{
		"CLAUDE_TOKEN_WORK":       "test-token-work",
		"CLAUDE_CODE_OAUTH_TOKEN": "test-token-oauth",
		"PATH":                    tmp,
		"HOME":                    "/home/tester",
		"TMPDIR":                  tmp,
		"LANG":                    "en_GB.UTF-8",
	}
	var env []string
	cli := systemCLI(func(key string) string { return vars[key] }, "/home/tester", func() (string, error) { return switchboard, nil })
	cli.output = func(_ context.Context, e []string, _ string, _ ...string) ([]byte, error) {
		env = e
		return []byte("2.1.300 (Claude Code)\n"), nil
	}

	if _, err := cli.version(t.Context()); err != nil {
		t.Fatalf("version() error = %v", err)
	}
	// PATH is its own directory, then PATH as it was.
	if want := []string{"PATH=" + tmp + ":" + tmp, "HOME=/home/tester", "TMPDIR=" + tmp, "LANG=en_GB.UTF-8"}; !slices.Equal(env, want) {
		t.Errorf("claude --version ran with the environment %q, want %q alone", env, want)
	}
}

func TestTheCLIFindsTheInterpreterBesideIt(t *testing.T) {
	// interpreter writes an interpreter at path that says it's a claude, and a
	// script at script that it runs, as env finds it on PATH.
	interpreter := func(path, script string) {
		t.Helper()
		writeScript(t, path, "#!/bin/sh\necho '2.1.303 (Claude Code)'\n")
		writeScript(t, script, "#!/usr/bin/env "+filepath.Base(path)+"\n")
	}
	tests := []struct {
		name string
		// install puts claude at path, where its installer puts it, and what
		// it runs by, under root.
		install func(root, path string)
	}{
		{
			name:    "a script, beside it",
			install: func(_, path string) { interpreter(filepath.Join(filepath.Dir(path), "stub-node"), path) },
		},
		{
			name: "through a link into its package, as npm's, beside the link",
			install: func(root, path string) {
				script := filepath.Join(root, "lib", "claude-code", "cli.js")
				interpreter(filepath.Join(filepath.Dir(path), "stub-node"), script)
				link(t, script, path)
			},
		},
		{
			name: "through a link, where it leads",
			install: func(root, path string) {
				script := filepath.Join(root, "prefix", "bin", "claude")
				interpreter(filepath.Join(root, "prefix", "bin", "stub-node"), script)
				link(t, script, path)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			tt.install(t.TempDir(), filepath.Join(home, ".local", "bin", "claude"))
			switchboard := filepath.Join(t.TempDir(), "switchboard")
			writeScript(t, switchboard, "#!/bin/sh\n")
			// A LaunchAgent's PATH, which holds the system's directories alone.
			getenv := func(key string) string {
				if key == "PATH" {
					return "/usr/bin:/bin"
				}
				return ""
			}

			got, err := systemCLI(getenv, home, func() (string, error) { return switchboard, nil }).version(t.Context())
			if err != nil || got != "2.1.303" {
				t.Errorf("version() = %q, %v; want 2.1.303", got, err)
			}
		})
	}
}

// writeScript writes a program at path holding script, making its directory.
func writeScript(t *testing.T, path, script string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}

// link makes a symbolic link at path leading to target, making its
// directory.
func link(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func TestVersionCache(t *testing.T) {
	failed := errors.New("signal: killed")
	// step is a call for the version some time after the first, and what the
	// CLI answers if it's asked.
	type step struct {
		after    time.Duration
		answer   string
		fails    bool
		want     string
		wantAsks bool
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{
			name: "asks the CLI once the version is an hour old",
			steps: []step{
				{after: 0, answer: "2.1.290", want: "2.1.290", wantAsks: true},
				{after: 59 * time.Minute, want: "2.1.290", wantAsks: false},
				{after: time.Hour, answer: "2.1.300", want: "2.1.300", wantAsks: true},
				{after: time.Hour + 59*time.Minute, want: "2.1.300", wantAsks: false},
				{after: 2 * time.Hour, answer: "2.1.300", want: "2.1.300", wantAsks: true},
			},
		},
		{
			name: "keeps the version the CLI last gave while it doesn't answer, and asks again an hour on",
			steps: []step{
				{after: 0, answer: "2.1.290", want: "2.1.290", wantAsks: true},
				{after: time.Hour, fails: true, want: "2.1.290", wantAsks: true},
				{after: time.Hour + 30*time.Minute, want: "2.1.290", wantAsks: false},
				{after: 2 * time.Hour, answer: "2.1.300", want: "2.1.300", wantAsks: true},
			},
		},
		{
			name: "the floor until the CLI gives a version",
			steps: []step{
				{after: 0, fails: true, want: fallbackVersion, wantAsks: true},
				{after: 30 * time.Minute, want: fallbackVersion, wantAsks: false},
				{after: time.Hour, answer: "2.1.300", want: "2.1.300", wantAsks: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
			var now time.Time
			var current step
			asked := false
			cache := &versionCache{
				ask: func() (string, error) {
					asked = true
					if current.fails {
						return "", failed
					}
					return current.answer, nil
				},
				now: func() time.Time { return now },
			}

			for _, s := range tt.steps {
				now, current, asked = start.Add(s.after), s, false
				if got := cache.get(); got != s.want || asked != s.wantAsks {
					t.Errorf("%v on, get() = %q, asking the CLI %v; want %q, asking it %v", s.after, got, asked, s.want, s.wantAsks)
				}
			}
		})
	}
}

func TestVersionCacheLogsEachAsk(t *testing.T) {
	failed := errors.New(`exec: "claude": executable file not found in $PATH`)
	start := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		name string
		// fails says whether each ask, an hour after the one before, fails.
		fails []bool
		// want is what the last ask's line says.
		want []string
	}{
		{
			name:  "a version, at debug",
			fails: []bool{false},
			want:  []string{"level=DEBUG", `msg="read the claude CLI's version" component=claude`, "version=2.1.300"},
		},
		{
			name:  "none before one, at warn, as the floor stands in",
			fails: []bool{true},
			want: []string{
				"level=WARN", `msg="claude CLI gave no version; claiming the floor" component=claude`, "version=2.1.283",
				`error="exec: \"claude\": executable file not found in $PATH"`,
			},
		},
		{
			name:  "none after one, at debug, as that one stands",
			fails: []bool{false, true},
			want:  []string{"level=DEBUG", `msg="claude CLI gave no version; keeping the last" component=claude`, "version=2.1.300", "error="},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			var now time.Time
			var fails bool
			cache := &versionCache{
				ask: func() (string, error) {
					if fails {
						return "", failed
					}
					return "2.1.300", nil
				},
				now: func() time.Time { return now },
			}

			for i, f := range tt.fails {
				now, fails = start.Add(time.Duration(i)*time.Hour), f
				cache.get()
			}
			now = now.Add(time.Minute)
			cache.get()

			var asks []string
			for _, line := range log.Lines() {
				if strings.Contains(line, "component=claude") {
					asks = append(asks, line)
				}
			}
			if len(asks) != len(tt.fails) {
				t.Fatalf("log reads\n%s\nwant a line for each of the %d asks", log, len(tt.fails))
			}
			if last := asks[len(asks)-1]; !containsAll(last, tt.want) {
				t.Errorf("the last ask logged\n%s\nwant a line with %q", last, tt.want)
			}
		})
	}
}

func containsAll(s string, parts []string) bool {
	return !slices.ContainsFunc(parts, func(part string) bool { return !strings.Contains(s, part) })
}
