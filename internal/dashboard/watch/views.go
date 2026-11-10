package watch

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/theme"
)

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
	return m.keepPrefs("the view shown", func(p *theme.Prefs) { p.Tab = string(tab) }, "view", tab)
}

// toggle shows the Overview's section given where it's hidden, and hides it
// where it shows, as its key does, and keeps the sections shown, for the next
// watch to open with.
func (m Model) toggle(section dashboard.Section) Model {
	m.sections = m.sections.Toggled(section)
	names := m.sections.Names()
	return m.keepPrefs("the sections shown", func(p *theme.Prefs) { p.Sections = names }, "sections", strings.Join(names, ","))
}

// toggleGraphs shows Graphs' section given where it's hidden, and hides it
// where it shows, as its key does, and keeps the sections shown, for the
// next watch to open with.
func (m Model) toggleGraphs(section dashboard.GraphsSection) Model {
	m.graphs = m.graphs.Toggled(section)
	names := m.graphs.Names()
	return m.keepPrefs("Graphs' sections shown", func(p *theme.Prefs) { p.GraphsTab.Sections = names }, "sections", strings.Join(names, ","))
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
	return m.keepPrefs("how the windows are grouped", func(p *theme.Prefs) { p.By = layout }, "by", layout)
}

// toggleModels shows each model's own window, as Fable's week, where the
// live views hide it, and hides it where they show it, as w does, and keeps
// it, for the next watch to open with.
func (m Model) toggleModels() Model {
	m.models = !m.models
	shown := m.models
	return m.keepPrefs("whether models' windows show", func(p *theme.Prefs) { p.Models = shown }, "models", shown)
}

// listsModels reports whether the page shown lists windows as they stand
// now, each model's own among them while w shows them: the Overview,
// Accounts' Detail, Sessions' Switchboard and a session's page.
func (m Model) listsModels() bool {
	switch m.shownView() {
	case dashboard.Overview, dashboard.SessionPage:
		return true
	case dashboard.Accounts:
		return m.accounts.shown.View() == dashboard.AccountsDetail
	case dashboard.Sessions:
		return m.sessionsTab.shown.Shown == dashboard.SessionsSwitchboard
	}
	return false
}

// modelsKey is w, as the key line and help list it, its word faint while
// each model's own window is hidden, as a hidden section's is.
func (m Model) modelsKey() dashboard.Key {
	return dashboard.Key{Key: "w", Does: "models", Hidden: !m.models}
}

// layoutOf is the layout the config, and the preferences, name grouping by
// by: window or account.
func layoutOf(by dashboard.By) config.Layout {
	if by == dashboard.ByAccount {
		return config.ByAccount
	}
	return config.ByWindow
}
