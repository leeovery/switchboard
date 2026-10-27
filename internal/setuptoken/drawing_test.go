package setuptoken

import (
	"slices"
	"strings"
	"testing"
)

// madeToken is a placeholder token, shaped as setup-token's are.
var madeToken = "sk-ant-oat01-test-token-work-" + strings.Repeat("0123456789", 8)

// drawn returns a drawing of lines, each written after the one above it, as a
// frame is drawn.
func drawn(lines ...string) drawing {
	d := make(drawing, len(lines))
	for i, text := range lines {
		d[i] = line{text: text, written: i + 1}
	}
	return d
}

// texts returns the text of each of d's lines.
func texts(d drawing) []string {
	if d == nil {
		return nil
	}
	return strings.Split(d.text(), "\n")
}

// madeScreen is setup-token's screen once it has made madeToken, as it draws
// it, but for the token line, which is token, a row for each of its lines.
func madeScreen(token string) drawing {
	return drawn(slices.Concat(
		[]string{" ✓ Long-lived authentication token created successfully!", "", " Your OAuth token (valid for 1 year):", ""},
		strings.Split(token, "\n"),
		[]string{"", " Store this token securely. You won't be able to see it again.", "", " Use this token by setting: export CLAUDE_CODE_OAUTH_TOKEN=<token>"},
	)...)
}

// waitingFrame is setup-token's frame as it waits on the sign-in, drawn as
// Ink draws one.
const waitingFrame = " Opening browser to sign in…\r\n\r\n Browser didn't open? Use the url below to sign in:\r\n\r\n https://claude.example/oauth/authorize\r\n\r\n\r\n Paste code here if prompted >\r\n"

// madeFrame is setup-token's frame once it has made madeToken, cut short after
// as many of its characters as cut, or whole where cut is -1.
func madeFrame(cut int) string {
	frame := madeScreen(" " + madeToken).text()
	if cut >= 0 {
		frame = frame[:strings.Index(frame, madeToken)+cut]
	}
	return strings.ReplaceAll(frame, "\n", "\r\n")
}

// inPlace is frame drawn over the frame before it, from its first row, each
// row erased as it's drawn, as a renderer drawing what's changed does: the
// rows of the frame before past where frame stops are left as they were.
func inPlace(frame string) string {
	return "\x1b[8A\x1b[2K" + strings.ReplaceAll(frame, "\r\n", "\r\n\x1b[2K")
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
		{name: "none made", drawing: drawn(" Opening browser to sign in…", " "+madeToken)},
		{name: "one shown before the line saying it's made", drawing: drawn(append([]string{" " + madeToken}, texts(madeScreen(" <token>"))...)...)},
		{name: "one broken across two rows", drawing: madeScreen(" " + madeToken[:40] + "\n " + madeToken[40:])},
		{name: "one cut short", drawing: madeScreen(" sk-ant-oat01-test-token")},
		{name: "one drawn last, where setup-token was stopped drawing it", drawing: madeScreen(" " + madeToken)[:5]},
		{name: "one with nothing drawn below it but blank rows", drawing: drawn(append(texts(madeScreen(" " + madeToken)[:5]), "", "")...)},
		{name: "one with a line below it written before it", drawing: append(madeScreen(" " + madeToken)[:5], line{text: " Paste code here if prompted >", written: 1})},
		{name: "one on a line with more", drawing: madeScreen(" token: " + madeToken)},
		{name: "none shown", drawing: madeScreen("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := tt.drawing.token(); ok {
				t.Errorf("token() = %q, want none from %q", got.Reveal(), tt.drawing.text())
			}
		})
	}
}

func TestDrawingReadsATokenOnlyWhereItsFrameWasDrawnPastIt(t *testing.T) {
	tests := []struct {
		name   string
		drawn  string
		wantOK bool
	}{
		{name: "the made frame drawn below the last", drawn: waitingFrame + madeFrame(-1), wantOK: true},
		{name: "the made frame drawn over the last, erased first", drawn: waitingFrame + "\x1b[8A\x1b[J" + madeFrame(-1), wantOK: true},
		{name: "the made frame drawn over the last in place", drawn: waitingFrame + inPlace(madeFrame(-1)), wantOK: true},
		{name: "the made frame drawn over the last in place, cut short in the token", drawn: waitingFrame + inPlace(madeFrame(40))},
		{name: "the made frame drawn over the last, erased first, cut short in the token", drawn: waitingFrame + "\x1b[8A\x1b[J" + madeFrame(40)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newScreen(columns, rows)
			s.write([]byte(tt.drawn))
			got, ok := s.drawing().token()
			if ok != tt.wantOK || ok && got.Reveal() != madeToken {
				t.Errorf("token() = %q, %v; want the token whole: %v, from %q", got.Reveal(), ok, tt.wantOK, s.drawing().text())
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
		{name: "made, with the check mark drawn otherwise", drawing: drawn(" √ Long-lived authentication token created successfully!"), want: true},
		{name: "signing in", drawing: drawn(" Opening browser to sign in…")},
		{name: "nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.drawing.made(); got != tt.want {
				t.Errorf("made() of %q = %v, want %v", tt.drawing.text(), got, tt.want)
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
		{name: "an OAuth error", drawing: drawn(" OAuth error: Request failed with status code 400", "", " Press Enter to retry."), wantMessage: "Request failed with status code 400", wantFailed: true},
		{name: "an error shown as it is", drawing: drawn(" The sign-in timed out.", "", " Press Enter to retry."), wantFailed: true},
		{name: "an OAuth error it doesn't wait at", drawing: drawn(" OAuth error: Request failed with status code 400")},
		{name: "none", drawing: drawn(" Opening browser to sign in…")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, failed := tt.drawing.failed()
			if message != tt.wantMessage || failed != tt.wantFailed {
				t.Errorf("failed() of %q = %q, %v; want %q, %v", tt.drawing.text(), message, failed, tt.wantMessage, tt.wantFailed)
			}
		})
	}
}
