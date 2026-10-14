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
// as held counts its changes; what the stream told of each request's row,
// as logStream gives it and sameRows compares it; the filter; and the day.
// Neither the moment nor what's told as an answer streams is of it: a row
// in flight counts how long it has taken, and its characters, as it's
// drawn, as views.RequestRow.Live has it, so neither the clock moving nor
// the stream's progress makes anything afresh.
type requestsBasis struct {
	held   int
	told   streamTold
	filter views.LogFilter
	day    string
}

// same reports whether b is o, the Requests made of one as of the other.
func (b *requestsBasis) same(o requestsBasis) bool {
	return b != nil && b.held == o.held && b.told.sameRows(o.told) && b.filter.Equal(o.filter) && b.day == o.day
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

// sameRows reports whether s tells what o does of the requests' rows, as
// views.Told.SameRow compares each.
func (s streamTold) sameRows(o streamTold) bool {
	return s.live == o.live && maps.EqualFunc(s.requests, o.requests, views.Told.SameRow)
}

// sameSlice reports whether a and b are the very same slice: as long, and
// of the same elements, not alike ones.
func sameSlice[T any](a, b []T) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// requestsBasis is what the Log's Requests are made of at now, through the
// filter given.
func (m Model) requestsBasis(filter views.LogFilter, now time.Time) requestsBasis {
	return requestsBasis{held: m.log.held, told: toldBy(m.logStream(now)), filter: filter, day: views.Today(now).Date}
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
