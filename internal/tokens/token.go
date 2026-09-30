package tokens

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"unicode"

	"github.com/leeovery/switchboard/internal/redact"
)

var (
	// ErrMissing is what an account without a token fails with, wrapped: its
	// token file isn't there, or holds nothing but whitespace.
	ErrMissing = errors.New("token missing")
	// ErrEmpty is what an account whose token file is there, but holds
	// nothing but whitespace, fails with, wrapped, as a writer that empties
	// the file before it writes the token leaves it for a moment. It reads
	// as ErrMissing does, and wraps it.
	ErrEmpty = fmt.Errorf("%w", ErrMissing)
	// ErrNotAToken is what reading a token from text holding more than a
	// token fails with.
	ErrNotAToken = errors.New("more than a token")
)

// Token is an account's setup token. Printing or logging one shows a
// placeholder; only Reveal returns the secret.
type Token struct {
	// A pointer, not a string: where fmt bypasses String (%#v, %d, a Token in
	// an unexported field) it prints the field itself, and for a pointer that
	// is only an address. The redaction test leaks on all three with a string.
	secret *string
}

// Parse returns the token text holds, ignoring the whitespace around it. It
// fails with ErrMissing when text holds nothing else, and with ErrNotAToken
// when it holds more than one word: a token has no whitespace or control
// character in it, which would end or break the header it goes out in.
func Parse(text string) (Token, error) {
	secret := strings.TrimSpace(text)
	switch {
	case secret == "":
		return Token{}, ErrMissing
	case strings.ContainsFunc(secret, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }):
		return Token{}, ErrNotAToken
	}
	return Token{secret: &secret}, nil
}

// ParseFrom returns the token r holds, read to its end, as a token file holds
// one: as Parse does, but failing with ErrNotAToken for more than a token file
// can hold.
func ParseFrom(r io.Reader) (Token, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxFile+1))
	if err != nil {
		return Token{}, err
	}
	return parse(data)
}

// Reveal returns the secret itself, for passing on to whatever authenticates
// with it.
func (t Token) Reveal() string {
	if t.secret == nil {
		return ""
	}
	return *t.secret
}

// String returns a placeholder, never the secret.
func (t Token) String() string {
	return redact.Placeholder
}

// LogValue returns the same placeholder as String.
func (t Token) LogValue() slog.Value {
	return slog.StringValue(t.String())
}
