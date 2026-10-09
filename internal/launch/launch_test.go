package launch_test

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/tokens"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

const (
	workToken = "test-token-work"
	sideToken = "test-token-side"
	// pid is the launcher's process id, and sessionPID that of a switchboard
	// that started a Claude Code session, which a claude started within it
	// inherits the mark of.
	pid        = 5150
	sessionPID = 4141
	// proxyAddr is where a healthy router says its proxy listens, which isn't
	// where the config says: the router's word is the one that counts.
	proxyAddr = "127.0.0.1:4848"
	// unhealthy is why an unhealthy router says it is.
	unhealthy = "5 of the 8 requests in the last 5 minutes failed"
	// homeDir is the user's home directory, and project a directory in it
	// that Claude Code starts in.
	homeDir = "/home/tester"
	project = homeDir + "/Code/project"
)

// launched is when, by the wall clock, the launcher starts claude.
var launched = time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)

// mark is the mark of a claude the switchboard whose process id is id
// started at, at path.
func mark(id int, at time.Time, path string) string {
	return strconv.Itoa(id) + ":" + strconv.FormatInt(at.Unix(), 10) + ":" + path
}

// accounts are the configured accounts: work and side have tokens, personal
// doesn't.
var accounts = []config.Account{
	{ID: "work", Label: "Work"},
	{ID: "personal", Label: "Personal"},
	{ID: "side", Label: "Side"},
}

var testTokens = tokenstest.Files{"work": workToken, "side": sideToken}

// route is a launch through r, pinned to account unless it's "".
func route(r launch.Router, account string) launch.Route {
	return launch.Route{
		Config:  &config.Config{Listen: "127.0.0.1:4747", Accounts: accounts},
		Token:   testTokens.Read,
		Pause:   func(time.Duration) {},
		Router:  r,
		Account: account,
	}
}

func TestRunThroughAHealthyRouter(t *testing.T) {
	path := t.TempDir()
	h := newHarness(t, "HOME=/home/tester", "PATH="+path, "ANTHROPIC_CUSTOM_HEADERS=X-Trace: on")
	h.launcher.Dir = project
	args := []string{"--print", "a prompt", "--account", "work", "", "--", "--direct"}

	if err := h.launcher.Run(t.Context(), route(healthy(), "side"), args); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := h.only(t)
	if want := append([]string{"claude"}, args...); got.path != h.claude || !slices.Equal(got.argv, want) {
		t.Errorf("started %s as %q, want %s as %q", got.path, got.argv, h.claude, want)
	}
	want := map[string]string{
		"HOME":                             "/home/tester",
		"PATH":                             path,
		"ANTHROPIC_BASE_URL":               "http://" + proxyAddr,
		"CLAUDE_CODE_OAUTH_TOKEN":          workToken,
		"ANTHROPIC_CUSTOM_HEADERS":         "X-Trace: on\nX-Switchboard-Account: side\nX-Switchboard-Dir: ~/Code/project",
		"CLAUDE_CODE_GATEWAY_HINT_HEADERS": "1",
	}
	if env := h.environment(t); !maps.Equal(env, want) {
		t.Errorf("started with the environment\n%q\nwant\n%q", env, want)
	}
	if said := h.stderr.String(); said != "" {
		t.Errorf("said %q on stderr, want nothing", said)
	}
}

