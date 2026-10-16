package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// openLogPanel opens the panel of what the Log's table shown shows, as f
// does, over the page, picking what it shows now, its figures yet to be
// made.
func (m Model) openLogPanel() (Model, tea.Cmd) {
	m.log.panel, m.log.built.panel = m.log.page.Panel(), nil
	return m.cover(logPanelOpen), nil
}

// logPanelKey acts on a key while the Log's panel is open, which takes
// every key but ctrl+c: ← and → move between its sides; ↑ and ↓ through the
// ticks of the side the cursor's on; space ticks the one it's on; enter
// applies what it picks and closes it; esc closes it, changing nothing; and
// /, as the panel offers, closes it so and opens the find. The rest do
// nothing.
func (m Model) logPanelKey(key string) (tea.Model, tea.Cmd) {
	p := m.log.panel
	switch key {
	case "left":
		p = p.Sided(-1)
	case "right":
		p = p.Sided(1)
	case "up":
		p = p.Moved(-1)
	case "down":
		p = p.Moved(1)
	case "space":
		p = p.Ticked()
	case "enter":
		return m.appliedLogPanel()
	case "esc":
		return m.uncover()
	case "/":
		closed, cmd := m.uncover()
		found, find := closed.openLogFind()
		return found, tea.Batch(cmd, find)
	}
	m.log.panel = p
	return m, nil
}

// clickedLogPanel acts on a click at x along row y while the Log's panel is
// open: on a tick, it ticks it, as space does, the cursor moving to it.
func (m Model) clickedLogPanel(x, y int) Model {
	f, _, _ := m.screen(m.now())
	if p, ok := m.log.panel.Clicked(f, x, y); ok {
		m.log.panel = p
	}
	return m
}

// appliedLogPanel shows what the Log's panel picks and closes it, keeping
// the columns it shows, where they've changed, for the next watch to open
// with: the filters aren't kept, so the Log opens on everything.
func (m Model) appliedLogPanel() (Model, tea.Cmd) {
	was := m.log.page.Columns
	m.log.page = m.log.page.Applied(m.log.panel)
	if kept := m.log.page.Columns; kept != was {
		m.keepPrefs("the Log's columns", func(prefs *theme.Prefs) { prefs.Log = kept.Kept() }, "columns", kept.Count())
	}
	return m.uncover()
}

// builtLogPanel is m with the Log's panel built at now of what the watch
// holds, while it's open: its figures, made again only as what they're made
// of changes, as logBuilt has it; and every account.
func (m Model) builtLogPanel(now time.Time) Model {
	if m.open != logPanelOpen {
		return m
	}
	p := m.log.panel
	if basis := m.panelBasis(p, now); !m.log.built.panel.same(basis) {
		p, m.log.built.panel = m.panelFigures(p, now), &basis
	}
	p.Accounts = m.moment(now).Accounts
	m.log.panel = p
	return m
}

// panelFigures are the Log's panel p's figures made at now: today's lines
// and events counted, those it would show through what it picks among
// them, the lines of the Requests built where it picks what the page
// shows; and today's sessions, those with lines today on Requests', those
// with events today on Events', each as the List has it, and those its
// filter ticks that have none, with what the find found of them, as
// foundSessions has it, their doing told at now.
func (m Model) panelFigures(p dashboard.LogPanel, now time.Time) dashboard.LogPanel {
	src := views.LogSessionSources{
		Read: m.log.read, Held: m.held, Ticked: p.TickedSessions(), Found: m.foundSessions(), Running: m.sessions, Unlisted: !m.answering(), Stream: m.logStream(now),
		Now: now,
	}
	if p.Of == dashboard.LogEvents {
		p.Events = views.LogEvents(m.logEventSources(p.Shows, now)).Today
		src.Events = p.Events.Sessions
	} else {
		p.Lines = m.requestsThrough(p.Filter, now).Count(views.Today(now).Date)
		src.Lines = p.Lines.Sessions
	}
	p.Sessions, p.Now = views.LogSessions(src), now
	return p
}

// requestsThrough are the Log's Requests at now through the filter given:
// those built, where it's the page's, as requestsBuilt built them at now;
// else made of what the Log holds.
func (m Model) requestsThrough(filter views.LogFilter, now time.Time) views.LogTable {
	if p := m.log.page; filter.Equal(p.Filter) {
		return p.Requests
	}
	src := m.logSources(filter, now)
	return views.LogLinesOf(m.log.lines, src).Rows(src)
}

// logPanel is the Log's panel, as it was built last.
func (m Model) logPanel(time.Time) dashboard.Opened {
	return m.log.panel
}
