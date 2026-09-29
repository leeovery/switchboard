package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
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
		{name: "a session without an id", args: []string{"status", "--session", ""}, wantUsage: true},
		{name: "a session's status without the router", args: []string{"status", "--session", "0b5c6f2e"}, wantUsage: false},
		{name: "pin without an account", args: []string{"pin"}, wantUsage: true},
		{name: "pin with two accounts", args: []string{"pin", "work", "side"}, wantUsage: true},
		{name: "pin auto, moving sessions", args: []string{"pin", "auto", "--move"}, wantUsage: true},
		{name: "pin without the router", args: []string{"pin", "work"}, wantUsage: false},
		{name: "unexpected usage argument", args: []string{"usage", "extra"}, wantUsage: true},
		{name: "invalid config for usage", args: []string{"usage", "--config", invalid}, wantUsage: false},
		{name: "invalid watch interval", args: []string{"usage", "--watch", "soon"}, wantUsage: true},
		{name: "invalid config for usage --watch", args: []string{"usage", "--watch", "--config", invalid}, wantUsage: false},
		{name: "unknown log", args: []string{"logs", "extra"}, wantUsage: true},
		{name: "number of lines that isn't one", args: []string{"logs", "-n", "many"}, wantUsage: true},
		{name: "log that doesn't exist yet", args: []string{"logs", "router"}, wantUsage: false},
		{name: "unexpected serve argument", args: []string{"serve", "extra"}, wantUsage: true},
		{name: "unknown log level", args: []string{"serve", "--log-level", "loud"}, wantUsage: true},
		{name: "invalid config for serve", args: []string{"serve", "--config", invalid}, wantUsage: false},
		{name: "claude's flags without --", args: []string{"run", "--resume"}, wantUsage: true},
		{name: "claude's arguments before --", args: []string{"run", "a prompt", "--", "--print"}, wantUsage: true},
		{name: "an account without its id", args: []string{"run", "--account", ""}, wantUsage: true},
		{name: "an account on claude's own login", args: []string{"run", "--account", "work", "--direct"}, wantUsage: true},
		{name: "run when claude can't start", args: []string{"run", "--direct"}, wantUsage: false},
		{name: "invalid config for run", args: []string{"run", "--config", invalid}, wantUsage: false},
		{name: "init without a shell", args: []string{"init"}, wantUsage: true},
		{name: "init for another shell", args: []string{"init", "bash"}, wantUsage: true},
		{name: "a prefix that can't start a name", args: []string{"init", "zsh", "--prefix=cx;"}, wantUsage: true},
		{name: "invalid config for init", args: []string{"init", "zsh", "--config", invalid}, wantUsage: false},
		{name: "unknown service command", args: []string{"service", "start"}, wantUsage: true},
		{name: "unexpected service install argument", args: []string{"service", "install", "extra"}, wantUsage: true},
		{name: "service install without launchctl", args: []string{"service", "install"}, wantUsage: false},
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

func TestCommandsLogOnlyToTheirLog(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), map[string]string{"SWITCHBOARD_LOG_LEVEL": "debug"})

	got := run(t, deps, "status", "--json")
	if got.code != 0 || got.stderr != "" || !json.Valid([]byte(got.stdout)) {
		t.Fatalf("switchboard status --json = %+v, want exit status 0, JSON alone on stdout and nothing on stderr", got)
	}
	if want := run(t, statusDeps(t, fakeClaudeAPI(t), nil), "status", "--json").stdout; got.stdout != want {
		t.Errorf("logging at debug, status --json printed\n%s\nwant what it prints at info\n%s", got.stdout, want)
	}
	log := readLog(t, deps, "cli.log")
	for _, want := range [][]string{
		{"level=DEBUG", "msg=start component=process", "role=cli", `command="switchboard status"`},
		{"level=DEBUG", `msg="loaded config" component=cli`, "accounts=3"},
		{"level=DEBUG", `msg="probed account" component=status`, "account=work", "windows=3"},
		{"level=DEBUG", `msg="not probed: token missing" component=status`, "account=personal"},
		{"level=WARN", `msg="probe failed" component=status`, "account=side", `error="HTTP 401 · Invalid bearer token"`},
		{"level=DEBUG", `msg="probed accounts" component=cli`, "accounts=3", "best=work", "claude_version=" + testClaudeVersion},
		{"level=DEBUG", "msg=exit component=process", "status=0", "duration="},
	} {
		if !hasLine(log, want...) {
			t.Errorf("cli.log reads\n%s\nwant a line with %q", log, want)
		}
	}
	for _, token := range []string{"test-token-work", "test-token-side"} {
		if strings.Contains(log, token) {
			t.Errorf("cli.log shows the token set for an account:\n%s", log)
		}
	}
}

