package watch

import (
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// todayHour is the local hour now falls in, where the period is today, as
// today's charts mark it now's, else -1: what a page of today is built of
// besides, so it's built afresh as the hour turns, though nothing arrives.
func todayHour(period views.Period, now time.Time) int {
	if period != views.PeriodToday {
		return -1
	}
	return now.Local().Hour()
}

// periodStepped is m with the dial's period stepped, by -1 to a shorter one,
// as < steps it, or by 1 to a longer, as > does, the view shown from its top
// but Projects, whose SESSIONS keeps its place for projectsBuilt to judge
// and keep the session selected in view by, and kept in the preferences for
// the next watch to open with, the page built afresh as accountsBuilt has
// it. At either end it stays as it is.
func (m Model) periodStepped(by int) Model {
	period, stepped := m.period.Stepped(by)
	if !stepped {
		return m
	}
	m.period = period
	if m.shownView() != dashboard.Projects {
		m.scroll = 0
	}
	return m.keepPrefs("the period", func(prefs *theme.Prefs) { dashboard.KeepPeriod(prefs, period) }, "period", string(period))
}
