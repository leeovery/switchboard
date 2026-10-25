package watch

import "github.com/leeovery/switchboard/internal/dashboard"

// graphsHoursWanted reports whether Graphs wants the ledger's days: while it
// shows, WHEN YOU WORK among its sections. History's reads read them, and
// Graphs shares what they hold, as Accounts does.
func (m Model) graphsHoursWanted() bool {
	return m.shownView() == dashboard.Graphs && m.graphs.Has(dashboard.GraphsWhen)
}

// graphsShown is what the Graphs tab shows: its sections shown, and, while
// it wants the ledger's days, as graphsHoursWanted says, and History's reads
// hold the last 12 months, WHEN YOU WORK's page of them.
func (m Model) graphsShown() dashboard.GraphsShown {
	shown := dashboard.GraphsShown{Sections: m.graphs}
	if m.graphsHoursWanted() && m.past.last.held() {
		shown.Hours = new(m.past.last.past.Hours())
	}
	return shown
}