func TestRunSendsClaudeCodeWhereTheRouterListens(t *testing.T) {
	tests := []struct {
		listen string
		want   string
	}{
		{listen: "127.0.0.1:4848", want: "http://127.0.0.1:4848"},
		{listen: "[::1]:4747", want: "http://[::1]:4747"},
	}
	for _, tt := range tests {
		t.Run(tt.listen, func(t *testing.T) {
			h := newHarness(t)
			r := &fakeRouter{health: router.Health{OK: true, Listen: tt.listen, PID: 4242}}

			if err := h.launcher.Run(t.Context(), route(r, ""), nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got := h.environment(t)["ANTHROPIC_BASE_URL"]; got != tt.want {
				t.Errorf("started with ANTHROPIC_BASE_URL %q, want %q, where the router listens, not %q, where the config says", got, tt.want, "127.0.0.1:4747")
			}
		})
	}
}

func TestRunChoosesTheToken(t *testing.T) {
	tests := []struct {
		name   string
		router *fakeRouter
		// primary is the account marked primary, the first when it's "".
		primary string
		account string
		want    string
	}{
		{name: "routed, the primary's, the first when none is marked", router: healthy(), want: workToken},
		{name: "routed, the primary's", router: healthy(), primary: "side", want: sideToken},
		{name: "routed and pinned, the primary's still", router: healthy(), account: "side", want: workToken},
		{name: "routed and pinned elsewhere, the primary's still", router: healthy(), primary: "side", account: "work", want: sideToken},
		{name: "routed, the first with a token, when the primary has none", router: healthy(), primary: "personal", want: workToken},
		{name: "routed and pinned, the pinned account's, when the primary has no token", router: healthy(), primary: "personal", account: "side", want: sideToken},
		{name: "direct, the primary's", router: notRunning(), primary: "side", want: sideToken},
		{name: "direct and pinned, the pinned account's", router: notRunning(), account: "side", want: sideToken},
		{name: "direct, the first with a token, when the primary has none", router: notRunning(), primary: "personal", want: workToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			r := route(tt.router, tt.account)
			r.Config.Accounts = primaryOf(tt.primary)

			if err := h.launcher.Run(t.Context(), r, nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got := h.environment(t)["CLAUDE_CODE_OAUTH_TOKEN"]; got != tt.want {
				t.Errorf("started on %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunLooksAgainAtATokenFileCaughtEmpty(t *testing.T) {
	exposed := func(id string) error {
		return fmt.Errorf("other users can read the token file (mode 0644): chmod 600 tokens/%s", id)
	}
	tests := []struct {
		name    string
		router  *fakeRouter
		account string
		// rewritten is the account whose token file the first look finds as
		// caught says, and written whether it holds the token at a second.
		rewritten string
		caught    func(id string) error
		written   bool
		want      string
		wantErr   string
		// wantPaused is whether the launch paused, 200ms, to look again.
		wantPaused bool
	}{
		{name: "the primary's, at the first look", router: healthy(), want: workToken},
		{name: "the primary's, written again", router: healthy(), rewritten: "work", caught: tokenstest.Empty, written: true, want: workToken, wantPaused: true},
		{name: "the primary's, empty still, the first with a token's", router: healthy(), rewritten: "work", caught: tokenstest.Empty, want: sideToken, wantPaused: true},
		{
			name:       "the primary's, written again, pinned to another",
			router:     healthy(),
			account:    "side",
			rewritten:  "work",
			caught:     tokenstest.Empty,
			written:    true,
			want:       workToken,
			wantPaused: true,
		},
		{
			name:       "the primary's, empty still, pinned to another, the pinned account's",
			router:     healthy(),
			account:    "side",
			rewritten:  "work",
			caught:     tokenstest.Empty,
			want:       sideToken,
			wantPaused: true,
		},
		{name: "the primary's missing, looked at once", router: healthy(), rewritten: "work", caught: tokenstest.Missing, written: true, want: sideToken},
		{name: "the primary's readable by others, looked at once", router: healthy(), rewritten: "work", caught: exposed, written: true, want: sideToken},
		{
			name:       "the pinned account's, written again",
			router:     notRunning(),
			account:    "side",
			rewritten:  "side",
			caught:     tokenstest.Empty,
			written:    true,
			want:       sideToken,
			wantPaused: true,
		},
		{
			name:       "the pinned account's, empty still",
			router:     notRunning(),
			account:    "side",
			rewritten:  "side",
			caught:     tokenstest.Empty,
			wantErr:    "account side has no usable token for Claude Code to start on: token missing: write it to tokens/side, which is empty",
			wantPaused: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			r := route(tt.router, tt.account)
			var paused []time.Duration
			r.Token = func(id string) (tokens.Token, error) {
				if id == tt.rewritten && (len(paused) == 0 || !tt.written) {
					return tokens.Token{}, tt.caught(id)
				}
				return testTokens.Read(id)
			}
			r.Pause = func(d time.Duration) { paused = append(paused, d) }

			err := h.launcher.Run(t.Context(), r, nil)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr || len(h.starts) > 0 {
					t.Errorf("Run() error = %v, starting %q; want %q, starting nothing", err, h.starts, tt.wantErr)
				}
			} else if got := h.environment(t)["CLAUDE_CODE_OAUTH_TOKEN"]; err != nil || got != tt.want {
				t.Errorf("Run() error = %v, starting on %q; want %q", err, got, tt.want)
			}
			var want []time.Duration
			if tt.wantPaused {
				want = []time.Duration{200 * time.Millisecond}
			}
			if !slices.Equal(paused, want) {
				t.Errorf("paused %v, want %v", paused, want)
			}
		})
	}
}

// primaryOf is the accounts with the one whose id is given marked primary, or
// none marked when it's "".
func primaryOf(id string) config.Accounts {
	marked := slices.Clone(accounts)
	for i := range marked {
		marked[i].Primary = marked[i].ID == id
	}
	return marked
}

func TestRunWithoutAHealthyRouter(t *testing.T) {
	inherited := []string{
		"HOME=/home/tester",
		"ANTHROPIC_BASE_URL=http://127.0.0.1:4747",
		"CLAUDE_CODE_OAUTH_TOKEN=test-token-stale",
		"ANTHROPIC_CUSTOM_HEADERS=X-Trace: on\nX-Switchboard-Account: side\nX-Switchboard-Dir: ~/Code/elsewhere",
	}
	tests := []struct {
		name      string
		router    *fakeRouter
		account   string
		wantSaid  string
		wantToken string
	}{
		{
			name:      "not running",
			router:    notRunning(),
			wantSaid:  "switchboard: the router isn't running — connecting directly on work · Work\n",
			wantToken: workToken,
		},
		{
			name:      "unhealthy",
			router:    &fakeRouter{health: router.Health{OK: false, Reason: unhealthy, PID: 4242}},
			wantSaid:  "switchboard: the router is unhealthy (" + unhealthy + ") — connecting directly on work · Work\n",
			wantToken: workToken,
		},
		{
			name:      "answering as no router does",
			router:    &fakeRouter{healthErr: errors.New("the router answered GET /health with 404 Not Found")},
			wantSaid:  "switchboard: the router is unhealthy (the router answered GET /health with 404 Not Found) — connecting directly on work · Work\n",
			wantToken: workToken,
		},
		{
			name:      "not saying where it listens",
			router:    &fakeRouter{health: router.Health{OK: true, PID: 4242}},
			wantSaid:  "switchboard: the router is unhealthy (it doesn't say where it listens) — connecting directly on work · Work\n",
			wantToken: workToken,
		},
		{
			name:      "not running, pinned",
			router:    notRunning(),
			account:   "side",
			wantSaid:  "switchboard: the router isn't running — connecting directly on side · Side\n",
			wantToken: sideToken,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, inherited...)
			h.launcher.Dir = project

			if err := h.launcher.Run(t.Context(), route(tt.router, tt.account), []string{"--resume"}); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if said := h.stderr.String(); said != tt.wantSaid {
				t.Errorf("said %q on stderr, want %q", said, tt.wantSaid)
			}
			want := map[string]string{
				"HOME":                     "/home/tester",
				"CLAUDE_CODE_OAUTH_TOKEN":  tt.wantToken,
				"ANTHROPIC_CUSTOM_HEADERS": "X-Trace: on",
			}
			if env := h.environment(t); !maps.Equal(env, want) {
				t.Errorf("started with the environment\n%q\nwant\n%q", env, want)
			}
			if argv := h.only(t).argv; !slices.Equal(argv, []string{"claude", "--resume"}) {
				t.Errorf("started claude as %q, want %q", argv, []string{"claude", "--resume"})
			}
		})
	}
}

func TestRunGivesTheRouterHalfASecondToAnswer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		began := time.Now()

		if err := h.launcher.Run(t.Context(), route(&fakeRouter{hangs: true}, ""), nil); err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if waited := time.Since(began); waited != 500*time.Millisecond {
			t.Errorf("waited %v for the router, want 500ms", waited)
		}
		want := "switchboard: the router is unhealthy (no answer within 500ms) — connecting directly on work · Work\n"
		if said := h.stderr.String(); said != want {
			t.Errorf("said %q on stderr, want %q", said, want)
		}
		if _, set := h.environment(t)["ANTHROPIC_BASE_URL"]; set {
			t.Error("started with ANTHROPIC_BASE_URL set, want it connecting directly")
		}
	})
}

