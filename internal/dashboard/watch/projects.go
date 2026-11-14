package watch

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// projectsTab is what the watch holds of Projects: what it shows; whether a
// project has been chosen yet, the top row chosen till one is; what its page
// and the chosen project's were built of, as pageOf and projectOf have them,
// and how many times each was built; and where the user was as its page was
// let go to await a read, nil where it wasn't. The sessions' days it's built
// of are the watch's, shared with Sessions' List, as periodDays has them.
type projectsTab struct {
	shown     dashboard.ProjectsShown
	chose     bool
	pageOf    periodBuiltOf
	projectOf projectOf
	pages     int
	projects  int
	left      *projectsPlace
}

// projectsPlace is where the user was on Projects: whether ↑ and ↓ acted in
// SESSIONS, and how far the list and SESSIONS were scrolled.
type projectsPlace struct {
	inSessions         bool
	listScroll, scroll int
}

// projectOf is what the chosen project's page was built of: the page,
// counted as it's built, and the project chosen, by its path.
type projectOf struct {
	page   int
	chosen string
}

// runningOf are the sessions the List given lists running, in its order, by
// what Projects draws of each, as runningKey has it.
func runningOf(list views.SessionList) []runningKey {
	return listingKeys(list, func(s views.ListedSession) bool { return s.Running })
}

// projectsBuilt is m with Projects' page, while Projects shows, built afresh
// where the days it's built of are ready, as daysReady has it, and what
// it's built of has changed, as projectsRebuilt has it, and shown, as
// projectsShown has it. Where there are no days to build it of, it waits, as
// projectsWaiting has it. Never a frame at a time, which takes nothing in.
func (m Model) projectsBuilt(msg tea.Msg) Model {
	if _, frame := msg.(frameMsg); frame || m.view != dashboard.Projects {
		return m
	}
	now := m.now()
	if !m.daysReady(now) {
		return m.projectsWaiting(now)
	}
	return m.projectsShown(now, m.projectsRebuilt(now))
}

// projectsBuild is a page of Projects built afresh, and what it's built of.
type projectsBuild struct {
	page views.ProjectsPage
	of   periodBuiltOf
}

// projectsRebuilt is Projects' page built afresh at now of the days it's
// built of, as joinedDays has them, where what it's built of has changed
// since it was built, as periodBuiltOf has it: the days read, those the
// ledger's tail fed, the period, the day, or the sessions the List lists
// running, so a change while another tab shows builds it only as Projects
// shows again; nil where it hasn't.
func (m Model) projectsRebuilt(now time.Time) *projectsBuild {
	list := m.held.List(m.listLive(now, m.traffic.stream(now)))
	of := m.daysBuiltOf(now, runningOf(list))
	if m.projects.shown.Page != nil && m.projects.pageOf.same(of) {
		return nil
	}
	return &projectsBuild{page: m.joinedDays().Projects(m.periodInput(now, list)), of: of}
}

// projectsShown is m with Projects' page shown at now: the page built,
// where one is given, else the page shown kept; the chosen project's page
// of it, where the page or the project chosen has changed, as chosenBuilt
// has it; SESSIONS scrolled no further than it now goes, and to show the
// session selected, where it showed before, as selectionShows has it, or
// where the user was in it as the page was let go, as projectsReturned has
// it. m as it is where neither the page nor the project chosen has changed.
func (m Model) projectsShown(now time.Time, built *projectsBuild) Model {
	t := m.projects
	if built == nil && t.projectOf == (projectOf{page: t.pages, chosen: t.chosenIn(*t.shown.Page)}) {
		return m
	}
	sighted := m.selectionShows(now)
	m, returned := m.projectsReturned()
	sighted = sighted || returned
	if built != nil {
		m.projects.shown.Page, m.projects.pageOf = &built.page, built.of
		m.projects.pages++
	}
	m = m.chosenBuilt(now)
	f := m.frame(now)
	m.scroll = min(m.scroll, f.Scrolling(m.doc, now).Most)
	if sighted && m.projects.shown.InSessions {
		f.Scroll = m.scroll
		m.scroll = f.ProjectsSessionsScroll(m.doc, now, m.projects.shown.Selection())
	}
	return m
}

