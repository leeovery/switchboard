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
	}{
		{value: "sk-ant-oat01-fake_token-shaped", want: true},
		{value: "https://sk-ant-oat01-fake_token-shaped.gateway.example.com", want: true},
		{value: "https://user:secret@gateway.example.com", want: true},
		{value: "https://user@gateway.example.com", want: true},
		{value: "https://gateway.example.com/v1?key=abc", want: true},
		{value: "https://gateway.example.com/v1"},
		{value: "http://127.0.0.1:4747"},
		{value: "https://gateway.example.com/v1?"},
		{value: "0"},
		{value: ""},
		{value: "\n"},
	}
	for _, tt := range tests {
		if got := redact.HoldsSecret(tt.value); got != tt.want {
			t.Errorf("HoldsSecret(%q) = %v, want %v", tt.value, got, tt.want)
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
