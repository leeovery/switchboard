package watch

import (
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayListings are Runway's own keys, as the key line and help list them,
// in a group of their own, a phone's line keeping both, live or stepped back
// alike: < > window; and ←→ day, or ←→ week in the week view, as ← steps back
// a day or a week.
func (m Model) runwayListings() []keyListing {
	runway, step := m.view == dashboard.Runway, "day"
	if m.runwayTab.shown.Window == views.RunwayWeek {
		step = "week"
	}
	return []keyListing{
		{Key: dashboard.Key{Key: "< >", Does: "window"}, works: runway, apart: true},
		{Key: dashboard.Key{Key: "←→", Does: step}, works: runway},
	}
}
