package views_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/views"
)

// sameUnkept reports whether a and b tell alike of the days no longer kept.
func sameUnkept(a, b views.UnkeptDays) bool {
	return a.KeptFrom.Equal(b.KeptFrom) && a.Every == b.Every
}

// A period of a single day is that day, today's at now and one gone a step
// back, by its date; a longer one isn't one day, gone or not.
func TestAPeriodOfADayIsThatDay(t *testing.T) {
	tests := []struct {
		length     views.Length
		keys, date string
		gone, ok   bool
	}{
		{length: views.LengthToday, date: "2026-10-01", ok: true},
		{length: views.LengthToday, keys: "<", date: "2026-09-30", gone: true, ok: true},
		{length: views.LengthToday, keys: "<<", date: "2026-09-29", gone: true, ok: true},
		{length: views.LengthThisWeek},
		{length: views.LengthThisWeek, keys: "<"},
		{length: views.LengthThirtyDays, keys: "<"},
	}
	for _, tt := range tests {
		t.Run(tt.length.Name()+" "+tt.keys, func(t *testing.T) {
			date, gone, ok := pressed(t, tt.length, tt.keys, framesNow, time.Monday, time.Time{}).Day(framesNow, time.Monday)
			if date != tt.date || gone != tt.gone || ok != tt.ok {
				t.Errorf("its day is %q, gone %v, one day %v; want %q, gone %v, one day %v", date, gone, ok, tt.date, tt.gone, tt.ok)
			}
		})
	}
}

// A day before the work began is never charted by the hour, though its
// fold is given, on Projects and on Accounts alike, as a pool whose work
// began today shows yesterday: none of its hours are counted.
func TestADayBeforeTheWorkBeganIsNeverChartedByTheHour(t *testing.T) {
	const dir = "/code/switchboard"
	world := projectAsks(endedInside, dir, dir, "work", september(30, 22, 0))
	l, days := keptFrom(world...), heldDays(framesNow, world...)
	yesterday := pastAt(t, views.LengthToday, "<", time.Monday)
	fold := views.ReadDayFold(l, "2026-09-30", 0, framesNow, views.PoolRead{})
	in := projectsIn(yesterday, framesNow)
	in.Hours, in.Began = fold, framesNow
	project, ok := days.Projects(in).Project(dir)
	if !ok {
		t.Fatalf("yesterday has no project %s", dir)
	}
	accountsIn := input(nil)
	accountsIn.Now, accountsIn.Accounts, accountsIn.Began = framesNow, workAndSide, framesNow
	accounts := views.NewPast(accountsIn, nil).Accounts(yesterday).Hourly(yesterday.HoursFold(framesNow, time.Monday, fold, days.TodayFold()))
	for _, c := range []struct {
		of     string
		charts views.WorthCharts
	}{{"the project", project.WorthBy}, {"work", accounts.Rows[0].WorthBy}} {
		if len(c.charts) != 1 {
			t.Errorf("begun today, %s's yesterday is charted %d ways, want one", c.of, len(c.charts))
			continue
		}
		if c.charts[0].Unit != views.ByDay || len(c.charts[0].Buckets) != 0 {
			t.Errorf("begun today, %s's yesterday is charted by %d in %d buckets; want by the day, none", c.of, c.charts[0].Unit, len(c.charts[0].Buckets))
		}
	}
}

// Just after midnight, before the tail's next read, a day gone's sessions
// are the tail's fold of it, and so are its hours, though the day's read is
// given: a project's hours sum to its row's worth.
func TestADayJustAfterMidnightIsChartedByTheFoldItsSessionsAreOf(t *testing.T) {
	const dir = "/code/switchboard"
	early := projectAsks(endedInside, dir, dir, "work", september(30, 22, 0))
	late := projectAsks(endedInside, dir, dir, "work", september(30, 23, 30))
	days := heldDays(september(30, 23, 45), slices.Concat(early, late)...)
	in := projectsIn(pastAt(t, views.LengthToday, "<", time.Monday), framesNow)
	in.Hours = views.ReadDayFold(keptFrom(early...), "2026-09-30", 0, framesNow, views.PoolRead{})
	project, ok := days.Projects(in).Project(dir)
	if !ok || project.Requests != 2 {
		t.Fatalf("yesterday's %s is %+v, want the tail's two requests", dir, project.ProjectRow)
	}
	var sum views.Worth
	for _, b := range project.WorthBy[0].Buckets {
		sum = sumOf(sum, b.Worth)
	}
	if project.WorthBy[0].Unit != views.ByHour || !reflect.DeepEqual(sum, project.Worth) {
		t.Errorf("yesterday's hours, by %d, sum to %+v; want by the hour, its row's worth, %+v", project.WorthBy[0].Unit, sum, project.Worth)
	}
}

