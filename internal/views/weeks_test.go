package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// windowsDay is an account's day, of no requests, as a summary of version 2
// that read its windows gives it: each window's highest use and how far it
// rose, by key, and its resets and limits.
type windowsDay struct {
	highest, rise map[string]float64
	resets        []ledger.Reset
	limits        []ledger.Limit
}

// of is the day as the account's with the given id.
func (w windowsDay) of(id string) ledger.AccountDay {
	return ledger.AccountDay{Account: id, Highest: w.highest, Rise: w.rise, Resets: w.resets, Limits: w.limits,
		SessionIDs: []string{}, MinutesAtCap: new(0), MinutesAtLimit: new(0)}
}

// worksDays are the summaries of each day from the one with the date first
// to today's, work's windows as given, by the days' dates.
func worksDays(first string, given map[string]windowsDay) []ledger.Summary {
	var days []ledger.Summary
	for date, w := range given {
		days = append(days, day(date, w.of("work")))
	}
	return daysFrom(first, days...)
}

// onThursdays are work's days from Monday 14 September, its week resetting
// at 10:00 each Thursday: it hit its week's limit on the 30th, the week
// ending on 1 October, a 5-hour limit on the 15th, and this week's use has
// reached 25%.
var onThursdays = worksDays("2026-09-14", map[string]windowsDay{
	"2026-09-15": {limits: []ledger.Limit{limit("5h", "2026-09-15", "16:00")}},
	"2026-09-17": {resets: []ledger.Reset{reset("7d", "2026-09-17", "10:00", 0.3)}},
	"2026-09-24": {resets: []ledger.Reset{reset("7d", "2026-09-24", "10:00", 0.6)}},
	"2026-09-30": {limits: []ledger.Limit{limit("7d", "2026-09-30", "18:00")}},
	"2026-10-01": {resets: []ledger.Reset{reset("7d", "2026-10-01", "10:00", 1)}},
	"2026-10-06": {highest: map[string]float64{"7d": 0.25}},
})

