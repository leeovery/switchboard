package router

import (
	"net/http"
	"reflect"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// sessionEnds is when work's session ends in these tests, and nextSession
// when the one after it does.
var (
	sessionEnds = start.Add(2 * time.Hour)
	nextSession = sessionEnds.Add(5 * time.Hour)
)

func TestACapIsToldAsTheReserveFirstHoldsAWindowBack(t *testing.T) {
	k := newTelling(t)
	k.reads(0.5, 0.5)
	k.expect()

	k.reads(0.95, 0.5)
	reached := status.Event{ID: 1, At: start, Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Until: sessionEnds, Reserve: 0.1}
	k.expect(reached)

	k.reads(0.97, 0.5)
	k.expect(reached)
}

func TestACapIsNoNewsAsTheAccountIsFirstRead(t *testing.T) {
	k := newTelling(t)
	k.look()
	k.reads(0.95, 0.5)
	k.reads(0.97, 0.5)
	k.expect()
}

func TestACapReachedAgainInAnotherWindowJoinsItWhileItHolds(t *testing.T) {
	k := newTelling(t)
	k.reads(0.5, 0.5)
	k.reads(0.95, 0.5)
	k.after(time.Minute)
	k.reads(0.95, 0.95)
	reached := status.Event{ID: 1, At: start, Kind: status.EventCap, Account: "work", Windows: []string{"5h", "7d"}, Until: weekEnds, Reserve: 0.1}
	k.expect(reached)

	// The session resets, and the next reaches its reserve, while the week
	// holds work back.
	k.clock.now = sessionEnds
	k.look()
	k.readsUntil(0.95, nextSession, 0.95)
	k.expect(reached)
}

func TestACapEndsAsItsWindowsResetAndIsToldAgainOnceReachedAgain(t *testing.T) {
	k := newTelling(t)
	k.reads(0.5, 0.5)
	k.reads(0.95, 0.5)
	k.clock.now = sessionEnds
	k.look()
	k.readsUntil(0.95, nextSession, 0.5)
	k.expect(
		status.Event{ID: 3, At: sessionEnds, Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Until: nextSession, Reserve: 0.1},
		status.Event{ID: 2, At: sessionEnds, Kind: status.EventRoom, Account: "work", Windows: []string{"5h"}},
		status.Event{ID: 1, At: start, Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Until: sessionEnds, Reserve: 0.1},
	)
}

func TestAnAccountTheGlobalPinNamesIsntCapped(t *testing.T) {
	k := newTelling(t)
	k.r.sessions.setPin(status.Pin{Accounts: []string{"work"}, Since: start}, false)
	k.reads(0.5, 0.5)
	k.reads(0.5, 0.95)
	k.expect()

	// Unpinned, its reserve holds it back.
	k.after(time.Minute)
	k.r.sessions.unpin(false)
	k.look()
	k.expect(status.Event{ID: 1, At: k.clock.now, Kind: status.EventCap, Account: "work", Windows: []string{"7d"}, Until: weekEnds, Reserve: 0.1})
}

func TestACapCountsTheSessionsItMoved(t *testing.T) {
	tests := []struct {
		name string
		// before are the moves heard as work reaches its reserve, before the
		// look that finds it; after, those heard after it.
		before, after []Moved
		wantCount     int
		wantTo        string
		// counted are the moves the cap counts, by their place, before's
		// first.
		counted []int
	}{
		{
			name:      "each session once, whatever its models",
			after:     []Moved{cappedOff("one", opus, "side"), cappedOff("one", haiku, "side"), cappedOff("two", opus, "side")},
			wantCount: 2, wantTo: "side", counted: []int{0, 1, 2},
		},
		{
			name:      "to several accounts, naming none",
			after:     []Moved{cappedOff("one", opus, "side"), cappedOff("two", opus, "personal")},
			wantCount: 2, counted: []int{0, 1},
		},
		{
			name:      "those heard before it's told",
			before:    []Moved{cappedOff("one", opus, "side")},
			after:     []Moved{cappedOff("two", opus, "side")},
			wantCount: 2, wantTo: "side", counted: []int{0, 1},
		},
		{
			name:  "but a move by choice",
			after: []Moved{{Session: "one", Model: opus, From: "work", To: "side", Reason: "moved by pin"}},
		},
		{
			name:  "but a move off another account at its reserve",
			after: []Moved{{Session: "one", Model: opus, From: "side", To: "work", Reason: "moved: side is at its reserve", Held: HeldByReserve}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := newTelling(t)
			k.reads(0.5, 0.5)
			k.record(0.95, sessionEnds, 0.5)
			k.moved(tt.before...)
			k.look()
			k.moved(tt.after...)

			got := k.r.events()
			reached := got[len(got)-1-len(tt.before)]
			want := status.Event{ID: 1 + len(tt.before), At: start, Kind: status.EventCap, Account: "work", To: tt.wantTo, Windows: []string{"5h"}, Until: sessionEnds, Count: tt.wantCount, Reserve: 0.1}
			if !reflect.DeepEqual(reached, want) {
				t.Errorf("the cap's event = %+v, want %+v", reached, want)
			}
			for i, m := range slices.Concat(tt.before, tt.after) {
				move := k.move(m)
				if wantBy := forcedByIf(slices.Contains(tt.counted, i), reached.ID); move.ForcedBy != wantBy || move.Limit != 0 {
					t.Errorf("move %d's event = %+v, want it forced by %d, and naming no limit", i+1, move, wantBy)
				}
			}
		})
	}
}

func TestAMoveWaitsForItsCapNoLongerThanTheLookAfterIt(t *testing.T) {
	k := newTelling(t)
	// Work is at its reserve as it's first read, which is no news, so a
	// session's move off it is counted in no cap: not in the one told once
	// work's next session reaches its reserve.
	k.reads(0.95, 0.5)
	k.hear(cappedOff("one", opus, "side"))
	k.look()
	k.clock.now = sessionEnds
	k.look()
	k.readsUntil(0.95, nextSession, 0.5)

	got := k.r.events()
	if got[0].Kind != status.EventCap || got[0].Count != 0 || k.move(cappedOff("one", opus, "side")).ForcedBy != 0 {
		t.Errorf("events() = %+v, want the move counted in no cap", got)
	}
}

func TestARefusalRenewedWhileItHoldsJoinsItsEvent(t *testing.T) {
	first := Refused{Account: "work", Status: http.StatusForbidden, Family: "opus", Request: someRequest}
	eightDays := events.ChangesFor * 24 * time.Hour
	tests := []struct {
		name string
		// holds is how long the first refusal holds from start, as renewals
		// since would have it; lifted is set where it lifts early. after is
		// how long after start the second comes, refusing another request
		// as again has it.
		holds  time.Duration
		lifted bool
		after  time.Duration
		again  func(r Refused) Refused
		// joins is set where the second joins the first's event.
		joins bool
	}{
		{name: "of the same account, status and family, while it holds", holds: refusedFor, after: time.Minute, again: same, joins: true},
		{name: "eight days after its event began", holds: eightDays + refusedFor, after: eightDays, again: same, joins: true},
		{name: "but more than eight days after, though it holds", holds: eightDays + refusedFor, after: eightDays + time.Minute, again: same},
		{name: "but once it has ended", holds: refusedFor, after: refusedFor, again: same},
		{name: "but once it has lifted early", holds: refusedFor, lifted: true, after: time.Minute, again: same},
		{name: "but of another family", holds: refusedFor, after: time.Minute, again: func(r Refused) Refused { r.Family = "haiku"; return r }},
		{name: "but of the token", holds: refusedFor, after: time.Minute, again: func(r Refused) Refused { r.Status, r.Family = http.StatusUnauthorized, ""; return r }},
		{name: "but of another account", holds: refusedFor, after: time.Minute, again: func(r Refused) Refused { r.Account = "side"; return r }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := newTelling(t)
			held := first
			held.Until = start.Add(tt.holds)
			k.hear(held)
			if tt.lifted {
				k.hear(RefusalLifted{Account: "work", Family: "opus", Request: someRequest})
			}
			k.clock.now = start.Add(tt.after)
			again := tt.again(first)
			again.Request, again.Until = "e5f6a7b8", k.clock.now.Add(refusedFor)
			k.hear(again)

			got := k.r.events()
			if !tt.joins {
				if len(got) != 2 || got[0].ID != 2 || got[0].Kind != status.EventRefused || !got[0].At.Equal(k.clock.now) {
					t.Errorf("events() = %+v, want the second refusal an event of its own", got)
				}
				return
			}
			want := status.Event{ID: 1, At: start, Kind: status.EventRefused, Account: "work", Until: again.Until, Status: first.Status, Family: first.Family}
			if !reflect.DeepEqual(got, []status.Event{want}) {
				t.Errorf("events() = %+v, want the first's event alone, holding till the second's end: %+v", got, want)
			}
		})
	}
}

