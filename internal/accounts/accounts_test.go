package accounts_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude"
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

// now is when the user gives a token, by the registry's clock.
var now = time.Date(2026, 10, 27, 9, 30, 0, 0, time.UTC)

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
			w.checkTaken(t, "side", sideToken, now)
			if want := []string{upstream + " " + sideToken}; !slices.Equal(api.asked, want) {
				t.Errorf("the API was asked about %q, want %q", api.asked, want)
			}
			if want := map[bool]string{true: sidePrompt}[tt.atTerminal]; user.shown.String() != want {
				t.Errorf("the user was shown %q, want %q", user.shown.String(), want)
			}
		})
	}
}

func TestARegistryWithoutAClockTakesTheTokenAtTheSystemsTime(t *testing.T) {
	w := newWorld(t, twoAccounts)
	registry := w.registry(&user{gives: sideToken}, &fakeAPI{})
	registry.Now = nil
	before := time.Now().Truncate(time.Second)

	if _, err := registry.Add(t.Context(), config.NewAccount{ID: "side", Label: "Side"}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	at, err := w.tokens().Taken("side", tokenOf(t, sideToken))
	if err != nil || at.Before(before) || at.After(time.Now()) {
		t.Errorf("side's token was taken at %v (%v), want the system's time, from %v", at, err, before)
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
	w.checkTaken(t, "side", sideToken, time.Time{})
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
	if err != nil || taken.Kept || errorText(taken.Unchecked) != unanswered.Error() {
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
			name:      "the user interrupting the token's typing",
			account:   config.NewAccount{ID: "side"},
			user:      &user{atTerminal: true, gives: sideToken, fails: accounts.ErrInterrupted},
			wantErr:   "read the token: interrupted",
			wantIs:    accounts.ErrInterrupted,
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
			w.checkTaken(t, "side", sideToken, now)
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
		name   string
		answer error
		// wantUnchecked is why the token went unchecked, as Taken says.
		wantUnchecked string
		wantErr       string
		wantToken     string
		// wantAt is when the token in the file was taken, as its record says:
		// zero where that isn't known.
		wantAt time.Time
	}{
		{name: "refusing the token, which isn't saved", answer: refusal{}, wantErr: refusedErr, wantToken: "test-token-stale"},
		{name: "not answering, the token saved all the same", answer: unanswered, wantUnchecked: unanswered.Error(), wantToken: sideToken, wantAt: now},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, withSide)
			w.writeToken(t, "side", "test-token-stale", 0o600)

			taken, err := w.registry(&user{gives: sideToken}, &fakeAPI{err: tt.answer}).SetToken(t.Context(), w.load(t), "side")
			if taken.Kept || errorText(taken.Unchecked) != tt.wantUnchecked || errorText(err) != tt.wantErr {
				t.Errorf("SetToken() = %+v, %v, want unchecked %q, %q", taken, err, tt.wantUnchecked, tt.wantErr)
			}
			w.checkToken(t, "side", tt.wantToken)
			w.checkTaken(t, "side", tt.wantToken, tt.wantAt)
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
			if want := (accounts.Removed{File: tt.wantRemoved}); err != nil || removed != want {
				t.Fatalf("Remove() = %+v, %v, want %+v", removed, err, want)
			}
			w.checkConfig(t, twoAccounts)
			w.checkNoToken(t, "side")
			w.checkToken(t, "work", workToken)
		})
	}
}

func TestRemoveSaysWhichAccountIsThePrimaryOnceTheOneThatWasGoes(t *testing.T) {
	tests := []struct {
		name   string
		config string
		// id is the account removed, and primary the primary once it's gone.
		id, primary string
		// wantPrimary is the primary Remove says of, "" for none.
		wantPrimary string
	}{
		{name: "the first, the primary for want of one marked", config: withSide, id: "work", primary: "personal", wantPrimary: "personal"},
		{
			name:        "the one marked, leaving the first the primary",
			config:      withSide + "primary = true\n",
			id:          "side",
			primary:     "work",
			wantPrimary: "work",
		},
		{name: "another than the primary", config: withSide, id: "side", primary: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, tt.config)
			w.writeToken(t, tt.id, sideToken, 0o600)

			removed, err := w.registry(&user{}, &fakeAPI{}).Remove(tt.id)
			if want := (accounts.Removed{File: true, Primary: tt.wantPrimary}); err != nil || removed != want {
				t.Fatalf("Remove() = %+v, %v, want %+v", removed, err, want)
			}
			if got := w.load(t).Accounts.Primary().ID; got != tt.primary {
				t.Errorf("the config's primary is %s, want %s", got, tt.primary)
			}
		})
	}
}

