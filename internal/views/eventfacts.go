package views

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// eventFacts work out the facts of the router's events that its tellers
// say, the Log's and LATELY's alike, over a run of events taken in oldest
// first, as they're told: when an account a reason passed over under
// pressure runs out, as its latest pressure event before said; the sessions
// a limit, a cap, a refusal or a hold moved, counted, and where to; and the
// windows each event began with, and those that joined it since, with when,
// each named by its key as name names it. Its zero value names a window as
// status.NameOfKey does. eventFacts aren't safe for concurrent use.
type eventFacts struct {
	name func(key string) string
	// pool labels a pool by its id, where there are several, as
	// Telling's NamingPools has it: nil where there are one or none.
	pool func(id string) string
	// runsOut is when each account the run's pressure events told of runs
	// out, by its id, cleaned, as the latest of them to say said it.
	runsOut map[string]time.Time
	// holds are the holds paired with their lifts, as holdLifts pairs them:
	// the zero holdPairs, as for events told one at a time, pairs none.
	holds holdPairs
}

// eventFact is what's told of an event beyond its own fields, as eventFacts
// work it out.
type eventFact struct {
	// runsOut is when the account the event's reason passed over under
	// pressure runs out, as that account's latest pressure event before it
	// said, where the event came before then: zero where it passed none over,
	// none of the run's pressure events said, or the run-out had come.
	runsOut time.Time
	// moved counts the sessions a limit, a cap, a refusal or a hold moved,
	// each once, and to is the account they went to, where they all went to
	// one: "" where they went to several, or it moved none.
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
	// lifted is when a hold lifted, as liftOf has it: zero for any other
	// event, and a hold not known to have lifted. holding is set of a hold
	// that is its account's last none tells the lift of, as holdLifts has
	// it, the one hold of the account that can be holding still.
	lifted  time.Time
	holding bool
	// pool is the label of the pool whose sessions' own pins a pin set or
	// cleared with force cleared, where there are several pools, as
	// eventFacts' pool labels it: "" where there are one or none, so it
	// cleared every session's.
	pool string
}

// joinedWindow is a window that joined an event after it began, named as a
// sentence has it, and when it joined.
type joinedWindow struct {
	name string
	at   time.Time
}

// told takes in e, the next event of the run, as take does, and returns its
// facts with when it lifted, where it's a hold, as liftOf has it, and
// whether it can be holding still.
func (f *eventFacts) told(e Event) eventFact {
	fact := f.take(e.Event)
	fact.lifted = f.liftOf(e)
	open, ok := f.holds.open[e.Event.Account]
	fact.holding = e.Event.Kind == status.EventHeld && ok && open == e.Key.same()
	return fact
}

// liftOf is when the hold e lifted: its until, where the router ended it in
// the run it began, within the days an event can change; else when the room
// again that told its lift came, as holdLifts pairs them; zero for any other
// event, and a hold not known to have lifted.
func (f *eventFacts) liftOf(e Event) time.Time {
	if e.Event.Kind != status.EventHeld {
		return time.Time{}
	}
	return cmp.Or(e.Event.Until, f.holds.lifts[e.Key.same()])
}

// holdPairs are the holds of a run of events paired with their lifts, as
// holdLifts pairs them: lifts, when each lifted, by its key; and open, each
// account's last hold none tells the lift of, by the account's id.
type holdPairs struct {
	lifts map[eventKey]time.Time
	open  map[string]eventKey
}

// holdLifts pairs each hold of events the router didn't end, as across a
// restart, or past the days an event can change, with its lift: the first
// room again of its account after it, by when each happened, that names the
// cause of a hold lifting, as the router tells a hold's lift. A hold none
// follows hasn't lifted, as far as events tell, and isn't among the lifts;
// nor is one a newer hold of its account follows first, which is never
// open, as the newer one tells the account held since.
func holdLifts(events []Event) holdPairs {
	lifts := make(map[eventKey]time.Time)
	open := make(map[string]eventKey)
	byTime := slices.SortedFunc(slices.Values(events), func(a, b Event) int {
		return cmp.Or(a.Event.At.Compare(b.Event.At), a.Key.Run.Compare(b.Key.Run), cmp.Compare(a.Key.ID, b.Key.ID))
	})
	for _, e := range byTime {
		switch held, ok := open[e.Event.Account]; {
		case e.Event.Kind == status.EventHeld && e.Event.Until.IsZero():
			open[e.Event.Account] = e.Key.same()
		case e.Event.Kind == status.EventHeld:
			delete(open, e.Event.Account)
		case e.Event.Kind == status.EventRoom && e.Event.Reason != "" && ok:
			lifts[held] = e.Event.At
			delete(open, e.Event.Account)
		}
	}
	return holdPairs{lifts: lifts, open: open}
}

