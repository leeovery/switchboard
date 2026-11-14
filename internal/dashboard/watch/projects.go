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
// landed; the sessions' days the ledger's tail fed, today's at every read
// of it from the first, whether or not Projects has shown, and each day
// gone since it first asked for its own, as projectsFeed has it, and how
// many reads told otherwise of them;
// whether a read is under way, and how many were asked, a read another
// overtook passed over; what its page and the chosen project's were built
// of, as pageOf and projectOf have them, and how many times each was built;
// and where the user was as its page was let go to await a read, nil where
// it wasn't.
type projectsTab struct {
	shown     dashboard.ProjectsShown
	chose     bool
	days      *views.SessionDays
	from      time.Time
	read      int
	tailed    views.SessionDays
	fed       int
	reading   bool
	asked     int
	pageOf    projectsPageOf
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

// projectsPageOf is what Projects' page was built of: the reads of the days
// landed, and the reads of the ledger's tail that told otherwise of the days
// it fed; the period; the local day it was built on, and of today, the
// hour, as todayHour has it; and the sessions running, as the List lists
// them, by what's drawn of each, as runningOf keys them.
type projectsPageOf struct {
	read, fed int
	period    views.Period
	day       string
	hour      int
	running   []runningKey
}

// same reports whether the page was built of what o says.
func (b projectsPageOf) same(o projectsPageOf) bool {
	return b.read == o.read && b.fed == o.fed && b.period == o.period && b.day == o.day && b.hour == o.hour && slices.Equal(b.running, o.running)
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
// the days from the start of the day from to the day before now's, as
// readProjects read them.
type projectsReadMsg struct {
	asked int
	days  views.SessionDays
	from  time.Time
}

// readProjects has the watch read the sessions' days for Projects while it
// shows, one read at a time: once, as it's first shown, from the start of
// the period's first day; and again where the dial steps to a period
// reaching further back than the days held. Each reads the days before
// today's alone, today's the tail's, as projectsDays joins them.
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
		return projectsReadMsg{asked: asked, days: views.ReadSessionDays(l, from, now), from: from}
	}
}

// projectsRead takes in a read of the sessions' days for Projects, but one
// another overtook: held in place of those held before, the days the tail
// fed letting go of those it stands for, as withoutRead has it, for its
// page to be built afresh of them.
func (m Model) projectsRead(msg projectsReadMsg) Model {
	if msg.asked != m.projects.asked {
		return m
	}
	m.projects.reading = false
	m.projects.days, m.projects.from = &msg.days, msg.from
	m.projects.tailed = m.projects.withoutRead(m.projects.tailed)
	m.projects.read++
	return m
}

// withoutRead is days, as the ledger's tail fed them, without those the
// days read for Projects stand for, as views.SessionDays' Without has it:
// every one kept till a read has landed.
func (t projectsTab) withoutRead(days views.SessionDays) views.SessionDays {
	if t.days == nil {
		return days
	}
	return days.Without(*t.days)
}

// projectsFeed is what a read of the ledger's tail feeds the sessions' days
// Projects is built of: the days as the read was asked; whether Projects
// had asked for its own by then, from which on the days keep each day
// gone, else today's alone, as Projects' first read reads every day gone it
// shows; and the dates of the days before today's the read reads again
// whole for them, each once, the oldest first, none till Projects asks.
type projectsFeed struct {
	days  views.SessionDays
	keeps bool
	owed  []string
}

// projectsFeed is what a read of the ledger's tail asked at now feeds the
// sessions' days, as projectsFeed has it: the days read again for them
// those they owe, as Owed has them, and those the tail followed no further,
// as missed has them.
func (m Model) projectsFeed(now time.Time) projectsFeed {
	f := projectsFeed{days: m.projects.tailed, keeps: m.projects.asked > 0}
	if f.keeps {
		owed := slices.Concat(f.days.Owed(now), m.tail.missed(views.Today(now)))
		f.owed = slices.Compact(slices.Sorted(slices.Values(owed)))
	}
	return f
}

// fed is the sessions' days fed, at now, what a read of the tail read: the
// days owed among those it read again whole, and every day it read whole
// taken in, so a line another reader awaited reaches them too; or, till
// Projects asks for its own, today's alone, none read again for them.
func (f projectsFeed) fed(read tailLines, now time.Time) views.SessionDays {
	if !f.keeps {
		return f.days.Tailed(read.gained, read.afresh, nil, nil, now).TodayAlone()
	}
	return f.days.Tailed(read.gained, read.afresh, f.owed, read.days, now)
}

