package dashboard

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// projectRowOf is a project's row of the list, named as given, its last
// request at 14:33 on routedAt's day, all its worth opus's.
func projectRowOf(name string, sessions, requests, cents int) views.ProjectRow {
	return views.ProjectRow{
		Path: "/work/Code/" + name, Name: name,
		ProjectTotals: views.ProjectTotals{Sessions: sessions, Requests: requests, Worth: worthOf(cents)},
		ByFamily:      []views.FamilyWorth{{Family: "opus", Worth: worthOf(cents)}},
		Last:          routedClock(1, 14, 33),
	}
}

// projectRows are n projects' rows, the one worth most first, the first
// named with seventeen cells, to be cut.
func projectRows(n int) []views.ProjectRow {
	rows := []views.ProjectRow{projectRowOf("agentic-workflows", 12, 2059, 23900)}
	for i := 1; i < n; i++ {
		rows = append(rows, projectRowOf(fmt.Sprintf("project-%d", i), 3, 412, 9000-i*100))
	}
	return rows
}

// projectSessionOf is a session of the chosen project, on work, doing as
// given, started at started and ended on the date given, "" where it runs.
func projectSessionOf(id string, doing views.Activity, started time.Time, endedOn string) views.ProjectSession {
	s := listedSession{id: id, dir: "~/Code/agentic-workflows", on: "work", doing: doing, started: started, requests: 40, worth: 1200}.listed()
	if endedOn != "" {
		s.Ended = doing.Since
	}
	return views.ProjectSession{ListedSession: s, EndedOn: endedOn}
}

// endedOn is a session's Activity having ended on the day days before
// routedAt's, at the time given.
func endedOn(days, hour, minute int) views.Activity {
	return views.Activity{Doing: views.Ended, Since: routedClock(1, hour, minute).AddDate(0, 0, -days)}
}

// chosenSessions are the chosen project's sessions, as the List orders
// them: one running, started yesterday; then one ended today; two
// yesterday, the later started the day before; one on Tuesday; and one ten
// days before.
func chosenSessions() []views.ProjectSession {
	return []views.ProjectSession{
		projectSessionOf("d28c5e17", answering, yesterdayClock(15, 8), ""),
		projectSessionOf("5b19d3e7", endedAt(11, 21), routedClock(1, 10, 2), "2026-10-01"),
		projectSessionOf("3d7a8e12", endedOn(1, 16, 40), yesterdayClock(9, 0).AddDate(0, 0, -1), "2026-09-30"),
		projectSessionOf("e0c4f5a9", endedOn(1, 10, 2), yesterdayClock(9, 12), "2026-09-30"),
		projectSessionOf("a8f1b2c6", endedOn(2, 9, 28), yesterdayClock(8, 31).AddDate(0, 0, -1), "2026-09-29"),
		projectSessionOf("2b6d9f04", endedOn(10, 9, 5), yesterdayClock(7, 58).AddDate(0, 0, -9), "2026-09-21"),
	}
}

// chosenPage is the page of the first of projectRows, this week by day,
// Thursday's peak, three days to come; on two versions and three accounts;
// its sessions those given.
func chosenPage(sessions []views.ProjectSession) views.ProjectPage {
	half, third := 0.5, 1.0/3
	page := views.ProjectPage{ProjectRow: projectRowOf("agentic-workflows", 12, 2059, 23900), Share: &half, Sessions: sessions}
	page.Tokens.Total = 365_000_000
	page.WorthBy = views.WorthBy{Unit: views.ByDay, Peak: 3, ToCome: 3}
	for i, day := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		page.WorthBy.Buckets = append(page.WorthBy.Buckets, views.WorthBucket{Start: routedClock(1, 0, 0).AddDate(0, 0, i-3), Label: day, Axis: day, Worth: worthOf((i + 1) * 1000 % 5000)})
	}
	for _, v := range []string{"Opus 5.5", "Sonnet 5"} {
		page.ByModel = append(page.ByModel, views.VersionShare{Version: ledger.Version{Name: v, Family: "opus", Shade: 1}, ModelShare: views.ModelShare{Share: &half}})
	}
	for _, a := range []string{"work", "personal", "side"} {
		page.OnAccounts = append(page.OnAccounts, views.AccountShare{Account: a, Worth: worthOf(7000), Share: &third})
	}
	return page
}

