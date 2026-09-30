package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
)

func TestLogs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "the last fifty lines unless told, reaching into the rolled-over log", args: []string{"logs"}, want: "old 1\nold 2\nold 3\nold 4\nnew 1\nnew 2\n"},
		{name: "the last lines asked for", args: []string{"logs", "-n", "1"}, want: "new 2\n"},
		{name: "the last lines asked for at length", args: []string{"logs", "--lines", "3"}, want: "old 4\nnew 1\nnew 2\n"},
		{name: "none", args: []string{"logs", "-n", "0"}, want: ""},
		{name: "the log named", args: []string{"logs", "cli", "-n", "3"}, want: "old 4\nnew 1\nnew 2\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps(nil, t.TempDir())
			writeLog(t, deps, "cli.log", "new 1\nnew 2\n")
			writeLog(t, deps, "cli.log.1", "old 1\nold 2\nold 3\nold 4\n")

			if got, want := run(t, deps, tt.args...), (result{stdout: tt.want}); got != want {
				t.Errorf("switchboard %s = %+v, want %+v", strings.Join(tt.args, " "), got, want)
			}
		})
	}
}

func TestLogsShowsTheRoutersOnceThereIsOne(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	writeLog(t, deps, "cli.log", "from a command\n")
	if got := run(t, deps, "logs"); got.stdout != "from a command\n" {
		t.Errorf("before the router has logged, switchboard logs = %+v, want the CLI's", got)
	}

	writeLog(t, deps, "router.log", "from the router\n")
	if got := run(t, deps, "logs"); got.stdout != "from the router\n" {
		t.Errorf("once the router has logged, switchboard logs = %+v, want the router's", got)
	}
	if got := run(t, deps, "logs", "cli"); got.stdout != "from a command\n" {
		t.Errorf("switchboard logs cli = %+v, want the CLI's", got)
	}
}

func TestLogsPath(t *testing.T) {
	home := t.TempDir()
	state := t.TempDir()
	tests := []struct {
		name   string
		env    map[string]string
		router bool
		args   []string
		want   string
	}{
		{name: "the CLI's", args: []string{"logs", "--path"}, want: filepath.Join(home, ".local", "state", "switchboard", "logs", "cli.log")},
		{name: "the router's once there is one", router: true, args: []string{"logs", "--path"}, want: filepath.Join(home, ".local", "state", "switchboard", "logs", "router.log")},
		{name: "the one named", args: []string{"logs", "router", "--path"}, want: filepath.Join(home, ".local", "state", "switchboard", "logs", "router.log")},
		{name: "under XDG_STATE_HOME", env: map[string]string{"XDG_STATE_HOME": state}, args: []string{"logs", "cli", "--path"}, want: filepath.Join(state, "switchboard", "logs", "cli.log")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps(tt.env, home)
			router := filepath.Join(logDir(t, deps), "router.log")
			if err := os.RemoveAll(router); err != nil {
				t.Fatal(err)
			}
			if tt.router {
				writeLog(t, deps, "router.log", "listening\n")
			}

			if got, want := run(t, deps, tt.args...), (result{stdout: tt.want + "\n"}); got != want {
				t.Errorf("switchboard %s = %+v, want %+v", strings.Join(tt.args, " "), got, want)
			}
		})
	}
}

func TestLogsArguments(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "an unknown log", args: []string{"logs", "proxy"}, wantErr: `unknown log "proxy": give router or cli`},
		{name: "a token given as the log, never quoted", args: []string{"logs", tokenShaped}, wantErr: `unknown log "[redacted]": give router or cli`},
		{name: "two logs", args: []string{"logs", "router", "cli"}, wantErr: "give one log, router or cli, not 2"},
		{name: "fewer than no lines", args: []string{"logs", "-n", "-1"}, wantErr: "invalid -n -1: give a number of lines, 0 or more"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, testDeps(nil, t.TempDir()), tt.args...)
			if want := "Error: " + tt.wantErr + "\n"; got.code != 1 || !strings.HasPrefix(got.stderr, want) || !strings.Contains(got.stdout+got.stderr, "Usage:") {
				t.Errorf("switchboard %s = %+v, want exit status 1, an error starting %q, and usage", strings.Join(tt.args, " "), got, want)
			}
		})
	}
}

func TestLogsWithoutALog(t *testing.T) {
	deps := testDeps(nil, t.TempDir())

	got := run(t, deps, "logs", "router")
	want := result{stderr: "Error: no log at " + filepath.Join(logDir(t, deps), "router.log") + "\n", code: 1}
	if got != want {
		t.Errorf("switchboard logs router = %+v, want %+v", got, want)
	}
}

func TestLogsFollow(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	deps.FollowEvery = 5 * time.Millisecond
	path := writeLog(t, deps, "router.log", "one\ntwo\n")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	root := cli.NewRootCommand(deps)
	root.SetContext(ctx)
	root.SetArgs([]string{"logs", "router", "-f", "-n", "1"})
	var stdout syncBuffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	code := make(chan int, 1)
	go func() { code <- cli.Execute(root) }()

	stdout.waitFor(t, "two\n")
	appendTo(t, path, "three\n")
	stdout.waitFor(t, "two\nthree\n")
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	appendTo(t, path, "four\n")
	stdout.waitFor(t, "two\nthree\nfour\n")

	cancel()
	if got := <-code; got != 0 || stderr.Len() > 0 {
		t.Errorf("switchboard logs -f exited %d, printing %q on stderr, once interrupted; want 0 and nothing", got, stderr.String())
	}
}

// writeLog writes the log called name for commands run with deps, and returns
// its path.
func writeLog(t *testing.T, deps cli.Deps, name, content string) string {
	t.Helper()
	dir := logDir(t, deps)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func appendTo(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// syncBuffer is a buffer one goroutine can write while another reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitFor waits a few seconds at most for the buffer to hold exactly want.
func (b *syncBuffer) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for b.String() != want {
		if time.Now().After(deadline) {
			t.Fatalf("printed %q, want %q", b.String(), want)
		}
		time.Sleep(time.Millisecond)
	}
}
