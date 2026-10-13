package watch

import (
	tea "charm.land/bubbletea/v2"
)

// openLogFind opens the Log's find line, as / does: nothing yet, the find
// being built after the page.
func (m Model) openLogFind() (Model, tea.Cmd) {
	return m, nil
}

// finding reports whether the Log's find is open, taking every key: as
// yet, never.
func (m Model) finding() bool {
	return false
}

// logFindKey acts on a key while the Log's find is open.
func (m Model) logFindKey(string) (tea.Model, tea.Cmd) {
	return m, nil
}
