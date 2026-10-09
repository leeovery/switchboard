package views_test

import (
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
		`"cost":50.769230769231,"against":0.03939393939393922,` +
		`"by_account":[{"account":"side","input":1000000,"output":0,"cache_write":0,"cache_read":0,"total":1000000,"worth":1,` +
		`"by_model":[{"model":"claude-haiku-4-5","input":1000000,"output":0,"cache_write":0,"cache_read":0,"total":1000000,"worth":1}]},` +
		`{"account":"spare","input":5,"output":7,"cache_write":0,"cache_read":0,"total":12,"unpriced":["claude-opus-9"],` +
		`"by_model":[{"model":"claude-opus-9","input":5,"output":7,"cache_write":0,"cache_read":0,"total":12,"unpriced":["claude-opus-9"]}]},` +
		`{"account":"work","input":1000000,"output":0,"cache_write":0,"cache_read":0,"total":1000000,"worth":1,` +
		`"by_model":[{"model":"claude-haiku-4-5","input":1000000,"output":0,"cache_write":0,"cache_read":0,"total":1000000,"worth":1}]}],` +
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
			// Work's and side's plans, $200 and $20 a month, cost 12/52 of
			// it a week, this week's whole, spare's not known.
			name: "the empty ledger: this week, whole", days: daysFrom("2026-10-07"),
			want: `[{"week":"2026-10-05","input":0,"output":0,"cache_write":0,"cache_read":0,"total":0,"cost":50.769230769231,"against":0}]`,
		},
		{
			// Claude Haiku 4.5's ids are one version, by its alias; checks'
			// and counts' tokens are among the week's, as ccusage counts
			// them; and against the plans counts work's and side's worth
			// alone.
			name: "across a week's end, weeks from Monday",
			days: acrossTheWeeksEnd,
			want: `[{"week":"2026-09-28",` + saturdaysTokens + `,"cost":50.769230769231,"against":0.14969696969696902,` + saturdaysSplits + `},` +
				`{"week":"2026-10-05",` + mondaysRows + `}]`,
		},
		{
			name:       "weeks from the day the config starts them on",
			days:       acrossTheWeeksEnd,
			weekStarts: new(time.Sunday),
			want: `[{"week":"2026-09-27",` + saturdaysTokens + `,"cost":50.769230769231,"against":0.14969696969696902,` + saturdaysSplits + `},` +
				`{"week":"2026-10-04",` + mondaysRows + `}]`,
		},
		{
			name:   "at the config's price of a plan",
			days:   daysFrom("2026-10-07", day("2026-10-07", account("work", sent("claude-opus-5-5", 1, inputUsage)))),
			prices: config.Prices{Plans: map[string]float64{"max20x": 32, "pro": 20}},
			want: `[{"week":"2026-10-05","input":1000000,"output":0,"cache_write":0,"cache_read":0,"total":1000000,"worth":4,"cost":12,"against":0.3333333333333333,` +
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
