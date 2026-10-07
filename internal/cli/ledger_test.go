package cli_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

// october is the local time on the given day of October 2026, at
// hour:minute:second: the ledger files its lines under local days.
func october(day, hour, minute, second int) time.Time {
	return time.Date(2026, 10, day, hour, minute, second, 0, time.Local)
}

// ledgerNow is the time by the clock in the tests of the commands that read
// the ledger: Wednesday 7 October, 13:12 local time.
var ledgerNow = october(7, 13, 12, 0)

// The sessions of the ledger's tests: the first two share the start of their
// ids.
const (
	sessionA = "5b0e7c1a-1f2a-4b3c-9d8e-7f6a5b4c3d2e"
	sessionB = "5b0e9d33-8a41-4c2e-b7f0-1d2c3b4a5e6f"
	sessionC = "18bb2c41-6d5e-4f3a-8b2c-9e0d1f2a3b4c"
)

// The usages the ledger's tests' answers give.
const (
	cachedUsage = `{"input_tokens":12,"cache_creation_input_tokens":3120,"cache_read_input_tokens":182340,` +
		`"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":3120},"output_tokens":845,"service_tier":"standard"}`
	checkUsage    = `{"input_tokens":8,"output_tokens":1,"service_tier":"standard"}`
	searchedUsage = `{"input_tokens":5,"cache_read_input_tokens":20000,"output_tokens":50,"server_tool_use":{"web_search_requests":1}}`
	unknownUsage  = `{"input_tokens":100,"output_tokens":10}`
)

// fixtureLines are the ledger's tests' lines, in the order their requests
// ended, by the day each arrived on: the 4th's compressed, and summarised;
// the 6th's not yet summarised, as when the router stopped as it ended; and
// today's.
var fixtureLines = map[string][]ledger.Line{
	"2026-10-04": {
		{At: october(4, 9, 0, 0), Request: "a1", Kind: ledger.KindMessage, Session: sessionA, Model: "claude-opus-5-5", Account: "work", Reason: "sticky",
			Status: 200, Attempts: 1, TotalMS: 14230, Usage: json.RawMessage(cachedUsage)},
	},
	"2026-10-06": {
		{At: october(6, 22, 15, 0), Request: "b2", Kind: ledger.KindMessage, Session: sessionB, Model: "claude-opus-5-5", Account: "side",
			Reason: "moved: work hit its limit", From: "work", Tried: []ledger.Tried{{Account: "work", Why: "hit its limit"}}, Status: 200, Attempts: 2,
			TotalMS: 3100, Usage: json.RawMessage(searchedUsage)},
	},
	"2026-10-07": {
		{At: october(7, 9, 0, 0), Request: "c3", Kind: ledger.KindCheck, Session: sessionC, Model: "claude-haiku-4-5", Account: "work", Reason: "new",
			Status: 200, Attempts: 1, TotalMS: 640, Usage: json.RawMessage(checkUsage)},
		{At: october(7, 9, 1, 5), Request: "c5", Kind: ledger.KindMessage, Session: sessionB, Model: "claude-opus-5-5", Reason: "no account has room",
			Status: 429, TotalMS: 3},
		{At: october(7, 9, 1, 0), Request: "c4", Kind: ledger.KindMessage, Session: sessionA, Model: "claude-opus-5-5", Account: "work", Reason: "sticky",
			Status: 200, Attempts: 1, TotalMS: 14230, Usage: json.RawMessage(cachedUsage)},
		{At: october(7, 10, 0, 0), Request: "c6", Kind: ledger.KindMessage, Session: sessionB, Model: "claude-opus-5-5", Account: "side", Reason: "sticky",
			Status: 200, Canceled: true, Attempts: 1, TotalMS: 2000},
		{At: october(7, 11, 0, 0), Request: "c7", Kind: ledger.KindMessage, Session: sessionC, Model: "claude-opus-9", Account: "work", Reason: "new",
			Status: 200, Attempts: 1, TotalMS: 950, Usage: json.RawMessage(unknownUsage)},
		{At: october(7, 12, 0, 0), Request: "c8", Kind: ledger.KindCount, Session: sessionA, Model: "claude-opus-5-5", Account: "work", Reason: "sticky",
			Status: 200, Attempts: 1, TotalMS: 140},
	},
}

