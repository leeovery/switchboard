package watch

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// logGuardPace is the Log's guarded world: busy enough that the new day's
// rows fill the Log's view, drawn at logGuardSize, a few minutes after
// midnight.
var logGuardPace = soakPace{rate: 1000, step: 5 * time.Second, listed: 3 * time.Minute}

// logGuardSize is the terminal the Log's guard draws at: short, so its view
// holds a few rows.
var logGuardSize = tea.WindowSizeMsg{Width: 160, Height: 12}

// logGuardDays are the days the Log's guarded world has run as the guard
// starts: the day before midnight alone, for the Log to read back to.
const logGuardDays = 1

// The Log's part: the Log open on its Requests across midnight, following,
// read by read. Each read for the Log reads a day only before those its
// tables span, as it reads back, or the day before today in the first
// guardLateFor of the day, for the lines of the requests that ended across
// midnight; and no session whole the store holds, but for a page's, as it
// opens. Once the new day's rows fill its view, it lets go of the day
// before. A while after, a line chosen, the page of its session is opened
// over it, and closed, the Log following again: the page's session is read
// whole once, as it opens, and at every read of the tail while it's open
// the page holds what the page's part holds it to, once compared with a
// page read whole. Paged back, the Log reads the day before again;
// following again, it lets go of it again, its read holding today's lines
// alone, its Events none before today's reach. As it lets go, under the
// page, paged back, and at the end, following again, its Requests are those
// of one read whole, from the day they span from, the store fed it whole,
// and nothing of the Log's reaches a day before it.
func TestTheLogHoldsOnlyWhatItShows(t *testing.T) {
	midnight := dayfile.DayStart(guardStart, 1)
	covered, uncovered := midnight.Add(3*time.Minute+10*time.Second), midnight.Add(3*time.Minute+40*time.Second)
	paged, followed, end := midnight.Add(4*time.Minute), midnight.Add(4*time.Minute+20*time.Second), midnight.Add(5*time.Minute)
	g := &logGuard{t: t, midnight: midnight, checks: []time.Time{covered.Add(20 * time.Second), paged.Add(10 * time.Second), end.Add(-2 * lookEvery)}}
	s := newSoak(t, soakSetup{start: guardStart, days: logGuardDays, pace: logGuardPace, guard: g})
	s.resized(logGuardSize)
	s.show("log")
	page := &pageGuard{t: t, s: s, checks: []time.Time{covered.Add(20 * time.Second)}}
	s.schedule = append(s.schedule,
		soakAction{at: covered, do: func() {
			s.keyed(tea.KeyPressMsg{Code: tea.KeyUp})
			page.id, g.page = s.h.model.log.page.ChosenSession(), page
			s.typed("s")
		}},
		soakAction{at: uncovered, do: func() {
			s.keyed(tea.KeyPressMsg{Code: tea.KeyEscape})
			s.keyed(tea.KeyPressMsg{Code: tea.KeyEnd})
		}},
		soakAction{at: paged, do: func() {
			g.pagedFrom = len(g.daysRead)
			s.pagedBack()
		}},
		soakAction{at: followed, do: func() {
			g.followedFrom = len(g.daysRead)
			s.keyed(tea.KeyPressMsg{Code: tea.KeyEnd})
		}},
	)
	s.runUntil(end)
	today, yesterday := dayfile.DateOf(midnight), dayfile.DateOf(guardStart)
	m := s.h.model
	if g.letGo.IsZero() || !g.letGo.Before(paged) {
		t.Errorf("the Log let go of the day before at %v, want it let go once today's rows filled its view, before %v", g.letGo, paged)
	}
	if !g.underPage {
		t.Error("no read of the ledger landed with the page open over the Log to hold its Requests to one read whole at")
	}
	if page.id == "" || page.wholes != 1 || page.fed == 0 || len(page.checks) > 0 || m.session != nil {
		t.Errorf("over the Log, a page opened of %q, its session read whole %d times, the store fed for it %d times, %d of its compares to one read whole left, still open %t; "+
			"want a page opened of the chosen line's session, read whole once as it opened, the store fed at each read of the tail, compared once, closed by esc",
			page.id, page.wholes, page.fed, len(page.checks), m.session != nil)
	}
	if !slices.Contains(g.daysRead[g.pagedFrom:g.followedFrom], yesterday) {
		t.Errorf("paged back, the Log read the days %v, want the day before, %s, among them", g.daysRead[g.pagedFrom:g.followedFrom], yesterday)
	}
	if m.log.page.Stands[dashboard.LogRequests].Paused() || m.log.from.Date != today || held(m.log.read, "days") != 1 ||
		m.log.events.Since().Before(views.EventsFrom(midnight)) {
		t.Errorf("following again, the Log is paused %t, spans from %s, its read holding %d days, its Events since %v; want it following, from %s, "+
			"its read holding today's alone, its Events none before %v",
			m.log.page.Stands[dashboard.LogRequests].Paused(), m.log.from.Date, held(m.log.read, "days"), m.log.events.Since(), today, views.EventsFrom(midnight))
	}
	if len(g.checks) > 0 {
		t.Errorf("no read of the ledger landed after %v to hold the Log's Requests to one read whole at", g.checks)
	}
	if unwanted := unwantedHeld(m); len(unwanted) > 0 {
		t.Errorf("following again, its page closed, the store holds %v, which neither the Log's days nor the sessions running want", unwanted)
	}
}