func TestFailedCommandsAreLogged(t *testing.T) {
	path := writeConfig(t, "[[account]]\nid = \"work\"\n")
	deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": path}, t.TempDir())

	if got := run(t, deps, "status"); got.code != 1 {
		t.Fatalf("switchboard status = %+v, want exit status 1", got)
	}
	log := readLog(t, deps, "cli.log")
	want := []string{"level=WARN", `msg="command failed" component=cli`, `command="switchboard status"`, `error="invalid config ` + path + `:\naccount \"work\": token_env is required`}
	if !hasLine(log, want...) {
		t.Errorf("cli.log reads\n%s\nwant a line with %q", log, want)
	}
}

func TestAStatuslinesEverydayFailuresAreLoggedAtDebug(t *testing.T) {
	const session = "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e"
	tests := []struct {
		name string
		// running starts the router first.
		running bool
		args    []string
		wantErr string
		// wantLevel is the level the failure is logged at.
		wantLevel string
	}{
		{
			name:      "a session's status without the router",
			args:      []string{"status", "--session", session},
			wantErr:   "the router isn't running: start it with switchboard serve",
			wantLevel: "DEBUG",
		},
		{
			name:      "a session's status the router hasn't seen",
			running:   true,
			args:      []string{"status", "--session", session, "--json"},
			wantErr:   "the router hasn't seen session " + session,
			wantLevel: "DEBUG",
		},
		{
			name:      "a pin without the router",
			args:      []string{"pin", "side"},
			wantErr:   "the router isn't running: start it with switchboard serve",
			wantLevel: "WARN",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"SWITCHBOARD_LOG_LEVEL": "debug"})
			if tt.running {
				srv.start(t)
			}

			got := run(t, srv.deps, tt.args...)
			if want := (result{stderr: "Error: " + tt.wantErr + "\n", code: 1}); got != want {
				t.Errorf("switchboard %s = %+v, want %+v", strings.Join(tt.args, " "), got, want)
			}
			log := srv.cliLog(t)
			want := []string{"level=" + tt.wantLevel, `msg="command failed"`, `command="switchboard ` + tt.args[0] + `"`, `error="` + tt.wantErr + `"`}
			if !hasLine(log, want...) {
				t.Errorf("cli.log reads\n%s\nwant a line with %q", log, want)
			}
			other := map[string]string{"DEBUG": "WARN", "WARN": "DEBUG"}[tt.wantLevel]
			if hasLine(log, "level="+other, `msg="command failed"`) {
				t.Errorf("cli.log reads\n%s\nwant the failure at %s alone", log, tt.wantLevel)
			}
		})
	}
}

