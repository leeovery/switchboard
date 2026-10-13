package cli_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/views"
)

// weeksDeps are ledgerDeps whose ledger also holds the summaries of Mondays
// in September and October, of no requests, on which work's and side's weeks
// reset at 00:30, their peaks those of the weeks ending then: work hit its
// limit in the week of the 14th.
func weeksDeps(t *testing.T) cli.Deps {
	t.Helper()
	return mondaysDeps(t, []mondayPeaks{
		{day: 14, month: time.September, work: 0.7, side: 0.4},
		{day: 21, month: time.September, work: 1, side: 0.2},
		{day: 28, month: time.September, work: 0.6, side: 0.3},
		{day: 5, month: time.October, work: 0.5, side: 0.1},
	})
}

// mondayPeaks are work's and side's peaks of the weeks ending as their weeks
// reset at 00:30 on a Monday, by its day of the month.
type mondayPeaks struct {
	day        int
	month      time.Month
	work, side float64
}

// mondaysDeps are ledgerDeps whose ledger also holds the summaries of the
// Mondays given, of no requests, on which work's and side's weeks reset.
func mondaysDeps(t *testing.T, mondays []mondayPeaks) cli.Deps {
	t.Helper()
	deps, _ := ledgerDeps(t)
	for _, p := range mondays {
		at := time.Date(2026, p.month, p.day, 0, 30, 0, 0, time.Local)
		date := at.Format(time.DateOnly)
		reset := func(id string, before float64) ledger.AccountDay {
			return ledger.AccountDay{Account: id, SessionIDs: []string{}, Rise: map[string]float64{"7d": 0}, MinutesAtCap: new(0), MinutesAtLimit: new(0),
				Resets: []ledger.Reset{{Window: "7d", At: at.UTC(), Before: before}}}
		}
		data, err := json.Marshal(ledger.Summary{Version: 2, Day: date, Accounts: []ledger.AccountDay{reset("side", p.side), reset("work", p.work)}})
		if err != nil {
			t.Fatal(err)
		}
		writeStateFile(t, ledger.Dir(stateDir(t, deps)), "day-"+date+".json", append(data, '\n'))
	}
	return deps
}

func TestHistoryPrintsEachWeeksPeaksAndWeeksVerdicts(t *testing.T) {
	deps := weeksDeps(t)

	// Of three whole weeks, work hit its limit in one; without either
	// account, the other's peak rising by all of its, as both count as one,
	// on no plan, that week alone is short, so side, used less, is the one to
	// drop; two fewer would leave none, so one more keeps its place. This
	// week's so far is work's highest since Monday's reset, today's; side's
	// isn't known.
	want := `
Weeks at their peaks  14 Sep  21 Sep  28 Sep  5 Oct
  work                100%    60%     50%     41%…
  side                20%     30%     10%     -
  all accounts        60%     45%     30%     41%…

Weeks: two is one more than you need
  as now: short on 1 of 3 weeks  ·  with one fewer: short on 1  ·  with one more: short on none
  work: ends most weeks with room  ·  limit hit 1 of 3 weeks  ·  30% left at a reset, on average  ·  without it, the rest: short on 1
  side: barely used  ·  limit hit 0 of 3 weeks  ·  80% left at a reset, on average  ·  without it, the rest: short on 1
  all accounts: room to spare together  ·  55% left at a reset, on average
  side is the one to drop
  an estimate, from replaying these weeks with one account fewer or more
` + historyPricedAt
	got := run(t, deps, "history", "--pretty", "--since", "2026-09-14")
	if !strings.HasSuffix(got.stdout, want) || got.code != 0 {
		t.Errorf("switchboard history --since 2026-09-14 printed\n%s(%d, %s)\nwant it to end\n%s", got.stdout, got.code, got.stderr, want)
	}
}

