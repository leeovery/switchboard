package accounts_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/tokens"
)

const (
	workToken = "test-token-work"
	sideToken = "test-token-side"
	// upstream is where the config has requests go.
	upstream = "http://127.0.0.1:4700"
	// sidePrompt is how the user at a terminal is asked for side's token,
	// with the newline that ends it, which doesn't show as it's typed.
	sidePrompt = "Paste side's token, from claude setup-token run while signed in to that subscription (it won't show): \n"
	// refusedErr is the error of a token the API refuses.
	refusedErr = "the API refused the token (HTTP 401 · Invalid bearer token), so nothing is saved: " +
		"make another with claude setup-token, run while signed in to that subscription"
)

// twoAccounts is a config of two accounts, work and personal.
var twoAccounts = fmt.Sprintf("upstream = %q\n\n[[account]]\nid    = \"work\"\nlabel = \"Work\"\n\n[[account]]\nid = \"personal\"\n", upstream)

// withSide is twoAccounts once side, labelled Side, is added to it.
var withSide = twoAccounts + "\n[[account]]\nid    = \"side\"\nlabel = \"Side\"\n"

// users are the ways a user gives a token: typed at a terminal, or piped in.
var users = []struct {
	name       string
	atTerminal bool
}{
	{name: "piped in"},
	{name: "typed at a terminal", atTerminal: true},
}

func TestAddTakesTheTokenTheUserGives(t *testing.T) {
	for _, tt := range users {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, twoAccounts)
			user := &user{atTerminal: tt.atTerminal, gives: sideToken + "\n"}
			api := &fakeAPI{}

			taken, err := w.registry(user, api).Add(t.Context(), config.NewAccount{ID: "side", Label: "Side"})
			if err != nil || taken != (accounts.Taken{}) {
				t.Fatalf("Add() = %+v, %v, want the token taken and checked", taken, err)
			}
			w.checkConfig(t, withSide)
			w.checkToken(t, "side", sideToken)
			if want := []string{upstream + " " + sideToken}; !slices.Equal(api.asked, want) {
				t.Errorf("the API was asked about %q, want %q", api.asked, want)
			}
			if want := map[bool]string{true: sidePrompt}[tt.atTerminal]; user.shown.String() != want {
				t.Errorf("the user was shown %q, want %q", user.shown.String(), want)
			}
		})
	}
}

func TestAddMakesTheFirstConfig(t *testing.T) {
	w := newWorld(t, "")
	w.config = filepath.Join(filepath.Dir(w.config), "config", "switchboard", "config.toml")
	api := &fakeAPI{}

	_, err := w.registry(&user{gives: workToken}, api).Add(t.Context(), config.NewAccount{ID: "work", Label: "Work", Primary: true})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	w.checkConfig(t, "[[account]]\nid      = \"work\"\nlabel   = \"Work\"\nprimary = true\n")
	w.checkToken(t, "work", workToken)
	if want := []string{"https://api.anthropic.com " + workToken}; !slices.Equal(api.asked, want) {
		t.Errorf("the API was asked about %q, want %q: the config's default upstream", api.asked, want)
	}
}

func TestAddKeepsAUsableToken(t *testing.T) {
	w := newWorld(t, twoAccounts)
	w.writeToken(t, "side", sideToken, 0o600)
	user := &user{atTerminal: true}
	api := &fakeAPI{}

	taken, err := w.registry(user, api).Add(t.Context(), config.NewAccount{ID: "side", Label: "Side"})
	if err != nil || taken != (accounts.Taken{Kept: true}) {
		t.Fatalf("Add() = %+v, %v, want the token kept", taken, err)
	}
	w.checkConfig(t, withSide)
	w.checkToken(t, "side", sideToken)
	if user.asked || len(api.asked) > 0 {
		t.Errorf("the user was asked: %v; the API about %q; want neither", user.asked, api.asked)
	}
}

