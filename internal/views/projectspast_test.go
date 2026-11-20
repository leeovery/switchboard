package views_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// september is the local time on the given day of September 2026, at
// hour:minute.
func september(day, hour, minute int) time.Time {
	return time.Date(2026, 9, day, hour, minute, 0, 0, time.Local)
}

// The sessions of the past periods' tests, at framesNow, Thursday 1
// October, last week Monday 21 to Sunday 27 September.
const (
	// begunBefore began on Saturday 19 September in api, and ended on
	// Monday 21st.
	begunBefore = "b0000000-0000-4000-8000-000000000001"
	// endedInside ran on Tuesday 22nd alone, in switchboard.
	endedInside = "b0000000-0000-4000-8000-000000000002"
	// stillRunning ran in api on Wednesday 23rd and Sunday 27th, and runs
	// now in web, sending nothing since.
	stillRunning = "b0000000-0000-4000-8000-000000000003"
	// wentOnLater ran in switchboard on Saturday 26th, and again on Tuesday
	// 29th.
	wentOnLater = "b0000000-0000-4000-8000-000000000004"
	// todayOnly runs in web, sending today alone.
	todayOnly = "b0000000-0000-4000-8000-000000000005"
)

// pastWeekWorld is the past periods' tests' lines, as the constants above
// tell them, oldest first.
func pastWeekWorld() []ledger.Held {
	return byArrival(slices.Concat(
		projectAsks(begunBefore, "/code/api", "/code/api", "work", september(19, 9, 0), september(21, 9, 0), september(21, 9, 1), september(21, 9, 2)),
		projectAsks(endedInside, "/code/switchboard", "/code/switchboard", "side", times(september(22, 10, 0), 2)...),
		projectAsks(stillRunning, "/code/api", "/code/api", "work", september(23, 20, 0), september(27, 20, 0)),
		projectAsks(wentOnLater, "/code/switchboard", "/code/switchboard", "work", september(26, 11, 0), september(26, 11, 1), september(29, 11, 0)),
		projectAsks(todayOnly, "/code/web", "/code/web", "work", october(1, 9, 0)),
	))
}

// byArrival are the lines given, oldest first, as the ledger files them.
func byArrival(lines []ledger.Held) []ledger.Held {
	slices.SortStableFunc(lines, func(a, b ledger.Held) int { return a.At.Compare(b.At) })
	return lines
}

// requestsOn are how many of the lines given arrived on the days from the
// one with the date first to the one with the date last.
func requestsOn(lines []ledger.Held, first, last string) int {
	n := 0
	for _, h := range lines {
		if date := dayfile.DateOf(h.At); first <= date && date <= last {
			n++
		}
	}
	return n
}

