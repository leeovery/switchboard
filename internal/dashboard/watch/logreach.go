package watch

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// keptReach is how far back the lines a filter of the Requests' keeps
// reach, as the days' summaries told, as KeptFrom reads them: the filter,
// the date of the earliest day that may hold one, "" where none may, and
// whether they've been read for it.
type keptReach struct {
	filter views.LogFilter
	from   string
	read   bool
}

// reachMsg is how far back the lines a filter of the Requests' keeps reach,
// as a read of the days' summaries found.
type reachMsg struct {
	reach keptReach
}

// of reports whether r is how far back the lines f keeps reach.
func (r keptReach) of(f views.LogFilter) bool {
	return r.read && r.filter.Equal(f)
}

// reachBounds reports whether how far back the lines the filter of the
// table shown keeps reach bounds how far back it reads: the Requests'
// through a filter choosing some sessions, accounts or models, as Chooses
// has it, whose days the days' summaries tell. Read back a day at a time,
// as every day from the first the tables span is held, it reads no further
// than the earliest of them, of a session alone its first day.
func (t logTab) reachBounds() bool {
	return t.page.Shown == dashboard.LogRequests && t.page.Filter.Chooses()
}

// wantsReach reports whether the table shown, in a view rows rows tall,
// wants the day before, its reach bounding it, as reachBounds has it, not
// yet known for the filter it shows through, and the oldest day the ledger
// keeps known, which the days' summaries are read from.
func (t logTab) wantsReach(rows int) bool {
	return t.reachBounds() && !t.reach.of(t.page.Filter) && t.oldest.Day.Date != "" && t.page.WantsDayBefore(rows)
}

// wantsDayBefore reports whether the table shown, in a view rows rows tall,
// wants the day before those the tables span read, as WantsDayBefore has
// it: where its reach bounds it, only while the lines its filter keeps reach
// further back than the first day they span.
func (t logTab) wantsDayBefore(rows int) bool {
	if !t.page.WantsDayBefore(rows) {
		return false
	}
	return !t.reachBounds() || t.reach.of(t.page.Filter) && t.reach.from != "" && t.reach.from < t.from.Date
}

// readReach reads the summaries of the days the ledger keeps for how far
// back the lines the Requests' filter keeps reach.
func (m Model) readReach() (Model, tea.Cmd) {
	l, oldest, filter := m.cfg.Ledger, m.log.oldest.Day, m.log.page.Filter
	m.log.reading = true
	return m, func() tea.Msg {
		return reachMsg{reach: keptReach{filter: filter, from: filter.KeptFrom(l.Days(oldest.Start)), read: true}}
	}
}

// reachRead takes in how far back the lines a filter of the Requests' keeps
// reach, as a read of the days' summaries found.
func (m Model) reachRead(msg reachMsg) Model {
	m.log.reading, m.log.reach = false, msg.reach
	return m
}
