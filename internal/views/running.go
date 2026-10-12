package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// Running is a session running on an account, as SESSIONS lists it: a row
// on each account its models go to, grouped by account in the config's
// order.
type Running struct {
	// Account is the account's id, and Session the session's.
	Account, Session string
	// Family is the family of the model it used last there, as "opus".
	Family string
	// LastSeen is when the router last saw it there.
	LastSeen time.Time
	// Why is why it's there, in the views' words for the router's reasons,
	// with when, as "started here 14:41, the best".
	Why status.Words
}

// RunningOf are the sessions running on doc's accounts at now, as the router
// lists them in sessions, a row on each account their models go to, the
// accounts in the config's order.
func RunningOf(doc status.Document, sessions []status.Session, now time.Time) []Running {
	return nil
}
