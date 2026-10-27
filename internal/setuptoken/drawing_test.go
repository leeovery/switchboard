package setuptoken

import (
	"strings"
	"testing"
)

// madeToken is a placeholder token, shaped as setup-token's are.
var madeToken = "sk-ant-oat01-test-token-work-" + strings.Repeat("0123456789", 8)

// madeScreen is setup-token's screen once it has made madeToken, as it draws
// it, but for the token line, which is token.
func madeScreen(token string) drawing {
	return drawing{
		" ✓ Long-lived authentication token created successfully!",
		"",
		" Your OAuth token (valid for 1 year):",
		"",
		token,
		"",
		" Store this token securely. You won't be able to see it again.",
		"",
		" Use this token by setting: export CLAUDE_CODE_OAUTH_TOKEN=<token>",
	}
}

func TestDrawingTakesTheTokenShownBelowTheLineSayingItsMade(t *testing.T) {
	got, ok := madeScreen(" " + madeToken).token()

	if !ok || got.Reveal() != madeToken {
		t.Errorf("token() = %q, %v; want the token shown", got.Reveal(), ok)
	}
}

func TestDrawingTakesNoTokenItCantReadWhole(t *testing.T) {
	tests := []struct {
		name    string
		drawing drawing
	}{
		{name: "none made", drawing: drawing{" Opening browser to sign in…", " " + madeToken}},
		{name: "one shown before the line saying it's made", drawing: append(drawing{" " + madeToken}, madeScreen(" <token>")...)},
		{name: "one broken across two rows", drawing: madeScreen(" " + madeToken[:40] + "\n " + madeToken[40:])},
		{name: "one cut short", drawing: madeScreen(" sk-ant-oat01-test-token")},
		{name: "one on a line with more", drawing: madeScreen(" token: " + madeToken)},
		{name: "none shown", drawing: madeScreen("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := drawing(strings.Split(tt.drawing.text(), "\n"))
			if got, ok := lines.token(); ok {
				t.Errorf("token() = %q, want none from %q", got.Reveal(), lines)
			}
		})
	}
}

func TestDrawingSaysWhetherSetupTokenMadeTheToken(t *testing.T) {
	tests := []struct {
		name    string
		drawing drawing
		want    bool
	}{
		{name: "made", drawing: madeScreen(" " + madeToken), want: true},
		{name: "made, with the check mark drawn otherwise", drawing: drawing{" √ Long-lived authentication token created successfully!"}, want: true},
		{name: "signing in", drawing: drawing{" Opening browser to sign in…"}},
		{name: "nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.drawing.made(); got != tt.want {
				t.Errorf("made() of %q = %v, want %v", tt.drawing, got, tt.want)
			}
		})
	}
}

func TestDrawingFindsTheErrorSetupTokenWaitsToRetryAt(t *testing.T) {
	tests := []struct {
		name        string
		drawing     drawing
		wantMessage string
		wantFailed  bool
	}{
		{name: "an OAuth error", drawing: drawing{" OAuth error: Request failed with status code 400", "", " Press Enter to retry."}, wantMessage: "Request failed with status code 400", wantFailed: true},
		{name: "an error shown as it is", drawing: drawing{" The sign-in timed out.", "", " Press Enter to retry."}, wantFailed: true},
		{name: "an OAuth error it doesn't wait at", drawing: drawing{" OAuth error: Request failed with status code 400"}},
		{name: "none", drawing: drawing{" Opening browser to sign in…"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, failed := tt.drawing.failed()
			if message != tt.wantMessage || failed != tt.wantFailed {
				t.Errorf("failed() of %q = %q, %v; want %q, %v", tt.drawing, message, failed, tt.wantMessage, tt.wantFailed)
			}
		})
	}
}
