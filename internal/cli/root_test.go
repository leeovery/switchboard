package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
)

func TestVersion(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	deps.Version = "1.2.3"

	got := run(t, deps, "--version")
	want := result{stdout: "switchboard version 1.2.3\n", code: 0}
	if got != want {
		t.Errorf("switchboard --version = %+v, want %+v", got, want)
	}
}

func TestNoCommandPrintsHelp(t *testing.T) {
	got := run(t, testDeps(nil, t.TempDir()))
	if got.code != 0 || !strings.Contains(got.stdout, "Usage:") || !strings.Contains(got.stdout, "accounts") {
		t.Errorf("switchboard = %+v, want help listing the commands", got)
	}
}

func TestConfigFlagOverridesResolution(t *testing.T) {
	fromEnv := writeConfig(t, "[[account]]\nid = \"from-env\"\ntoken_env = \"CLAUDE_TOKEN_WORK\"\n")
	fromFlag := writeConfig(t, "[[account]]\nid = \"from-flag\"\ntoken_env = \"CLAUDE_TOKEN_WORK\"\n")
	deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": fromEnv}, t.TempDir())

	got := run(t, deps, "accounts", "--config", fromFlag)
	if got.code != 0 || !strings.Contains(got.stdout, "from-flag") || strings.Contains(got.stdout, "from-env") {
		t.Errorf("switchboard accounts --config = %+v, want the accounts from the --config file", got)
	}
}

func TestUsageOnlyFollowsCommandLineMistakes(t *testing.T) {
	invalid := writeConfig(t, "listen = \"0.0.0.0:4747\"\n")
	tests := []struct {
		name      string
		args      []string
		wantUsage bool
	}{
		{name: "unknown flag", args: []string{"accounts", "--bogus"}, wantUsage: true},
		{name: "unexpected argument", args: []string{"accounts", "extra"}, wantUsage: true},
		{name: "invalid config", args: []string{"accounts", "--config", invalid}, wantUsage: false},
		{name: "unexpected status argument", args: []string{"status", "extra"}, wantUsage: true},
		{name: "invalid config for status", args: []string{"status", "--json", "--config", invalid}, wantUsage: false},
		{name: "unexpected usage argument", args: []string{"usage", "extra"}, wantUsage: true},
		{name: "invalid config for usage", args: []string{"usage", "--config", invalid}, wantUsage: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, testDeps(nil, t.TempDir()), tt.args...)
			if got.code != 1 {
				t.Errorf("exit status = %d, want 1", got.code)
			}
			if printed := strings.Contains(got.stdout+got.stderr, "Usage:"); printed != tt.wantUsage {
				t.Errorf("usage printed = %v, want %v; output:\n%s%s", printed, tt.wantUsage, got.stdout, got.stderr)
			}
		})
	}
}

// result is what one run of the command line printed and returned.
type result struct {
	stdout, stderr string
	code           int
}

// run executes the command line in-process with deps, capturing its output.
func run(t *testing.T, deps cli.Deps, args ...string) result {
	t.Helper()
	if args == nil {
		args = []string{} // cobra parses os.Args when given nil
	}
	root := cli.NewRootCommand(deps)
	var stdout, stderr bytes.Buffer
	root.SetArgs(args)
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	code := cli.Execute(root)
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// testNow is the time by every command's clock in tests: a Monday, 13:12 UTC.
var testNow = time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)

// testClaudeVersion is the Claude Code version every command's probes claim in tests.
const testClaudeVersion = "2.1.300"

// testDeps gives commands env as their whole environment, home as their home
// directory, a stopped clock and a fixed Claude Code version, so no test reads
// the real ones or runs the real claude.
func testDeps(env map[string]string, home string) cli.Deps {
	return cli.Deps{
		Getenv: func(key string) string { return env[key] },
		Environ: func() []string {
			var environ []string
			for key, value := range env {
				environ = append(environ, key+"="+value)
			}
			return environ
		},
		HomeDir:       func() (string, error) { return home, nil },
		Now:           func() time.Time { return testNow },
		ClaudeVersion: func() string { return testClaudeVersion },
	}
}

// writeConfig writes a config file under the test's temp dir and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
