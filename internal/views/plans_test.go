package views_test

import (
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
			days: usedToday,
			want: `[{"account":"work","plan":"max20x","price":200,"cost":200,"against":0.02,"worth":4,"unpriced":["claude-opus-9"]},` +
				`{"account":"side","plan":"pro","price":20,"cost":20,"against":0.1,"worth":2},{"account":"spare","worth":1},` +
				`{"all":true,"price":220,"cost":220,"against":0.02727272727272727,"worth":6,"unpriced":["claude-opus-9"]}]`,
		},
		{
			name: "the empty ledger",
			days: []ledger.Summary{day("2026-10-07")},
			want: `[{"account":"work","plan":"max20x","price":200,"cost":200,"against":0},{"account":"side","plan":"pro","price":20,"cost":20,"against":0},` +
				`{"account":"spare"},{"all":true,"price":220,"cost":220,"against":0}]`,
		},
		{
			name:   "at the config's price of a plan",
			days:   usedToday,
			prices: config.Prices{Plans: map[string]float64{"max20x": 160}},
			want: `[{"account":"work","plan":"max20x","price":160,"cost":160,"against":0.025,"worth":4,"unpriced":["claude-opus-9"]},` +
				`{"account":"side","plan":"pro","price":20,"cost":20,"against":0.1,"worth":2},{"account":"spare","worth":1},` +
				`{"all":true,"price":180,"cost":180,"against":0.03333333333333333,"worth":6,"unpriced":["claude-opus-9"]}]`,
		},
		{
			name:     "with no account on a plan known",
			days:     usedToday,
			accounts: config.Accounts{{ID: "work"}, {ID: "side"}},
			want:     `[{"account":"work","worth":4,"unpriced":["claude-opus-9"]},{"account":"side","worth":2},{"all":true}]`,
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
		{name: "before the ledger began, every day asked for", from: "2025-10-07", weekStarts: time.Monday,
			want: "2406.575342465753 240.657534246575 2647.232876712328"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(usedToday)
			in.From, in.WeekStarts = on(tt.from), tt.weekStarts
			plans := views.NewHistory(in).Plans
			got := jsonOf(t, plans[0].Cost) + " " + jsonOf(t, plans[1].Cost) + " " + jsonOf(t, plans[3].Cost)
			if got != tt.want {
				t.Errorf("from %s, the plans cost %s, want %s", tt.from, got, tt.want)
			}
		})
	}
}

func TestAPeriodAPlanPriceChangedInCostsItsPriceOnItsFirstDay(t *testing.T) {
	in := input(usedToday)
	in.From, in.Accounts = on("2026-09-08"), config.Accounts{{ID: "work", Plan: "max20x"}}
	in.Prices.Plans = []ledger.Plan{{ID: "max20x", Name: "Max 20x", Size: 20, Prices: []ledger.PlanPrice{
		{From: "2025-04-09", Monthly: 200 * dollar}, {From: "2026-09-09", Monthly: 300 * dollar},
	}}}

	if got, want := jsonOf(t, views.NewHistory(in).Plans[0].Cost), "200"; got != want {
		t.Errorf("over the last 30 days, a price changed on the second, work's plan cost %s, want %s", got, want)
	}
}
