package cli_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/setup"
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
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	realClaude := claudetest.Program(t, filepath.Join(root, "claude-code", "claude"))
	claudeConfig := filepath.Join(root, "claude")
	s.setenv("PATH", bin+string(filepath.ListSeparator)+filepath.Dir(realClaude))
	s.setenv("CLAUDE_CONFIG_DIR", claudeConfig)
	atATerminal(&s.srv.deps)
	answers := strings.Join([]string{
		"", // no token for personal, for now
		"", // no more accounts
		"", // work, the first, the primary
		"", // no priming
		"", // yes, link claude
	}, "\n") + "\n"

	got := runWithInput(t, s.srv.deps, answers, "setup")
	if got.code != 0 || got.stderr != "" {
		t.Fatalf("switchboard setup = %+v, want exit status 0, and nothing on stderr", got)
	}
	link := filepath.Join(bin, "claude")
	skill := filepath.Join(claudeConfig, "skills", "switchboard", "SKILL.md")
	for _, want := range []string{
		"\n1. Accounts\nwork · Work: its token is usable.\npersonal · Personal has no usable token: ",
		"personal has none still: no token given.",
		"The primary is work · Work.\n",
		"\n2. Priming\n",
		"Priming stays off.\n",
		"\n3. The service\nInstalled " + s.plist + ": ",
		"The router is up: healthy, pid " + strconv.Itoa(os.Getpid()) + ".\n",
		"\n4. The claude link\n",
		"Linked " + link + " to " + s.binary + ": every claude goes through switchboard.\n",
		"\n5. The skill\nInstalled the skill, which tells Claude what switchboard does under claude: " + skill + "\n",
		"\n6. Usage\n",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("switchboard setup printed\n%s\nwant it to hold\n%s", got.stdout, want)
		}
	}
	if _, usage, _ := strings.Cut(got.stdout, "\n6. Usage\n"); !strings.Contains(usage, "work · Work") || !strings.Contains(usage, "side · Side") {
		t.Errorf("setup ended\n%s\nwant it to end showing every account's usage", usage)
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
