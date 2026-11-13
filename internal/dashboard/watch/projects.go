package watch

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/views"
)

// projectsTab is what the watch holds of Projects: what it shows; whether a
// project has been chosen yet, the top row chosen till one is; the sessions'
// days read for it, as views.ReadSessionDays reads them, from the start of
// the day from, none till the first read lands, and how many reads have
// landed; whether a read is under way, and how many were asked, a read
// another overtook passed over; and what its page and the chosen project's
// were built of, as pageOf and projectOf have them, and how many times each
// was built.
type projectsTab struct {
	shown     dashboard.ProjectsShown
	chose     bool
	days      *views.SessionDays
	from      time.Time
	read      int
	reading   bool
	asked     int
	pageOf    projectsPageOf
	projectOf projectOf
	pages     int
	projects  int
}

// projectsPageOf is what Projects' page was built of: the reads of the days
// landed, the period, the local day it was built on, and the sessions
// running, as the List lists them, by what's drawn of each, as runningOf
// keys them.
type projectsPageOf struct {
	read    int
	period  views.Period
	day     string
	running []runningKey
}

// same reports whether the page was built of what o says.
func (b projectsPageOf) same(o projectsPageOf) bool {
	return b.read == o.read && b.period == o.period && b.day == o.day && slices.Equal(b.running, o.running)
}

// projectOf is what the chosen project's page was built of: the page,
// counted as it's built, and the project chosen, by its path.
type projectOf struct {
	page   int
	chosen string
}

// runningKey is a session the List lists running, by what of it Projects
// draws and is built of, but what changes as its answer streams: its id,
// where it runs, its account and model, its mark, and what it's doing.
type runningKey struct {
	id, dir, project, account, model string
	mark                             views.RowMark
	doing                            views.Doing
	since                            time.Time
	status                           int
	stale                            bool
}

// runningOf are the sessions the List given lists running, in its order, by
// what Projects draws of each, as runningKey has it.
func runningOf(list views.SessionList) []runningKey {
	var keys []runningKey
	for _, s := range list.Sessions {
		if s.Running {
			a := s.Activity
			keys = append(keys, runningKey{
				id: s.Session, dir: s.Dir, project: s.Project, account: s.Account, model: s.Model, mark: s.Mark,
				doing: a.Doing, since: a.Since, status: a.Status, stale: a.Stale,
			})
		}
	}
	return keys
}

// projectsReadMsg is a read of the sessions' days for Projects, the asked'th:
// the days, from the start of the day from to now, and today's whole, as
// readProjects read them.
type projectsReadMsg struct {
	asked int
	days  views.SessionDays
	from  time.Time
}

// readProjects has the watch read the sessions' days for Projects while it
// shows, one read at a time: once, as it's first shown, from the start of
// the period's first day; and again where the dial steps to a period
// reaching further back than the days held. Each read reads today's lines
// whole, as the ledger's Day gives them, the tail feeding none of them in
// yet.
func (m Model) readProjects() (Model, tea.Cmd) {
	t := m.projects
	if m.view != dashboard.Projects || t.reading || m.cfg.Ledger == nil {
		return m, nil
	}
	now := m.now()
	from := m.period.From(now, m.cfg.WeekStarts)
	if t.days != nil && !from.Before(t.from) {
		return m, nil
	}
	l := m.cfg.Ledger
	m.projects.asked++
	m.projects.reading = true
	asked := m.projects.asked
	return m, func() tea.Msg {
		days := views.ReadSessionDays(l, from, now).Tailed(l.Day(dayfile.DateOf(now)), true, nil, nil, now)
		return projectsReadMsg{asked: asked, days: days, from: from}
	}
}

// projectsRead takes in a read of the sessions' days for Projects, but one
// another overtook: held in place of those held before, for its page to be
// built afresh of them.
func (m Model) projectsRead(msg projectsReadMsg) Model {
	if msg.asked != m.projects.asked {
		return m
	}
	m.projects.reading = false
	m.projects.days, m.projects.from = &msg.days, msg.from
	m.projects.read++
	return m
}

// projectsBuilt is m with Projects' page built afresh of the days it's
// built of at now, as projectsDays has them, while Projects shows, where
// what it's built of has changed since it was built, as projectsPageOf has
// it: the days read, the period, the day, or the sessions the List lists
// running; and the chosen project's page, where the page or the project
// chosen has, as chosenBuilt has it; SESSIONS scrolled to show the session
// selected, where it showed before, as selectionShows has it. Where there
// are no days to build it of, it shows none, as projectsAwaited has it.
// Never a frame at a time, which takes nothing in.
func (m Model) projectsBuilt(msg tea.Msg) Model {
	if _, frame := msg.(frameMsg); frame || m.view != dashboard.Projects {
		return m
	}
	now := m.now()
	days, ok := m.projectsDays(now)
	if !ok {
		return m.projectsAwaited()
	}
	list := m.held.List(m.listLive(now, m.traffic.stream(now)))
	of := projectsPageOf{read: m.projects.read, period: m.period, day: dayfile.DateOf(now), running: runningOf(list)}
	t := m.projects
	rebuilt := t.shown.Page == nil || !t.pageOf.same(of)
	if !rebuilt && t.projectOf == (projectOf{page: t.pages, chosen: t.chosenIn(*t.shown.Page)}) {
		return m
	}
	sighted := m.selectionShows(now)
	if rebuilt {
		page := days.Projects(views.ProjectsInput{
			Period: m.period, List: list, Names: m.moment(now), Prices: m.cfg.Prices, WeekStarts: m.cfg.WeekStarts, Family: m.cfg.Family, Now: now,
		})
		m.projects.shown.Page, m.projects.pageOf = &page, of
		m.projects.pages++
	}
	m = m.chosenBuilt(now)
	if sighted && m.projects.shown.InSessions {
		m.scroll = m.frame(now).ProjectsSessionsScroll(m.doc, now, m.projects.shown.Selection())
	}
	return m
}

// projectsDays are the days Projects' page is built of at now: those read,
// where they reach back to the period's first day; none where there's no
// ledger, the page built of the sessions running alone. It reports false
// where those read don't reach, as before the first read lands, or as the
// dial reaches further back, till the read of the days it reaches lands.
func (m Model) projectsDays(now time.Time) (views.SessionDays, bool) {
	t := m.projects
	switch {
	case m.cfg.Ledger == nil:
		return views.SessionDays{}, true
	case t.days == nil || m.period.From(now, m.cfg.WeekStarts).Before(t.from):
		return views.SessionDays{}, false
	}
	return *t.days, true
}

// projectsAwaited is m with Projects showing no page while its days are
// read, as before its first read lands: the project chosen and the session
// selected kept, to choose again once it's built, ↑ and ↓ back in the list,
// and the list and SESSIONS from their tops.
func (m Model) projectsAwaited() Model {
	s := &m.projects.shown
	if s.Page == nil {
		return m
	}
	s.Page, s.Project, s.InSessions, s.ListScroll = nil, nil, false, 0
	m.scroll = 0
	return m
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
	period := dashboard.Key{Key: "< >", Does: "period", State: m.period.Name()}
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
