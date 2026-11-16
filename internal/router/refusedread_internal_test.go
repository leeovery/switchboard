package router

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
)

// Why a probe read nothing, as the API answers it: an ended subscription,
// its organization's login refused, its message cut to the 60 characters a
// probe's error keeps; a token refused; and a probe that wasn't answered.
var (
	errSubscriptionEnded = errors.New("HTTP 403 · OAuth authentication is currently not allowed for this organ")
	errTokenRefused      = errors.New("HTTP 401 · Invalid bearer token")
	errTimedOut          = errors.New("timed out after 5s")
)

// readFailures are the ways a probe of work reads nothing, and whether each
// holds work back from every request till a read comes back clean: only the
// API refusing the read does.
var readFailures = []struct {
	name string
	err  error
	held bool
}{
	{name: "its subscription ended", err: errSubscriptionEnded, held: true},
	{name: "its token refused", err: errTokenRefused, held: true},
	{name: "the upstream overloaded", err: errUnreadable},
	{name: "the probe timed out", err: errTimedOut},
}

// readWorkFirst has r read work's quota and side's, work's needing using
// first, so a new session goes to work while it can take one.
func readWorkFirst(r *Router) {
	r.state.record("work", []quota.Window{session, soonWeek}, r.state.mark())
	r.state.record("side", []quota.Window{session, laterWeek}, r.state.mark())
}

// failWorksRead has a probe of work read nothing, as err says why.
func failWorksRead(r *Router, err error) {
	r.state.recordProbe("work", quota.Probe{}, err, r.state.mark(), readings.FromProbe)
}

func TestAnAccountWhoseReadWasRefusedIsNeverChosen(t *testing.T) {
	for _, tt := range readFailures {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(t, at(start), &stubProber{})
			readWorkFirst(r)
			failWorksRead(r, tt.err)
			want := "work"
			if tt.held {
				want = "side"
			}

			if got := choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"}); got.Account != want {
				t.Errorf("a new session went to %s, want %s: work's quota needs using first", got.Account, want)
			}
			if got := r.state.document().Best; got != want {
				t.Errorf("the document's best, ▲, is %q, want %q, where the router sends a new session", got, want)
			}
		})
	}
}

func TestASessionOnAnAccountWhoseReadWasRefusedMovesOff(t *testing.T) {
	for _, tt := range readFailures {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(t, at(start), &stubProber{})
			readWorkFirst(r)
			assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
			failWorksRead(r, tt.err)
			want := Choice{Account: "work", Reason: reasonSticky}
			if tt.held {
				want = Choice{Account: "side", Reason: "moved: work has no room", From: "work"}
			}

			got := choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
			if got.Account != want.Account || got.Reason != want.Reason || got.From != want.From {
				t.Errorf("the session's request went to %s, %q, from %q, want %s, %q, from %q", got.Account, got.Reason, got.From, want.Account, want.Reason, want.From)
			}
		})
	}
}

func TestAPinToAnAccountWhoseReadWasRefusedYieldsAsToOneWhoseTokenTheRouterRefused(t *testing.T) {
	tests := []struct {
		name   string
		refuse func(r *Router)
	}{
		{name: "the router refused its token", refuse: func(r *Router) { r.state.refuse("work", http.StatusUnauthorized, someRequest, r.state.mark()) }},
		{name: "its subscription ended", refuse: func(r *Router) { failWorksRead(r, errSubscriptionEnded) }},
		{name: "its token refused as it was read", refuse: func(r *Router) { failWorksRead(r, errTokenRefused) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(t, at(start), &stubProber{})
			readWorkFirst(r)
			tt.refuse(r)

			got := choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work", Pin: "work"})
			if got.Account != "side" || got.Reason != "pin yields: work has no room" {
				t.Errorf("the pinned session's request went to %s, %q, want side, as its pin to work yields", got.Account, got.Reason)
			}
		})
	}
}