// unwantedHeld are the sessions the store m holds that nothing wants it to:
// none of the Log's days' lines, nor running, as the router lists them. The
// List isn't open, to want today's.
func unwantedHeld(m Model) []string {
	wanted := m.log.read.Wanted()
	for _, s := range m.sessions {
		wanted = append(wanted, s.ID)
	}
	var unwanted []string
	for id := range m.held.Holding().Open {
		if !slices.Contains(wanted, id) {
			unwanted = append(unwanted, id)
		}
	}
	return unwanted
}

// logGuard holds the Log to what it shows, read by read: s is the soak;
// unjudged are the reads of the ledger's lines since a read last landed;
// daysRead the dates of the days read whole, in turn; pagedFrom and
// followedFrom how many had been read as the Log was paged back, and as it
// followed again; letGo when the Log first let go of the day before; and
// checks the moments still to hold its Requests to one read whole at, the
// earliest first, the moment it let go among them, and whether one was held
// under a page; and page the guard of the page opened over it, once it's
// opened.
type logGuard struct {
	t            *testing.T
	s            *soak
	midnight     time.Time
	unjudged     []ledgerRead
	daysRead     []string
	pagedFrom    int
	followedFrom int
	letGo        time.Time
	checks       []time.Time
	underPage    bool
	page         *pageGuard
}

func (g *logGuard) reads(s *soak, l views.Ledger) views.Ledger {
	g.s = s
	return &countingLedger{Ledger: l, read: func(r ledgerRead) {
		if g.page == nil {
			g.unjudged = append(g.unjudged, r)
			return
		}
		g.page.counted(r)
		if !g.pageOpening(r) {
			g.unjudged = append(g.unjudged, r)
		}
	}}
}

// pageOpening reports whether r is the read of the page opened over the Log
// as it opens, its session read whole, as the page's part has it, whether or
// not the store holds it; the page guard counts it.
func (g *logGuard) pageOpening(r ledgerRead) bool {
	return g.s.h.model.session == nil && slices.Equal(r.asked, []string{g.page.id})
}

