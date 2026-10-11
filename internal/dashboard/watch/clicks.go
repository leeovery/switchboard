package watch

import tea "charm.land/bubbletea/v2"

// clicked acts on a click, as the page's targets say, where it changes only
// what's shown. Until clicks are built, it does nothing.
func (m Model) clicked(tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	return m, nil
}
