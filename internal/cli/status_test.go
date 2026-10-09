package cli_test

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/cli"
)

func TestStatus(t *testing.T) {
	for _, form := range prettyForms() {
		t.Run(form.name, func(t *testing.T) {
			deps := statusDeps(t, fakeClaudeAPI(t), nil)

			got := form.run(t, deps, "status")
			want := result{
				stdout: `work · Work (primary)
  Session     23%  resets in 4h 58m · Mon 18:10
  Week        93%  resets in 4d 7h · Fri 21:00 · runs out ~Mon 18:01
  Fable week 100%  resets in 5d 11h · Sun 01:10 · exhausted

personal · Personal
  token missing: write it to ` + tokenPath(t, deps, "personal") + `

side · Side
  HTTP 401 · Invalid bearer token

best next: work · Work
probed directly: the router isn't running
`,
				code: 0,
			}
			if got != want {
				t.Errorf("switchboard status %s =\n%+v\nwant\n%+v", strings.Join(form.args, " "), got, want)
			}
		})
	}
}

func TestStatusJSON(t *testing.T) {
	for _, form := range jsonForms() {
		t.Run(form.name, func(t *testing.T) {
			deps := statusDeps(t, fakeClaudeAPI(t), nil)

			got := form.run(t, deps, "status")
			if want := statusDocument(t, deps); got != want {
				t.Errorf("switchboard status %s =\n%+v\nwant\n%+v", strings.Join(form.args, " "), got, want)
			}
		})
	}
}

// statusDocument is what status prints as JSON of the accounts statusDeps
// configures.
func statusDocument(t *testing.T, deps cli.Deps) result {
	t.Helper()
	return result{
		stdout: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
  "fallback": {
    "router": "not running"
  },
  "best": "work",
  "primary": "work",
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "primary": true,
      "reserve": 0.05,
      "token_set": true,
      "fetched_at": "2026-09-28T13:12:00Z",
      "windows": [
        {
          "key": "5h",
          "label": "Session",
          "utilization": 0.23,
          "resets_at": "2026-09-28T18:10:00Z",
          "status": "allowed"
        },
        {
          "key": "7d",
          "label": "Week",
          "utilization": 0.93,
          "resets_at": "2026-10-02T21:00:00Z",
          "status": "allowed_warning"
        },
        {
          "key": "7d_oi",
          "label": "Fable week",
          "utilization": 1,
          "resets_at": "2026-10-04T01:10:00Z",
          "status": "rejected"
        }
      ]
    },
    {
      "id": "personal",
      "label": "Personal",
      "token_set": false,
      "error": "token missing: write it to ` + tokenPath(t, deps, "personal") + `"
    },
    {
      "id": "side",
      "label": "Side",
      "token_set": true,
      "error": "HTTP 401 · Invalid bearer token"
    }
  ]
}
`,
		code: 0,
	}
}

func TestStatusRefreshWithoutTheRouterProbesAsStatusDoes(t *testing.T) {
	// runStatus runs switchboard status with args, with a token for personal,
	// as goldenDeps gives it, and returns how it went and what the API was
	// asked.
	runStatus := func(args ...string) (result, []string) {
		api := newClaudeAPI(t)
		deps := statusDeps(t, api.URL, nil)
		writeToken(t, deps, "personal", "test-token-personal")
		return run(t, deps, append([]string{"status"}, args...)...), api.questions()
	}
	for _, flags := range [][]string{{"--pretty"}, {"--json"}} {
		want, probes := runStatus(flags...)
		if want.code != 0 || len(probes) == 0 {
			t.Fatalf("switchboard status %s = %+v, asking the API %q, want exit status 0 and every account probed", strings.Join(flags, " "), want, probes)
		}
		for _, flag := range []string{"--refresh", "-r"} {
			args := append(slices.Clone(flags), flag)
			got, asked := runStatus(args...)
			if got != want {
				t.Errorf("switchboard status %s =\n%+v\nwant what status %s prints\n%+v", strings.Join(args, " "), got, strings.Join(flags, " "), want)
			}
			if !slices.Equal(asked, probes) {
				t.Errorf("switchboard status %s asked the API %q, want %q, every account probed as status probes it", strings.Join(args, " "), asked, probes)
			}
		}
	}
}

func TestStatusSessionTakesNoRefresh(t *testing.T) {
	got := run(t, statusDeps(t, fakeClaudeAPI(t), nil), "status", "--session", "0b5c", "--refresh")
	const want = "Error: --session asks after one session, not the accounts, so it takes no --refresh\n"
	if got.code != 1 || !strings.HasPrefix(got.stderr, want) {
		t.Errorf("switchboard status --session 0b5c --refresh = %+v, want exit status 1 and an error starting %q", got, want)
	}
}

func TestStatusListsTheRoutersSessions(t *testing.T) {
	srv := newServeSetup(t, fakeClaudeAPI(t), nil)
	srv.start(t)
	srv.waitForProbes(t)
	srv.routePinned(t, sessionThree, "claude-haiku-4-5-20251001", "work")
	srv.route(t, sessionOne, "claude-haiku-4-5-20251001")
	if got := run(t, srv.deps, "pin", "side", "--session", "18bb"); got.code != 0 {
		t.Fatalf("switchboard pin side --session 18bb = %+v", got)
	}

	got := run(t, srv.deps, "status", "--pretty")
	want := result{stdout: workAtStatus + "  2 sessions\n\n" + othersAtStatus(t, srv.deps) + `
