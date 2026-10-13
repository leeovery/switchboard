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
// events, the figures of its days, and how far back its tables read.
type logGrown struct {
	read   views.LogRead
	from   views.LedgerDay
	events *views.EventsRead
	totals []views.DayTotals
	logBounds
}

// logBounds are how far back the Log's tables read, where a read asked: the
// oldest day the ledger keeps, and the oldest the Events read back to; none
// of either where it couldn't be said, and why.
type logBounds struct {
	oldest, filed *views.OldestDay
	err           error
}

// logReaders are what the Log reads, as the watch's config gives them: the
// request ledger, the router's files of events, the prices the ledger's
// lines are priced at, and how long the ledger and the files keep them.
type logReaders struct {
	ledger views.Ledger
	filed  views.Events
	prices ledger.Table
	keep   time.Duration
}

// logReaders are what the Log reads.
func (m Model) logReaders() logReaders {
	return logReaders{ledger: m.cfg.Ledger, filed: m.cfg.Events, prices: m.cfg.Prices, keep: m.cfg.LedgerKeep}
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
// as the event its details card is open on does.
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
	case m.log.page.WantsDayBefore(m.logRows()), m.log.wantsFactsBack():
		return m.readBack(now)
	}
	return m, nil
}

// openLog reads the Log as it first shows, at now: every one of today's
// lines, the tail's first read where it's unread, its mark otherwise left
// where its readers fold from, the lines read again past it held already;
// the router's events from today, as ReadEvents reads them; and how far
// back its tables read, as bounds has it.
func (m Model) openLog(now time.Time) (Model, tea.Cmd) {
	r := m.logReaders()
	joins := m.tail.unread()
	if joins {
		m.tail = m.tail.asking()
	}
	m.log.reading = true
	return m, func() tea.Msg {
		today := views.Today(now)
		lines, next, _ := r.ledger.Today(ledger.Mark{})
		events := views.ReadEvents(r.filed, today.Start, now)
		msg := logReadMsg{grown: logGrown{read: views.ReadLogHeld(r.ledger, lines, r.prices, now), from: today, events: &events}}
		msg.grown.logBounds = r.bounds(now)
		if joins {
			msg.tail = &tailRead{mark: next, day: today}
		}
		return msg
	}
}

// readTail reads what the ledger gained since the tail's mark, at now, and
// grows the Log's read by it, as logFold has it.
func (m Model) readTail(now time.Time) (Model, tea.Cmd) {
	r, mark := m.logReaders(), m.tail.mark
	fold := m.logFold(now)
	m.tail, m.log.reading = m.tail.asking(), true
	return m, func() tea.Msg {
		lines, next, _ := r.ledger.Today(mark)
		return tailedMsg{mark: next, day: views.Today(now), log: fold.grown(r, lines, now)}
	}
}

// logFold is what a read of the ledger's tail does for the Log, as it
// stands as the read is asked: grows its read by the lines the ledger
// gained, and by those of the days before today it reads again whole, late,
// as the tail followed them no further, or the line of a request across
// midnight comes on its day after the day turned; where it reads any so,
// reads again the figures of the days its tables span, from from; and where
// how far back they read isn't known, asks it again.
type logFold struct {
	read views.LogRead
	late []string
	from views.LedgerDay
	asks bool
}

// logFold is what a read of the ledger's tail at now does for the Log: its
// late days those the tail missed, as missed has them, and those before
// today the Requests await a line on, shown or hidden, as their Awaited has
// them, each once, the oldest first.
func (m Model) logFold(now time.Time) logFold {
	today := views.Today(now)
	late := slices.Concat(m.log.page.Requests.Awaited(today), m.tail.missed(today))
	return logFold{read: m.log.read, late: slices.Compact(slices.Sorted(slices.Values(late))), from: m.log.from, asks: !m.log.bounded()}
}

// grown is the Log grown, through r, at now, by lines, the ledger's gain
// since the tail's mark, as the fold says.
func (f logFold) grown(r logReaders, lines []ledger.Held, now time.Time) *logGrown {
	l := r.ledger
	g := logGrown{read: f.read.Grown(l, lines, now)}
	for _, date := range f.late {
		g.read = g.read.Back(l, l.Day(date), now)
	}
	if len(f.late) > 0 {
		g.totals = views.TotalsByDay(l, f.from.Start, now, r.prices)
	}
	if f.asks {
		g.logBounds = r.bounds(now)
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
	r := m.logReaders()
	m.log.reading = true
	return m, func() tea.Msg {
		l, events := r.ledger, t.events.Back(r.filed, day.Start)
		return logReadMsg{grown: logGrown{
			read: t.read.Back(l, l.Day(day.Date), now), from: day, events: &events, totals: views.TotalsByDay(l, day.Start, now, r.prices),
		}}
	}
}

// bounds are how far back the Log's tables read at now: the oldest day the
// ledger keeps, of the first it holds lines of; and the oldest the Events
// read back to, of the first the router filed events on, none filed where
// there are no files to read; each as keptFrom has it.
func (r logReaders) bounds(now time.Time) logBounds {
	lines, linesErr := r.ledger.FirstLinesDay(now)
	filed, filedErr := time.Time{}, error(fs.ErrNotExist)
	if r.filed != nil {
		filed, filedErr = r.filed.FirstDay()
	}
	var b logBounds
	b.oldest, linesErr = keptFrom(lines, linesErr, r.keep, now)
	b.filed, filedErr = keptFrom(filed, filedErr, r.keep, now)
	b.err = errors.Join(linesErr, filedErr)
	return b
}

// keptFrom is the oldest day kept at now of what began on the day first
// starts, kept as long as keep says, as views.OldestKept has it: given no
// first day where err says there's none yet; none where err says it
// couldn't be read, and err.
func keptFrom(first time.Time, err error, keep time.Duration, now time.Time) (*views.OldestDay, error) {
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

// grown is the Log grown as g says, its read no longer under way: where
// how far back its tables read couldn't be said, logged, the first time
// alone, as each read after asks again.
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
	if g.filed != nil {
		t.filed = *g.filed
	}
	if g.err != nil && !t.warned {
		logger.Warn("couldn't read where the request ledger's lines, or the router's events, begin", "error", g.err)
		t.warned = true
	}
	return t
}

// bounded reports whether the Log knows how far back its tables read.
func (t logTab) bounded() bool {
	return t.oldest.Day.Date != "" && t.filed.Day.Date != ""
}
