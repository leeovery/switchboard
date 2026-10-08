package router

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/score"
)

// Event is news from the router that something outside it may want to act
// on, such as by notifying the user: SessionStarted, LimitReached, Moved,
// Refused, RefusalLifted, HealthChanged, Primed, RestartDue, Pinned or
// Unpinned. Config.Events hears each as it happens.
type Event interface {
	event()
}

// SessionStarted is a session first remembered, for its requests of Model:
// the first of them answered with success, as its answer comes, on Account,
// where the session went as the routed line's Reason says.
type SessionStarted struct {
	Session string
	Model   string
	Account string
	Reason  string
}

// LimitReached is an account's limit reached, as the upstream answered a
// request on it: Windows are the keys of those it's reached in, which can be
// none when only its overall verdict said so, and Until is when the account is
// to have room again. Limit is the limit's identity, which it keeps while it
// holds, and a new limit takes afresh: one reached while none holds, or
// naming windows the one that holds names none of. Again is set when it's
// the limit that holds, reached again, which now holds as Windows and Until
// say.
type LimitReached struct {
	Account string
	Windows []string
	Until   time.Time
	Limit   int
	Again   bool
}

// Moved is a session's requests of a model moving to another account, and
// why, as the routed line's reason says. Held is what held the request back
// on From, as the choice that moved the session found it, where From couldn't
// take it: zero where it could, for a move by choice, as by pin, and where
// only From's windows left it no room, with no limit reached. Limit is the
// identity of the limit that held the request back, as LimitReached gives
// it, where Held is HeldByLimit, and zero otherwise.
type Moved struct {
	Session string
	Model   string
	From    string
	To      string
	Reason  string
	Held    Hold
	Limit   int
}

// Hold is what held a request back on an account it couldn't go out on,
// which forces a session there to move: zero for nothing.
type Hold int

const (
	// HeldByLimit is a limit the account reached.
	HeldByLimit Hold = iota + 1
	// HeldByReserve is the account's reserve, which its windows have
	// reached: the account is at its cap.
	HeldByReserve
	// HeldByRefusal is the upstream refusing the account's token, or the
	// request's model family on it.
	HeldByRefusal
)

// Refused is the upstream refusing the request with the id Request on an
// account, answering with Status: its token, which holds back every request,
// or, when Family is set, the request alone, which holds back the requests of
// that model family, either until Until, unless RefusalLifted tells of it
// lifting sooner.
type Refused struct {
	Account string
	Status  int
	Family  string
	Until   time.Time
	Request string
}

// RefusalLifted is a refusal on Account lifting before its time: of every
// request, its token refused, when Family is "", as the account goes out on
// another token, each such refusal in force; or of the requests of Family,
// as the request with the id Request, which the upstream refused there, was
// refused on every account it went out on, which says more of the request
// than of the accounts.
type RefusalLifted struct {
	Account string
	Family  string
	Request string
}

// HealthChanged is the router turning unhealthy, saying why, or healthy
// again.
type HealthChanged struct {
	Healthy bool
	Reason  string
}

// Primed is a prime starting an account's window with the key Window, which
// resets at ResetsAt, zero when the prime didn't read it.
type Primed struct {
	Account  string
	Window   string
	ResetsAt time.Time
}

// RestartDue is a restart falling due, for Reason, such as "config changed",
// as the status document's restart gives it.
type RestartDue struct {
	Reason string
}

// Pinned is routing set by hand: where Session is "", the global pin, to
// Accounts, in the order configured, Account the first of them, moving the
// running sessions too where Move is set, and having cleared every session's
// own pin where Force is; else that session's own pin, to Account. By says
// who set it.
type Pinned struct {
	Accounts    []string
	Account     string
	Session     string
	Move, Force bool
	By          By
}

// Unpinned is routing given back to the router: where Session is "", the
// global pin cleared, which named Accounts, none when only Force cleared
// anything, with every session's own pin where Force is set; else that
// session's own pin cleared, which named Account. By says who cleared it.
type Unpinned struct {
	Accounts []string
	Account  string
	Session  string
	Force    bool
	By       By
}

func (SessionStarted) event() {}
func (LimitReached) event()   {}
func (Moved) event()          {}
func (Refused) event()        {}
func (RefusalLifted) event()  {}
func (HealthChanged) event()  {}
func (Primed) event()         {}
func (RestartDue) event()     {}
func (Pinned) event()         {}
func (Unpinned) event()       {}

// hearing returns what hears each event with every one of listeners that
// isn't nil, in turn.
func hearing(listeners ...func(Event)) func(Event) {
	listeners = slices.DeleteFunc(listeners, func(l func(Event)) bool { return l == nil })
	return func(e Event) {
		for _, hear := range listeners {
			hear(e)
		}
	}
}

// limitMoves is a limit an account reached, as each time it's reached again
// while it holds joins it, and the sessions it has moved.
type limitMoves struct {
	LimitReached
	moves
}

func newLimitMoves(e LimitReached) *limitMoves {
	m := &limitMoves{LimitReached: e}
	m.Windows = slices.Clone(e.Windows)
	return m
}

// join takes in the limit reached again while it holds: in other windows,
// perhaps, and until later. The news of a limit can come after the news of
// it reached again, so it holds until the latest it's told of. It reports
// whether that changed the limit.
func (m *limitMoves) join(e LimitReached) bool {
	var changed bool
	m.Windows, m.Until, changed = widen(m.Windows, m.Until, e.Windows, e.Until)
	return changed
}

// widen returns windows with each of more that isn't among them after them,
// and the later of until and later, reporting whether that changed either:
// what holds an account back, reached again while it holds, in more windows
// perhaps, and till later.
func widen(windows []string, until time.Time, more []string, later time.Time) ([]string, time.Time, bool) {
	had, was := len(windows), until
	windows, until = withEach(windows, more...), score.Later(until, later)
	return windows, until, len(windows) != had || !until.Equal(was)
}

// withEach returns keys with each of more that isn't among them after them.
func withEach(keys []string, more ...string) []string {
	for _, key := range more {
		if !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys
}

// moves are the sessions a limit, a cap or a refusal has moved, each once,
// with the accounts they went to, each once, in the order they came.
type moves struct {
	sessions, to []string
}

// add takes in a session moved to the account with the id to, reporting
// whether it changed what the event of what moved it gives: how many
// sessions it moved, or where they went.
func (m *moves) add(session, to string) bool {
	count, went := m.moved()
	if !slices.Contains(m.sessions, session) {
		m.sessions = append(m.sessions, session)
	}
	if !slices.Contains(m.to, to) {
		m.to = append(m.to, to)
	}
	nowCount, nowWent := m.moved()
	return nowCount != count || nowWent != went
}

// moved returns how many sessions have moved, and the account they went to
// when they all went to one, "" otherwise.
func (m *moves) moved() (count int, to string) {
	if len(m.to) == 1 {
		to = m.to[0]
	}
	return len(m.sessions), to
}