func TestACleanReadBringsAnAccountWhoseReadWasRefusedBack(t *testing.T) {
	tests := []struct {
		name string
		read func(r *Router)
	}{
		{name: "a probe reads it", read: func(r *Router) {
			r.state.recordProbe("work", probed(nil, session, soonWeek), nil, r.state.mark(), readings.FromProbe)
		}},
		{name: "an answer reads it", read: func(r *Router) { r.state.record("work", []quota.Window{session, soonWeek}, r.state.mark()) }},
		{name: "it goes out on another token", read: func(r *Router) { r.state.tokenReplaced("work") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(t, at(start), &stubProber{})
			readWorkFirst(r)
			failWorksRead(r, errSubscriptionEnded)
			if got := r.state.document().Best; got != "side" {
				t.Fatalf("the document's best before work is read again is %q, want side: work's subscription ended", got)
			}
			tt.read(r)

			if got := choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"}); got.Account != "work" {
				t.Errorf("a new session went to %s, want work, back as its quota needs using first", got.Account)
			}
			if got := r.state.document().Best; got != "work" {
				t.Errorf("the document's best is %q, want work", got)
			}
		})
	}
}

func TestAPoolWhoseEveryAccountsReadWasRefusedWaitsAsOneWhoseEveryAccountIsOut(t *testing.T) {
	tests := []struct {
		name string
		// hold holds back personal, Acme's one account.
		hold func(r *Router)
	}{
		{name: "personal at a limit", hold: func(r *Router) { r.state.limit("personal", nil, start.Add(time.Hour), r.state.mark()) }},
		{name: "personal's subscription ended", hold: func(r *Router) {
			r.state.recordProbe("personal", quota.Probe{}, errSubscriptionEnded, r.state.mark(), readings.FromProbe)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newPoolsRouter(t, at(start), true)
			readPersonalFirst(r)
			tt.hold(r)

			got := choose(t.Context(), r, Request{Session: "one", Pool: acmePool, Model: opus, Client: "personal"})
			if !got.NoRoom || got.Reason != reasonNoRoom {
				t.Errorf("a new session in Acme went to %q, %q, no room %v, want no account to have room", got.Account, got.Reason, got.NoRoom)
			}
			if best := r.state.documentOf(r.readOf(acmePool), nil).Best; best != "" {
				t.Errorf("Acme's document's best is %q, want none", best)
			}
		})
	}
}

func TestAnAccountWhoseReadWasRefusedIsReadOnItsRoundsAndPrimed(t *testing.T) {
	for _, tt := range readFailures {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: onDay(1, 0, 20)}
			r := newPrimingRouter(t, clock.read, &stubProber{}, daytime)
			// Side's slot is 06:40. Its session, as last read at 00:20, ran
			// till 00:30, and has lapsed since: a probe of it on the rounds
			// within five hours of its slot would start its session off the
			// schedule, but for one that can take no request anyway.
			lapsing := quota.Window{Key: "5h", Label: "Session", Utilization: 0.4, ResetsAt: onDay(1, 0, 30).UTC()}
			r.state.record("side", []quota.Window{lapsing, week}, r.state.mark())
			r.state.recordProbe("side", quota.Probe{}, tt.err, r.state.mark(), readings.FromProbe)
			clock.now = onDay(1, 2, 0)

			if due := r.state.rounding(r.rounds.clears)("side", clock.now); due != tt.held {
				t.Errorf("side due on the router's rounds within five hours of its slot = %v, want %v", due, tt.held)
			}
			clock.now = afterResets(onDay(1, 6, 40), 1)
			if !r.primer.due("side", clock.now) {
				t.Error("side isn't due a prime at its slot, want it primed, which reads it again")
			}
		})
	}
}

func TestTheRoutersDocumentGivesTheErrorItsReadWasRefusedWith(t *testing.T) {
	r := newTestRouter(t, at(start), &stubProber{})
	readWorkFirst(r)
	failWorksRead(r, errSubscriptionEnded)

	work, _ := r.state.document().Account("work")
	if got := status.RefusedRead(work.Error); got != status.StaleLapsed {
		t.Errorf("status reads work's error, %q, as %d, want StaleLapsed: status and the router judge it alike", work.Error, got)
	}
}

func TestARefreshReadsAnAccountWhoseReadWasRefusedHoweverLatelyItWasRead(t *testing.T) {
	for _, tt := range readFailures {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			s := newTestState(clock)
			s.record("work", []quota.Window{session, soonWeek}, s.mark())
			s.recordProbe("work", quota.Probe{}, tt.err, s.mark(), readings.FromProbe)

			if due := s.refreshing(30*time.Minute)("work", start.Add(reprobeAfter)); due != tt.held {
				t.Errorf("work due a refresh asking for usage no older than 30 minutes, a minute after it was read = %v, want %v", due, tt.held)
			}
		})
	}
}
