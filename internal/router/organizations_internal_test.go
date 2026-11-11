package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
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
	var kept changeCount
	r.probes.kept = kept.hear
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
	if kept != 1 {
		t.Errorf("the state file heard of %d changes to side's tokens, want one: the finding", kept)
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

	// Its own token given, as accounts token gives it, it takes requests at
	// once, as the token found another's is no longer the one it holds, and
	// the next round's probe on that token finds it its own, which is no news.
	files.set(tokenstest.Files{"work": workToken, "side": sideOwnToken})
	r.upkeep.tokens.look()
	wantSideTakes(t, r)
	if got := choose(t.Context(), r, newSession("four")).Account; got != "side" {
		t.Errorf("a new session goes to %q, want side, which the global pin names, with a token of its own again", got)
	}
	clock.now = start.Add(roundEvery)
	r.rounds.look(t.Context())
	if n := prober.counts()[sideOwnToken]; n != 1 {
		t.Errorf("side's own token probed %d times on the next round, want once", n)
	}
	wantSideTakes(t, r)
	if log.Has(`msg="token is the account's own again"`) {
		t.Errorf("log reads\n%s\nwant no token told its own again: none taken up since was found another's", log)
	}
	if kept != 1 {
		t.Errorf("the state file heard of %d changes to side's tokens from probes, want one: the finding", kept)
	}
	if events := foreignEvents(r); len(events) != 1 {
		t.Errorf("the router keeps %+v of tokens found another subscription's, want the one", events)
	}
	checkNoToken(t, log)
}

func TestAFindingOfAnothersTokenIsForgottenOnceTheAccountTakesUpAnother(t *testing.T) {
	log := logstest.Capture(t)
	clock := &testClock{now: start}
	files := &changingFiles{files: testTokens}
	prober := &stubProber{readings: map[string]quota.Probe{sideToken: orgProbed("test-org-other")}}
	r := newHeldRouter(t, clock, prober, files.read, heldWork, heldSide)
	defer r.probes.stop()
	var kept changeCount
	r.probes.kept = kept.hear
	probeNow(r, "side")
	wantForeign(t, r, true)

	// Another taken up, then the one found another's put back by hand: each
	// takes requests at once, as any token taken up does, till a probe finds
	// otherwise.
	for _, token := range []string{sideOwnToken, sideToken} {
		files.set(tokenstest.Files{"work": workToken, "side": token})
		r.upkeep.tokens.look()
		wantSideTakes(t, r)
	}
	prober.readings[sideToken] = orgProbed("test-org-side")
	probeNow(r, "side")
	wantSideTakes(t, r)
	if kept != 1 {
		t.Errorf("the state file heard of %d changes to side's tokens, want one, the finding: a probe that finds a token its own clears no finding where none stands", kept)
	}
	if log.Has(`msg="token is the account's own again"`) {
		t.Errorf("log reads\n%s\nwant no token told its own again: none was found another's since it was taken up", log)
	}
}