func TestHistorySaysTooFewWhereOneMoreWouldStillLeaveWeeksShort(t *testing.T) {
	// Work's week peaked at twice its room each week from the 7th: with one
	// more account every peak falls by a third, still over its room.
	deps := mondaysDeps(t, []mondayPeaks{
		{day: 7, month: time.September, work: 0.5, side: 0.1},
		{day: 14, month: time.September, work: 2, side: 0.1},
		{day: 21, month: time.September, work: 2, side: 0.1},
		{day: 28, month: time.September, work: 2, side: 0.1},
		{day: 5, month: time.October, work: 2, side: 0.1},
	})

	text := run(t, deps, "history", "--pretty", "--since", "2026-09-07")
	want := "\nWeeks: two is too few\n  as now: short on 4 of 4 weeks  ·  with one fewer: short on 4  ·  with one more: short on 4\n"
	if !strings.Contains(text.stdout, want) || text.code != 0 {
		t.Errorf("switchboard history --since 2026-09-07 printed\n%s(%d, %s)\nwant it to say\n%s", text.stdout, text.code, text.stderr, want)
	}
	got := run(t, deps, "history", "--json", "--since", "2026-09-07")
	var doc struct {
		Capacity views.Capacity `json:"capacity"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil || got.code != 0 || doc.Capacity.Verdict != "too few" {
		t.Errorf("switchboard history --json gives the verdict %q (%v, %d), want too few", doc.Capacity.Verdict, err, got.code)
	}
}

func TestHistoryOfTodayAloneGivesThisWeeksPeakSoFar(t *testing.T) {
	deps := weeksDeps(t)

	// Asked for today alone, the reset that began this week, on Monday, is
	// read from the days before for that alone: work's limit of the 6th
	// isn't among today's.
	got := run(t, deps, "history", "--json", "--since", "10:00")
	var doc struct {
		Weeks json.RawMessage `json:"weeks"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil || got.code != 0 {
		t.Fatalf("switchboard history --json = %+v (%v), want the blocks", got, err)
	}
	want := `[{"week":"2026-10-05","accounts":[{"account":"work","peaks":{"7d":0.41},"open":["7d"]},{"account":"side","open":["7d"]},` +
		`{"all":true,"peaks":{"7d":0.41},"open":["7d"]}]}]`
	var compact bytes.Buffer
	if err := json.Compact(&compact, doc.Weeks); err != nil || compact.String() != want {
		t.Errorf("switchboard history --json --since 10:00 gives the weeks\n%s (%v)\nwant\n%s", compact.String(), err, want)
	}
}

func TestHistoryJSONGivesEachWeeksPeaksAndTheCapacity(t *testing.T) {
	deps := weeksDeps(t)

	got := run(t, deps, "history", "--json", "--since", "2026-09-14")
	var doc struct {
		Weeks    json.RawMessage `json:"weeks"`
		Capacity json.RawMessage `json:"capacity"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil || got.code != 0 {
		t.Fatalf("switchboard history --json = %+v (%v), want the blocks", got, err)
	}
	// Work's limit of the 6th is in its week begun on Monday.
	tests := []struct {
		block     string
		got, want json.RawMessage
	}{
		{block: "weeks", got: doc.Weeks, want: json.RawMessage(`[` +
			`{"week":"2026-09-14","accounts":[{"account":"work","peaks":{"7d":1}},{"account":"side","peaks":{"7d":0.2}},{"all":true,"peaks":{"7d":0.6}}]},` +
			`{"week":"2026-09-21","accounts":[{"account":"work","peaks":{"7d":0.6}},{"account":"side","peaks":{"7d":0.3}},{"all":true,"peaks":{"7d":0.45}}]},` +
			`{"week":"2026-09-28","accounts":[{"account":"work","peaks":{"7d":0.5}},{"account":"side","peaks":{"7d":0.1}},{"all":true,"peaks":{"7d":0.3}}]},` +
			`{"week":"2026-10-05","accounts":[{"account":"work","peaks":{"7d":0.41},"open":["7d"],"limits":{"5h":1}},{"account":"side","open":["7d"]},{"all":true,"peaks":{"7d":0.41},"open":["7d"]}]}]`)},
		{block: "capacity", got: doc.Capacity, want: json.RawMessage(`{"weeks":3,"short":1,"with_one_fewer":1,"with_one_more":0,"drop":"side",` +
			`"verdict":"one more than you need","accounts":[` +
			`{"account":"work","verdict":"ends most weeks with room","weeks":3,"limits":1,"left_at_reset":0.3,"without_it":1},` +
			`{"account":"side","verdict":"barely used","weeks":3,"limits":0,"left_at_reset":0.8,"without_it":1},` +
			`{"all":true,"verdict":"room to spare together","weeks":3,"limits":1,"left_at_reset":0.55}]}`)},
	}
	for _, tt := range tests {
		var compact bytes.Buffer
		if err := json.Compact(&compact, tt.got); err != nil || compact.String() != string(tt.want) {
			t.Errorf("switchboard history --json gives the %s\n%s (%v)\nwant\n%s", tt.block, compact.String(), err, tt.want)
		}
	}
}

// windowsFromThe6th are the readings history --windows gives from the 6th:
// work's 5-hour window reaching its limit, and its week, then side's 5-hour
// window, each a reading at a time, the configured accounts in the config's
// order.
const windowsFromThe6th = `work  5-hour
  Tue 6 Oct 21:00  90%   resets Tue 6 Oct 23:10  allowed   answer
  Tue 6 Oct 22:14  100%  resets Tue 6 Oct 23:10  rejected  answer
  Wed 7 Oct 09:00  30%   resets Wed 7 Oct 14:00  allowed   answer

work  week
  Wed 7 Oct 09:00  41%  resets Mon 12 Oct 10:00  allowed  answer

side  5-hour
  Tue 6 Oct 22:15  10%  resets Wed 7 Oct 03:15  allowed  answer
  Wed 7 Oct 10:00  5%   resets Wed 7 Oct 15:00  allowed  answer
`

func TestHistoryWindowsPrintsEachAccountsWindowsReadings(t *testing.T) {
	deps, _ := ledgerDeps(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "from a day", args: []string{"--since", "2026-10-06"}, want: windowsFromThe6th},
		{name: "from the day of a time today, whatever the time", args: []string{"--since", "10:00"}, want: `work  5-hour
  Wed 7 Oct 09:00  30%  resets Wed 7 Oct 14:00  allowed  answer

work  week
  Wed 7 Oct 09:00  41%  resets Mon 12 Oct 10:00  allowed  answer

side  5-hour
  Wed 7 Oct 10:00  5%  resets Wed 7 Oct 15:00  allowed  answer
`},
	}
	for _, tt := range tests {
		for _, form := range prettyForms() {
			t.Run(tt.name+", "+form.name, func(t *testing.T) {
				got := form.run(t, deps, append([]string{"history", "--windows"}, tt.args...)...)
				if got.stdout != tt.want || got.code != 0 {
					t.Errorf("switchboard history --windows %s %s printed\n%s(%d, %s)\nwant\n%s",
						strings.Join(tt.args, " "), strings.Join(form.args, " "), got.stdout, got.code, got.stderr, tt.want)
				}
			})
		}
	}
}

func TestHistoryWindowsJSONGivesEachReadingAsTheReadingsHistoryHoldsIt(t *testing.T) {
	deps, _ := ledgerDeps(t)
	// A reading is as ledgerDeps writes it: when it was read in UTC, and
	// when its window resets as fixtureReadings give it.
	reading := func(when time.Time, use string, resets time.Time, status string) string {
		return `{"at":"` + when.UTC().Format(time.RFC3339) + `","utilization":` + use + `,"resets_at":"` + resets.Format(time.RFC3339) +
			`","status":"` + status + `","source":"answer"}`
	}
	want := `{"windows":[` +
		`{"account":"work","window":"5h","readings":[` + reading(october(6, 21, 0, 0), "0.9", october(6, 23, 10, 0), "allowed") + `,` +
		reading(october(6, 22, 14, 0), "1", october(6, 23, 10, 0), "rejected") + `,` + reading(october(7, 9, 0, 0), "0.3", october(7, 14, 0, 0), "allowed") + `]},` +
		`{"account":"work","window":"7d","readings":[` + reading(october(7, 9, 0, 0), "0.41", october(12, 10, 0, 0), "allowed") + `]},` +
		`{"account":"side","window":"5h","readings":[` + reading(october(6, 22, 15, 0), "0.1", october(7, 3, 15, 0), "allowed") + `,` +
		reading(october(7, 10, 0, 0), "0.05", october(7, 15, 0, 0), "allowed") + `]}]}`
	for _, form := range jsonForms() {
		t.Run(form.name, func(t *testing.T) {
			got := form.run(t, deps, "history", "--windows", "--since", "2026-10-06")
			var compact bytes.Buffer
			if err := json.Compact(&compact, []byte(got.stdout)); err != nil || got.code != 0 || compact.String() != want {
				t.Errorf("switchboard history --windows %s printed\n%s(%v, %d, %s)\nwant\n%s", strings.Join(form.args, " "), compact.String(), err, got.code, got.stderr, want)
			}
		})
	}
}

func TestHistoryWindowsSaysSoWhereThereAreNoReadings(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	configure(t, deps)
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"history", "--windows", "--pretty"}, want: "no readings since Tue 8 Sep 2026\n"},
		{args: []string{"history", "--windows", "--json"}, want: "{\n  \"windows\": []\n}\n"},
	}
	for _, tt := range tests {
		if got := run(t, deps, tt.args...); got != (result{stdout: tt.want}) {
			t.Errorf("switchboard %s = %+v, want it to print\n%s", strings.Join(tt.args, " "), got, tt.want)
		}
	}
}

