package watch

import (
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// graphsHoursWanted reports whether Graphs wants the ledger's days: while it
// shows, WHEN YOU WORK among its sections. History's reads read them, and
// Graphs shares what they hold, as Accounts does.
func (m Model) graphsHoursWanted() bool {
	return m.shownView() == dashboard.Graphs && m.graphs.Has(dashboard.GraphsWhen)
}

// graphsHours is WHEN YOU WORK's page at now, of the last 12 months' days
// History's reads hold, never a calendar year stepped back to, so its four
// weeks move on at midnight before a look's read lands, as Accounts' do:
// nil where Graphs doesn't want them, as graphsHoursWanted says, or they
// aren't read yet.
func (m Model) graphsHours() *views.HoursPage {
	if !m.graphsHoursWanted() || !m.past.last.held() {
		return nil
	}
	return new(m.past.last.past.Since(nil, m.now()).Hours())
}

// graphsShown is what Trends shows: its sections shown; the configured
// accounts, which lay its page out; what graphsBuilt built at the last look,
// its lanes, POOL's stack, GLANCE's pool's room over the week's span and
// WHEN YOU WORK's page; and what v has YEAR show, and the pages trendsBuilt
// built of the ledger's days, once the last 12 months are read.
func (m Model) graphsShown() dashboard.GraphsShown {
	t, p := m.graphsTab, m.trends
	return dashboard.GraphsShown{
		Sections: m.graphs, Accounts: m.cfg.Accounts, Lanes: t.shown, Hours: t.hours, Pool: t.pool, Spans: t.spans,
		Split: p.split, Year: p.year, Days: p.days, Read: m.past.last.held(),
	}
}
