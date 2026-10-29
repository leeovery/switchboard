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

func TestAccountsAddRefusesABlankLabel(t *testing.T) {
	const blank = `account "work": label is blank, showing nothing: give one that shows, or leave --label out to show the account by its id`
	tests := []struct {
		name    string
		label   string
		wantErr string
	}{
		{name: "spaces alone", label: "   ", wantErr: blank},
		{name: "a zero-width joiner alone", label: "\u2060", wantErr: blank},
		{name: "text that isn't UTF-8", label: "a\xffb", wantErr: `account "work": label isn't UTF-8 text, as a config's text must be`},
		{name: "a sign that hides the character after it", label: "Work\u06dd1", wantErr: `account "work": label holds a sign that joins the character after it, such as an Arabic number sign, which can hide that character`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newClaudeAPI(t)
			deps, path := accountsDeps(t, api.URL, personalAndSide)
			before := readFile(t, path)

			got := runWithInput(t, deps, "test-token-work\n", "accounts", "add", "work", "--label", tt.label)
			if want := (result{stderr: "Error: " + tt.wantErr + "\n", code: 1}); got != want {
				t.Errorf("switchboard accounts add = %+v, want %+v", got, want)
			}
			if readFile(t, path) != before || len(api.questions()) > 0 {
				t.Errorf("the config reads\n%s\nand the API was asked %q, want the config as it was, and the API asked nothing", readFile(t, path), api.questions())
			}
			checkNoTokenFile(t, deps, "work")
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

func TestAccountsAddAnAccountConfiguredAlreadyInAnotherCase(t *testing.T) {
	api := newClaudeAPI(t)
	deps, path := accountsDeps(t, api.URL, "\n[[account]]\nid = \"work\"\n"+personalAndSide)
	// Work's token file holds no token, so a token given Work, whose file
	// is work's where case is ignored, would be written in its place.
	writeToken(t, deps, "work", "")
	before, token := readFile(t, path), readFile(t, tokenPath(t, deps, "work"))

	got := runWithInput(t, deps, "test-token-work\n", "accounts", "add", "Work")
	want := result{stderr: "Error: account ids \"work\" and \"Work\" differ only in case, so they'd share a token file, as macOS ignores case in file names\n", code: 1}
	if got != want {
		t.Errorf("switchboard accounts add Work = %+v, want %+v", got, want)
	}
	if readFile(t, path) != before || len(api.questions()) > 0 {
		t.Errorf("the config reads\n%s\nand the API was asked %q, want the config as it was, and the API asked nothing", readFile(t, path), api.questions())
	}
	checkTokenFile(t, deps, "work", token)
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

func TestAccountsTokenNamesAnIDConfiguredInAnotherCase(t *testing.T) {
	deps, _ := accountsDeps(t, newClaudeAPI(t).URL, personalAndSide)

	got := runWithInput(t, deps, "test-token-side\n", "accounts", "token", "Side")
	if want := (result{stderr: "Error: account \"Side\" is not configured: side is; give an id in its own case\n", code: 1}); got != want {
		t.Errorf("switchboard accounts token Side = %+v, want %+v", got, want)
	}
	checkNoTokenFile(t, deps, "side")
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

func TestAccountsRemoveThePrimary(t *testing.T) {
	tests := []struct {
		name string
		// sideToken is set when side, the primary once personal goes, has a
		// token file.
		sideToken bool
		wantOut   string
	}{
		{
			name:      "leaving another with a usable token the primary",
			sideToken: true,
			wantOut: "removed personal, and its token file\n" +
				"side is the primary now; the sessions running on personal's token stay routed, as side's, for a week from when the router takes this up\n",
		},
		{
			name: "leaving another without one the primary",
			wantOut: "removed personal, and its token file\n" +
				"side is the primary now, but has no usable token: the sessions running on personal's token aren't routed until it has one " +
				"(switchboard accounts token side)\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _ := accountsDeps(t, newClaudeAPI(t).URL, personalAndSide)
			writeToken(t, deps, "personal", "test-token-work")
			if tt.sideToken {
				writeToken(t, deps, "side", "test-token-side")
			}

			got := run(t, deps, "accounts", "remove", "personal")
			if want := (result{stdout: tt.wantOut}); got != want {
				t.Errorf("switchboard accounts remove = %+v, want %+v", got, want)
			}
			checkNoTokenFile(t, deps, "personal")
		})
	}
}

func TestAccountsRemoveATokenFileThatsALink(t *testing.T) {
	deps, _ := accountsDeps(t, newClaudeAPI(t).URL, personalAndSide)
	writeToken(t, deps, "personal", "test-token-work")
	// side's token file, a link to a file kept elsewhere, as by a dotfiles
	// step.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "side")
	if err := os.WriteFile(target, []byte("test-token-side\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, tokenPath(t, deps, "side")); err != nil {
		t.Fatal(err)
	}

	got := run(t, deps, "accounts", "remove", "side")
	if want := (result{stdout: "removed side, and its token file, a link: the file it led to, " + target + ", is left as it is\n"}); got != want {
		t.Errorf("switchboard accounts remove = %+v, want %+v", got, want)
	}
	checkNoTokenFile(t, deps, "side")
	if data, err := os.ReadFile(target); err != nil || string(data) != "test-token-side\n" {
		t.Errorf("where the link led holds %q (%v), want the token still", data, err)
	}
}

func TestAccountsRemoveATokenFileThatsALinkToASecondNameOfIt(t *testing.T) {
	deps, _ := accountsDeps(t, newClaudeAPI(t).URL, personalAndSide)
	writeToken(t, deps, "personal", "test-token-work")
	// side's token file, a link to its record's path, a second name of a
	// file kept elsewhere.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	elsewhere := filepath.Join(dir, "side")
	if err := os.WriteFile(elsewhere, []byte("test-token-side\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	record, err := filepath.EvalSymlinks(filepath.Dir(tokenPath(t, deps, "side")))
	if err != nil {
		t.Fatal(err)
	}
	record = filepath.Join(record, "side.taken")
	if err := os.Link(elsewhere, record); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(record, tokenPath(t, deps, "side")); err != nil {
		t.Fatal(err)
	}

	got := run(t, deps, "accounts", "remove", "side")
	if want := (result{stdout: "removed side, and its token file, a link, with the name it led to, " + record + ": the file's kept under its other name\n"}); got != want {
		t.Errorf("switchboard accounts remove = %+v, want %+v", got, want)
	}
	checkNoTokenFile(t, deps, "side")
	if _, err := os.Lstat(record); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the name the link led to: %v, want it gone", err)
	}
	if data, err := os.ReadFile(elsewhere); err != nil || string(data) != "test-token-side\n" {
		t.Errorf("the file elsewhere holds %q (%v), want the token still", data, err)
	}
}

func TestAccountsRemoveThatCantDeleteTheTokenFileSaysTheAccountIsTakenOut(t *testing.T) {
	deps, path := accountsDeps(t, newClaudeAPI(t).URL, personalAndSide)
	// Where side's token file goes, a directory with something in it, which
	// the token file's removal can't delete.
	where := tokenPath(t, deps, "side")
	if err := os.MkdirAll(filepath.Join(where, "kept"), 0o700); err != nil {
		t.Fatal(err)
	}

	got := run(t, deps, "accounts", "remove", "side")
	if want := (result{stderr: "Error: side is taken out of the config, but its token file, " + where + ", isn't deleted: directory not empty\n", code: 1}); got != want {
		t.Errorf("switchboard accounts remove = %+v, want %+v", got, want)
	}
	if strings.Contains(readFile(t, path), `"side"`) {
		t.Errorf("the config reads\n%s\nwant side taken out", readFile(t, path))
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

// sideOnAPlan is personalAndSide with side on a plan, with a reserve, a
// comment beside each.
const sideOnAPlan = personalAndSide + "plan    = \"pro\"   # for now\nreserve = 0.1     # the app's share\n"

// primaryFromPersonal is what accounts edit says of the sessions already
// running once it's made another account the primary in place of personal.
var primaryFromPersonal = keepingTheFormer("personal")

// keepingTheFormer is what accounts edit says of the sessions already
// running, once it's made another account the primary in place of the one
// with the given id, which has a usable token.
func keepingTheFormer(former string) string {
	return "sessions running through the router keep " + former + "'s token as Claude Code's own until they restart, " +
		"so what they send outside the conversation, such as artifacts and uploads, still goes out on " + former + "\n"
}

// onTheFirst is what accounts edit says of the sessions running through the
// router, once it's made another account the primary in place of former,
// which has no usable token, where first is the first account with one, and
// pinned, whether another account has one, which a session can be pinned to.
func onTheFirst(former, first string, pinned bool) string {
	var orPinned string
	if pinned {
		orPinned = ", or, pinned, on their pinned account's token"
	}
	return former + " has no usable token, so sessions running through the router started on " + first +
		"'s, the first account with one" + orPinned + ", as Claude Code's own, " +
		"and keep it until they restart, so what they send outside the conversation, such as artifacts and uploads, still goes out there\n"
}

// onTheirPins is what accounts edit says of the sessions running through the
// router, once it's made the first account with a usable token the primary
// in place of former, which has none, while another account has one: only
// the pinned ones started on another account's.
func onTheirPins(former string) string {
	return former + " has no usable token, so sessions running through the router pinned to another account " +
		"started on their pinned account's token as Claude Code's own, and keep it until they restart, " +
		"so what they send outside the conversation, such as artifacts and uploads, still goes out there\n"
}

func TestAccountsEdit(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		args    []string
		wantOut string
		// old is what the config held that the edit replaces with new.
		old, new string
	}{
		{
			name:    "a label",
			config:  personalAndSide,
			args:    []string{"--label", "Day job"},
			wantOut: "edited side: label \"Day job\" (was \"Side\")\n",
			old:     `label = "Side"`,
			new:     `label = "Day job"`,
		},
		{
			name:    "a label that reads as more changes, quoted",
			config:  personalAndSide,
			args:    []string{"--label", "a, plan b (was c)"},
			wantOut: "edited side: label \"a, plan b (was c)\" (was \"Side\")\n",
			old:     `label = "Side"`,
			new:     `label = "a, plan b (was c)"`,
		},
		{
			name:    "a label for an account that had none",
			config:  "\n[[account]]\nid = \"personal\"\n\n[[account]]\nid = \"side\"\n",
			args:    []string{"--label", "Day job"},
			wantOut: "edited side: label \"Day job\" (was none)\n",
			old:     "id = \"side\"\n",
			new:     "id = \"side\"\nlabel = \"Day job\"\n",
		},
		{
			name:    "a tidy label in place of the same written untidily, saved and said so",
			config:  "\n[[account]]\nid = \"personal\"\n\n[[account]]\nid    = \"side\"\nlabel = \"Day  job \"\n",
			args:    []string{"--label", "Day job"},
			wantOut: "edited side: label \"Day job\" (was written untidily)\n",
			old:     `label = "Day  job "`,
			new:     `label = "Day job"`,
		},
		{
			name:    "its id as its label, which takes the label out, as it shows the same",
			config:  personalAndSide,
			args:    []string{"--label", "side"},
			wantOut: "edited side: label none, shown by its id (was \"Side\")\n",
			old:     "label = \"Side\"\n",
			new:     "",
		},
		{
			name:    "its id with an invisible separator, which is dropped, so the label's taken out",
			config:  personalAndSide,
			args:    []string{"--label", "side\u2063"},
			wantOut: "edited side: label none, shown by its id (was \"Side\")\n",
			old:     "label = \"Side\"\n",
			new:     "",
		},
		{
			name:    "a label in place of one written by hand with a zero-width space, which is said escaped",
			config:  "\n[[account]]\nid = \"personal\"\n\n[[account]]\nid    = \"side\"\nlabel = \"Day job\\u200B\"\n",
			args:    []string{"--label", "Day job"},
			wantOut: "edited side: label \"Day job\" (was \"Day job\\u200B\")\n",
			old:     `label = "Day job\u200B"`,
			new:     `label = "Day job"`,
		},
		{
			name:    "a label in place of one written by hand with an override, which is said escaped",
			config:  "\n[[account]]\nid = \"personal\"\n\n[[account]]\nid    = \"side\"\nlabel = \"\\u202Ekrow\"\n",
			args:    []string{"--label", "work"},
			wantOut: "edited side: label \"work\" (was \"\\u202Ekrow\")\n",
			old:     `label = "\u202Ekrow"`,
			new:     `label = "work"`,
		},
		{
			name:    "a label in place of its id written as one by hand, which is said as written",
			config:  "\n[[account]]\nid = \"personal\"\n\n[[account]]\nid    = \"side\"\nlabel = \"side\"\n",
			args:    []string{"--label", "Side"},
			wantOut: "edited side: label \"Side\" (was \"side\")\n",
			old:     `label = "side"`,
			new:     `label = "Side"`,
		},
		{
			name:    "a label with a fake flag, its tags hiding text dropped",
			config:  personalAndSide,
			args:    []string{"--label", "Side \U0001f3f4\U000e0068\U000e0069\U000e0064\U000e0065\U000e007f"},
			wantOut: "edited side: label \"Side \U0001f3f4\" (was \"Side\")\n",
			old:     `label = "Side"`,
			new:     "label = \"Side \U0001f3f4\"",
		},
		{
			name:    "a label with Wales's flag, kept whole",
			config:  personalAndSide,
			args:    []string{"--label", "Side \U0001f3f4\U000e0067\U000e0062\U000e0077\U000e006c\U000e0073\U000e007f"},
			wantOut: "edited side: label \"Side \U0001f3f4\U000e0067\U000e0062\U000e0077\U000e006c\U000e0073\U000e007f\" (was \"Side\")\n",
			old:     `label = "Side"`,
			new:     "label = \"Side \U0001f3f4\U000e0067\U000e0062\U000e0077\U000e006c\U000e0073\U000e007f\"",
		},
		{
			name:    "a label with an emoji sequence's joiner and a private-use glyph, said as it shows",
			config:  personalAndSide,
			args:    []string{"--label", "\ue0a0 Team \U0001f469\u200d\U0001f4bb"},
			wantOut: "edited side: label \"\ue0a0 Team \U0001f469\u200d\U0001f4bb\" (was \"Side\")\n",
			old:     `label = "Side"`,
			new:     "label = \"\ue0a0 Team \U0001f469\u200d\U0001f4bb\"",
		},
		{
			name:    "a label without the characters that play no part in how it shows",
			config:  personalAndSide,
			args:    []string{"--label", "Work \u200b Team"},
			wantOut: "edited side: label \"Work Team\" (was \"Side\")\n",
			old:     `label = "Side"`,
			new:     `label = "Work Team"`,
		},
		{
			name:    "a label as it shows, its inner whitespace one space",
			config:  personalAndSide,
			args:    []string{"--label", "  Day   job "},
			wantOut: "edited side: label \"Day job\" (was \"Side\")\n",
			old:     `label = "Side"`,
			new:     `label = "Day job"`,
		},
		{
			name:    "the label it has but for the whitespace within it, which changes nothing",
			config:  "\n[[account]]\nid = \"personal\"\n\n[[account]]\nid    = \"side\"\nlabel = \"Day job\"\n",
			args:    []string{"--label", "Day  job"},
			wantOut: "side has every value given already, so nothing changed\n",
		},
		{
			name:    "no label, showing the account by its id",
			config:  personalAndSide,
			args:    []string{"--label", ""},
			wantOut: "edited side: label none, shown by its id (was \"Side\")\n",
			old:     "label = \"Side\"\n",
			new:     "",
		},
		{
			name:    "a plan",
			config:  personalAndSide,
			args:    []string{"--plan", "max5x"},
			wantOut: "edited side: plan max5x (was none)\n",
			old:     "label = \"Side\"\n",
			new:     "label = \"Side\"\nplan = \"max5x\"\n",
		},
		{
			name:    "the primary, in place of the first, the primary for want of a mark",
			config:  personalAndSide,
			args:    []string{"--primary"},
			wantOut: "edited side: the primary in place of personal\n" + primaryFromPersonal,
			old:     "label = \"Side\"\n",
			new:     "label = \"Side\"\nprimary = true\n",
		},
		{
			name:    "a reserve, as a percentage",
			config:  personalAndSide,
			args:    []string{"--reserve", "5%"},
			wantOut: "edited side: reserve 5% (was none)\n",
			old:     "label = \"Side\"\n",
			new:     "label = \"Side\"\nreserve = 0.05\n",
		},
		{
			name:    "everything together",
			config:  personalAndSide,
			args:    []string{"--label", "Day job", "--plan", "max20x", "--primary", "--reserve", "2.5%"},
			wantOut: "edited side: label \"Day job\" (was \"Side\"), plan max20x (was none), the primary in place of personal, reserve 2.5% (was none)\n" + primaryFromPersonal,
			old:     "label = \"Side\"\n",
			new:     "label = \"Day job\"\nplan = \"max20x\"\nprimary = true\nreserve = 0.025\n",
		},
		{
			name:    "no plan and no reserve, taking out their lines and the comments beside them",
			config:  sideOnAPlan,
			args:    []string{"--plan", "none", "--reserve", "0"},
			wantOut: "edited side: plan none (was pro), reserve none (was 10%)\n",
			old:     "plan    = \"pro\"   # for now\nreserve = 0.1     # the app's share\n",
			new:     "",
		},
		{
			name:    "a plan and a reserve in place of those it has, keeping the comments beside them",
			config:  sideOnAPlan,
			args:    []string{"--plan", "max5x", "--reserve", "7.5%"},
			wantOut: "edited side: plan max5x (was pro), reserve 7.5% (was 10%)\n",
			old:     "plan    = \"pro\"   # for now\nreserve = 0.1     # the app's share\n",
			new:     "plan    = \"max5x\"   # for now\nreserve = 0.075     # the app's share\n",
		},
		{
			name:    "no reserve, written as 0%",
			config:  sideOnAPlan,
			args:    []string{"--reserve", "0%"},
			wantOut: "edited side: reserve none (was 10%)\n",
			old:     "reserve = 0.1     # the app's share\n",
			new:     "",
		},
		{
			name:    "a reserve in place of one written by hand past two decimal places, which shows as it was",
			config:  personalAndSide + "reserve = 0.99999\n",
			args:    []string{"--reserve", "5%"},
			wantOut: "edited side: reserve 5% (was 99.999%)\n",
			old:     "reserve = 0.99999\n",
			new:     "reserve = 0.05\n",
		},
		{
			name:    "no reserve, in place of a sliver written by hand",
			config:  personalAndSide + "reserve = 0.00001\n",
			args:    []string{"--reserve", "0"},
			wantOut: "edited side: reserve none (was 0.001%)\n",
			old:     "reserve = 0.00001\n",
			new:     "",
		},
		{
			name:    "the label it has but for surrounding whitespace, which changes nothing",
			config:  personalAndSide,
			args:    []string{"--label", "Side "},
			wantOut: "side has every value given already, so nothing changed\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, path := accountsDeps(t, newClaudeAPI(t).URL, tt.config)
			writeToken(t, deps, "personal", "test-token-personal")
			writeToken(t, deps, "side", "test-token-side")
			before := readFile(t, path)
			args := append([]string{"accounts", "edit", "side"}, tt.args...)

			got := run(t, deps, args...)
			if want := (result{stdout: tt.wantOut}); got != want {
				t.Errorf("switchboard %s = %+v, want %+v", strings.Join(args, " "), got, want)
			}
			if want := strings.Replace(before, tt.old, tt.new, 1); !strings.Contains(before, tt.old) || readFile(t, path) != want {
				t.Errorf("the config reads\n%s\nwant\n%s", readFile(t, path), want)
			}
		})
	}
}

func TestAccountsEditWarnsOfAPrimaryWithoutAUsableToken(t *testing.T) {
	deps, path := accountsDeps(t, newClaudeAPI(t).URL, personalAndSide)
	writeToken(t, deps, "personal", "test-token-personal")

	got := run(t, deps, "accounts", "edit", "side", "--primary")
	if want := "edited side: the primary in place of personal\n" + primaryFromPersonal; got.stdout != want || got.code != 0 {
		t.Errorf("switchboard accounts edit side --primary = %+v, want stdout %q", got, want)
	}
	const (
		warned = "switchboard: side, the primary now, has no usable token ("
		then   = ") — new sessions take another account's as Claude Code's own until it has one: switchboard accounts token side\n"
	)
	if !strings.HasPrefix(got.stderr, warned) || !strings.HasSuffix(got.stderr, then) || strings.Count(got.stderr, "\n") != 1 {
		t.Errorf("switchboard accounts edit side --primary warned %q, want a line starting %q and ending %q", got.stderr, warned, then)
	}
	if !strings.HasSuffix(readFile(t, path), "label = \"Side\"\nprimary = true\n") {
		t.Errorf("the config reads\n%s\nwant side marked the primary", readFile(t, path))
	}
}

func TestAccountsEditSaysWhichTokenTheRunningSessionsKeep(t *testing.T) {
	const fourAccounts = "\n[[account]]\nid      = \"work\"\nprimary = true\n\n[[account]]\nid = \"side\"\n\n" +
		"[[account]]\nid = \"personal\"\n\n[[account]]\nid = \"spare\"\n"
	tests := []struct {
		name    string
		tokens  []string
		wantOut string
	}{
		{
			name:    "the former primary's, which it has",
			tokens:  []string{"work", "personal", "side"},
			wantOut: "edited side: the primary in place of work\n" + keepingTheFormer("work"),
		},
		{
			name:    "the first with a usable token, the former primary without one, or a pinned account's",
			tokens:  []string{"personal", "spare"},
			wantOut: "edited side: the primary in place of work\n" + onTheFirst("work", "personal", true),
		},
		{
			name:    "the first with a usable token alone, as none is pinned elsewhere",
			tokens:  []string{"personal"},
			wantOut: "edited side: the primary in place of work\n" + onTheFirst("work", "personal", false),
		},
		{
			name:    "a pinned account's, where the rest took the new primary's",
			tokens:  []string{"side", "personal"},
			wantOut: "edited side: the primary in place of work\n" + onTheirPins("work"),
		},
		{
			name:    "nothing, where the new primary alone has a usable token, so none is pinned elsewhere",
			tokens:  []string{"side"},
			wantOut: "edited side: the primary in place of work\n",
		},
		{
			name:    "nothing, where no account has a usable token",
			wantOut: "edited side: the primary in place of work\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _ := accountsDeps(t, newClaudeAPI(t).URL, fourAccounts)
			for _, id := range tt.tokens {
				writeToken(t, deps, id, "test-token-"+id)
			}

			got := run(t, deps, "accounts", "edit", "side", "--primary")
			if got.stdout != tt.wantOut || got.code != 0 {
				t.Errorf("switchboard accounts edit side --primary = %+v, want stdout %q", got, tt.wantOut)
			}
		})
	}
}

func TestAccountsEditTakesItsIDWithAStrayVariationSelectorAsNoLabel(t *testing.T) {
	deps, path := accountsDeps(t, newClaudeAPI(t).URL, "\n[[account]]\nid    = \"work\"\nlabel = \"Work\"\n")

	got := run(t, deps, "accounts", "edit", "work", "--label", "work\ufe0f")
	if want := (result{stdout: "edited work: label none, shown by its id (was \"Work\")\n"}); got != want {
		t.Errorf("switchboard accounts edit work --label = %+v, want %+v", got, want)
	}
	if got := readFile(t, path); !strings.HasSuffix(got, "\n[[account]]\nid    = \"work\"\n") || strings.Contains(got, "label") {
		t.Errorf("the config reads\n%s\nwant work's label taken out", got)
	}
}

func TestAccountsRemoveNamesAnIDConfiguredInAnotherCase(t *testing.T) {
	deps, path := accountsDeps(t, newClaudeAPI(t).URL, personalAndSide)
	before := readFile(t, path)

	got := run(t, deps, "accounts", "remove", "Side")
	if want := (result{stderr: "Error: account \"Side\" is not configured: side is; give an id in its own case\n", code: 1}); got != want {
		t.Errorf("switchboard accounts remove Side = %+v, want %+v", got, want)
	}
	if readFile(t, path) != before {
		t.Errorf("the config reads\n%s\nwant it as it was\n%s", readFile(t, path), before)
	}
}

func TestAccountsEditMarksTheFirstThePrimaryWhereNoneIsMarked(t *testing.T) {
	deps, path := accountsDeps(t, newClaudeAPI(t).URL, personalAndSide)
	writeToken(t, deps, "personal", "test-token-personal")
	before := readFile(t, path)

	got := run(t, deps, "accounts", "edit", "personal", "--primary")
	if want := (result{stdout: "edited personal: marked the primary, which it was as the first with none marked\n"}); got != want {
		t.Errorf("switchboard accounts edit personal --primary = %+v, want %+v", got, want)
	}
	want := strings.Replace(before, "label = \"Personal\"\n", "label = \"Personal\"\nprimary = true\n", 1)
	if readFile(t, path) != want {
		t.Errorf("the config reads\n%s\nwant\n%s", readFile(t, path), want)
	}
}

func TestAccountsEditChangesNothingGivenWhatTheAccountHas(t *testing.T) {
	deps, path := accountsDeps(t, newClaudeAPI(t).URL, sideOnAPlan)
	before := readFile(t, path)

	got := run(t, deps, "accounts", "edit", "side", "--label", "Side", "--plan", "pro", "--reserve", "10%")
	if want := (result{stdout: "side has every value given already, so nothing changed\n"}); got != want {
		t.Errorf("switchboard accounts edit = %+v, want %+v", got, want)
	}
	if readFile(t, path) != before {
		t.Errorf("the config reads\n%s\nwant it as it was\n%s", readFile(t, path), before)
	}
}

func TestAccountsEditRefuses(t *testing.T) {
	const (
		noFlags      = "give one or more of --label, --plan, --primary or --reserve"
		primaryFalse = "--primary marks this account the primary, and can't be false: the primary moves by giving --primary to another account"
	)
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no flags", args: []string{"side"}, wantErr: noFlags},
		{name: "--primary=false alone", args: []string{"side", "--primary=false"}, wantErr: primaryFalse},
		{name: "--primary=false with another flag", args: []string{"side", "--plan", "pro", "--primary=false"}, wantErr: primaryFalse},
		{
			name:    "a reserve for an account not configured, which is what's said",
			args:    []string{"nosuch", "--reserve", "5"},
			wantErr: `account "nosuch" is not configured: add it with switchboard accounts add nosuch`,
		},
		{
			name:    "a plan for an account not configured, which is what's said",
			args:    []string{"nosuch", "--plan", "free"},
			wantErr: `account "nosuch" is not configured: add it with switchboard accounts add nosuch`,
		},
		{
			name:    "no flags for an account not configured, which is what's said",
			args:    []string{"nosuch"},
			wantErr: `account "nosuch" is not configured: add it with switchboard accounts add nosuch`,
		},
		{
			name:    "a reserve with a sign",
			args:    []string{"side", "--reserve", "+5%"},
			wantErr: `reserve "+5%": must be a percentage, to two decimal places at most, such as 5% or 2.5%, or 0 for none`,
		},
		{
			name:    "an account not configured",
			args:    []string{"work", "--plan", "pro"},
			wantErr: `account "work" is not configured: add it with switchboard accounts add work`,
		},
		{
			name:    "a token given as the id, never shown",
			args:    []string{tokenShaped, "--plan", "pro"},
			wantErr: `account "[redacted]" is not configured: add it with switchboard accounts add [redacted]`,
		},
		{
			name:    "a plan that isn't one",
			args:    []string{"side", "--plan", "free"},
			wantErr: `plan "free": must be pro, max5x, max20x or none`,
		},
		{
			name:    "a reserve as a number alone",
			args:    []string{"side", "--reserve", "5"},
			wantErr: `reserve "5": must be written as a percentage, such as 5%, as a number alone could be a share or a percentage`,
		},
		{
			name:    "a reserve as a share alone",
			args:    []string{"side", "--reserve", "0.05"},
			wantErr: `reserve "0.05": must be written as a percentage, such as 5%, as a number alone could be a share or a percentage`,
		},
		{
			name:    "a reserve of the whole window",
			args:    []string{"side", "--reserve", "100%"},
			wantErr: `reserve "100%": must be at least 0% and less than 100%, the share of every window the router leaves unused`,
		},
		{
			name:    "a reserve past two decimal places",
			args:    []string{"side", "--reserve", "99.999%"},
			wantErr: `reserve "99.999%": must be a percentage, to two decimal places at most, such as 5% or 2.5%, or 0 for none`,
		},
		{
			name:    "a blank label",
			args:    []string{"side", "--label", "  ", "--plan", "pro"},
			wantErr: `account "side": label is blank, showing nothing: give one that shows, or --label "" to show the account by its id`,
		},
		{
			name:    "a label holding a newline, a control character",
			args:    []string{"side", "--label", "Side\nTeam"},
			wantErr: `account "side": label holds a control character, such as a newline or a tab, which can't show`,
		},
		{
			name:    "a blank label for an account not configured, which is what's said",
			args:    []string{"nosuch", "--label", "  "},
			wantErr: `account "nosuch" is not configured: add it with switchboard accounts add nosuch`,
		},
		{
			name:    "a label of a zero-width space alone",
			args:    []string{"side", "--label", "\u200b"},
			wantErr: `account "side": label is blank, showing nothing: give one that shows, or --label "" to show the account by its id`,
		},
		{
			name:    "an id configured in another case",
			args:    []string{"Side", "--plan", "pro"},
			wantErr: `account "Side" is not configured: side is; give an id in its own case`,
		},
		{
			name:    "a label shaped like a token, never shown",
			args:    []string{"side", "--label", tokenShaped},
			wantErr: `account "side": label looks like a token, which a label mustn't, as it shows wherever the account does`,
		},
		{
			name:    "a label holding a sign that hides the character after it",
			args:    []string{"side", "--label", "Side \u0600123"},
			wantErr: `account "side": label holds a sign that joins the character after it, such as an Arabic number sign, which can hide that character`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, path := accountsDeps(t, newClaudeAPI(t).URL, personalAndSide)
			before := readFile(t, path)
			args := append([]string{"accounts", "edit"}, tt.args...)

			got := run(t, deps, args...)
			if want := (result{stderr: "Error: " + tt.wantErr + "\n", code: 1}); got != want {
				t.Errorf("switchboard %s = %+v, want %+v", strings.Join(args, " "), got, want)
			}
			if readFile(t, path) != before {
				t.Errorf("the config reads\n%s\nwant it as it was\n%s", readFile(t, path), before)
			}
			checkNoToken(t, got, tokenShaped)
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
