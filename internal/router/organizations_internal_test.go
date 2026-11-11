package router

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

// sideOwnToken is a token of side's own subscription, given in place of one
// of another's.
const sideOwnToken = "test-token-side-own"

// The accounts as the accounts record keeps them: work and side, each held
// to its organization; side, its organization not known; and an account
// removed, held to its.
var (
	heldWork    = identity.Entry{ID: "work", Organization: "test-org-work", Added: start}
	heldSide    = identity.Entry{ID: "side", Organization: "test-org-side", Added: start}
	unknownSide = identity.Entry{ID: "side", Added: start}
	heldGone    = identity.Entry{ID: "gone", Organization: "test-org-gone", Label: "Gone", Added: start, Removed: start}
)

// orgProbed is a probe that read the session and the week, its answers
// naming organization: none where it's "".
func orgProbed(organization string) quota.Probe {
	p := probed(nil, session, week)
	p.Organization = organization
	return p
}

// newHeldRouter builds a router as newTestRouterReading does, its state
// directory holding an accounts record of the accounts given.
func newHeldRouter(t *testing.T, clock *testClock, prober Prober, read func(id string) (tokens.Token, error), record ...identity.Entry) *Router {
	t.Helper()
	cfg := testConfig(clock.read, prober, read)
	cfg.StateDir = t.TempDir()
	if err := identity.NewFile(cfg.StateDir).Update(func(r *identity.Record) error {
		r.Accounts = record
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return newTestRouterOf(t, cfg)
}

// recorded is the organization the router's accounts record holds the
// account with the given id to: "" where it holds none.
func recorded(t *testing.T, r *Router, id string) string {
	t.Helper()
	record, err := identity.NewFile(r.cfg.StateDir).Read()
	if err != nil {
		t.Fatal(err)
	}
	e, _ := record.ByID(id)
	return e.Organization
}

// probeNow probes the account with the given id at once, whenever it was
// last probed, and waits for the probe to end.
func probeNow(r *Router, id string) {
	for _, u := range r.probes.start(r.accounts.only([]string{id}), func(string, time.Time) bool { return true }) {
		<-u.done
	}
}

// foreignEvents are the events the router keeps of an account's token found
// another subscription's.
func foreignEvents(r *Router) []status.Event {
	var found []status.Event
	for _, e := range r.recent.events() {
		if e.Kind == status.EventForeign {
			found = append(found, e)
		}
	}
	return found
}

// wantForeign fails the test unless side stands as foreign says: its token
// another subscription's, so it takes no requests, its status saying so and
// what puts it right, the one event of it kept; or its own, which it does,
// its status saying nothing of it, with no such event.
func wantForeign(t *testing.T, r *Router, foreign bool) {
	t.Helper()
	side, _ := r.accounts.byID("side")
	if side.hasToken() == foreign || !side.holdsToken() {
		t.Errorf("side has a token of its own: %v, a token: %v; want %v and true", side.hasToken(), side.holdsToken(), !foreign)
	}
	st, _ := r.Status().Account("side")
	why := ""
	if foreign {
		why = prose.ForeignTokenFix("side")
	}
	if st.TokenSet == foreign || st.TokenForeign != foreign || st.Error != why {
		t.Errorf("side's status has TokenSet = %v, TokenForeign = %v, Error = %q; want %v, %v, %q", st.TokenSet, st.TokenForeign, st.Error, !foreign, foreign, why)
	}
	events := foreignEvents(r)
	switch {
	case foreign && (len(events) != 1 || events[0].Account != "side"):
		t.Errorf("the router keeps %+v of tokens found another subscription's, want one, of side", events)
	case !foreign && len(events) > 0:
		t.Errorf("the router keeps %+v of tokens found another subscription's, want none", events)
	}
}

func TestAProbesOrganizationIsHeldToItsAccounts(t *testing.T) {
	tests := []struct {
		name   string
		record []identity.Entry
		// damaged replaces the record with what isn't one.
		damaged      bool
		organization string
		// foreign is whether side's token is found another subscription's,
		// and recorded the organization the record holds side to after.
		foreign  bool
		recorded string
	}{
		{name: "the account's own", record: []identity.Entry{heldWork, heldSide}, organization: "test-org-side", recorded: "test-org-side"},
		{name: "another than the account's", record: []identity.Entry{heldWork, heldSide}, organization: "test-org-other", foreign: true, recorded: "test-org-side"},
		{name: "another account's, the account's not known", record: []identity.Entry{heldWork, unknownSide}, organization: "test-org-work", foreign: true},
		{name: "a removed account's, the account's not known", record: []identity.Entry{heldGone, unknownSide}, organization: "test-org-gone", foreign: true},
		{name: "one no account has, the account's not known, which it's recorded as", record: []identity.Entry{heldWork, unknownSide}, organization: "test-org-side", recorded: "test-org-side"},
		{name: "none", record: []identity.Entry{heldWork, heldSide}, recorded: "test-org-side"},
		{name: "none, the account's not known", record: []identity.Entry{heldWork, unknownSide}},
		{name: "another, the account not in the record", record: []identity.Entry{heldWork}, organization: "test-org-other"},
		{name: "another, the record damaged", damaged: true, organization: "test-org-other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			clock := &testClock{now: start}
			prober := &stubProber{readings: map[string]quota.Probe{sideToken: orgProbed(tt.organization)}}
			r := newHeldRouter(t, clock, prober, testTokens.Read, tt.record...)
			if tt.damaged {
				if err := os.WriteFile(identity.NewFile(r.cfg.StateDir).Path(), []byte("not a record"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			probeNow(r, "side")
			wantForeign(t, r, tt.foreign)
			if got, want := len(r.state.usage["side"].windows), 2; tt.foreign == (got == want) {
				t.Errorf("side has %d windows read, want them read only of a token of its own", got)
			}
			if !tt.damaged {
				if got := recorded(t, r, "side"); got != tt.recorded {
					t.Errorf("the record holds side to %q, want %q", got, tt.recorded)
				}
			}
			checkNoToken(t, log)
		})
	}
}

func TestNoProbesOrganizationIsHeldWhileTheMoveOntoTheIDsIsOwed(t *testing.T) {
	tests := []struct {
		name         string
		record       []identity.Entry
		organization string
		recorded     string
	}{
		{name: "another than the account's", record: []identity.Entry{heldWork, heldSide}, organization: "test-org-other", recorded: "test-org-side"},
		{name: "the account's not known", record: []identity.Entry{heldWork, unknownSide}, organization: "test-org-side"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			prober := &stubProber{readings: map[string]quota.Probe{sideToken: orgProbed(tt.organization)}}
			r := newHeldRouter(t, clock, prober, testTokens.Read, tt.record...)
			r.moving.standing(status.Moving{Stage: status.MoveRunning})

			probeNow(r, "side")
			wantForeign(t, r, false)
			if got := recorded(t, r, "side"); got != tt.recorded {
				t.Errorf("the record holds side to %q, want %q: the config's ids and its differ till the move is done", got, tt.recorded)
			}
		})
	}
}

func TestAnAccountWhoseTokenIsAnothersTakesNoRequestsTillItHasItsOwn(t *testing.T) {
	log := logstest.Capture(t)
	clock := &testClock{now: start}
	files := &changingFiles{files: testTokens}
	prober := &stubProber{readings: map[string]quota.Probe{
		workToken: orgProbed("test-org-work"), sideToken: orgProbed("test-org-other"), sideOwnToken: orgProbed("test-org-side"),
	}}
	r := newHeldRouter(t, clock, prober, files.read, heldWork, heldSide)
	defer r.probes.stop()
	r.state.record("work", []quota.Window{session, week}, r.state.mark())
	r.state.record("side", []quota.Window{session, week}, r.state.mark())
	if err := r.pin(PinRequest{Account: "side", By: ByCLI}); err != nil {
		t.Fatal(err)
	}
	newSession := func(id string) Request { return Request{ID: id, Session: id, Model: opus, Client: "work"} }
	if got := choose(t.Context(), r, newSession("one")).Account; got != "side" {
		t.Fatalf("a new session goes to %q, want side, which the global pin names", got)
	}

	probeNow(r, "side")
	probeNow(r, "side")
	wantForeign(t, r, true)
	if !log.Has("level=WARN", `msg="token is another subscription's; nothing will go out on the account till it has its own"`, "account=side") {
		t.Errorf("log reads\n%s\nwant a warning that side's token is another subscription's", log)
	}
	if got := choose(t.Context(), r, newSession("two")).Account; got != "work" {
		t.Errorf("a new session goes to %q, want work: side, though pinned, takes no requests", got)
	}
	wantRefusal := "account side has no usable token, so nothing can go out on it: " + prose.ForeignTokenFix("side")
	if err := r.pin(PinRequest{Account: "side", By: ByCLI}); err == nil || err.Error() != wantRefusal {
		t.Errorf("pinning side fails with %v, want %q", err, wantRefusal)
	}
	req := httptest.NewRequest("POST", "/v1/messages", nil)
	req.Header.Set(PinHeader, "side")
	if got := r.proxy.pin(req, &exchange{id: someRequest}, "three"); got != "" {
		t.Errorf("a session's own pin to side is honoured as %q, want it ignored", got)
	}

	// Its own token given, as accounts token gives it, it takes requests once
	// a probe on that token finds it its own.
	files.set(tokenstest.Files{"work": workToken, "side": sideOwnToken})
	r.upkeep.tokens.look()
	if side, _ := r.accounts.byID("side"); side.hasToken() {
		t.Error("side takes requests as it takes up a token, want it not to till a probe finds that token its own")
	}
	clock.now = start.Add(roundEvery)
	r.rounds.look(t.Context())
	if n := prober.counts()[sideOwnToken]; n != 1 {
		t.Errorf("side's own token probed %d times on the next round, want once", n)
	}
	wantSideOwn(t, r, log)
	if got := choose(t.Context(), r, newSession("four")).Account; got != "side" {
		t.Errorf("a new session goes to %q, want side, which the global pin names, with a token of its own again", got)
	}
	if events := foreignEvents(r); len(events) != 1 {
		t.Errorf("the router keeps %+v of tokens found another subscription's, want the one", events)
	}
	checkNoToken(t, log)
}

// wantSideOwn fails the test unless side takes requests again, its token its
// own once more, as the log tells.
func wantSideOwn(t *testing.T, r *Router, log *logstest.Log) {
	t.Helper()
	side, _ := r.accounts.byID("side")
	st, _ := r.Status().Account("side")
	if !side.hasToken() || !st.TokenSet || st.TokenForeign || st.Error != "" {
		t.Errorf("side has a token of its own: %v, its status TokenSet = %v, TokenForeign = %v, Error = %q; want it its own", side.hasToken(), st.TokenSet, st.TokenForeign, st.Error)
	}
	if !log.Has("level=INFO", `msg="token is the account's own again; requests can go out on it"`, "account=side") {
		t.Errorf("log reads\n%s\nwant side's token told its own again", log)
	}
}

func TestATokenTakenUpIsProbedOnTheNextRoundHoweverLatelyItsAccountWasRead(t *testing.T) {
	clock := &testClock{now: start}
	files := &changingFiles{files: tokenstest.Files{"work": workToken, "side": sideOwnToken}}
	prober := &stubProber{readings: map[string]quota.Probe{sideOwnToken: orgProbed("test-org-side"), sideToken: orgProbed("test-org-other")}}
	r := newHeldRouter(t, clock, prober, files.read, heldWork, heldSide)
	defer r.probes.stop()
	probeNow(r, "side")
	due := r.state.rounding(r.rounds.clears)
	clock.now = start.Add(roundEvery)
	if due("side", clock.now) {
		t.Fatal("side due on its rounds, read a minute before on its own token, want it not")
	}

	// Its token replaced by hand with another subscription's.
	files.set(testTokens)
	r.upkeep.tokens.look()
	if !due("side", clock.now) {
		t.Fatal("side not due on its rounds, want the token it took up probed")
	}
	r.rounds.look(t.Context())
	if n := prober.counts()[sideToken]; n != 1 {
		t.Errorf("the token side took up probed %d times on the round, want once", n)
	}
	wantForeign(t, r, true)
	if due("side", clock.now) {
		t.Error("side due on its rounds again at once, want its probes backing off")
	}
}

func TestAProbeOnTheTokenBeforeSaysNothingOfTheTokenHeld(t *testing.T) {
	tests := []struct {
		name   string
		record []identity.Entry
		// organization is what the probe on the token before names.
		organization string
	}{
		{name: "another subscription's", record: []identity.Entry{heldWork, heldSide}, organization: "test-org-other"},
		{name: "one no account has, the account's not known", record: []identity.Entry{heldWork, unknownSide}, organization: "test-org-before"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				clock := &testClock{now: start}
				files := &changingFiles{files: testTokens}
				prober := &stubProber{gate: make(chan struct{}), readings: map[string]quota.Probe{sideToken: orgProbed(tt.organization)}}
				r := newHeldRouter(t, clock, prober, files.read, tt.record...)
				defer r.probes.stop()
				want := recorded(t, r, "side")

				underway := r.probes.start(r.accounts.only([]string{"side"}), func(string, time.Time) bool { return true })
				synctest.Wait()
				files.set(tokenstest.Files{"work": workToken, "side": sideOwnToken})
				r.upkeep.tokens.look()
				close(prober.gate)
				<-underway[0].done

				wantForeign(t, r, false)
				if got := recorded(t, r, "side"); got != want {
					t.Errorf("the record holds side to %q, want %q, as before", got, want)
				}
			})
		})
	}
}

