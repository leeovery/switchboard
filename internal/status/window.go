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
	// WindowLimited is a window Claude's limit holds: it reads spent, or the
	// router's limit names it, or names no window. Its room is back At, zero
	// where that's unknown.
	WindowLimited
	// WindowStale is a window of an account whose token is refused, or whose
	// last read failed, last read At. A refused token is tried before
	// WindowNotStarted, and a failed read after it.
	WindowStale
	// WindowNotStarted is the window a request starts, lapsed, as HasLapsed
	// judges it: it isn't running, and reads empty, until a request starts it.
	WindowNotStarted
	// WindowCapped is a window whose use has reached its account's cap, which
	// holds the account back: its room is back At its reset, zero where that's
	// unknown.
	WindowCapped
	// WindowRunsOut is a window that runs out At, before it resets, as
	// RunsOut says.
	WindowRunsOut
	// WindowOpen is any other window: its allowance runs to its reset, At.
	WindowOpen
)

// WindowState is how a window of an account stands at a moment.
type WindowState struct {
	Kind WindowKind
	// Used is how much of the window is used, as it stands at the moment, as
	// Account.WindowAt has it: none once it has reset since it was read,
	// stale or not; none of one not started; and none where nothing is read
	// of it. Pace is its even pace, of the window so standing, as of when the
	// document was read, as paceOf has it: nil where it isn't running, its
	// reset isn't known, or nothing is read of it. They're the one reading of
	// the window that whatever draws its use or its pace draws.
	Used float64
	Pace *float64
	// At is when what Kind says comes, or came: zero where Kind says no time.
	At time.Time
	// Allowance is what can be spent of an open window, so standing, and
	// still last to its reset, At, as allowanceOf has it: as of when the
	// document was read, as its pace is, or, where it has reset since then,
	// from empty, as of its last reset. It's zero of any other kind, and
	// where nothing can be.
	Allowance quota.Allowance
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
// was built, as RunsOut's does, and so do their even paces and allowances, of
// the windows as they stand at now.
func (d Document) WindowStatesOf(a Account, now time.Time, policy score.Policy) WindowStates {
	hold, held := a.Held(now, policy)
	s := WindowStates{doc: d, account: a, now: now, policy: policy, hold: hold, held: held}
	if d.ReserveHolds(a) {
		s.capped = a.ReachedReserve(now)
	}
	return s
}

// Of is how the account's window with the given key stands: the first that
// holds of the kinds, in their order, which is ROUTING's, as RoutingOf says an
// account's state: no usable token; nothing read; a limit holding the window;
// its token refused; lapsed, not started, ahead of a failed read, as ROUTING
// says idle ahead of one once anything was read; its last read failed; the
// cap; running out; and open.
func (s WindowStates) Of(key string) WindowState {
	a := s.account
	w, reported := a.Window(key)
	switch {
	case !a.TokenSet:
		return WindowState{Kind: WindowTokenless}
	case !reported || a.FetchedAt.IsZero():
		return WindowState{Kind: WindowUnread}
	}
	lapsed := a.lapsed(w, s.now, s.policy)
	stands := windowAsOf(w, lapsed, s.now)
	st := WindowState{Used: stands.Utilization}
	st.Kind, st.At = s.standing(w, stands, lapsed)
	if pace, ok := s.doc.paceOf(stands, s.now); ok {
		st.Pace = &pace
	}
	if st.Kind == WindowOpen {
		st.Allowance = s.doc.allowanceOf(a, stands, s.now)
	}
	return st
}

// standing is the kind of the account's window w, read, as it stands, as
// stands, lapsed or not as given, and when what the kind says comes: the
// first that holds of a limit, its token refused, not started, its last read
// failed, the cap, running out, and open, its allowance running to its
// reset. It projects the window only where none before running out holds.
func (s WindowStates) standing(w, stands quota.Window, lapsed bool) (WindowKind, time.Time) {
	d, a, now := s.doc, s.account, s.now
	switch {
	case s.held && s.hold.Holds(w.Key):
		return WindowLimited, s.hold.Lifts(w.Key)
	case a.TokenRefused(now):
		return WindowStale, a.FetchedAt
	case lapsed:
		return WindowNotStarted, time.Time{}
	case a.Error != "":
		return WindowStale, a.FetchedAt
	case slices.Contains(s.capped, w.Key):
		return WindowCapped, stands.ResetsAt
	}
	if out, ok := d.RunsOut(a, w, now); ok {
		return WindowRunsOut, out.At
	}
	return WindowOpen, stands.ResetsAt
}