// projectsFrame is a frame width by height showing Projects in nord this
// week, of the rows given, the one at chosen chosen, its page project.
func projectsFrame(t *testing.T, width, height int, rows []views.ProjectRow, chosen int, project *views.ProjectPage) Frame {
	t.Helper()
	f := frameOf(width, height)
	f.Look = Screen(builtin(t, "nord"))
	f.View, f.Projects = Projects, projectsShownOf(rows, chosen, project)
	return f
}

// projectsShownOf is Projects of the rows given, the one at chosen chosen,
// its page project.
func projectsShownOf(rows []views.ProjectRow, chosen int, project *views.ProjectPage) ProjectsShown {
	page := views.ProjectsPage{Rows: rows, All: views.ProjectTotals{Sessions: 27, Requests: 5410, Worth: worthOf(63500)}}
	return ProjectsShown{Chosen: rows[chosen].Path, Page: &page, Project: project}
}

// The list's columns give way as it narrows: the bar narrows to its least
// first, so wherever a column comes back as the list widens the bar is at
// its least; requests leave, then last, and only then is the name cut; the
// bar never leaves.
func TestTheProjectsListsColumnsGiveWayInTheirOrder(t *testing.T) {
	rows := projectRows(3)
	whole := len(rows[0].Name)
	shown := 0
	for width := 40; width <= 220; width++ {
		f := projectsFrame(t, width, 53, rows, 0, nil)
		columns := projectsListOf(f, 10, true, routedAt).columns
		at := func(p projectsPart) (projectColumn, bool) {
			i := slices.IndexFunc(columns, func(c projectColumn) bool { return c.part == p })
			if i < 0 {
				return projectColumn{}, false
			}
			return columns[i], true
		}
		bar, barShown := at(listedBar)
		_, requests := at(listedRequests)
		_, last := at(listedLast)
		name, _ := at(listedName)
		switch {
		case !barShown || bar.width < projectBarLeast:
			t.Errorf("at %d the bar is %+v, want it at %d cells at the fewest", width, bar, projectBarLeast)
		case len(columns) > shown && shown > 0 && bar.width != projectBarLeast:
			t.Errorf("at %d a column came back with the bar %d cells, want it at its least first", width, bar.width)
		case !last && requests:
			t.Errorf("at %d last left before requests", width)
		case name.width < whole && last:
			t.Errorf("at %d the name is cut to %d with last shown, want last gone first", width, name.width)
		}
		shown = len(columns)
	}
	if shown != 7 {
		t.Errorf("at 220 the list has %d columns, want all 7", shown)
	}
}

