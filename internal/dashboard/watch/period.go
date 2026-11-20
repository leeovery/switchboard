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
	if period.Length() != views.LengthToday {
		return -1
	}
	return now.Local().Hour()
}

// periodStepped is m with the dial's period stepped, by -1 to a shorter one,
// as < steps it, or by 1 to a longer, as > does, its length kept in the
// preferences for the next watch to open with, the page built afresh as
// accountsBuilt has it. The view's shown from its top but on Projects, whose
// SESSIONS keeps its place for projectsBuilt to judge and keep the session
// selected in view by; on Sessions' List, which keeps its place for
// sessionsLooked to judge alike; and on Detail with its SESSIONS held for
// the focus, which keeps its place for sessionsKept to judge. At either end
// it stays as it is.
func (m Model) periodStepped(by int) Model {
	length, stepped := m.period.Length().Stepped(by)
	if !stepped {
		return m
	}
	m.period = length.Now()
	if m.shownView() != dashboard.Projects && !m.listShown() && !m.sessionsHeld() {
		m.scroll = 0
	}
	return m.keepPrefs("the period", func(prefs *theme.Prefs) { dashboard.KeepLength(prefs, length) }, "period", string(length))
}

// periodKey is < >, which steps the dial's period, as the key line and help
// list it on each tab the dial shows on, its state the period's length's
// name.
func (m Model) periodKey() dashboard.Key {
	return dashboard.Key{Key: "< >", Does: "period", State: m.period.Length().Name()}
}
