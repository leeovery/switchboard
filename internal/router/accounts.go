package router

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/tokens"
)

// account is a configured account, with its token when it has a usable one.
type account struct {
	config.Account
	// secret holds the account's token, which every copy of the account
	// shares. It's nil for an account without a usable token: it's listed,
	// but nothing goes out on it.
	secret *secret
	// problem says why an account has no usable token.
	problem string
}

// secret is an account's token as the router holds it: read from its file as
// the router starts, and again when the upstream refuses it, so it can change
// while requests on the account are under way. Once it's replaced, it's
// still the account's for formerFor, as a former token.
type secret struct {
	mu    sync.Mutex
	token tokens.Token
	// former are the tokens the account had before this one, in the order
	// they were replaced.
	former []formerToken
}

func (s *secret) get() tokens.Token {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// is reports whether token is the account's token. The comparison takes as
// long however much of the token matches, so response times can't give it
// away a byte at a time.
func (s *secret) is(token string) bool {
	return subtle.ConstantTimeCompare([]byte(s.get().Reveal()), []byte(token)) == 1
}

// hasToken reports whether the account has a usable token, which requests
// can go out on.
func (a account) hasToken() bool {
	return a.secret != nil
}

// token returns the account's token as the router holds it now. The account
// must have one.
func (a account) token() tokens.Token {
	return a.secret.get()
}

// accounts are the configured accounts, in the order they're shown.
type accounts []account

// resolve reads each configured account's token, as read reads it, noting why
// for each account without a usable one.
func resolve(configured []config.Account, read func(id string) (tokens.Token, error)) accounts {
	resolved := make(accounts, len(configured))
	for i, c := range configured {
		resolved[i] = account{Account: c}
		if token, err := read(c.ID); err != nil {
			resolved[i].problem = err.Error()
		} else {
			resolved[i].secret = &secret{token: token}
		}
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

// byToken returns the account whose token is token, or was until it was
// replaced, while it still counts as the account's at now: a client started
// before carries on sending it. An account's token now wins over one another
// account had before. Each comparison takes as long however much of the token
// matches, so response times can't give a token away a byte at a time.
func (as accounts) byToken(token string, now time.Time) (account, bool) {
	if token == "" {
		return account{}, false
	}
	if a, ok := as.find(func(a account) bool { return a.secret.is(token) }); ok {
		return a, true
	}
	sum := hash(token)
	return as.find(func(a account) bool { return a.secret.was(sum, now) })
}

// find returns the first account with a token that match reports true of.
func (as accounts) find(match func(account) bool) (account, bool) {
	i := slices.IndexFunc(as, func(a account) bool { return a.hasToken() && match(a) })
	if i < 0 {
		return account{}, false
	}
	return as[i], true
}

// checkTokens fails when no account has a usable token, saying why of each.
func (as accounts) checkTokens() error {
	if slices.ContainsFunc(as, account.hasToken) {
		return nil
	}
	problems := make([]error, len(as))
	for i, a := range as {
		problems[i] = fmt.Errorf("%s: %s", a.ID, a.problem)
	}
	return fmt.Errorf("no account has a usable token, so there's nothing to route to:\n%w", errors.Join(problems...))
}

// sendable returns the accounts with a token, which requests can go out on.
func (as accounts) sendable() accounts {
	return slices.DeleteFunc(slices.Clone(as), func(a account) bool { return !a.hasToken() })
}

// only returns the accounts with the ids given, in order.
func (as accounts) only(ids []string) accounts {
	return slices.DeleteFunc(slices.Clone(as), func(a account) bool { return !slices.Contains(ids, a.ID) })
}

// canSend reports whether requests can go out on the account with the given
// id: there is one, and it has a token.
func (as accounts) canSend(id string) bool {
	a, ok := as.byID(id)
	return ok && a.hasToken()
}

// configured returns the accounts as the config gives them.
func (as accounts) configured() config.Accounts {
	configured := make(config.Accounts, len(as))
	for i, a := range as {
		configured[i] = a.Account
	}
	return configured
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