func TestAProbeOfAnotherSubscriptionsTokenReadsNothingOfTheAccount(t *testing.T) {
	clock := &testClock{now: start}
	prober := &stubProber{readings: map[string]quota.Probe{sideToken: orgProbed("test-org-other")}}
	r := newHeldRouter(t, clock, prober, testTokens.Read, heldWork, heldSide)
	probeNow(r, "side")
	u := r.state.usage["side"]
	if len(u.windows) > 0 || !u.updated.IsZero() {
		t.Errorf("side has %d windows read, updated %v; want none: they're another subscription's", len(u.windows), u.updated)
	}
	if !u.probed.Equal(start) || u.misses != 1 || !strings.HasPrefix(u.probeErr, prose.ForeignToken) {
		t.Errorf("side's probe ended %v, misses %d, error %q; want it ended now, read nothing, saying why", u.probed, u.misses, u.probeErr)
	}
}

func TestAnAccountWhoseTokenIsAnothersIsDueOnItsRoundsWhateverItsWindowsRead(t *testing.T) {
	clock := &testClock{now: start.Add(-6 * time.Hour)}
	s := newTestState(clock)
	lapsed := session
	lapsed.ResetsAt = start.Add(-time.Hour)
	s.record("side", []quota.Window{lapsed, week}, s.mark())
	clock.now = start
	u := s.usage["side"]
	if due, ifClear := u.onRounds(testPolicy, false, start); due || !ifClear {
		t.Fatalf("side, its session lapsed, due = %v, due if clear = %v on its rounds; want only if clear", due, ifClear)
	}
	if due, ifClear := u.onRounds(testPolicy, true, start); !due || ifClear {
		t.Errorf("side, its token another subscription's, due = %v, due if clear = %v on its rounds; want due", due, ifClear)
	}
}

func TestATokenTakenUpWhileTheRouterWasAwayIsProbedOnItsFirstRound(t *testing.T) {
	for _, held := range []bool{true, false} {
		clock := &testClock{now: start}
		before := newTestRouter(t, clock.read, &stubProber{})
		before.state.record("side", []quota.Window{session, week}, before.state.mark())
		kept := before.state.saved()

		// The router restarts a minute on, side holding the token it held,
		// or another taken up meanwhile.
		clock.now = start.Add(roundEvery)
		after := newTestRouter(t, clock.read, &stubProber{})
		after.state.recall(kept, func(string) bool { return held })
		if due := after.state.rounding(after.rounds.clears)("side", clock.now); due == held {
			t.Errorf("side, read a minute before, holding the token it held then: %v, due on its first round = %v; want %v", held, due, !held)
		}
	}
}
