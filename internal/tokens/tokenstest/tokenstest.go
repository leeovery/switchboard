// Package tokenstest stands in for the token files, for tests that read the
// accounts' tokens without files of their own.
package tokenstest

import (
	"fmt"

	"github.com/leeovery/switchboard/internal/tokens"
)

// Files are the accounts' tokens, by account id, as their token files hold
// them. An account without a token in Files has none, as though its file
// were missing.
type Files map[string]string

// Read returns the token of the account with the given id, as tokens.Store's
// Read does, failing as Missing says for an account without one.
func (f Files) Read(id string) (tokens.Token, error) {
	secret, ok := f[id]
	if !ok {
		return tokens.Token{}, Missing(id)
	}
	return tokens.Parse(secret)
}

// Missing is the error Read fails with for the account with the given id,
// which has no token in Files.
func Missing(id string) error {
	return fmt.Errorf("%w: write it to tokens/%s", tokens.ErrMissing, id)
}
