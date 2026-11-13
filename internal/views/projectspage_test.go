package views_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// linesLedger is a ledger holding the lines given, filed under the local day
// each arrived on, and no day's head: each day's sessions are read of its
// lines.
type linesLedger struct {
	ledger.Empty
	days map[string][]ledger.Held
}

func (l linesLedger) Day(date string) []ledger.Held {
	return l.days[date]
}

// ledgerOfLines is the ledger of the lines given.
func ledgerOfLines(lines ...ledger.Held) linesLedger {
	l := linesLedger{days: make(map[string][]ledger.Held)}
	for _, h := range lines {
		date := dayfile.DateOf(h.At)
		l.days[date] = append(l.days[date], h)
	}
	return l
}

// projectAsk is a session's request of its own conversation, in dir, of the
// project given, on the account given, asking for the model given, arrived
// at at, its answer's usage hourUsage's.
func projectAsk(session, dir, project, account, model string, at time.Time) ledger.Held {
	return ledger.Held{
		At: at.UTC(), Request: fmt.Sprintf("%s-%d", session, at.UnixNano()), Session: session, Kind: ledger.KindMessage, Dir: dir, Project: project,
		Account: account, Model: model, Status: 200, Class: ledger.ClassMain, Usage: json.RawMessage(hourUsage)}
}

// times are n times from at, a minute apart.
func times(at time.Time, n int) []time.Time {
	all := make([]time.Time, n)
	for i := range all {
		all[i] = at.Add(time.Duration(i) * time.Minute)
	}
	return all
}

// projectAsks are a session's requests as projectAsk has them, one at each time
// given.
func projectAsks(session, dir, project, account string, at ...time.Time) []ledger.Held {
	lines := make([]ledger.Held, len(at))
	for i, t := range at {
		lines[i] = projectAsk(session, dir, project, account, opus, t)
	}
	return lines
}

// heldDays are the sessions of the lines given, the days before now's read
// from a year back, as ReadSessionDays reads them, and today's as the tail
// gives them, every one of today's.
func heldDays(now time.Time, lines ...ledger.Held) views.SessionDays {
	l := ledgerOfLines(lines...)
	return views.ReadSessionDays(l, now.AddDate(-1, 0, 0), now).Tailed(l.days[dayfile.DateOf(now)], true, nil, now)
}

// projectsIn is what Projects' page is built of over the period given at
// now, the List's sessions given, weeks starting on Monday.
func projectsIn(period views.Period, now time.Time, listed ...views.ListedSession) views.ProjectsInput {
	return views.ProjectsInput{
		Period: period, List: views.SessionList{Sessions: listed}, Names: status.Names{}, Prices: ledger.Pricing,
		WeekStarts: time.Monday, Family: family, Now: now,
	}
}

// hoursWorth is n requests of Claude Opus 5.5 worth what hourUsage's are.
func hoursWorth(n int64) views.Worth {
	return priced(n * hourWorth)
}

// The sessions of the projects' tests: two of switchboard's, one in a
// worktree Claude Code made, from a line before projects, the other in its
// repository, running; one of each of two projects named api; and one in
// no directory.
const (
	worktreeSession = "5b0a0000-0000-4000-8000-000000000001"
	repoSession     = "5b0a0000-0000-4000-8000-000000000002"
	clientSession   = "c1a00000-0000-4000-8000-000000000003"
	ownSession      = "0a1a0000-0000-4000-8000-000000000004"
	nowhereSession  = "00d00000-0000-4000-8000-000000000005"
)

// projectsWorld is the projects' tests' lines, at now, Wednesday 7 October:
// switchboard's seven requests, three on Monday in a worktree of it, two on
// Tuesday and two today in it; five on Monday in no directory; client/api's
// one on Tuesday, in a subdirectory of it; and own/api's two today.
func projectsWorld() []ledger.Held {
	return slices.Concat(
		projectAsks(worktreeSession, "/code/switchboard/.claude/worktrees/fix", "", "work", times(october(5, 10, 0), 3)...),
		projectAsks(nowhereSession, "", "", "work", times(october(5, 11, 0), 5)...),
		projectAsks(repoSession, "/code/switchboard", "/code/switchboard", "side", times(october(6, 9, 0), 2)...),
		projectAsks(clientSession, "/code/client/api/web", "/code/client/api", "personal", october(6, 15, 0)),
		projectAsks(repoSession, "/code/switchboard", "/code/switchboard", "side", times(october(7, 9, 0), 2)...),
		projectAsks(ownSession, "/code/own/api", "/code/own/api", "work", times(october(7, 12, 0), 2)...),
	)
}

// listedRunning is the List's session with the given id the router lists, in dir
// of the project given.
func listedRunning(id, dir, project string) views.ListedSession {
	return views.ListedSession{Session: id, Dir: dir, Project: project, Running: true, Activity: views.Activity{Doing: views.Idle}}
}