func TestAddReplacesATokenFileItCantUse(t *testing.T) {
	w := newWorld(t, twoAccounts)
	w.writeToken(t, "side", "test-token-stale", 0o644)

	_, err := w.registry(&user{gives: sideToken}, &fakeAPI{}).Add(t.Context(), config.NewAccount{ID: "side", Label: "Side"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	w.checkConfig(t, withSide)
	w.checkToken(t, "side", sideToken)
}

func TestAddSavesATokenTheAPIDidntAnswerFor(t *testing.T) {
	w := newWorld(t, twoAccounts)
	unanswered := errors.New("timed out after 5s")
	log := logstest.Capture(t)

	taken, err := w.registry(&user{gives: sideToken}, &fakeAPI{err: unanswered}).Add(t.Context(), config.NewAccount{ID: "side", Label: "Side"})
	if err != nil || taken != (accounts.Taken{Unchecked: unanswered}) {
		t.Fatalf("Add() = %+v, %v, want the token taken, unchecked", taken, err)
	}
	w.checkConfig(t, withSide)
	w.checkToken(t, "side", sideToken)
	if !log.Has("level=WARN", `msg="couldn't check a token with the API"`, "account=side", `error="timed out after 5s"`) {
		t.Errorf("the log reads\n%s\nwant a warning that the token went unchecked", log)
	}
	if strings.Contains(log.String(), sideToken) {
		t.Errorf("the log shows the token:\n%s", log)
	}
}

func TestAddSavesNothingTheAPIRefuses(t *testing.T) {
	w := newWorld(t, twoAccounts)

	_, err := w.registry(&user{gives: sideToken}, &fakeAPI{err: refusal{}}).Add(t.Context(), config.NewAccount{ID: "side", Label: "Side"})
	if err == nil || err.Error() != refusedErr || !errors.Is(err, accounts.ErrRefused) {
		t.Errorf("Add() error = %v, want %q", err, refusedErr)
	}
	w.checkConfig(t, twoAccounts)
	w.checkNoToken(t, "side")
}

func TestAddRefuses(t *testing.T) {
	failed := errors.New("input/output error")
	tests := []struct {
		name    string
		account config.NewAccount
		user    *user
		wantErr string
		wantIs  error
		// wantAsked is set when the user is asked for a token first.
		wantAsked bool
	}{
		{
			name:    "an id an account has already",
			account: config.NewAccount{ID: "personal"},
			user:    &user{gives: sideToken},
			wantErr: `account "personal" is already configured`,
			wantIs:  config.ErrConfigured,
		},
		{
			name:    "an id that isn't a plain file name",
			account: config.NewAccount{ID: "../side"},
			user:    &user{gives: sideToken},
			wantErr: `account "../side": id must start with a letter or digit and contain only letters, digits, '-' and '_'`,
		},
		{
			name:      "nothing piped in",
			account:   config.NewAccount{ID: "side"},
			user:      &user{gives: " \n"},
			wantErr:   "no token given",
			wantAsked: true,
		},
		{
			name:      "nothing typed",
			account:   config.NewAccount{ID: "side"},
			user:      &user{atTerminal: true},
			wantErr:   "no token given",
			wantAsked: true,
		},
		{
			name:      "the terminal closed before anything was typed",
			account:   config.NewAccount{ID: "side"},
			user:      &user{atTerminal: true, fails: io.EOF},
			wantErr:   "no token given",
			wantAsked: true,
		},
		{
			name:      "more than a token",
			account:   config.NewAccount{ID: "side"},
			user:      &user{gives: "export CLAUDE_CODE_OAUTH_TOKEN=" + sideToken},
			wantErr:   "what was given is more than a token: give the token alone",
			wantAsked: true,
		},
		{
			name:      "a terminal that can't be read",
			account:   config.NewAccount{ID: "side"},
			user:      &user{atTerminal: true, gives: sideToken, fails: failed},
			wantErr:   "read the token: input/output error",
			wantIs:    failed,
			wantAsked: true,
		},
		{
			name:      "a pipe that can't be read",
			account:   config.NewAccount{ID: "side"},
			user:      &user{gives: sideToken, fails: failed},
			wantErr:   "read the token: input/output error",
			wantIs:    failed,
			wantAsked: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, twoAccounts)
			api := &fakeAPI{}

			_, err := w.registry(tt.user, api).Add(t.Context(), tt.account)
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("Add() error = %v, want %q", err, tt.wantErr)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("Add() error = %v, want one matching %v", err, tt.wantIs)
			}
			if tt.user.asked != tt.wantAsked || len(api.asked) > 0 {
				t.Errorf("the user was asked: %v, want %v; the API about %q, want nothing", tt.user.asked, tt.wantAsked, api.asked)
			}
			w.checkConfig(t, twoAccounts)
			w.checkNoToken(t, tt.account.ID)
		})
	}
}

