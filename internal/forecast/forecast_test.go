package forecast

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// now is the time by the clock in every test: a Sunday, 09:00 UTC.
var now = time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)

var policy = score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d", Tiebreak: "5h", Started: "5h", Pressure: "5h"}

// ultra is an Ultra week's size, in Pro weeks.
const ultra = 20

// week is a shared week used so far, resetting so long after now.
func week(used float64, resetsIn time.Duration) quota.Window {
	return quota.Window{Key: "7d", Utilization: used, ResetsAt: now.Add(resetsIn)}
}

// account is a Pro account that takes new sessions, without a reserve, its
// room ending at its limit, as each option given changes it.
func account(id string, w quota.Window, options ...func(*Account)) Account {
	a := Account{ID: id, Week: w, Floor: 1, Size: 1, Takes: true}
	for _, option := range options {
		option(&a)
	}
	return a
}

func fast(rate float64) func(*Account) {
	return func(a *Account) { a.Fast = rate }
}

func sized(size float64) func(*Account) {
	return func(a *Account) { a.Size = size }
}

// reserved gives the account a reserve that holds it back, its room ending
// there.
func reserved(reserve float64) func(*Account) {
	return func(a *Account) { a.Reserve, a.Floor = reserve, 1-reserve }
}

// spendable gives the account a reserve its room runs past, as a pin spends
// it.
func spendable(reserve float64) func(*Account) {
	return func(a *Account) { a.Reserve = reserve }
}

func refusingNewSessions(a *Account) { a.Takes = false }

// holding returns a Held that keeps share of an account's fast rate on it,
// however long.
func holding(share float64) func(time.Duration) float64 {
	return func(time.Duration) float64 { return share }
}

// hours is a count of hours as a duration.
func hours(h float64) time.Duration {
	return time.Duration(h * float64(time.Hour))
}

// hoursUntil returns the hours from now, within a week, at which used, rising
// with the hours, reaches room.
func hoursUntil(used func(h float64) float64, room float64) float64 {
	lo, hi := 0.0, playSpan.Hours()
	for range 100 {
		if mid := (lo + hi) / 2; used(mid) < room {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo
}

// fading is the pool's use over the first h hours, in Pro weeks, at the day's
// rate plus how far its fast rate stands from it, fading with a time constant
// of 3 hours.
func fading(fast, day, h float64) float64 {
	return day*h + (fast-day)*3*(1-math.Exp(-h/3))
}

// outOf returns the account's Out among those given, failing where it has
// none.
func outOf(t *testing.T, outs map[string]Out, id string) Out {
	t.Helper()
	out, ok := outs[id]
	if !ok {
		t.Fatalf("Forecast() = %v, want an Out for %q", outs, id)
	}
	return out
}

// checkRunsOut fails unless out runs out within tolerance of want, or, with
// want zero, doesn't run out.
func checkRunsOut(t *testing.T, id string, out Out, want time.Time, tolerance time.Duration) {
	t.Helper()
	if out.RunsOut.IsZero() != want.IsZero() || out.RunsOut.Sub(want).Abs() > tolerance {
		t.Errorf("Forecast()[%q].RunsOut = %v, want %v, within %v", id, out.RunsOut, want, tolerance)
	}
}

// checkAtReset fails unless out's use at its reset is within tolerance of
// want.
func checkAtReset(t *testing.T, id string, out Out, want, tolerance float64) {
	t.Helper()
	if math.Abs(out.AtReset-want) > tolerance {
		t.Errorf("Forecast()[%q].AtReset = %v, want %v, within %v", id, out.AtReset, want, tolerance)
	}
}

func TestASteadyWeekRunsOutWhereItsRoomIsSpentAtItsRate(t *testing.T) {
	tests := []struct {
		name     string
		size     float64
		used     float64
		reserve  float64
		rate     float64
		resetsIn time.Duration
		// wantRunsOut is how long until it runs out: zero where it doesn't.
		wantRunsOut time.Duration
		wantAtReset float64
	}{
		{
			name: "runs out at its limit", size: 1, used: 0.4, rate: 0.01, resetsIn: 6 * quota.Day,
			wantRunsOut: 60 * time.Hour, wantAtReset: 1,
		},
		{
			name: "runs out where its reserve starts", size: 1, used: 0.4, reserve: 0.1, rate: 0.01, resetsIn: 6 * quota.Day,
			wantRunsOut: 50 * time.Hour, wantAtReset: 0.9,
		},
		{
			name: "a Max week runs out at its share of the pool's use", size: 5, used: 0.4, rate: 0.01, resetsIn: 6 * quota.Day,
			wantRunsOut: 60 * time.Hour, wantAtReset: 1,
		},
		{
			name: "reaches its reset short of its limit", size: 1, used: 0.2, rate: 0.005, resetsIn: 72 * time.Hour,
			wantAtReset: 0.56,
		},
		{
			name: "runs out within a step, just before its reset", size: 1, used: 0.4025, rate: 0.06, resetsIn: 10 * time.Hour,
			wantRunsOut: 9*time.Hour + 57*time.Minute + 30*time.Second, wantAtReset: 1,
		},
		{
			name: "reaches its limit within a step just after its reset, which isn't running out", size: 1, used: 0.4025, rate: 0.06,
			resetsIn: 9*time.Hour + 55*time.Minute, wantAtReset: 0.4025 + 0.06*(9+55.0/60),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := account("work", week(tt.used, tt.resetsIn), sized(tt.size), fast(tt.rate), reserved(tt.reserve))
			p := Pool{Now: now, Policy: policy, Accounts: []Account{a}, Day: tt.size * tt.rate}

			out := outOf(t, Forecast(p), "work")

			var want time.Time
			if tt.wantRunsOut > 0 {
				want = now.Add(tt.wantRunsOut)
			}
			checkRunsOut(t, "work", out, want, time.Second)
			checkAtReset(t, "work", out, tt.wantAtReset, 1e-9)
		})
	}
}

func TestABurstOrALullRunsOutWhereItsFadingUseReachesItsRoom(t *testing.T) {
	tests := []struct {
		name      string
		fast, day float64
	}{
		{"a burst, fast well above the day's rate", 0.1, 0.01},
		{"a lull, fast well below the day's rate", 0.001, 0.01},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := account("work", week(0.2, 6*quota.Day), fast(tt.fast))
			p := Pool{Now: now, Policy: policy, Accounts: []Account{a}, Day: tt.day}

			out := outOf(t, Forecast(p), "work")

			h := hoursUntil(func(h float64) float64 { return fading(tt.fast, tt.day, h) }, 0.8)
			checkRunsOut(t, "work", out, now.Add(hours(h)), playStep)
		})
	}
}

