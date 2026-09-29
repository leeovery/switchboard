package config

import (
	"log/slog"
	"strings"

	"github.com/leeovery/switchboard/internal/redact"
)

// Token is an account's setup token. Printing or logging one shows a
// placeholder; only Reveal returns the secret.
type Token struct {
	// A pointer, not a string: where fmt bypasses String (%#v, %d, a Token in
	// an unexported field) it prints the field itself, and for a pointer that
	// is only an address. The redaction test leaks on all three with a string.
	secret *string
}

// Token reads the account's token from its token_env variable, trimming
// surrounding whitespace. It reports false when the variable is unset or blank.
func (a Account) Token(getenv func(string) string) (Token, bool) {
	secret := strings.TrimSpace(getenv(a.TokenEnv))
	if secret == "" {
		return Token{}, false
	}
	return Token{secret: &secret}, true
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
