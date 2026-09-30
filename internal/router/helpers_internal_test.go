package router

import (
	"context"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/tokens"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

const (
	workToken = "test-token-work"
	sideToken = "test-token-side"
	// someRequest is the id of the request a test's refusal is of, where
	// which request doesn't matter.
	someRequest = "a1b2c3d4"
)

// The models requests ask for in tests, one of each family that matters.
// Sonnet's thinking is bound to the account that produced it.
const (
	opus   = "claude-opus-5-5"
	haiku  = "claude-haiku-4-5-20251001"
	fable  = "claude-fable-5-1"
	sonnet = "claude-sonnet-5-5"
)

// start is the time by the clock in tests: a Monday, 13:12 UTC.
var start = time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)

var (
	session   = quota.Window{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC), Status: quota.StatusAllowed}
	week      = quota.Window{Key: "7d", Label: "Week", Utilization: 0.93, ResetsAt: time.Date(2026, 10, 2, 21, 0, 0, 0, time.UTC), Status: quota.StatusAllowedWarning}
	fableWeek = quota.Window{Key: "7d_oi", Label: "Fable week", Utilization: 0.05, ResetsAt: time.Date(2026, 10, 4, 1, 10, 0, 0, time.UTC), Status: quota.StatusAllowed}
)

// testPolicy scores as Claude's policy does: the session and the week apply
// to every model, the week is perishable, and the session's reset decides
// between accounts scoring near enough equal.
var testPolicy = score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d", Tiebreak: "5h", Started: "5h", Pressure: "5h"}

// testConfigured are three accounts: work and side, whose tokens testTokens
// has, and personal, whose it hasn't.
var testConfigured = []config.Account{
	{ID: "work", Label: "Work"},
	{ID: "personal", Label: "Personal"},
	{ID: "side", Label: "Side"},
}

var testTokens = tokenstest.Files{"work": workToken, "side": sideToken}

// personalMissing is why personal has no token.
var personalMissing = tokenstest.Missing("personal").Error()

// testAccounts are testConfigured's, with their tokens.
func testAccounts() accounts {
	return resolve(testConfigured, testTokens.Read)
}

// newTestState builds the state of testAccounts on clock's time, knowing
// Claude's model families and scoring as Claude's policy does.
func newTestState(clock *testClock) *state {
	return newState(testAccounts(), testPolicy, claude.Provider{}.Family, clock.read, unkept, unkept)
}

// newTestFile builds a state file on now's time keeping what's known of the
// accounts given: their sessions, their tokens and their usage, as Claude's
// policy judges it.
func newTestFile(now func() time.Time, as accounts) *stateFile {
	changes := newChanges()
	usage := newState(as, testPolicy, claude.Provider{}.Family, now, changes.note, changes.routine)
	return newStateFile(now, changes, newSessions(now, changes.note, changes.routine), as, usage)
}

// unkept hears of a change for the state file, and keeps nothing of it.
func unkept() {}

// changeCount counts the changes it hears of, for the state file to keep.
type changeCount int

func (c *changeCount) hear() {
	*c++
}

// newTestRouter builds a router of testConfigured on now's time, probing with
// prober.
func newTestRouter(t *testing.T, now func() time.Time, prober Prober) *Router {
	t.Helper()
	return newTestRouterReading(t, now, prober, testTokens.Read)
}

// newTestRouterReading builds a router as newTestRouter does, reading the
// accounts' tokens with read.
func newTestRouterReading(t *testing.T, now func() time.Time, prober Prober, read func(id string) (tokens.Token, error)) *Router {
	t.Helper()
	r, err := New(Config{
		Accounts: testConfigured,
		Token:    read,
		Upstream: "http://127.0.0.1:1",
		Provider: claude.Provider{},
		Prober:   prober,
		Policy:   testPolicy,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return r
}

// choose has the router choose the account for req.
func choose(ctx context.Context, r *Router, req Request) Choice {
	return r.proxy.chooser.Choose(ctx, req)
}

// assign has the session and model k go where d says at at, carrying pin as
// the session's own, as a choice made on the assignment found then would.
func assign(s *sessions, k key, pin string, d decision, at time.Time) {
	assignFor(s, Request{Session: k.session, Model: k.model, Pin: pin}, d, at)
}

// assignFor has req go where d says at at, as a choice made on the assignment
// found then would.
func assignFor(s *sessions, req Request, d decision, at time.Time) {
	s.remember(req, s.lookup(req.key()).current, d, at)
}

// at is a clock stopped at t.
func at(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// testClock is a clock that reads now, which a test moves as it goes.
type testClock struct {
	now time.Time
}

func (c *testClock) read() time.Time {
	return c.now
}

// stubProber answers each token with its probe, and counts the probes of
// each. When it has a gate, each probe waits for the gate to close, or for
// its context to end.
type stubProber struct {
	readings map[string]quota.Probe
	gate     chan struct{}

	mu     sync.Mutex
	probes map[string]int
}

func (p *stubProber) Probe(ctx context.Context, token string) (quota.Probe, error) {
	p.mu.Lock()
	if p.probes == nil {
		p.probes = make(map[string]int)
	}
	p.probes[token]++
	p.mu.Unlock()
	if p.gate != nil {
		select {
		case <-p.gate:
		case <-ctx.Done():
			return quota.Probe{}, ctx.Err()
		}
	}
	return p.readings[token], nil
}

// counts returns how many probes each token has had.
func (p *stubProber) counts() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return maps.Clone(p.probes)
}

// probed is a probe that read windows, each reported by the models given under
// its key.
func probed(models map[string][]string, windows ...quota.Window) quota.Probe {
	return quota.Probe{Windows: windows, Models: models}
}
