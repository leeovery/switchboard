package setuptoken

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// madeToken is a placeholder token, shaped as setup-token's are.
var madeToken = "sk-ant-oat01-test-token-work-" + strings.Repeat("0123456789", 8)

// drawn returns a drawing of lines, each character written after the one
// before it, as a frame is drawn.
func drawn(lines ...string) drawing {
	d := make(drawing, len(lines))
	writes := 0
	for i, text := range lines {
		for _, r := range text {
			writes++
			d[i] = append(d[i], cell{r: r, written: writes})
		}
	}
	return d
}

// writtenAt returns a line of text, each of its characters written at at.
func writtenAt(text string, at int) line {
	var l line
	for _, r := range text {
		l = append(l, cell{r: r, written: at})
	}
	return l
}

// texts returns the text of each of d's lines.
func texts(d drawing) []string {
	if d == nil {
		return nil
	}
	return strings.Split(d.text(), "\n")
}

// madeRows are the rows of setup-token's frame once it has made a token, the
// token's own row for each of token's lines.
func madeRows(token string) []string {
	return slices.Concat(
		[]string{" ✓ Long-lived authentication token created successfully!", "", " Your OAuth token (valid for 1 year):", ""},
		strings.Split(token, "\n"),
		[]string{"", " Store this token securely. You won't be able to see it again.", "", " Use this token by setting: export CLAUDE_CODE_OAUTH_TOKEN=<token>"},
	)
}

// madeScreen is setup-token's screen once it has made a token, drawn as
// madeRows has it.
func madeScreen(token string) drawing {
	return drawn(madeRows(token)...)
}

// waitingRows are the rows of setup-token's frame as it waits on the sign-in:
// text on each row the token's frame draws the token, and the line below it,
// on, the last of which shares characters, in place, with the line below.
var waitingRows = []string{
	" Opening browser to sign in…",
	"",
	" Browser didn't open? Use the url below to sign in:",
	"",
	" https://claude.example/oauth/authorize?code=true",
	" Paste code here if prompted >",
	" Signing in…",
	" Waiting for the sign-in…",
}

// frame returns rows drawn as lines, from the cursor down.
func frame(rows []string) string {
	return strings.Join(rows, "\r\n") + "\r\n"
}

// madeFrame is setup-token's frame once it has made madeToken, drawn below
// the last.
func madeFrame() string {
	return frame(madeRows(" " + madeToken))
}

// A renderer draws the frame of rows over the frame of was on the screen's
// first rows, as a terminal program redraws its frame.
type renderer func(was, rows []string) string

// The ways renderers draw: below the last frame; over it, erasing it first;
// over it in place, erasing each row as it's drawn; over it a row at a time,
// the rows that changed; and over it a character at a time, those that
// changed.
var renderers = map[string]renderer{
	"below": func(_, rows []string) string { return frame(rows) },
	"erasing first": func(_, rows []string) string {
		return "\x1b[H\x1b[J" + frame(rows)
	},
	"in place": func(_, rows []string) string {
		return "\x1b[H\x1b[2K" + strings.ReplaceAll(frame(rows), "\r\n", "\r\n\x1b[2K")
	},
	"a row at a time": func(was, rows []string) string {
		var b strings.Builder
		for i := range max(len(was), len(rows)) {
			row := at(rows, i)
			if row != at(was, i) {
				fmt.Fprintf(&b, "\x1b[%dH%s\x1b[K", i+1, row)
			}
		}
		return b.String()
	},
	"a character at a time": func(was, rows []string) string {
		var b strings.Builder
		for i := range max(len(was), len(rows)) {
			old, row := []rune(at(was, i)), []rune(at(rows, i))
			for j := range max(len(old), len(row)) {
				if r := runeAt(row, j); r != runeAt(old, j) {
					fmt.Fprintf(&b, "\x1b[%d;%dH%c", i+1, j+1, r)
				}
			}
		}
		return b.String()
	},
}

// at returns rows' row i, "" past their end.
func at(rows []string, i int) string {
	if i < len(rows) {
		return rows[i]
	}
	return ""
}

// runeAt returns row's character at column j, a blank past its end.
func runeAt(row []rune, j int) rune {
	if j < len(row) {
		return row[j]
	}
	return ' '
}

func TestDrawingTakesTheTokenShownBelowTheLineSayingItsMade(t *testing.T) {
	for _, final := range []bool{false, true} {
		got, ok := madeScreen(" " + madeToken).token(final)

		if !ok || got.Reveal() != madeToken {
			t.Errorf("token(%v) = %q, %v; want the token shown", final, got.Reveal(), ok)
		}
	}
}

func TestDrawingTakesNoTokenItCantReadWhole(t *testing.T) {
	made := madeScreen(" " + madeToken)
	tests := []struct {
		name    string
		drawing drawing
		// final says it's no token where setup-token has finished drawing.
		final bool
	}{
		{name: "none made", drawing: drawn(" Opening browser to sign in…", " "+madeToken), final: true},
		{name: "one shown before the line saying it's made", drawing: drawn(append([]string{" " + madeToken}, madeRows(" <token>")...)...), final: true},
		{name: "one broken across two rows", drawing: madeScreen(" " + madeToken[:60] + "\n " + madeToken[60:]), final: true},
		{name: "one cut short", drawing: madeScreen(" sk-ant-oat01-test-token"), final: true},
		{name: "one on a line with more", drawing: madeScreen(" token: " + madeToken), final: true},
		{name: "none shown", drawing: madeScreen(""), final: true},
		{name: "one drawn last", drawing: made[:5]},
		{name: "one with nothing drawn below it but blank rows", drawing: append(made[:5:5], nil, nil)},
		{name: "one with a line below it written before it", drawing: append(made[:5:5], writtenAt(" Paste code here if prompted >", 1))},
		{name: "one with a line below it written in part before it", drawing: append(made[:5:5], append(writtenAt(" 0123456789", 1000), writtenAt(" here if prompted >", 1)...))},
		{name: "one with a line below it written in part before it, beginning as the line setup-token draws there", drawing: append(made[:5:5], append(writtenAt(" Store this", 1000), writtenAt(" token", 1)...))},
		{name: "one cut short, the line setup-token draws below it written before it", drawing: append(madeScreen(" " + madeToken[:40])[:5:5], nil, writtenAt(" Store this token securely. You won't be able to see it again.", 1))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := tt.drawing.token(false); ok {
				t.Errorf("token(false) = %q, want none from %q", got.Reveal(), tt.drawing.text())
			}
			if got, ok := tt.drawing.token(true); ok && tt.final {
				t.Errorf("token(true) = %q, want none from %q", got.Reveal(), tt.drawing.text())
			}
		})
	}
}