// projectsWaiting is m with Projects waiting at now on what its page is
// built of, as daysWaited has it: the page kept as it was built, and shown,
// as projectsShown has it, so a project chosen meanwhile is followed; else
// let go to await what it's built of, as projectsAwaited has it.
func (m Model) projectsWaiting(now time.Time) Model {
	m, kept := m.daysWaited(now, m.projects.shown.Page != nil, m.projects.pageOf.period)
	if kept {
		return m.projectsShown(now, nil)
	}
	return m.projectsAwaited()
}

// projectsAwaited is m with Projects showing no page while its days are
// read, as before its first read lands, ↑ and ↓ in the list and the list and
// SESSIONS from their tops: the project chosen and the session selected
// kept, and where the user was, as projectsPlace has it, for the page built
// once they land to return to, as projectsReturned has it.
func (m Model) projectsAwaited() Model {
	s := &m.projects.shown
	if s.Page == nil {
		return m
	}
	m.projects.left = &projectsPlace{inSessions: s.InSessions, listScroll: s.ListScroll, scroll: m.scroll}
	s.Page, s.Project, s.InSessions, s.ListScroll = nil, nil, false, 0
	m.scroll = 0
	return m
}

// projectsReturned is m returned to where the user was as its page was let
// go, as projectsAwaited left it, reporting whether ↑ and ↓ return to
// SESSIONS, to show the session selected; and m as it is where it wasn't
// let go. The chosen project's page, as chosenBuilt builds it, lets go of
// SESSIONS where it has no sessions, and of the place where the project
// left the list.
func (m Model) projectsReturned() (Model, bool) {
	left := m.projects.left
	if left == nil {
		return m, false
	}
	m.projects.left = nil
	m.projects.shown.InSessions, m.projects.shown.ListScroll, m.scroll = left.inSessions, left.listScroll, left.scroll
	return m, left.inSessions
}

// chosenBuilt is m with the chosen project's page built afresh at now,
// where the page or the project chosen has changed since it was built: the
// one chosen, where the page lists it, else its top row, as chosenIn has
// it. Where the one chosen leaves the list, the list has the choice again,
// SESSIONS' selection let go and SESSIONS from its top, as before any is
// chosen.
func (m Model) chosenBuilt(now time.Time) Model {
	t := m.projects
	if chosen := t.chosenIn(*t.shown.Page); chosen != t.shown.Chosen {
		t.chose, t.shown.Chosen, t.shown.InSessions, t.shown.Selected = false, chosen, false, ""
		m.scroll = 0
	}
	if chosen := (projectOf{page: t.pages, chosen: t.shown.Chosen}); t.projectOf != chosen {
		t.shown.Project, t.projectOf = nil, chosen
		if p, ok := t.shown.Page.Project(chosen.chosen); ok {
			t.shown.Project, t.shown.Measured = &p, dashboard.MeasureSessions(&p, m.moment(now), now)
		}
		t.projects++
	}
	t.shown.InSessions = t.shown.InSessions && t.shown.Project != nil && len(t.shown.Project.Sessions) > 0
	m.projects = t
	return m
}

// selectionShows reports whether SESSIONS shows the session selected, as
// the frame lays Projects out at now: where ↑ and ↓ act in it, and
// scrolling to show it would scroll it no further.
func (m Model) selectionShows(now time.Time) bool {
	s := m.projects.shown
	return s.InSessions && s.Project != nil && m.frame(now).ProjectsSessionsScroll(m.doc, now, s.Selection()) == m.scroll
}

// chosenIn is the project chosen of the page given, by its path: the one
// chosen, where the page lists it; else its top row's, as before any is
// chosen; "" where it lists none.
func (t projectsTab) chosenIn(page views.ProjectsPage) string {
	if t.chose && slices.ContainsFunc(page.Rows, func(r views.ProjectRow) bool { return r.Path == t.shown.Chosen }) {
		return t.shown.Chosen
	}
	if len(page.Rows) == 0 {
		return ""
	}
	return page.Rows[0].Path
}

// projectsSteps are the ways Projects' keys move: back, -1, or on, 1.
var projectsSteps = map[string]int{"<": -1, ">": 1, "left": -1, "right": 1, "up": -1, "down": 1}

