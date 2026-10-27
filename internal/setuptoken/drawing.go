package setuptoken

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/tokens"
)

// drawing is what setup-token's screen shows, a line a row.
type drawing []line

// line is a row of setup-token's screen, a cell a column, without the blanks
// that end it.
type line []cell

// text returns the line's text.
func (l line) text() string {
	runes := make([]rune, len(l))
	for i, c := range l {
		runes[i] = c.r
	}
	return string(runes)
}

// first returns when the first of the line's characters but its blanks was
// written, 0 where it has none.
func (l line) first() int {
	first := 0
	for _, c := range l {
		if c.r != ' ' && (first == 0 || c.written < first) {
			first = c.written
		}
	}
	return first
}

// last returns when the last of the line's cells was written.
func (l line) last() int {
	last := 0
	for _, c := range l {
		last = max(last, c.written)
	}
	return last
}

// drawnSince returns how many of the line's characters but its blanks were
// written at or after at.
func (l line) drawnSince(at int) int {
	n := 0
	for _, c := range l {
		if c.r != ' ' && c.written >= at {
			n++
		}
	}
	return n
}

// text returns the drawing as a block of text.
func (d drawing) text() string {
	texts := make([]string, len(d))
	for i, l := range d {
		texts[i] = l.text()
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
	return slices.IndexFunc(d, func(l line) bool { return strings.Contains(l.text(), claude.SetupTokenMade) })
}

// drawnSinceMade returns how many characters but blanks the drawing shows
// that were written since the line saying the token's made last was: as
// setup-token draws its frame, the more of it, the more; -1 where no line
// says it's made.
func (d drawing) drawnSinceMade() int {
	at := d.madeAt()
	if at < 0 {
		return -1
	}
	since, n := d[at].last(), 0
	for _, l := range d {
		n += l.drawnSince(since)
	}
	return n
}

// token returns the token shown below the line saying it's made: the first
// line after it that's a setup token whole. As setup-token draws, it's whole
// only once every character of the line below it but blanks is drawn after
// it, as setup-token draws the line after it in the same frame: the start of
// a token, cut short where setup-token was stopped drawing it, or broken
// across two rows, the rest on the next, is shaped as a token too, and a
// frame drawn over the last in place can leave the last's characters below
// the start. Once setup-token has finished drawing, by exiting by itself,
// what it drew is all there is, so where final, it's whole where nothing
// below it is the rest of it.
func (d drawing) token(final bool) (tokens.Token, bool) {
	at := d.madeAt()
	if at < 0 {
		return tokens.Token{}, false
	}
	for i := at + 1; i < len(d); i++ {
		text := strings.TrimSpace(d[i].text())
		if !claude.IsSetupToken(text) {
			continue
		}
		if !d.endsAt(i, final) {
			return tokens.Token{}, false
		}
		token, err := tokens.Parse(text)
		return token, err == nil
	}
	return tokens.Token{}, false
}

// endsAt reports whether the token on the line at index at is drawn whole, as
// token says.
func (d drawing) endsAt(at int, final bool) bool {
	for _, l := range d[at+1:] {
		if text := strings.TrimSpace(l.text()); text != "" {
			return (final || l.first() > d[at].last()) && !claude.IsSetupTokenPart(text)
		}
	}
	return final
}

// failed returns the error setup-token shows, which it waits to retry at:
// its message, without what begins it, or "" where it shows none but asks to
// retry all the same. It's false where setup-token isn't waiting to retry.
func (d drawing) failed() (string, bool) {
	if !slices.ContainsFunc(d, func(l line) bool { return strings.Contains(l.text(), claude.SetupTokenRetry) }) {
		return "", false
	}
	for _, l := range d {
		if _, message, ok := strings.Cut(l.text(), claude.SetupTokenError); ok {
			return strings.TrimSpace(message), true
		}
	}
	return "", true
}