func TestARefusalCountsTheSessionsItMoved(t *testing.T) {
	until := start.Add(refusedFor)
	token := Refused{Account: "work", Status: http.StatusUnauthorized, Until: until, Request: someRequest}
	opusAlone := Refused{Account: "work", Status: http.StatusForbidden, Family: "opus", Until: until, Request: someRequest}
	tests := []struct {
		name  string
		heard []Event
		// forcedBy is the id of the refusal's event each move heard is
		// counted in, 0 for none, which counts them as wantCount and wantTo
		// say.
		forcedBy  int
		wantCount int
		wantTo    string
	}{
		{
			name:     "each session once, of any model, its token's",
			heard:    []Event{token, refusedOff("one", opus, "side"), refusedOff("one", haiku, "side"), refusedOff("two", haiku, "side")},
			forcedBy: 1, wantCount: 2, wantTo: "side",
		},
		{
			name:     "to several accounts, naming none",
			heard:    []Event{opusAlone, refusedOff("one", opus, "side"), refusedOff("two", opus, "personal")},
			forcedBy: 1, wantCount: 2,
		},
		{
			name:     "heard before it's told",
			heard:    []Event{refusedOff("one", opus, "side"), opusAlone},
			forcedBy: 2, wantCount: 1, wantTo: "side",
		},
		{
			name:     "its token's, while its family's holds too",
			heard:    []Event{opusAlone, token, refusedOff("one", opus, "side")},
			forcedBy: 2, wantCount: 1, wantTo: "side",
		},
		{
			name:  "but of another family",
			heard: []Event{opusAlone, refusedOff("one", haiku, "side")},
		},
		{
			name:  "but of another family, heard before it's told",
			heard: []Event{refusedOff("one", haiku, "side"), opusAlone},
		},
		{
			name:  "but by choice",
			heard: []Event{token, Moved{Session: "one", Model: opus, From: "work", To: "side", Reason: "moved by pin"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := newTelling(t)
			k.hear(tt.heard...)

			for _, e := range k.r.events() {
				switch {
				case e.Kind == status.EventMoved && e.ForcedBy != tt.forcedBy:
					t.Errorf("the move %+v is forced by %d, want %d", e, e.ForcedBy, tt.forcedBy)
				case e.Kind == status.EventRefused && e.ID == tt.forcedBy && (e.Count != tt.wantCount || e.To != tt.wantTo):
					t.Errorf("the refusal's event = %+v, want it counting %d moved to %q", e, tt.wantCount, tt.wantTo)
				case e.Kind == status.EventRefused && e.ID != tt.forcedBy && e.Count != 0:
					t.Errorf("the refusal's event = %+v, want it counting none", e)
				}
			}
		})
	}
}

func TestAMoveNudgesTheLookThatFindsItsCap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reserving := slices.Clone(testConfigured)
		reserving[0].Reserve = 0.1
		h := newRecordingOf(t, reserving)
		h.state.record("work", []quota.Window{h.session(0.5, 2*time.Hour), h.week(0.5)}, h.state.mark())
		h.start()

		// Work's answer to a session's request reads its session at its
		// reserve, so the session's next request moves it to side.
		h.state.record("work", []quota.Window{h.session(0.95, 2*time.Hour), h.week(0.5)}, h.state.mark())
		h.r.hear(cappedOff("one", opus, "side"))
		synctest.Wait()
		h.expect(
			status.Event{ID: 2, At: h.began, Kind: status.EventCap, Account: "work", To: "side", Windows: []string{"5h"}, Until: h.began.Add(2 * time.Hour), Count: 1, Reserve: 0.1},
			status.Event{ID: 1, At: h.began, Kind: status.EventMoved, Session: "one", Model: opus, From: "work", To: "side", Reason: "moved: work is at its reserve", ForcedBy: 2},
		)
	})
}

