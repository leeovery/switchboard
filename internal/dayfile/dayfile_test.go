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
// where a test doesn't say.
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
		if date, ok := f.DateOf(tt.name); date != tt.want.date || ok != (tt.want != dayFile{}) {
			t.Errorf("DateOf(%q) = %q, %v; want %q", tt.name, date, ok, tt.want.date)
		}
	}
}

func TestADaysDateGivesWhenItStartsAndEnds(t *testing.T) {
	start, end, ok := Day("2026-09-28")
	if !ok || !start.Equal(time.Date(2026, 9, 28, 0, 0, 0, 0, time.Local)) || !end.Equal(time.Date(2026, 9, 29, 0, 0, 0, 0, time.Local)) {
		t.Errorf("Day(2026-09-28) = %v, %v, %v; want its local midnight and the next day's", start, end, ok)
	}
	for _, date := range []string{"2026-9-28", "2026-09-31", "soon", ""} {
		if _, _, ok := Day(date); ok {
			t.Errorf("Day(%q) reports a day, want none: it isn't a date", date)
		}
	}
}

func TestADayStartsAtItsFirstInstantWhereTheClocksGoForwardAtMidnight(t *testing.T) {
	tests := []struct {
		zone string
		// date is of a day the clocks went forward at its midnight, from
		// 00:00 to 01:00, and before of the day before it.
		date, before string
	}{
		{zone: "America/Santiago", date: "2026-09-06", before: "2026-09-05"},
		{zone: "America/Havana", date: "2026-03-08", before: "2026-03-07"},
		{zone: "Atlantic/Azores", date: "2026-03-29", before: "2026-03-28"},
	}
	for _, tt := range tests {
		t.Run(tt.zone, func(t *testing.T) {
			loc, err := time.LoadLocation(tt.zone)
			if err != nil {
				t.Fatalf("load %s: %v", tt.zone, err)
			}
			day, _ := time.Parse(time.DateOnly, tt.date)
			y, m, d := day.Date()
			forward, nextMidnight := time.Date(y, m, d, 1, 0, 0, 0, loc), time.Date(y, m, d+1, 0, 0, 0, 0, loc)

			start, end, ok := dayIn(tt.date, loc)
			if !ok || !start.Equal(forward) || !end.Equal(nextMidnight) {
				t.Errorf("the day of %s starts at %v and ends at %v (%v), want %v, as the clocks went forward, and %v", tt.date, start, end, ok, forward, nextMidnight)
			}
			beforeStart, beforeEnd, _ := dayIn(tt.before, loc)
			if !beforeStart.Equal(time.Date(y, m, d-1, 0, 0, 0, 0, loc)) || !beforeEnd.Equal(forward) {
				t.Errorf("the day before starts at %v and ends at %v, want its midnight, and %v, as the clocks went forward", beforeStart, beforeEnd, forward)
			}
			for _, within := range []time.Time{forward, forward.Add(11 * time.Hour), nextMidnight.Add(-time.Nanosecond)} {
				if got := DayStart(within, 0); !got.Equal(forward) {
					t.Errorf("DayStart(%v, 0) = %v, want %v", within, got, forward)
				}
			}
			if got := DayStart(beforeStart, 1); !got.Equal(forward) {
				t.Errorf("DayStart(%v, 1) = %v, want %v", beforeStart, got, forward)
			}
			if got := DayStart(nextMidnight, -1); !got.Equal(forward) {
				t.Errorf("DayStart(%v, -1) = %v, want %v", nextMidnight, got, forward)
			}
		})
	}
}