// instantOf is the instant t, written in UTC, to compare.
func instantOf(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// Last week's projects and List are of its days alone, as now's are of its
// own: each project's sessions, requests and worth those of its days, so
// every project's together come to the same days' sums; none running, none
// live, none doing anything, though one runs now, and is counted where it
// last ran in the period, not where it runs now; each session's requests
// and worth the period's, one that went on after it first, by its start,
// then those that ended in it, by the day they ended; one begun before it
// started at its first request. The List rows each as its project's
// SESSIONS does, and neither tells anything afresh of what runs now.
func TestAPastWeeksProjectsAndListAreOfItsDaysAlone(t *testing.T) {
	world := pastWeekWorld()
	days := heldDays(framesNow, world...)
	lastWeek := pastAt(t, views.LengthThisWeek, "<", time.Monday)
	in := projectsIn(lastWeek, framesNow, listedRunning(stillRunning, "/code/web", "/code/web"), listedRunning(todayOnly, "/code/web", "/code/web"))
	page, log := days.Projects(in), days.SessionLog(in)

	type row struct {
		path                     string
		live, sessions, requests int
		worth                    views.Worth
		last                     string
		activity                 views.Activity
	}
	var rows []row
	for _, r := range page.Rows {
		rows = append(rows, row{r.Path, r.Live, r.Sessions, r.Requests, r.Worth, instantOf(r.Last), r.Activity})
	}
	want := []row{
		{"/code/api", 0, 2, 5, hoursWorth(5), instantOf(september(27, 20, 0)), views.Activity{}},
		{"/code/switchboard", 0, 2, 4, hoursWorth(4), instantOf(september(26, 11, 1)), views.Activity{}},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("last week's projects are\n%+v\nwant\n%+v", rows, want)
	}
	inWeek := requestsOn(world, "2026-09-21", "2026-09-27")
	all := views.ProjectTotals{Sessions: 4, Requests: inWeek, Worth: hoursWorth(int64(inWeek))}
	if !reflect.DeepEqual(page.All, all) || !reflect.DeepEqual(log.All, all) {
		t.Errorf("last week totals %+v on Projects and %+v on the List, want its days' sums, %+v", page.All, log.All, all)
	}

	type session struct {
		id, endedOn     string
		wentOn, running bool
		requests        int
		worth           views.Worth
		started         string
	}
	var sessions []session
	for _, s := range log.Sessions {
		sessions = append(sessions, session{s.Session, s.EndedOn, s.WentOn, s.Running, s.Requests, s.Worth, instantOf(s.Started)})
	}
	wantSessions := []session{
		{stillRunning, "", true, false, 2, hoursWorth(2), instantOf(september(23, 20, 0))},
		{wentOnLater, "", true, false, 2, hoursWorth(2), instantOf(september(26, 11, 0))},
		{endedInside, "2026-09-22", false, false, 2, hoursWorth(2), instantOf(september(22, 10, 0))},
		{begunBefore, "2026-09-21", false, false, 3, hoursWorth(3), instantOf(september(19, 9, 0))},
	}
	if !reflect.DeepEqual(sessions, wantSessions) {
		t.Errorf("last week's List rows\n%+v\nwant\n%+v", sessions, wantSessions)
	}

	rowed := make(map[string]views.PeriodSession)
	for _, r := range page.Rows {
		project, _ := page.Project(r.Path)
		if project.Activity != (views.Activity{}) {
			t.Errorf("%s's page is doing %+v, want nothing, as nothing runs in a period gone", r.Path, project.Activity)
		}
		for _, s := range project.Sessions {
			s.ProjectName = r.Name
			rowed[s.Session] = s
		}
	}
	for _, s := range log.Sessions {
		if !reflect.DeepEqual(s, rowed[s.Session]) {
			t.Errorf("the List rows %s as\n%+v\nwant it as its project's SESSIONS rows it,\n%+v", s.Session, s, rowed[s.Session])
		}
	}

	asking := views.SessionList{Sessions: relisted(in.List.Sessions, stillRunning, views.Activity{Doing: views.Asking, Since: framesNow, LastSeen: framesNow})}
	if live, changed := page.Live(asking); changed || !reflect.DeepEqual(live.Rows, page.Rows) {
		t.Errorf("last week's page told afresh of a session asking now: changed %v, rows\n%+v\nwant it unchanged", changed, live.Rows)
	}
	if live, changed := log.Live(asking); changed || !reflect.DeepEqual(live.Sessions, log.Sessions) {
		t.Errorf("last week's List told afresh of a session asking now: changed %v, rows\n%+v\nwant it unchanged", changed, live.Sessions)
	}
}

// A session with requests in a period gone sits first in its List where it
// went on after its last day, as one running does at now: where it made a
// request on a day after it, today among them, or runs now; never where it
// sent only a token count after it, or nothing. Either way its requests and
// worth are the period's alone.
func TestASessionOfAPeriodGoneSitsFirstWhereItWentOnAfterIt(t *testing.T) {
	const dir = "/code/switchboard"
	tests := []struct {
		name    string
		after   []ledger.Held
		running bool
		wentOn  bool
	}{
		{name: "a request on a day after", after: projectAsks(wentOnLater, dir, dir, "work", september(29, 9, 0)), wentOn: true},
		{name: "a request today", after: projectAsks(wentOnLater, dir, dir, "work", october(1, 9, 0)), wentOn: true},
		{name: "running now", running: true, wentOn: true},
		{name: "a token count alone after", after: []ledger.Held{{Line: counted(wentOnLater, dir, september(29, 9, 0))}}},
		{name: "nothing after"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := byArrival(slices.Concat(
				projectAsks(wentOnLater, dir, dir, "work", september(26, 11, 0), september(26, 11, 1)),
				projectAsks(endedInside, dir, dir, "work", september(27, 15, 0)),
				tt.after,
			))
			var listed []views.ListedSession
			if tt.running {
				listed = append(listed, listedRunning(wentOnLater, dir, dir))
			}
			in := projectsIn(pastAt(t, views.LengthThisWeek, "<", time.Monday), framesNow, listed...)
			log := heldDays(framesNow, world...).SessionLog(in)
			i := slices.IndexFunc(log.Sessions, func(s views.PeriodSession) bool { return s.Session == wentOnLater })
			if i < 0 {
				t.Fatalf("the List rows %+v, want %s among them", log.Sessions, wentOnLater)
			}
			s := log.Sessions[i]
			wantAt, wantEnded := 1, "2026-09-26"
			if tt.wentOn {
				wantAt, wantEnded = 0, ""
			}
			if s.WentOn != tt.wentOn || i != wantAt || s.EndedOn != wantEnded || s.Running {
				t.Errorf("it went on %v, sits at %d, ended on %q, running %v; want went on %v, at %d, ended on %q, not running",
					s.WentOn, i, s.EndedOn, s.Running, tt.wentOn, wantAt, wantEnded)
			}
			if s.Requests != 2 || !reflect.DeepEqual(s.Worth, hoursWorth(2)) {
				t.Errorf("it made %d requests worth %+v, want the period's, 2 worth %+v", s.Requests, s.Worth, hoursWorth(2))
			}
		})
	}
}