// The days whose lines the ledger no longer keeps are the Log's: the later
// of where its lines begin and the oldest its keep keeps, so a day the
// ledger still holds, past its keep, is no longer kept for the List,
// Projects and a day's hours as for the Log, none of its sessions counted.
func TestTheDaysNoLongerKeptAreTheLogs(t *testing.T) {
	const dir = "/code/switchboard"
	const keep = 5 * 24 * time.Hour
	world := byArrival(slices.Concat(
		projectAsks(endedInside, dir, dir, "work", september(24, 10, 0)),
		projectAsks(wentOnLater, dir, dir, "work", september(27, 10, 0)),
	))
	l := keptFrom(world...)
	days := views.ReadSessionDays(l, framesNow.AddDate(0, -1, 0), framesNow, views.PoolRead{})
	in := projectsIn(pastAt(t, views.LengthThisWeek, "<", time.Monday), framesNow)
	in.Began, in.Keep = september(1, 0, 0), keep
	oldest := views.OldestKept(september(1, 0, 0), keep, framesNow).Day.Date
	log := days.SessionLog(in)
	if want := (views.UnkeptDays{KeptFrom: civilDate(oldest)}); oldest != "2026-09-26" || !sameUnkept(log.Unkept, want) {
		t.Errorf("kept 5 days, the List tells %+v no longer kept, the Log reads back to %s; want both from 26 Sep", log.Unkept, oldest)
	}
	if len(log.Sessions) != 1 || log.Sessions[0].Session != wentOnLater {
		t.Errorf("kept 5 days, the List rows %+v; want the 27th's session alone", log.Sessions)
	}
	tests := []struct {
		keys, date string
		known      bool
	}{
		{"<<<<<<<", "2026-09-24", false},
		{"<<<<", "2026-09-27", true},
	}
	for _, tt := range tests {
		fold := views.ReadDayFold(l, tt.date, keep, framesNow, views.PoolRead{})
		chosen := pastAt(t, views.LengthToday, tt.keys, time.Monday).HoursFold(framesNow, time.Monday, fold, views.DayFold{})
		if got := !reflect.DeepEqual(chosen, views.DayFold{}); got != tt.known {
			t.Errorf("kept 5 days, %s's hours are known %v, want %v", tt.date, got, tt.known)
		}
	}
}

// The fold a day is charted by tells whether it's the ledger's tail's: the
// tail's wherever it's of the day, as yesterday's is till the tail's first
// read after midnight, and today's; else a read of a day gone, which isn't;
// none of a period of more than a day.
func TestTheFoldADayIsChartedByTellsWhetherItsTheTails(t *testing.T) {
	const dir = "/code/switchboard"
	l := keptFrom(projectAsks(endedInside, dir, dir, "work", september(30, 10, 0))...)
	read := views.ReadDayFold(l, "2026-09-30", 0, framesNow, views.PoolRead{})
	turnedFrom := views.ReadSessionDays(l, september(29, 0, 0), september(30, 11, 0), views.PoolRead{}).TodayFold()
	today := views.ReadSessionDays(l, september(29, 0, 0), framesNow, views.PoolRead{}).TodayFold()
	yesterday := pastAt(t, views.LengthToday, "<", time.Monday)
	tests := []struct {
		name            string
		period          views.Period
		tail            views.DayFold
		charted, tailed bool
	}{
		{name: "yesterday, the tail not read since", period: yesterday, tail: turnedFrom, charted: true, tailed: true},
		{name: "yesterday, the tail read today", period: yesterday, tail: today, charted: true},
		{name: "today", period: views.LengthToday.Now(), tail: today, charted: true, tailed: true},
		{name: "this week", period: views.LengthThisWeek.Now(), tail: today},
	}
	for _, tt := range tests {
		chosen := tt.period.HoursFold(framesNow, time.Monday, read, tt.tail)
		if charted := !reflect.DeepEqual(chosen, views.DayFold{}); charted != tt.charted || chosen.Tailed() != tt.tailed {
			t.Errorf("%s, charted by the hour %v, the tail's fold %v; want %v, %v", tt.name, charted, chosen.Tailed(), tt.charted, tt.tailed)
		}
	}
}

