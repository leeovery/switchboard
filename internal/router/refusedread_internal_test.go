package router

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

// Why a probe read nothing, as the provider gives it: the API refusing an
// ended subscription's login, and a token; a proxy between refusing a token
// in the API's place, its text a refused token's, which says nothing of the
// account; and a probe that wasn't answered.
var (
	errSubscriptionEnded = claudetest.ErrSubscriptionEnded
	errTokenRefused      = claudetest.ErrTokenRefused
	errProxyRefused      = errors.New(claudetest.ErrTokenRefused.Error())
	errTimedOut          = errors.New("timed out after 5s")
)

// readFailures are the ways an account's probes end, in order, and why they
// leave it held back from every request, as status.RefusedRead says, zero
// where they don't: only the API refusing a read holds it, as the probe's
// failure says, never its text, till a read comes back clean, however many
// fail otherwise after, the last refusal saying why.
var readFailures = []struct {
	name string
	// reads are how the probes end, in order: each error why one read
	// nothing, and nil one that came back clean.
	reads []error
	held  status.StaleCause
}{
	{name: "its subscription ended", reads: []error{errSubscriptionEnded}, held: status.StaleLapsed},
	{name: "its token refused", reads: []error{errTokenRefused}, held: status.StaleRefused},
	{name: "the upstream overloaded", reads: []error{errUnreadable}},
	{name: "the probe timed out", reads: []error{errTimedOut}},
	{name: "a proxy between refusing its token", reads: []error{errProxyRefused}},
	{name: "its subscription ended, then a proxy between refused its token", reads: []error{errSubscriptionEnded, errProxyRefused}, held: status.StaleLapsed},
	{name: "its subscription ended, then a probe timed out", reads: []error{errSubscriptionEnded, errTimedOut}, held: status.StaleLapsed},
	{name: "its subscription ended, then the upstream overloaded", reads: []error{errSubscriptionEnded, errUnreadable}, held: status.StaleLapsed},
	{name: "its subscription ended, then its token refused", reads: []error{errSubscriptionEnded, errTokenRefused}, held: status.StaleRefused},
	{name: "its token refused, then its subscription ended", reads: []error{errTokenRefused, errSubscriptionEnded}, held: status.StaleLapsed},
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
			if tt.held != 0 {
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
			if tt.held != 0 {
				want = Choice{Account: "side", Reason: "moved: work had its read refused", From: "work"}
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
		want   string
	}{
		{name: "the router refused its token", refuse: func(r *Router) { r.state.refuse("work", http.StatusUnauthorized, someRequest, r.state.mark()) }, want: "pin yields: work has no room"},
		{name: "its subscription ended", refuse: func(r *Router) { failWorksRead(r, errSubscriptionEnded) }, want: "pin yields: work had its read refused"},
		{name: "its token refused as it was read", refuse: func(r *Router) { failWorksRead(r, errTokenRefused) }, want: "pin yields: work had its read refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(t, at(start), &stubProber{})
			readWorkFirst(r)
			tt.refuse(r)

			got := choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work", Pin: "work"})
			if got.Account != "side" || got.Reason != tt.want {
				t.Errorf("the pinned session's request went to %s, %q, want side, %q, as its pin to work yields", got.Account, got.Reason, tt.want)
			}
		})
	}
}

