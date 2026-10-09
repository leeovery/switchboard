package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/ledger"
)

// weeksDeps are ledgerDeps whose ledger also holds the summaries of Mondays
// in September and October, of no requests, on which work's and side's weeks
// reset at 00:30, their peaks those of the weeks ending then: work hit its
// limit in the week of the 14th.
func weeksDeps(t *testing.T) cli.Deps {
	t.Helper()
	deps, _ := ledgerDeps(t)
	peaks := []struct {
		day        int
		month      time.Month
		work, side float64
	}{
		{day: 14, month: time.September, work: 0.7, side: 0.4},
		{day: 21, month: time.September, work: 1, side: 0.2},
		{day: 28, month: time.September, work: 0.6, side: 0.3},
		{day: 5, month: time.October, work: 0.5, side: 0.1},
	}
	for _, p := range peaks {
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
	// drop, and with two fewer, in one more's place, none is left for the
	// weeks' use. This week's so far is work's highest since Monday's reset,
	// today's; side's isn't known.
	want := `
Weeks at their peaks  14 Sep  21 Sep  28 Sep  5 Oct
  work                100%    60%     50%     41%…
  side                20%     30%     10%     -
  all accounts        60%     45%     30%     41%…

Weeks: two is one more than you need
  as now: short on 1 of 3 weeks  ·  with one fewer: short on 1  ·  with two fewer: short on 3
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
			`{"week":"2026-10-05","accounts":[{"account":"work","peaks":{"7d":0.41},"limits":{"5h":1}},{"account":"side"},{"all":true,"peaks":{"7d":0.41}}]}]`)},
		{block: "capacity", got: doc.Capacity, want: json.RawMessage(`{"weeks":3,"short":1,"with_one_fewer":1,"with_two_fewer":3,"with_one_more":0,"drop":"side",` +
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
const windowsFromThe6th = `work  Session
  Tue 6 Oct 21:00  90%   resets Tue 6 Oct 23:10  allowed   answer
  Tue 6 Oct 22:14  100%  resets Tue 6 Oct 23:10  rejected  answer
  Wed 7 Oct 09:00  30%   resets Wed 7 Oct 14:00  allowed   answer

work  Week
  Wed 7 Oct 09:00  41%  resets Mon 12 Oct 10:00  allowed  answer

side  Session
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
		{name: "from the day of a time today, whatever the time", args: []string{"--since", "10:00"}, want: `work  Session
  Wed 7 Oct 09:00  30%  resets Wed 7 Oct 14:00  allowed  answer

work  Week
  Wed 7 Oct 09:00  41%  resets Mon 12 Oct 10:00  allowed  answer

side  Session
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