func TestRemoveLeavesWhereATokenFilesLinkLeads(t *testing.T) {
	w := newWorld(t, withSide)
	// side's token file, a link to a file kept elsewhere, as by a dotfiles
	// step.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "side")
	if err := os.WriteFile(target, []byte(sideToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(w.tokens().Path("side")), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, w.tokens().Path("side")); err != nil {
		t.Fatal(err)
	}

	removed, err := w.registry(&user{}, &fakeAPI{}).Remove("side")
	if want := (accounts.Removed{File: true, LinkedTo: target}); err != nil || removed != want {
		t.Fatalf("Remove() = %+v, %v, want %+v", removed, err, want)
	}
	w.checkNoToken(t, "side")
	if data, err := os.ReadFile(target); err != nil || string(data) != sideToken+"\n" {
		t.Errorf("where the link led holds %q (%v), want the token still", data, err)
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

// organization is a placeholder id of the organization an account is in.
const organization = "test-org-side"

func TestCheck(t *testing.T) {
	// echoed is an API error message that quotes the token, as no answer
	// should, to show none of it gets through.
	echoed := "bad bearer token " + sideToken
	tests := []struct {
		name    string
		answer  answerWith
		want    accounts.Checked
		wantErr string
		wantIs  error
	}{
		{name: "taken, naming its organization", answer: answerWith{status: http.StatusOK, organization: organization}, want: accounts.Checked{Organization: organization}},
		{name: "taken, naming no organization", answer: answerWith{status: http.StatusOK}, want: accounts.Checked{}},
		{
			name:    "refused as a token, answering 401",
			answer:  answerWith{status: http.StatusUnauthorized, message: echoed},
			wantErr: "the API refused the token (HTTP 401 · bad bearer token [redacted]), so nothing is saved: make another with claude setup-token, run while signed in to that subscription",
			wantIs:  accounts.ErrRefused,
		},
		{
			name:    "refused as a request, answering 403",
			answer:  answerWith{status: http.StatusForbidden, message: echoed},
			wantErr: "the API refused the token (HTTP 403 · bad bearer token [redacted]), so nothing is saved: make another with claude setup-token, run while signed in to that subscription",
			wantIs:  accounts.ErrRefused,
		},
		{
			name:   "unanswered, the API failing",
			answer: answerWith{status: http.StatusInternalServerError, message: echoed},
			want:   accounts.Checked{Unchecked: true, Why: "HTTP 500 · bad bearer token [redacted]"},
		},
		{
			name:   "unanswered, the API overloaded",
			answer: answerWith{status: 529},
			want:   accounts.Checked{Unchecked: true, Why: "HTTP 529"},
		},
		{
			name:   "unanswered, the API taking too long",
			answer: answerWith{hang: true},
			want:   accounts.Checked{Unchecked: true, Why: "timed out after 50ms"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, strings.Replace(twoAccounts, upstream, newRealAPI(t, tt.answer), 1))
			log := logstest.Capture(t)

			got, err := realRegistry(w).Check(t.Context(), tokenOf(t, sideToken))
			if got != tt.want || errorText(err) != tt.wantErr {
				t.Errorf("Check() = %+v, %v, want %+v, %q", got, err, tt.want, tt.wantErr)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("Check() error = %v, want one matching %v", err, tt.wantIs)
			}
			checkNoneOfTheToken(t, sideToken, got.Why, errorText(err), log.String())
		})
	}
}

func TestCheckWithNoNetwork(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	w := newWorld(t, fmt.Sprintf("upstream = %q\n", srv.URL))
	log := logstest.Capture(t)

	got, err := realRegistry(w).Check(t.Context(), tokenOf(t, sideToken))
	if err != nil || !got.Unchecked || got.Why == "" {
		t.Errorf("Check() = %+v, %v, want it unchecked, saying why", got, err)
	}
	checkNoneOfTheToken(t, sideToken, got.Why, log.String())
}

func TestCheckAsksTheConfigsUpstream(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   string
	}{
		{name: "the config's, its accounts set up", config: twoAccounts, want: upstream},
		{name: "the config's, before any account", config: fmt.Sprintf("upstream = %q\n", upstream), want: upstream},
		{name: "the default, without a config file", want: "https://api.anthropic.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, tt.config)
			api := &fakeAPI{}

			if _, err := w.registry(&user{}, api).Check(t.Context(), tokenOf(t, sideToken)); err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if want := []string{tt.want + " " + sideToken}; !slices.Equal(api.asked, want) {
				t.Errorf("the API was asked about %q, want %q", api.asked, want)
			}
		})
	}
}

