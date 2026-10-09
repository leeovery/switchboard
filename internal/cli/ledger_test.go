package cli_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/views"
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
const heldSummary = `{"version":2,"day":"2026-10-04","lines":1,"accounts":[{"account":"work","models":[{"model":"claude-opus-5-5","upstream":1,"no_usage":0,"unsent":0,` +
	`"checks":0,"counts":0,"sessions":1,"usage":{"cache_creation":{"ephemeral_1h_input_tokens":3120,"ephemeral_5m_input_tokens":0},` +
	`"cache_creation_input_tokens":3120,"cache_read_input_tokens":182340,"input_tokens":12,"output_tokens":845}}],"sessions":1,` +
	`"session_ids":["` + sessionA + `"],"moved_on":0,"moved_off":0,"highest":{"5h":0.5},"rise":{"5h":0.2},"resets":[],"minutes_at_cap":0,"minutes_at_limit":0}]}`

// ledgerConfig is the config the ledger's tests run with: work, keeping a
// tenth of every window in reserve, and side.
const ledgerConfig = "[[account]]\nid = \"work\"\nreserve = 0.1\n\n[[account]]\nid = \"side\"\n"

// configure writes ledgerConfig where commands run with deps find their
// config.
func configure(t *testing.T, deps cli.Deps) {
	t.Helper()
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte(ledgerConfig))
}

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
// summary and readings, by a clock stopped at ledgerNow, configured as
// ledgerConfig says, and the JSON of each line written, by its request's id.
func ledgerDeps(t *testing.T) (cli.Deps, map[string]string) {
	t.Helper()
	deps := testDeps(nil, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	configure(t, deps)
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

// requestsFromThe6th is what requests prints of the 6th's lines and today's.
const requestsFromThe6th = `Tue 6 Oct 2026
  22:15:00  5b0e9d33  claude-opus-5-5  side  200  20k in, 50 out  3.1s

` + todaysRequests

// everyRequest is what requests prints of every line the ledger holds.
const everyRequest = `Sun 4 Oct 2026
  09:00:00  5b0e7c1a  claude-opus-5-5  work  200  185k in, 845 out  14.2s

` + requestsFromThe6th

func TestRequestsPrintsTheLedgersLinesOldestFirst(t *testing.T) {
	deps, _ := ledgerDeps(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "today's, unless told otherwise", want: todaysRequests},
		{name: "from the start of a day, under each day's", args: []string{"--since", "2026-10-04"}, want: everyRequest},
		{name: "from a time today", args: []string{"--since", "10:00"}, want: `Wed 7 Oct 2026, so far
  10:00:00  5b0e9d33  claude-opus-5-5  side  200 canceled                  2.0s
  11:00:00  18bb2c41  claude-opus-9    work  200           100 in, 10 out  950ms
  12:00:00  5b0e7c1a  claude-opus-5-5  work  200                           140ms
`},
		{name: "from hours ago", args: []string{"--since", "3h"}, want: `Wed 7 Oct 2026, so far
  11:00:00  18bb2c41  claude-opus-9    work  200  100 in, 10 out  950ms
  12:00:00  5b0e7c1a  claude-opus-5-5  work  200                  140ms
`},
		{name: "from days ago", args: []string{"--since", "2d"}, want: requestsFromThe6th},
		{name: "from the most days ago", args: []string{"--since", "106751d"}, want: everyRequest},
		{name: "from more days ago than a duration holds", args: []string{"--since", "99999999999999999999d"}, want: everyRequest},
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
		for _, form := range prettyForms() {
			t.Run(tt.name+", "+form.name, func(t *testing.T) {
				got := form.run(t, deps, append([]string{"requests"}, tt.args...)...)
				if got.stdout != tt.want || got.code != 0 {
					t.Errorf("switchboard requests %s %s printed\n%s(%d, %s)\nwant\n%s",
						strings.Join(tt.args, " "), strings.Join(form.args, " "), got.stdout, got.code, got.stderr, tt.want)
				}
			})
		}
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
		for _, form := range jsonForms() {
			t.Run(tt.name+", "+form.name, func(t *testing.T) {
				got := form.run(t, deps, append([]string{"requests"}, tt.args...)...)
				if got.stdout != tt.want || got.code != 0 {
					t.Errorf("switchboard requests %s %s printed\n%s(%d, %s)\nwant\n%s",
						strings.Join(tt.args, " "), strings.Join(form.args, " "), got.stdout, got.code, got.stderr, tt.want)
				}
			})
		}
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

func TestRequestsOfASessionReadTheLedgerOnce(t *testing.T) {
	deps, _ := ledgerDeps(t)
	appendStateFile(t, ledger.Dir(stateDir(t, deps)), "requests-2026-10-07.jsonl", []byte("not a line\n"))

	if got := run(t, deps, "requests", "--session", "5b0e7"); got.code != 0 {
		t.Fatalf("switchboard requests --session 5b0e7 = %+v, want exit status 0", got)
	}
	log := readLog(t, deps, "cli.log")
	if n := strings.Count(log, `msg="request ledger lines unread"`); n != 1 {
		t.Errorf("cli.log reads\n%s\nwant the line that doesn't read warned of once, as the ledger is read once, not %d times", log, n)
	}
}

func TestHowLongARequestTookIsAsBriefAsARowHasRoomFor(t *testing.T) {
	tests := []struct {
		ms   int64
		want string
	}{
		{ms: 0, want: "0ms"}, {ms: 999, want: "999ms"}, {ms: 1000, want: "1.0s"}, {ms: 14230, want: "14.2s"},
		{ms: 59949, want: "59.9s"}, {ms: 59950, want: "1m0s"}, {ms: 59999, want: "1m0s"}, {ms: 60000, want: "1m0s"},
		{ms: 60499, want: "1m0s"}, {ms: 60500, want: "1m1s"}, {ms: 3599499, want: "59m59s"}, {ms: 3599500, want: "1h0m0s"},
	}
	for _, tt := range tests {
		if got := cli.Took(tt.ms); got != tt.want {
			t.Errorf("Took(%d) = %q, want %q", tt.ms, got, tt.want)
		}
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
		{since: "+2d", want: `Error: --since "+2d" isn't a day`},
		{since: "99999999999999999999.5d", want: `Error: --since "99999999999999999999.5d" isn't a day`},
		{since: "23:00", want: "Error: --since 23:00 is still to come"},
		{since: "2026-10-08", want: "Error: --since 2026-10-08 is still to come"},
		{since: "", want: `Error: --since "" isn't a day`},
	}
	for _, tt := range tests {
		for _, command := range []string{"requests", "history", "events"} {
			got := run(t, deps, command, "--since", tt.since)
			if got.code != 1 || !strings.Contains(got.stderr, tt.want) || !strings.Contains(got.stdout+got.stderr, "Usage:") {
				t.Errorf("switchboard %s --since %q = %+v, want exit status 1, the usage, and %q", command, tt.since, got, tt.want)
			}
		}
	}
}

func TestSinceATimeTodayIsWhenTheClocksFirstReadItThatDay(t *testing.T) {
	tests := []struct {
		name, zone string
		// now is the time by the clock, in the zone, and want when --since
		// 00:00 starts, in UTC.
		now  [3]int
		want time.Time
	}{
		{name: "as the clocks went forward over midnight", zone: "America/Santiago", now: [3]int{2026, 9, 6}, want: time.Date(2026, 9, 6, 4, 0, 0, 0, time.UTC)},
		{name: "at the first of a midnight that came twice", zone: "Asia/Gaza", now: [3]int{2020, 10, 24}, want: time.Date(2020, 10, 23, 21, 0, 0, 0, time.UTC)},
		{name: "at midnight", zone: "America/Santiago", now: [3]int{2026, 9, 7}, want: time.Date(2026, 9, 7, 3, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := time.LoadLocation(tt.zone)
			if err != nil {
				t.Fatalf("load %s: %v", tt.zone, err)
			}
			now := time.Date(tt.now[0], time.Month(tt.now[1]), tt.now[2], 10, 0, 0, 0, loc)

			if got, err := cli.Since("00:00", now); err != nil || !got.Equal(tt.want) {
				t.Errorf("--since 00:00 at %v starts at %v (%v), want %v, the day's first instant, never the day before", now, got.In(loc), err, tt.want.In(loc))
			}
		})
	}
}

// historyFromThe4th is what history prints of the 4th's table.
const historyFromThe4th = `Sun 4 Oct 2026
  work  claude-opus-5-5  1 request  186k tokens  1 session  $0.08
  work: 1 session  ·  highest: Session 50%

` + historyFromThe6th

// historyFromThe6th is what history prints of the 6th's table and today's.
const historyFromThe6th = `Tue 6 Oct 2026
  side  claude-opus-5-5  1 request  20k tokens  1 session  $0.02
  side: 1 session  ·  1 moved on  ·  highest: Session 10%
  work: no sessions  ·  1 moved off  ·  limits: Session at 22:14  ·  highest: Session 100%

` + historyOfToday

// historyOfToday is what history prints of today's table: side's request,
// canceled, gave no usage, so its worth isn't known; and work's quota check
// and count of tokens are no requests.
const historyOfToday = `Wed 7 Oct 2026, so far
  no account  claude-opus-5-5   1 request   0 tokens     1 session    $0.00
  side        claude-opus-5-5   1 request   0 tokens     1 session    $0.00, part unpriced
  work        claude-haiku-4-5  0 requests  9 tokens     no sessions  $0.00
              claude-opus-5-5   1 request   186k tokens  1 session    $0.08
              claude-opus-9     1 request   110 tokens   1 session    unpriced
  side: 1 session  ·  highest: Session 10%
  work: 2 sessions  ·  highest: Session 30%, Week 41%
`

// historyPricedAt is what history prints last, the prices its worth is at.
const historyPricedAt = `
worth is what they'd have cost through the API, at its prices as of 7 Oct 2026
`

// historyTotalsFromThe6th are the totals history prints over the 6th to
// today: work's requests are its messages; side's session moved onto it is
// one of every account's moves; side's limit is work's alone; work's time at
// its cap and its limit is the 6th's; the share of use is weighed at API
// prices, so work's quota check of Haiku is under 1%.
const historyTotalsFromThe6th = `
Over these days           work                  side                  all accounts
  requests                2                     2                     5
  sessions served         2                     1                     3
  sessions moved          1 out                 1 in                  1 move
  limits hit              Session ×1            none                  1
  at cap or limit         2h 10m                none                  2h 10m
  left at its last reset  Session 0%            Session 90%           -
  plan                    -                     -                     -
  worth                   $0.08, part unpriced  $0.02, part unpriced  $0.09, part unpriced
  against its price       -                     -                     -
  busiest day             Wed 7 Oct             Tue 6 Oct             Wed 7 Oct
  longest run             1 day                 2 days                2 days
  by model
    Opus 5.5              100%                  100%                  100%
    Haiku 4.5             <1%                   -                     <1%
    opus-9                -                     -                     -
`

// The weeks history prints, from the 28th and from this week, of which the
// readings history read no week's reset, so no week's peak is known.
const (
	historyNoWeeksFromThe28th = `
Weeks at their peaks  28 Sep  5 Oct
  work                -       -
  side                -       -
  all accounts        -       -

Weeks: not enough weeks yet
`
	historyNoWeeksThisWeek = `
Weeks at their peaks  5 Oct
  work                -
  side                -
  all accounts        -

Weeks: not enough weeks yet
`
)

func TestHistoryPrintsEachDayByAccountAndModel(t *testing.T) {
	deps, _ := ledgerDeps(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "the last 30 days, of each day the ledger holds", want: historyFromThe4th + `
Over these days           work                  side                  all accounts
  requests                3                     2                     6
  sessions served         2                     1                     3
  sessions moved          1 out                 1 in                  1 move
  limits hit              Session ×1            none                  1
  at cap or limit         2h 10m                none                  2h 10m
  left at its last reset  Session 0%            Session 90%           -
  plan                    -                     -                     -
  worth                   $0.16, part unpriced  $0.02, part unpriced  $0.17, part unpriced
  against its price       -                     -                     -
  busiest day             Wed 7 Oct             Tue 6 Oct             Wed 7 Oct
  longest run             1 day                 2 days                2 days
  by model
    Opus 5.5              100%                  100%                  100%
    Haiku 4.5             <1%                   -                     <1%
    opus-9                -                     -                     -
` + historyNoWeeksFromThe28th + historyPricedAt},
		{name: "from a day", args: []string{"--since", "2026-10-06"}, want: historyFromThe6th + historyTotalsFromThe6th + historyNoWeeksThisWeek + historyPricedAt},
		{name: "from the day of a time today", args: []string{"--since", "10:00"}, want: historyOfToday + `
Over these days           work                  side                  all accounts
  requests                2                     1                     4
  sessions served         2                     1                     3
  sessions moved          none                  none                  none
  limits hit              none                  none                  none
  at cap or limit         none                  none                  none
  left at its last reset  -                     Session 90%           -
  plan                    -                     -                     -
  worth                   $0.08, part unpriced  $0.00, part unpriced  $0.08, part unpriced
  against its price       -                     -                     -
  busiest day             Wed 7 Oct             Wed 7 Oct             Wed 7 Oct
  longest run             1 day                 1 day                 1 day
  by model
    Opus 5.5              100%                  -                     100%
    Haiku 4.5             <1%                   -                     <1%
    opus-9                -                     -                     -
` + historyNoWeeksThisWeek + historyPricedAt},
		{name: "from the day of days ago", args: []string{"--since", "2d"}, want: historyFromThe6th + historyTotalsFromThe6th + historyNoWeeksThisWeek + historyPricedAt},
	}
	for _, tt := range tests {
		for _, form := range prettyForms() {
			t.Run(tt.name+", "+form.name, func(t *testing.T) {
				got := form.run(t, deps, append([]string{"history"}, tt.args...)...)
				if got.stdout != tt.want || got.code != 0 {
					t.Errorf("switchboard history %s %s printed\n%s(%d, %s)\nwant\n%s",
						strings.Join(tt.args, " "), strings.Join(form.args, " "), got.stdout, got.code, got.stderr, tt.want)
				}
			})
		}
	}
}

// historyBlocks are the blocks history's JSON gives over the 4th to today,
// after the days, as ledgerConfig configures the accounts, on no plan, and
// starts the weeks on Monday: today's quota check and count of tokens are no
// requests; work's check is of claude-haiku-4-5, its version's alias. The
// readings history read no week's reset, so no week's peak is known, and
// work's limit of the 6th is in the calendar week it was reached in.
const historyBlocks = `"year":{"cuts":[1,1,4],"days":[{"day":"2026-10-04","requests":1,"level":3},{"day":"2026-10-05","requests":0,"level":0},` +
	`{"day":"2026-10-06","requests":1,"level":3},{"day":"2026-10-07","requests":4,"level":4}]},` +
	`"months":[{"month":"2026-10","requests":6,"worth":0.171785,"unpriced":["claude-opus-9","no_usage"],"limits":{"5h":1},` +
	`"by_family":[{"family":"opus","worth":0.171772,"unpriced":["claude-opus-9","no_usage"]},{"family":"haiku","worth":0.000013}]}],` +
	`"weeks":[{"week":"2026-09-28","accounts":[{"account":"work"},{"account":"side"},{"all":true}]},` +
	`{"week":"2026-10-05","accounts":[{"account":"work","limits":{"5h":1}},{"account":"side"},{"all":true}]}],` +
	`"tokens":[{"week":"2026-09-28","input":12,"output":845,"cache_write":3120,"cache_read":182340,"total":186317,"worth":0.078376,` +
	`"by_account":[{"account":"work","input":12,"output":845,"cache_write":3120,"cache_read":182340,"total":186317,"worth":0.078376,` +
	`"by_model":[{"model":"claude-opus-5-5","input":12,"output":845,"cache_write":3120,"cache_read":182340,"total":186317,"worth":0.078376}]}],` +
	`"by_model":[{"model":"claude-opus-5-5","on":["work"],"input":12,"output":845,"cache_write":3120,"cache_read":182340,"total":186317,"worth":0.078376}]},` +
	`{"week":"2026-10-05","input":125,"output":906,"cache_write":3120,"cache_read":202340,"total":206491,"worth":0.093409,"unpriced":["claude-opus-9","no_usage"],` +
	`"by_account":[{"input":0,"output":0,"cache_write":0,"cache_read":0,"total":0,"worth":0,` +
	`"by_model":[{"model":"claude-opus-5-5","input":0,"output":0,"cache_write":0,"cache_read":0,"total":0,"worth":0}]},` +
	`{"account":"side","input":5,"output":50,"cache_write":0,"cache_read":20000,"total":20055,"worth":0.01502,"unpriced":["no_usage"],` +
	`"by_model":[{"model":"claude-opus-5-5","input":5,"output":50,"cache_write":0,"cache_read":20000,"total":20055,"worth":0.01502,"unpriced":["no_usage"]}]},` +
	`{"account":"work","input":120,"output":856,"cache_write":3120,"cache_read":182340,"total":186436,"worth":0.078389,"unpriced":["claude-opus-9"],` +
	`"by_model":[{"model":"claude-opus-5-5","input":12,"output":845,"cache_write":3120,"cache_read":182340,"total":186317,"worth":0.078376},` +
	`{"model":"claude-haiku-4-5","input":8,"output":1,"cache_write":0,"cache_read":0,"total":9,"worth":0.000013},` +
	`{"model":"claude-opus-9","input":100,"output":10,"cache_write":0,"cache_read":0,"total":110,"unpriced":["claude-opus-9"]}]}],` +
	`"by_model":[{"model":"claude-opus-5-5","on":["side","work"],"input":17,"output":895,"cache_write":3120,"cache_read":202340,"total":206372,` +
	`"worth":0.093396,"unpriced":["no_usage"]},` +
	`{"model":"claude-haiku-4-5","on":["work"],"input":8,"output":1,"cache_write":0,"cache_read":0,"total":9,"worth":0.000013},` +
	`{"model":"claude-opus-9","on":["work"],"input":100,"output":10,"cache_write":0,"cache_read":0,"total":110,"unpriced":["claude-opus-9"]}]}],` +
	// Every account's counts the requests no account answered, and session B
	// once, though it ran on side and on none.
	`"totals":{"accounts":[{"account":"work","requests":3,"sessions":2,"moved_on":0,"moved_off":1,"limits":{"5h":1},"minutes_at_cap":74,"minutes_at_limit":56,` +
	`"left_at_reset":{"5h":0},"worth":0.156765,"unpriced":["claude-opus-9"],"by_model":[{"model":"claude-opus-5-5","share":0.9999170733263164,"worth":0.156752},` +
	`{"model":"claude-haiku-4-5","share":0.00008292667368353906,"worth":0.000013},{"model":"claude-opus-9","unpriced":["claude-opus-9"]}],` +
	`"busiest_day":"2026-10-07","longest_run":1},` +
	`{"account":"side","requests":2,"sessions":1,"moved_on":1,"moved_off":0,"minutes_at_cap":0,"minutes_at_limit":0,"left_at_reset":{"5h":0.9},` +
	`"worth":0.01502,"unpriced":["no_usage"],"by_model":[{"model":"claude-opus-5-5","share":1,"worth":0.01502,"unpriced":["no_usage"]}],` +
	`"busiest_day":"2026-10-06","longest_run":2},` +
	`{"all":true,"requests":6,"sessions":3,"moved_on":1,"moved_off":1,"limits":{"5h":1},"minutes_at_cap":74,"minutes_at_limit":56,` +
	`"worth":0.171785,"unpriced":["claude-opus-9","no_usage"],"by_model":[{"model":"claude-opus-5-5","share":0.9999243240096632,"worth":0.171772,"unpriced":["no_usage"]},` +
	`{"model":"claude-haiku-4-5","share":0.00007567599033675815,"worth":0.000013},{"model":"claude-opus-9","unpriced":["claude-opus-9"]}],` +
	`"busiest_day":"2026-10-07","longest_run":2}]},` +
	`"plans":[{"account":"work","worth":0.156765,"unpriced":["claude-opus-9"]},{"account":"side","worth":0.01502,"unpriced":["no_usage"]},{"all":true}],` +
	`"capacity":{"weeks":0,"short":0,"with_one_fewer":0,"with_one_more":0,"accounts":[` +
	`{"account":"work","weeks":0,"limits":0,"without_it":0},{"account":"side","weeks":0,"limits":0,"without_it":0},{"all":true,"weeks":0,"limits":0}]}`

func TestHistoryJSONPrintsEachDaysSummaryWithEachModelsWorth(t *testing.T) {
	deps, _ := ledgerDeps(t)
	limit, sideReset := fixtureReadings[1], fixtureReadings[2]
	// at is t as the JSON gives it.
	at := func(t time.Time) string { return t.UTC().Format(time.RFC3339) }
	want := `{"prices_as_of":"2026-10-07","days":[` +
		`{"version":2,"day":"2026-10-04","lines":1,"accounts":[{"account":"work","sessions":1,"session_ids":["` + sessionA + `"],"moved_on":0,"moved_off":0,` +
		`"highest":{"5h":0.5},"rise":{"5h":0.2},"resets":[],"minutes_at_cap":0,"minutes_at_limit":0,` +
		`"models":[{"model":"claude-opus-5-5","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"cache_creation":` +
		`{"ephemeral_1h_input_tokens":3120,"ephemeral_5m_input_tokens":0},"cache_creation_input_tokens":3120,"cache_read_input_tokens":182340,` +
		`"input_tokens":12,"output_tokens":845},"worth":0.078376}]}],"by_family":[{"family":"opus","worth":0.078376}]},` +
		`{"version":2,"day":"2026-10-05","lines":0},` +
		`{"version":2,"day":"2026-10-06","lines":1,"accounts":[{"account":"side","sessions":1,"session_ids":["` + sessionB + `"],"moved_on":1,"moved_off":0,` +
		`"highest":{"5h":0.1},"rise":{"5h":0.1},"resets":[],"minutes_at_cap":0,"minutes_at_limit":0,` +
		`"models":[{"model":"claude-opus-5-5","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"cache_read_input_tokens":20000,` +
		`"input_tokens":5,"output_tokens":50,"server_tool_use":{"web_search_requests":1}},"worth":0.01502}]},` +
		// Work at its cap, by its reserve, from 21:00, and at its limit from
		// 22:14 to its session's reset.
		`{"account":"work","sessions":0,"session_ids":[],"moved_on":0,"moved_off":1,"highest":{"5h":1},"rise":{"5h":1},` +
		`"resets":[{"window":"5h","at":"` + at(limit.ResetsAt) + `","before":1}],` +
		`"limits":[{"window":"5h","at":"` + at(limit.At) + `","resets_at":"` + at(limit.ResetsAt) + `"}],"minutes_at_cap":74,"minutes_at_limit":56}],` +
		`"by_family":[{"family":"opus","worth":0.01502}]},` +
		`{"version":2,"day":"2026-10-07","lines":6,"accounts":[` +
		// The requests no account answered, of no window the readings
		// history held.
		`{"sessions":1,"session_ids":["` + sessionB + `"],"moved_on":0,"moved_off":0,` +
		`"models":[{"model":"claude-opus-5-5","upstream":0,"no_usage":0,"unsent":1,"checks":0,"counts":0,"sessions":1,"worth":0}]},` +
		`{"account":"side","sessions":1,"session_ids":["` + sessionB + `"],"moved_on":0,"moved_off":0,"highest":{"5h":0.1},"rise":{"5h":0.05},` +
		`"resets":[{"window":"5h","at":"` + at(sideReset.ResetsAt) + `","before":0.1}],"minutes_at_cap":0,"minutes_at_limit":0,"read_before":["5h"],` +
		`"models":[{"model":"claude-opus-5-5","upstream":1,"no_usage":1,"unsent":0,"checks":0,"counts":0,"sessions":1,"worth":0,"unpriced":["no_usage"]}]},` +
		`{"account":"work","sessions":2,"session_ids":["` + sessionC + `","` + sessionA + `"],"moved_on":0,"moved_off":0,"highest":{"5h":0.3,"7d":0.41},` +
		`"rise":{"5h":0.3,"7d":0},"resets":[],"minutes_at_cap":0,"minutes_at_limit":0,"read_before":["5h"],"models":[` +
		`{"model":"claude-haiku-4-5","upstream":0,"no_usage":0,"unsent":0,"checks":1,"counts":0,"sessions":0,"usage":{"input_tokens":8,"output_tokens":1},` +
		`"worth":0.000013},` +
		`{"model":"claude-opus-5-5","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":1,"sessions":1,"usage":{"cache_creation":` +
		`{"ephemeral_1h_input_tokens":3120,"ephemeral_5m_input_tokens":0},"cache_creation_input_tokens":3120,"cache_read_input_tokens":182340,` +
		`"input_tokens":12,"output_tokens":845},"worth":0.078376},` +
		`{"model":"claude-opus-9","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"input_tokens":100,"output_tokens":10}}]}],` +
		// Of Opus, claude-opus-9, which the price table doesn't know, and
		// side's request that gave no usage are unpriced.
		`"by_family":[{"family":"opus","worth":0.078376,"unpriced":["claude-opus-9","no_usage"]},{"family":"haiku","worth":0.000013}]}],` +
		historyBlocks + `}`
	for _, form := range jsonForms() {
		t.Run(form.name, func(t *testing.T) {
			got := form.run(t, deps, "history", "--since", "2026-10-04")
			var compact bytes.Buffer
			if err := json.Compact(&compact, []byte(got.stdout)); err != nil || got.code != 0 || !strings.HasPrefix(got.stdout, "{\n  \"prices_as_of\"") {
				t.Fatalf("switchboard history %s = %+v (%v), want indented JSON", strings.Join(form.args, " "), got, err)
			}
			if compact.String() != want {
				t.Errorf("switchboard history %s printed\n%s\nwant\n%s", strings.Join(form.args, " "), compact.String(), want)
			}
		})
	}
}

func TestHistoryJSONPricesThePlansAndStartsTheWeeksAsTheConfigSays(t *testing.T) {
	deps, _ := ledgerDeps(t)
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := "week_starts = \"sunday\"\n\n[[account]]\nid = \"work\"\nreserve = 0.1\nplan = \"max20x\"\n\n[[account]]\nid = \"side\"\n"
	writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte(cfg))

	got := run(t, deps, "history", "--json")
	var doc struct {
		Tokens []struct {
			Week string `json:"week"`
		} `json:"tokens"`
		Plans json.RawMessage `json:"plans"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil || got.code != 0 {
		t.Fatalf("switchboard history --json = %+v (%v), want the blocks", got, err)
	}
	var weeks []string
	for _, week := range doc.Tokens {
		weeks = append(weeks, week.Week)
	}
	if want := []string{"2026-10-04"}; !slices.Equal(weeks, want) {
		t.Errorf("switchboard history --json gives the weeks %q, want %q: the 4th to today, a week from Sunday", weeks, want)
	}
	// Over the last 30 days, work's plan costs its month, and side's isn't
	// known.
	want := `[{"account":"work","plan":"max20x","price":200,"cost":200,"against":0.000783825,"worth":0.156765,"unpriced":["claude-opus-9"]},` +
		`{"account":"side","worth":0.01502,"unpriced":["no_usage"]},{"all":true,"price":200,"cost":200,"against":0.000783825,"worth":0.156765,"unpriced":["claude-opus-9"]}]`
	var plans bytes.Buffer
	if err := json.Compact(&plans, doc.Plans); err != nil || plans.String() != want {
		t.Errorf("switchboard history --json gives the plans\n%s (%v)\nwant\n%s", plans.String(), err, want)
	}
}

func TestHistoryPricesAtTheConfigsPricesWhereItGivesAny(t *testing.T) {
	tests := []struct {
		name   string
		prices string
		// worth is what work's requests of Claude Opus 5.5 today are worth,
		// as the text gives it and as the JSON does.
		worth, worthJSON string
	}{
		{name: "a model's, in place of the table's", prices: "\n[prices.models.claude-opus-5-5]\noutput = 0\n", worth: "$0.06", worthJSON: "0.061476"},
		{name: "a plan's alone, the models' as the table prices them", prices: "\n[prices.plans]\nmax5x = 90\n", worth: "$0.08", worthJSON: "0.078376"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _ := ledgerDeps(t)
			path, err := config.Path(deps.Getenv, deps.HomeDir)
			if err != nil {
				t.Fatal(err)
			}
			writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte(ledgerConfig+tt.prices))

			got := run(t, deps, "history", "--pretty", "--since", "10:00")
			table := strings.Replace(historyOfToday, "$0.08", tt.worth, 1)
			pricedAt := strings.Replace(historyPricedAt, "as of 7 Oct 2026\n", "as of 7 Oct 2026 and the config's\n", 1)
			if !strings.HasPrefix(got.stdout, table) || !strings.HasSuffix(got.stdout, pricedAt) || got.code != 0 {
				t.Errorf("switchboard history printed\n%s(%d, %s)\nwant today's table\n%s\nand last%s", got.stdout, got.code, got.stderr, table, pricedAt)
			}

			got = run(t, deps, "history", "--json", "--since", "10:00")
			var doc struct {
				PricesAsOf       string `json:"prices_as_of"`
				PricesFromConfig *bool  `json:"prices_from_config"`
				Days             []struct {
					Accounts []struct {
						Account string `json:"account"`
						Models  []struct {
							Model string          `json:"model"`
							Worth json.RawMessage `json:"worth"`
						} `json:"models"`
					} `json:"accounts"`
				} `json:"days"`
			}
			if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil || got.code != 0 || len(doc.Days) != 1 {
				t.Fatalf("switchboard history --json = %+v (%v), want today", got, err)
			}
			var worth string
			for _, a := range doc.Days[0].Accounts {
				for _, m := range a.Models {
					if a.Account == "work" && m.Model == "claude-opus-5-5" {
						worth = string(m.Worth)
					}
				}
			}
			if doc.PricesAsOf != "2026-10-07" || doc.PricesFromConfig == nil || !*doc.PricesFromConfig || worth != tt.worthJSON {
				t.Errorf("switchboard history --json gives prices as of %s, from the config: %v, and work's Claude Opus 5.5 worth %s, "+
					"want 2026-10-07, from the config, and %s", doc.PricesAsOf, doc.PricesFromConfig, worth, tt.worthJSON)
			}
		})
	}
}

func TestHistoryJSONSaysNothingOfTheConfigsPricesWhereNoneStandIn(t *testing.T) {
	for _, prices := range []string{"", "\n[prices.models.claude-test-1]\ninput = 1\n"} {
		deps, _ := ledgerDeps(t)
		path, err := config.Path(deps.Getenv, deps.HomeDir)
		if err != nil {
			t.Fatal(err)
		}
		writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte(ledgerConfig+prices))

		got := run(t, deps, "history", "--json")
		var doc map[string]json.RawMessage
		if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil || got.code != 0 {
			t.Fatalf("switchboard history --json = %+v (%v), want the days", got, err)
		}
		if given, ok := doc["prices_from_config"]; ok {
			t.Errorf("with the config's prices%s, switchboard history --json gives prices_from_config %s, want it left out", prices, given)
		}
	}
}

func TestHistoryWarnsOnceOfAModelTheConfigCantPrice(t *testing.T) {
	deps, _ := ledgerDeps(t)
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	every := "input = 1\noutput = 5\ncache_read = 0.1\ncache_write_5m = 1.25\ncache_write_1h = 2\n"
	writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte(ledgerConfig+
		"\n[prices.models.claude-test-1]\ninput = 1\n"+
		"\n[prices.models.\""+tokenShaped+"\"]\noutput = 2\n"+
		"\n[prices.models.claude-test-2]\n"+every))
	const warning = `msg="the config prices a model the price table doesn't know, but not all five of its prices, so it's left unpriced"`

	if got := run(t, deps, "requests"); got.code != 0 {
		t.Fatalf("switchboard requests = %+v, want it to succeed", got)
	}
	if log := readLog(t, deps, "cli.log"); strings.Contains(log, warning) {
		t.Errorf("after requests, which prices nothing, cli.log reads\n%s\nwant no warning of the config's prices", log)
	}

	if got := run(t, deps, "history"); got.code != 0 || strings.Contains(got.stdout+got.stderr, "sk-ant-") {
		t.Fatalf("switchboard history = %+v, want it to succeed, quoting no token", got)
	}
	log := readLog(t, deps, "cli.log")
	var warned []string
	for line := range strings.Lines(log) {
		if strings.Contains(line, warning) {
			warned = append(warned, line)
		}
	}
	needs := `needs="input, output, cache_read, cache_write_5m and cache_write_1h"`
	if strings.Contains(log, "sk-ant-") || len(warned) != 2 ||
		!hasLine(warned[0], "level=WARN", "model=claude-test-1", needs) || !hasLine(warned[1], "level=WARN", "model=[redacted]", needs) {
		t.Errorf("cli.log reads\n%s\nwant a warning each, once, of the models given some prices, the token hidden, and none of the model given all five", log)
	}
}

func TestHistoryReportsASummaryOfADayThatIsntOne(t *testing.T) {
	work := ledger.PricedAccount{Account: "work", Sessions: 1, Models: []ledger.PricedModel{{Model: "claude-opus-5-5", Upstream: 1, Sessions: 1}}}
	days := []views.Day{
		{Version: 1, Day: "2026-10-06", Lines: 1, Accounts: []ledger.PricedAccount{work}},
		{Version: 1, Day: "2026-13-45", Lines: 1, Accounts: []ledger.PricedAccount{work}},
	}

	var out bytes.Buffer
	err := cli.WriteHistory(&out, views.History{Days: days}, ledger.Pricing, october(6, 0, 0, 0), ledgerNow)
	if err == nil || !strings.Contains(err.Error(), `"2026-13-45"`) || out.Len() > 0 {
		t.Errorf("history of a summary of 2026-13-45 printed\n%s(%v)\nwant nothing printed, and the summary's day reported", out.String(), err)
	}
}

func TestTheLedgersCommandsNeverEchoAToken(t *testing.T) {
	deps, _ := ledgerDeps(t)
	// Of a token given as an id, the text says there are no requests, and the
	// JSON is none.
	for _, flag := range []string{"--session", "--account"} {
		for _, tt := range []struct {
			forms []printForm
			want  result
		}{
			{forms: prettyForms(), want: result{stdout: "no requests since Wed 7 Oct 2026 00:00\n"}},
			{forms: jsonForms()},
		} {
			for _, form := range tt.forms {
				t.Run(flag+" <a token>, "+form.name, func(t *testing.T) {
					if got := form.run(t, deps, "requests", flag, tokenShaped); got != tt.want {
						t.Errorf("switchboard requests %s <a token> %s = %+v, want %+v", flag, strings.Join(form.args, " "), got, tt.want)
					}
				})
			}
		}
	}
	for _, command := range []string{"requests", "history", "events"} {
		refusals := []struct {
			name string
			args []string
			want string
		}{
			{name: "--since <a token>", args: []string{"--since", tokenShaped}, want: `Error: --since "[redacted]" isn't a day`},
			{name: "<a token>", args: []string{tokenShaped}, want: `Error: unknown command "[redacted]" for "switchboard ` + command + `"`},
		}
		for _, tt := range refusals {
			got := run(t, deps, append([]string{command}, tt.args...)...)
			if got.code != 1 || !strings.Contains(got.stderr, tt.want) || strings.Contains(got.stdout+got.stderr, "sk-ant-") {
				t.Errorf("switchboard %s %s = %+v, want it refused as %s, the token hidden", command, tt.name, got, tt.want)
			}
		}
	}
}