// heldSummary is the summary the ledger holds of the 4th, as the router wrote
// it.
const heldSummary = `{"version":1,"day":"2026-10-04","accounts":[{"account":"work","models":[{"model":"claude-opus-5-5","upstream":1,"unsent":0,` +
	`"checks":0,"counts":0,"sessions":1,"usage":{"cache_creation":{"ephemeral_1h_input_tokens":3120,"ephemeral_5m_input_tokens":0},` +
	`"cache_creation_input_tokens":3120,"cache_read_input_tokens":182340,"input_tokens":12,"output_tokens":845}}],"sessions":1,` +
	`"moved_on":0,"moved_off":0,"highest":{"5h":0.5}}]}`

// fixtureReadings are the readings history's readings in the ledger's tests:
// work reaching its session's limit on the 6th, its window reset by the next
// day, and side's still running into it.
var fixtureReadings = []readings.Reading{
	{At: october(6, 21, 0, 0), Account: "work", Key: "5h", Utilization: 0.9, ResetsAt: october(6, 23, 10, 0), Status: quota.StatusAllowed},
	{At: october(6, 22, 14, 0), Account: "work", Key: "5h", Utilization: 1, ResetsAt: october(6, 23, 10, 0), Status: quota.StatusRejected},
	{At: october(6, 22, 15, 0), Account: "side", Key: "5h", Utilization: 0.1, ResetsAt: october(7, 3, 15, 0), Status: quota.StatusAllowed},
	{At: october(7, 9, 0, 0), Account: "work", Key: "5h", Utilization: 0.3, ResetsAt: october(7, 14, 0, 0), Status: quota.StatusAllowed},
	{At: october(7, 9, 0, 0), Account: "work", Key: "7d", Utilization: 0.41, ResetsAt: october(12, 10, 0, 0), Status: quota.StatusAllowed},
	{At: october(7, 10, 0, 0), Account: "side", Key: "5h", Utilization: 0.05, ResetsAt: october(7, 15, 0, 0), Status: quota.StatusAllowed},
}

// ledgerDeps are deps whose state directory holds the ledger's tests' lines,
// summary and readings, by a clock stopped at ledgerNow, and the JSON of each
// line written, by its request's id.
func ledgerDeps(t *testing.T) (cli.Deps, map[string]string) {
	t.Helper()
	deps := testDeps(nil, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	state := stateDir(t, deps)
	dir, history := ledger.Dir(state), readings.Dir(state)
	written := make(map[string]string)
	for date, lines := range fixtureLines {
		var day bytes.Buffer
		for _, line := range lines {
			line.At = line.At.UTC()
			data, err := json.Marshal(line)
			if err != nil {
				t.Fatal(err)
			}
			written[line.Request] = string(data)
			day.Write(append(data, '\n'))
		}
		name, data := "requests-"+date+".jsonl", day.Bytes()
		if date == "2026-10-04" {
			name, data = name+".gz", gzipOf(t, data)
		}
		writeStateFile(t, dir, name, data)
	}
	writeStateFile(t, dir, "day-2026-10-04.json", []byte(heldSummary+"\n"))
	for _, r := range fixtureReadings {
		r.At, r.Source = r.At.UTC(), readings.FromAnswer
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		appendStateFile(t, history, "readings-"+r.At.Local().Format(time.DateOnly)+".jsonl", append(data, '\n'))
	}
	return deps, written
}

// gzipOf returns data compressed, as a gzip member.
func gzipOf(t *testing.T, data []byte) []byte {
	t.Helper()
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}

// writeStateFile writes data to the file with the given name in dir, making
// dir, private, where it isn't there.
func writeStateFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// appendStateFile appends data to the file with the given name in dir, as
// writeStateFile writes it.
func appendStateFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	held, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	writeStateFile(t, dir, name, append(held, data...))
}

// linesOf returns the JSON of the lines of the requests with the given ids,
// as written, a line each.
func linesOf(written map[string]string, requests ...string) string {
	var lines strings.Builder
	for _, request := range requests {
		lines.WriteString(written[request] + "\n")
	}
	return lines.String()
}

// todaysRequests is what requests prints of today's lines.
const todaysRequests = `Wed 7 Oct 2026, so far
  09:00:00  18bb2c41  claude-haiku-4-5  work  200           8 in, 1 out       640ms
  09:01:00  5b0e7c1a  claude-opus-5-5   work  200           185k in, 845 out  14.2s
  09:01:05  5b0e9d33  claude-opus-5-5   -     429                             3ms
  10:00:00  5b0e9d33  claude-opus-5-5   side  200 canceled                    2.0s
  11:00:00  18bb2c41  claude-opus-9     work  200           100 in, 10 out    950ms
  12:00:00  5b0e7c1a  claude-opus-5-5   work  200                             140ms
`