func TestCheckRefusesAnUpstreamAConfigCantHave(t *testing.T) {
	w := newWorld(t, "upstream = \"http://192.0.2.1:4700\"\n")
	api := &fakeAPI{}

	_, err := w.registry(&user{}, api).Check(t.Context(), tokenOf(t, sideToken))
	if err == nil || !strings.HasPrefix(err.Error(), "invalid config "+w.config+":\nupstream ") {
		t.Errorf("Check() error = %v, want the upstream refused", err)
	}
	if len(api.asked) > 0 {
		t.Errorf("the API was asked about %q, want nothing", api.asked)
	}
}

func TestAddGiven(t *testing.T) {
	w := newWorld(t, twoAccounts)
	w.writeToken(t, "side", "test-token-stale", 0o600)
	user := &user{atTerminal: true}
	api := &fakeAPI{organization: organization}

	got, err := w.registry(user, api).AddGiven(t.Context(), config.NewAccount{ID: "side", Label: "Side", Plan: "max20x"}, tokenOf(t, sideToken))
	if want := (accounts.Checked{Organization: organization}); err != nil || got != want {
		t.Fatalf("AddGiven() = %+v, %v, want %+v", got, err, want)
	}
	w.checkConfig(t, twoAccounts+"\n[[account]]\nid    = \"side\"\nlabel = \"Side\"\nplan  = \"max20x\"\n")
	w.checkToken(t, "side", sideToken)
	w.checkTaken(t, "side", sideToken, now)
	if want := []string{upstream + " " + sideToken}; !slices.Equal(api.asked, want) {
		t.Errorf("the API was asked about %q, want %q", api.asked, want)
	}
	if user.asked {
		t.Error("the user was asked for a token, want the one given taken")
	}
}

func TestAddGivenWhenTheAPIDoesntSayYes(t *testing.T) {
	tests := []struct {
		name       string
		answer     error
		want       accounts.Checked
		wantErr    string
		wantConfig string
	}{
		{name: "refusing the token, nothing saved", answer: refusal{}, wantErr: refusedErr, wantConfig: twoAccounts},
		{
			name:       "not answering, the account and its token saved all the same",
			answer:     errors.New("timed out after 5s"),
			want:       accounts.Checked{Unchecked: true, Why: "timed out after 5s"},
			wantConfig: withSide,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, twoAccounts)

			got, err := w.registry(&user{}, &fakeAPI{err: tt.answer}).AddGiven(t.Context(), config.NewAccount{ID: "side", Label: "Side"}, tokenOf(t, sideToken))
			if got != tt.want || errorText(err) != tt.wantErr {
				t.Errorf("AddGiven() = %+v, %v, want %+v, %q", got, err, tt.want, tt.wantErr)
			}
			w.checkConfig(t, tt.wantConfig)
			if tt.wantErr != "" {
				w.checkNoToken(t, "side")
				return
			}
			w.checkToken(t, "side", sideToken)
			w.checkTaken(t, "side", sideToken, now)
		})
	}
}

