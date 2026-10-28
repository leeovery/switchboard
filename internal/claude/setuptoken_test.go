package claude_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
)

func TestIsSetupToken(t *testing.T) {
	token := "sk-ant-oat01-" + strings.Repeat("test-token-work_", 6)
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "a setup token", text: token, want: true},
		{name: "one of a later version", text: "sk-ant-oat02-" + strings.Repeat("work", 10), want: true},
		{name: "a refresh token", text: "sk-ant-ort01-" + strings.Repeat("work", 10)},
		{name: "an API key", text: "sk-ant-api03-" + strings.Repeat("work", 10)},
		{name: "one cut short", text: "sk-ant-oat01-test-token"},
		{name: "one with a space in it", text: "sk-ant-oat01-test-token-work " + strings.Repeat("work", 10)},
		{name: "one on a line with more", text: "export CLAUDE_CODE_OAUTH_TOKEN=" + token},
		{name: "one with a space after it", text: token + " "},
		{name: "the placeholder setup-token shows", text: "<token>"},
		{name: "nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claude.IsSetupToken(tt.text); got != tt.want {
				t.Errorf("IsSetupToken(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestIsSetupTokenPart(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "the rest of a token", text: "0123456789_test-token-work", want: true},
		{name: "a whole token", text: "sk-ant-oat01-" + strings.Repeat("work", 10), want: true},
		{name: "a sentence", text: "Store this token securely."},
		{name: "a placeholder", text: "<token>"},
		{name: "nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claude.IsSetupTokenPart(tt.text); got != tt.want {
				t.Errorf("IsSetupTokenPart(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestSetupTokenUnsetNamesWhatWouldSendItElsewhereOrWarn(t *testing.T) {
	want := []string{"ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"}
	if got := claude.SetupTokenUnset(); !slices.Equal(got, want) {
		t.Errorf("SetupTokenUnset() = %q, want %q", got, want)
	}
}