func TestADayStartsAtTheFirstOfItsMidnightsWhereTheClocksGoBackOverIt(t *testing.T) {
	tests := []struct {
		zone string
		// date is of a day the clocks went back over the midnight it starts
		// at, in a zone ahead of UTC, so the midnight came twice, first at
		// first; and before is of the day before it.
		date, before string
		first        time.Time
	}{
		{zone: "Asia/Gaza", date: "2020-10-24", before: "2020-10-23", first: time.Date(2020, 10, 23, 21, 0, 0, 0, time.UTC)},
		{zone: "Asia/Amman", date: "2020-10-30", before: "2020-10-29", first: time.Date(2020, 10, 29, 21, 0, 0, 0, time.UTC)},
		{zone: "Antarctica/Casey", date: "2023-03-09", before: "2023-03-08", first: time.Date(2023, 3, 8, 13, 0, 0, 0, time.UTC)},
		{zone: "Antarctica/Vostok", date: "2023-12-18", before: "2023-12-17", first: time.Date(2023, 12, 17, 17, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.zone, func(t *testing.T) {
			loc, err := time.LoadLocation(tt.zone)
			if err != nil {
				t.Fatalf("load %s: %v", tt.zone, err)
			}
			day, _ := time.Parse(time.DateOnly, tt.date)
			y, m, d := day.Date()
			nextMidnight := time.Date(y, m, d+1, 0, 0, 0, 0, loc)

			start, end, ok := dayIn(tt.date, loc)
			if !ok || !start.Equal(tt.first) || !end.Equal(nextMidnight) {
				t.Errorf("the day of %s starts at %v and ends at %v (%v), want its first midnight, %v, and %v", tt.date, start, end, ok, tt.first.In(loc), nextMidnight)
			}
			if _, beforeEnd, _ := dayIn(tt.before, loc); !beforeEnd.Equal(tt.first) {
				t.Errorf("the day before ends at %v, want the day's first midnight, %v", beforeEnd, tt.first.In(loc))
			}
			// Each instant of the day, those before its second midnight among
			// them, starts there.
			for _, within := range []time.Time{tt.first, tt.first.Add(30 * time.Minute), tt.first.Add(11 * time.Hour), nextMidnight.Add(-time.Nanosecond)} {
				if got := DayStart(within.In(loc), 0); !got.Equal(tt.first) {
					t.Errorf("DayStart(%v, 0) = %v, want %v", within.In(loc), got, tt.first.In(loc))
				}
			}
			if got := DayStart(nextMidnight, -1); !got.Equal(tt.first) {
				t.Errorf("DayStart(%v, -1) = %v, want %v", nextMidnight, got, tt.first.In(loc))
			}
		})
	}
}

func TestATimeOfDayIsWhenTheClocksFirstReadIt(t *testing.T) {
	tests := []struct {
		name, zone string
		// date and clock are the day and the time of day asked for, and want
		// when it is, in UTC.
		date, clock string
		want        time.Time
	}{
		{name: "a time of an ordinary day", zone: "Europe/London", date: "2026-10-07", clock: "14:00", want: time.Date(2026, 10, 7, 13, 0, 0, 0, time.UTC)},
		{name: "the midnight the clocks went forward over, west of UTC, as they did", zone: "America/Santiago", date: "2026-09-06", clock: "00:00",
			want: time.Date(2026, 9, 6, 4, 0, 0, 0, time.UTC)},
		{name: "a time the clocks went forward over, west of UTC, as they did", zone: "America/New_York", date: "2026-03-08", clock: "02:30",
			want: time.Date(2026, 3, 8, 7, 0, 0, 0, time.UTC)},
		{name: "a time the clocks went forward over, east of UTC, as they did", zone: "Europe/Berlin", date: "2026-03-29", clock: "02:30",
			want: time.Date(2026, 3, 29, 1, 0, 0, 0, time.UTC)},
		{name: "the midnight the clocks went forward over, east of UTC, as they did", zone: "Africa/Cairo", date: "2026-04-24", clock: "00:00",
			want: time.Date(2026, 4, 23, 22, 0, 0, 0, time.UTC)},
		{name: "the first of a midnight that came twice", zone: "Asia/Gaza", date: "2020-10-24", clock: "00:00", want: time.Date(2020, 10, 23, 21, 0, 0, 0, time.UTC)},
		{name: "the first of a time that came twice, east of UTC", zone: "Europe/Berlin", date: "2026-10-25", clock: "02:30",
			want: time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)},
		{name: "the first of a time that came twice, west of UTC", zone: "America/New_York", date: "2026-11-01", clock: "01:30",
			want: time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := time.LoadLocation(tt.zone)
			if err != nil {
				t.Fatalf("load %s: %v", tt.zone, err)
			}
			day, _ := time.Parse(time.DateOnly, tt.date)
			clock, _ := time.Parse("15:04", tt.clock)
			y, m, d := day.Date()

			if got := TimeOfDay(time.Date(y, m, d, 12, 0, 0, 0, loc), clock.Hour(), clock.Minute()); !got.Equal(tt.want) {
				t.Errorf("TimeOfDay(%s, %s) in %s = %v, want %v", tt.date, tt.clock, tt.zone, got.UTC(), tt.want)
			}
		})
	}
}

func TestADayEndsAtTheNextOnesStartWhereTheClocksGoBackAtMidnight(t *testing.T) {
	santiago, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Fatalf("load America/Santiago: %v", err)
	}
	// Chile's clocks went back from 00:00 on 5 April 2026 to 23:00 on the 4th,
	// which ran 25 hours.
	start, end, ok := dayIn("2026-04-04", santiago)
	if !ok || !start.Equal(time.Date(2026, 4, 4, 0, 0, 0, 0, santiago)) || end.Sub(start) != 25*time.Hour {
		t.Errorf("the 4th runs from %v to %v (%v), want from its midnight for 25 hours", start, end, ok)
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
	for date, want := range map[string]string{DateOf(yesterday): linesOf("one"), DateOf(start): linesOf("two", "three", "four")} {
		if got := heldIn(t, f, plainFile(date)); got != want {
			t.Errorf("the plain file of %s holds\n%s\nwant its day's lines, in the order they came\n%s", date, got, want)
		}
	}
	for path, mode := range map[string]fs.FileMode{dir: fs.ModeDir | 0o700, f.path(plainFile(DateOf(start))): 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode() != mode {
			t.Errorf("%s is %v, want %v", filepath.Base(path), info.Mode(), mode)
		}
	}
}

