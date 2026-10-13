package views

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/status"
)

// EventKey tells one of the router's events apart from every other, as its
// files do: Run, when the router that told it started, as its GET /health
// gives started_at, and its ID, which starts again from 1 as each router
// starts.
type EventKey struct {
	Run time.Time
	ID  int
}

// After reports whether the event k keys was told after the one o keys: by
// a router started later, or by the same one, later. Every event is told
// after the zero key's.
func (k EventKey) After(o EventKey) bool {
	if c := k.Run.Compare(o.Run); c != 0 {
		return c > 0
	}
	return k.ID > o.ID
}

// same is the key as a map holds it: its run to the nanosecond, as two
// readings of one router's start, in different time zones, are one run.
func (k EventKey) same() eventKey {
	return eventKey{run: k.Run.UnixNano(), id: k.ID}
}

// eventKey is an EventKey as a map holds it.
type eventKey struct {
	run int64
	id  int
}

// Event is one of the router's events as EventsSince gives it: its key, and
// the event as it last stands, which views tell.
type Event struct {
	Key   EventKey
	Event status.Event
}

// EventsSince are the router's events that happened from since on, as the
// Log's Events and the watch's news read them: those the status document
// gives, as the moment m has them, told by the router running, whose run
// began at run, merged with those its files hold, as filed reads them up to
// the end of m's day, by run and id, each once, as it last stands, the
// document's in place of the files' as the router holds it now; oldest
// first, by when each happened, then by run and id. A nil filed reads no
// files.
func EventsSince(since time.Time, m status.Moment, run time.Time, filed Events) []Event {
	held := make(map[eventKey]Event)
	hold := func(e Event) { held[e.Key.same()] = e }
	if filed != nil {
		for line := range filed.Between(since, dayfile.DayStart(m.Now.Local(), 1)) {
			hold(Event{Key: EventKey{Run: line.Run, ID: line.ID}, Event: line.Event})
		}
	}
	for _, e := range m.Events {
		if !e.At.Before(since) {
			hold(Event{Key: EventKey{Run: run, ID: e.ID}, Event: e})
		}
	}
	return slices.SortedFunc(maps.Values(held), func(a, b Event) int {
		return cmp.Or(a.Event.At.Compare(b.Event.At), a.Key.Run.Compare(b.Key.Run), cmp.Compare(a.Key.ID, b.Key.ID))
	})
}
