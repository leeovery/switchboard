package cli_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

func TestAccounts(t *testing.T) {
	path := writeConfig(t, `
[[account]]
id    = "work"
label = "Work"

[[account]]
id      = "personal"
primary = true

[[account]]
id    = "side-project"
label = "Side project"

[[account]]
id    = "spare"
label = "Spare"
`)
	deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": path}, t.TempDir())
	writeToken(t, deps, "work", "test-token-work")
	writeToken(t, deps, "side-project", "test-token-side")
	if err := os.Chmod(tokenPath(t, deps, "side-project"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeToken(t, deps, "spare", " ")

	got := run(t, deps, "accounts")
	want := result{
		stdout: "ID            LABEL         PRIMARY  TOKEN\n" +
			"work          Work                   usable\n" +
			"personal      personal      yes      token missing: write it to " + tokenPath(t, deps, "personal") + "\n" +
			"side-project  Side project           other users can read the token file (mode 0644): chmod 600 " + tokenPath(t, deps, "side-project") + "\n" +
			"spare         Spare                  token missing: write it to " + tokenPath(t, deps, "spare") + ", which is empty\n",
	}
	if got != want {
		t.Errorf("switchboard accounts =\n%+v\nwant\n%+v", got, want)
	}
	for _, token := range []string{"test-token-work", "test-token-side"} {
		if strings.Contains(got.stdout+got.stderr, token) {
			t.Errorf("switchboard accounts printed a token:\n%s", got.stdout)
		}
	}
}

func TestAccountsMarksTheFirstPrimaryWhenNoneIsMarked(t *testing.T) {
	path := writeConfig(t, "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\n")
	deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": path}, t.TempDir())

	got := run(t, deps, "accounts")
	want := result{
		stdout: "ID    LABEL  PRIMARY  TOKEN\n" +
			"work  work   yes      token missing: write it to " + tokenPath(t, deps, "work") + "\n" +
			"side  side            token missing: write it to " + tokenPath(t, deps, "side") + "\n",
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
	path := writeConfig(t, invalidConfig)

	got := run(t, testDeps(map[string]string{"SWITCHBOARD_CONFIG": path}, t.TempDir()), "accounts")
	wantErr := "Error: invalid config " + path + ":\n" + `unknown key "account.token_env": tokens now live in files`
	if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, wantErr) {
		t.Errorf("switchboard accounts = %+v, want exit status 1 and an error starting %q", got, wantErr)
	}
}

func TestAccountsWithoutAStateDirectory(t *testing.T) {
	path := writeConfig(t, "[[account]]\nid = \"work\"\n")
	deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": path}, t.TempDir())
	deps.HomeDir = func() (string, error) { return "", errors.New("no home directory") }

	got := run(t, deps, "accounts")
	if want := (result{stderr: "Error: locate state directory: no home directory\n", code: 1}); got != want {
		t.Errorf("switchboard accounts = %+v, want %+v", got, want)
	}
}