func TestRequestsPrintsTheLedgersLinesOldestFirst(t *testing.T) {
	deps, _ := ledgerDeps(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "today's, unless told otherwise", want: todaysRequests},
		{name: "from the start of a day, under each day's", args: []string{"--since", "2026-10-04"}, want: `Sun 4 Oct 2026
  09:00:00  5b0e7c1a  claude-opus-5-5  work  200  185k in, 845 out  14.2s

Tue 6 Oct 2026
  22:15:00  5b0e9d33  claude-opus-5-5  side  200  20k in, 50 out  3.1s

` + todaysRequests},
		{name: "from a time today", args: []string{"--since", "10:00"}, want: `Wed 7 Oct 2026, so far
  10:00:00  5b0e9d33  claude-opus-5-5  side  200 canceled                  2.0s
  11:00:00  18bb2c41  claude-opus-9    work  200           100 in, 10 out  950ms
  12:00:00  5b0e7c1a  claude-opus-5-5  work  200                           140ms
`},
		{name: "from hours ago", args: []string{"--since", "3h"}, want: `Wed 7 Oct 2026, so far
  11:00:00  18bb2c41  claude-opus-9    work  200  100 in, 10 out  950ms
  12:00:00  5b0e7c1a  claude-opus-5-5  work  200                  140ms
`},
		{name: "from days ago", args: []string{"--since", "2d"}, want: `Tue 6 Oct 2026
  22:15:00  5b0e9d33  claude-opus-5-5  side  200  20k in, 50 out  3.1s

` + todaysRequests},
		{name: "a session's, by as much of its id as is unique", args: []string{"--session", "5b0e7"}, want: `Wed 7 Oct 2026, so far
  09:01:00  5b0e7c1a  claude-opus-5-5  work  200  185k in, 845 out  14.2s
  12:00:00  5b0e7c1a  claude-opus-5-5  work  200                    140ms
`},
		{name: "a session's, by its id", args: []string{"--session", sessionB}, want: `Wed 7 Oct 2026, so far
  09:01:05  5b0e9d33  claude-opus-5-5  -     429             3ms
  10:00:00  5b0e9d33  claude-opus-5-5  side  200 canceled    2.0s
`},
		{name: "an account's", args: []string{"--account", "side"}, want: `Wed 7 Oct 2026, so far
  10:00:00  5b0e9d33  claude-opus-5-5  side  200 canceled    2.0s
`},
		{name: "an account's session's, from a day", args: []string{"--account", "work", "--session", "18bb", "--since", "2026-10-04"},
			want: `Wed 7 Oct 2026, so far
  09:00:00  18bb2c41  claude-haiku-4-5  work  200  8 in, 1 out     640ms
  11:00:00  18bb2c41  claude-opus-9     work  200  100 in, 10 out  950ms
`},
		{name: "none", args: []string{"--account", "nobody"}, want: "no requests since Wed 7 Oct 2026 00:00\n"},
		{name: "a session no line is of", args: []string{"--session", "77aa"}, want: "no requests since Wed 7 Oct 2026 00:00\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, deps, append([]string{"requests"}, tt.args...)...)
			if got.stdout != tt.want || got.code != 0 {
				t.Errorf("switchboard requests %s printed\n%s(%d, %s)\nwant\n%s", strings.Join(tt.args, " "), got.stdout, got.code, got.stderr, tt.want)
			}
		})
	}
}

func TestRequestsJSONPrintsEachLineAsTheLedgerHoldsIt(t *testing.T) {
	deps, written := ledgerDeps(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "today's, oldest first", want: linesOf(written, "c3", "c4", "c5", "c6", "c7", "c8")},
		{name: "from a day", args: []string{"--since", "2026-10-04"}, want: linesOf(written, "a1", "b2", "c3", "c4", "c5", "c6", "c7", "c8")},
		{name: "a session's", args: []string{"--session", "5b0e9"}, want: linesOf(written, "c5", "c6")},
		{name: "an account's", args: []string{"--account", "work", "--since", "11:00"}, want: linesOf(written, "c7", "c8")},
		{name: "none", args: []string{"--account", "nobody"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, deps, append([]string{"requests", "--json"}, tt.args...)...)
			if got.stdout != tt.want || got.code != 0 {
				t.Errorf("switchboard requests --json %s printed\n%s(%d, %s)\nwant\n%s", strings.Join(tt.args, " "), got.stdout, got.code, got.stderr, tt.want)
			}
		})
	}
}

