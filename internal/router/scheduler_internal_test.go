package router

import (
	"cmp"
	"context"
	"maps"
	"net/http"
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
	"github.com/leeovery/switchboard/internal/status"
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
	r.state.record("side", []quota.Window{session, week}, r.state.mark())
	clock.now = start.Add(-15 * time.Minute)
	r.state.record("work", []quota.Window{session, week}, r.state.mark())
	clock.now = start

	choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
	if got, want := prober.counts(), map[string]int{sideToken: 1}; !maps.Equal(got, want) {
		t.Errorf("probes = %v, want %v: work was read 15 minutes ago, side 20", got, want)
	}
}

func TestAChoiceAfreshProbesNoAccountWhoseSessionHasLapsed(t *testing.T) {
	clock := &testClock{now: start.Add(-20 * time.Minute)}
	prober := &stubProber{}
	r := newTestRouter(t, clock.read, prober)
	// Both were read 20 minutes ago: work's session has lapsed since, with
	// nothing read of it, and side's still runs.
	lapsing, running := session, session
	lapsing.ResetsAt, running.ResetsAt = start.Add(-5*time.Minute), start.Add(2*time.Hour)
	r.state.record("work", []quota.Window{lapsing, week}, r.state.mark())
	r.state.record("side", []quota.Window{running, week}, r.state.mark())
	clock.now = start

	choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
	if got, want := prober.counts(), map[string]int{sideToken: 1}; !maps.Equal(got, want) {
		t.Errorf("probes = %v, want %v: side's usage is stale, and so is work's, but a probe would start work's session", got, want)
	}
}

func TestAChoiceAfreshProbesAnAccountWhoseSessionHasLapsedUnderALimit(t *testing.T) {
	clock := &testClock{now: start.Add(-20 * time.Minute)}
	prober := &stubProber{}
	r := newTestRouter(t, clock.read, prober)
	// Both were read 20 minutes ago: work's session has lapsed since, with
	// nothing read of it, but work is under a limit reached in its week, which
	// holds back every request; side's session still runs.
	lapsing, running, spentWeek := session, session, week
	lapsing.ResetsAt, running.ResetsAt = start.Add(-5*time.Minute), start.Add(2*time.Hour)
	spentWeek.Utilization, spentWeek.Status = 1, quota.StatusRejected
	r.state.record("work", []quota.Window{lapsing, spentWeek}, r.state.mark())
	r.state.limit("work", []string{"7d"}, start.Add(72*time.Hour), r.state.mark())
	r.state.record("side", []quota.Window{running, week}, r.state.mark())
	clock.now = start

	choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
	if got, want := prober.counts(), map[string]int{workToken: 1, sideToken: 1}; !maps.Equal(got, want) {
		t.Errorf("probes = %v, want %v: work can take no request, so a probe starting its session costs nothing", got, want)
	}
}

func TestWithNoRoomAnAccountWhoseSessionHasLapsedIsProbedAgainOnlyWhenSpent(t *testing.T) {
	spentSession, spentWeek, spentFableWeek := session, week, fableWeek
	spentSession.Utilization, spentSession.Status = 1, quota.StatusRejected
	spentWeek.Utilization, spentWeek.Status = 1, quota.StatusRejected
	spentFableWeek.Utilization, spentFableWeek.Status = 1, quota.StatusRejected
	tests := []struct {
		name string
		// week and fableWeek are work's, as read.
		week, fableWeek quota.Window
		want            map[string]int
	}{
		{
			name:      "not work, its Fable week spent, whose session a probe would start",
			week:      week,
			fableWeek: spentFableWeek,
			want:      map[string]int{sideToken: 1},
		},
		{
			name:      "work, its week spent, which can take no request anyway",
			week:      spentWeek,
			fableWeek: fableWeek,
			want:      map[string]int{workToken: 1, sideToken: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start.Add(-2 * time.Minute)}
			prober := &stubProber{}
			r := newTestRouter(t, clock.read, prober)
			// Read two minutes ago, neither has room for a Fable request:
			// work's session has lapsed since, and side's is spent till later.
			lapsing := session
			lapsing.ResetsAt = start.Add(-time.Minute)
			r.state.record("work", []quota.Window{lapsing, tt.week, tt.fableWeek}, r.state.mark())
			r.state.record("side", []quota.Window{spentSession, week, fableWeek}, r.state.mark())
			clock.now = start

			if got := choose(t.Context(), r, Request{Session: "one", Model: fable, Client: "work"}); !got.NoRoom {
				t.Fatalf("Choose() = %+v, want no account with room", got)
			}
			if got := prober.counts(); !maps.Equal(got, tt.want) {
				t.Errorf("probes = %v, want %v", got, tt.want)
			}
		})
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
	if want := (Choice{Account: "side", Reason: "new", New: true}); got != want {
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
	r.state.record("work", []quota.Window{refused, laterWeek}, r.state.mark())
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
			if want := (Choice{Account: "side", Reason: "new", New: true}); got != want {
				t.Errorf("session %d: Choose() = %+v, want %+v", i+1, got, want)
			}
		}
	})
}

