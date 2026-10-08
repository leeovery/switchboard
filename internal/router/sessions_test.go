package router

import (
	"fmt"
	"maps"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
)

func TestRememberNotesARequestOnTheAssignmentItsChoiceFoundAlone(t *testing.T) {
	s := newSessions(at(start), unkept, unkept)
	req := Request{Session: "one", Model: opus}
	k := req.key()
	if _, noted := s.remember(req, assignment{}, decision{account: "work", reason: reasonNew}, start); !noted {
		t.Fatal("remember() didn't note a new session's first request")
	}
	onWork := s.lookup(k).current

	found, noted := s.remember(req, assignment{}, decision{account: "side", reason: reasonNew}, start.Add(time.Second))
	if noted || found != onWork {
		t.Errorf("remember() of another request that found the session new = %+v, noted %v, want work's assignment found, and the request not noted", found, noted)
	}
	stay := decision{account: "work", reason: reasonSticky, sticky: true}
	for _, at := range []time.Time{start.Add(time.Minute), start.Add(2 * time.Minute)} {
		if _, noted := s.remember(req, onWork, stay, at); !noted {
			t.Errorf("remember() of a request that stayed, at %v, didn't note it: staying leaves the assignment it found", at)
		}
	}
	if got := s.lookup(k).current; !got.same(onWork) || got.LastSeen != start.Add(2*time.Minute) {
		t.Errorf("the session is assigned %+v, want work's assignment, last seen when it last stayed", got)
	}
}

func TestRememberTellsAnAssignmentUsedAgainFromAChange(t *testing.T) {
	req := Request{Session: "one", Model: opus}
	stay := decision{account: "work", reason: reasonSticky, sticky: true}
	tests := []struct {
		name string
		// req goes where d says, a minute after req gave the session work.
		req                   Request
		d                     decision
		wantChanged, wantUsed changeCount
	}{
		{name: "used again as it was", req: req, d: stay, wantUsed: 1},
		{name: "moved", req: req, d: decision{account: "side", reason: "moved: work hit its limit"}, wantChanged: 1},
		{name: "its pin changed", req: Request{Session: "one", Model: opus, Pin: "work"}, d: stay, wantChanged: 1},
		{name: "chosen again, for another reason", req: req, d: decision{account: "work", reason: "rescored after 1h"}, wantChanged: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var changed, used changeCount
			s := newSessions(at(start), changed.hear, used.hear)
			assignFor(s, req, decision{account: "work", reason: reasonNew}, start)
			changed, used = 0, 0

			assignFor(s, tt.req, tt.d, start.Add(time.Minute))
			if changed != tt.wantChanged || used != tt.wantUsed {
				t.Errorf("told of %d changes and %d uses again, want %d and %d", changed, used, tt.wantChanged, tt.wantUsed)
			}
		})
	}
}

func TestForgetForgetsAnAssignmentWhileItsRequestWasTheLastNotedOnIt(t *testing.T) {
	req := Request{ID: "a1b2c3d4", Session: "one", Model: opus}
	other := Request{ID: "e5f6a7b8", Session: "one", Model: opus}
	moved := decision{account: "side", reason: "moved: work hit its limit"}
	stay := decision{account: "work", reason: reasonSticky, sticky: true}
	tests := []struct {
		name string
		// since is what befell the session after req gave it work.
		since      func(s *sessions)
		wantForgot bool
		// want is the session's account once it's done, "" for none.
		want string
	}{
		{name: "made by the request", since: func(*sessions) {}, wantForgot: true},
		{
			name:       "moved on by the request",
			since:      func(s *sessions) { assignFor(s, req, moved, start.Add(time.Second)) },
			wantForgot: true,
		},
		{
			name:  "moved by another request since",
			since: func(s *sessions) { assignFor(s, other, moved, start.Add(time.Second)) },
			want:  "side",
		},
		{
			name:  "stayed on by another request since",
			since: func(s *sessions) { assignFor(s, other, stay, start.Add(time.Second)) },
			want:  "work",
		},
		{
			name:  "forgotten already, unused for a week",
			since: func(s *sessions) { s.prune(start.Add(forgetAfter)) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var changes changeCount
			s := newSessions(at(start), changes.hear, unkept)
			assignFor(s, req, decision{account: "work", reason: reasonNew}, start)
			tt.since(s)
			before := changes

			if back, forgot := s.forget(req); back != "" || forgot != tt.wantForgot {
				t.Errorf("forget() = %q, %v, want \"\", %v", back, forgot, tt.wantForgot)
			}
			if got := s.lookup(req.key()); got.assigned != (tt.want != "") || got.current.Account != tt.want {
				t.Errorf("the session is assigned %+v (%v), want %q", got.current, got.assigned, tt.want)
			}
			var wantChanges changeCount
			if tt.wantForgot {
				wantChanges = 1
			}
			if got := changes - before; got != wantChanges {
				t.Errorf("forget() noted %d changes for the state file, want %d", got, wantChanges)
			}
		})
	}
}