sessions
  0b5c6f2e  haiku on work  ·  seen just now
  18bb978f  haiku on work  ·  pinned to side  ·  seen just now

best next: work · Work
from the router: healthy  ·  2 sessions  ·  routing automatically
`}
	if got != want {
		t.Errorf("switchboard status =\n%+v\nwant\n%+v", got, want)
	}
}

func TestStatusSaysWhenClaudeOnPathIsntSwitchboard(t *testing.T) {
	const unrouted = "claude on PATH isn't switchboard, so the sessions it starts don't go through the router: run switchboard setup\n\n"
	root := t.TempDir()
	// switchboard, its claude link in its bin directory, and Claude Code.
	binary := claudetest.Program(t, filepath.Join(root, "brew", "switchboard"))
	linked := filepath.Dir(claudetest.Link(t, binary, filepath.Join(root, "switchboard", "bin", "claude")))
	claudeCode := filepath.Dir(claudetest.Program(t, filepath.Join(root, "claude-code", "claude")))
	tests := []struct {
		name string
		// path lists the directories on PATH.
		path     []string
		args     []string
		wantCode int
		// wantSaid is set when the text starts saying so.
		wantSaid bool
	}{
		{name: "switchboard, first on PATH", path: []string{linked, claudeCode}, args: []string{"status", "--pretty"}},
		{name: "Claude Code, first on PATH", path: []string{claudeCode, linked}, args: []string{"status", "--pretty"}, wantSaid: true},
		{name: "no claude on PATH", args: []string{"status", "--pretty"}, wantSaid: true},
		{name: "Claude Code, first on PATH, as JSON", path: []string{claudeCode}, args: []string{"status", "--json"}},
		{name: "Claude Code, first on PATH, as JSON off a terminal", path: []string{claudeCode}, args: []string{"status"}},
		{name: "Claude Code, first on PATH, a session asked after", path: []string{claudeCode}, args: []string{"status", "--session", "0b5c"}, wantCode: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := statusDeps(t, fakeClaudeAPI(t), map[string]string{"PATH": strings.Join(tt.path, string(filepath.ListSeparator))})
			deps.Executable = func() (string, error) { return binary, nil }

			got := run(t, deps, tt.args...)
			if said := strings.HasPrefix(got.stdout, unrouted); got.code != tt.wantCode || said != tt.wantSaid || strings.Count(got.stdout+got.stderr, "isn't switchboard") > 1 {
				t.Errorf("switchboard %s = %+v; want exit status %d, and saying claude on PATH isn't switchboard, first, once: %v",
					strings.Join(tt.args, " "), got, tt.wantCode, tt.wantSaid)
			}
			if !tt.wantSaid && strings.Contains(got.stdout+got.stderr, "isn't switchboard") {
				t.Errorf("switchboard %s = %+v; want nothing said of claude on PATH", strings.Join(tt.args, " "), got)
			}
		})
	}
}

func TestStatusWithInvalidConfig(t *testing.T) {
	path := writeConfig(t, invalidConfig)

	got := run(t, testDeps(map[string]string{"SWITCHBOARD_CONFIG": path}, t.TempDir()), "status")
	wantErr := "Error: invalid config " + path + ":\n" + `unknown key "account.token_env": tokens now live in files`
	if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, wantErr) {
		t.Errorf("switchboard status = %+v, want exit status 1 and an error starting %q", got, wantErr)
	}
}

func TestStatusNeverPrintsTheToken(t *testing.T) {
	const token = "test-token-work"
	leaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid bearer token `+token+`"}}`)
	}))
	t.Cleanup(leaky.Close)
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	tests := []struct {
		name     string
		upstream string
		args     []string
	}{
		{name: "from the API's error message, as text", upstream: leaky.URL, args: []string{"status", "--pretty"}},
		{name: "from the API's error message, as JSON", upstream: leaky.URL, args: []string{"status", "--json"}},
		{name: "from the upstream's path, as text", upstream: closed.URL + "/" + token, args: []string{"status", "--pretty"}},
		{name: "from the upstream's path, as JSON", upstream: closed.URL + "/" + token, args: []string{"status", "--json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, fmt.Sprintf("upstream = %q\n\n[[account]]\nid = \"work\"\n", tt.upstream))
			deps := testDeps(map[string]string{"SWITCHBOARD_CONFIG": path, "SWITCHBOARD_LOG_LEVEL": "debug"}, t.TempDir())
			writeToken(t, deps, "work", token)

			got := run(t, deps, tt.args...)
			if strings.Contains(got.stdout+got.stderr, token) {
				t.Fatal("output contains the token")
			}
			if got.code != 0 || !strings.Contains(got.stdout, "[redacted]") {
				t.Errorf("switchboard %s = %+v, want exit status 0 and the account's error, redacted", strings.Join(tt.args, " "), got)
			}
			log := readLog(t, deps, "cli.log")
			if strings.Contains(log, token) {
				t.Fatal("log contains the token")
			}
			if !hasLine(log, "level=WARN", `msg="probe failed"`, "account=work", "[redacted]") {
				t.Errorf("cli.log reads\n%s\nwant the account's error, redacted", log)
			}
		})
	}
}

