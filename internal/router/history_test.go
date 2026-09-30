package router

import (
	"context"
	"encoding/json"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// keeping has h write what's noted to it, and returns what stops it, once it
// has written everything noted before.
func keeping(t *testing.T, h *history) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		h.run(ctx)
		close(done)
	}()
	return func() {
		cancel()
		<-done
	}
}

// historyOf is the readings history's file of the local day t falls on, in
// dir.
func historyOf(dir string, t time.Time) string {
	return filepath.Join(dir, historyFile(t.Local().Format(historyDay)))
}

func TestTheHistoryHoldsEachReadingThatChangesAWindow(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	dir := filepath.Join(t.TempDir(), "history")
	h := newHistory(at(start))
	h.open(dir)
	s.history = h.note
	stop := keeping(t, h)

	s.record("work", []quota.Window{session, week}, s.mark())
	s.record("work", []quota.Window{session, week}, s.mark())
	clock.now = start.Add(time.Minute)
	busier := session
	busier.Utilization = 0.31
	s.record("work", []quota.Window{busier, week}, s.mark())
	s.recordProbe("side", probed(nil, session), nil, s.mark(), fromProbe)
	s.recordProbe("side", probed(nil, session, week), nil, s.mark(), fromPrime)
	stop()

	data, err := os.ReadFile(historyOf(dir, start))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"at":"2026-09-28T13:12:00Z","account":"work","window":"5h","utilization":0.23,"resets_at":"2026-09-28T18:10:00Z","status":"allowed","source":"answer"}
{"at":"2026-09-28T13:12:00Z","account":"work","window":"7d","utilization":0.93,"resets_at":"2026-10-02T21:00:00Z","status":"allowed_warning","source":"answer"}
{"at":"2026-09-28T13:13:00Z","account":"work","window":"5h","utilization":0.31,"resets_at":"2026-09-28T18:10:00Z","status":"allowed","source":"answer"}
{"at":"2026-09-28T13:13:00Z","account":"side","window":"5h","utilization":0.23,"resets_at":"2026-09-28T18:10:00Z","status":"allowed","source":"probe"}
{"at":"2026-09-28T13:13:00Z","account":"side","window":"7d","utilization":0.93,"resets_at":"2026-10-02T21:00:00Z","status":"allowed_warning","source":"prime"}
`
	if string(data) != want {
		t.Errorf("the history holds\n%s\nwant a line for each reading that changed a window, where it came from, and nothing else\n%s", data, want)
	}
	for path, mode := range map[string]fs.FileMode{dir: fs.ModeDir | 0o700, historyOf(dir, start): 0o600} {
		if info, err := os.Stat(path); err != nil || info.Mode() != mode {
			t.Errorf("%s is %v (%v), want %v", filepath.Base(path), info.Mode(), err, mode)
		}
	}
}

func TestTheHistoryKeepsAFileADayForTwoWeeks(t *testing.T) {
	dir := t.TempDir()
	today := start.Local()
	dayFile := func(back int) string { return historyFile(today.AddDate(0, 0, -back).Format(historyDay)) }
	files := map[string]bool{
		dayFile(0):  true,
		dayFile(14): true,
		dayFile(15): false,
		dayFile(30): false,
		// Anything but the history's own is left alone.
		"notes.txt":           true,
		"readings-soon.jsonl": true,
	}
	for name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h := newHistory(at(start))
	h.open(dir)

	keeping(t, h)()
	for name, kept := range files {
		if _, err := os.Stat(filepath.Join(dir, name)); (err == nil) != kept {
			t.Errorf("%s kept = %v, want %v: a day's file goes 14 days after its day ends", name, err == nil, kept)
		}
	}
}

func TestTheRecentRatesOutlastARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	dir := filepath.Join(t.TempDir(), "history")
	// Over 20 minutes, work's session rises at 30% an hour, and its week at
	// 3%.
	clock := &testClock{now: start}
	before := newTestFile(clock.read, testAccounts())
	before.load(path)
	h := newHistory(at(start))
	h.open(dir)
	before.state.history = h.note
	stop := keeping(t, h)
	for i, u := range []float64{0.5, 0.55, 0.6} {
		clock.now = start.Add(time.Duration(i) * 10 * time.Minute)
		weekly := week
		weekly.Utilization = 0.4 + 0.005*float64(i)
		before.state.record("work", []quota.Window{{Key: "5h", Label: "Session", Utilization: u, ResetsAt: session.ResetsAt}, weekly}, before.state.mark())
	}
	before.save()
	stop()
	now := clock.now.Add(time.Minute)

	after := newTestFile(at(now), testAccounts())
	after.load(path)
	restarted := newHistory(at(now))
	restarted.open(dir)
	if kept := after.state.seed(restarted.readBack(now)); kept != 6 {
		t.Errorf("took up %d readings, want the 6 of the last half hour", kept)
	}
	// A minute on from the last reading, the rise is over 21 minutes.
	since := (21 * time.Minute).Hours()
	if got, want := after.state.document().Accounts[0].Rates, []status.Rate{{Window: "5h", Rate: 0.1 / since}, {Window: "7d", Rate: 0.01 / since}}; !sameRates(got, want) {
		t.Errorf("after the restart, work's recent rates are %+v, want %+v, as before it", got, want)
	}
	if pace, _ := after.state.usage["work"].pace(testPolicy, now); !pace.Recent || math.Abs(pace.Rate-0.1/since) > 1e-9 {
		t.Errorf("after the restart, work's session is used at %+v, want its recent rate, as before it", pace)
	}
}

func TestAQuietAccountStaysQuietAcrossARestart(t *testing.T) {
	// Work's session was last read two hours before the restart, and hasn't
	// been used since.
	now := start.Add(2 * time.Hour)
	dir := t.TempDir()
	h := newHistory(at(now))
	h.open(dir)
	s := newTestState(&testClock{now: now})
	s.usage["work"].windows["5h"] = session
	lines := []reading{{At: start, Account: "work", Window: "5h", Utilization: session.Utilization, ResetsAt: session.ResetsAt, Source: fromAnswer}}
	h.write(lines)

	s.seed(h.readBack(now))
	if pace, ok := s.usage["work"].pace(testPolicy, now); !ok || !pace.Recent || pace.Rate != 0 {
		t.Errorf("after the restart, work's session is used at %+v, %v, want quiet, as before it", pace, ok)
	}
}

func TestARiseAcrossAGapInTheHistoryIsSpreadOverItAfterARestart(t *testing.T) {
	// Work's session was last read two hours before the restart, then a
	// probe read the use outside the router it came to since, just before.
	now := start.Add(2 * time.Hour)
	dir := t.TempDir()
	h := newHistory(at(now))
	h.open(dir)
	s := newTestState(&testClock{now: now})
	busier := session
	busier.Utilization = 0.43
	s.usage["work"].windows["5h"] = busier
	h.write([]reading{
		{At: start, Account: "work", Window: "5h", Utilization: 0.23, ResetsAt: session.ResetsAt, Source: fromAnswer},
		{At: now.Add(-time.Minute), Account: "work", Window: "5h", Utilization: 0.43, ResetsAt: session.ResetsAt, Source: fromProbe},
	})

	s.seed(h.readBack(now))
	if pace, ok := s.usage["work"].pace(testPolicy, now); !ok || !pace.Recent || math.Abs(pace.Rate-0.1) > 1e-9 {
		t.Errorf("after the restart, work's session is used at %+v, %v, want 10%% an hour, its rise spread over the two hours since it was read", pace, ok)
	}
}

func TestReadingBackTakesTheTwoNewestFilesByName(t *testing.T) {
	now := time.Date(2026, 9, 29, 0, 10, 0, 0, time.Local)
	lineAt := func(at time.Time, u float64) reading {
		return reading{At: at.UTC(), Account: "work", Window: "5h", Utilization: u, ResetsAt: session.ResetsAt, Source: fromAnswer}
	}
	tests := []struct {
		name string
		// files are the lines each file holds, by the day it's named for.
		files map[string][]reading
		want  []reading
	}{
		{
			name: "yesterday's and today's, across midnight",
			files: map[string][]reading{
				"2026-09-27": {lineAt(now.Add(-26*time.Hour), 0.1)},
				"2026-09-28": {lineAt(now.Add(-20*time.Minute), 0.2)},
				"2026-09-29": {lineAt(now.Add(-5*time.Minute), 0.3)},
			},
			want: []reading{lineAt(now.Add(-20*time.Minute), 0.2), lineAt(now.Add(-5*time.Minute), 0.3)},
		},
		{
			name: "named for a day the clock hasn't come to, as after a change of time zone",
			files: map[string][]reading{
				"2026-09-28": {lineAt(now.Add(-40*time.Minute), 0.1)},
				"2026-09-29": {lineAt(now.Add(-20*time.Minute), 0.2)},
				"2026-09-30": {lineAt(now.Add(-5*time.Minute), 0.3)},
			},
			want: []reading{lineAt(now.Add(-20*time.Minute), 0.2), lineAt(now.Add(-5*time.Minute), 0.3)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for day, lines := range tt.files {
				var data []byte
				for _, r := range lines {
					line, err := json.Marshal(r)
					if err != nil {
						t.Fatal(err)
					}
					data = append(append(data, line...), '\n')
				}
				if err := os.WriteFile(filepath.Join(dir, historyFile(day)), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			h := newHistory(at(now))
			h.open(dir)

			if got := slices.Collect(h.readBack(now)); !slices.EqualFunc(got, tt.want, func(a, b reading) bool { return a.At.Equal(b.At) && a.Utilization == b.Utilization }) {
				t.Errorf("readBack() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFilesOfADayAfterTomorrowAreNeitherTakenUpNorKept(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.Local)
	dir := t.TempDir()
	day := func(offset int) string { return historyFile(now.AddDate(0, 0, offset).Format(historyDay)) }
	lineAt := func(at time.Time, u float64) []byte {
		line, err := json.Marshal(reading{At: at.UTC(), Account: "work", Window: "5h", Utilization: u, ResetsAt: session.ResetsAt, Source: fromAnswer})
		if err != nil {
			t.Fatal(err)
		}
		return append(line, '\n')
	}
	// A clock once set days ahead named a file for a day to come.
	files := map[string][]byte{
		day(-1): lineAt(now.Add(-20*time.Hour), 0.1),
		day(0):  lineAt(now.Add(-time.Hour), 0.2),
		day(5):  lineAt(now.Add(-time.Minute), 0.9),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h := newHistory(at(now))
	h.open(dir)

	got := slices.Collect(h.readBack(now))
	if len(got) != 2 || got[0].Utilization != 0.1 || got[1].Utilization != 0.2 {
		t.Errorf("readBack() = %+v, want yesterday's and today's, not the one of a day to come", got)
	}
	h.prune()
	if _, err := os.Stat(filepath.Join(dir, day(5))); err == nil {
		t.Error("the file of a day to come stayed, want it pruned")
	}
	if _, err := os.Stat(filepath.Join(dir, day(0))); err != nil {
		t.Errorf("today's file went: %v", err)
	}
}

func TestReadingBackSkipsALineTooLongToHoldAlone(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	lineAt := func(u float64, pad int) []byte {
		line := `{"at":"` + start.UTC().Format(time.RFC3339) + `","account":"work","window":"5h","utilization":` +
			strconv.FormatFloat(u, 'f', -1, 64) + `,"pad":"` + strings.Repeat("x", pad) + `","source":"answer"}`
		return []byte(line + "\n")
	}
	data := slices.Concat(lineAt(0.1, 0), lineAt(0.2, historyLineMax), lineAt(0.3, 0))
	if err := os.WriteFile(historyOf(dir, start), data, 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHistory(at(start))
	h.open(dir)

	got := slices.Collect(h.readBack(start))
	if len(got) != 2 || got[0].Utilization != 0.1 || got[1].Utilization != 0.3 {
		t.Errorf("readBack() = %+v, want the lines either side of the one too long to hold", got)
	}
	if !log.Has("level=WARN", `msg="readings history lines unread"`, "lines=1") {
		t.Errorf("log reads\n%s\nwant the line too long noted", log)
	}
}

func TestTakingUpADaysHistoryKeepsWhatTheTrailsNeedAlone(t *testing.T) {
	now := start.Add(24 * time.Hour)
	dir := t.TempDir()
	h := newHistory(at(now))
	h.open(dir)
	// Work's session, read every half minute for the day before, rising a
	// little each time, in a window that resets after now.
	var readings []reading
	for i := range 2880 {
		readings = append(readings, reading{
			At: start.Add(time.Duration(i) * 30 * time.Second), Account: "work", Window: "5h",
			Utilization: float64(i) / 10000, ResetsAt: now.Add(time.Hour), Source: fromAnswer,
		})
	}
	h.write(readings)
	s := newTestState(&testClock{now: now})
	s.usage["work"].windows["5h"] = quota.Window{Key: "5h", Utilization: readings[len(readings)-1].Utilization, ResetsAt: now.Add(time.Hour)}

	s.seed(h.readBack(now))
	if got := len(s.usage["work"].trails["5h"]); got > 61 {
		t.Errorf("the trail holds %d levels, want the baseline and the half hour's since alone, 61 at most", got)
	}
}

func TestTakingUpTheHistoryLeavesOutWhatCantBeTakenUp(t *testing.T) {
	now := start.Add(20 * time.Minute)
	resets := session.ResetsAt.UTC().Format(time.RFC3339)
	line := func(ago time.Duration, account string, utilization string) string {
		return `{"at":"` + now.Add(-ago).UTC().Format(time.RFC3339) + `","account":"` + account +
			`","window":"5h","utilization":` + utilization + `,"resets_at":"` + resets + `","source":"answer"}`
	}
	lines := []string{
		line(40*time.Minute, "work", "0.1"),
		line(20*time.Minute, "work", "0.5"),
		`{"at": "2026-09-28T13:20:00Z", "account": "work", "window": "5h", "utilization":`,
		`{"at": "` + strings.Repeat("x", 70*1024) + `"}`,
		line(15*time.Minute, "work", "-0.2"),
		"",
		line(10*time.Minute, "gone", "0.9"),
		line(10*time.Minute, "work", "0.55"),
		`{"at":"` + now.UTC().Format(time.RFC3339) + `","window":"5h","utilization":0.9}`,
		line(-time.Minute, "work", "0.9"),
		line(0, "work", "0.6"),
	}
	tests := []struct {
		name string
		// held is work's session as the state file kept it.
		held     quota.Window
		wantRate float64
		wantOK   bool
	}{
		{name: "the window as it runs, from its baseline, read 40 minutes back and taken as read last then", held: quota.Window{Key: "5h", Utilization: 0.6, ResetsAt: session.ResetsAt}, wantRate: 0.75, wantOK: true},
		{name: "the window reset since", held: quota.Window{Key: "5h", Utilization: 0.02, ResetsAt: session.ResetsAt.Add(5 * time.Hour)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			dir := t.TempDir()
			if err := os.WriteFile(historyOf(dir, now), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			s := newTestState(&testClock{now: now})
			s.usage["work"].windows["5h"] = tt.held
			h := newHistory(at(now))
			h.open(dir)

			s.seed(h.readBack(now))
			rate, _, ok := score.RecentRate(tt.held, s.usage["work"].trails["5h"], now)
			if math.Abs(rate-tt.wantRate) > 1e-9 || ok != tt.wantOK {
				t.Errorf("work's session's recent rate = %v, %v, want %v, %v", rate, ok, tt.wantRate, tt.wantOK)
			}
			if !log.Has("level=WARN", `msg="readings history lines unread"`, "lines=5") {
				t.Errorf("log reads\n%s\nwant the five lines that don't read as readings noted, the one over 64 KiB among them", log)
			}
		})
	}
}

