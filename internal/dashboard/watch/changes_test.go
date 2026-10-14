package watch

import (
	"cmp"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
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

// worded are the ids of the accounts whose states in a word are picked out
// at now.
func (h *harness) worded() []string {
	return slices.Sorted(maps.Keys(h.model.changes.wordFaded(h.clock.now)))
}

// An account's state in a word, as Accounts says it, is picked out only
// where it turns: a session lapsing on the clock turns ROUTING's state to
// idle, but the account is open in a word still; reaching its cap turns
// both.
func TestAnAccountsStateInAWordIsPickedOutOnlyWhereItTurns(t *testing.T) {
	h := newHarness(t, document(account("work", "Work", session(0.4, 10*time.Second), week(0.5)), three()[1]))
	h.start()
	h.tickUntil(start.Add(11 * time.Second))
	if routing, worded := h.changed(), h.worded(); !slices.Equal(routing, []string{"work"}) || len(worded) > 0 {
		t.Errorf("as work's session lapsed, picked out %v in ROUTING and %v in a word, want work and none", routing, worded)
	}

	capped := three()
	capped[0] = reserving(account("work", "Work", session(0.97, 3*time.Hour), week(0.5)), 0.05)
	h = routedHarness(t, routerDocument(three()...))
	h.start()
	h.startRouter(routerDocument(capped...))
	h.tickUntilAsked()
	if routing, worded := h.changed(), h.worded(); !slices.Equal(routing, []string{"work"}) || !slices.Equal(worded, []string{"work"}) {
		t.Errorf("as work reached its cap, picked out %v in ROUTING and %v in a word, want work in both", routing, worded)
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
			if was, is := routingAt(before, start, policy), routingAt(then, start, policy); was.String() == is.String() {
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

func TestARefusalOfOneFamilyTurningToTheTokensIsPickedOut(t *testing.T) {
	refusedBy := func(family string) status.Document {
		doc := routerDocument(three()...)
		doc.Accounts[0].Refused = status.Refusal{Until: start.Add(time.Hour).UTC(), Status: 403, Family: family}
		return doc
	}
	h := routedHarness(t, refusedBy("opus"))
	h.start()
	h.startRouter(refusedBy(""))
	h.tickUntilAsked()

	if got := h.changed(); !slices.Equal(got, []string{"work"}) {
		t.Errorf("as work's token was refused, after one family's requests, picked out %v, want work", got)
	}
}

func TestAStatesCueComparesItsKindAndItsWords(t *testing.T) {
	routing := func(kind status.RoutingKind, says string) status.Routing {
		return status.Routing{Kind: kind, Says: says, Clauses: []string{"back 15:54"}}
	}
	tests := []struct {
		name    string
		was, is status.Routing
		turned  bool
	}{
		{name: "another kind in the same words", was: routing(status.RoutingModelRefused, "refused (403)"), is: routing(status.RoutingRefused, "refused (403)"), turned: true},
		{name: "the same kind in other words", was: routing(status.RoutingCapped, "at its cap"), is: routing(status.RoutingCapped, "Fable wk at its cap"), turned: true},
		{name: "the same kind and words, its clauses moving on", was: routing(status.RoutingLimited, "limit reached"), is: status.Routing{Kind: status.RoutingLimited, Says: "limit reached"}},
		{name: "open, new sessions coming to it", was: routing(status.RoutingOpen, "open"), is: routing(status.RoutingNext, "open")},
		{name: "at its cap, the pin spending it", was: routing(status.RoutingCapped, "at its cap"), is: routing(status.RoutingCappedPinned, "at its cap")},
	}
	for _, tt := range tests {
		if got := stateOf(tt.was) != stateOf(tt.is); got != tt.turned {
			t.Errorf("%s: turned = %v, want %v", tt.name, got, tt.turned)
		}
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

// workWeek is work's week, as a window's cue is keyed.
var workWeek = dashboard.Ref{Account: "work", Window: "7d"}

// runningOut is three, work's week used lately fast enough to run out ~10
// hours on, long before it resets.
func runningOut() []status.Account {
	accounts := three()
	accounts[0].Rates = []status.Rate{{Window: "7d", Rate: 0.05, Since: start.Add(-30 * time.Minute).UTC()}}
	return accounts
}

// turnedOut are the windows whose words are picked out at now.
func (h *harness) turnedOut() []dashboard.Ref {
	return slices.SortedFunc(maps.Keys(h.model.changes.turnedOut(h.clock.now)), func(a, b dashboard.Ref) int {
		return cmp.Or(cmp.Compare(a.Account, b.Account), cmp.Compare(a.Window, b.Window))
	})
}

func TestTheFirstLookPicksOutNoWindowsWords(t *testing.T) {
	h := routedHarness(t, routerDocument(runningOut()...))
	h.start()

	if got := h.turnedOut(); len(got) > 0 {
		t.Errorf("the first look picked out %v, want none: there's no look before it to turn from", got)
	}
}

func TestAWindowsWordsALookSeesTurnToOutArePickedOutFadingBack(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.startRouter(routerDocument(runningOut()...))
	seen := h.tickUntilAsked()

	tests := []struct {
		name  string
		after time.Duration
		want  map[dashboard.Ref]float64
	}{
		{name: "as the look sees it", after: 0, want: map[dashboard.Ref]float64{workWeek: 0}},
		{name: "half its time on, faded an eighth", after: highlightFor / 2, want: map[dashboard.Ref]float64{workWeek: 0.125}},
		{name: "its time over", after: highlightFor, want: nil},
	}
	for _, tt := range tests {
		if got := h.model.changes.turnedOut(seen.Add(tt.after)); !maps.Equal(got, tt.want) {
			t.Errorf("%s: turnedOut = %v, want %v", tt.name, got, tt.want)
		}
	}
	if got := h.model.frame(seen).TurnedOut; !maps.Equal(got, map[dashboard.Ref]float64{workWeek: 0}) {
		t.Errorf("the frame drawn as the look sees it cues %v, want work's week", got)
	}
	if got := h.changed(); len(got) > 0 {
		t.Errorf("picked out the states of %v, want none: work's state's first words stay", got)
	}
	if _, ok := h.pendingFrame(); !ok {
		t.Fatal("no frames drawn as the cue fades")
	}
}

func TestAWindowsWordsCueDrawsFramesOnlyOnScreen(t *testing.T) {
	// runsOut has an account's week used lately fast enough to run out long
	// before it resets, its bar and its numbers as they were.
	runsOut := func(a *status.Account) {
		a.Rates = []status.Rate{{Window: "7d", Rate: 0.05, Since: start.Add(-30 * time.Minute).UTC()}}
	}
	h := tenOnAShortScreen(t, tenWith(runsOut))
	for _, read := range []struct {
		name   string
		moved  []string
		words  string
		frames bool
	}{
		{name: "a read cueing words scrolled out of view", moved: []string{"studio"}, words: "studio"},
		{name: "a read cueing words on screen", moved: []string{"studio", "work"}, words: "work", frames: true},
	} {
		h.startRouter(tenWith(runsOut, read.moved...))
		seen := h.tickUntilAsked()
		if _, ok := h.model.changes.turnedOut(seen)[dashboard.Ref{Account: read.words, Window: "7d"}]; !ok {
			t.Fatalf("%s: %s's week's words aren't cued, want them turned to out ~", read.name, read.words)
		}
		if eased, bright := h.model.ease.eased(h.model.frame(seen), h.model.doc, seen), h.brightened(); len(eased)+len(bright) > 0 {
			t.Fatalf("%s: the look eased %v and brightened %v, want the cue alone", read.name, eased, bright)
		}
		if _, framing := h.pendingFrame(); framing != read.frames {
			t.Errorf("%s: a frame is armed: %t, want %t", read.name, framing, read.frames)
		}
	}
}

func TestWindowsWordsThatDontTurnToOutArentPickedOut(t *testing.T) {
	tests := []struct {
		name         string
		before, then []status.Account
	}{
		{name: "out ~ still, as it was", before: runningOut(), then: runningOut()},
		{name: "turning from out ~", before: runningOut(), then: three()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routerDocument(tt.before...))
			h.start()
			h.startRouter(routerDocument(tt.then...))
			h.tickUntilAsked()
			if got := h.turnedOut(); len(got) > 0 {
				t.Errorf("picked out %v, want none: no window's words turned to out ~", got)
			}
		})
	}
}

func TestAWindowsWordsTheClockTurnsToOutArePickedOutAsItTicks(t *testing.T) {
	accounts := runningOut()
	accounts[0].Limit = status.Limit{Windows: []string{"7d"}, Until: start.Add(10 * time.Second).UTC()}
	h := newHarness(t, document(accounts...))
	h.start()
	h.tickUntil(start.Add(9 * time.Second))
	if got := h.turnedOut(); len(got) > 0 {
		t.Fatalf("while work's week is at its limit, picked out %v, want none", got)
	}
	h.tickUntil(start.Add(11 * time.Second))
	if got, reads := h.turnedOut(), h.source.reads; !slices.Equal(got, []dashboard.Ref{workWeek}) || reads != 1 {
		t.Errorf("once work's limit lifted, with %d reads, picked out %v, want work's week, out ~ as the second ticked, with no read since the first", reads, got)
	}
}

// At each hard moment, the cues on an account's state and a window's words
// compare the moments of the two looks they come between, each look's facts
// its own moment's: a look as the document was built, then the clock ticking
// on to the hard moment, picks out each account whose state differs between
// the two moments, and each window whose words turn to out ~ between them.
func TestTheCuesCompareTheMomentsOfTheirTwoLooksAtEachHardMoment(t *testing.T) {
	for _, hm := range statustest.HardMoments(policy) {
		t.Run(hm.Name, func(t *testing.T) {
			built := hm.Doc.GeneratedAt
			was, is := hm.Doc.At(built, policy), hm.Doc.At(hm.Now, policy)
			c := changes{}.looked(was, router.Health{}).ticked(is)
			var states []string
			outs := map[dashboard.Ref]float64{}
			for i, a := range is.Accounts {
				then := was.Accounts[i]
				if stateOf(a.Routing) != stateOf(then.Routing) {
					states = append(states, a.ID)
				}
				for j, w := range a.Windows {
					if w.RunsOut && !then.Windows[j].RunsOut {
						outs[dashboard.Ref{Account: a.ID, Window: w.Key}] = 0
					}
				}
			}
			slices.Sort(states)
			if got := slices.Sorted(maps.Keys(c.faded(hm.Now))); !slices.Equal(got, states) {
				t.Errorf("states picked out: %v, want %v, as the two moments' differ", got, states)
			}
			if got := c.turnedOut(hm.Now); !maps.Equal(got, outs) {
				t.Errorf("windows' words picked out: %v, want %v, as they turn to out ~ between the two moments", got, outs)
			}
		})
	}
}

// routingAt is how doc's first account stands with the router at now, as the
// document's moment then has it, as policy judges it.
func routingAt(doc status.Document, now time.Time, policy score.Policy) status.Routing {
	return doc.At(now, policy).Accounts[0].Routing
}
