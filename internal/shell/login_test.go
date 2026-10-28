package shell_test

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/shell"
)

// sentinel is what the login shell's asked to print before its answer.
const sentinel = "switchboard-asks-which-claude"

// environ is the environment setup runs in.
var environ = []string{
	"HOME=/home/tester", "USER=tester", "LOGNAME=tester", "SHELL=/bin/zsh", "TMPDIR=/tmp/tester",
	"TERM=xterm-256color", "LANG=en_GB.UTF-8", "LC_ALL=en_GB.UTF-8", "LC_CTYPE=UTF-8",
	"PATH=/home/tester/.local/share/switchboard/bin:/usr/bin", "ZDOTDIR=/home/tester/.config/zsh",
	"XDG_DATA_HOME=/home/tester/data", "EDITOR=vi", "LANGUAGE=en", "TERM_PROGRAM=Apple_Terminal",
}

// loginEnv is the environment the login shell's started in, from environ.
var loginEnv = []string{
	"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
	"HOME=/home/tester", "USER=tester", "LOGNAME=tester", "SHELL=/bin/zsh", "TMPDIR=/tmp/tester",
	"TERM=xterm-256color", "LANG=en_GB.UTF-8", "LC_ALL=en_GB.UTF-8", "LC_CTYPE=UTF-8",
}

// ran is a run of the login shell, as a fake runner was given it.
type ran struct {
	path       string
	args, env  []string
	hadTimeout bool
}

// fakeShell answers as a login shell does, with out and err, noting how it
// was run.
func fakeShell(out string, err error, runs *[]ran) shell.Runner {
	return func(ctx context.Context, path string, args, env []string) ([]byte, error) {
		_, deadline := ctx.Deadline()
		*runs = append(*runs, ran{path: path, args: args, env: env, hadTimeout: deadline})
		return []byte(out), err
	}
}

func TestAskingTheLoginShellWhichClaudeItRuns(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "switchboard", "bin")
	claudeLink := filepath.Join(bin, "claude")
	write(t, filepath.Join(dir, "switchboard-binary"), "")
	link(t, filepath.Join(dir, "switchboard-binary"), claudeLink)
	linkedBin := filepath.Join(dir, "linked-bin")
	link(t, bin, linkedBin)
	const banner = "Last login: Mon Oct 26 09:00:00 on ttys001\n\x1b]7;file://host/home\x07\x1b[?2004h"
	tests := []struct {
		name string
		out  string
		err  error
		want shell.Answer
	}{
		{name: "the link", out: sentinel + "\n" + claudeLink + "\n", want: shell.Answer{Kind: shell.Link}},
		{name: "the link, after the startup files' banners and escape codes", out: banner + "\n" + sentinel + "\n" + claudeLink + "\n", want: shell.Answer{Kind: shell.Link}},
		{name: "the link, by a path that's not quite as it's named", out: sentinel + "\n" + bin + "//claude\n", want: shell.Answer{Kind: shell.Link}},
		{name: "the link, through a directory linked to its own", out: sentinel + "\n" + filepath.Join(linkedBin, "claude") + "\n", want: shell.Answer{Kind: shell.Link}},
		{name: "the link, with a carriage return", out: sentinel + "\r\n" + claudeLink + "\r\n", want: shell.Answer{Kind: shell.Link}},
		{name: "another claude", out: banner + "\n" + sentinel + "\n/opt/homebrew/bin/claude\n", want: shell.Answer{Kind: shell.Other, Claude: "/opt/homebrew/bin/claude"}},
		{
			name: "another claude, the sentinel in a banner before ignored",
			out:  "echo " + sentinel + "\n" + sentinel + "\n/opt/homebrew/bin/claude\n",
			want: shell.Answer{Kind: shell.Other, Claude: "/opt/homebrew/bin/claude"},
		},
		{name: "an alias, as given", out: sentinel + "\nalias claude='claude --verbose'\n", want: shell.Answer{Kind: shell.Other, Claude: "alias claude='claude --verbose'"}},
		{name: "a function, as given", out: sentinel + "\nclaude\n", want: shell.Answer{Kind: shell.Other, Claude: "claude"}},
		{
			name: "an alias holding a token, hidden",
			out:  sentinel + "\nalias claude='ANTHROPIC_API_KEY=sk-ant-oat01-fake_token-shaped claude'\n",
			want: shell.Answer{Kind: shell.Other, Claude: "alias claude='ANTHROPIC_API_KEY=[redacted] claude'"},
		},
		{name: "none, as command -v fails", out: banner + "\n" + sentinel + "\n", err: exitStatus(1), want: shell.Answer{Kind: shell.None}},
		{name: "exited without the sentinel", out: banner, err: exitStatus(2), want: shell.Answer{Why: "it exited with status 2 without answering"}},
		{name: "exited with 0, without the sentinel", out: banner, want: shell.Answer{Why: "it exited without answering"}},
		{name: "couldn't be started", err: errNoShell{}, want: shell.Answer{Why: "fork/exec /bin/zsh: no such file or directory"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var runs []ran
			login := shell.Login{Environ: environ, Run: fakeShell(tt.out, tt.err, &runs)}

			if got := login.Which(t.Context(), claudeLink); got != tt.want {
				t.Errorf("Which() = %+v, want %+v", got, tt.want)
			}
			if len(runs) != 1 {
				t.Fatalf("the login shell ran %d times, want once", len(runs))
			}
			r := runs[0]
			wantArgs := []string{"-l", "-i", "-c", "printf '%s\\n' " + sentinel + "; command -v claude"}
			if r.path != "/bin/zsh" || !slices.Equal(r.args, wantArgs) || !r.hadTimeout {
				t.Errorf("ran %s %q, with a time limit: %v; want /bin/zsh %q, with one", r.path, r.args, r.hadTimeout, wantArgs)
			}
			if !slices.Equal(r.env, loginEnv) {
				t.Errorf("ran it in\n%q\nwant\n%q", r.env, loginEnv)
			}
		})
	}
}