// The lines of today's of fixtureLines that aren't requests: work's quota
// check, of no session it counts, and its count of tokens, of session A.
var (
	worksCheck = fixtureLines["2026-10-07"][0]
	worksCount = fixtureLines["2026-10-07"][5]
)

// checksAloneDeps are deps whose ledger holds today's lines given alone, by a
// clock stopped at ledgerNow, and whose readings history holds the readings
// given.
func checksAloneDeps(t *testing.T, today []ledger.Line, read ...readings.Reading) cli.Deps {
	t.Helper()
	deps := testDeps(nil, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	configure(t, deps)
	var lines bytes.Buffer
	for _, line := range today {
		line.At = line.At.UTC()
		data, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		lines.Write(append(data, '\n'))
	}
	writeStateFile(t, ledger.Dir(stateDir(t, deps)), "requests-2026-10-07.jsonl", lines.Bytes())
	writeReadings(t, readings.Dir(stateDir(t, deps)), read...)
	return deps
}

func TestHistoryOfDaysWithoutRequestsSaysSoThenNotesTheirLimitsAndWeeksReset(t *testing.T) {
	const none = "no requests since Wed 7 Oct 2026\n"
	tests := []struct {
		name  string
		today []ledger.Line
		read  []readings.Reading
		// want is the text, the blocks and the prices left out, as they tell
		// of no request.
		want string
	}{
		{name: "a quota check alone", today: []ledger.Line{worksCheck}, want: none},
		// Work's count of tokens is of a session, which makes no note on a day
		// of no requests.
		{name: "a check and a count", today: []ledger.Line{worksCheck, worksCount}, want: none},
		// Each account's highest use alone makes no note either.
		{name: "a check and a count, each account's windows read", today: []ledger.Line{worksCheck, worksCount}, read: fixtureReadings, want: none},
		{
			name: "a check and a count, work's session reaching its limit", today: []ledger.Line{worksCheck, worksCount},
			read: append(slices.Clone(fixtureReadings), readings.Reading{At: october(7, 9, 30, 0), Account: "work", Key: "5h", Utilization: 1,
				ResetsAt: october(7, 14, 0, 0), Status: quota.StatusRejected}),
			want: none + "\nWed 7 Oct 2026, so far\n  work: 1 session  ·  limits: 5-hour at 09:30  ·  highest: 5-hour 100%, week 41%\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, checksAloneDeps(t, tt.today, tt.read...), "history", "--pretty", "--since", "10:00")
			if got != (result{stdout: tt.want}) {
				t.Errorf("switchboard history --pretty = %+v, want it to print\n%s", got, tt.want)
			}
		})
	}
}

