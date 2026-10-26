package router

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
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
	// startedAgain is w, started again at t, as by a reset made by hand.
	startedAgain := func(w quota.Window, t time.Time) quota.Window {
		w.RestartedAt = t
		return w
	}
	earlier := start.Add(-time.Hour)
	tests := []struct {
		name     string
		held     quota.Window
		incoming quota.Window
		// since is set when incoming's request was sent after held was taken
		// in; otherwise it was sent before, and its answer arrives late.
		since bool
		want  quota.Window
	}{
		{
			name:     "a later reset is a new window, taken however little it's used",
			held:     session,
			incoming: nextFive,
			want:     nextFive,
		},
		{
			name:     "the same reset is the same window, whose use has risen",
			held:     session,
			incoming: busier,
			want:     busier,
		},
		{
			name:     "the same reset read before, arriving late, leaves the higher use",
			held:     busierWarned,
			incoming: session,
			want:     busierWarned,
		},
		{
			name:     "the same reset and use takes the latest reading",
			held:     busier,
			incoming: busierWarned,
			want:     busierWarned,
		},
		{
			name:     "the same reset read before with a rejection, arriving late, leaves the higher use, rejected",
			held:     busierWarned,
			incoming: rejected,
			want:     busierRejected,
		},
		{
			name:     "a rejection stands against the same reset read with room before it",
			held:     busierRejected,
			incoming: session,
			want:     busierRejected,
		},
		{
			name:     "a rejection stands against the same reset read as high before it",
			held:     busierRejected,
			incoming: busierWarned,
			want:     busierRejected,
		},
		{
			name:     "a rejection stands against the same reset read higher before it",
			held:     rejected,
			incoming: busierWarned,
			want:     busierRejected,
		},
		{
			name:     "a rejection gives way to the same reset read as high since",
			held:     busierRejected,
			incoming: busierWarned,
			since:    true,
			want:     busierWarned,
		},
		{
			name:     "a rejection gives way to the same reset read with room since",
			held:     busierRejected,
			incoming: session,
			since:    true,
			want:     session,
		},
		{
			name:     "the same reset read lower since stands, as the upstream's latest word",
			held:     busierWarned,
			incoming: session,
			since:    true,
			want:     session,
		},
		{
			name:     "the same reset read emptied since stands, started again then, as after a reset made by hand",
			held:     busierWarned,
			incoming: sessionAt(0.01, session.ResetsAt),
			since:    true,
			want:     startedAgain(sessionAt(0.01, session.ResetsAt), later),
		},
		{
			name:     "the same reset read as high since keeps when it started again",
			held:     startedAgain(session, earlier),
			incoming: busier,
			since:    true,
			want:     startedAgain(busier, earlier),
		},
		{
			name:     "the same reset read lower, arriving late, keeps when it started again",
			held:     startedAgain(busier, earlier),
			incoming: session,
			want:     startedAgain(busier, earlier),
		},
		{
			name:     "a rejection standing against a reading from before keeps when it started again",
			held:     startedAgain(busier, earlier),
			incoming: rejected,
			want:     startedAgain(busierRejected, earlier),
		},
		{
			name:     "a later reset, a new window, runs a whole length before it",
			held:     startedAgain(session, earlier),
			incoming: nextFive,
			want:     nextFive,
		},
		{
			name:     "an earlier reset is a window that's gone, and ignored",
			held:     session,
			incoming: lastFive,
			want:     session,
		},
		{
			name:     "an earlier reset read since stands, as the upstream's latest word",
			held:     session,
			incoming: lastFive,
			since:    true,
			want:     lastFive,
		},
		{
			name:     "without a reset, the newest reading stands",
			held:     unsure,
			incoming: unsureLower,
			want:     unsureLower,
		},
		{
			name:     "a reading without a reset replaces one with",
			held:     session,
			incoming: unsureLower,
			want:     unsureLower,
		},
		{
			name:     "a reading with a reset replaces one without",
			held:     unsure,
			incoming: session,
			want:     session,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			s := newTestState(clock)
			sent := s.mark()
			s.record("work", []quota.Window{tt.held}, s.mark())
			// Held stands as given, when it started again included, as a
			// reading the state file kept does.
			s.usage["work"].windows["5h"] = tt.held
			clock.now = later
			if tt.since {
				sent = s.mark()
			}

			s.record("work", []quota.Window{tt.incoming}, sent)
			if got := s.usage["work"].windows["5h"]; got != tt.want {
				t.Errorf("5h reads %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAReadingFromBeforeAResetMadeByHandIsStaleAfterIt(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	usedAt := func(u float64) []quota.Window {
		w := session
		w.Utilization = u
		return []quota.Window{w}
	}
	s.record("work", usedAt(0.5), s.mark())
	// One request is sent, then another; the second is answered first,
	// reading the session reset by hand.
	before := s.mark()
	clock.now = start.Add(time.Minute)
	s.record("work", usedAt(0.02), s.mark())

	s.record("work", usedAt(0.55), before)
	if got := s.usage["work"].windows["5h"]; got.Utilization != 0.02 || !got.RestartedAt.Equal(clock.now) {
		t.Errorf("the session reads %v, started again at %v, want 0.02, started again at %v: the late answer is from before the reset",
			got.Utilization, got.RestartedAt, clock.now)
	}
	s.record("work", usedAt(0.05), s.mark())
	if got := s.usage["work"].windows["5h"]; got.Utilization != 0.05 || !got.RestartedAt.Equal(clock.now) {
		t.Errorf("the session reads %v, started again at %v, want 0.05, used since the reset", got.Utilization, got.RestartedAt)
	}
}

func TestALimitFromBeforeAResetMadeByHandHoldsInTheWindowsNotReset(t *testing.T) {
	tests := []struct {
		name    string
		windows []string
		// since is set when the limit's request was sent after the one whose
		// answer showed the reset.
		since bool
		// want are the windows the limit holds in, when it holds.
		want  []string
		holds bool
	}{
		{name: "reached in the window reset", windows: []string{"5h"}},
		{name: "reached in it by a request sent since", windows: []string{"5h"}, since: true, want: []string{"5h"}, holds: true},
		{name: "reached in it and a window not reset", windows: []string{"5h", "7d"}, want: []string{"7d"}, holds: true},
		{name: "reached in no window named", holds: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			s := newTestState(clock)
			reset := session
			reset.Utilization = 0.02
			s.record("work", []quota.Window{session, week}, s.mark())
			// One request is sent, then another, answered first, reading the
			// session reset by hand.
			sent := s.mark()
			clock.now = start.Add(time.Minute)
			s.record("work", []quota.Window{reset}, s.mark())
			if tt.since {
				sent = s.mark()
			}

			reached, holds := s.limit("work", tt.windows, week.ResetsAt, sent)
			if holds != tt.holds || holds && !slices.Equal(reached.Windows, tt.want) {
				t.Errorf("limit() = %+v, %t, want it to hold (%t) in %q", reached, holds, tt.holds, tt.want)
			}
			if got := s.usage["work"].limited; got.inForce(clock.now) != tt.holds || !slices.Equal(got.windows, tt.want) {
				t.Errorf("work's limit is %+v, want it in force (%t) in %q", got, tt.holds, tt.want)
			}
			if got := s.view(opus, clock.now).room("work"); got == tt.holds {
				t.Errorf("work has room: %t, want %t", got, !tt.holds)
			}
		})
	}
}

func TestAReadingReadAgainSinceOutweighsALateAnswerToARequestSentBefore(t *testing.T) {
	s := newTestState(&testClock{now: start})
	busier := session
	busier.Utilization = 0.31
	s.record("work", []quota.Window{busier}, s.mark())
	// A request is sent, then another, sent after it, reads the session as
	// it was; the first is answered last, reading it lower.
	sent := s.mark()
	s.record("work", []quota.Window{busier}, s.mark())
	s.record("work", []quota.Window{session}, sent)

	if got := s.usage["work"].windows["5h"]; got != busier {
		t.Errorf("5h reads %+v, want %+v: the reading since may be the newer", got, busier)
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
	earlier, earlierStill := s.mark(), s.mark()
	s.record("work", []quota.Window{sessionAt(0.97, quota.StatusAllowedWarning), week}, s.mark())
	// A 429 reads the session a little lower than it stands, as an answer to
	// a request sent earlier can, and rejects it; then an answer to one sent
	// earlier still reads it with room.
	clock.now = start.Add(time.Second)
	s.record("work", []quota.Window{sessionAt(0.96, quota.StatusRejected)}, earlier)
	s.limit("work", []string{"5h"}, session.ResetsAt, s.mark())
	clock.now = start.Add(2 * time.Second)
	s.record("work", []quota.Window{sessionAt(0.965, quota.StatusAllowed)}, earlierStill)

	if s.view(opus, clock.now).room("work") {
		t.Error("work has room, want its limit to hold: the reading with room is from before it")
	}
	if got, want := s.usage["work"].windows["5h"], sessionAt(0.97, quota.StatusRejected); got != want {
		t.Errorf("work's session reads %+v, want %+v: its highest use, rejected", got, want)
	}
}

func TestAReadingAsHighArrivingLateLiftsNoLimit(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	spent := session
	spent.Utilization, spent.Status = 0.97, quota.StatusRejected
	s.record("work", []quota.Window{spent, week}, s.mark())
	s.limit("work", []string{"5h"}, session.ResetsAt, s.mark())
	// A request sent since the limit was set is answered late, reading the
	// session as high, with room, after a probe sent later read it rejected.
	sent := s.mark()
	s.record("work", []quota.Window{spent}, s.mark())
	withRoom := spent
	withRoom.Status = quota.StatusAllowedWarning
	s.record("work", []quota.Window{withRoom}, sent)

	if s.view(opus, clock.now).room("work") {
		t.Error("work has room, want its limit to hold: a reading as high that may be the older can't lift a rejection")
	}
	if got := s.usage["work"].windows["5h"]; got != spent {
		t.Errorf("work's session reads %+v, want %+v, rejected", got, spent)
	}
}

// A limit lifts on the answer to a request sent since it was set, and the
// answer tells of it lifting early, the news of it, where it was in force
// as it lifted: not of one that had ended by then.
func TestALimitLiftsOnlyOnTheAnswerToARequestSentSinceItWasSet(t *testing.T) {
	admitted := func(s *state, sent moment) []LimitLifted { return s.admitted("work", sent) }
	// readRoom reads work's Fable week with room, which the 429 that set the
	// limit didn't read.
	readRoom := func(s *state, sent moment) []LimitLifted {
		return s.record("work", []quota.Window{fableWeek}, sent)
	}
	// probeRoom has a probe read it so, answered with success.
	probeRoom := func(s *state, sent moment) []LimitLifted {
		return s.recordProbe("work", quota.Probe{Windows: []quota.Window{fableWeek}, Admitted: true}, nil, sent, readings.FromProbe).lifted
	}
	until := start.Add(72 * time.Hour)
	tests := []struct {
		name string
		// reached are the windows the limit was reached in.
		reached []string
		// answer answers a request on work, sent at sent, at.
		answer func(s *state, sent moment) []LimitLifted
		at     time.Time
		// since is set when the request was sent after the limit was set.
		since      bool
		wantLifted bool
		// wantTold is set where the answer tells of it lifting early.
		wantTold bool
	}{
		{name: "a success since, of a limit in no window named", answer: admitted, since: true, wantLifted: true, wantTold: true},
		{name: "a success sent before, of a limit in no window named", answer: admitted},
		{name: "a success since, of a limit in a window named", reached: []string{"7d_oi"}, answer: admitted, since: true},
		{name: "a reading with room since, of a limit in a window named", reached: []string{"7d_oi"}, answer: readRoom, since: true, wantLifted: true, wantTold: true},
		{name: "a reading with room sent before, of a limit in a window named", reached: []string{"7d_oi"}, answer: readRoom},
		{name: "a probe's reading with room since", reached: []string{"7d_oi"}, answer: probeRoom, since: true, wantLifted: true, wantTold: true},
		{name: "a probe answered with success since", answer: probeRoom, since: true, wantLifted: true, wantTold: true},
		{name: "a reading with room since, of a limit that had ended", reached: []string{"7d_oi"}, answer: readRoom, at: until, since: true, wantLifted: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			s := newTestState(clock)
			s.record("work", []quota.Window{session, week, fableWeek}, s.mark())
			sent := s.mark()
			s.limit("work", tt.reached, until, s.mark())
			if tt.since {
				sent = s.mark()
			}
			if !tt.at.IsZero() {
				clock.now = tt.at
			}

			news := tt.answer(s, sent)
			if lifted := !s.usage["work"].limited.inForce(start); lifted != tt.wantLifted {
				t.Errorf("the limit lifted = %v, want %v", lifted, tt.wantLifted)
			}
			var want []LimitLifted
			if tt.wantTold {
				want = []LimitLifted{{Account: "work", Limit: 1}}
			}
			if !slices.Equal(news, want) {
				t.Errorf("the answer tells of %+v, want %+v", news, want)
			}
		})
	}
}

// An older limit a later one took the place of lifts as a reading shows its
// windows with room, by the rule that lifts the one the account holds, and
// the reading tells of it, the later limit holding still; lifted, it's
// never reached again, so a limit reached in its window after is another.
func TestAnOlderLimitLiftsAsTheReadingsShowItThoughALaterOneHolds(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	s.record("work", []quota.Window{session, week, fableWeek}, s.mark())
	s.limit("work", []string{"7d_oi"}, start.Add(72*time.Hour), s.mark())
	clock.now = start.Add(time.Minute)
	s.limit("work", []string{"5h"}, start.Add(3*time.Hour), s.mark())
	clock.now = start.Add(2 * time.Minute)

	if got, want := s.record("work", []quota.Window{fableWeek}, s.mark()), []LimitLifted{{Account: "work", Limit: 1}}; !slices.Equal(got, want) {
		t.Errorf("the reading tells of %+v lifting, want %+v, the Fable week's limit", got, want)
	}
	if got := s.usage["work"].limited; got.id != 2 || !got.inForce(clock.now) {
		t.Errorf("work holds %+v, want the 5-hour's limit, 2, still in force", got)
	}
	if got, _ := s.limit("work", []string{"7d_oi"}, start.Add(72*time.Hour), s.mark()); got.Again || got.Limit == 1 {
		t.Errorf("limit() = %+v, want a limit of its own, not the Fable week's lifted one reached again", got)
	}
}

// An older limit, reached again before a later one took its place, lifts by
// the rule the one the account holds does: only on the answer to a request
// sent since it was last reached, as one sent before may show room there was
// before it.
func TestAnOlderLimitLiftsOnlyOnTheAnswerToARequestSentSinceItWasLastReached(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	s.record("work", []quota.Window{session, week, fableWeek}, s.mark())
	s.limit("work", []string{"7d_oi"}, start.Add(72*time.Hour), s.mark())
	between := s.mark()
	s.limit("work", []string{"7d_oi"}, start.Add(72*time.Hour), s.mark())
	clock.now = start.Add(time.Minute)
	s.limit("work", []string{"5h"}, start.Add(3*time.Hour), s.mark())
	clock.now = start.Add(2 * time.Minute)

	if got := s.record("work", []quota.Window{fableWeek}, between); len(got) > 0 {
		t.Errorf("the answer to a request sent before the Fable week's limit was reached again tells of %+v lifting, want none", got)
	}
	if got, want := s.record("work", []quota.Window{fableWeek}, s.mark()), []LimitLifted{{Account: "work", Limit: 1}}; !slices.Equal(got, want) {
		t.Errorf("the answer to a request sent since tells of %+v lifting, want %+v, the Fable week's limit", got, want)
	}
}

func TestRecordMergesOnlyTheWindowsItReads(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	later := start.Add(time.Minute)
	busier := session
	busier.Utilization = 0.31

	s.record("work", []quota.Window{session, week}, s.mark())
	clock.now = later
	s.record("work", []quota.Window{busier}, s.mark())

	u := s.usage["work"]
	want := map[string]quota.Window{"5h": busier, "7d": week}
	if !reflect.DeepEqual(u.windows, want) {
		t.Errorf("windows =\n%+v\nwant\n%+v", u.windows, want)
	}
	if u.updated != later {
		t.Errorf("updated %v, want %v", u.updated, later)
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
	sent := s.mark()
	s.recordProbe("work", quota.Probe{Windows: []quota.Window{session, week}}, nil, s.mark(), readings.FromProbe)
	s.recordProbe("work", quota.Probe{}, errors.New("HTTP 529 · Overloaded"), s.mark(), readings.FromProbe)
	clock.now = start.Add(time.Minute)

	s.record("work", []quota.Window{lastWeek}, sent)
	u := s.usage["work"]
	if u.windows["7d"] != week || u.updated != start || u.probeErr == "" {
		t.Errorf("after a stale reading, work reads %+v, want it as it was", u)
	}
}

func TestRecordTimesAreWallClockUTC(t *testing.T) {
	local := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	s := newTestState(&testClock{now: local})

	s.record("work", []quota.Window{session}, s.mark())
	if got := s.usage["work"].updated; got != local.UTC() {
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

	s.recordProbe("work", quota.Probe{}, errors.New("HTTP 401 · Invalid bearer token"), s.mark(), readings.FromProbe)
	if got := work(); got.Error != "HTTP 401 · Invalid bearer token" || !got.FetchedAt.IsZero() {
		t.Errorf("after a failed probe, work reads %+v, want the probe's error and nothing read", got)
	}

	s.recordProbe("work", quota.Probe{Windows: []quota.Window{session, week}, Failures: []quota.Failure{fableDown, opusDown}}, nil, s.mark(), readings.FromProbe)
	want := status.Account{
		ID: "work", Label: "Work", TokenSet: true, FetchedAt: start,
		Windows: []quota.Window{session, week}, Failures: []quota.Failure{fableDown, opusDown},
	}
	if got := work(); !reflect.DeepEqual(got, want) {
		t.Errorf("after a probe that read, work reads\n%+v\nwant\n%+v", got, want)
	}

	clock.now = start.Add(time.Minute)
	s.record("work", []quota.Window{session, fableWeek}, s.mark())
	if got := work().Failures; !reflect.DeepEqual(got, []quota.Failure{opusDown}) {
		t.Errorf("once the Fable week is read, failures = %+v, want only %+v", got, opusDown)
	}

	s.recordProbe("work", quota.Probe{}, errors.New("dial tcp: connection refused"), s.mark(), readings.FromProbe)
	if got := work(); got.Error != "dial tcp: connection refused" || len(got.Windows) != 3 || got.FetchedAt != clock.now {
		t.Errorf("after another failed probe, work reads %+v, want its error beside the windows last read", got)
	}
	clock.now = start.Add(2 * time.Minute)
	s.record("work", []quota.Window{session}, s.mark())
	if got := work().Error; got != "" {
		t.Errorf("once traffic reads the account, its error = %q, want none", got)
	}
}

func TestProbesThatReadNothingAreCountedToTheBackoffsEnd(t *testing.T) {
	s := newTestState(&testClock{now: start})
	for range backingOff + 2 {
		s.recordProbe("work", quota.Probe{}, errUnreadable, s.mark(), readings.FromProbe)
	}
	if got := s.usage["work"].misses; got != backingOff {
		t.Errorf("after %d probes in a row that read nothing, counted %d, want %d: more change nothing", backingOff+2, got, backingOff)
	}
}

func TestTheStartupProbeIsDueOfAnAccountNothingHasReadThatCanBeProbed(t *testing.T) {
	failed := func(s *state) {
		s.recordProbe("work", quota.Probe{}, errors.New("HTTP 529 · Overloaded"), s.mark(), readings.FromProbe)
	}
	kept := func(windows ...quota.Window) func(s *state) {
		return func(s *state) {
			s.recall(savedUsage{Readings: map[string]savedReading{"work": {Windows: windows}}}, stillHeld)
		}
	}
	lapsed, spentWeek := session, week
	lapsed.ResetsAt = start.Add(-time.Hour)
	spentWeek.Utilization, spentWeek.Status = 1, quota.StatusRejected
	tests := []struct {
		name   string
		before func(s *state)
		after  time.Duration
		want   bool
	}{
		{name: "never probed", before: func(*state) {}, want: true},
		{name: "a probe of it failed just now", before: failed, want: false},
		{name: "a probe of it failed a second short of a minute ago", before: failed, after: reprobeAfter - time.Second, want: false},
		{name: "a probe of it failed a minute ago", before: failed, after: reprobeAfter, want: true},
		{name: "a probe of it read", before: func(s *state) {
			s.recordProbe("work", quota.Probe{Windows: []quota.Window{session, week}}, nil, s.mark(), readings.FromProbe)
		}, after: reprobeAfter, want: false},
		{name: "kept without a read time, its 5-hour window lapsed", before: kept(lapsed, week), want: false},
		{name: "kept without a read time, its 5-hour window lapsed and its week spent", before: kept(lapsed, spentWeek), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestState(&testClock{now: start})
			tt.before(s)
			if got := s.startDue("work", start.Add(tt.after)); got != tt.want {
				t.Errorf("%v on, the startup probe due of work: %v, want %v", tt.after, got, tt.want)
			}
		})
	}
}

func TestDocument(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	soonerWeek := week
	soonerWeek.Utilization, soonerWeek.ResetsAt = 0.5, start.Add(24*time.Hour)
	s.record("work", []quota.Window{week, session}, s.mark())
	clock.now = start.Add(time.Minute)
	s.record("side", []quota.Window{session, soonerWeek}, s.mark())

	want := status.Document{
		GeneratedAt: clock.now,
		Source:      "router",
		Best:        "side",
		Accounts: []status.Account{
			{ID: "work", Label: "Work", TokenSet: true, FetchedAt: start, Windows: []quota.Window{session, week}},
			{ID: "personal", Label: "Personal", Error: personalMissing},
			{ID: "side", Label: "Side", TokenSet: true, FetchedAt: clock.now, Windows: []quota.Window{session, soonerWeek}},
		},
	}
	if got := s.document(); !reflect.DeepEqual(got, want) {
		t.Errorf("document() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestTheDocumentGivesEachAccountsRefusalWhileItsInForce(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	s.forbid("work", "opus", http.StatusForbidden, someRequest)
	s.refuse("work", http.StatusUnauthorized, someRequest, s.mark())
	s.forbid("side", "opus", http.StatusForbidden, someRequest)
	clock.now = start.Add(time.Minute)
	s.forbid("side", "fable", http.StatusForbidden, someRequest)
	refused := func(id string) status.Refusal {
		account, _ := s.document().Account(id)
		return account.Refused
	}

	steps := []struct {
		after      time.Duration
		work, side status.Refusal
	}{
		{
			after: time.Minute,
			work:  status.Refusal{Until: start.Add(refusedFor), Status: http.StatusUnauthorized},
			side:  status.Refusal{Until: start.Add(time.Minute + refusedFor), Status: http.StatusForbidden, Family: "fable"},
		},
		{
			after: refusedFor,
			side:  status.Refusal{Until: start.Add(time.Minute + refusedFor), Status: http.StatusForbidden, Family: "fable"},
		},
		{after: time.Minute + refusedFor},
	}
	for _, step := range steps {
		clock.now = start.Add(step.after)
		if got := refused("work"); got != step.work {
			t.Errorf("%v on, work is refused %+v, want %+v: its token's refusal over its family's", step.after, got, step.work)
		}
		if got := refused("side"); got != step.side {
			t.Errorf("%v on, side is refused %+v, want %+v: the latest in force", step.after, got, step.side)
		}
	}
}

func TestARequestTakesBackItsOwnRefusalsOfItsFamilyAlone(t *testing.T) {
	const first, second = "a1b2c3d4", "e5f6a7b8"
	clock := &testClock{now: start}
	s := newTestState(clock)
	s.forbid("work", "opus", http.StatusForbidden, first)
	clock.now = start.Add(time.Minute)
	s.forbid("work", "opus", http.StatusForbidden, second)
	s.forbid("side", "opus", http.StatusForbidden, second)
	s.refuse("personal", http.StatusUnauthorized, second, s.mark())
	refused := func(id string) status.Refusal {
		account, _ := s.document().Account(id)
		return account.Refused
	}

	want := []RefusalLifted{{Account: "work", Family: "opus", Request: second}, {Account: "side", Family: "opus", Request: second}}
	if got := s.takeBack(second); !reflect.DeepEqual(got, want) {
		t.Errorf("takeBack() = %+v, want %+v: the second request's refusals of Opus taken back, each lifting", got, want)
	}
	if got, want := refused("work"), (status.Refusal{Until: start.Add(refusedFor), Status: http.StatusForbidden, Family: "opus"}); got != want {
		t.Errorf("with the second request's refusals taken back, work is refused %+v, want %+v: the first request's refusal stands", got, want)
	}
	if got := refused("side"); got != (status.Refusal{}) {
		t.Errorf("with the second request's refusals taken back, side is refused %+v, want not", got)
	}
	if got, want := s.usage["personal"].refused.latest(), (refusal{at: start.Add(time.Minute), status: http.StatusUnauthorized, by: second}); got != want {
		t.Errorf("with the second request's refusals taken back, personal's token is refused %+v, want %+v: that says something of personal", got, want)
	}
	s.takeBack(first)
	if got := refused("work"); got != (status.Refusal{}) {
		t.Errorf("with both requests' refusals taken back, work is refused %+v, want not", got)
	}
	if got := s.takeBack(first); got != nil {
		t.Errorf("takeBack() = %+v, want none: the first request's refusals are gone already", got)
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
		{name: "not side once its token is refused", bar: func(s *state) { s.refuse("side", http.StatusUnauthorized, someRequest, s.mark()) }, want: "work"},
		{
			name: "side once a request of one family is refused on it, which holds back that family alone",
			bar:  func(s *state) { s.forbid("side", "opus", http.StatusForbidden, someRequest) },
			want: "side",
		},
		{
			name: "not side under a limit reached in a window every model shares",
			bar:  func(s *state) { s.limit("side", []string{"5h"}, start.Add(time.Hour), s.mark()) },
			want: "work",
		},
		{
			name: "not side under a limit reached in no window named",
			bar:  func(s *state) { s.limit("side", nil, start.Add(time.Hour), s.mark()) },
			want: "work",
		},
		{
			name: "side under a limit reached in a model's own window, which holds back that model alone",
			bar:  func(s *state) { s.limit("side", []string{"7d_oi"}, start.Add(time.Hour), s.mark()) },
			want: "side",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestState(&testClock{now: start})
			s.record("work", []quota.Window{session, week}, s.mark())
			s.record("side", []quota.Window{session, soonerWeek}, s.mark())
			s.learn(fable, []quota.Window{fableWeek})

			tt.bar(s)
			if got := s.document().Best; got != tt.want {
				t.Errorf("Best = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTheBestIsWhereANewSessionGoes(t *testing.T) {
	r := newTestRouter(t, at(start), &stubProber{})
	// Work's quota needs using first.
	r.state.record("work", []quota.Window{session, soonWeek}, r.state.mark())
	r.state.record("side", []quota.Window{session, laterWeek}, r.state.mark())
	steps := []struct {
		name string
		// change changes what the choice is made on.
		change func()
		want   string
	}{
		{name: "unpinned", change: func() {}, want: "work"},
		{name: "pinned to side", change: func() { r.sessions.setPin(status.Pin{Accounts: []string{"side"}, Since: start}, false) }, want: "side"},
		{name: "pinned to side, whose token is refused", change: func() { r.state.refuse("side", http.StatusUnauthorized, someRequest, r.state.mark()) }, want: "work"},
	}
	for i, step := range steps {
		step.change()
		if got := r.Status().Best; got != step.want {
			t.Errorf("%s, Best = %q, want %q", step.name, got, step.want)
		}
		session := fmt.Sprintf("new-%d", i)
		if got := choose(t.Context(), r, Request{ID: session, Session: session, Model: opus, Client: "work"}); got.Account != step.want {
			t.Errorf("%s, a new session goes to %s, want %s, the best", step.name, got.Account, step.want)
		}
	}
}

func TestDocumentBeforeAnythingIsRead(t *testing.T) {
	s := newTestState(&testClock{now: start})

	want := status.Document{
		GeneratedAt: start,
		Source:      "router",
		Accounts: []status.Account{
			{ID: "work", Label: "Work", TokenSet: true},
			{ID: "personal", Label: "Personal", Error: personalMissing},
			{ID: "side", Label: "Side", TokenSet: true},
		},
	}
	if got := s.document(); !reflect.DeepEqual(got, want) {
		t.Errorf("document() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestTheWindowARequestStartsReadsEmptyOnceItHasLapsed(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	s.record("work", []quota.Window{session, week}, s.mark())
	empty := quota.Window{Key: "5h", Label: "Session"}
	started := session
	started.Utilization, started.ResetsAt = 0.02, session.ResetsAt.Add(3*time.Hour)
	steps := []struct {
		name string
		at   time.Time
		// read are windows read of work at the step's time, if any.
		read       []quota.Window
		want       []quota.Window
		wantLapsed []string
	}{
		{name: "while it runs, as read", at: session.ResetsAt.Add(-time.Second), want: []quota.Window{session, week}},
		{name: "at its reset, empty, the week standing as read", at: session.ResetsAt, want: []quota.Window{empty, week}, wantLapsed: []string{"5h"}},
		{name: "long after, with nothing read since, empty", at: session.ResetsAt.Add(2 * time.Hour), want: []quota.Window{empty, week}, wantLapsed: []string{"5h"}},
		{name: "once a request starts it again, as read", at: session.ResetsAt.Add(2 * time.Hour), read: []quota.Window{started}, want: []quota.Window{started, week}},
	}
	for _, step := range steps {
		clock.now = step.at
		if step.read != nil {
			s.record("work", step.read, s.mark())
		}
		work, _ := s.document().Account("work")
		if !reflect.DeepEqual(work.Windows, step.want) || !slices.Equal(work.Lapsed, step.wantLapsed) {
			t.Errorf("%s: work reads %+v, lapsed %q; want %+v, lapsed %q", step.name, work.Windows, work.Lapsed, step.want, step.wantLapsed)
		}
		if !s.view(opus, step.at).room("work") || !s.standings(step.at)[0].quota {
			t.Errorf("%s: work has no room, want room: a request would start its session", step.name)
		}
	}
}

func TestTheStateTellsOfEachChangeTheStateFileKeeps(t *testing.T) {
	lastWeek := week
	lastWeek.ResetsAt = week.ResetsAt.Add(-7 * 24 * time.Hour)
	overloaded := errors.New("HTTP 529 · Overloaded")
	tests := []struct {
		name string
		// change changes s, whose reading of work was taken in after earlier.
		change func(s *state, earlier moment)
		// want counts the changes told of, and wantRoutine the routine ones:
		// readings off the answers to requests, readings as before, and how
		// probes went.
		want, wantRoutine changeCount
	}{
		{name: "a reading off an answer", change: func(s *state, _ moment) { s.record("side", []quota.Window{session}, s.mark()) }, wantRoutine: 1},
		{name: "a stale reading, which changes nothing", change: func(s *state, earlier moment) { s.record("work", []quota.Window{lastWeek}, earlier) }},
		{name: "a probe that read", change: func(s *state, _ moment) {
			s.recordProbe("side", probed(nil, session), nil, s.mark(), readings.FromProbe)
		}, want: 1},
		{name: "a probe that read the windows as they were", change: func(s *state, _ moment) {
			s.recordProbe("work", probed(nil, session, week), nil, s.mark(), readings.FromProbe)
		}, wantRoutine: 1},
		{name: "a probe that failed", change: func(s *state, _ moment) {
			s.recordProbe("side", quota.Probe{}, overloaded, s.mark(), readings.FromProbe)
		}, wantRoutine: 1},
		{name: "a probe that read stale windows", change: func(s *state, earlier moment) {
			s.recordProbe("work", probed(nil, lastWeek), nil, earlier, readings.FromProbe)
		}, wantRoutine: 1},
		{name: "a probe on the token before that read the windows as they were", change: func(s *state, _ moment) {
			sent := s.mark()
			s.tokenReplaced("work")
			s.recordProbe("work", probed(nil, session, week), nil, sent, readings.FromProbe)
		}, wantRoutine: 1},
		{name: "a probe on the token before that failed", change: func(s *state, earlier moment) {
			s.tokenReplaced("side")
			s.recordProbe("side", quota.Probe{}, overloaded, earlier, readings.FromProbe)
		}},
		{name: "a window seen on a family anew", change: func(s *state, _ moment) { s.learn(fable, []quota.Window{week}) }, want: 1},
		{name: "a window seen on its family before", change: func(s *state, _ moment) { s.learn(opus, []quota.Window{week}) }},
		{name: "a refusal, which isn't kept", change: func(s *state, _ moment) { s.refuse("work", http.StatusUnauthorized, someRequest, s.mark()) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var changes, routine changeCount
			s := newState(testAccounts(), testPolicy, claude.Provider{}.Family, at(start), changes.hear, routine.hear)
			earlier := s.mark()
			s.record("work", []quota.Window{session, week}, s.mark())
			s.learn(opus, []quota.Window{session, week})
			changes, routine = 0, 0

			tt.change(s, earlier)
			if changes != tt.want || routine != tt.wantRoutine {
				t.Errorf("told of %d changes and %d routine ones, want %d and %d", changes, routine, tt.want, tt.wantRoutine)
			}
		})
	}
}

func TestStandings(t *testing.T) {
	spent := session
	spent.Utilization, spent.Status = 1, quota.StatusRejected
	readWithRoom := func(s *state) { s.record("side", []quota.Window{session, week}, s.mark()) }
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
			side: func(s *state) { s.record("side", []quota.Window{spent, week}, s.mark()) },
			want: judged{known: true},
		},
		{
			name: "no quota under a limit in a shared window, whatever its windows read",
			side: func(s *state) {
				readWithRoom(s)
				s.limit("side", []string{"5h"}, start.Add(time.Hour), s.mark())
			},
			want: judged{known: true},
		},
		{
			name: "quota under a limit a model's own window holds",
			side: func(s *state) {
				s.record("side", []quota.Window{session, week, fableWeek}, s.mark())
				s.limit("side", []string{"7d_oi"}, start.Add(time.Hour), s.mark())
			},
			want: judged{quota: true, known: true},
		},
		{
			name: "refused, with its quota unknown when never read",
			side: func(s *state) { s.refuse("side", http.StatusUnauthorized, someRequest, s.mark()) },
			want: judged{refused: true},
		},
		{
			name: "refused, whatever its quota",
			side: func(s *state) {
				readWithRoom(s)
				s.refuse("side", http.StatusUnauthorized, someRequest, s.mark())
			},
			want: judged{quota: true, known: true, refused: true},
		},
		{
			name: "no longer refused ten minutes on",
			side: func(s *state) {
				readWithRoom(s)
				s.refuse("side", http.StatusUnauthorized, someRequest, s.mark())
			},
			after: refusedFor,
			want:  judged{quota: true, known: true},
		},
		{
			name: "not refused once a request of one family alone is",
			side: func(s *state) {
				readWithRoom(s)
				s.forbid("side", "opus", http.StatusForbidden, someRequest)
			},
			want: judged{quota: true, known: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestState(&testClock{now: start})
			s.record("work", []quota.Window{session, week}, s.mark())
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
	s.record("work", []quota.Window{spent, week}, s.mark())

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
		wg.Go(func() { s.record("work", []quota.Window{session, week}, s.mark()) })
		wg.Go(func() { s.learn(opus, []quota.Window{session, week}) })
		wg.Go(func() {
			s.recordProbe("side", quota.Probe{Windows: []quota.Window{session}}, nil, s.mark(), readings.FromProbe)
		})
		wg.Go(func() {
			s.recordProbe("work", quota.Probe{}, errors.New("HTTP 529 · Overloaded"), s.mark(), readings.FromProbe)
		})
		wg.Go(func() { _ = s.document() })
		wg.Go(func() { _ = s.view(opus, start).room("work") })
		wg.Go(func() { _ = s.standings(start) })
		wg.Go(func() { _ = s.due("side", start) })
		wg.Go(func() { _ = s.dueAgain("side", start) })
		wg.Go(func() { _ = s.startDue("side", start) })
		wg.Go(func() { _ = s.saved() })
		wg.Go(func() { s.refuse("side", http.StatusUnauthorized, someRequest, s.mark()) })
		wg.Go(func() { s.forbid("work", "opus", http.StatusForbidden, someRequest) })
		wg.Go(func() { _, _ = s.limit("work", []string{"5h"}, start.Add(time.Hour), s.mark()) })
	}
	wg.Wait()
}
