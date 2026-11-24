package forecast

import (
	"fmt"
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
