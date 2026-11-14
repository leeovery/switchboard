package views_test

import (
	"reflect"
	"testing"

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

// Today's log is today's List: the sessions running, as the List has them,
// then those that ended today, none of a day gone.
func TestTodaysLogIsTodaysList(t *testing.T) {
	listed := listedRunning(repoSession, "/code/switchboard", "/code/switchboard")
	log := heldDays(now, projectsWorld()...).SessionLog(projectsIn(views.PeriodToday, now, listed))
	var got []string
	for _, s := range log.Sessions {
		got = append(got, s.Session+" "+s.EndedOn)
	}
	if want := []string{repoSession + " ", ownSession + " 2026-10-07"}; !reflect.DeepEqual(got, want) {
		t.Errorf("today's log lists\n%v\nwant\n%v", got, want)
	}
	if want := (views.ProjectTotals{Live: 1, Sessions: 2, Requests: 4, Worth: hoursWorth(4)}); !reflect.DeepEqual(log.All, want) {
		t.Errorf("today's log totals %+v, want %+v", log.All, want)
	}
}
