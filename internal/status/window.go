package status

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// WindowKind is how a window of an account stands, as a WindowNow says.
type WindowKind int

// The kinds, in the order kindOf tries them, which is ROUTING's, as an
// account's Routing at a moment says its state: the first that holds is the
// one said.
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
	// runOutOf says.
	WindowRunsOut
	// WindowOpen is any other window: its allowance runs to its reset, At.
	WindowOpen
)

// windowStates are how the windows of an account of a document stand at a
// moment, as a policy judges them, and how the account stands then: what
// holds it back, which of its windows have reached its reserve, whether it's
// idle and whether it's under pressure, worked out once for them all.
type windowStates struct {
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
	// back is when the account is back from every hold that keeps it from
	// new work, as stood's back has it, and capLast is set where its cap is
	// the last of its quota's holds to lift.
	back    time.Time
	capLast bool
	// refusing is set where the upstream refuses any of its requests at now,
	// its token's or a model family's; idle where it's idle, as idleAt judges
	// it; and pressed where it's under pressure, as underPressure judges it.
	refusing, idle, pressed bool
}

// windowStatesOf are how doc's account a's windows stand at now, as policy
// judges them, as usage's words say them. Their projections go from when doc
// was built, as runOutOf's does, and so do their even paces and allowances, of
// the windows as they stand at now.
func (d Document) windowStatesOf(a Account, now time.Time, policy score.Policy) windowStates {
	s := windowStates{
		doc: d, account: a, now: now, policy: policy, reached: a.reachedReserve(now),
		refusing: a.Refused.holds(now), idle: a.idleAt(now, policy), pressed: a.underPressure(now),
	}
	s.stood = standing{lapsed: a.lapsedAt(now, policy), refused: a.tokenRefused(now)}
	s.stood.held, s.stood.limited = a.held(now, policy)
	s.stood.capped = d.reserveHolds(a) && slices.ContainsFunc(s.reached, policy.IsShared)
	if s.stood.capped {
		s.capBack, _ = score.LetGo(a.Windows, a.Reserve, policy.IsShared, now)
	}
	s.back, s.capLast = s.stood.back(s.capBack, a.Refused.Until)
	return s
}

// capped reports whether the account's window with the given key has reached
// its reserve at now, which holds the account back, as reserveHolds says.
func (s windowStates) capped(key string) bool {
	return s.doc.reserveHolds(s.account) && slices.Contains(s.reached, key)
}

// unknown is the kind of a window of the account nothing is known of,
// reporting false where something is: of an account without a usable token,
// and of one the account hasn't reported, as reported says, or nothing read.
func (s windowStates) unknown(reported bool) (WindowKind, bool) {
	switch {
	case !s.account.TokenSet:
		return WindowTokenless, true
	case !reported || s.account.FetchedAt.IsZero():
		return WindowUnread, true
	}
	return 0, false
}

// kindOf is the kind of the account's window, standing as j has it, and when
// what that says comes, as known says: of a window read, as kind says.
func (s windowStates) kindOf(j judged) (WindowKind, time.Time) {
	return s.known(j, s.kind)
}

// unheldOf is the kind of the account's window, standing as j has it, with
// any limit holding it set aside, as it stands once that lifts, and when what
// that says comes, as known says: of a window read, as unheld says.
func (s windowStates) unheldOf(j judged) (WindowKind, time.Time) {
	return s.known(j, s.unheld)
}

// known is the kind of the account's window, standing as j has it, and when
// what that says comes: of an account without a usable token, or nothing
// read, as unknown says, that alone; else as read says of the window read.
func (s windowStates) known(j judged, read func(quota.Window, judged) (WindowKind, time.Time)) (WindowKind, time.Time) {
	if kind, ok := s.unknown(true); ok {
		return kind, time.Time{}
	}
	return read(j.window, j)
}