// A ledger kept as long as no keep says keeps its lines 400 days, as the
// router keeps them, not for good: a year gone tells the days before then
// as no longer kept, and counts none of their sessions; kept for good, it
// counts every session of it.
func TestAKeepOfNoneKeeps400DaysNotForever(t *testing.T) {
	const dir = "/code/switchboard"
	on := func(month time.Month, day int) time.Time { return time.Date(2025, month, day, 10, 0, 0, 0, time.Local) }
	world := byArrival(slices.Concat(
		projectAsks(endedInside, dir, dir, "work", on(time.August, 1)),
		projectAsks(wentOnLater, dir, dir, "work", on(time.September, 15)),
	))
	l := keepingLedger{linesLedger: ledgerOfLines(world...), from: "2025-07-01"}
	days := views.ReadSessionDays(l, framesNow.AddDate(-2, 0, 0), framesNow, views.PoolRead{})
	tests := []struct {
		name     string
		keep     time.Duration
		want     views.UnkeptDays
		sessions int
	}{
		{name: "no keep given", want: views.UnkeptDays{KeptFrom: civilDate("2025-08-27")}, sessions: 1},
		{name: "kept for good", keep: dayfile.Forever, sessions: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := projectsIn(pastAt(t, views.LengthTwelveMonths, "<", time.Monday), framesNow)
			in.Began, in.Keep = on(time.July, 1), tt.keep
			log := days.SessionLog(in)
			if !sameUnkept(log.Unkept, tt.want) || len(log.Sessions) != tt.sessions {
				t.Errorf("the year to 1 Oct 2025 tells %+v no longer kept and rows %d sessions; want %+v, %d", log.Unkept, len(log.Sessions), tt.want, tt.sessions)
			}
		})
	}
}

// A session counted of a day kept that began on a day held but no longer
// kept starts at its first request, wherever it fell, though none of that
// day's requests is counted.
func TestASessionBegunOnADayNoLongerKeptStartsAtItsFirstRequest(t *testing.T) {
	const dir = "/code/switchboard"
	world := projectAsks(wentOnLater, dir, dir, "work", september(24, 10, 0), september(27, 10, 0))
	days := views.ReadSessionDays(keptFrom(world...), framesNow.AddDate(0, -1, 0), framesNow, views.PoolRead{})
	in := projectsIn(pastAt(t, views.LengthThisWeek, "<", time.Monday), framesNow)
	in.Began, in.Keep = september(1, 0, 0), 5*24*time.Hour
	log := days.SessionLog(in)
	if want := (views.UnkeptDays{KeptFrom: civilDate("2026-09-26")}); !sameUnkept(log.Unkept, want) {
		t.Fatalf("kept 5 days, last week tells %+v no longer kept, want %+v", log.Unkept, want)
	}
	if len(log.Sessions) != 1 {
		t.Fatalf("kept 5 days, last week rows %d sessions, want one", len(log.Sessions))
	}
	if s, want := log.Sessions[0], september(24, 10, 0); s.Requests != 1 || !s.Started.Equal(want) {
		t.Errorf("kept 5 days, the session counts %d requests, started %s; want its 27th's alone, started %s", s.Requests, instantOf(s.Started), instantOf(want))
	}
}

// A read of the sessions' days made before the ledger held a line, held as
// the day turns, keeps the day it was read on as where the lines begin: the
// sessions the tail gained of it are counted a step back, after midnight,
// and none of its days is told as no longer kept.
func TestADayReadBeforeItsFirstLineIsKeptAfterTheDayTurns(t *testing.T) {
	const dir = "/code/switchboard"
	lines := projectAsks(endedInside, dir, dir, "work", september(30, 10, 0))
	days := views.ReadSessionDays(ledgerOfLines(), september(1, 0, 0), september(30, 9, 0), views.PoolRead{}).
		Tailed(lines, true, nil, nil, september(30, 11, 0), views.PoolRead{}).
		Tailed(nil, true, nil, nil, framesNow, views.PoolRead{})
	in := projectsIn(pastAt(t, views.LengthToday, "<", time.Monday), framesNow)
	in.Began = september(30, 10, 0)
	log := days.SessionLog(in)
	if !sameUnkept(log.Unkept, views.UnkeptDays{}) || len(log.Sessions) != 1 {
		t.Errorf("read before its first line, yesterday tells %+v no longer kept and rows %d sessions; want none told, one", log.Unkept, len(log.Sessions))
	}
}