func TestRunKeepsTheOtherCustomHeaders(t *testing.T) {
	tests := []struct {
		name string
		// headers are those the environment gives Claude Code, "" for none.
		headers string
		account string
		// dir is the directory Claude Code starts in, "" where it isn't known.
		dir string
		// want are those Claude Code starts with, "" for none.
		want string
	}{
		{name: "none, unpinned"},
		{name: "none, pinned", account: "side", want: "X-Switchboard-Account: side"},
		{name: "others, unpinned", headers: "X-Trace: on\nX-Team: core", want: "X-Trace: on\nX-Team: core"},
		{name: "others, pinned", headers: "X-Trace: on\nX-Team: core", account: "side", want: "X-Trace: on\nX-Team: core\nX-Switchboard-Account: side"},
		{
			name:    "others, pinned, in a directory",
			headers: "X-Trace: on\nX-Team: core",
			account: "side",
			dir:     project,
			want:    "X-Trace: on\nX-Team: core\nX-Switchboard-Account: side\nX-Switchboard-Dir: ~/Code/project",
		},
		{name: "a pin inherited, unpinned", headers: "X-Switchboard-Account: work"},
		{name: "a pin inherited among others, pinned", headers: "x-switchboard-account : work\r\nX-Trace: on\n\n", account: "side", want: "X-Trace: on\nX-Switchboard-Account: side"},
		{name: "a directory inherited, in none known", headers: "X-Switchboard-Dir: ~/Code/elsewhere"},
		{
			name:    "a directory inherited among others, in another",
			headers: "x-switchboard-dir : ~/Code/elsewhere\r\nX-Trace: on\n\n",
			dir:     project,
			want:    "X-Trace: on\nX-Switchboard-Dir: ~/Code/project",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var environ []string
			if tt.headers != "" {
				environ = append(environ, "ANTHROPIC_CUSTOM_HEADERS="+tt.headers)
			}
			h := newHarness(t, environ...)
			h.launcher.Dir = tt.dir

			if err := h.launcher.Run(t.Context(), route(healthy(), tt.account), nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			got, set := h.environment(t)["ANTHROPIC_CUSTOM_HEADERS"]
			if got != tt.want || set != (tt.want != "") {
				t.Errorf("started with ANTHROPIC_CUSTOM_HEADERS %q (set: %v), want %q", got, set, tt.want)
			}
		})
	}
}

func TestRunTellsTheRouterTheDirectoryClaudeCodeStartsIn(t *testing.T) {
	tests := []struct {
		name string
		// dir is the directory Claude Code starts in, and home the user's home
		// directory, each "" where it isn't known.
		dir, home string
		// want is the directory the router reads, "" for none, and wantSent
		// the header's value, where it isn't want.
		want, wantSent string
	}{
		{name: "within home, from there", dir: project, home: homeDir, want: "~/Code/project"},
		{name: "home itself", dir: homeDir, home: homeDir, want: "~"},
		{name: "within a home given with a slash at its end", dir: project, home: homeDir + "/", want: "~/Code/project"},
		{name: "outside home, as it is", dir: "/srv/project", home: homeDir, want: "/srv/project"},
		{name: "beside home, its name starting as home's does, as it is", dir: homeDir + "-old/project", home: homeDir, want: homeDir + "-old/project"},
		{name: "without a home directory, as it is", dir: project, want: project},
		{name: "without a working directory, none", home: homeDir},
		{name: "without either, none"},
		{name: "with a space", dir: homeDir + "/Code/my project", home: homeDir, want: "~/Code/my project"},
		{name: "ending in a space, which a header would lose", dir: project + " ", home: homeDir, want: "~/Code/project ", wantSent: "~/Code/project%20"},
		{
			name:     "with a newline, a percent sign and characters past ASCII",
			dir:      homeDir + "/Code/café\n100% ☃",
			home:     homeDir,
			want:     "~/Code/café\n100% ☃",
			wantSent: "~/Code/caf%C3%A9%0A100%25 %E2%98%83",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.launcher.Dir, h.launcher.Home = tt.dir, tt.home

			if err := h.launcher.Run(t.Context(), route(healthy(), ""), nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			headers, set := h.environment(t)["ANTHROPIC_CUSTOM_HEADERS"]
			var want string
			if tt.want != "" {
				want = "X-Switchboard-Dir: " + cmp.Or(tt.wantSent, tt.want)
			}
			if headers != want || set != (want != "") {
				t.Errorf("started with ANTHROPIC_CUSTOM_HEADERS %q (set: %v), want %q", headers, set, want)
			}
			sent := strings.TrimPrefix(headers, "X-Switchboard-Dir: ")
			if got := router.DecodeDir(sent); got != tt.want {
				t.Errorf("the router reads the directory sent as %q as %q, want %q", sent, got, tt.want)
			}
		})
	}
}

