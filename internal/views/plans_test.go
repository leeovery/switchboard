package views_test

import (
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// usedToday are the days of the plans' tests: today's alone, work's Claude
// Opus 5.5 and a model the price table doesn't know, side's Claude Sonnet
// 5.5, and spare's Claude Haiku 4.5.
var usedToday = []ledger.Summary{day("2026-10-07",
	account("side", sent("claude-sonnet-5-5", 1, inputUsage)),
	account("spare", sent("claude-haiku-4-5", 1, inputUsage)),
	account("work", sent("claude-opus-5-5", 1, inputUsage), sent("claude-opus-9", 1, inputUsage)),
)}

// sinceJuly are the days of a ledger begun on 1 July, of no requests but
// usedToday's.
var sinceJuly = daysFrom("2026-07-01", usedToday...)

func TestPlansAreEachAccountsPlanAndTheWorthAgainstIt(t *testing.T) {
	tests := []struct {
		name     string
		days     []ledger.Summary
		accounts config.Accounts
		prices   config.Prices
		want     string
	}{
		{
			// Spare's plan isn't known, so it has its worth alone, and every
			// account's sums work's and side's: (4 + 2) / 220.
			name: "over the last 30 days, a month",
			days: sinceJuly,
			want: `[{"account":"work","plan":"max20x","price":200,"cost":200,"against":0.02,"worth":4,"unpriced":["claude-opus-9"]},` +
				`{"account":"side","plan":"pro","price":20,"cost":20,"against":0.1,"worth":2},{"account":"spare","worth":1},` +
				`{"all":true,"price":220,"cost":220,"against":0.02727272727272727,"plans":{"known":2,"of":3},"worth":6,"unpriced":["claude-opus-9"]}]`,
		},
		{
			// Begun today, the ledger prices the plans from today, a day's
			// worth.
			name: "the empty ledger",
			days: []ledger.Summary{day("2026-10-07")},
			want: `[{"account":"work","plan":"max20x","price":200,"cost":6.575342465753,"against":0},` +
				`{"account":"side","plan":"pro","price":20,"cost":0.657534246575,"against":0},` +
				`{"account":"spare"},{"all":true,"price":220,"cost":7.232876712328,"against":0,"plans":{"known":2,"of":3}}]`,
		},
		{
			name:   "at the config's price of a plan",
			days:   sinceJuly,
			prices: config.Prices{Plans: map[string]float64{"max20x": 160}},
			want: `[{"account":"work","plan":"max20x","price":160,"cost":160,"against":0.025,"worth":4,"unpriced":["claude-opus-9"]},` +
				`{"account":"side","plan":"pro","price":20,"cost":20,"against":0.1,"worth":2},{"account":"spare","worth":1},` +
				`{"all":true,"price":180,"cost":180,"against":0.03333333333333333,"plans":{"known":2,"of":3},"worth":6,"unpriced":["claude-opus-9"]}]`,
		},
		{
			name:     "with no account on a plan known",
			days:     sinceJuly,
			accounts: config.Accounts{{ID: "work"}, {ID: "side"}},
			want:     `[{"account":"work","worth":4,"unpriced":["claude-opus-9"]},{"account":"side","worth":2},{"all":true,"plans":{"known":0,"of":2}}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(tt.days)
			in.From, in.Prices = on("2026-09-08"), overridden(t, tt.prices)
			if tt.accounts != nil {
				in.Accounts = tt.accounts
			}
			if got := jsonOf(t, views.NewHistory(in).Plans); got != tt.want {
				t.Errorf("the plans are\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestAPlansCostIsProratedAsThePeriodAskedFor(t *testing.T) {
	tests := []struct {
		name       string
		from       string
		weekStarts time.Weekday
		// want is what work's Max 20x and side's Pro cost, and every
		// account's together.
		want string
	}{
		{name: "today, a day at 12/365 of a month", from: "2026-10-07", weekStarts: time.Monday,
			want: "6.575342465753 0.657534246575 7.232876712328"},
		{name: "this week, at 12/52 of a month, whole", from: "2026-10-05", weekStarts: time.Monday,
			want: "46.153846153846 4.615384615385 50.769230769231"},
		{name: "this week, from the day weeks start on", from: "2026-10-04", weekStarts: time.Sunday,
			want: "46.153846153846 4.615384615385 50.769230769231"},
		{name: "the last 30 days, a month", from: "2026-09-08", weekStarts: time.Monday,
			want: "200 20 220"},
		{name: "the last 12 weeks, twelve weeks'", from: "2026-07-20", weekStarts: time.Monday,
			want: "553.846153846154 55.384615384615 609.230769230769"},
		{name: "the last 12 weeks, from the day weeks start on", from: "2026-07-19", weekStarts: time.Sunday,
			want: "553.846153846154 55.384615384615 609.230769230769"},
		{name: "any other days, a day each", from: "2026-10-01", weekStarts: time.Monday,
			want: "46.027397260274 4.602739726027 50.630136986301"},
		{name: "a week from another day than weeks start on, a day each", from: "2026-10-05", weekStarts: time.Sunday,
			want: "19.72602739726 1.972602739726 21.698630136986"},
		{name: "from before the ledger began, each day from its first", from: "2025-10-07", weekStarts: time.Monday,
			want: "650.958904109589 65.095890410959 716.054794520548"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(sinceJuly)
			in.From, in.WeekStarts = on(tt.from), tt.weekStarts
			plans := views.NewHistory(in).Plans
			got := jsonOf(t, plans[0].Cost) + " " + jsonOf(t, plans[1].Cost) + " " + jsonOf(t, plans[3].Cost)
			if got != tt.want {
				t.Errorf("from %s, the plans cost %s, want %s", tt.from, got, tt.want)
			}
		})
	}
}

// On the week's first day, today and this week begin together: the days
// asked for from it are the longer, this week, Accounts' own to begin with,
// priced a week, as Accounts' this week and the tokens' row of it are.
// Accounts' today, priced a day, is the one period history can't be asked
// for by its first day.
func TestOnTheWeeksFirstDayTheDaysAskedForFromItAreThisWeek(t *testing.T) {
	monday := on("2026-10-05").Add(15 * time.Hour)
	days := daysTo("2026-10-01", monday, day("2026-10-05", account("work", sent("claude-opus-5-5", 1, inputUsage))))
	in := input(days)
	in.From, in.Now = on("2026-10-05"), monday
	history := views.NewHistory(in)
	past := views.NewPast(in, days)
	got := map[string]string{
		"history's plans":     jsonOf(t, history.Plans[len(history.Plans)-1].Cost),
		"its tokens' row":     jsonOf(t, history.Tokens[len(history.Tokens)-1].Cost),
		"Accounts' this week": jsonOf(t, past.Accounts(views.AccountsThisWeek).All.Plan.Cost),
		"Accounts' today":     jsonOf(t, past.Accounts(views.AccountsToday).All.Plan.Cost),
	}
	want := map[string]string{
		"history's plans": "50.769230769231", "its tokens' row": "50.769230769231",
		"Accounts' this week": "50.769230769231", "Accounts' today": "7.232876712328",
	}
	if !maps.Equal(got, want) {
		t.Errorf("asked for from the week's first day, on it, the plans cost %v, want %v", got, want)
	}
}

func TestAPeriodAPlanPriceChangedInCostsItsPriceOnItsFirstDay(t *testing.T) {
	in := input(sinceJuly)
	in.From, in.Accounts = on("2026-09-08"), config.Accounts{{ID: "work", Plan: "max20x"}}
	in.Prices.Plans = []ledger.Plan{{ID: "max20x", Name: "Max 20x", Size: 20, Prices: []ledger.PlanPrice{
		{From: "2025-04-09", Monthly: 200 * dollar}, {From: "2026-09-09", Monthly: 300 * dollar},
	}}}

	if got, want := jsonOf(t, views.NewHistory(in).Plans[0].Cost), "200"; got != want {
		t.Errorf("over the last 30 days, a price changed on the second, work's plan cost %s, want %s", got, want)
	}
}

func TestAMonthOrWeekTheDaysAskedForBeginPartWayThroughCostsItsDays(t *testing.T) {
	tests := []struct {
		name string
		from string
		// months and weeks are what BY MONTH's months and Tokens' weeks
		// cost, work's Max 20x and side's Pro together.
		months, weeks string
	}{
		{
			// September from the 29th is 2 days at 12/365 of a month, the
			// week of the 28th 6; October and this week are whole.
			name: "from a Tuesday at September's end", from: "2026-09-29",
			months: "14.465753424658 220", weeks: "43.397260273973 50.769230769231",
		},
		{
			// October is whole, from its first day; the week of the 28th
			// from the Thursday is 4 days.
			name: "from the first of the month", from: "2026-10-01",
			months: "220", weeks: "28.931506849315 50.769230769231",
		},
		{
			// A month or week still running is priced to its end: October
			// from the 6th is 26 days, this week from the Tuesday 6.
			name: "from part-way through this week and month", from: "2026-10-06",
			months: "188.054794520548", weeks: "43.397260273973",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(daysFrom(tt.from))
			in.From = on(tt.from)
			history := views.NewHistory(in)
			var months, weeks []string
			for _, m := range history.Months {
				months = append(months, jsonOf(t, m.Cost))
			}
			for _, w := range history.Tokens {
				weeks = append(weeks, jsonOf(t, w.Cost))
			}
			if got := strings.Join(months, " "); got != tt.months {
				t.Errorf("from %s, the months cost %s, want %s", tt.from, got, tt.months)
			}
			if got := strings.Join(weeks, " "); got != tt.weeks {
				t.Errorf("from %s, the weeks cost %s, want %s", tt.from, got, tt.weeks)
			}
		})
	}
}
