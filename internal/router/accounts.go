package router

import (
	"crypto/subtle"
	"net/http"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
)

// account is a configured account, with its token when the environment sets
// one.
type account struct {
	config.Account
	token config.Token
	// hasToken is false for an account whose token isn't set: it's listed,
	// but nothing goes out on it.
	hasToken bool
}

// accounts are the configured accounts, in the order they're shown.
type accounts []account

// resolve reads each configured account's token from the environment.
func resolve(configured []config.Account, getenv func(string) string) accounts {
	resolved := make(accounts, len(configured))
	for i, c := range configured {
		token, ok := c.Token(getenv)
		resolved[i] = account{Account: c, token: token, hasToken: ok}
	}
	return resolved
}

// byID returns the account with the given id.
func (as accounts) byID(id string) (account, bool) {
	i := slices.IndexFunc(as, func(a account) bool { return a.ID == id })
	if i < 0 {
		return account{}, false
	}
	return as[i], true
}

// byToken returns the account whose token is token. Each comparison takes as
// long however much of the token matches, so response times can't give a
// token away a byte at a time.
func (as accounts) byToken(token string) (account, bool) {
	i := slices.IndexFunc(as, func(a account) bool {
		return a.hasToken && subtle.ConstantTimeCompare([]byte(a.token.Reveal()), []byte(token)) == 1
	})
	if i < 0 {
		return account{}, false
	}
	return as[i], true
}

// anyToken reports whether any account has a token.
func (as accounts) anyToken() bool {
	return slices.ContainsFunc(as, func(a account) bool { return a.hasToken })
}

// sendable returns the accounts with a token, which requests can go out on.
func (as accounts) sendable() accounts {
	return slices.DeleteFunc(slices.Clone(as), func(a account) bool { return !a.hasToken })
}

// canSend reports whether requests can go out on the account with the given
// id: there is one, and it has a token.
func (as accounts) canSend(id string) bool {
	a, ok := as.byID(id)
	return ok && a.hasToken
}

// ids lists the accounts' ids.
func (as accounts) ids() []string {
	ids := make([]string, len(as))
	for i, a := range as {
		ids[i] = a.ID
	}
	return ids
}

// tokenEnvs lists the variables the accounts' tokens are read from.
func (as accounts) tokenEnvs() []string {
	envs := make([]string, len(as))
	for i, a := range as {
		envs[i] = a.TokenEnv
	}
	return envs
}

// bearer returns the token a request's Authorization header carries, or ""
// when it carries none.
func bearer(h http.Header) string {
	scheme, token, ok := strings.Cut(h.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
