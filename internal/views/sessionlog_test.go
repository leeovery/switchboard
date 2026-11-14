package views_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The log lists every session of the period, whatever its project: those
// running first, then those ended, the latest ended first, each rowed as
// its project's SESSIONS rows it, and every session's totals as Projects'
// every project's.
func TestTheLogRowsEverySessionOfThePeriodAsItsProjectRowsIt(t *testing.T) {
	in := projectsIn(views.PeriodThisWeek, now, listedRunning(repoSession, "/code/switchboard", "/code/switchboard"))
	days := heldDays(now, projectsWorld()...)
	log, page := days.SessionLog(in), days.Projects(in)
	var ids []string
	for _, s := range log.Sessions {
		ids = append(ids, s.Session)
	}
	if want := []string{repoSession, ownSession, clientSession, nowhereSession, worktreeSession}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("the log lists\n%v\nwant\n%v", ids, want)
	}
	rowed := make(map[string]views.PeriodSession)
	for _, r := range page.Rows {
		project, _ := page.Project(r.Path)
		for _, s := range project.Sessions {
			rowed[s.Session] = s
		}
	}
	for _, s := range log.Sessions {
		if !reflect.DeepEqual(s, rowed[s.Session]) {
			t.Errorf("the log rows %s as\n%+v\nwant it as its project rows it,\n%+v", s.Session, s, rowed[s.Session])
		}
	}
	if log.Period != views.PeriodThisWeek || !reflect.DeepEqual(log.All, page.All) {
		t.Errorf("the log is of %q, totalling %+v; want this week, totalling every project's, %+v", log.Period, log.All, page.All)
	}
}

// The sessions of the log's tests beside the projects': one asked of across
// midnight, one with requests yesterday and a token count alone today, and
// one with a token count alone today.
const (
	overnightSession  = "0e000000-0000-4000-8000-000000000001"
	yesterdaysSession = "0c000000-0000-4000-8000-000000000002"
	countOnlySession  = "0d000000-0000-4000-8000-000000000003"
)

// counted is a token count of the session given, in dir, at at: a line of
// no request.
func counted(session, dir string, at time.Time) ledger.Line {
	return ledger.Line{At: at.UTC(), Request: session + "-count-" + at.Format(time.TimeOnly), Session: session, Kind: ledger.KindCount, Dir: dir, Account: "work", Model: opus, Status: 200}
}

// routedOn is the session given as the router lists it, its Claude Opus 5.5
// on work, in dir.
func routedOn(id, dir string) status.Session {
	return status.Session{ID: id, Assignments: []status.Assignment{{Model: opus, Account: "work", Dir: dir, Project: dir, Reason: "sticky", AssignedAt: now, LastSeen: now}}}
}

// todaysListAndLog are today's List of the lines given at now, as the verbs
// build it, of the sessions running given, as the router lists them; and
// today's log of the same lines, their days read as the watch reads them,
// over that List.
func todaysListAndLog(lines []ledger.Line, running ...status.Session) (views.SessionList, views.SessionLog) {
	l := sessionsLedger(lines, now)
	list := views.ListSessions(views.SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: now})
	days := views.ReadSessionDays(l, now.AddDate(0, 0, -40), now).Tailed(l.Day(dayfile.DateOf(now)), true, nil, nil, now)
	return list, days.SessionLog(projectsIn(views.PeriodToday, now, list.Sessions...))
}

// linesOf are the lines the requests given hold.
func linesOf(held ...ledger.Held) []ledger.Line {
	lines := make([]ledger.Line, len(held))
	for i, h := range held {
		lines[i] = h.Line
	}
	return lines
}

