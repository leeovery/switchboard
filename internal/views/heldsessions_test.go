package views_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// sessionAsked is the line of the request with the given id of the
// session's own conversation, of Claude Opus 5.5 on the account given, for
// the reason given, at at.
func sessionAsked(id, session, account, reason string, at time.Time) ledger.Line {
	return filedAs(id, took(10*time.Second, serving(p1, ledger.ClassMain, line(session, opus, account, reason, at, hourUsage))))
}

// listedAsking is the session with the given id as the router lists it,
// its Claude Opus 5.5 on the account given, asking since at.
func listedAsking(id, account string, at time.Time) status.Session {
	return status.Session{ID: id, Assignments: []status.Assignment{{Model: opus, Account: account, AssignedAt: at, LastSeen: at, InFlight: status.Asking}}}
}

// sessionsLedger is a ledger of the lines given, read at now, each day's
// summary naming the sessions of its lines.
func sessionsLedger(lines []ledger.Line, now time.Time) fakeLedger {
	l := summarisedLedger(lines)
	l.now = now
	return l
}

// listHeld is the store holding the List, as the watch's first read of it
// holds it: read through l at now, of the sessions running gives, as the
// router lists them.
func listHeld(l views.LedgerSessions, running []status.Session, now time.Time) views.HeldSessions {
	held, _ := views.HoldSessions(ledger.Pricing).Fed(l, views.Taken{}, views.ListWanted(running, views.Stream{}), now)
	return held
}

// fedList is held fed, at now, the lines today's gained, as a read of the
// ledger's tail gives them, through l, as the List wants them, of the
// sessions running gives.
func fedList(held views.HeldSessions, l views.LedgerSessions, running []status.Session, now time.Time, gained ...ledger.Line) views.HeldSessions {
	held, _ = held.Fed(l, views.Took(heldOf(gained...), nil), views.ListWanted(running, views.Stream{}), now)
	return held
}

// The store holding the List reads the ledger once, as it's first held:
// each feed after, and each line of a session it holds that it takes in,
// builds of what it holds.
func TestTheStoreReadsTheLedgerOnceHoweverOftenItsFed(t *testing.T) {
	counts := &readCounts{}
	l := countedLedger{counts: counts, fakeLedger: sessionsLedger([]ledger.Line{sessionAsked("a1", older, "work", "new", local(12, 0, 0))}, now)}
	held := listHeld(l, nil, now)
	for i := range 5 {
		at := now.Add(time.Duration(i) * time.Second)
		held = fedList(held, l, nil, at, sessionAsked("a"+string(rune('2'+i)), older, "work", "sticky", local(13, 1+i, 0)))
		if list := held.List(views.ListLive{Now: at}); len(list.Sessions) != 1 || list.Sessions[0].Requests != 2+i {
			t.Fatalf("look %d gave %+v; want older, with its %d requests", i, list.Sessions, 2+i)
		}
	}
	if want := (readCounts{sessionsRead: 1}); *counts != want {
		t.Errorf("the store read the ledger %+v, want it read once, as it was held", *counts)
	}
}

// A session the store holding the List meets first, resumed after days
// without a request, is read whole once, alone, and its start found on the
// day it began, as the List built afresh has it: whether its line today is
// the tail's first of it, or the router lists it asking before its line
// comes.
func TestASessionTheStoreFirstMeetsIsReadWholeOnceItsStartFound(t *testing.T) {
	began := sessionAsked("r1", older, "work", "new", october(4, 22, 0))
	again := sessionAsked("r2", older, "work", status.Rescored(62*time.Hour), local(12, 30, 0))
	tests := []struct {
		name    string
		tail    []ledger.Line
		running []status.Session
	}{
		{name: "its line today the tail's first of it", tail: []ledger.Line{again}},
		{name: "listed by the router asking, its line to come", running: []status.Session{listedAsking(older, "work", local(13, 11, 0))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counts := &readCounts{}
			lines := slices.Concat([]ledger.Line{began}, tt.tail)
			held := listHeld(countedLedger{counts: counts, fakeLedger: sessionsLedger(nil, local(12, 0, 0))}, nil, local(12, 0, 0))
			l := countedLedger{counts: counts, fakeLedger: sessionsLedger(lines, now)}
			held = fedList(held, l, tt.running, now, tt.tail...)
			for range 3 {
				held = fedList(held, l, tt.running, now)
			}
			got := held.List(views.ListLive{Running: tt.running, Now: now})
			assertList(t, got, views.ListSessions(views.SessionSources{Running: tt.running, Ledger: l.fakeLedger, Prices: ledger.Pricing, Now: now}))
			if len(got.Sessions) != 1 || !got.Sessions[0].Started.Equal(october(4, 22, 0)) {
				t.Errorf("the List holds %+v, want older, started on 4 October", got.Sessions)
			}
			if counts.sessionsRead != 2 {
				t.Errorf("the ledger was read whole %d times, want twice: as the List was held, and for older once", counts.sessionsRead)
			}
		})
	}
}

