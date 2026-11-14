package views_test

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
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
	return views.ReadSessionDays(l, now.AddDate(-1, 0, 0), now).Tailed(l.days[dayfile.DateOf(now)], true, nil, nil, now)
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

// A project's row does what its busiest session running does, so its last
// request reads as now while one is busy; one with none running is idle.
func TestAProjectsRowDoesWhatItsBusiestSessionRunningDoes(t *testing.T) {
	answering := listedRunning(worktreeSession, "/code/switchboard/.claude/worktrees/fix", "/code/switchboard")
	answering.Activity = views.Activity{Doing: views.Answering, Since: october(7, 9, 1)}
	running := []views.ListedSession{listedRunning(repoSession, "/code/switchboard", "/code/switchboard"), answering}
	page := heldDays(now, projectsWorld()...).Projects(projectsIn(views.PeriodThisWeek, now, running...))
	for _, r := range page.Rows {
		want := views.Activity{}
		if r.Path == "/code/switchboard" {
			want = answering.Activity
		}
		if !reflect.DeepEqual(r.Activity, want) {
			t.Errorf("%q does %+v, want %+v", r.Path, r.Activity, want)
		}
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
// again there cost, marked ▸.
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
	if s.Mark != views.MarkMoved {
		t.Errorf("its mark is %v, want ▸, as the List marks a session a move brought", s.Mark)
	}
}

// BY MODEL gives each version's share of the project's worth as Accounts
// reckons a row's, the greatest first, one whose share rounds to none left
// out; ON ACCOUNTS each account's, the greatest first, of the worth of the
// requests that went to one, so their shares make the whole.
func TestAProjectsWorthIsSharedByModelAndByAccount(t *testing.T) {
	small := projectAsk(repoSession, "/code/switchboard", "", "work", haiku, october(7, 9, 30))
	small.Usage = json.RawMessage(smallUsage)
	lines := slices.Concat(
		projectAsks(repoSession, "/code/switchboard", "", "side", times(october(7, 9, 0), 3)...),
		[]ledger.Held{
			projectAsk(repoSession, "/code/switchboard", "", "work", "claude-sonnet-5-5", october(7, 9, 10)), small,
			projectAsk(repoSession, "/code/switchboard", "", "", "claude-sonnet-5-5", october(7, 9, 20)),
		},
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
	shares := 0.0
	for _, a := range project.OnAccounts {
		shares += *a.Share
	}
	if math.Abs(shares-1) > 1e-9 {
		t.Errorf("ON ACCOUNTS' shares make %v, want the whole", shares)
	}
	if project.Share == nil || *project.Share != 1 {
		t.Errorf("its share of every project's worth is %v, want all of it", project.Share)
	}
}

// WORTH BY gives a project's worth in the period's buckets: today's by hour,
// this week's and the last 30 days' by day, the last 12 weeks' by day and by
// week, the last 12 months' by week; each its label, the peak the first
// bucket worth most, and those of the period still to come counted, the one
// now falls in the last before them. The peak is named as the line under
// the chart says it; the axis names each hour and day, and a week only where
// its month starts.
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
		unit                       views.BucketUnit
		buckets, peak, toCome, now int
		first, peakLabel           string
		peakWorth                  views.Worth
		peakNamed                  string
		axis                       [2]string
	}
	of := func(by views.WorthBy) bucketed {
		return bucketed{
			by.Unit, len(by.Buckets), by.Peak, by.ToCome, by.Now(), by.Buckets[0].Label, by.Buckets[by.Peak].Label, by.Buckets[by.Peak].Worth,
			by.PeakNamed(), [2]string{by.Buckets[0].Axis, by.Buckets[1].Axis},
		}
	}
	tests := []struct {
		period views.Period
		want   bucketed
		weeks  *bucketed
	}{
		{period: views.PeriodToday, want: bucketed{views.ByHour, 24, 13, 10, 13, "00", "13", hoursWorth(2), "13:00", [2]string{"00", "01"}}},
		{period: views.PeriodThisWeek, want: bucketed{views.ByDay, 7, 1, 4, 2, "Mon", "Tue", hoursWorth(3), "Tue", [2]string{"Mon", "Tue"}}},
		{period: views.PeriodThirtyDays, want: bucketed{views.ByDay, 30, 24, 0, 29, "8 Sep", "2 Oct", hoursWorth(4), "2 Oct", [2]string{"8 Sep", "9 Sep"}}},
		{
			period: views.PeriodTwelveWeeks, want: bucketed{views.ByDay, 84, 74, 4, 79, "20 Jul", "2 Oct", hoursWorth(4), "2 Oct", [2]string{"20 Jul", "21 Jul"}},
			weeks: &bucketed{views.ByWeek, 12, 11, 0, 11, "20 Jul", "5 Oct", hoursWorth(7), "week of 5 Oct", [2]string{"Jul", ""}},
		},
		{period: views.PeriodTwelveMonths, want: bucketed{views.ByWeek, 53, 52, 0, 52, "6 Oct", "5 Oct", hoursWorth(7), "week of 5 Oct", [2]string{"Oct", ""}}},
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
			for h, b := range project.WorthBy.Buckets {
				sum = sumOf(sum, b.Worth)
				if at := b.Start.Local(); tt.period == views.PeriodToday && (at.Hour() != h || dayfile.DateOf(at) != dayfile.DateOf(now)) {
					t.Errorf("hour %d's bucket starts at %v, want as today's clock reads it", h, at)
				}
			}
			if !reflect.DeepEqual(sum, project.Worth) {
				t.Errorf("the buckets sum to %+v, want the project's worth, %+v", sum, project.Worth)
			}
		})
	}
}

