package capture

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// projectsOf is the world's Projects page over the period given, at the
// frames' moment, at the frames' prices, weeks from Monday: of each day's
// sessions as its records' ledger gives them, today's of today's lines, and
// the List of the sessions its router lists.
func projectsOf(w accountWorld, period views.Period) views.ProjectsPage {
	now := worldsNow().now
	l := w.ledger()
	today, _, _ := l.Today(ledger.Mark{})
	days := views.ReadSessionDays(l, now.AddDate(-1, 0, 0), now).Tailed(today, true, nil, nil, now)
	list := views.ListSessions(views.SessionSources{Running: w.routed(now), Ledger: l, Prices: drawnPrices, Now: now})
	return days.Projects(views.PeriodInput{
		Period: period, List: list, Names: status.Names{}, Prices: drawnPrices, WeekStarts: time.Monday, Family: claude.Provider{}.Family, Now: now,
	})
}

// projectFigures are a project's row's figures, its worth in cents.
type projectFigures struct {
	name                     string
	live, sessions, requests int
	cents                    int64
}

// figuresOf are the figures of the row given, its worth to the cent.
func figuresOf(t *testing.T, name string, totals views.ProjectTotals) projectFigures {
	t.Helper()
	if totals.Amount == nil || len(totals.Unpriced) > 0 {
		t.Fatalf("%s's worth %v isn't priced whole", name, totals.Worth)
	}
	return projectFigures{name, totals.Live, totals.Sessions, totals.Requests, int64(*totals.Amount / 10_000_000_000)}
}

// This week the auto world's projects are the design's world's: six, each
// running a session now, of the design's sessions, summing to Accounts'
// every account this week, 27 sessions, 5,410 requests and $635; docs and
// mobile, all of whose sessions ran yesterday or today, as their lines tell
// them, what the design gives them besides going to the other four.
func TestTheProjectsOfThisWeekAreTheDesignsWorld(t *testing.T) {
	page := projectsOf(autoWorld(), views.PeriodThisWeek)
	var got []projectFigures
	for _, r := range page.Rows {
		got = append(got, figuresOf(t, r.Name, r.ProjectTotals))
	}
	want := []projectFigures{
		{"switchboard", 1, 8, 2059, 23864}, {"website", 1, 5, 1281, 15885}, {"infra", 1, 5, 992, 11241},
		{"api", 1, 4, 823, 9992}, {"docs", 1, 3, 213, 2181}, {"mobile", 1, 2, 42, 333},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("this week's projects are\n%+v\nwant\n%+v", got, want)
	}
	if all := figuresOf(t, "every project", page.All); all != (projectFigures{"every project", 6, 27, 5410, 63500}) {
		t.Errorf("every project's week is %+v, want 6 live, 27 sessions, 5,410 requests and $635.00", all)
	}
}

// Switchboard's eight sessions this week are named by their own
// directories: three in its worktrees, fix-login, and pools twice, and one
// in release, whose lines are from before projects.
func TestSwitchboardsSessionsKeepTheirWorktreesNames(t *testing.T) {
	page, ok := projectsOf(autoWorld(), views.PeriodThisWeek).Project(switchboardProject)
	if !ok {
		t.Fatal("this week has no switchboard")
	}
	var names []string
	for _, s := range page.Sessions {
		names = append(names, status.DirName(s.Dir, 20))
	}
	slices.Sort(names)
	if want := []string{"fix-login", "pools", "pools", "release", "switchboard", "switchboard", "switchboard", "switchboard"}; !reflect.DeepEqual(names, want) {
		t.Errorf("switchboard's sessions are named %v, want %v", names, want)
	}
}

// Over 12 months the auto world's projects are the design's eight, each with
// as many sessions as it draws, summing to History's Year, 212,885 requests
// and $24,736; kit last active on Saturday 13 June, the design's Sunday 14
// June being a day of no requests, and old-site on 2 March.
func TestTheProjectsOfTwelveMonthsAreTheDesignsWorld(t *testing.T) {
	page := projectsOf(autoWorld(), views.PeriodTwelveMonths)
	sessions, last := make(map[string]int), make(map[string]string)
	for _, r := range page.Rows {
		sessions[r.Name], last[r.Name] = r.Sessions, r.Last.Local().Format(time.DateOnly)
	}
	want := map[string]int{"switchboard": 506, "website": 304, "infra": 253, "api": 219, "kit": 152, "docs": 135, "mobile": 68, "old-site": 51}
	if !reflect.DeepEqual(sessions, want) {
		t.Errorf("over 12 months the projects' sessions are %v, want %v", sessions, want)
	}
	if last["kit"] != "2026-06-13" || last["old-site"] != "2026-03-02" {
		t.Errorf("kit was last active on %s and old-site on %s, want 2026-06-13 and 2026-03-02", last["kit"], last["old-site"])
	}
	all := figuresOf(t, "every project", page.All)
	if all.live != 6 || all.sessions != 1688 || all.requests != 212885 || (all.cents+50)/100 != 24736 {
		t.Errorf("every project's 12 months are %+v, want 6 live, 1,688 sessions, 212,885 requests and $24,736", all)
	}
}