// checkedWrites are the ways to save a token Check has checked: with the
// account it's added for, or as an account's token in place of the one it
// had.
var checkedWrites = []struct {
	name   string
	config string
	write  func(ctx context.Context, r accounts.Registry, token tokens.Token, checked accounts.Checked) error
}{
	{
		name:   "AddChecked",
		config: twoAccounts,
		write: func(ctx context.Context, r accounts.Registry, token tokens.Token, checked accounts.Checked) error {
			return r.AddChecked(ctx, config.NewAccount{ID: "side", Label: "Side"}, token, checked)
		},
	},
	{
		name:   "SetTokenChecked",
		config: withSide,
		write: func(ctx context.Context, r accounts.Registry, token tokens.Token, checked accounts.Checked) error {
			return r.SetTokenChecked(ctx, "side", token, checked)
		},
	},
}

func TestTheCheckedWritesSaveWithoutAskingTheAPI(t *testing.T) {
	checks := []struct {
		name    string
		checked accounts.Checked
		// wantWarned is set when the write warns the token went unchecked.
		wantWarned bool
	}{
		{name: "checked", checked: accounts.Checked{Organization: organization}},
		{name: "unchecked", checked: accounts.Checked{Unchecked: true, Why: "timed out after 5s"}, wantWarned: true},
	}
	for _, write := range checkedWrites {
		for _, c := range checks {
			t.Run(write.name+", "+c.name, func(t *testing.T) {
				w := newWorld(t, write.config)
				w.writeToken(t, "side", "test-token-stale", 0o600)
				log := logstest.Capture(t)
				user := &user{atTerminal: true}

				if err := write.write(t.Context(), w.unasked(t, user), tokenOf(t, sideToken), c.checked); err != nil {
					t.Fatalf("%s() error = %v", write.name, err)
				}
				w.checkConfig(t, withSide)
				w.checkToken(t, "side", sideToken)
				w.checkTaken(t, "side", sideToken, now)
				warned := log.Has("level=WARN", `msg="couldn't check a token with the API"`, "account=side", `error="timed out after 5s"`)
				if warned != c.wantWarned {
					t.Errorf("the log reads\n%s\nwant the unchecked warning: %v", log, c.wantWarned)
				}
				if want := fmt.Sprintf("checked=%t", !c.checked.Unchecked); !log.Has("level=INFO", `msg="saved a token"`, "account=side", want) {
					t.Errorf("the log reads\n%s\nwant the token saved, %s", log, want)
				}
				if user.asked {
					t.Error("the user was asked for a token, want the one given taken")
				}
			})
		}
	}
}

func TestAddCheckedRefusesAnAccountAConfigCantAdd(t *testing.T) {
	w := newWorld(t, twoAccounts)

	err := w.unasked(t, &user{}).AddChecked(t.Context(), config.NewAccount{ID: "personal"}, tokenOf(t, sideToken), accounts.Checked{})
	if !errors.Is(err, config.ErrConfigured) {
		t.Errorf("AddChecked() error = %v, want one matching %v", err, config.ErrConfigured)
	}
	w.checkConfig(t, twoAccounts)
	w.checkNoToken(t, "personal")
}

func TestSetTokenCheckedRefusesAnAccountNotConfigured(t *testing.T) {
	for _, given := range []string{twoAccounts, ""} {
		w := newWorld(t, given)

		err := w.unasked(t, &user{}).SetTokenChecked(t.Context(), "side", tokenOf(t, sideToken), accounts.Checked{})
		if want := `account "side" is not configured`; err == nil || err.Error() != want || !errors.Is(err, config.ErrNotConfigured) {
			t.Errorf("SetTokenChecked() with the config %q: error = %v, want %q", given, err, want)
		}
		w.checkNoToken(t, "side")
	}
}