func TestHistoryNotesAnAccountThatServedNoRequestOnlyWhereALimitWasReachedOrAWeekReset(t *testing.T) {
	// Work's request of claude-opus-9, side serving none, its windows read.
	worksRequest := fixtureLines["2026-10-07"][4]
	const busy = "Wed 7 Oct 2026, so far\n  work  claude-opus-9  1 request  110 tokens  1 session  unpriced\n"
	const works = "  work: 1 session  ·  highest: 5-hour 30%, week 41%\n\n"
	tests := []struct {
		name string
		read []readings.Reading
		want string
	}{
		{name: "its highest use alone, its session reset", read: fixtureReadings, want: busy + works},
		{
			name: "a limit reached",
			read: append(slices.Clone(fixtureReadings), readings.Reading{At: october(7, 10, 30, 0), Account: "side", Key: "5h", Utilization: 1,
				ResetsAt: october(7, 15, 0, 0), Status: quota.StatusRejected}),
			want: busy + "  side: no sessions  ·  limits: 5-hour at 10:30  ·  highest: 5-hour 100%\n" + works,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, checksAloneDeps(t, []ledger.Line{worksRequest}, tt.read...), "history", "--pretty", "--since", "10:00")
			if !strings.HasPrefix(got.stdout, tt.want) || got.code != 0 {
				t.Errorf("switchboard history --pretty printed\n%s(%d, %s)\nwant it to begin\n%s", got.stdout, got.code, got.stderr, tt.want)
			}
		})
	}
}

