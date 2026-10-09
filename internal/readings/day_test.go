package readings_test

import (
	"cmp"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

// read is a's window with the given key read at at, at u of its use.
func read(account, key string, at time.Time, u float64) readings.Reading {
	return readings.Reading{At: at.UTC(), Account: account, Key: key, Utilization: u, ResetsAt: at.Add(5 * time.Hour).UTC(), Status: quota.StatusAllowed,
		Source: readings.FromAnswer}
}

// wholeDay returns what a summary of the local day that starts at start and
// ends at end reads of the history in files, as a whole read of it from from
// up to the day's end gives it: the last reading of each window before the
// day, ordered by when they were read, then the account's and window's, and
// the day's, in the order they were read.
func wholeDay(files *dayfile.Files, from, start, end time.Time) []readings.Reading {
	type window struct{ account, key string }
	last := make(map[window]readings.Reading)
	var day []readings.Reading
	for r := range readings.Between(files, from, end) {
		if r.At.Before(start) {
			last[window{account: r.Account, key: r.Key}] = r
			continue
		}
		day = append(day, r)
	}
	before := slices.SortedFunc(maps.Values(last), func(a, b readings.Reading) int {
		return cmp.Or(a.At.Compare(b.At), strings.Compare(a.Account, b.Account), strings.Compare(a.Key, b.Key))
	})
	return append(before, day...)
}

// appendDay appends lines to the plain file with the given name in dir.
func appendDay(t *testing.T, dir, name, lines string) {
	t.Helper()
	out, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := out.WriteString(lines); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// compressDay compresses the history's plain file in dir of the local day
// with the given date, as a prune does: its lines added to the day's
// compressed file, as a gzip member of their own, and the plain file gone.
func compressDay(t *testing.T, dir, date string) {
	t.Helper()
	plain, compressed := filepath.Join(dir, "readings-"+date+".jsonl"), filepath.Join(dir, "readings-"+date+".jsonl.gz")
	lines, err := os.ReadFile(plain)
	if err != nil {
		t.Fatal(err)
	}
	held, err := os.ReadFile(compressed)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.WriteFile(compressed, append(held, gzipOf(t, string(lines))...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(plain); err != nil {
		t.Fatal(err)
	}
}

func TestADaysReadingsReadAsTheyGrowAreThoseAWholeReadGivesItsSummary(t *testing.T) {
	start := time.Date(2026, 9, 27, 0, 0, 0, 0, time.Local)
	end, from := dayfile.DayStart(start, 1), start.Add(-7*24*time.Hour)
	on := func(days, hour, minute int) time.Time {
		return time.Date(2026, 9, 27+days, hour, minute, 0, 0, time.Local)
	}
	name := func(days int) string { return "readings-" + on(days, 12, 0).Format(time.DateOnly) + ".jsonl" }
	steps := []struct {
		name string
		do   func(t *testing.T, dir string)
	}{
		{
			name: "the week before, its oldest days compressed, a reading from before it, and the day's first",
			do: func(t *testing.T, dir string) {
				appendDay(t, dir, name(-8), linesOf(t, read("work", "7d", on(-8, 12, 0), 0.9)))
				appendDay(t, dir, name(-6), linesOf(t, read("work", "7d", on(-6, 9, 0), 0.1), read("personal", "7d", on(-6, 10, 0), 0.2)))
				appendDay(t, dir, name(-3), linesOf(t, read("work", "7d", on(-3, 9, 0), 0.3)))
				for _, days := range []int{-8, -6, -3} {
					compressDay(t, dir, on(days, 12, 0).Format(time.DateOnly))
				}
				appendDay(t, dir, name(-1), linesOf(t, read("work", "5h", on(-1, 22, 0), 0.4), read("work", "7d", on(-1, 22, 0), 0.5)))
				appendDay(t, dir, name(0), linesOf(t, read("work", "5h", on(0, 9, 0), 0.1)))
			},
		},
		{
			name: "the day's readings appended to",
			do: func(t *testing.T, dir string) {
				appendDay(t, dir, name(0), linesOf(t, read("work", "5h", on(0, 10, 0), 0.2), read("personal", "5h", on(0, 10, 5), 0.3)))
			},
		},
		{
			name: "a reading filed late into the day before's file",
			do: func(t *testing.T, dir string) {
				appendDay(t, dir, name(-1), linesOf(t, read("work", "5h", on(-1, 23, 59), 0.45)))
			},
		},
		{
			name: "one of the day filed under the day after, as after a change of time zone, and one of the day after",
			do: func(t *testing.T, dir string) {
				appendDay(t, dir, name(1), linesOf(t, read("work", "5h", on(0, 23, 30), 0.6), read("work", "5h", on(1, 1, 0), 0.7)))
			},
		},
		{
			name: "one of the week before filed under the day, as after a change of time zone, and read at the same time as another",
			do: func(t *testing.T, dir string) {
				appendDay(t, dir, name(0), linesOf(t, read("work", "7d", on(-1, 22, 0), 0.55)))
			},
		},
		{name: "the day before compressed", do: func(t *testing.T, dir string) { compressDay(t, dir, on(-1, 12, 0).Format(time.DateOnly)) }},
		{
			name: "the oldest of the week pruned",
			do: func(t *testing.T, dir string) {
				if err := os.Remove(filepath.Join(dir, name(-6)+".gz")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{name: "nothing changed", do: func(*testing.T, string) {}},
	}
	dir := t.TempDir()
	files := readings.Files(dir, logs.For("cli"))
	day := readings.NewDay(files, from, start, end)
	for _, step := range steps {
		step.do(t, dir)

		got, want := day.Read(), wholeDay(files, from, start, end)
		if !slices.EqualFunc(got, want, sameReading) {
			t.Errorf("%s: Read() =\n%+v\nwant\n%+v, as a whole read gives", step.name, got, want)
		}
	}
}