func TestAMoveNamesTheEventItsCountedIn(t *testing.T) {
	limit := LimitReached{Account: "work", Windows: []string{"5h"}, Until: sessionEnds, Limit: 1}
	tests := []struct {
		name string
		// force forces work's sessions off it, and moved is one's move.
		force func(k *telling)
		moved Moved
		// wantBy is the id of the event the move is counted in, 0 for none;
		// wantLimit is set where it names it as its limit too.
		wantBy    int
		wantLimit bool
	}{
		{
			name:   "a limit's",
			force:  func(k *telling) { k.hear(limit) },
			moved:  forced("one", "work", "side"),
			wantBy: 1, wantLimit: true,
		},
		{
			name:   "a cap's",
			force:  func(k *telling) { k.reads(0.5, 0.5); k.reads(0.95, 0.5) },
			moved:  cappedOff("one", opus, "side"),
			wantBy: 1,
		},
		{
			name: "a refusal's",
			force: func(k *telling) {
				k.hear(Refused{Account: "work", Status: http.StatusUnauthorized, Until: start.Add(refusedFor)})
			},
			moved:  refusedOff("one", opus, "side"),
			wantBy: 1,
		},
		{
			name:  "none, for a move by choice",
			force: func(k *telling) { k.hear(limit) },
			moved: Moved{Session: "one", Model: opus, From: "work", To: "side", Reason: "rescored after 2h idle"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := newTelling(t)
			tt.force(k)
			k.moved(tt.moved)

			move := k.move(tt.moved)
			wantLimit := forcedByIf(tt.wantLimit, tt.wantBy)
			if move.ForcedBy != tt.wantBy || move.Limit != wantLimit {
				t.Errorf("the move's event = %+v, want it forced by %d, naming the limit %d", move, tt.wantBy, wantLimit)
			}
		})
	}
}