func TestRunTellsARouterFromBeforeNoDirectory(t *testing.T) {
	// The health of a router from before it took every X-Switchboard- header
	// off what it sends upstream, as it answers: it took off the pin alone.
	var older router.Health
	if err := json.Unmarshal([]byte(`{"ok": true, "listen": "`+proxyAddr+`", "version": "0.1.0", "pid": 4242}`), &older); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, "ANTHROPIC_CUSTOM_HEADERS=X-Trace: on\nX-Switchboard-Dir: ~/Code/elsewhere")
	h.launcher.Dir = project

	if err := h.launcher.Run(t.Context(), route(&fakeRouter{health: older}, "side"), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := map[string]string{
		"ANTHROPIC_BASE_URL":               "http://" + proxyAddr,
		"CLAUDE_CODE_OAUTH_TOKEN":          workToken,
		"ANTHROPIC_CUSTOM_HEADERS":         "X-Trace: on\nX-Switchboard-Account: side",
		"CLAUDE_CODE_GATEWAY_HINT_HEADERS": "1",
	}
	if env := h.environment(t); !maps.Equal(env, want) {
		t.Errorf("started with the environment\n%q\nwant\n%q: routed and pinned, the router told no directory, which it would send on", env, want)
	}
}

func TestRunHasClaudeCodeTellTheRouterWhatItTellsTheAPIOfEachRequest(t *testing.T) {
	const hints = "CLAUDE_CODE_GATEWAY_HINT_HEADERS"
	tests := []struct {
		name   string
		router *fakeRouter
		// environ is what Claude Code starts from, and want the setting it
		// starts with, wantSet whether it's set at all.
		environ []string
		want    string
		wantSet bool
	}{
		{name: "routed, sent", router: healthy(), want: "1", wantSet: true},
		{name: "routed, turned off already, kept off", router: healthy(), environ: []string{hints + "=0"}, want: "0", wantSet: true},
		{name: "routed, turned on already, kept", router: healthy(), environ: []string{hints + "=1"}, want: "1", wantSet: true},
		{name: "routed, set to nothing already, kept so", router: healthy(), environ: []string{hints + "="}, wantSet: true},
		{name: "direct, left unset, as Claude Code sends the API them anyway", router: notRunning()},
		{name: "direct, turned off already, kept off", router: notRunning(), environ: []string{hints + "=0"}, want: "0", wantSet: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.environ...)

			if err := h.launcher.Run(t.Context(), route(tt.router, ""), nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got, set := h.environment(t)[hints]; got != tt.want || set != tt.wantSet {
				t.Errorf("started with %s %q (set: %v), want %q (set: %v)", hints, got, set, tt.want, tt.wantSet)
			}
		})
	}
}

func TestRunRefusesAPinItCantKeep(t *testing.T) {
	tests := []struct {
		name    string
		account string
		// tokens are the token files there are, testTokens unless given.
		tokens  tokenstest.Files
		wantErr string
	}{
		{name: "not configured, naming those with a usable token", account: "nope", wantErr: `there's no account "nope": pin work or side`},
		{
			name:    "not configured, while no account has a usable token",
			account: "nope",
			tokens:  tokenstest.Files{"personal": " "},
			wantErr: `there's no account "nope", and no account has a usable token to pin`,
		},
		{
			name:    "a token given as the account, never quoted",
			account: "sk-ant-oat01-fake_token-shaped",
			wantErr: `there's no account "[redacted]": pin work or side`,
		},
		{
			name:    "without a usable token",
			account: "personal",
			wantErr: "account personal has no usable token for Claude Code to start on: " + tokenstest.Missing("personal").Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			r := route(healthy(), tt.account)
			if tt.tokens != nil {
				r.Token = tt.tokens.Read
			}

			if err := h.launcher.Run(t.Context(), r, nil); err == nil || err.Error() != tt.wantErr {
				t.Errorf("Run() error = %v, want %q", err, tt.wantErr)
			}
			if len(h.starts) > 0 {
				t.Errorf("started %q, want nothing started", h.starts)
			}
		})
	}
}

func TestRunWithoutATokenStartsClaudeAsIfSwitchboardWerentThere(t *testing.T) {
	inherited := []string{
		"HOME=/home/tester",
		"CLAUDE_CODE_OAUTH_TOKEN=test-token-stale",
		"ANTHROPIC_CUSTOM_HEADERS=X-Switchboard-Account: side",
	}
	log := logstest.Capture(t)
	h := newHarness(t, inherited...)
	r := route(healthy(), "")
	r.Token = tokenstest.Files{"personal": " "}.Read

	if err := h.launcher.Run(t.Context(), r, []string{"--print", "a prompt"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := h.only(t)
	if want := []string{"claude", "--print", "a prompt"}; !slices.Equal(got.argv, want) || !slices.Equal(got.env, inherited) {
		t.Errorf("started claude as %q with %q, want %q with the environment untouched, %q", got.argv, got.env, want, inherited)
	}
	want := "switchboard: no account has a usable token (work: " + tokenstest.Missing("work").Error() + ") — starting claude without it\n"
	if said := h.stderr.String(); said != want {
		t.Errorf("said %q on stderr, want %q", said, want)
	}
	wantLog := []string{
		"level=WARN", `msg="starting claude without switchboard"`, `reason="no account has a usable token"`,
		`error="work: ` + tokenstest.Missing("work").Error() + `\npersonal: ` + tokenstest.Empty("personal").Error() + `\nside: ` + tokenstest.Missing("side").Error() + `"`,
	}
	if !log.Has(wantLog...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, wantLog)
	}
}

func TestUnaidedStartsClaudeAsIfSwitchboardWerentThere(t *testing.T) {
	const config = "/home/tester/.config/switchboard/config.toml"
	tests := []struct {
		name     string
		err      error
		wantSaid string
	}{
		{
			name:     "without a config",
			err:      errors.New("no config file at " + config + "\n\nCreate one like this:\n\n[[account]]\nid = \"work\"\n"),
			wantSaid: "switchboard: couldn't read the config (no config file at " + config + ") — starting claude without it\n",
		},
		{
			name:     "with an invalid config",
			err:      errors.New("invalid config " + config + ":\nunknown key \"listn\""),
			wantSaid: "switchboard: couldn't read the config (invalid config " + config + ") — starting claude without it\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inherited := []string{"HOME=/home/tester", "ANTHROPIC_BASE_URL=http://127.0.0.1:4747", "CLAUDE_CODE_OAUTH_TOKEN=test-token-work"}
			h := newHarness(t, inherited...)
			args := []string{"--account", "side", "--resume"}

			if err := h.launcher.Unaided(args, "couldn't read the config", tt.err); err != nil {
				t.Fatalf("Unaided() error = %v", err)
			}

			got := h.only(t)
			if want := append([]string{"claude"}, args...); got.path != h.claude || !slices.Equal(got.argv, want) || !slices.Equal(got.env, inherited) {
				t.Errorf("started %s as %q with %q, want %s as %q with the environment untouched, %q", got.path, got.argv, got.env, h.claude, want, inherited)
			}
			if said := h.stderr.String(); said != tt.wantSaid {
				t.Errorf("said %q on stderr, want %q", said, tt.wantSaid)
			}
		})
	}
}