func TestNoProbeStartsOnceTheRouterHasStopped(t *testing.T) {
	prober := &stubProber{}
	r := newTestRouter(t, at(start), prober)
	r.probes.stop()
	always := func(string, time.Time) bool { return true }

	for _, launch := range []func(accounts, func(string, time.Time) bool) []probing{r.probes.start, r.probes.prime} {
		if underway := launch(r.accounts.sendable(), always); len(underway) > 0 {
			t.Errorf("once the router has stopped, %d probes are under way, want none", len(underway))
		}
	}
	if got := prober.counts(); len(got) > 0 {
		t.Errorf("once the router has stopped, probes = %v, want none", got)
	}
}

func TestAChoiceAwaitsOnlyTheProbesOfTheAccountsItWouldProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		prober := &stubProber{gate: make(chan struct{})}
		r := newTestRouter(t, at(start), prober)
		defer r.probes.stop()
		// Both accounts were read just now, and a prime of work is under
		// way, which never ends.
		r.state.record("work", []quota.Window{session, laterWeek}, r.state.mark())
		r.state.record("side", []quota.Window{session, soonWeek}, r.state.mark())
		r.probes.prime(r.accounts.only([]string{"work"}), func(string, time.Time) bool { return true })
		synctest.Wait()

		began := time.Now()
		got := choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
		if waited := time.Since(began); waited != 0 {
			t.Errorf("Choose() waited %v, want no wait: neither account wants probing", waited)
		}
		if want := (Choice{Account: "side", Reason: "new", New: true}); got != want {
			t.Errorf("Choose() = %+v, want %+v", got, want)
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
		if want := (Choice{Account: "work", Reason: "no account has room", NoRoom: true, New: true}); got != want {
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
	if got := r.sessions.lookup(k).current; got != first {
		t.Errorf("after its first request, the session is assigned %+v, want %+v", got, first)
	}

	clock.now = start.Add(10 * time.Minute)
	choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
	sticky := first
	sticky.LastSeen = clock.now
	if got := r.sessions.lookup(k).current; got != sticky {
		t.Errorf("after a request that stuck, the session is assigned %+v, want %+v: seen again, but assigned as before", got, sticky)
	}

	clock.now = start.Add(20 * time.Minute)
	choose(t.Context(), r, Request{Session: "one", Model: opus, Pin: "side", Client: "work"})
	moved := assignment{Account: "side", Pin: "side", Reason: "pinned", AssignedAt: clock.now, LastSeen: clock.now}
	if got := r.sessions.lookup(k).current; got != moved {
		t.Errorf("after a request pinned elsewhere, the session is assigned %+v, want %+v", got, moved)
	}
}

func TestASessionStartedByARequestThatStuckIsToldOfAsItsAssignmentSays(t *testing.T) {
	r := newTestRouter(t, at(start), &stubProber{readings: map[string]quota.Probe{
		workToken: probed(nil, session, soonWeek),
		sideToken: probed(nil, session, laterWeek),
	}})
	choose(t.Context(), r, Request{ID: "first", Session: "one", Model: opus, Client: "work"})
	// The session's next request, chosen while its first is out, stays where
	// the first went, and is answered first.
	next := Request{ID: "next", Session: "one", Model: opus, Client: "work"}
	c := choose(t.Context(), r, next)
	if c.Account != "work" || c.Reason != reasonSticky {
		t.Fatalf("the next request went to %s (%s), want work, sticking with the first", c.Account, c.Reason)
	}
	r.proxy.chooser.Answered(next, c.Account, c.Reason)

	if got := r.recent.events(); len(got) != 1 || got[0].Kind != status.EventStarted || got[0].Account != "work" || got[0].Reason != reasonNew {
		t.Errorf("events() = %+v, want one started on work, as new: why its assignment went there", got)
	}
}

func TestANewSessionIsRememberedOnceItsAnswered(t *testing.T) {
	forgot := []string{"level=DEBUG", `msg="forgot a new session whose request went unanswered"`, "session=one", "model=" + opus}
	tests := []struct {
		name     string
		upstream *scriptedUpstream
		// goneAfter is when the client goes, if it does.
		goneAfter time.Duration
		// wantStatus is what the client is answered, or zero when it's gone.
		wantStatus int
		wantSent   []string
		// want is the account the session is remembered on, "" for none.
		want string
	}{
		{name: "answered", upstream: scripted(served, served), wantStatus: http.StatusOK, wantSent: []string{"work"}, want: "work"},
		{
			name:       "answered once it moved on",
			upstream:   scripted(limitHit, served),
			wantStatus: http.StatusOK,
			wantSent:   []string{"work", "side"},
			want:       "side",
		},
		{name: "a 429 passed on", upstream: scripted(refusedAlone, served), wantStatus: http.StatusTooManyRequests, wantSent: []string{"work"}},
		{
			name:       "at every account's limit, having moved on",
			upstream:   scripted(limitHit, limitHit),
			wantStatus: http.StatusTooManyRequests,
			wantSent:   []string{"work", "side"},
		},
		{name: "refused on every account", upstream: scripted(forbidden, forbidden), wantStatus: http.StatusBadGateway, wantSent: []string{"work", "side"}},
		{name: "another error", upstream: scripted(serverError, served), wantStatus: http.StatusInternalServerError, wantSent: []string{"work"}},
		{name: "the upstream unreached", upstream: scripted(unreachable, served), wantStatus: http.StatusBadGateway, wantSent: []string{"work"}},
		{name: "the client gone", upstream: scripted(throttled("5"), served), goneAfter: time.Second, wantSent: []string{"work"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				log := logstest.Capture(t)
				// Work's quota needs using first, so the session goes there.
				r := newTestRouter(t, at(start), &stubProber{})
				r.state.record("work", []quota.Window{session, soonWeek}, r.state.mark())
				r.state.record("side", []quota.Window{session, laterWeek}, r.state.mark())
				r.proxy.transport = tt.upstream

				if got := routeAlone(clientGoing(t, tt.goneAfter), r, "one"); got != tt.wantStatus {
					t.Errorf("answered %d, want %d", got, tt.wantStatus)
				}
				if got := tt.upstream.sent(); !slices.Equal(got, tt.wantSent) {
					t.Errorf("the request went out on %q, want %q", got, tt.wantSent)
				}
				remembered, wantSessions := tt.want != "", 0
				if remembered {
					wantSessions = 1
				}
				if found := r.sessions.lookup(key{session: "one", model: opus}); found.assigned != remembered || found.current.Account != tt.want {
					t.Errorf("the session is assigned %+v (%v), want %q", found.current, found.assigned, tt.want)
				}
				if got := r.Status().Sessions; got != wantSessions {
					t.Errorf("the status counts %d sessions, want %d", got, wantSessions)
				}
				if got := len(r.sessions.saved().Sessions); got != wantSessions {
					t.Errorf("the state file keeps %d sessions' assignments, want %d", got, wantSessions)
				}
				if log.Has(forgot...) == remembered {
					t.Errorf("log reads\n%s\nwant a line with %q only where the session is forgotten", log, forgot)
				}
			})
		})
	}
}

