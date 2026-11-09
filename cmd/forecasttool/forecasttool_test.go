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
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
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
// work's, begun as the history begins and resetting a week in, rises a
// thousandth an hour from none till a reading 108 hours in finds it run out;
// side's, 45.2% used five days in, rises a thousandth an hour to its reset,
// 96 hours in, short, and is read again an hour later; personal's, a fifth
// used, resets after the history ends, short. A message of work's session
// answers every hour. It returns the config and the state directory, and the
// time after it.
func shortHistory(t *testing.T) (configPath, stateDir string, now time.Time) {
	t.Helper()
	read := hourly("work", begins, 108, 0, hoursIn(168))
	read = append(read, readings.Reading{At: hoursIn(108), Account: "work", Key: "7d", Utilization: 1, ResetsAt: hoursIn(168), Status: quota.StatusRejected})
	read = append(read, hourly("side", begins, 96, 0.452, hoursIn(96))...)
	read = append(read, weekRead("side", hoursIn(97), 0.01, hoursIn(96+7*24)), weekRead("personal", begins, 0.2, hoursIn(150)))
	var lines []*ledger.Line
	for i := range 108 {
		lines = append(lines, messageOf(hoursIn(float64(i))))
	}
	configPath = configOf(t, account{id: "work"}, account{id: "side"}, account{id: "personal"})
	now = hoursIn(118)
	return configPath, stateOf(t, now, configPath, read, lines), now
}

func TestScoresAWeekForecastShortThatRanOutAndOneThatHeadedShort(t *testing.T) {
	// Each hour is forecast from an hour in, and scored from two days in.
	// Work's week is forecast short at each of its 60 hours scored before it
	// runs out, 60 hours before its reset: 60 hours late, all in its first
	// 40%. Side's is forecast short at each of its 48 scored before its
	// reset, as it was: on time, all in 40–70%. Personal's resets after the
	// history ends, and side's next week too, so they're never scored. Each
	// week heads short at its pace since it started too. Neither week's
	// first forecast scored pairs with the hour before's, unscored.
	configPath, stateDir, now := shortHistory(t)

	got := scoring(t, now, "--state", stateDir, "--config", configPath)
	scores := `  0–40% used                60   60.0h     0%   100%     59    0.0h
  40–70% used               48    0.0h     0%     0%     47    0.0h
  70–100% used               0       –      –      –      0       –
  all                      108   33.3h     0%    56%    106    0.0h
`
	want := "forecasts of each week at each hour, 2026-09-07 01:00 UTC to 2026-09-11 12:00 UTC, of work, side, personal\n" +
		"scored from 2026-09-09 00:00 UTC, two days in\n\n" +
		"                     forecasts     off  early   late  pairs   moved\n" +
		"the router's forecast\n" + scores + "pace since start\n" + scores + "\n" + legend
	if got != want {
		t.Errorf("printed\n%s\nwant\n%s", got, want)
	}
}

func TestSaysNothingCouldBeScoredOfAHistoryShorterThanTwoDays(t *testing.T) {
	// The history's last reading is a minute short of two days in.
	configPath := configOf(t, account{id: "work"})
	read := []readings.Reading{weekRead("work", begins, 0.1, hoursIn(150)), weekRead("work", hoursIn(48).Add(-time.Minute), 0.2, hoursIn(150))}
	now := hoursIn(60)
	stateDir := stateOf(t, now, configPath, read, nil)

	got := scoring(t, now, "--state", stateDir, "--config", configPath)
	want := "forecasts of each week at each hour, 2026-09-07 01:00 UTC to 2026-09-08 23:00 UTC, of work\n" +
		"nothing could be scored: the history ends before 2026-09-09 00:00 UTC, two days in, where scoring starts\n"
	if got != want {
		t.Errorf("printed\n%s\nwant\n%s", got, want)
	}
	var printed report
	if err := json.Unmarshal([]byte(scoring(t, now, "--state", stateDir, "--config", configPath, "--json")), &printed); err != nil {
		t.Fatal(err)
	}
	if printed.Scored || !printed.ScoredFrom.Equal(hoursIn(48)) || printed.Forecast.All.Forecasts > 0 || printed.Baseline.All.Forecasts > 0 {
		t.Errorf("printed %+v, want nothing scored, scoring from two days in", printed)
	}
}

