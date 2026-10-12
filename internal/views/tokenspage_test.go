package views_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// usedAcrossTwoMonths are the days from Monday 28 September to today, used on
// Tuesday 29 September, Saturday 3 October, Monday 5 October and today, the
// 3rd by two versions, Claude Opus 5.5 and Claude Haiku 4.5, the rest by
// Claude Opus 5.5.
var usedAcrossTwoMonths = daysFrom("2026-09-28",
	day("2026-09-29", account("work", sent("claude-opus-5-5", 1, inputUsage))),
	day("2026-10-03", account("work", sent("claude-opus-5-5", 1, inputUsage), sent("claude-haiku-4-5", 1, inputUsage))),
	day("2026-10-05", account("side", sent("claude-opus-5-5", 1, inputUsage))),
	day("2026-10-07", account("work", sent("claude-opus-5-5", 1, inputUsage))),
)

// Tokens' periods are by day, by week or by month, newest first, those with
// nothing recorded in them left out, each named by its first day, or its
// month, with how many versions it used; and the one today is in is so far.
func TestTokensIsEachPeriodNewestFirstLeavingOutThoseWithNothingRecorded(t *testing.T) {
	tests := []struct {
		name     string
		period   views.TokensPeriod
		first    []string
		versions []int
		soFar    []bool
	}{
		{
			name: "by day", period: views.TokensByDay,
			first: []string{"2026-10-07", "2026-10-05", "2026-10-03", "2026-09-29"}, versions: []int{1, 1, 2, 1}, soFar: []bool{true, false, false, false},
		},
		{
			name: "by week, as it opens", period: views.TokensByWeek,
			first: []string{"2026-10-05", "2026-09-28"}, versions: []int{1, 2}, soFar: []bool{true, false},
		},
		{
			name: "by month", period: views.TokensByMonth,
			first: []string{"2026-10", "2026-09"}, versions: []int{2, 1}, soFar: []bool{true, false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := views.NewPast(input(usedAcrossTwoMonths), usedAcrossTwoMonths).Tokens(views.TokensAsk{Period: tt.period})
			var first []string
			var versions []int
			var soFar []bool
			for _, p := range page.Periods {
				first, versions, soFar = append(first, p.First), append(versions, p.Versions), append(soFar, p.SoFar)
			}
			if !slices.Equal(first, tt.first) || !slices.Equal(versions, tt.versions) || !slices.Equal(soFar, tt.soFar) {
				t.Errorf("the periods are %q, of %v versions, so far %v; want %q, of %v, so far %v", first, versions, soFar, tt.first, tt.versions, tt.soFar)
			}
		})
	}
}

// A period with nothing recorded in it never shows, and today's shows only
// once something is: a day used last month and not since shows that month
// alone, not so far.
func TestTokensLeavesOutTheRunningPeriodWithNothingRecordedYet(t *testing.T) {
	days := daysFrom("2026-09-28", day("2026-09-29", account("work", sent("claude-opus-5-5", 1, inputUsage))))
	page := views.NewPast(input(days), days).Tokens(views.TokensAsk{Period: views.TokensByMonth})
	if len(page.Periods) != 1 || page.Periods[0].First != "2026-09" || page.Periods[0].SoFar {
		t.Errorf("the periods are %+v, want September's alone, not so far", page.Periods)
	}
}

// A day costs its plans 12/365 of a month, a week 12/52, and a calendar month
// a month, each as of its first day, the period so far priced whole: work's
// $200 and side's $20, spare's plan not known. One the days were asked for
// from part-way through costs its days alone; one the ledger began part-way
// through, whole.
func TestTokensPricesEachPeriodAgainstItsShareOfAMonth(t *testing.T) {
	tests := []struct {
		name   string
		period views.TokensPeriod
		from   string
		costs  string
	}{
		{name: "by day", period: views.TokensByDay, from: "2026-01-01", costs: "7.232876712328 7.232876712328 7.232876712328 7.232876712328"},
		{name: "by week", period: views.TokensByWeek, from: "2026-01-01", costs: "50.769230769231 50.769230769231"},
		{name: "by month, the ledger begun part-way through September", period: views.TokensByMonth, from: "2026-01-01", costs: "220 220"},
		{name: "by month, the days asked for from the 29th", period: views.TokensByMonth, from: "2026-09-29", costs: "220 14.465753424658"},
		{name: "by week, the days asked for from the 29th", period: views.TokensByWeek, from: "2026-09-29", costs: "50.769230769231 43.397260273973"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(usedAcrossTwoMonths)
			in.From = on(tt.from)
			asked := usedAcrossTwoMonths
			if i := slices.IndexFunc(asked, func(s ledger.Summary) bool { return s.Day >= tt.from }); i > 0 {
				asked = asked[i:]
			}
			var costs []string
			for _, p := range views.NewPast(in, asked).Tokens(views.TokensAsk{Period: tt.period}).Periods {
				costs = append(costs, jsonOf(t, p.Cost))
			}
			if got := strings.Join(costs, " "); got != tt.costs {
				t.Errorf("the periods cost %s, want %s", got, tt.costs)
			}
		})
	}
}

