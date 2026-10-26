package redact_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/redact"
)

func TestHoldsToken(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{text: "sk-ant-oat01-fake_token-shaped", want: true},
		{text: "pasted sk-ant-oat01-fake_token-shaped here", want: true},
		{text: "work"},
		{text: "test-token-work"},
		{text: "sk-ant-"},
	}
	for _, tt := range tests {
		if got := redact.HoldsToken(tt.text); got != tt.want {
			t.Errorf("HoldsToken(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestHoldsSecret(t *testing.T) {
	tests := []struct {
		value string
		want  bool
		why   string
	}{
		{value: "sk-ant-oat01-fake_token-shaped", want: true, why: "a token"},
		{value: "https://sk-ant-oat01-fake_token-shaped.gateway.example.com", want: true, why: "a token within a URL"},
		{value: "https://user:secret@gateway.example.com", want: true, why: "userinfo with a password"},
		{value: "https://user@gateway.example.com", want: true, why: "userinfo"},
		{value: "https://gateway.example.com/v1?key=abc", want: true, why: "a query"},
		{value: "https://user:pa%zz@gateway.example.com", want: true, why: "userinfo with a bad escape"},
		{value: "https://gateway.example.com/%zz?key=abc", want: true, why: "a query beside a bad escape"},
		{value: "https:user:pass@gateway.example.com", want: true, why: "userinfo without slashes, which Node's URL reads"},
		{value: `https:\\user:pass@gateway.example.com`, want: true, why: "userinfo after backslashes, which Node's URL reads"},
		{value: "user:pass@gateway.example.com", want: true, why: "userinfo without a scheme"},
		{value: "admin:pw@10.0.0.1:8080", want: true, why: "userinfo without a scheme, on an address and port"},
		{value: "https://gateway.example.com/v1?", want: true, why: "a bare ?, the cost of reading every ? as a query's"},
		{value: "https://gateway.example.com/a@b", want: true, why: "an @ in a path, the cost of reading every @ as userinfo's"},
		{value: "https://gateway.example.com/v1", why: "a plain gateway URL"},
		{value: "http://127.0.0.1:4747", why: "the router's address"},
		{value: "https://gateway.example.com/%zz", why: "a bad escape alone"},
		{value: "0", why: "the hint turned off"},
		{value: "", why: "nothing"},
		{value: "\n", why: "blank headers"},
	}
	for _, tt := range tests {
		if got := redact.HoldsSecret(tt.value); got != tt.want {
			t.Errorf("HoldsSecret(%q) = %v, want %v: %s", tt.value, got, tt.want, tt.why)
		}
	}
}

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
