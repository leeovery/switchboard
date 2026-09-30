package router

import (
	"context"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
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
	if kept := after.state.seed(restarted.recent(now)); kept != 6 {
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
		{name: "the window as it runs", held: quota.Window{Key: "5h", Utilization: 0.6, ResetsAt: session.ResetsAt}, wantRate: 0.3, wantOK: true},
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

			s.seed(h.recent(now))
			rate, ok := score.RecentRate(tt.held, s.usage["work"].trails["5h"], now)
			if math.Abs(rate-tt.wantRate) > 1e-9 || ok != tt.wantOK {
				t.Errorf("work's session's recent rate = %v, %v, want %v, %v", rate, ok, tt.wantRate, tt.wantOK)
			}
			if !log.Has("level=WARN", `msg="readings history lines unread"`, "lines=4") {
				t.Errorf("log reads\n%s\nwant the four lines that don't read as readings noted", log)
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
	read := func(u float64) {
		busier := session
		busier.Utilization = u
		r.state.record("work", []quota.Window{busier, soonWeek}, r.state.mark())
		r.state.record("side", []quota.Window{session, laterWeek}, r.state.mark())
	}

	stop := keeping(t, r.history)
	read(0.3)
	read(0.4)
	if got := choose(t.Context(), r, Request{ID: "5f3a9c2e", Session: "one", Model: opus, Client: "work"}); got.Account != "work" {
		t.Errorf("a new session went to %q, want work, whatever the history", got.Account)
	}
	stop()
	failures := slices.DeleteFunc(log.Lines(), func(line string) bool { return !strings.Contains(line, "can't write the readings history") })
	if len(failures) != 1 || !strings.Contains(failures[0], "level=WARN") {
		t.Errorf("log reads\n%s\nwant the history's failure noted once", log)
	}

	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	stop = keeping(t, r.history)
	read(0.5)
	stop()
	if !log.Has("level=INFO", `msg="writing the readings history again"`) {
		t.Errorf("log reads\n%s\nwant the history noted as written again", log)
	}
	if _, err := os.Stat(historyOf(filepath.Join(blocker, "history"), start)); err != nil {
		t.Errorf("the history wasn't written once it could be: %v", err)
	}
}

func TestAHistoryFallingBehindDropsReadingsRatherThanWait(t *testing.T) {
	log := logstest.Capture(t)
	h := newHistory(at(start))
	h.open(t.TempDir())
	one := readingsOf("work", []quota.Window{session}, start, fromAnswer)

	for range historyQueue + 2 {
		h.note(one)
	}
	if lines := slices.DeleteFunc(log.Lines(), func(line string) bool { return !strings.Contains(line, "fell behind") }); len(lines) != 1 {
		t.Errorf("log reads\n%s\nwant the readings dropped noted once", log)
	}
}

func TestAHistoryNotYetOpenedTakesNothing(t *testing.T) {
	h := newHistory(at(start))

	h.note(readingsOf("work", []quota.Window{session}, start, fromAnswer))
	if n := len(h.queue); n != 0 {
		t.Errorf("%d readings queued, want none before the history is opened", n)
	}
}
