package router

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
)

// Readings of an account whose quota needs using soon, and one whose can
// wait: the week of the first resets tomorrow, the second's in five days.
var (
	soonWeek  = quota.Window{Key: "7d", Label: "Week", Utilization: 0.5, ResetsAt: start.Add(24 * time.Hour)}
	laterWeek = quota.Window{Key: "7d", Label: "Week", Utilization: 0.5, ResetsAt: start.Add(5 * 24 * time.Hour)}
)

func TestAChoiceAfreshProbesTheAccountsWhoseUsageIsStale(t *testing.T) {
	clock := &testClock{now: start.Add(-20 * time.Minute)}
	prober := &stubProber{}
	r := newTestRouter(t, clock.read, prober)
	r.state.record("side", []quota.Window{session, week})
	clock.now = start.Add(-15 * time.Minute)
	r.state.record("work", []quota.Window{session, week})
	clock.now = start

	choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
	if got, want := prober.counts(), map[string]int{sideToken: 1}; !maps.Equal(got, want) {
		t.Errorf("probes = %v, want %v: work was read 15 minutes ago, side 20", got, want)
	}
}

func TestAStickyChoiceProbesNothing(t *testing.T) {
	prober := &stubProber{}
	r := newTestRouter(t, at(start), prober)
	assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "side", reason: reasonNew}, start.Add(-time.Minute))

	got := choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
	if want := (Choice{Account: "side", Reason: "sticky"}); got != want {
		t.Errorf("Choose() = %+v, want %+v", got, want)
	}
	if got := prober.counts(); len(got) > 0 {
		t.Errorf("probes = %v, want none: nothing read of any account is stale enough to move a warm session", got)
	}
}

func TestAChoiceAfreshIsMadeOnFreshUsage(t *testing.T) {
	prober := &stubProber{readings: map[string]quota.Probe{
		workToken: probed(nil, session, laterWeek),
		sideToken: probed(nil, session, soonWeek),
	}}
	r := newTestRouter(t, at(start), prober)

	got := choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
	if want := (Choice{Account: "side", Reason: "new"}); got != want {
		t.Errorf("Choose() = %+v, want %+v, chosen on what the probes read", got, want)
	}
}

func TestASessionStaysWhenFreshUsageFindsRoomOnItsAccount(t *testing.T) {
	clock := &testClock{now: start.Add(-20 * time.Minute)}
	refused := session
	refused.Status = quota.StatusRejected
	prober := &stubProber{readings: map[string]quota.Probe{
		workToken: probed(nil, session, laterWeek),
		sideToken: probed(nil, session, soonWeek),
	}}
	r := newTestRouter(t, clock.read, prober)
	r.state.record("work", []quota.Window{refused, laterWeek})
	clock.now = start
	assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start.Add(-time.Minute))

	got := choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
	if want := (Choice{Account: "work", Reason: "sticky"}); got != want {
		t.Errorf("Choose() = %+v, want %+v: work was refused in a reading since read again with room", got, want)
	}
}

func TestChoicesMadeTogetherShareTheirProbes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		prober := &stubProber{gate: make(chan struct{}), readings: map[string]quota.Probe{
			workToken: probed(nil, session, laterWeek),
			sideToken: probed(nil, session, soonWeek),
		}}
		r := newTestRouter(t, at(start), prober)
		var choosing sync.WaitGroup
		chosen := make([]Choice, 3)
		for i, id := range []string{"one", "two", "three"} {
			choosing.Go(func() { chosen[i] = choose(t.Context(), r, Request{Session: id, Model: opus, Client: "work"}) })
		}

		synctest.Wait()
		want := map[string]int{workToken: 1, sideToken: 1}
		if got := prober.counts(); !maps.Equal(got, want) {
			t.Errorf("while three new sessions wait, probes = %v, want %v", got, want)
		}
		close(prober.gate)
		choosing.Wait()
		if got := prober.counts(); !maps.Equal(got, want) {
			t.Errorf("once they're chosen, probes = %v, want %v", got, want)
		}
		for i, got := range chosen {
			if want := (Choice{Account: "side", Reason: "new"}); got != want {
				t.Errorf("session %d: Choose() = %+v, want %+v", i+1, got, want)
			}
		}
	})
}

func TestAChoiceWaitsForProbesEightSecondsAtMost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		prober := &stubProber{gate: make(chan struct{})}
		r := newTestRouter(t, at(start), prober)
		defer r.probes.stop()

		began := time.Now()
		got := choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
		if waited := time.Since(began); waited != 8*time.Second {
			t.Errorf("Choose() waited %v for probes that never end, want 8s", waited)
		}
		if want := (Choice{Account: "work", Reason: "no account has room", NoRoom: true}); got != want {
			t.Errorf("Choose() = %+v, want %+v", got, want)
		}
		if !log.Has("level=DEBUG", `msg="stopped waiting for probes before choosing"`, "accounts=work,side", "after=8s") {
			t.Errorf("log reads\n%s\nwant the wait cut short", log)
		}
	})
}

func TestAChoiceStopsWaitingForProbesWhenItsClientGoes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newTestRouter(t, at(start), &stubProber{gate: make(chan struct{})})
		defer r.probes.stop()
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(time.Second, cancel)

		began := time.Now()
		choose(ctx, r, Request{Session: "one", Model: opus, Client: "work"})
		if waited := time.Since(began); waited != time.Second {
			t.Errorf("Choose() waited %v, want 1s, when its client went", waited)
		}
	})
}