// wantSideTakes fails the test unless side takes requests, its token its own
// as far as the router knows, its status saying nothing of another
// subscription's.
func wantSideTakes(t *testing.T, r *Router) {
	t.Helper()
	side, _ := r.accounts.byID("side")
	st, _ := r.Status().Account("side")
	if !side.hasToken() || !st.TokenSet || st.TokenForeign || st.Error != "" {
		t.Errorf("side has a token of its own: %v, its status TokenSet = %v, TokenForeign = %v, Error = %q; want it its own", side.hasToken(), st.TokenSet, st.TokenForeign, st.Error)
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

func TestATokenFoundAnothersIsProbedAgainOnlyOnceTheAccountTakesUpAnother(t *testing.T) {
	tests := []struct {
		name  string
		probe func(context.Context, *Router)
	}{
		{name: "on its rounds", probe: func(ctx context.Context, r *Router) { r.rounds.look(ctx) }},
		{name: "on a refresh", probe: func(ctx context.Context, r *Router) { r.refresh(ctx, 0) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			files := &changingFiles{files: testTokens}
			prober := &stubProber{readings: map[string]quota.Probe{sideToken: orgProbed("test-org-other"), sideOwnToken: orgProbed("test-org-side")}}
			r := newHeldRouter(t, clock, prober, files.read, heldWork, heldSide)
			defer r.probes.stop()
			r.state.record("side", []quota.Window{session, week}, r.state.mark())
			probeNow(r, "side")
			wantForeign(t, r, true)

			// An hour on, unread since, its probes long past backing off.
			clock.now = start.Add(time.Hour)
			tt.probe(t.Context(), r)
			if n := prober.counts()[sideToken]; n != 1 {
				t.Errorf("the token found another subscription's probed %d times, want once, as it was found", n)
			}

			files.set(tokenstest.Files{"work": workToken, "side": sideOwnToken})
			r.upkeep.tokens.look()
			tt.probe(t.Context(), r)
			if n := prober.counts()[sideOwnToken]; n != 1 {
				t.Errorf("the token side took up probed %d times, want once", n)
			}
			wantSideTakes(t, r)
		})
	}
}

func TestATokenTakenUpInPlaceOfOneFoundAnothersIsProbedOnItsRoundsAsAnyIs(t *testing.T) {
	clock := &testClock{now: start}
	files := &changingFiles{files: testTokens}
	prober := &stubProber{readings: map[string]quota.Probe{sideToken: orgProbed("test-org-other")}}
	r := newHeldRouter(t, clock, prober, files.read, heldWork, heldSide)
	defer r.probes.stop()
	r.state.record("side", []quota.Window{session, week}, r.state.mark())
	probeNow(r, "side")

	// Hours on, its session lapsed, another taken up: a probe of it would
	// start the session, so it's due only where that clears its next prime.
	clock.now = start.Add(6 * time.Hour)
	files.set(tokenstest.Files{"work": workToken, "side": sideOwnToken})
	r.upkeep.tokens.look()
	for clear, want := range map[bool]bool{false: false, true: true} {
		if due := r.state.rounding(func(string, time.Time) bool { return clear })("side", clock.now); due != want {
			t.Errorf("side due on its rounds = %v, its probe clear of its next prime: %v; want %v", due, clear, want)
		}
	}
}

func TestAProbeNotesWhatItFindsOfTheTokenTheAccountHoldsAloneAndOnlyAsItChanges(t *testing.T) {
	tests := []struct {
		name string
		// foundBefore has side's token found another subscription's before
		// the probe.
		foundBefore bool
		// probed is the token the probe went out on, and found whether it
		// found it another subscription's.
		probed string
		found  bool
		// changed is whether what's noted changes, and foreign whether side
		// holds another subscription's token after.
		changed, foreign bool
	}{
		{name: "another's, of the token it holds", probed: sideToken, found: true, changed: true, foreign: true},
		{name: "another's, of the token before", probed: sideOwnToken, found: true},
		{name: "another's again", foundBefore: true, probed: sideToken, found: true, foreign: true},
		{name: "its own, of the token found another's", foundBefore: true, probed: sideToken, changed: true},
		{name: "its own, of a token none found another's", probed: sideToken},
		{name: "its own, of the token before, the one held found another's", foundBefore: true, probed: sideOwnToken, foreign: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			side, _ := testAccounts().byID("side")
			if tt.foundBefore {
				side.secret.disown(hash(sideToken), true)
			}
			if changed := side.secret.disown(hash(tt.probed), tt.found); changed != tt.changed {
				t.Errorf("changed = %v, want %v", changed, tt.changed)
			}
			if _, foreign := side.lacks(); foreign != tt.foreign || side.hasToken() == tt.foreign {
				t.Errorf("side's token another subscription's: %v, has a token of its own: %v; want %v, %v", foreign, side.hasToken(), tt.foreign, !tt.foreign)
			}
		})
	}
}

// routeOn has the router's proxy route a messages request of the session
// given on token, and returns what it answered.
func routeOn(t *testing.T, r *Router, session, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"`+opus+`","max_tokens":1}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(claude.SessionHeader, session)
	rec := httptest.NewRecorder()
	r.Proxy().ServeHTTP(rec, req)
	return rec
}

// sideBeforeToken is a token side had before the one it holds.
const sideBeforeToken = "test-token-side-before"

// everyToken is an upstream that serves a request on any token the tests
// give, noting each it went out on.
func everyToken() *scriptedUpstream {
	return &scriptedUpstream{answers: map[string][]answer{
		workToken: {served}, sideToken: {served}, sideOwnToken: {served}, sideBeforeToken: {served},
	}}
}

