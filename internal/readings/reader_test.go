package readings_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/readings"
)

func TestAReaderReadsTheHistoryInTheStateDirectory(t *testing.T) {
	state := t.TempDir()
	if err := os.Mkdir(readings.Dir(state), 0o700); err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 9, 27, 0, 0, 0, 0, time.Local)
	before, first, last := workRead(from.Add(-time.Minute), 0.1), workRead(from.Add(time.Hour), 0.2), workRead(from.Add(2*time.Hour), 0.3)
	writeDay(t, readings.Dir(state), "readings-2026-09-26.jsonl.gz", gzipOf(t, linesOf(t, before)))
	writeDay(t, readings.Dir(state), "readings-2026-09-27.jsonl", []byte(linesOf(t, first, last)))

	got := slices.Collect(readings.NewReader(state, logs.For("cli")).Between(from, from.AddDate(0, 0, 1)))
	if want := []readings.Reading{first, last}; !slices.EqualFunc(got, want, sameReading) {
		t.Errorf("Between() = %+v, want %+v: the readings of the time asked for, as Between reads them", got, want)
	}
}

func TestAReaderWarnsOnceOfTheLinesOfTheHistoryItCouldntRead(t *testing.T) {
	from := time.Date(2026, 9, 27, 0, 0, 0, 0, time.Local)
	first, last := workRead(from.Add(time.Hour), 0.2), workRead(from.Add(2*time.Hour), 0.3)
	tests := []struct {
		name    string
		day     string
		wantLog []string
	}{
		{
			name:    "two damaged lines among the readable",
			day:     linesOf(t, first) + "not a reading\n" + `{"at": "2026-09-27T01:30:00Z", "account":` + "\n" + linesOf(t, last),
			wantLog: []string{"level=WARN", `msg="readings history lines unread"`, "lines=2"},
		},
		{name: "a clean history", day: linesOf(t, first, last)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			state := t.TempDir()
			if err := os.Mkdir(readings.Dir(state), 0o700); err != nil {
				t.Fatal(err)
			}
			writeDay(t, readings.Dir(state), "readings-2026-09-27.jsonl", []byte(tt.day))

			got := slices.Collect(readings.NewReader(state, logs.For("cli")).Between(from, from.AddDate(0, 0, 1)))
			if want := []readings.Reading{first, last}; !slices.EqualFunc(got, want, sameReading) {
				t.Errorf("Between() = %+v, want %+v: the readable lines, as ever", got, want)
			}
			warned := slices.DeleteFunc(log.Lines(), func(line string) bool { return !strings.Contains(line, "level=WARN") })
			switch {
			case tt.wantLog == nil && len(warned) > 0:
				t.Errorf("log reads\n%s\nwant no warning: every line read", log)
			case tt.wantLog != nil && (len(warned) != 1 || !log.Has(tt.wantLog...)):
				t.Errorf("log reads\n%s\nwant one warning of the lines unread, %q", log, tt.wantLog)
			}
		})
	}
}

func TestAReadersTailReadsTheHistoryInTheStateDirectoryAsItGrows(t *testing.T) {
	state := t.TempDir()
	if err := os.Mkdir(readings.Dir(state), 0o700); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 27, 0, 0, 0, 0, time.Local)
	from, end := start.AddDate(0, 0, -7), start.AddDate(0, 0, 1)
	periodBefore := workRead(start.Add(-3*time.Hour), 0.9)
	periodBefore.ResetsAt = start.Add(-150 * time.Minute).UTC()
	runFirst, carried := workRead(start.Add(-2*time.Hour), 0.05), workRead(start.Add(-time.Hour), 0.1)
	first, gained := workRead(start.Add(time.Hour), 0.2), workRead(start.Add(2*time.Hour), 0.3)
	writeDay(t, readings.Dir(state), "readings-2026-09-26.jsonl.gz", gzipOf(t, linesOf(t, periodBefore, runFirst, carried)))
	writeDay(t, readings.Dir(state), "readings-2026-09-27.jsonl", []byte(linesOf(t, first)))
	read := readings.NewReader(state, logs.For("cli")).Tail(from, start, end)

	if got, want := read(), []readings.Reading{runFirst, carried, first}; !slices.EqualFunc(got, want, sameReading) {
		t.Errorf("the first read gave %+v, want %+v: each window's run before the start carried in, then those since", got, want)
	}
	appendDay(t, readings.Dir(state), "readings-2026-09-27.jsonl", linesOf(t, gained))
	if got, want := read(), []readings.Reading{runFirst, carried, first, gained}; !slices.EqualFunc(got, want, sameReading) {
		t.Errorf("once the day's file gained a reading, the read gave %+v, want %+v", got, want)
	}
}

func TestAReaderOfAStateDirectoryThatIsntThereReadsNothing(t *testing.T) {
	state := filepath.Join(t.TempDir(), "switchboard")
	from := time.Date(2026, 9, 27, 0, 0, 0, 0, time.Local)

	if got := slices.Collect(readings.NewReader(state, logs.For("cli")).Between(from, from.AddDate(0, 0, 1))); len(got) > 0 {
		t.Errorf("Between() = %+v, want none", got)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Errorf("the state directory is there (%v), want it left as it was, not there", err)
	}
}

func TestAnEmptyHistoryReadsNothing(t *testing.T) {
	from := time.Date(2026, 9, 27, 0, 0, 0, 0, time.Local)
	if got := slices.Collect(readings.Empty{}.Between(from, from.AddDate(0, 0, 1))); len(got) > 0 {
		t.Errorf("Between() = %+v, want none", got)
	}
	if got := (readings.Empty{}).Tail(from.AddDate(0, 0, -7), from, from.AddDate(0, 0, 1))(); len(got) > 0 {
		t.Errorf("Tail()() = %+v, want none", got)
	}
	if _, err := (readings.Empty{}).FirstDay(); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("FirstDay() fails with %v, want fs.ErrNotExist", err)
	}
}

// The first day the history holds is the oldest its files' names give, plain
// or compressed; with none, or no directory, there's none.
func TestAReadersFirstDayIsTheOldestTheHistoryHolds(t *testing.T) {
	read := linesOf(t, workRead(time.Date(2026, 9, 27, 9, 0, 0, 0, time.Local), 0.1))
	tests := []struct {
		name  string
		files map[string][]byte
		want  time.Time
	}{
		{name: "none held"},
		{name: "a day held", files: map[string][]byte{"readings-2026-09-27.jsonl": []byte(read)}, want: time.Date(2026, 9, 27, 0, 0, 0, 0, time.Local)},
		{
			name:  "an older day compressed",
			files: map[string][]byte{"readings-2026-09-27.jsonl": []byte(read), "readings-2026-09-25.jsonl.gz": gzipOf(t, read)},
			want:  time.Date(2026, 9, 25, 0, 0, 0, 0, time.Local),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			if err := os.Mkdir(readings.Dir(state), 0o700); err != nil {
				t.Fatal(err)
			}
			for name, data := range tt.files {
				writeDay(t, readings.Dir(state), name, data)
			}
			got, err := readings.NewReader(state, logs.For("cli")).FirstDay()
			if tt.want.IsZero() != errors.Is(err, fs.ErrNotExist) || !got.Equal(tt.want) {
				t.Errorf("FirstDay() = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestAReaderOfAStateDirectoryThatIsntThereHoldsNoFirstDay(t *testing.T) {
	if _, err := readings.NewReader(filepath.Join(t.TempDir(), "switchboard"), logs.For("cli")).FirstDay(); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("FirstDay() fails with %v, want fs.ErrNotExist", err)
	}
}
