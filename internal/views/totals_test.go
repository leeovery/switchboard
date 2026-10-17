package views_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

func TestTheLimitsHitAreSaidByWindowTheWeekFirst(t *testing.T) {
	limits := map[string]int{"7d": 1, "5h": 1240, "7d_oi": 3}
	got := views.LimitsHit(limits, status.NameOfKey)
	if want := []string{"week ×1", "5-hour ×1,240", "Fable week ×3"}; !slices.Equal(got, want) {
		t.Errorf("LimitsHit(%v) = %q, want %q", limits, got, want)
	}
	if got := views.LimitsHit(nil, status.NameOfKey); len(got) != 0 {
		t.Errorf("LimitsHit(none) = %q, want none", got)
	}
}

// The limits reached are counted every window's together, of an account's
// totals and of a month alike, and the time at a cap or a limit is the two
// together, known only where both are.
func TestTheLimitsAreCountedAndTheTimeAtACapOrLimitIsTheTwoTogether(t *testing.T) {
	limits := map[string]int{"5h": 3, "7d": 1}
	if got := (views.AccountTotals{Limits: limits}).LimitCount(); got != 4 {
		t.Errorf("an account's limits count %d, want 4", got)
	}
	if got := (views.Month{Limits: limits}).LimitCount(); got != 4 {
		t.Errorf("a month's limits count %d, want 4", got)
	}
	if got := (views.AccountTotals{}).LimitCount(); got != 0 {
		t.Errorf("no limits count %d, want 0", got)
	}
	tests := []struct {
		name           string
		atCap, atLimit *int
		want           int
		known          bool
	}{
		{name: "both read", atCap: new(40), atLimit: new(12), want: 52, known: true},
		{name: "both read, none spent", atCap: new(0), atLimit: new(0), want: 0, known: true},
		{name: "the cap never read", atLimit: new(12)},
		{name: "the limit never read", atCap: new(40)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, known := views.AccountTotals{MinutesAtCap: tt.atCap, MinutesAtLimit: tt.atLimit}.AtCapOrLimit()
			if got != tt.want || known != tt.known {
				t.Errorf("AtCapOrLimit() = %d, %t, want %d, %t", got, known, tt.want, tt.known)
			}
		})
	}
}

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

// A version whose requests were sent upstream and answered without a usage,
// none of its use priced, has no share of use.
func TestAModelsShareOfUseIsLeftOutWhereNoneOfItsUseIsPriced(t *testing.T) {
	unanswered := sent("claude-fable-5", 3, "")
	unanswered.NoUsage = 3
	days := daysFrom("2026-10-07", day("2026-10-07", account("work", sent("claude-opus-5-5", 1, inputUsage), unanswered)))

	for _, m := range views.NewHistory(input(days)).Totals.Accounts[0].ByModel {
		if m.Model == "claude-fable-5" && m.Share != nil {
			t.Errorf("fable's share of work's use is %v, want none, as none of its use is priced", *m.Share)
		}
	}
}