// stoodAt are the events given, each told before until, as each stood
// then. It exists because the router files an event again each time it
// changes, as late as events.ChangesFor days on, and a read of its files
// keeps the last version, so what's told of a time gone would otherwise
// change with when it's read. Each field the router changes after it files
// an event is put back as it was at until, as far as the events tell it:
//   - a hold's until, set as the router lifts it, zero where it lifted at
//     until or after;
//   - the windows that joined a limit, a cap or a refusal at until or after,
//     gone from its windows and from when each joined;
//   - a move's ForcedBy and Limit, set as the event that forced it is told
//     after it, zero where that event isn't among those given;
//   - a limit's, a cap's, a refusal's and a hold's Count and To, counted
//     again from the moves given folded into it, as foldedInto has it, as
//     the router counts them: each session once, To where they all went to
//     one account.
//
// What can't be put back, as the files don't keep what it was, is accepted:
//   - a limit's, a cap's and a refusal's until, widened as it's reached
//     again or brought forward as it lifts: nothing told before until says
//     it;
//   - a window a limit that named none learned later, as the router notes
//     when a window joined only an event that named one: it shows, though
//     learned after until.
//
// A hold told late, as at a router's start, filed after until but dated
// before it from when it began, is told: what it says was true by then.
func stoodAt(events []Event, until time.Time) []Event {
	given := make(map[eventKey]bool, len(events))
	for _, e := range events {
		given[e.Key.same()] = true
	}
	stood := make([]Event, len(events))
	moved := make(map[eventKey]*movedBy)
	for i, e := range events {
		e.Event = forcedAt(e, given)
		e.Event = joinedBefore(e.Event, until)
		if e.Event.Kind == status.EventHeld && !e.Event.Until.Before(until) {
			e.Event.Until = time.Time{}
		}
		if into, ok := lineOf(e); ok {
			if moved[into] == nil {
				moved[into] = &movedBy{}
			}
			moved[into].add(e.Event.Session, e.Event.To)
		}
		stood[i] = e
	}
	for i, e := range stood {
		switch e.Event.Kind {
		case status.EventLimit, status.EventCap, status.EventRefused, status.EventHeld:
			stood[i].Event.Count, stood[i].Event.To = moved[e.Key.same()].moved()
		}
	}
	return stood
}

// forcedAt is the move e as the events given tell what forced it: its
// ForcedBy and Limit zero where the event each names, of its run, isn't
// among them; any other event as it is.
func forcedAt(e Event, given map[eventKey]bool) status.Event {
	s := e.Event
	if s.Kind != status.EventMoved {
		return s
	}
	named := func(id int) int {
		if given[EventKey{Run: e.Key.Run, ID: id}.same()] {
			return id
		}
		return 0
	}
	s.ForcedBy, s.Limit = named(s.ForcedBy), named(s.Limit)
	return s
}

// joinedBefore is e without the windows that joined it at until or after,
// as they hadn't then.
func joinedBefore(e status.Event, until time.Time) status.Event {
	late := func(key string) bool {
		at, ok := e.Joined[key]
		return ok && !at.Before(until)
	}
	if !slices.ContainsFunc(e.Windows, late) {
		return e
	}
	e.Windows = slices.DeleteFunc(slices.Clone(e.Windows), late)
	joined := maps.Clone(e.Joined)
	maps.DeleteFunc(joined, func(key string, _ time.Time) bool { return late(key) })
	e.Joined = joined
	return e
}

// movedBy are the sessions an event's moves moved, each once, and the
// accounts they went to, as the router counts them in the event.
type movedBy struct {
	sessions, to []string
}

// add takes in a session moved to the account to.
func (m *movedBy) add(session, to string) {
	if !slices.Contains(m.sessions, session) {
		m.sessions = append(m.sessions, session)
	}
	if !slices.Contains(m.to, to) {
		m.to = append(m.to, to)
	}
}

// moved is how many sessions moved, and the account they went to where
// they all went to one, "" otherwise: none of a nil m.
func (m *movedBy) moved() (count int, to string) {
	if m == nil {
		return 0, ""
	}
	if len(m.to) == 1 {
		to = m.to[0]
	}
	return len(m.sessions), to
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
	case status.EventLimit, status.EventCap, status.EventRefused, status.EventHeld:
		fact.moved, fact.to = e.Count, e.To
	case status.EventPressure:
		f.pressed(e)
	}
	if e.Force && f.pool != nil {
		fact.pool = f.pool(e.Pool)
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
