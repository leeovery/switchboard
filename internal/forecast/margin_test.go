package forecast

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

// pastThePlay is when the weeks of a margin's pool reset: after the week it
// plays, so its room is only what it has now.
const pastThePlay = playSpan + quota.Day

// halfUsedPool is two Pro accounts, each half its week used, resetting after
// the play, the pool's use running steadily, its fast rate its day's, none of
// it held, its week's use share of the room they have together: an
// account's worth.
func halfUsedPool(share float64) Pool {
	day := share / playSpan.Hours()
	return Pool{
		Now:    now,
		Policy: policy,
		Accounts: []Account{
			account("work", week(0.5, pastThePlay), fast(day/2)),
			account("personal", week(0.5, pastThePlay), fast(day/2)),
		},
		Day:  day,
		Held: holding(0),
	}
}

func TestMarginOfAPoolThatLastsSaysHowMuchBusierItCouldGet(t *testing.T) {
	tests := []struct {
		name string
		// share is the pool's week's use, of the room it has.
		share     float64
		wantSpare int
	}{
		{name: "running out 5% busier, under 5% to spare", share: 1 / 1.025, wantSpare: 0},
		{name: "15% to spare", share: 1 / 1.175, wantSpare: 15},
		{name: "50% to spare, running out 55% busier", share: 1 / 1.525, wantSpare: 50},
		{name: "lasting 55% busier, over 50% to spare", share: 0.5, wantSpare: 55},
		{name: "no use at all", share: 0, wantSpare: 55},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MarginOf(halfUsedPool(tt.share))

			if want := (Margin{Kind: MarginLasts, Spare: tt.wantSpare}); got != want {
				t.Errorf("MarginOf() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestMarginOfAPoolThatRunsOutSaysWhenAndHowMuchLessWouldLast(t *testing.T) {
	tests := []struct {
		name     string
		share    float64
		wantLess int
	}{
		{name: "lasting at 25% less", share: 1 / 0.775, wantLess: 25},
		{name: "lasting at 5% less", share: 1 / 0.975, wantLess: 5},
		{name: "lasting at 50% less", share: 1 / 0.525, wantLess: 50},
		{name: "not lasting at 50% less", share: 1 / 0.45, wantLess: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MarginOf(halfUsedPool(tt.share))

			// The room the two have, an account's worth, spent at the day's
			// rate, within the step each run-out in it is spread across.
			wantOut := now.Add(hours(playSpan.Hours() / tt.share))
			if got.Kind != MarginRunsOut || got.Less != tt.wantLess || got.RunsOut.Sub(wantOut).Abs() > playStep {
				t.Errorf("MarginOf() = %+v, want running out at %v, lasting at %d%% less", got, wantOut, tt.wantLess)
			}
		})
	}
}

func TestThePoolRunsOutWhereTheLastOfItsRoomRunsOutWithinAStep(t *testing.T) {
	tests := []struct {
		name string
		// in is how far into the play the room runs out.
		in time.Duration
	}{
		{name: "part-way into a step", in: 30*time.Hour + 4*time.Minute},
		{name: "as a step ends", in: 30 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// personal has no room, so work's, half its week, is the pool's.
			p := halfUsedPool(playSpan.Hours() / tt.in.Hours() / 2)
			p.Accounts[1].Week.Utilization = 1

			got := MarginOf(p)

			if want := now.Add(tt.in); got.Kind != MarginRunsOut || got.RunsOut.Sub(want).Abs() > time.Second {
				t.Errorf("MarginOf() = %+v, want running out at %v", got, want)
			}
		})
	}
}

func TestThePoolRunsOutWhereUseHeldPastAnAccountsRoomFindsNoOther(t *testing.T) {
	// work's own use, all of the pool's, held on it, runs it out in 10 hours,
	// as personal, its room spent, has none to take it.
	p := Pool{
		Now:    now,
		Policy: policy,
		Accounts: []Account{
			account("work", week(0.9, pastThePlay), fast(0.01)),
			account("personal", week(1, pastThePlay)),
		},
		Day:  0.01,
		Held: holding(1),
	}

	got := MarginOf(p)

	if want := now.Add(10 * time.Hour); got.Kind != MarginRunsOut || got.RunsOut.Sub(want).Abs() > time.Second {
		t.Errorf("MarginOf() = %+v, want running out at %v", got, want)
	}
}

// steadyPool is a pool of the Pro accounts given, its use day Pro weeks an
// hour, steadily, its fast rate its day's, none of it held on an account.
func steadyPool(day float64, accounts ...Account) Pool {
	accounts = slices.Clone(accounts)
	accounts[0].Fast = day
	return Pool{Now: now, Policy: policy, Accounts: accounts, Day: day, Held: holding(0)}
}

// pinned is p, its pin naming the accounts with the given ids.
func pinned(p Pool, ids ...string) Pool {
	p.Pinned = ids
	return p
}

func TestThePoolRunsOutWhereTheLastOfTheRoomItCanUseRunsOut(t *testing.T) {
	// fillsAt is the day's rate that spends half a week by 30 hours and 3
	// minutes in, part-way into the step from 30 hours.
	const fillsAt = 0.5 / (30 + 3.0/60)
	tests := []struct {
		name   string
		pool   Pool
		want   time.Duration
		within time.Duration
	}{
		{
			name: "a reserve holding an account back, only the reserve left",
			pool: steadyPool(0.015, account("work", week(0.5, pastThePlay), reserved(0.1)), account("personal", week(0.5, pastThePlay))),
			// 0.4 and 0.5 of a week's room, at 1.5% an hour.
			want: 60 * time.Hour, within: playStep,
		},
		{
			name: "a pin, its account's reserve spent, falling back once it's out",
			pool: pinned(steadyPool(0.01, account("work", week(0.5, pastThePlay)), account("personal", week(0.85, pastThePlay), spendable(0.1))), "personal"),
			// 0.15 of personal's week past its reserve, then work's 0.5.
			want: 65 * time.Hour, within: playStep,
		},
		{
			name: "an account that takes no requests beside one that does",
			pool: steadyPool(0.01, account("work", week(0, pastThePlay), refused), account("personal", week(0.5, pastThePlay))),
			want: 50 * time.Hour, within: time.Second,
		},
		{
			name: "a week that resets mid-play, its room back",
			pool: steadyPool(0.01, account("work", week(0.5, 40*time.Hour)), account("personal", week(1, pastThePlay))),
			// 0.4 of work's week by its reset, then the whole of its next.
			want: 140 * time.Hour, within: time.Second,
		},
		{
			name: "a week resetting in the step before the last room runs out in it",
			pool: steadyPool(fillsAt, account("work", week(0.5, pastThePlay)), account("personal", week(1, 30*time.Hour+2*time.Minute))),
			// personal, back as the next step starts, takes the use from then.
			want: 30*time.Hour + 10*time.Minute + hours(1/fillsAt), within: time.Second,
		},
		{
			name: "a week resetting in the step after the last room runs out in it",
			pool: steadyPool(fillsAt, account("work", week(0.5, pastThePlay)), account("personal", week(1, 30*time.Hour+8*time.Minute))),
			want: 30*time.Hour + 10*time.Minute + hours(1/fillsAt), within: time.Second,
		},
		{
			name: "every account out as a step starts",
			// personal's reserve is its whole week, so the week it resets to
			// in the step work runs out in has no room either, as the next
			// step starts.
			pool: steadyPool(fillsAt, account("work", week(0.5, pastThePlay)), account("personal", week(0, 30*time.Hour+5*time.Minute), reserved(1))),
			want: 30*time.Hour + 10*time.Minute, within: time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MarginOf(tt.pool)

			if want := now.Add(tt.want); got.Kind != MarginRunsOut || got.RunsOut.Sub(want).Abs() > tt.within {
				t.Errorf("MarginOf() = %+v, want running out at %v, within %v", got, want, tt.within)
			}
		})
	}
}

