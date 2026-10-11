package views_test

import (
	"encoding/json"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

func TestHistorysDaysAreTheSummariesPricedWithTheirWorthByFamily(t *testing.T) {
	tests := []struct {
		name   string
		days   []ledger.Summary
		prices config.Prices
		want   string
	}{
		{name: "the empty ledger, today alone", days: daysFrom("2026-10-07"), want: `[{"version":2,"day":"2026-10-07","lines":0}]`},
		{
			name: "one day, the families in the version table's order, every account's together",
			days: daysFrom("2026-10-07", day("2026-10-07",
				account("side", sent("claude-opus-5", 1, inputUsage)),
				account("work", sent("claude-haiku-4-5", 1, inputUsage), sent("claude-opus-5-5", 1, inputUsage), sent("claude-sonnet-5-5", 1, inputUsage)))),
			want: `[{"version":2,"day":"2026-10-07","lines":0,"accounts":[` +
				`{"account":"side","sessions":0,"moved_on":0,"moved_off":0,"models":[` +
				`{"model":"claude-opus-5","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":0,"usage":{"input_tokens":1000000},"worth":5}]},` +
				`{"account":"work","sessions":0,"moved_on":0,"moved_off":0,"models":[` +
				`{"model":"claude-haiku-4-5","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":0,"usage":{"input_tokens":1000000},"worth":1},` +
				`{"model":"claude-opus-5-5","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":0,"usage":{"input_tokens":1000000},"worth":4},` +
				`{"model":"claude-sonnet-5-5","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":0,"usage":{"input_tokens":1000000},"worth":2}]}],` +
				`"by_family":[{"family":"opus","worth":9},{"family":"sonnet","worth":2},{"family":"haiku","worth":1}]}]`,
		},
		{
			name:   "at the config's prices",
			days:   daysFrom("2026-10-07", day("2026-10-07", account("work", sent("claude-opus-5-5", 1, inputUsage)))),
			prices: config.Prices{Models: map[string]config.ModelPrices{"claude-opus-5-5": {Input: new(8.0)}}},
			want: `[{"version":2,"day":"2026-10-07","lines":0,"accounts":[{"account":"work","sessions":0,"moved_on":0,"moved_off":0,"models":[` +
				`{"model":"claude-opus-5-5","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":0,"usage":{"input_tokens":1000000},"worth":8}]}],` +
				`"by_family":[{"family":"opus","worth":8}]}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(tt.days)
			in.Prices = overridden(t, tt.prices)
			if got := jsonOf(t, views.NewHistory(in).Days); got != tt.want {
				t.Errorf("the days are\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestADaysWorthByFamilyNamesWhatItCantPrice(t *testing.T) {
	tests := []struct {
		name  string
		model ledger.ModelDay
		want  string
	}{
		{name: "a model the price table doesn't know, by its id", model: sent("claude-opus-9", 1, inputUsage),
			want: `[{"family":"opus","worth":4,"unpriced":["claude-opus-9"]}]`},
		{name: "a family the version table doesn't know, by what reads it from the id, after those it does", model: sent("claude-quill-1", 1, inputUsage),
			want: `[{"family":"opus","worth":4},{"family":"quill","unpriced":["claude-quill-1"]}]`},
		{name: "requests that went upstream without usage, as no_usage", model: ledger.ModelDay{Model: "claude-opus-5", Upstream: 2, NoUsage: 1, Usage: json.RawMessage(inputUsage)},
			want: `[{"family":"opus","worth":9,"unpriced":["no_usage"]}]`},
		{name: "a count the price table can't price, by its path",
			model: sent("claude-opus-5", 1, `{"input_tokens":1000000,"server_tool_use":{"code_execution_requests":2}}`),
			want:  `[{"family":"opus","worth":9,"unpriced":["server_tool_use.code_execution_requests"]}]`},
		{name: "never a model the table doesn't know that spent nothing", model: ledger.ModelDay{Model: "claude-opus-9", Unsent: 2},
			want: `[{"family":"opus","worth":4}]`},
		{name: "never the requests whose bodies couldn't be read", model: ledger.ModelDay{Unsent: 1},
			want: `[{"family":"opus","worth":4}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days := daysFrom("2026-10-07", day("2026-10-07", account("work", sent("claude-opus-5-5", 1, inputUsage), tt.model)))
			if got := jsonOf(t, views.NewHistory(input(days)).Days[0].ByFamily); got != tt.want {
				t.Errorf("the day's worth by family is\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}