func TestRequestsOfASessionAsMuchOfWhoseIdIsGivenAsStartsSeveralFails(t *testing.T) {
	deps, _ := ledgerDeps(t)

	got := run(t, deps, "requests", "--session", "5b0e")
	want := "Error: session 5b0e could be any of these, so give more of its id:\n  " + sessionA + "\n  " + sessionB + "\n"
	if got.code != 1 || got.stdout != "" || got.stderr != want {
		t.Errorf("switchboard requests --session 5b0e = %+v, want exit status 1 and\n%s", got, want)
	}
}

func TestSinceIsADayATimeTodayOrHowLongAgo(t *testing.T) {
	deps, _ := ledgerDeps(t)
	tests := []struct {
		since string
		want  string
	}{
		{since: "soon", want: `Error: --since "soon" isn't a day, as 2026-10-01, a time today, as 14:00, or how long ago, as 3h or 2d`},
		{since: "2026-13-01", want: `Error: --since "2026-13-01" isn't a day`},
		{since: "0h", want: `Error: --since "0h" isn't a day`},
		{since: "-2d", want: `Error: --since "-2d" isn't a day`},
		{since: "23:00", want: "Error: --since 23:00 is still to come"},
		{since: "2026-10-08", want: "Error: --since 2026-10-08 is still to come"},
		{since: "", want: `Error: --since "" isn't a day`},
	}
	for _, tt := range tests {
		for _, command := range []string{"requests", "history"} {
			got := run(t, deps, command, "--since", tt.since)
			if got.code != 1 || !strings.Contains(got.stderr, tt.want) || !strings.Contains(got.stdout+got.stderr, "Usage:") {
				t.Errorf("switchboard %s --since %q = %+v, want exit status 1, the usage, and %q", command, tt.since, got, tt.want)
			}
		}
	}
}

// historyFromThe6th is what history prints of the 6th and today.
const historyFromThe6th = `Tue 6 Oct 2026
  side  claude-opus-5-5  1 request  20k tokens  1 session  $0.02
  side: 1 session  ·  1 moved on  ·  highest: Session 10%
  work: no sessions  ·  1 moved off  ·  limits: Session at 22:14  ·  highest: Session 100%

` + historyOfToday

// historyOfToday is what history prints of today, the prices last.
const historyOfToday = `Wed 7 Oct 2026, so far
  no account  claude-opus-5-5   1 request   0 tokens     1 session    $0.00
  side        claude-opus-5-5   1 request   0 tokens     1 session    $0.00
  work        claude-haiku-4-5  1 request   9 tokens     no sessions  $0.00
              claude-opus-5-5   2 requests  186k tokens  1 session    $0.08
              claude-opus-9     1 request   110 tokens   1 session    unpriced
  side: 1 session  ·  highest: Session 10%
  work: 2 sessions  ·  highest: Session 30%, Week 41%

worth is what they'd have cost through the API, at its prices as of 7 Oct 2026
`

func TestHistoryPrintsEachDayByAccountAndModel(t *testing.T) {
	deps, _ := ledgerDeps(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "the last 30 days, of each day the ledger holds", want: `Sun 4 Oct 2026
  work  claude-opus-5-5  1 request  186k tokens  1 session  $0.08
  work: 1 session  ·  highest: Session 50%

` + historyFromThe6th},
		{name: "from a day", args: []string{"--since", "2026-10-06"}, want: historyFromThe6th},
		{name: "from the day of a time today", args: []string{"--since", "10:00"}, want: historyOfToday},
		{name: "from the day of days ago", args: []string{"--since", "2d"}, want: historyFromThe6th},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, deps, append([]string{"history"}, tt.args...)...)
			if got.stdout != tt.want || got.code != 0 {
				t.Errorf("switchboard history %s printed\n%s(%d, %s)\nwant\n%s", strings.Join(tt.args, " "), got.stdout, got.code, got.stderr, tt.want)
			}
		})
	}
}

