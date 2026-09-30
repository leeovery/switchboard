package router

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// sessionReading is a reading of an account's session, used as given, which
// resets at resets, taken in after start.
type sessionReading struct {
	after       time.Duration
	utilization float64
	resets      time.Time
	// late is set when its request was sent before the reading the account
	// stood as then was taken in, so its answer arrives late.
	late bool
}

// readSessions has the state take in the readings of the account's session
// given, in turn, the clock moved on to each as it's taken.
func readSessions(s *state, clock *testClock, id string, readings []sessionReading) {
	before := s.mark()
	for _, r := range readings {
		clock.now = start.Add(r.after)
		sent := s.mark()
		if r.late {
			sent = before
		}
		s.record(id, []quota.Window{{Key: "5h", Label: "Session", Utilization: r.utilization, ResetsAt: r.resets}}, sent)
	}
}

func TestThePaceIsTheRiseAcrossTheLastHalfHoursReadings(t *testing.T) {
	// The session started two hours before start, and resets three hours
	// after it; the next resets five hours later.
	resets := start.Add(3 * time.Hour)
	next := start.Add(20*time.Minute + 5*time.Hour)
	tests := []struct {
		name     string
		readings []sessionReading
		// at is how long after start the pace is judged.
		at     time.Duration
		want   score.Pace
		wantOK bool
	}{
		{
			name:     "readings spanning 20 minutes",
			readings: []sessionReading{{0, 0.3, resets, false}, {10 * time.Minute, 0.35, resets, false}, {20 * time.Minute, 0.4, resets, false}},
			at:       20 * time.Minute,
			want:     score.Pace{Rate: 0.3, Recent: true}, wantOK: true,
		},
		{
			name:     "readings spanning 5 minutes, the use since the session started",
			readings: []sessionReading{{0, 0.3, resets, false}, {5 * time.Minute, 0.325, resets, false}},
			at:       5 * time.Minute,
			want:     score.Pace{Rate: 0.325 / (2*time.Hour + 5*time.Minute).Hours()}, wantOK: true,
		},
		{
			name: "readings older than the half hour left behind",
			readings: []sessionReading{
				{0, 0.1, resets, false}, {20 * time.Minute, 0.2, resets, false}, {40 * time.Minute, 0.3, resets, false}, {45 * time.Minute, 0.35, resets, false},
			},
			at:   45 * time.Minute,
			want: score.Pace{Rate: 0.36, Recent: true}, wantOK: true,
		},
		{
			name: "a later reset, a new window, starts the readings afresh",
			readings: []sessionReading{
				{0, 0.3, resets, false}, {15 * time.Minute, 0.4, resets, false}, {20 * time.Minute, 0.02, next, false}, {30 * time.Minute, 0.05, next, false},
			},
			at:   30 * time.Minute,
			want: score.Pace{Rate: 0.18, Recent: true}, wantOK: true,
		},
		{
			name: "a lower reading with the same reset, taken as current, starts the readings afresh",
			readings: []sessionReading{
				{0, 0.3, resets, false}, {10 * time.Minute, 0.4, resets, false}, {15 * time.Minute, 0, resets, false}, {25 * time.Minute, 0.1, resets, false},
			},
			at:   25 * time.Minute,
			want: score.Pace{Rate: 0.6, Recent: true}, wantOK: true,
		},
		{
			name: "a lower reading arriving late changes nothing",
			readings: []sessionReading{
				{0, 0.3, resets, false}, {10 * time.Minute, 0.4, resets, false}, {12 * time.Minute, 0.35, resets, true},
			},
			at:   12 * time.Minute,
			want: score.Pace{Rate: 0.6, Recent: true}, wantOK: true,
		},
		{
			name:     "a session reset since it was read",
			readings: []sessionReading{{0, 0.3, resets, false}, {20 * time.Minute, 0.4, resets, false}},
			at:       3*time.Hour + time.Minute,
		},
		{
			name: "no reading of the session",
			at:   20 * time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			s := newTestState(clock)
			readSessions(s, clock, "work", tt.readings)

			got, ok := s.usage["work"].pace(testPolicy, start.Add(tt.at))
			if math.Abs(got.Rate-tt.want.Rate) > 1e-9 || got.Recent != tt.want.Recent || ok != tt.wantOK {
				t.Errorf("pace() = %+v, %v, want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestOnlyAFallOfATenthShowsAWindowResetByHand(t *testing.T) {
	// Work's session, read over 20 minutes, rises to 30%; a minute on, a
	// request sent since reads it lower, its reset the same.
	resets := start.Add(3 * time.Hour)
	tests := []struct {
		name        string
		utilization float64
		wantReset   bool
	}{
		{name: "a point lower, as a 429's reading, is noise", utilization: 0.29},
		{name: "nine points lower is noise", utilization: 0.21},
		{name: "ten points lower is a reset made by hand", utilization: 0.2, wantReset: true},
		{name: "emptied is a reset made by hand", utilization: 0.01, wantReset: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			s := newTestState(clock)
			readSessions(s, clock, "work", []sessionReading{
				{0, 0.2, resets, false}, {10 * time.Minute, 0.25, resets, false}, {20 * time.Minute, 0.3, resets, false},
				{21 * time.Minute, tt.utilization, resets, false},
			})

			u := s.usage["work"]
			if got := u.windows["5h"]; got.Utilization != tt.utilization {
				t.Errorf("the session reads %v, want %v: the reading since is the upstream's latest word", got.Utilization, tt.utilization)
			}
			var wantRestart time.Time
			wantReadings := 4
			if tt.wantReset {
				wantRestart, wantReadings = clock.now, 1
			}
			if got := u.windows["5h"].RestartedAt; !got.Equal(wantRestart) {
				t.Errorf("the session started again at %v, want %v", got, wantRestart)
			}
			if got := len(u.trails["5h"]); got != wantReadings {
				t.Errorf("the session's trail holds %d readings, want %d", got, wantReadings)
			}
		})
	}
}

func TestTheDocumentGivesEachAccountsPressure(t *testing.T) {
	// Work keeps a tenth of every window back. Its session, read over 20
	// minutes, rises at 30% an hour, and resets 2h 40m on: at 60%, it reaches
	// its reserve an hour on, and its limit 1h 20m on. Side's rises at 15% an
	// hour, and reaches its limit 5h 40m on, after its reset.
	resets := start.Add(3 * time.Hour)
	configured := slices.Clone(testConfigured)
	configured[0].Reserve = 0.1
	clock := &testClock{now: start}
	s := newState(resolve(configured, testTokens.Read), testPolicy, claude.Provider{}.Family, clock.read, unkept, unkept)
	readSessions(s, clock, "work", []sessionReading{{0, 0.5, resets, false}, {20 * time.Minute, 0.6, resets, false}})
	readSessions(s, clock, "side", []sessionReading{{0, 0.1, resets, false}, {20 * time.Minute, 0.15, resets, false}})
	now := clock.now
	side := status.Pressure{Window: "5h", Rate: 0.15, Recent: true, RunsOut: now.Add(5*time.Hour + 40*time.Minute)}
	tests := []struct {
		name   string
		pinned []string
		want   map[string]status.Pressure
	}{
		{
			name: "unpinned, work reaching its reserve",
			want: map[string]status.Pressure{
				"work": {Window: "5h", Rate: 0.3, Recent: true, RunsOut: now.Add(time.Hour), Under: true},
				"side": side,
			},
		},
		{
			name:   "work pinned, reaching its limit, its reserve spent",
			pinned: []string{"work"},
			want: map[string]status.Pressure{
				"work": {Window: "5h", Rate: 0.3, Recent: true, RunsOut: now.Add(80 * time.Minute), Under: true},
				"side": side,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := s.document(tt.pinned...)
			for _, a := range doc.Accounts {
				got, want := a.Pressure, tt.want[a.ID]
				if got.Window != want.Window || math.Abs(got.Rate-want.Rate) > 1e-9 || got.Recent != want.Recent ||
					got.RunsOut.Sub(want.RunsOut).Abs() > time.Microsecond || got.Under != want.Under {
					t.Errorf("%s's pressure = %+v, want %+v", a.ID, got, want)
				}
			}
		})
	}
}

func TestTheDocumentGivesEachWindowsRecentRate(t *testing.T) {
	// Over 20 minutes, work's session rises at 30% an hour and its week at
	// 3%; its Fable week, read once, has no recent rate. Side is read once.
	clock := &testClock{now: start}
	s := newTestState(clock)
	resets := start.Add(3 * time.Hour)
	weekAt := func(u float64) quota.Window {
		w := week
		w.Utilization = u
		return w
	}
	s.record("work", []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.5, ResetsAt: resets}, weekAt(0.4), fableWeek}, s.mark())
	s.record("side", []quota.Window{session, week}, s.mark())
	clock.now = start.Add(20 * time.Minute)
	s.record("work", []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.6, ResetsAt: resets}, weekAt(0.41)}, s.mark())

	doc := s.document()
	want := map[string][]status.Rate{"work": {{Window: "5h", Rate: 0.3}, {Window: "7d", Rate: 0.03}}}
	for _, a := range doc.Accounts {
		if !sameRates(a.Rates, want[a.ID]) {
			t.Errorf("%s's recent rates are %+v, want %+v", a.ID, a.Rates, want[a.ID])
		}
	}
}

