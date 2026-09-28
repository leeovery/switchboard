package router

import "context"

// Chooser picks the account a routed request goes out on.
type Chooser interface {
	// Choose picks an account for req. It always picks one: when nothing
	// better can take the request, the client's own account can.
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
}

// Choice is the account a request goes out on, and briefly why, for the log.
type Choice struct {
	Account string
	Reason  string
}
