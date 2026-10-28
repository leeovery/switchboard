package shell_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/shell"
)

// The sentinels the login shell's asked to print its answers between.
const (
	startSentinel = "switchboard-asks-which-claude"
	endSentinel   = "switchboard-asked-which-claude"
)

// said is what the login shell prints between the sentinels: what claude is,
// which claude PATH finds, and where it reads its startup file.
func said(is, path, config string) string {
	return startSentinel + "\nis:" + is + "\npath:" + path + "\nconfig:" + config + "\n" + endSentinel + "\n"
}

// environ is the environment setup runs in.
var environ = []string{
	"HOME=/home/tester", "USER=tester", "LOGNAME=tester", "SHELL=/bin/zsh", "TMPDIR=/tmp/tester",
	"TERM=xterm-256color", "TMUX=/tmp/tmux-501/default,4242,0", "LANG=en_GB.UTF-8", "LC_ALL=en_GB.UTF-8", "LC_CTYPE=UTF-8",
	"PATH=/home/tester/.local/share/switchboard/bin:/usr/bin", "ZDOTDIR=/home/tester/.config/zsh",
	"XDG_DATA_HOME=/home/tester/data", "EDITOR=vi", "LANGUAGE=en", "TERM_PROGRAM=Apple_Terminal", "TMUX_PANE=%1",
}

