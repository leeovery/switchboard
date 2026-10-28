package shell_test

import (
	"cmp"
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
			name: "the link, a prompt's mark before each answer, as a hook prints it, passed over",
			out:  strings.ReplaceAll(strings.ReplaceAll(said(claudeLink, claudeLink, ""), "\npath:", "\n\x1b]133;C\x07path:"), "\nis:", "\n\x1b]133;C\x1b\\is:"),
			want: linked,
		},
		{
			name: "another claude, colour codes about its answer passed over",
			out:  strings.ReplaceAll(said(other, other, ""), "\npath:", "\n\x1b[0m\x1b[?2004lpath:"),
			want: shell.Answer{Kind: shell.Other, Claude: other},
		},
		{
			name: "a path for PATH's claude that isn't one: no answer",
			out:  said("claude", "claude", ""),
			want: shell.Answer{Why: `it answered "claude" for the claude on its PATH, which isn't a path`},
		},
		{
			name: "an alias in front of the link, said",
			out:  said("alias claude='claude --verbose'", claudeLink, ""),
			want: shell.Answer{Kind: shell.Link, Front: shell.Front{Text: "alias claude='claude --verbose'", Runs: shell.ByName}},
		},
		{
			name: "an alias in front of another claude, said",
			out:  said("alias claude=/opt/homebrew/bin/claude", other, ""),
			want: shell.Answer{Kind: shell.Other, Claude: other, Front: shell.Front{Text: "alias claude=/opt/homebrew/bin/claude", Runs: shell.APath, Path: other}},
		},
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