func TestForgetPutsASessionBackAsItWasBeforeTheRequest(t *testing.T) {
	req := Request{ID: "a1b2c3d4", Session: "one", Model: opus}
	other := Request{ID: "e5f6a7b8", Session: "one", Model: opus}
	toSide := decision{account: "side", reason: "moved: work was refused"}
	toPersonal := decision{account: "personal", reason: "moved: side was refused"}
	// onWork is the session's assignment before req, which another request
	// made an hour before.
	onWork := assignment{Account: "work", Reason: reasonNew, AssignedAt: start.Add(-time.Hour), LastSeen: start.Add(-time.Minute), by: other.ID}
	tests := []struct {
		name string
		// moves are what befell the session from req on.
		moves    func(s *sessions)
		wantBack bool
		// want is the session's assignment once it's done.
		want assignment
	}{
		{
			name:     "moved by the request",
			moves:    func(s *sessions) { assignFor(s, req, toSide, start) },
			wantBack: true,
			want:     onWork,
		},
		{
			name: "moved by the request twice, as it was replayed",
			moves: func(s *sessions) {
				assignFor(s, req, toSide, start)
				assignFor(s, req, toPersonal, start.Add(time.Second))
			},
			wantBack: true,
			want:     onWork,
		},
		{
			name: "moved by another request since",
			moves: func(s *sessions) {
				assignFor(s, req, toSide, start)
				assignFor(s, other, toPersonal, start.Add(time.Second))
			},
			want: assignment{Account: "personal", Reason: toPersonal.reason, AssignedAt: start.Add(time.Second), LastSeen: start.Add(time.Second), by: other.ID},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var changes changeCount
			s := newSessions(at(start), changes.hear, unkept)
			s.assignments[req.key()] = onWork
			tt.moves(s)
			before := changes

			back, ok := s.forget(req)
			if wantBack := map[bool]string{true: "work"}[tt.wantBack]; back != wantBack || ok != tt.wantBack {
				t.Errorf("forget() = %q, %v, want %q, %v", back, ok, wantBack, tt.wantBack)
			}
			if got := s.lookup(req.key()).current; got != tt.want {
				t.Errorf("the session is assigned %+v, want %+v", got, tt.want)
			}
			if got, want := changes-before, map[bool]changeCount{true: 1}[tt.wantBack]; got != want {
				t.Errorf("forget() noted %d changes for the state file, want %d", got, want)
			}
		})
	}
}

func TestSessionsAreSafeForConcurrentUse(t *testing.T) {
	s := newSessions(at(start), unkept, unkept)
	var wg sync.WaitGroup
	for i := range 8 {
		k := key{session: fmt.Sprint(i % 2), model: opus}
		wg.Go(func() { assign(s, k, "", decision{account: "work", reason: reasonNew}, start) })
		wg.Go(func() { _ = s.lookup(k) })
		wg.Go(func() { s.setPin(status.Pin{Accounts: []string{"side"}, Since: start}, i%3 == 0) })
		wg.Go(func() { _, _ = s.unpin(i%3 == 1) })
		wg.Go(func() { s.pinSession(k.session, "side") })
		wg.Go(func() { _, _ = s.session(k.session) })
		wg.Go(func() { _ = s.running(start) })
		wg.Go(func() { _, _ = s.active(start) })
		wg.Go(func() { s.prune(start) })
	}
	wg.Wait()
}

func TestActiveCountsEachSessionOnceByAccountAndOnceInAll(t *testing.T) {
	s := newSessions(at(start), unkept, unkept)
	remember := func(session, model, account string, lastSeen time.Time) {
		assign(s, key{session: session, model: model}, "", decision{account: account, reason: reasonNew}, lastSeen)
	}
	remember("one", opus, "work", start)
	remember("one", haiku, "work", start.Add(-time.Hour))
	remember("two", opus, "work", start.Add(-time.Minute))
	remember("two", haiku, "side", start.Add(-time.Minute))
	remember("three", opus, "side", start.Add(-time.Hour-time.Second))

	want := map[string]int{"work": 2, "side": 1}
	byAccount, all := s.active(start)
	if !maps.Equal(byAccount, want) {
		t.Errorf("active() by account = %v, want %v: sessions used within the hour, each once an account", byAccount, want)
	}
	if all != 2 {
		t.Errorf("active() in all = %d, want 2: two's models went to two accounts, and it counts once", all)
	}
}

