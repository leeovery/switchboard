package router

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
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

	"github.com/leeovery/switchboard/internal/config"
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

// dayFile is one of the readings history's two files of a local day, by the
// day's date: its plain file, readings-<date>.jsonl, or its compressed file,
// readings-<date>.jsonl.gz.
type dayFile struct {
	date       string
	compressed bool
}

// plainFile is the history's plain file of the local day with the given date.
func plainFile(date string) dayFile {
	return dayFile{date: date}
}

// compressedFile is the history's compressed file of the local day with the
// given date.
func compressedFile(date string) dayFile {
	return dayFile{date: date, compressed: true}
}

// name is the file's name.
func (f dayFile) name() string {
	name := "readings-" + f.date + ".jsonl"
	if f.compressed {
		name += ".gz"
	}
	return name
}

// historyOf is the readings history's plain file of the local day t falls on,
// in dir.
func historyOf(dir string, t time.Time) string {
	return filepath.Join(dir, plainFile(t.Local().Format(time.DateOnly)).name())
}

// workRead is work's session as an answer read it at at, at u of its use.
func workRead(at time.Time, u float64) reading {
	return reading{At: at.UTC(), Account: "work", Window: "5h", Utilization: u, ResetsAt: session.ResetsAt, Source: fromAnswer}
}

// linesOf returns readings as the history's lines.
func linesOf(t *testing.T, readings ...reading) string {
	t.Helper()
	var lines strings.Builder
	for _, r := range readings {
		line, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		lines.Write(line)
		lines.WriteByte('\n')
	}
	return lines.String()
}

// gzipOf returns texts compressed, a gzip member each, one after another.
func gzipOf(t *testing.T, texts ...string) []byte {
	t.Helper()
	var data bytes.Buffer
	for _, text := range texts {
		w := gzip.NewWriter(&data)
		if _, err := w.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return data.Bytes()
}

// writeDay writes the history's file f in dir, holding lines: compressed, as
// a gzip member of their own, when f is.
func writeDay(t *testing.T, dir string, f dayFile, lines string) {
	t.Helper()
	data := []byte(lines)
	if f.compressed {
		data = gzipOf(t, lines)
	}
	if err := os.WriteFile(filepath.Join(dir, f.name()), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// holdsDay reports whether the history in dir holds a file of the local day
// with the given date, plain or compressed.
func holdsDay(dir, date string) bool {
	for _, f := range []dayFile{plainFile(date), compressedFile(date)} {
		if _, err := os.Stat(filepath.Join(dir, f.name())); err == nil {
			return true
		}
	}
	return false
}

func TestTheHistoryHoldsEachReadingThatChangesAWindow(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	dir := filepath.Join(t.TempDir(), "history")
	h := newHistory(config.History{}, at(start))
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
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode() != mode {
			t.Errorf("%s is %v, want %v", filepath.Base(path), info.Mode(), mode)
		}
	}
}

func TestTheHistoryKeepsADaysFilesAsLongAsTheConfigSays(t *testing.T) {
	const day = 24 * time.Hour
	tests := []struct {
		name string
		keep time.Duration
		// days is how many days a day's files are kept once it has ended.
		days int
	}{
		{name: "two weeks where it doesn't say", days: 14},
		{name: "a week and a day", keep: 8 * day, days: 8},
		{name: "400 days", keep: 400 * day, days: 400},
	}
	today := time.Date(2026, 9, 28, 12, 0, 0, 0, time.Local)
	dateOf := func(back int) string { return today.AddDate(0, 0, -back).Format(time.DateOnly) }
	lines := linesOf(t, workRead(start, 0.2))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			// kept says whether a day's files are kept, by how many days before
			// today it is. Those past keeping are compressed as well as plain.
			kept := map[int]bool{0: true, tt.days: true, tt.days + 1: false, tt.days + 30: false}
			for back, stays := range kept {
				writeDay(t, dir, plainFile(dateOf(back)), lines)
				if !stays {
					writeDay(t, dir, compressedFile(dateOf(back)), lines)
				}
			}
			// Anything but the history's own is left alone, as a compressed file
			// left half written.
			others := []string{"notes.txt", "readings-soon.jsonl", "." + compressedFile(dateOf(tt.days+1)).name() + ".2961577413"}
			for _, name := range others {
				if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			h := newHistory(config.History{Keep: tt.keep}, at(today))
			h.open(dir)

			keeping(t, h)()
			for back, stays := range kept {
				if held := holdsDay(dir, dateOf(back)); held != stays {
					t.Errorf("the day %d days back is kept = %v, want %v: a day's files go %d days after it ends", back, held, stays, tt.days)
				}
			}
			for _, name := range others {
				if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
					t.Errorf("%s went (%v), want anything but the history's own files left alone", name, err)
				}
			}
		})
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
	h := newHistory(config.History{}, at(start))
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
	restarted := newHistory(config.History{}, at(now))
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
	h := newHistory(config.History{}, at(now))
	h.open(dir)
	s := newTestState(&testClock{now: now})
	s.usage["work"].windows["5h"] = session
	lines := []reading{{At: start, Account: "work", Window: "5h", Utilization: session.Utilization, ResetsAt: session.ResetsAt, Source: fromAnswer}}
	writeLines(t, h, lines...)

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
	h := newHistory(config.History{}, at(now))
	h.open(dir)
	s := newTestState(&testClock{now: now})
	busier := session
	busier.Utilization = 0.43
	s.usage["work"].windows["5h"] = busier
	writeLines(t, h,
		reading{At: start, Account: "work", Window: "5h", Utilization: 0.23, ResetsAt: session.ResetsAt, Source: fromAnswer},
		reading{At: now.Add(-time.Minute), Account: "work", Window: "5h", Utilization: 0.43, ResetsAt: session.ResetsAt, Source: fromProbe},
	)

	s.seed(h.readBack(now))
	if pace, ok := s.usage["work"].pace(testPolicy, now); !ok || !pace.Recent || math.Abs(pace.Rate-0.1) > 1e-9 {
		t.Errorf("after the restart, work's session is used at %+v, %v, want 10%% an hour, its rise spread over the two hours since it was read", pace, ok)
	}
}