func TestUnaidedWithoutClaude(t *testing.T) {
	h := newHarness(t)
	h.launcher.InstallPaths = []string{filepath.Join(t.TempDir(), "claude")}

	err := h.launcher.Unaided(nil, "couldn't read the config", errors.New("no config file at /home/tester/config.toml"))
	if err == nil || !strings.HasPrefix(err.Error(), "can't find claude") || len(h.starts) > 0 {
		t.Errorf("Unaided() error = %v, starting %q; want claude not found, and nothing started", err, h.starts)
	}
}

func TestDirectStartsClaudeCodeOnItsOwnLogin(t *testing.T) {
	h := newHarness(t,
		"HOME=/home/tester",
		"ANTHROPIC_BASE_URL=http://127.0.0.1:4747",
		"CLAUDE_CODE_OAUTH_TOKEN=test-token-stale",
		"ANTHROPIC_CUSTOM_HEADERS=X-Switchboard-Account: side\nX-Switchboard-Dir: ~/Code/elsewhere\nX-Trace: on",
	)
	h.launcher.Dir = project

	if err := h.launcher.Direct([]string{"--print", "a prompt"}); err != nil {
		t.Fatalf("Direct() error = %v", err)
	}

	got := h.only(t)
	if want := []string{"claude", "--print", "a prompt"}; got.path != h.claude || !slices.Equal(got.argv, want) {
		t.Errorf("started %s as %q, want %s as %q", got.path, got.argv, h.claude, want)
	}
	want := map[string]string{"HOME": "/home/tester", "ANTHROPIC_CUSTOM_HEADERS": "X-Trace: on"}
	if env := h.environment(t); !maps.Equal(env, want) {
		t.Errorf("started with the environment\n%q\nwant\n%q", env, want)
	}
}

func TestLocalStartsClaudeAsIfSwitchboardWerentThere(t *testing.T) {
	inherited := []string{
		"HOME=/home/tester",
		"ANTHROPIC_BASE_URL=http://127.0.0.1:4747",
		"CLAUDE_CODE_OAUTH_TOKEN=test-token-work",
		"ANTHROPIC_CUSTOM_HEADERS=X-Switchboard-Account: side",
	}
	h := newHarness(t, inherited...)
	args := []string{"mcp", "add", "--transport", "http", "docs", "https://docs.example.com/mcp"}

	if err := h.launcher.Local(args); err != nil {
		t.Fatalf("Local() error = %v", err)
	}

	got := h.only(t)
	if want := append([]string{"claude"}, args...); got.path != h.claude || !slices.Equal(got.argv, want) || !slices.Equal(got.env, inherited) {
		t.Errorf("started %s as %q with %q, want %s as %q with the environment untouched, %q", got.path, got.argv, got.env, h.claude, want, inherited)
	}
	if said := h.stderr.String(); said != "" {
		t.Errorf("said %q on stderr, want nothing", said)
	}
}