// sameRates reports whether two lists of rates match, to within rounding.
func sameRates(a, b []status.Rate) bool {
	return slices.EqualFunc(a, b, func(x, y status.Rate) bool { return x.Window == y.Window && math.Abs(x.Rate-y.Rate) < 1e-9 })
}

func TestTheBestIsWhereANewSessionGoesUnderPressure(t *testing.T) {
	// Work's quota needs using first, but its session, read over 20 minutes,
	// runs out at its rate 1h 20m on, before it resets two hours on; side's
	// doesn't.
	clock := &testClock{now: start}
	r := newTestRouter(t, clock.read, &stubProber{})
	resets := start.Add(2*time.Hour + 20*time.Minute)
	readings := func(id string, from, to float64, week quota.Window) {
		for i, u := range []float64{from, to} {
			clock.now = start.Add(time.Duration(i) * 20 * time.Minute)
			r.state.record(id, []quota.Window{{Key: "5h", Label: "Session", Utilization: u, ResetsAt: resets}, week}, r.state.mark())
		}
	}
	readings("work", 0.5, 0.6, soonWeek)
	readings("side", 0.1, 0.12, laterWeek)
	steps := []struct {
		name string
		pin  []string
		want string
	}{
		{name: "unpinned, work under pressure", want: "side"},
		{name: "pinned to work and side, work under pressure", pin: []string{"work", "side"}, want: "side"},
		{name: "pinned to work alone, under pressure", pin: []string{"work"}, want: "work"},
	}
	for i, step := range steps {
		if step.pin != nil {
			r.sessions.setPin(status.Pin{Accounts: step.pin, Since: clock.now}, false)
		}
		if got := r.Status().Best; got != step.want {
			t.Errorf("%s, Best = %q, want %q", step.name, got, step.want)
		}
		session := fmt.Sprintf("new-%d", i)
		if got := choose(t.Context(), r, Request{ID: session, Session: session, Model: opus, Client: "work"}); got.Account != step.want {
			t.Errorf("%s, a new session goes to %s, want %s, the best", step.name, got.Account, step.want)
		}
	}
}

