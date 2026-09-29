package cli_test

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
)

func TestStatus(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), nil)

	got := run(t, deps, "status")
	want := result{
		stdout: `work · Work
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
		t.Errorf("switchboard status =\n%+v\nwant\n%+v", got, want)
	}
}

func TestStatusJSON(t *testing.T) {
	deps := statusDeps(t, fakeClaudeAPI(t), nil)

	got := run(t, deps, "status", "--json")
	want := result{
		stdout: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
  "fallback": {
    "router": "not running"
  },
  "best": "work",
  "accounts": [
    {
      "id": "work",
      "label": "Work",
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
	if got != want {
		t.Errorf("switchboard status --json =\n%+v\nwant\n%+v", got, want)
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
		{name: "from the API's error message, as text", upstream: leaky.URL, args: []string{"status"}},
		{name: "from the API's error message, as JSON", upstream: leaky.URL, args: []string{"status", "--json"}},
		{name: "from the upstream's path, as text", upstream: closed.URL + "/" + token, args: []string{"status"}},
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

// threeAccounts are the accounts statusDeps configures.
const threeAccounts = `
[[account]]
id    = "work"
label = "Work"

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
	api := &claudeAPI{}
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
		api.mu.Unlock()
		headers, ok := usage[req.Model]
		if !ok || r.Header.Get("Authorization") != "Bearer test-token-work" || r.Header.Get("User-Agent") != "claude-code/"+testClaudeVersion {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"Invalid bearer token"}}`)
			return
		}
		for name, value := range headers {
			w.Header().Set(name, value)
		}
		_, _ = io.WriteString(w, `{"type":"message"}`)
	}))
	t.Cleanup(srv.Close)
	api.URL = srv.URL
	return api
}
