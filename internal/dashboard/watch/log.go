package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// logTab is what the watch holds of the Log: its page, built afresh of what
// it holds at each change, as builtLog has it; once opened, as the Log first
// shows, the ledger's lines read for its Requests and the router's events
// for its Events, over the local days from from, today's grown at each look
// since and the days before read back as the tables want them; the figures
// of those days, which their rules say; the oldest day the ledger keeps,
// none known till it says; whether a read for it is under way, one at a
// time; whether a failure to say where the ledger's lines begin has been
// logged, the once; and its details card, while it's open.
type logTab struct {
	page    dashboard.LogPage
	opened  bool
	read    views.LogRead
	events  views.EventsRead
	totals  []views.DayTotals
	from    views.LedgerDay
	oldest  views.OldestDay
	reading bool
	warned  bool
	card    logCarding
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
// first read; whether the router answers, and to when the ledger wrote;
// on opening, the table shown filled once it fills its view; and the
// details card, where it's open, of what it holds.
func (m Model) builtLog(now time.Time) Model {
	t := m.log
	if !t.opened {
		return m
	}
	var stream views.Stream
	if m.answering() {
		stream = m.traffic.stream(now)
	}
	p := t.page
	p.Read, p.From, p.Today, p.Totals = true, t.from, views.Today(now), t.totals
	p.Live, p.Written = m.answering(), t.read.WrittenTo()
	p.Requests = views.LogRows(views.LogSources{
		From: t.from.Start, Oldest: t.oldest, Read: t.read, Stream: stream, Totals: t.totals, Filter: p.Filter, Now: now,
	})
	p.Events = views.LogEvents(views.LogEventSources{
		From: t.from.Start, Events: t.events, Read: t.read, Filter: p.Shows, Names: m.cfg.WindowName, Now: now,
	})
	m.log.page = p.Filling(m.logRows())
	return m.followedByLogCard()
}

// looked is the Log's events with those the document look is a moment of
// gives taken in, told by the router from: a probed document's tell of
// none.
func (t logTab) looked(look status.Moment, from router.Health) logTab {
	if !t.opened || !look.Routed {
		return t
	}
	t.events = t.events.Look(look, from.StartedAt)
	return t
}

// lookedLog is m with the Log's events taken in from the document look is a
// moment of, read from the router from, and the page built afresh at its
// moment while the Log shows, so a number a look changes on it brightens.
func (m Model) lookedLog(look status.Moment, from router.Health) Model {
	m.log = m.log.looked(look, from)
	if m.view != dashboard.Log {
		return m
	}
	return m.builtLog(look.Now)
}

// logRows is how many of the Log's lines its view shows at once, at the
// size drawn at.
func (m Model) logRows() int {
	return dashboard.Frame{Width: m.size.Width, Height: m.size.Height, View: dashboard.Log, Log: m.log.page}.LogRows()
}

// logKey acts on a key of the Log's, reporting false for a key it doesn't
// take, or on another tab: [ and ] turn between Requests and Events; ↑ and
// ↓ choose a line, as Chose has it; end follows again, as esc does while
// the table shown is paused; enter opens the line chosen's details; f the
// panel of what the table shows; and / the find.
func (m Model) logKey(key string) (Model, tea.Cmd, bool) {
	if m.view != dashboard.Log {
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
	case key == "enter":
		next, cmd := m.openLogCard()
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
// details, and paused, s its session where the line chosen is of one, and
// end following again; then f and /, a group of their own while paused.
func (m Model) logListings() []keyListing {
	log, p := m.view == dashboard.Log, m.log.page
	paused := p.Paused()
	return []keyListing{
		{Key: logViewKey, works: log, apart: true},
		{Key: m.logChooseKey(), works: log, apart: true},
		{Key: logOpenKey, works: log},
		{Key: dashboard.Key{Key: "s", Does: "its session"}, works: log && m.chosenSession(), wide: true},
		{Key: logFollowKey, works: log && paused},
		{Key: dashboard.Key{Key: "f", Does: "what it shows"}, works: log, apart: paused, wide: true},
		{Key: dashboard.Key{Key: "/", Does: "find"}, works: log, wide: true},
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

// chosenSession reports whether the Log's line chosen is of a session: a
// request's of one, or an event's.
func (m Model) chosenSession() bool {
	row, event, ok := m.log.page.Chosen()
	switch {
	case !ok:
		return false
	case event != nil:
		return event.Session != ""
	}
	return row.Session != ""
}
