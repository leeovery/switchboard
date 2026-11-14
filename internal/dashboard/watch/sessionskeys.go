package watch

import "github.com/leeovery/switchboard/internal/dashboard"

// sessionsListings are Sessions' own keys, as the key line and help list
// them: [ ] view; on the List, < > and the period; ↑↓ choosing a session;
// enter opening the one chosen, faint till one is, as Opens has it; while
// one is, esc deselecting it; and on the Switchboard, w, as modelsKey has
// it: the last two the key line leaves to help. On the Switchboard, enter
// shares ↑↓'s group, and on the List each key has a group of its own, as
// Projects' do. A phone's line sets them as sessionsPhoneKeys has it.
func (m Model) sessionsListings() []keyListing {
	sessions := m.view == dashboard.Sessions
	list := sessions && m.sessionsTab.shown.Shown == dashboard.SessionsList
	return []keyListing{
		{Key: sessionsViewKey, works: sessions, apart: true},
		{Key: m.periodKey(), works: list, apart: true},
		{Key: sessionsChooseKey, works: sessions, apart: true},
		{Key: m.sessionsOpenKey("its page"), works: sessions, apart: list},
		{Key: dashboard.Key{Key: "esc", Does: "deselect"}, works: sessions && m.sessionsTab.shown.Opens(), helpful: true},
		{Key: m.modelsKey(), works: sessions && m.listsModels(), helpful: true},
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
// order rather than the wide line's: on the Switchboard, ↑↓ session, enter
// open and [ ] view; on the List, ↑↓ session, [ ] view and < > period, its
// state left off, as the design's frame has them.
func (m Model) sessionsPhoneKeys() []dashboard.Key {
	if m.sessionsTab.shown.Shown == dashboard.SessionsList {
		return []dashboard.Key{sessionsChooseKey, sessionsViewKey, {Key: "< >", Does: "period"}}
	}
	return []dashboard.Key{sessionsChooseKey, m.sessionsOpenKey("open"), sessionsViewKey}
}
