package router

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// cacheLife is how long a session's prompt cache lasts unused: Claude Code
// asks for an hour on a subscription. A session idle for longer has nothing
// cached to lose by moving.
const cacheLife = time.Hour

// Why a request goes out on the account it does, as the log shows it, those
// the dashboard reads among them, which status names.
const (
	reasonPinned      = status.ReasonPinned
	reasonMovedByPin  = status.ReasonMovedByPin
	reasonSticky      = "sticky"
	reasonBound       = "bound"
	reasonGlobalPin   = status.ReasonGlobalPin
	reasonNew         = status.ReasonNew
	reasonUnsessioned = "unsessioned"
	reasonNoRoom      = "no account has room"
)

// situation is what's known when a request's account is chosen.
type situation struct {
	// req is pinned as its session's own pin has it.
	req Request
	now time.Time
	// pinnedAt is when the session was given its own pin while it ran, zero
	// for the one it was launched with.
	pinnedAt time.Time
	// current is the session's assignment for the request's model, when
	// assigned is set.
	current  assignment
	assigned bool
	// pin is the global pin, zero when there's none.
	pin status.Pin
	// accounts are what's known of the accounts, among which those the
	// request has been tried on have no room.
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
	// passedOver is the account under pressure the choice passed over, when
	// passing over those under pressure changed it: "" when it didn't. pressure
	// is how it stood, as the choice judged it, at the room the choice gave it.
	passedOver string
	pressure   score.Pressure
	// noRoom is set when no account has room for the request, so account is
	// only where it falls back to, or none: when reserved is set, or the
	// request has been tried already.
	noRoom bool
	// reserved is set when the request goes out on no account, as every one
	// it could fall back to is held back by its reserve alone, which the
	// router never spends; back is when the first of them is let go, zero
	// when that isn't known.
	reserved bool
	back     time.Time
	// held is what holds the request back on the session's account, where
	// the choice moves the session off it, and limit the identity of the
	// limit, where that's what does, as view's holding says: what forces the
	// move.
	held  Hold
	limit int
}

// decide chooses the account a request goes out on, in this order, room
// ending at an account's reserve but where a pin spends it:
//
//  1. The session's own pin, while its account has room: the one it was
//     given while it ran, else the one it was launched with. A session that
//     yielded its pin at a limit stays where it went while step 3 would keep
//     it there, until it's given a pin again.
//  2. The best of the global pin's accounts, for a session on another
//     account, assigned before a pin that moves running sessions: once each,
//     while one of them has room.
//  3. The session's account, while it has room and its cache there is warm,
//     or its model's thinking is bound to it, which a move would lose.
//  4. Afresh: the best of the global pin's accounts while one has room, else
//     the account whose quota most needs using, of every account; either
//     way, a session that has idled keeps to its own account unless another
//     is well ahead, and those under pressure are passed over while another
//     isn't.
//  5. When no account has room, the session's account, else the client's,
//     else any, passing over those that refused the request lately and those
//     held back by their reserve alone; with none left but the latter, none.
//     A request tried already goes out on none.
//
// A choice that passing over those under pressure changed says so, naming
// the account passed over, after why it was made; and one that moves the
// session off its account says what held the request back there, if
// anything did.
func decide(s situation) decision {
	s.accounts = s.accounts.spend(s.req.Pin).spend(s.pin.Accounts...)
	d := s.choose()
	if d.passedOver != "" {
		d.reason += ", " + d.passedOver + " under pressure"
	}
	if s.assigned && d.account != s.current.Account {
		d.held, d.limit = s.accounts.holding(s.current.Account)
	}
	return d
}

// choose chooses as decide does, but for saying which account a choice passed
// over for pressure, and what held the request back on the session's.
func (s situation) choose() decision {
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
	if !d.noRoom {
		d.reason = status.ReasonPinYields + pin + " " + s.unable(pin)
	}
	return d
}

// unpinned chooses as decide does from step 2 on.
func (s situation) unpinned() decision {
	if to, ok := s.moving(); ok {
		return decision{account: to.ID, reason: reasonMovedByPin, passedOver: to.PassedOver, pressure: to.Pressure}
	}
	if s.keepable() {
		return s.keep()
	}
	return s.afresh()
}

// yielded reports whether the session left the account its own pin names,
// having found it without room, and can stay where it went: bringing it back
// would cost a cache rebuild for nothing, or its reasoning. A pin given since
// the session was last routed is the user's say, which it heeds.
func (s situation) yielded() bool {
	return s.current.Pin == s.req.Pin && s.current.LastSeen.After(s.pinnedAt) &&
		s.current.Account != s.req.Pin && s.keepable()
}

