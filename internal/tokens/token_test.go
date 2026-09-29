package tokens_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/leeovery/switchboard/internal/tokens"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		want    string
		wantErr error
	}{
		{name: "a token", text: "test-token-work", want: "test-token-work"},
		{name: "the whitespace around it ignored", text: " \ttest-token-work\r\n\n", want: "test-token-work"},
		{name: "nothing", text: "", wantErr: tokens.ErrMissing},
		{name: "whitespace alone", text: " \n\t", wantErr: tokens.ErrMissing},
		{name: "two tokens", text: "test-token-work test-token-side", wantErr: tokens.ErrNotAToken},
		{name: "a token a line", text: "test-token-work\ntest-token-side\n", wantErr: tokens.ErrNotAToken},
		{name: "a shell's export of one", text: "export CLAUDE_CODE_OAUTH_TOKEN=test-token-work", wantErr: tokens.ErrNotAToken},
		{name: "a control character within", text: "test-token\x00work", wantErr: tokens.ErrNotAToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := tokens.Parse(tt.text)
			checkParsed(t, token, err, tt.want, tt.wantErr)
		})
	}
}

func TestParseFrom(t *testing.T) {
	errRead := errors.New("read failed")
	tests := []struct {
		name    string
		r       io.Reader
		want    string
		wantErr error
	}{
		{name: "a token", r: strings.NewReader("test-token-work\n"), want: "test-token-work"},
		{name: "as large as a token file can be", r: strings.NewReader(strings.Repeat("x", 4<<10)), want: strings.Repeat("x", 4<<10)},
		{name: "nothing", r: strings.NewReader(""), wantErr: tokens.ErrMissing},
		{name: "two tokens", r: strings.NewReader("test-token-work test-token-side\n"), wantErr: tokens.ErrNotAToken},
		{name: "larger than a token file can be", r: strings.NewReader(strings.Repeat("x", 4<<10+1)), wantErr: tokens.ErrNotAToken},
		{name: "a read that fails", r: iotest.ErrReader(errRead), wantErr: errRead},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := tokens.ParseFrom(tt.r)
			checkParsed(t, token, err, tt.want, tt.wantErr)
		})
	}
}

// checkParsed checks a token parsed from text is want, or that parsing it
// failed with wantErr, without showing the text.
func checkParsed(t *testing.T, token tokens.Token, err error, want string, wantErr error) {
	t.Helper()
	switch {
	case wantErr != nil:
		if !errors.Is(err, wantErr) {
			t.Errorf("error = %v, want %v", err, wantErr)
		}
	case err != nil:
		t.Fatalf("error = %v", err)
	case token.Reveal() != want:
		t.Errorf("token = %q, want %q", token.Reveal(), want)
	}
	if err != nil && strings.Contains(err.Error(), "test-token") {
		t.Errorf("error = %q, which shows what was parsed", err)
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