// Sessions that went on after a period gone sit first by their start, the
// oldest first, however they ended in the period and whatever their ids,
// on the List and a project's SESSIONS alike.
func TestSessionsThatWentOnAfterAPeriodGoneSitByTheirStart(t *testing.T) {
	const dir = "/code/switchboard"
	const later, earlier = "b0000000-0000-4000-8000-000000000009", "b0000000-0000-4000-8000-000000000008"
	tests := []struct {
		name          string
		first, second []time.Time
	}{
		{
			name:  "the first started ended last, its id the higher",
			first: []time.Time{september(22, 10, 0), september(27, 10, 0)}, second: []time.Time{september(24, 10, 0), september(25, 10, 0)},
		},
		{
			name:  "the first started ended first, its id the higher",
			first: []time.Time{september(22, 10, 0), september(23, 10, 0)}, second: []time.Time{september(24, 10, 0), september(26, 10, 0)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := byArrival(slices.Concat(
				projectAsks(later, dir, dir, "work", append(tt.first, september(29, 10, 0))...),
				projectAsks(earlier, dir, dir, "work", append(tt.second, september(29, 11, 0))...),
			))
			in := projectsIn(pastAt(t, views.LengthThisWeek, "<", time.Monday), framesNow)
			days := heldDays(framesNow, world...)
			project, _ := days.Projects(in).Project(dir)
			for _, c := range []struct {
				of   string
				rows []views.PeriodSession
			}{{"the List", days.SessionLog(in).Sessions}, {"SESSIONS", project.Sessions}} {
				var ids []string
				for _, s := range c.rows {
					ids = append(ids, s.Session)
				}
				if want := []string{later, earlier}; !reflect.DeepEqual(ids, want) {
					t.Errorf("%s rows %v, want the one started first first, %v", c.of, ids, want)
				}
			}
		})
	}
}