func TestSessionReportsItsPinAndItsAssignmentsTheOneUsedLastFirst(t *testing.T) {
	s := newSessions(at(start), unkept, unkept)
	assign(s, key{session: "one", model: haiku}, "", decision{account: "side", reason: reasonNew}, start.Add(-time.Hour))
	assign(s, key{session: "one", model: opus}, "work", decision{account: "work", reason: reasonPinned}, start)
	assign(s, key{session: "two", model: opus}, "", decision{account: "side", reason: reasonNew}, start)

	want := status.Session{
		ID:  "one",
		Pin: "work",
		Assignments: []status.Assignment{
			{Model: opus, Account: "work", Pinned: true, Reason: "pinned", AssignedAt: start, LastSeen: start},
			{Model: haiku, Account: "side", Reason: "new", AssignedAt: start.Add(-time.Hour), LastSeen: start.Add(-time.Hour)},
		},
	}
	if got, seen := s.session("one"); !seen || !reflect.DeepEqual(got, want) {
		t.Errorf("session() =\n%+v, %v\nwant\n%+v", got, seen, want)
	}
	if got, seen := s.session("nope"); seen {
		t.Errorf("session() of a session never seen = %+v, want none", got)
	}
}

func TestASessionsOwnPin(t *testing.T) {
	tests := []struct {
		name string
		// launched are the pins the session's requests of Opus, and of Haiku
		// a minute later, carried.
		launched [2]string
		// given is the pin the session is given while it runs, "" to clear
		// it, and nil for none.
		given *string
		want  string
	}{
		{name: "none"},
		{name: "the one it was launched with", launched: [2]string{"work", "work"}, want: "work"},
		{name: "the one its last request carried, launched again", launched: [2]string{"work", "side"}, want: "side"},
		{name: "one given while it runs, over the one it was launched with", launched: [2]string{"work", "work"}, given: new("side"), want: "side"},
		{name: "one given while it runs, launched without", given: new("side"), want: "side"},
		{name: "none, once cleared, the one it was launched with included", launched: [2]string{"work", "work"}, given: new("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSessions(at(start), unkept, unkept)
			assign(s, key{session: "one", model: opus}, tt.launched[0], decision{account: "work", reason: reasonNew}, start.Add(-time.Minute))
			assign(s, key{session: "one", model: haiku}, tt.launched[1], decision{account: "work", reason: reasonNew}, start)
			if tt.given != nil {
				if _, seen := s.pinSession("one", *tt.given); !seen {
					t.Fatal("pinSession() = false, want the session found")
				}
			}

			if got, _ := s.session("one"); got.Pin != tt.want {
				t.Errorf("session's pin = %q, want %q", got.Pin, tt.want)
			}
		})
	}
}

