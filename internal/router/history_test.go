package router

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
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

// historyOf is the readings history's plain file of the local day t falls on,
// in dir.
func historyOf(dir string, t time.Time) string {
	return filepath.Join(dir, plainFile(t.Local().Format(historyDay)).name())
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

// heldIn returns the lines the history's file f in dir holds: a compressed
// file's, its members' one after another.
func heldIn(t *testing.T, dir string, f dayFile) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, f.name()))
	if err != nil {
		t.Fatal(err)
	}
	if !f.compressed {
		return string(data)
	}
	members, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	lines, err := io.ReadAll(members)
	if err != nil {
		t.Fatal(err)
	}
	return string(lines)
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
	dateOf := func(back int) string { return today.AddDate(0, 0, -back).Format(historyDay) }
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

func TestADaysFilesAreNamedForItsDate(t *testing.T) {
	tests := []struct {
		name string
		// want is the history's file it names, the zero dayFile when none.
		want dayFile
	}{
		{name: "readings-2026-09-28.jsonl", want: plainFile("2026-09-28")},
		{name: "readings-2026-09-28.jsonl.gz", want: compressedFile("2026-09-28")},
		{name: ".readings-2026-09-28.jsonl.gz.2961577413"},
		{name: "readings-2026-09-28.jsonl.gz.gz"},
		{name: "readings-2026-09-28.gz"},
		{name: "readings-2026-9-28.jsonl"},
		{name: "readings-soon.jsonl"},
		{name: "notes.txt"},
	}
	for _, tt := range tests {
		f, day, ok := dayFileNamed(tt.name)
		if f != tt.want || ok != (tt.want != dayFile{}) {
			t.Errorf("dayFileNamed(%q) = %+v, %v; want %+v", tt.name, f, ok, tt.want)
		}
		if ok && (f.name() != tt.name || !day.Equal(time.Date(2026, 9, 28, 0, 0, 0, 0, time.Local))) {
			t.Errorf("dayFileNamed(%q) names %q, of %v; want it named as it is, of 28 September", tt.name, f.name(), day)
		}
	}
}

func TestADaysFileIsCompressedOnceItEndedTwoDaysAgo(t *testing.T) {
	day := time.Date(2026, 9, 25, 0, 0, 0, 0, time.Local)
	date := day.Format(historyDay)
	lines := linesOf(t, workRead(day.Add(9*time.Hour), 0.2), workRead(day.Add(10*time.Hour), 0.3))
	tests := []struct {
		name string
		// ended is how long before now the day ended.
		ended time.Duration
		// in is the day's file that holds its lines once the history is
		// pruned, and gone the one that isn't there.
		in, gone dayFile
	}{
		{name: "plain, a minute short of two days on", ended: 48*time.Hour - time.Minute, in: plainFile(date), gone: compressedFile(date)},
		{name: "compressed, two days on", ended: 48 * time.Hour, in: compressedFile(date), gone: plainFile(date)},
		{name: "compressed, a minute past two days on", ended: 48*time.Hour + time.Minute, in: compressedFile(date), gone: plainFile(date)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeDay(t, dir, plainFile(date), lines)
			h := newHistory(config.History{}, at(day.AddDate(0, 0, 1).Add(tt.ended)))
			h.open(dir)

			h.prune()
			if got := heldIn(t, dir, tt.in); got != lines {
				t.Errorf("%s holds\n%s\nwant the day's lines\n%s", tt.in.name(), got, lines)
			}
			info, err := os.Stat(filepath.Join(dir, tt.in.name()))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode() != 0o600 {
				t.Errorf("%s is %v, want %v", tt.in.name(), info.Mode(), fs.FileMode(0o600))
			}
			if _, err := os.Stat(filepath.Join(dir, tt.gone.name())); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("%s is there (%v), want it not", tt.gone.name(), err)
			}
		})
	}
}

