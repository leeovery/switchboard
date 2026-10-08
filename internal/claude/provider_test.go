package claude_test

import (
	"encoding/json"
	"math"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
)

func TestProviderRoutable(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "sending a message", path: "/v1/messages", want: true},
		{name: "counting its tokens", path: "/v1/messages/count_tokens", want: true},
		{name: "batches", path: "/v1/messages/batches", want: false},
		{name: "a batch", path: "/v1/messages/batches/msgbatch_01HkcTjaV5uDC8jWR4ZsDV8d", want: false},
		{name: "a batch's results", path: "/v1/messages/batches/msgbatch_01HkcTjaV5uDC8jWR4ZsDV8d/results", want: false},
		{name: "another path under messages", path: "/v1/messages/other", want: false},
		{name: "messages with a trailing slash", path: "/v1/messages/", want: false},
		{name: "counting tokens with a trailing slash", path: "/v1/messages/count_tokens/", want: false},
		{name: "a path that only starts like messages", path: "/v1/messagesfoo", want: false},
		{name: "messages in another case", path: "/v1/Messages", want: false},
		{name: "the connectivity check", path: "/api/hello", want: false},
		{name: "an identity-bound path", path: "/v1/code/sessions", want: false},
		{name: "files", path: "/v1/files", want: false},
		{name: "the root", path: "/", want: false},
		{name: "no path", path: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (claude.Provider{}).Routable(tt.path); got != tt.want {
				t.Errorf("Routable(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestProviderSpends(t *testing.T) {
	for path, want := range map[string]bool{"/v1/messages": true, "/v1/messages/count_tokens": false} {
		if got := (claude.Provider{}).Spends(path); got != want {
			t.Errorf("Spends(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestProviderSession(t *testing.T) {
	h := http.Header{}
	if got := (claude.Provider{}).Session(h); got != "" {
		t.Errorf("Session() without the header = %q, want empty", got)
	}
	h.Set("x-claude-code-session-id", "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e")
	if got, want := (claude.Provider{}).Session(h), "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e"; got != want {
		t.Errorf("Session() = %q, want %q", got, want)
	}
}

func TestProviderBetas(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   []string
	}{
		{name: "none without the header", header: http.Header{}},
		{name: "one", header: http.Header{"Anthropic-Beta": {"oauth-2025-04-20"}}, want: []string{"oauth-2025-04-20"}},
		{
			name:   "a list, trimmed, in its order",
			header: http.Header{"Anthropic-Beta": {"oauth-2025-04-20, context-1m-2025-08-07 ,interleaved-thinking-2025-05-14"}},
			want:   []string{"oauth-2025-04-20", "context-1m-2025-08-07", "interleaved-thinking-2025-05-14"},
		},
		{
			name:   "a list over several lines, its empty entries passed over",
			header: http.Header{"Anthropic-Beta": {"oauth-2025-04-20,,", " ", "context-1m-2025-08-07"}},
			want:   []string{"oauth-2025-04-20", "context-1m-2025-08-07"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (claude.Provider{}).Betas(tt.header); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Betas() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProviderAsks(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantModel string
		wantCheck bool
	}{
		{name: "a model leading the body", body: `{"model":"claude-opus-5-5","max_tokens":32000,"messages":[]}`, wantModel: "claude-opus-5-5"},
		{name: "a model after the rest", body: `{"messages":[{"role":"user","content":"model"}],"model":"claude-haiku-4-5-20251001"}`, wantModel: "claude-haiku-4-5-20251001"},
		{name: "no model", body: `{"messages":[]}`},
		{name: "a model that isn't a string", body: `{"model":7}`},
		{name: "a model beside tokens that aren't a number", body: `{"model":"claude-opus-5-5","max_tokens":"lots"}`, wantModel: "claude-opus-5-5"},
		{name: "a body that isn't JSON", body: `model=claude-opus-5-5`},
		{name: "an empty body", body: ``},
		{
			name:      "the quota check",
			body:      `{"model":"claude-opus-5-5","max_tokens":1,"messages":[{"role":"user","content":"quota"}],"metadata":{"user_id":"user_test"}}`,
			wantModel: "claude-opus-5-5",
			wantCheck: true,
		},
		{
			name:      "the quota check asking in a block of text",
			body:      `{"messages":[{"role":"user","content":[{"type":"text","text":"quota"}]}],"max_tokens" : 1,"model":"claude-haiku-4-5-20251001"}`,
			wantModel: "claude-haiku-4-5-20251001",
			wantCheck: true,
		},
		{
			name:      "quota asked with room to answer",
			body:      `{"model":"claude-opus-5-5","max_tokens":2,"messages":[{"role":"user","content":"quota"}]}`,
			wantModel: "claude-opus-5-5",
		},
		{
			name:      "a token asked for something else",
			body:      `{"model":"claude-opus-5-5","max_tokens":1,"messages":[{"role":"user","content":"quotas"}]}`,
			wantModel: "claude-opus-5-5",
		},
		{
			name:      "quota asked after another message",
			body:      `{"model":"claude-opus-5-5","max_tokens":1,"messages":[{"role":"user","content":"hello"},{"role":"user","content":"quota"}]}`,
			wantModel: "claude-opus-5-5",
		},
		{
			name:      "quota asked beside another block",
			body:      `{"model":"claude-opus-5-5","max_tokens":1,"messages":[{"role":"user","content":[{"type":"text","text":"quota"},{"type":"text","text":"now"}]}]}`,
			wantModel: "claude-opus-5-5",
		},
		{
			name:      "a block that isn't text",
			body:      `{"model":"claude-opus-5-5","max_tokens":1,"messages":[{"role":"user","content":[{"type":"image","text":"quota"}]}]}`,
			wantModel: "claude-opus-5-5",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, check, _ := (claude.Provider{}).Asks([]byte(tt.body))
			if model != tt.wantModel || check != tt.wantCheck {
				t.Errorf("Asks(%s) = %q, %v, want %q, %v", tt.body, model, check, tt.wantModel, tt.wantCheck)
			}
		})
	}
}

func TestProviderAsksTheShapeOfARequest(t *testing.T) {
	// secret is shaped like a token, as an MCP server's can be, though it's
	// none.
	const secret = "sk-ant-oat01-fake_token-shaped"
	tests := []struct {
		name string
		body string
		want ledger.Shape
	}{
		{
			name: "a request deep into a session, with every setting known",
			body: `{"model":"claude-opus-5-5","max_tokens":32000,` +
				`"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":[{"type":"text","text":"hi"}]},{"role":"user","content":"more"}],` +
				`"system":[{"type":"text","text":"You are Claude Code."},{"type":"text","text":"The environment."}],` +
				`"tools":[{"name":"Bash","input_schema":{}},{"name":"Read","input_schema":{}}],` +
				`"thinking":{"type":"enabled","budget_tokens":31999,"display":"summarized","block_binding":{"prefix_mismatch_behavior":"drop_block"}},` +
				`"stream":true,"tool_choice":{"type":"auto","disable_parallel_tool_use":true},"temperature":0.7,"top_k":40,"top_p":0.95,"service_tier":"auto",` +
				`"output_config":{"effort":"high","format":{"type":"json_schema","schema":{"description":"` + secret + `"}},"task_budget":{"type":"tokens","total":64000}},` +
				`"speed":"fast","inference_geo":"us",` +
				`"context_management":{"edits":[{"type":"clear_tool_uses_20250919","trigger":{"type":"input_tokens","value":100000},"exclude_tools":["` + secret + `"]},` +
				`{"type":"compact_20260112"}]},` +
				`"metadata":{"user_id":"user_device_account"},"mcp_servers":[{"name":"notes","authorization_token":"` + secret + `"}]}`,
			want: ledger.Shape{Messages: 3, System: 2, Tools: 2, MaxTokens: new(int64(32000)),
				Thinking: ledger.Thinking{Type: "enabled", BudgetTokens: new(int64(31999)), Display: "summarized"},
				Stream:   new(true), ToolChoice: ledger.ToolChoice{Type: "auto"}, Temperature: new(0.7), TopK: new(int64(40)), TopP: new(0.95),
				ServiceTier: "auto", OutputConfig: ledger.OutputConfig{Effort: "high"}, Speed: "fast", InferenceGeo: "us",
				ContextManagement: ledger.ContextManagement{Edits: []string{"clear_tool_uses_20250919", "compact_20260112"}}},
		},
		{
			name: "a system prompt given as a string, not streamed",
			body: `{"model":"claude-opus-5-5","max_tokens":1024,"system":"You are Claude Code.","messages":[{"role":"user","content":"hello"}],"stream":false}`,
			want: ledger.Shape{Messages: 1, System: 1, MaxTokens: new(int64(1024)), Stream: new(false)},
		},
		{
			name: "a system prompt whose texts hold what structures a list",
			body: `{"model":"claude-opus-5-5","system":[{"type":"text","text":"a [list], {of} \"quoted\" things\\"},{"type":"text","text":"]},{\\\"\\\\"}],"messages":[]}`,
			want: ledger.Shape{System: 2},
		},
		{
			name: "lists given empty, and settings at zero",
			body: `{"model":"claude-opus-5-5","max_tokens":0,"system":[ ],"messages":[],"tools":[],"temperature":0,"top_k":0,"top_p":0,"context_management":{"edits":[]}}`,
			want: ledger.Shape{MaxTokens: new(int64(0)), Temperature: new(0.0), TopK: new(int64(0)), TopP: new(0.0)},
		},
		{
			name: "settings given as null",
			body: `{"model":"claude-opus-5-5","max_tokens":null,"system":null,"messages":null,"thinking":null,"stream":null,"tool_choice":null,"temperature":null,` +
				`"top_k":null,"top_p":null,"service_tier":null,"output_config":null,"speed":null,"inference_geo":null,"context_management":null}`,
		},
		{
			name: "settings and lists of types the API wouldn't take",
			body: `{"model":"claude-opus-5-5","max_tokens":"lots","messages":"hello","tools":{"name":"Bash"},` +
				`"thinking":"on","stream":"yes","tool_choice":"auto","temperature":"hot","top_k":"forty","top_p":"most","service_tier":1,` +
				`"output_config":"high","speed":1,"inference_geo":["us"],"context_management":{"edits":["clear_tool_uses_20250919"]}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, _, got := (claude.Provider{}).Asks([]byte(tt.body))
			tt.want.Bytes = len(tt.body)
			if model != "claude-opus-5-5" || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Asks() = %q, %+v, want claude-opus-5-5, %+v", model, got, tt.want)
			}
			kept, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			for _, unknown := range []string{secret, "user_device_account", "block_binding", "disable_parallel_tool_use", "json_schema", "task_budget",
				"trigger", "exclude_tools", "Bash", "hello", "You are"} {
				if strings.Contains(string(kept), unknown) {
					t.Errorf("the shape holds %s, want nothing of %q", kept, unknown)
				}
			}
		})
	}
}

func TestProviderAsksNoShapeOfABodyThatIsntARequest(t *testing.T) {
	for _, body := range []string{`model=claude-opus-5-5`, ``, `["claude-opus-5-5"]`, `"claude-opus-5-5"`, `{"model":"claude-opus-5-5",`} {
		if model, check, shape := (claude.Provider{}).Asks([]byte(body)); model != "" || check || !reflect.DeepEqual(shape, ledger.Shape{}) {
			t.Errorf("Asks(%s) = %q, %v, %+v, want nothing read", body, model, check, shape)
		}
	}
}

func TestProviderFamily(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{model: "claude-haiku-4-5-20251001", want: "haiku"},
		{model: "claude-3-5-haiku-20241022", want: "haiku"},
		{model: "claude-sonnet-4-5", want: "sonnet"},
		{model: "claude-opus-5-5", want: "opus"},
		{model: "claude-opus-4-1-20250805", want: "opus"},
		{model: "claude-fable-5-1", want: "fable"},
		{model: "claude-fable-5", want: "fable"},
		{model: "claude-opusplus-1", want: "claude-opusplus-1"},
		{model: "some-other-model", want: "some-other-model"},
		{model: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := (claude.Provider{}).Family(tt.model); got != tt.want {
				t.Errorf("Family(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestProviderErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "the API's error",
			body: `{"type":"error","error":{"type":"permission_error","message":"This model isn't on your plan"}}`,
			want: "This model isn't on your plan",
		},
		{
			name: "with the token hidden",
			body: `{"type":"error","error":{"type":"authentication_error","message":"token ` + token + ` revoked"}}`,
			want: "token [redacted] revoked",
		},
		{
			name: "with anything shaped like a token hidden",
			body: `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key sk-ant-oat01-fake_token-shaped"}}`,
			want: "invalid x-api-key [redacted]",
		},
		{name: "an error without a message", body: `{"type":"error","error":{"type":"api_error"}}`, want: ""},
		{name: "a body that isn't JSON", body: "<html>Forbidden</html>", want: ""},
		{name: "an empty body", body: "", want: ""},
		{
			name: "a message past the first 64 KiB",
			body: `{"padding":"` + strings.Repeat("x", 64<<10) + `","error":{"message":"read too late"}}`,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (claude.Provider{}).ErrorMessage(strings.NewReader(tt.body), token); got != tt.want {
				t.Errorf("ErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProviderClassify(t *testing.T) {
	// throttledAfter is a throttled verdict asking for a wait of d.
	throttledAfter := func(d time.Duration) quota.Outcome {
		return quota.Outcome{Verdict: quota.Throttled, RetryAfter: d}
	}
	// allowedAfter is the header of a 429 whose overall status is allowed,
	// asking for a wait of retryAfter.
	allowedAfter := func(retryAfter string) http.Header {
		return header("anthropic-ratelimit-unified-status", "allowed", "retry-after", retryAfter)
	}
	// limitIn is the limit reached in the windows with the given keys.
	limitIn := func(rejected ...string) quota.Outcome {
		return quota.Outcome{Verdict: quota.LimitReached, Rejected: rejected}
	}
	// limitUntil is the limit reached in the windows with the given keys,
	// until the Unix time given.
	limitUntil := func(unix int64, rejected ...string) quota.Outcome {
		return quota.Outcome{Verdict: quota.LimitReached, Rejected: rejected, LimitedUntil: time.Unix(unix, 0).UTC()}
	}
	tests := []struct {
		name   string
		status int
		header http.Header
		want   quota.Outcome
	}{
		{name: "a 200", status: http.StatusOK, want: quota.Outcome{Verdict: quota.Served}},
		{name: "a 400", status: http.StatusBadRequest, want: quota.Outcome{Verdict: quota.Served}},
		{name: "a 500, which Claude Code retries", status: http.StatusInternalServerError, want: quota.Outcome{Verdict: quota.Served}},
		{name: "a 529 overloaded, which Claude Code retries", status: 529, want: quota.Outcome{Verdict: quota.Served}},
		{
			name:   "a 200 whose window is rejected, served on overage",
			status: http.StatusOK,
			header: header("anthropic-ratelimit-unified-5h-status", "rejected", "anthropic-ratelimit-unified-status", "rejected"),
			want:   quota.Outcome{Verdict: quota.Served},
		},
		{name: "a 401, refusing the token", status: http.StatusUnauthorized, want: quota.Outcome{Verdict: quota.Refused}},
		{name: "a 403, refusing the request alone", status: http.StatusForbidden, want: quota.Outcome{Verdict: quota.Forbidden}},
		{
			name:   "a 429 whose overall status is rejected",
			status: http.StatusTooManyRequests,
			header: header("anthropic-ratelimit-unified-status", "rejected", "anthropic-ratelimit-unified-5h-status", "allowed_warning", "retry-after", "30"),
			want:   limitIn(),
		},
		{
			name:   "a 429 whose session is rejected",
			status: http.StatusTooManyRequests,
			header: header("anthropic-ratelimit-unified-5h-status", "rejected", "anthropic-ratelimit-unified-7d-status", "allowed"),
			want:   limitIn("5h"),
		},
		{
			name:   "a 429 whose model's week is rejected, in any case of header name",
			status: http.StatusTooManyRequests,
			header: http.Header{"Anthropic-Ratelimit-Unified-7D_OI-Status": {"rejected"}},
			want:   limitIn("7d_oi"),
		},
		{
			name:   "a 429 whose rejected window has no utilization",
			status: http.StatusTooManyRequests,
			header: header("anthropic-ratelimit-unified-7d-status", "rejected"),
			want:   limitIn("7d"),
		},
		{
			name:   "a 429 whose model's week is rejected, its reset given but not its utilization",
			status: http.StatusTooManyRequests,
			header: header(
				"anthropic-ratelimit-unified-status", "rejected",
				"anthropic-ratelimit-unified-5h-utilization", "0.23",
				"anthropic-ratelimit-unified-5h-status", "allowed",
				"anthropic-ratelimit-unified-7d_oi-status", "rejected",
				"anthropic-ratelimit-unified-7d_oi-reset", "1790974800",
			),
			want: limitUntil(1790974800, "7d_oi"),
		},
		{
			name:   "a limit until the overall reset, the rejecting claim's, over any window's",
			status: http.StatusTooManyRequests,
			header: header(
				"anthropic-ratelimit-unified-status", "rejected",
				"anthropic-ratelimit-unified-reset", "1790619000",
				"anthropic-ratelimit-unified-5h-status", "rejected",
				"anthropic-ratelimit-unified-5h-reset", "1790974800",
			),
			want: limitUntil(1790619000, "5h"),
		},
		{
			name:   "a limit until the rejected window's reset, not those with room",
			status: http.StatusTooManyRequests,
			header: header(
				"anthropic-ratelimit-unified-5h-status", "rejected",
				"anthropic-ratelimit-unified-5h-reset", "1790619000",
				"anthropic-ratelimit-unified-7d-status", "allowed",
				"anthropic-ratelimit-unified-7d-reset", "1790974800",
			),
			want: limitUntil(1790619000, "5h"),
		},
		{
			name:   "a limit until the latest of the rejected windows' resets",
			status: http.StatusTooManyRequests,
			header: header(
				"anthropic-ratelimit-unified-7d-status", "rejected",
				"anthropic-ratelimit-unified-7d-reset", "1790974800",
				"anthropic-ratelimit-unified-5h-status", "rejected",
				"anthropic-ratelimit-unified-5h-reset", "1790619000",
			),
			want: limitUntil(1790974800, "5h", "7d"),
		},
		{
			name:   "a limit until the reset of the rejected window that gives one",
			status: http.StatusTooManyRequests,
			header: header(
				"anthropic-ratelimit-unified-5h-status", "rejected",
				"anthropic-ratelimit-unified-7d-status", "rejected",
				"anthropic-ratelimit-unified-7d-reset", "1790974800",
			),
			want: limitUntil(1790974800, "5h", "7d"),
		},
		{
			name:   "a limit whose overall reset can't be read, until the windows'",
			status: http.StatusTooManyRequests,
			header: header(
				"anthropic-ratelimit-unified-status", "rejected",
				"anthropic-ratelimit-unified-reset", "soon",
				"anthropic-ratelimit-unified-5h-status", "rejected",
				"anthropic-ratelimit-unified-5h-reset", "1790619000",
			),
			want: limitUntil(1790619000, "5h"),
		},
		{
			name:   "a 429 rejecting overage alone, which isn't a window",
			status: http.StatusTooManyRequests,
			header: header(
				"anthropic-ratelimit-unified-status", "allowed",
				"anthropic-ratelimit-unified-5h-status", "allowed",
				"anthropic-ratelimit-unified-overage-status", "rejected",
				"anthropic-ratelimit-unified-overage-disabled-reason", "org_level_disabled",
				"retry-after", "7",
			),
			want: throttledAfter(7 * time.Second),
		},
		{name: "a 429 without headers", status: http.StatusTooManyRequests, want: quota.Outcome{Verdict: quota.Served}},
		{
			name:   "a 429 without usage headers, asking for a wait and a retry",
			status: http.StatusTooManyRequests,
			header: header("retry-after", "7", "x-should-retry", "true"),
			want:   quota.Outcome{Verdict: quota.Served},
		},
		{
			name:   "a 429 whose overall status is allowed",
			status: http.StatusTooManyRequests,
			header: header("anthropic-ratelimit-unified-status", "allowed", "x-should-retry", "true"),
			want:   throttledAfter(0),
		},
		{
			name:   "a 429 whose windows alone are allowed",
			status: http.StatusTooManyRequests,
			header: header("anthropic-ratelimit-unified-5h-utilization", "0.23", "anthropic-ratelimit-unified-5h-status", "allowed", "retry-after", "3"),
			want:   throttledAfter(3 * time.Second),
		},
		{name: "a 429 asking for a wait", status: http.StatusTooManyRequests, header: allowedAfter(" 12 "), want: throttledAfter(12 * time.Second)},
		{name: "a 429 asking for no wait", status: http.StatusTooManyRequests, header: allowedAfter("0"), want: throttledAfter(0)},
		{name: "a 429 asking for a wait as a date", status: http.StatusTooManyRequests, header: allowedAfter("Mon, 28 Sep 2026 13:12:30 GMT"), want: throttledAfter(0)},
		{name: "a 429 asking for a wait in fractions", status: http.StatusTooManyRequests, header: allowedAfter("1.5"), want: throttledAfter(0)},
		{name: "a 429 asking for a negative wait", status: http.StatusTooManyRequests, header: allowedAfter("-5"), want: throttledAfter(0)},
		{
			name:   "a 429 asking for a wait longer than a duration holds",
			status: http.StatusTooManyRequests,
			header: allowedAfter("99999999999999999"),
			want:   throttledAfter(time.Duration(math.MaxInt64 / int64(time.Second) * int64(time.Second))),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.header
			if h == nil {
				h = http.Header{}
			}
			if got := (claude.Provider{}).Classify(tt.status, h); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Classify(%d, %v) = %+v, want %+v", tt.status, h, got, tt.want)
			}
		})
	}
}

func TestProviderMarkLimited(t *testing.T) {
	until := time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC)
	tests := []struct {
		name  string
		until time.Time
		want  http.Header
		// wantOutcome is what Classify reads off the headers of a 429.
		wantOutcome quota.Outcome
	}{
		{
			name:        "until a reset",
			until:       until,
			want:        header("anthropic-ratelimit-unified-status", "rejected", "anthropic-ratelimit-unified-reset", "1790619000"),
			wantOutcome: quota.Outcome{Verdict: quota.LimitReached, LimitedUntil: until},
		},
		{
			name:        "until a reset that isn't known",
			want:        header("anthropic-ratelimit-unified-status", "rejected"),
			wantOutcome: quota.Outcome{Verdict: quota.LimitReached},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := header("Content-Type", "application/json")
			(claude.Provider{}).MarkLimited(h, tt.until)

			tt.want.Set("Content-Type", "application/json")
			if !reflect.DeepEqual(h, tt.want) {
				t.Errorf("MarkLimited() left the headers %v, want %v", h, tt.want)
			}
			if got := (claude.Provider{}).Classify(http.StatusTooManyRequests, h); !reflect.DeepEqual(got, tt.wantOutcome) {
				t.Errorf("Classify() of a 429 with those headers = %+v, want %+v", got, tt.wantOutcome)
			}
		})
	}
}

func TestProviderUsage(t *testing.T) {
	h := header(
		"anthropic-ratelimit-unified-5h-utilization", "0.23",
		"anthropic-ratelimit-unified-5h-reset", "1790619000",
		"anthropic-ratelimit-unified-5h-status", "allowed",
		"anthropic-ratelimit-unified-overage-status", "rejected",
		"anthropic-ratelimit-unified-overage-utilization", "0",
		"anthropic-ratelimit-unified-overage-reset", "1790619000",
	)
	want := quota.Usage{
		Windows: []quota.Window{session},
		Extra:   quota.ExtraUsage{Status: quota.StatusRejected, Utilization: new(0.0), ResetsAt: session.ResetsAt},
	}
	if got := (claude.Provider{}).Usage(h); !reflect.DeepEqual(got, want) {
		t.Errorf("Usage() =\n%+v\nwant\n%+v", got, want)
	}
}
