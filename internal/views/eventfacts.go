package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// eventFacts work out the facts of the router's events that its tellers
// say, the Log's and LATELY's alike, over a run of events taken in oldest
// first, as they're told: when an account a reason passed over under
// pressure runs out, as its latest pressure event before said; the sessions
// a limit, a cap or a refusal moved, counted, and where to; and the windows
// each event began with, and those that joined it since, with when, each
// named by its key as name names it. Its zero value names a window as
// status.NameOfKey does. eventFacts aren't safe for concurrent use.
type eventFacts struct {
	name func(key string) string
	// runsOut is when each account the run's pressure events told of runs
	// out, by its id, cleaned, as the latest of them to say said it.
	runsOut map[string]time.Time
}

// eventFact is what's told of an event beyond its own fields, as eventFacts
// work it out.
type eventFact struct {
	// runsOut is when the account the event's reason passed over under
	// pressure runs out, as that account's latest pressure event before it
	// said, where the event came before then: zero where it passed none over,
	// none of the run's pressure events said, or the run-out had come.
	runsOut time.Time
	// moved counts the sessions a limit, a cap or a refusal moved, each
	// once, and to is the account they went to, where they all went to one:
	// "" where they went to several, or it moved none.
	moved int
	to    string
	// windows are the windows the event began with, in its order, each as
	// a sentence has it, as "5-hour", "week" or "Fable week": those it names
	// but those that joined it after it began, as a limit's or a cap's can,
	// which joined has, so what's said of it at its start is what held then.
	windows []string
	// joined are the windows that joined the event after it began, in its
	// order, each named as windows are, with when it joined.
	joined []joinedWindow
}

// joinedWindow is a window that joined an event after it began, named as a
// sentence has it, and when it joined.
type joinedWindow struct {
	name string
	at   time.Time
}

// take takes in e, the next event of the run, and returns its facts.
func (f *eventFacts) take(e status.Event) eventFact {
	fact := eventFact{runsOut: f.passedOver(e)}
	for _, key := range e.Windows {
		if at, ok := e.Joined[key]; ok {
			fact.joined = append(fact.joined, joinedWindow{name: f.windowOf(key), at: at})
		} else {
			fact.windows = append(fact.windows, f.windowOf(key))
		}
	}
	switch e.Kind {
	case status.EventLimit, status.EventCap, status.EventRefused:
		fact.moved, fact.to = e.Count, e.To
	case status.EventPressure:
		f.pressed(e)
	}
	return fact
}

// passedOver is when the account e's reason passed over under pressure runs
// out, as eventFact's runsOut says.
func (f *eventFacts) passedOver(e status.Event) time.Time {
	passed := status.PassedOver(e.Reason)
	if passed == "" {
		return time.Time{}
	}
	if until := f.runsOut[passed]; e.At.Before(until) {
		return until
	}
	return time.Time{}
}

// pressed takes in when the account a pressure event told of runs out,
// where it says.
func (f *eventFacts) pressed(e status.Event) {
	if e.Until.IsZero() {
		return
	}
	if f.runsOut == nil {
		f.runsOut = make(map[string]time.Time)
	}
	f.runsOut[prose.Clean(e.Account)] = e.Until
}

// windowOf names the window with the given key as a sentence has it, as
// status.InProse puts the name name gives, or status.NameOfKey where there's
// no name: "window" where there's no key.
func (f *eventFacts) windowOf(key string) string {
	switch {
	case key == "":
		return "window"
	case f.name == nil:
		return status.InProse(status.NameOfKey(key))
	}
	return status.InProse(f.name(key))
}
