package views

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/status"
)

// FactsReach is how long before the first day the Log's Events show they're
// read from: an event's facts are worked out over those told before it, and a
// move after midnight passing an account over under pressure says when that
// account runs out as its pressure event before midnight said, which a
// 5-hour window's run-out comes within.
const FactsReach = 5 * time.Hour

// EventsFrom is when the events the Log's Events show from the local day
// first falls on are read from: FactsReach before the day starts.
func EventsFrom(first time.Time) time.Time {
	return Today(first).Start.Add(-FactsReach)
}

// EventsRead are the router's events the Log's Events are built from, from
// when its span begins, each once, by run and id, as it last stands: those
// the router's files held as the span was read, and as each day it reached
// back to was, the files read once for each; and those the status document
// gave at each look since, the document's version in place of the files'.
// Those the document gave are kept as it gives newer ones, and as a router
// restarted tells its own, so none it gave is lost between looks. What
// changes it makes it afresh: a frame holding it never sees it change.
type EventsRead struct {
	since time.Time
	held  map[eventKey]Event
	// newest is the newest event the last document taken in gave, by its
	// router's run and its id, the id 0 where it gave none; looked is set
	// once a document is taken in.
	newest EventKey
	looked bool
}

// ReadEvents reads, through filed, the router's files' events of the span
// from EventsFrom of the local day first falls on to the end of now's day,
// each as it last stands.
func ReadEvents(filed Events, first, now time.Time) EventsRead {
	r := EventsRead{since: EventsFrom(first), held: make(map[eventKey]Event)}
	r.file(filed, r.since, dayfile.DayStart(now.Local(), 1))
	return r
}

// Since is when the events read begin.
func (r EventsRead) Since() time.Time {
	return r.since
}

// EventsFiled are what a read of the router's files for an EventsRead
// found, apart from the events held as it was asked, so those a look takes
// in while it's under way are kept as it lands, as Took has it: when the
// events read reach back to with them, and the events found, each as the
// files last hold it.
type EventsFiled struct {
	since  time.Time
	events []Event
}

// Took is r with what the files were found to hold taken in: reaching back
// as far as f does, where that's further, and each event found in place of
// the version held, those held besides kept as they stand.
func (r EventsRead) Took(f EventsFiled) EventsRead {
	took := r.clone()
	if f.since.Before(took.since) {
		took.since = f.since
	}
	for _, e := range f.events {
		took.hold(e)
	}
	return took
}

// From is r reaching back no further than EventsFrom of the local day
// given, as the Log no longer shows the days before it: the events before
// then let go of, those after kept as they stand.
func (r EventsRead) From(day LedgerDay) EventsRead {
	since := EventsFrom(day.Start)
	if !since.After(r.since) {
		return r
	}
	from := r.clone()
	from.since = since
	maps.DeleteFunc(from.held, func(_ eventKey, e Event) bool { return e.Event.At.Before(since) })
	return from
}

// Back is what r reaching back, through filed, to EventsFrom of the local
// day first falls on finds, where that's before it reaches, to be taken in
// as Took has it: the files' events from then up to when r began. It finds
// nothing where r reaches that far already.
func (r EventsRead) Back(filed Events, first time.Time) EventsFiled {
	since := EventsFrom(first)
	if !since.Before(r.since) {
		return EventsFiled{since: r.since}
	}
	return EventsFiled{since: since, events: filedBetween(filed, since, r.since)}
}

// Look is r with the events the status document m is a moment of gives,
// told by the router whose run began at run, each in place of the version
// held, as the router holds it now: those that happened before r begins
// passed over.
func (r EventsRead) Look(m status.Moment, run time.Time) EventsRead {
	looked, _ := r.Looked(m, run)
	return looked
}

// Looked is r with the events the status document m is a moment of gives
// taken in, as Look has it, reporting whether what it holds changed by them:
// false where each it gives is held already as it gives it, as status.Event
// Same has it, so what's built of r before stands, and r's events are its
// own still.
func (r EventsRead) Looked(m status.Moment, run time.Time) (EventsRead, bool) {
	newest := EventKey{Run: run}
	var fresh []Event
	for _, e := range m.Events {
		newest.ID = max(newest.ID, e.ID)
		given := Event{Key: EventKey{Run: run, ID: e.ID}, Event: e}
		if held, ok := r.held[given.Key.same()]; !e.At.Before(r.since) && (!ok || !held.Event.Same(e)) {
			fresh = append(fresh, given)
		}
	}
	looked := r
	if len(fresh) > 0 {
		looked = r.clone()
		for _, e := range fresh {
			looked.hold(e)
		}
	}
	looked.newest, looked.looked = newest, true
	return looked, len(fresh) > 0
}

// Skips reports whether the status document m is a moment of, told by the
// router whose run began at run, skips events the router told after those
// the last document r took in gave: its oldest comes more than one after
// their newest, of the same run, as the router numbers its events one after
// another as it runs, and gives its newest alone in its document, so those
// between are on its files alone. It's false where the last document taken
// in was another run's, or none was.
func (r EventsRead) Skips(m status.Moment, run time.Time) bool {
	if !r.looked || !r.newest.Run.Equal(run) || len(m.Events) == 0 {
		return false
	}
	oldest := slices.MinFunc(m.Events, func(a, b status.Event) int { return cmp.Compare(a.ID, b.ID) })
	return oldest.ID > r.newest.ID+1
}

// Refiled is what reading, through filed, the events the router filed that
// happened from from to the end of now's day finds, to be taken in as Took
// has it, each as the files last hold it in place of the version held: as
// the router stops, or restarts, those it filed after the last look at its
// document, which no later document tells of. Those before r begins are
// passed over.
func (r EventsRead) Refiled(filed Events, from, now time.Time) EventsFiled {
	if from.Before(r.since) {
		from = r.since
	}
	return EventsFiled{since: r.since, events: filedBetween(filed, from, dayfile.DayStart(now.Local(), 1))}
}

// Events are the events read, oldest first, by when each happened, then by
// run and id.
func (r EventsRead) Events() []Event {
	return slices.SortedFunc(maps.Values(r.held), func(a, b Event) int {
		return cmp.Or(a.Event.At.Compare(b.Event.At), a.Key.Run.Compare(b.Key.Run), cmp.Compare(a.Key.ID, b.Key.ID))
	})
}

// clone is r as a read of its own, which changing leaves r as it was.
func (r EventsRead) clone() EventsRead {
	held := maps.Clone(r.held)
	if held == nil {
		held = make(map[eventKey]Event)
	}
	return EventsRead{since: r.since, held: held, newest: r.newest, looked: r.looked}
}

// file holds the events filed reads as having happened from from up to to,
// each as it last stands, as filedBetween finds them.
func (r *EventsRead) file(filed Events, from, to time.Time) {
	for _, e := range filedBetween(filed, from, to) {
		r.hold(e)
	}
}

// filedBetween are the events filed reads as having happened from from up
// to to, in the order filed, so the last of an event's versions is as it
// last stands: none of a nil filed.
func filedBetween(filed Events, from, to time.Time) []Event {
	if filed == nil {
		return nil
	}
	var found []Event
	for line := range filed.Between(from, to) {
		found = append(found, Event{Key: EventKey{Run: line.Run, ID: line.ID}, Event: line.Event})
	}
	return found
}

// hold holds e in place of any version of its event held.
func (r *EventsRead) hold(e Event) {
	r.held[e.Key.same()] = e
}
