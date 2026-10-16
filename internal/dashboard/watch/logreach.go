package watch

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// keptReach is how far back the lines a filter of the ledger's keeps reach,
// as the days' summaries told, as KeptFrom reads them: the filter, the date
// of the earliest day that may hold one, "" where none may, and whether
// they've been read for it.
type keptReach struct {
	filter views.LogFilter
	from   string
	read   bool
}

// reachMsg is how far back the lines a filter of the ledger's keeps reach,
// as a read of the days' summaries found.
type reachMsg struct {
	reach keptReach
}

// of reports whether r is how far back the lines f keeps reach.
func (r keptReach) of(f views.LogFilter) bool {
	return r.read && r.filter.Equal(f)
}

// filedReach is how far back the events a filter of the Events' keeps by
// their kind and the accounts they name reach, as the router's files hold
// them, as FiledFrom reads them: the filter, its sessions left out, the date
// of the earliest day holding one, "" where none does, and whether they've
// been read for it.
type filedReach struct {
	shows views.EventFilter
	from  string
	read  bool
}

// filedReachMsg is how far back the events a filter of the Events' keeps
// reach, as a read of the router's files found.
type filedReachMsg struct {
	reach filedReach
}

// of reports whether r is how far back the events f keeps by their kind and
// the accounts they name reach.
func (r filedReach) of(f views.EventFilter) bool {
	f.Sessions = nil
	return r.read && r.shows.Equal(f)
}

// reaches reports whether the tables, spanning from the day given, span
// from as far back as from, a reach's earliest day, "" where there's none.
func reaches(from string, span views.LedgerDay) bool {
	return from == "" || from >= span.Date
}

// wantsReach reports whether the table shown, in a view rows rows tall,
// wants the day before, the page's Reach bounding it, not yet known for the
// filter of the lines it gives, and the oldest day the ledger keeps known,
// which the days' summaries are read from.
func (t logTab) wantsReach(rows int) bool {
	f, bounds := t.page.Reach()
	return bounds && !t.reach.of(f) && t.oldest.Day.Date != "" && t.page.WantsDayBefore(rows)
}

// wantsDayBefore reports whether the table shown, in a view rows rows tall,
// wants the day before those the tables span read, as WantsDayBefore has
// it, the page told where it's at its reach, as atReach has it: where the
// page's Reach bounds it, only once the lines of the filter it gives are
// known to reach, as the days' summaries told, and where its kinds or
// accounts bound the Events, as boundsFiled has it, once the events it keeps
// are, as the router's files told. Read back a day at a time, as every day
// from the first the tables span is held.
func (t logTab) wantsDayBefore(rows int) bool {
	f, bounds := t.page.Reach()
	return t.page.WantsDayBefore(rows) && (!bounds || t.reach.of(f)) && (!t.boundsFiled() || t.filedReach.of(t.page.Shows))
}

// boundsFiled reports whether the Events show, through a filter keeping
// some kinds or accounts alone, as views' EventFilter.Bounds has it, the
// router's files known to read from, so how far back the events it keeps
// reach bounds how far back they read, once FiledFrom has read it, as no
// summary tells the days of an event's kind or account.
func (t logTab) boundsFiled() bool {
	return t.page.Shown == dashboard.LogEvents && t.page.Shows.Bounds() && t.filed.Day.Date != ""
}

// wantsFiledReach reports whether the Events, shown in a view rows rows
// tall, want the day before, the events their filter keeps bounding them, as
// boundsFiled has it, not yet known to reach.
func (t logTab) wantsFiledReach(rows int) bool {
	return t.boundsFiled() && !t.filedReach.of(t.page.Shows) && t.page.WantsDayBefore(rows)
}

// atReach reports whether the page's Reach bounds the table shown and the
// tables span from as far back as the lines of the filter it gives reach,
// as the days' summaries told: to the earliest day that may hold one, of a
// session its first day, or where none may; or, of the Events, whether the
// events their filter keeps by kind and account bound them, as boundsFiled
// has it, and the tables span from the earliest day the router filed one,
// or none was. The page fills its table no further back, and of the
// Requests reads back no further at all.
func (t logTab) atReach() bool {
	f, bounds := t.page.Reach()
	kept := bounds && t.reach.of(f) && reaches(t.reach.from, t.from)
	return kept || t.boundsFiled() && t.filedReach.of(t.page.Shows) && reaches(t.filedReach.from, t.from)
}

// readReach reads the summaries of the days the ledger keeps for how far
// back the lines of the filter the page's Reach gives reach.
func (m Model) readReach() (Model, tea.Cmd) {
	l, oldest := m.cfg.Ledger, m.log.oldest.Day
	filter, _ := m.log.page.Reach()
	m.log.reading = true
	return m, func() tea.Msg {
		return reachMsg{reach: keptReach{filter: filter, from: filter.KeptFrom(l.Days(oldest.Start)), read: true}}
	}
}

// reachRead takes in how far back the lines of a filter of the ledger's
// keep reach, as a read of the days' summaries found.
func (m Model) reachRead(msg reachMsg) Model {
	m.log.reading, m.log.reach = false, msg.reach
	return m
}

// readFiledReach reads the router's files of events, from the oldest the
// Events read back to, for how far back the events the Events' filter
// keeps by kind and account reach, as FiledFrom reads them.
func (m Model) readFiledReach() (Model, tea.Cmd) {
	filed, from, now := m.cfg.Events, m.log.filed.Day.Start, m.now()
	shows := m.log.page.Shows
	shows.Sessions = nil
	m.log.reading = true
	return m, func() tea.Msg {
		return filedReachMsg{reach: filedReach{shows: shows, from: shows.FiledFrom(filed, from, now), read: true}}
	}
}

// filedReachRead takes in how far back the events a filter of the Events'
// keeps reach, as a read of the router's files found.
func (m Model) filedReachRead(msg filedReachMsg) Model {
	m.log.reading, m.log.filedReach = false, msg.reach
	return m
}
