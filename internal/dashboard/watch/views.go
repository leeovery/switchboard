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
	if err := m.cfg.Prefs.Update(func(p *theme.Prefs) { p.Tab = string(m.view) }); err != nil {
		logger.Warn("couldn't keep the view shown", "view", m.view, "error", err)
	}
}

// nextWindow moves on the window w moves through, as the view shown has
// it: in Runway, from the day to the week, and back; elsewhere, the window
// the cards feature, as feature says.
func (m Model) nextWindow() (tea.Model, tea.Cmd) {
	if m.view == dashboard.Runway {
		m.span = m.span.Next()
		return m, nil
	}
	return m.feature()
}

// feature has every card feature the next window w moves to, as the
// document on screen stands, and keeps it, for the next watch to open with.
// It works on the Overview alone, while there's a document to show.
func (m Model) feature() (tea.Model, tea.Cmd) {
	if m.view != dashboard.Overview || m.updated.IsZero() {
		return m, nil
	}
	m.featured = m.featured.Next(m.doc, m.now(), m.cfg.Policy)
	m.keepFeatured()
	return m, nil
}

// nextChart has every card draw its chart in the next style g moves to, and
// keeps it, for the next watch to open with. It works on the Overview alone.
func (m Model) nextChart() Model {
	if m.view != dashboard.Overview {
		return m
	}
	m.chart = m.chart.Next()
	m.keepChart()
	return m
}

// keepChart keeps the style every card draws its chart in, in the
// preferences, where there are preferences to keep it in. One that can't be
// kept is drawn all the same.
func (m Model) keepChart() {
	if m.cfg.Prefs == nil {
		return
	}
	if err := m.cfg.Prefs.Update(func(p *theme.Prefs) { p.Chart = string(m.chart) }); err != nil {
		logger.Warn("couldn't keep the chart style", "chart", m.chart.Name(), "error", err)
	}
}

// keepFeatured keeps the window every card features in the preferences,
// where there are preferences to keep it in. One that can't be kept is
// featured all the same.
func (m Model) keepFeatured() {
	if m.cfg.Prefs == nil {
		return
	}
	if err := m.cfg.Prefs.Update(func(p *theme.Prefs) { p.Featured = string(m.featured) }); err != nil {
		logger.Warn("couldn't keep the window the cards feature", "window", m.featured, "error", err)
	}
}
