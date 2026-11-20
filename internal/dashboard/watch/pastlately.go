package watch

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/views"
)

// pastLately is what the watch holds of the router's events Detail's LATELY
// tells in a past period: those read up to the end of its last day, until,
// as views.ReadLatelyTo reads them, zero till one lands, and how many times
// it has held those of a day; the end of the last day a read under way reads
// to, zero while none is; and whether one was asked for another day while it
// was, to be asked again as it lands.
type pastLately struct {
	until   time.Time
	events  []views.Event
	held    int
	reading time.Time
	again   bool
}

// holding is p holding the events given, up to until, counted.
func (p pastLately) holding(until time.Time, events []views.Event) pastLately {
	p.until, p.events, p.held = until, events, p.held+1
	return p
}

// pastLatelyReadMsg is what a read for a past period's LATELY found: the
// router's events up to until, the end of its last day.
type pastLatelyReadMsg struct {
	until  time.Time
	events []views.Event
}

// askPastLately has the watch read the router's files for Detail's LATELY
// in the dial's period while it's gone, as a command, once for each last
// day, one read at a time: none where what's held, or a read under way, is
// of its last day already; where another's is under way, it's asked again
// as that lands. Without a reader of the files there's nothing to read: it
// holds no events at once, and the page says what it says where LATELY has
// none to tell.
// The reader takes concurrent reads, as an events.Reader holds the files to
// read them, so it reads beside the Log's and Runway's reads.
func (m Model) askPastLately() (Model, tea.Cmd) {
	until, gone := m.period.Until(m.now(), m.cfg.WeekStarts)
	p := m.pastLately
	switch {
	case !gone || p.until.Equal(until) || p.reading.Equal(until):
		return m, nil
	case !p.reading.IsZero():
		m.pastLately.again = true
		return m, nil
	case m.cfg.Events == nil:
		m.pastLately = m.pastLately.holding(until, nil)
		return m, nil
	}
	filed := m.cfg.Events
	m.pastLately.reading = until
	return m, func() tea.Msg {
		return pastLatelyReadMsg{until: until, events: views.ReadLatelyTo(filed, until)}
	}
}

// pastLatelyRead takes in a read for a past period's LATELY, the read no
// longer under way: held where the dial's period still ends where it read
// to, else dropped, the dial having stepped past it; then asked again where
// it was asked for another day as it was under way, while Detail still
// shows, as readPastLately reads it.
func (m Model) pastLatelyRead(msg pastLatelyReadMsg) (Model, tea.Cmd) {
	again := m.pastLately.again
	m.pastLately.reading, m.pastLately.again = time.Time{}, false
	if until, gone := m.period.Until(m.now(), m.cfg.WeekStarts); gone && until.Equal(msg.until) {
		m.pastLately = m.pastLately.holding(msg.until, msg.events)
	}
	if again {
		return m.readPastLately()
	}
	return m, nil
}

// pastLatelyHeld are the router's events Detail's LATELY tells in the dial's
// period while it's gone, oldest first, and the end of its last day they
// run to, as views.AccountLatelyTo takes them; false while none are held of
// its last day, as at now, or while their read is under way.
func (m Model) pastLatelyHeld() ([]views.Event, time.Time, bool) {
	until, gone := m.period.Until(m.now(), m.cfg.WeekStarts)
	if !gone || !m.pastLately.until.Equal(until) {
		return nil, time.Time{}, false
	}
	return m.pastLately.events, until, true
}

// readPastLately has the watch read the router's events for Detail's LATELY
// while it shows a past period, as askPastLately reads them.
func (m Model) readPastLately() (Model, tea.Cmd) {
	if !m.detailShown() {
		return m, nil
	}
	return m.askPastLately()
}

// pastLatelyAwaited reports whether Detail shows a past period whose LATELY
// waits on its events' read, as pastLatelyHeld has them.
func (m Model) pastLatelyAwaited() bool {
	_, _, held := m.pastLatelyHeld()
	return m.detailShown() && m.gone(m.now()) && !held
}

// latelyOf is what Detail's LATELY in a past period was told of: the account
// it tells of, by id, "" for all accounts; the events held, as pastLately
// counts them; the day it was told on, as its words say a time beside it;
// and the sessions held, as stored counts their changes, whose directories
// name them.
type latelyOf struct {
	id     string
	held   int
	day    string
	stored int
}

// pastLatelyBuilt is Detail's LATELY in a past period as last told, and what
// it was told of, as latelyOf has it.
type pastLatelyBuilt struct {
	of        latelyOf
	sentences []views.Sentence
}

// pastLatelyTold is m with Detail's LATELY in the dial's period while it's
// gone told at now, at the moment of the document then, as
// views.AccountLatelyTo tells it of the events pastLatelyHeld gives, of the
// account Detail shows, every one, each session named by its directory, as
// the router listed it or the watch holds it: as last told where nothing
// it's told of has changed, as latelyOf has it, never a frame at a time;
// none while Detail shows no past period, nor while its events are read.
func (m Model) pastLatelyTold(now time.Time) Model {
	events, until, held := m.pastLatelyHeld()
	if !m.detailShown() || !held {
		m.accounts.lately = pastLatelyBuilt{}
		return m
	}
	of := latelyOf{id: m.accounts.shown.Account(), held: m.pastLately.held, day: views.Today(now).Date, stored: m.stored}
	if m.accounts.lately.of == of {
		return m
	}
	dirs := views.SessionDirsOf(m.sessions, m.held)
	m.accounts.lately = pastLatelyBuilt{of: of, sentences: views.AccountLatelyTo(m.moment(now), events, of.id, until, math.MaxInt, dirs)}
	return m
}
