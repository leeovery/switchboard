package watch

import (
	"errors"
	"io/fs"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// logGrown is the Log as a read for it grew it: its read of the ledger's
// lines; and where the read read them, the day its tables span from, its
// events, the figures of its days, and the oldest day the ledger keeps, or
// why the ledger couldn't say where its lines begin.
type logGrown struct {
	read   views.LogRead
	from   views.LedgerDay
	events *views.EventsRead
	totals []views.DayTotals
	oldest *views.OldestDay
	err    error
}

// logReadMsg is what a read for the Log of its own found, as it first shows
// or as its tables reach back a day; and where it read today's lines whole
// for the ledger's tail, unread till then, where it read them to.
type logReadMsg struct {
	grown logGrown
	tail  *tailRead
}

// readLog has the watch read for the Log while it shows, as it wants, one
// read of the ledger at a time, what's wanted meanwhile waiting for it to
// land, its page first built of what it holds: as it first shows, every one
// of today's lines, the router's events from today, and the oldest day the
// ledger keeps; at each look since, what the ledger gained, as the tail
// reads it; and the day before those its tables span, as they want it, or
// as the find's pick, of an earlier day, does, as reaching has it.
func (m Model) readLog() (Model, tea.Cmd) {
	if m.view != dashboard.Log || m.cfg.Ledger == nil {
		return m, nil
	}
	now := m.now()
	m = m.builtLog(now)
	if m.tail.reading || m.log.reading {
		return m, nil
	}
	switch {
	case !m.log.opened:
		return m.openLog(now)
	case m.tail.due:
		return m.readTail(now)
	case m.log.page.WantsDayBefore(m.logRows()) || m.reaching():
		return m.readBack(now)
	}
	return m, nil
}

// openLog reads the Log as it first shows, at now: every one of today's
// lines, the tail's first read where it's unread, its mark otherwise left
// where its readers fold from, the lines read again past it held already;
// the router's events from today, as ReadEvents reads them; and the oldest
// day the ledger keeps, as oldestKept has it.
func (m Model) openLog(now time.Time) (Model, tea.Cmd) {
	l, filed, prices, keep := m.cfg.Ledger, m.cfg.Events, m.cfg.Prices, m.cfg.LedgerKeep
	joins := m.tail.unread()
	if joins {
		m.tail = m.tail.asking()
	}
	m.log.reading = true
	return m, func() tea.Msg {
		today := views.Today(now)
		lines, next, _ := l.Today(ledger.Mark{})
		events := views.ReadEvents(filed, today.Start, now)
		msg := logReadMsg{grown: logGrown{read: views.ReadLogHeld(l, lines, prices, now), from: today, events: &events}}
		msg.grown.oldest, msg.grown.err = oldestKept(l, keep, now)
		if joins {
			msg.tail = &tailRead{mark: next, day: today}
		}
		return msg
	}
}

// readTail reads what the ledger gained since the tail's mark, at now, and
// grows the Log's read by it, as logFold has it.
func (m Model) readTail(now time.Time) (Model, tea.Cmd) {
	l, prices, keep, mark := m.cfg.Ledger, m.cfg.Prices, m.cfg.LedgerKeep, m.tail.mark
	fold := m.logFold(now)
	m.tail, m.log.reading = m.tail.asking(), true
	return m, func() tea.Msg {
		lines, next, _ := l.Today(mark)
		return tailedMsg{mark: next, day: views.Today(now), log: fold.grown(l, lines, prices, keep, now)}
	}
}

// logFold is what a read of the ledger's tail does for the Log, as it
// stands as the read is asked: grows its read by the lines the ledger
// gained, and by those of the days before today it reads again whole, late,
// as the line of a request across midnight comes on its day after the day
// turned; where the day turned, reads again the figures of the days its
// tables span, from from; and where the oldest day the ledger keeps isn't
// known, asks it again.
type logFold struct {
	read   views.LogRead
	late   []string
	from   views.LedgerDay
	turned bool
	asks   bool
}

// logFold is what a read of the ledger's tail at now does for the Log: its
// late days the day the tail was last read on, where the day turned since,
// and those before today the Requests await a line on, shown or hidden, as
// their Awaited has them.
func (m Model) logFold(now time.Time) logFold {
	today := views.Today(now)
	turned := m.tail.turned(today)
	late := m.log.page.Requests.Awaited(today)
	if turned && !slices.Contains(late, m.tail.day.Date) {
		late = append(late, m.tail.day.Date)
	}
	return logFold{read: m.log.read, late: late, from: m.log.from, turned: turned, asks: m.log.oldest.Day.Date == ""}
}

// grown is the Log grown, through l, at now, by lines, the ledger's gain
// since the tail's mark, as the fold says, each line priced by prices, the
// oldest day asked of a ledger keeping its lines as long as keep says.
func (f logFold) grown(l views.Ledger, lines []ledger.Held, prices ledger.Table, keep time.Duration, now time.Time) *logGrown {
	g := logGrown{read: f.read.Grown(l, lines, now)}
	for _, date := range f.late {
		g.read = g.read.Back(l, l.Day(date), now)
	}
	if f.turned {
		g.totals = views.TotalsByDay(l, f.from.Start, now, prices)
	}
	if f.asks {
		g.oldest, g.err = oldestKept(l, keep, now)
	}
	return &g
}

// readBack reads the day before those the Log's tables span, at now, as the
// oldest day the ledger keeps allows, once known: its lines, its events and
// the figures of the days from it.
func (m Model) readBack(now time.Time) (Model, tea.Cmd) {
	t := m.log
	day, ok := t.oldest.DayBefore(t.from)
	if !ok {
		return m, nil
	}
	l, filed, prices := m.cfg.Ledger, m.cfg.Events, m.cfg.Prices
	m.log.reading = true
	return m, func() tea.Msg {
		events := t.events.Back(filed, day.Start)
		return logReadMsg{grown: logGrown{
			read: t.read.Back(l, l.Day(day.Date), now), from: day, events: &events, totals: views.TotalsByDay(l, day.Start, now, prices),
		}}
	}
}

// oldestKept is the oldest day the ledger l keeps at now, its lines kept as
// long as keep says, as views.OldestKept has it, given no first day of lines
// where l holds none yet: none where it fails to say where its lines begin,
// and why.
func oldestKept(l views.Ledger, keep time.Duration, now time.Time) (*views.OldestDay, error) {
	first, err := l.FirstLinesDay(now)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		first = time.Time{}
	case err != nil:
		return nil, err
	}
	oldest := views.OldestKept(first, keep, now)
	return &oldest, nil
}