// barredUntil has the account take no requests until so long after now, as a
// refusal of its token lifts then.
func barredUntil(after time.Duration) func(*Account) {
	return func(a *Account) { a.TakesFrom = now.Add(after) }
}

func TestARefusalThatLiftsMidPlayBringsTheAccountsRoomBack(t *testing.T) {
	// personal's half a week runs out 50 hours on; work, back at 20 hours,
	// takes the new sessions from then, holding none of the use it had.
	p := steadyPool(0.01, account("work", week(0.5, pastThePlay), barredUntil(20*time.Hour)), account("personal", week(0.5, pastThePlay)))

	got := MarginOf(p)

	if want := now.Add(100 * time.Hour); got.Kind != MarginRunsOut || got.RunsOut.Sub(want).Abs() > playStep {
		t.Errorf("MarginOf() = %+v, want running out at %v, as the two's room is spent", got, want)
	}
	out := outOf(t, Forecast(p), "work")
	if !out.RunsOut.After(now.Add(20*time.Hour)) || out.RunsOut.After(now.Add(100*time.Hour+playStep)) {
		t.Errorf("Forecast()[work].RunsOut = %v, want it running out once back, by %v", out.RunsOut, now.Add(100*time.Hour))
	}
}

func TestAnAccountBarredAtNowHoldsNoneOfItsUseOnceBack(t *testing.T) {
	// work's use, all of the pool's, would stay on it from an hour on were
	// it held there; it isn't, so the pin sends it all to personal, whose
	// half a week runs out 50 hours on.
	p := pinned(steadyPool(0.01, account("work", week(0.5, pastThePlay), barredUntil(time.Hour)), account("personal", week(0.5, pastThePlay))), "personal")
	p.Held = holding(1)

	out := outOf(t, Forecast(p), "personal")

	checkRunsOut(t, "personal", out, now.Add(50*time.Hour), time.Second)
}