func TestTheGivenWritesCheckTheAccountBeforeAskingTheAPI(t *testing.T) {
	tests := []struct {
		name   string
		config string
		give   func(ctx context.Context, r accounts.Registry, token tokens.Token) error
		wantIs error
	}{
		{
			name:   "AddGiven, an id an account has already",
			config: twoAccounts,
			give: func(ctx context.Context, r accounts.Registry, token tokens.Token) error {
				_, err := r.AddGiven(ctx, config.NewAccount{ID: "personal"}, token)
				return err
			},
			wantIs: config.ErrConfigured,
		},
		{
			name:   "AddGiven, an id that isn't a plain file name",
			config: twoAccounts,
			give: func(ctx context.Context, r accounts.Registry, token tokens.Token) error {
				_, err := r.AddGiven(ctx, config.NewAccount{ID: "../side"}, token)
				return err
			},
		},
		{
			name:   "SetTokenGiven, an account not configured",
			config: twoAccounts,
			give: func(ctx context.Context, r accounts.Registry, token tokens.Token) error {
				_, err := r.SetTokenGiven(ctx, "side", token)
				return err
			},
			wantIs: config.ErrNotConfigured,
		},
		{
			name: "SetTokenGiven, without a config file",
			give: func(ctx context.Context, r accounts.Registry, token tokens.Token) error {
				_, err := r.SetTokenGiven(ctx, "side", token)
				return err
			},
			wantIs: config.ErrNotConfigured,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, tt.config)

			err := tt.give(t.Context(), w.unasked(t, &user{}), tokenOf(t, sideToken))
			if err == nil || (tt.wantIs != nil && !errors.Is(err, tt.wantIs)) {
				t.Errorf("error = %v, want one matching %v", err, tt.wantIs)
			}
			if tt.config != "" {
				w.checkConfig(t, tt.config)
			}
			w.checkNoToken(t, "side")
		})
	}
}

// givenWrites are the ways to check and save a token in one: with the
// account it's added for, or as an account's token in place of the one it
// had, which is test-token-stale.
var givenWrites = []struct {
	name   string
	config string
	give   func(ctx context.Context, r accounts.Registry, token tokens.Token) (accounts.Checked, error)
}{
	{
		name:   "AddGiven",
		config: twoAccounts,
		give: func(ctx context.Context, r accounts.Registry, token tokens.Token) (accounts.Checked, error) {
			return r.AddGiven(ctx, config.NewAccount{ID: "side", Label: "Side"}, token)
		},
	},
	{
		name:   "SetTokenGiven",
		config: withSide,
		give: func(ctx context.Context, r accounts.Registry, token tokens.Token) (accounts.Checked, error) {
			return r.SetTokenGiven(ctx, "side", token)
		},
	},
}

// givingUp are the ways a caller gives up on a check under way: cancelling
// it once the API is asked, or a deadline sooner than the prober's own.
var givingUp = []struct {
	name   string
	ctx    func(t *testing.T, asked <-chan struct{}) context.Context
	wantIs error
}{
	{
		name: "cancelled mid-probe",
		ctx: func(t *testing.T, asked <-chan struct{}) context.Context {
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			go func() {
				<-asked
				cancel()
			}()
			return ctx
		},
		wantIs: context.Canceled,
	},
	{
		name: "a deadline passed",
		ctx: func(t *testing.T, _ <-chan struct{}) context.Context {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
			t.Cleanup(cancel)
			return ctx
		},
		wantIs: context.DeadlineExceeded,
	},
}

func TestCheckFailsOnceTheCallerGivesUp(t *testing.T) {
	for _, up := range givingUp {
		t.Run(up.name, func(t *testing.T) {
			asked := make(chan struct{}, 1)
			w := newWorld(t, strings.Replace(twoAccounts, upstream, newRealAPI(t, answerWith{hang: true, asked: asked}), 1))

			got, err := patientRegistry(w).Check(up.ctx(t, asked), tokenOf(t, sideToken))
			if !errors.Is(err, up.wantIs) || !strings.HasPrefix(errorText(err), "check the token with the API: ") || got != (accounts.Checked{}) {
				t.Errorf("Check() = %+v, %v, want the caller's error, %v", got, err, up.wantIs)
			}
		})
	}
}

func TestTheGivenWritesWriteNothingOnceTheCallerGivesUp(t *testing.T) {
	for _, write := range givenWrites {
		for _, up := range givingUp {
			t.Run(write.name+", "+up.name, func(t *testing.T) {
				asked := make(chan struct{}, 1)
				given := strings.ReplaceAll(write.config, upstream, newRealAPI(t, answerWith{hang: true, asked: asked}))
				w := newWorld(t, given)
				w.writeToken(t, "side", "test-token-stale", 0o600)

				got, err := write.give(up.ctx(t, asked), patientRegistry(w), tokenOf(t, sideToken))
				if !errors.Is(err, up.wantIs) || got != (accounts.Checked{}) {
					t.Errorf("%s() = %+v, %v, want the caller's error, %v", write.name, got, err, up.wantIs)
				}
				w.checkConfig(t, given)
				w.checkToken(t, "side", "test-token-stale")
			})
		}
	}
}