// tailed takes in what a read of the ledger's tail found: the tail read to
// its mark, and the Log grown by it, where it was read for the Log.
func (m Model) tailed(msg tailedMsg) Model {
	m.tail = m.tail.read(msg.tailRead)
	if msg.log != nil {
		m.log = m.log.grown(*msg.log)
	}
	return m
}

// logRead takes in what a read for the Log of its own found: the tail read
// to where it read today's lines, where it read them for it; the Log grown
// by it, and opened, its events taking in again those of the document on
// screen, as a look taken while the read was under way gave them to the
// events it grew from.
func (m Model) logRead(msg logReadMsg) Model {
	if msg.tail != nil {
		m.tail = m.tail.read(*msg.tail)
	}
	m.log = m.log.grown(msg.grown)
	m.log.opened = true
	m.log = m.log.looked(m.moment(m.now()), m.news.router)
	return m
}

// grown is the Log grown as g says, its read no longer under way: where the
// ledger couldn't say where its lines begin, logged, the first time alone,
// as each read after asks again.
func (t logTab) grown(g logGrown) logTab {
	t.reading, t.read = false, g.read
	if g.events != nil {
		t.events = *g.events
	}
	if !g.from.Start.IsZero() {
		t.from = g.from
	}
	if g.totals != nil {
		t.totals = g.totals
	}
	if g.oldest != nil {
		t.oldest = *g.oldest
	}
	if g.err != nil && !t.warned {
		logger.Warn("couldn't read where the request ledger's lines begin", "error", g.err)
		t.warned = true
	}
	return t
}
