package cli_test

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
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

func TestAccountsCleansLabels(t *testing.T) {
	path := writeConfig(t, "[[account]]\nid    = \"work\"\nlabel = \"Work\\u001b[2J\\tday\"\n")
	deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": path}, t.TempDir())
	writeToken(t, deps, "work", "test-token-work")

	got := run(t, deps, "accounts")
	want := result{stdout: "ID    LABEL         PRIMARY  TOKEN\n" + "work  Work [2J day  yes      usable\n"}
	if got != want {
		t.Errorf("switchboard accounts =\n%+v\nwant\n%+v", got, want)
	}
}

// personalAndSide are the accounts configured before work is added.
const personalAndSide = `
[[account]]
id    = "personal"
label = "Personal"

[[account]]
id    = "side"
label = "Side"
`

// tokenShaped is shaped like a Claude token, though it's none, as a user
// might give one where an id or a label goes.
const tokenShaped = "sk-ant-oat01-fake_token-shaped"

// probedWork is what the Claude API is asked, probing the token it takes.
var probedWork = []string{"test-token-work claude-fable-5-1", "test-token-work claude-haiku-4-5-20251001"}

func TestAccountsAdd(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantOut   string
		wantTable string
	}{
		{
			name:      "with a label",
			args:      []string{"accounts", "add", "work", "--label", "Work"},
			wantOut:   "added work\n",
			wantTable: "[[account]]\nid    = \"work\"\nlabel = \"Work\"\n",
		},
		{
			name:      "as the primary",
			args:      []string{"accounts", "add", "work", "--primary"},
			wantOut:   "added work, the primary\n",
			wantTable: "[[account]]\nid      = \"work\"\nprimary = true\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newClaudeAPI(t)
			deps, path := accountsDeps(t, api.URL, personalAndSide)
			before := readFile(t, path)

			got := runWithInput(t, deps, "test-token-work\n", tt.args...)
			if want := (result{stdout: tt.wantOut}); got != want {
				t.Errorf("switchboard %s = %+v, want %+v", strings.Join(tt.args, " "), got, want)
			}
			if want := before + "\n" + tt.wantTable; readFile(t, path) != want {
				t.Errorf("the config reads\n%s\nwant\n%s", readFile(t, path), want)
			}
			checkTokenFile(t, deps, "work", "test-token-work\n")
			if !slices.Equal(api.questions(), probedWork) {
				t.Errorf("the API was asked %q, want %q", api.questions(), probedWork)
			}
		})
	}
}

func TestAccountsAddWhenTheAPIRefusesTheToken(t *testing.T) {
	deps, path := accountsDeps(t, newClaudeAPI(t).URL, personalAndSide)
	before := readFile(t, path)

	got := runWithInput(t, deps, "test-token-side\n", "accounts", "add", "work")
	want := result{
		stderr: "Error: the API refused the token (HTTP 401 · Invalid bearer token), so nothing is saved: " +
			"make another with claude setup-token, run while signed in to that subscription\n",
		code: 1,
	}
	if got != want {
		t.Errorf("switchboard accounts add = %+v, want %+v", got, want)
	}
	if readFile(t, path) != before {
		t.Errorf("the config reads\n%s\nwant it as it was\n%s", readFile(t, path), before)
	}
	checkNoTokenFile(t, deps, "work")
}

func TestAccountsAddWhenTheAPIDoesntAnswer(t *testing.T) {
	upstream := closedServerURL(t)
	deps, path := accountsDeps(t, upstream, personalAndSide)
	before := readFile(t, path)

	got := runWithInput(t, deps, "test-token-work\n", "accounts", "add", "work")
	warning := "switchboard: couldn't check the token with the API (Post \"" + upstream + "/v1/messages\": "
	if got.code != 0 || got.stdout != "added work\n" || !strings.HasPrefix(got.stderr, warning) ||
		!strings.HasSuffix(got.stderr, ") — saved it all the same\n") || strings.Count(got.stderr, "\n") != 1 {
		t.Errorf("switchboard accounts add = %+v, want work added, and a warning starting %q", got, warning)
	}
	if want := before + "\n[[account]]\nid = \"work\"\n"; readFile(t, path) != want {
		t.Errorf("the config reads\n%s\nwant\n%s", readFile(t, path), want)
	}
	checkTokenFile(t, deps, "work", "test-token-work\n")
	checkNoToken(t, got, "test-token-work")
}

