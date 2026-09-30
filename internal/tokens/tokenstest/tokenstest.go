// Package tokenstest stands in for the token files, for tests that read the
// accounts' tokens without files of their own.
package tokenstest

import (
	"errors"
	"fmt"

	"github.com/leeovery/switchboard/internal/tokens"
)

// Files are the accounts' tokens, by account id, as their token files hold
// them. An account without a token in Files has none, as though its file
// were missing, and one whose token is nothing but whitespace, such as "",
// has a file holding none, as though it were caught while it's rewritten.
type Files map[string]string

// Read returns the token of the account with the given id, as tokens.Store's
// Read does, failing as Missing says for an account without one, and as Empty
// says for one whose token is nothing but whitespace.
func (f Files) Read(id string) (tokens.Token, error) {
	secret, ok := f[id]
	if !ok {
		return tokens.Token{}, Missing(id)
	}
	token, err := tokens.Parse(secret)
	if errors.Is(err, tokens.ErrMissing) {
		return tokens.Token{}, Empty(id)
	}
	return token, err
}

// Missing is the error Read fails with for the account with the given id,
// which has no token in Files.
func Missing(id string) error {
	return fmt.Errorf("%w: write it to tokens/%s", tokens.ErrMissing, id)
}

// Empty is the error Read fails with for the account with the given id, whose
// token in Files is nothing but whitespace.
func Empty(id string) error {
	return fmt.Errorf("%w: write it to tokens/%s, which is empty", tokens.ErrEmpty, id)
}
