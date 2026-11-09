package launch_test

import (
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

// errUnread is why a launch can't read the config, as when an upgrade's
// stricter rules refuse the one the running router took.
var errUnread = errors.New("invalid config /home/tester/.config/switchboard/config.toml:\naccount \"work\": its label is longer than 100 characters")

// What stderr hears where the config can't be read: claude starting through
// the running router, or without switchboard.
const (
	throughRunning = "switchboard: couldn't read the config (invalid config /home/tester/.config/switchboard/config.toml) — starting claude through the running router\n"
	unreadUnaided  = "switchboard: couldn't read the config (invalid config /home/tester/.config/switchboard/config.toml) — starting claude without it\n"
)

// naming returns r, its status document naming accounts, in their order, the
// one whose id is primary the primary, none where it's "".
func (r *fakeRouter) naming(primary string, accounts ...config.Account) *fakeRouter {
	r.doc = status.Document{Primary: primary}
	for _, a := range accounts {
		r.doc.Accounts = append(r.doc.Accounts, status.Account{ID: a.ID, Label: a.Label, Primary: a.ID == primary})
	}
	return r
}

// unreadRoute is a launch through r where the config can't be read, pinned to
// account unless it's "", the accounts being those r's status document names.
func unreadRoute(r *fakeRouter, account string) launch.Route {
	return launch.Route{
		Token:    testTokens.Read,
		Pause:    func(time.Duration) {},
		Router:   r,
		Document: r.Status,
		Patience: launch.Patience{Answer: launch.AskTimeout},
		Account:  account,
	}
}

func TestRunUnreadRoutesThroughTheRunningRouter(t *testing.T) {
	tests := []struct {
		name        string
		account     string
		wantHeaders string
	}{
		{name: "unpinned", wantHeaders: "X-Trace: on\nX-Switchboard-Dir: ~/Code/project"},
		{name: "pinned", account: "work", wantHeaders: "X-Trace: on\nX-Switchboard-Account: work\nX-Switchboard-Dir: ~/Code/project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := t.TempDir()
			h := newHarness(t, "HOME=/home/tester", "PATH="+path, "ANTHROPIC_CUSTOM_HEADERS=X-Trace: on")
			h.launcher.Dir = project
			args := []string{"--print", "a prompt"}

			if err := h.launcher.RunUnread(t.Context(), unreadRoute(healthy().naming("side", accounts...), tt.account), args, errUnread); err != nil {
				t.Fatalf("RunUnread() error = %v", err)
			}

			got := h.only(t)
			if want := append([]string{"claude"}, args...); got.path != h.claude || !slices.Equal(got.argv, want) {
				t.Errorf("started %s as %q, want %s as %q", got.path, got.argv, h.claude, want)
			}
			// The base URL is where the router says it listens, there being no
			// config to say otherwise; the token the primary's, as the router's
			// document names it.
			want := map[string]string{
				"HOME":                             "/home/tester",
				"PATH":                             path,
				"ANTHROPIC_BASE_URL":               "http://" + proxyAddr,
				"CLAUDE_CODE_OAUTH_TOKEN":          sideToken,
				"ANTHROPIC_CUSTOM_HEADERS":         tt.wantHeaders,
				"CLAUDE_CODE_GATEWAY_HINT_HEADERS": "1",
			}
			if env := h.environment(t); !maps.Equal(env, want) {
				t.Errorf("started with the environment\n%q\nwant\n%q", env, want)
			}
			if len(got.sets) != 1 {
				t.Errorf("started marked with what run set %q, want one mark", got.sets)
			}
			if said := h.stderr.String(); said != throughRunning {
				t.Errorf("said %q on stderr, want %q", said, throughRunning)
			}
		})
	}
}

