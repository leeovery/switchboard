package watch

import (
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// logBuilt is what each of the Log's builds was last made of, none where
// it's not been made: its Requests, its Events, and while it's open, its
// panel. None is made again till what it's made of changes, so a frame, a
// tick or the pointer moving makes nothing afresh.
type logBuilt struct {
	requests *requestsBasis
	events   *eventsBasis
	panel    *panelBasis
}

// requestsBasis is what the Log's Requests are made of: what the Log holds,
// as held counts its changes; what the stream told, as logStream gives it;
// the filter; and the moment, as toldAt has it.
type requestsBasis struct {
	held   int
	told   streamTold
	filter views.LogFilter
	at     time.Time
}

// same reports whether b is o, the Requests made of one as of the other.
func (b *requestsBasis) same(o requestsBasis) bool {
	return b != nil && b.held == o.held && b.told.same(o.told) && b.filter.Equal(o.filter) && b.at.Equal(o.at)
}

// eventsBasis is what the Log's Events are made of: what the Log holds, as
// held counts its changes; the filter; and the day.
type eventsBasis struct {
	held  int
	shows views.EventFilter
	day   string
}

// same reports whether b is o, the Events made of one as of the other.
func (b *eventsBasis) same(o eventsBasis) bool {
	return b != nil && b.held == o.held && b.shows.Equal(o.shows) && b.day == o.day
}

// panelBasis is what the Log's panel's figures are made of: what the Log
// holds, as held counts its changes; what the stream told; the table it's
// of, what it picks and the sessions it ticks; the sessions the router
// lists, as the look that listed them gave them, and the one the find
// picked; and the minute, which its sessions' doing is told at.
type panelBasis struct {
	held   int
	told   streamTold
	of     dashboard.LogView
	filter views.LogFilter
	shows  views.EventFilter
	ticked []string
	listed []status.Session
	found  *views.FoundSession
	minute time.Time
}

// same reports whether b is o, the panel's figures made of one as of the
// other.
func (b *panelBasis) same(o panelBasis) bool {
	return b != nil && b.held == o.held && b.told.same(o.told) && b.of == o.of && b.filter.Equal(o.filter) && b.shows.Equal(o.shows) &&
		slices.Equal(b.ticked, o.ticked) && sameSlice(b.listed, o.listed) && b.found == o.found && b.minute.Equal(o.minute)
}

// streamTold is what the stream told, as a build of the Log's reads it:
// whether it's live, and the requests it told of.
type streamTold struct {
	live     bool
	requests map[string]views.Told
}

// toldBy is what s tells a build of the Log's.
func toldBy(s views.Stream) streamTold {
	return streamTold{live: s.Live, requests: s.Requests}
}

// same reports whether s tells what o does.
func (s streamTold) same(o streamTold) bool {
	return s.live == o.live && maps.Equal(s.requests, o.requests)
}

// toldAt is the moment the Requests are made at, at now, s being what the
// stream tells, as fine as they tell it: to a tenth of a second while a
// request s tells of is in flight, its row counting how long it's taken, to
// a tenth; else the day.
func toldAt(s views.Stream, now time.Time) time.Time {
	if s.Live && anyInFlight(s.Requests) {
		return now.Truncate(time.Second / 10)
	}
	return views.Today(now).Start
}

// anyInFlight reports whether any of the requests told of is in flight.
func anyInFlight(requests map[string]views.Told) bool {
	for _, t := range requests {
		if t.InFlight() {
			return true
		}
	}
	return false
}

// sameSlice reports whether a and b are the very same slice: as long, and
// of the same elements, not alike ones.
func sameSlice[T any](a, b []T) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// requestsBasis is what the Log's Requests are made of at now, through the
// filter given.
func (m Model) requestsBasis(filter views.LogFilter, now time.Time) requestsBasis {
	s := m.logStream(now)
	return requestsBasis{held: m.log.held, told: toldBy(s), filter: filter, at: toldAt(s, now)}
}

// eventsBasis is what the Log's Events are made of at now, through the
// filter given.
func (m Model) eventsBasis(shows views.EventFilter, now time.Time) eventsBasis {
	return eventsBasis{held: m.log.held, shows: shows, day: views.Today(now).Date}
}

// panelBasis is what the Log's panel p's figures are made of at now.
func (m Model) panelBasis(p dashboard.LogPanel, now time.Time) panelBasis {
	return panelBasis{
		held: m.log.held, told: toldBy(m.logStream(now)), of: p.Of, filter: p.Filter, shows: p.Shows, ticked: p.TickedSessions(),
		listed: m.sessions, found: m.log.find.picked, minute: now.Truncate(time.Minute),
	}
}
