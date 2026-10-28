package shell_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/shell"
)

func TestTheInitFileText(t *testing.T) {
	const bin = `/data/it's "$x"/switchboard/bin`
	const completion = "# the completion script\n"
	tests := []struct {
		sh shell.Shell
		// bin is how the file holds the bin directory, quoted for its shell.
		bin string
		// loads is how the completions are loaded, within a guard where the
		// shell can take them: bash's from a file of their own, which sh,
		// reading bash's startup file, needn't parse.
		loads string
	}{
		{sh: shell.Zsh, bin: `	local bin='/data/it'\''s "$x"/switchboard/bin'`, loads: "if (( ${+functions[compdef]} )); then\n" + completion},
		{
			sh:  shell.Bash,
			bin: `__switchboard_bin='/data/it'\''s "$x"/switchboard/bin'`,
			loads: `if [ -n "${BASH_VERSION-}" ] && ! shopt -oq posix && declare -F _get_comp_words_by_ref >/dev/null 2>&1 && [ -r '/data/switchboard/shell/completion.bash' ]; then` +
				"\n\t. '/data/switchboard/shell/completion.bash'\nfi\n",
		},
		{sh: shell.Fish, bin: `	set -l bin '/data/it\'s "$x"/switchboard/bin'`, loads: "\n" + completion},
	}
	for _, tt := range tests {
		t.Run(string(tt.sh), func(t *testing.T) {
			text, err := shell.Init(tt.sh, "/data/switchboard/shell", bin, []byte(completion))
			if err != nil {
				t.Fatal(err)
			}
			got := string(text)
			if !strings.Contains(got, "\n"+tt.bin) {
				t.Errorf("the file reads\n%s\nwant it to hold the bin directory as %s", got, tt.bin)
			}
			if !strings.Contains(got, tt.loads) {
				t.Errorf("the file reads\n%s\nwant it to load the completions as\n%s", got, tt.loads)
			}
			if tt.sh == shell.Bash && strings.Contains(got, completion) {
				t.Errorf("the file reads\n%s\nwant the completion script in a file of its own", got)
			}
			if !strings.HasSuffix(got, "\ntrue\n") {
				t.Errorf("the file reads\n%s\nwant it to end with true, so sourcing it ends with a status of 0", got)
			}
		})
	}
}

func TestInitFailsForAShellSwitchboardDoesntKnow(t *testing.T) {
	if _, err := shell.Init("tcsh", "/data/switchboard/shell", "/bin", nil); err == nil {
		t.Error("Init(tcsh) succeeded, want it to fail: switchboard has no file for tcsh")
	}
}

// realShell is a shell installed here, started to run a script without
// reading any startup file of its own.
type realShell struct {
	sh   shell.Shell
	path string
	args []string
}

// realShells are the shells installed here to source the files in: zsh and
// bash, which macOS has, bash at 3.2, and fish where it's installed, as
// Homebrew puts it.
func realShells(t *testing.T) []realShell {
	t.Helper()
	shells := []realShell{
		{sh: shell.Zsh, path: "/bin/zsh", args: []string{"-f", "-c"}},
		{sh: shell.Bash, path: "/bin/bash", args: []string{"--noprofile", "--norc", "-c"}},
	}
	for _, fish := range []string{"/opt/homebrew/bin/fish", "/usr/local/bin/fish"} {
		if _, err := os.Stat(fish); err == nil {
			return append(shells, realShell{sh: shell.Fish, path: fish, args: []string{"--no-config", "-c"}})
		}
	}
	t.Log("fish isn't installed at /opt/homebrew/bin/fish or /usr/local/bin/fish, so its file isn't sourced in it")
	return shells
}

// sourceInit sources $INIT, what that says going to $OUT.
func (s realShell) sourceInit() string {
	if s.sh == shell.Fish {
		return "source $INIT >>$OUT 2>&1\n"
	}
	return ". \"$INIT\" >>\"$OUT\" 2>&1\n"
}

// quietly runs lines, what they say going to $OUT.
func (s realShell) quietly(lines string) string {
	if s.sh == shell.Fish {
		return "begin\n" + lines + "\nend >>$OUT 2>&1\n"
	}
	return "{\n" + lines + "\n} >>\"$OUT\" 2>&1\n"
}

