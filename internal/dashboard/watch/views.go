package watch

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
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

// shownView is what the watch shows, as a frame's Shown has it: a session's
// page, as dashboard.SessionPage, while one is open over its tab; else its
// tab, the view, which a page open leaves under it. What acts on what's
// shown reads it, so none passes over a page open.
func (m Model) shownView() dashboard.View {
	if m.session != nil {
		return dashboard.SessionPage
	}
	return m.view
}

// turn shows the tab by places on from the one shown, round, as tab and
// shift-tab move.
func (m Model) turn(by int) (tea.Model, tea.Cmd) {
	tabs := dashboard.Views()
	i := slices.Index(tabs, m.view) + by
	return m.showTab(tabs[(i%len(tabs)+len(tabs))%len(tabs)]), nil
}

// showTab shows the tab given, from its top, and keeps it, for the next watch
// to open on, leaving a session's page open over the tab shown, or asked to
// open, as closeSession has it. The tab shown already stays as it is.
func (m Model) showTab(tab dashboard.View) Model {
	m = m.closeSession()
	if tab == m.view {
		return m
	}
	m.view, m.scroll = tab, 0
	m.keepPrefs("the view shown", func(p *theme.Prefs) { p.Tab = string(m.view) }, "view", m.view)
	return m
}

// keepPrefs keeps what change changes in the preferences, where there are
// preferences to keep it in, noting what it is, and attrs saying how it
// stands, where it can't be kept. What can't be kept is shown all the same.
func (m Model) keepPrefs(what string, change func(*theme.Prefs), attrs ...any) {
	if m.cfg.Prefs == nil {
		return
	}
	if err := m.cfg.Prefs.Update(change); err != nil {
		logger.Warn("couldn't keep "+what, append(attrs, "error", err)...)
	}
}

// toggle shows the Overview's section given where it's hidden, and hides it
// where it shows, as its key does, and keeps the sections shown, for the next
// watch to open with.
func (m Model) toggle(section dashboard.Section) Model {
	m.sections = m.sections.Toggled(section)
	names := m.sections.Names()
	m.keepPrefs("the sections shown", func(p *theme.Prefs) { p.Sections = names }, "sections", strings.Join(names, ","))
	return m
}

// regroup switches how the Overview's windows are grouped, as b does, from by
// window to by account, and back, and keeps it, for the next watch to open
// with.
func (m Model) regroup() Model {
	if m.by == dashboard.ByAccount {
		m.by = dashboard.ByWindow
	} else {
		m.by = dashboard.ByAccount
	}
	layout := string(layoutOf(m.by))
	m.keepPrefs("how the windows are grouped", func(p *theme.Prefs) { p.By = layout }, "by", layout)
	return m
}

// layoutOf is the layout the config, and the preferences, name grouping by
// by: window or account.
func layoutOf(by dashboard.By) config.Layout {
	if by == dashboard.ByAccount {
		return config.ByAccount
	}
	return config.ByWindow
}