func TestSetToken(t *testing.T) {
	for _, tt := range users {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, withSide)
			w.writeToken(t, "side", "test-token-stale", 0o600)
			user := &user{atTerminal: tt.atTerminal, gives: sideToken}
			api := &fakeAPI{}

			taken, err := w.registry(user, api).SetToken(t.Context(), w.load(t), "side")
			if err != nil || taken != (accounts.Taken{}) {
				t.Fatalf("SetToken() = %+v, %v, want the token taken and checked", taken, err)
			}
			w.checkToken(t, "side", sideToken)
			w.checkConfig(t, withSide)
			if want := []string{upstream + " " + sideToken}; !slices.Equal(api.asked, want) {
				t.Errorf("the API was asked about %q, want %q", api.asked, want)
			}
			if want := map[bool]string{true: sidePrompt}[tt.atTerminal]; user.shown.String() != want {
				t.Errorf("the user was shown %q, want %q", user.shown.String(), want)
			}
		})
	}
}

func TestSetTokenWhenTheAPIDoesntSayYes(t *testing.T) {
	unanswered := errors.New("timed out after 5s")
	tests := []struct {
		name      string
		answer    error
		wantTaken accounts.Taken
		wantErr   string
		wantToken string
	}{
		{name: "refusing the token, which isn't saved", answer: refusal{}, wantErr: refusedErr, wantToken: "test-token-stale"},
		{name: "not answering, the token saved all the same", answer: unanswered, wantTaken: accounts.Taken{Unchecked: unanswered}, wantToken: sideToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, withSide)
			w.writeToken(t, "side", "test-token-stale", 0o600)

			taken, err := w.registry(&user{gives: sideToken}, &fakeAPI{err: tt.answer}).SetToken(t.Context(), w.load(t), "side")
			if taken != tt.wantTaken || errorText(err) != tt.wantErr {
				t.Errorf("SetToken() = %+v, %v, want %+v, %q", taken, err, tt.wantTaken, tt.wantErr)
			}
			w.checkToken(t, "side", tt.wantToken)
		})
	}
}

func TestSetTokenRefusesAnAccountNotConfigured(t *testing.T) {
	w := newWorld(t, twoAccounts)
	user := &user{gives: sideToken}
	api := &fakeAPI{}

	_, err := w.registry(user, api).SetToken(t.Context(), w.load(t), "side")
	if want := `account "side" is not configured`; err == nil || err.Error() != want || !errors.Is(err, config.ErrNotConfigured) {
		t.Errorf("SetToken() error = %v, want %q", err, want)
	}
	if user.asked || len(api.asked) > 0 {
		t.Errorf("the user was asked: %v; the API about %q; want neither", user.asked, api.asked)
	}
	w.checkNoToken(t, "side")
}

func TestRemove(t *testing.T) {
	tests := []struct {
		name string
		// token is set when the account has a token file.
		token       bool
		wantRemoved bool
	}{
		{name: "with its token file", token: true, wantRemoved: true},
		{name: "without a token file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, withSide)
			w.writeToken(t, "work", workToken, 0o600)
			if tt.token {
				w.writeToken(t, "side", sideToken, 0o600)
			}

			removed, err := w.registry(&user{}, &fakeAPI{}).Remove("side")
			if err != nil || removed != tt.wantRemoved {
				t.Fatalf("Remove() = %v, %v, want %v", removed, err, tt.wantRemoved)
			}
			w.checkConfig(t, twoAccounts)
			w.checkNoToken(t, "side")
			w.checkToken(t, "work", workToken)
		})
	}
}

func TestRemoveRefuses(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		id      string
		wantErr string
		wantIs  error
	}{
		{name: "an account not configured", config: twoAccounts, id: "side", wantErr: `account "side" is not configured`, wantIs: config.ErrNotConfigured},
		{name: "the only account", config: "[[account]]\nid = \"side\"\n", id: "side", wantErr: `account "side" is the only one, and a config needs one at least`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, tt.config)
			w.writeToken(t, "side", sideToken, 0o600)

			_, err := w.registry(&user{}, &fakeAPI{}).Remove(tt.id)
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("Remove() error = %v, want %q", err, tt.wantErr)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("Remove() error = %v, want one matching %v", err, tt.wantIs)
			}
			w.checkConfig(t, tt.config)
			w.checkToken(t, "side", sideToken)
		})
	}
}