func TestASessionKeepsItsAccountWhateverItsRequestsEnd(t *testing.T) {
	tests := []struct {
		name     string
		upstream *scriptedUpstream
		// goneAfter is when the client goes, if it does.
		goneAfter time.Duration
		// want is the account the session is on once it's done.
		want string
	}{
		{name: "a 429 passed on", upstream: scripted(refusedAlone, served), want: "work"},
		{name: "another error", upstream: scripted(serverError, served), want: "work"},
		{name: "the upstream unreached", upstream: scripted(unreachable, served), want: "work"},
		{name: "the client gone", upstream: scripted(throttled("5"), served), goneAfter: time.Second, want: "work"},
		{name: "at every account's limit, having moved on", upstream: scripted(limitHit, limitHit), want: "side"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := newTestRouter(t, at(start), &stubProber{})
				r.state.record("work", []quota.Window{session, soonWeek}, r.state.mark())
				r.state.record("side", []quota.Window{session, laterWeek}, r.state.mark())
				k := key{session: "one", model: opus}
				assign(r.sessions, k, "", decision{account: "work", reason: reasonNew}, start.Add(-time.Minute))
				r.proxy.transport = tt.upstream

				routeAlone(clientGoing(t, tt.goneAfter), r, "one")
				if found := r.sessions.lookup(k); !found.assigned || found.current.Account != tt.want {
					t.Errorf("the session is assigned %+v (%v), want %s", found.current, found.assigned, tt.want)
				}
			})
		})
	}
}

