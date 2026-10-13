package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// openLogPanel opens the panel of what the Log's table shows, as f does:
// nothing yet, the panel being built after the page.
func (m Model) openLogPanel() (Model, tea.Cmd) {
	return m, nil
}

// logPanelKey acts on a key while the Log's panel is open.
func (m Model) logPanelKey(string) (tea.Model, tea.Cmd) {
	return m, nil
}

// logPanel is the Log's panel, as the model stands at now.
func (m Model) logPanel(time.Time) dashboard.Opened {
	return dashboard.LogPanel{}
}