func TestAClientOfAnAccountWhoseTokenIsAnothersIsRoutedNeverSentOnThatToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		// gone has side's token file hold none after the finding, for two
		// looks, so side has no usable token at all.
		gone bool
	}{
		{name: "the token it holds", token: sideToken},
		{name: "the token it had before", token: sideBeforeToken},
		{name: "the token it held, its file gone since", token: sideToken, gone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			// Side's own token replaced by hand with another subscription's.
			files := &changingFiles{files: tokenstest.Files{"work": workToken, "side": sideBeforeToken}}
			prober := &stubProber{readings: map[string]quota.Probe{workToken: orgProbed("test-org-work"), sideToken: orgProbed("test-org-other")}}
			r := newHeldRouter(t, clock, prober, files.read, heldWork, heldSide)
			defer r.probes.stop()
			files.set(testTokens)
			r.upkeep.tokens.look()
			probeNow(r, "side")
			wantForeign(t, r, true)
			if tt.gone {
				files.set(tokenstest.Files{"work": workToken})
				r.upkeep.tokens.look()
				r.upkeep.tokens.look()
				if side, _ := r.accounts.byID("side"); side.holdsToken() {
					t.Fatal("side holds a usable token, want none: its file is gone")
				}
			}
			upstream := everyToken()
			r.proxy.transport = upstream

			if rec := routeOn(t, r, "one", tt.token); rec.Code != http.StatusOK {
				t.Errorf("answered %d, want 200", rec.Code)
			}
			if sent := upstream.sentWith(); !slices.Equal(sent, []string{workToken}) {
				t.Errorf("the request went out on %q, want work's token alone", sent)
			}
		})
	}
}

func TestARequestNoAccountCanTakeButOneWhoseTokenIsAnothersIsAnsweredUnsent(t *testing.T) {
	log := logstest.Capture(t)
	clock := &testClock{now: start}
	prober := &stubProber{readings: map[string]quota.Probe{sideToken: orgProbed("test-org-other")}}
	r := newHeldRouter(t, clock, prober, tokenstest.Files{"side": sideToken}.Read, heldWork, heldSide)
	defer r.probes.stop()
	probeNow(r, "side")
	wantForeign(t, r, true)
	upstream := everyToken()
	r.proxy.transport = upstream

	rec := routeOn(t, r, "one", sideToken)
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("X-Should-Retry") != "false" {
		t.Errorf("answered %d, X-Should-Retry %q; want 503, false: nothing changes till side has a token of its own", rec.Code, rec.Header().Get("X-Should-Retry"))
	}
	want := "switchboard: no account can take the request; account side: " + prose.ForeignTokenFix("side")
	if !strings.Contains(rec.Body.String(), want) {
		t.Errorf("answered %s, want it to say %q", rec.Body, want)
	}
	if sent := upstream.sentWith(); len(sent) > 0 {
		t.Errorf("the request went out on %q, want nothing sent", sent)
	}
	if !log.Has("level=WARN", `msg="no account can take the request; answering 503"`, "account=side") {
		t.Errorf("log reads\n%s\nwant the request refused, of side's client", log)
	}
	checkNoToken(t, log)
}

func TestARequestNoAccountItCanGoOutOnTakesLeavesItsSessionWhereItWas(t *testing.T) {
	log := logstest.Capture(t)
	clock := &testClock{now: start}
	prober := &stubProber{readings: map[string]quota.Probe{workToken: orgProbed("test-org-work"), sideToken: orgProbed("test-org-other")}}
	r := newHeldRouter(t, clock, prober, testTokens.Read, heldWork, heldSide)
	defer r.probes.stop()
	r.state.record("work", []quota.Window{session, week}, r.state.mark())
	probeNow(r, "side")
	wantForeign(t, r, true)
	one := key{session: "one", model: opus}
	assign(r.sessions, one, "", decision{account: "work", reason: reasonNew}, start)
	r.state.forbid("work", r.state.family(opus), http.StatusForbidden, someRequest)
	upstream := everyToken()
	r.proxy.transport = upstream

	// The session's next request carries side's token: work, its account,
	// refused the model lately, and side can't be sent on.
	rec := routeOn(t, r, "one", sideToken)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("answered %d, want 503", rec.Code)
	}
	if sent := upstream.sentWith(); len(sent) > 0 {
		t.Errorf("the request went out on %q, want nothing sent", sent)
	}
	if got := r.sessions.lookup(one).current; got.Account != "work" {
		t.Errorf("the session is on %q, want work, where it was: the request went out on no account", got.Account)
	}
	for _, e := range r.recent.events() {
		if e.Kind == status.EventMoved {
			t.Errorf("the router keeps %+v, want no move", e)
		}
	}
	if log.Has("msg=moved") {
		t.Errorf("log reads\n%s\nwant no move", log)
	}
	checkNoToken(t, log)
}