// A renderer that writes only the characters that changed leaves the S of
// the line before in place.
func TestDrawingTakesTheTokenWithTheLineSetupTokenDrawsBelowIt(t *testing.T) {
	below := append(writtenAt(" S", 1), writtenAt("tore this token securely. You won't be able to see it again.", 1000)...)
	d := append(madeScreen(" " + madeToken)[:5:5], nil, below)

	if got, ok := d.token(false); !ok || got.Reveal() != madeToken {
		t.Errorf("token(false) = %q, %v; want the token from %q", got.Reveal(), ok, d.text())
	}
}

func TestDrawingTakesTheTokenSetupTokenFinishedDrawingWhereNothingBelowIsTheRestOfIt(t *testing.T) {
	made := madeScreen(" " + madeToken)
	tests := []struct {
		name    string
		drawing drawing
	}{
		{name: "one drawn last", drawing: made[:5]},
		{name: "one with nothing drawn below it but blank rows", drawing: append(made[:5:5], nil, nil)},
		{name: "one with a line below it written before it", drawing: append(made[:5:5], writtenAt(" Paste code here if prompted >", 1))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := tt.drawing.token(true); !ok || got.Reveal() != madeToken {
				t.Errorf("token(true) = %q, %v; want the token from %q", got.Reveal(), ok, tt.drawing.text())
			}
		})
	}
}

// Each renderer draws the made frame over the waiting one, the token whole on
// a row, or broken across two over a row that held other text, and the
// drawing's read after every byte, as a read can end anywhere.
func TestDrawingReadsATokenOnlyOnceItsFrameIsDrawnPastIt(t *testing.T) {
	for name, render := range renderers {
		for _, broken := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, broken %v", name, broken), func(t *testing.T) {
				token := " " + madeToken
				if broken {
					token = " " + madeToken[:60] + "\n " + madeToken[60:]
				}
				s := newScreen(columns, rows)
				s.write([]byte(frame(waitingRows)))
				for _, b := range []byte(render(waitingRows, madeRows(token))) {
					s.write([]byte{b})
					if got, ok := s.drawing().token(false); ok && (broken || got.Reveal() != madeToken) {
						t.Fatalf("token(false) = %q from %q, want none but the token whole", got.Reveal(), s.drawing().text())
					}
				}
				if got, ok := s.drawing().token(false); !broken && (!ok || got.Reveal() != madeToken) {
					t.Errorf("token(false) = %q, %v; want the token from %q", got.Reveal(), ok, s.drawing().text())
				}
			})
		}
	}
}

// Each renderer draws the token into the made frame drawn before it, without
// one, the line below it on the screen already, and the drawing's read after
// every byte.
func TestDrawingTakesNoTokenCutShortAboveTheLineSetupTokenDrewBeforeIt(t *testing.T) {
	for name, render := range renderers {
		t.Run(name, func(t *testing.T) {
			was := madeRows("")
			s := newScreen(columns, rows)
			s.write([]byte(frame(was)))
			for _, b := range []byte(render(was, madeRows(" "+madeToken))) {
				s.write([]byte{b})
				if got, ok := s.drawing().token(false); ok && got.Reveal() != madeToken {
					t.Fatalf("token(false) = %q from %q, want none but the token whole", got.Reveal(), s.drawing().text())
				}
			}
		})
	}
}

func TestDrawingCountsWhatsDrawnSinceItSaidTheTokensMade(t *testing.T) {
	tests := []struct {
		name    string
		drawing drawing
		want    int
	}{
		{name: "the made line alone", drawing: drawn(madeRows(" ")[0]), want: 1},
		{name: "a line drawn after it", drawing: drawn(madeRows(" ")[0], "", "ab c"), want: 4},
		{name: "a line drawn before it", drawing: append(drawing{writtenAt("before", 1)}, drawn(madeRows(" ")[0])...), want: 1},
		{name: "none made", drawing: drawn("ab"), want: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.drawing.drawnSinceMade(); got != tt.want {
				t.Errorf("drawnSinceMade() of %q = %d, want %d", tt.drawing.text(), got, tt.want)
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
		name    string
		drawing drawing
		want    bool
	}{
		{name: "an OAuth error", drawing: drawn(" OAuth error: Request failed with status code 400", "", " Press Enter to retry."), want: true},
		{name: "an error shown as it is", drawing: drawn(" The sign-in timed out.", "", " Press Enter to retry."), want: true},
		{name: "an OAuth error it doesn't wait at", drawing: drawn(" OAuth error: Request failed with status code 400")},
		{name: "none", drawing: drawn(" Opening browser to sign in…")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.drawing.failed(); got != tt.want {
				t.Errorf("failed() of %q = %v, want %v", tt.drawing.text(), got, tt.want)
			}
		})
	}
}
