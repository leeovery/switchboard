package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// projectRowOf is a project's row of the list, named as given, its last
// request at 14:33 on routedAt's day, all its worth opus's.
func projectRowOf(name string, sessions, requests, cents int) views.ProjectRow {
	return views.ProjectRow{
		Path: "/work/Code/" + name, Name: name, Sessions: sessions, Requests: requests, Worth: worthOf(cents),
		ByFamily: []views.FamilyWorth{{Family: "opus", Worth: worthOf(cents)}},
		Last:     routedClock(1, 14, 33),
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
func projectSessionOf(id string, doing views.Activity, started time.Time, endedOn string) views.PeriodSession {
	s := listedSession{id: id, dir: "~/Code/agentic-workflows", on: "work", doing: doing, started: started, requests: 40, worth: 1200}.listed()
	if endedOn != "" {
		s.Ended = doing.Since
	}
	return views.PeriodSession{ListedSession: s, EndedOn: endedOn}
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
func chosenSessions() []views.PeriodSession {
	return []views.PeriodSession{
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
func chosenPage(sessions []views.PeriodSession) views.ProjectPage {
	half, third := 0.5, 1.0/3
	page := views.ProjectPage{ProjectRow: projectRowOf("agentic-workflows", 12, 2059, 23900), Share: &half, Sessions: sessions}
	page.Tokens.Total = 365_000_000
	byDay := views.WorthBy{Unit: views.ByDay, Peak: 3, ToCome: 3}
	for i, day := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		byDay.Buckets = append(byDay.Buckets, views.WorthBucket{Start: routedClock(1, 0, 0).AddDate(0, 0, i-3), Label: day, Axis: day, Worth: worthOf((i + 1) * 1000 % 5000)})
	}
	page.WorthBy = views.WorthCharts{byDay}
	for _, v := range []string{"Opus 5.5", "Sonnet 5"} {
		page.ByModel = append(page.ByModel, views.VersionShare{Version: ledger.Version{Name: v, Family: "opus", Shade: 1}, Share: &half})
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

// With no session in the period, the list says so as the List does, the
// period in its words.
func TestTheProjectsListOfNoSessionsSaysSoAsTheListDoes(t *testing.T) {
	for _, tt := range []struct {
		period views.Period
		want   string
	}{
		{views.PeriodToday, "no sessions today"},
		{views.PeriodThisWeek, "no sessions this week"},
		{views.PeriodThirtyDays, "no sessions in the last 30 days"},
	} {
		f := frameOf(108, 30)
		f.View, f.Period, f.Projects = Projects, tt.period, ProjectsShown{Page: &views.ProjectsPage{Period: tt.period}}
		if got := strings.Join(textOf(f.Draw(threeRouted(), routedAt)), "\n"); !strings.Contains(got, tt.want) {
			t.Errorf("Projects over %s drew\n%s\nwant %q", tt.period.Name(), got, tt.want)
		}
	}
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

// As rows run short, Projects gives way in its order: WORTH BY's columns
// squeeze from worthRows to worthRowsLeast; then the list gives rows back
// from its third to listBeforeLeaving; only then do the sections leave, ON
// ACCOUNTS first, then BY MODEL, then WORTH BY; and past that the list is as
// it is with nothing between it and SESSIONS, up to its third, giving way to
// two. SESSIONS keeps three of its lines while anything else can give way. Wide, BY MODEL and ON ACCOUNTS lie beside WORTH BY, which keeps the
// rows squeezing wouldn't free, and BY MODEL never leaves without ON
// ACCOUNTS, as that frees none.
func TestProjectsGivesWayInItsOrder(t *testing.T) {
	page := chosenPage(chosenSessions())
	for _, n := range []int{35, 6, 3} {
		for _, width := range []int{173, 114, 108, 73} {
			t.Run(fmt.Sprintf("%d projects at %d", n, width), func(t *testing.T) {
				doc := framedDoc(nil, nil)
				before := projectsGiving{list: n, kept: projectSections, chart: worthRows}
				seen := map[string]bool{}
				for height := 70; height >= 22; height-- {
					f := projectsFrame(t, width, height, projectRows(n), 0, &page)
					l := f.projectsLaying(doc, routedAt)
					g := projectsGiving{list: l.list.linesShown(), kept: l.project.kept, chart: l.project.chart}
					third := l.listThird()
					least := min(third, listBeforeLeaving)
					squeezes := l.project.squeezed(worthRowsLeast).height(g.kept) < l.project.height(g.kept)
					at := fmt.Sprintf("at %d rows, the list showing %d of its third's %d, WORTH BY's columns %d rows and %d sections kept,", height, g.list, third, g.chart, g.kept)
					lastLeft := g.kept == 0 && before.kept > 0
					if g.list > before.list && !lastLeft || g.kept > before.kept || g.chart > before.chart {
						t.Errorf("%s more than a row taller's %+v", at, before)
					}
					if g.list < third && squeezes {
						t.Errorf("%s the list gave rows back while WORTH BY could squeeze", at)
					}
					if g.kept < projectSections && (squeezes || g.kept > 0 && g.list > least) {
						t.Errorf("%s a section left while WORTH BY could squeeze or the list give rows back to %d", at, least)
					}
					if lines := l.sessions.rows - 2; g.kept == 0 && g.list < third && lines > 3 {
						t.Errorf("%s every section gone, the list is short of its third with SESSIONS showing %d lines", at, lines)
					}
					if g.list < least && g.kept > 0 {
						t.Errorf("%s the list shows fewer than %d with a section kept", at, least)
					}
					if lines := l.sessions.rows - 2; lines < 3 && (g.kept > 0 || g.list > min(n, 2)) {
						t.Errorf("%s SESSIONS shows %d lines, want 3 at the fewest", at, lines)
					}
					checkProjectsDrawnAsLaid(t, at, textOf(f.page(doc, routedAt).rows(Look{})), l)
					seen[givingStage(g, third, least)], before = true, g
				}
				want := []string{"whole", "squeezed", "kept 2", "kept 1", "kept 0", "list under"}
				if n > listBeforeLeaving+1 {
					want = append(want, "list shrunk", "kept 3")
				}
				if width == 173 {
					want = slices.DeleteFunc(want, func(stage string) bool { return stage == "kept 1" })
				}
				for _, stage := range want {
					if !seen[stage] {
						t.Errorf("no height shows the stage %q", stage)
					}
				}
			})
		}
	}
}

// givingStage is the stage of giving way g is at, of the list's third and
// the least it gives rows back to: whole; squeezed, WORTH BY's columns
// squeezed, the list at its third; list shrunk, the list under its third;
// kept and the sections kept, the list at its least; and list under, past
// its least.
func givingStage(g projectsGiving, third, least int) string {
	switch {
	case g.list < least:
		return "list under"
	case g.kept < projectSections || g.list == least && g.list < third:
		return fmt.Sprintf("kept %d", g.kept)
	case g.list < third:
		return "list shrunk"
	case g.chart < worthRows:
		return "squeezed"
	}
	return "whole"
}

// checkProjectsDrawnAsLaid checks that rows draw Projects as l lays it out:
// each section shown that's kept, WORTH BY's peak under its columns' rows,
// and SESSIONS' heading on the row laid for it.
func checkProjectsDrawnAsLaid(t *testing.T, at string, rows []string, l projectsLaid) {
	t.Helper()
	for i, h := range []string{"WORTH BY DAY", "BY MODEL", "ON ACCOUNTS"} {
		if shown := rowWith(rows, h) >= 0; shown != (i < l.project.kept) {
			t.Errorf("%s %s shows %v:\n%s", at, h, shown, strings.Join(rows, "\n"))
		}
	}
	if head := rowWith(rows, "WORTH BY"); head >= 0 && !strings.HasPrefix(rows[head+l.project.chart+2], "peak ") {
		t.Errorf("%s WORTH BY's peak isn't %d rows under its heading:\n%s", at, l.project.chart+2, strings.Join(rows, "\n"))
	}
	if y := rowWith(rows, "SESSIONS"); y != l.sessions.top {
		t.Errorf("%s SESSIONS' heading is on row %d, laid on %d:\n%s", at, y, l.sessions.top, strings.Join(rows, "\n"))
	}
}

// On the owner's iPad, 35 projects at 114 by 48, Projects shows every
// section, WORTH BY whole and the list its third.
func TestProjectsOnAnIPadShowsEverySection(t *testing.T) {
	page := chosenPage(chosenSessions())
	f := projectsFrame(t, 114, 48, projectRows(35), 0, &page)
	l := f.projectsLaying(framedDoc(nil, nil), routedAt)
	if l.project.kept != projectSections || l.project.chart != worthRows || l.list.linesShown() != f.viewRows()/3 {
		t.Errorf("at 114x48 %d sections show, WORTH BY's columns %d rows and the list %d lines; want %d, %d and a third of %d",
			l.project.kept, l.project.chart, l.list.linesShown(), projectSections, worthRows, f.viewRows())
	}
}

// bucketedPage is chosenPage with WORTH BY in n buckets of the unit given,
// and where weeks are given, by week in as many beside them, as of the last
// 12 weeks.
func bucketedPage(unit views.BucketUnit, n, weeks int) views.ProjectPage {
	buckets := func(unit views.BucketUnit, n int) views.WorthBy {
		by := views.WorthBy{Unit: unit, Peak: 0}
		for i := range n {
			by.Buckets = append(by.Buckets, views.WorthBucket{Start: routedClock(1, 0, 0).AddDate(0, 0, i-n), Worth: worthOf(1000 + i)})
		}
		return by
	}
	page := chosenPage(chosenSessions())
	page.WorthBy = views.WorthCharts{buckets(unit, n)}
	if weeks > 0 {
		page.WorthBy = append(page.WorthBy, buckets(views.ByWeek, weeks))
	}
	return page
}

// The chosen project lies wide, on a MacBook, at every period, and
// middling on an iPad; 12 weeks
// are by day where a cell for each of their days fits the cells WORTH BY
// takes as the sections lie, the left half's wide, else by week.
func TestTheChosenProjectLiesWideAtEveryPeriodOnAMacBook(t *testing.T) {
	doc := framedDoc(nil, nil)
	drawn := func(width int, page views.ProjectPage) projectDrawn {
		f := projectsFrame(t, width, 53, projectRows(3), 0, &page)
		return projectDrawnOf(f, f.Moment(doc, routedAt), routedAt, page)
	}
	periods := []struct {
		name           string
		unit           views.BucketUnit
		buckets, weeks int
	}{
		{"today", views.ByHour, 24, 0}, {"this week", views.ByDay, 7, 0}, {"30 days", views.ByDay, 30, 0},
		{"12 weeks", views.ByDay, 84, 12}, {"12 months", views.ByWeek, 53, 0},
	}
	for _, p := range periods {
		d := drawn(173, bucketedPage(p.unit, p.buckets, p.weeks))
		if d.form != projectWide {
			t.Errorf("%s at 173 lies as form %d, want wide", p.name, d.form)
		}
		if d := drawn(108, bucketedPage(p.unit, p.buckets, p.weeks)); d.form != projectMiddling {
			t.Errorf("%s at 108 lies as form %d, want middling", p.name, d.form)
		}
	}
	twelveWeeks := []struct {
		width int
		form  projectForm
		unit  views.BucketUnit
	}{
		{173, projectWide, views.ByDay}, {171, projectWide, views.ByWeek}, {108, projectMiddling, views.ByDay}, {73, projectStacked, views.ByWeek},
	}
	for _, tt := range twelveWeeks {
		if d := drawn(tt.width, bucketedPage(views.ByDay, 84, 12)); d.form != tt.form || d.worth.Unit != tt.unit {
			t.Errorf("12 weeks at %d lie as form %d by %s, want form %d by %s", tt.width, d.form, bucketWord[d.worth.Unit], tt.form, bucketWord[tt.unit])
		}
	}
}

// Over 12 months WORTH BY is by week where a cell for each week fits the
// cells it takes, else by month, as on a narrow screen, its heading saying
// which.
func TestTwelveMonthsAreByMonthWhereTheirWeeksDontFit(t *testing.T) {
	doc := framedDoc(nil, nil)
	page := bucketedPage(views.ByWeek, 53, 0)
	page.WorthBy = append(page.WorthBy, bucketedPage(views.ByMonth, 13, 0).WorthBy...)
	for i, by := range page.WorthBy {
		page.WorthBy[i].Peak = len(by.Buckets) - 1
	}
	for _, tt := range []struct {
		width int
		unit  views.BucketUnit
		said  string
	}{{73, views.ByWeek, "WORTH BY WEEK"}, {52, views.ByMonth, "WORTH BY MONTH"}} {
		f := projectsFrame(t, tt.width, 53, projectRows(3), 0, &page)
		if d := projectDrawnOf(f, f.Moment(doc, routedAt), routedAt, page); d.worth.Unit != tt.unit {
			t.Errorf("12 months at %d are by %s, want by %s", tt.width, bucketWord[d.worth.Unit], bucketWord[tt.unit])
		}
		if rows := textOf(f.Draw(doc, routedAt)); rowWith(rows, tt.said) < 0 {
			t.Errorf("12 months at %d, no row reads %q, in\n%s", tt.width, tt.said, strings.Join(rows, "\n"))
		}
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

// SESSIONS' columns give way as it narrows: the session's narrows to its
// least first, so wherever a column comes back as it widens the session's is
// at its least; then model leaves, then started, then requests; worth never
// leaves, nor on.
func TestSessionsColumnsGiveWayInTheirOrder(t *testing.T) {
	page := chosenPage(chosenSessions())
	m := frameOf(173, 53).Moment(framedDoc(nil, nil), routedAt)
	before := 0
	for width := 30; width <= 220; width++ {
		columns := projectSessionsOf(sessionWords{m: m, now: routedAt}, &page, SessionsMeasured{}, width).columns
		shown := map[listPart]listColumn{}
		for _, c := range columns {
			shown[c.part] = c
		}
		_, model := shown[listModel]
		_, started := shown[listStarted]
		_, requests := shown[listRequests]
		_, worth := shown[listWorth]
		_, on := shown[listOn]
		switch {
		case !worth || !on:
			t.Errorf("at %d worth shows %v and on %v, want both always", width, worth, on)
		case model && !started, started && !requests:
			t.Errorf("at %d model shows %v, started %v and requests %v; want model to leave first, then started, then requests", width, model, started, requests)
		case len(columns) > before && before > 0 && shown[listSession].width != listSessionLeast:
			t.Errorf("at %d a column came back with the session's %d cells, want it at its least, %d, first", width, shown[listSession].width, listSessionLeast)
		}
		before = len(columns)
		if width == 220 && len(columns) != len(projectColumns) {
			t.Errorf("at 220 SESSIONS has %d columns, want all %d", len(columns), len(projectColumns))
		}
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
		{id: "d28c", state: "answering", started: "yesterday 15:08"},
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
		if got := barUnits(tt.w, tt.largest, tt.cells*8); got != tt.want {
			t.Errorf("barUnits(%d, %d, %d) = %d, want %d", tt.w, tt.largest, tt.cells*8, got, tt.want)
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

// Projects' help says how its periods run, as Accounts' does, each from
// where it truly starts, and that it shares the period with Accounts and
// Sessions' List.
func TestProjectsHelpSaysHowItsPeriodsRun(t *testing.T) {
	var said []string
	for _, p := range projectsHelp() {
		said = append(said, plainLines(p.says)...)
	}
	all := strings.Join(said, " ")
	for _, want := range []string{"today and this week run from their starts, 30 days and 12 months back from today, and 12 weeks over this week and the 11 before", "< and > step through today, this week, 30 days, 12 weeks and 12 months, the period Accounts and Sessions' List share."} {
		if !strings.Contains(all, want) {
			t.Errorf("Projects' help reads\n%s\nwant %q", all, want)
		}
	}
}

// Projects' help says the order the page gives way in as rows run short.
func TestProjectsHelpSaysTheOrderItGivesWayIn(t *testing.T) {
	var said []string
	for _, p := range projectsHelp() {
		said = append(said, plainLines(p.says)...)
	}
	all := strings.Join(said, " ")
	want := "Where rows run short, WORTH BY's columns squeeze from 6 rows to 3, then the list shrinks from its third to 5 rows; " +
		"only then does ON ACCOUNTS leave, then BY MODEL, then WORTH BY, and with them gone the list takes up to its third again, shrinking to 2, " +
		"so SESSIONS keeps three rows at least."
	if !strings.Contains(all, want) {
		t.Errorf("Projects' help reads\n%s\nwant %q", all, want)
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

// A project's row names it by its name, bold; the sessions with no
// directory's, and those of temporary folders, by their words, faint.
func TestARowOfNoPathIsNamedByItsWordsFaint(t *testing.T) {
	tests := []struct {
		name string
		row  views.ProjectRow
		want line
	}{
		{name: "a project", row: projectRowOf("api", 1, 1, 100), want: line{{"api", titleInk}}},
		{name: "no directory", row: views.ProjectRow{}, want: line{{noDirectory, dimInk}}},
		{name: "temporary folders", row: views.ProjectRow{Path: status.TemporaryFolders}, want: line{{temporaryFolders, dimInk}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := projectNamed(tt.row); !slices.Equal(got, tt.want) {
				t.Errorf("projectNamed = %v, want %v", got, tt.want)
			}
		})
	}
}

// The chosen project's head gives its path, but temporary folders', which
// has none, as no directory's has none.
func TestTheHeadOfTemporaryFoldersGivesNoPath(t *testing.T) {
	temporary := views.ProjectRow{Path: status.TemporaryFolders, Sessions: 4, Requests: 4, Worth: worthOf(400)}
	rows := append(projectRows(2), temporary)
	page := chosenPage(nil)
	page.ProjectRow = temporary
	c := projectsFrame(t, 173, 60, rows, 2, &page).page(framedDoc(nil, nil), routedAt)
	drawn := textOf(c.rows(Look{}))
	listed := rowWith(drawn, "▸ "+temporaryFolders)
	if listed < 0 {
		t.Fatalf("no row of temporary folders chosen in the list:\n%s", strings.Join(drawn, "\n"))
	}
	head := slices.IndexFunc(drawn[listed+1:], func(r string) bool { return strings.HasPrefix(strings.TrimSpace(r), temporaryFolders) })
	if head < 0 {
		t.Fatalf("no head of temporary folders under the list:\n%s", strings.Join(drawn, "\n"))
	}
	if got := drawn[listed+1+head]; strings.Count(got, temporaryFolders) != 1 {
		t.Errorf("the head reads %q, want temporary folders named once, with no path", got)
	}
}
