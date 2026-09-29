package router

import (
	"errors"
	"net/http"
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
	busierRejected := busier
	busierRejected.Status = quota.StatusRejected
	rejected := session
	rejected.Status = quota.StatusRejected
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
			name:     "the same reset read before with a rejection, arriving late, leaves the higher use, rejected",
			held:     busierWarned,
			incoming: rejected,
			want:     reading{Window: busierRejected, at: later},
		},
		{
			name:     "a rejection stands against the same reset read with room before it",
			held:     busierRejected,
			incoming: session,
			want:     reading{Window: busierRejected, at: later},
		},
		{
			name:     "a rejection gives way to the same reset read as high since",
			held:     busierRejected,
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

func TestAReadingFromBeforeALimitNeverLiftsIt(t *testing.T) {
	sessionAt := func(utilization float64, status quota.Status) quota.Window {
		w := session
		w.Utilization, w.Status = utilization, status
		return w
	}
	clock := &testClock{now: start}
	s := newTestState(clock)
	s.record("work", []quota.Window{sessionAt(0.97, quota.StatusAllowedWarning), week}, fromResponse)
	// A 429 reads the session a little lower than it stands, as an answer to
	// a request sent earlier can, and rejects it; then an answer to one sent
	// earlier still reads it with room.
	clock.now = start.Add(time.Second)
	s.record("work", []quota.Window{sessionAt(0.96, quota.StatusRejected)}, fromResponse)
	s.limit("work", []string{"5h"}, session.ResetsAt)
	clock.now = start.Add(2 * time.Second)
	s.record("work", []quota.Window{sessionAt(0.965, quota.StatusAllowed)}, fromResponse)

	if s.view(opus, clock.now).room("work") {
		t.Error("work has room, want its limit to hold: the reading with room is from before it")
	}
	if got, want := s.usage["work"].windows["5h"].Window, sessionAt(0.97, quota.StatusRejected); got != want {
		t.Errorf("work's session reads %+v, want %+v: its highest use, rejected", got, want)
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

func TestTheBestIsNeverAnAccountBarredFromEveryRequest(t *testing.T) {
	soonerWeek := week
	soonerWeek.Utilization, soonerWeek.ResetsAt = 0.5, start.Add(24*time.Hour)
	tests := []struct {
		name string
		bar  func(s *state)
		want string
	}{
		{name: "side, whose quota needs using first, when nothing bars it", bar: func(*state) {}, want: "side"},
		{name: "not side once its token is refused", bar: func(s *state) { s.refuse("side", http.StatusUnauthorized) }, want: "work"},
		{
			name: "side once a request of one family is refused on it, which holds back that family alone",
			bar:  func(s *state) { s.forbid("side", "opus", http.StatusForbidden) },
			want: "side",
		},
		{
			name: "not side under a limit reached in a window every model shares",
			bar:  func(s *state) { s.limit("side", []string{"5h"}, start.Add(time.Hour)) },
			want: "work",
		},
		{
			name: "not side under a limit reached in no window named",
			bar:  func(s *state) { s.limit("side", nil, start.Add(time.Hour)) },
			want: "work",
		},
		{
			name: "side under a limit reached in a model's own window, which holds back that model alone",
			bar:  func(s *state) { s.limit("side", []string{"7d_oi"}, start.Add(time.Hour)) },
			want: "side",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestState(&testClock{now: start})
			s.record("work", []quota.Window{session, week}, fromResponse)
			s.record("side", []quota.Window{session, soonerWeek}, fromResponse)
			s.learn(fable, []quota.Window{fableWeek})

			tt.bar(s)
			if got := s.document().Best; got != tt.want {
				t.Errorf("Best = %q, want %q", got, tt.want)
			}
		})
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

func TestStandings(t *testing.T) {
	spent := session
	spent.Utilization, spent.Status = 1, quota.StatusRejected
	readWithRoom := func(s *state) { s.record("side", []quota.Window{session, week}, fromResponse) }
	// judged is how a standing judges an account.
	type judged struct {
		quota, known, refused bool
	}
	tests := []struct {
		name string
		// side sets side up, which after has passed since; work has room
		// throughout.
		side  func(s *state)
		after time.Duration
		want  judged
	}{
		{name: "nothing known of an account never read", side: func(*state) {}},
		{
			name: "quota while its shared windows have room",
			side: readWithRoom,
			want: judged{quota: true, known: true},
		},
		{
			name: "no quota while a shared window is spent",
			side: func(s *state) { s.record("side", []quota.Window{spent, week}, fromResponse) },
			want: judged{known: true},
		},
		{
			name: "no quota under a limit in a shared window, whatever its windows read",
			side: func(s *state) {
				readWithRoom(s)
				s.limit("side", []string{"5h"}, start.Add(time.Hour))
			},
			want: judged{known: true},
		},
		{
			name: "quota under a limit a model's own window holds",
			side: func(s *state) {
				s.record("side", []quota.Window{session, week, fableWeek}, fromResponse)
				s.limit("side", []string{"7d_oi"}, start.Add(time.Hour))
			},
			want: judged{quota: true, known: true},
		},
		{
			name: "refused, with its quota unknown when never read",
			side: func(s *state) { s.refuse("side", http.StatusUnauthorized) },
			want: judged{refused: true},
		},
		{
			name: "refused, whatever its quota",
			side: func(s *state) {
				readWithRoom(s)
				s.refuse("side", http.StatusUnauthorized)
			},
			want: judged{quota: true, known: true, refused: true},
		},
		{
			name: "no longer refused ten minutes on",
			side: func(s *state) {
				readWithRoom(s)
				s.refuse("side", http.StatusUnauthorized)
			},
			after: refusedFor,
			want:  judged{quota: true, known: true},
		},
		{
			name: "not refused once a request of one family alone is",
			side: func(s *state) {
				readWithRoom(s)
				s.forbid("side", "opus", http.StatusForbidden)
			},
			want: judged{quota: true, known: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestState(&testClock{now: start})
			s.record("work", []quota.Window{session, week}, fromResponse)
			tt.side(s)

			got := s.standings(start.Add(tt.after))
			if len(got) != 2 || got[0].ID != "work" || got[1].ID != "side" {
				t.Fatalf("standings() = %+v, want work's and side's: personal has no token", got)
			}
			if work := got[0]; !work.quota || !work.known || work.refused {
				t.Errorf("work stands %+v, want it known to have quota, and not refused", work)
			}
			if side := got[1]; (judged{side.quota, side.known, side.refused}) != tt.want {
				t.Errorf("side stands %+v, want %+v", judged{side.quota, side.known, side.refused}, tt.want)
			}
		})
	}
}

func TestASpentWindowsQuotaIsBackOnceItResets(t *testing.T) {
	spent := session
	spent.Utilization, spent.Status = 1, quota.StatusRejected
	s := newTestState(&testClock{now: start})
	s.record("work", []quota.Window{spent, week}, fromResponse)

	for at, want := range map[time.Time]bool{start: false, spent.ResetsAt.Add(-time.Second): false, spent.ResetsAt: true} {
		if got := s.standings(at)[0].quota; got != want {
			t.Errorf("at %v, work has quota: %v, want %v", at, got, want)
		}
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
		wg.Go(func() { _ = s.standings(start) })
		wg.Go(func() { _ = s.due("side", start) })
		wg.Go(func() { _ = s.dueAgain("side", start) })
		wg.Go(func() { s.refuse("side", http.StatusUnauthorized) })
		wg.Go(func() { s.forbid("work", "opus", http.StatusForbidden) })
		wg.Go(func() { _ = s.limit("work", []string{"5h"}, start.Add(time.Hour)) })
	}
	wg.Wait()
}
