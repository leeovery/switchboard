package watch

import (
	"maps"
	"slices"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// findWalk is what the watch holds of the Log's find beside its line: the
// sessions of each past day whose lines it has read, by the day's date,
// held for the run, so it walks back through the days once a run; the
// summaries of the days the ledger keeps, which name the sessions of each
// before its lines are read, and whether they're read since the find last
// opened; whether a read for it is under way, one at a time; the session
// enter picked, once it has ended, for the table shown to pause at its
// newest line once the tables reach its day; and the session enter picked
// last, which joins the panel's sessions while the filter keeps it.
type findWalk struct {
	days      map[string][]views.DaySession
	summaries []ledger.Summary
	summed    bool
	reading   bool
	landing   *views.FoundSession
	picked    *views.FoundSession
}

// findReadMsg is what a read for the Log's find found: the summaries of the
// days the ledger keeps, where summed says it read them; else the sessions
// of the past day with the given date, as its lines name them.
type findReadMsg struct {
	summed    bool
	summaries []ledger.Summary
	date      string
	sessions  []views.DaySession
}

// openLogFind opens the Log's find line, as / does, nothing typed, its
// summaries of the days to be read afresh, as a day may have been summarised
// since it was last open.
func (m Model) openLogFind() (Model, tea.Cmd) {
	m.log.page.Finding = dashboard.LogFinding{Open: true}
	m.log.find.summed = false
	return m, nil
}

// finding reports whether the Log's find is open, on the Log, taking every
// key.
func (m Model) finding() bool {
	return m.view == dashboard.Log && m.log.page.Finding.Open
}

// logFindKey acts on a key while the Log's find is open, which takes every
// key but ctrl+c: esc cancels it; enter filters both tables to the session
// picked, as foundPicked has it; ↑ and ↓ step the pick; backspace takes
// back what was typed last; and a printable key, space among them, types,
// the pick going back to the first found. The rest do nothing.
func (m Model) logFindKey(key string) (tea.Model, tea.Cmd) {
	f := m.log.page.Finding
	switch key {
	case "esc":
		f = dashboard.LogFinding{}
	case "enter":
		return m.foundPicked(), nil
	case "up":
		f = f.Stepped(-1)
	case "down":
		f = f.Stepped(1)
	case "backspace":
		_, size := utf8.DecodeLastRuneInString(f.Typed)
		f.Typed, f.Picked = f.Typed[:len(f.Typed)-size], ""
	default:
		r, ok := typedRune(key)
		if !ok {
			return m, nil
		}
		f.Typed, f.Picked = f.Typed+string(r), ""
	}
	m.log.page.Finding = f
	return m, nil
}

// typedRune is what the key types on the find's line: a space, or a
// printable character; false for any other key.
func typedRune(key string) (rune, bool) {
	if key == "space" {
		return ' ', true
	}
	r, size := utf8.DecodeRuneInString(key)
	if size == 0 || size != len(key) || !unicode.IsPrint(r) {
		return 0, false
	}
	return r, true
}

// foundPicked is m with both of the Log's tables filtered to the session
// the find picked, as FoundOnly has it, the find closed, the session joining
// the panel's, as foundSessions has it: where it has ended, its newest line
// to be paused at once the tables reach its day, as landed has it. With
// none found, enter does nothing.
func (m Model) foundPicked() Model {
	s, ok := m.log.page.Finding.Pick()
	if !ok {
		return m
	}
	m.log.page = m.log.page.FoundOnly(s.ID)
	m.log.find.picked, m.log.find.landing = &s, nil
	if s.Ended {
		m.log.find.landing = &s
	}
	return m
}

// foundSessions are the sessions the find picked, which tell the panel what
// the find knows of one its filter ticks, as views.LogSessions lists them:
// the one picked last, none before.
func (m Model) foundSessions() []views.FoundSession {
	if s := m.log.find.picked; s != nil {
		return []views.FoundSession{*s}
	}
	return nil
}

// readFind has the watch, while the Log shows, pause the table shown at the
// newest line of the session the find picked, as landed has it; and while
// the find is open, find afresh at each change what's typed matches, as
// views.FindSessions finds it, of today's sessions and the days read so
// far, walking back a day as walkFind has it.
func (m Model) readFind() (Model, tea.Cmd) {
	if m.view != dashboard.Log {
		return m, nil
	}
	m = m.landed()
	if !m.finding() {
		return m, nil
	}
	now, w := m.now(), m.log.find
	m.log.page.Finding.Found = views.FindSessions(views.FindSources{
		Typed: m.log.page.Finding.Typed, Today: m.logSessions(now), Read: w.days, Summaries: w.summaries, Oldest: m.log.oldest, Now: now,
	})
	return m.walkFind()
}

// walkFind reads for the find, while it's open, one read at a time: the
// summaries of the days the ledger keeps, once each time it opens, once the
// oldest is known; then the lines of the next day it names to read, newest
// first, for its sessions' directories, each day once a run.
func (m Model) walkFind() (Model, tea.Cmd) {
	l, oldest, next := m.cfg.Ledger, m.log.oldest.Day, m.log.page.Finding.Found.Next
	if m.log.find.reading || l == nil || oldest.Date == "" {
		return m, nil
	}
	var read func() tea.Msg
	switch {
	case !m.log.find.summed:
		read = func() tea.Msg { return findReadMsg{summed: true, summaries: l.Days(oldest.Start)} }
	case next != "":
		read = func() tea.Msg { return findReadMsg{date: next, sessions: views.DaySessions(l.Day(next))} }
	default:
		return m, nil
	}
	m.log.find.reading = true
	return m, read
}

// findRead takes in what a read for the find found: the summaries of the
// days, or a day's sessions, held for the run, each read made afresh, so
// the find built of what was held never sees it change.
func (m Model) findRead(msg findReadMsg) Model {
	m.log.find.reading = false
	if msg.summed {
		m.log.find.summaries, m.log.find.summed = msg.summaries, true
		return m
	}
	days := maps.Clone(m.log.find.days)
	if days == nil {
		days = make(map[string][]views.DaySession)
	}
	days[msg.date] = msg.sessions
	m.log.find.days = days
	return m
}

// landingOn is the session the find picked, its newest line to be paused
// at, while the Requests are filtered to it alone: nil once they aren't, as
// esc letting the filter go has them.
func (m Model) landingOn() *views.FoundSession {
	s := m.log.find.landing
	if s == nil || !slices.Equal(m.log.page.Filter.Sessions, []string{s.ID}) {
		return nil
	}
	return s
}

// reaching reports whether the Log's tables are to read back toward the
// day of the session the find picked, to pause at its newest line, as
// landingOn has it: they start after it, and the oldest day the ledger
// keeps allows a day before.
func (m Model) reaching() bool {
	s := m.landingOn()
	if s == nil {
		return false
	}
	_, ok := m.log.oldest.DayBefore(m.log.from)
	return ok && m.log.from.Date > s.Day.Date
}

// landed is m with the table shown paused at the newest line of the session
// the find picked, chosen, as ↑ pauses it from following, once the tables
// reach its day, no read for them under way; or with it let go, the filter
// no longer keeping it alone.
func (m Model) landed() Model {
	switch {
	case m.landingOn() == nil:
		m.log.find.landing = nil
	case !m.reaching() && !m.log.reading:
		m.log.page = m.log.page.Chose(-1, m.logRows())
		m.log.find.landing = nil
	}
	return m
}

// logSessions are today's sessions as the find finds them first, as
// views.LogSessions has them, of the Log's read, today's lines and events
// counted by session, the sessions the router lists, and what the request
// stream tells: none it picked of another day among them, which would be
// found as today's.
func (m Model) logSessions(now time.Time) []views.LogSession {
	p := m.log.page
	return views.LogSessions(views.LogSessionSources{
		Read: m.log.read, Lines: p.Requests.Count(views.Today(now).Date).Sessions, Events: p.Events.Today.Sessions,
		Running: m.sessions, Unlisted: !m.answering(), Stream: m.logStream(now), Now: now,
	})
}

// findKeys are the keys the key line lists while the find is open: ↑↓
// picking among the sessions it finds, enter filtering to the one picked,
// and esc cancelling it.
func findKeys() []dashboard.Key {
	return []dashboard.Key{{Key: "↑↓", Does: "pick"}, {Key: "enter", Does: "filter"}, {Key: "esc", Does: "cancel"}}
}
