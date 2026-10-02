package router

import (
	"slices"
	"time"
)

// Event is news from the router that something outside it may want to act
// on, such as by notifying the user: SessionStarted, LimitReached, Moved,
// Refused, HealthChanged, Primed or RestartDue. Config.Events hears each as
// it happens.
type Event interface {
	event()
}

// SessionStarted is a session first remembered, for its requests of Model:
// its first answered with success, on Account, where it went as the routed
// line's Reason says.
type SessionStarted struct {
	Session string
	Model   string
	Account string
	Reason  string
}

// LimitReached is an account's limit reached, as the upstream answered a
// request on it: Windows are the keys of those it's reached in, which can be
// none when only its overall verdict said so, and Until is when the account is
// to have room again. Again is set when the account's last limit still held:
// it's that limit, reached again, which now holds as Windows and Until say.
type LimitReached struct {
	Account string
	Windows []string
	Until   time.Time
	Again   bool
}

// Moved is a session's requests of a model moving to another account, and
// why, as the routed line's reason says. Forced is set when From couldn't
// take the request, as when it reached its limit, so the session had to
// move, rather than moving by choice, as after an idle hour or by pin.
type Moved struct {
	Session string
	Model   string
	From    string
	To      string
	Reason  string
	Forced  bool
}

// Refused is the upstream refusing a request on an account, answering with
// Status: its token, which holds back every request, or, when Family is set,
// the request alone, which holds back the requests of that model family,
// either until Until.
type Refused struct {
	Account string
	Status  int
	Family  string
	Until   time.Time
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

// join takes in the account reaching its limit again while it holds: in
// other windows, perhaps, and until later.
func (m *limitMoves) join(e LimitReached) {
	for _, key := range e.Windows {
		if !slices.Contains(m.Windows, key) {
			m.Windows = append(m.Windows, key)
		}
	}
	m.Until = e.Until
}

// add takes in a session the limit moved.
func (m *limitMoves) add(e Moved) {
	if !slices.Contains(m.sessions, e.Session) {
		m.sessions = append(m.sessions, e.Session)
	}
	if !slices.Contains(m.to, e.To) {
		m.to = append(m.to, e.To)
	}
}
