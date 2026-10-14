package views_test

import (
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

// A held List reads the ledger once, as it's held: each look after, and
// each line of a session it holds that it takes in, builds of what it holds.
func TestAHeldListReadsTheLedgerOnceHoweverOftenItsLookedAt(t *testing.T) {
	counts := &readCounts{}
	l := countedLedger{counts: counts, fakeLedger: sessionsLedger([]ledger.Line{sessionAsked("a1", older, "work", "new", local(12, 0, 0))}, now)}
	held := views.HoldList(views.ReadList(l, nil), ledger.Pricing, now)
	for i := range 5 {
		held = held.Take("2026-10-07", heldOf(sessionAsked("a"+string(rune('2'+i)), older, "work", "sticky", local(12, 1+i, 0))))
		live := views.ListLive{Now: now.Add(time.Duration(i) * time.Second)}
		if list := held.List(live); len(list.Sessions) != 1 || list.Sessions[0].Requests != 2+i {
			t.Fatalf("look %d gave %+v; want older, with its %d requests", i, list.Sessions, 2+i)
		}
		if unfound := held.Unfound(live); unfound != nil {
			t.Fatalf("look %d leaves %v to read whole, want none, older read whole as it was held", i, unfound)
		}
	}
	if want := (readCounts{sessionsRead: 1}); *counts != want {
		t.Errorf("the held List read the ledger %+v, want it read once, as it was held", *counts)
	}
}

// A session the List held meets first, resumed after days without a request,
// is read whole once, and its start found on the day it began, as the List
// built afresh has it: whether its line today is the tail's first of it, or
// the router lists it asking before its line comes. Till then the List
// lists it as what it holds tells.
func TestASessionTheHeldListFirstMeetsIsReadWholeOnceItsStartFound(t *testing.T) {
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
			held := views.HoldList(views.ReadList(countedLedger{counts: counts, fakeLedger: sessionsLedger(lines[:1], local(12, 0, 0))}, nil), ledger.Pricing, local(12, 0, 0))
			held = held.Take("2026-10-07", heldOf(tt.tail...))
			live := views.ListLive{Running: tt.running, Now: now}
			if got := held.Unfound(live); !slices.Equal(got, []string{older}) {
				t.Fatalf("the held List leaves %v to read whole, want older", got)
			}
			l := countedLedger{counts: counts, fakeLedger: sessionsLedger(lines, now)}
			held = held.Found(views.ReadList(l, nil, held.Unfound(live)...))
			for range 3 {
				held = held.Take("2026-10-07", nil)
				if got := held.Unfound(live); got != nil {
					t.Fatalf("the held List leaves %v to read whole once read, want none", got)
				}
			}
			got := held.List(live)
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

// A List held first after midnight lists a session the router lists that
// has no line today, its start yesterday's, none of its requests today, as
// the List built afresh does.
func TestAHeldListListsASessionStartedYesterdayWithNoLineToday(t *testing.T) {
	night := []ledger.Line{sessionAsked("n1", older, "work", "new", october(6, 23, 30))}
	after := october(7, 0, 10)
	running := []status.Session{{ID: older, Assignments: []status.Assignment{{Model: opus, Account: "work", LastSeen: october(6, 23, 30)}}}}
	l := sessionsLedger(night, after)
	got := views.HoldList(views.ReadList(l, running), ledger.Pricing, after).List(views.ListLive{Running: running, Now: after})
	assertList(t, got, views.ListSessions(views.SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: after}))
	if len(got.Sessions) != 1 || !got.Sessions[0].Started.Equal(october(6, 23, 30)) || got.Sessions[0].Requests != 0 {
		t.Errorf("the List holds %+v, want older, started yesterday, with no requests today", got.Sessions)
	}
}

// At midnight a held List lets go of the sessions done an hour or more
// before it, as no List of the new day lists them; one asking again is read
// whole once more, its start found, while one that ran into the night is
// held still.
func TestAHeldListLetsGoAtMidnightOfTheSessionsDoneBeforeIt(t *testing.T) {
	yesterday := []ledger.Line{sessionAsked("e1", early, "work", "new", october(6, 9, 0)), sessionAsked("o1", older, "work", "new", october(6, 23, 30))}
	held := views.HoldList(views.ReadList(sessionsLedger(yesterday, october(6, 23, 40)), nil), ledger.Pricing, october(6, 23, 40))
	held = held.Take("2026-10-07", nil)
	today := []ledger.Line{sessionAsked("e2", early, "work", "sticky", october(7, 9, 0)), sessionAsked("o2", older, "work", "sticky", october(7, 9, 5))}
	held = held.Take("2026-10-07", heldOf(today...))
	live := views.ListLive{Now: now}
	if got := held.Unfound(live); !slices.Equal(got, []string{early}) {
		t.Fatalf("the held List leaves %v to read whole, want early alone, let go of at midnight", got)
	}
	l := sessionsLedger(slices.Concat(yesterday, today), now)
	held = held.Found(views.ReadList(l, nil, early))
	assertList(t, held.List(live), views.ListSessions(views.SessionSources{Ledger: l, Prices: ledger.Pricing, Now: now}))
}

// A held List taking more in, or rolling on, is never changed under a copy
// of it that its holder kept.
func TestAHeldListNeverChangesUnderItsHolder(t *testing.T) {
	held := views.HoldList(views.ReadList(sessionsLedger([]ledger.Line{sessionAsked("a1", older, "work", "new", local(12, 0, 0))}, now), nil), ledger.Pricing, now)
	live := views.ListLive{Now: now}
	before := held.List(live)
	held.Take("2026-10-07", heldOf(sessionAsked("a2", older, "work", "sticky", local(12, 5, 0)), sessionAsked("b1", early, "work", "new", local(12, 6, 0))))
	held.Take("2026-10-08", nil)
	held.Found(views.ReadList(sessionsLedger([]ledger.Line{sessionAsked("c1", mover, "work", "new", local(12, 7, 0))}, now), nil, mover))
	assertList(t, held.List(live), before)
}

// Of the days the ledger's tail followed no further, a held List reads again
// whole only the day it's held at and those after it, as it holds every line
// filed before it already.
func TestAHeldListReadsAgainTheDaysMissedFromTheDayItsHeldAt(t *testing.T) {
	night := local(23, 59, 50)
	held := views.HoldList(views.ReadList(sessionsLedger([]ledger.Line{sessionAsked("r1", older, "work", "new", local(23, 0, 0))}, night), nil), ledger.Pricing, night)
	missed := []string{"2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08"}
	tests := []struct {
		name   string
		held   views.HeldList
		unread []string
	}{
		{name: "held the day before last", held: held, unread: []string{"2026-10-07", "2026-10-08"}},
		{name: "rolled on to the day before", held: held.Take("2026-10-08", nil), unread: []string{"2026-10-08"}},
		{name: "rolled on past them", held: held.Take("2026-10-09", nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.held.Unread(missed); !slices.Equal(got, tt.unread) {
				t.Errorf("the List reads again whole the days %v of %v, want %v", got, missed, tt.unread)
			}
		})
	}
}

// A held List awaits a line on the day it's filed on, where the stream told
// its request ended, its line yet to come, before today, as a session's page
// does: of whichever session. It awaits none once the line is taken, nor for
// a request in flight, one lost, or one sent and arrived today.
func TestAHeldListAwaitsTheLineOfARequestThatEndedAcrossMidnight(t *testing.T) {
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
			held := views.HoldList(views.ReadList(sessionsLedger(first, night), nil), ledger.Pricing, night)
			if tt.taken {
				held = held.Take("2026-10-07", heldOf(lined))
			}
			if got := held.Awaited(views.ListLive{Stream: heard(tt.told), Now: morning}); !slices.Equal(got, tt.awaits) {
				t.Errorf("the List awaits lines on %v, want %v", got, tt.awaits)
			}
		})
	}
}