func TestProbesBeforeAChoiceAreLogged(t *testing.T) {
	log := logstest.Capture(t)
	r := newTestRouter(t, at(start), &stubProber{readings: map[string]quota.Probe{
		workToken: probed(nil, session, laterWeek),
	}})

	choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
	if !log.Has("level=DEBUG", `msg="probed before choosing"`, "accounts=work,side", "duration=") {
		t.Errorf("log reads\n%s\nwant the probes before the choice, and how long they took", log)
	}
}

func TestChoosingRemembersTheSession(t *testing.T) {
	clock := &testClock{now: start}
	r := newTestRouter(t, clock.read, &stubProber{readings: map[string]quota.Probe{
		workToken: probed(nil, session, soonWeek),
		sideToken: probed(nil, session, laterWeek),
	}})
	k := key{session: "one", model: opus}

	choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
	first := assignment{Account: "work", Reason: "new", AssignedAt: start, LastSeen: start}
	if got, _, _ := r.sessions.lookup(k); got != first {
		t.Errorf("after its first request, the session is assigned %+v, want %+v", got, first)
	}

	clock.now = start.Add(10 * time.Minute)
	choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
	sticky := first
	sticky.LastSeen = clock.now
	if got, _, _ := r.sessions.lookup(k); got != sticky {
		t.Errorf("after a request that stuck, the session is assigned %+v, want %+v: seen again, but assigned as before", got, sticky)
	}

	clock.now = start.Add(20 * time.Minute)
	choose(t.Context(), r, Request{Session: "one", Model: opus, Pin: "side", Client: "work"})
	moved := assignment{Account: "side", Pin: "side", Reason: "pinned", AssignedAt: clock.now, LastSeen: clock.now}
	if got, _, _ := r.sessions.lookup(k); got != moved {
		t.Errorf("after a request pinned elsewhere, the session is assigned %+v, want %+v", got, moved)
	}
}

func TestARequestWithoutASessionIsntRemembered(t *testing.T) {
	r := newTestRouter(t, at(start), &stubProber{readings: map[string]quota.Probe{
		workToken: probed(nil, session, soonWeek),
	}})

	got := choose(t.Context(), r, Request{Model: opus, Client: "side"})
	if want := (Choice{Account: "work", Reason: "unsessioned"}); got != want {
		t.Errorf("Choose() = %+v, want %+v", got, want)
	}
	if n := len(r.sessions.assignments); n > 0 {
		t.Errorf("sessions hold %d assignments, want none", n)
	}
}

func TestAChoiceLeavesTheAssignmentAnotherRequestMadeSinceItLooked(t *testing.T) {
	log := logstest.Capture(t)
	var heard []Event
	r, err := New(Config{
		Accounts: testConfigured,
		Token:    testTokens.Read,
		Upstream: "http://127.0.0.1:1",
		Provider: claude.Provider{},
		Prober:   &stubProber{},
		Policy:   testPolicy,
		Now:      at(start),
		Events:   func(e Event) { heard = append(heard, e) },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	s := r.proxy.chooser.(*scheduler)
	r.state.record("work", []quota.Window{session, soonWeek})
	r.state.record("side", []quota.Window{session, laterWeek})
	k := key{session: "one", model: opus}
	assign(r.sessions, k, "", decision{account: "work", reason: reasonNew}, start.Add(-time.Minute))
	first := Request{Session: "one", Model: opus, Client: "work"}

	// One request of the session finds it on work, and chooses to stay; then
	// another, which reached work's limit, moves it to side on its replay;
	// then the first remembers its choice.
	stay, was := s.decide(first)
	choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work", Tried: []Attempt{{Account: "work", Why: whyLimit}}})
	s.remember(first, was, stay)

	want := assignment{Account: "side", Reason: "moved: work hit its limit", AssignedAt: start, LastSeen: start}
	if got, _, _ := r.sessions.lookup(k); got != want {
		t.Errorf("the session is assigned %+v, want %+v: the move stands", got, want)
	}
	wantEvents := []Event{Moved{Session: "one", Model: opus, From: "work", To: "side", Reason: "moved: work hit its limit", Forced: true}}
	if !reflect.DeepEqual(heard, wantEvents) {
		t.Errorf("events = %+v, want %+v alone", heard, wantEvents)
	}
	if log.Has("msg=moved", "from=side") {
		t.Errorf("log reads\n%s\nwant no move back to work", log)
	}
	want2 := []string{"level=INFO", `msg="session moved meanwhile; its newer assignment stands"`, "session=one", "account=side", "chosen=work"}
	if !log.Has(want2...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want2)
	}
}

func TestMovesAreLogged(t *testing.T) {
	log := logstest.Capture(t)
	r := newTestRouter(t, at(start), &stubProber{})
	assign(r.sessions, key{session: "0b5c6f2e-7d41", model: opus}, "", decision{account: "work", reason: reasonNew}, start.Add(-time.Minute))

	choose(t.Context(), r, Request{Session: "0b5c6f2e-7d41", Model: opus, Pin: "side", Client: "work"})
	want := []string{"level=INFO", "msg=moved", "session=0b5c6f2e", "model=claude-opus-5-5", "from=work", "to=side", "reason=pinned"}
	if !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
	choose(t.Context(), r, Request{Session: "0b5c6f2e-7d41", Model: opus, Pin: "side", Client: "work"})
	if moves := slices.DeleteFunc(log.Lines(), func(line string) bool { return !strings.Contains(line, "msg=moved") }); len(moves) != 1 {
		t.Errorf("log reads\n%s\nwant one move alone, for the request that moved", log)
	}
}
