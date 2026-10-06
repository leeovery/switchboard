package dayfile

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// start is the time by the clock in tests: a Monday, 13:12 UTC.
var start = time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)

// twoWeeks is how long the tests' files are kept from the end of their day,
// where a test doesn't say: as long as the readings history keeps its own
// unless its config says.
const twoWeeks = 14 * 24 * time.Hour

// testLogger is what the tests' files log through.
var testLogger = logs.For("dayfile")

// readingsHistory returns files in dir named and logged of as the readings
// history's are.
func readingsHistory(dir string) *Files {
	return &Files{Dir: dir, Prefix: "readings", Name: "readings history", LineMax: 4096, Logger: testLogger}
}

// requestLedger returns files in dir named and logged of as the request
// ledger's are.
func requestLedger(dir string) *Files {
	return &Files{Dir: dir, Prefix: "requests", Name: "request ledger", LineMax: 4096, Logger: testLogger}
}

// at is a clock stopped at t.
func at(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// linesOf returns texts as lines, a line each.
func linesOf(texts ...string) string {
	var lines strings.Builder
	for _, text := range texts {
		lines.WriteString(text)
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

// writeDay writes the file of f, holding members, one after another:
// compressed, a gzip member each, when the file is.
func writeDay(t *testing.T, f *Files, file dayFile, members ...string) {
	t.Helper()
	data := []byte(strings.Join(members, ""))
	if file.compressed {
		data = gzipOf(t, members...)
	}
	if err := os.WriteFile(f.path(file), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// heldIn returns the lines the file of f holds: a compressed file's, its
// members' one after another.
func heldIn(t *testing.T, f *Files, file dayFile) string {
	t.Helper()
	data, err := os.ReadFile(f.path(file))
	if err != nil {
		t.Fatal(err)
	}
	if !file.compressed {
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

// holdsDay reports whether f holds a file of the local day with the given
// date, plain or compressed.
func holdsDay(f *Files, date string) bool {
	for _, file := range []dayFile{plainFile(date), compressedFile(date)} {
		if _, err := os.Stat(f.path(file)); err == nil {
			return true
		}
	}
	return false
}

// linesWith returns the lines of the log that hold text.
func linesWith(log *logstest.Log, text string) []string {
	return slices.DeleteFunc(log.Lines(), func(line string) bool { return !strings.Contains(line, text) })
}

func TestADaysFilesAreNamedForItsDate(t *testing.T) {
	f := readingsHistory(t.TempDir())
	tests := []struct {
		name string
		// want is the file it names, the zero dayFile when none.
		want dayFile
	}{
		{name: "readings-2026-09-28.jsonl", want: plainFile("2026-09-28")},
		{name: "readings-2026-09-28.jsonl.gz", want: compressedFile("2026-09-28")},
		{name: ".readings-2026-09-28.jsonl.gz.2961577413"},
		{name: "readings-2026-09-28.jsonl.gz.gz"},
		{name: "readings-2026-09-28.gz"},
		{name: "readings-2026-9-28.jsonl"},
		{name: "readings-soon.jsonl"},
		{name: "requests-2026-09-28.jsonl"},
		{name: "notes.txt"},
	}
	for _, tt := range tests {
		file, day, ok := f.named(tt.name)
		if file != tt.want || ok != (tt.want != dayFile{}) {
			t.Errorf("named(%q) = %+v, %v; want %+v", tt.name, file, ok, tt.want)
		}
		if ok && (file.name(f.Prefix) != tt.name || !day.Equal(time.Date(2026, 9, 28, 0, 0, 0, 0, time.Local))) {
			t.Errorf("named(%q) names %q, of %v; want it named as it is, of 28 September", tt.name, file.name(f.Prefix), day)
		}
	}
}

func TestLinesAreAppendedToThePlainFilesOfTheirDays(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	f := readingsHistory(dir)
	yesterday := start.Local().AddDate(0, 0, -1)
	lines := make(Lines)
	lines.Add(yesterday, []byte("one"))
	lines.Add(start, []byte("two"))
	lines.Add(start, []byte("three"))
	more := make(Lines)
	more.Add(start, []byte("four"))

	for _, l := range []Lines{lines, more} {
		if err := f.Append(l); err != nil {
			t.Fatalf("Append() = %v", err)
		}
	}
	for date, want := range map[string]string{dateOf(yesterday): linesOf("one"), dateOf(start): linesOf("two", "three", "four")} {
		if got := heldIn(t, f, plainFile(date)); got != want {
			t.Errorf("the plain file of %s holds\n%s\nwant its day's lines, in the order they came\n%s", date, got, want)
		}
	}
	for path, mode := range map[string]fs.FileMode{dir: fs.ModeDir | 0o700, f.path(plainFile(dateOf(start))): 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode() != mode {
			t.Errorf("%s is %v, want %v", filepath.Base(path), info.Mode(), mode)
		}
	}
}

func TestADaysFilesGoOnceItsDayEndedAsLongAgoAsTheyreKept(t *testing.T) {
	const day = 24 * time.Hour
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.Local)
	dated := func(back int) string { return now.AddDate(0, 0, -back).Format(time.DateOnly) }
	for _, days := range []int{8, 14, 400} {
		t.Run(strconv.Itoa(days)+" days", func(t *testing.T) {
			f := readingsHistory(t.TempDir())
			// kept says whether a day's files are kept, by how many days before
			// now it is. Those past keeping are compressed as well as plain.
			kept := map[int]bool{0: true, days: true, days + 1: false, days + 30: false}
			for back, stays := range kept {
				writeDay(t, f, plainFile(dated(back)), linesOf("a"))
				if !stays {
					writeDay(t, f, compressedFile(dated(back)), linesOf("a"))
				}
			}

			f.prune(now, time.Duration(days)*day)
			for back, stays := range kept {
				if held := holdsDay(f, dated(back)); held != stays {
					t.Errorf("the day %d days back is kept = %v, want %v: a day's files go %d days after it ends", back, held, stays, days)
				}
			}
		})
	}
}

func TestPruningLeavesAnythingButItsOwnFilesAlone(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.Local)
	dir := t.TempDir()
	f, other := readingsHistory(dir), requestLedger(dir)
	old, done := dateOf(now.AddDate(0, 0, -30)), dateOf(now.AddDate(0, 0, -3))
	// Another prefix's files, one past keeping and one of a day done with, and
	// anything else, as a compressed file left half written.
	writeDay(t, other, plainFile(old), linesOf("old"))
	writeDay(t, other, plainFile(done), linesOf("done"))
	others := []string{other.path(plainFile(old)), other.path(plainFile(done))}
	for _, name := range []string{"notes.txt", "day-" + done + ".json", "readings-soon.jsonl", "." + compressedFile(done).name(f.Prefix) + ".2961577413"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		others = append(others, path)
	}
	writeDay(t, f, plainFile(old), linesOf("old"))
	writeDay(t, f, plainFile(done), linesOf("done"))

	f.prune(now, twoWeeks)
	for _, path := range others {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s went (%v), want anything but the files' own left alone", filepath.Base(path), err)
		}
	}
	if holdsDay(f, old) {
		t.Error("the files' own day past keeping is kept, want it gone")
	}
	if got := heldIn(t, f, compressedFile(done)); got != linesOf("done") {
		t.Errorf("the files' own day done with holds\n%s\nwant its lines, compressed\n%s", got, linesOf("done"))
	}
}

func TestADaysFileIsCompressedOnceItEndedTwoDaysAgo(t *testing.T) {
	day := time.Date(2026, 9, 25, 0, 0, 0, 0, time.Local)
	date := day.Format(time.DateOnly)
	lines := linesOf("nine", "ten")
	tests := []struct {
		name string
		// ended is how long before now the day ended.
		ended time.Duration
		// in is the day's file that holds its lines once the files are pruned,
		// and gone the one that isn't there.
		in, gone dayFile
	}{
		{name: "plain, a minute short of two days on", ended: 48*time.Hour - time.Minute, in: plainFile(date), gone: compressedFile(date)},
		{name: "compressed, two days on", ended: 48 * time.Hour, in: compressedFile(date), gone: plainFile(date)},
		{name: "compressed, a minute past two days on", ended: 48*time.Hour + time.Minute, in: compressedFile(date), gone: plainFile(date)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := readingsHistory(t.TempDir())
			writeDay(t, f, plainFile(date), lines)

			f.prune(day.AddDate(0, 0, 1).Add(tt.ended), twoWeeks)
			if got := heldIn(t, f, tt.in); got != lines {
				t.Errorf("%s holds\n%s\nwant the day's lines\n%s", tt.in.name(f.Prefix), got, lines)
			}
			info, err := os.Stat(f.path(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode() != 0o600 {
				t.Errorf("%s is %v, want %v", tt.in.name(f.Prefix), info.Mode(), fs.FileMode(0o600))
			}
			if _, err := os.Stat(f.path(tt.gone)); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("%s is there (%v), want it not", tt.gone.name(f.Prefix), err)
			}
		})
	}
}

func TestCompressingADayAddsItsLinesToTheEndOfItsCompressedFile(t *testing.T) {
	day := time.Date(2026, 9, 25, 12, 0, 0, 0, time.Local)
	date := day.Format(time.DateOnly)
	older, newer := linesOf("nine"), linesOf("ten")
	tests := []struct {
		name string
		// members are those the day's compressed file holds already.
		members []string
	}{
		{name: "after its lines, as when the clock was set back to the day", members: []string{older}},
		{name: "not again when it ends with them, as when a writer stopped before the plain file went", members: []string{older, newer}},
		{name: "not again when the last of its lines are them", members: []string{older + newer}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := readingsHistory(t.TempDir())
			writeDay(t, f, compressedFile(date), tt.members...)
			writeDay(t, f, plainFile(date), newer)

			// Days on, wherever the clocks changed between.
			f.prune(day.AddDate(0, 0, 4), twoWeeks)
			if got := heldIn(t, f, compressedFile(date)); got != older+newer {
				t.Errorf("the day's compressed file holds\n%s\nwant its lines, older first, each once\n%s", got, older+newer)
			}
			if _, err := os.Stat(f.path(plainFile(date))); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("the day's plain file is there (%v), want it gone", err)
			}
		})
	}
}

func TestACompressionThatFailsLeavesThePlainFileToCompressAgain(t *testing.T) {
	log := logstest.Capture(t)
	day := time.Date(2026, 9, 25, 12, 0, 0, 0, time.Local)
	date := day.Format(time.DateOnly)
	lines := linesOf("a line read at nine")
	f := readingsHistory(t.TempDir())
	writeDay(t, f, plainFile(date), lines)
	// The day's compressed file isn't gzip, so its lines can't be added to.
	damaged := f.path(compressedFile(date))
	if err := os.WriteFile(damaged, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	// Days on, wherever the clocks changed between.
	now := day.AddDate(0, 0, 4)

	f.prune(now, twoWeeks)
	if got := heldIn(t, f, plainFile(date)); got != lines {
		t.Errorf("the day's plain file holds\n%s\nwant it as it was\n%s", got, lines)
	}
	want := []string{"level=WARN", `msg="can't compress the readings history"`, "file=" + plainFile(date).name(f.Prefix), `error="read ` + damaged + `: gzip: invalid header"`}
	if !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant the compression that failed noted, naming the compressed file it couldn't read: a line with %q", log, want)
	}
	if err := os.Remove(damaged); err != nil {
		t.Fatal(err)
	}
	f.prune(now, twoWeeks)
	if got := heldIn(t, f, compressedFile(date)); got != lines {
		t.Errorf("at the next prune, the day's compressed file holds\n%s\nwant its lines\n%s", got, lines)
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
			f := readingsHistory(t.TempDir())
			// The files of the last ten days, the older compressed, as kept
			// before the clock was set back.
			var days []dayFile
			for back := range 10 {
				file := plainFile(today.AddDate(0, 0, -back).Format(time.DateOnly))
				if back >= 3 {
					file = compressedFile(file.date)
				}
				days = append(days, file)
				writeDay(t, f, file, linesOf("a"))
			}

			f.prune(tt.now, twoWeeks)
			for _, file := range days {
				if _, err := os.Stat(f.path(file)); err != nil {
					t.Errorf("%s went (%v), want every day's file kept", file.name(f.Prefix), err)
				}
			}
		})
	}
}