// The chosen project's sections leave as rows run short, ON ACCOUNTS first,
// then BY MODEL, then WORTH BY, SESSIONS keeping three of its lines, in each
// form the width lays them in: wide, BY MODEL and ON ACCOUNTS lie beside
// WORTH BY, within its rows, so all three leave together.
func TestTheChosenProjectsSectionsLeaveInTheirOrder(t *testing.T) {
	headings := []string{"WORTH BY DAY", "BY MODEL", "ON ACCOUNTS"}
	page := chosenPage(chosenSessions())
	forms := []struct {
		width int
		kept  []int
	}{{173, []int{0, 3}}, {108, []int{0, 1, 2, 3}}, {73, []int{0, 1, 2, 3}}}
	for _, form := range forms {
		width := form.width
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			seen := map[int]bool{}
			before := projectSections
			for height := 70; height >= 24; height-- {
				f := projectsFrame(t, width, height, projectRows(3), 0, &page)
				doc := framedDoc(nil, nil)
				l := f.projectsLaying(doc, routedAt)
				kept := l.project.kept
				rows := textOf(f.page(doc, routedAt).rows(Look{}))
				for i, h := range headings {
					if shown := rowWith(rows, h) >= 0; shown != (i < kept) {
						t.Errorf("at %d rows %s shows %v, want the first %d sections shown:\n%s", height, h, shown, kept, strings.Join(rows, "\n"))
					}
				}
				if kept > before {
					t.Errorf("at %d rows %d sections show, more than the %d a row taller", height, kept, before)
				}
				if lines := l.sessions.rows - 2; kept > 0 && lines < 3 {
					t.Errorf("at %d rows SESSIONS shows %d lines beside %d sections, want 3 at the fewest", height, lines, kept)
				}
				seen[kept], before = true, kept
			}
			for kept := range projectSections + 1 {
				if want := slices.Contains(form.kept, kept); seen[kept] != want {
					t.Errorf("a height showing %d sections is %v, want %v", kept, seen[kept], want)
				}
			}
		})
	}
}

// The list shows its projects in a third of the view's rows, the line
// saying how many more among them, and scrolls only as far as it must to
// keep the chosen in view.
func TestTheProjectsListTakesAThirdOfTheRowsAndScrollsToTheChosen(t *testing.T) {
	rows := projectRows(30)
	page := chosenPage(chosenSessions())
	tests := []struct {
		name             string
		chosen, scrolled int
		first            int
	}{
		{name: "the top chosen", chosen: 0, scrolled: 0, first: 0},
		{name: "one past the rows chosen", chosen: 20, scrolled: 0},
		{name: "one in view chosen", chosen: 12, scrolled: 10, first: 10},
		{name: "one above the view chosen", chosen: 3, scrolled: 10, first: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := projectsFrame(t, 173, 53, rows, tt.chosen, &page)
			f.Projects.ListScroll = tt.scrolled
			l := f.projectsLaying(framedDoc(nil, nil), routedAt).list
			if got, want := l.linesShown(), f.viewRows()/3; got != want {
				t.Errorf("the list shows %d lines, want a third of %d rows, %d", got, f.viewRows(), want)
			}
			if tt.chosen < l.first || tt.chosen >= l.first+l.shown {
				t.Errorf("rows %d to %d show, want %d, chosen, among them", l.first, l.first+l.shown-1, tt.chosen)
			}
			if want := max(tt.first, tt.chosen-l.shown+1); tt.chosen >= tt.scrolled && l.first != want {
				t.Errorf("the list scrolls to %d, want %d", l.first, want)
			} else if tt.chosen < tt.scrolled && l.first != tt.first {
				t.Errorf("the list scrolls to %d, want %d", l.first, tt.first)
			}
			if got := f.ProjectsListScroll(framedDoc(nil, nil), routedAt); got != l.first {
				t.Errorf("ProjectsListScroll is %d, want %d, as the list is laid out", got, l.first)
			}
		})
	}
}

// SESSIONS groups the sessions ended under a rule for the day they ended:
// today, yesterday, a weekday within the week, else the day and its date;
// the sessions running over the first, under none.
func TestSessionsRulesNameTheDayTheirSessionsEnded(t *testing.T) {
	page := chosenPage(chosenSessions())
	f := projectsFrame(t, 173, 60, projectRows(3), 0, &page)
	rows := textOf(f.page(framedDoc(nil, nil), routedAt).rows(Look{}))
	heads := rowWith(rows, "SESSIONS") + 1
	var said []string
	for _, row := range rows[heads+1:] {
		if strings.HasPrefix(row, "  ended ") {
			said = append(said, strings.TrimSpace(strings.TrimRight(row, "─ ")))
		}
	}
	want := []string{"ended today", "ended yesterday", "ended Tue", "ended Mon 21 Sep"}
	if !slices.Equal(said, want) {
		t.Errorf("SESSIONS' rules read %q, want %q:\n%s", said, want, strings.Join(rows[heads:], "\n"))
	}
	if !strings.Contains(rows[heads+1], "d28c") {
		t.Errorf("SESSIONS' first line reads %q, want d28c, running, under no rule", rows[heads+1])
	}
}

