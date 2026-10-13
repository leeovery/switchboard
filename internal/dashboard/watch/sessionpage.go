package watch

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// sessionShown is what the watch holds of a session's page while it's open:
// the page as the frame shows it.
type sessionShown struct {
	shown dashboard.SessionShown
}

// frame is the page s holds as the frame draws it: nil while none is open.
func (s *sessionShown) frame() *dashboard.SessionShown {
	if s == nil {
		return nil
	}
	shown := s.shown
	return &shown
}

// sessionKey acts on a key while a session's page is open, as pressed hands
// it on: each as pageKey takes it.
func (m Model) sessionKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return m.pageKey(key)
}