func TestCommandsSucceedWhenTheyCantLog(t *testing.T) {
	tests := []struct {
		name string
		// block stops deps logging.
		block func(t *testing.T, deps *cli.Deps)
	}{
		{
			name: "with a file where the log directory should be",
			block: func(t *testing.T, deps *cli.Deps) {
				home, _ := deps.HomeDir()
				state := filepath.Join(home, ".local", "state", "switchboard")
				if err := os.MkdirAll(state, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(state, "logs"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "without a home directory to find the state directory in",
			block: func(_ *testing.T, deps *cli.Deps) {
				deps.HomeDir = func() (string, error) { return "", errors.New("no home directory") }
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			deps := statusDeps(t, fakeClaudeAPI(t), map[string]string{"SWITCHBOARD_LOG_LEVEL": "debug"})
			tt.block(t, &deps)

			got := run(t, deps, "status", "--json")
			if want := run(t, statusDeps(t, fakeClaudeAPI(t), nil), "status", "--json"); got != want {
				t.Errorf("switchboard status --json = %+v, want what it gives when it can log, %+v", got, want)
			}
			if entries, err := os.ReadDir("."); err != nil || len(entries) > 0 {
				t.Errorf("working directory holds %v (%v), want no log there", entries, err)
			}
		})
	}
}

func TestLogsUnderXDGStateHome(t *testing.T) {
	state := t.TempDir()
	path := writeConfig(t, "[[account]]\nid = \"work\"\ntoken_env = \"CLAUDE_TOKEN_WORK\"\n")
	deps := testDeps(map[string]string{"XDG_STATE_HOME": state, "SWITCHBOARD_CONFIG": path, "SWITCHBOARD_LOG_LEVEL": "debug"}, t.TempDir())
	run(t, deps, "accounts")

	data, err := os.ReadFile(filepath.Join(state, "switchboard", "logs", "cli.log"))
	if err != nil || !strings.Contains(string(data), `command="switchboard accounts"`) {
		t.Errorf("$XDG_STATE_HOME/switchboard/logs/cli.log holds %q (%v), want the command's start", data, err)
	}
}

func TestACommandCanLogAsTheRouter(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	root := cli.NewRootCommand(deps)
	root.AddCommand(&cobra.Command{
		Use:         "as-router",
		Annotations: map[string]string{cli.RoleAnnotation: "router"},
		RunE:        func(*cobra.Command, []string) error { return nil },
	})
	root.SetArgs([]string{"as-router"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	if code := cli.Execute(root); code != 0 {
		t.Fatalf("switchboard as-router exited %d, want 0", code)
	}
	log := readLog(t, deps, "router.log")
	for _, want := range [][]string{
		{"level=INFO", "msg=start component=process", "role=router", `command="switchboard as-router"`},
		{"level=INFO", "msg=exit component=process", "status=0"},
	} {
		if !hasLine(log, want...) {
			t.Errorf("router.log reads\n%s\nwant a line with %q", log, want)
		}
	}
	if _, err := os.Stat(filepath.Join(logDir(t, deps), "cli.log")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("cli.log: %v, want no such file", err)
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
// directory, a stopped clock, a fixed Claude Code version and a notifier that
// posts nothing, so no test reads the real ones, runs the real claude or
// posts a notification. The switchboard binary can't be found, nor can
// claude on PATH, and starting a program or running launchctl fails, unless
// a test says otherwise; the system is macOS, for the user 501. Watch is the
// real one: a test's output is never a terminal, so it fails before it would
// take one over.
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
		Executable:    func() (string, error) { return "", errors.New("no test knows its switchboard binary") },
		Now:           func() time.Time { return testNow },
		ClaudeVersion: func() string { return testClaudeVersion },
		Watch:         watch.Run,
		Notifier:      &recordingNotifier{},
		LookPath:      func(string) (string, error) { return "", exec.ErrNotFound },
		Exec:          func(string, []string, []string) error { return errors.New("no test starts a program") },
		Launchctl: func(context.Context, ...string) ([]byte, error) {
			return nil, errors.New("no test runs launchctl")
		},
		GOOS: "darwin",
		UID:  501,
	}
}

// recordingNotifier notes each notification it's given, and posts none.
type recordingNotifier struct {
	mu       sync.Mutex
	messages []string
}

func (n *recordingNotifier) Notify(message string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.messages = append(n.messages, message)
	return nil
}

// posted returns the notifications given so far.
func (n *recordingNotifier) posted() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.messages)
}

// logDir is where commands run with deps log.
func logDir(t *testing.T, deps cli.Deps) string {
	t.Helper()
	home, err := deps.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".local", "state", "switchboard", "logs")
}

// readLog returns what the log called name holds, for commands run with deps.
func readLog(t *testing.T, deps cli.Deps, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(logDir(t, deps), name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// hasLine reports whether a line of log contains every one of parts.
func hasLine(log string, parts ...string) bool {
	for line := range strings.Lines(log) {
		if !slices.ContainsFunc(parts, func(part string) bool { return !strings.Contains(line, part) }) {
			return true
		}
	}
	return false
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