func TestRoomAgainSaysWhatHeldTheAccountBack(t *testing.T) {
	tests := []struct {
		name string
		// hold holds work back, as it's looked at, and free has it room
		// again, as it's looked at once more.
		hold, free  func(k *telling)
		wantWindows []string
	}{
		{
			name:        "a limit, in the windows it was reached in",
			hold:        func(k *telling) { k.r.state.limit("work", []string{"5h"}, start.Add(time.Minute), k.r.state.mark()) },
			free:        func(k *telling) { k.after(time.Minute) },
			wantWindows: []string{"5h"},
		},
		{
			name: "a limit, naming none",
			hold: func(k *telling) { k.r.state.limit("work", nil, start.Add(time.Minute), k.r.state.mark()) },
			free: func(k *telling) { k.after(time.Minute) },
		},
		{
			name:        "a cap",
			hold:        func(k *telling) { k.record(0.95, sessionEnds, 0.5) },
			free:        func(k *telling) { k.clock.now = sessionEnds },
			wantWindows: []string{"5h"},
		},
		{
			name: "a limit, then a cap in another window as it holds",
			hold: func(k *telling) {
				k.r.state.limit("work", []string{"5h"}, sessionEnds, k.r.state.mark())
				k.look()
				k.record(1, sessionEnds, 0.95)
				k.look()
				// The limit lifts, and the cap holds work back.
				k.clock.now = sessionEnds
			},
			free:        func(k *telling) { k.clock.now = weekEnds },
			wantWindows: []string{"5h", "7d"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := newTelling(t)
			k.reads(0.5, 0.5)
			tt.hold(k)
			k.look()
			tt.free(k)
			k.look()

			got := k.r.events()
			want := status.Event{ID: got[0].ID, At: k.clock.now, Kind: status.EventRoom, Account: "work", Windows: tt.wantWindows}
			if !reflect.DeepEqual(got[0], want) {
				t.Errorf("events() = %+v, want the newest %+v", got, want)
			}
		})
	}
}

