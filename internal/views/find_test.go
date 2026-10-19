package views_test

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// dayCounted is a ledger that counts the reads of each day's lines, by the
// day's date.
type dayCounted struct {
	views.Ledger
	read map[string]int
}

func (d dayCounted) Day(date string) []ledger.Held {
	d.read[date]++
	return d.Ledger.Day(date)
}

// walkBack goes back through the days as the watch's find does while it's
// open, from the days read before, held in src's Read: it reads, through l,
// the day the find names next, one at a time, holding each day's sessions
// for the run, until the find is done or it has read the days given, and
// gives what the find finds then.
func walkBack(t *testing.T, l views.Ledger, src views.FindSources, days int) views.Found {
	t.Helper()
	for range days {
		found := views.FindSessions(src)
		if found.Done() {
			return found
		}
		if _, held := src.Read[found.Next]; held {
			t.Fatalf("the find names %s to read next, read before", found.Next)
		}
		src.Read[found.Next] = views.DaySessions(views.RecordsOf(l.Day(found.Next)))
	}
	return views.FindSessions(src)
}

// foundOf is what the find found, as a moment wants it.
func foundOf(found views.Found) []viewstest.Found {
	var all []viewstest.Found
	for _, s := range found.Sessions {
		all = append(all, viewstest.Found{Session: s.ID, Dir: s.Dir, Date: s.Day.Date, Ended: s.Ended, Last: s.Last})
	}
	return all
}

// sameFound reports whether a and b are the same sessions found alike, in
// the same order.
func sameFound(a, b []viewstest.Found) bool {
	return slices.EqualFunc(a, b, func(x, y viewstest.Found) bool {
		return x.Session == y.Session && x.Dir == y.Dir && x.Date == y.Date && x.Ended == y.Ended && x.Last.Equal(y.Last)
	})
}

// oldestOf is the oldest day the ledger l keeps the lines of at now, kept
// as long as the router keeps them unless it's told, as the watch reads it.
func oldestOf(t *testing.T, l views.Ledger, now time.Time) views.OldestDay {
	t.Helper()
	first, err := l.FirstLinesDay(now)
	if err != nil {
		t.Fatal(err)
	}
	return views.OldestKept(first, 0, now)
}

// At each moment the find has to get right, it finds each session matching
// what's typed once, on its newest day, today's first, in the panel's order,
// then each day's back, newest first, having read each past day with
// sessions once.
func TestTheFindFindsEachSessionOnceOnItsNewestDay(t *testing.T) {
	for _, hm := range viewstest.FindMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			l := dayCounted{Ledger: ledgerHolding(hm.Now, hm.Lines...), read: make(map[string]int)}
			src := views.FindSources{
				Typed: hm.Typed, Today: panelOf(hm.Lines, hm.Sessions, false, views.Stream{}, nil, hm.Now),
				Read: make(map[string][]views.DaySession), Summaries: l.Days(time.Time{}), Oldest: oldestOf(t, l, hm.Now), Now: hm.Now,
			}

			found := walkBack(t, l, src, len(src.Summaries))
			if got := foundOf(found); !found.Done() || !sameFound(got, hm.Want) {
				t.Errorf("typed %q, the find, done %t, finds\n%+v\nwant\n%+v", hm.Typed, found.Done(), got, hm.Want)
			}
			for date, n := range l.read {
				if n != 1 {
					t.Errorf("the find read %s's lines %d times, want once", date, n)
				}
			}
		})
	}
}

// The cost tests' sessions: one of today's; one of yesterday's; one of a day
// summarised before version 2, whose summary names no session; and one of
// five days back, whose id starts 7f3a.
const (
	findToday   = "a1f0c3d2-0000-4000-8000-000000000001"
	findYester  = "b2e1d4c3-0000-4000-8000-000000000002"
	findUnnamed = "c3d2e5f4-0000-4000-8000-000000000003"
	findBack    = "7f3a0b1c-0000-4000-8000-000000000004"
)