func TestASessionsAssignmentsSayWhetherItsOwnPinYieldedAndWhenItWasGiven(t *testing.T) {
	yields := decision{account: "work", reason: "pin yields: side has no room"}
	tests := []struct {
		name  string
		model string
		// route routes the session's requests of the model, on clock's time,
		// its own pin naming side.
		route func(s *sessions, clock *testClock, k key)
		// after is how long after start the session is asked after.
		after        time.Duration
		wantYielded  bool
		wantPinnedAt time.Time
	}{
		{
			name:  "launched pinned to side, gone to work for want of room there",
			model: opus,
			route: func(s *sessions, _ *testClock, k key) { assign(s, k, "side", yields, start) },
			after: time.Minute, wantYielded: true,
		},
		{
			name:  "launched pinned to side, gone to work, its cache cold since",
			model: opus,
			route: func(s *sessions, _ *testClock, k key) { assign(s, k, "side", yields, start) },
			after: 2 * time.Hour,
		},
		{
			name:  "launched pinned to side, gone to work, cold since, but its model's thinking bound to work",
			model: sonnet,
			route: func(s *sessions, _ *testClock, k key) { assign(s, k, "side", yields, start) },
			after: 2 * time.Hour, wantYielded: true,
		},
		{
			name:  "pinned to side while it runs, then gone to work",
			model: opus,
			route: func(s *sessions, clock *testClock, k key) {
				assign(s, k, "", decision{account: "work", reason: reasonNew}, start.Add(-time.Hour))
				clock.now = start.Add(-time.Minute)
				s.pinSession(k.session, "side")
				assign(s, k, "side", yields, start)
			},
			after: time.Minute, wantYielded: true, wantPinnedAt: start.Add(-time.Minute),
		},
		{
			name:  "gone to work as its pin yielded, then pinned to side again",
			model: opus,
			route: func(s *sessions, clock *testClock, k key) {
				assign(s, k, "side", yields, start.Add(-time.Minute))
				clock.now = start
				s.pinSession(k.session, "side")
			},
			after: time.Minute, wantPinnedAt: start,
		},
		{
			name:  "pinned to side, and on side",
			model: opus,
			route: func(s *sessions, _ *testClock, k key) {
				assign(s, k, "side", decision{account: "side", reason: reasonPinned}, start)
			},
			after: time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			s := newSessions(clock.read, unkept, unkept)
			s.bound = claude.Provider{}.ThinkingBound
			tt.route(s, clock, key{session: "one", model: tt.model})
			clock.now = start.Add(tt.after)

			got, _ := s.session("one")
			if a := got.Assignments[0]; a.Yielded != tt.wantYielded || !a.PinnedAt.Equal(tt.wantPinnedAt) {
				t.Errorf("the session's assignment is %+v, want it yielded: %v, pinned at %v", a, tt.wantYielded, tt.wantPinnedAt)
			}
			listed := s.running(clock.now)
			if len(listed) > 0 && !reflect.DeepEqual(listed[0], got) {
				t.Errorf("running() lists the session as\n%+v\nwant it as session() gives it\n%+v", listed[0], got)
			}
		})
	}
}

func TestASessionNeverSeenIsGivenNoPin(t *testing.T) {
	var changes changeCount
	s := newSessions(at(start), changes.hear, unkept)
	if was, seen := s.pinSession("nope", "side"); seen || was != "" {
		t.Errorf("pinSession() of a session never seen = %q, %v, want no pin, and false", was, seen)
	}
	if len(s.own) > 0 || changes > 0 {
		t.Errorf("sessions' pins = %+v, and %d changes to save, want none, and nothing to save", s.own, changes)
	}
}

func TestPinningASessionSaysWhatItsOwnPinWas(t *testing.T) {
	s := newSessions(at(start), unkept, unkept)
	assign(s, key{session: "one", model: opus}, "work", decision{account: "work", reason: reasonNew}, start)
	steps := []struct {
		pin, wantWas string
	}{
		{pin: "side", wantWas: "work"},
		{pin: "", wantWas: "side"},
		{pin: "", wantWas: ""},
		{pin: "work", wantWas: ""},
	}
	for _, step := range steps {
		if was, seen := s.pinSession("one", step.pin); !seen || was != step.wantWas {
			t.Errorf("pinSession(%q) = %q, %v, want %q, the session's own pin before, the one it was launched with first, and true", step.pin, was, seen, step.wantWas)
		}
	}
}

func TestRunningListsTheSessionsRoutedInTheLastHourTheOneSeenLastFirst(t *testing.T) {
	s := newSessions(at(start), unkept, unkept)
	remember := func(session, model, account string, lastSeen time.Time) {
		assign(s, key{session: session, model: model}, "", decision{account: account, reason: reasonNew}, lastSeen)
	}
	remember("earlier", opus, "work", start.Add(-30*time.Minute))
	remember("latest", opus, "side", start.Add(-time.Minute))
	remember("latest", haiku, "work", start.Add(-3*time.Hour))
	remember("b-tied", opus, "work", start.Add(-45*time.Minute))
	remember("a-tied", opus, "side", start.Add(-45*time.Minute))
	remember("an-hour-ago", opus, "work", start.Add(-time.Hour))
	remember("idle", opus, "work", start.Add(-time.Hour-time.Second))
	s.pinSession("earlier", "side")

	got := s.running(start)
	ids := make([]string, len(got))
	for i, session := range got {
		ids[i] = session.ID
	}
	if want := []string{"latest", "earlier", "a-tied", "b-tied", "an-hour-ago"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("running() lists %q, want %q: those routed in the last hour, the one seen last first", ids, want)
	}
	if want, _ := s.session("latest"); !reflect.DeepEqual(got[0], want) {
		t.Errorf("running() lists\n%+v\nwant the session as session() gives it, every model included\n%+v", got[0], want)
	}
	if got[1].Pin != "side" {
		t.Errorf("running() lists %+v, want its own pin with it", got[1])
	}
	if got := newSessions(at(start), unkept, unkept).running(start); got == nil || len(got) > 0 {
		t.Errorf("running() with no sessions = %#v, want an empty list", got)
	}
}