// keepingLedger is a ledger of the lines given, as ledgerOfLines files them,
// keeping those of the day with the date from and after alone, as a ledger
// whose lines before it were pruned, as its FirstLinesDay tells, or failing
// to tell where err is given.
type keepingLedger struct {
	linesLedger
	from string
	err  error
}

func (l keepingLedger) Day(date string) []ledger.Held {
	if date < l.from {
		return nil
	}
	return l.linesLedger.Day(date)
}

func (l keepingLedger) FirstLinesDay(time.Time) (time.Time, error) {
	if l.err != nil {
		return time.Time{}, l.err
	}
	start, _, _ := dayfile.Day(l.from)
	return start, nil
}

// keptFrom is a ledger of the lines given keeping those from the 1st of
// September, as keepingLedger has it.
func keptFrom(lines ...ledger.Held) keepingLedger {
	return keepingLedger{linesLedger: ledgerOfLines(lines...), from: "2026-09-01"}
}

// A day gone is charted by the hour of its own fold, its hours not now's
// and none to come, on Projects and on Accounts alike, the fold chosen as
// HoursFold chooses it; given no fold, or another day's, today's among them,
// it's charted by the day.
func TestADayGoneIsChartedByItsOwnFoldsHoursAlone(t *testing.T) {
	const dir = "/code/switchboard"
	world := byArrival(slices.Concat(
		projectAsks(endedInside, dir, dir, "work", september(30, 22, 0)),
		projectAsks(todayOnly, dir, dir, "work", october(1, 9, 0)),
	))
	l, days := keptFrom(world...), heldDays(framesNow, world...)
	yesterday := pastAt(t, views.LengthToday, "<", time.Monday)
	hourly := []chartFrom{{views.ByHour, 24, "2026-09-30"}, {views.ByTwoHours, 12, "2026-09-30"}}
	daily := []chartFrom{{views.ByDay, 1, "2026-09-30"}}
	tests := []struct {
		name  string
		hours views.DayFold
		want  []chartFrom
	}{
		{"its own fold", views.ReadDayFold(l, "2026-09-30", framesNow, views.PoolRead{}), hourly},
		{"no fold", views.DayFold{}, daily},
		{"another day's fold", views.ReadDayFold(l, "2026-09-29", framesNow, views.PoolRead{}), daily},
		{"today's fold", days.TodayFold(), daily},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := projectsIn(yesterday, framesNow)
			in.Hours = tt.hours
			project, ok := days.Projects(in).Project(dir)
			if !ok {
				t.Fatalf("yesterday has no project %s", dir)
			}
			accountsIn := input(nil)
			accountsIn.Now, accountsIn.Accounts = framesNow, workAndSide
			accounts := views.NewPast(accountsIn, nil).Accounts(yesterday).Hourly(yesterday.HoursFold(framesNow, time.Monday, tt.hours, days.TodayFold()))
			for _, c := range []struct {
				of     string
				charts views.WorthCharts
			}{{"the project", project.WorthBy}, {"work", accounts.Rows[0].WorthBy}} {
				if got := chartsFrom(c.charts); !reflect.DeepEqual(got, tt.want) {
					t.Fatalf("%s's yesterday is charted %+v, want %+v", c.of, got, tt.want)
				}
				by := c.charts[0]
				if by.Now() != -1 || by.ToCome != 0 {
					t.Errorf("%s's yesterday has bucket %d now's and %d to come, want none", c.of, by.Now(), by.ToCome)
				}
				if by.Unit == views.ByHour && !reflect.DeepEqual(by.Buckets[22].Worth, hoursWorth(1)) {
					t.Errorf("%s's 22:00 is worth %+v, want its request's, %+v", c.of, by.Buckets[22].Worth, hoursWorth(1))
				}
			}
			var sum views.Worth
			for _, b := range project.WorthBy[0].Buckets {
				sum = sumOf(sum, b.Worth)
			}
			if !reflect.DeepEqual(sum, project.Worth) {
				t.Errorf("the project's buckets sum to %+v, want its worth, %+v", sum, project.Worth)
			}
		})
	}
}