func TestWeeksPlaceEachAccountsOwnWeeksUnderTheWeekTheyBeganNearest(t *testing.T) {
	tests := []struct {
		name       string
		days       []ledger.Summary
		weekStarts time.Weekday
		want       string
	}{
		{
			// A week from a Thursday is nearer the Monday before it. The
			// first, whose start the days don't give, began a week before
			// its end, so under the week before the 14th, with the 5-hour
			// limit in it; the limit of the week is in the week it was
			// reached in, though the calendar's next began before it.
			name: "weeks from Monday", days: onThursdays, weekStarts: time.Monday,
			want: `[{"week":"2026-09-14","accounts":[{"account":"work","peaks":{"7d":0.6}},{"account":"side"},{"all":true,"peaks":{"7d":0.6}}]},` +
				`{"week":"2026-09-21","accounts":[{"account":"work","peaks":{"7d":1},"limits":{"7d":1}},{"account":"side"},{"all":true,"peaks":{"7d":1}}]},` +
				`{"week":"2026-09-28","accounts":[{"account":"work","peaks":{"7d":0.25}},{"account":"side"},{"all":true,"peaks":{"7d":0.25}}]},` +
				`{"week":"2026-10-05","accounts":[{"account":"work"},{"account":"side"},{"all":true}]}]`,
		},
		{
			// A week from a Thursday is nearer the Sunday after it.
			name: "weeks from Sunday", days: onThursdays, weekStarts: time.Sunday,
			want: `[{"week":"2026-09-13","accounts":[{"account":"work","peaks":{"7d":0.3},"limits":{"5h":1}},{"account":"side"},{"all":true,"peaks":{"7d":0.3}}]},` +
				`{"week":"2026-09-20","accounts":[{"account":"work","peaks":{"7d":0.6}},{"account":"side"},{"all":true,"peaks":{"7d":0.6}}]},` +
				`{"week":"2026-09-27","accounts":[{"account":"work","peaks":{"7d":1},"limits":{"7d":1}},{"account":"side"},{"all":true,"peaks":{"7d":1}}]},` +
				`{"week":"2026-10-04","accounts":[{"account":"work","peaks":{"7d":0.25}},{"account":"side"},{"all":true,"peaks":{"7d":0.25}}]}]`,
		},
		{
			// Reset by hand on Wednesday the 23rd, the week begun on the
			// Monday ended then, though a week before its end is nearer the
			// 14th, and the next began then, ending a week after: two weeks
			// under the 21st, its peak the higher.
			name: "a week reset by hand", weekStarts: time.Monday,
			days: worksDays("2026-09-14", map[string]windowsDay{
				"2026-09-21": {resets: []ledger.Reset{reset("7d", "2026-09-21", "00:30", 0.5)}},
				"2026-09-23": {resets: []ledger.Reset{reset("7d", "2026-09-23", "13:30", 0.9)}},
				"2026-09-30": {resets: []ledger.Reset{reset("7d", "2026-09-30", "13:30", 0.4)}},
				"2026-10-06": {highest: map[string]float64{"7d": 0.1}},
			}),
			want: `[{"week":"2026-09-14","accounts":[{"account":"work","peaks":{"7d":0.5}},{"account":"side"},{"all":true,"peaks":{"7d":0.5}}]},` +
				`{"week":"2026-09-21","accounts":[{"account":"work","peaks":{"7d":0.9}},{"account":"side"},{"all":true,"peaks":{"7d":0.9}}]},` +
				`{"week":"2026-09-28","accounts":[{"account":"work","peaks":{"7d":0.1}},{"account":"side"},{"all":true,"peaks":{"7d":0.1}}]},` +
				`{"week":"2026-10-05","accounts":[{"account":"work"},{"account":"side"},{"all":true}]}]`,
		},
		{
			// Fable's week resets on Thursdays, the account's on Mondays: a
			// limit of Fable's is in its own week, under the 21st, though
			// the account's week it fell in is under the 28th. A 5-hour
			// window has no weeks, so its limit is in the account's week,
			// under the 28th, though it fell on the 5th. Neither week still
			// running has been read since it began, so neither's peak is
			// known.
			name: "a model's own week", weekStarts: time.Monday,
			days: worksDays("2026-09-21", map[string]windowsDay{
				"2026-09-21": {resets: []ledger.Reset{reset("7d", "2026-09-21", "00:30", 0.5)}},
				"2026-09-24": {resets: []ledger.Reset{reset("7d_oi", "2026-09-24", "10:00", 0.6)}},
				"2026-09-25": {resets: []ledger.Reset{reset("5h", "2026-09-25", "12:00", 1)}},
				"2026-09-28": {resets: []ledger.Reset{reset("7d", "2026-09-28", "00:30", 0.4)}},
				"2026-09-29": {limits: []ledger.Limit{limit("7d_oi", "2026-09-29", "07:00")}},
				"2026-10-01": {resets: []ledger.Reset{reset("7d_oi", "2026-10-01", "10:00", 0.9)}},
				"2026-10-05": {resets: []ledger.Reset{reset("7d", "2026-10-05", "00:30", 0.3)}, limits: []ledger.Limit{limit("5h", "2026-10-05", "00:10")}},
			}),
			want: `[{"week":"2026-09-21","accounts":[{"account":"work","peaks":{"7d":0.4,"7d_oi":0.9},"limits":{"7d_oi":1}},{"account":"side"},` +
				`{"all":true,"peaks":{"7d":0.4,"7d_oi":0.9}}]},` +
				`{"week":"2026-09-28","accounts":[{"account":"work","peaks":{"7d":0.3},"limits":{"5h":1}},{"account":"side"},{"all":true,"peaks":{"7d":0.3}}]},` +
				`{"week":"2026-10-05","accounts":[{"account":"work"},{"account":"side"},{"all":true}]}]`,
		},
		{
			// The week that reset on the 24th, a day of no requests, isn't
			// taken for part of the next: that began a week before its end,
			// and the week before it isn't known.
			name: "a reset the days don't give", weekStarts: time.Monday,
			days: worksDays("2026-09-14", map[string]windowsDay{
				"2026-09-17": {resets: []ledger.Reset{reset("7d", "2026-09-17", "10:00", 0.3)}},
				"2026-10-01": {resets: []ledger.Reset{reset("7d", "2026-10-01", "10:00", 0.8)}},
				"2026-10-03": {highest: map[string]float64{"7d": 0.2}},
			}),
			want: `[{"week":"2026-09-14","accounts":[{"account":"work"},{"account":"side"},{"all":true}]},` +
				`{"week":"2026-09-21","accounts":[{"account":"work","peaks":{"7d":0.8}},{"account":"side"},{"all":true,"peaks":{"7d":0.8}}]},` +
				`{"week":"2026-09-28","accounts":[{"account":"work","peaks":{"7d":0.2}},{"account":"side"},{"all":true,"peaks":{"7d":0.2}}]},` +
				`{"week":"2026-10-05","accounts":[{"account":"work"},{"account":"side"},{"all":true}]}]`,
		},
		{
			// Today's highest is the week before's, 80%: the week begun at
			// 09:00 rose 15% less the 10% it rose to 80% from yesterday's
			// 70%. The week that ended is placed under the week before these
			// days.
			name: "this week, begun today", weekStarts: time.Monday,
			days: worksDays("2026-10-05", map[string]windowsDay{
				"2026-10-06": {highest: map[string]float64{"7d": 0.7}, rise: map[string]float64{"7d": 0.1}},
				"2026-10-07": {highest: map[string]float64{"7d": 0.8}, rise: map[string]float64{"7d": 0.15}, resets: []ledger.Reset{reset("7d", "2026-10-07", "09:00", 0.8)}},
			}),
			want: `[{"week":"2026-10-05","accounts":[{"account":"work","peaks":{"7d":0.05}},{"account":"side"},{"all":true,"peaks":{"7d":0.05}}]}]`,
		},
		{
			name: "a day of a summary of version 1, which gave no resets", weekStarts: time.Monday,
			days: daysFrom("2026-10-05", ledger.Summary{Version: 1, Day: "2026-10-06", Accounts: []ledger.AccountDay{
				{Account: "work", Models: []ledger.ModelDay{sent("claude-opus-5-5", 1, inputUsage)}, Sessions: 1, Highest: map[string]float64{"7d": 0.4}},
			}}),
			want: `[{"week":"2026-10-05","accounts":[{"account":"work"},{"account":"side"},{"all":true}]}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(tt.days)
			in.WeekStarts, in.Accounts = tt.weekStarts, config.Accounts{{ID: "work"}, {ID: "side"}}
			if got := jsonOf(t, views.NewHistory(in).Weeks); got != tt.want {
				t.Errorf("the weeks are\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestEveryAccountsPeakIsTheMeanOfThoseThatHaveOne(t *testing.T) {
	days := daysFrom("2026-09-28", day("2026-10-05",
		windowsDay{resets: []ledger.Reset{reset("7d", "2026-10-05", "00:30", 0.3)}}.of("side"),
		windowsDay{resets: []ledger.Reset{reset("7d", "2026-10-05", "00:30", 0.6)}}.of("work"),
	))

	// Spare's week isn't known: it's no peak of none.
	want := `{"week":"2026-09-28","accounts":[{"account":"work","peaks":{"7d":0.6}},{"account":"side","peaks":{"7d":0.3}},{"account":"spare"},` +
		`{"all":true,"peaks":{"7d":0.45}}]}`
	if got := jsonOf(t, views.NewHistory(input(days)).Weeks[0]); got != want {
		t.Errorf("the week is\n%s\nwant\n%s", got, want)
	}
}