func TestTheFilesArentReadWhilePrunedNorPrunedWhileRead(t *testing.T) {
	pruning := func(f *Files) func() {
		f.mu.Lock()
		return f.mu.Unlock
	}
	tests := []struct {
		name string
		// hold holds the files as one at work on them does, returning what lets
		// them go, and run does what must wait for that.
		hold func(f *Files) (release func())
		run  func(f *Files)
	}{
		{name: "a read waits for pruning", hold: pruning, run: func(f *Files) { readAll(f, dateOf(start)) }},
		{name: "finding the newest days waits for pruning", hold: pruning, run: func(f *Files) { f.Newest(start, 2) }},
		{name: "appending waits for pruning", hold: pruning, run: func(f *Files) { _ = f.Append(make(Lines)) }},
		{
			name: "pruning waits for a read",
			hold: func(f *Files) func() {
				f.mu.RLock()
				return f.mu.RUnlock
			},
			run: func(f *Files) { f.prune(start, twoWeeks) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := readingsHistory(t.TempDir())
			release := tt.hold(f)
			done := make(chan struct{})
			go func() {
				defer close(done)
				tt.run(f)
			}()
			select {
			case <-done:
				t.Error("it went ahead while the files were held, want it to wait")
			case <-time.After(50 * time.Millisecond):
			}
			release()
			<-done
		})
	}
}