// walkedLines are the lines of the find's cost tests: today's session in
// ~/Code/api, and yesterday's in ~/Code/web; two days back, a quota check
// alone; three days back a session in ~/Code/legacy; and five days back one
// in ~/Code/infra.
func walkedLines() []ledger.Line {
	check := line(findYester, haiku, "work", "new", october(5, 9, 0), "")
	check.Kind = ledger.KindCheck
	return []ledger.Line{
		in("~/Code/infra", line(findBack, opus, "work", "new", october(2, 9, 0), smallUsage)),
		in("~/Code/legacy", line(findUnnamed, opus, "work", "new", october(4, 9, 0), smallUsage)),
		check,
		in("~/Code/web", line(findYester, opus, "work", "new", october(6, 9, 0), smallUsage)),
		in("~/Code/api", line(findToday, opus, "work", "new", october(7, 9, 0), smallUsage)),
	}
}

// walkedLedger is the ledger of walkedLines at now, counting its days' reads,
// each day's summary naming its sessions but three days back's, summarised
// before version 2, which names none, and two days back's, which has none.
func walkedLedger() dayCounted {
	summaries := []ledger.Summary{
		naming("2026-10-02", findBack),
		{Version: 1, Day: "2026-10-04", Accounts: []ledger.AccountDay{{Account: "work", Sessions: 1}}},
		naming("2026-10-05"),
		naming("2026-10-06", findYester),
		naming("2026-10-07", findToday),
	}
	return dayCounted{Ledger: fakeLedger{lines: walkedLines(), summaries: summaries, now: now}, read: make(map[string]int)}
}

// walkedSources are what the find's cost tests find from, of l, what's typed
// given, no day read yet: today's session running, the oldest day kept the
// ledger's first.
func walkedSources(t *testing.T, l dayCounted, typed string) views.FindSources {
	t.Helper()
	running := []status.Session{{ID: findToday, Assignments: []status.Assignment{seatOn(opus, "opus", "work", status.ReasonNew, time.Hour, time.Minute, "")}}}
	return views.FindSources{
		Typed: typed, Today: panelOf(walkedLines(), running, false, views.Stream{}, nil, now),
		Read: make(map[string][]views.DaySession), Summaries: l.Days(time.Time{}), Oldest: oldestOf(t, l, now), Now: now,
	}
}

// foundIDs are the ids of the sessions found, in order.
func foundIDs(found views.Found) []string {
	var ids []string
	for _, s := range found.Sessions {
		ids = append(ids, s.ID)
	}
	return ids
}

// Today's sessions are found as the find opens, before any past day is read,
// and today's lines are never read by the find: they're the Log's own.
func TestTodaysMatchesReadNoPastDay(t *testing.T) {
	l := walkedLedger()
	src := walkedSources(t, l, "api")

	if got := views.FindSessions(src); !slices.Equal(foundIDs(got), []string{findToday}) || len(l.read) != 0 || got.Next != "2026-10-06" {
		t.Errorf("as it opens, the find finds %q, to read %q next, having read %v; want today's session, yesterday to read next, no day read",
			foundIDs(got), got.Next, l.read)
	}
	walkBack(t, l, src, len(src.Summaries))
	if n := l.read["2026-10-07"]; n != 0 {
		t.Errorf("the find read today's lines %d times, want none", n)
	}
}

// Each past day with sessions is read once a run, newest first, a find
// opened again going on from the day the last stopped at, and one opened
// once every day is read reading none; a day whose summary names no session,
// and might name none, is never read.
func TestEachPastDayIsReadOnceARun(t *testing.T) {
	l := walkedLedger()
	src := walkedSources(t, l, "api")

	walkBack(t, l, src, 1)
	if want := map[string]int{"2026-10-06": 1}; !maps.Equal(l.read, want) {
		t.Errorf("the first find, closed after a read, read %v, want %v, yesterday alone", l.read, want)
	}
	src.Typed = "web"
	if found := walkBack(t, l, src, len(src.Summaries)); !found.Done() {
		t.Errorf("the find opened again isn't done, to read %s next; want it done", found.Next)
	}
	if want := map[string]int{"2026-10-06": 1, "2026-10-04": 1, "2026-10-02": 1}; !maps.Equal(l.read, want) {
		t.Errorf("the finds read %v, want %v: each day with sessions once, and none of quota checks alone", l.read, want)
	}
	src.Typed = "7f3a"
	if found := views.FindSessions(src); !found.Done() || !slices.Equal(foundIDs(found), []string{findBack}) {
		t.Errorf("a find opened once every day's read finds %q, done %t; want %s, done", foundIDs(found), found.Done(), findBack[:4])
	}
}

