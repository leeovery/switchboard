package watch

import (
	"errors"
	"io/fs"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/views"
)

// logGrown is the Log as a read for it grew it: its read of the ledger's
// lines; and where the read read them, the day its tables span from, its
// events as it first shows, or what the router's files were found to hold
// since, to be taken in beside the events looks took in while the read was
// under way, the figures of its days, and how far back its tables read.
type logGrown struct {
	read   views.LogRead
	from   views.LedgerDay
	events *views.EventsRead
	found  *views.EventsFiled
	totals []views.DayTotals
	logBounds
}

// lines reports whether g changes what the Log's Requests read, its read
// held as given before it: lines taken in, or the figures of days read, or
// where the tables start or how far back the ledger keeps its lines said,
// as a read of the tail that gained nothing changes none of them.
func (g logGrown) lines(held views.LogRead) bool {
	return g.read.Len() != held.Len() || len(g.totals) > 0 || !g.from.Start.IsZero() || g.oldest != nil
}

// changes reports whether g changes what the Log holds, its read held as
// given before it: what its Requests read, as lines has it, or its events,
// or how far back they read, as a read of the tail that gained nothing
// changes none of them.
func (g logGrown) changes(held views.LogRead) bool {
	return g.lines(held) || g.events != nil || g.found != nil || g.filed != nil
}

// logBounds are how far back the Log's tables read, where a read asked: the
// oldest day the ledger keeps, and the oldest the Events read back to; none
// of either where it couldn't be said, and why; and the date of the day it
// asked on.
type logBounds struct {
	oldest, filed *views.OldestDay
	err           error
	asked         string
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
// for the ledger's tail, unread till then, where it read them to, and the
// lines it read.
type logReadMsg struct {
	grown logGrown
	tail  *tailRead
	lined []ledger.Held
}

// readLog has the watch read for the Log while it shows, as it wants, one
// read of the ledger at a time, what's wanted meanwhile waiting for it to
// land, and for a session's page asked to open to read first, its page
// first built of what it holds: as it first shows, every one of today's
// lines, the router's events from today, and the oldest day the ledger
// keeps; the router's files of events again, as the router stops or
// restarts, or a document skips events; at each look since, what the ledger
// gained, as the tail reads it; and the day before those its tables span, as
// they want it, as far back as the lines of the filter the page's Reach
// gives reach, the Events paused at their top past it, as wantsDayBefore
// and the page have it, the days' summaries read first where it wants them,
// and of the Events through some kinds or accounts, the router's files, as
// wantsFiledReach has it; or as the event its details card is open on does,
// or the find's pick, of an earlier day, as reaching has it, or a row its
// Requests show awaits a line filed on it, as awaitsBefore has it.
func (m Model) readLog() (Model, tea.Cmd) {
	if m.view != dashboard.Log || m.cfg.Ledger == nil {
		return m, nil
	}
	now := m.now()
	m = m.builtLog(now)
	if m.tail.busy() || m.log.reading || m.pending != "" {
		return m, nil
	}
	switch {
	case !m.log.opened:
		return m.openLog(now)
	case m.log.refiles(m.answering(), m.news.router):
		return m.readRefiled(now)
	case !m.log.skipped.IsZero():
		return m.readSkipped(now)
	case m.tail.due:
		return m.readTail(now)
	case m.log.wantsReach(m.logRows()):
		return m.readReach()
	case m.log.wantsFiledReach(m.logRows()):
		return m.readFiledReach()
	case m.log.wantsDayBefore(m.logRows()), m.log.wantsFactsBack(), m.reaching(), m.log.awaitsBefore():
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
			msg.tail, msg.lined = &tailRead{mark: next, day: today}, lines
		}
		return msg
	}
}

// logTold is the router whose document the Log's Events last took in, by
// when its run started, and when that document was built: none before the
// first, nor once its files are read again.
type logTold struct {
	run, at time.Time
}

// refiles reports whether the Log's Events want the router's files read
// again from when the document they last took in was built: the router
// that told it no longer answering, as answering says, or another, routed,
// answering in its place. What it filed after that document no document
// tells of.
func (t logTab) refiles(answering bool, routed router.Health) bool {
	return !t.told.at.IsZero() && (!answering || !t.told.run.Equal(routed.StartedAt))
}