// A store holding the List first after midnight lists a session the router
// lists that has no line today, its start yesterday's, none of its requests
// today, as the List built afresh does.
func TestTheStoreListsASessionStartedYesterdayWithNoLineToday(t *testing.T) {
	night := []ledger.Line{sessionAsked("n1", older, "work", "new", october(6, 23, 30))}
	after := october(7, 0, 10)
	running := []status.Session{{ID: older, Assignments: []status.Assignment{{Model: opus, Account: "work", LastSeen: october(6, 23, 30)}}}}
	l := sessionsLedger(night, after)
	got := listHeld(l, running, after).List(views.ListLive{Running: running, Now: after})
	assertList(t, got, views.ListSessions(views.SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: after}))
	if len(got.Sessions) != 1 || !got.Sessions[0].Started.Equal(october(6, 23, 30)) || got.Sessions[0].Requests != 0 {
		t.Errorf("the List holds %+v, want older, started yesterday, with no requests today", got.Sessions)
	}
}

// At midnight the store holding the List lets go of the sessions with no
// request on the new day that don't run, as no List of it lists them; each
// asking again is read whole once more, its start found, and the List is
// the one built afresh.
func TestTheStoreLetsGoAtMidnightOfTheSessionsNoListWants(t *testing.T) {
	yesterday := []ledger.Line{sessionAsked("e1", early, "work", "new", october(6, 9, 0)), sessionAsked("o1", older, "work", "new", october(6, 23, 30))}
	running := []status.Session{{ID: older, Assignments: []status.Assignment{{Model: opus, Account: "work", LastSeen: october(6, 23, 30)}}}}
	held := listHeld(sessionsLedger(yesterday, october(6, 23, 40)), running, october(6, 23, 40))
	midnight := october(7, 0, 0)
	held = fedList(held, sessionsLedger(yesterday, midnight), running, midnight)
	if open := held.Holding().Open; len(open) != 1 || !hasKey(open, older) {
		t.Fatalf("past midnight the store holds %v, want older alone, which runs, early let go", open)
	}
	today := []ledger.Line{sessionAsked("e2", early, "work", "sticky", october(7, 9, 0)), sessionAsked("o2", older, "work", "sticky", october(7, 9, 5))}
	counts := &readCounts{}
	l := countedLedger{counts: counts, fakeLedger: sessionsLedger(slices.Concat(yesterday, today), now)}
	held = fedList(held, l, nil, now, today...)
	assertList(t, held.List(views.ListLive{Now: now}), views.ListSessions(views.SessionSources{Ledger: l.fakeLedger, Prices: ledger.Pricing, Now: now}))
	if counts.sessionsRead != 1 {
		t.Errorf("the ledger was read whole %d times, want once, for early, asking again", counts.sessionsRead)
	}
}

// A look past midnight before a read of the ledger's tail has fed the store
// lists as the List built afresh then does: none of the day before's
// requests counted today, the sessions running listed, those that aren't
// let go.
func TestALookPastMidnightBeforeAFeedListsTheNewDay(t *testing.T) {
	lines := []ledger.Line{sessionAsked("o1", older, "work", "new", october(6, 23, 30)), sessionAsked("e1", early, "work", "new", october(6, 23, 40))}
	running := []status.Session{{ID: older, Assignments: []status.Assignment{{Model: opus, Account: "work", LastSeen: october(6, 23, 30)}}}}
	held := listHeld(sessionsLedger(lines, october(6, 23, 50)), running, october(6, 23, 50))
	after := october(7, 0, 5)
	live := views.ListLive{Running: running, Now: after}
	got := held.List(live)
	assertList(t, got, views.ListSessions(views.SessionSources{Running: running, Ledger: sessionsLedger(lines, after), Prices: ledger.Pricing, Now: after}))
	if len(got.Sessions) != 1 || got.Sessions[0].Requests != 0 {
		t.Errorf("past midnight the List holds %+v, want older alone, running, with no requests today", got.Sessions)
	}
}