func TestCompressingADayAddsItsLinesToTheEndOfItsCompressedFile(t *testing.T) {
	day := time.Date(2026, 9, 25, 12, 0, 0, 0, time.Local)
	date := day.Format(historyDay)
	older, newer := linesOf(t, workRead(day.Add(-3*time.Hour), 0.2)), linesOf(t, workRead(day.Add(-2*time.Hour), 0.3))
	tests := []struct {
		name string
		// members are those the day's compressed file holds already.
		members []string
	}{
		{name: "after its lines, as when the clock was set back to the day", members: []string{older}},
		{name: "not again when it ends with them, as when the router stopped before the plain file went", members: []string{older, newer}},
		{name: "not again when the last of its lines are them", members: []string{older + newer}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, compressedFile(date).name()), gzipOf(t, tt.members...), 0o600); err != nil {
				t.Fatal(err)
			}
			writeDay(t, dir, plainFile(date), newer)
			// Days on, wherever the clocks changed between.
			h := newHistory(config.History{}, at(day.AddDate(0, 0, 4)))
			h.open(dir)

			h.prune()
			if got := heldIn(t, dir, compressedFile(date)); got != older+newer {
				t.Errorf("the day's compressed file holds\n%s\nwant its lines, older first, each once\n%s", got, older+newer)
			}
			if _, err := os.Stat(filepath.Join(dir, plainFile(date).name())); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("the day's plain file is there (%v), want it gone", err)
			}
		})
	}
}