// The list ranks the projects with a session in the period by their worth,
// the sessions with no directory one row among them, each named by its
// folder, with its parent where two share it; and totals every project's.
func TestProjectsListsEachProjectByWorthWithEveryProjectsTotals(t *testing.T) {
	page := heldDays(now, projectsWorld()...).Projects(projectsIn(views.PeriodThisWeek, now, listedRunning(repoSession, "/code/switchboard", "/code/switchboard")))
	type row struct {
		path, name               string
		live, sessions, requests int
		worth                    views.Worth
	}
	var got []row
	for _, r := range page.Rows {
		got = append(got, row{r.Path, r.Name, r.Live, r.Sessions, r.Requests, r.Worth})
	}
	want := []row{
		{"/code/switchboard", "switchboard", 1, 2, 7, hoursWorth(7)},
		{"", "", 0, 1, 5, hoursWorth(5)},
		{"/code/own/api", "own/api", 0, 1, 2, hoursWorth(2)},
		{"/code/client/api", "client/api", 0, 1, 1, hoursWorth(1)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the rows are\n%+v\nwant\n%+v", got, want)
	}
	if want := (views.ProjectTotals{Live: 1, Sessions: 5, Requests: 15, Worth: hoursWorth(15)}); !reflect.DeepEqual(page.All, want) {
		t.Errorf("every project's totals are %+v, want %+v", page.All, want)
	}
	if !page.Rows[1].NoDirectory() || page.Rows[0].NoDirectory() {
		t.Error("want the second row alone of the sessions with no directory")
	}
	if last := page.Rows[0].Last; !last.Equal(october(7, 9, 1)) {
		t.Errorf("switchboard's last request came at %v, want its last today, %v", last, october(7, 9, 1))
	}
}

// A session is its project's as status.ProjectOf reads its lines: one in a
// worktree Claude Code made, from a line before projects, its repository's;
// one in a subdirectory, the project run told. Inside its project, a session
// keeps its own directory.
func TestASessionIsItsProjectsAsItsLinesName(t *testing.T) {
	page := heldDays(now, projectsWorld()...).Projects(projectsIn(views.PeriodThisWeek, now))
	tests := []struct {
		path string
		dirs map[string]string
	}{
		{path: "/code/switchboard", dirs: map[string]string{worktreeSession: "/code/switchboard/.claude/worktrees/fix", repoSession: "/code/switchboard"}},
		{path: "/code/client/api", dirs: map[string]string{clientSession: "/code/client/api/web"}},
	}
	for _, tt := range tests {
		project, ok := page.Project(tt.path)
		if !ok {
			t.Fatalf("no page of %s", tt.path)
		}
		got := make(map[string]string)
		for _, s := range project.Sessions {
			got[s.Session] = s.Dir
		}
		if !reflect.DeepEqual(got, tt.dirs) {
			t.Errorf("%s's sessions are in %v, want %v", tt.path, got, tt.dirs)
		}
	}
}

// SESSIONS lists a project's sessions in the List's order, those running
// first, each as the List has it, its requests and worth over the period;
// those ended after, the latest ended first, each with the day it ended on.
func TestAProjectsSessionsAreRowedAsTheListRowsThem(t *testing.T) {
	listed := listedRunning(repoSession, "/code/switchboard", "/code/switchboard")
	listed.Requests, listed.Worth = 2, hoursWorth(2)
	page := heldDays(now, projectsWorld()...).Projects(projectsIn(views.PeriodThisWeek, now, listed))
	project, _ := page.Project("/code/switchboard")
	type row struct {
		id       string
		running  bool
		requests int
		worth    views.Worth
		endedOn  string
	}
	var got []row
	for _, s := range project.Sessions {
		got = append(got, row{s.Session, s.Running, s.Requests, s.Worth, s.EndedOn})
	}
	want := []row{{repoSession, true, 4, hoursWorth(4), ""}, {worktreeSession, false, 3, hoursWorth(3), "2026-10-05"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("switchboard's sessions are\n%+v\nwant\n%+v", got, want)
	}
	ended := project.Sessions[1]
	if ended.Account != "work" || ended.Model != opus || !ended.Started.Equal(october(5, 10, 0)) || !ended.Ended.Equal(october(5, 10, 2)) {
		t.Errorf("the ended session is on %q, of %q, from %v to %v; want on work, of %s, from %v to %v",
			ended.Account, ended.Model, ended.Started, ended.Ended, opus, october(5, 10, 0), october(5, 10, 2))
	}
}

// A session ended on a day gone is brought to its account as the List says
// it of one ended today: by its model's latest move, onto the account it's
// named on, its words the router's reason and what writing its context
// again there cost.
func TestAnEndedSessionKeepsTheMoveThatBroughtItToItsAccount(t *testing.T) {
	moved := projectAsk(worktreeSession, "/code/switchboard", "", "work", opus, october(6, 10, 0))
	moved.From, moved.Reason = "side", "moved: side hit its limit"
	moved.Tried = []ledger.Tried{{Account: "side", Why: "hit its limit"}}
	lines := []ledger.Held{projectAsk(worktreeSession, "/code/switchboard", "", "side", opus, october(6, 9, 0)), moved}
	page := heldDays(now, lines...).Projects(projectsIn(views.PeriodThisWeek, now))
	project, _ := page.Project("/code/switchboard")
	s := project.Sessions[0]
	if s.Account != "work" || s.Moved == nil || s.Moved.From != "side" || s.Routing.From != "side" || !s.Routing.Rewrote || s.Routing.Cost == nil {
		t.Errorf("the session is on %q, moved %+v, routed %+v; want on work, moved from side, its context written again at a cost", s.Account, s.Moved, s.Routing)
	}
	if s.Mark != views.NoMark {
		t.Errorf("its mark is %v, want none", s.Mark)
	}
}

// BY MODEL gives each version's share of the project's worth as Accounts
// reckons a row's, the greatest first, one whose share rounds to none left
// out; ON ACCOUNTS each account's, the greatest first.
func TestAProjectsWorthIsSharedByModelAndByAccount(t *testing.T) {
	small := projectAsk(repoSession, "/code/switchboard", "", "work", haiku, october(7, 9, 30))
	small.Usage = json.RawMessage(smallUsage)
	lines := slices.Concat(
		projectAsks(repoSession, "/code/switchboard", "", "side", times(october(7, 9, 0), 3)...),
		[]ledger.Held{projectAsk(repoSession, "/code/switchboard", "", "work", "claude-sonnet-5-5", october(7, 9, 10)), small},
	)
	page := heldDays(now, lines...).Projects(projectsIn(views.PeriodToday, now))
	project, _ := page.Project("/code/switchboard")
	var models []string
	for _, v := range project.ByModel {
		models = append(models, v.Model)
	}
	if want := []string{opus, "claude-sonnet-5-5"}; !reflect.DeepEqual(models, want) {
		t.Errorf("BY MODEL names %v, want %v, the greatest first and Haiku's sliver left out", models, want)
	}
	var accounts []string
	for _, a := range project.OnAccounts {
		accounts = append(accounts, a.Account)
	}
	if want := []string{"side", "work"}; !reflect.DeepEqual(accounts, want) {
		t.Errorf("ON ACCOUNTS names %v, want %v, the greatest first", accounts, want)
	}
	if project.Share == nil || *project.Share != 1 {
		t.Errorf("its share of every project's worth is %v, want all of it", project.Share)
	}
}

// WORTH BY gives a project's worth in the period's buckets: today's by hour,
// this week's and the last 30 days' by day, the last 12 weeks' by day and by
// week, the last 12 months' by week; each its label, the peak the first
// bucket worth most, and those of the period still to come counted.
func TestAProjectsWorthIsBucketedAsItsPeriodHasIt(t *testing.T) {
	world := slices.Concat(
		projectAsks(repoSession, "/code/switchboard", "", "work", times(october(7, 13, 0), 2)...),
		projectAsks(repoSession, "/code/switchboard", "", "work", october(7, 9, 0)),
		projectAsks(repoSession, "/code/switchboard", "", "work", times(october(6, 9, 0), 3)...),
		projectAsks(repoSession, "/code/switchboard", "", "work", october(5, 9, 0)),
		projectAsks(repoSession, "/code/switchboard", "", "work", times(october(2, 9, 0), 4)...),
		projectAsks(repoSession, "/code/switchboard", "", "work", time.Date(2026, 9, 20, 9, 0, 0, 0, time.Local)),
		projectAsks(repoSession, "/code/switchboard", "", "work", times(time.Date(2026, 8, 1, 9, 0, 0, 0, time.Local), 2)...),
		projectAsks(repoSession, "/code/switchboard", "", "work", time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local)),
	)
	days := heldDays(now, world...)
	type bucketed struct {
		unit                  views.BucketUnit
		buckets, peak, toCome int
		first, peakLabel      string
		peakWorth             views.Worth
	}
	of := func(by views.WorthBy) bucketed {
		return bucketed{by.Unit, len(by.Buckets), by.Peak, by.ToCome, by.Buckets[0].Label, by.Buckets[by.Peak].Label, by.Buckets[by.Peak].Worth}
	}
	tests := []struct {
		period views.Period
		want   bucketed
		weeks  *bucketed
	}{
		{period: views.PeriodToday, want: bucketed{views.ByHour, 24, 13, 10, "00", "13", hoursWorth(2)}},
		{period: views.PeriodThisWeek, want: bucketed{views.ByDay, 7, 1, 4, "Mon", "Tue", hoursWorth(3)}},
		{period: views.PeriodThirtyDays, want: bucketed{views.ByDay, 30, 24, 0, "8 Sep", "2 Oct", hoursWorth(4)}},
		{
			period: views.PeriodTwelveWeeks, want: bucketed{views.ByDay, 84, 74, 4, "20 Jul", "2 Oct", hoursWorth(4)},
			weeks: &bucketed{views.ByWeek, 12, 11, 0, "20 Jul", "5 Oct", hoursWorth(7)},
		},
		{period: views.PeriodTwelveMonths, want: bucketed{views.ByWeek, 53, 52, 0, "6 Oct", "5 Oct", hoursWorth(7)}},
	}
	for _, tt := range tests {
		t.Run(tt.period.Name(), func(t *testing.T) {
			project, _ := days.Projects(projectsIn(tt.period, now)).Project("/code/switchboard")
			if got := of(project.WorthBy); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("WORTH BY is %+v, want %+v", got, tt.want)
			}
			switch {
			case tt.weeks == nil && project.WorthByWeek != nil:
				t.Errorf("WORTH BY WEEK is %+v, want none", *project.WorthByWeek)
			case tt.weeks != nil && project.WorthByWeek == nil:
				t.Error("WORTH BY WEEK is none, want the period's weeks")
			case tt.weeks != nil && !reflect.DeepEqual(of(*project.WorthByWeek), *tt.weeks):
				t.Errorf("WORTH BY WEEK is %+v, want %+v", of(*project.WorthByWeek), *tt.weeks)
			}
			var sum views.Worth
			for _, b := range project.WorthBy.Buckets {
				sum = sumOf(sum, b.Worth)
			}
			if !reflect.DeepEqual(sum, project.Worth) {
				t.Errorf("the buckets sum to %+v, want the project's worth, %+v", sum, project.Worth)
			}
		})
	}
}