// One choice of fold charts a day by the hour: at now, the tail's of today
// alone, a read of today never standing for it; of a day gone, the day's
// read, its hours known, else the tail's where it's of that day, as till
// the tail's first read after midnight; none where neither is that day's
// with its hours known, nor of a period of more than a day.
func TestOneChoiceOfFoldChartsADayByTheHour(t *testing.T) {
	const dir = "/code/switchboard"
	lines := byArrival(slices.Concat(
		projectAsks(endedInside, dir, dir, "work", september(27, 10, 0), september(30, 22, 0)),
		projectAsks(todayOnly, dir, dir, "work", october(1, 9, 0)),
	))
	l := keptFrom(lines...)
	read := func(date string) views.DayFold { return views.ReadDayFold(l, date, framesNow, views.PoolRead{}) }
	todays := heldDays(framesNow, slices.Concat(lines, projectAsks(todayOnly, dir, dir, "work", october(1, 13, 0)))...).TodayFold()
	yesterdays := heldDays(september(30, 23, 0), lines[:2]...).TodayFold()
	pruned := views.ReadDayFold(keepingLedger{linesLedger: ledgerOfLines(lines...), from: "2026-10-01"}, "2026-09-30", framesNow, views.PoolRead{})
	tests := []struct {
		name             string
		length           views.Length
		keys             string
		read, tail, want views.DayFold
	}{
		{name: "today at now, a read of today given", length: views.LengthToday, read: read("2026-10-01"), tail: todays, want: todays},
		{name: "today at now, the tail's of yesterday", length: views.LengthToday, read: read("2026-10-01"), tail: yesterdays},
		{name: "yesterday, its read", length: views.LengthToday, keys: "<", read: read("2026-09-30"), tail: todays, want: read("2026-09-30")},
		{name: "yesterday, no read, the tail's of yesterday", length: views.LengthToday, keys: "<", tail: yesterdays, want: yesterdays},
		{name: "yesterday, its lines no longer kept", length: views.LengthToday, keys: "<", read: pruned, tail: todays},
		{name: "last week, a read of its last day", length: views.LengthThisWeek, keys: "<", read: read("2026-09-27"), tail: todays},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pastAt(t, tt.length, tt.keys, time.Monday).HoursFold(framesNow, time.Monday, tt.read, tt.tail); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("the fold chosen is %+v, want %+v", got, tt.want)
			}
		})
	}
	in := projectsIn(views.LengthToday.Now(), framesNow)
	in.Hours = read("2026-10-01")
	days := heldDays(framesNow, slices.Concat(lines, projectAsks(todayOnly, dir, dir, "work", october(1, 13, 0)))...)
	project, _ := days.Projects(in).Project(dir)
	if got := project.WorthBy[0].Buckets[13].Worth; !reflect.DeepEqual(got, hoursWorth(1)) {
		t.Errorf("handed a read of today, today's 13:00 is worth %+v, want the tail's request's, %+v", got, hoursWorth(1))
	}
}

