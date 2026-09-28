package claude_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
)

func TestProviderRoutable(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/v1/messages", want: true},
		{path: "/v1/messages/count_tokens", want: true},
		{path: "/api/hello", want: false},
		{path: "/v1/code/sessions", want: false},
		{path: "/v1/files", want: false},
		{path: "/v1/messagesfoo", want: false},
		{path: "/v1/Messages", want: false},
		{path: "/", want: false},
		{path: "", want: false},
	}
	for _, tt := range tests {
		if got := (claude.Provider{}).Routable(tt.path); got != tt.want {
			t.Errorf("Routable(%q) = %v, want %v", tt.path, got, tt.want)
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