func TestAccountsAddRefusesATokenAsTheIDOrTheLabel(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "as the id",
			args:    []string{"accounts", "add", tokenShaped},
			wantErr: `account "[redacted]": id looks like a token, which an id mustn't, as it shows wherever the account does`,
		},
		{
			name:    "as the label",
			args:    []string{"accounts", "add", "work", "--label", tokenShaped},
			wantErr: `account "work": label looks like a token, which a label mustn't, as it shows wherever the account does`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newClaudeAPI(t)
			deps, path := accountsDeps(t, api.URL, personalAndSide)
			before := readFile(t, path)

			got := runWithInput(t, deps, "test-token-work\n", tt.args...)
			if want := (result{stderr: "Error: " + tt.wantErr + "\n", code: 1}); got != want {
				t.Errorf("switchboard accounts add = %+v, want %+v", got, want)
			}
			if readFile(t, path) != before || len(api.questions()) > 0 {
				t.Errorf("the config reads\n%s\nand the API was asked %d questions, want the config as it was, and the API asked nothing", readFile(t, path), len(api.questions()))
			}
			checkNoTokenFile(t, deps, "work")
			checkNoTokenFile(t, deps, tokenShaped)
		})
	}
}

func TestAccountsAddAnAccountConfiguredAlready(t *testing.T) {
	api := newClaudeAPI(t)
	deps, path := accountsDeps(t, api.URL, personalAndSide)
	before := readFile(t, path)

	got := runWithInput(t, deps, "test-token-work\n", "accounts", "add", "side")
	want := result{stderr: "Error: account \"side\" is already configured: replace its token with switchboard accounts token side\n", code: 1}
	if got != want {
		t.Errorf("switchboard accounts add = %+v, want %+v", got, want)
	}
	if readFile(t, path) != before || len(api.questions()) > 0 {
		t.Errorf("the config reads\n%s\nand the API was asked %q, want the config as it was, and the API asked nothing", readFile(t, path), api.questions())
	}
	checkNoTokenFile(t, deps, "side")
}

func TestAccountsAddMakesTheFirstConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "switchboard", "config.toml")
	deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": path}, t.TempDir())
	writeToken(t, deps, "work", "test-token-work")

	got := run(t, deps, "accounts", "add", "work")
	if want := (result{stdout: "added work, keeping the token in " + tokenPath(t, deps, "work") + "\n"}); got != want {
		t.Errorf("switchboard accounts add = %+v, want %+v", got, want)
	}
	if want := "[[account]]\nid = \"work\"\n"; readFile(t, path) != want {
		t.Errorf("the config reads\n%s\nwant\n%s", readFile(t, path), want)
	}
	if got := run(t, deps, "accounts"); !strings.Contains(got.stdout, "work  work   yes      usable") {
		t.Errorf("switchboard accounts = %+v, want work, the primary, with a usable token", got)
	}
}

func TestAccountsToken(t *testing.T) {
	api := newClaudeAPI(t)
	deps, path := accountsDeps(t, api.URL, personalAndSide+"\n[[account]]\nid = \"work\"\n")
	before := readFile(t, path)
	writeToken(t, deps, "work", "test-token-stale")

	got := runWithInput(t, deps, "test-token-work\n", "accounts", "token", "work")
	if want := (result{stdout: "saved work's token\n"}); got != want {
		t.Errorf("switchboard accounts token = %+v, want %+v", got, want)
	}
	checkTokenFile(t, deps, "work", "test-token-work\n")
	if readFile(t, path) != before || !slices.Equal(api.questions(), probedWork) {
		t.Errorf("the config reads\n%s\nand the API was asked %q, want the config as it was, and the API asked %q", readFile(t, path), api.questions(), probedWork)
	}
}

func TestAccountsTokenRefuses(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		input   string
		wantErr string
	}{
		{
			name:    "a token the API refuses",
			config:  personalAndSide,
			input:   "test-token-side\n",
			wantErr: "the API refused the token (HTTP 401 · Invalid bearer token), so nothing is saved: make another with claude setup-token, run while signed in to that subscription",
		},
		{
			name:    "no token",
			config:  personalAndSide,
			wantErr: "no token given",
		},
		{
			name:    "an account not configured",
			config:  "\n[[account]]\nid = \"personal\"\n",
			input:   "test-token-work\n",
			wantErr: `account "side" is not configured: add it with switchboard accounts add side`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _ := accountsDeps(t, newClaudeAPI(t).URL, tt.config)
			writeToken(t, deps, "side", "test-token-stale")

			got := runWithInput(t, deps, tt.input, "accounts", "token", "side")
			if want := (result{stderr: "Error: " + tt.wantErr + "\n", code: 1}); got != want {
				t.Errorf("switchboard accounts token = %+v, want %+v", got, want)
			}
			checkTokenFile(t, deps, "side", "test-token-stale\n")
		})
	}
}