// ownersPool is four equal Ultra accounts as the owner's stood: account 2
// carrying the pool's use at 6% of its week an hour, account 1 at its
// reserve, and 3 and 4 at their limits, 3 resetting in 17 hours.
func ownersPool() Pool {
	return Pool{
		Now:    now,
		Policy: policy,
		Accounts: []Account{
			account("1", week(0.98, 28*time.Hour), sized(ultra), reserved(0.02)),
			account("2", week(0.19, 6*quota.Day+9*time.Hour), sized(ultra), fast(0.06)),
			account("3", week(1, 17*time.Hour), sized(ultra)),
			account("4", week(1, 87*time.Hour), sized(ultra)),
		},
		Day: 0.013 * ultra,
	}
}

func TestAWeekCarryingThePoolLastsAsTheOtherWeeksComeBack(t *testing.T) {
	out := outOf(t, Forecast(ownersPool()), "2")

	// Its fast rate alone runs it out 13.5 hours on; each of the others,
	// coming back, takes the new sessions while it scores higher.
	checkRunsOut(t, "2", out, time.Time{}, 0)
	checkAtReset(t, "2", out, 0.9908, 0.005)
}

func TestAWeekCarryingThePoolRunsOutWithoutTheResetsThatSaveIt(t *testing.T) {
	alone := hoursUntil(func(h float64) float64 { return fading(0.06*ultra, 0.013*ultra, h) }, 0.81*ultra)
	tests := []struct {
		name string
		// away are the accounts whose weeks don't come back within the
		// forecast, resetting a week on.
		away []string
		// want is when account 2 runs out, within tolerance.
		want, tolerance time.Duration
	}{
		{
			name: "no other week comes back, so it takes the pool's use alone",
			away: []string{"1", "3", "4"}, want: hours(alone), tolerance: playStep,
		},
		{
			name: "account 1's alone comes back, at 28 hours",
			away: []string{"3", "4"}, want: 5*quota.Day + 7*time.Hour, tolerance: 6 * time.Hour,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ownersPool()
			for i, a := range p.Accounts {
				if slices.Contains(tt.away, a.ID) {
					p.Accounts[i].Week.ResetsAt = now.Add(playSpan)
				}
			}

			out := outOf(t, Forecast(p), "2")

			checkRunsOut(t, "2", out, now.Add(tt.want), tt.tolerance)
		})
	}
}

