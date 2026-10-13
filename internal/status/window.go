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
	// WindowNotStarted is the window a request starts, lapsed, as hasLapsed
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
// moment, as a policy judges them, and how the account stands then: what
// holds it back, which of its windows have reached its reserve, whether it's
// idle and whether it's under pressure, worked out once for them all.
type WindowStates struct {
	doc     Document
	account Account
	now     time.Time
	policy  score.Policy
	stood   standing
	// reached are the keys of the account's windows that have reached its
	// reserve at now, as reachedReserve has them, whether or not the global
	// pin spends it.
	reached []string
	// capBack is when the account's cap lets it go, where it holds it back
	// from every request, as stood has it: once the last of its windows
	// every model shares at its reserve resets, as score.LetGo has it; zero
	// where its cap doesn't hold, or that isn't known.
	capBack time.Time
	// refusing is set where the upstream refuses any of its requests at now,
	// its token's or a model family's; idle where it's idle, as idleAt judges
	// it; and pressed where it's under pressure, as underPressure judges it.
	refusing, idle, pressed bool
}

// WindowStatesOf are how doc's account a's windows stand at now, as policy
// judges them, as usage's words say them. Their projections go from when doc
// was built, as RunsOut's does, and so do their even paces and allowances, of
// the windows as they stand at now.
func (d Document) WindowStatesOf(a Account, now time.Time, policy score.Policy) WindowStates {
	s := WindowStates{
		doc: d, account: a, now: now, policy: policy, reached: a.reachedReserve(now),
		refusing: a.Refused.Holds(now), idle: a.idleAt(now, policy), pressed: a.underPressure(now),
	}
	s.stood = standing{lapsed: a.lapsedAt(now, policy), refused: a.TokenRefused(now)}
	s.stood.held, s.stood.limited = a.held(now, policy)
	s.stood.capped = d.reserveHolds(a) && slices.ContainsFunc(s.reached, policy.IsShared)
	if s.stood.capped {
		s.capBack, _ = score.LetGo(a.Windows, a.Reserve, policy.IsShared, now)
	}
	return s
}

// capped reports whether the account's window with the given key has reached
// its reserve at now, which holds the account back, as reserveHolds says.
func (s WindowStates) capped(key string) bool {
	return s.doc.reserveHolds(s.account) && slices.Contains(s.reached, key)
}

// Of is how the account's window with the given key stands: the first that
// holds of the kinds, in their order, which is ROUTING's, as an account's
// Routing at a moment says its state: no usable token; nothing read; a limit
// holding the window; its token refused; lapsed, not started, ahead of a
// failed read, as ROUTING says idle ahead of one once anything was read; its
// last read failed; the cap; running out; and open.
func (s WindowStates) Of(key string) WindowState {
	a := s.account
	w, reported := a.Window(key)
	if kind, ok := s.unknown(reported); ok {
		return WindowState{Kind: kind}
	}
	j := s.judge(w)
	st := WindowState{Used: j.stands.Utilization, Pace: j.pace}
	st.Kind, st.At = s.kind(w, j)
	if st.Kind == WindowOpen {
		st.Allowance = j.allowance
	}
	return st
}

// unknown is the kind of a window of the account nothing is known of,
// reporting false where something is: of an account without a usable token,
// and of one the account hasn't reported, as reported says, or nothing read.
func (s WindowStates) unknown(reported bool) (WindowKind, bool) {
	switch {
	case !s.account.TokenSet:
		return WindowTokenless, true
	case !reported || s.account.FetchedAt.IsZero():
		return WindowUnread, true
	}
	return 0, false
}

// kindOf is the kind of the account's window, standing as j has it, and when
// what that says comes: of an account without a usable token, or nothing
// read, as unknown says, that alone; else as kind says.
func (s WindowStates) kindOf(j judged) (WindowKind, time.Time) {
	if kind, ok := s.unknown(true); ok {
		return kind, time.Time{}
	}
	return s.kind(j.window, j)
}

// kind is the kind of the account's window w, read, standing as j has it, and
// when what the kind says comes: a limit holding it, until the hold lifts off
// it; else as unheld says.
func (s WindowStates) kind(w quota.Window, j judged) (WindowKind, time.Time) {
	if s.stood.limited && s.stood.held.Holds(w.Key) {
		return WindowLimited, s.stood.held.Lifts(w.Key)
	}
	return s.unheld(w, j)
}

// unheld is the kind of the account's window w, read, standing as j has it,
// with any limit holding it set aside, as it stands once that lifts, and when
// what the kind says comes: the first that holds of its token refused, not
// started, its last read failed, the cap, running out, and open, its
// allowance running to its reset.
func (s WindowStates) unheld(w quota.Window, j judged) (WindowKind, time.Time) {
	a, stood := s.account, s.stood
	switch {
	case stood.refused:
		return WindowStale, a.FetchedAt
	case j.lapsed:
		return WindowNotStarted, time.Time{}
	case a.Error != "":
		return WindowStale, a.FetchedAt
	case s.capped(w.Key):
		return WindowCapped, j.stands.ResetsAt
	case j.runsOut:
		return WindowRunsOut, j.out.At
	}
	return WindowOpen, j.stands.ResetsAt
}

// judged is how a window of an account stands at a moment, worked out once:
// as it stands, as WindowAt has it, lapsed or not; its even pace and its
// allowance, as measure has them; where it runs out, as RunsOut says, runsOut
// set where it does before it resets; and the room the pool counts it as
// having, as poolRoom has it. Whatever its account's token or read, it's
// worked out of the window as given, as the document's own pace, allowance
// and pool are.
type judged struct {
	window    quota.Window
	stands    quota.Window
	lapsed    bool
	pace      *float64
	allowance quota.Allowance
	out       RunOut
	runsOut   bool
	room      float64
}

// judge is how the account's window w, read, stands at now, as judged has it.
func (s WindowStates) judge(w quota.Window) judged {
	j := judged{window: w, lapsed: !s.stood.running(w.Key)}
	j.stands = windowAsOf(w, j.lapsed, s.now)
	j.pace, j.allowance = s.doc.measure(s.account, s.stood, w.Key, j.stands, s.now)
	j.out, j.runsOut = s.doc.RunsOut(s.account, w, s.now)
	j.room = s.doc.poolRoom(s.account, s.stood, w, s.now, s.policy)
	return j
}