// Where SESSIONS has room for a line alone under its heads, no day's rule
// shows, as a rule shows with the first session under it or not at all:
// scrolled to each line, it shows a session.
func TestSessionsOfALineShowsNoRuleAlone(t *testing.T) {
	page := chosenPage(chosenSessions())
	f := projectsFrame(t, 160, 20, projectRows(6), 0, &page)
	doc := framedDoc(nil, nil)
	if lines := f.projectsLaying(doc, routedAt).sessions.paged().page(); lines != 1 {
		t.Fatalf("SESSIONS shows %d lines at 160×20, want 1, for the test to hold", lines)
	}
	for scroll := range len(page.Sessions) {
		f.Scroll = scroll
		rows := textOf(f.page(doc, routedAt).rows(Look{}))
		line := rows[rowWith(rows, "SESSIONS")+2]
		if strings.Contains(line, "ended ") && !strings.Contains(line, page.Sessions[scroll].Session[:4]) {
			t.Errorf("scrolled %d, SESSIONS' line reads %q, want the session there, no rule alone", scroll, line)
		}
	}
}

// Where no line of SESSIONS fits under its heading and heads, SESSIONS
// shows nothing, no heading without what's under it.
func TestSessionsWithoutRoomForALineShowsNothing(t *testing.T) {
	page := chosenPage(chosenSessions())
	f := projectsFrame(t, 80, 18, projectRows(6), 0, &page)
	doc := framedDoc(nil, nil)
	if lines := f.projectsLaying(doc, routedAt).sessions.paged().page(); lines != 0 {
		t.Fatalf("SESSIONS shows %d lines at 80×18, want none, for the test to hold", lines)
	}
	if rows := textOf(f.page(doc, routedAt).rows(Look{})); rowWith(rows, "SESSIONS") >= 0 {
		t.Errorf("the frame reads\n%s\nwant no SESSIONS, no line of it fitting", strings.Join(rows, "\n"))
	}
}

