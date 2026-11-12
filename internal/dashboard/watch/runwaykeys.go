package watch

import "github.com/leeovery/switchboard/internal/dashboard"

// runwayListings are Runway's own keys, as the key line and help list them,
// live or stepped back alike: in a group of their own, which a phone's line
// keeps, ←→ week, as ← steps back a week; then, apart, a collapse, its word
// faint while the account column is collapsed, as a hidden section's is,
// which a phone's line leaves to help.
func (m Model) runwayListings() []keyListing {
	runway := m.view == dashboard.Runway
	return []keyListing{
		{Key: dashboard.Key{Key: "←→", Does: "week"}, works: runway, apart: true},
		{Key: dashboard.Key{Key: "a", Does: "collapse", Hidden: m.runwayTab.shown.Collapsed}, works: runway, apart: true, wide: true},
	}
}