func TestARefusalLiftingPastThePlayBarsTheAccountAllWeek(t *testing.T) {
	personal := account("personal", week(0.5, pastThePlay))
	lifting := steadyPool(0.01, account("work", week(0.5, pastThePlay), barredUntil(playSpan+time.Hour)), personal)
	barred := steadyPool(0.01, account("work", week(0.5, pastThePlay), refused), personal)

	if got, want := MarginOf(lifting), MarginOf(barred); got != want || got.Kind != MarginRunsOut {
		t.Errorf("MarginOf() = %+v, want %+v, running out as with the account barred all week", got, want)
	}
	if got, want := Forecast(lifting), Forecast(barred); !maps.Equal(got, want) {
		t.Errorf("Forecast() = %v, want %v, as with the account barred all week", got, want)
	}
}

func TestAWeekResettingInTheStepOnAnAccountBarredThenLeavesTheRunOut(t *testing.T) {
	const fillsAt = 0.5 / (30 + 3.0/60)
	// personal's week resets at 30 hours and 5 minutes, but it takes no
	// requests till 40 hours on.
	p := steadyPool(fillsAt, account("work", week(0.5, pastThePlay)), account("personal", week(1, 30*time.Hour+5*time.Minute), barredUntil(40*time.Hour)))

	got := MarginOf(p)

	if want := now.Add(30*time.Hour + 3*time.Minute); got.Kind != MarginRunsOut || got.RunsOut.Sub(want).Abs() > time.Second {
		t.Errorf("MarginOf() = %+v, want running out at %v", got, want)
	}
}