func TestAnAppendStartsALineOfItsOwnAfterOneCutShort(t *testing.T) {
	const appended = `{"n":3}`
	tests := []struct {
		name string
		// held is what the day's plain file holds before the append, and
		// wantHeld what it holds after.
		held, wantHeld string
		// wantRead are the lines read back after the append, and wantUnread
		// how many lines weren't.
		wantRead   []string
		wantUnread int
	}{
		{
			name:       "a line cut short, ended first, and passed over in reading",
			held:       linesOf(`{"n":1}`) + `{"n":`,
			wantHeld:   linesOf(`{"n":1}`, `{"n":`, appended),
			wantRead:   []string{`{"n":1}`, appended},
			wantUnread: 1,
		},
		{
			name:     "a line cut short of its line ending alone, ended first, and read",
			held:     linesOf(`{"n":1}`) + `{"n":2}`,
			wantHeld: linesOf(`{"n":1}`, `{"n":2}`, appended),
			wantRead: []string{`{"n":1}`, `{"n":2}`, appended},
		},
		{
			name:     "whole lines, after which it starts",
			held:     linesOf(`{"n":1}`),
			wantHeld: linesOf(`{"n":1}`, appended),
			wantRead: []string{`{"n":1}`, appended},
		},
		{
			name:     "an empty file, where it starts",
			wantHeld: linesOf(appended),
			wantRead: []string{appended},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())
			date := DateOf(start)
			writeDay(t, f, plainFile(date), tt.held)
			lines := make(Lines)
			lines.Add(start, []byte(appended))

			if err := f.Append(lines); err != nil {
				t.Fatalf("Append() = %v", err)
			}
			if got := heldIn(t, f, plainFile(date)); got != tt.wantHeld {
				t.Errorf("the day's plain file holds\n%s\nwant\n%s", got, tt.wantHeld)
			}
			if read, unread := readJSON(f, date); !slices.Equal(read, tt.wantRead) || unread != tt.wantUnread {
				t.Errorf("read %q, %d unread; want %q, %d unread", read, unread, tt.wantRead, tt.wantUnread)
			}
		})
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

func TestADaysFilesKeptForeverAreCompressedButNeverGo(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.Local)
	today := DateOf(now)
	// Today's, then days done with: one, one past the keep the config gives
	// unless it says, and one that ended longer ago than a time.Duration
	// holds.
	dates := []string{today, DateOf(now.AddDate(0, 0, -3)), DateOf(now.AddDate(0, 0, -401)), "0001-01-01"}
	for _, files := range []func(dir string) *Files{readingsHistory, requestLedger} {
		f := files(t.TempDir())
		t.Run(f.Name, func(t *testing.T) {
			for _, date := range dates {
				writeDay(t, f, plainFile(date), linesOf(date))
			}

			f.prune(now, Forever)
			for _, date := range dates {
				file := compressedFile(date)
				if date == today {
					file = plainFile(date)
				}
				if !holdsDay(f, date) {
					t.Errorf("the day of %s went, want every day kept forever", date)
				} else if got := heldIn(t, f, file); got != linesOf(date) {
					t.Errorf("%s holds %q, want the day's lines %q", file.name(f.Prefix), got, linesOf(date))
				}
			}
		})
	}
}

func TestADaysFilesAreKeptUntilTheDayOfTheirLastInstantKept(t *testing.T) {
	const day = 24 * time.Hour
	tests := []struct {
		name, zone, date string
		keep             time.Duration
		want             string
	}{
		{name: "a week and a day", zone: "UTC", date: "2026-10-07", keep: 8 * day, want: "2026-10-15"},
		{name: "90 days", zone: "UTC", date: "2026-09-30", keep: 90 * day, want: "2026-12-29"},
		{name: "90 days, the clocks going back between", zone: "Europe/London", date: "2026-09-30", keep: 90 * day, want: "2026-12-29"},
		{name: "400 days, the clocks going back and forward between", zone: "Europe/London", date: "2026-09-30", keep: 400 * day, want: "2027-11-04"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := time.LoadLocation(tt.zone)
			if err != nil {
				t.Fatalf("load %s: %v", tt.zone, err)
			}
			if got, ok := keptUntilIn(tt.date, tt.keep, loc); got != tt.want || !ok {
				t.Errorf("the files of %s kept %v are kept until %q (%v), want %q", tt.date, tt.keep, got, ok, tt.want)
			}
		})
	}
	for _, date := range []string{"2026-10-07", "soon"} {
		if got, ok := KeptUntil(date, Forever); ok {
			t.Errorf("the files of %s kept forever are kept until %q, want for good", date, got)
		}
	}
	if got, ok := KeptUntil("soon", 8*day); ok {
		t.Errorf("the files of a day that isn't one are kept until %q, want none", got)
	}
}

