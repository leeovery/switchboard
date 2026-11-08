package router

import (
	"math"
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

// sameDemand reports whether two demands match, to within rounding: neither
// is there, or both are, alike, without a hold curve.
func sameDemand(got, want *status.Demand) bool {
	if got == nil || want == nil {
		return got == want
	}
	return near(got.Day, want.Day) && near(got.Hours, want.Hours) && got.Hold == nil && want.Hold == nil
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
			name:     "side's week unchanged, with room throughout",
			sideWeek: []quota.Window{weekReading(0.3, next)},
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