// A session matching by its id on a day not read yet is found from the day's
// summary, with no line read, named by its id alone till its day is read.
func TestIDMatchesOnDaysNotReadYetComeFromTheSummaries(t *testing.T) {
	l := walkedLedger()
	src := walkedSources(t, l, "7F3A")

	found := views.FindSessions(src)
	if want := []viewstest.Found{{Session: findBack, Date: "2026-10-02", Ended: true}}; !sameFound(foundOf(found), want) || len(l.read) != 0 {
		t.Errorf("before any day is read, the find finds %+v, having read %v; want %+v, no day read", foundOf(found), l.read, want)
	}
	found = walkBack(t, l, src, len(src.Summaries))
	if want := []viewstest.Found{{Session: findBack, Dir: "~/Code/infra", Date: "2026-10-02", Ended: true, Last: october(2, 9, 0)}}; !sameFound(foundOf(found), want) {
		t.Errorf("once its day is read, the find finds %+v, want %+v", foundOf(found), want)
	}
}

// A day summarised before version 2 names none of its sessions, so a session
// of it is found by its id only once its lines are read.
func TestADaySummarisedBeforeVersion2IsFoundByItsLines(t *testing.T) {
	l := walkedLedger()
	src := walkedSources(t, l, "c3d2")

	if found := views.FindSessions(src); len(found.Sessions) != 0 {
		t.Errorf("before its day is read, the find finds %q, want none", foundIDs(found))
	}
	found := walkBack(t, l, src, len(src.Summaries))
	if want := []viewstest.Found{{Session: findUnnamed, Dir: "~/Code/legacy", Date: "2026-10-04", Ended: true, Last: october(4, 9, 0)}}; !sameFound(foundOf(found), want) || l.read["2026-10-04"] != 1 {
		t.Errorf("once its day is read, %d times, the find finds %+v, want %+v, its day read once", l.read["2026-10-04"], foundOf(found), want)
	}
}

// The find goes back through the days the ledger keeps the lines of, to the
// oldest, and no further, though their summaries are kept for good: a day
// past keeping is neither read nor judged by the sessions its summary names;
// and where the oldest day isn't known, it goes back to none.
func TestTheFindGoesBackNoFurtherThanTheOldestDayKept(t *testing.T) {
	keptFrom := func(day time.Time) views.OldestDay {
		return views.OldestDay{Day: views.Today(day), Keep: 400 * 24 * time.Hour}
	}
	tests := []struct {
		name   string
		oldest views.OldestDay
		found  []string
		read   map[string]int
	}{
		{name: "kept from the ledger's first day: every day", oldest: keptFrom(october(2, 0, 0)), found: []string{findBack}, read: map[string]int{"2026-10-06": 1, "2026-10-04": 1, "2026-10-02": 1}},
		{name: "kept from three days back: none before", oldest: keptFrom(october(4, 0, 0)), read: map[string]int{"2026-10-06": 1, "2026-10-04": 1}},
		{name: "kept from today: no past day", oldest: keptFrom(october(7, 0, 0)), read: map[string]int{}},
		{name: "the oldest day not known: no past day", read: map[string]int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := walkedLedger()
			src := walkedSources(t, l, "7f3a")
			src.Oldest = tt.oldest
			if found := views.FindSessions(src); !slices.Equal(foundIDs(found), tt.found) {
				t.Errorf("before any day is read, the find finds %q, want %q", foundIDs(found), tt.found)
			}
			found := walkBack(t, l, src, len(src.Summaries))
			if !found.Done() || !slices.Equal(foundIDs(found), tt.found) || !maps.Equal(l.read, tt.read) {
				t.Errorf("the find, done %t, finds %q, having read %v; want it done, finding %q, having read %v",
					found.Done(), foundIDs(found), l.read, tt.found, tt.read)
			}
		})
	}
}

