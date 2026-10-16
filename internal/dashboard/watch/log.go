package watch

import (
	"cmp"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// logTab is what the watch holds of the Log: its page, built afresh of what
// it holds at each change, as builtLog has it, and what each of its builds
// was last made of; once opened, as the Log first shows, the ledger's lines
// read for its Requests and the router's events for its Events, over the
// local days from from, today's grown at each look since and the days
// before read back as the tables want them; the figures of those days,
// which their rules say; how many times what it holds has changed, as
// grown and looked change it, and of those, how many times what its
// Requests read has, as grown changes it; when each day the Requests await
// a line on was last read for it; how far back the lines of the filter the
// page's Reach gives reach, as the days' summaries told; the oldest day the
// ledger keeps, and the oldest the Events read back to, none known till
// each is said, and the date of the day both were last said on; who told
// the events the Events last took in, and when; when the router's files are
// to be read again from for the events a document skipped, as Skips has
// it, none while none did; whether a read for it is under way, one at a
// time; whether a failure to say how far back its tables read has been
// logged, the once; its details card, while it's open; the panel of what a
// table shows, while f has it open; and its find's walk back through the
// days.
type logTab struct {
	page       dashboard.LogPage
	lines      views.LogLines
	built      logBuilt
	opened     bool
	read       views.LogRead
	events     views.EventsRead
	totals     []views.DayTotals
	held       int
	lined      int
	awaited    awaitedReads
	reach      keptReach
	filedReach filedReach
	from       views.LedgerDay
	oldest     views.OldestDay
	filed      views.OldestDay
	boundedOn  string
	told       logTold
	skipped    time.Time
	reading    bool
	warned     bool
	card       logCarding
	panel      dashboard.LogPanel
	find       findWalk
}

// openingLog is the Log as it first shows: its columns as the preferences
// keep them, the lines its filter shows at opening, and every event.
func openingLog(columns dashboard.LogTicks) logTab {
	return logTab{page: dashboard.LogPage{Columns: columns, Filter: views.LogOpening()}}
}

// builtLog is m with the Log's page built at now of what the watch holds,
// once its first read has landed: its Requests of the lines read and, while
// the router answers, what the stream tells of those in flight; its Events
// of the events read; each as its filter has them, over the days from the
// first read, and made again only as what it's made of changes, as
// logBuilt has it, its lines held, as Lined holds them; whether the router
// answers, what the stream tells, which a row in flight is drawn as, and
// to when the ledger wrote; whether the table shown is at its reach, as
// atReach has it; on opening, the table shown filled once it fills its
// view; the details card, where it's open, of what it holds; and
// while it's open, the panel of what a table shows, as builtLogPanel has
// it. Built, it lets go of the days its tables no longer show, as
// lettingGo has it, and is built again of what it holds then.
func (m Model) builtLog(now time.Time) Model {
	if !m.log.opened {
		return m
	}
	m = m.requestsBuilt(now)
	t := m.log
	p := t.page
	p.Read, p.From, p.Today, p.Totals = true, t.from, views.Today(now), t.totals
	p.Live, p.Stream, p.Written, p.AtReach = m.answering(), m.logStream(now), t.read.WrittenTo(), t.atReach()
	if basis := m.eventsBasis(p.Shows, now); !t.built.events.same(basis) {
		p.Events, m.log.built.events = views.LogEvents(m.logEventSources(p.Shows, now)), &basis
	}
	m.log.page = p.Lined().Filling(m.logRows())
	m = m.followedByLogCard().builtLogPanel(now)
	if day, ok := m.log.lettingGo(m.logRows()); ok {
		m.log = m.log.spanningFrom(day)
		return m.builtLog(now)
	}
	return m
}

// lettingGo is the latest day the Log could span from, its tables showing
// in a view rows rows tall all they show now, as the page's KeptFrom has
// it: false while either table is paused, a card or the find is open, or
// lands, a read for it is under way, or a row of its Requests awaits a line
// filed before that day, each of which may reach back past it; or the read
// holds before it the line of a request the stream tells of, as HoldsTold
// has it, whose row would await it there, reading it back again.
func (t logTab) lettingGo(rows int) (views.LedgerDay, bool) {
	p := t.page
	if t.reading || p.Stands[dashboard.LogRequests].Paused() || p.Stands[dashboard.LogEvents].Paused() || t.card.request != nil || t.card.event != nil ||
		p.Finding.Open || t.find.landing != nil {
		return views.LedgerDay{}, false
	}
	day, ok := p.KeptFrom(rows)
	return day, ok && len(p.Requests.Awaited(day)) == 0 && !t.read.HoldsTold(p.Stream, day)
}

// spanningFrom is the Log spanning the days from the one given, a later one than
// it spans from: its read, its events and the figures of its days holding
// none of those before, as From has it, what each holds counted changed.
func (t logTab) spanningFrom(day views.LedgerDay) logTab {
	t.from, t.read, t.events = day, t.read.From(day), t.events.From(day)
	t.totals = slices.DeleteFunc(slices.Clone(t.totals), func(d views.DayTotals) bool { return d.Date < day.Date })
	t.lined++
	t.held++
	return t
}

// requestsBuilt is m with the Log's Requests built at now of what it holds,
// shown or hidden: of the lines read, placed and tabled as views.LogLinesOf
// has them, each where it was placed before; and, while the router answers,
// the rows of what the stream tells of those whose lines are yet to come,
// taken in among them as views.LogLines' Rows has it; each as its filter
// has them, and made again only as what it's made of changes, as logBuilt
// has it, so what the stream tells builds nothing of the lines afresh.
func (m Model) requestsBuilt(now time.Time) Model {
	m = m.linesPlaced(now)
	p := &m.log.page
	if basis := m.requestsBasis(p.Filter, now); !m.log.built.requests.same(basis) {
		p.Requests, m.log.built.requests = m.log.lines.Rows(m.logSources(p.Filter, now)), &basis
	}
	return m
}

// linesPlaced is m with the Log's lines placed and tabled at now, as
// views.LogLinesOf has them, each where it was placed before, those it
// hasn't placed where the stream tells their requests were sent: only as
// they, or the filter, change.
func (m Model) linesPlaced(now time.Time) Model {
	filter := m.log.page.Filter
	if basis := m.linesBasis(filter, now); !m.log.built.lines.same(basis) {
		m.log.lines, m.log.built.lines = views.LogLinesOf(m.log.lines, m.logSources(filter, now)), &basis
	}
	return m
}

// logSources are what the Log's Requests are built from at now, through
// the filter given: the lines read and, while the router answers, what the
// stream tells of those in flight.
func (m Model) logSources(filter views.LogFilter, now time.Time) views.LogSources {
	t := m.log
	return views.LogSources{
		From: t.from.Start, Oldest: t.oldest, Read: t.read, Stream: m.logStream(now), Totals: t.totals, Filter: filter, Now: now,
	}
}

// logEventSources are what the Log's Events are built from at now, through
// the filter given.
func (m Model) logEventSources(filter views.EventFilter, now time.Time) views.LogEventSources {
	t := m.log
	return views.LogEventSources{
		From: t.from.Start, Events: t.events, Held: m.held, Filter: filter, Names: m.cfg.WindowName, Totals: t.totals, Oldest: t.oldest, Filed: t.filed,
		Now: now,
	}
}

// logStream is what the request stream tells at now, while the router
// answers: nothing while it doesn't, as the stream isn't live.
func (m Model) logStream(now time.Time) views.Stream {
	if !m.answering() {
		return views.Stream{}
	}
	return m.traffic.stream(now)
}

// looked is the Log's events with those the document look is a moment of
// gives taken in, told by the router from, while it answers, as answering
// says: a probed document's tell of none, and once the router's gone, the
// document on screen is stale, its files read in its place. Who told them,
// and when, is noted, as logTold has it, the first router's alone till its
// files are read again. Where the document skips events the router told
// since the last, as Skips has it, its files are to be read again from when
// the last was built, or where that's not noted, as since they were read
// again, from where the events begin, the earliest such moment kept till
// they are.
func (t logTab) looked(look status.Moment, from router.Health, answering bool) logTab {
	if !t.opened || !look.Routed || !answering {
		return t
	}
	if t.events.Skips(look, from.StartedAt) && t.skipped.IsZero() {
		t.skipped = cmp.Or(t.told.at, t.events.Since())
	}
	events, changed := t.events.Looked(look, from.StartedAt)
	t.events = events
	if changed {
		t.held++
	}
	if t.told.at.IsZero() || t.told.run.Equal(from.StartedAt) {
		t.told = logTold{run: from.StartedAt, at: look.ReadAt}
	}
	return t
}

// lookedLog is m with the Log's events taken in from the document look is a
// moment of, read from the router from, and the page built afresh at its
// moment while the Log shows, so a number a look changes on it brightens.
func (m Model) lookedLog(look status.Moment, from router.Health) Model {
	m.log = m.log.looked(look, from, m.answering())
	if m.view != dashboard.Log {
		return m
	}
	return m.builtLog(look.Now)
}

// logFrame is the Log's frame at the size drawn at, which lays its page
// out: how many lines its view shows, where its card sits, and whether its
// find has a row.
func (m Model) logFrame() dashboard.Frame {
	return dashboard.Frame{Width: m.size.Width, Height: m.size.Height, View: dashboard.Log, Log: m.log.page}
}

// logRows is how many of the Log's lines its view shows at once, at the
// size drawn at.
func (m Model) logRows() int {
	return m.logFrame().LogRows()
}

// logKey acts on a key of the Log's, reporting false for a key it doesn't
// take, or on another tab: [ and ] turn between Requests and Events; ↑ and
// ↓ choose a line, as Chose has it; end follows again, as esc does while
// the table shown is paused; esc, while it isn't, lets go of what the
// filter keeps alone, as Unfiltered has it; enter opens the line chosen's
// details; s the page of its session, as openLineSession has it; f the
// panel of what the table shows; and / the find.
func (m Model) logKey(key string) (Model, tea.Cmd, bool) {
	if m.shownView() != dashboard.Log {
		return m, nil, false
	}
	p, rows := m.log.page, m.logRows()
	switch {
	case key == "[":
		p = p.Turned(-1)
	case key == "]":
		p = p.Turned(1)
	case key == "up":
		p = p.Chose(-1, rows)
	case key == "down":
		p = p.Chose(1, rows)
	case key == "end", key == "esc" && p.Paused():
		p = p.Followed()
	case key == "esc" && p.Filtered():
		p = p.Unfiltered()
	case key == "enter":
		next, cmd := m.openLogCard()
		return next, cmd, true
	case key == "s":
		next, cmd := m.openLineSession()
		return next, cmd, true
	case key == "f":
		next, cmd := m.openLogPanel()
		return next, cmd, true
	case key == "/":
		next, cmd := m.openLogFind()
		return next, cmd, true
	default:
		return m, nil, false
	}
	m.log.page = p
	return m, nil, true
}

// scrollLog is m with the Log's table shown scrolled as by says, as Scrolled
// has it, a page its view's rows.
func (m Model) scrollLog(by scrollStep) Model {
	rows := m.logRows()
	m.log.page = m.log.page.Scrolled(by.rows+by.pages*rows, rows)
	return m
}

// logListings are the Log's own keys, in the design's order: [ ] view, in a
// group of its own; ↑↓ choosing a request or an event, enter opening its
// details, and paused, end following again, as esc does; then f and /, a
// group of their own while paused, / working where its find has a row, as
// LogFinds says; and with a filter on while following, esc clearing it.
// The key line leaves esc to help.
func (m Model) logListings() []keyListing {
	log, p := m.shownView() == dashboard.Log, m.log.page
	paused := p.Paused()
	return []keyListing{
		{Key: logViewKey, works: log, apart: true},
		{Key: m.logChooseKey(), works: log, apart: true},
		{Key: logOpenKey, works: log},
		{Key: dashboard.Key{Key: "s", Does: "its session"}, works: log && p.ChosenSession() != "", wide: true},
		{Key: logFollowKey, works: log && paused},
		{Key: dashboard.Key{Key: "esc", Does: "follow"}, works: log && paused, helpful: true},
		{Key: dashboard.Key{Key: "f", Does: "what it shows"}, works: log, apart: paused, wide: true},
		{Key: dashboard.Key{Key: "/", Does: "find"}, works: log && m.logFrame().LogFinds(), wide: true},
		{Key: dashboard.Key{Key: "esc", Does: "clear the filter"}, works: log && !paused && p.Filtered(), helpful: true},
	}
}

// The Log's keys a phone's line keeps, as the key line words them.
var (
	logViewKey   = dashboard.Key{Key: "[ ]", Does: "view"}
	logOpenKey   = dashboard.Key{Key: "enter", Does: "open"}
	logFollowKey = dashboard.Key{Key: "end", Does: "follow"}
)

// logChooseKey is ↑↓, choosing a request on Requests and an event on
// Events.
func (m Model) logChooseKey() dashboard.Key {
	if m.log.page.Shown == dashboard.LogEvents {
		return dashboard.Key{Key: "↑↓", Does: "event"}
	}
	return dashboard.Key{Key: "↑↓", Does: "request"}
}

// logPhoneKeys are the Log's keys a phone's line keeps, in the frame's
// order rather than the wide line's: ↑↓, enter open, end follow while
// paused, and [ ] view.
func (m Model) logPhoneKeys() []dashboard.Key {
	keys := []dashboard.Key{m.logChooseKey(), logOpenKey}
	if m.log.page.Paused() {
		keys = append(keys, logFollowKey)
	}
	return append(keys, logViewKey)
}

// openLineSession opens the page of the session the Log's line chosen is
// of, as ChosenSession has it, as s does, as openSession has it, over the
// Log, which waits under it paused at that line for esc to come back to:
// nothing where it's of none.
func (m Model) openLineSession() (Model, tea.Cmd) {
	id := m.log.page.ChosenSession()
	if id == "" {
		return m, nil
	}
	return m.openSession(id)
}
