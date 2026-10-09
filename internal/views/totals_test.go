package views_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// The sessions of the totals' tests.
const (
	sessionA = "aaaa1111"
	sessionB = "bbbb2222"
	sessionC = "cccc3333"
	sessionD = "dddd4444"
	sessionE = "eeee5555"
)

// overFiveDays are the days from Saturday 3 October to today: work's on the
// 3rd, then the 5th to today, a session of its, B, running across days and
// onto side, which is used on the 3rd and today; the 4th of no requests; and
// on the 5th, a request no account answered, of a session of its own.
var overFiveDays = daysFrom("2026-10-03",
	day("2026-10-03",
		ledger.AccountDay{Account: "side", Models: []ledger.ModelDay{sent("claude-sonnet-5-5", 1, inputUsage)},
			Sessions: 1, SessionIDs: []string{sessionB}, MovedOff: 1, MinutesAtCap: new(0), MinutesAtLimit: new(0),
			Resets: []ledger.Reset{reset("7d", "2026-10-03", "09:00", 0.55)}},
		ledger.AccountDay{Account: "work", Models: []ledger.ModelDay{sent("claude-opus-5-5", 2, inputUsage)},
			Sessions: 2, SessionIDs: []string{sessionA, sessionB}, MovedOn: 1, MinutesAtCap: new(10), MinutesAtLimit: new(20),
			Limits: []ledger.Limit{limit("5h", "2026-10-03", "13:00")}, Resets: []ledger.Reset{reset("5h", "2026-10-03", "14:00", 1)}}),
	day("2026-10-05",
		ledger.AccountDay{Models: []ledger.ModelDay{{Model: "claude-opus-5-5", Unsent: 1, Sessions: 1}}, Sessions: 1, SessionIDs: []string{sessionD}},
		ledger.AccountDay{Account: "work", Models: []ledger.ModelDay{sent("claude-haiku-4-5", 3, inputUsage)},
			Sessions: 2, SessionIDs: []string{sessionB, sessionC}, MovedOff: 2, MinutesAtCap: new(5), MinutesAtLimit: new(0),
			Resets: []ledger.Reset{reset("5h", "2026-10-05", "12:00", 0.4)}}),
	day("2026-10-06",
		ledger.AccountDay{Account: "work", Models: []ledger.ModelDay{{Model: "claude-opus-5-5", Upstream: 1, Checks: 4, Usage: []byte(inputUsage)}},
			Sessions: 1, SessionIDs: []string{sessionC}, MinutesAtCap: new(0), MinutesAtLimit: new(0)}),
	day("2026-10-07",
		ledger.AccountDay{Account: "side", Models: []ledger.ModelDay{sent("claude-sonnet-5-5", 1, inputUsage)},
			Sessions: 1, SessionIDs: []string{sessionE}, MinutesAtCap: new(30), MinutesAtLimit: new(0)},
		ledger.AccountDay{Account: "work", Models: []ledger.ModelDay{sent("claude-opus-5-5", 1, inputUsage)},
			Sessions: 1, SessionIDs: []string{sessionC}, MinutesAtCap: new(0), MinutesAtLimit: new(15)}),
)

// beforeAndSince are the days from 6 October, its summary of version 1, which
// named no sessions and read no minutes, to today, on which side's windows
// weren't read.
var beforeAndSince = []ledger.Summary{
	{Version: 1, Day: "2026-10-06", Accounts: []ledger.AccountDay{
		{Account: "side", Models: []ledger.ModelDay{sent("claude-sonnet-5-5", 1, inputUsage)}, Sessions: 1},
		{Account: "work", Models: []ledger.ModelDay{sent("claude-opus-5-5", 2, inputUsage)}, Sessions: 2},
	}},
	day("2026-10-07",
		ledger.AccountDay{Account: "side", Models: []ledger.ModelDay{sent("claude-sonnet-5-5", 1, inputUsage)}, Sessions: 1, SessionIDs: []string{sessionB}},
		ledger.AccountDay{Account: "work", Models: []ledger.ModelDay{sent("claude-opus-5-5", 1, inputUsage)},
			Sessions: 1, SessionIDs: []string{sessionA}, MinutesAtCap: new(10), MinutesAtLimit: new(5)}),
}

