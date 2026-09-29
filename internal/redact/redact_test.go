package redact_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/redact"
)

func TestText(t *testing.T) {
	// tokenShaped is shaped like a Claude token, though it's none.
	const tokenShaped = "sk-ant-oat01-fake_token-shaped"
	tests := []struct {
		name    string
		text    string
		secrets []string
		want    string
	}{
		{name: "nothing to hide", text: "HTTP 529 · Overloaded", want: "HTTP 529 · Overloaded"},
		{name: "anything shaped like a token", text: "invalid x-api-key " + tokenShaped + ".", want: "invalid x-api-key [redacted]."},
		{name: "every one of them", text: tokenShaped + " and " + tokenShaped, want: "[redacted] and [redacted]"},
		{name: "a secret given", text: "Bearer test-token-work refused", secrets: []string{"test-token-work"}, want: "Bearer [redacted] refused"},
		{name: "each secret given", text: "test-token-work, test-token-side", secrets: []string{"test-token-work", "test-token-side"}, want: "[redacted], [redacted]"},
		{name: "a secret given as none", text: "HTTP 401", secrets: []string{""}, want: "HTTP 401"},
		{name: "a secret and a token's shape together", text: "test-token-work " + tokenShaped, secrets: []string{"test-token-work"}, want: "[redacted] [redacted]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redact.Text(tt.text, tt.secrets...); got != tt.want {
				t.Errorf("Text(%q, %q) = %q, want %q", tt.text, tt.secrets, got, tt.want)
			}
		})
	}
}