func TestAHistoryThatCantBeWrittenLeavesRoutingAlone(t *testing.T) {
	log := logstest.Capture(t)
	r := newTestRouter(t, at(start), &stubProber{})
	// The history's directory can't be made where a file is.
	blocker := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r.history.open(filepath.Join(blocker, "history"))
	stop := keeping(t, r.history)
	busier := session
	busier.Utilization = 0.4
	r.state.record("work", []quota.Window{busier, soonWeek}, r.state.mark())
	r.state.record("side", []quota.Window{session, laterWeek}, r.state.mark())
	if got := choose(t.Context(), r, Request{ID: "5f3a9c2e", Session: "one", Model: opus, Client: "work"}); got.Account != "work" {
		t.Errorf("a new session went to %q, want work, whatever the history", got.Account)
	}
	stop()
	if got, _ := r.Status().Account("work"); got.Windows[0].Utilization != 0.4 {
		t.Errorf("work's session reads %v, want 0.4: the reading taken, whatever the history", got.Windows[0].Utilization)
	}
	if failures := linesWith(log, "can't write the readings history"); len(failures) != 1 {
		t.Errorf("log reads\n%s\nwant the history's failure noted once", log)
	}
}

func TestAHistoryThatStallsHoldsNothingUp(t *testing.T) {
	r := newTestRouter(t, at(start), &stubProber{})
	// Opened, with nothing writing it, the history's queue fills, and stays
	// full.
	r.history.open(t.TempDir())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range historyQueue + 10 {
			busier := session
			busier.Utilization = float64(i) / (historyQueue + 10)
			r.state.record("work", []quota.Window{busier}, r.state.mark())
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("taking readings in waited on the history, want it never to")
	}
	if got := choose(t.Context(), r, Request{ID: "5f3a9c2e", Session: "one", Model: opus, Client: "work"}); got.Account == "" {
		t.Errorf("a new session went nowhere, want an account, whatever the history")
	}
}

