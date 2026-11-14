package router

import (
	"context"
	"slices"
	"time"
)

// Chooser picks the account a routed request goes out on.
type Chooser interface {
	// Choose picks an account for req, of those in its pool. When nothing
	// better can take the request, it picks one for the upstream to refuse
	// it on, and the choice says no account has room; or it picks none, and
	// says so, when every account it could fall back to is held back by its
	// reserve alone, which the router never spends, when none in the pool
	// will take it, or when the request has been tried already.
	// A session is remembered on the account picked, and on none when none
	// is, but for the client's quota check, which no session is remembered
	// for.
	Choose(ctx context.Context, req Request) Choice
	// Answered notes that req was answered with success, as its answer
	// comes, by the account with the id given, where it went for reason: its
	// session's assignment for the request's model, made by req or another,
	// is told of as started once, as the first is answered, on that account,
	// as the assignment says while the session is still there, else as req
	// went there, another request having moved it since. The client's quota
	// check starts none.
	Answered(req Request, account, reason string)
	// Forget takes back the accounts chosen for req, as they came to nothing:
	// its session's assignment for the request's model goes back to what it
	// was before req, none for a session req's first choice said was new,
	// which is remembered once a request is answered with success, or when
	// req was refused on every account it went out on. An account chosen for
	// another request of the session since stands. It returns the account
	// the session is back on, "" when it has none, and reports whether it
	// took the accounts back: not when one chosen since stands.
	Forget(req Request) (string, bool)
	// OnAccount returns how long, at now, req's session has been on the
	// account with the id given, for its requests of the request's model:
	// since it came to the account, which a choice that keeps it there
	// doesn't start again. It's none where the session isn't remembered on
	// the account, as a request without a session never is.
	OnAccount(req Request, account string, now time.Time) time.Duration
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
	// Dir is the directory the session was started in, as run tells the
	// router, in the form the request ledger gives it, or "" when the request
	// doesn't say.
	Dir string
	// Pool is the id of the pool the request's header names, or "" where it
	// names none, as a request from a session started before pools: each
	// choice finds the request in that pool, else the first, as the pools
	// stand as it's made, so no choice mixes two readings of them.
	Pool string
	// Model is the model the request asks for, or "" when it doesn't say.
	Model string
	// Check is set when the request is the client's quota check, which asks
	// nothing of the model: it goes where it's chosen to go, but its session
	// is never remembered for it, so it neither starts the session nor moves
	// it.
	Check bool
	// Bound is set when the model's thinking is bound to the account that
	// produced it: moving the session loses its reasoning, so it stays on its
	// account, cache cold or not, while the account can serve it.
	Bound bool
	// Compacted is set when the request is the first of its session's main
	// conversation after Claude Code compacted it, which has little of its
	// cache left to lose: the moment a busy session's move is judged, as
	// decide says.
	Compacted bool
	// Pin is the account the request is pinned to, as the session was
	// launched with it, or "" when it isn't: a pin the session is given while
	// it runs passes over it. It's only ever an account with a token.
	Pin string
	// Client is the account whose token the client sent, which the request
	// falls back to only where it's in the request's pool.
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

// remembered reports whether the request's session is remembered on the
// account with the given id, chosen for the request: one is chosen, "" being
// none, the request names its session, and it isn't the client's quota
// check.
func (r Request) remembered(account string) bool {
	return r.Session != "" && account != "" && !r.Check
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
	// Pool is the id of the pool the choice found the request in, as the
	// request's line records it: "" in a config from before pools.
	Pool string
	// NoRoom is set when no account has room for the request: Account is only
	// where it falls back to, for the upstream to answer, or none, when
	// Reserved or Closed is set, the request has been tried already, or no
	// account the router can send on will take it.
	NoRoom bool
	// Reserved is set when the request goes out on no account, as every one
	// it could fall back to is held back by its reserve alone. Back is when
	// the first of them has room again, zero when that isn't known.
	Reserved bool
	Back     time.Time
	// Closed is set when the request goes out on no account, as none in its
	// pool will take it, and its client's account, which it would fall back
	// to last, isn't in the pool: switchboard answers it as at a limit,
	// sending nothing on another pool's account.
	Closed bool
	// New is set when the choice gave the request's session its first account
	// for the request's model: the session is new, and is remembered once the
	// request is answered, as Chooser's Forget says.
	New bool
	// From is the account the choice moved the request's session from, for
	// its requests of the request's model, as Moved tells of it: "" when it
	// didn't move it.
	From string
}