// A session moves off an account whose read was refused saying so, whatever
// else holds it back, as a limit reached before the request, which alone
// would be no room: the refusal is said first, as status says it, and
// lasts till a read comes back clean.
func TestAMoveOffAnAccountWhoseReadWasRefusedSaysSoThoughALimitHoldsItToo(t *testing.T) {
	tests := []struct {
		name  string
		reads []error
		want  string
	}{
		{name: "a limit alone", want: "moved: work has no room"},
		{name: "a limit, its subscription ended", reads: []error{errSubscriptionEnded}, want: "moved: work had its read refused"},
		{name: "a limit, its token refused as it was read", reads: []error{errTokenRefused}, want: "moved: work had its read refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(t, at(start), &stubProber{})
			readWorkFirst(r)
			assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
			r.state.limit("work", nil, start.Add(time.Hour), r.state.mark())
			probesEnd(r.state, "work", tt.reads)

			got := choose(t.Context(), r, Request{Session: "one", Model: opus, Client: "work"})
			if got.Account != "side" || got.Reason != tt.want || got.From != "work" {
				t.Errorf("the session's request went to %s, %q, from %q, want side, %q, from work", got.Account, got.Reason, got.From, tt.want)
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

func TestARequestOnTheClientsAccountWhoseReadWasRefusedIsAnsweredWithTheUpstreamsRefusalAsA502(t *testing.T) {
	// Personal, Acme's one account, the client's, was refused its read, its
	// subscription ended, and the upstream refuses the request on it alike.
	r := newPoolsRouter(t, at(start), true)
	readPersonalFirst(r)
	probesEnd(r.state, "personal", []error{errSubscriptionEnded})
	upstream := servingAll(served, forbidden, served)
	r.proxy.transport = upstream

	w := routeIn(t, r, personalToken, "one", acmePool, nil)
	if w.Code != http.StatusBadGateway || w.Header().Get("X-Should-Retry") != "false" {
		t.Errorf("the request was answered %d, retry %q, want 502, not to be retried: Claude Code takes a 403 as its own login failing", w.Code, w.Header().Get("X-Should-Retry"))
	}
	want := `"switchboard: the upstream refused account personal (HTTP 403): This model isn't on your plan"`
	if body := w.Body.String(); !strings.Contains(body, `"api_error"`) || !strings.Contains(body, want) {
		t.Errorf("the request was answered %s, want switchboard's api error saying %s", body, want)
	}
	if sent := upstream.sent(); !slices.Equal(sent, []string{"personal"}) {
		t.Errorf("the request went out on %q, want personal alone, for the upstream to say why", sent)
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

			if due, want := r.state.rounding(r.rounds.clears)("side", clock.now), tt.held != 0; due != want {
				t.Errorf("side due on the router's rounds within five hours of its slot = %v, want %v", due, want)
			}
			clock.now = afterResets(onDay(1, 6, 40), 1)
			if !r.primer.due("side", clock.now) {
				t.Error("side isn't due a prime at its slot, want it primed, which reads it again")
			}
		})
	}
}

func TestTheRoutersDocumentSaysWhyItsReadWasRefusedAsTheLastRefusalDid(t *testing.T) {
	for _, tt := range readFailures {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(t, at(start), &stubProber{})
			readWorkFirst(r)
			probesEnd(r.state, "work", tt.reads)

			doc := r.state.document()
			work, _ := doc.Account("work")
			if got := status.RefusedRead(work.RefusedRead); got != tt.held {
				t.Errorf("status reads work's read refused, %q, as %d, want %d: status and the router judge it alike", work.RefusedRead, got, tt.held)
			}
			if tt.held == 0 {
				return
			}
			said, _ := tt.held.Said()
			if now, _ := doc.At(start, r.state.policy).Account("work"); now.Routing.Says != said {
				t.Errorf("work's ROUTING says %q, want %q, as its last refusal says", now.Routing.Says, said)
			}
		})
	}
}

func TestAnAccountWhoseReadWasRefusedIsHeldTillItGoesOutOnAnotherToken(t *testing.T) {
	for _, tt := range takeUps {
		t.Run(tt.name, func(t *testing.T) {
			// Side was never read: its first probe was refused, its
			// subscription ended. A minute on, it takes up the token its
			// file holds.
			clock := &testClock{now: start}
			files := &changingFiles{files: testTokens}
			r := newTestRouterReading(t, clock.read, &stubProber{}, files.read)
			probesEnd(r.state, "side", []error{errSubscriptionEnded})
			clock.now = start.Add(time.Minute)

			tt.do(r, files)
			held := tt.token == sideToken
			r.state.mu.Lock()
			u := r.state.usage["side"]
			refused, since := u.readRefused(), u.refusedSince
			kept, _ := u.saved()
			r.state.mu.Unlock()
			wantSince := time.Time{}
			if held {
				wantSince = start
			}
			if refused != held || !since.Equal(wantSince) {
				t.Errorf("side's read refused: %v, since %v; want %v, since %v: only another token lifts it", refused, since, held, wantSince)
			}
			if got := status.RefusedRead(kept.RefusedRead); (got != 0) != held || !kept.RefusedSince.Equal(wantSince) {
				t.Errorf("the state file keeps side's probes as %+v, want its read refused kept %v, since %v", kept, held, wantSince)
			}
		})
	}
}