// loginEnv is the environment the login shell's started in, from environ.
var loginEnv = []string{
	"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
	"HOME=/home/tester", "USER=tester", "LOGNAME=tester", "SHELL=/bin/zsh", "TMPDIR=/tmp/tester",
	"TERM=xterm-256color", "TMUX=/tmp/tmux-501/default,4242,0", "LANG=en_GB.UTF-8", "LC_ALL=en_GB.UTF-8", "LC_CTYPE=UTF-8",
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
	// What iTerm2's shell integration prints as a startup file runs, which
	// ends no line.
	const unended = "\x1b]1337;ShellIntegrationVersion=13;shell=zsh\x07"
	const other = "/opt/homebrew/bin/claude"
	linked := shell.Answer{Kind: shell.Link}
	tests := []struct {
		name string
		out  string
		err  error
		want shell.Answer
	}{
		{name: "the link", out: said(claudeLink, claudeLink, ""), want: linked},
		{name: "the link, after the startup files' banners and escape codes", out: banner + "\n" + said(claudeLink, claudeLink, ""), want: linked},
		{name: "the link, after an escape code that ends no line", out: banner + unended + said(claudeLink, claudeLink, ""), want: linked},
		{name: "the link, by a path that's not quite as it's named", out: said(claudeLink, bin+"//claude", ""), want: linked},
		{name: "the link, through a directory linked to its own", out: said(claudeLink, filepath.Join(linkedBin, "claude"), ""), want: linked},
		{name: "the link, with carriage returns", out: strings.ReplaceAll(said(claudeLink, claudeLink, ""), "\n", "\r\n"), want: linked},
		{name: "the link, what logging out prints after the answers ignored", out: said(claudeLink, claudeLink, "") + "Saving session...\n" + other + "\n", want: linked},
		{name: "another claude", out: banner + "\n" + said(other, other, ""), want: shell.Answer{Kind: shell.Other, Claude: other}},
		{
			name: "another claude, the sentinel in a banner before ignored",
			out:  "echo " + startSentinel + "\n" + said(other, other, ""),
			want: shell.Answer{Kind: shell.Other, Claude: other},
		},
		{
			name: "an alias to the name in front of the link",
			out:  said("alias claude='claude --verbose'", claudeLink, ""),
			want: shell.Answer{Kind: shell.Link, Front: "alias claude='claude --verbose'"},
		},
		{
			name: "an alias to a path in front of the link",
			out:  said("alias claude="+other, claudeLink, ""),
			want: shell.Answer{Kind: shell.Link, Front: "alias claude=" + other},
		},
		{name: "a function in front of the link", out: said("claude", claudeLink, ""), want: shell.Answer{Kind: shell.Link, Front: "function claude"}},
		{name: "fish's function in front of the link", out: said("function claude", claudeLink, ""), want: shell.Answer{Kind: shell.Link, Front: "function claude"}},
		{
			name: "an alias in front of another claude",
			out:  said("alias claude='claude --verbose'", other, ""),
			want: shell.Answer{Kind: shell.Other, Claude: other, Front: "alias claude='claude --verbose'"},
		},
		{
			name: "an alias holding a token, hidden",
			out:  said("alias claude='ANTHROPIC_API_KEY=sk-ant-oat01-fake_token-shaped claude'", claudeLink, ""),
			want: shell.Answer{Kind: shell.Link, Front: "alias claude='ANTHROPIC_API_KEY=[redacted] claude'"},
		},
		{name: "an alias, and no claude on PATH", out: said("alias claude='claude --verbose'", "", ""), want: shell.Answer{Kind: shell.None, Front: "alias claude='claude --verbose'"}},
		{name: "none", out: banner + "\n" + said("", "", ""), want: shell.Answer{Kind: shell.None}},
		{
			name: "none, what logging out prints after the answers ignored",
			out:  said("", "", "") + other + "\n",
			want: shell.Answer{Kind: shell.None},
		},
		{
			name: "where it reads its startup file",
			out:  said(claudeLink, claudeLink, "/home/tester/.config/zsh"),
			want: shell.Answer{Kind: shell.Link, ConfigDir: "/home/tester/.config/zsh"},
		},
		{name: "where it reads its startup file, passed over when relative", out: said(claudeLink, claudeLink, ".config/zsh"), want: linked},
		{
			name: "the start sentinel without the end, as it exited between",
			out:  banner + "\n" + startSentinel + "\nis:\n",
			err:  exitStatus(1),
			want: shell.Answer{Why: "it exited with status 1 without answering: a startup file may start something that needs a terminal, such as tmux"},
		},
		{
			name: "exited without the sentinels",
			out:  banner,
			err:  exitStatus(2),
			want: shell.Answer{Why: "it exited with status 2 without answering: a startup file may start something that needs a terminal, such as tmux"},
		},
		{
			name: "exited with 0, without the sentinels",
			out:  banner,
			want: shell.Answer{Why: "it exited without answering: a startup file may start something that needs a terminal, such as tmux"},
		},
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
			if r.path != "/bin/zsh" || !slices.Equal(r.args[:3], []string{"-l", "-i", "-c"}) || !r.hadTimeout {
				t.Errorf("ran %s %q, with a time limit: %v; want /bin/zsh -l -i -c, with one", r.path, r.args, r.hadTimeout)
			}
			if !slices.Equal(r.env, loginEnv) {
				t.Errorf("ran it in\n%q\nwant\n%q", r.env, loginEnv)
			}
		})
	}
}

func TestEachShellIsAskedInItsOwnWords(t *testing.T) {
	tests := []struct {
		shell string
		// is, path and config are how it's asked what claude is, which
		// claude PATH finds, and where it reads its startup file.
		is, path, config string
	}{
		{shell: "/bin/zsh", is: `"$(command -v claude)"`, path: `"$(whence -p claude)"`, config: `"${ZDOTDIR:-$HOME}"`},
		{shell: "/bin/bash", is: `"$(command -v claude)"`, path: `"$(type -P claude)"`, config: `""`},
		{
			shell: "/opt/homebrew/bin/fish", is: "(functions -q claude; and echo function claude; or command --search claude)",
			path: "(command --search claude)", config: "$__fish_config_dir",
		},
		{shell: "/bin/ksh", is: `"$(command -v claude)"`, path: `"$(command -v claude)"`, config: `""`},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			var runs []ran
			env := slices.Clone(environ)
			env[slices.Index(env, "SHELL=/bin/zsh")] = "SHELL=" + tt.shell
			login := shell.Login{Environ: env, Run: fakeShell("", nil, &runs)}

			login.Which(t.Context(), "/data/switchboard/bin/claude")
			want := []string{"-l", "-i", "-c", strings.Join([]string{
				`printf '%s\n' ` + startSentinel,
				`printf 'is:%s\n' ` + tt.is,
				`printf 'path:%s\n' ` + tt.path,
				`printf 'config:%s\n' ` + tt.config,
				`printf '%s\n' ` + endSentinel,
			}, "; ")}
			if len(runs) != 1 || runs[0].path != tt.shell || !slices.Equal(runs[0].args, want) {
				t.Errorf("ran %+v, want %s %q", runs, tt.shell, want)
			}
		})
	}
}