// A line without an id, as no router has written since requests had ids,
// can't be told from the same line given again, nor from another that
// arrived with it, so the store reads its session whole as it comes: given
// again, it's counted once, and one that arrived with it once more, as the
// List built afresh counts them.
func TestALineWithoutAnIDIsReadWhole(t *testing.T) {
	first := sessionAsked("a1", older, "work", "new", local(13, 0, 0))
	nameless := sessionAsked("", older, "work", "sticky", local(13, 5, 0))
	beside := took(time.Minute, nameless)
	l := sessionsLedger([]ledger.Line{first, nameless, beside}, now)
	held := listHeld(sessionsLedger([]ledger.Line{first}, now), nil, now)
	for _, gained := range []ledger.Line{nameless, nameless, beside} {
		held = fedList(held, l, nil, now, gained)
	}
	assertList(t, held.List(views.ListLive{Now: now}), views.ListSessions(views.SessionSources{Ledger: l, Prices: ledger.Pricing, Now: now}))
}

// A line of the same id that arrived at the same moment as one before it is
// the same request given again, as a ledger holding a line twice gives it:
// the store folds it once, read whole or fed, in one read or two, as the
// List built afresh does.
func TestALineGivenTwiceIsOneRequest(t *testing.T) {
	first := sessionAsked("a1", older, "work", "new", local(13, 0, 0))
	again := sessionAsked("a2", older, "work", "sticky", local(13, 5, 0))
	tests := []struct {
		name  string
		held  func() views.HeldSessions
		lines []ledger.Line
	}{
		{name: "read whole", lines: []ledger.Line{first, again, again}, held: func() views.HeldSessions {
			return listHeld(sessionsLedger([]ledger.Line{first, again, again}, now), nil, now)
		}},
		{name: "fed in one read", lines: []ledger.Line{first, again}, held: func() views.HeldSessions {
			l := sessionsLedger([]ledger.Line{first}, now)
			return fedList(listHeld(l, nil, now), l, nil, now, again, again)
		}},
		{name: "fed in two", lines: []ledger.Line{first, again}, held: func() views.HeldSessions {
			l := sessionsLedger([]ledger.Line{first}, now)
			return fedList(fedList(listHeld(l, nil, now), l, nil, now, again), l, nil, now, again)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.held().List(views.ListLive{Now: now})
			if len(got.Sessions) != 1 || got.Sessions[0].Requests != 2 {
				t.Errorf("the List holds %+v, want older with its two requests", got.Sessions)
			}
		})
	}
}

// hasKey reports whether m has the key k.
func hasKey[V any](m map[string]V, k string) bool {
	_, ok := m[k]
	return ok
}

// The store, fed more, rolled on or given another's sessions, is never
// changed under a copy of it that its holder kept.
func TestTheStoreNeverChangesUnderItsHolder(t *testing.T) {
	l := sessionsLedger([]ledger.Line{sessionAsked("a1", older, "work", "new", local(12, 0, 0))}, now)
	held := listHeld(l, nil, now)
	live := views.ListLive{Now: now}
	before, holding := held.List(live), held.Holding()
	fedList(held, l, nil, now, sessionAsked("a2", older, "work", "sticky", local(12, 5, 0)), sessionAsked("b1", early, "work", "new", local(12, 6, 0)))
	fedList(held, l, nil, october(8, 0, 1))
	held.With(listHeld(sessionsLedger([]ledger.Line{sessionAsked("c1", mover, "work", "new", local(12, 7, 0))}, now), nil, now))
	held.List(views.ListLive{Now: october(8, 0, 1)})
	assertList(t, held.List(live), before)
	if got := held.Holding(); !holdingSame(got, holding) {
		t.Errorf("the store holds %+v, want %+v, as it was held", got, holding)
	}
}

// holdingSame reports whether two Holdings are alike.
func holdingSame(a, b views.Holding) bool {
	return a.Day == b.Day && a.Keys == b.Keys && a.Gained == b.Gained && a.Folded == b.Folded && a.Read == b.Read && len(a.Open) == len(b.Open) &&
		!slices.ContainsFunc(keysOf(a.Open), func(id string) bool { return a.Open[id] != b.Open[id] })
}