// projectsKey acts on a key of Projects', reporting false for a key it
// doesn't take, or on another tab, and the command it starts, if any: < and
// > step the period, as periodStepped has it; ↑ and ↓ choose a project in
// the list, or a session in SESSIONS, as Choosing has it; → and ← move
// between the two, as Moved has it; and enter opens the session selected,
// in SESSIONS, as openSession opens it. A key that changes what's shown
// shows it as showProjects has it.
func (m Model) projectsKey(key string) (Model, tea.Cmd, bool) {
	if m.shownView() != dashboard.Projects {
		return m, nil, false
	}
	s, by := m.projects.shown, projectsSteps[key]
	var next dashboard.ProjectsShown
	var changed bool
	switch key {
	case "<", ">":
		return m.periodStepped(by), nil, true
	case "up", "down":
		next, changed = s.Choosing(by)
	case "left", "right":
		next, changed = s.Moved(by)
	case "enter":
		if !s.Opens() {
			return m, nil, true
		}
		opened, cmd := m.openSession(s.Selection())
		return opened, cmd, true
	default:
		return m, nil, false
	}
	if changed {
		m = m.showProjects(next)
	}
	return m, nil, true
}

// showProjects shows Projects as given, the project shown held as chosen
// from then on, however it came to be shown: where another project is
// chosen, its page built for it, SESSIONS from its top, the list scrolled
// to show it; and where SESSIONS has the choice, scrolled to show the
// session selected, as the frame lays them out, each from as far as it was
// left.
func (m Model) showProjects(s dashboard.ProjectsShown) Model {
	if s.Chosen != m.projects.shown.Chosen {
		m.scroll = 0
	}
	m.projects.chose = true
	m.projects.shown = s
	m = m.projectsBuilt(nil)
	now := m.now()
	f := m.frame(now)
	m.projects.shown.ListScroll = f.ProjectsListScroll(m.doc, now)
	if m.projects.shown.InSessions {
		m.scroll = f.ProjectsSessionsScroll(m.doc, now, m.projects.shown.Selection())
	}
	return m
}

// projectsClick acts on a click on Projects, reporting false for one it
// doesn't take, or on another tab, or under a session's page: on a
// project's row, it chooses it, as ChosenOn has it; and on a session in
// SESSIONS, it selects it, and on the one selected, opens its page, as
// enter opens it. The page acts as acting has it.
func (m Model) projectsClick(t dashboard.Target) (tea.Model, tea.Cmd, bool) {
	if m.shownView() != dashboard.Projects {
		return m, nil, false
	}
	s := m.projects.shown
	switch t.On {
	case dashboard.OnProject:
		return m.acting().showProjects(s.ChosenOn(t.Project)), nil, true
	case dashboard.OnSession:
		if s.InSessions && t.Session == s.Selection() {
			opened, cmd := m.openSession(t.Session)
			return opened, cmd, true
		}
		return m.acting().showProjects(s.SelectedOn(t.Session)), nil, true
	}
	return m, nil, false
}

// projectsListings are Projects' own keys, each move in a group of its own,
// as the key line sets them: in the list, ↑↓ through the projects, and →
// into the chosen one's sessions, where it has any; in SESSIONS, ↑↓ through
// them, ← back to the list and enter opening the one selected; then < > and
// the period. A phone's line keeps the moves, → worded sessions, and the
// period without its name, but in SESSIONS.
func (m Model) projectsListings() []keyListing {
	shown := m.shownView() == dashboard.Projects
	s := m.projects.shown
	list, sessions := shown && !s.InSessions, shown && s.InSessions
	period := m.periodKey()
	return []keyListing{
		{Key: dashboard.Key{Key: "↑↓", Does: "project"}, works: list, apart: true},
		{Key: dashboard.Key{Key: "→", Does: "its sessions"}, works: list && s.Project != nil && len(s.Project.Sessions) > 0, apart: true, phoneDoes: "sessions"},
		{Key: dashboard.Key{Key: "↑↓", Does: "session"}, works: sessions, apart: true},
		{Key: dashboard.Key{Key: "←", Does: "projects"}, works: sessions, apart: true},
		{Key: dashboard.Key{Key: "enter", Does: "its page", Off: !s.Opens()}, works: sessions, apart: true},
		{Key: period, works: list, apart: true, phoneDoes: "period"},
		{Key: period, works: sessions, apart: true, wide: true},
	}
}