// forcedByIf is id when counted is set, else 0.
func forcedByIf(counted bool, id int) int {
	if counted {
		return id
	}
	return 0
}

// same is r, as it comes again.
func same(r Refused) Refused {
	return r
}

// cappedOff is a session's requests of model moving off work, at its
// reserve, to the account with the id to.
func cappedOff(session, model, to string) Moved {
	return Moved{Session: session, Model: model, From: "work", To: to, Reason: "moved: work is at its reserve", Held: HeldByReserve}
}

// refusedOff is a session's requests of model moving off work, which refused
// them, to the account with the id to.
func refusedOff(session, model, to string) Moved {
	return Moved{Session: session, Model: model, From: "work", To: to, Reason: "moved: work was refused", Held: HeldByRefusal}
}

// weekEnds is when work's week ends in these tests.
var weekEnds = start.Add(3 * day)

// telling is the store of a router's events over testAccounts, work keeping a
// tenth of every window back as its reserve, on a clock of the test's, which
// starts at start, filing the events.
type telling struct {
	t     *testing.T
	clock *testClock
	r     *recent
	files *eventFiles
}

// newTelling returns a store of the test's, which checks as the test ends
// that the files read back the events as they were last kept.
func newTelling(t *testing.T) *telling {
	t.Helper()
	clock := &testClock{now: start}
	reserving := slices.Clone(testConfigured)
	reserving[0].Reserve = 0.1
	s := newState(resolve(reserving, testTokens.Read), testPolicy, claude.Provider{}.Family, clock.read, unkept, unkept)
	k := &telling{t: t, clock: clock, r: newRecent(s, newSessions(clock.read, unkept, unkept), clock.read)}
	k.files = fileEvents(t, k.r)
	t.Cleanup(func() {
		if got, want := k.files.read(), asFiled(k.r.events()); !reflect.DeepEqual(got, want) {
			t.Errorf("the files read back\n%+v\nwant\n%+v: each event as it was last kept", got, want)
		}
	})
	return k
}

// reads has work read with its session and its week used as given, the
// session ending at sessionEnds, and looks at the accounts.
func (k *telling) reads(session, week float64) {
	k.readsUntil(session, sessionEnds, week)
}

// readsUntil has work read with its session used as given, ending at ends,
// and its week used as given, and looks at the accounts.
func (k *telling) readsUntil(session float64, ends time.Time, week float64) {
	k.record(session, ends, week)
	k.look()
}

// record has work read with its session used as given, ending at ends, and
// its week used as given.
func (k *telling) record(session float64, ends time.Time, week float64) {
	k.r.state.record("work", []quota.Window{
		{Key: "5h", Label: "Session", Utilization: session, ResetsAt: ends, Status: quota.StatusAllowed},
		{Key: "7d", Label: "Week", Utilization: week, ResetsAt: weekEnds, Status: quota.StatusAllowed},
	}, k.r.state.mark())
}

// look has the store look at the accounts.
func (k *telling) look() {
	k.r.look()
}

// after lets d pass.
func (k *telling) after(d time.Duration) {
	k.clock.now = k.clock.now.Add(d)
}

// hear has the store hear each event.
func (k *telling) hear(heard ...Event) {
	for _, e := range heard {
		k.r.hear(e)
	}
}

// moved has the store hear each move.
func (k *telling) moved(moves ...Moved) {
	for _, m := range moves {
		k.r.hear(m)
	}
}

// expect checks the events kept, the newest first.
func (k *telling) expect(want ...status.Event) {
	k.t.Helper()
	if got := k.r.events(); !reflect.DeepEqual(got, want) {
		k.t.Errorf("events() =\n%+v\nwant\n%+v", got, want)
	}
}

// move returns the event of the move m, the newest of its session and model.
func (k *telling) move(m Moved) status.Event {
	k.t.Helper()
	for _, e := range k.r.events() {
		if e.Kind == status.EventMoved && e.Session == m.Session && e.Model == m.Model && e.From == m.From {
			return e
		}
	}
	k.t.Fatalf("no move of %s's %s is kept", m.Session, m.Model)
	return status.Event{}
}
