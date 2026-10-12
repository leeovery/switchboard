package status

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// WindowKind is how a window of an account stands, as a WindowState says.
type WindowKind int

// The kinds of WindowState, in the order WindowStates.Of tries them: the first
// that holds is the one said.
const (
	// WindowTokenless is a window of an account without a usable token.
	WindowTokenless WindowKind = iota + 1
	// WindowUnread is a window nothing has been read of: of an account
	// nothing has been read of, or one the account hasn't reported.
	WindowUnread
	// WindowStale is a window of an account whose token is refused, or whose
	// last read failed: as it was last read, at At.
	WindowStale
	// WindowLimited is a window Claude's limit holds: it reads spent, or the
	// router's limit names it, or names no window. Its room is back At, zero
	// where that's unknown.
	WindowLimited
	// WindowCapped is a window whose use has reached its account's cap, which
	// holds the account back: its room is back At its reset, zero where that's
	// unknown.
	WindowCapped
	// WindowRunsOut is a window that runs out At, before it resets, as
	// RunsOut says.
	WindowRunsOut
	// WindowNotStarted is the window a request starts, lapsed: it isn't running
	// until a request starts it.
	WindowNotStarted
	// WindowOpen is any other window: its allowance runs to its reset, At.
	WindowOpen
)

// WindowState is how a window of an account stands at a moment.
type WindowState struct {
	Kind WindowKind
	// Used is how much of the window is used: as it was last read, of a stale
	// one; else as it runs at the moment, none once it has reset since it was
	// read.
	Used float64
	// At is when what Kind says comes, or came: zero where Kind says no time.
	At time.Time
}

// WindowStates are how the windows of an account of a document stand at a
// moment, as a policy judges them, what holds it back at its limit, and which
// windows its cap holds, worked out once for them all.
type WindowStates struct {
	doc     Document
	account Account
	now     time.Time
	policy  score.Policy
	hold    Hold
	held    bool
	capped  []string
}

// WindowStatesOf are how doc's account a's windows stand at now, as policy
// judges them, as usage's words say them. Their projections go from when doc
// was built, as RunsOut's does, and so do the resets their allowances run to.
func (d Document) WindowStatesOf(a Account, now time.Time, policy score.Policy) WindowStates {
	hold, held := a.Held(now, policy)
	s := WindowStates{doc: d, account: a, now: now, policy: policy, hold: hold, held: held}
	if d.ReserveHolds(a) {
		s.capped = a.ReachedReserve(now)
	}
	return s
}

// Of is how the account's window with the given key stands: the first that
// holds of the kinds, in their order.
func (s WindowStates) Of(key string) WindowState {
	a, now := s.account, s.now
	w, reported := a.Window(key)
	switch {
	case !a.TokenSet:
		return WindowState{Kind: WindowTokenless}
	case !reported || a.FetchedAt.IsZero():
		return WindowState{Kind: WindowUnread}
	case a.TokenRefused(now) || a.Error != "":
		return WindowState{Kind: WindowStale, Used: w.Utilization, At: a.FetchedAt}
	}
	kind, at := s.standing(key, w)
	return WindowState{Kind: kind, Used: score.Current(w, now).Utilization, At: at}
}

// Unstarted reports whether the account's window with the given key stands
// not started, as Of has it: it has lapsed, as HasLapsed judges it, and
// nothing Of tries first holds, as the account's last read failing, or a
// limit holding the window, which no request can start. Whatever draws a
// window as not started, or leaves it out as not running, asks this.
func (s WindowStates) Unstarted(key string) bool {
	return s.Of(key).Kind == WindowNotStarted
}

// standing is the kind of the account's window w, with the given key, as it
// was read and stands, and when what the kind says comes: the first that
// holds of a limit, the cap, running out, not started, and open. It projects
// the window only once neither a limit nor the cap holds it.
func (s WindowStates) standing(key string, w quota.Window) (WindowKind, time.Time) {
	d, a, now := s.doc, s.account, s.now
	switch {
	case s.held && s.hold.Holds(key):
		return WindowLimited, s.hold.Lifts(key)
	case slices.Contains(s.capped, key):
		return WindowCapped, w.ResetsAt
	}
	if out, ok := d.RunsOut(a, w, now); ok {
		return WindowRunsOut, out.At
	}
	if a.HasLapsed(key, now, s.policy) {
		return WindowNotStarted, time.Time{}
	}
	return WindowOpen, score.Current(w, d.readAt(now)).ResetsAt
}