func (g *logGuard) took(msg tea.Msg, was Model) {
	g.t.Helper()
	m := g.s.h.model
	now := m.now()
	if g.letGo.IsZero() && was.log.from.Date < m.log.from.Date && m.log.from.Date == dayfile.DateOf(g.midnight) {
		g.letGo = now
		g.checks = append([]time.Time{now}, g.checks...)
	}
	switch msg := msg.(type) {
	case tailedMsg:
		if g.page != nil {
			g.page.tailed(msg)
		}
	case logReadMsg, listReadMsg:
	default:
		return
	}
	g.judged(was)
	if len(g.checks) > 0 && !now.Before(g.checks[0]) && !m.log.reading {
		g.sameAsOneRead(now)
		g.checks, g.underPage = g.checks[1:], g.underPage || m.session != nil
	}
}

// judged judges the reads of the ledger's lines since a read last landed,
// the model as it was before: a day read whole only before those the Log
// spans, or the day before today in the first guardLateFor of the day; and
// no session read whole that the store holds.
func (g *logGuard) judged(was Model) {
	g.t.Helper()
	now := g.s.h.model.now()
	held := was.held.Holding().Open
	for _, r := range g.unjudged {
		switch r.method {
		case "Day":
			date := r.asked[0]
			g.daysRead = append(g.daysRead, date)
			late := date == dayfile.DateOf(dayfile.DayStart(now, -1)) && now.Sub(dayfile.DayStart(now, 0)) < guardLateFor
			if date >= was.log.from.Date && !late {
				g.t.Errorf("at %s the day %s was read whole, the Log spanning from %s, want only a day before it, or the day before today late", now.Format(time.DateTime), date,
					was.log.from.Date)
			}
		case "SessionsOf", "Session":
			for _, id := range r.asked {
				if _, ok := held[id]; ok {
					g.t.Errorf("at %s %s read %s whole, which the store holds", now.Format(time.DateTime), r.method, id)
				}
			}
		}
	}
	g.unjudged = nil
}

// sameAsOneRead judges the Log's Requests at now, of the lines it has
// tabled read by read, those of one read whole at now: each day's lines the
// ledger holds, from the day the Log spans from, the store fed them whole,
// through the same filter, of what the stream tells; the Log reaching
// nothing of a day before it spans from, by any path, as daysHeld finds;
// and none of the readers holding what they decode once, as heldRaw has it.
func (g *logGuard) sameAsOneRead(now time.Time) {
	g.t.Helper()
	m := g.s.h.model
	heldRaw(g.t, m, now)
	if held := daysHeld(m.log, m.log.from.Date); len(held) > 0 {
		g.t.Errorf("at %s the Log, spanning from %s, still reaches %v", now.Format(time.DateTime), m.log.from.Date, held)
	}
	stream := m.logStream(now)
	read := viewstest.ReadingLog(m.cfg.Prices)
	for day := views.Today(now); !day.Start.Before(m.log.from.Start); day = views.Today(day.Start.Add(-time.Hour)) {
		read = read.Took(g.s.follower, g.s.follower.Day(day.Date), true, stream, now)
	}
	src := m.logSources(m.log.page.Filter, now)
	tabled := m.log.lines.Rows(src)
	src.Read = read.Read
	got, want := shownOf(tabled), shownOf(views.LogRows(src))
	if reflect.DeepEqual(got, want) {
		return
	}
	first := ""
	for i := range min(len(got), len(want)) {
		if !reflect.DeepEqual(got[i], want[i]) {
			first = fmt.Sprintf(": the first unlike is\n%+v\nwant\n%+v", got[i], want[i])
			break
		}
	}
	g.t.Errorf("at %s the Log's Requests, read by read, show %d lines, want the %d of one read whole, alike%s", now.Format(time.DateTime), len(got), len(want), first)
}

// shownOf is what a table of the Log's Requests shows, line by line: each
// row, rule and oldest day's line, and what each row's day counts.
func shownOf(table views.LogTable) []any {
	var shown []any
	for _, l := range table.All() {
		shown = append(shown, views.LogLine{Row: l.Row, Rule: l.Rule, Oldest: l.Oldest})
		if l.IsRow() {
			shown = append(shown, table.Count(views.Today(l.Row.At).Date))
		}
	}
	return shown
}