func TestFishIsAskedInItsOwnWords(t *testing.T) {
	var runs []ran
	env := slices.Clone(environ)
	env[slices.Index(env, "SHELL=/bin/zsh")] = "SHELL=/opt/homebrew/bin/fish"
	login := shell.Login{Environ: env, Run: fakeShell(sentinel+"\n/opt/homebrew/bin/claude\n", nil, &runs)}

	login.Which(t.Context(), "/data/switchboard/bin/claude")
	want := []string{"-l", "-i", "-c", "printf '%s\\n' " + sentinel + "; command --search claude"}
	if len(runs) != 1 || runs[0].path != "/opt/homebrew/bin/fish" || !slices.Equal(runs[0].args, want) {
		t.Errorf("ran %+v, want /opt/homebrew/bin/fish %q", runs, want)
	}
}

func TestTheLoginShellIsntAskedWithoutShell(t *testing.T) {
	var runs []ran
	env := slices.DeleteFunc(slices.Clone(environ), func(kv string) bool { return strings.HasPrefix(kv, "SHELL=") })
	login := shell.Login{Environ: env, Run: fakeShell("", nil, &runs)}

	if got, want := login.Which(t.Context(), "/data/switchboard/bin/claude"), (shell.Answer{Why: "$SHELL isn't set"}); got != want {
		t.Errorf("Which() = %+v, want %+v", got, want)
	}
	if len(runs) > 0 {
		t.Errorf("ran %+v, want nothing run", runs)
	}
}

func TestTheLoginShellIsGivenATimeLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit time.Duration
		// want is the time the shell's given, and answer what it's taken to
		// say where it runs out, as it's waited out.
		want   time.Duration
		answer *shell.Answer
	}{
		{name: "the one given", limit: time.Second, want: time.Second, answer: &shell.Answer{Why: "it didn't answer within 1s"}},
		{name: "15 seconds, given none", want: 15 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var given time.Duration
			stalls := func(ctx context.Context, _ string, _, _ []string) ([]byte, error) {
				deadline, _ := ctx.Deadline()
				given = time.Until(deadline)
				if tt.answer == nil {
					return nil, exitStatus(1)
				}
				<-ctx.Done()
				return []byte("still starting\n"), ctx.Err()
			}
			login := shell.Login{Environ: environ, Run: stalls, Limit: tt.limit}

			got := login.Which(t.Context(), "/data/switchboard/bin/claude")
			if given > tt.want || given < tt.want/2 {
				t.Errorf("the shell was given %s, want %s", given, tt.want)
			}
			if tt.answer != nil && got != *tt.answer {
				t.Errorf("Which() = %+v, want %+v", got, *tt.answer)
			}
		})
	}
}

func TestShellOf(t *testing.T) {
	tests := []struct {
		path  string
		want  shell.Shell
		known bool
	}{
		{path: "/bin/zsh", want: shell.Zsh, known: true},
		{path: "/opt/homebrew/bin/bash", want: shell.Bash, known: true},
		{path: "/usr/local/bin/fish", want: shell.Fish, known: true},
		{path: "/bin/tcsh", want: "tcsh"},
		{path: ""},
	}
	for _, tt := range tests {
		if got, known := shell.Of(tt.path); got != tt.want || known != tt.known {
			t.Errorf("Of(%q) = %q, %v; want %q, %v", tt.path, got, known, tt.want, tt.known)
		}
	}
}

// exitStatus is a program's exit with a status other than 0, as
// *exec.ExitError reports it.
type exitStatus int

func (e exitStatus) Error() string { return "exit status " + strconv.Itoa(int(e)) }

func (e exitStatus) ExitCode() int { return int(e) }

// errNoShell is the shell's failing to start, as it isn't there.
type errNoShell struct{}

func (errNoShell) Error() string { return "fork/exec /bin/zsh: no such file or directory" }