func TestADaysFilesGoTheDayAfterTheyreKeptUntil(t *testing.T) {
	const date = "2026-09-30"
	for _, days := range []int{8, 90, 400} {
		t.Run(strconv.Itoa(days)+" days", func(t *testing.T) {
			keep := time.Duration(days) * 24 * time.Hour
			until, _ := KeptUntil(date, keep)
			last, after, _ := Day(until)
			for _, tt := range []struct {
				now   time.Time
				stays bool
			}{{now: last, stays: true}, {now: after, stays: false}} {
				f := readingsHistory(t.TempDir())
				writeDay(t, f, compressedFile(date), linesOf("a"))
				f.prune(tt.now, keep)
				if held := holdsDay(f, date); held != tt.stays {
					t.Errorf("pruned at %v, the files of %s, kept until %s, are kept = %v, want %v", tt.now, date, until, held, tt.stays)
				}
			}
		})
	}
}

func TestPruningLeavesAnythingButItsOwnFilesAlone(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.Local)
	dir := t.TempDir()
	f, other := readingsHistory(dir), requestLedger(dir)
	old, done := DateOf(now.AddDate(0, 0, -30)), DateOf(now.AddDate(0, 0, -3))
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
	// torn is older and a line cut short after them.
	torn := older + "elev"
	tests := []struct {
		name string
		// members are those the day's compressed file holds already, and want
		// the lines it holds once the day's compressed.
		members []string
		want    string
	}{
		{name: "after its lines, as when the clock was set back to the day", members: []string{older}, want: older + newer},
		{name: "not again when it ends with them, as when a writer stopped before the plain file went", members: []string{older, newer}, want: older + newer},
		{name: "not again when the last of its lines are them", members: []string{older + newer}, want: older + newer},
		{name: "after a line ending, where its lines end in one cut short", members: []string{torn}, want: torn + "\n" + newer},
		{
			name:    "not again when it ends with them after that line ending, as when a writer stopped before the plain file went",
			members: []string{torn, "\n" + newer},
			want:    torn + "\n" + newer,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := readingsHistory(t.TempDir())
			writeDay(t, f, compressedFile(date), tt.members...)
			writeDay(t, f, plainFile(date), newer)

			// Days on, wherever the clocks changed between.
			f.prune(day.AddDate(0, 0, 4), twoWeeks)
			if got := heldIn(t, f, compressedFile(date)); got != tt.want {
				t.Errorf("the day's compressed file holds\n%s\nwant its lines, older first, each once\n%s", got, tt.want)
			}
			if _, err := os.Stat(f.path(plainFile(date))); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("the day's plain file is there (%v), want it gone", err)
			}
		})
	}
}

func TestALineCutShortIsCompressedAsItIsAndTheLinesCompressedAfterItRead(t *testing.T) {
	day := time.Date(2026, 9, 25, 12, 0, 0, 0, time.Local)
	date := day.Format(time.DateOnly)
	torn := linesOf(`{"n":1}`) + `{"n":`
	f := requestLedger(t.TempDir())
	writeDay(t, f, plainFile(date), torn)
	// Days on, wherever the clocks changed between.
	later := day.AddDate(0, 0, 4)

	f.prune(later, twoWeeks)
	if got := heldIn(t, f, compressedFile(date)); got != torn {
		t.Errorf("the day's compressed file holds %q, want its lines as they were, the one cut short too: %q", got, torn)
	}
	// The clock set back to the day, a line is written to it again, and then
	// compressed in turn.
	lines := make(Lines)
	lines.Add(day, []byte(`{"n":3}`))
	if err := f.Append(lines); err != nil {
		t.Fatalf("Append() = %v", err)
	}
	f.prune(later, twoWeeks)
	if read, unread := readJSON(f, date); !slices.Equal(read, []string{`{"n":1}`, `{"n":3}`}) || unread != 1 {
		t.Errorf(`read %q, %d unread; want {"n":1} and {"n":3}, and the line cut short alone unread`, read, unread)
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
		{name: "a read waits for pruning", hold: pruning, run: func(f *Files) { readAll(f, DateOf(start)) }},
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