func TestForceClearsEverySessionsOwnPin(t *testing.T) {
	later := start.Add(time.Minute)
	tests := []struct {
		name  string
		force func(s *sessions) int
		// wantPin is the global pin once forced.
		wantPin status.Pin
	}{
		{
			name: "setting the global pin",
			force: func(s *sessions) int {
				return s.setPin(status.Pin{Accounts: []string{"side"}, Since: later, Move: true}, true)
			},
			wantPin: status.Pin{Accounts: []string{"side"}, Since: later, Move: true},
		},
		{
			name: "setting the global pin to several accounts",
			force: func(s *sessions) int {
				return s.setPin(status.Pin{Accounts: []string{"work", "side"}, Since: later}, true)
			},
			wantPin: status.Pin{Accounts: []string{"work", "side"}, Since: later},
		},
		{
			name: "clearing the global pin",
			force: func(s *sessions) int {
				_, cleared := s.unpin(true)
				return cleared
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			var changes changeCount
			s := newSessions(clock.read, changes.hear, unkept)
			s.setPin(status.Pin{Accounts: []string{"work"}, Since: start.Add(-time.Hour)}, false)
			assign(s, key{session: "launched", model: opus}, "work", decision{account: "work", reason: reasonPinned}, start)
			assign(s, key{session: "given", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
			assign(s, key{session: "unpinned", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
			s.pinSession("given", "side")
			clock.now = later
			changes = 0

			if cleared := tt.force(s); cleared != 2 {
				t.Errorf("cleared %d sessions' pins, want 2: the one launched with a pin, and the one given one", cleared)
			}
			want := map[string]ownPin{"launched": {Since: later}, "given": {Since: later}}
			if !maps.Equal(s.own, want) {
				t.Errorf("sessions' pins = %+v, want %+v: each cleared, the unpinned session's left alone", s.own, want)
			}
			if !reflect.DeepEqual(s.pin, tt.wantPin) || changes == 0 {
				t.Errorf("global pin = %+v, and %d changes to save, want %+v, due to be saved", s.pin, changes, tt.wantPin)
			}
			for _, id := range []string{"launched", "given"} {
				if got := s.lookup(key{session: id, model: opus}); !got.given || got.own.Account != "" {
					t.Errorf("session %s's own pin = %+v, want one given as cleared, which passes over the one it was launched with", id, got)
				}
			}
		})
	}
}

func TestUnpinningWithoutAPinOrForceChangesNothing(t *testing.T) {
	var changes changeCount
	s := newSessions(at(start), changes.hear, unkept)
	assign(s, key{session: "launched", model: opus}, "work", decision{account: "work", reason: reasonPinned}, start)
	changes = 0

	if was, cleared := s.unpin(false); !was.IsZero() || cleared != 0 || len(s.own) > 0 || changes > 0 {
		t.Errorf("unpin() = %+v, %d, sessions' pins %+v, and %d changes to save, want nothing changed", was, cleared, s.own, changes)
	}
}

func TestAPinReplacesTheOneBeforeAndUnpinningSaysWhatItWas(t *testing.T) {
	var changes changeCount
	s := newSessions(at(start), changes.hear, unkept)
	s.setPin(status.Pin{Accounts: []string{"work", "side"}, Since: start.Add(-time.Hour), Move: true}, false)

	pin := status.Pin{Accounts: []string{"side"}, Since: start}
	if cleared := s.setPin(pin, false); cleared != 0 || !reflect.DeepEqual(s.globalPin(), pin) {
		t.Errorf("setPin() cleared %d sessions' pins, leaving the pin %+v, want none cleared, and %+v alone", cleared, s.globalPin(), pin)
	}
	if was, cleared := s.unpin(false); !reflect.DeepEqual(was, pin) || cleared != 0 || !s.globalPin().IsZero() {
		t.Errorf("unpin() = %+v, %d, leaving the pin %+v, want %+v, 0, and no pin", was, cleared, s.globalPin(), pin)
	}
	if changes != 3 {
		t.Errorf("%d changes to save, want 3: each pin set, and the unpinning", changes)
	}
}
