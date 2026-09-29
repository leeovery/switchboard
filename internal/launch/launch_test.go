package launch_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	workToken = "test-token-work"
	sideToken = "test-token-side"
	// claudeOnPath is where claude is on PATH.
	claudeOnPath = "/opt/tools/bin/claude"
	// unhealthy is why an unhealthy router says it is.
	unhealthy = "5 of the 8 requests in the last 5 minutes failed"
)

// accounts are the configured accounts: work and side have tokens, personal
// doesn't.
var accounts = []config.Account{
	{ID: "work", Label: "Work", TokenEnv: "CLAUDE_TOKEN_WORK"},
	{ID: "personal", Label: "Personal", TokenEnv: "CLAUDE_TOKEN_PERSONAL"},
	{ID: "side", Label: "Side", TokenEnv: "CLAUDE_TOKEN_SIDE"},
}

func getenv(key string) string {
	return map[string]string{"CLAUDE_TOKEN_WORK": workToken, "CLAUDE_TOKEN_SIDE": sideToken}[key]
}

// route is a launch through r, pinned to account unless it's "".
func route(r launch.Router, account string) launch.Route {
	return launch.Route{
		Config:  &config.Config{Listen: "127.0.0.1:4747", Accounts: accounts},
		Getenv:  getenv,
		Router:  r,
		Account: account,
	}
}

func TestRunThroughAHealthyRouter(t *testing.T) {
	h := newHarness("HOME=/home/tester", "PATH=/usr/bin:/bin", "ANTHROPIC_CUSTOM_HEADERS=X-Trace: on")
	args := []string{"--print", "a prompt", "--account", "work", "", "--", "--direct"}

	if err := h.launcher.Run(t.Context(), route(healthy("work"), "side"), args); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := h.only(t)
	if want := append([]string{"claude"}, args...); got.path != claudeOnPath || !slices.Equal(got.argv, want) {
		t.Errorf("started %s as %q, want %s as %q", got.path, got.argv, claudeOnPath, want)
	}
	want := map[string]string{
		"HOME":                     "/home/tester",
		"PATH":                     "/usr/bin:/bin",
		"ANTHROPIC_BASE_URL":       "http://127.0.0.1:4747",
		"CLAUDE_CODE_OAUTH_TOKEN":  sideToken,
		"ANTHROPIC_CUSTOM_HEADERS": "X-Trace: on\nX-Switchboard-Account: side",
	}
	if env := h.environment(t); !maps.Equal(env, want) {
		t.Errorf("started with the environment\n%q\nwant\n%q", env, want)
	}
	if said := h.stderr.String(); said != "" {
		t.Errorf("said %q on stderr, want nothing", said)
	}
}

func TestRunChoosesTheToken(t *testing.T) {
	tests := []struct {
		name    string
		router  *fakeRouter
		account string
		want    string
		// wantAsked is whether the router is asked which account is best.
		wantAsked bool
	}{
		{name: "the pinned account's", router: healthy("work"), account: "side", want: sideToken},
		{name: "the router's best", router: healthy("side"), want: sideToken, wantAsked: true},
		{name: "the first with a token, when the best has none here", router: healthy("personal"), want: workToken, wantAsked: true},
		{name: "the first with a token, when the router rates none best", router: healthy(""), want: workToken, wantAsked: true},
		{
			name:      "the first with a token, when the router can't say which is best",
			router:    &fakeRouter{health: router.Health{OK: true}, best: "side", statusErr: errors.New("the router answered GET /status with 500 Internal Server Error")},
			want:      workToken,
			wantAsked: true,
		},
		{name: "the first with a token, without the router", router: notRunning(), want: workToken},
		{name: "the pinned account's, without the router", router: notRunning(), account: "side", want: sideToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness()

			if err := h.launcher.Run(t.Context(), route(tt.router, tt.account), nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got := h.environment(t)["CLAUDE_CODE_OAUTH_TOKEN"]; got != tt.want {
				t.Errorf("started on %q, want %q", got, tt.want)
			}
			if asked := tt.router.statusAsked > 0; asked != tt.wantAsked {
				t.Errorf("asked the router which account is best: %v, want %v", asked, tt.wantAsked)
			}
		})
	}
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
			router:    &fakeRouter{health: router.Health{OK: false, Reason: unhealthy, PID: 4242}, best: "side"},
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
			name:      "not running, pinned",
			router:    notRunning(),
			account:   "side",
			wantSaid:  "switchboard: the router isn't running — connecting directly on side · Side\n",
			wantToken: sideToken,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(inherited...)

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
		h := newHarness()
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
			h := newHarness(environ...)

			if err := h.launcher.Run(t.Context(), route(healthy("work"), tt.account), nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			got, set := h.environment(t)["ANTHROPIC_CUSTOM_HEADERS"]
			if got != tt.want || set != (tt.want != "") {
				t.Errorf("started with ANTHROPIC_CUSTOM_HEADERS %q (set: %v), want %q", got, set, tt.want)
			}
		})
	}
}

