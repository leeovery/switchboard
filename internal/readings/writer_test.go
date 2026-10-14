package readings_test

import (
	"context"
	"math"
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

// keep is how long the tests' writers keep a day's files.
const keep = 30 * 24 * time.Hour

// write notes each taking in to a writer of the history in dir, by a clock
// stopped at now, and has it write them all before it returns.
func write(t *testing.T, dir string, now time.Time, taken ...[]readings.Reading) {
	t.Helper()
	w := readings.Open(readings.Files(dir, logs.For("router")), keep, func() time.Time { return now }, nil)
	for _, read := range taken {
		w.Note(read)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	w.Run(ctx)
}

// dayLines returns the lines of the plain file of the local day with the
// given date in dir.
func dayLines(t *testing.T, dir, date string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "readings-"+date+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func TestTheWriterFilesEachReadingAsALineUnderTheLocalDayItWasReadOn(t *testing.T) {
	dir := t.TempDir()
	late := time.Date(2026, 9, 27, 23, 30, 0, 0, time.Local)
	early := late.Add(time.Hour)
	first, second, third := workRead(late, 0.2), workRead(late.Add(time.Minute), 0.25), workRead(early, 0.3)

	write(t, dir, early, []readings.Reading{first, second}, []readings.Reading{third})

	for date, want := range map[string][]string{
		"2026-09-27": {
			`{"at":"` + first.At.Format(time.RFC3339) + `","account":"work","window":"5h","utilization":0.2,"resets_at":"2026-09-28T18:10:00Z","status":"allowed","source":"answer"}`,
			`{"at":"` + second.At.Format(time.RFC3339) + `","account":"work","window":"5h","utilization":0.25,"resets_at":"2026-09-28T18:10:00Z","status":"allowed","source":"answer"}`,
		},
		"2026-09-28": {
			`{"at":"` + third.At.Format(time.RFC3339) + `","account":"work","window":"5h","utilization":0.3,"resets_at":"2026-09-28T18:10:00Z","status":"allowed","source":"answer"}`,
		},
	} {
		if got := dayLines(t, dir, date); !slices.Equal(got, want) {
			t.Errorf("%s's file holds\n%s\nwant\n%s", date, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

func TestWhatTheWriterWroteReadsBackAsTheReadingsNoted(t *testing.T) {
	dir := t.TempDir()
	from := time.Date(2026, 9, 27, 22, 0, 0, 0, time.Local)
	noted := []readings.Reading{workRead(from, 0.1), workRead(from.Add(90*time.Minute), 0.2), workRead(from.Add(3*time.Hour), 0.3)}

	write(t, dir, from.Add(3*time.Hour), noted[:2], noted[2:])

	got := slices.Collect(readings.Between(readings.Files(dir, logs.For("cli")), from, from.Add(4*time.Hour), nil))
	if !slices.EqualFunc(got, noted, sameReading) {
		t.Errorf("Between() = %+v, want %+v: the readings noted, as they were read, across the days' files", got, noted)
	}
}

func TestTheWriterQueues1024TakingsInAndDropsThoseBeyond(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.Local)
	taken := make([][]readings.Reading, 1030)
	for i := range taken {
		taken[i] = []readings.Reading{workRead(now, float64(i)/1030)}
	}

	write(t, dir, now, taken...)

	if got := dayLines(t, dir, "2026-09-27"); len(got) != 1024 {
		t.Errorf("the file holds %d lines, want the 1,024 queued", len(got))
	}
	if dropped := linesWith(log, "fell behind"); len(dropped) != 1 || !strings.Contains(dropped[0], `msg="readings history fell behind; readings dropped from it"`) {
		t.Errorf("log reads\n%s\nwant the readings dropped warned of once", log)
	}
}

func TestAReadingTheHistoryCantHoldIsLoggedOnce(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.Local)
	bad := readings.Reading{At: now, Account: "work", Key: "5h", Utilization: math.NaN(), Source: readings.FromAnswer}
	good := workRead(now, 0.2)

	write(t, dir, now, []readings.Reading{bad, good}, []readings.Reading{bad})

	if got := linesWith(log, "readings history can't hold a reading"); len(got) != 1 || !log.Has("level=WARN", "account=work", "window=5h", "component=router") {
		t.Errorf("log reads\n%s\nwant the reading it can't hold warned of once, as the writer's logger's", log)
	}
	if got, want := dayLines(t, dir, "2026-09-27"), strings.Split(strings.TrimSuffix(linesOf(t, good), "\n"), "\n"); !slices.Equal(got, want) {
		t.Errorf("the file holds %q, want %q: the reading it could hold alone", got, want)
	}
}

// linesWith returns the lines of the log that hold text.
func linesWith(log *logstest.Log, text string) []string {
	return slices.DeleteFunc(log.Lines(), func(line string) bool { return !strings.Contains(line, text) })
}