func TestOnlyAnUnspacedRefreshReadsAnAccountWhoseReadWasRefusedHoweverLatelyItWasRead(t *testing.T) {
	for _, tt := range readFailures {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			s := newTestState(clock)
			s.record("work", []quota.Window{session, soonWeek}, s.mark())
			probesEnd(s, "work", tt.reads)
			minuteOn := start.Add(reprobeAfter)

			if due, want := s.refreshing(refreshAsk{maxAge: 30 * time.Minute})("work", minuteOn), tt.held != 0; due != want {
				t.Errorf("work due a refresh asking for usage no older than 30 minutes, a minute after it was read = %v, want %v", due, want)
			}
			if s.refreshing(refreshAsk{maxAge: 30 * time.Minute, spaced: true})("work", minuteOn) {
				t.Error("work due a spaced refresh asking for usage no older than 30 minutes, a minute after it was read, want it left to its backoff")
			}
		})
	}
}

func TestRefreshLeavesAnAccountWhoseReadWasRefusedToItsBackoffWhereSpaced(t *testing.T) {
	tests := []struct {
		name, body string
		want       bool
	}{
		{name: "as R asks", body: `{"max_age": "1m"}`, want: true},
		{name: "as the dashboard's look after a reset asks", body: `{"max_age": "1m", "spaced": true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			prober := &stubProber{}
			r := newTestRouter(t, clock.read, prober)
			readWorkFirst(r)
			failWorksRead(r, errSubscriptionEnded)
			clock.now = start.Add(reprobeAfter)

			if got := askToRefresh(t.Context(), r, tt.body); got.Code != http.StatusOK {
				t.Fatalf("POST /refresh %s answered %d %s, want 200", tt.body, got.Code, got.Body)
			}
			if probed := prober.counts()[workToken] > 0; probed != tt.want {
				t.Errorf("work probed a minute after its read was refused = %v, want %v", probed, tt.want)
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

func TestAChoiceOrASpacedRefreshProbesAnAccountWhoseReadWasRefusedNoSoonerThanItsRounds(t *testing.T) {
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
			}{
				{"before a choice", s.due},
				{"with no room", s.dueAgain},
				{"on a spaced refresh", s.refreshing(refreshAsk{maxAge: status.FreshFor, spaced: true})},
			} {
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

func TestARestartHoldsAnAccountByItsReadRefusedOnlyFromWhenItWasFirstRefused(t *testing.T) {
	tests := []struct {
		name string
		// then is why work's probe an hour after its read was refused read
		// nothing.
		then error
	}{
		{name: "refused again", then: errSubscriptionEnded},
		{name: "its token refused after", then: errTokenRefused},
		{name: "timed out after", then: errTimedOut},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Work keeps a tenth back, and nothing else is read: its week
			// reads 50%, two hours on its subscription ends, and an hour
			// after that its probe reads nothing again. Four hours on, the
			// router restarts.
			path := filepath.Join(t.TempDir(), "state.json")
			clock := &testClock{now: start}
			before := newTestFile(clock.read, resolve(workReserved(), testTokens.Read, unrecorded))
			before.load(path)
			before.state.record("work", []quota.Window{weekUsed(0.5)}, before.state.mark())
			clock.now = start.Add(2 * time.Hour)
			probesEnd(before.state, "work", []error{errSubscriptionEnded})
			clock.now = start.Add(3 * time.Hour)
			probesEnd(before.state, "work", []error{tt.then})
			before.save()
			now := start.Add(4 * time.Hour)
			h := newHistory(config.History{}, at(now))
			h.open(t.TempDir())
			writeLines(t, h, workReads(start, 0.5, week.ResetsAt))

			after := newTestFile(at(now), resolve(workReserved(), testTokens.Read, unrecorded))
			after.load(path)
			after.state.seed(h.readBack(now))
			if got := after.state.document().Demand; got == nil || got.Hours != 2 {
				t.Errorf("after the restart, the pool's demand is %+v, want it measured over 2 hours: room on work from its reading till its read was first refused, not its last probe", got)
			}
			if roomAt(after.state, now) {
				t.Error("after the restart, the pool has room, want none: work's subscription has ended")
			}
		})
	}
}

func TestARestartWithoutItsTokenHoldsAnAccountWhoseReadWasRefusedTillAnotherTokenComes(t *testing.T) {
	tests := []struct {
		name string
		// back is the token side's file holds once it's back.
		back string
		held bool
	}{
		{name: "its own back", back: sideToken, held: true},
		{name: "another", back: renewedToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Side's subscription ended, and the router restarts an hour on
			// with side's token file holding none it can use; then the
			// file holds a token again.
			path := filepath.Join(t.TempDir(), "state.json")
			first := newTestFile(at(start), testAccounts())
			first.load(path)
			probesEnd(first.state, "side", []error{errSubscriptionEnded})
			first.save()
			files := &changingFiles{files: tokenstest.Files{"work": workToken}}
			r := newTestRouterReading(t, at(start.Add(time.Hour)), &stubProber{}, files.read)
			r.file.load(path)
			heldSince := func() (bool, time.Time) {
				r.state.mu.Lock()
				defer r.state.mu.Unlock()
				u := r.state.usage["side"]
				return u.readRefused(), u.refusedSince
			}
			if refused, since := heldSince(); !refused || !since.Equal(start) {
				t.Fatalf("without its token, side's read refused: %v, since %v; want refused since %v, as on its loss", refused, since, start)
			}

			files.set(tokenstest.Files{"work": workToken, "side": tt.back})
			r.upkeep.tokens.look()
			wantSince := time.Time{}
			if tt.held {
				wantSince = start
			}
			if refused, since := heldSince(); refused != tt.held || !since.Equal(wantSince) {
				t.Errorf("with %s, side's read refused: %v, since %v; want %v, since %v: only another token lifts it", tt.name, refused, since, tt.held, wantSince)
			}
		})
	}
}

func TestARefusalKeptByARouterStartedWithoutItsTokenHoldsOnceItsBack(t *testing.T) {
	// Side's subscription ended, and the router restarts an hour on with
	// side's token file holding none it can use, saving the state file as
	// work's read changes; then it restarts again, its same token back.
	path := filepath.Join(t.TempDir(), "state.json")
	first := newTestFile(at(start), testAccounts())
	first.load(path)
	probesEnd(first.state, "side", []error{errSubscriptionEnded})
	first.save()
	tokenless := newTestFile(at(start.Add(time.Hour)), resolve(testConfigured, tokenstest.Files{"work": workToken}.Read, unrecorded))
	tokenless.load(path)
	tokenless.state.record("work", []quota.Window{session, soonWeek}, tokenless.state.mark())
	if !tokenless.changes.unsaved.Load() {
		t.Fatal("the router without side's token has nothing to save, want work's read saved")
	}
	tokenless.save()

	back := newTestFile(at(start.Add(2*time.Hour)), testAccounts())
	back.load(path)
	if u := back.state.usage["side"]; !u.readRefused() || !u.refusedSince.Equal(start) {
		t.Errorf("with its same token back, side's read refused: %v, since %v; want refused since %v, as the router without it saved it",
			u.readRefused(), u.refusedSince, start)
	}
}

// A read refused loads as the state file kept why: as a value, whatever its
// error's text reads. A file from a router that judged the hold by the
// error's text, keeping when the refusal began but not why, holds the account
// as its text says, and is saved again with why. One from before the hold,
// keeping no start, holds nothing, as its router held nothing, and neither
// does a refusal this router doesn't know: the account's next read judges it.
// A launch with the router down reads the file alike, as KeptHolds has it.
func TestARefusedReadLoadsAsTheStateFileKeptIt(t *testing.T) {
	probed, began := start.Add(2*time.Hour), start.Add(time.Hour)
	tests := []struct {
		name string
		kept savedReading
		// want is why work's read is refused once loaded, since when, and
		// resaved whether the file is due to be saved again.
		want    quota.ReadRefusal
		since   time.Time
		resaved bool
	}{
		{
			name: "kept as a value",
			kept: savedReading{ProbeError: errSubscriptionEnded.Error(), RefusedRead: quota.SubscriptionEnded, RefusedSince: began},
			want: quota.SubscriptionEnded, since: began,
		},
		{
			name: "kept as a value, its error reading otherwise",
			kept: savedReading{ProbeError: errUnreadable.Error(), RefusedRead: quota.TokenRefused, RefusedSince: began},
			want: quota.TokenRefused, since: began,
		},
		{
			name: "kept as a value without when it began, as by hand: from its last probe",
			kept: savedReading{ProbeError: errTokenRefused.Error(), RefusedRead: quota.TokenRefused},
			want: quota.TokenRefused, since: probed, resaved: true,
		},
		{
			name: "kept by its start and its error's text alone, as a router from before the value kept it",
			kept: savedReading{ProbeError: errSubscriptionEnded.Error(), RefusedSince: began},
			want: quota.SubscriptionEnded, since: began, resaved: true,
		},
		{
			name: "kept by its start, its error's text a token refused",
			kept: savedReading{ProbeError: errTokenRefused.Error(), RefusedSince: began},
			want: quota.TokenRefused, since: began, resaved: true,
		},
		{
			name:    "kept by its start, its error's text no refusal",
			kept:    savedReading{ProbeError: errUnreadable.Error(), RefusedSince: began},
			resaved: true,
		},
		{
			name: "an error reading as a refusal, kept from before the hold, with no start",
			kept: savedReading{ProbeError: errSubscriptionEnded.Error()},
		},
		{
			name:    "a refusal this router doesn't know",
			kept:    savedReading{ProbeError: errSubscriptionEnded.Error(), RefusedRead: quota.ReadRefusal("something_new"), RefusedSince: began},
			resaved: true,
		},
		{
			name: "a read that failed otherwise",
			kept: savedReading{ProbeError: errUnreadable.Error()},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			kept := tt.kept
			kept.ReadAt, kept.Windows, kept.Probed, kept.Misses = start, []quota.Window{weekUsed(0.5)}, probed, 1
			putState(t, path, savedState{
				Version:  stateVersion,
				Tokens:   map[string]savedTokens{"work": {SHA256: workHash}, "side": {SHA256: sideHash}},
				Readings: map[string]savedReading{"work": kept},
			})
			f := newTestFile(at(start.Add(3*time.Hour)), resolve(workReserved(), testTokens.Read, unrecorded))
			read, err := KeptHoldsIn(filepath.Dir(path), f.refusalIn)
			if got := read.Of("work", mustToken(t, workToken)).RefusedRead; err != nil || got != tt.want {
				t.Errorf("a launch with the router down reads work's read refused as %q (%v), want %q, as the router loads it", got, err, tt.want)
			}

			f.load(path)
			u := f.state.usage["work"]
			if u.refusedRead != tt.want || !u.refusedSince.Equal(tt.since) || u.readRefused() != (tt.want != "") {
				t.Errorf("work loaded as refused %q since %v, held %v; want %q since %v", u.refusedRead, u.refusedSince, u.readRefused(), tt.want, tt.since)
			}
			if u.probeErr != tt.kept.ProbeError {
				t.Errorf("work's error loaded as %q, want %q, as kept", u.probeErr, tt.kept.ProbeError)
			}
			if got := f.changes.unsaved.Load(); got != tt.resaved {
				t.Errorf("the file due to be saved again: %v, want %v", got, tt.resaved)
			}
			if !tt.resaved {
				return
			}
			f.save()
			if got := readState(t, path).Readings["work"]; got.RefusedRead != tt.want || !got.RefusedSince.Equal(tt.since) {
				t.Errorf("the file saved again keeps work refused %q since %v, want %q since %v", got.RefusedRead, got.RefusedSince, tt.want, tt.since)
			}
		})
	}
}

// A launch with the router down reads a read refused off the state file as a
// router started again would hold the account back by it: on the token the
// file kept it of alone, as a token replaced since starts afresh; of the
// account it was refused alone; and never where there's no file.
func TestALaunchReadsARefusalTheStateFileKeepsOnItsTokenAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	saving := newTestFile(at(start), testAccounts())
	saving.load(path)
	probesEnd(saving.state, "side", []error{errSubscriptionEnded})
	saving.save()
	tests := []struct {
		name  string
		dir   string
		id    string
		token string
		want  quota.ReadRefusal
	}{
		{name: "its token as kept", dir: filepath.Dir(path), id: "side", token: sideToken, want: quota.SubscriptionEnded},
		{name: "another token since", dir: filepath.Dir(path), id: "side", token: renewedToken},
		{name: "another account", dir: filepath.Dir(path), id: "work", token: workToken},
		{name: "another account, on the token kept of the one refused", dir: filepath.Dir(path), id: "work", token: sideToken},
		{name: "no file", dir: t.TempDir(), id: "side", token: sideToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kept, err := KeptHoldsIn(tt.dir, saving.refusalIn)
			if got := kept.Of(tt.id, mustToken(t, tt.token)); err != nil || got != (KeptHold{RefusedRead: tt.want}) {
				t.Errorf("Of(%q) = %+v (error %v), want its read refused as %q, its token its own", tt.id, got, err, tt.want)
			}
		})
	}
}

// A launch with the router down reads a token kept as another
// subscription's as the router, started again, takes it in: of the token the
// file kept it of alone, as a token replaced since is the account's own till
// a probe finds otherwise.
func TestALaunchReadsAForeignTokenTheStateFileKeepsAsTheRouterLoadsIt(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{name: "the token kept", token: sideToken, want: true},
		{name: "another token since", token: renewedToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			putState(t, path, savedState{
				Version: stateVersion,
				Tokens:  map[string]savedTokens{"work": {SHA256: workHash}, "side": {SHA256: sideHash, Foreign: sideHash}},
			})
			as := resolve(testConfigured, tokenstest.Files{"work": workToken, "side": tt.token}.Read, unrecorded)
			f := newTestFile(at(start), as)

			kept, err := KeptHoldsIn(filepath.Dir(path), f.refusalIn)
			if got := kept.Of("side", mustToken(t, tt.token)); err != nil || got != (KeptHold{Foreign: tt.want}) {
				t.Errorf("Of(side) = %+v (error %v), want its token another subscription's: %v", got, err, tt.want)
			}
			f.load(path)
			if side, _ := as.byID("side"); side.hasToken() == tt.want {
				t.Errorf("the router loads side as having a token of its own: %v, want %v, as a launch reads it", side.hasToken(), !tt.want)
			}
		})
	}
}

// A state file a launch can't read keeps no refusal, and fails saying why,
// left as it is: only the router, as it starts, sets a corrupt one aside.
func TestALaunchLeavesAStateFileItCantReadAsItIs(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantError string
	}{
		{name: "not JSON", content: "{\"version\": 1, \"readi", wantError: "unexpected end of JSON input"},
		{name: "a version unknown", content: `{"version": 2, "readings": {"side": {"refused_read": "subscription_ended"}}}`, wantError: "version 2, where 1 is known"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "state.json")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}

			kept, err := KeptHoldsIn(dir, newTestFile(at(start), testAccounts()).refusalIn)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) || !strings.Contains(err.Error(), path) {
				t.Errorf("KeptHoldsIn() error = %v, want one naming %s, saying %q", err, path, tt.wantError)
			}
			if got := kept.Of("side", mustToken(t, sideToken)); got != (KeptHold{}) {
				t.Errorf("Of(side) = %+v, want nothing held", got)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != tt.content {
				t.Errorf("the state file holds %q (%v), want it left as it was", data, err)
			}
		})
	}
}