func TestRunRefusesWithoutAnAccountToStartOn(t *testing.T) {
	tests := []struct {
		name    string
		account string
		// getenv, when set, reads the tokens in place of the tests' own.
		getenv  func(string) string
		wantErr string
	}{
		{name: "pinned to no account", account: "nope", wantErr: `there's no account "nope": pin work or personal or side`},
		{name: "pinned to an account without a token", account: "personal", wantErr: "account personal has no token for Claude Code to start on: set CLAUDE_TOKEN_PERSONAL"},
		{
			name:    "no account with a token",
			getenv:  func(string) string { return "" },
			wantErr: "no account has a token for Claude Code to start on: set CLAUDE_TOKEN_WORK or CLAUDE_TOKEN_PERSONAL or CLAUDE_TOKEN_SIDE, or give --direct to start it on its own login",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := route(healthy("work"), tt.account)
			if tt.getenv != nil {
				r.Getenv = tt.getenv
			}
			h := newHarness()

			if err := h.launcher.Run(t.Context(), r, nil); err == nil || err.Error() != tt.wantErr {
				t.Errorf("Run() error = %v, want %q", err, tt.wantErr)
			}
			if len(h.starts) > 0 {
				t.Errorf("started %q, want nothing started", h.starts)
			}
		})
	}
}

func TestDirectStartsClaudeCodeOnItsOwnLogin(t *testing.T) {
	h := newHarness(
		"HOME=/home/tester",
		"ANTHROPIC_BASE_URL=http://127.0.0.1:4747",
		"CLAUDE_CODE_OAUTH_TOKEN=test-token-stale",
		"ANTHROPIC_CUSTOM_HEADERS=X-Switchboard-Account: side\nX-Trace: on",
	)

	if err := h.launcher.Direct([]string{"--print", "a prompt"}); err != nil {
		t.Fatalf("Direct() error = %v", err)
	}

	got := h.only(t)
	if want := []string{"claude", "--print", "a prompt"}; got.path != claudeOnPath || !slices.Equal(got.argv, want) {
		t.Errorf("started %s as %q, want %s as %q", got.path, got.argv, claudeOnPath, want)
	}
	want := map[string]string{"HOME": "/home/tester", "ANTHROPIC_CUSTOM_HEADERS": "X-Trace: on"}
	if env := h.environment(t); !maps.Equal(env, want) {
		t.Errorf("started with the environment\n%q\nwant\n%q", env, want)
	}
}