// moving returns the account the global pin moves the session to, the best of
// its accounts, as pinned says: it moves running sessions, it was set after
// the session was assigned, and the session's account isn't one of them. It
// reports false when the pin doesn't move the session, or none of its
// accounts has room.
func (s situation) moving() (score.Choice, bool) {
	if !s.assigned || !s.pin.Move || !s.current.AssignedAt.Before(s.pin.Since) || s.pin.Has(s.current.Account) {
		return score.Choice{}, false
	}
	return s.pinned("")
}

// keepable reports whether the session can stay on its account: there's room,
// and a move would cost the session something.
func (s situation) keepable() bool {
	return s.assigned && !s.movesFree() && s.accounts.room(s.current.Account)
}

// movesFree reports whether the session can move at no cost: its cache has
// gone cold, and a move loses none of its reasoning, as its model's thinking
// isn't bound to its account.
func (s situation) movesFree() bool {
	return !s.current.warm(s.now) && !s.req.Bound
}

// keep keeps the session on its account: sticky while its cache there is
// warm, and bound once it's cold, kept only for its model's thinking, which
// is bound to the account.
func (s situation) keep() decision {
	reason := reasonSticky
	if !s.current.warm(s.now) {
		reason = reasonBound
	}
	return decision{account: s.current.Account, reason: reason, sticky: true}
}

// afresh chooses the account of a new session, of one that can move at no
// cost, or of one whose account has no room: among the global pin's accounts
// while one has room, else among every account.
func (s situation) afresh() decision {
	reason, preferred := s.why()
	if c, ok := s.pinned(preferred); ok {
		return chosen(c, reasonGlobalPin)
	}
	if c, ok := s.accounts.pick(preferred); ok {
		return chosen(c, reason)
	}
	return s.noRoom()
}

// chosen is the decision to send the request to the account chosen afresh,
// for reason, and the account under pressure the choice passed over, if any,
// and how it stood.
func chosen(c score.Choice, reason string) decision {
	return decision{account: c.ID, reason: reason, afresh: true, passedOver: c.PassedOver, pressure: c.Pressure}
}

// pinned returns the account of the global pin's the request goes to: of
// those with room, the one whose quota most needs using, passing over those
// under pressure while another isn't, and keeping to preferred unless another
// is well ahead, else the first, as a pin sends requests to an account whose
// quota can't be scored, or that nothing has been read of. It reports false
// when none has room.
func (s situation) pinned(preferred string) (score.Choice, bool) {
	if c, ok := s.accounts.within(s.pin.Accounts).pick(preferred); ok {
		return c, true
	}
	i := slices.IndexFunc(s.pin.Accounts, s.accounts.room)
	if i < 0 {
		return score.Choice{}, false
	}
	return score.Choice{ID: s.pin.Accounts[i]}, true
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
	case s.movesFree():
		return "rescored after " + status.Countdown(s.current.LastSeen, s.now) + " idle", s.current.Account
	default:
		return status.ReasonMovedOff + s.current.Account + " " + s.unable(s.current.Account), ""
	}
}

// unable says why the account with the given id can't take the request: what
// went wrong when the request went out on it, else that it has reached its
// reserve, when that alone holds it back, else that it has no room.
func (s situation) unable(id string) string {
	switch a, tried := s.req.attempt(id); {
	case tried:
		return a.Why
	case s.accounts.reserved(id):
		return "is at its reserve"
	default:
		return "has no room"
	}
}

// noRoom is where a request goes when no account has room for it, for the
// upstream to refuse it there, saying why: the session's account, else the
// client's, else any other the view falls back to. With every one refused,
// it's the client's. With none left but those held back by their reserve
// alone, it's none, and switchboard answers for the upstream: the router never
// spends a reserve. A request tried already goes nowhere else: the upstream
// has said why on the accounts it was tried on.
func (s situation) noRoom() decision {
	d := decision{reason: reasonNoRoom, afresh: true, noRoom: true}
	if len(s.req.Tried) > 0 {
		return d
	}
	first := []string{s.req.Client}
	if s.assigned {
		first = []string{s.current.Account, s.req.Client}
	}
	if id, ok := s.accounts.fallback(first...); ok {
		d.account = id
		return d
	}
	if back, held := s.accounts.letGo(); held {
		d.reserved, d.back = true, back
		return d
	}
	d.account = s.req.Client
	return d
}
