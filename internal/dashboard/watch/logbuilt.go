package watch

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// logBuilt is what each of the Log's builds was last made of, none where
// it's not been made: its Requests' lines, its Requests, its Events, and
// while it's open, its panel. None is made again till what it's made of
// changes, so a frame, a tick or the pointer moving makes nothing afresh.
type logBuilt struct {
	lines    *linesBasis
	requests *requestsBasis
	events   *eventsBasis
	panel    *panelBasis
}

// linesBasis is what the Log's Requests' lines are made of, as
// views.LogLinesOf makes them: as requestsBasis has it, but nothing the
// stream told, which places a line once alone, as it's first taken in.
type linesBasis struct {
	lined  int
	filter views.LogFilter
	day    string
}

// same reports whether b is o, the lines made of one as of the other.
func (b *linesBasis) same(o linesBasis) bool {
	return b != nil && b.lined == o.lined && b.filter.Equal(o.filter) && b.day == o.day
}

// requestsBasis is what the Log's Requests are made of: the lines and
// figures the Log holds, as lined counts their changes, and not its
// events, which a look changes; what the stream told of each request's row,
// as logTold gives it and sameRows compares it; the filter; and the day.
// Neither the moment nor what's told as an answer streams is of it: a row
// in flight counts how long it has taken, and its characters, as it's
// drawn, as views.RequestRow.Live has it, so neither the clock moving nor
// the stream's progress makes anything afresh.
type requestsBasis struct {
	lined  int
	told   streamTold
	filter views.LogFilter
	day    string
}

// same reports whether b is o, the Requests made of one as of the other.
func (b *requestsBasis) same(o requestsBasis) bool {
	return b != nil && b.lined == o.lined && b.told.sameRows(o.told) && b.filter.Equal(o.filter) && b.day == o.day
}

// eventsBasis is what the Log's Events are made of: what the Log holds, as
// held counts its changes; the store their rows are told by, as stored
// counts its changes; the filter; and the day.
type eventsBasis struct {
	held   int
	stored int
	shows  views.EventFilter
	day    string
}

// same reports whether b is o, the Events made of one as of the other.
func (b *eventsBasis) same(o eventsBasis) bool {
	return b != nil && b.held == o.held && b.stored == o.stored && b.shows.Equal(o.shows) && b.day == o.day
}

// panelBasis is what the Log's panel's figures are made of: what the Log
// holds of the table it's of, as lined counts the changes to its lines, of
// Requests', and held every change, of Events'; the store its sessions are
// told by, as stored counts its changes; what the stream told of
// the requests' rows, as logTold gives it and sameRows compares it, which
// says what its sessions are doing, an answer streaming on saying nothing
// more; the table it's of, what it picks and the sessions it ticks; the
// sessions the router lists, kept and compared as views.KeptListings and
// views.SameListings have them, so a look listing them the same makes
// nothing afresh, and the one the find picked; and the minute, which its
// sessions' doing is told at.
type panelBasis struct {
	held   int
	stored int
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
	return b != nil && b.held == o.held && b.stored == o.stored && b.told.sameRows(o.told) && b.of == o.of && b.filter.Equal(o.filter) && b.shows.Equal(o.shows) &&
		slices.Equal(b.ticked, o.ticked) && views.SameListings(b.listed, o.listed) && b.found == o.found && b.minute.Equal(o.minute)
}

// streamTold is what the stream told of the requests' rows, as a build of
// the Log's reads it, as logStream gives it: whether the router answers,
// the Log reading nothing of the stream while it doesn't; whether it's
// live; and how many times the requests' rows have changed, as the traffic
// counts them, so comparing two costs nothing of the requests themselves.
// What's told as an answer streams on is none of it: a row in flight draws
// that as it's drawn.
type streamTold struct {
	answering, live bool
	rowChanges      int
}

// logTold is what the stream tells a build of the Log's.
func (m Model) logTold() streamTold {
	if !m.answering() {
		return streamTold{}
	}
	t := m.traffic
	return streamTold{answering: true, live: t.live, rowChanges: t.rowChanges}
}

// sameRows reports whether s tells what o does of the requests' rows, as
// views.Told.SameRow compares each.
func (s streamTold) sameRows(o streamTold) bool {
	return s == o
}

// linesBasis is what the Log's Requests' lines are made of at now, through
// the filter given.
func (m Model) linesBasis(filter views.LogFilter, now time.Time) linesBasis {
	return linesBasis{lined: m.log.lined, filter: filter, day: views.Today(now).Date}
}

// requestsBasis is what the Log's Requests are made of at now, through the
// filter given.
func (m Model) requestsBasis(filter views.LogFilter, now time.Time) requestsBasis {
	return requestsBasis{lined: m.log.lined, told: m.logTold(), filter: filter, day: views.Today(now).Date}
}

// eventsBasis is what the Log's Events are made of at now, through the
// filter given.
func (m Model) eventsBasis(shows views.EventFilter, now time.Time) eventsBasis {
	return eventsBasis{held: m.log.held, stored: m.stored, shows: shows, day: views.Today(now).Date}
}

// panelBasis is what the Log's panel p's figures are made of at now.
func (m Model) panelBasis(p dashboard.LogPanel, now time.Time) panelBasis {
	held := m.log.lined
	if p.Of == dashboard.LogEvents {
		held = m.log.held
	}
	return panelBasis{
		held: held, stored: m.stored, told: m.logTold(), of: p.Of, filter: p.Filter, shows: p.Shows, ticked: p.TickedSessions(),
		listed: views.KeptListings(m.sessions), found: m.log.find.picked, minute: now.Truncate(time.Minute),
	}
}