func TestTheQuestionInARealShell(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "switchboard", "bin")
	claudeLink := filepath.Join(bin, "claude")
	write(t, filepath.Join(dir, "switchboard-binary"), "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(dir, "switchboard-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	link(t, filepath.Join(dir, "switchboard-binary"), claudeLink)
	other := filepath.Join(dir, "other", "claude")
	write(t, other, "#!/bin/sh\n")
	if err := os.Chmod(other, 0o700); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "home")
	shells := []struct {
		path string
		// args start it interactive, reading no startup file, in place of a
		// login shell's.
		args []string
		// front is what it says of an alias in front of PATH's claude, and
		// of a function.
		alias, function string
		config          string
	}{
		{path: "/bin/zsh", args: []string{"-f", "-i"}, alias: "alias claude='claude --verbose'", function: "function claude", config: home},
		{path: "/bin/bash", args: []string{"--noprofile", "--norc", "-i"}, alias: "alias claude='claude --verbose'", function: "function claude"},
	}
	tests := []struct {
		name string
		// before defines claude in front of PATH's, before the shell's asked.
		before string
		path   string
		// want is the answer, front standing for what the shell says of
		// what's in front, config for where it reads its startup file.
		want shell.Answer
	}{
		{name: "the link", path: bin, want: shell.Answer{Kind: shell.Link, ConfigDir: "config"}},
		{name: "another", path: filepath.Dir(other) + ":" + bin, want: shell.Answer{Kind: shell.Other, Claude: other, ConfigDir: "config"}},
		{name: "none", path: "/usr/bin", want: shell.Answer{Kind: shell.None, ConfigDir: "config"}},
		{
			name:   "an alias to the name, in front of the link",
			before: "alias claude='claude --verbose'; ",
			path:   bin, want: shell.Answer{Kind: shell.Link, Front: "alias", ConfigDir: "config"},
		},
		{
			name:   "a function, in front of the link",
			before: "claude() { command claude \"$@\"; }; ",
			path:   bin, want: shell.Answer{Kind: shell.Link, Front: "function", ConfigDir: "config"},
		},
	}
	for _, s := range shells {
		for _, tt := range tests {
			t.Run(filepath.Base(s.path)+"/"+tt.name, func(t *testing.T) {
				// The shell, run as the test's: reading none of its startup
				// files, on PATH, with claude defined before it's asked.
				run := func(ctx context.Context, _ string, args, env []string) ([]byte, error) {
					question := tt.before + args[len(args)-1]
					env = append(slices.DeleteFunc(slices.Clone(env), func(kv string) bool { return strings.HasPrefix(kv, "PATH=") }), "PATH="+tt.path)
					return shell.Run(ctx, s.path, append(slices.Clone(s.args), "-c", question), env)
				}
				env := []string{"SHELL=" + s.path, "HOME=" + home}
				login := shell.Login{Environ: env, Run: run}

				want := tt.want
				want.Front = map[string]string{"alias": s.alias, "function": s.function}[want.Front]
				want.ConfigDir = map[string]string{"config": s.config}[want.ConfigDir]
				if got := login.Which(t.Context(), claudeLink); got != want {
					t.Errorf("Which() = %+v, want %+v", got, want)
				}
			})
		}
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
