package config_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

func TestAccountToken(t *testing.T) {
	account := config.Account{ID: "work", TokenEnv: "CLAUDE_TOKEN_WORK"}
	tests := []struct {
		name   string
		env    map[string]string
		want   string
		wantOK bool
	}{
		{name: "set", env: map[string]string{"CLAUDE_TOKEN_WORK": "token-work"}, want: "token-work", wantOK: true},
		{name: "surrounding whitespace trimmed", env: map[string]string{"CLAUDE_TOKEN_WORK": " token-work\n"}, want: "token-work", wantOK: true},
		{name: "unset", env: map[string]string{"CLAUDE_TOKEN_PERSONAL": "token-personal"}, want: "", wantOK: false},
		{name: "empty", env: map[string]string{"CLAUDE_TOKEN_WORK": ""}, want: "", wantOK: false},
		{name: "whitespace only", env: map[string]string{"CLAUDE_TOKEN_WORK": " \t\n"}, want: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, ok := account.Token(envFrom(tt.env))
			if got := token.Reveal(); got != tt.want || ok != tt.wantOK {
				t.Errorf("Token() = %q, %v, want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestTokenNeverPrintsItsSecret(t *testing.T) {
	const secret = "token-work-secret"
	token, ok := config.Account{TokenEnv: "CLAUDE_TOKEN_WORK"}.Token(envFrom(map[string]string{"CLAUDE_TOKEN_WORK": secret}))
	if !ok {
		t.Fatal("Token() found no token")
	}

	var text, json bytes.Buffer
	slog.New(slog.NewTextHandler(&text, nil)).Info("request", "token", token)
	slog.New(slog.NewJSONHandler(&json, nil)).Info("request", "token", token)

	outputs := []struct {
		name string
		got  string
	}{
		{name: "%v", got: fmt.Sprintf("%v", token)},
		{name: "%+v", got: fmt.Sprintf("%+v", token)},
		{name: "%#v", got: fmt.Sprintf("%#v", token)},
		{name: "%q", got: fmt.Sprintf("%q", token)},
		{name: "%d", got: fmt.Sprintf("%d", token)},
		{name: "exported field", got: fmt.Sprintf("%+v", struct{ Token config.Token }{token})},
		{name: "unexported field", got: fmt.Sprintf("%+v", struct{ token config.Token }{token})},
		{name: "unexported field, Go syntax", got: fmt.Sprintf("%#v", struct{ token config.Token }{token})},
		{name: "error", got: fmt.Errorf("request failed with %v", token).Error()},
		{name: "slog text", got: text.String()},
		{name: "slog JSON", got: json.String()},
	}
	for _, out := range outputs {
		if strings.Contains(out.got, secret) {
			t.Errorf("%s output contains the secret", out.name)
		}
	}

	if got := token.String(); got != "[redacted]" {
		t.Errorf("String() = %q, want %q", got, "[redacted]")
	}
	if !strings.Contains(text.String(), "token=[redacted]") || !strings.Contains(json.String(), `"token":"[redacted]"`) {
		t.Errorf("slog output = %q and %q, want the token shown as [redacted]", text.String(), json.String())
	}
}