func TestSaysScoringWaitsForEveryAccountsWeekToBeRead(t *testing.T) {
	// Work's week is read every hour for 70 hours; side's, in the history
	// most cases play, first 60h20m in, and every hour after; personal's and
	// other's never.
	works := hourly("work", begins, 71, 0.1, hoursIn(150))
	sideFrom := hoursIn(60).Add(20 * time.Minute)
	read := slices.Concat(works, hourly("side", sideFrom, 10, 0.3, hoursIn(160)))
	unreadPersonal := "the history never reads the week of personal, which the router knew, so the playback can't stand in for it; " +
		"a config without personal plays the rest\n"
	tests := []struct {
		name     string
		read     []readings.Reading
		accounts []account
		// want is what's said after the first line, up to the scores.
		want string
	}{
		{
			name: "side read past two days in", read: read, accounts: []account{{id: "work"}, {id: "side"}},
			want: "scored from 2026-09-09 13:00 UTC, once every account's week had been read\n",
		},
		{
			name: "side not configured, read past two days in", read: read, accounts: []account{{id: "work"}},
			want: "scored from 2026-09-09 00:00 UTC, two days in\n",
		},
		{
			name: "personal never read", read: read, accounts: []account{{id: "work"}, {id: "side"}, {id: "personal"}},
			want: "nothing could be scored: " + unreadPersonal,
		},
		{
			name: "side read only its 5-hour window", accounts: []account{{id: "work"}, {id: "side"}},
			read: slices.Concat(works, []readings.Reading{{At: sideFrom, Account: "side", Key: "5h", Utilization: 0.3, ResetsAt: hoursIn(64)}}),
			want: "nothing could be scored: the history never reads the week of side, which the router knew, so the playback can't stand in for it; " +
				"a config without side plays the rest\n",
		},
		{
			name: "no account configured read", read: read, accounts: []account{{id: "personal"}, {id: "other"}},
			want: "nothing could be scored: the history reads no configured account's week\n",
		},
		{
			name: "side first read as the history ends", accounts: []account{{id: "work"}, {id: "side"}},
			read: slices.Concat(works, []readings.Reading{weekRead("side", hoursIn(70).Add(20*time.Minute), 0.3, hoursIn(160))}),
			want: "nothing could be scored: the history ends before 2026-09-09 23:00 UTC, once every account's week had been read, where scoring starts\n",
		},
		{
			name: "a history short of two days in, personal never read", accounts: []account{{id: "work"}, {id: "personal"}},
			read: works[:47],
			want: "nothing could be scored: the history ends before 2026-09-09 00:00 UTC, two days in, where scoring starts\nand " + unreadPersonal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := configOf(t, tt.accounts...)
			now := hoursIn(80)
			stateDir := stateOf(t, now, configPath, tt.read, nil)

			_, rest, _ := strings.Cut(scoring(t, now, "--state", stateDir, "--config", configPath), "\n")
			got, _, scored := strings.Cut(rest, "\n\n")
			if scored {
				got += "\n"
			}
			if got != tt.want {
				t.Errorf("printed after its first line\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestLeavesOutOfItsJSONTheHoursThereAreNoneOf(t *testing.T) {
	// Work's week is read every hour for 70 hours, and personal's never.
	configPath := configOf(t, account{id: "work"}, account{id: "personal"})
	now := hoursIn(80)
	stateDir := stateOf(t, now, configPath, hourly("work", begins, 71, 0.1, hoursIn(150)), nil)

	printed := scoring(t, now, "--state", stateDir, "--config", configPath, "--json")
	for _, field := range []string{`"scored_from"`, `"all_read"`} {
		if strings.Contains(printed, field) {
			t.Errorf("printed\n%s\nwant no %s: there's none", printed, field)
		}
	}
	var got report
	if err := json.Unmarshal([]byte(printed), &got); err != nil {
		t.Fatal(err)
	}
	if got.Scored || !slices.Equal(got.Unread, []string{"personal"}) || !got.TwoDaysIn.Equal(hoursIn(48)) {
		t.Errorf("printed %+v, want nothing scored, personal unread, two days in still said", got)
	}
}

func TestScoresAWeekThatRunsOutBesideItsPaceSinceItStarted(t *testing.T) {
	// Work's week, begun as the history begins and resetting a week in, is
	// read every minute: rising 0.4 of a point an hour for a day, then a point
	// an hour, till it runs out 114.4 hours in; a message of its session
	// answers every ten minutes. Side, keeping half back, is half used, and
	// personal used up, both resetting after work's week, so the pool's use
	// is work's alone.
	var read []readings.Reading
	for k := range 6865 {
		read = append(read, weekRead("work", begins.Add(time.Duration(k)*time.Minute), float64(4*min(k, 1440)+10*max(k-1440, 0))/60000, hoursIn(168)))
	}
	read = append(read, weekRead("side", begins, 0.5, hoursIn(178)),
		readings.Reading{At: begins, Account: "personal", Key: "7d", Utilization: 1, ResetsAt: hoursIn(178), Status: quota.StatusRejected})
	var lines []*ledger.Line
	for at := begins; at.Before(hoursIn(114.4)); at = at.Add(10 * time.Minute) {
		lines = append(lines, messageOf(at))
	}
	configPath := configOf(t, account{id: "work"}, account{id: "side", reserve: 0.5}, account{id: "personal"})
	now := hoursIn(120)
	stateDir := stateOf(t, now, configPath, read, lines)

	var got report
	if err := json.Unmarshal([]byte(scoring(t, now, "--state", stateDir, "--config", configPath, "--json")), &got); err != nil {
		t.Fatal(err)
	}
	if !got.From.Equal(hoursIn(1)) || !got.ScoredFrom.Equal(hoursIn(48)) || !got.To.Equal(hoursIn(114)) || !got.Scored {
		t.Errorf("it forecast from %v, scored from %v, to %v, scored %v; want from an hour in, scored from two days in, to 114 hours in, the hour of the last reading",
			got.From, got.ScoredFrom, got.To, got.Scored)
	}
	// The router's forecasts, at each of the 67 hours scored, all work's,
	// the day before them a point an hour, as the day's rate is: they run
	// out within 2 minutes of 114.4 hours in, moving under 2 minutes an hour
	// on average. The first is at 33.6% used, and pairs with none, the hour
	// before it unscored.
	tests := []struct {
		name      string
		got       row
		forecasts int
		pairs     int
	}{
		{name: "0–40%", got: got.Forecast.ByUse[0], forecasts: 7, pairs: 6},
		{name: "40–70%", got: got.Forecast.ByUse[1], forecasts: 30, pairs: 30},
		{name: "70–100%", got: got.Forecast.ByUse[2], forecasts: 30, pairs: 30},
		{name: "all", got: got.Forecast.All, forecasts: 67, pairs: 66},
	}
	within := (2 * time.Minute).Hours()
	for _, tt := range tests {
		t.Run("the router's "+tt.name, func(t *testing.T) {
			r := tt.got
			if r.Forecasts != tt.forecasts || r.Early != 0 || r.Late != 0 || r.Pairs != tt.pairs || r.OffHours > within || r.MovedHours > within {
				t.Errorf("scored %+v, want %d forecasts, none early or late, off under %v hours, %d pairs moved under %v hours",
					r, tt.forecasts, within, tt.pairs, within)
			}
		})
	}
	// The pace since the week started, 0.01t − 0.144 used at the tth hour,
	// runs out at t/u: 28.46 hours late at the first, 12.47 at the 68th
	// hour and 11.97 at the 69th, so late by more than 12 hours at the
	// first 21.
	baseline := sheet{
		ByUse: []row{
			{UsedFrom: 0, UsedTo: 0.4, Forecasts: 7, OffHours: 25.062364092889396, Late: 7, Pairs: 6, MovedHours: 1.0822510822510822},
			{UsedFrom: 0.4, UsedTo: 0.7, Forecasts: 30, OffHours: 12.409393413987152, Late: 14, Pairs: 30, MovedHours: 0.522466039707419},
			{UsedFrom: 0.7, UsedTo: 1, Forecasts: 30, OffHours: 2.6996434414975607, Pairs: 30, MovedHours: 0.2077274615704196},
		},
		All: row{UsedFrom: 0, UsedTo: 1, Forecasts: 67, OffHours: 9.383696333056227, Late: 21, Pairs: 66, MovedHours: 0.43029259896729777},
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
			{UsedFrom: 0, UsedTo: 0.4, Forecasts: 60, OffHours: 60, Late: 60, Pairs: 59},
			{UsedFrom: 0.4, UsedTo: 0.7, Forecasts: 48, Pairs: 47},
			{UsedFrom: 0.7, UsedTo: 1},
		},
		All: row{UsedFrom: 0, UsedTo: 1, Forecasts: 108, OffHours: 60 * 60.0 / 108, Late: 60, Pairs: 106},
	}
	want := report{
		From: hoursIn(1), To: hoursIn(108), ScoredFrom: hoursIn(48), TwoDaysIn: hoursIn(48), AllRead: begins, Scored: true,
		Accounts: []string{"work", "side", "personal"}, Forecast: scored, Baseline: scored,
	}
	if !sameReport(got, want) {
		t.Errorf("printed\n%+v\nwant\n%+v", got, want)
	}
}

// sameReport reports whether two reports match, their hours to within
// rounding.
func sameReport(a, b report) bool {
	return a.From.Equal(b.From) && a.To.Equal(b.To) && a.ScoredFrom.Equal(b.ScoredFrom) && a.TwoDaysIn.Equal(b.TwoDaysIn) && a.AllRead.Equal(b.AllRead) &&
		slices.Equal(a.Unread, b.Unread) && a.Scored == b.Scored &&
		slices.Equal(a.Accounts, b.Accounts) && sameSheet(a.Forecast, b.Forecast) && sameSheet(a.Baseline, b.Baseline)
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
		a.Pairs == b.Pairs && hoursNear(a.OffHours, b.OffHours) && hoursNear(a.MovedHours, b.MovedHours)
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

func TestSaysWhatTheReadersCantReadOnStderr(t *testing.T) {
	configPath, stateDir, now := shortHistory(t)
	days, err := filepath.Glob(filepath.Join(ledger.Dir(stateDir), "requests-*.jsonl"))
	if err != nil || len(days) == 0 {
		t.Fatalf("the ledger holds %v (%v), want its days", days, err)
	}
	unreadable, err := os.OpenFile(days[0], os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unreadable.WriteString("not a line of the ledger\n"); err != nil {
		t.Fatal(err)
	}
	if err := unreadable.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	logTo(&stderr)
	err = run([]string{"--state", stateDir, "--config", configPath}, &stdout, &stderr, func() time.Time { return now })
	logs.Close(0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), `level=WARN msg="request ledger lines unread" component=forecasttool`) {
		t.Errorf("stderr took\n%s\nwant the ledger's line it couldn't read", stderr.String())
	}
	if strings.Contains(stderr.String(), "log file") {
		t.Errorf("stderr took\n%s\nwant no word of a log file", stderr.String())
	}
}

func TestItsHelpSaysWhatThePlaybackHasNoneOf(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--help"}, &stdout, &stderr, time.Now); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"--state", "--config", "neither has a default", "reaches a limit", "token refused", "no global pin",
		"router was down", "two days after the history's first", "once every account's week has been read", "keeps only readings that changed",
		"nothing is scored", "a config without the account plays the rest", "nothing could be scored", "a reset by hand", "moved or not",
		"they say on stderr",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("--help says\n%s\nwant it to say %q", stderr.String(), want)
		}
	}
}
