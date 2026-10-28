package router

import (
	"net/http"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

const spareToken = "test-token-spare"

// threeTokens are the tokens of work, side and spare.
var threeTokens = tokenstest.Files{"work": workToken, "side": sideToken, "spare": spareToken}

// newThreeRouter builds a router of work, side and spare, each with a token,
// in that order, each keeping back the share of its windows reserves gives
// it, on clock's time, probing with prober.
func newThreeRouter(t *testing.T, clock *testClock, prober Prober, reserves map[string]float64) *Router {
	t.Helper()
	r, err := New(Config{
		Accounts: threeAccounts(reserves),
		Token:    threeTokens.Read,
		Upstream: "http://127.0.0.1:1",
		Provider: claude.Provider{},
		Prober:   prober,
		Policy:   testPolicy,
		Now:      clock.read,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return r
}

// threeAccounts are work, side and spare, as configured, each keeping back the
// share of its windows reserves gives it.
func threeAccounts(reserves map[string]float64) config.Accounts {
	return config.Accounts{
		{ID: "work", Label: "Work", Reserve: reserves["work"]},
		{ID: "side", Label: "Side", Reserve: reserves["side"]},
		{ID: "spare", Label: "Spare", Reserve: reserves["spare"]},
	}
}

// newSession is the account the router sends a new session's first request
// of model to: "" where no account has room for it, and it goes out only for
// the upstream to say why, or on none.
func newSession(t *testing.T, r *Router, model string) string {
	t.Helper()
	c := choose(t.Context(), r, Request{ID: "first-" + model, Session: "new-" + model, Model: model, Client: "work"})
	if c.NoRoom {
		return ""
	}
	return c.Account
}

// sessionIn is a 5-hour window used as given that resets resetsIn after start.
func sessionIn(utilization float64, resetsIn time.Duration) quota.Window {
	return quota.Window{Key: "5h", Label: "Session", Utilization: utilization, ResetsAt: start.Add(resetsIn)}
}

// weekIn is a week every model shares, used as given, that resets resetsIn
// after start.
func weekIn(utilization float64, resetsIn time.Duration) quota.Window {
	return quota.Window{Key: "7d", Label: "Week", Utilization: utilization, ResetsAt: start.Add(resetsIn)}
}

// The document's best is the router's own choice for a new session's
// request that only what holds back every model holds back: the windows every
// model shares count it, and a limit in one of them, or in none named, or a
// refusal of the account's token, holds it back. Haiku's is one here: no
// window of its own has been seen, and nothing refuses its family. A request
// of a model held back on its own, by its own window or its family's
// refusal, can go elsewhere.
func TestTheBestIsWhereTheRouterSendsANewSession(t *testing.T) {
	// Spare's week resets first, then side's, then work's: unpinned, spare's
	// quota most needs using.
	read := map[string][]quota.Window{
		"work":  {sessionIn(0.2, 3*time.Hour), weekIn(0.5, 5*day)},
		"side":  {sessionIn(0.2, 3*time.Hour), weekIn(0.5, 3*day)},
		"spare": {sessionIn(0.2, 3*time.Hour), weekIn(0.5, day)},
	}
	spentFableWeek := fableWeek
	spentFableWeek.Utilization, spentFableWeek.Status = 1, quota.StatusRejected
	pin := func(r *Router, ids ...string) {
		r.sessions.setPin(status.Pin{Accounts: ids, Since: start}, false)
	}
	tests := []struct {
		name     string
		reserves map[string]float64
		// change changes what the router knows of the accounts, as read,
		// before the best is asked for and a new session is sent.
		change func(r *Router, clock *testClock)
		want   string
		// heldOwn is where a new session of a model held back on its own goes
		// instead, by model.
		heldOwn map[string]string
	}{
		{name: "every account open", want: "spare"},
		{
			name: "one under pressure, its session running out at its rate before it resets",
			change: func(r *Router, clock *testClock) {
				clock.now = start.Add(20 * time.Minute)
				r.state.record("spare", []quota.Window{sessionIn(0.7, 3*time.Hour)}, r.state.mark())
			},
			want: "side",
		},
		{
			name:     "one at its cap",
			reserves: map[string]float64{"spare": 0.1},
			change: func(r *Router, _ *testClock) {
				r.state.record("spare", []quota.Window{weekIn(0.92, day)}, r.state.mark())
			},
			want: "side",
		},
		{
			name: "one shut by a limit in a window every model shares",
			change: func(r *Router, _ *testClock) {
				r.state.limit("spare", []string{"5h"}, start.Add(time.Hour), r.state.mark())
			},
			want: "side",
		},
		{
			name:   "one shut by a limit naming no window",
			change: func(r *Router, _ *testClock) { r.state.limit("spare", nil, start.Add(time.Hour), r.state.mark()) },
			want:   "side",
		},
		{
			name: "its token refused",
			change: func(r *Router, _ *testClock) {
				r.state.refuse("spare", http.StatusUnauthorized, someRequest, r.state.mark())
			},
			want: "side",
		},
		{
			name:   "a global pin of one account",
			change: func(r *Router, _ *testClock) { pin(r, "work") },
			want:   "work",
		},
		{
			name:   "a global pin of two accounts, the best of them",
			change: func(r *Router, _ *testClock) { pin(r, "work", "side") },
			want:   "side",
		},
		{
			name: "a global pin of two, the better under pressure",
			change: func(r *Router, clock *testClock) {
				pin(r, "work", "side")
				clock.now = start.Add(20 * time.Minute)
				r.state.record("side", []quota.Window{sessionIn(0.7, 3*time.Hour)}, r.state.mark())
			},
			want: "work",
		},
		{
			name:     "a global pin of one at its cap, which the pin spends",
			reserves: map[string]float64{"work": 0.1},
			change: func(r *Router, _ *testClock) {
				r.state.record("work", []quota.Window{weekIn(0.95, 5*day)}, r.state.mark())
				pin(r, "work")
			},
			want: "work",
		},
		{
			name:     "a global pin of two, the better at its cap, which the pin spends",
			reserves: map[string]float64{"work": 0.1},
			change: func(r *Router, _ *testClock) {
				r.state.record("work", []quota.Window{weekIn(0.9, 10*time.Hour)}, r.state.mark())
				pin(r, "work", "side")
			},
			want: "work",
		},
		{
			name: "the pin's accounts all unable, the best of every account",
			change: func(r *Router, _ *testClock) {
				pin(r, "work", "side")
				r.state.refuse("work", http.StatusUnauthorized, someRequest, r.state.mark())
				r.state.limit("side", nil, start.Add(time.Hour), r.state.mark())
			},
			want: "spare",
		},
		{
			name: "the pin's accounts none scored, the first with room",
			change: func(r *Router, _ *testClock) {
				pin(r, "work", "side")
				r.state.record("work", []quota.Window{{Key: "7d", Label: "Week", Utilization: 1, Status: quota.StatusRejected}}, r.state.mark())
				r.state.record("side", []quota.Window{{Key: "7d", Label: "Week", Utilization: 0.5}}, r.state.mark())
			},
			want: "side",
		},
		{
			name:     "reserves set, judging each by the room before its reserve",
			reserves: map[string]float64{"spare": 0.45},
			want:     "side",
		},
		{
			name: "a model's own window holding one account",
			change: func(r *Router, _ *testClock) {
				r.state.learn(fable, []quota.Window{fableWeek})
				r.state.record("spare", []quota.Window{spentFableWeek}, r.state.mark())
				r.state.limit("spare", []string{"7d_oi"}, start.Add(time.Hour), r.state.mark())
			},
			want:    "spare",
			heldOwn: map[string]string{fable: "side"},
		},
		{
			name:    "a model's family refused on one account",
			change:  func(r *Router, _ *testClock) { r.state.forbid("spare", "opus", http.StatusForbidden, someRequest) },
			want:    "spare",
			heldOwn: map[string]string{opus: "side"},
		},
		{
			name: "no account able at all",
			change: func(r *Router, _ *testClock) {
				r.state.refuse("work", http.StatusUnauthorized, someRequest, r.state.mark())
				r.state.limit("side", []string{"7d"}, start.Add(time.Hour), r.state.mark())
				r.state.record("spare", []quota.Window{weekIn(1, day)}, r.state.mark())
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			r := newThreeRouter(t, clock, &stubProber{}, tt.reserves)
			for _, id := range []string{"work", "side", "spare"} {
				r.state.record(id, read[id], r.state.mark())
			}
			if tt.change != nil {
				tt.change(r, clock)
			}

			best := r.Status().Best
			if got := newSession(t, r, haiku); got != tt.want || best != got {
				t.Errorf("a new session goes to %q, and best is %q; want both %q", got, best, tt.want)
			}
			for model, want := range tt.heldOwn {
				if got := newSession(t, r, model); got != want {
					t.Errorf("a new session of %s, held back on its own, goes to %q, want %q, though best is %q", model, got, want, best)
				}
			}
		})
	}
}

// A document built by probing, with no router, gives as its best where a
// router that had read the accounts only as the probes did would send a new
// session: no pin, no limit and no refusal known, and each account judged
// for pressure at its use since its session started, which is all its
// reading says of its pace.
func TestTheBestOfADocumentBuiltByProbingIsWhereANewSessionGoes(t *testing.T) {
	// Work's week resets first, but its session, three hours in, is 70% used,
	// and at that pace runs out before it resets in two hours. Side's runs at
	// a tenth an hour, an hour in.
	shared := map[string][]string{"5h": {haiku}, "7d": {haiku}}
	tests := []struct {
		name  string
		reads map[string]quota.Probe
		want  string
	}{
		{
			name: "every account open",
			reads: map[string]quota.Probe{
				workToken:  probed(shared, sessionIn(0.2, 3*time.Hour), weekIn(0.5, day)),
				sideToken:  probed(shared, sessionIn(0.1, 4*time.Hour), weekIn(0.5, 5*day)),
				spareToken: probed(shared, sessionIn(0.1, 4*time.Hour), weekIn(0.5, 6*day)),
			},
			want: "work",
		},
		{
			name: "the best under pressure at its use since its session started",
			reads: map[string]quota.Probe{
				workToken:  probed(shared, sessionIn(0.7, 2*time.Hour), weekIn(0.5, day)),
				sideToken:  probed(shared, sessionIn(0.1, 4*time.Hour), weekIn(0.5, 5*day)),
				spareToken: probed(shared, sessionIn(0.1, 4*time.Hour), weekIn(0.5, 6*day)),
			},
			want: "side",
		},
		{
			name: "none read",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			prober := &stubProber{readings: tt.reads}
			collector := status.Collector{Prober: prober, Policy: testPolicy, Token: threeTokens.Read, Taken: tokenstest.Taken{}.Read, Now: clock.read}
			best := collector.Collect(t.Context(), threeAccounts(nil)).Best

			r := newThreeRouter(t, clock, prober, nil)
			if got := newSession(t, r, haiku); got != tt.want || best != got {
				t.Errorf("a new session goes to %q, and the probed document's best is %q; want both %q", got, best, tt.want)
			}
		})
	}
}
