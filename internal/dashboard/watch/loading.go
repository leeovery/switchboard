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
// drawn of: Accounts and History on the ledger's days, and Detail's
// SESSIONS, listing a session, as followed held them for the message, on
// the List held, as the Overview's SESSIONS and Sessions do, which names
// their sessions, till its first read lands, as listAwaited has it; Runway
// on the readings history; Graphs on its live tail, while it reads lanes of
// it, as graphsWindows has them, never on a span Runway stepped back to; and
// the Log on its lines and events, from as it first shows till its first
// read lands, whether or not that read has begun, as it waits for another
// read of the ledger to land first. A session's page says its own, as
// pageLoading has it.
func (m Model) awaiting() bool {
	switch m.shownView() {
	case dashboard.Overview:
		return m.sections.Has(dashboard.SectionSessions) && m.listAwaited()
	case dashboard.Accounts:
		return m.past.reading || m.detailShown() && len(m.accounts.running) > 0 && m.listAwaited()
	case dashboard.History:
		return m.past.reading
	case dashboard.Sessions:
		return m.listAwaited()
	case dashboard.Runway:
		return m.runwayTab.reading
	case dashboard.Graphs:
		return len(m.graphsWindows()) > 0 && m.runwayTab.reading && m.runwayTab.readingLive
	case dashboard.Log:
		return m.log.reading || m.cfg.Ledger != nil && !m.log.opened
	}
	return false
}

// listAwaited reports whether the List, whose lines name the sessions
// running, is awaited: the ledger read, and its first read yet to land,
// whether or not it has begun, as it waits for a read of the tail to land
// first.
func (m Model) listAwaited() bool {
	return m.cfg.Ledger != nil && !m.sessionsTab.listed
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