// losingChooser is the router's chooser, but that the first time it chooses
// the account lost, the account loses its token as soon as it's chosen, as
// when a probe finds the token another subscription's just then.
type losingChooser struct {
	Chooser
	lost account
	once sync.Once
}

func (c *losingChooser) Choose(ctx context.Context, req Request) Choice {
	choice := c.Chooser.Choose(ctx, req)
	if choice.Account == c.lost.ID {
		c.once.Do(func() { c.lost.secret.disown(hash(c.lost.token().Reveal()), true) })
	}
	return choice
}

func TestAnAccountAReplayMovesOnToThatLosesItsTokenBeforeTheRequestGoesOutIsChosenAgain(t *testing.T) {
	clock := &testClock{now: start}
	read := tokenstest.Files{"work": workToken, "personal": personalToken, "side": sideToken}.Read
	r := newHeldRouter(t, clock, &stubProber{}, read, heldWork, heldSide)
	defer r.probes.stop()
	// Each has room, side's quota needing using sooner than personal's.
	r.state.record("work", []quota.Window{session, laterWeek}, r.state.mark())
	r.state.record("personal", []quota.Window{session, laterWeek}, r.state.mark())
	r.state.record("side", []quota.Window{session, soonWeek}, r.state.mark())
	one := key{session: "one", model: opus}
	assign(r.sessions, one, "", decision{account: "work", reason: reasonNew}, start)
	side, _ := r.accounts.byID("side")
	r.proxy.chooser = &losingChooser{Chooser: r.proxy.chooser, lost: side}
	upstream := &scriptedUpstream{answers: map[string][]answer{workToken: {limitHit}, personalToken: {served}, sideToken: {served}}}
	r.proxy.transport = upstream

	// Work's limit moves the request on, to side, which loses its token as
	// it's chosen.
	if rec := routeOn(t, r, "one", workToken); rec.Code != http.StatusOK {
		t.Errorf("answered %d, want 200: personal has room", rec.Code)
	}
	if sent := upstream.sentWith(); !slices.Equal(sent, []string{workToken, personalToken}) {
		t.Errorf("the request went out on %q, want work's token, then personal's", sent)
	}
	if got := r.sessions.lookup(one).current; got.Account != "personal" {
		t.Errorf("the session is on %q, want personal", got.Account)
	}
}

func TestAClientIsTheAccountWhoseOwnTokenItSendsBeforeOneFoundHoldingIt(t *testing.T) {
	tests := []struct {
		name string
		// side is the token side's file holds; work's holds side's, found
		// another subscription's, and gone has it hold none since, for two
		// looks.
		side string
		gone bool
		want string
	}{
		{name: "the account holding it its own, after the one found holding it", side: sideToken, want: "side"},
		{name: "the one found holding it, where none holds it its own", side: sideOwnToken, want: "work"},
		{name: "the one found holding it, its file gone since, where none holds it its own", side: sideOwnToken, gone: true, want: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			as := resolve(testConfigured, tokenstest.Files{"work": sideToken, "side": tt.side}.Read, unrecorded)
			work, _ := as.byID("work")
			work.secret.disown(hash(sideToken), true)
			if tt.gone {
				for range 2 {
					work.secret.reload(tokens.Token{}, tokenstest.Missing("work"), takenNote{}, start)
				}
			}
			if got, ok := as.byToken(sideToken, start); !ok || got.ID != tt.want {
				t.Errorf("a client sending side's token is %q (found: %v), want %s", got.ID, ok, tt.want)
			}
		})
	}
}

