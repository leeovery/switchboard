package router

import (
	"fmt"
	"math"
	"net/http"
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
			name:     "readings from 20 minutes back",
			readings: []sessionReading{{0, 0.3, resets, false}, {10 * time.Minute, 0.35, resets, false}, {20 * time.Minute, 0.4, resets, false}},
			at:       20 * time.Minute,
			want:     score.Pace{Rate: 0.3, Recent: true}, wantOK: true,
		},
		{
			name:     "readings from 5 minutes back, the use since the session started",
			readings: []sessionReading{{0, 0.3, resets, false}, {5 * time.Minute, 0.325, resets, false}},
			at:       5 * time.Minute,
			want:     score.Pace{Rate: 0.325 / (2*time.Hour + 5*time.Minute).Hours()}, wantOK: true,
		},
		{
			name: "a baseline read once, 45 minutes back: its rise spread since, those older left behind",
			readings: []sessionReading{
				{0, 0.1, resets, false}, {20 * time.Minute, 0.2, resets, false}, {40 * time.Minute, 0.3, resets, false}, {45 * time.Minute, 0.35, resets, false},
			},
			at:   45 * time.Minute,
			want: score.Pace{Rate: 0.25 / 0.75, Recent: true}, wantOK: true,
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
			name: "a reading fallen by a tenth with the same reset, taken as current, starts the readings afresh",
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
			want: score.Pace{Rate: 0.5, Recent: true}, wantOK: true,
		},
		{
			name:     "use outside the router, read two hours on by a probe, spread over the gap",
			readings: []sessionReading{{0, 0.3, resets, false}, {2 * time.Hour, 0.5, resets, false}},
			at:       2 * time.Hour,
			want:     score.Pace{Rate: 0.1, Recent: true}, wantOK: true,
		},
		{
			name:     "climbing back from a dip too small to be a reset made by hand, no use",
			readings: []sessionReading{{0, 0.5, resets, false}, {5 * time.Minute, 0.41, resets, false}, {35 * time.Minute, 0.5, resets, false}},
			at:       40 * time.Minute,
			want:     score.Pace{Recent: true}, wantOK: true,
		},
		{
			name:     "read once, quiet since, quiet",
			readings: []sessionReading{{0, 0.3, resets, false}},
			at:       45 * time.Minute,
			want:     score.Pace{Recent: true}, wantOK: true,
		},
		{
			name:     "probed after 45 quiet minutes, quiet at once",
			readings: []sessionReading{{0, 0.3, resets, false}, {45 * time.Minute, 0.3, resets, false}},
			at:       45 * time.Minute,
			want:     score.Pace{Recent: true}, wantOK: true,
		},
		{
			name:     "a burst of 10 minutes, quiet for 20 since, slowing",
			readings: []sessionReading{{0, 0.3, resets, false}, {10 * time.Minute, 0.4, resets, false}},
			at:       30 * time.Minute,
			want:     score.Pace{Rate: 0.2, Recent: true}, wantOK: true,
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

func TestATrailKeepsALevelForEachChangeOfUseAlone(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	for i := range 100 {
		clock.now = start.Add(time.Duration(i) * 12 * time.Second)
		s.record("work", []quota.Window{session}, s.mark())
	}

	trail := s.usage["work"].trails["5h"]
	if len(trail) != 1 || !trail[0].At.Equal(start) || !trail[0].Last.Equal(clock.now) {
		t.Fatalf("after 100 readings of the same use, the trail holds %+v, want one level, first read at %v and last at %v", trail, start, clock.now)
	}
	busier := session
	busier.Utilization += 0.01
	s.record("work", []quota.Window{busier}, s.mark())
	if got := len(s.usage["work"].trails["5h"]); got != 2 {
		t.Errorf("after the use changed, the trail holds %d levels, want 2", got)
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
			wantReadings := 3
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

func TestAnAccountIsUnderPressureOnlyWhileItCanTakeARequestOfSomeModel(t *testing.T) {
	// Work keeps a tenth of every window back. Its session, read over 20
	// minutes, rises at 30% an hour, from 40%: it runs out before it resets.
	far := start.Add(3 * time.Hour)
	roomyWeek, fullWeek := week, week
	roomyWeek.Utilization, fullWeek.Utilization = 0.5, 0.95
	tests := []struct {
		name string
		// holdBack holds work back, or not.
		holdBack func(s *state)
		pinned   []string
		want     bool
	}{
		{name: "held back by nothing", holdBack: func(*state) {}, want: true},
		{name: "its token refused", holdBack: func(s *state) { s.refuse("work", http.StatusUnauthorized, someRequest) }},
		{name: "a limit holding back every request", holdBack: func(s *state) { s.limit("work", nil, far, s.mark()) }},
		{name: "a limit on its week, which every model shares", holdBack: func(s *state) { s.limit("work", []string{"7d"}, far, s.mark()) }},
		{name: "a limit on Fable's week alone", holdBack: func(s *state) { s.limit("work", []string{"7d_oi"}, far, s.mark()) }, want: true},
		{name: "a refusal of Opus requests alone", holdBack: func(s *state) { s.forbid("work", "opus", http.StatusForbidden, someRequest) }, want: true},
		{name: "its week at its reserve", holdBack: func(s *state) { s.record("work", []quota.Window{fullWeek}, s.mark()) }},
		{
			name:     "its week at its reserve, which the global pin spends",
			holdBack: func(s *state) { s.record("work", []quota.Window{fullWeek}, s.mark()) },
			pinned:   []string{"work"},
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configured := slices.Clone(testConfigured)
			configured[0].Reserve = 0.1
			clock := &testClock{now: start}
			s := newState(resolve(configured, testTokens.Read), testPolicy, claude.Provider{}.Family, clock.read, unkept, unkept)
			s.learn(fable, []quota.Window{fableWeek})
			readSessions(s, clock, "work", []sessionReading{{0, 0.4, start.Add(3 * time.Hour), false}, {20 * time.Minute, 0.5, start.Add(3 * time.Hour), false}})
			s.record("work", []quota.Window{roomyWeek, fableWeek}, s.mark())
			tt.holdBack(s)

			work, _ := s.document(tt.pinned...).Account("work")
			if work.Pressure.Under != tt.want {
				t.Errorf("work under pressure = %v, want %v: %+v", work.Pressure.Under, tt.want, work.Pressure)
			}
		})
	}
}

func TestTheDocumentGivesEachWindowsRecentRate(t *testing.T) {
	// Over 20 minutes, work's session rises at 30% an hour and its week at
	// 3%; its Fable week, read as they started, hasn't risen since. Side,
	// read then too, has been quiet since.
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
	want := map[string][]status.Rate{
		"work": {{Window: "5h", Rate: 0.3}, {Window: "7d", Rate: 0.03}, {Window: "7d_oi", Rate: 0}},
		"side": {{Window: "5h", Rate: 0}, {Window: "7d", Rate: 0}},
	}
	for _, a := range doc.Accounts {
		if !sameRates(a.Rates, want[a.ID]) {
			t.Errorf("%s's recent rates are %+v, want %+v", a.ID, a.Rates, want[a.ID])
		}
		for _, r := range a.Rates {
			if !r.Since.Equal(start) {
				t.Errorf("%s's %s rate is measured from %v, want %v, its first level, with none before the half hour", a.ID, r.Window, r.Since, start)
			}
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

func TestThePressureLoggedIsAtTheRoomTheChoiceGaveTheAccount(t *testing.T) {
	// Work keeps a tenth of every window back, and the global pin names it
	// and side, so a choice spends its reserve: its session runs out at its
	// limit. Work's quota needs using first; side's session doesn't rise.
	tests := []struct {
		name string
		// from and to are work's session over 20 minutes, which resets two
		// hours after the last.
		from, to float64
		rate     string
		runsOut  time.Duration
	}{
		{name: "short of its reserve", from: 0.6, to: 0.7, rate: "30% an hour", runsOut: time.Hour},
		{name: "past its reserve", from: 0.8, to: 0.92, rate: "36% an hour", runsOut: 13*time.Minute + 20*time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			configured := slices.Clone(testConfigured)
			configured[0].Reserve = 0.1
			clock := &testClock{now: start}
			r, err := New(Config{
				Accounts: configured, Token: testTokens.Read, Upstream: "http://127.0.0.1:1",
				Provider: claude.Provider{}, Prober: &stubProber{}, Policy: testPolicy, Now: clock.read,
			})
			if err != nil {
				t.Fatal(err)
			}
			resets := start.Add(2*time.Hour + 20*time.Minute)
			for i, u := range []float64{tt.from, tt.to} {
				clock.now = start.Add(time.Duration(i) * 20 * time.Minute)
				r.state.record("work", []quota.Window{{Key: "5h", Label: "Session", Utilization: u, ResetsAt: resets}, soonWeek}, r.state.mark())
				r.state.record("side", []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.1, ResetsAt: resets}, laterWeek}, r.state.mark())
			}
			r.sessions.setPin(status.Pin{Accounts: []string{"work", "side"}, Since: start}, false)

			if got := choose(t.Context(), r, Request{ID: "5f3a9c2e", Session: "one", Model: opus, Client: "work"}); got.Reason != "pinned (global), work under pressure" {
				t.Fatalf("the choice = %+v, want side, passing over work", got)
			}
			want := []string{`msg="passed over under pressure"`, "account=work", `rate="` + tt.rate + `"`,
				"runs_out=" + clock.now.Add(tt.runsOut).Local().Format("2006-01-02T15:04:05.000-07:00")}
			if !log.Has(want...) {
				t.Errorf("log reads\n%s\nwant a line with %q: work judged at its limit, as the pin spends its reserve", log, want)
			}
		})
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