func TestRunUnreadRoutesThroughAnUnhealthyRouter(t *testing.T) {
	h := newHarness(t, "HOME=/home/tester")
	h.launcher.Dir = project

	if err := h.launcher.RunUnread(t.Context(), unreadRoute(unhealthyRouter().naming("side", accounts...), ""), nil, errUnread); err != nil {
		t.Fatalf("RunUnread() error = %v", err)
	}

	want := map[string]string{
		"HOME":                             "/home/tester",
		"ANTHROPIC_BASE_URL":               "http://" + proxyAddr,
		"CLAUDE_CODE_OAUTH_TOKEN":          sideToken,
		"ANTHROPIC_CUSTOM_HEADERS":         "X-Switchboard-Dir: ~/Code/project",
		"CLAUDE_CODE_GATEWAY_HINT_HEADERS": "1",
	}
	if env := h.environment(t); !maps.Equal(env, want) {
		t.Errorf("started with the environment\n%q\nwant\n%q, routed as through a healthy router", env, want)
	}
	wantSaid := throughRunning + "switchboard: the router is unhealthy (" + unhealthy + ") — starting claude through it all the same\n"
	if said := h.stderr.String(); said != wantSaid {
		t.Errorf("said %q on stderr, want %q", said, wantSaid)
	}
}