func TestAChoicePassingOverAnAccountUnderPressureIsLogged(t *testing.T) {
	// Work's quota needs using first, but its session, read over 20 minutes,
	// rises at 30% an hour, and runs out 1h 20m on, before it resets two
	// hours on; side's doesn't rise.
	log := logstest.Capture(t)
	clock := &testClock{now: start}
	r := newTestRouter(t, clock.read, &stubProber{})
	resets := start.Add(2*time.Hour + 20*time.Minute)
	for i, u := range []float64{0.5, 0.6} {
		clock.now = start.Add(time.Duration(i) * 20 * time.Minute)
		r.state.record("work", []quota.Window{{Key: "5h", Label: "Session", Utilization: u, ResetsAt: resets}, soonWeek}, r.state.mark())
		r.state.record("side", []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.1, ResetsAt: resets}, laterWeek}, r.state.mark())
	}
	req := Request{ID: "5f3a9c2e", Session: "0b5c6f2e-7d41", Model: opus, Client: "work"}

	if got := choose(t.Context(), r, req); got.Account != "side" || got.Reason != "new, work under pressure" {
		t.Errorf("a new session's choice = %+v, want side, passing over work under pressure", got)
	}
	want := []string{"level=INFO", `msg="passed over under pressure"`, "id=5f3a9c2e", "account=work", `rate="30% an hour"`,
		"runs_out=" + clock.now.Add(80*time.Minute).Local().Format("2006-01-02T15:04:05.000-07:00"), "chosen=side"}
	if !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
	choose(t.Context(), r, req)
	if lines := slices.DeleteFunc(log.Lines(), func(line string) bool { return !strings.Contains(line, "under pressure") }); len(lines) != 1 {
		t.Errorf("log reads\n%s\nwant one line of pressure alone, for the choice it changed, none for the session kept where it went", log)
	}
}
