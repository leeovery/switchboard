package watch

import (
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayListings are Runway's own keys, as the key line and help list them,
// live or stepped back alike: in a group of their own, a phone's line keeping
// both, < > window, where Runway offers another window to show, as Switches
// has it, and ←→ day, or ←→ week in the week view, as ← steps back a day or
// a week; then, apart, a names, its word faint while the account column is
// collapsed, as a hidden section's is, which a phone's line leaves to help.
func (m Model) runwayListings() []keyListing {
	shown := m.runwayTab.shown
	runway, switches, step := m.view == dashboard.Runway, shown.Switches(), "day"
	if shown.Window == views.RunwayWeek {
		step = "week"
	}
	return []keyListing{
		{Key: dashboard.Key{Key: "< >", Does: "window"}, works: runway && switches, apart: true},
		{Key: dashboard.Key{Key: "←→", Does: step}, works: runway, apart: !switches},
		{Key: dashboard.Key{Key: "a", Does: "names", Hidden: shown.Collapsed}, works: runway, apart: true, wide: true},
	}
}