func TestReadingBackTakesTheTwoNewestDays(t *testing.T) {
	now := time.Date(2026, 9, 29, 0, 10, 0, 0, time.Local)
	// The router stopped while it compressed a day ten days back, once it had
	// written the day's compressed file, before it removed its plain one, which
	// holds the last two of these. Read twice, they'd fall back from 0.45 to
	// 0.3, as a reset by hand does.
	stopped := []reading{workRead(now.Add(-240*time.Hour), 0.2), workRead(now.Add(-239*time.Hour), 0.3), workRead(now.Add(-238*time.Hour), 0.45)}
	tests := []struct {
		name string
		// files are the readings each of the history's files holds.
		files map[dayFile][]reading
		want  []reading
	}{
		{
			name: "yesterday's and today's, across midnight",
			files: map[dayFile][]reading{
				plainFile("2026-09-27"): {workRead(now.Add(-26*time.Hour), 0.1)},
				plainFile("2026-09-28"): {workRead(now.Add(-20*time.Minute), 0.2)},
				plainFile("2026-09-29"): {workRead(now.Add(-5*time.Minute), 0.3)},
			},
			want: []reading{workRead(now.Add(-20*time.Minute), 0.2), workRead(now.Add(-5*time.Minute), 0.3)},
		},
		{
			name: "named for a day the clock hasn't come to, as after a change of time zone",
			files: map[dayFile][]reading{
				plainFile("2026-09-28"): {workRead(now.Add(-40*time.Minute), 0.1)},
				plainFile("2026-09-29"): {workRead(now.Add(-20*time.Minute), 0.2)},
				plainFile("2026-09-30"): {workRead(now.Add(-5*time.Minute), 0.3)},
			},
			want: []reading{workRead(now.Add(-20*time.Minute), 0.2), workRead(now.Add(-5*time.Minute), 0.3)},
		},
		{
			name: "compressed, as when the router restarts days after the last reading it wrote",
			files: map[dayFile][]reading{
				compressedFile("2026-09-17"): {workRead(now.Add(-12*24*time.Hour), 0.1)},
				compressedFile("2026-09-18"): {workRead(now.Add(-11*24*time.Hour), 0.2)},
				compressedFile("2026-09-19"): {workRead(now.Add(-10*24*time.Hour), 0.3)},
			},
			want: []reading{workRead(now.Add(-11*24*time.Hour), 0.2), workRead(now.Add(-10*24*time.Hour), 0.3)},
		},
		{
			name: "a day's compressed lines before its plain file's, as when the clock was set back to it",
			files: map[dayFile][]reading{
				plainFile("2026-09-27"):      {workRead(now.Add(-26*time.Hour), 0.05)},
				plainFile("2026-09-28"):      {workRead(now.Add(-20*time.Minute), 0.1)},
				compressedFile("2026-09-29"): {workRead(now.Add(-8*time.Minute), 0.2)},
				plainFile("2026-09-29"):      {workRead(now.Add(-5*time.Minute), 0.3)},
			},
			want: []reading{workRead(now.Add(-20*time.Minute), 0.1), workRead(now.Add(-8*time.Minute), 0.2), workRead(now.Add(-5*time.Minute), 0.3)},
		},
		{
			name: "a day's lines once, when its compressed file ends with its plain file's, as after the router stopped compressing it",
			files: map[dayFile][]reading{
				plainFile("2026-09-18"):      {workRead(now.Add(-264*time.Hour), 0.1)},
				compressedFile("2026-09-19"): stopped,
				plainFile("2026-09-19"):      stopped[1:],
			},
			want: append([]reading{workRead(now.Add(-264*time.Hour), 0.1)}, stopped...),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for f, readings := range tt.files {
				writeDay(t, dir, f, linesOf(t, readings...))
			}
			h := newHistory(config.History{}, at(now))
			h.open(dir)

			if got := slices.Collect(h.readBack(now)); !slices.EqualFunc(got, tt.want, func(a, b reading) bool { return a.At.Equal(b.At) && a.Utilization == b.Utilization }) {
				t.Errorf("readBack() = %+v, want %+v", got, tt.want)
			}
		})
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
	h := newHistory(config.History{}, at(start))
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
	h := newHistory(config.History{}, at(now))
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
	writeLines(t, h, readings...)
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
			h := newHistory(config.History{}, at(now))
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

func TestWhatTheHistoryCantDoIsLoggedAsTheRoutersReadingsHistory(t *testing.T) {
	log := logstest.Capture(t)
	h := newHistory(config.History{}, at(start))
	// The history's directory can't be made where a file is, and nothing
	// writes the history while its queue fills.
	blocker := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	h.open(filepath.Join(blocker, "history"))
	one := readingsOf("work", []quota.Window{session}, start, fromAnswer)
	for range historyQueue + 1 {
		h.note(one)
	}

	keeping(t, h)()
	for range h.readBack(start) {
	}
	for _, want := range []string{
		`msg="readings history fell behind; readings dropped from it"`,
		`msg="can't make the readings history private"`,
		`msg="can't prune the readings history"`,
		`msg="can't write the readings history; readings go unwritten until it can"`,
		`msg="can't read the readings history"`,
	} {
		if !log.Has("level=WARN", want, "component=router") {
			t.Errorf("log reads\n%s\nwant a line of the router's with %s", log, want)
		}
	}
}

func TestAReadingTheHistoryCantHoldIsLoggedOnce(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	h := newHistory(config.History{}, at(start))
	h.open(dir)
	bad := reading{At: start, Account: "work", Window: "5h", Utilization: math.NaN(), Source: fromAnswer}
	good := readingsOf("work", []quota.Window{session}, start, fromAnswer)

	writeLines(t, h, append([]reading{bad}, good...)...)
	writeLines(t, h, bad)
	if got := len(linesWith(log, "readings history can't hold a reading")); got != 1 {
		t.Errorf("log reads\n%s\nwant the reading it can't hold noted once", log)
	}
	if data, err := os.ReadFile(historyOf(dir, start)); err != nil || strings.Count(string(data), "\n") != 1 {
		t.Errorf("the history holds %q (%v), want the reading it could hold alone", data, err)
	}
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
	dir := t.TempDir()
	h := newHistory(config.History{}, at(start))

	h.note(readingsOf("work", []quota.Window{session}, start, fromAnswer))
	h.open(dir)
	keeping(t, h)()
	if _, err := os.Stat(historyOf(dir, start)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the history holds today's file (%v), want nothing written of readings noted before it was opened", err)
	}
}
