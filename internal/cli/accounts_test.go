package cli_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

func TestAccounts(t *testing.T) {
	path := writeConfig(t, `
[[account]]
id        = "work"
label     = "Work"
token_env = "CLAUDE_TOKEN_WORK"

[[account]]
id        = "personal"
token_env = "CLAUDE_TOKEN_PERSONAL"

[[account]]
id        = "side-project"
label     = "Side project"
token_env = "CLAUDE_TOKEN_SIDE"
`)
	deps := testDeps(map[string]string{
		"SWITCHBOARD_CONFIG": path,
		"CLAUDE_TOKEN_WORK":  "token-work",
		"CLAUDE_TOKEN_SIDE":  "  ",
	}, t.TempDir())

	got := run(t, deps, "accounts")
	want := result{
		stdout: `ID            LABEL         TOKEN_ENV              TOKEN
work          Work          CLAUDE_TOKEN_WORK      set
personal      personal      CLAUDE_TOKEN_PERSONAL  missing
side-project  Side project  CLAUDE_TOKEN_SIDE      missing
`,
		code: 0,
	}
	if got != want {
		t.Errorf("switchboard accounts =\n%+v\nwant\n%+v", got, want)
	}
}

func TestAccountsWithoutConfig(t *testing.T) {
	home := t.TempDir()

	got := run(t, testDeps(nil, home), "accounts")
	want := result{
		stderr: "Error: no config file at " + filepath.Join(home, ".config", "switchboard", "config.toml") +
			"\n\nCreate one like this:\n\n" + config.Example,
		code: 1,
	}
	if got != want {
		t.Errorf("switchboard accounts =\n%+v\nwant\n%+v", got, want)
	}
}

func TestAccountsWithInvalidConfig(t *testing.T) {
	path := writeConfig(t, "[[account]]\nid = \"work\"\n")

	got := run(t, testDeps(map[string]string{"SWITCHBOARD_CONFIG": path}, t.TempDir()), "accounts")
	wantErr := "Error: invalid config " + path + ":\n" + `account "work": token_env is required`
	if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, wantErr) {
		t.Errorf("switchboard accounts = %+v, want exit status 1 and an error starting %q", got, wantErr)
	}
}
