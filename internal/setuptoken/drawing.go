package setuptoken

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/tokens"
)

// drawing is what setup-token's screen shows, a line a row.
type drawing []string

// text returns the drawing as a block of text.
func (d drawing) text() string {
	return strings.Join(d, "\n")
}

// made reports whether setup-token says it has made the token.
func (d drawing) made() bool {
	return d.madeAt() >= 0
}

// madeAt returns the index of the line saying the token's made, -1 where none
// does.
func (d drawing) madeAt() int {
	return slices.IndexFunc(d, func(line string) bool { return strings.Contains(line, claude.SetupTokenMade) })
}

// token returns the token shown below the line saying it's made: the first
// line after it that's a setup token whole, and not the start of one broken
// across two rows, the rest on the next, which isn't taken for one.
func (d drawing) token() (tokens.Token, bool) {
	at := d.madeAt()
	if at < 0 {
		return tokens.Token{}, false
	}
	for i := at + 1; i < len(d); i++ {
		text := strings.TrimSpace(d[i])
		if !claude.IsSetupToken(text) {
			continue
		}
		if i+1 < len(d) && claude.IsSetupTokenPart(strings.TrimSpace(d[i+1])) {
			return tokens.Token{}, false
		}
		token, err := tokens.Parse(text)
		return token, err == nil
	}
	return tokens.Token{}, false
}

// failed returns the error setup-token shows, which it waits to retry at:
// its message, without what begins it, or "" where it shows none but asks to
// retry all the same. It's false where setup-token isn't waiting to retry.
func (d drawing) failed() (string, bool) {
	if !slices.ContainsFunc(d, func(line string) bool { return strings.Contains(line, claude.SetupTokenRetry) }) {
		return "", false
	}
	for _, line := range d {
		if _, message, ok := strings.Cut(line, claude.SetupTokenError); ok {
			return strings.TrimSpace(message), true
		}
	}
	return "", true
}