func TestTheCheckedWritesWriteNothingOnceTheContextIsDone(t *testing.T) {
	for _, write := range checkedWrites {
		t.Run(write.name, func(t *testing.T) {
			w := newWorld(t, write.config)
			w.writeToken(t, "side", "test-token-stale", 0o600)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err := write.write(ctx, w.unasked(t, &user{}), tokenOf(t, sideToken), accounts.Checked{})
			if !errors.Is(err, context.Canceled) {
				t.Errorf("%s() error = %v, want one matching %v", write.name, err, context.Canceled)
			}
			w.checkConfig(t, write.config)
			w.checkToken(t, "side", "test-token-stale")
		})
	}
}

// unasked returns the world's registry, as registry does, whose prober
// fails the test if it's asked anything.
func (w *world) unasked(t *testing.T, user *user) accounts.Registry {
	r := w.registry(user, nil)
	r.Prober = func(upstream string) accounts.Prober {
		t.Errorf("the API at %s was asked about a token, want nothing", upstream)
		return prober{api: &fakeAPI{}, upstream: upstream}
	}
	return r
}

func TestSetTokenGiven(t *testing.T) {
	tests := []struct {
		name      string
		answer    error
		want      accounts.Checked
		wantErr   string
		wantToken string
		// wantAt is when the token in the file was taken, as its record says:
		// zero where that isn't known.
		wantAt time.Time
	}{
		{name: "taken, naming its organization", want: accounts.Checked{Organization: organization}, wantToken: sideToken, wantAt: now},
		{name: "refusing the token, which isn't saved", answer: refusal{}, wantErr: refusedErr, wantToken: "test-token-stale"},
		{
			name:      "not answering, the token saved all the same",
			answer:    errors.New("timed out after 5s"),
			want:      accounts.Checked{Unchecked: true, Why: "timed out after 5s"},
			wantToken: sideToken,
			wantAt:    now,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, withSide)
			w.writeToken(t, "side", "test-token-stale", 0o600)
			user := &user{atTerminal: true}
			api := &fakeAPI{err: tt.answer, organization: organization}

			got, err := w.registry(user, api).SetTokenGiven(t.Context(), "side", tokenOf(t, sideToken))
			if got != tt.want || errorText(err) != tt.wantErr {
				t.Errorf("SetTokenGiven() = %+v, %v, want %+v, %q", got, err, tt.want, tt.wantErr)
			}
			w.checkToken(t, "side", tt.wantToken)
			w.checkTaken(t, "side", tt.wantToken, tt.wantAt)
			w.checkConfig(t, withSide)
			if user.asked {
				t.Error("the user was asked for a token, want the one given taken")
			}
		})
	}
}

func TestTheTokenGivenNeverShows(t *testing.T) {
	w := newWorld(t, twoAccounts)
	log := logstest.Capture(t)
	quoting := errors.New("HTTP 500 · bad bearer token " + sideToken)
	registry := w.registry(&user{}, &fakeAPI{err: quoting})
	token := tokenOf(t, sideToken)

	checked, checkErr := registry.Check(t.Context(), token)
	added, addErr := registry.AddGiven(t.Context(), config.NewAccount{ID: "side"}, token)
	set, setErr := registry.SetTokenGiven(t.Context(), "side", token)
	if err := errors.Join(checkErr, addErr, setErr); err != nil {
		t.Fatal(err)
	}
	typed := w.registry(&user{gives: sideToken}, &fakeAPI{err: quoting})
	retaken, retakeErr := typed.SetToken(t.Context(), w.load(t), "side")
	if retakeErr != nil {
		t.Fatal(retakeErr)
	}
	_, refuseErr := w.registry(&user{}, &fakeAPI{err: quotingRefusal{}}).SetTokenGiven(t.Context(), "side", token)
	if !errors.Is(refuseErr, accounts.ErrRefused) {
		t.Errorf("SetTokenGiven() error = %v, want the token refused", refuseErr)
	}
	for _, why := range []string{checked.Why, added.Why, set.Why, errorText(retaken.Unchecked)} {
		if !strings.HasPrefix(why, "HTTP 500") {
			t.Errorf("why the token went unchecked = %q, want the API's failure", why)
		}
	}
	checkNoneOfTheToken(t, sideToken, checked.Why, added.Why, set.Why, errorText(retaken.Unchecked), errorText(refuseErr), log.String())
}