func TestHistoryJSONPrintsEachDaysSummaryWithEachModelsWorth(t *testing.T) {
	deps, _ := ledgerDeps(t)

	got := run(t, deps, "history", "--json", "--since", "2026-10-04")
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(got.stdout)); err != nil || got.code != 0 || !strings.HasPrefix(got.stdout, "{\n  \"prices_as_of\"") {
		t.Fatalf("switchboard history --json = %+v (%v), want indented JSON", got, err)
	}
	limit := fixtureReadings[1]
	want := `{"prices_as_of":"2026-10-07","days":[` +
		`{"version":1,"day":"2026-10-04","accounts":[{"account":"work","sessions":1,"moved_on":0,"moved_off":0,"highest":{"5h":0.5},` +
		`"models":[{"model":"claude-opus-5-5","upstream":1,"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"cache_creation":` +
		`{"ephemeral_1h_input_tokens":3120,"ephemeral_5m_input_tokens":0},"cache_creation_input_tokens":3120,"cache_read_input_tokens":182340,` +
		`"input_tokens":12,"output_tokens":845},"worth":0.078376}]}]},` +
		`{"version":1,"day":"2026-10-06","accounts":[{"account":"side","sessions":1,"moved_on":1,"moved_off":0,"highest":{"5h":0.1},` +
		`"models":[{"model":"claude-opus-5-5","upstream":1,"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"cache_read_input_tokens":20000,` +
		`"input_tokens":5,"output_tokens":50,"server_tool_use":{"web_search_requests":1}},"worth":0.01502}]},` +
		`{"account":"work","sessions":0,"moved_on":0,"moved_off":1,"highest":{"5h":1},"limits":[{"window":"5h","at":"` +
		limit.At.UTC().Format(time.RFC3339) + `","resets_at":"` + limit.ResetsAt.UTC().Format(time.RFC3339) + `"}]}]},` +
		`{"version":1,"day":"2026-10-07","accounts":[` +
		`{"sessions":1,"moved_on":0,"moved_off":0,"models":[{"model":"claude-opus-5-5","upstream":0,"unsent":1,"checks":0,"counts":0,"sessions":1,"worth":0}]},` +
		`{"account":"side","sessions":1,"moved_on":0,"moved_off":0,"highest":{"5h":0.1},"models":[{"model":"claude-opus-5-5","upstream":1,"unsent":0,` +
		`"checks":0,"counts":0,"sessions":1,"worth":0}]},` +
		`{"account":"work","sessions":2,"moved_on":0,"moved_off":0,"highest":{"5h":0.3,"7d":0.41},"models":[` +
		`{"model":"claude-haiku-4-5","upstream":0,"unsent":0,"checks":1,"counts":0,"sessions":0,"usage":{"input_tokens":8,"output_tokens":1},"worth":0.000013},` +
		`{"model":"claude-opus-5-5","upstream":1,"unsent":0,"checks":0,"counts":1,"sessions":1,"usage":{"cache_creation":` +
		`{"ephemeral_1h_input_tokens":3120,"ephemeral_5m_input_tokens":0},"cache_creation_input_tokens":3120,"cache_read_input_tokens":182340,` +
		`"input_tokens":12,"output_tokens":845},"worth":0.078376},` +
		`{"model":"claude-opus-9","upstream":1,"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"input_tokens":100,"output_tokens":10}}]}]}]}`
	if compact.String() != want {
		t.Errorf("switchboard history --json printed\n%s\nwant\n%s", compact.String(), want)
	}
}

func TestTheLedgersCommandsNeverEchoAToken(t *testing.T) {
	deps, _ := ledgerDeps(t)
	tests := []struct {
		args []string
		want result
	}{
		{args: []string{"requests", "--session", tokenShaped}, want: result{stdout: "no requests since Wed 7 Oct 2026 00:00\n"}},
		{args: []string{"requests", "--account", tokenShaped}, want: result{stdout: "no requests since Wed 7 Oct 2026 00:00\n"}},
	}
	for _, tt := range tests {
		if got := run(t, deps, tt.args...); got != tt.want {
			t.Errorf("switchboard %s = %+v, want %+v", strings.Join(tt.args, " "), got, tt.want)
		}
	}
	for _, command := range []string{"requests", "history"} {
		got := run(t, deps, command, "--since", tokenShaped)
		if want := `Error: --since "[redacted]" isn't a day`; got.code != 1 || !strings.Contains(got.stderr, want) || strings.Contains(got.stdout+got.stderr, "sk-ant-") {
			t.Errorf("switchboard %s --since <a token> = %+v, want it refused as %s, the token hidden", command, got, want)
		}
	}
}

func TestTheLedgersCommandsNeedNeitherTheRouterNorTheLedger(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"requests"}, want: "no requests since Wed 7 Oct 2026 00:00\n"},
		{args: []string{"requests", "--json"}},
		{args: []string{"history"}, want: "no requests since Tue 8 Sep 2026\n"},
		{args: []string{"history", "--json"}, want: "{\n  \"prices_as_of\": \"2026-10-07\",\n  \"days\": []\n}\n"},
	}
	for _, tt := range tests {
		if got := run(t, deps, tt.args...); got != (result{stdout: tt.want}) {
			t.Errorf("switchboard %s = %+v, want it to print\n%s", strings.Join(tt.args, " "), got, tt.want)
		}
	}
}