// statusAndPath prints the status the command before ended with, and PATH,
// a line each, PATH's directories joined by colons.
func (s realShell) statusAndPath() string {
	if s.sh == shell.Fish {
		return "printf '%s\\n%s\\n' $status (string join : $PATH)\n"
	}
	return "printf '%s\\n%s\\n' \"$?\" \"$PATH\"\n"
}

// status prints the status the command before ended with.
func (s realShell) status() string {
	if s.sh == shell.Fish {
		return "echo $status\n"
	}
	return "echo $?\n"
}

// source runs script in s, its environment PATH and HOME, and INIT, the file
// to source, and OUT, where what sourcing it says goes, and returns what the
// script printed.
func (s realShell) source(t *testing.T, script, path, home, init, out string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), s.path, append(s.args, script)...)
	cmd.Env = []string{"PATH=" + path, "HOME=" + home, "INIT=" + init, "OUT=" + out}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	printed, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s failed: %v; it said\n%s", s.path, err, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Errorf("%s said on stderr\n%s", s.path, stderr.String())
	}
	return string(printed)
}

// completion is the completion script for sh of a command named
// switchboard, as switchboard makes its own.
func completion(t *testing.T, sh shell.Shell) []byte {
	t.Helper()
	root := &cobra.Command{Use: "switchboard", CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true}}
	root.AddCommand(&cobra.Command{Use: "setup", Run: func(*cobra.Command, []string) {}})
	var b bytes.Buffer
	var err error
	switch sh {
	case shell.Zsh:
		err = root.GenZshCompletion(&b)
	case shell.Bash:
		err = root.GenBashCompletionV2(&b, true)
	case shell.Fish:
		err = root.GenFishCompletion(&b, true)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestSourcingTheInitFilePutsTheBinDirectoryFirstOnPathOnce(t *testing.T) {
	tests := []struct {
		name string
		// path is PATH, and want what it is once the file is sourced, once
		// or twice, bin standing for the bin directory, which has a space,
		// a quote and a dollar in its name.
		path, want string
		// posix is set where fish takes PATH otherwise, as it keeps empty
		// directories on it as its own.
		posix bool
	}{
		{name: "first already: left as it was, byte for byte", path: "bin:/a:bin:/b", want: "bin:/a:bin:/b"},
		{name: "first alone already", path: "bin", want: "bin"},
		{name: "not on it: put first", path: "/a:/b", want: "bin:/a:/b"},
		{name: "after others: moved first, every other copy removed", path: "/a:bin:/b:bin", want: "bin:/a:/b"},
		{name: "last: moved first", path: "/a:/b:bin", want: "bin:/a:/b"},
		{name: "names alike but not the same are kept, compared exactly", path: "/a:bin/:bin/x:bin:/b", want: "bin:/a:bin/:bin/x:/b"},
		{name: "empty directories kept as they are", path: "/a::bin:/b:", want: "bin:/a::/b:", posix: true},
	}
	for _, s := range realShells(t) {
		for _, tt := range tests {
			if tt.posix && s.sh == shell.Fish {
				continue
			}
			t.Run(string(s.sh)+"/"+tt.name, func(t *testing.T) {
				dir := t.TempDir()
				bin := filepath.Join(dir, `it's "$x" here`, "bin")
				init := writeInit(t, s.sh, dir, bin, completion(t, s.sh))
				out := filepath.Join(dir, "out")
				expand := func(path string) string { return strings.ReplaceAll(path, "bin", bin) }
				script := s.sourceInit() + s.statusAndPath() + s.sourceInit() + s.statusAndPath()

				printed := s.source(t, script, expand(tt.path), filepath.Join(dir, "home"), init, out)

				want := "0\n" + expand(tt.want) + "\n"
				if printed != want+want {
					t.Errorf("sourced once, then again, it ended with the status and PATH\n%s\nwant\n%s", printed, want+want)
				}
				checkSaidNothing(t, out)
			})
		}
	}
}

func TestTheInitFileLoadsCompletionsOnlyWhereTheShellCanTakeThem(t *testing.T) {
	tests := []struct {
		sh shell.Shell
		// ready makes the shell ready for completions as its startup file
		// leaves it, where its own completions are loaded first.
		ready string
		// loaded prints "loaded" where switchboard's completions are.
		loaded string
		// always is set for fish, which takes them as they are.
		always bool
	}{
		{sh: shell.Zsh, ready: "compdef() { :; }\n", loaded: "if (( ${+functions[_switchboard]} )); then print loaded; fi\n"},
		{sh: shell.Bash, ready: "_get_comp_words_by_ref() { :; }\n", loaded: "if complete -p switchboard >/dev/null 2>&1; then echo loaded; fi\n"},
		{sh: shell.Fish, loaded: "if functions -q __switchboard_perform_completion; echo loaded; end\n", always: true},
	}
	installed := make(map[shell.Shell]realShell)
	for _, s := range realShells(t) {
		installed[s.sh] = s
	}
	for _, tt := range tests {
		s, ok := installed[tt.sh]
		if !ok {
			continue
		}
		for _, ready := range []bool{false, true} {
			t.Run(string(tt.sh)+map[bool]string{false: "/not ready", true: "/ready"}[ready], func(t *testing.T) {
				dir := t.TempDir()
				init := writeInit(t, tt.sh, dir, filepath.Join(dir, "bin"), completion(t, tt.sh))
				out := filepath.Join(dir, "out")
				script := s.sourceInit() + s.status() + tt.loaded
				if ready {
					script = tt.ready + script
				}

				printed := s.source(t, script, "/usr/bin:/bin", filepath.Join(dir, "home"), init, out)

				status, rest, _ := strings.Cut(printed, "\n")
				if status != "0" {
					t.Errorf("sourcing it ended with status %s, want 0", status)
				}
				if loaded, want := rest == "loaded\n", ready || tt.always; loaded != want {
					t.Errorf("switchboard's completions loaded: %v, want %v", loaded, want)
				}
				checkSaidNothing(t, out)
			})
		}
	}
}

func TestTheInitFileHoldsUnderTheUsersOptions(t *testing.T) {
	// names lists every variable and function the shell has, into the file
	// it's given, so the file's sourcing can be seen to leave none behind.
	names := map[shell.Shell]string{
		shell.Zsh:  "names() { local f; print -rl -- ${(k)parameters} >\"$1\"; for f in ${(k)functions}; do print -r -- \"function $f\"; done >>\"$1\"; }\n",
		shell.Bash: "names() { compgen -v >\"$1\"; compgen -A function -P 'function ' >>\"$1\"; }\n",
	}
	tests := []struct {
		sh      shell.Shell
		options string
		// ready makes the shell ready for completions, which then load.
		ready string
	}{
		{sh: shell.Zsh, options: "setopt ksh_arrays"},
		{sh: shell.Zsh, options: "setopt sh_word_split"},
		{sh: shell.Zsh, options: "setopt nounset"},
		{sh: shell.Zsh, options: "setopt err_exit"},
		{sh: shell.Zsh, options: "setopt warn_create_global"},
		{sh: shell.Zsh, options: "setopt ksh_arrays sh_word_split nounset err_exit warn_create_global"},
		{sh: shell.Zsh, options: "setopt ksh_arrays sh_word_split nounset err_exit warn_create_global", ready: "compdef() { :; }\n"},
		{sh: shell.Bash, options: "set -eu"},
		{sh: shell.Bash, options: "set -eu", ready: "_get_comp_words_by_ref() { :; }\n"},
	}
	installed := make(map[shell.Shell]realShell)
	for _, s := range realShells(t) {
		installed[s.sh] = s
	}
	for _, tt := range tests {
		s := installed[tt.sh]
		name := string(tt.sh) + "/" + tt.options + map[bool]string{false: "", true: ", its completions loaded"}[tt.ready != ""]
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, `it's "$x" here`, "bin")
			init := writeInit(t, tt.sh, dir, bin, completion(t, tt.sh))
			out := filepath.Join(dir, "out")
			before, after := filepath.Join(dir, "before"), filepath.Join(dir, "after")
			script := names[tt.sh] + tt.ready + tt.options + "\n" +
				"names " + before + "\n" + s.sourceInit() + s.statusAndPath() + "names " + after + "\n"

			printed := s.source(t, script, "/a:"+bin+":/b", filepath.Join(dir, "home"), init, out)

			if want := "0\n" + bin + ":/a:/b\n"; printed != want {
				t.Errorf("sourcing it ended with the status and PATH\n%s\nwant\n%s", printed, want)
			}
			checkSaidNothing(t, out)
			if tt.ready != "" {
				return
			}
			had := lines(t, before)
			for _, n := range lines(t, after) {
				if !slices.Contains(had, n) {
					t.Errorf("sourcing it left %q defined, want nothing left", n)
				}
			}
		})
	}
}