func TestTheLedgersCommandsNeedNeitherTheRouterNorTheLedger(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	configure(t, deps)
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"requests", "--pretty"}, want: "no requests since Wed 7 Oct 2026 00:00\n"},
		{args: []string{"requests", "--json"}},
		{args: []string{"history", "--pretty"}, want: "no requests since Tue 8 Sep 2026\n"},
	}
	for _, tt := range tests {
		if got := run(t, deps, tt.args...); got != (result{stdout: tt.want}) {
			t.Errorf("switchboard %s = %+v, want it to print\n%s", strings.Join(tt.args, " "), got, tt.want)
		}
	}

	got := run(t, deps, "history", "--json")
	var doc struct {
		PricesAsOf string            `json:"prices_as_of"`
		Days       []json.RawMessage `json:"days"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil || got.code != 0 || got.stderr != "" || doc.PricesAsOf != "2026-10-07" {
		t.Fatalf("switchboard history --json = %+v (%v), want the days, priced as of 2026-10-07", got, err)
	}
	var days []string
	for _, day := range doc.Days {
		var compact bytes.Buffer
		if err := json.Compact(&compact, day); err != nil {
			t.Fatal(err)
		}
		days = append(days, compact.String())
	}
	if want := []string{`{"version":2,"day":"2026-10-07","lines":0}`}; !slices.Equal(days, want) {
		t.Errorf("switchboard history --json printed the days\n%s\nwant today's alone, of no requests: there's no ledger to hold a day before it", strings.Join(days, "\n"))
	}
}

func TestTheLedgersCommandsNeedAConfig(t *testing.T) {
	deps, _ := ledgerDeps(t)
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"requests", "history"} {
		got := run(t, deps, command)
		if want := "Error: no config file at " + path + "\n"; got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, want) {
			t.Errorf("switchboard %s = %+v, want it to fail, printing nothing, as %q: a summary counts the accounts' minutes at their caps by their reserves",
				command, got, want)
		}
	}
}

func TestHistoryHelpSaysWhichDaysItsJSONGives(t *testing.T) {
	got := run(t, testDeps(nil, t.TempDir()), "history", "--help")
	help := strings.Join(strings.Fields(got.stdout), " ")
	want := "it prints as JSON the summaries of the days asked for, as the ledger holds them, from the first it holds, so the last is today's"
	if got.code != 0 || !strings.Contains(help, want) || strings.Contains(help, "every day since the ledger began") {
		t.Errorf("switchboard history --help = %+v, want help saying %q: the last 30 days, or --since's, not every day the ledger holds", got, want)
	}
}

func TestHistoryJSONGivesNoDayBeforeTheLedgerBegan(t *testing.T) {
	deps, _ := ledgerDeps(t)
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "the last 30 days, from the first the ledger holds", want: []string{"2026-10-04", "2026-10-05", "2026-10-06", "2026-10-07"}},
		{name: "from a day before the first it holds", args: []string{"--since", "2026-09-01"}, want: []string{"2026-10-04", "2026-10-05", "2026-10-06", "2026-10-07"}},
		{name: "from the most days ago", args: []string{"--since", "106751d"}, want: []string{"2026-10-04", "2026-10-05", "2026-10-06", "2026-10-07"}},
		{name: "from more days ago than a duration holds", args: []string{"--since", "106752d"}, want: []string{"2026-10-04", "2026-10-05", "2026-10-06", "2026-10-07"}},
		{name: "from a day after its first", args: []string{"--since", "2026-10-06"}, want: []string{"2026-10-06", "2026-10-07"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, deps, append([]string{"history", "--json"}, tt.args...)...)
			var doc struct {
				Days []struct {
					Day string `json:"day"`
				} `json:"days"`
			}
			if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil || got.code != 0 {
				t.Fatalf("switchboard history --json %s = %+v (%v), want the days", strings.Join(tt.args, " "), got, err)
			}
			var days []string
			for _, day := range doc.Days {
				days = append(days, day.Day)
			}
			if !slices.Equal(days, tt.want) {
				t.Errorf("switchboard history --json %s gave the days %q, want %q", strings.Join(tt.args, " "), days, tt.want)
			}
		})
	}
}