func TestRunUnreadChoosesTheTokenAsTheRoutersDocumentSays(t *testing.T) {
	reordered := []config.Account{accounts[2], accounts[1], accounts[0]}
	tests := []struct {
		name string
		// primary is the account the router's document names the primary, of
		// accounts, in its order; account the one the launch is pinned to.
		primary  string
		accounts []config.Account
		account  string
		want     string
	}{
		{name: "the primary's", primary: "side", accounts: accounts, want: sideToken},
		{name: "pinned, the primary's still", primary: "side", accounts: accounts, account: "work", want: sideToken},
		{name: "the first with a token, when the primary has none", primary: "personal", accounts: accounts, want: workToken},
		{name: "the first with a token in the document's order", primary: "personal", accounts: reordered, want: sideToken},
		{name: "pinned, the pinned account's, when the primary has no token", primary: "personal", accounts: accounts, account: "side", want: sideToken},
		{name: "the first, where the document names no primary", accounts: reordered, want: sideToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)

			if err := h.launcher.RunUnread(t.Context(), unreadRoute(healthy().naming(tt.primary, tt.accounts...), tt.account), nil, errUnread); err != nil {
				t.Fatalf("RunUnread() error = %v", err)
			}
			if got := h.environment(t)["CLAUDE_CODE_OAUTH_TOKEN"]; got != tt.want {
				t.Errorf("started on %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunUnreadStartsASessionAsRunDoes(t *testing.T) {
	own := []string{"HOME=" + homeDir, "ANTHROPIC_CUSTOM_HEADERS=X-Trace: on", "ANTHROPIC_BASE_URL=https://gateway.example.com"}
	routed := newHarness(t, own...)
	routed.launcher.Dir = project
	if err := routed.launcher.Run(t.Context(), route(healthy(), "side"), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	tests := []struct {
		name    string
		environ []string
	}{
		{name: "afresh", environ: own},
		{name: "within a routed session", environ: routedSession(t, own, "work")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.environ...)
			h.launcher.Dir = project

			if err := h.launcher.RunUnread(t.Context(), unreadRoute(healthy().naming("work", accounts...), "side"), nil, errUnread); err != nil {
				t.Fatalf("RunUnread() error = %v", err)
			}
			if got, want := envMap(h.only(t).session()), envMap(routed.only(t).session()); !maps.Equal(got, want) {
				t.Errorf("started a session with the environment\n%q\nwant\n%q, as Run starts it from the user's own, marked alike", got, want)
			}
		})
	}
}

func TestRunUnreadStartsWithoutSwitchboardWhereNoAccountTheRouterNamesCanBeStartedOn(t *testing.T) {
	tests := []struct {
		name   string
		router *fakeRouter
		// tokens are the token files there are, testTokens unless given.
		tokens  tokenstest.Files
		wantLog []string
	}{
		{
			name:    "no usable token",
			router:  healthy().naming("side", accounts...),
			tokens:  tokenstest.Files{"personal": " "},
			wantLog: []string{"level=WARN", `msg="no account the router routes for has a usable token"`, `error="work: ` + tokenstest.Missing("work").Error()},
		},
		{
			name:    "the router's document unread",
			router:  &fakeRouter{health: healthy().health, docErr: errors.New("the router answered GET /status with 500 Internal Server Error")},
			wantLog: []string{"level=WARN", `msg="can't read which accounts the router routes for"`, `error="the router answered GET /status with 500 Internal Server Error"`},
		},
		{
			name:    "the router's document naming no account",
			router:  healthy().naming(""),
			wantLog: []string{"level=WARN", `msg="can't read which accounts the router routes for"`, `error="the router's status document names no account"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inherited := []string{
				"HOME=/home/tester",
				"CLAUDE_CODE_OAUTH_TOKEN=test-token-stale",
				"ANTHROPIC_CUSTOM_HEADERS=X-Switchboard-Account: side",
			}
			log := logstest.Capture(t)
			h := newHarness(t, inherited...)
			r := unreadRoute(tt.router, "")
			if tt.tokens != nil {
				r.Token = tt.tokens.Read
			}

			if err := h.launcher.RunUnread(t.Context(), r, []string{"--resume"}, errUnread); err != nil {
				t.Fatalf("RunUnread() error = %v", err)
			}

			got := h.only(t)
			if want := []string{"claude", "--resume"}; !slices.Equal(got.argv, want) || !slices.Equal(got.env, inherited) {
				t.Errorf("started claude as %q with %q, want %q with the environment untouched, %q", got.argv, got.env, want, inherited)
			}
			if said := h.stderr.String(); said != unreadUnaided {
				t.Errorf("said %q on stderr, want %q", said, unreadUnaided)
			}
			if !log.Has(tt.wantLog...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, tt.wantLog)
			}
		})
	}
}

func TestRunUnreadStartsWithoutSwitchboardWhereTheRouterCantBeRoutedThrough(t *testing.T) {
	tests := []struct {
		name   string
		router *fakeRouter
		// wantState is how the log names the router's state.
		wantState string
	}{
		{name: "not running", router: notRunning(), wantState: `router="not running"`},
		{name: "not answering", router: notAnswering(), wantState: `router="not answering"`},
		{name: "answering without saying where it listens", router: unlistening(), wantState: "router=unusable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inherited := []string{"HOME=/home/tester", "CLAUDE_CODE_OAUTH_TOKEN=test-token-stale"}
			log := logstest.Capture(t)
			h := newHarness(t, inherited...)

			if err := h.launcher.RunUnread(t.Context(), unreadRoute(tt.router.naming("side", accounts...), "side"), []string{"--resume"}, errUnread); err != nil {
				t.Fatalf("RunUnread() error = %v", err)
			}

			got := h.only(t)
			if want := []string{"claude", "--resume"}; !slices.Equal(got.argv, want) || !slices.Equal(got.env, inherited) {
				t.Errorf("started claude as %q with %q, want %q with the environment untouched, %q", got.argv, got.env, want, inherited)
			}
			if said := h.stderr.String(); said != unreadUnaided {
				t.Errorf("said %q on stderr, want %q", said, unreadUnaided)
			}
			if want := []string{"level=INFO", `msg="the router can't be routed through"`, tt.wantState, "asked="}; !log.Has(want...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, want)
			}
		})
	}
}

func TestRunUnreadRefusesAPinItCantKeep(t *testing.T) {
	tests := []struct {
		name    string
		account string
		wantErr string
	}{
		{name: "one the router doesn't name", account: "nope", wantErr: `there's no account "nope": pin work or side`},
		{
			name:    "without a usable token",
			account: "personal",
			wantErr: "account personal has no usable token for Claude Code to start on: " + tokenstest.Missing("personal").Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)

			err := h.launcher.RunUnread(t.Context(), unreadRoute(healthy().naming("side", accounts...), tt.account), nil, errUnread)
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("RunUnread() error = %v, want %q", err, tt.wantErr)
			}
			if len(h.starts) > 0 {
				t.Errorf("started %q, want nothing started", h.starts)
			}
		})
	}
}
