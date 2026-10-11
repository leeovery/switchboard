package status

import (
	"time"

	"github.com/leeovery/switchboard/internal/score"
)

// WindowKind is how a window of an account stands, as a WindowState says.
type WindowKind int

// The kinds of WindowState, in the order WindowStateOf tries them: the first
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

// WindowStateOf is how doc's account a's window with the given key stands at
// now, as usage's words say it: the first that holds of the kinds, in their
// order. Its projection goes from when doc was built, as RunsOut's does, and
// so does the reset its allowance runs to.
func (d Document) WindowStateOf(a Account, key string, now time.Time) WindowState {
	w, reported := a.Window(key)
	switch {
	case !a.TokenSet:
		return WindowState{Kind: WindowTokenless}
	case !reported || a.FetchedAt.IsZero():
		return WindowState{Kind: WindowUnread}
	case a.TokenRefused(now) || a.Error != "":
		return WindowState{Kind: WindowStale, Used: w.Utilization, At: a.FetchedAt}
	}
	state := WindowState{Kind: WindowOpen, Used: score.Current(w, now).Utilization}
	// Which windows every model shares, which a policy says, has no say in
	// whether a hold holds a window, or when it lifts off it.
	hold, held := a.Held(now, score.Policy{})
	out, runsOut := d.RunsOut(a, w, now)
	switch {
	case held && hold.Holds(key):
		state.Kind, state.At = WindowLimited, hold.Lifts(key)
	case d.ReserveHolds(a) && score.HeldByReserve(w, a.Reserve) && !w.ResetBy(now):
		state.Kind, state.At = WindowCapped, w.ResetsAt
	case runsOut:
		state.Kind, state.At = WindowRunsOut, out.At
	case a.HasLapsed(w):
		state.Kind = WindowNotStarted
	default:
		state.At = score.Current(w, d.readAt(now)).ResetsAt
	}
	return state
}
