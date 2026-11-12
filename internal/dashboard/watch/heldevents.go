package watch

import (
	"cmp"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// heldEvents are the router's events the watch holds for the Log's Events
// and Runway's LATELY, one read for both, so neither reads again a day of
// the router's files the other read: the read, once the first of them to
// show has read the files, each look since taken in, as looked has it; how
// many times it has changed, which what's built of it is made again by; who
// told the events the last look took in, and when; when the files are to be
// read again from for the events a document skipped, as Skips has it, none
// while none did; whether Runway's LATELY wants them, which keeps the week
// it tells held, and while it does, the events, oldest first, as the read's
// Events gives them, made once as it changes, as LATELY's built at every
// look; and whether a read of LATELY's is under way, one at a time, the
// Log's reads waiting on it, as it waits on theirs.
type heldEvents struct {
	read    views.EventsRead
	opened  bool
	changes int
	told    logTold
	skipped time.Time
	lately  bool
	events  []views.Event
	reading bool
}

// holding is e holding the read given, counted changed, its events made
// afresh while LATELY wants them.
func (e heldEvents) holding(read views.EventsRead) heldEvents {
	e.read, e.changes = read, e.changes+1
	if e.lately {
		e.events = read.Events()
	}
	return e
}

// wantedLately is e as Runway's LATELY wants it, its events made where
// they weren't.
func (e heldEvents) wantedLately() heldEvents {
	if !e.lately {
		e.lately, e.events = true, e.read.Events()
	}
	return e
}

// took is e with what a read of the router's files found taken in: the
// events read as it first read them, where opened gives them; else those
// found since, beside those held, as Took has it.
func (e heldEvents) took(opened *views.EventsRead, found *views.EventsFiled) heldEvents {
	switch {
	case opened != nil:
		e.opened = true
		return e.holding(*opened)
	case found != nil:
		return e.holding(e.read.Took(*found))
	}
	return e
}

// looked is e with the events the document look is a moment of gives taken
// in, told by the router from, once its files are read, while the router
// answers, as answering says: a probed document's tell of none, and once
// the router's gone, the document on screen is stale, its files read in its
// place. Who told them, and when, is noted, the first router's alone till
// its files are read again, as refiling has it. Where the document skips
// events the router told since the last, as Skips has it, its files are to
// be read again from when the last was built, or where that's not noted, as
// since they were read again, from where the events begin, the earliest
// such moment kept till they are.
func (e heldEvents) looked(look status.Moment, from router.Health, answering bool) heldEvents {
	if !e.opened || !look.Routed || !answering {
		return e
	}
	if e.read.Skips(look, from.StartedAt) && e.skipped.IsZero() {
		e.skipped = cmp.Or(e.told.at, e.read.Since())
	}
	if read, changed := e.read.Looked(look, from.StartedAt); changed {
		e = e.holding(read)
	} else {
		e.read = read
	}
	if e.told.at.IsZero() || e.told.run.Equal(from.StartedAt) {
		e.told = logTold{run: from.StartedAt, at: look.ReadAt}
	}
	return e
}

// refiling is when the router's files of events are to be read again from,
// and e with that forgotten: from when the document the events last took in
// was built, where the router that told it no longer answers, as answering
// says, or another, routed, answers in its place, as what it filed after
// that document no document tells of; else from when the events want them
// for what a document skipped. It's false where neither wants them.
func (e heldEvents) refiling(answering bool, routed router.Health) (heldEvents, time.Time, bool) {
	switch {
	case !e.told.at.IsZero() && (!answering || !e.told.run.Equal(routed.StartedAt)):
		from := e.told.at
		e.told = logTold{}
		return e, from, true
	case !e.skipped.IsZero():
		from := e.skipped
		e.skipped = time.Time{}
		return e, from, true
	}
	return e, time.Time{}, false
}

// reaching is e, once its files are read, reaching back no further than
// since, as Reaching has it.
func (e heldEvents) reaching(since time.Time) heldEvents {
	if !e.opened {
		return e
	}
	if read := e.read.Reaching(since); !read.Since().Equal(e.read.Since()) {
		return e.holding(read)
	}
	return e
}

// eventsLetGo is m with the events held letting go at now of those neither
// the Log nor Runway's LATELY wants: before EventsFrom of the day the Log
// spans from, where it's opened, and before the day LATELY's week begins
// on, where LATELY wants them, as RunwayLatelyFrom has it, so a day at a
// time.
func (m Model) eventsLetGo(now time.Time) Model {
	var wanted []time.Time
	if m.log.opened {
		wanted = append(wanted, views.EventsFrom(m.log.from.Start))
	}
	if m.events.lately {
		wanted = append(wanted, dayfile.DayStart(views.RunwayLatelyFrom(now).Local(), 0))
	}
	if len(wanted) > 0 {
		m.events = m.events.reaching(slices.MinFunc(wanted, time.Time.Compare))
	}
	return m
}