// statusDeps configures three accounts against upstream: work, whose token
// fakeClaudeAPI accepts; personal, without a token file; and side, whose token
// fakeClaudeAPI rejects. env adds to their environment.
func statusDeps(t *testing.T, upstream string, env map[string]string) cli.Deps {
	t.Helper()
	path := writeConfig(t, fmt.Sprintf("upstream = %q\n", upstream)+threeAccounts)
	vars := map[string]string{"SWITCHBOARD_CONFIG": path}
	maps.Copy(vars, env)
	deps := testDeps(vars, t.TempDir())
	writeToken(t, deps, "work", "test-token-work")
	writeToken(t, deps, "side", "test-token-side")
	return deps
}

// threeAccounts are the accounts statusDeps configures. Work, the first, is
// the primary, keeping a twentieth of each window back, short of what
// fakeClaudeAPI reads of it.
const threeAccounts = `
[[account]]
id      = "work"
label   = "Work"
reserve = 0.05

[[account]]
id    = "personal"
label = "Personal"

[[account]]
id    = "side"
label = "Side"
`

// fakeClaudeAPI serves a probe of the work account from Claude Code
// testClaudeVersion with its usage headers, and rejects anything else. The
// account has used up its Fable week, which only Fable requests count, so it's
// still the account to use next. It returns the API's URL.
func fakeClaudeAPI(t *testing.T) string {
	t.Helper()
	return newClaudeAPI(t).URL
}