func TestAChoiceOfAnAccountThatLosesItsTokenBeforeTheRequestGoesOutIsMadeAgain(t *testing.T) {
	tests := []struct {
		name string
		// onSide has the session on side before its request; else it's new.
		onSide bool
		// wantMoved are the moves the router keeps of the session.
		wantMoved []string
	}{
		{name: "a session on it", onSide: true, wantMoved: []string{"side to work"}},
		{name: "a new session"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			r := newHeldRouter(t, clock, &stubProber{}, testTokens.Read, heldWork, heldSide)
			defer r.probes.stop()
			// Both have room, side's quota needing using sooner.
			r.state.record("work", []quota.Window{session, laterWeek}, r.state.mark())
			r.state.record("side", []quota.Window{session, soonWeek}, r.state.mark())
			one := key{session: "one", model: opus}
			if tt.onSide {
				assign(r.sessions, one, "", decision{account: "side", reason: reasonNew}, start)
			}
			side, _ := r.accounts.byID("side")
			r.proxy.chooser = &losingChooser{Chooser: r.proxy.chooser, lost: side}
			upstream := everyToken()
			r.proxy.transport = upstream

			if rec := routeOn(t, r, "one", sideToken); rec.Code != http.StatusOK {
				t.Errorf("answered %d, want 200: work has room", rec.Code)
			}
			if sent := upstream.sentWith(); !slices.Equal(sent, []string{workToken}) {
				t.Errorf("the request went out on %q, want work's token alone", sent)
			}
			if got := r.sessions.lookup(one).current; got.Account != "work" {
				t.Errorf("the session is on %q, want work", got.Account)
			}
			var moved []string
			for _, e := range r.recent.events() {
				if e.Kind == status.EventMoved {
					moved = append(moved, e.From+" to "+e.To)
				}
			}
			if !slices.Equal(moved, tt.wantMoved) {
				t.Errorf("the router keeps the session's moves %q, want %q", moved, tt.wantMoved)
			}
		})
	}
}

func TestARestartKeepsAnAccountWhoseTokenIsAnothersHeldOutTillItTakesUpAnother(t *testing.T) {
	tests := []struct {
		name string
		// files are the token files as the router restarts.
		files tokenstest.Files
		// fromBefore has the state file as a router from before kept it,
		// which kept none found another subscription's.
		fromBefore bool
		// foreign is whether side holds a token found another subscription's,
		// and heldOut whether nothing goes out on the token found so, its
		// file holding it or not, a client sending it routed elsewhere.
		foreign, heldOut bool
	}{
		{name: "holding the token found another's", files: testTokens, foreign: true, heldOut: true},
		{name: "holding another, taken up while the router was away", files: tokenstest.Files{"work": workToken, "side": sideOwnToken}},
		{name: "kept by a router from before", files: testTokens, fromBefore: true},
		{name: "holding none, its file gone while the router was away", files: tokenstest.Files{"work": workToken}, heldOut: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			prober := &stubProber{readings: map[string]quota.Probe{sideToken: orgProbed("test-org-other")}}
			before := newHeldRouter(t, clock, prober, testTokens.Read, heldWork, heldSide)
			path := filepath.Join(before.cfg.StateDir, stateFileName)
			before.file.load(path)
			probeNow(before, "side")
			before.file.save()
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			kept := `"foreign": "` + hash(sideToken) + `"`
			if !strings.Contains(string(data), kept) {
				t.Fatalf("the state file reads\n%s\nwant it to keep side's token found another subscription's", data)
			}
			if tt.fromBefore {
				var saved savedState
				if err := json.Unmarshal(data, &saved); err != nil {
					t.Fatal(err)
				}
				side := saved.Tokens["side"]
				side.Foreign = ""
				saved.Tokens["side"] = side
				if err := before.file.store(saved); err != nil {
					t.Fatal(err)
				}
			}

			// The router restarts a minute on.
			clock.now = start.Add(time.Minute)
			restartedProber := &stubProber{readings: map[string]quota.Probe{workToken: orgProbed("test-org-work")}}
			cfg := testConfig(clock.read, restartedProber, tt.files.Read)
			cfg.StateDir = before.cfg.StateDir
			after := newTestRouterOf(t, cfg)
			defer after.probes.stop()
			after.file.load(path)
			for _, u := range after.probes.start(after.accounts.sendable(), after.state.startDue) {
				<-u.done
			}
			after.rounds.look(t.Context())
			upstream := everyToken()
			after.proxy.transport = upstream

			side, _ := after.accounts.byID("side")
			if _, foreign := side.lacks(); foreign != tt.foreign || side.hasToken() == tt.heldOut {
				t.Errorf("side's token another subscription's: %v, has a token of its own: %v; want %v, %v", foreign, side.hasToken(), tt.foreign, !tt.heldOut)
			}
			if !tt.heldOut {
				return
			}
			if rec := routeOn(t, after, "one", sideToken); rec.Code != http.StatusOK {
				t.Errorf("answered %d, want 200", rec.Code)
			}
			if sent := upstream.sentWith(); !slices.Equal(sent, []string{workToken}) {
				t.Errorf("the first request went out on %q, want work's token alone", sent)
			}
			if n := restartedProber.counts()[sideToken]; n != 0 {
				t.Errorf("side's token probed %d times, want none: the state file kept it found another subscription's", n)
			}
		})
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