// readRefiled reads the router's files of events again at now, from when
// the document the Log's Events last took in was built, as refiledFrom does,
// who told it forgotten.
func (m Model) readRefiled(now time.Time) (Model, tea.Cmd) {
	from := m.log.told.at
	m.log.told = logTold{}
	return m.refiledFrom(from, now)
}

// readSkipped reads the router's files of events again at now, from when
// the Log's Events want them for what a document skipped, as refiledFrom
// does.
func (m Model) readSkipped(now time.Time) (Model, tea.Cmd) {
	from := m.log.skipped
	m.log.skipped = time.Time{}
	return m.refiledFrom(from, now)
}

// refiledFrom reads the router's files of events again at now, from the
// moment given, into the Log's Events, as Refiled does.
func (m Model) refiledFrom(from, now time.Time) (Model, tea.Cmd) {
	t, filed := m.log, m.cfg.Events
	m.log.reading = true
	return m, func() tea.Msg {
		refiled := t.events.Refiled(filed, from, now)
		return logReadMsg{grown: logGrown{read: t.read, found: &refiled}}
	}
}

// logFold is what a read of the ledger's tail does for the Log, as it
// stands as the read is asked: grows its read by the lines the ledger
// gained, and by those of the days before today it reads again whole, late,
// as the tail followed them no further, or the line of a request across
// midnight could have come on its day after the day turned; reads again the
// figures of those days alone; and where how far back they read isn't
// known today, as bounded has it, asks it again. Awaited is when each day awaiting a line was last
// read for it, as the read leaves it.
type logFold struct {
	read    views.LogRead
	late    []string
	awaited awaitedReads
	asks    bool
}

// logFold is what a read of the ledger's tail at now does for the Log: its
// late days those the tail missed, as missed has them, and those before
// today the Requests await a line on, shown or hidden, as their Awaited has
// them, whose lines could have come since they were last read, as due has
// them; each once, the oldest first, and none before the first day its
// tables span, as spanned has it, which a read back reads, as awaitsBefore
// has it.
func (m Model) logFold(now time.Time) logFold {
	today := views.Today(now)
	due, awaited := m.log.awaited.due(m.log.spanned(m.log.page.Requests.Awaited(today)), m.logStream(now), now)
	late := m.log.spanned(slices.Concat(due, m.tail.missed(today)))
	return logFold{read: m.log.read, late: slices.Compact(slices.Sorted(slices.Values(late))), awaited: awaited, asks: !m.log.bounded(today.Date)}
}

// spanned are those of the dates given of the days the Log's tables span.
func (t logTab) spanned(dates []string) []string {
	return slices.DeleteFunc(slices.Clone(dates), func(date string) bool { return date < t.from.Date })
}

// awaitsBefore reports whether a row the Log's Requests show awaits a line
// filed on a day before the first they span, as their Awaited has it, as a
// request sent just after midnight that arrived before it is filed on the
// day before: they read back to it, to take the line in as it comes, so the
// request's row is never let go of untaken, as letGo has it.
func (t logTab) awaitsBefore() bool {
	return len(t.page.Requests.Awaited(t.from)) > 0
}

// grown is the Log grown, through r, at now, by what a read of the tail
// read: the ledger's gain since the tail's mark, and the late days' lines,
// as the fold says.
func (f logFold) grown(r logReaders, read tailLines, now time.Time) *logGrown {
	l := r.ledger
	g := logGrown{read: f.read.Grown(l, read.gained, now), totals: views.TotalsOn(l, f.late, now, r.prices)}
	for _, date := range f.late {
		g.read = g.read.Grown(l, read.days[date], now)
	}
	if f.asks {
		g.logBounds = r.bounds(now)
	}
	return &g
}

// awaitedReads are when each day the Requests await a line on was last read
// whole for it, by the day's date.
type awaitedReads map[string]time.Time

// awaitedEvery is how long a day awaiting a line goes unread while none of
// the requests filed on it ends: the router writes a request's line moments
// after the stream tells it ended, so the read at the look after nearly
// always finds it, and this bounds the wait for one written later.
const awaitedEvery = time.Minute

// due are those of the dates given, of the days awaiting a line, to be read
// whole at now, s being what the stream tells: each one a request filed on
// ended since it was last read for it, as Told.FiledOn files it, or read
// for it awaitedEvery ago or more; and r as it'll stand, those due read at
// now, those no longer awaited let go.
func (r awaitedReads) due(dates []string, s views.Stream, now time.Time) ([]string, awaitedReads) {
	next := make(awaitedReads, len(dates))
	var due []string
	for _, date := range dates {
		read := r[date]
		if now.Sub(read) >= awaitedEvery || endedSince(s, date, read) {
			due, read = append(due, date), now
		}
		next[date] = read
	}
	return due, next
}