// A session found by its id on a day's summary stays found once the day is
// read, where its lines don't name it, as where they're pruned, or its own
// is damaged: the day's summary still names it, and the day isn't read
// again.
func TestASessionFoundByItsIDOnItsDaysSummaryStaysFoundOnceItsDayIsRead(t *testing.T) {
	const named = "abcd1234-0000-4000-8000-000000000001"
	summaries := []ledger.Summary{naming("2026-10-03", named, older), naming("2026-10-07", mover)}
	tests := []struct {
		name string
		read []ledger.Held
	}{
		{name: "its day's lines pruned"},
		{name: "its own line damaged, another's read", read: heldOf(in("~/Code/web", line(older, opus, "work", "new", october(3, 9, 0), smallUsage)))},
	}
	want := []viewstest.Found{{Session: named, Date: "2026-10-03", Ended: true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := views.FindSources{
				Typed: "abcd", Read: make(map[string][]views.DaySession), Summaries: summaries,
				Oldest: views.OldestDay{Day: views.Today(october(1, 0, 0)), Keep: 400 * 24 * time.Hour}, Now: now,
			}
			if found := views.FindSessions(src); !sameFound(foundOf(found), want) || found.Next != "2026-10-03" {
				t.Fatalf("before its day is read, the find finds %+v, to read %q next; want %+v, its day next", foundOf(found), found.Next, want)
			}
			src.Read["2026-10-03"] = views.DaySessions(views.RecordsOf(tt.read))
			if found := views.FindSessions(src); !sameFound(foundOf(found), want) || !found.Done() {
				t.Errorf("once its day is read, the find finds %+v, to read %q next; want %+v, done", foundOf(found), found.Next, want)
			}
		})
	}
}

// A day's sessions are those of its lines but quota checks, as its summary
// names them, each once, the last to arrive first: its directory as those
// lines last named it, how many they are, and when the last came.
func TestADaysSessionsAreThoseItsSummaryNames(t *testing.T) {
	check := line(quiet, haiku, "work", "new", october(6, 8, 0), "")
	check.Kind = ledger.KindCheck
	count := line(early, opus, "work", "sticky", october(6, 11, 0), "")
	count.Kind = ledger.KindCount
	checkOfEarly := line(early, haiku, "work", "new", october(6, 7, 0), "")
	checkOfEarly.Kind = ledger.KindCheck
	day := []ledger.Held{
		{Line: checkOfEarly},
		{Line: check},
		{Line: in("~/Code/api", line(older, opus, "work", "new", october(6, 9, 0), smallUsage))},
		{Line: in("~/Code/web", line(early, opus, "work", "new", october(6, 10, 0), smallUsage))},
		{Line: line(older, opus, "work", "sticky", october(6, 10, 30), smallUsage)},
		{Line: line("", opus, "work", "new", october(6, 10, 45), smallUsage)},
		{Line: count},
		{Line: in("~/Code/api-v2", line(older, opus, "work", "sticky", october(6, 10, 50), smallUsage))},
	}
	got := views.DaySessions(views.RecordsOf(day))
	want := []views.DaySession{
		{ID: early, Dir: "~/Code/web", Lines: 2, Last: october(6, 11, 0)},
		{ID: older, Dir: "~/Code/api-v2", Lines: 3, Last: october(6, 10, 50)},
	}
	if !slices.EqualFunc(got, want, func(a, b views.DaySession) bool {
		return a.ID == b.ID && a.Dir == b.Dir && a.Lines == b.Lines && a.Last.Equal(b.Last)
	}) {
		t.Errorf("DaySessions() = %+v, want %+v", got, want)
	}
}