// kind is the kind of the account's window w, read, standing as j has it, and
// when what the kind says comes: a limit holding it, until the hold lifts off
// it; else as unheld says.
func (s windowStates) kind(w quota.Window, j judged) (WindowKind, time.Time) {
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
func (s windowStates) unheld(w quota.Window, j judged) (WindowKind, time.Time) {
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
// allowance, as measure has them; where it runs out, as runOutOf says, runsOut
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
func (s windowStates) judge(w quota.Window) judged {
	j := judged{window: w, lapsed: !s.stood.running(w.Key)}
	j.stands = windowAsOf(w, j.lapsed, s.now)
	j.pace, j.allowance = s.doc.measure(s.account, s.stood, w.Key, j.stands, s.now)
	j.out, j.runsOut = s.doc.runOutOf(s.account, w, s.now)
	j.room = s.doc.poolRoom(s.account, s.stood, w, s.now, s.policy)
	return j
}

// reachedReserve are the keys of account a's windows that have reached its
// reserve at now, in Usage's order, as the router judges them afresh at each
// request: a window that has reset by now is none of them, though it isn't
// read again till later. Whatever says whether its cap holds judges it so, on
// the clock's tick, rather than as AtReserve noted it when the document was
// built.
func (a Account) reachedReserve(now time.Time) []string {
	return score.AtReserve(a.Windows, a.Reserve, now)
}

// Hold is what holds an account back at its limit: the keys of the windows
// it's at its limit in, as far as they're known, until when it is, and
// whether it's held back from every request.
type Hold struct {
	Windows []string
	// Until is when the hold lifts: zero where that's unknown. Its cap, or
	// its token refused, can hold the account back past it, as the
	// account's Back at a moment has it.
	Until time.Time
	// Every is set when it's held back from every request: at its limit in a
	// window every model shares, or with a limit naming none.
	Every bool
	// limit is when the router's limit lifts, while one holds, and named the
	// windows it names, none naming every window; spent is when each window
	// read spent resets, by key, zero where that's unknown.
	limit time.Time
	named []string
	spent map[string]time.Time
}

// Holds reports whether the hold holds the window with the given key: one it
// names, or any, for a limit naming none, which holds every window.
func (h Hold) Holds(key string) bool {
	return len(h.Windows) == 0 || slices.Contains(h.Windows, key)
}

// Lifts returns when the hold lifts off the window with the given key: the
// later of when the router's limit lifts, where it holds the window, and when
// the window resets, where it was read spent. It's zero where that isn't
// known, and for a window the hold doesn't hold.
func (h Hold) Lifts(key string) time.Time {
	reset, spent := h.spent[key]
	limited := !h.limit.IsZero() && (len(h.named) == 0 || slices.Contains(h.named, key))
	switch {
	case spent && reset.IsZero():
		return time.Time{}
	case limited:
		return score.Later(h.limit, reset)
	default:
		return reset
	}
}

// held returns what holds the account back at its limit at now, as policy
// says which windows every model shares, reporting false where nothing does:
// the limit the router saw it reach, while it holds, and every window read
// spent, until it resets, as probing reads them, those every model shares
// first. It's back once the last of what holds it back has lifted, as the
// router reckons it: of what holds back every request, where it's held back
// from every request.
func (a Account) held(now time.Time, policy score.Policy) (Hold, bool) {
	h := Hold{spent: make(map[string]time.Time)}
	if a.Limit.holds(now) {
		h.limit, h.named, h.Windows = a.Limit.Until, a.Limit.Windows, slices.Clone(a.Limit.Windows)
	}
	unnamed := !h.limit.IsZero() && len(h.named) == 0
	for _, w := range a.Windows {
		if score.Project(w, now).Kind != score.Exhausted {
			continue
		}
		h.spent[w.Key] = w.ResetsAt
		if !unnamed && !slices.Contains(h.Windows, w.Key) {
			h.Windows = append(h.Windows, w.Key)
		}
	}
	if h.limit.IsZero() && len(h.spent) == 0 {
		return Hold{}, false
	}
	slices.SortStableFunc(h.Windows, func(x, y string) int { return sharedFirst(policy, x, y) })
	h.Every = unnamed || slices.ContainsFunc(h.Windows, policy.IsShared)
	h.Until = h.back(policy)
	return h, true
}

// back is when the account is back from the hold, as policy says which
// windows every model shares: once the last of what holds it back has
// lifted, of what holds back every request where the hold does; zero where
// one of those lifts at no known time.
func (h Hold) back(policy score.Policy) time.Time {
	var last time.Time
	if !h.limit.IsZero() && (!h.Every || len(h.named) == 0 || slices.ContainsFunc(h.named, policy.IsShared)) {
		last = h.limit
	}
	for key, reset := range h.spent {
		if h.Every && !policy.IsShared(key) {
			continue
		}
		if reset.IsZero() {
			return time.Time{}
		}
		last = score.Later(last, reset)
	}
	return last
}