// A day gone's hours are known where the ledger keeps its lines, as where
// its lines begin says, a quiet day's too, which is charted by its empty
// hours; where it no longer keeps them, as where it keeps none at all, the
// day is charted by the day; where it can't tell where they begin, every
// day reads as kept.
func TestADayGonesHoursAreKnownWhereTheLedgerKeepsItsLines(t *testing.T) {
	const dir = "/code/switchboard"
	busy := projectAsks(endedInside, dir, dir, "work", september(29, 10, 0), september(30, 22, 0))
	quiet := busy[:1]
	tests := []struct {
		name   string
		ledger keepingLedger
		hourly bool
		worth  views.Worth
	}{
		{name: "kept, with a request", ledger: keptFrom(busy...), hourly: true, worth: hoursWorth(1)},
		{name: "kept and quiet", ledger: keptFrom(quiet...), hourly: true},
		{name: "no longer kept", ledger: keepingLedger{linesLedger: ledgerOfLines(busy...), from: "2026-10-01"}},
		{name: "no lines kept at all", ledger: keepingLedger{linesLedger: ledgerOfLines(), err: fmt.Errorf("no lines: %w", fs.ErrNotExist)}},
		{name: "where its lines begin can't be told", ledger: keepingLedger{linesLedger: ledgerOfLines(busy...), err: errors.New("unlistable")}, hourly: true, worth: hoursWorth(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yesterday := pastAt(t, views.LengthToday, "<", time.Monday)
			in := input(nil)
			in.Now, in.Accounts = framesNow, workAndSide
			fold := yesterday.HoursFold(framesNow, time.Monday, views.ReadDayFold(tt.ledger, "2026-09-30", framesNow, views.PoolRead{}), views.DayFold{})
			by := views.NewPast(in, nil).Accounts(yesterday).Hourly(fold).Rows[0].WorthBy[0]
			if got := by.Unit == views.ByHour; got != tt.hourly {
				t.Fatalf("yesterday is charted by %d, by the hour %v; want by the hour %v", by.Unit, got, tt.hourly)
			}
			if tt.hourly && !reflect.DeepEqual(by.Buckets[22].Worth, tt.worth) {
				t.Errorf("its 22:00 is worth %+v, want %+v", by.Buckets[22].Worth, tt.worth)
			}
		})
	}
}

// Where some of a period gone's days are no longer kept, its lines pruned,
// the first day whose sessions are kept is told, alike on Projects, a
// project's page and the List, and whether every day of the period from the
// day the work began is before it. Nothing is told at now, nor where the
// period's first day counted is kept, nor where the period lies before the
// work began, nor where when it began, or where the ledger's lines begin,
// isn't known; where the ledger keeps no lines at all, none is kept.
func TestTheDaysOfAPeriodGoneWhoseSessionsAreNoLongerKeptAreTold(t *testing.T) {
	const dir = "/code/switchboard"
	world := byArrival(slices.Concat(
		projectAsks(endedInside, dir, dir, "work", september(24, 10, 0)),
		projectAsks(wentOnLater, dir, dir, "work", september(29, 10, 0)),
	))
	tests := []struct {
		name, keys   string
		length       views.Length
		linesFrom    string
		err          error
		began        time.Time
		want         views.UnkeptDays
		wantSessions int
	}{
		{
			name: "last week, every day gone", length: views.LengthThisWeek, keys: "<", linesFrom: "2026-09-28", began: september(1, 0, 0),
			want: views.UnkeptDays{KeptFrom: "2026-09-28", Every: true},
		},
		{
			name: "30 days to 27 Sep, its first counted days gone", length: views.LengthThisWeek, keys: "<+", linesFrom: "2026-09-20", began: september(15, 9, 0),
			want: views.UnkeptDays{KeptFrom: "2026-09-20"}, wantSessions: 1,
		},
		{name: "last week, kept from its first day", length: views.LengthThisWeek, keys: "<", linesFrom: "2026-09-21", began: september(1, 0, 0), wantSessions: 1},
		{name: "the week of 7 Sep, before the work began", length: views.LengthThisWeek, keys: "<<<", linesFrom: "2026-09-20", began: september(15, 9, 0)},
		{name: "last week, the work's start unknown", length: views.LengthThisWeek, keys: "<", linesFrom: "2026-09-28"},
		{name: "12 months at now", length: views.LengthTwelveMonths, linesFrom: "2026-09-20", began: september(1, 0, 0), wantSessions: 2},
		{
			name: "last week, no lines kept at all", length: views.LengthThisWeek, keys: "<", linesFrom: "2026-10-02", err: fmt.Errorf("no lines: %w", fs.ErrNotExist),
			began: september(1, 0, 0), want: views.UnkeptDays{KeptFrom: "2026-10-01", Every: true},
		},
		{
			name: "last week, where the lines begin can't be told", length: views.LengthThisWeek, keys: "<", err: errors.New("unlistable"),
			began: september(1, 0, 0), wantSessions: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := keepingLedger{linesLedger: ledgerOfLines(world...), from: tt.linesFrom, err: tt.err}
			days := views.ReadSessionDays(l, framesNow.AddDate(0, -2, 0), framesNow, views.PoolRead{}).
				Tailed(l.Day(dayfile.DateOf(framesNow)), true, nil, nil, framesNow, views.PoolRead{})
			in := projectsIn(pastAt(t, tt.length, tt.keys, time.Monday), framesNow)
			in.Began = tt.began
			page, log := days.Projects(in), days.SessionLog(in)
			if page.Unkept != tt.want || log.Unkept != tt.want {
				t.Errorf("Projects tells %+v no longer kept, the List %+v; want %+v", page.Unkept, log.Unkept, tt.want)
			}
			if len(log.Sessions) != tt.wantSessions || page.All.Sessions != tt.wantSessions {
				t.Errorf("the List rows %d sessions, Projects %d; want %d", len(log.Sessions), page.All.Sessions, tt.wantSessions)
			}
			for _, r := range page.Rows {
				if project, _ := page.Project(r.Path); project.Unkept != tt.want {
					t.Errorf("%s's page tells %+v no longer kept, want %+v", r.Path, project.Unkept, tt.want)
				}
			}
		})
	}
}

