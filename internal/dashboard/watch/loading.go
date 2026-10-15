package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// LoadingAfter is how long what's shown waits on a read before it says it's
// loading: a read landing sooner draws nothing more, so nothing flashes.
const LoadingAfter = 150 * time.Millisecond

// waiting is the tab shown as it waits on a read of what it draws, as
// awaiting has it, and since when: zero while it doesn't.
type waiting struct {
	view  dashboard.View
	since time.Time
}

// loadingMsg wakes the watch as the tab shown has waited LoadingAfter on a
// read, for its heading to say it's loading.
type loadingMsg struct{}

// pageLoadingMsg is the page the asked'th open asked for, of the session
// with the given id, having waited LoadingAfter on its read.
type pageLoadingMsg struct {
	asked int
	id    string
}

// awaiting reports whether the tab shown waits on a read of the files it's
// drawn of: Accounts and History on the ledger's days, Sessions on its
// List, Runway on the readings history, and the Log on its lines and
// events. A session's page says its own, as pageLoading has it.
func (m Model) awaiting() bool {
	switch m.shownView() {
	case dashboard.Accounts, dashboard.History:
		return m.past.reading
	case dashboard.Sessions:
		return m.sessionsTab.reading
	case dashboard.Runway:
		return m.runwayTab.reading
	case dashboard.Log:
		return m.log.reading
	}
	return false
}

// waited is m noting, at now, when the tab shown began to wait on a read,
// as awaiting has it, and arming loadingMsg for when it will have waited
// LoadingAfter: once, as it begins, or as another tab shown begins to.
func (m Model) waited(now time.Time) (Model, tea.Cmd) {
	shown := m.shownView()
	switch {
	case !m.awaiting():
		m.waiting = waiting{}
	case m.waiting.since.IsZero() || m.waiting.view != shown:
		m.waiting = waiting{view: shown, since: now}
		return m, m.after(LoadingAfter, loadingMsg{})
	}
	return m, nil
}

// loading reports whether the tab shown has waited on a read LoadingAfter or
// longer at now, as waited noted it.
func (m Model) loading(now time.Time) bool {
	return !m.waiting.since.IsZero() && m.waiting.view == m.shownView() && !now.Before(m.waiting.since.Add(LoadingAfter))
}

// pageLoading is m with the page asked to open shown loading, as
// views.AskedPage names it, its read under way LoadingAfter after it was
// asked: not where it has landed, nor where the page was dropped since, or
// another asked to open, as unasked has it.
func (m Model) pageLoading(msg pageLoadingMsg) Model {
	if msg.asked != m.opening || m.arrived >= msg.asked {
		return m
	}
	m.session = &sessionShown{id: msg.id, shown: dashboard.SessionShown{Page: views.AskedPage(msg.id, m.sessions), Home: m.cfg.Home, Loading: true}}
	return m
}