func TestKeyEnvIsTheOneClaudeCodeStartsWith(t *testing.T) {
	// This process's own environment sets a key, which counts for nothing:
	// the environment Claude Code starts with is the one that counts.
	t.Setenv("ANTHROPIC_API_KEY", "test-key-own")
	tests := []struct {
		name    string
		environ []string
		want    string
	}{
		{name: "none", environ: []string{"HOME=/home/tester", "CLAUDE_CODE_OAUTH_TOKEN=test-token-work"}},
		{name: "an API key", environ: []string{"HOME=/home/tester", "ANTHROPIC_API_KEY=test-key"}, want: "ANTHROPIC_API_KEY"},
		{name: "a token of Claude Code's own", environ: []string{"ANTHROPIC_AUTH_TOKEN=test-key"}, want: "ANTHROPIC_AUTH_TOKEN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (launch.Launcher{Environ: tt.environ}).KeyEnv(); got != tt.want {
				t.Errorf("KeyEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStepAsideStartsClaudeAsIfSwitchboardWerentThere(t *testing.T) {
	inherited := []string{
		"HOME=/home/tester",
		"ANTHROPIC_API_KEY=test-key",
		"ANTHROPIC_BASE_URL=http://127.0.0.1:4747",
		"CLAUDE_CODE_OAUTH_TOKEN=test-token-work",
		"ANTHROPIC_CUSTOM_HEADERS=X-Switchboard-Account: side",
	}
	h := newHarness(t, inherited...)
	args := []string{"--print", "a prompt"}

	if err := h.launcher.StepAside(args, "ANTHROPIC_API_KEY"); err != nil {
		t.Fatalf("StepAside() error = %v", err)
	}

	got := h.only(t)
	if want := append([]string{"claude"}, args...); got.path != h.claude || !slices.Equal(got.argv, want) || !slices.Equal(got.env, inherited) {
		t.Errorf("started %s as %q with %q, want %s as %q with the environment untouched, %q", got.path, got.argv, got.env, h.claude, want, inherited)
	}
	if said, want := h.stderr.String(), "switchboard: ANTHROPIC_API_KEY is set, so Claude Code uses it — starting claude without switchboard\n"; said != want {
		t.Errorf("said %q on stderr, want %q", said, want)
	}
}

func TestFindsClaude(t *testing.T) {
	// This process's own PATH leads to a claude of its own, which none of
	// these finds: the environment Claude Code starts with is the one that
	// counts.
	t.Setenv("PATH", filepath.Dir(claudetest.Program(t, filepath.Join(t.TempDir(), "claude"))))
	onPath := claudetest.Program(t, filepath.Join(t.TempDir(), "claude"))
	installed := claudetest.Program(t, filepath.Join(t.TempDir(), "claude"))
	missing := filepath.Join(t.TempDir(), "claude")
	tests := []struct {
		name string
		// path is PATH in the environment Claude Code starts with.
		path     string
		installs []string
		want     string
		wantErr  string
	}{
		{name: "on the PATH it starts with, before any install path", path: filepath.Dir(onPath), installs: []string{installed}, want: onPath},
		{name: "at the first install path holding a program, off that PATH", installs: []string{missing, installed}, want: installed},
		{name: "nowhere", path: filepath.Dir(missing), installs: []string{missing}, wantErr: "can't find claude: it isn't on PATH, nor at " + missing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, "PATH="+tt.path)
			h.launcher.InstallPaths = tt.installs

			err := h.launcher.Direct(nil)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr || len(h.starts) > 0 {
					t.Errorf("Direct() error = %v, starting %q; want %q, starting nothing", err, h.starts, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Direct() error = %v", err)
			}
			if got := h.only(t).path; got != tt.want {
				t.Errorf("started %s, want %s", got, tt.want)
			}
		})
	}
}

// everyLaunch starts claude each way a launcher can.
var everyLaunch = []struct {
	name   string
	launch func(ctx context.Context, l launch.Launcher) error
}{
	{name: "through the router", launch: func(ctx context.Context, l launch.Launcher) error { return l.Run(ctx, route(healthy(), ""), nil) }},
	{name: "directly", launch: func(ctx context.Context, l launch.Launcher) error { return l.Run(ctx, route(notRunning(), ""), nil) }},
	{name: "on its own login", launch: func(_ context.Context, l launch.Launcher) error { return l.Direct(nil) }},
	{
		name: "without switchboard",
		launch: func(_ context.Context, l launch.Launcher) error {
			return l.Unaided(nil, "couldn't read the config", errors.New("no config file at /home/tester/config.toml"))
		},
	},
	{name: "a local subcommand", launch: func(_ context.Context, l launch.Launcher) error { return l.Local([]string{"doctor"}) }},
	{name: "stepping aside for a key", launch: func(_ context.Context, l launch.Launcher) error { return l.StepAside(nil, "ANTHROPIC_API_KEY") }},
}

func TestNoLaunchStartsSwitchboardsClaudeLink(t *testing.T) {
	for _, tt := range everyLaunch {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			// switchboard's claude link, ahead of Claude Code on PATH.
			links := filepath.Dir(claudetest.Link(t, h.switchboard, filepath.Join(t.TempDir(), "claude")))
			h.launcher.Environ = []string{"PATH=" + links + string(filepath.ListSeparator) + filepath.Dir(h.claude)}

			if err := tt.launch(t.Context(), h.launcher); err != nil {
				t.Fatalf("launch error = %v", err)
			}
			if got := h.only(t).path; got != h.claude {
				t.Errorf("started %s, want %s, Claude Code past switchboard's claude link", got, h.claude)
			}
		})
	}
}

func TestAClaudeThatLeadsBackToSwitchboardStartsOnce(t *testing.T) {
	for _, tt := range everyLaunch {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			h := newHarness(t)
			// A wrapper named claude, ahead of Claude Code on PATH, that starts
			// switchboard in its own place, as exec does, keeping the process's
			// id and environment.
			wrapper := claudetest.Program(t, filepath.Join(t.TempDir(), "claude"))
			h.launcher.Environ = []string{"PATH=" + filepath.Dir(wrapper) + string(filepath.ListSeparator) + filepath.Dir(h.claude)}
			if err := tt.launch(t.Context(), h.launcher); err != nil {
				t.Fatalf("launch error = %v", err)
			}
			first := h.only(t)
			if first.path != wrapper || len(first.marks) != 1 {
				t.Fatalf("started %s marked %q, want %s, marked once", first.path, first.marks, wrapper)
			}
			h.starts = nil
			again := h.launcher
			again.Environ = append(first.env, markEnv+"="+first.marks[0])

			if err := tt.launch(t.Context(), again); err != nil {
				t.Fatalf("launch error = %v", err)
			}
			if got := h.only(t).path; got != h.claude {
				t.Errorf("started %s, want %s, Claude Code past the wrapper, rather than the wrapper again", got, h.claude)
			}
			if want := []string{"level=INFO", `msg="started again in place of the claude it started; looking past it"`, "claude=" + wrapper}; !log.Has(want...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, want)
			}
		})
	}
}

func TestNothingPastAClaudeThatLeadsBackToSwitchboard(t *testing.T) {
	h := newHarness(t)
	wrapper := claudetest.Program(t, filepath.Join(t.TempDir(), "claude"))
	missing := filepath.Join(t.TempDir(), "claude")
	h.launcher.InstallPaths = []string{missing}
	h.launcher.Environ = []string{"PATH=" + filepath.Dir(wrapper), markEnv + "=" + mark(pid, launched, wrapper)}

	err := h.launcher.Run(t.Context(), route(healthy(), ""), nil)
	want := "can't find claude past " + wrapper + ", which leads back to switchboard: it isn't on PATH, nor at " + missing
	if err == nil || err.Error() != want || len(h.starts) > 0 {
		t.Errorf("Run() error = %v, starting %q; want %q, starting nothing", err, h.starts, want)
	}
}

