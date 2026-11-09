package router

import (
	"math"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
)

// weekReading is the week as read at utilization, resetting at resets.
func weekReading(utilization float64, resets time.Time) quota.Window {
	return quota.Window{Key: "7d", Label: "Week", Utilization: utilization, ResetsAt: resets}
}

// weekUsed is the week as read at utilization, resetting as week does.
func weekUsed(utilization float64) quota.Window {
	return weekReading(utilization, week.ResetsAt)
}

// workReads is work's week as a probe read it at at, at u of its use,
// resetting at resets, as the readings history holds it.
func workReads(at time.Time, u float64, resets time.Time) readings.Reading {
	return readings.Reading{At: at.UTC(), Account: "work", Key: "7d", Utilization: u, ResetsAt: resets, Source: readings.FromProbe}
}

// planned are testConfigured on plans: work on Max, personal on Pro and side
// on Ultra.
func planned() []config.Account {
	configured := slices.Clone(testConfigured)
	configured[0].Plan, configured[1].Plan, configured[2].Plan = config.PlanMax, config.PlanPro, config.PlanUltra
	return configured
}

// workReserved are testConfigured, work keeping a tenth of every window back.
func workReserved() []config.Account {
	configured := slices.Clone(testConfigured)
	configured[0].Reserve = 0.1
	return configured
}

// newStateOf builds the state of the accounts configured, with testTokens's
// tokens, on clock's time, as newTestState does.
func newStateOf(clock *testClock, configured []config.Account) *state {
	return newState(resolve(configured, testTokens.Read, unrecorded), testPolicy, claude.Provider{}.Family, clock.read, unkept, unkept)
}

// fastOf returns the fast rate the account's rates give its window with the
// given key: none where they give none.
func fastOf(a status.Account, key string) float64 {
	i := slices.IndexFunc(a.Rates, func(r status.Rate) bool { return r.Window == key })
	if i < 0 {
		return 0
	}
	return a.Rates[i].Fast
}