func TestAccountsTokenWithoutConfig(t *testing.T) {
	home := t.TempDir()

	got := runWithInput(t, testDeps(nil, home), "test-token-work\n", "accounts", "token", "work")
	if want := "Error: no config file at " + filepath.Join(home, ".config", "switchboard", "config.toml") + "\n"; got.code != 1 || !strings.HasPrefix(got.stderr, want) {
		t.Errorf("switchboard accounts token = %+v, want an error starting %q", got, want)
	}
}

func TestAccountsRemove(t *testing.T) {
	tests := []struct {
		name string
		// token is set when side has a token file.
		token   bool
		wantOut string
	}{
		{name: "with its token file", token: true, wantOut: "removed side, and its token file\n"},
		{name: "without a token file", wantOut: "removed side\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, path := accountsDeps(t, newClaudeAPI(t).URL, personalAndSide)
			writeToken(t, deps, "personal", "test-token-work")
			if tt.token {
				writeToken(t, deps, "side", "test-token-side")
			}

			got := run(t, deps, "accounts", "remove", "side")
			if want := (result{stdout: tt.wantOut}); got != want {
				t.Errorf("switchboard accounts remove = %+v, want %+v", got, want)
			}
			if want := "\n\n[[account]]\nid    = \"personal\"\nlabel = \"Personal\"\n"; !strings.HasSuffix(readFile(t, path), want) {
				t.Errorf("the config reads\n%s\nwant it to end with personal's table", readFile(t, path))
			}
			checkNoTokenFile(t, deps, "side")
			checkTokenFile(t, deps, "personal", "test-token-work\n")
		})
	}
}

func TestAccountsRemoveRefuses(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{name: "an account not configured", config: "\n[[account]]\nid = \"personal\"\n\n[[account]]\nid = \"work\"\n", wantErr: `account "side" is not configured`},
		{name: "the only account", config: "\n[[account]]\nid = \"side\"\n", wantErr: `account "side" is the only one, and a config needs one at least`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, path := accountsDeps(t, newClaudeAPI(t).URL, tt.config)
			before := readFile(t, path)
			writeToken(t, deps, "side", "test-token-side")

			got := run(t, deps, "accounts", "remove", "side")
			if want := (result{stderr: "Error: " + tt.wantErr + "\n", code: 1}); got != want {
				t.Errorf("switchboard accounts remove = %+v, want %+v", got, want)
			}
			if readFile(t, path) != before {
				t.Errorf("the config reads\n%s\nwant it as it was\n%s", readFile(t, path), before)
			}
			checkTokenFile(t, deps, "side", "test-token-side\n")
		})
	}
}

// accountsDeps gives commands a config, whose path it returns, holding the
// accounts given and sending requests to upstream, and a state directory of
// their own.
func accountsDeps(t *testing.T, upstream, accounts string) (cli.Deps, string) {
	t.Helper()
	path := writeConfig(t, fmt.Sprintf("upstream = %q\n", upstream)+accounts)
	return testDeps(map[string]string{"SWITCHBOARD_CONFIG": path}, t.TempDir()), path
}

// closedServerURL returns the URL of a server that has shut down, so
// connecting to it fails.
func closedServerURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	return srv.URL
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// checkTokenFile checks the token file of the account with the given id holds
// want, and no one else can read or write it.
func checkTokenFile(t *testing.T, deps cli.Deps, id, want string) {
	t.Helper()
	path := tokenPath(t, deps, id)
	if got := readFile(t, path); got != want {
		t.Errorf("%s's token file holds %q, want %q", id, got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("%s's token file has mode %v, want it 0600", id, info.Mode())
	}
}

func checkNoTokenFile(t *testing.T, deps cli.Deps, id string) {
	t.Helper()
	if _, err := os.Lstat(tokenPath(t, deps, id)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s's token file: %v, want none", id, err)
	}
}

// checkNoToken checks a run showed none of secrets.
func checkNoToken(t *testing.T, got result, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(got.stdout+got.stderr, secret) {
			t.Errorf("the command showed a token:\n%s%s", got.stdout, got.stderr)
		}
	}
}