// keysOf are the keys of m.
func keysOf[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Of the days the ledger's tail followed no further, the store reads again
// whole only the day it was last fed on and those after it, as it took in
// every line filed before it already.
func TestTheStoreReadsAgainTheDaysMissedFromTheDayItWasLastFedOn(t *testing.T) {
	night := local(23, 59, 50)
	l := sessionsLedger([]ledger.Line{sessionAsked("r1", older, "work", "new", local(23, 0, 0))}, night)
	held := listHeld(l, nil, night)
	missed := []string{"2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08"}
	tests := []struct {
		name   string
		held   views.HeldSessions
		unread []string
	}{
		{name: "fed the day before last", held: held, unread: []string{"2026-10-07", "2026-10-08"}},
		{name: "fed the day before", held: fedList(held, l, nil, october(8, 0, 5)), unread: []string{"2026-10-08"}},
		{name: "fed past them", held: fedList(held, l, nil, october(9, 0, 5))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.held.Unread(missed); !slices.Equal(got, tt.unread) {
				t.Errorf("the store reads again whole the days %v of %v, want %v", got, missed, tt.unread)
			}
		})
	}
}

// The store awaits a line on the day it's filed on, where the stream told
// its request ended, its line yet to come, before today, as a session's page
// does: of whichever session. It awaits none once the line is taken, nor for
// a request in flight, one lost, or one sent and arrived today.
func TestTheStoreAwaitsTheLineOfARequestThatEndedAcrossMidnight(t *testing.T) {
	night := local(23, 59, 50)
	morning := night.Add(10 * time.Minute)
	lined := filedAs("r2", took(22*time.Second, serving(p1, ledger.ClassMain, line(paged, opus, "side", "sticky", night, hourUsage))))
	sent := toldOf("r2", p1, ledger.ClassMain, opus, "side", night.Add(100*time.Millisecond))
	tests := []struct {
		name   string
		told   views.Told
		taken  bool
		awaits []string
	}{
		{name: "ended after midnight, its line to come", told: endedAt(night.Add(22*time.Second), sent), awaits: []string{"2026-10-07"}},
		{name: "ended, its line taken", told: endedAt(night.Add(22*time.Second), sent), taken: true},
		{name: "in flight", told: sent},
		{name: "lost", told: lostAt(night.Add(time.Minute), sent)},
		{name: "sent today", told: endedAt(morning.Add(time.Second), toldOf("r3", p1, ledger.ClassMain, opus, "side", morning))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := []ledger.Line{sessionAsked("r1", paged, "side", "new", local(23, 0, 0))}
			l := sessionsLedger(first, night)
			held := listHeld(l, nil, night)
			if tt.taken {
				held = fedList(held, l, nil, night, lined)
			}
			if got := held.Awaited(views.ListLive{Stream: heard(tt.told), Now: morning}); !slices.Equal(got, tt.awaits) {
				t.Errorf("the store awaits lines on %v, want %v", got, tt.awaits)
			}
		})
	}
}

// A store given the sessions of another's read, as a reader's own read holds
// them, is as last fed as the earlier of the two, whichever it is: across
// midnight, it reads the day before again whole, as the tail missed it, for
// the sessions held of the read on it, and takes in the line one of them
// gained on it after that read, as one read whole holds it.
func TestTheStoreGivenAnothersSessionsReadsAgainTheDaysEitherMissed(t *testing.T) {
	first := sessionAsked("o1", older, "work", "new", october(6, 23, 0))
	late := sessionAsked("o2", older, "work", "sticky", october(6, 23, 55))
	other := sessionAsked("e1", early, "work", "new", october(7, 0, 5))
	night, morning, now := october(6, 23, 50), october(7, 0, 10), october(7, 0, 11)
	atNight := listHeld(sessionsLedger([]ledger.Line{first}, night), nil, night)
	atMorning := listHeld(sessionsLedger([]ledger.Line{first, late, other}, morning), nil, morning)
	running := []status.Session{{ID: older, Assignments: []status.Assignment{{Model: opus, Account: "work", LastSeen: october(6, 23, 55)}}}}
	l := sessionsLedger([]ledger.Line{first, late, other}, now)
	tests := []struct {
		name string
		held views.HeldSessions
	}{
		{name: "fed the day before, given a read after midnight", held: atNight.With(atMorning)},
		{name: "fed after midnight, given a read the day before", held: atMorning.With(atNight)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unread := tt.held.Unread([]string{"2026-10-06"})
			if !slices.Equal(unread, []string{"2026-10-06"}) {
				t.Fatalf("the store reads again whole the days %v, want the day before, its sessions held of the read on it", unread)
			}
			days := make(map[string][]ledger.Held)
			for _, date := range unread {
				days[date] = l.Day(date)
			}
			fed, _ := tt.held.Fed(l, views.Took(nil, days), views.ListWanted(running, views.Stream{}), now)
			assertList(t, fed.List(views.ListLive{Running: running, Now: now}),
				views.ListSessions(views.SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: now}))
			if got, want := fed.Holding().Open, listHeld(l, running, now).Holding().Open; !maps.Equal(got, want) {
				t.Errorf("the store holds %v requests open, want %v, as one read whole holds them", got, want)
			}
		})
	}
}