func TestHistoryJSONOfQuotaChecksAndCountsAloneHoldsThem(t *testing.T) {
	deps := checksAloneDeps(t, []ledger.Line{worksCheck, worksCount})
	// The JSON holds the day as the ledger does, its check and count among it.
	got := run(t, deps, "history", "--json", "--since", "10:00")
	var doc struct {
		Days []struct {
			Accounts []struct {
				Models []struct {
					Upstream, Checks, Counts int
				} `json:"models"`
			} `json:"accounts"`
		} `json:"days"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil || len(doc.Days) != 1 || len(doc.Days[0].Accounts) != 1 {
		t.Fatalf("switchboard history --json = %+v (%v), want today's day, work's alone", got, err)
	}
	var checks, counts, upstream int
	for _, m := range doc.Days[0].Accounts[0].Models {
		checks, counts, upstream = checks+m.Checks, counts+m.Counts, upstream+m.Upstream
	}
	if checks != 1 || counts != 1 || upstream != 0 {
		t.Errorf("switchboard history --json gives today %d checks, %d counts and %d requests upstream, want 1, 1 and none", checks, counts, upstream)
	}
}

// aDayOfARequest is a day of work's, of a request.
var aDayOfARequest = []views.Day{{Version: 2, Day: "2026-10-07", Accounts: []ledger.PricedAccount{
	{Account: "work", Sessions: 1, Models: []ledger.PricedModel{{Model: "claude-opus-5-5", Upstream: 1}}},
}}}

func TestHistorysTextOfBlocksNotBuiltAsHistoryBuildsThemWritesWhatTheyGive(t *testing.T) {
	work, all := views.AccountTotals{Account: "work"}, views.AccountTotals{All: true}
	tests := []struct {
		name    string
		history views.History
	}{
		{name: "no blocks"},
		{name: "totals without plans", history: views.History{Totals: views.Totals{Accounts: []views.AccountTotals{work, all}}}},
		{name: "totals without every account's", history: views.History{Totals: views.Totals{Accounts: []views.AccountTotals{work}},
			Plans: []views.PlanCost{{Account: "side"}}}},
		{name: "weeks whose accounts differ", history: views.History{Weeks: []views.Week{
			{Week: "2026-09-28", Accounts: []views.AccountWeek{{Account: "work"}, {Account: "side"}, {All: true}}},
			{Week: "2026-10-05", Accounts: []views.AccountWeek{{All: true, Peaks: map[string]float64{"7d": 0.5}}}},
		}}},
		{name: "weeks judged, no account's verdict", history: views.History{Capacity: views.Capacity{Weeks: 2, Verdict: "about right"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.history.Days = aDayOfARequest
			var out bytes.Buffer
			if err := cli.WriteHistory(&out, tt.history, ledger.Pricing, october(7, 0, 0, 0), ledgerNow); err != nil || !strings.Contains(out.String(), "Wed 7 Oct 2026") {
				t.Errorf("history of %s printed\n%s(%v)\nwant the day, and what the blocks give", tt.name, out.String(), err)
			}
		})
	}
}

func TestHistorysTextCountsOneAsOne(t *testing.T) {
	price := ledger.Picodollars(200_000_000_000_000)
	history := views.History{
		Days:     aDayOfARequest,
		Totals:   views.Totals{Accounts: []views.AccountTotals{{Account: "work", LongestRun: 1}, {All: true, LongestRun: 1}}},
		Plans:    []views.PlanCost{{Account: "work", Plan: "max20x", Price: &price}, {All: true, Price: &price}},
		Capacity: views.Capacity{Weeks: 1, Verdict: "about right", Accounts: []views.AccountVerdict{{Account: "work", Verdict: "hits its limit most weeks", Weeks: 1, Limits: 1, LeftAtReset: new(0.0)}}},
	}

	var out bytes.Buffer
	if err := cli.WriteHistory(&out, history, ledger.Pricing, october(7, 0, 0, 0), ledgerNow); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"$200/mo · 1 plan\n", "1 day ", "Weeks: one is about right", "short on none of 1 week  ·", "limit hit 1 of 1 week  ·"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("history printed\n%s\nwant it to say %q", out.String(), want)
		}
	}
}

func TestHistorysTotalsSetTheirThousandsApart(t *testing.T) {
	history := views.History{Days: aDayOfARequest, Totals: views.Totals{Accounts: []views.AccountTotals{
		{Account: "work", Requests: 12345, Sessions: 1240}, {All: true, Requests: 12345, Sessions: 1240},
	}}}

	var out bytes.Buffer
	if err := cli.WriteHistory(&out, history, ledger.Pricing, october(7, 0, 0, 0), ledgerNow); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"12,345  12,345\n", "1,240   1,240\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("history printed\n%s\nwant it to say %q", out.String(), want)
		}
	}
}

func TestHistorysTotalsSayTheirFiguresAsTheDashboardDoes(t *testing.T) {
	dollars := func(cents int64) *ledger.Picodollars {
		p := ledger.Picodollars(cents * 10_000_000_000)
		return &p
	}
	tests := []struct {
		name   string
		totals views.AccountTotals
		plan   views.PlanCost
		want   string
	}{
		{name: "worth under half a cent, as more than none", totals: views.AccountTotals{Amount: new(ledger.Picodollars(4_000_000_000))},
			want: "  worth                   <$0.01\n"},
		{name: "worth part unpriced", totals: views.AccountTotals{Amount: dollars(150), Unpriced: []string{"no_usage"}},
			want: "  worth                   $1.50+\n"},
		{name: "the plan's price a month, its thousands set apart", plan: views.PlanCost{Plan: "max20x", Price: dollars(100_000)},
			want: "  plan                    Max 20x · $1,000/mo\n"},
		{name: "against its price to a tenth, a half rounded to the even one", plan: views.PlanCost{Cost: dollars(4600), Ratio: new(3.85)},
			want: "  against its price       3.8× its $46\n"},
		{name: "against a price of thousands, set apart", plan: views.PlanCost{Cost: dollars(138_000), Ratio: new(1.0)},
			want: "  against its price       1.0× its $1,380\n"},
		{name: "each window left at its reset named as status names it", totals: views.AccountTotals{LeftAtReset: map[string]float64{"5h": 0, "7d": 0.59}},
			want: "  left at its last reset  5-hour 0% · week 59%\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.totals.Account, tt.plan.Account = "work", "work"
			history := views.History{Days: aDayOfARequest, Totals: views.Totals{Accounts: []views.AccountTotals{tt.totals}}, Plans: []views.PlanCost{tt.plan}}

			var out bytes.Buffer
			if err := cli.WriteHistory(&out, history, ledger.Pricing, october(7, 0, 0, 0), ledgerNow); err != nil || !strings.Contains(out.String(), tt.want) {
				t.Errorf("history printed\n%s(%v)\nwant it to say\n%s", out.String(), err, tt.want)
			}
		})
	}
}

func TestHistorysTotalsOfEveryAccountCountTheirLimitsAndMovesWithTheirThousandsApart(t *testing.T) {
	all := views.AccountTotals{All: true, MovedOn: 1240, Limits: map[string]int{"5h": 1200, "7d": 34}}
	work := views.AccountTotals{Account: "work", MovedOn: 1240, MovedOff: 1100}
	history := views.History{Days: aDayOfARequest, Totals: views.Totals{Accounts: []views.AccountTotals{work, all}}}

	var out bytes.Buffer
	if err := cli.WriteHistory(&out, history, ledger.Pricing, october(7, 0, 0, 0), ledgerNow); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"  sessions moved          1,240 in · 1,100 out  1,240 moves\n", "  limits hit              none                  1,234\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("history printed\n%s\nwant it to say %q", out.String(), want)
		}
	}
}

func TestHistorysDayOfAModelThatSpentNothingSaysNoWorthAsTheTotalsDo(t *testing.T) {
	// The ledger prices none of a model it doesn't know that spent nothing,
	// and names nothing unpriced: its worth is none, not unknown.
	day := []views.Day{{Version: 2, Day: "2026-10-07", Accounts: []ledger.PricedAccount{
		{Account: "work", Sessions: 1, Models: []ledger.PricedModel{{Model: "claude-opus-9", Upstream: 1, Sessions: 1}}},
	}}}

	var out bytes.Buffer
	const want = "  work  claude-opus-9  1 request  0 tokens  1 session  -\n"
	if err := cli.WriteHistory(&out, views.History{Days: day}, ledger.Pricing, october(7, 0, 0, 0), ledgerNow); err != nil || !strings.Contains(out.String(), want) {
		t.Errorf("history printed\n%s(%v)\nwant it to say\n%s", out.String(), err, want)
	}
}

func TestHistoryPassesOverADayOfNoAccountsUnread(t *testing.T) {
	// A day of no accounts whose date isn't one never fails the text.
	days := append([]views.Day{{Version: 2, Day: "2026-13-45"}}, aDayOfARequest...)

	var out bytes.Buffer
	if err := cli.WriteHistory(&out, views.History{Days: days}, ledger.Pricing, october(6, 0, 0, 0), ledgerNow); err != nil || !strings.HasPrefix(out.String(), "Wed 7 Oct 2026") {
		t.Errorf("history printed\n%s(%v)\nwant today's table, the empty day passed over", out.String(), err)
	}
}