func TestAWeekBackFromItsResetTakesTheNewSessionsWhileItScoresHigher(t *testing.T) {
	run := newPlay(ownersPool())
	run.playTo(now.Add(17 * time.Hour))
	carrying := run.weeks[1].Utilization
	run.playTo(now.Add(28 * time.Hour))

	// Until account 1 resets at 28 hours, account 3 scores higher than 2, so
	// 2 gains only what it holds, its fast rate fading as Held does, and 3
	// takes the rest of the pool's use.
	held := 0.06 * 6 * (math.Exp(-17.0/6) - math.Exp(-28.0/6))
	use := 0.013*ultra*11 + (0.06-0.013)*ultra*3*(math.Exp(-17.0/3)-math.Exp(-28.0/3))
	if got := run.weeks[1].Utilization - carrying; math.Abs(got-held) > 1e-5 {
		t.Errorf("account 2 gained %v of its week from 17 to 28 hours, want %v, what it holds alone", got, held)
	}
	if got, want := run.weeks[2].Utilization, use/ultra-held; math.Abs(got-want) > 1e-5 {
		t.Errorf("account 3 used %v of its week from its reset to 28 hours, want %v, the rest of the pool's use", got, want)
	}
}

func TestHeldUseStaysOnItsAccountAfterAnotherResets(t *testing.T) {
	falling := func(after time.Duration) float64 { return max(1-after.Hours()/4, 0) }
	p := Pool{
		Now:    now,
		Policy: policy,
		Accounts: []Account{
			account("work", week(0.9, 6*quota.Day), fast(0.06)),
			account("personal", week(1, time.Hour)),
		},
		Day:  0.06,
		Held: falling,
	}

	out := outOf(t, Forecast(p), "work")

	// work takes the pool's use until personal resets an hour on, then holds
	// alone what Held keeps of its fast rate, none after 4 hours.
	h := hoursUntil(func(h float64) float64 {
		if h <= 1 {
			return 0.06 * h
		}
		h = min(h, 4)
		return 0.06 + 0.06*((h-1)-(h*h-1)/8)
	}, 0.1)
	checkRunsOut(t, "work", out, now.Add(hours(h)), time.Minute)
}

func TestAWeekThatResetsHoldsNothingMore(t *testing.T) {
	p := Pool{
		Now:    now,
		Policy: policy,
		Accounts: []Account{
			account("work", week(0.5, 2*time.Hour), fast(0.06), refusingNewSessions),
			account("personal", week(0.4975, 6*quota.Day)),
		},
		Day:  0.06,
		Held: holding(0.5),
	}

	out := outOf(t, Forecast(p), "personal")

	// personal takes what work doesn't hold, then, once work's week resets,
	// all of it.
	checkRunsOut(t, "personal", out, now.Add(2*time.Hour+hours((0.5025-0.03*2)/0.06)), time.Second)
}

func TestTheGlobalPinSendsNewUseToItsAccountsAloneTheirReservesSpent(t *testing.T) {
	p := Pool{
		Now:    now,
		Policy: policy,
		Accounts: []Account{
			account("work", week(0.8575, 5*quota.Day), fast(0.03), spendable(0.1)),
			account("personal", week(0, quota.Day)),
		},
		Pinned: []string{"work"},
		Day:    0.03,
		Held:   holding(0),
	}

	outs := Forecast(p)

	work, personal := outOf(t, outs, "work"), outOf(t, outs, "personal")
	checkRunsOut(t, "work", work, now.Add(4*time.Hour+45*time.Minute), time.Second)
	checkRunsOut(t, "personal", personal, time.Time{}, 0)
	checkAtReset(t, "personal", personal, 0.03*(24-4.75), 1e-9)
}

