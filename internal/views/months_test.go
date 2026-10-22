package views_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// limitedDay is an account's day, of the models' days given, on which it
// reached the limits of the windows given.
func limitedDay(id string, windows []string, models ...ledger.ModelDay) ledger.AccountDay {
	a := account(id, models...)
	for _, w := range windows {
		a.Limits = append(a.Limits, ledger.Limit{Window: w, At: now})
	}
	return a
}

// acrossTheMonthsEnd are the days from 29 September, the ledger's first, to
// today: work's, side's and spare's requests in September, work reaching its
// 5-hour limit, and in October, side reaching both its windows', and an
// unpriced model and requests without usage.
var acrossTheMonthsEnd = daysFrom("2026-09-29",
	day("2026-09-29",
		limitedDay("work", []string{"5h"}, sent("claude-opus-5-5", 1, opusUsage)),
		account("side", sent("claude-sonnet-5-5", 1, inputUsage))),
	day("2026-09-30", account("spare", sent("claude-opus-5-5", 1, cacheHourUsage))),
	day("2026-10-01", limitedDay("side", []string{"5h", "7d"}, sent("claude-haiku-4-5", 3, inputUsage))),
	day("2026-10-07",
		account("", ledger.ModelDay{Model: "claude-opus-5-5", Unsent: 1}),
		account("work", sent("claude-opus-9", 1, inputUsage), ledger.ModelDay{Model: "claude-opus-5-5", Upstream: 2, NoUsage: 1, Checks: 4, Counts: 6,
			Usage: []byte(inputUsage)})),
)

func TestByMonthIsEachMonthsRequestsWorthAndLimitsAgainstThePlans(t *testing.T) {
	tests := []struct {
		name     string
		days     []ledger.Summary
		accounts config.Accounts
		prices   config.Prices
		want     string
	}{
		{
			// Begun today, the ledger prices the month so far from today to
			// its end, 25 days at 12/365 of a month, with no ratio, two of
			// the three accounts' plans known.
			name: "the empty ledger: the month so far from its first day, and no ratio", days: daysFrom("2026-10-07"),
			want: `[{"month":"2026-10","requests":0,"cost":180.82191780822,"plans":{"known":2,"of":3}}]`,
		},
		{name: "one day", days: daysFrom("2026-10-07", day("2026-10-07", account("work", sent("claude-opus-5-5", 2, inputUsage)))),
			want: `[{"month":"2026-10","requests":2,"worth":4,"cost":180.82191780822,"plans":{"known":2,"of":3},"by_family":[{"family":"opus","worth":4}]}]`},
		{
			// September, which the ledger began part-way through, costs its
			// plans from the ledger's first day, the 29th, 2 days at 12/365
			// of a month, and spare, on no plan known, adds nothing to the
			// cost or the worth against it: (7.60 + 2) / 14.47. Of October,
			// whole, so far, the requests are messages alone, and the worth
			// names what it can't price.
			name: "across a month's end",
			days: acrossTheMonthsEnd,
			want: `[{"month":"2026-09","requests":3,"worth":17.6,"cost":14.465753424658,"against":0.6636363636363423,"plans":{"known":2,"of":3},` +
				`"limits":{"5h":1},"by_family":[{"family":"opus","worth":15.6},{"family":"sonnet","worth":2}]},` +
				`{"month":"2026-10","requests":7,"worth":5,"unpriced":["claude-opus-9","no_usage"],"cost":220,"plans":{"known":2,"of":3},"limits":{"5h":1,"7d":1},` +
				`"by_family":[{"family":"opus","worth":4,"unpriced":["claude-opus-9","no_usage"]},{"family":"haiku","worth":1}]}]`,
		},
		{
			name:   "at the config's price of a plan",
			days:   daysFrom("2026-09-30", day("2026-09-30", account("work", sent("claude-opus-5-5", 1, inputUsage)))),
			prices: config.Prices{Plans: map[string]float64{"max20x": 150}},
			want: `[{"month":"2026-09","requests":1,"worth":4,"cost":5.58904109589,"against":0.7156862745098566,"plans":{"known":2,"of":3},` +
				`"by_family":[{"family":"opus","worth":4}]},{"month":"2026-10","requests":0,"cost":170,"plans":{"known":2,"of":3}}]`,
		},
		{
			name:     "with no account on a plan known, no cost",
			days:     daysFrom("2026-09-30", day("2026-09-30", account("work", sent("claude-opus-5-5", 1, inputUsage)))),
			accounts: config.Accounts{{ID: "work"}},
			want: `[{"month":"2026-09","requests":1,"worth":4,"plans":{"known":0,"of":1},"by_family":[{"family":"opus","worth":4}]},` +
				`{"month":"2026-10","requests":0,"plans":{"known":0,"of":1}}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(tt.days)
			in.Prices = overridden(t, tt.prices)
			if tt.accounts != nil {
				in.Accounts = tt.accounts
			}
			if got := jsonOf(t, views.NewHistory(in).Months); got != tt.want {
				t.Errorf("the months are\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestAMonthAPlanPriceChangedInCostsItsPriceOnItsFirstDay(t *testing.T) {
	tests := []struct {
		name, began, want string
	}{
		{name: "each month whole, at its first day's price", began: "2026-09-01",
			want: `[{"month":"2026-09","requests":0,"cost":200,"against":0,"plans":{"known":1,"of":1}},` +
				`{"month":"2026-10","requests":0,"cost":250,"plans":{"known":1,"of":1}}]`},
		{
			// September from the 20th is 11 days at 12/365 of a month, at
			// the price of the ledger's first day.
			name: "the month the ledger began in, at its first day's price", began: "2026-09-20",
			want: `[{"month":"2026-09","requests":0,"cost":90.41095890411,"against":0,"plans":{"known":1,"of":1}},` +
				`{"month":"2026-10","requests":0,"cost":250,"plans":{"known":1,"of":1}}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(daysFrom(tt.began))
			in.Accounts = config.Accounts{{ID: "work", Plan: "max20x"}}
			in.Prices.Plans = []ledger.Plan{{ID: "max20x", Name: "Max 20x", Size: 20, Prices: []ledger.PlanPrice{
				{From: "2025-04-09", Monthly: 200 * ledger.Dollar}, {From: "2026-09-15", Monthly: 250 * ledger.Dollar}, {From: "2026-10-02", Monthly: 300 * ledger.Dollar},
			}}}
			if got := jsonOf(t, views.NewHistory(in).Months); got != tt.want {
				t.Errorf("the months are\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}