// Tokens' accounts are in the config's order, those it doesn't configure
// after, by id, the requests no account answered among them.
func TestTokensListsTheAccountsInTheConfigsOrder(t *testing.T) {
	days := daysFrom("2026-10-07", day("2026-10-07",
		account("", sent("claude-opus-5-5", 1, "")),
		account("extra", sent("claude-opus-5-5", 1, inputUsage)),
		account("side", sent("claude-opus-5-5", 1, inputUsage)),
		account("work", sent("claude-opus-5-5", 1, inputUsage))))

	var listed []string
	for _, a := range views.NewPast(input(days), days).Tokens(views.TokensAsk{}).Periods[0].ByAccount {
		listed = append(listed, a.Account)
	}
	if want := []string{"work", "side", "", "extra"}; !slices.Equal(listed, want) {
		t.Errorf("today's tokens are by the accounts %q, want %q", listed, want)
	}
}

// Each of Tokens' accounts carries what the dashboard calls it: the config's
// label for it, else its id, as for one it doesn't configure, and nothing
// for the requests none answered.
func TestTokensLabelsEachAccountAsTheConfigDoes(t *testing.T) {
	days := daysFrom("2026-10-07", day("2026-10-07",
		account("", sent("claude-opus-5-5", 1, "")),
		account("extra", sent("claude-opus-5-5", 1, inputUsage)),
		account("side", sent("claude-opus-5-5", 1, inputUsage)),
		account("work", sent("claude-opus-5-5", 1, inputUsage))))
	in := input(days)
	in.Accounts = config.Accounts{{ID: "work", Label: "Work laptop"}, {ID: "side"}}

	var labels []string
	for _, a := range views.NewPast(in, days).Tokens(views.TokensAsk{}).Periods[0].ByAccount {
		labels = append(labels, a.Label)
	}
	if want := []string{"Work laptop", "side", "", "extra"}; !slices.Equal(labels, want) {
		t.Errorf("today's accounts are labelled %q, want %q", labels, want)
	}
}

// A version's accounts are the one it was worth most on first, then by id.
func TestAVersionsAccountsAreTheOneItWasWorthMostOnFirst(t *testing.T) {
	twice := `{"input_tokens":2000000}`
	days := daysFrom("2026-10-07", day("2026-10-07",
		account("extra", sent("claude-haiku-4-5", 1, inputUsage)),
		account("side", sent("claude-haiku-4-5", 1, inputUsage)),
		account("work", sent("claude-haiku-4-5", 1, twice))))

	on := views.NewPast(input(days), days).Tokens(views.TokensAsk{}).Periods[0].ByModel[0].On
	if want := []string{"work", "extra", "side"}; !slices.Equal(on, want) {
		t.Errorf("Claude Haiku 4.5 ran on %q, want %q", on, want)
	}
}

// With nothing recorded, Tokens has no periods, and its prices are as of the
// price table's date.
func TestTokensOfNothingRecordedHasNoPeriods(t *testing.T) {
	days := daysFrom("2026-10-01")
	page := views.NewPast(input(days), days).Tokens(views.TokensAsk{})
	if len(page.Periods) != 0 || page.PricesAsOf != ledger.Pricing.AsOf {
		t.Errorf("Tokens is %+v, want no periods, its prices as of %s", page, ledger.Pricing.AsOf)
	}
}
