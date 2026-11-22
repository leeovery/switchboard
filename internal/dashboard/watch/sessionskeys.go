package watch

import (
	"slices"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// sessionsListings are Sessions' own keys, as the key line and help list
// them: [ ] view; on the List, the dial's keys, of its period as dial says
// it, as dialListings has them; ↑↓ choosing a session;
// enter opening the one chosen, faint till one is, as Opens has it; and
// while one is, esc deselecting it, which the key line leaves to help. On
// the Switchboard, enter
// shares ↑↓'s group, and on the List each key has a group of its own, as
// Projects' do. A phone's line sets them as sessionsPhoneKeys has it.
func (m Model) sessionsListings(dial views.Dial) []keyListing {
	sessions := m.view == dashboard.Sessions
	list := sessions && m.sessionsTab.shown.Shown == dashboard.SessionsList
	return slices.Concat([]keyListing{
		{Key: sessionsViewKey, works: sessions, apart: true},
	}, dialListings(dial, list), []keyListing{
		{Key: sessionsChooseKey, works: sessions, apart: true},
		{Key: m.sessionsOpenKey("its page"), works: sessions, apart: list},
		{Key: dashboard.Key{Key: "esc", Does: "deselect"}, works: sessions && m.sessionsTab.shown.Opens(), helpful: true},
	})
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
// order rather than the wide line's: on the Switchboard, ↑↓ session, enter
// open and [ ] view; on the List, ↑↓ session and [ ] view, the dial's n
// kept to the last, as the design's frame has them.
func (m Model) sessionsPhoneKeys() []dashboard.Key {
	if m.sessionsTab.shown.Shown == dashboard.SessionsList {
		return []dashboard.Key{sessionsChooseKey, sessionsViewKey}
	}
	return []dashboard.Key{sessionsChooseKey, m.sessionsOpenKey("open"), sessionsViewKey}
}