func TestAHistoryWriteThatFailsIsLoggedOnceUntilOneSucceeds(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	h := newHistory(at(start))
	h.open(dir)
	one := readingsOf("work", []quota.Window{session}, start, fromAnswer)
	// Today's file can't be opened to append while a directory stands at its
	// path.
	block := func() {
		if err := os.Mkdir(historyOf(dir, start), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	unblock := func() {
		if err := os.Remove(historyOf(dir, start)); err != nil {
			t.Fatal(err)
		}
	}

	block()
	h.write(one)
	h.write(one)
	unblock()
	h.write(one)
	if err := os.Remove(historyOf(dir, start)); err != nil {
		t.Fatal(err)
	}
	block()
	h.write(one)
	if got := len(linesWith(log, "can't write the readings history")); got != 2 {
		t.Errorf("log reads\n%s\nwant the failure noted twice, once before the write that succeeded and once after", log)
	}
	if got := len(linesWith(log, "writing the readings history again")); got != 1 {
		t.Errorf("log reads\n%s\nwant the history noted as written again once", log)
	}
}

func TestAHistoryFallingBehindDropsReadingsRatherThanWait(t *testing.T) {
	log := logstest.Capture(t)
	h := newHistory(at(start))
	h.open(t.TempDir())
	one := readingsOf("work", []quota.Window{session}, start, fromAnswer)
	fill := func() {
		for range historyQueue + 2 {
			h.note(one)
		}
	}

	fill()
	if got := len(linesWith(log, "fell behind")); got != 1 {
		t.Errorf("log reads\n%s\nwant the readings dropped noted once", log)
	}
	h.write(<-h.queue)
	fill()
	if got := len(linesWith(log, "fell behind")); got != 2 {
		t.Errorf("log reads\n%s\nwant the readings dropped noted again, once a write has caught up", log)
	}
}

func TestAReadingTheHistoryCantHoldIsLoggedOnce(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	h := newHistory(at(start))
	h.open(dir)
	bad := reading{At: start, Account: "work", Window: "5h", Utilization: math.NaN(), Source: fromAnswer}
	good := readingsOf("work", []quota.Window{session}, start, fromAnswer)

	h.write(append([]reading{bad}, good...))
	h.write([]reading{bad})
	if got := len(linesWith(log, "readings history can't hold a reading")); got != 1 {
		t.Errorf("log reads\n%s\nwant the reading it can't hold noted once", log)
	}
	if data, err := os.ReadFile(historyOf(dir, start)); err != nil || strings.Count(string(data), "\n") != 1 {
		t.Errorf("the history holds %q (%v), want the reading it could hold alone", data, err)
	}
}

func TestAHistoryFileThatCantBeReadIsLogged(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	if err := os.WriteFile(historyOf(dir, start), []byte("{}\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	h := newHistory(at(start))
	h.open(dir)

	if got := slices.Collect(h.readBack(start)); len(got) != 0 {
		t.Errorf("readBack() = %+v, want none", got)
	}
	if !log.Has("level=WARN", `msg="can't read the readings history"`) {
		t.Errorf("log reads\n%s\nwant the file that can't be read noted", log)
	}
}

func TestTheHistorysDirectoryIsMadePrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	h := newHistory(at(start))
	h.open(dir)

	keeping(t, h)()
	if info, err := os.Stat(dir); err != nil || info.Mode() != fs.ModeDir|0o700 {
		t.Errorf("the history's directory is %v (%v), want %v", info.Mode(), err, fs.ModeDir|0o700)
	}
}

func TestTheHistoryPrunesOnTheFirstWriteOfANewDay(t *testing.T) {
	dir := t.TempDir()
	clock := &testClock{now: start}
	h := newHistory(clock.read)
	h.open(dir)
	h.prune()
	// Its day ended 13 days before start's, so it goes on the day after.
	old := filepath.Join(dir, historyFile(start.Local().AddDate(0, 0, -14).Format(historyDay)))
	if err := os.WriteFile(old, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	h.write(readingsOf("work", []quota.Window{session}, clock.now, fromAnswer))
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("the file went on the day it's kept: %v", err)
	}
	clock.now = start.Add(24 * time.Hour)
	h.write(readingsOf("work", []quota.Window{session}, clock.now, fromAnswer))
	if _, err := os.Stat(old); err == nil {
		t.Error("the file stayed on the day after, want it pruned with that day's first write")
	}
}

func TestTheHistoryPrunesOnANewDayWithNothingToWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		clock := newBubbleClock(start)
		h := newHistory(clock.read)
		h.open(dir)
		old := filepath.Join(dir, historyFile(start.Local().AddDate(0, 0, -14).Format(historyDay)))
		if err := os.WriteFile(old, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		stop := keeping(t, h)
		defer stop()

		time.Sleep(25 * time.Hour)
		synctest.Wait()
		if _, err := os.Stat(old); err == nil {
			t.Error("the file stayed a day on, want it pruned though nothing was written")
		}
	})
}

func TestAPrimeIsKeptInTheHistoryAsAPrime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := newBubbleClock(onDay(1, 4, 0))
		r := newPrimingRouter(t, clock.read, newWindowsUpstream(clock), daytime)
		dir := t.TempDir()
		r.history.open(dir)
		stopKeeping := keeping(t, r.history)
		stopPriming := startPriming(r)

		time.Sleep(20 * time.Minute)
		synctest.Wait()
		stopPriming()
		stopKeeping()
		data, err := os.ReadFile(historyOf(dir, onDay(1, 4, 10)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"account":"work","window":"5h"`) || !strings.Contains(string(data), `"source":"prime"`) {
			t.Errorf("the history holds\n%s\nwant work's prime, as a prime", data)
		}
	})
}

// linesWith returns the lines of the log that hold text.
func linesWith(log *logstest.Log, text string) []string {
	return slices.DeleteFunc(log.Lines(), func(line string) bool { return !strings.Contains(line, text) })
}

func TestAHistoryNotYetOpenedTakesNothing(t *testing.T) {
	h := newHistory(at(start))

	h.note(readingsOf("work", []quota.Window{session}, start, fromAnswer))
	if n := len(h.queue); n != 0 {
		t.Errorf("%d readings queued, want none before the history is opened", n)
	}
}
