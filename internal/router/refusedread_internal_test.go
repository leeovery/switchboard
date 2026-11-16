package router

import (
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
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

// readFailures are the ways an account's probes end, in order, and whether
// they leave it held back from every request: only the API refusing a read
// holds it, till a read comes back clean, however many fail otherwise after.
var readFailures = []struct {
	name string
	// reads are how the probes end, in order: each error why one read
	// nothing, and nil one that came back clean.
	reads []error
	held  bool
}{
	{name: "its subscription ended", reads: []error{errSubscriptionEnded}, held: true},
	{name: "its token refused", reads: []error{errTokenRefused}, held: true},
	{name: "the upstream overloaded", reads: []error{errUnreadable}},
	{name: "the probe timed out", reads: []error{errTimedOut}},
	{name: "its subscription ended, then a probe timed out", reads: []error{errSubscriptionEnded, errTimedOut}, held: true},
	{name: "its subscription ended, then the upstream overloaded", reads: []error{errSubscriptionEnded, errUnreadable}, held: true},
	{name: "its subscription ended, then its token refused", reads: []error{errSubscriptionEnded, errTokenRefused}, held: true},
	{name: "its subscription ended, then a read came back clean", reads: []error{errSubscriptionEnded, nil}},
}

// readWorkFirst has r read work's quota and side's, work's needing using
// first, so a new session goes to work while it can take one.
func readWorkFirst(r *Router) {
	r.state.record("work", []quota.Window{session, soonWeek}, r.state.mark())
	r.state.record("side", []quota.Window{session, laterWeek}, r.state.mark())
}

// probesEnd has the probes of the account with the given id end as reads says,
// in order: each error why one read nothing, and nil one that came back
// clean, reading its windows as they stand.
func probesEnd(s *state, id string, reads []error) {
	for _, err := range reads {
		var read quota.Probe
		if err == nil {
			read = probed(nil, s.usage[id].latest()...)
		}
		s.recordProbe(id, read, err, s.mark(), readings.FromProbe)
	}
}

// failWorksRead has a probe of work read nothing, as err says why.
func failWorksRead(r *Router, err error) {
	probesEnd(r.state, "work", []error{err})
}

func TestAnAccountWhoseReadWasRefusedIsNeverChosen(t *testing.T) {
	for _, tt := range readFailures {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(t, at(start), &stubProber{})
			readWorkFirst(r)
			probesEnd(r.state, "work", tt.reads)
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
			probesEnd(r.state, "work", tt.reads)
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

// A pool whose every account is held has no room: a request in it goes out on
// the client's account, for the upstream to say why, as where its every
// account is at a limit.
func TestARequestInAPoolWhoseEveryAccountsReadWasRefusedGoesOutOnTheClientsWithNoRoom(t *testing.T) {
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
			if got.Account != "personal" || !got.NoRoom || got.Reason != reasonNoRoom {
				t.Errorf("a new session in Acme went to %q, %q, no room %v, want personal, the client's, with no account to have room", got.Account, got.Reason, got.NoRoom)
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
			probesEnd(r.state, "side", tt.reads)
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
			probesEnd(s, "work", tt.reads)

			if due := s.refreshing(30*time.Minute)("work", start.Add(reprobeAfter)); due != tt.held {
				t.Errorf("work due a refresh asking for usage no older than 30 minutes, a minute after it was read = %v, want %v", due, tt.held)
			}
		})
	}
}

// spacings are why three probes of an account in a row read nothing, and
// whether that spaces off a choice's probes and its primes as its rounds'
// backoff spaces theirs: only a read refused does.
var spacings = []struct {
	name   string
	err    error
	spaced bool
}{
	{name: "its subscription ended", err: errSubscriptionEnded, spaced: true},
	{name: "its token refused", err: errTokenRefused, spaced: true},
	{name: "the probe timed out", err: errTimedOut},
}

func TestAChoiceProbesAnAccountWhoseReadWasRefusedNoSoonerThanItsRounds(t *testing.T) {
	for _, tt := range spacings {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			s := newTestState(clock)
			s.record("work", []quota.Window{session, soonWeek}, s.mark())
			clock.now = start.Add(staleAfter + time.Minute)
			probesEnd(s, "work", []error{tt.err, tt.err, tt.err})
			sooner, backedOff := clock.now.Add(5*time.Minute), clock.now.Add(retryAfter(3))

			for _, ask := range []struct {
				name string
				due  func(id string, now time.Time) bool
			}{{"before a choice", s.due}, {"with no room", s.dueAgain}} {
				if due := ask.due("work", sooner); due == tt.spaced {
					t.Errorf("work due a probe %s 5 minutes after its third failed = %v, want %v", ask.name, due, !tt.spaced)
				}
				if !ask.due("work", backedOff) {
					t.Errorf("work not due a probe %s once its rounds' backoff has passed, want it due", ask.name)
				}
			}
		})
	}
}

func TestAPrimeOfAnAccountWhoseReadWasRefusedWaitsOutItsRoundsBackoff(t *testing.T) {
	for _, tt := range spacings {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: onDay(1, 0, 20)}
			r := newPrimingRouter(t, clock.read, &stubProber{}, daytime)
			// Side's slot is 06:40, and its session, as last read at 00:20,
			// ran till 00:30.
			lapsing := quota.Window{Key: "5h", Label: "Session", Utilization: 0.4, ResetsAt: onDay(1, 0, 30).UTC()}
			r.state.record("side", []quota.Window{lapsing, week}, r.state.mark())
			clock.now = afterResets(onDay(1, 6, 40), 1)
			for range 3 {
				r.state.recordProbe("side", quota.Probe{}, tt.err, r.state.mark(), readings.FromPrime)
			}

			if due := r.primer.due("side", clock.now.Add(reprimeAfter+time.Minute)); due == tt.spaced {
				t.Errorf("side due a prime a minute past the wait after a failed prime = %v, want %v", due, !tt.spaced)
			}
			if !r.primer.due("side", clock.now.Add(retryAfter(3))) {
				t.Error("side not due a prime once its rounds' backoff has passed, want it primed")
			}
		})
	}
}

func TestARestartHoldsAnAccountByItsReadRefusedOnlyFromWhenItWasRefused(t *testing.T) {
	// Work keeps a tenth back, and nothing else is read: its week reads 50%,
	// and two hours on its subscription ends. Three hours on, the router
	// restarts.
	path := filepath.Join(t.TempDir(), "state.json")
	clock := &testClock{now: start}
	before := newTestFile(clock.read, resolve(workReserved(), testTokens.Read, unrecorded))
	before.load(path)
	before.state.record("work", []quota.Window{weekUsed(0.5)}, before.state.mark())
	clock.now = start.Add(2 * time.Hour)
	before.state.recordProbe("work", quota.Probe{}, errSubscriptionEnded, before.state.mark(), readings.FromProbe)
	before.save()
	now := start.Add(3 * time.Hour)
	h := newHistory(config.History{}, at(now))
	h.open(t.TempDir())
	writeLines(t, h, workReads(start, 0.5, week.ResetsAt))

	after := newTestFile(at(now), resolve(workReserved(), testTokens.Read, unrecorded))
	after.load(path)
	after.state.seed(h.readBack(now))
	if got := after.state.document().Demand; got == nil || got.Hours != 2 {
		t.Errorf("after the restart, the pool's demand is %+v, want it measured over 2 hours: room on work from its reading till its subscription ended", got)
	}
	if roomAt(after.state, now) {
		t.Error("after the restart, the pool has room, want none: work's subscription has ended")
	}
}
