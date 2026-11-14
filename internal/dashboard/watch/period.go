package watch

import (
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/theme"
)

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

// periodKey is < >, which steps the dial's period, as the key line and help
// list it on each tab the dial shows on, its state the period's name.
func (m Model) periodKey() dashboard.Key {
	return dashboard.Key{Key: "< >", Does: "period", State: m.period.Name()}
}