func TestMarginOfAPoolWithoutRoomOrAccountsToSpeakOfTellsNothing(t *testing.T) {
	tests := []struct {
		name     string
		accounts []Account
	}{
		{name: "every account out", accounts: []Account{account("work", week(1, pastThePlay)), account("personal", week(1, pastThePlay))}},
		{name: "none taking requests", accounts: []Account{account("work", week(0, pastThePlay), refused), account("personal", week(0, pastThePlay), refused)}},
		{name: "one account", accounts: []Account{account("work", week(0, pastThePlay))}},
		{name: "none", accounts: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := halfUsedPool(2)
			p.Accounts = tt.accounts

			if got := MarginOf(p); got != (Margin{}) {
				t.Errorf("MarginOf() = %+v, want none", got)
			}
		})
	}
}

func TestAPaceScaledScalesTheFastRatesAndTheDayAlone(t *testing.T) {
	p := ownersPool()
	p.Pinned = []string{"2"}
	accounts := slices.Clone(p.Accounts)

	got := scaled(p, 120)

	if want := p.Day * 1.2; got.Day != want {
		t.Errorf("scaled().Day = %v, want %v", got.Day, want)
	}
	for i, a := range got.Accounts {
		want := accounts[i]
		want.Fast *= 1.2
		if a != want {
			t.Errorf("scaled().Accounts[%d] = %+v, want %+v", i, a, want)
		}
	}
	if !got.Now.Equal(p.Now) || !slices.Equal(got.Pinned, p.Pinned) || got.Policy.Perishable != p.Policy.Perishable {
		t.Errorf("scaled() = %+v, want the pool's Now, pin and policy", got)
	}
	if !slices.Equal(p.Accounts, accounts) {
		t.Errorf("scaled() left the pool's accounts %+v, want %+v", p.Accounts, accounts)
	}
}

func TestABusierPoolNeverHasMoreToSpare(t *testing.T) {
	pools := map[string]func(day float64) Pool{
		"half used": func(day float64) Pool {
			return halfUsedPool(day * 10)
		},
		"the owner's": func(day float64) Pool {
			p := ownersPool()
			p.Day = day
			return p
		},
	}
	for name, pool := range pools {
		t.Run(name, func(t *testing.T) {
			last, lastDay := slack(Margin{Kind: MarginLasts, Spare: MarginReach + MarginStep}), 0.0
			for day := 0.0; day <= 0.6; day += 0.02 {
				got := slack(MarginOf(pool(day)))
				if got > last {
					t.Errorf("MarginOf() at a day's rate of %v has %d to spare, more than %d at %v", day, got, last, lastDay)
				}
				last, lastDay = got, day
			}
		})
	}
}

// slack ranks a margin by how much the pool has to spare: what it has to
// spare, or how much less use would last, below none.
func slack(m Margin) int {
	switch m.Kind {
	case MarginLasts:
		return m.Spare
	case MarginRunsOut:
		if m.Less == 0 {
			return -MarginReach - MarginStep
		}
		return -m.Less
	case MarginNone:
	}
	panic(fmt.Sprintf("no slack in %+v", m))
}

func TestMarginOfLeavesThePoolAsItWas(t *testing.T) {
	p := ownersPool()
	accounts := slices.Clone(p.Accounts)

	first, second := MarginOf(p), MarginOf(p)

	if first != second {
		t.Errorf("MarginOf() = %+v, then %+v, want the same", first, second)
	}
	if !slices.Equal(p.Accounts, accounts) {
		t.Errorf("MarginOf() left the pool's accounts %+v, want %+v", p.Accounts, accounts)
	}
}

func TestTheBenchmarksPoolLastsAsFarAsAMarginPlays(t *testing.T) {
	for _, n := range []int{4, 10} {
		if got, want := MarginOf(benchPool(n)), (Margin{Kind: MarginLasts, Spare: MarginReach + MarginStep}); got != want {
			t.Errorf("MarginOf(benchPool(%d)) = %+v, want %+v", n, got, want)
		}
	}
}

func BenchmarkMarginOf(b *testing.B) {
	for _, n := range []int{4, 10} {
		b.Run(fmt.Sprintf("%d accounts", n), func(b *testing.B) {
			p := benchPool(n)
			b.ReportAllocs()
			for b.Loop() {
				MarginOf(p)
			}
		})
	}
}
