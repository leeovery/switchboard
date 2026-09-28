package router

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// cacheLife is how long a session's prompt cache lasts unused: Claude Code
// asks for an hour on a subscription. A session idle for longer has nothing
// cached to lose by moving.
const cacheLife = time.Hour

// Why a request goes out on the account it does, as the log shows it.
const (
	reasonPinned      = "pinned"
	reasonMovedByPin  = "moved by pin"
	reasonSticky      = "sticky"
	reasonGlobalPin   = "pinned (global)"
	reasonNew         = "new"
	reasonUnsessioned = "unsessioned"
	reasonNoRoom      = "no account has room"
)

// situation is what's known when a request's account is chosen.
type situation struct {
	req Request
	now time.Time
	// current is the session's assignment for the request's model, when
	// assigned is set.
	current  assignment
	assigned bool
	// pin is the global pin, zero when there's none.
	pin      status.Pin
	accounts view
}

// decision is the account a request goes out on, and why.
type decision struct {
	account string
	reason  string
	// sticky is set when the session's assignment stands as it was.
	sticky bool
	// afresh is set when the account was chosen afresh, as a new session's
	// is, so fresher usage could change it.
	afresh bool
}

// decide chooses the account a request goes out on, in this order:
//
//  1. The session's own pin, while its account has room. A session that
//     yielded its pin at a limit stays where it went while its cache is warm.
//  2. The global pin's account, for a session assigned before a pin that
//     moves running sessions: once each, while the account has room.
//  3. The session's account, while its cache is warm and it has room.
//  4. Afresh: the global pin's account while it has room, else the account
//     whose quota most needs using, keeping a session that has idled on its
//     own account unless another is well ahead.
//  5. When no account has room, the session's account, else the client's.
func decide(s situation) decision {
	pin := s.req.Pin
	switch {
	case pin == "":
		return s.unpinned()
	case s.yielded():
		return s.keep()
	case s.accounts.room(pin):
		return decision{account: pin, reason: reasonPinned}
	}
	d := s.unpinned()
	if d.reason != reasonNoRoom {
		d.reason = "pin yields: " + pin + " has no room"
	}
	return d
}

// unpinned chooses as decide does from step 2 on.
func (s situation) unpinned() decision {
	switch {
	case s.moving():
		return decision{account: s.pin.Account, reason: reasonMovedByPin}
	case s.keepable():
		return s.keep()
	}
	return s.afresh()
}

// yielded reports whether the session left the account its own pin names,
// having found it without room, and can stay where it went: bringing it back
// would cost a cache rebuild for nothing.
func (s situation) yielded() bool {
	return s.current.Pin == s.req.Pin && s.current.Account != s.req.Pin && s.keepable()
}

// moving reports whether the global pin moves the session to its account: it
// moves running sessions, and was set after the session was assigned.
func (s situation) moving() bool {
	return s.assigned && s.pin.Move && s.current.AssignedAt.Before(s.pin.Since) &&
		s.current.Account != s.pin.Account && s.accounts.room(s.pin.Account)
}

// keepable reports whether the session can stay on its account: its cache
// there is warm, and there's room.
func (s situation) keepable() bool {
	return s.assigned && s.current.warm(s.now) && s.accounts.room(s.current.Account)
}

func (s situation) keep() decision {
	return decision{account: s.current.Account, reason: reasonSticky, sticky: true}
}

// afresh chooses the account of a new session, of one idle past its cache's
// life, or of one whose account has no room.
func (s situation) afresh() decision {
	if s.pin.Account != "" && s.accounts.room(s.pin.Account) {
		return decision{account: s.pin.Account, reason: reasonGlobalPin, afresh: true}
	}
	reason, preferred := s.why()
	if id, ok := s.accounts.pick(preferred); ok {
		return decision{account: id, reason: reason, afresh: true}
	}
	return decision{account: s.fallback(), reason: reasonNoRoom, afresh: true}
}

// why says why the account is being chosen afresh, and which account the
// choice prefers: a session that has idled keeps to its own when another is
// only a little ahead, so near-equal accounts don't trade places.
func (s situation) why() (reason, preferred string) {
	switch {
	case s.req.Session == "":
		return reasonUnsessioned, ""
	case !s.assigned:
		return reasonNew, ""
	case !s.current.warm(s.now):
		return "rescored after " + status.Countdown(s.current.LastSeen, s.now) + " idle", s.current.Account
	default:
		return "moved: " + s.current.Account + " has no room", ""
	}
}

// fallback is where a request goes when no account has room: the session's
// account, else the client's. The upstream refuses it there.
func (s situation) fallback() string {
	if s.assigned && s.accounts.has(s.current.Account) {
		return s.current.Account
	}
	return s.req.Client
}