// Where a day's sessions are read from its side but the ledger no longer
// keeps its lines, as where its lines begin says, its hours aren't known,
// so it's charted by the day, on Projects and on Accounts alike: one
// bucket, the day's worth. The ledger never keeps a day's side without its
// lines, but the rule holds of the views whatever it keeps.
func TestADayWhoseHoursAreGoneButWhoseSideIsKeptIsChartedByTheDay(t *testing.T) {
	const dir = "/code/switchboard"
	build := views.Digests{}.Day()
	for _, h := range projectAsks(endedInside, dir, dir, "work", september(30, 22, 0), september(30, 22, 1)) {
		build.Take(h.Line)
	}
	built, err := build.Built()
	if err != nil {
		t.Fatal(err)
	}
	l := sidedLedger{sides: map[string]json.RawMessage{"2026-09-30": built.Side}}
	days := views.ReadSessionDays(l, september(24, 0, 0), framesNow, views.PoolRead{})
	yesterday := pastAt(t, views.LengthToday, "<", time.Monday)
	in := projectsIn(yesterday, framesNow)
	in.Hours = views.ReadDayFold(l, "2026-09-30", framesNow, views.PoolRead{})
	project, ok := days.Projects(in).Project(dir)
	if !ok || project.Requests != 2 {
		t.Fatalf("yesterday's %s is %+v, want its side's two requests", dir, project.ProjectRow)
	}
	if got := chartsFrom(project.WorthBy); !reflect.DeepEqual(got, []chartFrom{{views.ByDay, 1, "2026-09-30"}}) ||
		!reflect.DeepEqual(project.WorthBy[0].Buckets[0].Worth, hoursWorth(2)) {
		t.Errorf("its hours gone, the project's yesterday is charted %+v, its day worth %+v; want by the day, worth %+v", got, project.WorthBy[0].Buckets[0].Worth, hoursWorth(2))
	}
	accountsIn := input(nil)
	accountsIn.Now, accountsIn.Accounts = framesNow, workAndSide
	byDay := views.NewPast(accountsIn, nil).Accounts(yesterday)
	if got := byDay.Hourly(yesterday.HoursFold(framesNow, time.Monday, in.Hours, days.TodayFold())); !reflect.DeepEqual(got.Rows[0].WorthBy, byDay.Rows[0].WorthBy) {
		t.Errorf("its hours gone, work's yesterday is charted %+v, want by the day, %+v", chartsFrom(got.Rows[0].WorthBy), chartsFrom(byDay.Rows[0].WorthBy))
	}
}