func TestWhatsInFrontOfPATHsClaudeIsDecidedByWhatItRuns(t *testing.T) {
	home := t.TempDir()
	claudeLink := filepath.Join(home, ".local", "share", "switchboard", "bin", "claude")
	write(t, filepath.Join(home, "switchboard-binary"), "")
	link(t, filepath.Join(home, "switchboard-binary"), claudeLink)
	const other = "/opt/homebrew/bin/claude"
	// The old local installer's, from the home.
	local := filepath.Join(home, ".claude", "local", "claude")
	theLink := func(text string) shell.Front { return shell.Front{Text: text, Runs: shell.TheLink} }
	byName := func(text string) shell.Front { return shell.Front{Text: text, Runs: shell.ByName} }
	aPath := func(text, path string) shell.Front { return shell.Front{Text: text, Runs: shell.APath, Path: path} }
	runsElse := func(text string) shell.Front { return shell.Front{Text: text, Runs: shell.Else} }
	unknown := func(text string) shell.Front { return shell.Front{Text: text, Runs: shell.Unknown} }
	setsPath := func(text string) shell.Front { return shell.Front{Text: text, Runs: shell.SetsPath} }
	// quoted is the alias whose body is body, as bash and zsh give it.
	quoted := func(body string) string { return "alias claude='" + strings.ReplaceAll(body, "'", `'\''`) + "'" }
	tests := []struct {
		name string
		// sh is the shell asked, zsh where it's not set.
		sh   string
		is   string
		want shell.Front
	}{
		{name: "nothing", is: claudeLink, want: shell.Front{}},
		{name: "a function, by its name alone", is: "claude", want: unknown("function claude")},
		{name: "fish's function", is: "function claude", want: unknown("function claude")},

		// Leading variables: passed over, but for one that sets PATH.
		{name: "variables set for claude, passed over", is: `alias claude='ANTHROPIC_LOG=debug FOO="a b" claude --verbose'`, want: byName(`alias claude='ANTHROPIC_LOG=debug FOO="a b" claude --verbose'`)},
		{name: "a variable added to for claude, passed over", is: "alias claude='FOO+=1 claude'", want: byName("alias claude='FOO+=1 claude'")},
		{
			name: "a variable holding a token, hidden",
			is:   "alias claude='ANTHROPIC_API_KEY=sk-ant-oat01-fake_token-shaped claude'",
			want: byName("alias claude='ANTHROPIC_API_KEY=[redacted] claude'"),
		},
		{name: "PATH set for claude", is: "alias claude='PATH=/opt/real/bin:$PATH claude'", want: setsPath("alias claude='PATH=/opt/real/bin:$PATH claude'")},
		{name: "PATH set for claude, in bash, after another variable", sh: "/bin/bash", is: "alias claude='FOO=1 PATH=/opt/real/bin:$PATH claude'", want: setsPath("alias claude='FOO=1 PATH=/opt/real/bin:$PATH claude'")},
		{name: "PATH added to for claude, in bash", sh: "/bin/bash", is: "alias claude='PATH+=:/opt/real/bin claude'", want: setsPath("alias claude='PATH+=:/opt/real/bin claude'")},
		{name: "PATH added to for claude, in zsh", is: "alias claude='PATH+=:/opt/real/bin claude'", want: setsPath("alias claude='PATH+=:/opt/real/bin claude'")},
		{name: "zsh's path set for claude", is: "alias claude='path=(/opt/real/bin $path) claude'", want: setsPath("alias claude='path=(/opt/real/bin $path) claude'")},
		{name: "zsh's path added to for claude", is: "alias claude='path+=/opt/real/bin claude'", want: setsPath("alias claude='path+=/opt/real/bin claude'")},
		{name: "a variable named path, in bash, passed over", sh: "/bin/bash", is: "alias claude='path=/opt/real/bin claude'", want: byName("alias claude='path=/opt/real/bin claude'")},
		{name: "variables alone, which run no claude", is: "alias claude='FOO=1'", want: runsElse("alias claude='FOO=1'")},
		{name: "a variable quoted, which is the command", is: `alias claude='"FOO=1" npx'`, want: runsElse(`alias claude='"FOO=1" npx'`)},
		{name: "a variable quoted, which is the command, with claude after it", is: `alias claude='"FOO=1" claude'`, want: unknown(`alias claude='"FOO=1" claude'`)},
		{name: "PATH quoted, which is the command", is: `alias claude='"PATH=/opt/real/bin" claude'`, want: unknown(`alias claude='"PATH=/opt/real/bin" claude'`)},

		// Leading variables: what they're set to read whole, as the shell reads it.
		{name: "a variable set by a command's output", is: "alias claude='FOO=$(echo a b) claude'", want: byName("alias claude='FOO=$(echo a b) claude'")},
		{name: "a variable set by a command's output, quoted, a separator within", is: `alias claude='FOO="$(echo "a;b")" claude'`, want: byName(`alias claude='FOO="$(echo "a;b")" claude'`)},
		{name: "a variable set by arithmetic", is: "alias claude='FOO=$((1 + 2)) claude'", want: byName("alias claude='FOO=$((1 + 2)) claude'")},
		{name: "a variable set by a parameter", is: "alias claude='FOO=${BAR:-a b} claude'", want: byName("alias claude='FOO=${BAR:-a b} claude'")},
		{name: "a variable set by backquotes", is: "alias claude='FOO=`echo a b` claude'", want: byName("alias claude='FOO=`echo a b` claude'")},
		{name: "PATH set by a command's output", is: "alias claude='PATH=$(echo /opt/real/bin) claude'", want: setsPath("alias claude='PATH=$(echo /opt/real/bin) claude'")},
		{name: "a command's output that isn't closed", is: "alias claude='FOO=$(echo a claude'", want: unknown("alias claude='FOO=$(echo a claude'")},

		// PATH set by a word after another command, as env's.
		{name: "env, and PATH", is: "alias claude='env PATH=/opt/real/bin claude'", want: setsPath("alias claude='env PATH=/opt/real/bin claude'")},
		{name: "env -i, and PATH", sh: "/bin/bash", is: "alias claude='env -i PATH=/opt/real/bin claude'", want: setsPath("alias claude='env -i PATH=/opt/real/bin claude'")},
		{name: "env, and PATH+=, a variable env names PATH+", is: "alias claude='env PATH+=/opt/real/bin claude'", want: unknown("alias claude='env PATH+=/opt/real/bin claude'")},
		{name: "env, and zsh's path+=, a variable env names path+", is: "alias claude='env path+=/opt/real/bin claude'", want: unknown("alias claude='env path+=/opt/real/bin claude'")},
		{name: "env, and a variable named path, in bash", sh: "/bin/bash", is: "alias claude='env path=/opt/real/bin claude'", want: unknown("alias claude='env path=/opt/real/bin claude'")},
		{name: "env, and PATH quoted", is: `alias claude='env "PATH"=/opt/real/bin claude'`, want: unknown(`alias claude='env "PATH"=/opt/real/bin claude'`)},
		{name: "env, and PATH after claude, for it", is: "alias claude='env claude PATH=/opt/real/bin'", want: unknown("alias claude='env claude PATH=/opt/real/bin'")},
		{name: "env, and PATH, without claude", is: "alias claude='env PATH=/opt/real/bin npx'", want: runsElse("alias claude='env PATH=/opt/real/bin npx'")},

		// PATH set by a command before claude's.
		{name: "PATH set, then claude", is: "alias claude='PATH=/opt/real/bin; claude'", want: setsPath("alias claude='PATH=/opt/real/bin; claude'")},
		{name: "PATH exported, then claude", is: "alias claude='export PATH=/opt/real/bin:$PATH; claude'", want: setsPath("alias claude='export PATH=/opt/real/bin:$PATH; claude'")},
		{name: "cd, then PATH set for claude", is: "alias claude='cd /opt/work && PATH=/opt/real/bin claude'", want: setsPath("alias claude='cd /opt/work && PATH=/opt/real/bin claude'")},
		{name: "another variable exported, then claude", is: "alias claude='export FOO=1; claude'", want: unknown("alias claude='export FOO=1; claude'")},
		{name: "PATH set, then another command", is: "alias claude='PATH=/opt/real/bin; npx'", want: runsElse("alias claude='PATH=/opt/real/bin; npx'")},
		{name: "claude, then PATH set for another", is: "alias claude='claude; PATH=/opt/real/bin claude'", want: byName("alias claude='claude; PATH=/opt/real/bin claude'")},

		// The first command: claude by name.
		{name: "claude", is: "alias claude='claude --verbose'", want: byName("alias claude='claude --verbose'")},
		{name: "claude, unquoted", is: "alias claude=claude", want: byName("alias claude=claude")},
		{name: "claude, escaped", sh: "/bin/bash", is: `alias claude='\claude --verbose'`, want: byName(`alias claude='\claude --verbose'`)},
		{name: "claude, quoted within", is: `alias claude='"claude" --verbose'`, want: byName(`alias claude='"claude" --verbose'`)},
		{name: "claude, as bash quotes a quote", sh: "/bin/bash", is: `alias claude='claude --append-system-prompt '\''be brief'\'''`, want: byName(`alias claude='claude --append-system-prompt '\''be brief'\'''`)},
		{name: "claude, then another command", is: "alias claude='claude; echo done'", want: byName("alias claude='claude; echo done'")},
		{name: "command claude", is: "alias claude='command claude'", want: byName("alias claude='command claude'")},
		{name: "command -- claude", sh: "/bin/bash", is: "alias claude='command -- claude'", want: byName("alias claude='command -- claude'")},
		{name: "zsh's =claude, PATH's claude by its path", is: "alias claude='=claude --verbose'", want: byName("alias claude='=claude --verbose'")},
		{name: "zsh's =claude, quoted, a name", is: `alias claude='"=claude"'`, want: runsElse(`alias claude='"=claude"'`)},
		{name: "=claude, in bash, a name", sh: "/bin/bash", is: "alias claude='=claude'", want: runsElse("alias claude='=claude'")},
		{name: "claude in $'…'", is: "alias claude=$'claude\\t--verbose'", want: byName("alias claude=$'claude\\t--verbose'")},
		{name: "claude in $'…' within", is: quoted(`$'claude' --verbose`), want: byName(quoted(`$'claude' --verbose`))},
		{name: "claude in $'…', an escape not decoded", is: quoted(`$'\x63laude'`), want: unknown(quoted(`$'\x63laude'`))},
		{name: "claude in $'…', as zsh gives it, an escape not decoded", is: `alias claude=$'\x63laude'`, want: unknown(`alias claude=$'\x63laude'`)},

		// The first command: a path.
		{name: "another claude by its path", is: "alias claude=" + other, want: aPath("alias claude="+other, other)},
		{name: "another claude by its path, quoted", sh: "/bin/bash", is: "alias claude='" + other + " --verbose'", want: aPath("alias claude='"+other+" --verbose'", other)},
		{name: "the old local installer's alias, from ~", is: "alias claude=~/.claude/local/claude", want: aPath("alias claude=~/.claude/local/claude", local)},
		{name: "the old local installer's alias, from $HOME", is: `alias claude='$HOME/.claude/local/claude'`, want: aPath(`alias claude='$HOME/.claude/local/claude'`, local)},
		{name: "the old local installer's alias, expanded as it was set", is: "alias claude=" + local, want: aPath("alias claude="+local, local)},
		{name: "the link by its path", is: "alias claude=" + claudeLink, want: shell.Front{Text: "alias claude=" + claudeLink, Runs: shell.TheLink}},
		{name: "command, and another claude by its path", is: "alias claude='command " + other + "'", want: aPath("alias claude='command "+other+"'", other)},
		{name: "the link, from ~", is: "alias claude='~/.local/share/switchboard/bin/claude'", want: theLink("alias claude='~/.local/share/switchboard/bin/claude'")},
		{name: "the link, from $HOME", is: "alias claude='$HOME/.local/share/switchboard/bin/claude'", want: theLink("alias claude='$HOME/.local/share/switchboard/bin/claude'")},
		{name: "the link, from ${HOME}", is: "alias claude='${HOME}/.local/share/switchboard/bin/claude'", want: theLink("alias claude='${HOME}/.local/share/switchboard/bin/claude'")},
		{name: "the link, from $HOME double quoted", is: `alias claude='"$HOME/.local/share/switchboard/bin/claude"'`, want: theLink(`alias claude='"$HOME/.local/share/switchboard/bin/claude"'`)},
		{name: "the link, from ${HOME} double quoted, alone", sh: "/bin/bash", is: `alias claude='"${HOME}"/.local/share/switchboard/bin/claude'`, want: theLink(`alias claude='"${HOME}"/.local/share/switchboard/bin/claude'`)},
		{name: "the link, from $HOME double quoted, alone", is: `alias claude='"$HOME"/.local/share/switchboard/bin/claude'`, want: theLink(`alias claude='"$HOME"/.local/share/switchboard/bin/claude'`)},
		{name: "the link, from $HOME, an empty quote after it", is: `alias claude='$HOME""/.local/share/switchboard/bin/claude'`, want: theLink(`alias claude='$HOME""/.local/share/switchboard/bin/claude'`)},
		{name: "the link, the rest quoted", is: `alias claude='~/".local/share/switchboard/bin/claude"'`, want: theLink(`alias claude='~/".local/share/switchboard/bin/claude"'`)},
		{name: "a path quoted, its glob not one", is: `alias claude='"/opt/homebrew/bin/cl*de"'`, want: aPath(`alias claude='"/opt/homebrew/bin/cl*de"'`, "/opt/homebrew/bin/cl*de")},

		// The first command: a home split across quotes, which the shells
		// read apart.
		{name: "$ quoted apart from HOME", is: `alias claude='"$"HOME/.local/share/switchboard/bin/claude'`, want: unknown(`alias claude='"$"HOME/.local/share/switchboard/bin/claude'`)},
		{name: "$ quoted apart from {HOME}", is: `alias claude='"$"{HOME}/.local/share/switchboard/bin/claude'`, want: unknown(`alias claude='"$"{HOME}/.local/share/switchboard/bin/claude'`)},
		{name: "HOME quoted apart from $", sh: "/bin/bash", is: `alias claude='$"HOME"/.local/share/switchboard/bin/claude'`, want: unknown(`alias claude='$"HOME"/.local/share/switchboard/bin/claude'`)},
		{name: "$HOM quoted apart from E", is: `alias claude='"$HOM"E/.local/share/switchboard/bin/claude'`, want: unknown(`alias claude='"$HOM"E/.local/share/switchboard/bin/claude'`)},
		{name: "$H quoted apart from OME", is: `alias claude='"$H"OME/.local/share/switchboard/bin/claude'`, want: unknown(`alias claude='"$H"OME/.local/share/switchboard/bin/claude'`)},
		{name: "$HOME in two double quotes", is: `alias claude='"$H""OME"/.local/share/switchboard/bin/claude'`, want: unknown(`alias claude='"$H""OME"/.local/share/switchboard/bin/claude'`)},
		{name: "~ and an empty quote, which bash reads as it is", sh: "/bin/bash", is: `alias claude='~""/.local/share/switchboard/bin/claude'`, want: unknown(`alias claude='~""/.local/share/switchboard/bin/claude'`)},
		{name: "~ and its / quoted, which zsh expands", is: `alias claude='~"/.local/share/switchboard/bin/claude"'`, want: unknown(`alias claude='~"/.local/share/switchboard/bin/claude"'`)},
		{name: "an empty quote, then ~", is: `alias claude='""~/.local/share/switchboard/bin/claude'`, want: unknown(`alias claude='""~/.local/share/switchboard/bin/claude'`)},
		{name: "the home alone", is: "alias claude='~'", want: unknown("alias claude='~'")},

		// The first command: a glob or a brace, which the shell expands.
		{name: "a glob in a path", is: "alias claude='~/.local/share/switchboard/bin/cl*de'", want: unknown("alias claude='~/.local/share/switchboard/bin/cl*de'")},
		{name: "a glob's class in a path", is: "alias claude='/opt/homebrew/bin/[c]laude'", want: unknown("alias claude='/opt/homebrew/bin/[c]laude'")},
		{name: "a glob in a name", is: "alias claude='cl?ude'", want: unknown("alias claude='cl?ude'")},
		{name: "a brace in a name", sh: "/bin/bash", is: "alias claude='{claude,x}'", want: unknown("alias claude='{claude,x}'")},

		// The first command: a path quoted, which the shell doesn't expand.
		{name: "~ double quoted, a relative path", is: `alias claude='"~/.local/share/switchboard/bin/claude"'`, want: runsElse(`alias claude='"~/.local/share/switchboard/bin/claude"'`)},
		{name: "~ single quoted, a relative path", sh: "/bin/bash", is: `alias claude=''\''~/.local/share/switchboard/bin/claude'\'''`, want: runsElse(`alias claude=''\''~/.local/share/switchboard/bin/claude'\'''`)},
		{name: "~ escaped, a relative path", is: `alias claude='\~/.local/share/switchboard/bin/claude'`, want: runsElse(`alias claude='\~/.local/share/switchboard/bin/claude'`)},
		{name: "$HOME single quoted, a relative path", sh: "/bin/bash", is: `alias claude=''\''$HOME/.local/share/switchboard/bin/claude'\'''`, want: runsElse(`alias claude=''\''$HOME/.local/share/switchboard/bin/claude'\'''`)},
		{name: "$HOME escaped, a relative path", is: `alias claude='\$HOME/.local/share/switchboard/bin/claude'`, want: runsElse(`alias claude='\$HOME/.local/share/switchboard/bin/claude'`)},

		// The first command: a path only its expansion tells.
		{name: "another user's home", is: "alias claude='~other/.local/share/switchboard/bin/claude'", want: unknown("alias claude='~other/.local/share/switchboard/bin/claude'")},
		{name: "a variable within a path", is: "alias claude='/home/$USER/.local/share/switchboard/bin/claude'", want: unknown("alias claude='/home/$USER/.local/share/switchboard/bin/claude'")},
		{name: "a variable after $HOME", is: "alias claude='$HOME/$BIN/claude'", want: unknown("alias claude='$HOME/$BIN/claude'")},
		{name: "a variable whose name starts HOME", is: "alias claude='$HOMEBIN/claude'", want: unknown("alias claude='$HOMEBIN/claude'")},
		{name: "backquotes within a path", sh: "/bin/bash", is: "alias claude='/home/`whoami`/.local/share/switchboard/bin/claude'", want: unknown("alias claude='/home/`whoami`/.local/share/switchboard/bin/claude'")},

		// The first command: builtin, or command with an option, another
		// command, with claude after it.
		{name: "builtin, in zsh", is: "alias claude='builtin claude'", want: unknown("alias claude='builtin claude'")},
		{name: "builtin, in bash", sh: "/bin/bash", is: "alias claude='builtin claude'", want: unknown("alias claude='builtin claude'")},
		{name: "builtin, and another", is: "alias claude='builtin echo hi'", want: runsElse("alias claude='builtin echo hi'")},
		{name: "command -p, on the system's PATH", sh: "/bin/bash", is: "alias claude='command -p claude'", want: unknown("alias claude='command -p claude'")},
		{name: "command -v, which prints", is: "alias claude='command -v claude'", want: unknown("alias claude='command -v claude'")},
		{name: "command -V, which says", sh: "/bin/bash", is: "alias claude='command -V claude'", want: unknown("alias claude='command -V claude'")},
		{name: "command -v, and another", is: "alias claude='command -v npx'", want: runsElse("alias claude='command -v npx'")},
		{name: "command alone", is: "alias claude=command", want: runsElse("alias claude=command")},

		// The first command: a wrapper, with claude after it, setup doesn't read.
		{name: "time -p, in bash", sh: "/bin/bash", is: "alias claude='time -p claude'", want: unknown("alias claude='time -p claude'")},
		{name: "time -p, in zsh", is: "alias claude='time -p claude'", want: unknown("alias claude='time -p claude'")},
		{name: "noglob nocorrect, in zsh", is: "alias claude='noglob nocorrect claude'", want: unknown("alias claude='noglob nocorrect claude'")},
		{name: "noglob, in bash", sh: "/bin/bash", is: "alias claude='noglob claude'", want: unknown("alias claude='noglob claude'")},
		{name: "caffeinate -i", is: "alias claude='caffeinate -i claude'", want: unknown("alias claude='caffeinate -i claude'")},
		{name: "nohup", is: "alias claude='nohup claude'", want: unknown("alias claude='nohup claude'")},
		{name: "exec", sh: "/bin/bash", is: "alias claude='exec claude'", want: unknown("alias claude='exec claude'")},
		{name: "env, and a variable", is: "alias claude='env FOO=1 claude'", want: unknown("alias claude='env FOO=1 claude'")},
		{name: "what only its expansion tells", is: `alias claude='$CLAUDE_BIN'`, want: unknown(`alias claude='$CLAUDE_BIN'`)},
		{name: "a quote that isn't closed", is: `alias claude='"claude'`, want: unknown(`alias claude='"claude'`)},
		{name: "a wrapper, and another claude by its path", is: "alias claude='caffeinate -i " + other + "'", want: unknown("alias claude='caffeinate -i " + other + "'")},
		{name: "a wrapper, and the link from ~", is: "alias claude='caffeinate -i ~/.local/share/switchboard/bin/claude'", want: unknown("alias claude='caffeinate -i ~/.local/share/switchboard/bin/claude'")},

		// A compound alias, or a subshell, its first command another, and
		// claude later.
		{name: "cd, and claude", is: "alias claude='cd ~/work && claude'", want: unknown("alias claude='cd ~/work && claude'")},
		{name: "clear, and claude", is: "alias claude='clear; claude'", want: unknown("alias claude='clear; claude'")},
		{name: "a pipe into claude", sh: "/bin/bash", is: "alias claude='true | claude'", want: unknown("alias claude='true | claude'")},
		{name: "another, or claude by its path", is: "alias claude='false || " + other + "'", want: unknown("alias claude='false || " + other + "'")},
		{name: "claude in a subshell", is: "alias claude='(claude)'", want: unknown("alias claude='(claude)'")},
		{name: "a variable alone, and claude", is: "alias claude='FOO=1; claude'", want: unknown("alias claude='FOO=1; claude'")},
		{name: "builtin, and claude", is: "alias claude='builtin claude; claude'", want: unknown("alias claude='builtin claude; claude'")},
		{name: "command -v, and claude", is: "alias claude='command -v claude && claude'", want: unknown("alias claude='command -v claude && claude'")},
		{name: "claude, or true", is: "alias claude='claude || true'", want: byName("alias claude='claude || true'")},
		{name: "a redirect first, which ends the command", is: "alias claude='>/dev/null claude'", want: unknown("alias claude='>/dev/null claude'")},
		{name: "claude, a redirect after it", is: "alias claude='claude 2>/dev/null'", want: byName("alias claude='claude 2>/dev/null'")},

		// A claude within a word, in a group or quotes, after another command.
		{name: "a command's output", is: "alias claude='echo $(claude)'", want: unknown("alias claude='echo $(claude)'")},
		{name: "a wrapper, and claude's path by which", is: "alias claude='caffeinate -i $(which claude)'", want: unknown("alias claude='caffeinate -i $(which claude)'")},
		{name: "a wrapper, and claude's path by command -v, quoted", is: `alias claude='nohup "$(command -v claude)"'`, want: unknown(`alias claude='nohup "$(command -v claude)"'`)},
		{name: "a wrapper, and claude in its own quotes within the group", is: `alias claude='nohup $(command -v cl"aude")'`, want: unknown(`alias claude='nohup $(command -v cl"aude")'`)},
		{name: "bash -c", is: quoted(`bash -c 'claude --verbose'`), want: unknown(quoted(`bash -c 'claude --verbose'`))},
		{name: "tmux new", is: quoted(`tmux new 'claude --verbose'`), want: unknown(quoted(`tmux new 'claude --verbose'`))},
		{name: "a wrapper, and claude in $'…'", is: quoted(`bash -c $'claude'`), want: unknown(quoted(`bash -c $'claude'`))},
		{name: "a wrapper, and $'…' not decoded", is: quoted(`bash -c $'\x63laude'`), want: unknown(quoted(`bash -c $'\x63laude'`))},
		{name: "a wrapper, and claude-code, another name", is: "alias claude='nohup claude-code'", want: runsElse("alias claude='nohup claude-code'")},
		{name: "a wrapper, and .claude, a directory", is: "alias claude='cd ~/.claude && npx @anthropic-ai/claude-code'", want: runsElse("alias claude='cd ~/.claude && npx @anthropic-ai/claude-code'")},

		// A comment, from a # that starts a word.
		{name: "claude, and a comment", is: quoted(`claude --verbose  # don't ask`), want: byName(quoted(`claude --verbose  # don't ask`))},
		{name: "another, and claude in a comment", is: "alias claude='npx foo # claude'", want: runsElse("alias claude='npx foo # claude'")},
		{name: "another, and a # within a word", is: "alias claude='npx foo#claude'", want: unknown("alias claude='npx foo#claude'")},

		// The quotes within a word, read as the shell reads them.
		{name: `\$ within double quotes`, is: `alias claude='"\$HOME/.local/share/switchboard/bin/claude"'`, want: runsElse(`alias claude='"\$HOME/.local/share/switchboard/bin/claude"'`)},
		{name: `\" within double quotes`, is: `alias claude='FOO="a\"b" claude'`, want: byName(`alias claude='FOO="a\"b" claude'`)},
		{name: "a backslash within single quotes", is: quoted(`FOO='a\' claude`), want: byName(quoted(`FOO='a\' claude`))},
		{name: `\' within $'…'`, is: quoted(`FOO=$'a\'b' claude`), want: byName(quoted(`FOO=$'a\'b' claude`))},
		{name: `$'…' holding claude between \'`, is: quoted(`FOO=$'\' claude '\' npx`), want: runsElse(quoted(`FOO=$'\' claude '\' npx`))},
		{name: "a parameter that isn't closed", is: "alias claude='FOO=${BAR claude'", want: unknown("alias claude='FOO=${BAR claude'")},

		// The first command: something else, with no claude after it.
		{name: "npx", is: "alias claude='npx @anthropic-ai/claude-code'", want: runsElse("alias claude='npx @anthropic-ai/claude-code'")},
		{name: "another name", is: "alias claude=claude-code", want: runsElse("alias claude=claude-code")},
		{name: "a wrapper, and another name by its path", is: "alias claude='caffeinate -i /opt/homebrew/bin/claude-code'", want: runsElse("alias claude='caffeinate -i /opt/homebrew/bin/claude-code'")},
		{name: "cd, and another command", is: "alias claude='cd ~/work && npx @anthropic-ai/claude-code'", want: runsElse("alias claude='cd ~/work && npx @anthropic-ai/claude-code'")},
		{name: "an empty alias", is: "alias claude=''", want: runsElse("alias claude=''")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var runs []ran
			env := slices.Clone(environ)
			env[slices.Index(env, "SHELL=/bin/zsh")] = "SHELL=" + cmp.Or(tt.sh, "/bin/zsh")
			env[slices.Index(env, "HOME=/home/tester")] = "HOME=" + home
			login := shell.Login{Environ: env, Run: fakeShell(said(tt.is, claudeLink, ""), nil, &runs)}

			if got := login.Which(t.Context(), claudeLink); got.Front != tt.want {
				t.Errorf("Which().Front = %+v, want %+v", got.Front, tt.want)
			}
		})
	}
}