// tailFed is t with the sessions' days as a read of the ledger's tail fed
// them, without those the days read stand for, as withoutRead has it,
// counted as fed where they tell otherwise than those held, as Told has it,
// for Projects' page to be built afresh of them as it shows.
func (t projectsTab) tailFed(days views.SessionDays) projectsTab {
	days = t.withoutRead(days)
	if days.Told(t.tailed) {
		t.fed++
	}
	t.tailed = days
	return t
}

// projectsBuilt is m with Projects' page, while Projects shows, built afresh
// where the days it's built of are ready, as projectsReady has it, and what
// it's built of has changed, as projectsRebuilt has it, and shown, as
// projectsShown has it. Where there are no days to build it of, it waits, as
// projectsWaiting has it. Never a frame at a time, which takes nothing in.
func (m Model) projectsBuilt(msg tea.Msg) Model {
	if _, frame := msg.(frameMsg); frame || m.view != dashboard.Projects {
		return m
	}
	now := m.now()
	if !m.projectsReady(now) {
		return m.projectsWaiting(now)
	}
	return m.projectsShown(now, m.projectsRebuilt(now))
}

// projectsBuild is a page of Projects built afresh, and what it's built of.
type projectsBuild struct {
	page views.ProjectsPage
	of   projectsPageOf
}

// projectsRebuilt is Projects' page built afresh at now of the days it's
// built of, as projectsDays has them, where what it's built of has changed
// since it was built, as projectsPageOf has it: the days read, those the
// ledger's tail fed, the period, the day, or the sessions the List lists
// running, so a change while another tab shows builds it only as Projects
// shows again; nil where it hasn't.
func (m Model) projectsRebuilt(now time.Time) *projectsBuild {
	list := m.held.List(m.listLive(now, m.traffic.stream(now)))
	of := projectsPageOf{read: m.projects.read, fed: m.projects.fed, period: m.period, day: dayfile.DateOf(now), hour: todayHour(m.period, now), running: runningOf(list)}
	if m.projects.shown.Page != nil && m.projects.pageOf.same(of) {
		return nil
	}
	return &projectsBuild{page: m.projectsDays().Projects(m.projectsInput(now, list)), of: of}
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

// projectsInput is what Projects' page is built of at now beside its days:
// the dial's period, the List given, the accounts' names, the prices, the
// day weeks start on and how a model's family is read.
func (m Model) projectsInput(now time.Time, list views.SessionList) views.ProjectsInput {
	return views.ProjectsInput{
		Period: m.period, List: list, Names: m.moment(now), Prices: m.cfg.Prices, WeekStarts: m.cfg.WeekStarts, Family: m.cfg.Family, Now: now,
	}
}

// projectsReady reports whether Projects' page can be built at now: of the
// sessions running alone, where there's no ledger; else where the days read
// reach back to the period's first day, and the tail was last read today,
// so its fold of a day gone never stands for that day. Not before the first
// read lands, nor before the tail's first read of the day, as just past
// midnight or after a night asleep, nor as the dial reaches further back,
// till the read of the days it reaches lands.
func (m Model) projectsReady(now time.Time) bool {
	return m.cfg.Ledger == nil || m.projectsReached(now) && m.tail.readOn(views.Today(now))
}

// projectsReached reports whether the days read for Projects reach back to
// the first day of the period at now.
func (m Model) projectsReached(now time.Time) bool {
	t := m.projects
	return t.days != nil && !m.period.From(now, m.cfg.WeekStarts).Before(t.from)
}

// projectsWaiting is m with Projects waiting at now on what its page is
// built of, as projectsReady has it. Where the tail alone is yet to be read
// on the day, as just past midnight or after a night asleep, a read of it is
// due at once, not at the next look, where none is under way; and the page,
// where it was built for the period shown, is kept as it was built, never
// built again of the fold of a day gone, and shown, as projectsShown has it,
// so a project chosen meanwhile is followed. Else it's let go to await what
// it's built of, as projectsAwaited has it.
func (m Model) projectsWaiting(now time.Time) Model {
	if !m.projectsReached(now) {
		return m.projectsAwaited()
	}
	m.tail.due = m.tail.due || !m.tail.busy()
	if m.projects.shown.Page != nil && m.projects.pageOf.period == m.period {
		return m.projectsShown(now, nil)
	}
	return m.projectsAwaited()
}

// projectsDays are the days Projects' page is built of, once they're ready,
// as projectsReady has it: those read, joined to those the ledger's tail
// fed, as Joined joins them, so today's are the tail's, however late a read
// lands; none where there's no ledger.
func (m Model) projectsDays() views.SessionDays {
	if m.cfg.Ledger == nil {
		return views.SessionDays{}
	}
	return m.projects.tailed.Joined(*m.projects.days)
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