// A past period's weeks are named by the last day of their last whole
// week, for Detail's words, "the eight weeks to Sun 27 Sep": its last
// week's, but where that's this week, still running, the week before's, as
// of a past day this week, Tue 29 Sep; none at now.
func TestAPastPeriodsWeeksRunToTheLastDayOfTheirLastWholeWeek(t *testing.T) {
	tests := []struct {
		name     string
		length   views.Length
		keys     string
		starts   time.Weekday
		wholeTo  string
		thisWeek bool
	}{
		{name: "this week at now", length: views.LengthThisWeek, starts: time.Monday, thisWeek: true},
		{name: "Tue 29 Sep", length: views.LengthToday, keys: "<<", starts: time.Monday, wholeTo: "2026-09-27", thisWeek: true},
		{name: "last week", length: views.LengthThisWeek, keys: "<", starts: time.Monday, wholeTo: "2026-09-27"},
		{name: "the week of 14 Sep", length: views.LengthThisWeek, keys: "<<", starts: time.Monday, wholeTo: "2026-09-20"},
		{name: "30 days to 1 Sep", length: views.LengthThirtyDays, keys: "<", starts: time.Monday, wholeTo: "2026-09-06"},
		{name: "last week from Sunday", length: views.LengthThisWeek, keys: "<", starts: time.Sunday, wholeTo: "2026-09-26"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(nil)
			in.Now, in.Accounts, in.WeekStarts = framesNow, workAndSide, tt.starts
			page := views.NewPast(in, nil).Accounts(pressed(t, tt.length, tt.keys, framesNow, tt.starts, time.Time{}))
			want := time.Time{}
			if tt.wholeTo != "" {
				want = civilDate(tt.wholeTo)
			}
			if !page.WholeTo.Equal(want) || page.ThisWeek != tt.thisWeek {
				t.Errorf("its weeks run to %v, this week %v; want %v, %v", page.WholeTo, page.ThisWeek, want, tt.thisWeek)
			}
		})
	}
}

// Detail's weeks are said by where they end: its rule, "the last eight
// weeks" where they end at this week, a past day of it as Tue 29 Sep among
// them, else by the last day of the last, "the eight weeks to Sun 27 Sep";
// and its facts, in every period gone, by the last day of the last whole
// week, "limit hit 3 of the 7 weeks to Sun 27 Sep", in full and short, as
// at now they're "of the last 7 weeks". A day in another year than now's
// carries its year.
func TestDetailsWeeksAreSaidByWhereTheyEnd(t *testing.T) {
	tests := []struct {
		name, keys        string
		length            views.Length
		at                time.Time
		rule, full, short string
	}{
		{name: "this week at now", length: views.LengthThisWeek, at: framesNow,
			rule: "the last eight weeks", full: "limit hit 3 of the last 7 weeks", short: "limit hit 3 of 7 weeks"},
		{name: "Tue 29 Sep", length: views.LengthToday, keys: "<<", at: framesNow,
			rule: "the last eight weeks", full: "limit hit 3 of the 7 weeks to Sun 27 Sep", short: "limit hit 3 of 7 weeks to Sun 27 Sep"},
		{name: "last week", length: views.LengthThisWeek, keys: "<", at: framesNow,
			rule: "the eight weeks to Sun 27 Sep", full: "limit hit 3 of the 7 weeks to Sun 27 Sep", short: "limit hit 3 of 7 weeks to Sun 27 Sep"},
		{name: "the week of 14 Sep", length: views.LengthThisWeek, keys: "<<", at: framesNow,
			rule: "the eight weeks to Sun 20 Sep", full: "limit hit 3 of the 7 weeks to Sun 20 Sep", short: "limit hit 3 of 7 weeks to Sun 20 Sep"},
		{name: "the week of 21 Dec, seen in the new year", length: views.LengthThisWeek, keys: "<<", at: time.Date(2027, time.January, 6, 9, 0, 0, 0, time.Local),
			rule: "the eight weeks to Sun 27 Dec 2026", full: "limit hit 3 of the 7 weeks to Sun 27 Dec 2026", short: "limit hit 3 of 7 weeks to Sun 27 Dec 2026"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(nil)
			in.Now, in.Accounts, in.WeekStarts = tt.at, workAndSide, time.Monday
			page := views.NewPast(in, nil).Accounts(pressed(t, tt.length, tt.keys, tt.at, time.Monday, time.Time{}))
			full, short := page.LimitHitOver(3, 7, tt.at)
			got := []string{page.WeeksShown(8, tt.at), full.String(), short.String()}
			if want := []string{tt.rule, tt.full, tt.short}; !slices.Equal(got, want) {
				t.Errorf("the weeks are said %q, want %q", got, want)
			}
		})
	}
}

// The rows that lead, running or gone on after a period gone, lead, as
// Leads tells the drawing; those that ended don't.
func TestTheRowsThatLeadAreTold(t *testing.T) {
	tests := []struct {
		name string
		row  views.PeriodSession
		want bool
	}{
		{"running", views.PeriodSession{Running: true}, true},
		{"gone on", views.PeriodSession{WentOn: true}, true},
		{"ended", views.PeriodSession{EndedOn: "2026-09-27"}, false},
	}
	for _, tt := range tests {
		if got := tt.row.Leads(); got != tt.want {
			t.Errorf("%s: leads %v, want %v", tt.name, got, tt.want)
		}
	}
}