// checkNoneOfTheToken checks that none of what's shown holds the token, or
// its first or last characters, as cutting it short from either end would
// leave them.
func checkNoneOfTheToken(t *testing.T, token string, shown ...string) {
	t.Helper()
	const part = 8
	for _, s := range shown {
		for _, piece := range []string{token, token[:part], token[len(token)-part:]} {
			if strings.Contains(s, piece) {
				t.Errorf("%q shows the token, or part of it: %q", s, piece)
			}
		}
	}
}

// quotingRefusal is the API refusing a token, its failure quoting it.
type quotingRefusal struct{}

func (quotingRefusal) Error() string { return "HTTP 401 · bad bearer token " + sideToken }

func (quotingRefusal) Refused() bool { return true }

// answerWith is how a messages API answers each probe: with a status, and
// the usage headers of a 200, the organization named, and an API error's
// message, where given; or, hanging, not until the probe gives up.
type answerWith struct {
	status       int
	organization string
	message      string
	hang         bool
	// asked, where given, is told when the API is first asked, unless it's
	// told already.
	asked chan<- struct{}
}

// newRealAPI serves a messages API over loopback that answers every probe
// as a says, and returns its URL.
func newRealAPI(t *testing.T, a answerWith) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case a.asked <- struct{}{}:
		default:
		}
		if a.hang {
			<-r.Context().Done()
			return
		}
		if a.organization != "" {
			w.Header().Set("anthropic-organization-id", a.organization)
		}
		if a.status == http.StatusOK {
			for _, key := range []string{"5h", "7d", "7d_oi"} {
				w.Header().Set("anthropic-ratelimit-unified-"+key+"-utilization", "0.25")
			}
		}
		w.WriteHeader(a.status)
		if a.message != "" {
			_, _ = fmt.Fprintf(w, `{"type":"error","error":{"type":"api_error","message":%q}}`, a.message)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// realRegistry is the world's registry, checking tokens with the Claude
// provider's prober, which gives up on a probe after 50ms.
func realRegistry(w *world) accounts.Registry {
	return registryProbingWithin(w, 50*time.Millisecond)
}

// patientRegistry is realRegistry with a prober that waits a minute, so the
// caller gives up first however slowly a loaded machine runs its cancel.
func patientRegistry(w *world) accounts.Registry {
	return registryProbingWithin(w, time.Minute)
}

func registryProbingWithin(w *world, limit time.Duration) accounts.Registry {
	return accounts.Registry{
		ConfigPath: w.config,
		Tokens:     w.tokens(),
		Prober: func(upstream string) accounts.Prober {
			return &claude.Prober{Upstream: upstream, Timeout: limit}
		},
		Now: func() time.Time { return now },
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
	return accounts.Registry{ConfigPath: w.config, Tokens: w.tokens(), Prober: api.proberAt, Input: user.input(), Now: func() time.Time { return now }}
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

// checkTaken checks the account's record says its token, secret, was taken
// at want: that it says nothing of it where want is zero.
func (w *world) checkTaken(t *testing.T, id, secret string, want time.Time) {
	t.Helper()
	if at, err := w.tokens().Taken(id, tokenOf(t, secret)); !at.Equal(want) || (err == nil) == want.IsZero() {
		t.Errorf("%s's token was taken at %v (%v), want %v", id, at, err, want)
	}
}

// tokenOf is the token secret is.
func tokenOf(t *testing.T, secret string) tokens.Token {
	t.Helper()
	token, err := tokens.Parse(secret)
	if err != nil {
		t.Fatal(err)
	}
	return token
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

// fakeAPI answers every probe with err: nil takes the token, naming the
// organization given. It notes each token it's asked about, after the
// upstream the probe was made for.
type fakeAPI struct {
	err          error
	organization string
	asked        []string
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
	if p.api.err != nil {
		return quota.Probe{}, p.api.err
	}
	return quota.Probe{Organization: p.api.organization}, nil
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