func TestAnAccountThatDoesntTakeNewSessionsKeepsItsHeldUseAlone(t *testing.T) {
	p := Pool{
		Now:    now,
		Policy: policy,
		Accounts: []Account{
			account("work", week(0.4975, 2*quota.Day), fast(0.06), refusingNewSessions),
			account("personal", week(0, 6*quota.Day)),
		},
		Day:  0.06,
		Held: holding(0.5),
	}

	out := outOf(t, Forecast(p), "work")

	checkRunsOut(t, "work", out, now.Add(16*time.Hour+45*time.Minute), time.Second)
}

func TestAnAccountThatRunsOutHandsItsHeldUseOn(t *testing.T) {
	p := Pool{
		Now:    now,
		Policy: policy,
		Accounts: []Account{
			account("work", week(0.955, 5*quota.Day), fast(0.06)),
			account("personal", week(0, 6*quota.Day)),
		},
		Day:  0.06,
		Held: holding(1),
	}

	outs := Forecast(p)

	checkRunsOut(t, "work", outOf(t, outs, "work"), now.Add(45*time.Minute), time.Second)
	checkRunsOut(t, "personal", outOf(t, outs, "personal"), now.Add(17*time.Hour+25*time.Minute), time.Second)
}

func TestTheFastUseOfAWeekWithoutRoomGoesElsewhere(t *testing.T) {
	p := Pool{
		Now:    now,
		Policy: policy,
		Accounts: []Account{
			account("work", week(1, 5*quota.Day), fast(0.06)),
			account("personal", week(0.4975, 6*quota.Day)),
		},
		Day: 0.06,
	}

	out := outOf(t, Forecast(p), "personal")

	checkRunsOut(t, "personal", out, now.Add(8*time.Hour+22*time.Minute+30*time.Second), time.Second)
}

func TestHoldsBeyondThePoolsUseAreScaledDownToIt(t *testing.T) {
	p := Pool{
		Now:    now,
		Policy: policy,
		Accounts: []Account{
			account("work", week(0.1, quota.Day), fast(0.06)),
			account("personal", week(0.1, quota.Day), sized(5), fast(0.012)),
		},
		Day:  0.02,
		Held: holding(1),
	}

	outs := Forecast(p)

	// Each holds as much of the pool's use, in Pro weeks: half of it.
	use := fading(0.12, 0.02, 24)
	checkAtReset(t, "work", outOf(t, outs, "work"), 0.1+use/2, 1e-4)
	checkAtReset(t, "personal", outOf(t, outs, "personal"), 0.1+use/2/5, 1e-4)
}

func TestAWeekThatResetsIsForecastToItsResetAlone(t *testing.T) {
	resetsIn := 10*time.Hour + 5*time.Minute
	p := Pool{
		Now:      now,
		Policy:   policy,
		Accounts: []Account{account("work", week(0.2, resetsIn), fast(0.05))},
		Day:      0.05,
	}

	out := outOf(t, Forecast(p), "work")

	// Its next week runs out 30 hours on, which is no run-out of this one.
	checkRunsOut(t, "work", out, time.Time{}, 0)
	checkAtReset(t, "work", out, 0.2+0.05*resetsIn.Hours(), 1e-9)
}

func TestAnAccountWithoutAKnownResetHasNoOutButTakesWhatAPinSendsIt(t *testing.T) {
	tests := []struct {
		name   string
		pinned []string
		// want is how long until personal runs out.
		want time.Duration
	}{
		{
			name: "unpinned, it holds its use, and personal takes the rest",
			want: 16*time.Hour + 45*time.Minute,
		},
		{
			name:   "pinned, it takes the rest too, till its room runs out at 13h20, then personal takes it all",
			pinned: []string{"work"}, want: 13*time.Hour + 20*time.Minute + 8*time.Hour + 22*time.Minute + 30*time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Pool{
				Now:    now,
				Policy: policy,
				Accounts: []Account{
					account("work", quota.Window{Key: "7d", Utilization: 0.2}, fast(0.06)),
					account("personal", week(0.4975, 6*quota.Day)),
				},
				Pinned: tt.pinned,
				Day:    0.06,
				Held:   holding(0.5),
			}

			outs := Forecast(p)

			if out, ok := outs["work"]; ok {
				t.Errorf("Forecast()[%q] = %+v, want none", "work", out)
			}
			checkRunsOut(t, "personal", outOf(t, outs, "personal"), now.Add(tt.want), time.Second)
		})
	}
}