func TestFindsClaude(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing", "claude")
	directory := filepath.Join(dir, "directory", "claude")
	notRunnable := filepath.Join(dir, "not-runnable", "claude")
	first := filepath.Join(dir, "first", "claude")
	second := filepath.Join(dir, "second", "claude")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for path, mode := range map[string]os.FileMode{notRunnable: 0o600, first: 0o700, second: 0o700} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, mode); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name     string
		onPath   bool
		installs []string
		want     string
		wantErr  string
	}{
		{name: "on PATH, before any install path", onPath: true, installs: []string{first}, want: claudeOnPath},
		{name: "at the first install path holding a program", installs: []string{missing, directory, notRunnable, first, second}, want: first},
		{
			name:     "nowhere",
			installs: []string{missing, directory, notRunnable},
			wantErr:  "can't find claude: it isn't on PATH, nor at " + missing + ", " + directory + ", " + notRunnable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness()
			h.launcher.InstallPaths = tt.installs
			if !tt.onPath {
				h.launcher.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
			}

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

func TestRunFailsWhenClaudeCantStart(t *testing.T) {
	failed := errors.New("permission denied")
	h := newHarness()
	h.launcher.Exec = func(string, []string, []string) error { return failed }

	err := h.launcher.Run(t.Context(), route(healthy("work"), ""), nil)
	if want := "start claude at " + claudeOnPath + ": permission denied"; !errors.Is(err, failed) || err.Error() != want {
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
				return l.Run(ctx, route(healthy("work"), "side"), args)
			},
			want: []string{"level=INFO", `msg="starting claude" component=launch`, "mode=routed", "router=healthy", "account=side", "chosen=pinned", "claude=" + claudeOnPath},
		},
		{
			name: "through the router, on its best",
			launch: func(ctx context.Context, l launch.Launcher) error {
				return l.Run(ctx, route(healthy("side"), ""), args)
			},
			want: []string{"level=INFO", "mode=routed", "router=healthy", "account=side", `chosen="the router's best"`},
		},
		{
			name:   "directly, the router not running",
			launch: func(ctx context.Context, l launch.Launcher) error { return l.Run(ctx, route(notRunning(), ""), args) },
			want:   []string{"level=WARN", `msg="starting claude" component=launch`, "mode=direct", `router="not running"`, "account=work", `chosen="the first with a token"`},
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
			want:   []string{"level=INFO", `msg="starting claude" component=launch`, `mode="own login"`, "claude=" + claudeOnPath},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			h := newHarness("CLAUDE_TOKEN_WORK="+workToken, "CLAUDE_TOKEN_SIDE="+sideToken)

			if err := tt.launch(t.Context(), h.launcher); err != nil {
				t.Fatalf("launch error = %v", err)
			}
			if !log.Has(tt.want...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, tt.want)
			}
			for _, secret := range []string{prompt, workToken, sideToken} {
				if strings.Contains(log.String(), secret) {
					t.Errorf("log reads\n%s\nwant it without %q", log, secret)
				}
			}
		})
	}
}

// fakeRouter answers as a router does: its health, or healthErr; and a status
// document naming best as the best account, or statusErr.
type fakeRouter struct {
	health    router.Health
	healthErr error
	// hangs has Health answer only once its caller gives up.
	hangs     bool
	best      string
	statusErr error
	// statusAsked counts the status documents asked for.
	statusAsked int
}

func (r *fakeRouter) Health(ctx context.Context) (router.Health, error) {
	if r.hangs {
		<-ctx.Done()
		return router.Health{}, fmt.Errorf("ask the router: %w", ctx.Err())
	}
	return r.health, r.healthErr
}

func (r *fakeRouter) Status(context.Context) (status.Document, error) {
	r.statusAsked++
	return status.Document{Best: r.best}, r.statusErr
}

// healthy answers as a healthy router does, rating best the best account.
func healthy(best string) *fakeRouter {
	return &fakeRouter{health: router.Health{OK: true, PID: 4242}, best: best}
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

// harness is a launcher whose claude is on PATH, and which notes what it's
// asked to start, and what it says on stderr, starting nothing.
type harness struct {
	launcher launch.Launcher
	starts   []started
	stderr   strings.Builder
}

// newHarness returns a harness whose launches start from environ.
func newHarness(environ ...string) *harness {
	h := &harness{}
	h.launcher = launch.Launcher{
		Environ: environ,
		LookPath: func(file string) (string, error) {
			if file != "claude" {
				return "", exec.ErrNotFound
			}
			return claudeOnPath, nil
		},
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