// The ended sessions measured as the watch builds the page draw the frame
// as measuring them at each frame draws it; a measure of another page is
// passed over; and the sessions running are measured as they say now.
func TestSessionsMeasuredDrawAsMeasuredAtEachFrame(t *testing.T) {
	sessions := chosenSessions()
	sessions[0] = projectSessionOf("d28c5e17", idleFor(5*time.Second), yesterdayClock(15, 8), "")
	page := chosenPage(sessions)
	other := chosenPage(chosenSessions()[1:2])
	doc := framedDoc(nil, nil)
	later := routedAt.Add(10*time.Hour + 5*time.Minute)
	f := projectsFrame(t, 173, 60, projectRows(3), 0, &page)
	want := textOf(f.page(doc, later).rows(Look{}))
	for name, measured := range map[string]SessionsMeasured{
		"measured as built":        MeasureSessions(&page, f.Moment(doc, routedAt), routedAt),
		"measured of another page": MeasureSessions(&other, f.Moment(doc, routedAt), routedAt),
	} {
		f.Projects.Measured = measured
		if got := textOf(f.page(doc, later).rows(Look{})); !slices.Equal(got, want) {
			t.Errorf("%s, the frame reads\n%s\nwant\n%s", name, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
	if y := rowWith(want, "d28c"); y < 0 || !strings.Contains(want[y], " idle 10h 05m ") {
		t.Errorf("d28c's row reads %q, want its state as it says it now, idle 10h 05m, uncut, wider than any ended session's", want[max(y, 0)])
	}
}

// A session's started gives its day where it began on another than the one
// it ended on, or, of one running, today; and an ended session's state says
// when it ended.
func TestSessionsStartedGivesItsDayWhereItBeganOnAnother(t *testing.T) {
	page := chosenPage(chosenSessions())
	f := projectsFrame(t, 173, 60, projectRows(3), 0, &page)
	rows := textOf(f.page(framedDoc(nil, nil), routedAt).rows(Look{}))
	tests := []struct{ id, state, started string }{
		{id: "d28c", state: "answering", started: "Wed 15:08"},
		{id: "5b19", state: "ended 11:21", started: "10:02"},
		{id: "3d7a", state: "ended 16:40", started: "Tue 09:00"},
		{id: "e0c4", state: "ended 10:02", started: "09:12"},
	}
	for _, tt := range tests {
		y := rowWith(rows, tt.id)
		if y < 0 {
			t.Fatalf("no row of %s drawn:\n%s", tt.id, strings.Join(rows, "\n"))
		}
		if !strings.Contains(rows[y], tt.state+" ") || !strings.Contains(rows[y], " "+tt.started+" ") {
			t.Errorf("%s's row reads %q, want state %q and started %q", tt.id, rows[y], tt.state, tt.started)
		}
	}
}

// A bar of eighths is as long against the largest as its worth is, at least
// an eighth of any worth; its full cells split among its parts, and a
// part-filled last cell its eighth block in the last part's hue.
func TestABarOfEighthsEndsInItsLastPartsBlock(t *testing.T) {
	for _, tt := range []struct {
		w, largest ledger.Picodollars
		cells      int
		want       int
	}{{0, 10, 8, 0}, {1, 1000, 8, 1}, {500, 1000, 8, 32}, {1000, 1000, 10, 80}, {3, 16, 2, 3}} {
		if got := barEighths(tt.w, tt.largest, tt.cells); got != tt.want {
			t.Errorf("barEighths(%d, %d, %d) = %d, want %d", tt.w, tt.largest, tt.cells, got, tt.want)
		}
	}
	c := newCanvas(6, 1)
	first, second := hue{token: theme.AccentPrimary}, hue{token: theme.StatePositive}
	eighthsBar(c, Look{}, 1, 0, 20, []ledger.Picodollars{3, 1}, []hue{first, second})
	if got := rowOf(c, 0); got != " ██▌" {
		t.Errorf("the bar reads %q, want two full cells and a half", got)
	}
	for x, want := range map[int]theme.Token{1: theme.AccentPrimary, 2: theme.AccentPrimary, 3: theme.StatePositive} {
		if got := c.at(x, 0).ink.token; got != want {
			t.Errorf("cell %d is in %v, want %v", x, got, want)
		}
	}
}

// The chosen row is lit where ↑ and ↓ act, the list's or SESSIONS', and
// the other's, marked ▸, is bold on no surface.
func TestTheChosenRowIsLitOnlyWhereUpAndDownAct(t *testing.T) {
	page := chosenPage(chosenSessions())
	for _, inSessions := range []bool{false, true} {
		t.Run(fmt.Sprintf("in sessions %v", inSessions), func(t *testing.T) {
			f := projectsFrame(t, 173, 60, projectRows(3), 0, &page)
			f.Projects.InSessions = inSessions
			c := f.page(framedDoc(nil, nil), routedAt)
			rows := textOf(c.rows(Look{}))
			project := rowWith(rows, "▸ agentic-workflows")
			session := rowWith(rows, "d28c")
			if project < 0 || session < 0 {
				t.Fatalf("the chosen project's row at %d, the first session's at %d, want both drawn:\n%s", project, session, strings.Join(rows, "\n"))
			}
			lit := func(y int) bool { return c.at(2, y).ink.on.token == theme.BgSelection }
			if lit(project) == inSessions || lit(session) != inSessions {
				t.Errorf("the project's row lit %v and the session's %v, want only SESSIONS' lit %v", lit(project), lit(session), inSessions)
			}
			if inSessions && !c.at(2, project).ink.bold {
				t.Errorf("the chosen project's row isn't bold, want it, SESSIONS lit")
			}
		})
	}
}
