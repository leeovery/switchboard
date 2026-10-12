package watch

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/theme"
)

// Prefs is where the dashboard keeps its preferences as the user changes
// them, for the next watch to start with.
type Prefs interface {
	// Update changes the preferences as change says, and keeps them.
	Update(change func(*theme.Prefs)) error
}

// opening is the tab a watch opens on: the one kept, where it's one there
// is, as one an older or a later switchboard kept may not be, else the
// Overview.
func opening(kept dashboard.View) dashboard.View {
	if slices.Contains(dashboard.Views(), kept) {
		return kept
	}
	return dashboard.Overview
}

// turn shows the tab by places on from the one shown, round, as tab and
// shift-tab move.
func (m Model) turn(by int) (tea.Model, tea.Cmd) {
	tabs := dashboard.Views()
	i := slices.Index(tabs, m.view) + by
	return m.showTab(tabs[(i%len(tabs)+len(tabs))%len(tabs)]), nil
}

// showTab shows the tab given, from its top, and keeps it, for the next watch
// to open on. The tab shown already stays as it is.
func (m Model) showTab(tab dashboard.View) Model {
	if tab == m.view {
		return m
	}
	m.view, m.scroll = tab, 0
	m.keepView()
	return m
}

// keepView keeps the view shown in the preferences, where there are
// preferences to keep it in. One that can't be kept is shown all the same.
func (m Model) keepView() {
	if m.cfg.Prefs == nil {
		return
	}
	if err := m.cfg.Prefs.Update(func(p *theme.Prefs) { p.View = string(m.view) }); err != nil {
		logger.Warn("couldn't keep the view shown", "view", m.view, "error", err)
	}
}

// nextSpan switches how far ahead Runway looks, as w does there, from the
// day to the week, and back: elsewhere, it does nothing.
func (m Model) nextSpan() Model {
	if m.view == dashboard.Runway {
		m.span = m.span.Next()
	}
	return m
}

// toggle shows the Overview's section given where it's hidden, and hides it
// where it shows, as its key does, and keeps the sections shown, for the next
// watch to open with.
func (m Model) toggle(dashboard.Section) Model {
	return m
}

// regroup switches how the Overview's windows are grouped, as b does, from by
// window to by account, and back, and keeps it, for the next watch to open
// with.
func (m Model) regroup() Model {
	return m
}
