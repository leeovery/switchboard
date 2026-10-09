package views_test

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// peaking are the days, of no requests, from the Monday each account's first
// week began on to today, on which each account's week reset at 00:30 each
// Monday, its weeks' peaks those given, oldest first, the last of them the
// week before this one's.
func peaking(peaks map[string][]float64) []ledger.Summary {
	this := on("2026-10-05")
	first := this.Format(time.DateOnly)
	resets := make(map[string][]ledger.AccountDay)
	for _, id := range slices.Sorted(maps.Keys(peaks)) {
		n := len(peaks[id])
		for i, p := range peaks[id] {
			ended := this.AddDate(0, 0, -7*(n-1-i))
			date := ended.Format(time.DateOnly)
			resets[date] = append(resets[date], windowsDay{resets: []ledger.Reset{reset("7d", date, "00:30", p)}}.of(id))
			first = min(first, ended.AddDate(0, 0, -7).Format(time.DateOnly))
		}
	}
	var days []ledger.Summary
	for date, accounts := range resets {
		days = append(days, day(date, accounts...))
	}
	return daysFrom(first, days...)
}

// The accounts the capacity's tests configure.
var (
	workAndSide = config.Accounts{{ID: "work"}, {ID: "side"}}
	workAlone   = config.Accounts{{ID: "work"}}
)