func TestEveryLaunchMarksTheClaudeItStarts(t *testing.T) {
	for _, tt := range everyLaunch {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			// Started within a Claude Code session another switchboard started,
			// whose mark it inherits.
			h.launcher.Environ = []string{"PATH=" + filepath.Dir(h.claude), markEnv + "=" + mark(sessionPID, launched, h.claude)}

			if err := tt.launch(t.Context(), h.launcher); err != nil {
				t.Fatalf("launch error = %v", err)
			}
			got := h.only(t)
			if got.path != h.claude {
				t.Errorf("started %s, want %s, the session's mark being another process's", got.path, h.claude)
			}
			if want := []string{mark(pid, launched, h.claude)}; !slices.Equal(got.marks, want) {
				t.Errorf("started claude marked %q, want %q alone: this process's id, when, and where the claude is", got.marks, want)
			}
		})
	}
}

func TestOnlyThisProcesssMarkLooksPastAClaude(t *testing.T) {
	h := newHarness(t)
	wrapper := claudetest.Program(t, filepath.Join(t.TempDir(), "claude"))
	path := "PATH=" + filepath.Dir(wrapper) + string(filepath.ListSeparator) + filepath.Dir(h.claude)
	tests := []struct {
		name string
		// mark is the mark the launch inherits, "" for none.
		mark string
		want string
	}{
		{name: "none", want: wrapper},
		{name: "this process's, at the wrapper", mark: mark(pid, launched, wrapper), want: h.claude},
		{name: "this process's, at the wrapper, as long ago as a mark counts", mark: mark(pid, launched.Add(-30*time.Second), wrapper), want: h.claude},
		{name: "this process's, at the wrapper, longer ago than a mark counts", mark: mark(pid, launched.Add(-31*time.Second), wrapper), want: wrapper},
		{name: "this process's, at the wrapper, from after now, the clock set back", mark: mark(pid, launched.Add(time.Hour), wrapper), want: wrapper},
		{name: "this process's, at the wrapper, without when", mark: strconv.Itoa(pid) + ":" + wrapper, want: wrapper},
		{name: "another process's, at the wrapper", mark: mark(sessionPID, launched, wrapper), want: wrapper},
		{name: "this process's, at a claude nowhere to look", mark: mark(pid, launched, filepath.Join(t.TempDir(), "claude")), want: wrapper},
		{name: "this process's, at no claude", mark: strconv.Itoa(pid) + ":" + strconv.FormatInt(launched.Unix(), 10), want: wrapper},
		{name: "unreadable", mark: "work", want: wrapper},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h.starts = nil
			h.launcher.Environ = []string{path}
			if tt.mark != "" {
				h.launcher.Environ = append(h.launcher.Environ, markEnv+"="+tt.mark)
			}

			if err := h.launcher.Run(t.Context(), route(healthy(), ""), nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got := h.only(t).path; got != tt.want {
				t.Errorf("started %s, want %s", got, tt.want)
			}
		})
	}
}