func TestForgettingANewSessionLeavesTheAccountAnotherRequestWasChosenSince(t *testing.T) {
	log := logstest.Capture(t)
	r := newTestRouter(t, at(start), &stubProber{})
	r.state.record("work", []quota.Window{session, soonWeek}, r.state.mark())
	r.state.record("side", []quota.Window{session, laterWeek}, r.state.mark())
	first := Request{ID: "a1b2c3d4", Session: "one", Model: opus, Client: "work"}
	if got := choose(t.Context(), r, first); !got.New || got.Account != "work" {
		t.Fatalf("Choose() = %+v, want the new session on work", got)
	}

	// Another request of the session, which reached work's limit, moves it
	// to side on its replay; then the first goes unanswered.
	moving := Request{ID: "e5f6a7b8", Session: "one", Model: opus, Client: "work", Tried: []Attempt{{Account: "work", Why: whyLimit}}}
	if got := choose(t.Context(), r, moving); got.New || got.Account != "side" {
		t.Fatalf("Choose() = %+v, want the session moved to side, and not new", got)
	}
	r.proxy.chooser.Forget(first)

	if found := r.sessions.lookup(first.key()); found.current.Account != "side" {
		t.Errorf("the session is assigned %+v (%v), want side: the move stands", found.current, found.assigned)
	}
	if log.Has("forgot") {
		t.Errorf("log reads\n%s\nwant nothing forgotten", log)
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
	r.state.record("work", []quota.Window{session, soonWeek}, r.state.mark())
	r.state.record("side", []quota.Window{session, laterWeek}, r.state.mark())
	k := key{session: "one", model: opus}
	assign(r.sessions, k, "", decision{account: "work", reason: reasonNew}, start.Add(-time.Minute))
	first := Request{Session: "one", Model: opus, Client: "work"}

	// One request of the session finds it on work, and chooses to stay; then
	// another, which reached work's limit, moves it to side on its replay;
	// then the first remembers its choice.
	stay, on := s.decide(first)
	choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work", Tried: []Attempt{{Account: "work", Why: whyLimit}}})
	s.remember(on, stay)

	want := assignment{Account: "side", Reason: "moved: work hit its limit", AssignedAt: start, LastSeen: start}
	if got := r.sessions.lookup(k).current; got != want {
		t.Errorf("the session is assigned %+v, want %+v: the move stands", got, want)
	}
	wantEvents := []Event{Moved{Session: "one", Model: opus, From: "work", To: "side", Reason: "moved: work hit its limit"}}
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

func TestTheQuotaCheckIsChosenForButNeverRemembered(t *testing.T) {
	tests := []struct {
		name string
		// assigned is the account the session is on before its check, "" for
		// none, as for a new session.
		assigned string
	}{
		{name: "of a new session"},
		{name: "of a session whose account has no room", assigned: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			// Work has no room in its session; side has, and its quota needs
			// using soon.
			full := session
			full.Utilization = 1
			r.state.record("work", []quota.Window{full, laterWeek}, r.state.mark())
			r.state.record("side", []quota.Window{session, soonWeek}, r.state.mark())
			k := key{session: "one", model: opus}
			if tt.assigned != "" {
				assign(r.sessions, k, "", decision{account: tt.assigned, reason: reasonNew}, start.Add(-time.Minute))
			}
			before := r.sessions.lookup(k)

			got := choose(t.Context(), r, Request{ID: "check", Session: "one", Model: opus, Check: true, Client: "work"})
			if got.Account != "side" || got.New || got.From != "" {
				t.Errorf("Choose() = %+v, want side, the session neither new to it nor moved there", got)
			}
			if after := r.sessions.lookup(k); after.current != before.current || after.assigned != before.assigned {
				t.Errorf("the session is assigned %+v (%v), want %+v (%v), as before its check: the check is never remembered",
					after.current, after.assigned, before.current, before.assigned)
			}
			if len(heard) > 0 {
				t.Errorf("events = %+v, want none: the check moves no session", heard)
			}
		})
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

func TestAMoveSaysWhatHeldItsRequestBack(t *testing.T) {
	// fableAlone has work reach a limit in the Fable week, which holds back
	// Fable's requests alone.
	fableAlone := func(s *state) {
		s.learn(fable, []quota.Window{fableWeek})
		s.limit("work", []string{"7d_oi"}, start.Add(time.Hour), s.mark())
	}
	tests := []struct {
		name string
		// hold holds work back from the session's Opus request, which moves
		// it to side, for reason, "moved: work has no room" when it's "", work
		// keeping a tenth of every window back as its reserve.
		hold      func(s *state)
		reason    string
		wantHeld  Hold
		wantLimit int
	}{
		{
			name:     "its limit, in a window every model shares",
			hold:     func(s *state) { s.limit("work", []string{"5h"}, start.Add(time.Hour), s.mark()) },
			wantHeld: HeldByLimit, wantLimit: 1,
		},
		{
			name: "the second of its limits, the first in the Fable week",
			hold: func(s *state) {
				fableAlone(s)
				s.limit("work", []string{"5h"}, start.Add(time.Hour), s.mark())
			},
			wantHeld: HeldByLimit, wantLimit: 2,
		},
		{
			name: "its limit, though its token is refused too",
			hold: func(s *state) {
				s.limit("work", []string{"5h"}, start.Add(time.Hour), s.mark())
				s.refuse("work", http.StatusUnauthorized, someRequest)
			},
			wantHeld: HeldByLimit, wantLimit: 1,
		},
		{
			name: "a refusal of its token, while its limit holds back Fable alone",
			hold: func(s *state) {
				fableAlone(s)
				s.refuse("work", http.StatusUnauthorized, someRequest)
			},
			wantHeld: HeldByRefusal,
		},
		{
			name:     "a refusal of Opus on it",
			hold:     func(s *state) { s.forbid("work", "opus", http.StatusForbidden, someRequest) },
			wantHeld: HeldByRefusal,
		},
		{
			name: "its reserve",
			hold: func(s *state) {
				reserved := session
				reserved.Utilization = 0.95
				s.record("work", []quota.Window{reserved, soonWeek}, s.mark())
			},
			reason:   "moved: work is at its reserve",
			wantHeld: HeldByReserve,
		},
		{
			name: "nothing but its session spent, while its limit holds back Fable alone",
			hold: func(s *state) {
				fableAlone(s)
				spent := session
				spent.Utilization = 1
				s.record("work", []quota.Window{spent, soonWeek}, s.mark())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, heard := newReserving(t)
			assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start.Add(-time.Minute))
			tt.hold(r.state)

			choose(t.Context(), r, Request{ID: "5f3a9c2e", Session: "one", Model: opus, Client: "work"})
			want := Moved{Session: "one", Model: opus, From: "work", To: "side", Reason: cmp.Or(tt.reason, "moved: work has no room"), Held: tt.wantHeld, Limit: tt.wantLimit}
			if !reflect.DeepEqual(*heard, []Event{want}) {
				t.Errorf("events = %+v, want %+v: the move saying what held its request back, and which limit, if one did", *heard, want)
			}
		})
	}
}

func TestOnlyAMoveTheSessionHadToMakeSaysWhatHeldItsRequestBack(t *testing.T) {
	holds := []struct {
		name string
		// hold holds work back from the session's Opus request, as held and
		// limit say.
		hold  func(s *state)
		held  Hold
		limit int
	}{
		{
			name: "its limit",
			hold: func(s *state) { s.limit("work", []string{"5h"}, start.Add(time.Hour), s.mark()) },
			held: HeldByLimit, limit: 1,
		},
		{
			name: "a refusal of its token",
			hold: func(s *state) { s.refuse("work", http.StatusUnauthorized, someRequest) },
			held: HeldByRefusal,
		},
		{
			name: "its reserve",
			hold: func(s *state) {
				reserved := session
				reserved.Utilization = 0.95
				s.record("work", []quota.Window{reserved, soonWeek}, s.mark())
			},
			held: HeldByReserve,
		},
	}
	moves := []struct {
		name string
		// idle is how long the session has idled on work; launched is the pin
		// it was launched with, which its requests carry; own is the pin it's
		// given while it runs; and global is the global pin.
		idle     time.Duration
		launched string
		own      string
		global   status.Pin
		// forced is set where the session has to leave work.
		forced bool
	}{
		{name: "as work can't take its request", idle: time.Minute, forced: true},
		{name: "as its own pin to work yields", idle: time.Minute, launched: "work", forced: true},
		{name: "as its own pin to work yields, its cache cold", idle: 2 * time.Hour, launched: "work", forced: true},
		{name: "but by the global pin, which moves it", idle: time.Minute, global: status.Pin{Accounts: []string{"side"}, Since: start, Move: true}},
		{name: "but by its own pin", idle: time.Minute, own: "side"},
		{name: "but rescored after its idle hours", idle: 2 * time.Hour},
	}
	for _, h := range holds {
		for _, m := range moves {
			if m.launched == "work" && h.held == HeldByReserve {
				// A session's own pin spends its account's reserve, so it
				// yields at its limit or a refusal alone.
				continue
			}
			t.Run(h.name+", "+m.name, func(t *testing.T) {
				r, heard := newReserving(t)
				assign(r.sessions, key{session: "one", model: opus}, m.launched, decision{account: "work", reason: reasonNew}, start.Add(-m.idle))
				if m.own != "" {
					r.sessions.pinSession("one", m.own)
				}
				if len(m.global.Accounts) > 0 {
					r.sessions.setPin(m.global, false)
				}
				h.hold(r.state)

				choose(t.Context(), r, Request{ID: "5f3a9c2e", Session: "one", Model: opus, Pin: m.launched, Client: "work"})
				want := Moved{Session: "one", Model: opus, From: "work", To: "side"}
				if m.forced {
					want.Held, want.Limit = h.held, h.limit
				}
				if len(*heard) != 1 {
					t.Fatalf("events = %+v, want the session's move alone", *heard)
				}
				got, _ := (*heard)[0].(Moved)
				if got.Reason == "" || got.From != want.From || got.To != want.To || got.Held != want.Held || got.Limit != want.Limit {
					t.Errorf("events = %+v, want the move from work to side, held back by %d and limit %d", got, want.Held, want.Limit)
				}
			})
		}
	}
}

// newReserving returns a router of testConfigured at start, work keeping a
// tenth of every window back as its reserve, and its quota needing using
// before side's, with the events it tells, as it tells them.
func newReserving(t *testing.T) (*Router, *[]Event) {
	t.Helper()
	var heard []Event
	reserving := slices.Clone(testConfigured)
	reserving[0].Reserve = 0.1
	r, err := New(Config{
		Accounts: reserving,
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
	r.state.record("work", []quota.Window{session, soonWeek}, r.state.mark())
	r.state.record("side", []quota.Window{session, laterWeek}, r.state.mark())
	return r, &heard
}
