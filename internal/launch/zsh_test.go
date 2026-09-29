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

func TestZsh(t *testing.T) {
	var b strings.Builder
	if err := launch.Zsh(&b, "/usr/local/bin/switchboard", "cx", accounts); err != nil {
		t.Fatalf("Zsh() error = %v", err)
	}
	got := b.String()

	path := filepath.Join("testdata", "zsh.golden")
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
}

func TestZshQuotesTheBinary(t *testing.T) {
	var b strings.Builder
	if err := launch.Zsh(&b, `/Users/tester/it's here/switch board`, "cx", accounts); err != nil {
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
			var b strings.Builder
			if err := launch.Zsh(&b, "/usr/local/bin/switchboard", tt.prefix, accounts); err != nil {
				t.Fatalf("Zsh() error = %v", err)
			}

			lines := strings.Split(b.String(), "\n")
			for i, want := range append([]string{"function claude {"}, tt.want...) {
				if !strings.HasPrefix(lines[i], want) {
					t.Errorf("line %d = %q, want it to start %q", i+1, lines[i], want)
				}
			}
		})
	}
}

func TestZshRefusesAPrefixThatIsntAName(t *testing.T) {
	for _, prefix := range []string{"-cx", "cx ", "cx;", "$(cx)", "cx'", "c·x"} {
		t.Run(prefix, func(t *testing.T) {
			var b strings.Builder
			err := launch.Zsh(&b, "/usr/local/bin/switchboard", prefix, accounts)
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
	// A switchboard, at a path zsh must quote, that prints its arguments and
	// the token it's given, a line each.
	binary := filepath.Join(t.TempDir(), "it's a", "switch board")
	if err := os.MkdirAll(filepath.Dir(binary), 0o700); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" \"token=${CLAUDE_CODE_OAUTH_TOKEN-unset}\"\n"
	if err := os.WriteFile(binary, []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	var integration strings.Builder
	if err := launch.Zsh(&integration, binary, "cx", accounts); err != nil {
		t.Fatalf("Zsh() error = %v", err)
	}
	tests := []struct {
		name string
		// env is the shell's environment, beyond its home and PATH.
		env  []string
		call string
		want string
	}{
		{
			name: "claude",
			env:  []string{"CLAUDE_TOKEN_WORK=" + workToken},
			call: `claude --print 'a prompt'`,
			want: "run\n--\n--print\na prompt\ntoken=" + workToken + "\n",
		},
		{
			name: "an account's launcher",
			env:  []string{"CLAUDE_TOKEN_WORK=" + workToken},
			call: `cxside --resume ''`,
			want: "run\n--account\nside\n--\n--resume\n\ntoken=" + workToken + "\n",
		},
		{
			name: "claude, without the first account's token",
			call: "claude",
			want: "run\n--\ntoken=unset\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), zshPath, "-f", "-c", integration.String()+tt.call)
			cmd.Env = append([]string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}, tt.env...)

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