func TestChangesAreLoggedWithoutTheToken(t *testing.T) {
	w := newWorld(t, twoAccounts)
	log := logstest.Capture(t)
	registry := w.registry(&user{gives: sideToken}, &fakeAPI{})

	if _, err := registry.Add(t.Context(), config.NewAccount{ID: "side", Label: "Side", Primary: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Remove("side"); err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]string{
		{"level=INFO", `msg="saved a token"`, "account=side", "checked=true"},
		{"level=INFO", `msg="added an account"`, "account=side", "primary=true", "token_kept=false"},
		{"level=INFO", `msg="removed an account"`, "account=side", "token_file=true"},
	} {
		if !log.Has(want...) {
			t.Errorf("the log reads\n%s\nwant a line with %q", log, want)
		}
	}
	if strings.Contains(log.String(), sideToken) {
		t.Errorf("the log shows the token:\n%s", log)
	}
}

// world is where a registry keeps the accounts: a config file and a state
// directory, the test's own.
type world struct {
	config string
	state  string
}

// newWorld returns a world whose config holds config, or that has no config
// file when config is empty.
func newWorld(t *testing.T, config string) *world {
	t.Helper()
	dir := t.TempDir()
	w := &world{config: filepath.Join(dir, "config.toml"), state: filepath.Join(dir, "state")}
	if config != "" {
		if err := os.WriteFile(w.config, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

// registry returns the world's registry, which takes tokens from user and
// checks them with api.
func (w *world) registry(user *user, api *fakeAPI) accounts.Registry {
	return accounts.Registry{ConfigPath: w.config, Tokens: w.tokens(), Prober: api.proberAt, Input: user.input()}
}

func (w *world) tokens() tokens.Store {
	return tokens.NewStore(w.state, os.Getuid())
}

func (w *world) load(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(w.config)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func (w *world) writeToken(t *testing.T, id, secret string, perm fs.FileMode) {
	t.Helper()
	path := w.tokens().Path(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(secret+"\n"), perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
}

func (w *world) checkConfig(t *testing.T, want string) {
	t.Helper()
	data, err := os.ReadFile(w.config)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Errorf("the config reads\n%s\nwant\n%s", data, want)
	}
}

func (w *world) checkToken(t *testing.T, id, want string) {
	t.Helper()
	token, err := w.tokens().Read(id)
	if err != nil || token.Reveal() != want {
		t.Errorf("%s's token file holds %q (%v), want %q", id, token.Reveal(), err, want)
	}
}

func (w *world) checkNoToken(t *testing.T, id string) {
	t.Helper()
	if _, err := os.Lstat(w.tokens().Path(id)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s's token file: %v, want none", id, err)
	}
}

// user gives a token: typed at a terminal, when they're at one, or piped in.
type user struct {
	atTerminal bool
	gives      string
	// fails is what reading what they give fails with.
	fails error
	// shown is what they're shown, at a terminal.
	shown strings.Builder
	asked bool
	piped *strings.Reader
}

func (u *user) input() accounts.Input {
	if u.atTerminal {
		return accounts.Input{Hidden: u.typeIt, Prompt: &u.shown}
	}
	return accounts.Input{Piped: u}
}

// typeIt types what the user gives, unseen, as a terminal reads a line: without
// the newline that ends it.
func (u *user) typeIt() ([]byte, error) {
	u.asked = true
	return []byte(strings.TrimSuffix(u.gives, "\n")), u.fails
}

// Read pipes in what the user gives.
func (u *user) Read(p []byte) (int, error) {
	u.asked = true
	if u.fails != nil {
		return 0, u.fails
	}
	if u.piped == nil {
		u.piped = strings.NewReader(u.gives)
	}
	return u.piped.Read(p)
}

// fakeAPI answers every probe with err: nil takes the token. It notes each
// token it's asked about, after the upstream the probe was made for.
type fakeAPI struct {
	err   error
	asked []string
}

func (a *fakeAPI) proberAt(upstream string) accounts.Prober {
	return prober{api: a, upstream: upstream}
}

type prober struct {
	api      *fakeAPI
	upstream string
}

func (p prober) Probe(_ context.Context, token string) (quota.Probe, error) {
	p.api.asked = append(p.api.asked, p.upstream+" "+token)
	return quota.Probe{}, p.api.err
}

// refusal is a probe's failure as the API refuses the token, as the Claude
// provider reads one.
type refusal struct{}

func (refusal) Error() string { return "HTTP 401 · Invalid bearer token" }

func (refusal) Refused() bool { return true }

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
