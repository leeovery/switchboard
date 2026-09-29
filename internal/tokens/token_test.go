package tokens_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/tokens"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		want        string
		wantMissing bool
		wantErr     bool
	}{
		{name: "a token", text: "test-token-work", want: "test-token-work"},
		{name: "the whitespace around it ignored", text: " \ttest-token-work\r\n\n", want: "test-token-work"},
		{name: "nothing", text: "", wantMissing: true},
		{name: "whitespace alone", text: " \n\t", wantMissing: true},
		{name: "two tokens", text: "test-token-work test-token-side", wantErr: true},
		{name: "a token a line", text: "test-token-work\ntest-token-side\n", wantErr: true},
		{name: "a shell's export of one", text: "export CLAUDE_CODE_OAUTH_TOKEN=test-token-work", wantErr: true},
		{name: "a control character within", text: "test-token\x00work", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := tokens.Parse(tt.text)
			switch {
			case tt.wantMissing:
				if !errors.Is(err, tokens.ErrMissing) {
					t.Errorf("Parse() error = %v, want %v", err, tokens.ErrMissing)
				}
			case tt.wantErr:
				if err == nil || errors.Is(err, tokens.ErrMissing) {
					t.Errorf("Parse() error = %v, want one saying it's more than a token", err)
				}
			case err != nil:
				t.Fatalf("Parse() error = %v", err)
			case token.Reveal() != tt.want:
				t.Errorf("Parse() = %q, want %q", token.Reveal(), tt.want)
			}
			if err != nil && strings.Contains(err.Error(), "test-token") {
				t.Errorf("Parse() error = %q, which shows what it was given", err)
			}
		})
	}
}

func TestTokenNeverPrintsItsSecret(t *testing.T) {
	const secret = "test-token-work-secret"
	token, err := tokens.Parse(secret)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
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
		{name: "exported field", got: fmt.Sprintf("%+v", struct{ Token tokens.Token }{token})},
		{name: "unexported field", got: fmt.Sprintf("%+v", struct{ token tokens.Token }{token})},
		{name: "unexported field, Go syntax", got: fmt.Sprintf("%#v", struct{ token tokens.Token }{token})},
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