// A store fed, then given the session of a page read after the clock was set
// back, knows the clock was set back at its next feed, as it would without
// the page's read: it holds none of what it held, so a request that arrived
// after the clock's now is listed by neither, as the List built afresh has
// it.
func TestTheStoreGivenAReadAfterTheClockWasSetBackKnowsItWas(t *testing.T) {
	ahead := sessionAsked("a1", mover, "work", "new", local(9, 50, 0))
	b1 := sessionAsked("b1", older, "work", "new", local(8, 40, 0))
	fed, back, next := local(10, 0, 0), local(9, 0, 0), local(9, 1, 0)
	held := listHeld(sessionsLedger([]ledger.Line{b1, ahead}, fed), nil, fed)
	l := sessionsLedger([]ledger.Line{b1}, next)
	held = held.With(views.ReadPage(sessionsLedger([]ledger.Line{b1}, back), older, views.Stream{}, ledger.Pricing, 0, back).Found())
	got := fedList(held, l, nil, next).List(views.ListLive{Now: next})
	assertList(t, got, views.ListSessions(views.SessionSources{Ledger: l, Prices: ledger.Pricing, Now: next}))
	if len(got.Sessions) != 1 || got.Sessions[0].Session != older {
		t.Errorf("the List holds %+v, want older alone, mover's request arriving after the clock's now", got.Sessions)
	}
}

// A store holding a session over two midnights keeps the keys, for lines
// given again, of its requests of yesterday and today alone: those of the
// day before yesterday it lets go of as the day turns.
func TestTheStoreKeepsTheKeysOfYesterdaysAndTodaysRequestsAlone(t *testing.T) {
	want := views.SessionsWanted{Log: []string{older}}
	var lines []ledger.Line
	held := views.HoldSessions(ledger.Pricing)
	for day := 5; day <= 7; day++ {
		gained := sessionAsked(fmt.Sprintf("o%d", day), older, "work", "sticky", october(day, 11, 0))
		lines = append(lines, gained)
		at := october(day, 12, 0)
		held, _ = held.Fed(sessionsLedger(lines, at), views.Took(heldOf(gained), nil), want, at)
	}
	if got := held.Holding(); got.Keys != 2 || got.Day != "2026-10-07" {
		t.Errorf("on %s the store holds the keys of %d requests, want 2 on 7 October, yesterday's and today's", got.Day, got.Keys)
	}
}

// A request the stream tells of in flight pins its session's horizon for up
// to pinnedFor, though the stream tells too of an older one, lost longer ago
// than that: the line of the one in flight, landing two hours on, is taken
// in, its session not read whole, as the List built afresh has it.
func TestARequestInFlightPinsItsSessionPastAnOlderOneLost(t *testing.T) {
	first := sessionAsked("p1", paged, "work", "new", local(10, 0, 0))
	sent := toldOf("p3", p1, ledger.ClassMain, opus, "work", local(11, 0, 0))
	lost := lostAt(local(6, 0, 30), toldOf("p2", p1, ledger.ClassMain, opus, "work", local(6, 0, 0)))
	long := took(2*time.Hour+10*time.Minute, sessionAsked("p3", paged, "work", "sticky", local(10, 59, 59)))
	inFlight := views.ListWanted(nil, heard(lost, sent))
	held, _ := views.HoldSessions(ledger.Pricing).Fed(sessionsLedger([]ledger.Line{first}, local(11, 30, 0)), views.Taken{}, inFlight, local(11, 30, 0))
	for _, at := range []time.Time{local(12, 0, 0), local(12, 30, 0), local(13, 0, 0)} {
		held, _ = held.Fed(sessionsLedger([]ledger.Line{first}, at), views.Taken{}, inFlight, at)
	}
	ended := heard(lost, endedAt(local(13, 9, 59), sent))
	l := sessionsLedger([]ledger.Line{first, long}, now)
	held, _ = held.Fed(l, views.Took(heldOf(long), nil), views.ListWanted(nil, ended), now)
	if got := held.Holding(); got.Read != 0 || got.Gained != 1 {
		t.Errorf("the feed of its line read %d requests whole and gained %d, want none read whole, the line gained", got.Read, got.Gained)
	}
	assertList(t, held.List(views.ListLive{Stream: ended, Now: now}), views.ListSessions(views.SessionSources{Stream: ended, Ledger: l, Prices: ledger.Pricing, Now: now}))
}
