package launch_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

const (
	workToken = "test-token-work"
	sideToken = "test-token-side"
	// proxyAddr is where a healthy router says its proxy listens, which isn't
	// where the config says: the router's word is the one that counts.
	proxyAddr = "127.0.0.1:4848"
	// unhealthy is why an unhealthy router says it is.
	unhealthy = "5 of the 8 requests in the last 5 minutes failed"
)

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
		Router:  r,
		Account: account,
	}
}

func TestRunThroughAHealthyRouter(t *testing.T) {
	path := t.TempDir()
	h := newHarness(t, "HOME=/home/tester", "PATH="+path, "ANTHROPIC_CUSTOM_HEADERS=X-Trace: on")
	args := []string{"--print", "a prompt", "--account", "work", "", "--", "--direct"}

	if err := h.launcher.Run(t.Context(), route(healthy(), "side"), args); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := h.only(t)
	if want := append([]string{"claude"}, args...); got.path != h.claude || !slices.Equal(got.argv, want) {
		t.Errorf("started %s as %q, want %s as %q", got.path, got.argv, h.claude, want)
	}
	want := map[string]string{
		"HOME":                     "/home/tester",
		"PATH":                     path,
		"ANTHROPIC_BASE_URL":       "http://" + proxyAddr,
		"CLAUDE_CODE_OAUTH_TOKEN":  workToken,
		"ANTHROPIC_CUSTOM_HEADERS": "X-Trace: on\nX-Switchboard-Account: side",
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
		"ANTHROPIC_CUSTOM_HEADERS=X-Trace: on\nX-Switchboard-Account: side",
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
		// want are those Claude Code starts with, "" for none.
		want string
	}{
		{name: "none, unpinned"},
		{name: "none, pinned", account: "side", want: "X-Switchboard-Account: side"},
		{name: "others, unpinned", headers: "X-Trace: on\nX-Team: core", want: "X-Trace: on\nX-Team: core"},
		{name: "others, pinned", headers: "X-Trace: on\nX-Team: core", account: "side", want: "X-Trace: on\nX-Team: core\nX-Switchboard-Account: side"},
		{name: "a pin inherited, unpinned", headers: "X-Switchboard-Account: work"},
		{name: "a pin inherited among others, pinned", headers: "x-switchboard-account : work\r\nX-Trace: on\n\n", account: "side", want: "X-Trace: on\nX-Switchboard-Account: side"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var environ []string
			if tt.headers != "" {
				environ = append(environ, "ANTHROPIC_CUSTOM_HEADERS="+tt.headers)
			}
			h := newHarness(t, environ...)

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

func TestRunRefusesAPinItCantKeep(t *testing.T) {
	tests := []struct {
		account string
		wantErr string
	}{
		{account: "nope", wantErr: `there's no account "nope": pin work or personal or side`},
		{account: "personal", wantErr: "account personal has no usable token for Claude Code to start on: " + tokenstest.Missing("personal").Error()},
	}
	for _, tt := range tests {
		t.Run(tt.account, func(t *testing.T) {
			h := newHarness(t)

			if err := h.launcher.Run(t.Context(), route(healthy(), tt.account), nil); err == nil || err.Error() != tt.wantErr {
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
		`error="work: ` + tokenstest.Missing("work").Error() + `\npersonal: token missing\nside: ` + tokenstest.Missing("side").Error() + `"`,
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
		"ANTHROPIC_CUSTOM_HEADERS=X-Switchboard-Account: side\nX-Trace: on",
	)

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

func TestNoLaunchStartsSwitchboardsClaudeLink(t *testing.T) {
	tests := []struct {
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
	}
	for _, tt := range tests {
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
	return &fakeRouter{health: router.Health{OK: true, Listen: proxyAddr, PID: 4242}}
}

// notRunning answers as a router's client does when no router is listening.
func notRunning() *fakeRouter {
	return &fakeRouter{healthErr: fmt.Errorf("%w: dial unix /tmp/sb/control.sock: connect: no such file or directory", router.ErrNotRunning)}
}

// started is a program the launcher was asked to start.
type started struct {
	path string
	argv []string
	env  []string
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

// newHarness returns a harness whose launches start from environ.
func newHarness(t *testing.T, environ ...string) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{
		claude:      claudetest.Program(t, filepath.Join(dir, "installed", "claude")),
		switchboard: claudetest.Program(t, filepath.Join(dir, "switchboard")),
	}
	h.launcher = launch.Launcher{
		Environ:      environ,
		InstallPaths: []string{h.claude},
		Executable:   func() (string, error) { return h.switchboard, nil },
		Exec: func(path string, argv, env []string) error {
			h.starts = append(h.starts, started{path: path, argv: argv, env: env})
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
