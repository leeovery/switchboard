package launch_test

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/launch"
)

var update = flag.Bool("update", false, "rewrite the golden files with what the tests print")

// zshPath is the zsh the tests check scripts with: the system's.
const zshPath = "/bin/zsh"

// integration is the integration of this switchboard binary and the accounts.
func integration() launch.Integration {
	return launch.Integration{Binary: "/usr/local/bin/switchboard", Prefix: "cx", Accounts: accounts}
}

func TestZsh(t *testing.T) {
	withConfig := integration()
	withConfig.Config = "/Users/tester/it's my/config.toml"
	tests := []struct {
		name        string
		integration launch.Integration
	}{
		{name: "zsh", integration: integration()},
		{name: "zsh-config", integration: withConfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			if err := tt.integration.Zsh(&b); err != nil {
				t.Fatalf("Zsh() error = %v", err)
			}
			got := b.String()

			path := filepath.Join("testdata", tt.name+".golden")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden file (run with -update to create it): %v", err)
			}
			if got != string(want) {
				t.Errorf("Zsh() wrote\n%s\nwant (%s; run with -update to accept it)\n%s", got, path, want)
			}
			parses(t, got)
		})
	}
}

func TestZshWithoutAccountsDefinesClaudeAlone(t *testing.T) {
	i := integration()
	i.Accounts = nil
	var b strings.Builder
	if err := i.Zsh(&b); err != nil {
		t.Fatalf("Zsh() error = %v", err)
	}

	want := `function claude { '/usr/local/bin/switchboard' run -- "$@"; }` + "\n"
	if got := b.String(); got != want {
		t.Errorf("Zsh() wrote\n%s\nwant\n%s", got, want)
	}
	parses(t, b.String())
}

func TestZshQuotesTheBinary(t *testing.T) {
	i := integration()
	i.Binary = `/Users/tester/it's here/switch board`
	var b strings.Builder
	if err := i.Zsh(&b); err != nil {
		t.Fatalf("Zsh() error = %v", err)
	}

	want := `function claude { '/Users/tester/it'\''s here/switch board' run -- "$@"; }` + "\n"
	if got := b.String(); !strings.HasPrefix(got, want) {
		t.Errorf("Zsh() wrote\n%s\nwant it to start\n%s", got, want)
	}
	parses(t, b.String())
}

func TestZshNamesTheLaunchersWithThePrefix(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		want   []string
	}{
		{name: "a prefix", prefix: "claude-", want: []string{"function claude-work {", "function claude-personal {", "function claude-side {"}},
		{name: "none", prefix: "", want: []string{"function work {", "function personal {", "function side {"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := integration()
			i.Prefix = tt.prefix
			var b strings.Builder
			if err := i.Zsh(&b); err != nil {
				t.Fatalf("Zsh() error = %v", err)
			}

			lines := strings.Split(b.String(), "\n")
			for n, want := range append([]string{"function claude {"}, tt.want...) {
				if !strings.HasPrefix(lines[n], want) {
					t.Errorf("line %d = %q, want it to start %q", n+1, lines[n], want)
				}
			}
		})
	}
}

func TestZshRefusesAPrefixThatIsntAName(t *testing.T) {
	for _, prefix := range []string{"-cx", "cx ", "cx;", "$(cx)", "cx'", "c·x"} {
		t.Run(prefix, func(t *testing.T) {
			i := integration()
			i.Prefix = prefix
			var b strings.Builder
			err := i.Zsh(&b)
			if err == nil || b.Len() > 0 {
				t.Errorf("Zsh() = %v, writing %q; want an error, writing nothing", err, b.String())
			}
		})
	}
}

func TestZshStartsClaudeThroughRun(t *testing.T) {
	if _, err := os.Stat(zshPath); err != nil {
		t.Skipf("no zsh at %s", zshPath)
	}
	// A switchboard, at a path zsh must quote, that prints its arguments, a
	// line each, and the token Claude Code would start on, which run alone
	// gives it; and a config at such a path too.
	dir := filepath.Join(t.TempDir(), "it's a")
	binary := filepath.Join(dir, "switch board")
	config := filepath.Join(dir, `"$(my)" config.toml`)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" \"token=${CLAUDE_CODE_OAUTH_TOKEN-unset}\"\n"
	if err := os.WriteFile(binary, []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		config string
		call   string
		want   string
	}{
		{
			name: "claude",
			call: `claude --print 'a prompt'`,
			want: "run\n--\n--print\na prompt\ntoken=unset\n",
		},
		{
			name: "an account's launcher",
			call: `cxside --resume ''`,
			want: "run\n--account\nside\n--\n--resume\n\ntoken=unset\n",
		},
		{
			name:   "claude, reading the config given",
			config: config,
			call:   `claude --resume`,
			want:   "--config\n" + config + "\nrun\n--\n--resume\ntoken=unset\n",
		},
		{
			name:   "an account's launcher, reading the config given",
			config: config,
			call:   `cxwork`,
			want:   "--config\n" + config + "\nrun\n--account\nwork\n--\ntoken=unset\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := integration()
			i.Binary, i.Config = binary, tt.config
			var script strings.Builder
			if err := i.Zsh(&script); err != nil {
				t.Fatalf("Zsh() error = %v", err)
			}
			cmd := exec.CommandContext(t.Context(), zshPath, "-f", "-c", script.String()+tt.call)
			cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}

			out, err := cmd.CombinedOutput()
			if err != nil || string(out) != tt.want {
				t.Errorf("%s printed\n%s(%v)\nwant\n%s", tt.call, out, err, tt.want)
			}
		})
	}
}

// parses checks zsh can parse script, when there's a zsh to ask: its syntax
// alone, running none of it, and reading no startup file of the user's.
func parses(t *testing.T, script string) {
	t.Helper()
	if _, err := os.Stat(zshPath); err != nil {
		t.Logf("no zsh at %s to parse the script with", zshPath)
		return
	}
	cmd := exec.CommandContext(t.Context(), zshPath, "-f", "-n")
	cmd.Stdin = strings.NewReader(script)
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("zsh -n: %v\n%s", err, out)
	}
}
