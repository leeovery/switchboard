package watch

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// limitedWork is the router's document of three, work at the limit of its
// session, which holds until it resets.
func limitedWork() status.Document {
	doc := routerDocument(three()...)
	doc.Accounts[0].Limit = status.Limit{Windows: []string{"5h"}, Until: start.Add(3 * time.Hour).UTC()}
	return doc
}

// changed are the ids of the accounts whose states are picked out at now.
func (h *harness) changed() []string {
	return slices.Sorted(maps.Keys(h.model.changes.faded(h.clock.now)))
}

func TestTheFirstLookPicksOutNoState(t *testing.T) {
	h := routedHarness(t, limitedWork())
	h.start()

	if got := h.changed(); len(got) > 0 {
		t.Errorf("the first look picked out %v, want none: there's no look before it to change from", got)
	}
}

func TestAStateALookSeesChangeIsPickedOutFadingBack(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.startRouter(limitedWork())
	seen := h.tickUntilAsked()

	tests := []struct {
		name  string
		after time.Duration
		want  map[string]float64
	}{
		{name: "as the look sees it", after: 0, want: map[string]float64{"work": 0}},
		{name: "half its time on, faded an eighth", after: highlightFor / 2, want: map[string]float64{"work": 0.125}},
		{name: "its time over", after: highlightFor, want: nil},
	}
	for _, tt := range tests {
		if got := h.model.changes.faded(seen.Add(tt.after)); !maps.Equal(got, tt.want) {
			t.Errorf("%s: faded = %v, want %v", tt.name, got, tt.want)
		}
	}
	if _, ok := h.pendingFrame(); !ok {
		t.Fatal("no frames drawn as the cue fades")
	}
	h.tickUntilAsked()
	if got := h.changed(); len(got) > 0 {
		t.Errorf("the next look, seeing nothing more change, picked out %v, want none, work's faded", got)
	}
}

func TestAStateTheClockChangesIsPickedOutAsItTicks(t *testing.T) {
	spent := session(1, 10*time.Second)
	spent.Status = quota.StatusRejected
	h := newHarness(t, document(account("work", "Work", spent, week(0.5)), three()[1]))
	h.start()
	h.tickUntil(start.Add(9 * time.Second))
	if got := h.changed(); len(got) > 0 {
		t.Fatalf("before work's session resets, picked out %v, want none", got)
	}
	h.tickUntil(start.Add(11 * time.Second))
	if got, reads := h.changed(), h.source.reads; !slices.Equal(got, []string{"work"}) || reads != 1 {
		t.Errorf("once work's session reset, with %d reads, picked out %v, want work, as the second ticked, with no read since the first", reads, got)
	}
}

func TestACountdownMovingOnIsNoChange(t *testing.T) {
	h := routedHarness(t, limitedWork())
	h.start()
	h.tickUntil(start.Add(2 * time.Minute))

	if got := h.changed(); len(got) > 0 {
		t.Errorf("as work's limit counts down, picked out %v, want none: it's still at its limit", got)
	}
}

func TestAStateWhoseFirstWordsStayIsNoChange(t *testing.T) {
	tests := []struct {
		name         string
		before, then func(*status.Document)
	}{
		{
			name:   "new sessions coming to an account open",
			before: func(d *status.Document) { d.Best = "personal" },
			then:   func(d *status.Document) { d.Best = "work" },
		},
		{
			name:   "the global pin spending the cap of an account at it",
			before: func(d *status.Document) { d.Accounts[0] = reserving(d.Accounts[0], 0.8) },
			then: func(d *status.Document) {
				d.Accounts[0] = reserving(d.Accounts[0], 0.8)
				d.Pin = status.Pin{Accounts: []string{"work"}, Since: start.UTC()}
			},
		},
		{
			name:   "the time an account is back from its limit",
			before: func(d *status.Document) { *d = limitedWork() },
			then: func(d *status.Document) {
				*d = limitedWork()
				d.Accounts[0].Limit.Until = start.Add(4 * time.Hour).UTC()
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, then := routerDocument(three()...), routerDocument(three()...)
			tt.before(&before)
			tt.then(&then)
			h := routedHarness(t, before)
			policy := h.model.cfg.Policy
			if was, is := before.RoutingOf(before.Accounts[0], start, policy), then.RoutingOf(then.Accounts[0], start, policy); was.String() == is.String() {
				t.Fatalf("work stands as %q before and after, want its words changed past its first", was)
			}
			h.start()
			h.startRouter(then)
			h.tickUntilAsked()
			if got := h.changed(); len(got) > 0 {
				t.Errorf("picked out %v, want none: each account's state is said in the words it was", got)
			}
		})
	}
}

func TestAReadFailingWhileALimitHoldsPicksOutNoState(t *testing.T) {
	h := routedHarness(t, limitedWork())
	h.start()

	failing := limitedWork()
	failing.Accounts[0].Error = "timed out"
	h.startRouter(failing)
	h.tickUntilAsked()
	if got := h.changed(); len(got) > 0 {
		t.Errorf("as work's read failed, picked out %v, want none: it's still at its limit", got)
	}
	h.startRouter(limitedWork())
	h.tickUntilAsked()
	if got := h.changed(); len(got) > 0 {
		t.Errorf("as work was read again, picked out %v, want none: it's still at its limit", got)
	}
}

func TestStatesReadElsewhereStartAfresh(t *testing.T) {
	started := start.Add(-time.Hour).UTC()
	tests := []struct {
		name string
		to   func(h *harness)
		want []string
	}{
		{name: "the same router", to: func(*harness) {}, want: []string{"work"}},
		{name: "another router, as one restarted", to: func(h *harness) { h.source.health = router.Health{PID: 8, StartedAt: started} }, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.source.health = router.Health{PID: 7, StartedAt: started}
			h.start()

			tt.to(h)
			h.startRouter(limitedWork())
			h.tickUntilAsked()
			if got := h.changed(); !slices.Equal(got, tt.want) {
				t.Errorf("picked out %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatesProbedAfterTheRouterStartAfresh(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.stopRouter()
	h.source.doc = document(limitedWork().Accounts...)
	h.deliver(h.press("R")...)

	if h.model.routed() {
		t.Fatal("still showing the router's document, want the accounts probed")
	}
	if got := h.changed(); len(got) > 0 {
		t.Errorf("probing after the router, picked out %v, want none: its states start afresh", got)
	}
}
