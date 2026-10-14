package router

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
)

func TestAutoProbesNoAccountWhereAChoiceWould(t *testing.T) {
	clock := &testClock{now: start}
	prober := &stubProber{}
	r := newTestRouter(t, clock.read, prober)
	r.state.record("work", []quota.Window{session, soonWeek}, r.state.mark())
	r.state.record("side", []quota.Window{session, laterWeek}, r.state.mark())
	assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
	// The session's cache has gone cold, so it's chosen afresh, and every
	// account's usage has gone stale.
	clock.now = start.Add(2 * time.Hour)

	if got := r.scheduler.auto("one", opus, clock.now); got != "work" {
		t.Errorf("auto(one, %s) = %q, want work, whose quota needs using first", opus, got)
	}
	if got := prober.counts(); len(got) > 0 {
		t.Errorf("auto probed %v, want none: it chooses on what's known", got)
	}
	choose(t.Context(), r, Request{ID: "next", Session: "one", Model: opus, Client: "work"})
	if got := prober.counts(); len(got) == 0 {
		t.Errorf("a choice for the session's next request probed nothing, want the stale accounts probed, as the moment needs")
	}
}

func TestAutoLeavesTheLogToTellOfAReserveAsAChoiceFindsIt(t *testing.T) {
	log := logstest.Capture(t)
	clock := &testClock{now: start}
	r := newThreeRouter(t, clock, &stubProber{}, map[string]float64{"work": 0.1})
	r.state.record("work", []quota.Window{sessionIn(0.95, 3*time.Hour), weekIn(0.5, day)}, r.state.mark())
	for _, id := range []string{"side", "spare"} {
		r.state.record(id, []quota.Window{sessionIn(0.2, 3*time.Hour), weekIn(0.5, 5*day)}, r.state.mark())
	}
	assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)

	r.scheduler.auto("one", opus, start)
	if log.Has(`msg="held back by its reserve"`) {
		t.Errorf("log reads\n%s\nwant nothing of work's reserve once auto alone has looked", log)
	}
	choose(t.Context(), r, Request{ID: "next", Session: "one", Model: opus, Client: "work"})
	if !log.Has(`msg="held back by its reserve"`, "account=work") {
		t.Errorf("log reads\n%s\nwant work held back by its reserve, told as the session's next choice finds it", log)
	}
}

func TestAutoNamesNoAccountForAnAssignmentTheRouterHasNone(t *testing.T) {
	tests := []struct {
		name           string
		session, model string
	}{
		{name: "a model the session hasn't asked for", session: "one", model: haiku},
		{name: "a session never seen", session: "two", model: opus},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(t, at(start), &stubProber{})
			r.state.record("work", []quota.Window{session, soonWeek}, r.state.mark())
			r.state.record("side", []quota.Window{session, laterWeek}, r.state.mark())
			assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)

			if got := r.scheduler.auto(tt.session, tt.model, start); got != "" {
				t.Errorf("auto(%s, %s) = %q, want none", tt.session, tt.model, got)
			}
		})
	}
}

// The router's listing of its sessions glances at the accounts once for each
// model they run, however many run it, each glance taking the lock every
// request's choice takes; and says where each session's next request would
// go as auto alone says it.
func TestTheListingGlancesAtTheAccountsOnceForEachModel(t *testing.T) {
	r := newTestRouter(t, at(start), &stubProber{})
	r.state.record("work", []quota.Window{session, soonWeek}, r.state.mark())
	r.state.record("side", []quota.Window{session, laterWeek}, r.state.mark())
	for _, id := range []string{"one", "two", "three"} {
		assign(r.sessions, key{session: id, model: opus}, "", decision{account: "work", reason: reasonNew}, start)
		assign(r.sessions, key{session: id, model: haiku}, "", decision{account: "side", reason: reasonNew}, start)
	}
	looks := 0
	r.state.mu.Lock()
	family := r.state.family
	r.state.family = func(model string) string {
		looks++
		return family(model)
	}
	r.state.mu.Unlock()
	r.state.glance(opus, start)
	glance := looks

	looks = 0
	listed := r.running()
	if want := 2 * glance; looks != want {
		t.Errorf("listing three sessions of opus and haiku looked at the accounts %d times, want %d: a glance for each model", looks/glance, want/glance)
	}
	for _, s := range listed {
		for _, a := range s.Assignments {
			if want := r.scheduler.auto(s.ID, a.Model, start); a.Auto != want {
				t.Errorf("the listing says %s's %s goes to %q, auto %q", s.ID, a.Model, a.Auto, want)
			}
		}
	}
}
