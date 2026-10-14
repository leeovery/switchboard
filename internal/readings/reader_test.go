package readings_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs"
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

func TestAReadersTailReadsTheHistoryInTheStateDirectoryAsItGrows(t *testing.T) {
	state := t.TempDir()
	if err := os.Mkdir(readings.Dir(state), 0o700); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 27, 0, 0, 0, 0, time.Local)
	from, end := start.AddDate(0, 0, -7), start.AddDate(0, 0, 1)
	carried, first, gained := workRead(start.Add(-time.Hour), 0.1), workRead(start.Add(time.Hour), 0.2), workRead(start.Add(2*time.Hour), 0.3)
	writeDay(t, readings.Dir(state), "readings-2026-09-26.jsonl.gz", gzipOf(t, linesOf(t, workRead(start.Add(-2*time.Hour), 0.05), carried)))
	writeDay(t, readings.Dir(state), "readings-2026-09-27.jsonl", []byte(linesOf(t, first)))
	read := readings.NewReader(state, logs.For("cli")).Tail(from, start, end)

	if got, want := read(), []readings.Reading{carried, first}; !slices.EqualFunc(got, want, sameReading) {
		t.Errorf("the first read gave %+v, want %+v: each window's last before the start carried in, then those since", got, want)
	}
	appendDay(t, readings.Dir(state), "readings-2026-09-27.jsonl", linesOf(t, gained))
	if got, want := read(), []readings.Reading{carried, first, gained}; !slices.EqualFunc(got, want, sameReading) {
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
}