// claudeAPI is the API fakeClaudeAPI serves, noting what it's asked.
type claudeAPI struct {
	URL string

	mu sync.Mutex
	// asked holds each request's token and model, as "token model".
	asked []string
	// session is the share of its session work reads as having used, and
	// limited is set while work is at the limit of its week.
	session string
	limited bool
}

// readSessionAs has the API read work's session as used as given, such as
// "0.41", from now on.
func (a *claudeAPI) readSessionAs(used string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.session = used
}

// limitWeek has the API answer every request on work's token at the limit of
// its week from now on, or, given false, take them again, as after the week
// is reset by hand.
func (a *claudeAPI) limitWeek(limited bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.limited = limited
}

// questions returns what the API has been asked, sorted: each request's token
// and model, as "token model".
func (a *claudeAPI) questions() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Sorted(slices.Values(a.asked))
}

// newClaudeAPI starts the API fakeClaudeAPI describes.
func newClaudeAPI(t *testing.T) *claudeAPI {
	t.Helper()
	api := &claudeAPI{session: "0.23"}
	accountWide := map[string]string{
		"anthropic-ratelimit-unified-5h-utilization": "0.23",
		"anthropic-ratelimit-unified-5h-reset":       "1790619000", // Mon 28 Sep 2026 18:10 UTC
		"anthropic-ratelimit-unified-5h-status":      "allowed",
		"anthropic-ratelimit-unified-7d-utilization": "0.93",
		"anthropic-ratelimit-unified-7d-reset":       "1790974800", // Fri 2 Oct 2026 21:00 UTC
		"anthropic-ratelimit-unified-7d-status":      "allowed_warning",
	}
	fable := maps.Clone(accountWide)
	maps.Copy(fable, map[string]string{
		"anthropic-ratelimit-unified-7d_oi-utilization": "1",
		"anthropic-ratelimit-unified-7d_oi-reset":       "1791076200", // Sun 4 Oct 2026 01:10 UTC
		"anthropic-ratelimit-unified-7d_oi-status":      "rejected",
	})
	usage := map[string]map[string]string{
		"claude-haiku-4-5-20251001": accountWide,
		"claude-fable-5-1":          fable,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		api.mu.Lock()
		api.asked = append(api.asked, token+" "+req.Model)
		session, limited := api.session, api.limited
		api.mu.Unlock()
		if limited && token == "test-token-work" {
			w.Header().Set("anthropic-ratelimit-unified-status", "rejected")
			w.Header().Set("anthropic-ratelimit-unified-7d-utilization", "1")
			w.Header().Set("anthropic-ratelimit-unified-7d-reset", accountWide["anthropic-ratelimit-unified-7d-reset"])
			w.Header().Set("anthropic-ratelimit-unified-7d-status", "rejected")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"You've hit your weekly limit"}}`)
			return
		}
		headers, ok := usage[req.Model]
		if !ok || r.Header.Get("Authorization") != "Bearer test-token-work" || r.Header.Get("User-Agent") != "claude-code/"+testClaudeVersion {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"Invalid bearer token"}}`)
			return
		}
		for name, value := range headers {
			w.Header().Set(name, value)
		}
		w.Header().Set("anthropic-ratelimit-unified-5h-utilization", session)
		_, _ = io.WriteString(w, `{"type":"message"}`)
	}))
	t.Cleanup(srv.Close)
	api.URL = srv.URL
	return api
}
