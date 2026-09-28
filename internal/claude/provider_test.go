package claude_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
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

func TestProviderModel(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "leading the body", body: `{"model":"claude-opus-5-5","max_tokens":1,"messages":[]}`, want: "claude-opus-5-5"},
		{name: "after the rest", body: `{"messages":[{"role":"user","content":"model"}],"model":"claude-haiku-4-5-20251001"}`, want: "claude-haiku-4-5-20251001"},
		{name: "missing", body: `{"messages":[]}`, want: ""},
		{name: "not a string", body: `{"model":7}`, want: ""},
		{name: "not JSON", body: `model=claude-opus-5-5`, want: ""},
		{name: "empty", body: ``, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (claude.Provider{}).Model([]byte(tt.body)); got != tt.want {
				t.Errorf("Model(%s) = %q, want %q", tt.body, got, tt.want)
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

func TestProviderUsage(t *testing.T) {
	h := header(
		"anthropic-ratelimit-unified-5h-utilization", "0.23",
		"anthropic-ratelimit-unified-5h-reset", "1790619000",
		"anthropic-ratelimit-unified-5h-status", "allowed",
	)
	want := []quota.Window{session}
	if got := (claude.Provider{}).Usage(h); !reflect.DeepEqual(got, want) {
		t.Errorf("Usage() =\n%+v\nwant\n%+v", got, want)
	}
}