func TestAWeekStartingWithoutRoomStaysWhereItIs(t *testing.T) {
	refused := week(0.97, 3*quota.Day)
	refused.Status = quota.StatusRejected
	tests := []struct {
		name string
		a    Account
	}{
		{"at its reserve", account("work", week(0.98, 3*quota.Day), reserved(0.02))},
		{"past its reserve, at its limit", account("work", week(1, 3*quota.Day), reserved(0.02))},
		{"refused short of its limit", account("work", refused)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Pool{Now: now, Policy: policy, Accounts: []Account{tt.a}, Day: 0.05}

			out := outOf(t, Forecast(p), "work")

			if want := (Out{AtReset: tt.a.Week.Utilization}); out != want {
				t.Errorf("Forecast()[%q] = %+v, want %+v", "work", out, want)
			}
		})
	}
}

func TestASizeThatIsntAPositiveNumberCountsAsOne(t *testing.T) {
	for _, size := range []float64{0, -5, math.NaN(), math.Inf(1)} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			p := Pool{
				Now:    now,
				Policy: policy,
				Accounts: []Account{
					account("work", week(0.4975, 5*quota.Day), sized(size), fast(0.06), refusingNewSessions),
					account("personal", week(0.4, 6*quota.Day)),
				},
				Day:  0.06,
				Held: holding(0.5),
			}

			outs := Forecast(p)

			// work holds half its fast rate, a Pro's, and personal takes the
			// rest, then all of it once work runs out.
			checkRunsOut(t, "work", outOf(t, outs, "work"), now.Add(16*time.Hour+45*time.Minute), time.Second)
			checkRunsOut(t, "personal", outOf(t, outs, "personal"), now.Add(18*time.Hour+22*time.Minute+30*time.Second), time.Second)
		})
	}
}

func TestAccountsSharingAnIDAreForecastToTheEnd(t *testing.T) {
	p := Pool{
		Now:    now,
		Policy: policy,
		Accounts: []Account{
			account("work", week(0.99, 5*quota.Day)),
			account("work", week(0, 6*quota.Day)),
		},
		Day:  0.06,
		Held: holding(0),
	}

	if outs := Forecast(p); len(outs) != 1 {
		t.Errorf("Forecast() = %v, want an Out for %q alone", outs, "work")
	}
}

func TestWithoutAHoldCurveHeldUseFadesWithATimeConstantOfSixHours(t *testing.T) {
	p := Pool{
		Now:      now,
		Policy:   policy,
		Accounts: []Account{account("work", week(0.3, 2*quota.Day), fast(0.06), refusingNewSessions)},
		Day:      0.06,
	}

	out := outOf(t, Forecast(p), "work")

	checkAtReset(t, "work", out, 0.3+0.06*6*(1-math.Exp(-48.0/6)), 1e-4)
}

func TestForecastingNoAccountsGivesNone(t *testing.T) {
	if got := Forecast(Pool{Now: now, Policy: policy, Day: 0.26}); len(got) != 0 {
		t.Errorf("Forecast() = %v, want none", got)
	}
}

func TestForecastGivesTheSameEachTimeAndLeavesThePoolAsItWas(t *testing.T) {
	p := ownersPool()
	p.Pinned = []string{"2", "3"}
	accounts, pinned := slices.Clone(p.Accounts), slices.Clone(p.Pinned)

	first, second := Forecast(p), Forecast(p)

	if !maps.Equal(first, second) {
		t.Errorf("Forecast() = %v, then %v, want the same", first, second)
	}
	if !slices.Equal(p.Accounts, accounts) || !slices.Equal(p.Pinned, pinned) {
		t.Errorf("Forecast() left the pool's accounts %v and pin %v, want %v and %v", p.Accounts, p.Pinned, accounts, pinned)
	}
}

func BenchmarkForecast(b *testing.B) {
	for _, n := range []int{4, 10} {
		b.Run(fmt.Sprintf("%d accounts", n), func(b *testing.B) {
			p := Pool{Now: now, Policy: policy, Day: 0.013 * ultra * float64(n) / 4}
			for i := range n {
				used, resetsIn := float64(i%5)/5, time.Duration(i+1)*15*time.Hour
				p.Accounts = append(p.Accounts, account(fmt.Sprint(i+1), week(used, resetsIn), sized(ultra), fast(0.02)))
			}
			b.ReportAllocs()
			for b.Loop() {
				Forecast(p)
			}
		})
	}
}
