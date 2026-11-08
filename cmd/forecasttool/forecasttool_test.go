package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

// account is an account as a test's config configures it.
type account struct {
	id      string
	reserve float64
}

// configOf is a config file of the test's own, configuring accounts, in
// their order.
func configOf(t *testing.T, accounts ...account) string {
	t.Helper()
	var b strings.Builder
	for _, a := range accounts {
		b.WriteString("[[account]]\nid = \"" + a.id + "\"\n")
		if a.reserve > 0 {
			b.WriteString("reserve = " + strconv.FormatFloat(a.reserve, 'f', -1, 64) + "\n")
		}
		b.WriteString("\n")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// stateOf is a state directory of the test's own whose readings history
// holds read, and whose request ledger holds lines, each written by its
// package's own writer, by a clock stopped at now, of the accounts the
// config at configPath configures.
func stateOf(t *testing.T, now time.Time, configPath string, read []readings.Reading, lines []*ledger.Line) string {
	t.Helper()
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	clock := func() time.Time { return now }
	files := readings.Files(readings.Dir(dir), logger)
	history := readings.Open(files, config.DefaultHistoryKeep, clock, nil)
	history.Note(read)
	runStopped(t, history.Run)
	requests := ledger.Open(ledger.Dir(dir), config.DefaultLedgerKeep, clock, ledger.HistoryIn(files), ledger.CapsOf(cfg.Accounts, claude.SharedWindows()), logger)
	for _, l := range lines {
		requests.Note(l)
	}
	runStopped(t, requests.Run)
	return dir
}

// runStopped has run run with its context ended, as a writer's Run writes
// what's queued, and returns.
func runStopped(t *testing.T, run func(context.Context)) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	run(ctx)
}

// hourly are the readings of the account's week, resetting at resets, at
// from and each hour after it, n in all, its use rising by a thousandth an
// hour from used.
func hourly(id string, from time.Time, n int, used float64, resets time.Time) []readings.Reading {
	read := make([]readings.Reading, n)
	for i := range read {
		read[i] = weekRead(id, from.Add(time.Duration(i)*time.Hour), used+float64(i)/1000, resets)
	}
	return read
}

// messageOf is the line of a message of session one, on opus, that arrived
// at at and was answered on work, its usage a thousand tokens in.
func messageOf(at time.Time) *ledger.Line {
	return &ledger.Line{
		At: at, Request: "r" + strconv.FormatInt(at.Unix(), 10), Kind: ledger.KindMessage, Session: "one", Model: "claude-opus-5-5",
		Account: "work", Status: http.StatusOK, Attempts: 1, Usage: json.RawMessage(`{"input_tokens":1000}`),
	}
}

// scoring runs the tool on args, by a clock at now, and returns what it
// printed, failing on an error.
func scoring(t *testing.T, now time.Time, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if err := run(args, &stdout, &stderr, func() time.Time { return now }); err != nil {
		t.Fatalf("run(%q) error = %v; stderr:\n%s", args, err, stderr.String())
	}
	return stdout.String()
}

// shortHistory is a history of three accounts' weeks, all forecast short:
// work's, 10% used two days in, resetting 120 hours in, rises a thousandth an
// hour till a reading 60 hours in finds it run out; side's, half used five
// days in, rises a thousandth an hour to its reset, 48 hours in, short, and
// is read again an hour later; personal's, a fifth used, resets after the
// history ends, short. A message of work's session answers every hour. It
// returns the config and the state directory, and the time after it.
func shortHistory(t *testing.T) (configPath, stateDir string, now time.Time) {
	t.Helper()
	read := hourly("work", begins, 60, 0.1, hoursIn(120))
	read = append(read, readings.Reading{At: hoursIn(60), Account: "work", Key: "7d", Utilization: 1, ResetsAt: hoursIn(120), Status: quota.StatusRejected})
	read = append(read, hourly("side", begins, 48, 0.5, hoursIn(48))...)
	read = append(read, weekRead("side", hoursIn(49), 0.01, hoursIn(48+7*24)), weekRead("personal", begins, 0.2, hoursIn(144)))
	var lines []*ledger.Line
	for i := range 60 {
		lines = append(lines, messageOf(hoursIn(float64(i))))
	}
	configPath = configOf(t, account{id: "work"}, account{id: "side"}, account{id: "personal"})
	now = hoursIn(70)
	return configPath, stateOf(t, now, configPath, read, lines), now
}

func TestScoresAWeekForecastShortThatRanOutAndOneThatHeadedShort(t *testing.T) {
	// Work's week is forecast short at each hour of its 59 before it runs
	// out, 60 hours before its reset: 60 hours late, all in its first 40%.
	// Side's is forecast short at each of its 47 before its reset, as it was:
	// on time, all in 40–70%. Personal's resets after the history ends, and
	// side's next week too, so they're never scored. Each week heads short
	// at its pace since it started too.
	configPath, stateDir, now := shortHistory(t)

	got := scoring(t, now, "--state", stateDir, "--config", configPath)
	scores := `  0–40% used                59   60.0h     0%   100%    0.0h
  40–70% used               47    0.0h     0%     0%    0.0h
  70–100% used               0       –      –      –       –
  all                      106   33.4h     0%    56%    0.0h
`
	want := "forecasts of each week at each hour, 2026-09-07 01:00 UTC to 2026-09-09 12:00 UTC, of work, side, personal\n\n" +
		"                     forecasts     off  early   late   moved\n" +
		"the router's forecast\n" + scores + "pace since start\n" + scores + "\n" + legend
	if got != want {
		t.Errorf("printed\n%s\nwant\n%s", got, want)
	}
}

func TestScoresAWeekThatRunsOutBesideItsPaceSinceItStarted(t *testing.T) {
	// Work's week, two days in, resetting 120 hours in, is read 35.5% used,
	// and every minute after, a point an hour, till it runs out 64.5 hours in;
	// a message of its session answers every ten minutes. Side, keeping half
	// back, is half used, and personal used up, both resetting after work's
	// week, so the pool's use is work's alone.
	var read []readings.Reading
	for k := range 3871 {
		read = append(read, weekRead("work", begins.Add(time.Duration(k)*time.Minute), float64(2130+k)/6000, hoursIn(120)))
	}
	read = append(read, weekRead("side", begins, 0.5, hoursIn(130)),
		readings.Reading{At: begins, Account: "personal", Key: "7d", Utilization: 1, ResetsAt: hoursIn(130), Status: quota.StatusRejected})
	var lines []*ledger.Line
	for at := begins; at.Before(hoursIn(64.5)); at = at.Add(10 * time.Minute) {
		lines = append(lines, messageOf(at))
	}
	configPath := configOf(t, account{id: "work"}, account{id: "side", reserve: 0.5}, account{id: "personal"})
	now := hoursIn(70)
	stateDir := stateOf(t, now, configPath, read, lines)

	var got report
	if err := json.Unmarshal([]byte(scoring(t, now, "--state", stateDir, "--config", configPath, "--json")), &got); err != nil {
		t.Fatal(err)
	}
	if !got.From.Equal(hoursIn(1)) || !got.To.Equal(hoursIn(64)) {
		t.Errorf("it forecast from %v to %v, want an hour in to 64 hours in, the hour of the last reading", got.From, got.To)
	}
	// The router's forecasts, at each of the 64 hours, all work's: at a point
	// an hour, the day's rate, they run out within 2 minutes of 64.5 hours
	// in, moving under 2 minutes an hour on average, but for the first
	// four, the router's rates still taking up the history: at the first,
	// with an hour of it, at the fast rate alone, 0.637% an hour, 36.12 hours
	// late; at the next three, the fast rate below the day's, 0.38, 0.13 and
	// 0.03 hours late.
	forecast := got.Forecast
	tests := []struct {
		name        string
		got         row
		forecasts   int
		late, moves int
		off, moved  float64
		within      float64
	}{
		{name: "0–40%", got: forecast.ByUse[0], forecasts: 4, late: 1, moves: 3, off: (36.123 + 0.384 + 0.126 + 0.030) / 4, moved: (35.739 + 0.258 + 0.096) / 3, within: 0.05},
		{name: "40–70%", got: forecast.ByUse[1], forecasts: 30, moves: 30, within: 0.03},
		{name: "70–100%", got: forecast.ByUse[2], forecasts: 30, moves: 30, within: 0.03},
	}
	for _, tt := range tests {
		t.Run("the router's "+tt.name, func(t *testing.T) {
			r := tt.got
			if r.Forecasts != tt.forecasts || r.Early != 0 || r.Late != tt.late || r.Moves != tt.moves ||
				math.Abs(r.OffHours-tt.off) > tt.within || math.Abs(r.MovedHours-tt.moved) > tt.within {
				t.Errorf("scored %+v, want %d forecasts, none early, %d late, off %.3f hours, %d moves of %.3f hours, each to within %v",
					r, tt.forecasts, tt.late, tt.off, tt.moves, tt.moved, tt.within)
			}
		})
	}
	// The pace since the week started, two days before the history, 35.5% in
	// those two days, then a point an hour, runs out at j + (48 + j)(1 − u)/u
	// hours in at the jth hour, u used: 21.75 hours late at the first, 11.77 at the
	// 16th, and so late by more than 12 hours at the first 15.
	baseline := sheet{
		ByUse: []row{
			{UsedFrom: 0, UsedTo: 0.4, Forecasts: 4, OffHours: 20.423252690896177, Late: 4, Moves: 3, MovedHours: 0.8670019074042585},
			{UsedFrom: 0.4, UsedTo: 0.7, Forecasts: 30, OffHours: 10.816593842242723, Late: 11, Moves: 30, MovedHours: 0.45533193698205565},
			{UsedFrom: 0.7, UsedTo: 1, Forecasts: 30, OffHours: 2.36127530978274, Moves: 30, MovedHours: 0.1807599146813241},
		},
		All: row{UsedFrom: 0, UsedTo: 1, Forecasts: 64, OffHours: 7.453579458192946, Late: 15, Moves: 63, MovedHours: 0.34418668685895504},
	}
	if !sameSheet(got.Baseline, baseline) {
		t.Errorf("the pace since start scored\n%+v\nwant\n%+v", got.Baseline, baseline)
	}
}

func TestPrintsTheScoresAsJSONWithJSON(t *testing.T) {
	configPath, stateDir, now := shortHistory(t)

	var got report
	if err := json.Unmarshal([]byte(scoring(t, now, "--state", stateDir, "--config", configPath, "--json")), &got); err != nil {
		t.Fatal(err)
	}
	scored := sheet{
		ByUse: []row{
			{UsedFrom: 0, UsedTo: 0.4, Forecasts: 59, OffHours: 60, Late: 59, Moves: 58},
			{UsedFrom: 0.4, UsedTo: 0.7, Forecasts: 47, Moves: 46},
			{UsedFrom: 0.7, UsedTo: 1},
		},
		All: row{UsedFrom: 0, UsedTo: 1, Forecasts: 106, OffHours: 59 * 60.0 / 106, Late: 59, Moves: 104},
	}
	want := report{From: hoursIn(1), To: hoursIn(60), Accounts: []string{"work", "side", "personal"}, Forecast: scored, Baseline: scored}
	if !sameReport(got, want) {
		t.Errorf("printed\n%+v\nwant\n%+v", got, want)
	}
}

// sameReport reports whether two reports match, their hours to within
// rounding.
func sameReport(a, b report) bool {
	return a.From.Equal(b.From) && a.To.Equal(b.To) && strings.Join(a.Accounts, ",") == strings.Join(b.Accounts, ",") &&
		sameSheet(a.Forecast, b.Forecast) && sameSheet(a.Baseline, b.Baseline)
}

// sameSheet reports whether two sheets match, their hours to within
// rounding.
func sameSheet(a, b sheet) bool {
	if len(a.ByUse) != len(b.ByUse) || !sameRow(a.All, b.All) {
		return false
	}
	for i := range a.ByUse {
		if !sameRow(a.ByUse[i], b.ByUse[i]) {
			return false
		}
	}
	return true
}

// sameRow reports whether two rows match, their hours to within rounding.
func sameRow(a, b row) bool {
	hoursNear := func(x, y float64) bool { return math.Abs(x-y) < 1e-9 }
	return a.UsedFrom == b.UsedFrom && a.UsedTo == b.UsedTo && a.Forecasts == b.Forecasts && a.Early == b.Early && a.Late == b.Late &&
		a.Moves == b.Moves && hoursNear(a.OffHours, b.OffHours) && hoursNear(a.MovedHours, b.MovedHours)
}

func TestRefusesToRunWithoutBothTheStateDirectoryAndTheConfig(t *testing.T) {
	// Neither path is there: one read would fail as not there.
	gone := filepath.Join(t.TempDir(), "gone")
	tests := []struct {
		name string
		args []string
	}{
		{name: "neither", args: nil},
		{name: "the state directory alone", args: []string{"--state", gone}},
		{name: "the config alone", args: []string{"--config", gone}},
		{name: "the config alone, as JSON", args: []string{"--config", gone, "--json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(tt.args, &stdout, &stderr, time.Now)
			if !errors.Is(err, errUnnamed) || errors.Is(err, fs.ErrNotExist) {
				t.Errorf("run(%q) error = %v, want it refused for what it names, before reading anything", tt.args, err)
			}
			if stdout.Len() > 0 {
				t.Errorf("run(%q) printed %q, want nothing", tt.args, stdout.String())
			}
		})
	}
}

func TestFailsOnAStateDirectoryWithoutAReadingsHistory(t *testing.T) {
	configPath := configOf(t, account{id: "work"})

	var stdout, stderr bytes.Buffer
	if err := run([]string{"--state", t.TempDir(), "--config", configPath}, &stdout, &stderr, time.Now); err == nil {
		t.Errorf("run() printed %q, want it to fail: there's no history to play", stdout.String())
	}
}

func TestItsHelpSaysWhatThePlaybackHasNoneOf(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--help"}, &stdout, &stderr, time.Now); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--state", "--config", "neither has a default", "reaches a limit", "token refused", "no global pin"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("--help says\n%s\nwant it to say %q", stderr.String(), want)
		}
	}
}