func TestAMarkReadsAClaudeWhosePathHoldsAColon(t *testing.T) {
	h := newHarness(t)
	// Where an installer put a wrapper named claude, ahead of Claude Code.
	wrapper := claudetest.Program(t, filepath.Join(t.TempDir(), "a:b", "claude"))
	h.launcher.InstallPaths = []string{wrapper, h.claude}
	h.launcher.Environ = []string{markEnv + "=" + mark(pid, launched, wrapper)}

	if err := h.launcher.Run(t.Context(), route(healthy(), ""), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := h.only(t).path; got != h.claude {
		t.Errorf("started %s, want %s, Claude Code past the wrapper", got, h.claude)
	}
}

func TestRunFailsWhenClaudeCantStart(t *testing.T) {
	failed := errors.New("permission denied")
	h := newHarness(t)
	h.launcher.Exec = func(string, []string, []string) error { return failed }

	err := h.launcher.Run(t.Context(), route(healthy(), ""), nil)
	if want := "start claude at " + h.claude + ": permission denied"; !errors.Is(err, failed) || err.Error() != want {
		t.Errorf("Run() error = %v, want %q", err, want)
	}
}

func TestLaunchesLogWhatWasDecided(t *testing.T) {
	const prompt = "a prompt the log must never hold"
	args := []string{"--print", prompt}
	tests := []struct {
		name   string
		launch func(ctx context.Context, l launch.Launcher) error
		want   []string
	}{
		{
			name: "through the router, pinned",
			launch: func(ctx context.Context, l launch.Launcher) error {
				return l.Run(ctx, route(healthy(), "side"), args)
			},
			want: []string{
				"level=INFO", `msg="starting claude" component=launch`, "mode=routed", "router=healthy",
				"account=work", `chosen="the primary"`, "pin=side",
			},
		},
		{
			name: "through the router",
			launch: func(ctx context.Context, l launch.Launcher) error {
				return l.Run(ctx, route(healthy(), ""), args)
			},
			want: []string{"level=INFO", "mode=routed", "router=healthy", "account=work", `chosen="the primary"`, `pin=""`},
		},
		{
			name:   "directly, the router not running",
			launch: func(ctx context.Context, l launch.Launcher) error { return l.Run(ctx, route(notRunning(), ""), args) },
			want:   []string{"level=WARN", `msg="starting claude" component=launch`, "mode=direct", `router="not running"`, "account=work", `chosen="the primary"`},
		},
		{
			name: "directly, pinned",
			launch: func(ctx context.Context, l launch.Launcher) error {
				return l.Run(ctx, route(notRunning(), "side"), args)
			},
			want: []string{"level=WARN", "mode=direct", `router="not running"`, "account=side", "chosen=pinned"},
		},
		{
			name: "directly, the primary without a token",
			launch: func(ctx context.Context, l launch.Launcher) error {
				r := route(notRunning(), "")
				r.Config.Accounts = primaryOf("personal")
				return l.Run(ctx, r, args)
			},
			want: []string{"level=WARN", "mode=direct", "account=work", `chosen="the first with a token"`},
		},
		{
			name: "directly, the router unhealthy",
			launch: func(ctx context.Context, l launch.Launcher) error {
				return l.Run(ctx, route(&fakeRouter{health: router.Health{Reason: unhealthy}}, ""), args)
			},
			want: []string{"level=WARN", "mode=direct", "router=unhealthy", `reason="` + unhealthy + `"`, "account=work"},
		},
		{
			name:   "on its own login",
			launch: func(_ context.Context, l launch.Launcher) error { return l.Direct(args) },
			want:   []string{"level=INFO", `msg="starting claude" component=launch`, `mode="own login"`},
		},
		{
			name: "without switchboard",
			launch: func(_ context.Context, l launch.Launcher) error {
				return l.Unaided(args, "couldn't read the config", errors.New("no config file at /home/tester/config.toml"))
			},
			want: []string{
				"level=WARN", `msg="starting claude without switchboard" component=launch`, `reason="couldn't read the config"`,
				`error="no config file at /home/tester/config.toml"`,
			},
		},
		{
			name: "a local subcommand",
			launch: func(_ context.Context, l launch.Launcher) error {
				return l.Local(append([]string{"mcp", "add"}, args...))
			},
			want: []string{"level=DEBUG", `msg="starting claude" component=launch`, "mode=local"},
		},
		{
			name:   "stepping aside for a key",
			launch: func(_ context.Context, l launch.Launcher) error { return l.StepAside(args, "ANTHROPIC_API_KEY") },
			want:   []string{"level=INFO", `msg="starting claude without switchboard" component=launch`, `reason="ANTHROPIC_API_KEY is set"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			h := newHarness(t, "CLAUDE_CODE_OAUTH_TOKEN="+sideToken)

			if err := tt.launch(t.Context(), h.launcher); err != nil {
				t.Fatalf("launch error = %v", err)
			}
			if want := append(slices.Clone(tt.want), "claude="+h.claude); !log.Has(want...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, want)
			}
			for _, secret := range []string{prompt, workToken, sideToken} {
				if strings.Contains(log.String(), secret) {
					t.Errorf("log reads\n%s\nwant it without %q", log, secret)
				}
			}
		})
	}
}

// fakeRouter answers as a router does: its health, or healthErr.
type fakeRouter struct {
	health    router.Health
	healthErr error
	// hangs has Health answer only once its caller gives up.
	hangs bool
}

func (r *fakeRouter) Health(ctx context.Context) (router.Health, error) {
	if r.hangs {
		<-ctx.Done()
		return router.Health{}, fmt.Errorf("ask the router: %w", ctx.Err())
	}
	return r.health, r.healthErr
}

// healthy answers as a healthy router does.
func healthy() *fakeRouter {
	return &fakeRouter{health: router.Health{OK: true, Listen: proxyAddr, PID: 4242, StripsOwnHeaders: true}}
}

// notRunning answers as a router's client does when no router is listening.
func notRunning() *fakeRouter {
	return &fakeRouter{healthErr: fmt.Errorf("%w: dial unix /tmp/sb/control.sock: connect: no such file or directory", router.ErrNotRunning)}
}

// started is a program the launcher was asked to start.
type started struct {
	path string
	argv []string
	// env is the environment it was to start in, but for switchboard's mark
	// of the claude it starts, which marks gives every value of.
	env   []string
	marks []string
}

// markEnv is the variable switchboard marks the claude it starts with.
const markEnv = "SWITCHBOARD_STARTED"

// unmarked splits env into the values of switchboard's mark, and the rest.
func unmarked(env []string) (rest, marks []string) {
	for _, variable := range env {
		if mark, ok := strings.CutPrefix(variable, markEnv+"="); ok {
			marks = append(marks, mark)
		} else {
			rest = append(rest, variable)
		}
	}
	return rest, marks
}

// harness is a launcher whose claude, a stand-in, is where its installer put
// it, and which notes what it's asked to start, and what it says on stderr,
// starting nothing.
type harness struct {
	launcher launch.Launcher
	// claude is where the stand-in for Claude Code is, and switchboard the
	// stand-in for this switchboard binary.
	claude, switchboard string
	starts              []started
	stderr              strings.Builder
}

// newHarness returns a harness whose launches start from environ, for a user
// whose home is homeDir, in a working directory not known unless a test gives
// one.
func newHarness(t *testing.T, environ ...string) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{
		claude:      claudetest.Program(t, filepath.Join(dir, "installed", "claude")),
		switchboard: claudetest.Program(t, filepath.Join(dir, "switchboard")),
	}
	h.launcher = launch.Launcher{
		Environ:      environ,
		Home:         homeDir,
		InstallPaths: []string{h.claude},
		Executable:   func() (string, error) { return h.switchboard, nil },
		PID:          pid,
		Now:          func() time.Time { return launched },
		Exec: func(path string, argv, env []string) error {
			env, marks := unmarked(env)
			h.starts = append(h.starts, started{path: path, argv: argv, env: env, marks: marks})
			return nil
		},
		Stderr: &h.stderr,
	}
	return h
}

// only returns the one program the launcher was asked to start.
func (h *harness) only(t *testing.T) started {
	t.Helper()
	if len(h.starts) != 1 {
		t.Fatalf("asked to start %d programs, want 1", len(h.starts))
	}
	return h.starts[0]
}

// environment returns the variables the one program was started with, by
// name.
func (h *harness) environment(t *testing.T) map[string]string {
	t.Helper()
	env := make(map[string]string)
	for _, variable := range h.only(t).env {
		name, value, _ := strings.Cut(variable, "=")
		if _, twice := env[name]; twice {
			t.Errorf("started with %s set twice", name)
		}
		env[name] = value
	}
	return env
}