func TestCapacityIsWeeksVerdictsFromReplayingEachWholeWeek(t *testing.T) {
	tests := []struct {
		name     string
		accounts config.Accounts
		peaks    map[string][]float64
		want     string
	}{
		{
			// Work hit its limit once. Without either, the other's peak
			// rises by all of its, each counting as one, being on no plan:
			// 3 weeks short. With one more, none.
			name: "about right", accounts: workAndSide,
			peaks: map[string][]float64{"work": {1, 0.6, 0.5, 0.5, 0.4, 0.7, 0.4}, "side": {0.5, 0.5, 0.6, 0.3, 0.3, 0.2, 0.5}},
			want: `{"weeks":7,"short":1,"with_one_fewer":3,"with_one_more":0,"verdict":"about right","accounts":[` +
				`{"account":"work","verdict":"plenty to spare most weeks","weeks":7,"limits":1,"left_at_reset":0.4142857142857143,"without_it":3},` +
				`{"account":"side","verdict":"plenty to spare most weeks","weeks":7,"limits":0,"left_at_reset":0.5857142857142857,"without_it":3},` +
				`{"all":true,"verdict":"room to spare together","weeks":7,"limits":1,"left_at_reset":0.5}]}`,
		},
		{
			name: "one too few: short on 4 or more of 7, and one more would halve it", accounts: workAndSide,
			peaks: map[string][]float64{"work": {1, 1, 1, 1, 0.5, 0.6, 1}, "side": {1, 0.9, 1, 0.8, 0.4, 0.5, 0.9}},
			want: `{"weeks":7,"short":5,"with_one_fewer":6,"with_one_more":0,"verdict":"one too few","accounts":[` +
				`{"account":"work","verdict":"hits its limit most weeks","weeks":7,"limits":5,"left_at_reset":0.12857142857142856,"without_it":6},` +
				`{"account":"side","verdict":"ends most weeks with room","weeks":7,"limits":2,"left_at_reset":0.21428571428571427,"without_it":6},` +
				`{"all":true,"verdict":"little to spare together","weeks":7,"limits":5,"left_at_reset":0.17142857142857143}]}`,
		},
		{
			// By their plans, 20, 5 and 1 Pros, the rest cover side or
			// spare: of the two, spare used less, so it's the one to drop.
			// Without it, work covers side too, though side can't cover
			// work. Spare could go; side, used more than a tenth, is barely
			// used.
			name:     "one more than you need, by the accounts' plans",
			accounts: config.Accounts{{ID: "work", Plan: "max20x"}, {ID: "side", Plan: "max5x"}, {ID: "spare", Plan: "pro"}},
			peaks: map[string][]float64{"work": {0.5, 0.6, 0.4, 0.5, 0.6, 0.5, 0.4}, "side": {0.2, 0.2, 0.3, 0.2, 0.2, 0.3, 0.2},
				"spare": {0.05, 0.1, 0, 0.05, 0.1, 0.05, 0}},
			want: `{"weeks":7,"short":0,"with_one_fewer":0,"with_two_fewer":0,"with_one_more":0,"drop":"spare","verdict":"one more than you need","accounts":[` +
				`{"account":"work","verdict":"plenty to spare most weeks","weeks":7,"limits":0,"left_at_reset":0.5,"without_it":7},` +
				`{"account":"side","verdict":"barely used","weeks":7,"limits":0,"left_at_reset":0.7714285714285715,"without_it":0},` +
				`{"account":"spare","verdict":"could go","weeks":7,"limits":0,"left_at_reset":0.95,"without_it":0},` +
				`{"all":true,"verdict":"room to spare together","weeks":7,"limits":0,"left_at_reset":0.7404761904761905}]}`,
		},
		{
			// Without either, the same 3 weeks are short: side, used less,
			// is the one to drop. With two accounts, two fewer would leave
			// none, so it isn't given.
			name: "a tie in the replay: the account used less is dropped", accounts: workAndSide,
			peaks: map[string][]float64{"work": {1, 0.6, 1, 0.5, 1, 0.7, 0.4}, "side": {0.2, 0.3, 0.1, 0.2, 0.3, 0.1, 0.2}},
			want: `{"weeks":7,"short":3,"with_one_fewer":3,"with_one_more":0,"drop":"side","verdict":"one more than you need","accounts":[` +
				`{"account":"work","verdict":"hits its limit most weeks","weeks":7,"limits":3,"left_at_reset":0.2571428571428571,"without_it":3},` +
				`{"account":"side","verdict":"barely used","weeks":7,"limits":0,"left_at_reset":0.8,"without_it":3},` +
				`{"all":true,"verdict":"room to spare together","weeks":7,"limits":3,"left_at_reset":0.5285714285714286}]}`,
		},
		{
			// Short every week, so one fewer can be no worse: one too few
			// holds before one more than you need, and names no account to
			// drop, nor one that could go.
			name: "short most weeks, one fewer no worse: one too few", accounts: workAndSide,
			peaks: map[string][]float64{"work": {1, 1, 1, 1, 1, 1, 1}, "side": {0, 0, 0, 0, 0, 0, 0}},
			want: `{"weeks":7,"short":7,"with_one_fewer":7,"with_one_more":0,"verdict":"one too few","accounts":[` +
				`{"account":"work","verdict":"hits its limit most weeks","weeks":7,"limits":7,"left_at_reset":0,"without_it":7},` +
				`{"account":"side","verdict":"barely used","weeks":7,"limits":0,"left_at_reset":1,"without_it":7},` +
				`{"all":true,"verdict":"room to spare together","weeks":7,"limits":7,"left_at_reset":0.5}]}`,
		},
		{
			// Side, on no plan, counts as Max 20x, as work is: without
			// either, the other's peak rises by all of its, 7 weeks short.
			// Counted as one, side's loss would short none.
			name: "an account on no plan counts as the plan most of the others have", accounts: config.Accounts{{ID: "work", Plan: "max20x"}, {ID: "side"}},
			peaks: map[string][]float64{"work": {0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5}, "side": {0.6, 0.6, 0.6, 0.6, 0.6, 0.6, 0.6}},
			want: `{"weeks":7,"short":0,"with_one_fewer":7,"with_one_more":0,"verdict":"about right","accounts":[` +
				`{"account":"work","verdict":"plenty to spare most weeks","weeks":7,"limits":0,"left_at_reset":0.5,"without_it":7},` +
				`{"account":"side","verdict":"plenty to spare most weeks","weeks":7,"limits":0,"left_at_reset":0.4,"without_it":7},` +
				`{"all":true,"verdict":"room to spare together","weeks":7,"limits":0,"left_at_reset":0.45}]}`,
		},
		{
			// Side's peaks are known in the last 3 weeks alone: the replay
			// judges those, rather than taking side to have had no room in
			// the 4 before, which work's own verdict counts.
			name: "weeks some account's peak isn't known in", accounts: workAndSide,
			peaks: map[string][]float64{"work": {1, 1, 1, 1, 0.5, 0.4, 0.3}, "side": {0.2, 0.3, 0.1}},
			want: `{"weeks":3,"short":0,"with_one_fewer":0,"with_one_more":0,"drop":"side","verdict":"one more than you need","accounts":[` +
				`{"account":"work","verdict":"hits its limit most weeks","weeks":7,"limits":4,"left_at_reset":0.2571428571428571,"without_it":0},` +
				`{"account":"side","verdict":"barely used","weeks":3,"limits":0,"left_at_reset":0.8,"without_it":0},` +
				`{"all":true,"verdict":"room to spare together","weeks":3,"limits":0,"left_at_reset":0.7}]}`,
		},
		{
			name: "one account: none fewer, and none without it", accounts: workAlone,
			peaks: map[string][]float64{"work": {1, 1, 1, 1, 0.5, 0.2, 0.3}},
			want: `{"weeks":7,"short":4,"with_one_more":0,"verdict":"one too few","accounts":[` +
				`{"account":"work","verdict":"hits its limit most weeks","weeks":7,"limits":4,"left_at_reset":0.2857142857142857},` +
				`{"all":true,"verdict":"room to spare together","weeks":7,"limits":4,"left_at_reset":0.2857142857142857}]}`,
		},
		{
			// 3 limits of 7 is 1 of 2: most weeks.
			name: "two weeks: the thresholds of 7 in proportion", accounts: workAlone,
			peaks: map[string][]float64{"work": {1, 0.3}},
			want: `{"weeks":2,"short":1,"with_one_more":0,"verdict":"about right","accounts":[` +
				`{"account":"work","verdict":"hits its limit most weeks","weeks":2,"limits":1,"left_at_reset":0.35},` +
				`{"all":true,"verdict":"room to spare together","weeks":2,"limits":1,"left_at_reset":0.35}]}`,
		},
		{
			// 1 of 3 is fewer than 3 of 7.
			name: "three weeks: the thresholds of 7 in proportion", accounts: workAlone,
			peaks: map[string][]float64{"work": {1, 0.3, 0.2}},
			want: `{"weeks":3,"short":1,"with_one_more":0,"verdict":"about right","accounts":[` +
				`{"account":"work","verdict":"plenty to spare most weeks","weeks":3,"limits":1,"left_at_reset":0.5},` +
				`{"all":true,"verdict":"room to spare together","weeks":3,"limits":1,"left_at_reset":0.5}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(peaking(tt.peaks))
			in.Accounts = tt.accounts
			if got := jsonOf(t, views.NewHistory(in).Capacity); got != tt.want {
				t.Errorf("the capacity is\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestCapacityReplaysOnlyWholeWeeksTheDaysSpan(t *testing.T) {
	tests := []struct {
		name string
		days []ledger.Summary
	}{
		{name: "the empty ledger", days: daysFrom("2026-10-07")},
		{
			// The week of the 28th began before the days asked for.
			name: "a week begun before the first day",
			days: daysFrom("2026-09-30", day("2026-10-05", windowsDay{resets: []ledger.Reset{reset("7d", "2026-10-05", "00:30", 1)}}.of("work"))),
		},
		{
			// Work's week begun on Monday has hit its limit so far, but it's
			// still running.
			name: "this week",
			days: daysFrom("2026-10-05",
				day("2026-10-05", windowsDay{resets: []ledger.Reset{reset("7d", "2026-10-05", "00:30", 0.5)}}.of("work")),
				day("2026-10-06", windowsDay{highest: map[string]float64{"7d": 1}}.of("work"))),
		},
		{
			name: "a week no account's peak is known in",
			days: daysFrom("2026-09-21", day("2026-09-23", account("work", sent("claude-opus-5-5", 1, inputUsage)))),
		},
	}
	want := `{"weeks":0,"short":0,"with_one_fewer":0,"with_one_more":0,"accounts":[` +
		`{"account":"work","weeks":0,"limits":0,"without_it":0},{"account":"side","weeks":0,"limits":0,"without_it":0},{"all":true,"weeks":0,"limits":0}]}`
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(tt.days)
			in.Accounts = workAndSide
			if got := jsonOf(t, views.NewHistory(in).Capacity); got != want {
				t.Errorf("the capacity is\n%s\nwant\n%s", got, want)
			}
		})
	}
}
