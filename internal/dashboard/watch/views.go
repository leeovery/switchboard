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

// opening is the view a watch opens on: the one kept, where it's one of
// views, as one a later switchboard kept may not be, else the first.
func opening(kept dashboard.View, views []dashboard.View) dashboard.View {
	if slices.Contains(views, kept) {
		return kept
	}
	return views[0]
}

// turn shows the view by places on from the one shown, round, as tab and
// shift-tab move, from its top, ending the selection on a card's back, and
// keeps it, for the next watch to open on. With one view, there's nothing
// to move to.
func (m Model) turn(by int) (tea.Model, tea.Cmd) {
	i := slices.Index(m.views, m.view) + by
	next := m.views[(i%len(m.views)+len(m.views))%len(m.views)]
	if next == m.view {
		return m, nil
	}
	m.view, m.selected, m.scroll = next, dashboard.Seat{}, 0
	m.keepView()
	return m, nil
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
// It works in the Accounts view alone, while there's a document to show.
func (m Model) feature() (tea.Model, tea.Cmd) {
	if m.view != dashboard.Accounts || m.updated.IsZero() {
		return m, nil
	}
	m.featured = m.featured.Next(m.doc, m.now(), m.cfg.Policy)
	m.keepFeatured()
	return m, nil
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
