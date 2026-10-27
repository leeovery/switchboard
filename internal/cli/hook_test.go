package cli_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/sessionhook"
)

// notice is what the session hook prints for a session that didn't start
// through switchboard.
var notice = string(sessionhook.Notice(nil, false))

func TestSessionStartHook(t *testing.T) {
	tests := []struct {
		name  string
		input string
		// speaks is set where a session switchboard didn't start hears it
		// isn't going through switchboard.
		speaks bool
	}{
		{name: "starting", input: `{"session_id":"abc","hook_event_name":"SessionStart","source":"startup"}`, speaks: true},
		{name: "resumed", input: `{"source":"resume"}`, speaks: true},
		{name: "cleared", input: `{"source":"clear"}`},
		{name: "compacted", input: `{"source":"compact"}`},
		{name: "without a source", input: `{"session_id":"abc"}`, speaks: true},
		{name: "without input", input: "", speaks: true},
		{name: "junk", input: "not json\n", speaks: true},
		{name: "past what's read", input: `{"source":"clear","padding":"` + strings.Repeat("x", 64<<10) + `"}`, speaks: true},
	}
	for _, tt := range tests {
		for _, marked := range []bool{false, true} {
			name := tt.name + ", unmarked"
			env := map[string]string{}
			if marked {
				name = tt.name + ", started by switchboard"
				env["SWITCHBOARD_STARTED"] = "5150:1790000000:/opt/claude/bin/claude"
			}
			t.Run(name, func(t *testing.T) {
				// A config that can't be read, which the hook never reads.
				env["SWITCHBOARD_CONFIG"] = writeConfig(t, "not toml")
				deps := testDeps(env, t.TempDir())

				want := result{}
				if tt.speaks && !marked {
					want.stdout = notice
				}
				if got := runWithInput(t, deps, tt.input, "hook", "session-start"); got != want {
					t.Errorf("switchboard hook session-start = %+v, want %+v", got, want)
				}
			})
		}
	}
}

func TestSessionStartHookLogsWhatItCantRead(t *testing.T) {
	deps := testDeps(nil, t.TempDir())

	got := runWithInput(t, deps, strings.Repeat(" ", 64<<10+1), "hook", "session-start")
	if want := (result{stdout: notice}); got != want {
		t.Errorf("switchboard hook session-start = %+v, want %+v", got, want)
	}
	if log := readLog(t, deps, "cli.log"); !hasLine(log, "level=WARN", `msg="can't read the session hook's input"`) {
		t.Errorf("cli.log reads\n%s\nwant the input that couldn't be read noted", log)
	}
}

func TestSessionStartHookReadsNothingFromATerminal(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	deps.TerminalInput = func(io.Reader) bool { return true }

	got := execute(t, deps, unreadable{t}, &bytes.Buffer{}, "hook", "session-start")
	if want := (result{stdout: notice}); got != want {
		t.Errorf("switchboard hook session-start = %+v, want %+v", got, want)
	}
}

func TestSessionStartHookSucceedsWhateverFails(t *testing.T) {
	t.Run("its log can't be written", func(t *testing.T) {
		home := t.TempDir()
		blocked := filepath.Join(home, "a-file")
		if err := os.WriteFile(blocked, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		deps := testDeps(map[string]string{"XDG_STATE_HOME": filepath.Join(blocked, "state")}, home)

		got := runWithInput(t, deps, `{"source":"startup"}`, "hook", "session-start")
		if want := (result{stdout: notice}); got != want {
			t.Errorf("switchboard hook session-start = %+v, want %+v", got, want)
		}
	})
	t.Run("there's no home, so no log", func(t *testing.T) {
		deps := testDeps(nil, "")
		deps.HomeDir = func() (string, error) { return "", errors.New("no home") }

		got := runWithInput(t, deps, `{"source":"startup"}`, "hook", "session-start")
		if want := (result{stdout: notice}); got != want {
			t.Errorf("switchboard hook session-start = %+v, want %+v", got, want)
		}
	})
	t.Run("its output can't be written", func(t *testing.T) {
		deps := testDeps(nil, t.TempDir())

		got := execute(t, deps, strings.NewReader(`{"source":"startup"}`), failingWriter{}, "hook", "session-start")
		if want := (result{}); got != want {
			t.Errorf("switchboard hook session-start = %+v, want %+v", got, want)
		}
		if log := readLog(t, deps, "cli.log"); !hasLine(log, "level=WARN", `msg="can't tell the session it isn't going through switchboard"`) {
			t.Errorf("cli.log reads\n%s\nwant the failed write noted", log)
		}
	})
}

func TestTheHookCommandsAreHidden(t *testing.T) {
	got := run(t, testDeps(nil, t.TempDir()), "--help")
	if got.code != 0 || strings.Contains(got.stdout, "hook") {
		t.Errorf("switchboard --help = %+v, want the hook commands left out", got)
	}
}

// execute runs the command line in-process with deps, stdin and stdout as
// given, returning what it wrote to stdout where that's a buffer.
func execute(t *testing.T, deps cli.Deps, stdin io.Reader, stdout io.Writer, args ...string) result {
	t.Helper()
	root := cli.NewRootCommand(deps)
	var stderr bytes.Buffer
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(&stderr)
	code := cli.Execute(root)
	got := result{stderr: stderr.String(), code: code}
	if b, ok := stdout.(*bytes.Buffer); ok {
		got.stdout = b.String()
	}
	return got
}

// unreadable is a stdin no test may read, as a terminal waits for typing.
type unreadable struct{ t *testing.T }

func (u unreadable) Read([]byte) (int, error) {
	u.t.Error("the hook read from a terminal")
	return 0, io.EOF
}

// failingWriter is a stdout that takes nothing.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }
