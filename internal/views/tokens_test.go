package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// acrossTheWeeksEnd are the days from Saturday 3 October to today: work's
// Claude Opus 5.5 on the Saturday, and on the Monday, Claude Haiku 4.5 by
// both its ids, work's and side's, and spare's model the price table doesn't
// know.
var acrossTheWeeksEnd = daysFrom("2026-10-03",
	day("2026-10-03", account("work", sent("claude-opus-5-5", 1, opusUsage))),
	day("2026-10-05",
		account("side", sent("claude-haiku-4-5", 1, inputUsage)),
		account("spare", sent("claude-opus-9", 1, `{"input_tokens":5,"output_tokens":7}`)),
		account("work", sent("claude-haiku-4-5-20251001", 2, inputUsage), ledger.ModelDay{Model: "claude-haiku-4-5", Checks: 1, Counts: 1})),
)

// The rows acrossTheWeeksEnd's weeks give, by Tokens' columns: Saturday's,
// whatever day the week starts on, and the Monday's, its own week's from
// Monday or Sunday.
const (
	saturdaysTokens = `"input":1000000,"output":100000,"cache_write":200000,"cache_read":3000000,"total":4300000,"worth":7.6`
	saturdaysSplits = `"by_account":[{"account":"work",` + saturdaysTokens + `,"by_model":[{"model":"claude-opus-5-5",` + saturdaysTokens + `}]}],` +
		`"by_model":[{"model":"claude-opus-5-5","on":["work"],` + saturdaysTokens + `}]`
	mondaysRows = `"input":2000005,"output":7,"cache_write":0,"cache_read":0,"total":2000012,"worth":2,"unpriced":["claude-opus-9"],` +
		`"cost":50.769230769231,"against":0.03939393939393922,"plans":{"known":2,"of":3},` +
		`"by_account":[{"account":"work","input":1000000,"output":0,"cache_write":0,"cache_read":0,"total":1000000,"worth":1,` +
		`"by_model":[{"model":"claude-haiku-4-5","input":1000000,"output":0,"cache_write":0,"cache_read":0,"total":1000000,"worth":1}]},` +
		`{"account":"side","input":1000000,"output":0,"cache_write":0,"cache_read":0,"total":1000000,"worth":1,` +
		`"by_model":[{"model":"claude-haiku-4-5","input":1000000,"output":0,"cache_write":0,"cache_read":0,"total":1000000,"worth":1}]},` +
		`{"account":"spare","input":5,"output":7,"cache_write":0,"cache_read":0,"total":12,"unpriced":["claude-opus-9"],` +
		`"by_model":[{"model":"claude-opus-9","input":5,"output":7,"cache_write":0,"cache_read":0,"total":12,"unpriced":["claude-opus-9"]}]}],` +
		`"by_model":[{"model":"claude-haiku-4-5","on":["side","work"],"input":2000000,"output":0,"cache_write":0,"cache_read":0,"total":2000000,"worth":2},` +
		`{"model":"claude-opus-9","on":["spare"],"input":5,"output":7,"cache_write":0,"cache_read":0,"total":12,"unpriced":["claude-opus-9"]}]`
)

func TestTokensIsEachWeeksTokensByKindAccountAndModel(t *testing.T) {
	tests := []struct {
		name       string
		days       []ledger.Summary
		weekStarts *time.Weekday
		prices     config.Prices
		want       string
	}{
		{
			// Work's and side's plans, $200 and $20 a month, cost 12/365 of
			// it a day, this week's from the ledger's first day, today, to
			// its end, 5 days, spare's not known.
			name: "the empty ledger: this week from its first day", days: daysFrom("2026-10-07"),
			want: `[{"week":"2026-10-05","input":0,"output":0,"cache_write":0,"cache_read":0,"total":0,"cost":36.164383561644,"against":0,` +
				`"plans":{"known":2,"of":3}}]`,
		},
		{
			// Claude Haiku 4.5's ids are one version, by its alias; a
			// summary's checks and counts add no tokens; against the plans
			// counts work's and side's worth alone; and the week the ledger
			// began in, on the Saturday, costs its 2 days.
			name: "across a week's end, weeks from Monday",
			days: acrossTheWeeksEnd,
			want: `[{"week":"2026-09-28",` + saturdaysTokens + `,"cost":14.465753424658,"against":0.525378787878771,"plans":{"known":2,"of":3},` +
				saturdaysSplits + `},{"week":"2026-10-05",` + mondaysRows + `}]`,
		},
		{
			// The week the ledger began in, from Sunday, costs its last day.
			name:       "weeks from the day the config starts them on",
			days:       acrossTheWeeksEnd,
			weekStarts: new(time.Sunday),
			want: `[{"week":"2026-09-27",` + saturdaysTokens + `,"cost":7.232876712328,"against":1.0507575757576872,"plans":{"known":2,"of":3},` +
				saturdaysSplits + `},{"week":"2026-10-04",` + mondaysRows + `}]`,
		},
		{
			name:   "at the config's price of a plan",
			days:   daysFrom("2026-10-07", day("2026-10-07", account("work", sent("claude-opus-5-5", 1, inputUsage)))),
			prices: config.Prices{Plans: map[string]float64{"max20x": 32, "pro": 20}},
			want: `[{"week":"2026-10-05","input":1000000,"output":0,"cache_write":0,"cache_read":0,"total":1000000,"worth":4,"cost":8.54794520548,` +
				`"against":0.467948717948688,"plans":{"known":2,"of":3},` +
				`"by_account":[{"account":"work","input":1000000,"output":0,"cache_write":0,"cache_read":0,"total":1000000,"worth":4,` +
				`"by_model":[{"model":"claude-opus-5-5","input":1000000,"output":0,"cache_write":0,"cache_read":0,"total":1000000,"worth":4}]}],` +
				`"by_model":[{"model":"claude-opus-5-5","on":["work"],"input":1000000,"output":0,"cache_write":0,"cache_read":0,"total":1000000,"worth":4}]}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(tt.days)
			in.Prices = overridden(t, tt.prices)
			if tt.weekStarts != nil {
				in.WeekStarts = *tt.weekStarts
			}
			if got := jsonOf(t, views.NewHistory(in).Tokens); got != tt.want {
				t.Errorf("the weeks are\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

// A model whose days hold Claude Code's quota checks or counts of tokens
// alone wasn't used: no list of the models, versions or families used names
// it, nor of the accounts that used them an account that sent them alone,
// and no count of the versions counts it. Today, work used Opus 5.5 and
// checked its quota with Haiku 4.5, and spare only checked its.
func TestAModelOfQuotaChecksAloneIsListedNowhere(t *testing.T) {
	checks := ledger.ModelDay{Model: "claude-haiku-4-5", Checks: 2, Counts: 1}
	days := daysFrom("2026-10-07", day("2026-10-07",
		account("spare", checks), account("work", sent("claude-opus-5-5", 1, inputUsage), checks)))
	in := input(days)
	history, past := views.NewHistory(in), views.NewPast(in, days)
	tokens := past.Tokens(views.TokensAsk{}).Periods[0]
	totals := history.Totals.Accounts

	got := map[string][]string{
		"Tokens' models":            modelsOf(tokens.ByModel),
		"Tokens' accounts":          accountsOf(tokens.ByAccount),
		"history's tokens' models":  modelsOf(history.Tokens[0].ByModel),
		"work's models by share":    sharedModelsOf(totals[0].ByModel),
		"every account's by share":  sharedModelsOf(totals[len(totals)-1].ByModel),
		"the day's families":        familiesOf(history.Days[0].ByFamily),
		"the month's families":      familiesOf(history.Months[0].ByFamily),
		"the Year's versions":       past.Versions(),
		"the Year's versions, by v": past.Year(views.YearAsk{}).Versions,
	}
	want := map[string][]string{
		"Tokens' models": {"claude-opus-5-5"}, "Tokens' accounts": {"work"}, "history's tokens' models": {"claude-opus-5-5"},
		"work's models by share": {"claude-opus-5-5"}, "every account's by share": {"claude-opus-5-5"},
		"the day's families": {"opus"}, "the month's families": {"opus"},
		"the Year's versions": {"Opus 5.5"}, "the Year's versions, by v": {"Opus 5.5"},
	}
	for list, w := range want {
		if !slices.Equal(got[list], w) {
			t.Errorf("%s are %q, want %q", list, got[list], w)
		}
	}
	if tokens.Versions != 1 {
		t.Errorf("Tokens counts %d models, want 1", tokens.Versions)
	}
}

// modelsOf are the models of the rows given, by their ids.
func modelsOf(rows []views.ModelTokens) []string {
	var models []string
	for _, r := range rows {
		models = append(models, r.Model)
	}
	return models
}

// accountsOf are the accounts of the rows given, by their ids.
func accountsOf(rows []views.AccountTokens) []string {
	var accounts []string
	for _, r := range rows {
		accounts = append(accounts, r.Account)
	}
	return accounts
}

// sharedModelsOf are the models of the shares given, by their ids.
func sharedModelsOf(shares []views.ModelShare) []string {
	var models []string
	for _, s := range shares {
		models = append(models, s.Model)
	}
	return models
}

// familiesOf are the families of the worths given, by their names.
func familiesOf(worths []views.FamilyWorth) []string {
	var families []string
	for _, w := range worths {
		families = append(families, w.Family)
	}
	return families
}

func TestTokensListNoAccountWhoseDaysHoldNoModels(t *testing.T) {
	// Side's windows were read today, but it wasn't used.
	days := daysFrom("2026-10-07", day("2026-10-07",
		windowsDay{highest: map[string]float64{"5h": 0.2}}.of("side"),
		account("work", sent("claude-opus-5-5", 1, inputUsage))))

	var listed []string
	for _, a := range views.NewHistory(input(days)).Tokens[0].ByAccount {
		listed = append(listed, a.Account)
	}
	if want := []string{"work"}; !slices.Equal(listed, want) {
		t.Errorf("this week's tokens are by the accounts %q, want %q", listed, want)
	}
}
