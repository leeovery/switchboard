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
// under way, the figures of its days, how far back its tables read, and the
// sessions of its lines the store held none of, read whole into a store of
// their own, for the watch's to take in.
type logGrown struct {
	read   views.LogRead
	from   views.LedgerDay
	events *views.EventsRead
	found  *views.EventsFiled
	totals []views.DayTotals
	held   *views.HeldSessions
	logBounds
}

// lines reports whether g changes what the Log's Requests read, its read
// held as given before it: lines taken in or told again, or the figures of
// days read, or where the tables start or how far back the ledger keeps its
// lines said, as a read of the tail that gained nothing changes none of
// them.
func (g logGrown) lines(held views.LogRead) bool {
	return !g.read.Same(held) || len(g.totals) > 0 || !g.from.Start.IsZero() || g.oldest != nil
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
// or as its tables reach back a day; and as it first shows, what the read of
// the ledger's tail it opens with found for every reader, as tailedMsg has
// it.
type logReadMsg struct {
	grown  logGrown
	tailed *tailedMsg
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
// Requests show awaits a line filed on it, as awaitsBefore has it, each read
// back as readBack reads it. While it doesn't show, it lets go as
// keptHidden has it.
func (m Model) readLog() (Model, tea.Cmd) {
	if m.cfg.Ledger == nil {
		return m, nil
	}
	if m.view != dashboard.Log {
		return m.keptHidden(m.now()), nil
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

// keptHidden is m with the Log, opened but another tab showing, built at
// now of what it holds as its read grows, as builtLog builds it, so it lets
// go of the days its tables no longer show, as a read of the tail grows it
// whatever shows: once a read, as its read changes.
func (m Model) keptHidden(now time.Time) Model {
	if !m.log.opened || m.log.read.Gen() == m.log.kept {
		return m
	}
	m = m.builtLog(now)
	m.log.kept = m.log.read.Gen()
	return m
}

// openLog reads the Log as it first shows, at now, with a read of the
// ledger's tail afresh, as askingTail asks it: every one of today's lines,
// read once, from no mark, folded by every reader of the tail as a read of
// it, the store fed them wanting their sessions, and the tail's mark moved
// to where they were read to, so the Log is told of none the store wasn't
// fed, and the tail gives it none of them again; the Log takes them whole,
// each told by the store as fed. It reads too the router's events from
// today, as ReadEvents reads them, and how far back its tables read, as
// bounds has it.
func (m Model) openLog(now time.Time) (Model, tea.Cmd) {
	stream := m.logStream(now)
	m.log.reading = true
	m, t := m.askingTail(now, true)
	return m, func() tea.Msg {
		r := t.r
		read, next := readLines(r.ledger, t.mark, t.late)
		read.afresh = true
		today, opened := views.RecordsOf(read.gained), views.ReadLog(r.prices)
		tailed, _ := t.took(read, next, opened.Wanted(stream, today))
		events := views.ReadEvents(r.filed, t.today.Start, now)
		g := logGrown{
			read: opened.Taken(today, true, views.LogTaking{Held: *tailed.held, Stream: stream, Now: now}), from: t.today, events: &events,
			logBounds: r.bounds(now),
		}
		return logReadMsg{grown: g, tailed: &tailed}
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
// midnight could have come on its day after the day turned, each placed as
// the stream told as the read was asked; reads again the figures of those
// days alone; and where how far back they read isn't known today, as
// bounded has it, asks it again. Awaited is when each day awaiting a line
// was last read for it, as the read leaves it.
type logFold struct {
	read    views.LogRead
	late    []string
	stream  views.Stream
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
	due, awaited := m.log.awaited.due(m.log.spanned(m.log.page.Requests.Awaited(today)), m.logStream(now), now, m.tail.at)
	late := m.log.spanned(slices.Concat(due, m.tail.missed(today)))
	return logFold{
		read: m.log.read, late: slices.Compact(slices.Sorted(slices.Values(late))), stream: m.logStream(now), awaited: awaited, asks: !m.log.bounded(today.Date),
	}
}

// loggedLines are the lines of a read of the ledger's tail the Log takes
// in, each its record, but those its read holds already, as LogRead's
// RecordsToTake has them: of today's gained, afresh where they're every one
// of today's; and of each of its late days, read again whole, by the day's
// date.
type loggedLines struct {
	gained views.LogDay
	afresh bool
	days   map[string]views.LogDay
}

// logged are the lines of read the Log takes in as the fold f folds it:
// none where it doesn't fold it.
func (f *logFold) logged(read tailLines) loggedLines {
	if f == nil {
		return loggedLines{}
	}
	logged := loggedLines{gained: f.read.RecordsToTake(read.gained), afresh: read.afresh, days: make(map[string]views.LogDay, len(f.late))}
	for _, date := range f.late {
		logged.days[date] = f.read.RecordsToTake(read.days[date])
	}
	return logged
}

// wants are the sessions the Log wants the store to hold as a read of the
// ledger's tail folds read, as LogRead's Wanted has them, the fold's
// stream telling what's under way: of the lines it holds, and those it
// takes in of the read, as logged has them. None where it doesn't fold it.
func (f *logFold) wants(read loggedLines) []string {
	if f == nil {
		return nil
	}
	days := []views.LogDay{read.gained}
	for _, date := range f.late {
		days = append(days, read.days[date])
	}
	return f.read.Wanted(f.stream, days...)
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

// grown is the Log grown, through r, by what a read of the tail read, as
// logged has it, as t takes it in, its rows told by the store as the read
// fed it: the ledger's gain since the tail's mark, and the late days' lines,
// as the fold says, the rows of the sessions the feed changed told again
// once.
func (f logFold) grown(r logReaders, read loggedLines, t views.LogTaking) *logGrown {
	l, now := r.ledger, t.Now
	g := logGrown{read: f.read.Taken(read.gained, read.afresh, t), totals: views.TotalsOn(l, f.late, now, r.prices)}
	t.Gains = nil
	for _, date := range f.late {
		g.read = g.read.Taken(read.days[date], true, t)
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
// whole at now, s being what the stream tells, and tailed when the tail
// last read today's lines: each one a request whose line it awaits ended on
// since it was last read for it, as endedSince has it, or read for it
// awaitedEvery ago or more; and r as it'll stand, those due read at now,
// those no longer awaited let go.
func (r awaitedReads) due(dates []string, s views.Stream, now, tailed time.Time) ([]string, awaitedReads) {
	next := make(awaitedReads, len(dates))
	var due []string
	for _, date := range dates {
		read := r[date]
		if !read.IsZero() && now.Sub(read) >= awaitedEvery || endedSince(s, date, read, tailed) {
			due, read = append(due, date), now
		}
		next[date] = read
	}
	return due, next
}

// endedSince reports whether s told a request filed on the day with the
// given date, as Told.FiledOn files it, ended after since: one surely filed
// on it, as Told.FiledSurely has it; or one sent just after its midnight,
// most likely filed with the next day's, once the tail read today's lines
// after it ended, tailed, so its line, still awaited, wasn't among them.
// Each sent in the minute after midnight would else have the whole day
// before read again as it ended, for a line today's nearly always holds: at
// 25,000 requests a day, at nearly every look of the two minutes after.
func endedSince(s views.Stream, date string, since, tailed time.Time) bool {
	for _, t := range s.Requests {
		if day, ok := t.FiledOn(); ok && day.Date == date && t.Ended.After(since) && (t.FiledSurely() || t.Ended.Before(tailed)) {
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

// readBack reads back, at now, from the day before those the Log's tables
// span, as the oldest day the ledger keeps allows, once known: as logBack's
// from reads back, reading on past each day that adds nothing to the
// Requests where readsOn says.
func (m Model) readBack(now time.Time) (Model, tea.Cmd) {
	t := m.log
	day, ok := t.oldest.DayBefore(t.from)
	if !ok {
		return m, nil
	}
	b := logBack{readers: m.logReaders(), tab: t, held: m.held, stream: m.logStream(now), readsOn: m.readsOn(), now: now}
	m.log.reading = true
	return m, func() tea.Msg {
		return logReadMsg{grown: b.from(day)}
	}
}

// readsOn reports whether a read back of the Log's reads on past a day that
// adds nothing to the Requests: while they show, wanting the day before for
// themselves, as wantsDayBefore has it, or reaching back to the find's
// pick, as reaching has it. A day that adds nothing to them leaves them
// wanting the one before it, so the days before the next line their filter
// keeps come in one read, those their summaries rule out unread.
func (m Model) readsOn() bool {
	return m.log.page.Shown == dashboard.LogRequests && (m.log.wantsDayBefore(m.logRows()) || m.reaching())
}

// logBack is a read back of the Log's, as it's asked: what it reads; the
// Log as it stands; the store held, which tells the rows of the lines it
// takes in; what the stream tells; whether it reads on past a day that adds
// nothing to the Requests, as readsOn has it; and now.
type logBack struct {
	readers logReaders
	tab     logTab
	held    views.HeldSessions
	stream  views.Stream
	readsOn bool
	now     time.Time
}

// from is the Log grown by a read back from the day given: the day's lines,
// as took takes them in; and where b reads on, while a day it takes in
// keeps none of its lines and reads no session whole, the day before it
// too, as far as pastDay allows; the events from the last day it read, and
// the figures of the days it read alone, those of the days after held
// already.
func (b logBack) from(day views.LedgerDay) logGrown {
	g := logGrown{read: b.tab.read}
	var dates []string
	for {
		was := g.read.Len()
		g, dates = b.took(g.read, day), append(dates, day.Date)
		before, ok := b.tab.pastDay(day)
		if !b.readsOn || !ok || g.held != nil || g.read.Len() > was {
			break
		}
		day = before
	}
	back := b.tab.events.Back(b.readers.filed, day.Start)
	g.from, g.found, g.totals = day, &back, views.TotalsOn(b.readers.ledger, dates, b.now, b.readers.prices)
	return g
}

// took is read taken in the lines of the day given: read, as ReadDay reads
// them, through the filter the Log's read serves, as serving has it, made
// the day's as ThroughOn has it, b's stream telling what's under way; and
// taken in as TakenBack takes them, at b's now, beside the store b holds.
// It holds the sessions the take reads whole, for the watch's to take in.
func (b logBack) took(read views.LogRead, day views.LedgerDay) logGrown {
	l := b.readers.ledger
	through := b.tab.serving().ThroughOn(b.stream, day.Date)
	grown, found := read.TakenBack(l, views.ReadDay(l, day.Date, through), b.held, b.stream, b.now)
	return logGrown{read: grown, held: found}
}

// pastDay is the day before the one given, where a read back reading on
// past it may read it: one the ledger keeps, as the oldest day kept allows,
// the tables spanning from the day given short of the reach of the page's
// Reach, as atReach has it.
func (t logTab) pastDay(day views.LedgerDay) (views.LedgerDay, bool) {
	t.from = day
	before, ok := t.oldest.DayBefore(day)
	return before, ok && !t.atReach()
}

// serving is the filter the Log's read serves, as its page shows: the
// Requests', while they show; every line, the zero LogFilter, while the
// Events show, as an event's card reads the lines held, and a move row its
// session's story, as of every day whole.
func (t logTab) serving() views.LogFilter {
	if t.page.Shown == dashboard.LogEvents {
		return views.LogFilter{}
	}
	return t.page.Filter
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

// logRead takes in what a read for the Log of its own found: the read of
// the tail it opened with, as tailed takes it in; the sessions it read whole
// taken into the store, as heldWith takes them; and the Log grown by it,
// and opened, its events taking in again
// those of the document on screen while the router answers: as it first
// shows, as no look took them in before it opened; and since, as the router
// holds them, in place of the versions the read found on its files.
func (m Model) logRead(msg logReadMsg) Model {
	if msg.tailed != nil {
		m = m.tailed(*msg.tailed)
	}
	if msg.grown.held != nil {
		m = m.heldWith(*msg.grown.held)
	}
	m.log = m.log.grown(msg.grown)
	m.log.opened = true
	m.log = m.log.looked(m.moment(m.now()), m.news.router, m.answering())
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