func FuzzWhatsInFrontOfPATHsClaude(f *testing.F) {
	for _, body := range []string{
		"claude --verbose", `FOO=$'\' claude '\' npx`, `"$H""OME"/.local/share/switchboard/bin/claude`,
		"cd ~/work && claude", "path=(/opt/real/bin $path) claude", "(claude", "FOO=$(", "${", "`", `$'\`, `"\`, "# claude",
	} {
		f.Add(body)
	}
	f.Fuzz(func(t *testing.T, body string) {
		for _, sh := range []string{"/bin/zsh", "/bin/bash"} {
			var runs []ran
			env := slices.Clone(environ)
			env[slices.Index(env, "SHELL=/bin/zsh")] = "SHELL=" + sh
			login := shell.Login{Environ: env, Run: fakeShell(said("alias claude="+body, "/opt/homebrew/bin/claude", ""), nil, &runs)}

			if got := login.Which(t.Context(), "/data/switchboard/bin/claude").Front.Runs; got < shell.ByName || got > shell.SetsPath {
				t.Errorf("%s: Which(%q).Front.Runs = %d, not one of Runs", sh, body, got)
			}
		}
	})
}

func TestWhetherClaudeGoesThroughSwitchboard(t *testing.T) {
	tests := []struct {
		name   string
		answer shell.Answer
		want   bool
	}{
		{name: "PATH's the link", answer: shell.Answer{Kind: shell.Link}, want: true},
		{name: "PATH's another", answer: shell.Answer{Kind: shell.Other}},
		{name: "PATH's none", answer: shell.Answer{Kind: shell.None}},
		{name: "unasked", answer: shell.Answer{Why: "it didn't answer within 15s"}},
		{name: "PATH's the link, an alias to the name in front", answer: shell.Answer{Kind: shell.Link, Front: shell.Front{Runs: shell.ByName}}, want: true},
		{name: "PATH's another, an alias to the name in front", answer: shell.Answer{Kind: shell.Other, Front: shell.Front{Runs: shell.ByName}}},
		{name: "PATH's the link, a function in front, which may run it", answer: shell.Answer{Kind: shell.Link, Front: shell.Front{Runs: shell.Unknown}}, want: true},
		{name: "PATH's another, an alias to the link's path in front", answer: shell.Answer{Kind: shell.Other, Front: shell.Front{Runs: shell.TheLink}}, want: true},
		{name: "PATH's the link, an alias to another path in front", answer: shell.Answer{Kind: shell.Link, Front: shell.Front{Runs: shell.APath}}},
		{name: "PATH's the link, an alias to something else in front", answer: shell.Answer{Kind: shell.Link, Front: shell.Front{Runs: shell.Else}}},
		{name: "PATH's the link, an alias in front that sets PATH for claude", answer: shell.Answer{Kind: shell.Link, Front: shell.Front{Runs: shell.SetsPath}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.answer.Through(); got != tt.want {
				t.Errorf("Through() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAShellSwitchboardDoesntKnowIsntAsked(t *testing.T) {
	for _, path := range []string{"/bin/ksh", "/bin/tcsh", "/opt/homebrew/bin/nu", "/opt/homebrew/bin/xonsh"} {
		t.Run(path, func(t *testing.T) {
			var runs []ran
			env := slices.Clone(environ)
			env[slices.Index(env, "SHELL=/bin/zsh")] = "SHELL=" + path
			login := shell.Login{Environ: env, Run: fakeShell(said("", "/opt/homebrew/bin/claude", ""), nil, &runs)}

			if got, want := login.Which(t.Context(), "/data/switchboard/bin/claude"), (shell.Answer{Why: "switchboard asks zsh, bash and fish alone"}); got != want {
				t.Errorf("Which() = %+v, want %+v", got, want)
			}
			if len(runs) > 0 {
				t.Errorf("ran %+v, want nothing run", runs)
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
		// quoted is how it gives an alias's value.
		quoted func(string) string
		config string
	}{
		{
			path: "/bin/zsh", args: []string{"-f", "-i"}, config: home,
			quoted: func(v string) string {
				if strings.Contains(v, " ") {
					return "'" + v + "'"
				}
				return v
			},
		},
		{path: "/bin/bash", args: []string{"--noprofile", "--norc", "-i"}, quoted: func(v string) string { return "'" + v + "'" }},
	}
	// alias is what's in front, as a shell giving an alias's value as quoted
	// does.
	alias := func(quoted func(string) string, value string, runs shell.Runs, path string) shell.Front {
		return shell.Front{Text: "alias claude=" + quoted(value), Runs: runs, Path: path}
	}
	tests := []struct {
		name string
		// before defines claude in front of PATH's, before the shell's asked.
		before string
		path   string
		// want is the answer, config standing for where it reads its startup
		// file; and front what's in front, as the shell gives it, quoted.
		want  shell.Answer
		front func(quoted func(string) string) shell.Front
	}{
		{name: "the link", path: bin, want: shell.Answer{Kind: shell.Link, ConfigDir: "config"}},
		{name: "another", path: filepath.Dir(other) + ":" + bin, want: shell.Answer{Kind: shell.Other, Claude: other, ConfigDir: "config"}},
		{name: "none", path: "/usr/bin", want: shell.Answer{Kind: shell.None, ConfigDir: "config"}},
		{
			name:   "an alias to the name, in front of the link",
			before: "alias claude='claude --verbose'; ",
			path:   bin, want: shell.Answer{Kind: shell.Link, ConfigDir: "config"},
			front: func(q func(string) string) shell.Front { return alias(q, "claude --verbose", shell.ByName, "") },
		},
		{
			name:   "an alias to another claude by its path, in front of the link",
			before: "alias claude=" + other + "; ",
			path:   bin, want: shell.Answer{Kind: shell.Link, ConfigDir: "config"},
			front: func(q func(string) string) shell.Front { return alias(q, other, shell.APath, other) },
		},
		{
			name:   "an alias to the link by its path, in front of another claude",
			before: "alias claude=" + claudeLink + "; ",
			path:   filepath.Dir(other), want: shell.Answer{Kind: shell.Other, Claude: other, ConfigDir: "config"},
			front: func(q func(string) string) shell.Front { return alias(q, claudeLink, shell.TheLink, "") },
		},
		{
			name:   "a function, in front of the link",
			before: "claude() { command claude \"$@\"; }; ",
			path:   bin, want: shell.Answer{Kind: shell.Link, ConfigDir: "config"},
			front: func(func(string) string) shell.Front {
				return shell.Front{Text: "function claude", Runs: shell.Unknown}
			},
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
				if tt.front != nil {
					want.Front = tt.front(s.quoted)
				}
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
