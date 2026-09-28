package router

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

func TestRecordMergesEachWindowByItsReset(t *testing.T) {
	later := start.Add(time.Minute)
	sessionAt := func(utilization float64, resetsAt time.Time) quota.Window {
		w := session
		w.Utilization, w.ResetsAt = utilization, resetsAt
		return w
	}
	busier := sessionAt(0.31, session.ResetsAt)
	busierWarned := busier
	busierWarned.Status = quota.StatusAllowedWarning
	nextFive := sessionAt(0.01, session.ResetsAt.Add(5*time.Hour))
	lastFive := sessionAt(0.9, session.ResetsAt.Add(-5*time.Hour))
	unsure, unsureLower := sessionAt(0.5, time.Time{}), sessionAt(0.2, time.Time{})
	tests := []struct {
		name     string
		held     quota.Window
		incoming quota.Window
		want     reading
	}{
		{
			name:     "a later reset is a new window, taken however little it's used",
			held:     session,
			incoming: nextFive,
			want:     reading{Window: nextFive, at: later},
		},
		{
			name:     "the same reset is the same window, whose use has risen",
			held:     session,
			incoming: busier,
			want:     reading{Window: busier, at: later},
		},
		{
			name:     "the same reset read before, arriving late, leaves the higher use as of the later time",
			held:     busierWarned,
			incoming: session,
			want:     reading{Window: busierWarned, at: later},
		},
		{
			name:     "the same reset and use takes the latest reading",
			held:     busier,
			incoming: busierWarned,
			want:     reading{Window: busierWarned, at: later},
		},
		{
			name:     "an earlier reset is a window that's gone, and ignored",
			held:     session,
			incoming: lastFive,
			want:     reading{Window: session, at: start},
		},
		{
			name:     "without a reset, the newest reading stands",
			held:     unsure,
			incoming: unsureLower,
			want:     reading{Window: unsureLower, at: later},
		},
		{
			name:     "a reading without a reset replaces one with",
			held:     session,
			incoming: unsureLower,
			want:     reading{Window: unsureLower, at: later},
		},
		{
			name:     "a reading with a reset replaces one without",
			held:     unsure,
			incoming: session,
			want:     reading{Window: session, at: later},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			s := newTestState(clock)
			s.record("work", []quota.Window{tt.held}, fromResponse)
			clock.now = later

			s.record("work", []quota.Window{tt.incoming}, fromResponse)
			if got := s.usage["work"].windows["5h"]; got != tt.want {
				t.Errorf("5h reads %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRecordMergesOnlyTheWindowsItReads(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	later := start.Add(time.Minute)
	busier := session
	busier.Utilization = 0.31

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
	if other := s.usage["side"]; len(other.windows) > 0 || !other.updated.IsZero() {
		t.Errorf("side reads %+v, want nothing: every reading was work's", other)
	}
}

func TestRecordOfStaleWindowsLeavesTheAccountAsItWas(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	lastWeek := week
	lastWeek.Utilization, lastWeek.ResetsAt = 1, week.ResetsAt.Add(-7*24*time.Hour)
	s.recordProbe("work", quota.Probe{Windows: []quota.Window{session, week}}, nil)
	s.recordProbe("work", quota.Probe{}, errors.New("HTTP 529 · Overloaded"))
	clock.now = start.Add(time.Minute)

	s.record("work", []quota.Window{lastWeek}, fromResponse)
	u := s.usage["work"]
	if u.windows["7d"] != (reading{Window: week, at: start}) || u.updated != start || u.from != fromProbe || u.probeErr == "" {
		t.Errorf("after a stale reading, work reads %+v, want it as it was", u)
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

	s.recordProbe("work", quota.Probe{}, errors.New("HTTP 401 · Invalid bearer token"))
	if got := work(); got.Error != "HTTP 401 · Invalid bearer token" || !got.FetchedAt.IsZero() {
		t.Errorf("after a failed probe, work reads %+v, want the probe's error and nothing read", got)
	}

	s.recordProbe("work", quota.Probe{Windows: []quota.Window{session, week}, Failures: []quota.Failure{fableDown, opusDown}}, nil)
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

	s.recordProbe("work", quota.Probe{}, errors.New("dial tcp: connection refused"))
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
		wg.Go(func() { s.learn(opus, []quota.Window{session, week}) })
		wg.Go(func() { s.recordProbe("side", quota.Probe{Windows: []quota.Window{session}}, nil) })
		wg.Go(func() { s.recordProbe("work", quota.Probe{}, errors.New("HTTP 529 · Overloaded")) })
		wg.Go(func() { _ = s.document() })
		wg.Go(func() { _ = s.view(opus, start).room("work") })
		wg.Go(func() { _ = s.due("side", start) })
		wg.Go(func() { _ = s.dueAgain("side", start) })
		wg.Go(func() { s.refuse("side") })
	}
	wg.Wait()
}
