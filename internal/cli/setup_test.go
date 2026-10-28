package cli_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/shell"
)

func TestSetupNeedsATerminal(t *testing.T) {
	home := t.TempDir()
	claudeConfig := filepath.Join(t.TempDir(), "claude")
	deps := testDeps(map[string]string{"CLAUDE_CONFIG_DIR": claudeConfig}, home)

	got := run(t, deps, "setup")
	if want := (result{stderr: "Error: " + setup.ErrNoTerminal.Error() + "\n", code: 1}); got != want {
		t.Errorf("switchboard setup = %+v, want %+v", got, want)
	}
	for _, path := range []string{
		filepath.Join(home, ".config", "switchboard", "config.toml"),
		filepath.Join(home, "Library", "LaunchAgents", "io.github.leeovery.switchboard.plist"),
		claudeConfig,
	} {
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: %v, want nothing written", path, err)
		}
	}
}

func TestSetup(t *testing.T) {
	s := newServiceSetup(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "data")
	bin := filepath.Join(data, "switchboard", "bin")
	realClaude := claudetest.Program(t, filepath.Join(root, "claude-code", "claude"))
	claudeConfig := filepath.Join(root, "claude")
	s.setenv("XDG_DATA_HOME", data)
	s.setenv("PATH", filepath.Dir(realClaude))
	s.setenv("CLAUDE_CONFIG_DIR", claudeConfig)
	s.setenv("SHELL", "/bin/zsh")
	s.setenv("LANG", "en_GB.UTF-8")
	var asked [][]string
	s.srv.deps.LoginShell = func(_ context.Context, path string, args, env []string) ([]byte, error) {
		asked = append(asked, slices.Concat([]string{path}, args, env))
		return nil, errors.New("no test runs a login shell")
	}
	atATerminal(&s.srv.deps)
	// Setup prints usage as it stands, which the router's first probes may
	// not have reached: here they have, so it prints them.
	s.launchd.probed = true
	answers := strings.Join([]string{
		"",  // no token for personal, for now
		"",  // no more accounts
		"",  // work, the first, the primary
		"",  // no priming
		"",  // switchboard's line added to .zshrc
		"n", // the login shell, which can't be asked, left
	}, "\n") + "\n"

	got := runWithInput(t, s.srv.deps, answers, "setup")
	if got.code != 0 || got.stderr != "" {
		t.Fatalf("switchboard setup = %+v, want exit status 0, and nothing on stderr", got)
	}
	link := filepath.Join(bin, "claude")
	skill := filepath.Join(claudeConfig, "skills", "switchboard", "SKILL.md")
	shellDir := filepath.Join(data, "switchboard", "shell")
	home, err := s.srv.deps.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	line := `if [ -r "` + shellDir + `/init.zsh" ]; then . "` + shellDir + `/init.zsh"; fi  # switchboard`
	for _, want := range []string{
		"\n1. Accounts\nwork · Work: its token is usable.\npersonal · Personal has no usable token: ",
		"personal has none still: no token given.",
		"The primary is work · Work.\n",
		"\n2. Priming\n",
		"Priming stays off.\n",
		"\n3. The service\nInstalled " + s.plist + ": ",
		"The router is up: healthy, pid " + strconv.Itoa(os.Getpid()) + ".\n",
		"\n4. The claude link\nLinked " + link + " to " + s.binary + ".\n" +
			"Wrote switchboard's files for the shells, in " + shellDir + ": init.zsh, init.bash and init.fish.\n" +
			"Setup can't ask zsh which claude it runs: no test runs a login shell. This line, at the end of ~/.zshrc, has claude go through switchboard",
		"\n" + line + "\nAdd it? [Y/n] Added it to ~/.zshrc.\n",
		"\nCheck again? [Y/n] Left as it is: ",
		"\n5. The skill\nInstalled the skill, which tells Claude what switchboard does under claude: " + skill + "\n",
		"\n6. Usage\n",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("switchboard setup printed\n%s\nwant it to hold\n%s", got.stdout, want)
		}
	}
	_, usage, _ := strings.Cut(got.stdout, "\n6. Usage\n")
	if !strings.HasPrefix(usage, "\n SWITCHBOARD ") || !strings.HasSuffix(usage, "even pace: where you would be now, used evenly\n╎ cap: Work at 95%\n") {
		t.Errorf("setup ended\n%s\nwant it to end printing usage, from its header to its key line", usage)
	}
	for _, label := range []string{"Work", "Personal", "Side"} {
		if printedRow(usage, "5-HOUR", label) == "" || printedRow(usage, "WEEK", label) == "" {
			t.Errorf("setup ended\n%s\nwant it to show %s's windows", usage, label)
		}
	}
	if config := readFile(t, s.srv.config); !strings.Contains(config, "id      = \"work\"\nlabel   = \"Work\"\nreserve = 0.05\nprimary = true\n") {
		t.Errorf("the config reads\n%s\nwant work marked the primary", config)
	}
	s.checkPlist(t, "<key>CLAUDE_CONFIG_DIR</key>\n\t\t<string>"+claudeConfig+"</string>")
	if target, err := os.Readlink(link); err != nil || target != s.binary {
		t.Errorf("%s leads to %q (%v), want %s", link, target, err, s.binary)
	}
	if _, err := os.Stat(skill); err != nil {
		t.Errorf("the skill: %v, want it installed", err)
	}
	for _, sh := range shell.Shells {
		path := shell.InitFile(shellDir, sh)
		if got, want := readFile(t, path), string(shellFile(t, s.srv.deps, sh, bin)); got != want {
			t.Errorf("%s reads\n%s\nwant this switchboard's, its completions loaded\n%s", path, got, want)
		}
	}
	if rc := readFile(t, filepath.Join(home, ".zshrc")); rc != line+"\n" {
		t.Errorf("~/.zshrc reads %q, want switchboard's line alone", rc)
	}
	question := []string{"-l", "-i", "-c", "printf '%s\\n' switchboard-asks-which-claude; command -v claude"}
	env := []string{"LANG=en_GB.UTF-8", "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "SHELL=/bin/zsh", "TMPDIR=" + s.srv.deps.Getenv("TMPDIR")}
	want := slices.Concat([]string{"/bin/zsh"}, question, env)
	for i := range asked {
		slices.Sort(asked[i][len(question)+1:])
	}
	if len(asked) != 2 || !slices.Equal(asked[0], want) {
		t.Errorf("the login shell was asked %q, want twice, as %q: first, and once the line's added", asked, want)
	}
}

// atATerminal has commands run with deps take stdin for a terminal, reading
// what's typed there unseen a line at a time, as it's piped in.
func atATerminal(deps *cli.Deps) {
	deps.Hidden = func(stdin io.Reader) (func() ([]byte, error), bool) {
		return func() ([]byte, error) { return readLine(stdin) }, true
	}
}

// readLine reads r up to the end of the line, without it, a byte at a time,
// so it takes nothing past it.
func readLine(r io.Reader) ([]byte, error) {
	var line []byte
	b := make([]byte, 1)
	for {
		n, err := r.Read(b)
		if n == 1 && b[0] == '\n' {
			return line, nil
		}
		line = append(line, b[:n]...)
		if err != nil {
			return line, err
		}
	}
}
