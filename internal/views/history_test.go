package views_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
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
		{name: "the empty ledger, today alone", days: daysFrom("2026-10-07"), want: `[{"day":"2026-10-07"}]`},
		{
			name: "one day, the families in the version table's order, every account's together",
			days: daysFrom("2026-10-07", day("2026-10-07",
				account("side", sent("claude-opus-5", 1, inputUsage)),
				account("work", sent("claude-haiku-4-5", 1, inputUsage), sent("claude-opus-5-5", 1, inputUsage), sent("claude-sonnet-5-5", 1, inputUsage)))),
			want: `[{"day":"2026-10-07","accounts":[` +
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
			want: `[{"day":"2026-10-07","accounts":[{"account":"work","sessions":0,"moved_on":0,"moved_off":0,"models":[` +
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

// A day gives every field its type and the summary's have, one added since
// among them, but for what the summary keeps to know when to summarise the
// day again; and so does each account's day.
func TestHistorysDaysGiveEveryFieldButWhatTheSummaryKeepsForItself(t *testing.T) {
	work := account("work", sent("claude-opus-5-5", 1, inputUsage))
	work.Pool, work.Sessions, work.SessionIDs, work.MovedOn, work.MovedOff = "b2n8", 1, []string{"s1"}, 1, 1
	work.Pools = []ledger.PoolDay{{Pool: "b2n8", Models: []ledger.ModelDay{sent("claude-opus-5-5", 1, inputUsage)}, Sessions: 1}}
	work.Highest, work.Rise = map[string]float64{"7d": 0.4}, map[string]float64{"7d": 0.1}
	work.Resets = []ledger.Reset{{Window: "7d", At: onAt("2026-10-07", "10:00"), Before: 0.3}}
	work.Limits = []ledger.Limit{{Window: "7d", At: onAt("2026-10-07", "11:00"), ResetsAt: onAt("2026-10-08", "10:00")}}
	work.MinutesAtCap, work.MinutesAtLimit, work.ReadBefore = new(5), new(3), []string{"7d"}
	summary := day("2026-10-07", work)
	summary.Version, summary.Lines, summary.Bytes = 7, 12, ledger.Bytes{Plain: 3000, Compressed: 400}
	in := input(daysFrom("2026-10-07", summary))
	in.PoolLabel = func(string) string { return "Default" }

	var days []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonOf(t, views.NewHistory(in).Days)), &days); err != nil || len(days) == 0 {
		t.Fatalf("the days don't read as JSON objects: %v", err)
	}
	checkFields(t, "the day", days[0], reflect.TypeFor[views.Day](), "version", "lines", "bytes")
	var accounts []map[string]json.RawMessage
	if err := json.Unmarshal(days[0]["accounts"], &accounts); err != nil || len(accounts) != 1 {
		t.Fatalf("the day's accounts are %s (%v), want work's day", days[0]["accounts"], err)
	}
	checkFields(t, "work's day", accounts[0], reflect.TypeFor[ledger.PricedAccount](), "read_before")
}

// checkFields checks object, a value of type of as JSON gives it, gives
// every field of's fields are given as, those of the structs it embeds
// among them, but those left out, and none of those.
func checkFields(t *testing.T, what string, object map[string]json.RawMessage, of reflect.Type, leftOut ...string) {
	t.Helper()
	for _, f := range reflect.VisibleFields(of) {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || name == "" || name == "-" {
			continue
		}
		if _, given := object[name]; given == slices.Contains(leftOut, name) {
			t.Errorf("%s gives %s %t, want %t", what, name, given, !given)
		}
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