// endedSince reports whether s told a request filed on the day with the
// given date, as Told.FiledOn files it, ended after since.
func endedSince(s views.Stream, date string, since time.Time) bool {
	for _, t := range s.Requests {
		if day, ok := t.FiledOn(); ok && day.Date == date && t.Ended.After(since) {
			return true
		}
	}
	return false
}

// foldedTotals are the figures held with those read folded in, oldest
// first: each read in place of the one held of its day, where there's one.
func foldedTotals(held, read []views.DayTotals) []views.DayTotals {
	if len(read) == 0 {
		return held
	}
	folded := slices.Clone(held)
	for _, t := range read {
		i, found := slices.BinarySearchFunc(folded, t.Date, func(d views.DayTotals, date string) int { return strings.Compare(d.Date, date) })
		if found {
			folded[i] = t
		} else {
			folded = slices.Insert(folded, i, t)
		}
	}
	return folded
}

// readBack reads the day before those the Log's tables span, at now, as the
// oldest day the ledger keeps allows, once known: its lines, its events and
// its figures alone, those of the days after it held already.
func (m Model) readBack(now time.Time) (Model, tea.Cmd) {
	t := m.log
	day, ok := t.oldest.DayBefore(t.from)
	if !ok {
		return m, nil
	}
	r := m.logReaders()
	m.log.reading = true
	return m, func() tea.Msg {
		l, back := r.ledger, t.events.Back(r.filed, day.Start)
		return logReadMsg{grown: logGrown{
			read: t.read.Grown(l, l.Day(day.Date), now), from: day, found: &back, totals: views.TotalsOn(l, []string{day.Date}, now, r.prices),
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
	b := logBounds{asked: views.Today(now).Date}
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

// logRead takes in what a read for the Log of its own found: the Log grown
// by it, and opened, its events taking in again those of the document on
// screen while the router answers: as it first shows, as no look took them
// in before it opened; and since, as the router holds them, in place of the
// versions the read found on its files; then the tail read to where it read
// today's lines, where it read them for it, and the lines it read of the
// requests the stream told of held, as lined has it, for letGo.
func (m Model) logRead(msg logReadMsg) Model {
	m.log = m.log.grown(msg.grown)
	m.log.opened = true
	m.log = m.log.looked(m.moment(m.now()), m.news.router, m.answering())
	if msg.tail != nil {
		m.tail, m.traffic = m.tail.read(*msg.tail), m.traffic.lined(msg.lined)
	}
	return m
}

// grown is the Log grown as g says, its read no longer under way, what it
// holds counted as changed only where g changes it, as changes has it: its
// events those read as it first shows, or those held with what the router's
// files were found to hold taken in, as Took has it, so none a look took in
// while the read was under way is lost; the figures of the days read folded
// into those held; the day it asked how far back its tables read taken
// only where both were said, so the next read asks again whatever the day;
// and where they couldn't be, logged, the first time alone.
func (t logTab) grown(g logGrown) logTab {
	if g.lines(t.read) {
		t.lined++
	}
	if g.changes(t.read) {
		t.held++
	}
	t.reading, t.read = false, g.read
	if g.events != nil {
		t.events = *g.events
	}
	if g.found != nil {
		t.events = t.events.Took(*g.found)
	}
	if !g.from.Start.IsZero() {
		t.from = g.from
	}
	t.totals = foldedTotals(t.totals, g.totals)
	if g.oldest != nil {
		t.oldest = *g.oldest
	}
	if g.filed != nil {
		t.filed = *g.filed
	}
	if g.asked != "" && g.err == nil {
		t.boundedOn = g.asked
	}
	if g.err != nil && !t.warned {
		logger.Warn("couldn't read where the request ledger's lines, or the router's events, begin", "error", g.err)
		t.warned = true
	}
	return t
}

// bounded reports whether the Log knows how far back its tables read on
// the day with the date given: as it asked that day, as the oldest day kept
// moves on with each.
func (t logTab) bounded(today string) bool {
	return t.oldest.Day.Date != "" && t.filed.Day.Date != "" && t.boundedOn == today
}