// A version's share of a row's use is said as a whole percentage, "<1%"
// where it rounds to none, and not where it has none.
func TestAModelsShareOfUseIsSaidAsAWholePercentage(t *testing.T) {
	tests := []struct {
		name   string
		share  *float64
		want   string
		wantOK bool
	}{
		{name: "a share", share: new(0.55), want: "55%", wantOK: true},
		{name: "rounding to one", share: new(0.006), want: "1%", wantOK: true},
		{name: "a half percent, to the even, none", share: new(0.005), want: "<1%", wantOK: true},
		{name: "a sliver", share: new(0.0001), want: "<1%", wantOK: true},
		{name: "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := views.ShareSaid(tt.share); got != tt.want || ok != tt.wantOK {
				t.Errorf("ShareSaid = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestAnAccountsDayOfNoRequestsCountsNone(t *testing.T) {
	// Side's windows were read yesterday and today, and work's today, none
	// of them used but work yesterday.
	read := windowsDay{highest: map[string]float64{"5h": 0.2}}
	days := daysFrom("2026-10-06",
		day("2026-10-06", read.of("side"), account("work", sent("claude-opus-5-5", 1, inputUsage))),
		day("2026-10-07", read.of("side"), read.of("work")))
	history := views.NewHistory(input(days))

	var year []int
	for _, d := range history.Year.Days {
		year = append(year, d.Requests)
	}
	work, side, all := history.Totals.Accounts[0], history.Totals.Accounts[1], history.Totals.Accounts[3]
	got := []int{year[0], year[1], history.Months[0].Requests, work.Requests, work.LongestRun, side.Requests, side.LongestRun, all.Requests, all.LongestRun}
	if want := []int{1, 0, 1, 1, 1, 0, 0, 1, 1}; !slices.Equal(got, want) {
		t.Errorf("the year's days, the month and work's, side's and every account's totals count %v requests and runs, want %v", got, want)
	}
	if side.BusiestDay != "" || all.BusiestDay != "2026-10-06" {
		t.Errorf("side's busiest day is %q and every account's %q, want none and 2026-10-06", side.BusiestDay, all.BusiestDay)
	}
}

// Each window's totals say what it had left at its last reset, and the day it
// last reached its limit in the run that reset ended, by the window's key: a
// run from the reset before, or from the window's length before its end
// where none came within it, up to the instant it reset. Every account's say
// neither.
func TestTotalsSayTheDayEachWindowLastRanOutInTheRunItsLastResetEnded(t *testing.T) {
	tests := []struct {
		name   string
		work   windowsDay
		left   map[string]float64
		ranOut map[string]string
	}{
		{name: "a 5-hour window run out before it reset",
			work: windowsDay{limits: []ledger.Limit{limit("5h", "2026-10-03", "12:00")}, resets: []ledger.Reset{reset("5h", "2026-10-03", "14:00", 1)}},
			left: map[string]float64{"5h": 0}, ranOut: map[string]string{"5h": "2026-10-03"}},
		{name: "a week run out twice, the later day its",
			work: windowsDay{limits: []ledger.Limit{limit("7d", "2026-09-29", "18:00"), limit("7d", "2026-10-01", "11:00")}, resets: []ledger.Reset{reset("7d", "2026-10-05", "00:30", 1)}},
			left: map[string]float64{"7d": 0}, ranOut: map[string]string{"7d": "2026-10-01"}},
		{name: "a limit in the run before the last",
			work: windowsDay{limits: []ledger.Limit{limit("5h", "2026-10-03", "09:00")}, resets: []ledger.Reset{reset("5h", "2026-10-03", "10:00", 1), reset("5h", "2026-10-03", "16:00", 0.4)}},
			left: map[string]float64{"5h": 0.6}},
		{name: "a limit at the instant it reset, the next run's",
			work: windowsDay{limits: []ledger.Limit{limit("5h", "2026-10-03", "14:00")}, resets: []ledger.Reset{reset("5h", "2026-10-03", "14:00", 0.7)}},
			left: map[string]float64{"5h": 0.3}},
		{name: "a limit in a run not yet ended",
			work: windowsDay{limits: []ledger.Limit{limit("5h", "2026-10-03", "18:00")}, resets: []ledger.Reset{reset("5h", "2026-10-03", "14:00", 0.5)}},
			left: map[string]float64{"5h": 0.5}},
		{name: "a limit more than the window's length before its last reset, none between",
			work: windowsDay{limits: []ledger.Limit{limit("5h", "2026-10-03", "08:00")}, resets: []ledger.Reset{reset("5h", "2026-10-03", "14:00", 0.9)}},
			left: map[string]float64{"5h": 0.1}},
		{name: "a limit and no reset", work: windowsDay{limits: []ledger.Limit{limit("5h", "2026-10-03", "08:00")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days := daysFrom("2026-09-28", day("2026-10-03", tt.work.of("work")))
			totals := views.NewHistory(input(days)).Totals.Accounts
			work, all := totals[0], totals[len(totals)-1]
			if !maps.Equal(work.LeftAtReset, tt.left) || !maps.Equal(work.RanOut, tt.ranOut) {
				t.Errorf("work left %v at its last resets and ran out %v, want %v and %v", work.LeftAtReset, work.RanOut, tt.left, tt.ranOut)
			}
			if all.LeftAtReset != nil || all.RanOut != nil {
				t.Errorf("every account's left %v and ran out %v, want neither", all.LeftAtReset, all.RanOut)
			}
		})
	}
}

// Asked for from a day after the run that last reset began, the totals say
// the day it ran out all the same, from the days before those asked for, but
// none of the run before it; and a window that reset only before them, they
// say nothing of.
func TestTotalsSayTheDayAWindowRanOutBeforeTheDaysAskedFor(t *testing.T) {
	days := daysFrom("2026-09-27",
		day("2026-09-27", windowsDay{limits: []ledger.Limit{limit("7d", "2026-09-27", "12:00"), limit("5h", "2026-09-27", "13:00")},
			resets: []ledger.Reset{reset("5h", "2026-09-27", "15:00", 1)}}.of("work")),
		day("2026-09-28", windowsDay{resets: []ledger.Reset{reset("7d", "2026-09-28", "21:00", 1)}}.of("work")),
		day("2026-10-02", windowsDay{limits: []ledger.Limit{limit("7d", "2026-10-02", "16:00")}}.of("work")),
		day("2026-10-05", windowsDay{resets: []ledger.Reset{reset("7d", "2026-10-05", "21:00", 1)}}.of("work")))
	tests := []struct {
		from   string
		ranOut map[string]string
	}{
		{from: "2026-09-27", ranOut: map[string]string{"5h": "2026-09-27", "7d": "2026-10-02"}},
		{from: "2026-10-03", ranOut: map[string]string{"7d": "2026-10-02"}},
		{from: "2026-10-05", ranOut: map[string]string{"7d": "2026-10-02"}},
	}
	for _, tt := range tests {
		t.Run(tt.from, func(t *testing.T) {
			in := input(days)
			in.From = on(tt.from)
			if got := views.NewHistory(in).Totals.Accounts[0].RanOut; !maps.Equal(got, tt.ranOut) {
				t.Errorf("asked for from %s, work ran out %v, want %v", tt.from, got, tt.ranOut)
			}
		})
	}
}
