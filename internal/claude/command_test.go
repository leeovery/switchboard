package claude_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
)

func TestKeyEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "none", env: map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": "test-token-work"}},
		{name: "an API key", env: map[string]string{"ANTHROPIC_API_KEY": "test-key"}, want: "ANTHROPIC_API_KEY"},
		{name: "a token of its own", env: map[string]string{"ANTHROPIC_AUTH_TOKEN": "test-key"}, want: "ANTHROPIC_AUTH_TOKEN"},
		{name: "both", env: map[string]string{"ANTHROPIC_API_KEY": "test-key", "ANTHROPIC_AUTH_TOKEN": "test-key"}, want: "ANTHROPIC_API_KEY"},
		{name: "one set empty", env: map[string]string{"ANTHROPIC_API_KEY": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claude.KeyEnv(func(key string) string { return tt.env[key] }); got != tt.want {
				t.Errorf("KeyEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}
