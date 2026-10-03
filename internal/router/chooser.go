package router

import (
	"context"
	"slices"
	"time"
)

// Chooser picks the account a routed request goes out on.
type Chooser interface {
	// Choose picks an account for req. When nothing better can take the
	// request, it picks one for the upstream to refuse it on, and the choice
	// says no account has room; or it picks none, and says so, when every
	// account it could fall back to is held back by its reserve alone,
	// which the router never spends, or the request has been tried already.
	// A session is remembered on the account picked, and on none when none
	// is.
	Choose(ctx context.Context, req Request) Choice
	// Forget takes back the accounts chosen for req, as they came to nothing:
	// its session's assignment for the request's model goes back to what it
	// was before req, none for a session req's first choice said was new,
	// which is remembered once a request is answered with success, or when
	// req was refused on every account it went out on. An account chosen for
	// another request of the session since stands.
	Forget(req Request)
}

// Request is what the router knows of a routed request when it chooses the
// account to send it on.
type Request struct {
	// ID is the request's own, never "", which ties the choices made for it
	// together, as it does its lines in the log.
	ID string
	// Session is the id of the session the request belongs to, or "" when it
	// doesn't say.
	Session string
	// Model is the model the request asks for, or "" when it doesn't say.
	Model string
	// Bound is set when the model's thinking is bound to the account that
	// produced it: moving the session loses its reasoning, so it stays on its
	// account, cache cold or not, while the account can serve it.
	Bound bool
	// Pin is the account the request is pinned to, as the session was
	// launched with it, or "" when it isn't: a pin the session is given while
	// it runs passes over it. It's only ever an account with a token.
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
	// where it falls back to, for the upstream to answer, or none, when
	// Reserved is set, or the request has been tried already.
	NoRoom bool
	// Reserved is set when the request goes out on no account, as every one
	// it could fall back to is held back by its reserve alone. Back is when
	// the first of them has room again, zero when that isn't known.
	Reserved bool
	Back     time.Time
	// New is set when the choice gave the request's session its first account
	// for the request's model: the session is new, and is remembered once the
	// request is answered, as Chooser's Forget says.
	New bool
	// From is the account the choice moved the request's session from, for
	// its requests of the request's model, as Moved tells of it: "" when it
	// didn't move it.
	From string
}