// Today's log is today's List, row for row, built of the same lines: the
// session running, then the one that ended today; none ended on a day gone;
// none with no request today that the router doesn't list, as one whose
// line today, after midnight, is a token count, its requests yesterday's,
// which never shows as ended yesterday, nor one of a token count alone.
func TestTodaysLogIsTodaysList(t *testing.T) {
	lines := slices.Concat(
		linesOf(projectAsks(repoSession, "/code/switchboard", "/code/switchboard", "work", october(7, 10, 0), october(7, 11, 0))...),
		linesOf(projectAsks(ownSession, "/code/own", "/code/own", "work", october(7, 9, 0), october(7, 9, 30))...),
		linesOf(projectAsks(clientSession, "/code/client", "/code/client", "work", october(6, 15, 0))...),
		linesOf(projectAsks(yesterdaysSession, "/code/client", "/code/client", "work", october(6, 22, 0))...),
		[]ledger.Line{counted(yesterdaysSession, "/code/client", october(7, 0, 30)), counted(countOnlySession, "/code/client", october(7, 8, 0))},
	)
	slices.SortFunc(lines, func(a, b ledger.Line) int { return a.At.Compare(b.At) })
	list, log := todaysListAndLog(lines, routedOn(repoSession, "/code/switchboard"))
	listed := list.Named(status.Names{}).Sessions
	var rows []views.ListedSession
	for _, s := range log.Sessions {
		rows = append(rows, s.ListedSession)
	}
	if len(listed) != 2 || !reflect.DeepEqual(rows, listed) {
		t.Fatalf("today's log rows\n%+v\nwant today's List's two, row for row,\n%+v", rows, listed)
	}
	if got := []string{log.Sessions[0].EndedOn, log.Sessions[1].EndedOn}; !reflect.DeepEqual(got, []string{"", "2026-10-07"}) {
		t.Errorf("the rows end on %q, want the running one on none, the other today", got)
	}
	if log.All.Sessions != list.Today.Sessions || log.All.Requests != list.Today.Requests {
		t.Errorf("today's log totals %d sessions and %d requests, want the List's %d and %d", log.All.Sessions, log.All.Requests, list.Today.Sessions, list.Today.Requests)
	}
}

// A session running across midnight starts at its first request, yesterday,
// on the log and on its project's SESSIONS alike, the List listing it from
// today's lines, not at today's first.
func TestASessionAcrossMidnightStartsTheDayBefore(t *testing.T) {
	lines := linesOf(projectAsks(overnightSession, "/code/switchboard", "/code/switchboard", "work", october(6, 23, 30), october(7, 0, 30))...)
	list, log := todaysListAndLog(lines, routedOn(overnightSession, "/code/switchboard"))
	if len(log.Sessions) != 1 || !log.Sessions[0].Started.Equal(october(6, 23, 30)) {
		t.Fatalf("today's log is %+v, want the session started yesterday at 23:30", log.Sessions)
	}
	l := sessionsLedger(lines, now)
	days := views.ReadSessionDays(l, now, now).Tailed(l.Day(dayfile.DateOf(now)), true, nil, nil, now)
	page := days.Projects(projectsIn(views.PeriodToday, now, list.Sessions...))
	project, _ := page.Project("/code/switchboard")
	if len(project.Sessions) != 1 || !project.Sessions[0].Started.Equal(october(6, 23, 30)) {
		t.Errorf("its project's SESSIONS is %+v, want it started yesterday at 23:30", project.Sessions)
	}
}

// A session the List lists as neither running nor ended, as one the stream
// told of before the router lists it, is ruled by the day of its last
// request, yesterday's, among those of its day in the List's order, never
// apart from them.
func TestASessionNeitherRunningNorEndedIsRuledByItsLastRequestsDay(t *testing.T) {
	asking := views.ListedSession{Session: yesterdaysSession, Dir: "/code/client", Activity: views.Activity{Doing: views.Asking}, Ended: october(6, 20, 0), Requests: 1}
	days := heldDays(now, slices.Concat(
		projectAsks(ownSession, "/code/own", "/code/own", "work", october(7, 10, 0)),
		projectAsks(yesterdaysSession, "/code/client", "/code/client", "work", october(6, 20, 0)),
		projectAsks(worktreeSession, "/code/switchboard", "/code/switchboard", "work", october(7, 8, 0)),
		projectAsks(clientSession, "/code/client", "/code/client", "work", october(6, 15, 0)),
	)...)
	log := days.SessionLog(projectsIn(views.PeriodThisWeek, now, asking))
	var got []string
	for _, s := range log.Sessions {
		got = append(got, s.Session+" "+s.EndedOn)
	}
	want := []string{ownSession + " 2026-10-07", worktreeSession + " 2026-10-07", yesterdaysSession + " 2026-10-06", clientSession + " 2026-10-06"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the log lists\n%v\nwant\n%v", got, want)
	}
}
