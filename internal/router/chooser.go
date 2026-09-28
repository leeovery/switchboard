package router

import (
	"context"
	"slices"
)

// Chooser picks the account a routed request goes out on.
type Chooser interface {
	// Choose picks an account for req. It always picks one: when nothing
	// better can take the request, the client's own account can, and the
	// choice says no account has room.
	Choose(ctx context.Context, req Request) Choice
}

// Request is what the router knows of a routed request when it chooses the
// account to send it on.
type Request struct {
	// Session is the id of the session the request belongs to, or "" when it
	// doesn't say.
	Session string
	// Model is the model the request asks for, or "" when it doesn't say.
	Model string
	// Pin is the account the request is pinned to, or "" when it isn't. It's
	// only ever an account with a token.
	Pin string
	// Client is the account whose token the client sent.
	Client string
	// Tried are the accounts the request has gone out on already, none of
	// which could serve it, in the order tried: choosing again, the choice is
	// among the rest.
	Tried []Attempt
}

// Attempt is an account a request went out on that couldn't serve it, and
// why.
type Attempt struct {
	Account string
	// Why says what went wrong on the account, as the reason for moving on
	// from it gives it, such as "hit its limit".
	Why string
}

// tried returns the ids of the accounts the request has gone out on.
func (r Request) tried() []string {
	ids := make([]string, len(r.Tried))
	for i, a := range r.Tried {
		ids[i] = a.Account
	}
	return ids
}

// attempt returns the request's attempt on the account with the given id, if
// it went out on it.
func (r Request) attempt(id string) (Attempt, bool) {
	i := slices.IndexFunc(r.Tried, func(a Attempt) bool { return a.Account == id })
	if i < 0 {
		return Attempt{}, false
	}
	return r.Tried[i], true
}

// Choice is the account a request goes out on, and briefly why, for the log.
type Choice struct {
	Account string
	Reason  string
	// NoRoom is set when no account has room for the request: Account is only
	// where it falls back to, for the upstream to answer.
	NoRoom bool
}
