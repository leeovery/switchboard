package router

import (
	"slices"
	"time"
)

// Event is news from the router that something outside it may want to act
// on, such as by notifying the user: LimitReached, Moved, Refused or
// HealthChanged. Config.Events hears each as it happens.
type Event interface {
	event()
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

// Refused is the upstream refusing an account's token, answering with Status.
type Refused struct {
	Account string
	Status  int
}

// HealthChanged is the router turning unhealthy, saying why, or healthy
// again.
type HealthChanged struct {
	Healthy bool
	Reason  string
}

func (LimitReached) event()  {}
func (Moved) event()         {}
func (Refused) event()       {}
func (HealthChanged) event() {}

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