// near reports whether two numbers match, to within rounding.
func near(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// sameDemand reports whether two demands' rates match, to within rounding:
// neither is there, or both are, alike in their day's rate and its hours.
// Their hold curves are for the curve's own tests.
func sameDemand(got, want *status.Demand) bool {
	if got == nil || want == nil {
		return got == want
	}
	return near(got.Day, want.Day) && near(got.Hours, want.Hours)
}

func TestAWeeksRiseReachesItsFastRateAndThePoolsDayRate(t *testing.T) {
	// Work, on Max, reads its week at 40%, and at 42% ten minutes on; side,
	// on Ultra, reads its at 10%.
	clock := &testClock{now: start}
	s := newStateOf(clock, planned())
	s.record("work", []quota.Window{weekUsed(0.4)}, s.mark())
	s.record("side", []quota.Window{weekUsed(0.1)}, s.mark())
	clock.now = start.Add(10 * time.Minute)
	s.record("work", []quota.Window{weekUsed(0.42)}, s.mark())

	work, _ := s.document().Account("work")
	if got := fastOf(work, "7d"); !near(got, 0.02) {
		t.Errorf("work's week's fast rate is %v, want 0.02: two points' rise reads as two points an hour as it comes", got)
	}
	clock.now = start.Add(2 * time.Hour)
	// Over the two hours the pool has had room in, work's week rose by 2% of
	// a Max week, 5 Pro weeks.
	if got, want := s.document().Demand, (&status.Demand{Day: 0.02 * 5 / 2, Hours: 2}); !sameDemand(got, want) {
		t.Errorf("the pool's demand is %+v, want %+v", got, want)
	}
}

func TestAWeeksFastRateStartsAfreshWhereItsTrailDoes(t *testing.T) {
	// Work's week rises from 40% to 45% ten minutes on, then is read every ten
	// minutes after.
	next := week.ResetsAt.Add(7 * 24 * time.Hour)
	tests := []struct {
		name string
		then []quota.Window
		want float64
	}{
		{name: "risen again, its rises together", then: []quota.Window{weekUsed(0.47)}, want: 0.05*math.Exp(-1.0/6) + 0.02},
		{name: "a dip and back, no rise", then: []quota.Window{weekUsed(0.44), weekUsed(0.45)}, want: 0.05 * math.Exp(-2.0/6)},
		{name: "a reset made by hand, afresh", then: []quota.Window{weekUsed(0.3)}},
		{name: "a new week, afresh", then: []quota.Window{weekReading(0.01, next)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			s := newTestState(clock)
			s.record("work", []quota.Window{weekUsed(0.4)}, s.mark())
			for i, w := range append([]quota.Window{weekUsed(0.45)}, tt.then...) {
				clock.now = start.Add(time.Duration(i+1) * 10 * time.Minute)
				s.record("work", []quota.Window{w}, s.mark())
			}

			work, _ := s.document().Account("work")
			if got := fastOf(work, "7d"); !near(got, tt.want) {
				t.Errorf("work's week's fast rate is %v, want %v", got, tt.want)
			}
		})
	}
}

func TestThePoolHasNoRoomWhileEveryAccountsWeekIsAtItsReserveOrLimit(t *testing.T) {
	// Work keeps a tenth back. Both weeks read 50%; an hour on, work's reaches
	// its reserve and side's its limit, side's resetting six hours in.
	// Personal has no token, and counts for nothing.
	clock := &testClock{now: start}
	s := newStateOf(clock, workReserved())
	resets := start.Add(6 * time.Hour)
	s.record("work", []quota.Window{weekUsed(0.5)}, s.mark())
	s.record("side", []quota.Window{weekReading(0.5, resets)}, s.mark())
	clock.now = start.Add(time.Hour)
	s.record("work", []quota.Window{weekUsed(0.9)}, s.mark())
	s.record("side", []quota.Window{weekReading(1, resets)}, s.mark())

	steps := []struct {
		after time.Duration
		hours float64
		why   string
	}{
		{after: 5 * time.Hour, why: "an hour with room alone, too little for a day rate: none since"},
		{after: 6*time.Hour + 10*time.Minute, why: "side's week reset since it was read: room from now on"},
		{after: 7*time.Hour + 10*time.Minute, hours: 2, why: "the hour before the pool had none, and the hour since side's week reset"},
	}
	for _, step := range steps {
		clock.now = start.Add(step.after)
		if got := s.document().Demand; got == nil || !near(got.Hours, step.hours) {
			t.Errorf("%v in, the pool's demand is %+v, want it measured over %v hours: %s", step.after, got, step.hours, step.why)
		}
	}
	// Both weeks' rises came as the pool had room.
	if got, want := s.document().Demand, (&status.Demand{Day: 0.9 / 2, Hours: 2}); !sameDemand(got, want) {
		t.Errorf("the pool's demand is %+v, want %+v", got, want)
	}
}

func TestTheRatesOfTheWeeksOutlastARestart(t *testing.T) {
	// Work's week rose from 40% to 42% ten minutes on, and to 44% two hours
	// on, as the router restarts.
	now := start.Add(2 * time.Hour)
	h := newHistory(config.History{}, at(now))
	h.open(t.TempDir())
	writeLines(t, h, workReads(start, 0.4, week.ResetsAt), workReads(start.Add(10*time.Minute), 0.42, week.ResetsAt), workReads(now, 0.44, week.ResetsAt))
	s := newTestState(&testClock{now: now})
	s.usage["work"].windows["7d"] = weekUsed(0.44)

	s.seed(h.readBack(now))
	doc := s.document()
	work, _ := doc.Account("work")
	if got, want := fastOf(work, "7d"), 0.02*math.Exp(-110.0/60)+0.02; !near(got, want) {
		t.Errorf("after the restart, work's week's fast rate is %v, want %v, as before it", got, want)
	}
	if got, want := doc.Demand, (&status.Demand{Day: 0.04 / 2, Hours: 2}); !sameDemand(got, want) {
		t.Errorf("after the restart, the pool's demand is %+v, want %+v, as before it", got, want)
	}
}

func TestARestartJudgesThePoolsRoomByTheHistoryAsItGoes(t *testing.T) {
	// Work keeps a tenth back. Its week rose from 50% to its reserve an hour
	// on, and reset an hour after; four hours in, the router restarts.
	now := start.Add(4 * time.Hour)
	resets := start.Add(2 * time.Hour)
	next := resets.Add(7 * 24 * time.Hour)
	tests := []struct {
		name string
		// sideWeek is side's week as the state file kept it, of which the
		// history holds no change: none where it's unread.
		sideWeek []quota.Window
		want     status.Demand
	}{
		{
			name: "side unread, the pool without room while work's week was at its reserve",
			want: status.Demand{Day: 0.45 / 3, Hours: 3},
		},
		{
			name:     "side's week unchanged since before the history began, with room throughout",
			sideWeek: []quota.Window{weekUsed(0.3)},
			want:     status.Demand{Day: 0.45 / 4, Hours: 4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHistory(config.History{}, at(now))
			h.open(t.TempDir())
			writeLines(t, h, workReads(start, 0.5, resets), workReads(start.Add(time.Hour), 0.95, resets), workReads(resets, 0, next))
			s := newStateOf(&testClock{now: now}, workReserved())
			s.usage["work"].windows["7d"] = weekReading(0, next)
			for _, w := range tt.sideWeek {
				s.usage["side"].windows[w.Key] = w
			}

			s.seed(h.readBack(now))
			if got := s.document().Demand; !sameDemand(got, &tt.want) {
				t.Errorf("after the restart, the pool's demand is %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBeforeTheHistoryReadsAWeekItStandsAsFirstReadFromWhenItBegan(t *testing.T) {
	// Work keeps a tenth back. The history's readings of its session have the
	// pool judged an hour in, and later; side is never read, and personal has
	// no token.
	began := start.Add(6 * time.Hour)
	resets := began.Add(7 * 24 * time.Hour)
	sessionAt := func(after time.Duration) readings.Reading {
		at := start.Add(after)
		return readings.Reading{At: at.UTC(), Account: "work", Key: "5h", Utilization: 0.1, ResetsAt: at.Add(5 * time.Hour), Source: readings.FromProbe}
	}
	tests := []struct {
		name string
		// held is work's week as the state file kept it.
		held    quota.Window
		history []readings.Reading
		after   time.Duration
		want    status.Demand
	}{
		{
			name: "read at 89% ten hours in, then at its reserve, with room before it was read",
			held: weekUsed(0.9),
			history: []readings.Reading{
				sessionAt(time.Hour), workReads(start.Add(10*time.Hour), 0.89, week.ResetsAt), workReads(start.Add(16*time.Hour), 0.9, week.ResetsAt),
			},
			after: 20 * time.Hour,
			want:  status.Demand{Day: 0.01 / 15, Hours: 15},
		},
		{
			name:    "a week that began six hours in, first read later, with no room before it began",
			held:    weekReading(0.1, resets),
			history: []readings.Reading{sessionAt(time.Hour), sessionAt(4 * time.Hour), sessionAt(8 * time.Hour), workReads(start.Add(12*time.Hour), 0.1, resets)},
			after:   16 * time.Hour,
			want:    status.Demand{Hours: 8},
		},
		{
			name:    "a week the history doesn't read, as the state file kept it, from when it began",
			held:    weekReading(0.1, resets),
			history: []readings.Reading{sessionAt(time.Hour), sessionAt(4 * time.Hour), sessionAt(8 * time.Hour)},
			after:   16 * time.Hour,
			want:    status.Demand{Hours: 8},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := start.Add(tt.after)
			h := newHistory(config.History{}, at(now))
			h.open(t.TempDir())
			writeLines(t, h, tt.history...)
			s := newStateOf(&testClock{now: now}, workReserved())
			s.usage["work"].windows["7d"] = tt.held

			s.seed(h.readBack(now))
			if got := s.document().Demand; !sameDemand(got, &tt.want) {
				t.Errorf("after the restart, the pool's demand is %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestARestartForgetsTheFastRateOfAWeekResetSince(t *testing.T) {
	// The history read work's week rise two points ten minutes in; the state
	// file kept a week read since, which has reset.
	now := start.Add(20 * time.Minute)
	h := newHistory(config.History{}, at(now))
	h.open(t.TempDir())
	writeLines(t, h, workReads(start, 0.4, week.ResetsAt), workReads(start.Add(10*time.Minute), 0.42, week.ResetsAt))
	s := newTestState(&testClock{now: now})
	s.usage["work"].windows["7d"] = weekReading(0.01, week.ResetsAt.Add(7*24*time.Hour))

	s.seed(h.readBack(now))
	work, _ := s.document().Account("work")
	if got := fastOf(work, "7d"); got != 0 {
		t.Errorf("after the restart, work's week's fast rate is %v, want none: it was the week before's", got)
	}
}

func TestTheFirstRiseAfterARestartRisesFromTheReadingTheStateFileKept(t *testing.T) {
	// The state file kept work's week at 40%, and the history holds nothing
	// of it; after the restart, side is read, then work, a minute on, at 43%.
	clock := &testClock{now: start}
	s := newTestState(clock)
	s.usage["work"].windows["7d"] = weekUsed(0.4)
	s.record("side", []quota.Window{weekUsed(0.1)}, s.mark())
	clock.now = start.Add(time.Minute)
	s.record("work", []quota.Window{weekUsed(0.43)}, s.mark())

	work, _ := s.document().Account("work")
	if got := fastOf(work, "7d"); !near(got, 0.03) {
		t.Errorf("work's week's fast rate is %v, want 0.03, its rise from the 40%% the state file kept", got)
	}
	clock.now = start.Add(2 * time.Hour)
	if got, want := s.document().Demand, (&status.Demand{Day: 0.03 / 2, Hours: 2}); !sameDemand(got, want) {
		t.Errorf("the pool's demand is %+v, want %+v", got, want)
	}
}

func TestThePoolsDayRateTakesInTheWeeksRisesAlone(t *testing.T) {
	// Over two hours, work's session and its Fable week rise, and its week
	// holds.
	clock := &testClock{now: start}
	s := newTestState(clock)
	risen := func(w quota.Window, by float64) quota.Window {
		w.Utilization += by
		return w
	}
	s.record("work", []quota.Window{session, weekUsed(0.4), fableWeek}, s.mark())
	clock.now = start.Add(time.Hour)
	s.record("work", []quota.Window{risen(session, 0.3), weekUsed(0.4), risen(fableWeek, 0.1)}, s.mark())
	clock.now = start.Add(2 * time.Hour)

	if got, want := s.document().Demand, (&status.Demand{Hours: 2}); !sameDemand(got, want) {
		t.Errorf("the pool's demand is %+v, want %+v: the pool's use is its weeks'", got, want)
	}
}

func TestAFastRateFadedBelowAMillionthAnHourIsNone(t *testing.T) {
	// Work's week rose two points ten minutes in, and nothing since.
	clock := &testClock{now: start}
	s := newTestState(clock)
	s.record("work", []quota.Window{weekUsed(0.4)}, s.mark())
	clock.now = start.Add(10 * time.Minute)
	s.record("work", []quota.Window{weekUsed(0.42)}, s.mark())

	steps := []struct {
		after    time.Duration
		wantNone bool
	}{
		{after: 9 * time.Hour},
		{after: 10 * time.Hour, wantNone: true},
	}
	for _, step := range steps {
		clock.now = start.Add(10*time.Minute + step.after)
		work, _ := s.document().Account("work")
		if got := fastOf(work, "7d"); (got == 0) != step.wantNone {
			t.Errorf("%v after its rise, work's week's fast rate is %v, want none %v: 2%% faded below a millionth", step.after, got, step.wantNone)
		}
	}
}

// roomAt reports whether the pool has room for a new session at at, as the
// state judges it by the accounts' weeks as last read and the global pin as
// it stands.
func roomAt(s *state, at time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.roomy(s.weekHeld, s.pin.Accounts, at)
}

func TestThePoolHasRoomWhereTheRouterWouldSendANewSession(t *testing.T) {
	// Work keeps a tenth back. Side is read only where a case reads it.
	far := start.Add(3 * time.Hour)
	readWork := func(utilization float64) func(s *state) {
		return func(s *state) { s.record("work", []quota.Window{weekUsed(utilization)}, s.mark()) }
	}
	then := func(steps ...func(s *state)) func(s *state) {
		return func(s *state) {
			for _, step := range steps {
				step(s)
			}
		}
	}
	tests := []struct {
		name    string
		prepare func(s *state)
		want    bool
	}{
		{name: "work's week with room", prepare: readWork(0.5), want: true},
		{name: "nothing read", prepare: func(*state) {}},
		{name: "work's token refused", prepare: then(readWork(0.5), func(s *state) { s.refuse("work", http.StatusUnauthorized, someRequest, s.mark()) })},
		{name: "a limit holding back work's week", prepare: then(readWork(0.5), func(s *state) { s.limit("work", []string{"7d"}, far, s.mark()) })},
		{name: "a limit holding back every request on work", prepare: then(readWork(0.5), func(s *state) { s.limit("work", nil, far, s.mark()) })},
		{name: "a limit on work's session alone", prepare: then(readWork(0.5), func(s *state) { s.limit("work", []string{"5h"}, far, s.mark()) }), want: true},
		{name: "work's week at its reserve", prepare: readWork(0.9)},
		{name: "work's week at its reserve, the global pin naming it", prepare: then(readWork(0.9), func(s *state) { s.repin(status.Pin{Accounts: []string{"work"}}) }), want: true},
		{name: "work's week at its limit, the global pin naming it", prepare: then(readWork(1), func(s *state) { s.repin(status.Pin{Accounts: []string{"work"}}) })},
		{name: "work's week at its reserve, the global pin naming side, unread", prepare: then(readWork(0.9), func(s *state) { s.repin(status.Pin{Accounts: []string{"side"}}) })},
		{
			name:    "personal's week read with room, but its token missing",
			prepare: func(s *state) { s.usage["personal"].windows["7d"] = weekUsed(0.1) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStateOf(&testClock{now: start}, workReserved())
			tt.prepare(s)
			if got := roomAt(s, start); got != tt.want {
				t.Errorf("the pool has room = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestThePoolHasRoomOnAnAccountAtItsReserveWhileTheGlobalPinNamesIt(t *testing.T) {
	// Work keeps a tenth back, and its week is at its reserve; nothing else
	// has been read, nor is read again. The global pin names work an hour in,
	// and is cleared three hours in.
	clock := &testClock{now: start}
	cfg := testConfig(clock.read, &stubProber{}, testTokens.Read)
	cfg.Accounts = workReserved()
	r := newTestRouterOf(t, cfg)
	r.state.record("work", []quota.Window{weekUsed(0.92)}, r.state.mark())

	clock.now = start.Add(time.Hour)
	if err := r.pin(PinRequest{Accounts: []string{"work"}, By: ByCLI}); err != nil {
		t.Fatalf("pin() = %v", err)
	}
	if !roomAt(r.state, clock.now) {
		t.Error("pinned, the pool has no room, want room on work to its limit, as the pin spends its reserve")
	}
	clock.now = start.Add(3 * time.Hour)
	if got := r.state.document().Demand; got == nil || got.Hours != 2 {
		t.Errorf("two hours after the pin, the pool's demand is %+v, want it measured over 2 hours: room from the pin's own moment", got)
	}
	r.unpin(false, ByCLI)
	if roomAt(r.state, clock.now) {
		t.Error("unpinned, the pool has room, want none, work held back by its reserve")
	}
	clock.now = start.Add(4 * time.Hour)
	if got := r.state.document().Demand; got == nil || got.Hours != 2 {
		t.Errorf("an hour after the pin was cleared, the pool's demand is %+v, want it measured over 2 hours: none from the clearing's own moment", got)
	}
}

func TestARestartedRouterJudgesThePoolsRoomByTheGlobalPinItKept(t *testing.T) {
	// Work keeps a tenth back. The global pin names it from the start; its
	// week reads at its reserve an hour on, and the router restarts three
	// hours on.
	path := filepath.Join(t.TempDir(), "state.json")
	before := newTestFile(at(start), resolve(workReserved(), testTokens.Read, unrecorded))
	before.load(path)
	before.sessions.setPin(status.Pin{Accounts: []string{"work"}, Since: start}, false)
	before.state.record("work", []quota.Window{weekUsed(0.92)}, before.state.mark())
	before.save()
	now := start.Add(3 * time.Hour)
	h := newHistory(config.History{}, at(now))
	h.open(t.TempDir())
	writeLines(t, h, workReads(start.Add(time.Hour), 0.92, week.ResetsAt))

	after := newTestFile(at(now), resolve(workReserved(), testTokens.Read, unrecorded))
	after.load(path)
	after.state.seed(h.readBack(now))
	if got := after.state.document().Demand; got == nil || got.Hours != 2 {
		t.Errorf("after the restart, the pool's demand is %+v, want it measured over 2 hours: room on work from its reading on, as the pin kept spends its reserve", got)
	}
	if !roomAt(after.state, now) {
		t.Error("after the restart, the pool has no room, want room on work, which the pin it kept names")
	}
}

func TestARestartJudgesThePoolsRoomByTheGlobalPinFromWhenItWasSet(t *testing.T) {
	// Work keeps a tenth back, and nothing else is read. Its week reads 50%,
	// and its reserve an hour on; the global pin names it three hours on, and
	// a case reads its week again. Six hours on, the router restarts.
	tests := []struct {
		name string
		// readAgain is how long on work's week reads 94%, no sooner than the
		// pin: never where it's zero.
		readAgain time.Duration
	}{
		{name: "the pin set between two readings", readAgain: 5 * time.Hour},
		{name: "a reading at the pin's own moment", readAgain: 3 * time.Hour},
		{name: "the pin set after the last reading"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			clock := &testClock{now: start}
			cfg := testConfig(clock.read, &stubProber{}, testTokens.Read)
			cfg.Accounts = workReserved()
			live := newTestRouterOf(t, cfg)
			live.file.load(path)
			var heard []readings.Reading
			live.state.history = func(read []readings.Reading) { heard = append(heard, read...) }
			live.state.record("work", []quota.Window{weekUsed(0.5)}, live.state.mark())
			clock.now = start.Add(time.Hour)
			live.state.record("work", []quota.Window{weekUsed(0.92)}, live.state.mark())
			clock.now = start.Add(3 * time.Hour)
			if err := live.pin(PinRequest{Accounts: []string{"work"}, By: ByCLI}); err != nil {
				t.Fatalf("pin() = %v", err)
			}
			if tt.readAgain > 0 {
				clock.now = start.Add(tt.readAgain)
				live.state.record("work", []quota.Window{weekUsed(0.94)}, live.state.mark())
			}
			clock.now = start.Add(6 * time.Hour)
			live.file.save()
			want := live.state.document().Demand
			if want == nil || want.Hours != 4 {
				t.Fatalf("before the restart, the pool's demand is %+v, want it measured over 4 hours: the hour before work reached its reserve, and the three since the pin", want)
			}

			h := newHistory(config.History{}, at(clock.now))
			h.open(t.TempDir())
			writeLines(t, h, heard...)
			restarted := newTestFile(at(clock.now), resolve(workReserved(), testTokens.Read, unrecorded))
			restarted.load(path)
			restarted.state.seed(h.readBack(clock.now))
			if got := restarted.state.document().Demand; !sameDemand(got, want) {
				t.Errorf("after the restart, the pool's demand is %+v, want %+v, as before it: the pin held from when it was set alone", got, want)
			}
		})
	}
}

func TestTheDocumentGivesEachAccountsPlanAsConfigured(t *testing.T) {
	configured := planned()
	configured[1].Plan = ""
	s := newStateOf(&testClock{now: start}, configured)

	for i, a := range s.document().Accounts {
		if a.Plan != configured[i].Plan {
			t.Errorf("%s's plan is %q, want %q, as configured", a.ID, a.Plan, configured[i].Plan)
		}
	}
}

func TestTheDocumentGivesAWindowsFastRateWithoutARecentRate(t *testing.T) {
	// Work's week rose two points in five minutes: too few for a recent rate.
	clock := &testClock{now: start}
	s := newTestState(clock)
	s.record("work", []quota.Window{weekUsed(0.4)}, s.mark())
	clock.now = start.Add(5 * time.Minute)
	s.record("work", []quota.Window{weekUsed(0.42)}, s.mark())

	work, _ := s.document().Account("work")
	if rates := work.Rates; len(rates) != 1 || rates[0].Window != "7d" || rates[0].Rate != 0 || !rates[0].Since.IsZero() || !near(rates[0].Fast, 0.02) {
		t.Errorf("work's rates are %+v, want its week's fast rate of 0.02 alone, and no recent rate", rates)
	}
}

func TestTheDocumentsDemandIsThePoolsFastRateWithTooFewHoursOfRoom(t *testing.T) {
	// Five minutes in, work, on Max, has read its week rise two points, and
	// side, on Ultra, one.
	clock := &testClock{now: start}
	s := newStateOf(clock, planned())
	s.record("work", []quota.Window{weekUsed(0.4)}, s.mark())
	s.record("side", []quota.Window{weekUsed(0.1)}, s.mark())
	clock.now = start.Add(5 * time.Minute)
	s.record("work", []quota.Window{weekUsed(0.42)}, s.mark())
	s.record("side", []quota.Window{weekUsed(0.11)}, s.mark())

	// Work's 2% of 5 Pro weeks an hour, and side's 1% of 20.
	if got, want := s.document().Demand, (&status.Demand{Day: 0.02*5 + 0.01*20}); !sameDemand(got, want) {
		t.Errorf("the pool's demand is %+v, want %+v, its fast rate, measured over no hours", got, want)
	}
}

func TestAWeekResetSinceItWasReadHasNoFastRate(t *testing.T) {
	// Work's week, resetting half an hour in, rose two points in its last
	// five minutes read; ten minutes after its reset, nothing has read it
	// since.
	clock := &testClock{now: start}
	s := newTestState(clock)
	resets := start.Add(30 * time.Minute)
	s.record("work", []quota.Window{weekReading(0.4, resets)}, s.mark())
	clock.now = start.Add(5 * time.Minute)
	s.record("work", []quota.Window{weekReading(0.42, resets)}, s.mark())
	clock.now = resets.Add(10 * time.Minute)

	doc := s.document()
	work, _ := doc.Account("work")
	if got := fastOf(work, "7d"); got != 0 {
		t.Errorf("work's week's fast rate is %v, want none: it was the week before's", got)
	}
	if got, want := doc.Demand, (&status.Demand{}); !sameDemand(got, want) {
		t.Errorf("the pool's demand is %+v, want %+v: the pool's fast rate, without the week before's", got, want)
	}
}

func TestTheDocumentHasNoDemandBeforeAnAccountIsRead(t *testing.T) {
	if got := newTestState(&testClock{now: start}).document().Demand; got != nil {
		t.Errorf("the pool's demand is %+v, want none, as no account has been read", got)
	}
}
