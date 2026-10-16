package watch

import "github.com/leeovery/switchboard/internal/dashboard"

// sessionsListings are Sessions' own keys, as the key line and help list
// them, on both sub-tabs alike, each move with enter in a group of its own:
// [ ] view; ↑↓ choosing a session; enter opening the one chosen, faint
// till one is, as Opens has it; and while one is, esc deselecting it,
// which the key line leaves to help. A phone's line sets them as
// sessionsPhoneKeys has it.
func (m Model) sessionsListings() []keyListing {
	sessions := m.view == dashboard.Sessions
	return []keyListing{
		{Key: sessionsViewKey, works: sessions, apart: true},
		{Key: sessionsChooseKey, works: sessions, apart: true},
		{Key: m.sessionsOpenKey("its page"), works: sessions},
		{Key: dashboard.Key{Key: "esc", Does: "deselect"}, works: sessions && m.sessionsTab.shown.Opens(), helpful: true},
	}
}

// Sessions' keys that a phone's line keeps, as the key line words them.
var (
	sessionsViewKey   = dashboard.Key{Key: "[ ]", Does: "view"}
	sessionsChooseKey = dashboard.Key{Key: "↑↓", Does: "session"}
)

// sessionsOpenKey is enter, doing what's given, faint while no session is
// chosen to open, as Opens has it.
func (m Model) sessionsOpenKey(does string) dashboard.Key {
	return dashboard.Key{Key: "enter", Does: does, Off: !m.sessionsTab.shown.Opens()}
}

// sessionsPhoneKeys are Sessions' keys a phone's line keeps, in the frame's
// order rather than the wide line's: ↑↓ session, enter open and [ ] view.
func (m Model) sessionsPhoneKeys() []dashboard.Key {
	return []dashboard.Key{sessionsChooseKey, m.sessionsOpenKey("open"), sessionsViewKey}
}
