package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/views"
)

// pastLately is what the watch holds of the router's events Detail's LATELY
// tells in a past period: those read up to the end of its last day, until,
// as views.ReadLatelyTo reads them, zero till one lands; the end of the last
// day a read under way reads to, zero while none is; and whether one was
// asked for another day while it was, to be asked again as it lands.
type pastLately struct {
	until   time.Time
	events  []views.Event
	reading time.Time
	again   bool
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
// as that lands. Without a reader of the files there's nothing to read, and
// it holds no events at once, as LATELY at now tells none it can't read.
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
		m.pastLately = pastLately{until: until}
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
// it was asked for another day as it was under way.
func (m Model) pastLatelyRead(msg pastLatelyReadMsg) (Model, tea.Cmd) {
	again := m.pastLately.again
	m.pastLately.reading, m.pastLately.again = time.Time{}, false
	if until, gone := m.period.Until(m.now(), m.cfg.WeekStarts); gone && until.Equal(msg.until) {
		m.pastLately.until, m.pastLately.events = msg.until, msg.events
	}
	if again {
		return m.askPastLately()
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