func TestBashsFileIsPOSIXShellWhichLoadsNoCompletions(t *testing.T) {
	// The completion script, here, says it's loaded, where bash's would fail
	// to parse in sh, and stop it reading ~/.profile.
	const completion = "switchboard_completion_loaded=yes\n"
	shells := []realShell{
		{sh: shell.Bash, path: "/bin/sh", args: []string{"-c"}},
		{sh: shell.Bash, path: "/bin/bash", args: []string{"--posix", "--noprofile", "--norc", "-c"}},
		{sh: shell.Bash, path: "/bin/dash", args: []string{"-c"}},
		{sh: shell.Bash, path: "/bin/ksh", args: []string{"-c"}},
	}
	for _, s := range shells {
		t.Run(s.path+" "+strings.Join(s.args[:len(s.args)-1], " "), func(t *testing.T) {
			if _, err := os.Stat(s.path); err != nil {
				t.Skipf("%s isn't installed: %v", s.path, err)
			}
			dir := t.TempDir()
			bin := filepath.Join(dir, `it's "$x" here`, "bin")
			init := writeInit(t, shell.Bash, dir, bin, []byte(completion))
			out := filepath.Join(dir, "out")
			// bash-completion, as it's loaded, so it's POSIX alone that keeps
			// the completions out.
			script := "_get_comp_words_by_ref() { :; }\n" + s.sourceInit() + s.statusAndPath() + "echo \"${switchboard_completion_loaded-no}\"\n"

			printed := s.source(t, script, "/a:"+bin+":/b", filepath.Join(dir, "home"), init, out)

			if want := "0\n" + bin + ":/a:/b\nno\n"; printed != want {
				t.Errorf("sourcing it ended with the status, PATH and whether the completions loaded\n%s\nwant\n%s", printed, want)
			}
			checkSaidNothing(t, out)
		})
	}
}