func TestACompressionThatFailsLeavesThePlainFileToCompressAgain(t *testing.T) {
	log := logstest.Capture(t)
	day := time.Date(2026, 9, 25, 12, 0, 0, 0, time.Local)
	date := day.Format(historyDay)
	lines := linesOf(t, workRead(day.Add(-3*time.Hour), 0.2))
	dir := t.TempDir()
	writeDay(t, dir, plainFile(date), lines)
	// The day's compressed file isn't gzip, so its lines can't be added to.
	damaged := filepath.Join(dir, compressedFile(date).name())
	if err := os.WriteFile(damaged, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	// Days on, wherever the clocks changed between.
	h := newHistory(config.History{}, at(day.AddDate(0, 0, 4)))
	h.open(dir)

	h.prune()
	if got := heldIn(t, dir, plainFile(date)); got != lines {
		t.Errorf("the day's plain file holds\n%s\nwant it as it was\n%s", got, lines)
	}
	want := []string{"level=WARN", `msg="can't compress the readings history"`, "file=" + plainFile(date).name(), `error="read ` + damaged + `: gzip: invalid header"`}
	if !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant the compression that failed noted, naming the compressed file it couldn't read: a line with %q", log, want)
	}
	if err := os.Remove(damaged); err != nil {
		t.Fatal(err)
	}
	h.prune()
	if got := heldIn(t, dir, compressedFile(date)); got != lines {
		t.Errorf("at the next prune, the day's compressed file holds\n%s\nwant its lines\n%s", got, lines)
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
	h := newHistory(config.History{}, at(now))
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

func TestADamagedCompressedFileGivesTheLinesBeforeTheDamage(t *testing.T) {
	now := time.Date(2026, 9, 29, 0, 10, 0, 0, time.Local)
	yesterday := compressedFile("2026-09-28")
	first, second := linesOf(t, workRead(now.Add(-30*time.Minute), 0.1)), linesOf(t, workRead(now.Add(-25*time.Minute), 0.2))
	tests := []struct {
		name string
		// data is what yesterday's compressed file holds.
		data []byte
		// want are the uses read back, today's 0.3 last.
		want []float64
		log  []string
	}{
		{
			name: "cut short in its second member",
			data: append(gzipOf(t, first), gzipOf(t, second)[:5]...),
			want: []float64{0.1, 0.3},
			log:  []string{"level=WARN", `msg="readings history read short"`, "file=" + yesterday.name(), `error="unexpected EOF"`},
		},
		{
			name: "not gzip at all",
			data: []byte(first),
			want: []float64{0.3},
			log:  []string{"level=WARN", `msg="can't read the readings history"`, "file=" + yesterday.name(), `error="gzip: invalid header"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, yesterday.name()), tt.data, 0o600); err != nil {
				t.Fatal(err)
			}
			writeDay(t, dir, plainFile("2026-09-29"), linesOf(t, workRead(now.Add(-5*time.Minute), 0.3)))
			h := newHistory(config.History{}, at(now))
			h.open(dir)

			var got []float64
			for r := range h.readBack(now) {
				got = append(got, r.Utilization)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("read back the uses %v, want %v: the lines before the damage, and today's", got, tt.want)
			}
			if !log.Has(tt.log...) {
				t.Errorf("log reads\n%s\nwant the damage noted, a line with %q", log, tt.log)
			}
		})
	}
}

func TestFilesOfADayAfterTomorrowAreKeptButNotTakenUp(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.Local)
	dir := t.TempDir()
	dateOf := func(offset int) string { return now.AddDate(0, 0, offset).Format(historyDay) }
	// A clock once set days ahead named files for days to come, and compressed
	// one as more days went by.
	files := map[dayFile]reading{
		plainFile(dateOf(-1)):     workRead(now.Add(-20*time.Hour), 0.1),
		plainFile(dateOf(0)):      workRead(now.Add(-time.Hour), 0.2),
		plainFile(dateOf(5)):      workRead(now.Add(-time.Minute), 0.9),
		compressedFile(dateOf(6)): workRead(now.Add(-time.Minute), 0.95),
	}
	for f, r := range files {
		writeDay(t, dir, f, linesOf(t, r))
	}
	h := newHistory(config.History{}, at(now))
	h.open(dir)

	got := slices.Collect(h.readBack(now))
	if len(got) != 2 || got[0].Utilization != 0.1 || got[1].Utilization != 0.2 {
		t.Errorf("readBack() = %+v, want yesterday's and today's, not those of days to come", got)
	}
	h.prune()
	for f := range files {
		if _, err := os.Stat(filepath.Join(dir, f.name())); err != nil {
			t.Errorf("%s went (%v), want it kept: the clock may be the one that's wrong", f.name(), err)
		}
	}
}

func TestAClockSetBackKeepsTheDaysItIsBefore(t *testing.T) {
	today := time.Date(2026, 9, 28, 14, 12, 0, 0, time.Local)
	tests := []struct {
		name string
		// now is the clock, set back from today.
		now time.Time
	}{
		{name: "two days", now: today.AddDate(0, 0, -2)},
		{name: "a week", now: today.AddDate(0, 0, -7)},
		{name: "to 2001", now: time.Date(2001, 1, 1, 0, 0, 0, 0, time.Local)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			// The history of the last ten days, the older compressed, as kept
			// before the clock was set back.
			var days []dayFile
			for back := range 10 {
				f := plainFile(today.AddDate(0, 0, -back).Format(historyDay))
				if back >= 3 {
					f = compressedFile(f.date)
				}
				days = append(days, f)
				writeDay(t, dir, f, linesOf(t, workRead(today.AddDate(0, 0, -back), 0.2)))
			}
			h := newHistory(config.History{}, at(tt.now))
			h.open(dir)

			h.prune()
			for _, f := range days {
				if _, err := os.Stat(filepath.Join(dir, f.name())); err != nil {
					t.Errorf("%s went (%v), want every day's file kept", f.name(), err)
				}
			}
		})
	}
}

func TestDatesStepADayAtATimeWhereTheClocksChangeAtMidnight(t *testing.T) {
	santiago, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Fatalf("load America/Santiago: %v", err)
	}
	// Chile's clocks went from 00:00 to 01:00 on 6 September 2026.
	got := datesFrom(time.Date(2026, 9, 4, 15, 0, 0, 0, santiago), time.Date(2026, 9, 8, 10, 0, 0, 0, santiago))
	if want := []string{"2026-09-04", "2026-09-05", "2026-09-06", "2026-09-07", "2026-09-08"}; !slices.Equal(got, want) {
		t.Errorf("datesFrom() = %q, want %q: each day once, the last included", got, want)
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

func TestAHistoryWriteThatFailsIsLoggedOnceUntilOneSucceeds(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	h := newHistory(config.History{}, at(start))
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
	h := newHistory(config.History{}, at(start))
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
	h := newHistory(config.History{}, at(start))
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
	h := newHistory(config.History{}, at(start))
	h.open(dir)

	if got := slices.Collect(h.readBack(start)); len(got) != 0 {
		t.Errorf("readBack() = %+v, want none", got)
	}
	if !log.Has("level=WARN", `msg="can't read the readings history"`) {
		t.Errorf("log reads\n%s\nwant the file that can't be read noted", log)
	}
}

func TestADamagedHistoryFileIsWarnedOfOnceUntilItReads(t *testing.T) {
	yesterday := compressedFile(start.Local().AddDate(0, 0, -1).Format(historyDay))
	lines := linesOf(t, workRead(start.Add(-24*time.Hour), 0.1))
	tests := []struct {
		name string
		// data is what yesterday's compressed file holds, damaged, and
		// warning what it's warned of.
		data    []byte
		warning string
	}{
		{name: "one that can't be read", data: []byte(lines), warning: `msg="can't read the readings history"`},
		{name: "one cut short", data: append(gzipOf(t, lines), gzipOf(t, lines)[:5]...), warning: `msg="readings history read short"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			dir := t.TempDir()
			hold := func(data []byte) {
				if err := os.WriteFile(filepath.Join(dir, yesterday.name()), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			h := newHistory(config.History{}, at(start))
			h.open(dir)
			read := func() { h.windowReadings("5h", []string{"work"}, start.Add(-time.Hour), start) }

			// Read back as the router starts, then read twice as GET /history
			// reads it.
			hold(tt.data)
			for range h.readBack(start) {
			}
			read()
			read()
			warned := linesWith(log, "file="+yesterday.name())
			if len(warned) != 1 || !strings.Contains(warned[0], "level=WARN") || !strings.Contains(warned[0], tt.warning) {
				t.Fatalf("log reads\n%s\nwant the file warned of once, as the router started, a line with %s", log, tt.warning)
			}

			// Mended, it reads to its end; damaged again, it's warned of again.
			hold(gzipOf(t, lines))
			read()
			hold(tt.data)
			read()
			read()
			if warned := linesWith(log, "file="+yesterday.name()); len(warned) != 2 || !strings.Contains(warned[1], tt.warning) {
				t.Errorf("log reads\n%s\nwant the file warned of again once it's damaged after reading to its end, once", log)
			}
		})
	}
}

func TestTheHistorysDirectoryIsMadePrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	h := newHistory(config.History{}, at(start))
	h.open(dir)

	keeping(t, h)()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != fs.ModeDir|0o700 {
		t.Errorf("the history's directory is %v, want %v", info.Mode(), fs.ModeDir|0o700)
	}
}

func TestTheHistoryPrunesOnTheFirstWriteOfANewDay(t *testing.T) {
	dir := t.TempDir()
	clock := &testClock{now: start}
	h := newHistory(config.History{}, clock.read)
	h.open(dir)
	h.prune()
	// Its day ended 13 days before start's, so it goes on the day after.
	old := start.Local().AddDate(0, 0, -14)
	writeDay(t, dir, plainFile(old.Format(historyDay)), linesOf(t, workRead(old, 0.2)))

	h.write(readingsOf("work", []quota.Window{session}, clock.now, fromAnswer))
	if !holdsDay(dir, old.Format(historyDay)) {
		t.Fatal("the day's file went on a day it's kept")
	}
	clock.now = start.Add(24 * time.Hour)
	h.write(readingsOf("work", []quota.Window{session}, clock.now, fromAnswer))
	if holdsDay(dir, old.Format(historyDay)) {
		t.Error("the day's file stayed on the day after, want it pruned with that day's first write")
	}
}

func TestTheHistoryPrunesOnANewDayWithNothingToWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		clock := newBubbleClock(start)
		h := newHistory(config.History{}, clock.read)
		h.open(dir)
		stop := keeping(t, h)
		defer stop()
		synctest.Wait()
		// A day long past keeping, whose file comes once the history has
		// pruned for start's day, so it goes at the next prune.
		old := start.Local().AddDate(0, 0, -30)
		writeDay(t, dir, plainFile(old.Format(historyDay)), linesOf(t, workRead(old, 0.2)))
		y, m, d := start.Local().Date()
		tomorrow := time.Date(y, m, d+1, 0, 0, 0, 0, time.Local)

		time.Sleep(tomorrow.Sub(start) - time.Minute)
		synctest.Wait()
		if !holdsDay(dir, old.Format(historyDay)) {
			t.Fatal("the day's file went before the day after start's, want it kept until the history next prunes")
		}
		// The history looks every pruneLook, and the clocks may change by an
		// hour on the way.
		time.Sleep(time.Minute + 2*pruneLook)
		synctest.Wait()
		if holdsDay(dir, old.Format(historyDay)) {
			t.Error("the day's file stayed on the day after start's, want it pruned though nothing was written")
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
	h := newHistory(config.History{}, at(start))

	h.note(readingsOf("work", []quota.Window{session}, start, fromAnswer))
	if n := len(h.queue); n != 0 {
		t.Errorf("%d readings queued, want none before the history is opened", n)
	}
}
