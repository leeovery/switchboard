package cli_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/shell"
)

func TestServeBringsTheShellFilesUpToDate(t *testing.T) {
	data := t.TempDir()
	srv := newServeSetup(t, fakeClaudeAPI(t), map[string]string{"XDG_DATA_HOME": data})
	dir := filepath.Join(data, "switchboard", "shell")
	bin := filepath.Join(data, "switchboard", "bin")
	zsh, bash, fish := shell.InitFile(dir, shell.Zsh), shell.InitFile(dir, shell.Bash), shell.InitFile(dir, shell.Fish)
	bashCompletion := shell.CompletionFile(dir)
	writeShellFile(t, zsh, "# an older switchboard's\n")
	writeShellFile(t, bash, string(shellFile(t, srv.deps, shell.Bash, dir, bin)))
	writeShellFile(t, bashCompletion, "# an older switchboard's\n")

	srv.start(t)
	for _, want := range [][]string{
		{"level=INFO", `msg="brought switchboard's file for the shell up to date"`, "path=" + zsh},
		{"level=INFO", `msg="switchboard's file for the shell is up to date"`, "path=" + bash},
		{"level=INFO", `msg="brought switchboard's file for the shell up to date"`, "path=" + bashCompletion},
		{"level=INFO", `msg="no file of switchboard's for the shell to bring up to date; setup writes it"`, "path=" + fish},
	} {
		if log := srv.routerLog(t); !hasLine(log, want...) {
			t.Errorf("router.log reads\n%s\nwant a line with %q", log, want)
		}
	}
	for _, sh := range []shell.Shell{shell.Zsh, shell.Bash} {
		path := shell.InitFile(dir, sh)
		if got, want := readFile(t, path), string(shellFile(t, srv.deps, sh, dir, bin)); got != want {
			t.Errorf("%s reads\n%s\nwant this switchboard's\n%s", path, got, want)
		}
	}
	if got, want := readFile(t, bashCompletion), string(completion(t, srv.deps, shell.Bash)); got != want {
		t.Errorf("%s reads\n%s\nwant this switchboard's completions for bash\n%s", bashCompletion, got, want)
	}
	if _, err := os.Lstat(fish); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s: %v, want it not written, as setup didn't", fish, err)
	}
}

func TestTheShellFilesLoadSwitchboardsCompletions(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	const dir, bin = "/data/switchboard/shell", "/data/switchboard/bin"
	for sh, want := range map[shell.Shell]string{
		shell.Zsh:  "compdef _switchboard switchboard",
		shell.Fish: "complete -c switchboard -e",
	} {
		if text := string(shellFile(t, deps, sh, dir, bin)); !strings.Contains(text, want) {
			t.Errorf("switchboard's file for %s reads\n%s\nwant it to load switchboard's completions, with %q", sh, text, want)
		}
	}
	if text, want := string(shellFile(t, deps, shell.Bash, dir, bin)), "\t. '/data/switchboard/shell/completion.bash'\n"; !strings.Contains(text, want) {
		t.Errorf("switchboard's file for bash reads\n%s\nwant it to load switchboard's completions from their file, with %q", text, want)
	}
	if text, want := string(completion(t, deps, shell.Bash)), "complete -o default -F __start_switchboard switchboard"; !strings.Contains(text, want) {
		t.Errorf("switchboard's completions for bash read\n%s\nwant %q", text, want)
	}
}

func TestTheCompletionsCompleteCommandsAndFlags(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	tests := []struct {
		name string
		args []string
		want []string
		// not is what's never completed.
		not []string
	}{
		{name: "commands", args: []string{"__complete", ""}, want: []string{"\nsetup\t", "\nstatus\t", "\nusage\t"}, not: []string{"\ncompletion\t"}},
		{name: "a command's flags", args: []string{"__complete", "status", "--"}, want: []string{"\n--json\t"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, deps, tt.args...)
			out := "\n" + got.stdout
			for _, want := range tt.want {
				if got.code != 0 || !strings.Contains(out, want) {
					t.Errorf("switchboard %s = %+v, want it to complete %q", strings.Join(tt.args, " "), got, strings.TrimSpace(want))
				}
			}
			for _, not := range tt.not {
				if strings.Contains(out, not) {
					t.Errorf("switchboard %s printed\n%s\nwant no %q: the completion command stays disabled", strings.Join(tt.args, " "), got.stdout, strings.TrimSpace(not))
				}
			}
		})
	}
}

// shellFile is switchboard's file for sh, in dir, its shell directory, bin
// its bin directory, as a command tree from deps writes it, its completions
// loaded.
func shellFile(t *testing.T, deps cli.Deps, sh shell.Shell, dir, bin string) []byte {
	t.Helper()
	text, err := shell.Init(sh, dir, bin, completion(t, deps, sh))
	if err != nil {
		t.Fatal(err)
	}
	return text
}

// completion is switchboard's completion script for sh, as a command tree
// from deps writes it.
func completion(t *testing.T, deps cli.Deps, sh shell.Shell) []byte {
	t.Helper()
	root := cli.NewRootCommand(deps)
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

// writeShellFile writes text to the file at path, making its directory.
func writeShellFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
