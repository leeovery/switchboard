package setuptoken

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/tokens"
)

// drawing is what setup-token's screen shows, a line a row.
type drawing []line

// line is a row of setup-token's screen: its text, and when it was last
// written to, as the screen counts, later the more.
type line struct {
	text    string
	written int
}

// text returns the drawing as a block of text.
func (d drawing) text() string {
	texts := make([]string, len(d))
	for i, l := range d {
		texts[i] = l.text
	}
	return strings.Join(texts, "\n")
}

// made reports whether setup-token says it has made the token.
func (d drawing) made() bool {
	return d.madeAt() >= 0
}

// madeAt returns the index of the line saying the token's made, -1 where none
// does.
func (d drawing) madeAt() int {
	return slices.IndexFunc(d, func(l line) bool { return strings.Contains(l.text, claude.SetupTokenMade) })
}

// token returns the token shown below the line saying it's made: the first
// line after it that's a setup token whole. It's whole only once something
// else is drawn below it after it, as setup-token draws the line after it in
// the same frame: the start of a token, cut short where setup-token was
// stopped drawing it, or broken across two rows, the rest on the next, is
// shaped as a token too, and a frame drawn over the last in place can leave a
// row of the last below the start.
func (d drawing) token() (tokens.Token, bool) {
	at := d.madeAt()
	if at < 0 {
		return tokens.Token{}, false
	}
	for i := at + 1; i < len(d); i++ {
		text := strings.TrimSpace(d[i].text)
		if !claude.IsSetupToken(text) {
			continue
		}
		if !d.endsAt(i) {
			return tokens.Token{}, false
		}
		token, err := tokens.Parse(text)
		return token, err == nil
	}
	return tokens.Token{}, false
}

// endsAt reports whether the token on the line at index at is drawn whole:
// the first line below it that isn't blank is there, was written after it,
// and isn't the rest of the token.
func (d drawing) endsAt(at int) bool {
	for _, l := range d[at+1:] {
		if text := strings.TrimSpace(l.text); text != "" {
			return l.written > d[at].written && !claude.IsSetupTokenPart(text)
		}
	}
	return false
}

// failed returns the error setup-token shows, which it waits to retry at:
// its message, without what begins it, or "" where it shows none but asks to
// retry all the same. It's false where setup-token isn't waiting to retry.
func (d drawing) failed() (string, bool) {
	if !slices.ContainsFunc(d, func(l line) bool { return strings.Contains(l.text, claude.SetupTokenRetry) }) {
		return "", false
	}
	for _, l := range d {
		if _, message, ok := strings.Cut(l.text, claude.SetupTokenError); ok {
			return strings.TrimSpace(message), true
		}
	}
	return "", true
}
