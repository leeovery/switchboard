package watch

import (
	tea "charm.land/bubbletea/v2"

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
// it: where the page's Reach bounds it, only while the lines of the filter
// it gives reach further back than the first day the tables span, so it
// reads back no further than the earliest day that may hold one, of a
// session its first day, and none where no day may. Read back a day at a
// time, as every day from the first the tables span is held.
func (t logTab) wantsDayBefore(rows int) bool {
	if !t.page.WantsDayBefore(rows) {
		return false
	}
	f, bounds := t.page.Reach()
	return !bounds || t.reach.of(f) && t.reach.from != "" && t.reach.from < t.from.Date
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