func TestTotalsCountEachAccountsDaysAndEveryAccounts(t *testing.T) {
	tests := []struct {
		name string
		days []ledger.Summary
		want string
	}{
		{
			name: "the empty ledger",
			days: daysFrom("2026-10-07"),
			want: `[{"account":"work","requests":0,"sessions":0,"moved_on":0,"moved_off":0,"longest_run":0},` +
				`{"account":"side","requests":0,"sessions":0,"moved_on":0,"moved_off":0,"longest_run":0},` +
				`{"account":"spare","requests":0,"sessions":0,"moved_on":0,"moved_off":0,"longest_run":0},` +
				`{"all":true,"requests":0,"sessions":0,"moved_on":0,"moved_off":0,"longest_run":0}]`,
		},
		{
			// Work's requests are its messages, not its quota checks; its
			// sessions are each counted once, however many days they ran on;
			// its run is broken by the 4th; and what each window had left is
			// at its last reset. Every account's counts session B once,
			// though it ran on two accounts, and the request no account
			// answered, but no minutes of its, as it has no windows, nor
			// a share left at a reset.
			name: "over days and accounts",
			days: overFiveDays,
			want: `[{"account":"work","requests":7,"sessions":3,"moved_on":1,"moved_off":2,"limits":{"5h":1},"minutes_at_cap":15,"minutes_at_limit":35,` +
				`"left_at_reset":{"5h":0.6},"worth":13,"by_model":[{"model":"claude-opus-5-5","share":0.9230769230769231,"worth":12},` +
				`{"model":"claude-haiku-4-5","share":0.07692307692307693,"worth":1}],"busiest_day":"2026-10-05","longest_run":3},` +
				`{"account":"side","requests":2,"sessions":2,"moved_on":0,"moved_off":1,"minutes_at_cap":30,"minutes_at_limit":0,` +
				`"left_at_reset":{"7d":0.45},"worth":4,"by_model":[{"model":"claude-sonnet-5-5","share":1,"worth":4}],"busiest_day":"2026-10-03","longest_run":1},` +
				`{"account":"spare","requests":0,"sessions":0,"moved_on":0,"moved_off":0,"longest_run":0},` +
				`{"all":true,"requests":10,"sessions":5,"moved_on":1,"moved_off":3,"limits":{"5h":1},"minutes_at_cap":45,"minutes_at_limit":35,` +
				`"worth":17,"by_model":[{"model":"claude-opus-5-5","share":0.7058823529411765,"worth":12},` +
				`{"model":"claude-sonnet-5-5","share":0.23529411764705882,"worth":4},{"model":"claude-haiku-4-5","share":0.058823529411764705,"worth":1}],` +
				`"busiest_day":"2026-10-05","longest_run":3}]`,
		},
		{
			// A day of version 1 counts its sessions as distinct, and its
			// minutes as never read: work's are partial, side's, never read
			// on any day, not known.
			name: "a day of a summary of version 1",
			days: beforeAndSince,
			want: `[{"account":"work","requests":3,"sessions":3,"moved_on":0,"moved_off":0,"minutes_at_cap":10,"minutes_at_limit":5,` +
				`"worth":8,"by_model":[{"model":"claude-opus-5-5","share":1,"worth":8}],"busiest_day":"2026-10-06","longest_run":2,` +
				`"partial":["minutes_at_cap","minutes_at_limit"]},` +
				`{"account":"side","requests":2,"sessions":2,"moved_on":0,"moved_off":0,"worth":4,` +
				`"by_model":[{"model":"claude-sonnet-5-5","share":1,"worth":4}],"busiest_day":"2026-10-06","longest_run":2},` +
				`{"account":"spare","requests":0,"sessions":0,"moved_on":0,"moved_off":0,"longest_run":0},` +
				`{"all":true,"requests":5,"sessions":5,"moved_on":0,"moved_off":0,"minutes_at_cap":10,"minutes_at_limit":5,"worth":12,` +
				`"by_model":[{"model":"claude-opus-5-5","share":0.6666666666666666,"worth":8},{"model":"claude-sonnet-5-5","share":0.3333333333333333,"worth":4}],` +
				`"busiest_day":"2026-10-06","longest_run":2,"partial":["minutes_at_cap","minutes_at_limit"]}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jsonOf(t, views.NewHistory(input(tt.days)).Totals.Accounts); got != tt.want {
				t.Errorf("the totals are\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestAModelsShareOfUseIsLeftOutWhereItsWorthIsUnpriced(t *testing.T) {
	days := daysFrom("2026-10-07", day("2026-10-07", account("work",
		sent("claude-opus-5-5", 1, `{"input_tokens":1000000,"server_tool_use":{"code_execution_requests":2}}`),
		sent("claude-opus-9", 1, inputUsage))))

	want := `[{"model":"claude-opus-5-5","share":1,"worth":4,"unpriced":["server_tool_use.code_execution_requests"]},` +
		`{"model":"claude-opus-9","unpriced":["claude-opus-9"]}]`
	if got := jsonOf(t, views.NewHistory(input(days)).Totals.Accounts[0].ByModel); got != want {
		t.Errorf("work's use by model is\n%s\nwant\n%s", got, want)
	}
}