func TestTheStartupLineSourcesTheFileOnlyWhereItsThere(t *testing.T) {
	for _, s := range realShells(t) {
		for _, there := range []bool{true, false} {
			t.Run(string(s.sh)+map[bool]string{true: "/there", false: "/not there"}[there], func(t *testing.T) {
				home := filepath.Join(t.TempDir(), `it's "$x" home`)
				bin := filepath.Join(home, ".local", "share", "switchboard", "bin")
				dir := filepath.Join(home, ".local", "share", "switchboard", "shell")
				init := shell.InitFile(dir, s.sh)
				if there {
					writeInit(t, s.sh, dir, bin, completion(t, s.sh))
				}
				out := filepath.Join(home, "out")
				write(t, out, "")
				// false comes first, so the status is the line's own.
				script := "false\n" + s.quietly(shell.Line(s.sh, init, home)) + s.statusAndPath()

				printed := s.source(t, script, "/usr/bin:/bin", home, init, out)

				want := "0\n/usr/bin:/bin\n"
				if there {
					want = "0\n" + bin + ":/usr/bin:/bin\n"
				}
				if printed != want {
					t.Errorf("the line ended with the status and PATH\n%s\nwant\n%s", printed, want)
				}
				checkSaidNothing(t, out)
			})
		}
	}
}

// writeInit writes switchboard's files for bin in dir, making it, sh's with
// completion, the others with none, and returns where sh's is.
func writeInit(t *testing.T, sh shell.Shell, dir, bin string, completion []byte) string {
	t.Helper()
	files := shell.Files{Dir: dir, Bin: bin, Completion: func(s shell.Shell) ([]byte, error) {
		if s != sh {
			return nil, nil
		}
		return completion, nil
	}}
	if _, err := files.Write(); err != nil {
		t.Fatal(err)
	}
	return shell.InitFile(dir, sh)
}

// checkSaidNothing checks sourcing the file wrote nothing to out, where its
// output went.
func checkSaidNothing(t *testing.T, out string) {
	t.Helper()
	said, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(said) > 0 {
		t.Errorf("sourcing it said\n%s\nwant nothing", said)
	}
}

// lines are the lines of the file at path.
func lines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}
