package router

import (
	"slices"
	"time"
)

// Event is news from the router that something outside it may want to act
// on, such as by notifying the user: SessionStarted, LimitReached, Moved,
// Refused, RefusalLifted, HealthChanged, Primed or RestartDue. Config.Events
// hears each as it happens.
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
// why, as the routed line's reason says. Limit is the identity of the limit
// that held the request back on From, as LimitReached gives it, where that's
// why the session moved: zero for a move by choice, as after an idle hour or
// by pin, and for one that had to be for anything else, as From's reserve or
// a refusal.
type Moved struct {
	Session string
	Model   string
	From    string
	To      string
	Reason  string
	Limit   int
}

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

func (SessionStarted) event() {}
func (LimitReached) event()   {}
func (Moved) event()          {}
func (Refused) event()        {}
func (RefusalLifted) event()  {}
func (HealthChanged) event()  {}
func (Primed) event()         {}
func (RestartDue) event()     {}

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
// while it holds joins it, and the sessions it has moved, each once, with the
// accounts they went to, each once, in the order they came.
type limitMoves struct {
	LimitReached
	sessions, to []string
}

func newLimitMoves(e LimitReached) *limitMoves {
	m := &limitMoves{LimitReached: e}
	m.Windows = slices.Clone(e.Windows)
	return m
}

// join takes in the limit reached again while it holds: in other windows,
// perhaps, and until later. The news of a limit can come after the news of
// it reached again, so it holds until the latest it's told of.
func (m *limitMoves) join(e LimitReached) {
	for _, key := range e.Windows {
		if !slices.Contains(m.Windows, key) {
			m.Windows = append(m.Windows, key)
		}
	}
	m.Until = later(m.Until, e.Until)
}

// add takes in a session the limit moved to the account with the id to.
func (m *limitMoves) add(session, to string) {
	if !slices.Contains(m.sessions, session) {
		m.sessions = append(m.sessions, session)
	}
	if !slices.Contains(m.to, to) {
		m.to = append(m.to, to)
	}
}
