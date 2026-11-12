package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// latelyReadMsg is what a read of the router's files for Runway's LATELY
// found: the events read as they're first read, where none held them; else
// what the files were found to hold since, to be taken in beside the events
// looks took in while the read was under way.
type latelyReadMsg struct {
	opened *views.EventsRead
	found  *views.EventsFiled
}

// readLately has the events held wanted by Runway's LATELY while Runway's
// live week shows, LATELY's alone, as wantedLately has it, letting go of
// those neither it nor the Log wants, as eventsLetGo has it, and the watch
// read the router's files of events for it, as a command, one read of them
// at a time, none while one of the Log's is under way, so the events held,
// which both read, reach no day twice: where none has read them, the week
// LATELY tells, from RunwayLatelyFrom to the end of the day, as
// ReadEventsFrom reads it; else the files again as the router stops or
// restarts, or a document skips events, as refiling has it; else, where the
// Log read them first, the days of the week before those it read, as BackTo
// reads them. Without a reader of the files, it reads nothing.
func (m Model) readLately() (Model, tea.Cmd) {
	if m.view != dashboard.Runway || !m.runwayTab.shown.Back.IsZero() {
		return m, nil
	}
	now := m.now()
	m.events = m.events.wantedLately()
	m = m.eventsLetGo(now)
	if m.cfg.Events == nil || m.events.reading || m.log.reading {
		return m, nil
	}
	filed, week := m.cfg.Events, views.RunwayLatelyFrom(now)
	if !m.events.opened {
		return m.readingLately(func() latelyReadMsg {
			read := views.ReadEventsFrom(filed, week, now)
			return latelyReadMsg{opened: &read}
		})
	}
	events, from, refiled := m.events.refiling(m.answering(), m.news.router)
	held := events.read
	switch {
	case refiled:
		m.events = events
		return m.readingLately(func() latelyReadMsg {
			found := held.Refiled(filed, from, now)
			return latelyReadMsg{found: &found}
		})
	case held.Since().After(week):
		return m.readingLately(func() latelyReadMsg {
			found := held.BackTo(filed, week)
			return latelyReadMsg{found: &found}
		})
	}
	return m, nil
}

// readingLately is m reading for Runway's LATELY as read reads, as a
// command.
func (m Model) readingLately(read func() latelyReadMsg) (Model, tea.Cmd) {
	m.events.reading = true
	return m, func() tea.Msg { return read() }
}

// latelyRead takes in what a read for Runway's LATELY found, into the events
// held, as took has it, the read no longer under way, taking in again those
// of the document on screen while the router answers, in place of the
// versions the read found on the files, as logRead does.
func (m Model) latelyRead(msg latelyReadMsg) Model {
	m.events.reading = false
	m.events = m.events.took(msg.opened, msg.found).looked(m.moment(m.now()), m.news.router, m.answering())
	return m
}

// latelyHeld are the router's events Runway's LATELY tells at the moment
// mo, oldest first: those held, once the router's files are read, as made
// for LATELY where it wants them already; till then, those the document
// gives, as told by the router that answers.
func (m Model) latelyHeld(mo status.Moment) []views.Event {
	switch {
	case !m.events.opened:
		return views.EventsSince(time.Time{}, mo, m.news.router.StartedAt, nil)
	case m.events.lately:
		return m.events.events
	}
	return m.events.read.Events()
}