// Over 12 months, the axis names the first week by the month of its first
// day in the period, not of a day before it, where the week begins in the
// month before.
func TestTwelveMonthsAxisNamesAWeekByItsFirstDayInThePeriod(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.Local)
	days := heldDays(now, projectAsks(repoSession, "/code/switchboard", "", "work", now.Add(-time.Hour))...)
	project, _ := days.Projects(projectsIn(views.PeriodTwelveMonths, now)).Project("/code/switchboard")
	b := project.WorthBy.Buckets
	if got := [3]string{b[0].Label, b[0].Axis, b[1].Axis}; got != [3]string{"27 Oct", "Nov", ""} {
		t.Errorf("the first two weeks are named and on the axis as %q, want the week of 27 Oct on the axis as Nov, its first day in the period's month, and the next unnamed", got)
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
	held := views.ReadSessionDays(ledgerOfLines(), tuesday.AddDate(0, -1, 0), tuesday).Tailed([]ledger.Held{early}, true, nil, nil, tuesday)
	tests := []struct {
		name     string
		turned   views.SessionDays
		requests int
	}{
		{name: "what was folded kept", turned: held.Tailed([]ledger.Held{after}, true, nil, nil, wednesday), requests: 2},
		{
			name:     "the day gone given again whole",
			turned:   held.Tailed([]ledger.Held{after}, true, nil, map[string][]ledger.Held{"2026-10-06": {early, late}}, wednesday),
			requests: 3,
		},
		{
			name:     "a line of the day gone among today's passed over",
			turned:   held.Tailed([]ledger.Held{late, after}, true, nil, nil, wednesday),
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
	if again := held.Tailed(nil, false, nil, nil, tuesday).Projects(projectsIn(views.PeriodThisWeek, tuesday)); again.All.Requests != 1 {
		t.Errorf("held, the day not turned, the page counts %d requests, want 1", again.All.Requests)
	}
}

// Built once the day has turned, before the tail is read again, the page
// counts what was folded of the day gone as that day's; and the days held
// say the day gone is owed a read of its lines whole, each day the watch
// slept through too, till the tail's late read gives them, the lines of the
// day gone that came after its last read among them.
func TestTheDayGoneIsOwedTillItsLinesAreReadWhole(t *testing.T) {
	tuesday := october(6, 23, 0)
	early := projectAsk(repoSession, "/code/switchboard", "", "work", opus, october(6, 10, 0))
	late := projectAsk(repoSession, "/code/switchboard", "", "work", opus, october(6, 23, 59))
	held := views.ReadSessionDays(ledgerOfLines(), tuesday.AddDate(0, -1, 0), tuesday).Tailed([]ledger.Held{early}, true, nil, nil, tuesday)
	wednesday := october(7, 0, 10)
	if got := held.Projects(projectsIn(views.PeriodThisWeek, wednesday)).All.Requests; got != 1 {
		t.Errorf("built after the day turned, before the tail's read, the page counts %d requests, want Tuesday's 1", got)
	}
	if got := held.Projects(projectsIn(views.PeriodToday, wednesday)).All.Requests; got != 0 {
		t.Errorf("today's page counts %d requests of the day gone, want none", got)
	}
	if got, want := held.Owed(wednesday), []string{"2026-10-06"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the day turned, the days owe %v, want %v", got, want)
	}
	if got, want := held.Owed(october(9, 8, 0)), []string{"2026-10-06", "2026-10-07", "2026-10-08"}; !reflect.DeepEqual(got, want) {
		t.Errorf("days later, the days owe %v, want %v", got, want)
	}
	unread := held.Tailed(nil, true, nil, nil, wednesday)
	if got, want := unread.Owed(wednesday), []string{"2026-10-06"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tailed without the day gone, the days owe %v, want %v still", got, want)
	}
	read := unread.Tailed(nil, false, nil, map[string][]ledger.Held{"2026-10-06": {early, late}}, wednesday)
	if got := read.Owed(wednesday); len(got) != 0 {
		t.Errorf("given the day gone whole, the days owe %v, want none", got)
	}
	if got := read.Projects(projectsIn(views.PeriodThisWeek, wednesday)).All.Requests; got != 2 {
		t.Errorf("given the day gone whole, the page counts %d requests, want 2, its line after the last read among them", got)
	}
	friday := october(9, 8, 0)
	slept := held.Tailed(nil, true, nil, nil, friday)
	sleptThrough := []string{"2026-10-06", "2026-10-07", "2026-10-08"}
	if got := slept.Owed(friday); !reflect.DeepEqual(got, sleptThrough) {
		t.Errorf("tailed on Friday, the days owe %v, want %v, the days slept through among them", got, sleptThrough)
	}
	woken := slept.Tailed(nil, false, sleptThrough, map[string][]ledger.Held{"2026-10-06": {early, late}}, friday)
	if got := woken.Owed(friday); len(got) != 0 {
		t.Errorf("each day read, those of no lines among them, the days owe %v, want none", got)
	}
	if got := woken.Projects(projectsIn(views.PeriodThisWeek, friday)).All.Requests; got != 2 {
		t.Errorf("each day read, the page counts %d requests, want Tuesday's 2", got)
	}
}

// The days read, joined to those the tail fed, take today's sessions from
// the tail, never the read; the day the tail turned and read again whole in
// place of the read's of it, read before its last line came; the read's
// other days; and the read's walk back for the sessions running into its
// first day: so a read landing after the tail's turn builds the page the
// days read afresh give.
func TestTheDaysReadJoinTheDaysTheTailFed(t *testing.T) {
	sunday := projectAsk(worktreeSession, "/code/switchboard", "", "work", opus, october(4, 22, 0))
	monday := projectAsk(worktreeSession, "/code/switchboard", "", "work", opus, october(5, 9, 0))
	tuesday := projectAsks(repoSession, "/code/switchboard", "", "work", october(6, 10, 0), october(6, 23, 59))
	today := projectAsks(repoSession, "/code/switchboard", "", "work", october(7, 0, 1), october(7, 9, 0))
	turned := october(7, 0, 0)
	read := views.ReadSessionDays(ledgerOfLines(sunday, monday, tuesday[0]), october(5, 0, 0), turned).Tailed(today[:1], true, nil, nil, turned)
	wednesday := october(7, 9, 30)
	tailed := views.ReadSessionDays(ledgerOfLines(), october(6, 0, 0), october(6, 23, 0)).Tailed(tuesday[:1], true, nil, nil, october(6, 23, 0)).
		Tailed(today, true, []string{"2026-10-06"}, map[string][]ledger.Held{"2026-10-06": tuesday}, wednesday)

	joined := tailed.Joined(read).Projects(projectsIn(views.PeriodThisWeek, wednesday))
	if got := joined.All.Requests; got != 5 {
		t.Errorf("the page counts %d requests, want 5: Monday's of the read, Tuesday's 2 and today's 2 of the tail", got)
	}
	whole := heldDays(wednesday, slices.Concat([]ledger.Held{sunday, monday}, tuesday, today)...).Projects(projectsIn(views.PeriodThisWeek, wednesday))
	if !reflect.DeepEqual(joined.Rows, whole.Rows) {
		t.Errorf("the rows joined are\n%+v\nwant, as the days read afresh give\n%+v", joined.Rows, whole.Rows)
	}
	got, _ := joined.Project("/code/switchboard")
	want, _ := whole.Project("/code/switchboard")
	if !reflect.DeepEqual(got.Sessions, want.Sessions) {
		t.Errorf("the sessions joined are\n%+v\nwant, the worktree's started on Sunday, as the read walked back for it\n%+v", got.Sessions, want.Sessions)
	}
}

// Today's fold alone lets go of the days gone the days hold, owing none of
// them a read, and folds on what the tail gives of today, each fold told.
func TestTodayAloneLetsGoOfTheDaysGone(t *testing.T) {
	tuesday, wednesday := october(6, 23, 0), october(7, 0, 5)
	early := projectAsk(repoSession, "/code/switchboard", "", "work", opus, october(6, 10, 0))
	after := projectAsks(repoSession, "/code/switchboard", "", "work", october(7, 0, 1), october(7, 0, 3))
	turned := views.ReadSessionDays(ledgerOfLines(), october(5, 0, 0), tuesday).Tailed([]ledger.Held{early}, true, nil, nil, tuesday).
		Tailed(after[:1], true, nil, nil, wednesday)
	if got := turned.Projects(projectsIn(views.PeriodThisWeek, wednesday)).All.Requests; got != 2 {
		t.Fatalf("turned, the days count %d requests this week, want Tuesday's and today's, 2", got)
	}
	alone := turned.TodayAlone()
	if owed := alone.Owed(wednesday); len(owed) > 0 {
		t.Errorf("today's alone owes %v, want none", owed)
	}
	fed := alone.Tailed(after[1:], false, nil, nil, wednesday).TodayAlone()
	if got := fed.Projects(projectsIn(views.PeriodThisWeek, wednesday)).All.Requests; got != 2 {
		t.Errorf("today's alone, fed today's second, counts %d requests this week, want today's 2", got)
	}
	if !fed.Told(alone) {
		t.Error("today's alone, fed today's second, tells nothing otherwise, want it told")
	}
}

// A line the ledger's files hold twice is counted once, as the List and the
// Log count it, by its id, in a day read whole, in a day's side, and across
// the tail's reads of today; one with no id as often as a read gives it
// beyond those held alike, as countedOnce has it, as a read can't tell two
// such requests apart.
func TestALineHeldTwiceIsCountedOnce(t *testing.T) {
	twice := projectAsk(repoSession, "/code/switchboard", "", "work", opus, october(6, 10, 0))
	idless := projectAsk(repoSession, "/code/switchboard", "", "work", opus, october(6, 11, 0))
	idless.Request = ""
	if got := heldDays(now, twice, twice, idless, idless).Projects(projectsIn(views.PeriodThisWeek, now)).All.Requests; got != 3 {
		t.Errorf("a day read whole counts %d requests, want 3: the line with an id once, the two without each", got)
	}
	today := projectAsk(repoSession, "/code/switchboard", "", "work", opus, october(7, 9, 0))
	tailed := views.ReadSessionDays(ledgerOfLines(), now.AddDate(0, -1, 0), now).
		Tailed([]ledger.Held{today}, true, nil, nil, now).Tailed([]ledger.Held{today}, false, nil, nil, now)
	if got := tailed.Projects(projectsIn(views.PeriodToday, now)).All.Requests; got != 1 {
		t.Errorf("today's line given by two reads of the tail counts %d requests, want 1", got)
	}
	todayIdless := today
	todayIdless.Request = ""
	once := views.ReadSessionDays(ledgerOfLines(), now.AddDate(0, -1, 0), now).Tailed([]ledger.Held{todayIdless}, true, nil, nil, now)
	for _, tt := range []struct {
		name     string
		given    []ledger.Held
		requests int
	}{
		{"given again once, held once", []ledger.Held{todayIdless}, 1},
		{"given again twice, held once", []ledger.Held{todayIdless, todayIdless}, 2},
	} {
		if got := once.Tailed(tt.given, false, nil, nil, now).Projects(projectsIn(views.PeriodToday, now)).All.Requests; got != tt.requests {
			t.Errorf("a line with no id %s across the tail's reads counts %d requests, want %d", tt.name, got, tt.requests)
		}
	}
	build := views.Digests{}.Day()
	build.Take(twice.Line)
	build.Take(twice.Line)
	built, err := build.Built()
	if err != nil {
		t.Fatal(err)
	}
	sided := sidedLedger{sides: map[string]json.RawMessage{"2026-10-06": built.Side}}
	requests := 0
	for _, s := range views.DaySessionsOn(sided, "2026-10-06") {
		for _, u := range s.Uses {
			requests += u.Requests
		}
	}
	if requests != 1 {
		t.Errorf("a day's side built of its line twice holds %d requests, want 1", requests)
	}
}

// A session's start is its first request wherever it fell: before the
// period, as the days held give it, or as ReadSessionDays walks back for it
// from the first day it reads, while it ran on the day after, so a session
// that ran over midnight starts yesterday on today's page; and the walk
// stops 31 days back.
func TestASessionStartsAtItsFirstRequestBeforeThePeriod(t *testing.T) {
	sunday := projectAsk(worktreeSession, "/code/switchboard", "", "work", opus, october(4, 22, 0))
	monday := projectAsk(worktreeSession, "/code/switchboard", "", "work", opus, october(5, 9, 0))
	overnight := projectAsks(repoSession, "/code/switchboard", "", "work", october(6, 23, 30), october(7, 0, 30))
	l := ledgerOfLines(slices.Concat([]ledger.Held{sunday, monday}, overnight)...)
	todays := l.days["2026-10-07"]
	var month []ledger.Held
	for d := range 40 {
		month = append(month, projectAsk(clientSession, "/code/client", "", "work", opus, october(7, 8, 0).AddDate(0, 0, -d)))
	}
	lm := ledgerOfLines(month...)
	tests := []struct {
		name    string
		days    views.SessionDays
		period  views.Period
		session string
		want    time.Time
	}{
		{"of the days held", heldDays(now, sunday, monday), views.PeriodThisWeek, worktreeSession, october(4, 22, 0)},
		{"walked back from the week's first day", views.ReadSessionDays(l, october(5, 0, 0), now).Tailed(todays, true, nil, nil, now), views.PeriodThisWeek, worktreeSession, october(4, 22, 0)},
		{"over midnight, of today's", views.ReadSessionDays(l, now, now).Tailed(todays, true, nil, nil, now), views.PeriodToday, repoSession, october(6, 23, 30)},
		{"walked back no further than 31 days", views.ReadSessionDays(lm, now, now).Tailed(lm.days["2026-10-07"], true, nil, nil, now), views.PeriodToday, clientSession, october(7, 8, 0).AddDate(0, 0, -31)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := tt.days.Projects(projectsIn(tt.period, now))
			var started time.Time
			for _, r := range page.Rows {
				project, _ := page.Project(r.Path)
				for _, s := range project.Sessions {
					if s.Session == tt.session {
						started = s.Started
					}
				}
			}
			if !started.Equal(tt.want) {
				t.Errorf("the session started at %v, want %v", started, tt.want)
			}
		})
	}
}

// A session is one project's, the one of where its latest day ran, however
// its days name it: one in a subdirectory running across the upgrade that
// began recording projects, its lines first naming none, then the project
// run told, is counted once, in that project.
func TestASessionAcrossTheUpgradeIsOneProjects(t *testing.T) {
	before := projectAsk(repoSession, "/code/switchboard/internal", "", "work", opus, october(5, 10, 0))
	after := projectAsk(repoSession, "/code/switchboard/internal", "/code/switchboard", "work", opus, october(6, 10, 0))
	page := heldDays(now, before, after).Projects(projectsIn(views.PeriodThisWeek, now))
	var got []string
	for _, r := range page.Rows {
		got = append(got, fmt.Sprintf("%s %d sessions %d requests", r.Path, r.Sessions, r.Requests))
	}
	if want := []string{"/code/switchboard 1 sessions 2 requests"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the rows are %v, want %v", got, want)
	}
}

// The design's world sums as the design says: this week, six projects of
// 27 sessions, 5,410 requests and $635.00, each running one session;
// switchboard first, of eight sessions, 1,874 requests and $221.40, its
// worktrees' among them, each named by its own directory, its worth by day
// peaking on Wednesday at $84.70; and over the last 12 months, kit and
// old-site too, though no session of theirs ran this week, last active on
// 14 June and 2 March.
func TestTheDesignsWorldSumsAsTheDesignSays(t *testing.T) {
	w := viewstest.NewProjectsWorld()
	days := w.Days()
	week := days.Projects(w.Input(views.PeriodThisWeek))
	type row struct {
		name                     string
		live, sessions, requests int
		worth                    views.Worth
	}
	var got []row
	for _, r := range week.Rows {
		got = append(got, row{r.Name, r.Live, r.Sessions, r.Requests, r.Worth})
	}
	want := []row{
		{"switchboard", 1, 8, 1874, cents(22140)}, {"website", 1, 5, 1102, cents(13980)}, {"infra", 1, 5, 846, cents(9810)},
		{"api", 1, 4, 702, cents(8730)}, {"docs", 1, 3, 611, cents(6120)}, {"mobile", 1, 2, 275, cents(2720)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("this week's rows are\n%+v\nwant\n%+v", got, want)
	}
	if want := (views.ProjectTotals{Live: 6, Sessions: 27, Requests: 5410, Worth: cents(63500)}); !reflect.DeepEqual(week.All, want) {
		t.Errorf("every project's this week are %+v, want %+v", week.All, want)
	}
	switchboard, _ := week.Project(viewstest.Switchboard)
	var byDay []views.Worth
	for _, b := range switchboard.WorthBy.Buckets[:4] {
		byDay = append(byDay, b.Worth)
	}
	if want := []views.Worth{cents(2740), cents(3900), cents(8470), cents(7030)}; !reflect.DeepEqual(byDay, want) {
		t.Errorf("switchboard's worth Monday to today is %+v, want %+v", byDay, want)
	}
	if peak := switchboard.WorthBy.Buckets[switchboard.WorthBy.Peak]; peak.Label != "Wed" {
		t.Errorf("switchboard's peak is %s, want Wednesday's", peak.Label)
	}
	var dirs []string
	for _, s := range switchboard.Sessions {
		dirs = append(dirs, status.DirName(s.Dir, 20))
	}
	if want := []string{"switchboard", "fix-login", "switchboard", "pools", "pools", "switchboard", "switchboard", "release"}; !reflect.DeepEqual(dirs, want) {
		t.Errorf("switchboard's sessions are named %v, want %v", dirs, want)
	}
	year := days.Projects(w.Input(views.PeriodTwelveMonths))
	last := make(map[string]string)
	for _, r := range year.Rows {
		last[r.Name] = r.Last.Local().Format(time.DateOnly)
	}
	if last["kit"] != "2026-06-14" || last["old-site"] != "2026-03-02" {
		t.Errorf("over 12 months kit was last active on %q and old-site on %q, want 2026-06-14 and 2026-03-02", last["kit"], last["old-site"])
	}
}

// cents is a worth of n cents, priced.
func cents(n int64) views.Worth {
	return priced(n * 10_000)
}