// sumOf is a and b summed.
func sumOf(a, b views.Worth) views.Worth {
	switch {
	case a.Amount == nil:
		return b
	case b.Amount == nil:
		return a
	}
	sum := *a.Amount + *b.Amount
	return views.Worth{Amount: &sum}
}

// As the day turns while the days are held, what was folded of the day gone
// is kept as its, none of it lost or taken twice; and its lines given again
// whole, as the tail gives a day it followed no further, take its place, a
// line that came late among them: so the page is the one the days read
// afresh give.
func TestTheDayTurningLosesNoLineAndTakesNoneTwice(t *testing.T) {
	tuesday := october(6, 23, 0)
	early := projectAsk(repoSession, "/code/switchboard", "", "work", opus, october(6, 10, 0))
	late := projectAsk(repoSession, "/code/switchboard", "", "work", opus, october(6, 23, 59))
	after := projectAsk(repoSession, "/code/switchboard", "", "work", opus, october(7, 0, 1))
	wednesday := october(7, 0, 5)
	held := views.ReadSessionDays(ledgerOfLines(), tuesday.AddDate(0, -1, 0), tuesday).Tailed([]ledger.Held{early}, true, nil, tuesday)
	tests := []struct {
		name     string
		turned   views.SessionDays
		requests int
	}{
		{name: "what was folded kept", turned: held.Tailed([]ledger.Held{after}, true, nil, wednesday), requests: 2},
		{
			name:     "the day gone given again whole",
			turned:   held.Tailed([]ledger.Held{after}, true, map[string][]ledger.Held{"2026-10-06": {early, late}}, wednesday),
			requests: 3,
		},
		{
			name:     "a line of the day gone among today's passed over",
			turned:   held.Tailed([]ledger.Held{late, after}, true, nil, wednesday),
			requests: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := tt.turned.Projects(projectsIn(views.PeriodThisWeek, wednesday))
			if got := page.All.Requests; got != tt.requests {
				t.Errorf("the page counts %d requests, want %d", got, tt.requests)
			}
		})
	}
	whole := heldDays(wednesday, early, late, after).Projects(projectsIn(views.PeriodThisWeek, wednesday))
	given := tests[1].turned.Projects(projectsIn(views.PeriodThisWeek, wednesday))
	if !reflect.DeepEqual(given.Rows, whole.Rows) {
		t.Errorf("the page of the day given again whole is\n%+v\nwant, as the days read afresh give\n%+v", given.Rows, whole.Rows)
	}
	if again := held.Tailed(nil, false, nil, tuesday).Projects(projectsIn(views.PeriodThisWeek, tuesday)); again.All.Requests != 1 {
		t.Errorf("held, the day not turned, the page counts %d requests, want 1", again.All.Requests)
	}
}
