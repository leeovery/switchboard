package router

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

var (
	start     = time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	session   = quota.Window{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC), Status: quota.StatusAllowed}
	week      = quota.Window{Key: "7d", Label: "Week", Utilization: 0.93, ResetsAt: time.Date(2026, 10, 2, 21, 0, 0, 0, time.UTC), Status: quota.StatusAllowedWarning}
	fableWeek = quota.Window{Key: "7d_oi", Label: "Fable week", Utilization: 0.05, ResetsAt: time.Date(2026, 10, 4, 1, 10, 0, 0, time.UTC), Status: quota.StatusAllowed}
)

func TestRecordKeepsEachWindowsLatestReading(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	later := start.Add(time.Minute)
	busier := session
	busier.Utilization = 0.31
	// A new five hours: less used than the reading it replaces, as the
	// window has reset since.
	anew := session
	anew.Utilization, anew.ResetsAt = 0.01, session.ResetsAt.Add(5*time.Hour)

	s.record("work", []quota.Window{session, week}, fromResponse)
	clock.now = later
	s.record("work", []quota.Window{busier}, fromProbe)

	u := s.usage["work"]
	want := map[string]reading{"5h": {Window: busier, at: later}, "7d": {Window: week, at: start}}
	if !reflect.DeepEqual(u.windows, want) {
		t.Errorf("windows =\n%+v\nwant\n%+v", u.windows, want)
	}
	if u.updated != later || u.from != fromProbe {
		t.Errorf("updated %v from %s, want %v from %s", u.updated, u.from, later, fromProbe)
	}

	clock.now = later.Add(time.Minute)
	s.record("work", []quota.Window{anew}, fromResponse)
	if got := s.usage["work"].windows["5h"]; got.Window != anew || got.at != clock.now {
		t.Errorf("after the window reset, 5h reads %+v, want %+v read at %v", got, anew, clock.now)
	}
	if other := s.usage["side"]; len(other.windows) > 0 || !other.updated.IsZero() {
		t.Errorf("side reads %+v, want nothing: every reading was work's", other)
	}
}

func TestRecordTimesAreWallClockUTC(t *testing.T) {
	local := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	s := newTestState(&testClock{now: local})

	s.record("work", []quota.Window{session}, fromResponse)
	if got := s.usage["work"].windows["5h"].at; got != local.UTC() {
		t.Errorf("read at %v, want %v", got, local.UTC())
	}
}

func TestProbesErrorsAndFailuresLastUntilRead(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	fableDown := quota.Failure{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}
	opusDown := quota.Failure{Label: "Opus", Window: "7d_opus", Error: "HTTP 529 · Overloaded"}
	work := func() status.Account {
		account, _ := s.document().Account("work")
		return account
	}

	s.recordProbe("work", quota.Usage{}, errors.New("HTTP 401 · Invalid bearer token"))
	if got := work(); got.Error != "HTTP 401 · Invalid bearer token" || !got.FetchedAt.IsZero() {
		t.Errorf("after a failed probe, work reads %+v, want the probe's error and nothing read", got)
	}

	s.recordProbe("work", quota.Usage{Windows: []quota.Window{session, week}, Failures: []quota.Failure{fableDown, opusDown}}, nil)
	want := status.Account{
		ID: "work", Label: "Work", TokenSet: true, FetchedAt: start,
		Windows: []quota.Window{session, week}, Failures: []quota.Failure{fableDown, opusDown},
	}
	if got := work(); !reflect.DeepEqual(got, want) {
		t.Errorf("after a probe that read, work reads\n%+v\nwant\n%+v", got, want)
	}

	clock.now = start.Add(time.Minute)
	s.record("work", []quota.Window{session, fableWeek}, fromResponse)
	if got := work().Failures; !reflect.DeepEqual(got, []quota.Failure{opusDown}) {
		t.Errorf("once the Fable week is read, failures = %+v, want only %+v", got, opusDown)
	}

	s.recordProbe("work", quota.Usage{}, errors.New("dial tcp: connection refused"))
	if got := work(); got.Error != "dial tcp: connection refused" || len(got.Windows) != 3 || got.FetchedAt != clock.now {
		t.Errorf("after another failed probe, work reads %+v, want its error beside the windows last read", got)
	}
	clock.now = start.Add(2 * time.Minute)
	s.record("work", []quota.Window{session}, fromResponse)
	if got := work().Error; got != "" {
		t.Errorf("once traffic reads the account, its error = %q, want none", got)
	}
}

func TestDocument(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	soonerWeek := week
	soonerWeek.Utilization, soonerWeek.ResetsAt = 0.5, start.Add(24*time.Hour)
	s.record("work", []quota.Window{week, session}, fromResponse)
	clock.now = start.Add(time.Minute)
	s.record("side", []quota.Window{session, soonerWeek}, fromProbe)

	want := status.Document{
		GeneratedAt: clock.now,
		Source:      "router",
		Best:        "side",
		Accounts: []status.Account{
			{ID: "work", Label: "Work", TokenSet: true, FetchedAt: start, Windows: []quota.Window{session, week}},
			{ID: "personal", Label: "Personal", Error: "token missing: set CLAUDE_TOKEN_PERSONAL"},
			{ID: "side", Label: "Side", TokenSet: true, FetchedAt: clock.now, Windows: []quota.Window{session, soonerWeek}},
		},
	}
	if got := s.document(); !reflect.DeepEqual(got, want) {
		t.Errorf("document() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestDocumentBeforeAnythingIsRead(t *testing.T) {
	s := newTestState(&testClock{now: start})

	want := status.Document{
		GeneratedAt: start,
		Source:      "router",
		Accounts: []status.Account{
			{ID: "work", Label: "Work", TokenSet: true},
			{ID: "personal", Label: "Personal", Error: "token missing: set CLAUDE_TOKEN_PERSONAL"},
			{ID: "side", Label: "Side", TokenSet: true},
		},
	}
	if got := s.document(); !reflect.DeepEqual(got, want) {
		t.Errorf("document() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestStateIsSafeForConcurrentUse(t *testing.T) {
	s := newTestState(&testClock{now: start})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { s.record("work", []quota.Window{session, week}, fromResponse) })
		wg.Go(func() { s.recordProbe("side", quota.Usage{Windows: []quota.Window{session}}, nil) })
		wg.Go(func() { s.recordProbe("work", quota.Usage{}, errors.New("HTTP 529 · Overloaded")) })
		wg.Go(func() { _ = s.document() })
	}
	wg.Wait()
}

// newTestState builds the state of three accounts on clock's time: work and
// side, with tokens, and personal, without one.
func newTestState(clock *testClock) *state {
	env := map[string]string{"CLAUDE_TOKEN_WORK": "test-token-work", "CLAUDE_TOKEN_SIDE": "test-token-side"}
	accounts := resolve([]config.Account{
		{ID: "work", Label: "Work", TokenEnv: "CLAUDE_TOKEN_WORK"},
		{ID: "personal", Label: "Personal", TokenEnv: "CLAUDE_TOKEN_PERSONAL"},
		{ID: "side", Label: "Side", TokenEnv: "CLAUDE_TOKEN_SIDE"},
	}, func(key string) string { return env[key] })
	return newState(accounts, score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d"}, clock.read)
}

// testClock is a clock that reads now, which a test moves as it goes.
type testClock struct {
	now time.Time
}

func (c *testClock) read() time.Time {
	return c.now
}
