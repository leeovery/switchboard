package readings_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

// resets is when the session the tests read resets.
var resets = time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC)

// workRead is work's session as an answer read it at at, at u of its use.
func workRead(at time.Time, u float64) readings.Reading {
	return readings.Reading{At: at.UTC(), Account: "work", Key: "5h", Utilization: u, ResetsAt: resets, Status: quota.StatusAllowed, Source: readings.FromAnswer}
}

func TestAReadingIsALineOfJSON(t *testing.T) {
	at := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 3600))
	windows := []quota.Window{
		{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: resets, Status: quota.StatusAllowed},
		{Key: "7d", Label: "Week", Utilization: 0.93},
	}

	var got []string
	for _, r := range readings.Of("work", windows, at.UTC(), readings.FromProbe) {
		line, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, string(line))
	}
	want := []string{
		`{"at":"2026-09-28T13:12:00Z","account":"work","window":"5h","utilization":0.23,"resets_at":"2026-09-28T18:10:00Z","status":"allowed","source":"probe"}`,
		`{"at":"2026-09-28T13:12:00Z","account":"work","window":"7d","utilization":0.93,"source":"probe"}`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("the readings' lines are\n%s\nwant\n%s: the account by its id alone, a reset or status left out where none was read", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestAReadingIsTheWindowItRead(t *testing.T) {
	r := workRead(resets.Add(-time.Hour), 0.4)

	if got, want := r.Window(), (quota.Window{Key: "5h", Utilization: 0.4, ResetsAt: resets, Status: quota.StatusAllowed}); got != want {
		t.Errorf("Window() = %+v, want %+v", got, want)
	}
}

func TestALineReadsAsAReadingWhereItCanBeTakenUp(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{name: "a reading", line: `{"at":"2026-09-28T13:12:00Z","account":"work","window":"5h","utilization":0.23,"source":"answer"}`, want: true},
		{name: "a reading with fields this release doesn't know", line: `{"at":"2026-09-28T13:12:00Z","account":"work","window":"5h","utilization":0,"later":1}`, want: true},
		{name: "cut short", line: `{"at":"2026-09-28T13:12:00Z","account":"work","window":"5h","utili`},
		{name: "not JSON", line: `not a reading`},
		{name: "of no account", line: `{"at":"2026-09-28T13:12:00Z","window":"5h","utilization":0.23}`},
		{name: "of no window", line: `{"at":"2026-09-28T13:12:00Z","account":"work","utilization":0.23}`},
		{name: "of no time", line: `{"account":"work","window":"5h","utilization":0.23}`},
		{name: "of a use that can't be", line: `{"at":"2026-09-28T13:12:00Z","account":"work","window":"5h","utilization":-0.2}`},
		{name: "of a use that isn't a number", line: `{"at":"2026-09-28T13:12:00Z","account":"work","window":"5h","utilization":"high"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := readings.In([]byte(tt.line)); got != tt.want {
				t.Errorf("In(%s) reports %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestTheHistoryIsInItsOwnDirectoryOfTheStateDirectory(t *testing.T) {
	if got, want := readings.Dir("/state"), filepath.Join("/state", "history"); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestBetweenGivesTheReadingsOfATimeWhateverDaysFileTheyreIn(t *testing.T) {
	from := time.Date(2026, 9, 27, 0, 0, 0, 0, time.Local)
	to := from.AddDate(0, 0, 1)
	dir := t.TempDir()
	// The day's readings, and those either side of it, the first of the day
	// filed under the day before, as after a change of time zone, and one
	// compressed with the days before.
	before, first, middle, last, after := workRead(from.Add(-time.Minute), 0.1), workRead(from, 0.2), workRead(from.Add(time.Hour), 0.25),
		workRead(to.Add(-time.Minute), 0.3), workRead(to, 0.4)
	writeDay(t, dir, "readings-2026-09-26.jsonl.gz", gzipOf(t, linesOf(t, before, first)))
	writeDay(t, dir, "readings-2026-09-27.jsonl", []byte(linesOf(t, middle)+"not a reading\n"+linesOf(t, last)))
	writeDay(t, dir, "readings-2026-09-28.jsonl", []byte(linesOf(t, after)))

	got := slices.Collect(readings.Between(readings.Files(dir, logs.For("cli")), from, to))
	if want := []readings.Reading{first, middle, last}; !slices.EqualFunc(got, want, sameReading) {
		t.Errorf("Between() = %+v, want %+v: those of the time asked for, in the order they came, whatever day's file they're in", got, want)
	}
}

func TestBetweenGivesTheReadingsInTheOrderTheyWereReadAcrossTheDaysFiles(t *testing.T) {
	from := time.Date(2026, 9, 27, 0, 0, 0, 0, time.Local)
	to := from.AddDate(0, 0, 1)
	dir := t.TempDir()
	// A change of time zone files readings under the date beside their own:
	// two of the 27th's under the 28th, each day's file in the order written.
	at := func(hour, minute int) time.Time { return time.Date(2026, 9, 27, hour, minute, 0, 0, time.Local) }
	first, second, third, fourth := workRead(at(10, 0), 0.2), workRead(at(12, 0), 0.3), workRead(at(14, 0), 0.4), workRead(at(15, 30), 0.5)
	writeDay(t, dir, "readings-2026-09-27.jsonl", []byte(linesOf(t, first, third)))
	writeDay(t, dir, "readings-2026-09-28.jsonl", []byte(linesOf(t, second, fourth)))

	got := slices.Collect(readings.Between(readings.Files(dir, logs.For("cli")), from, to))
	if want := []readings.Reading{first, second, third, fourth}; !slices.EqualFunc(got, want, sameReading) {
		t.Errorf("Between() = %+v, want %+v: in the order they were read, whichever day's file each is in", got, want)
	}
}

func TestBetweenStopsWhenAskedTo(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 9, 27, 12, 0, 0, 0, time.Local)
	writeDay(t, dir, "readings-2026-09-27.jsonl", []byte(linesOf(t, workRead(at, 0.1), workRead(at.Add(time.Minute), 0.2))))

	var got []readings.Reading
	for r := range readings.Between(readings.Files(dir, logs.For("cli")), at, at.Add(time.Hour)) {
		got = append(got, r)
		break
	}
	if want := []readings.Reading{workRead(at, 0.1)}; !slices.EqualFunc(got, want, sameReading) {
		t.Errorf("Between() gave %+v before it was stopped, want %+v", got, want)
	}
}

func TestTheHistorysFilesPassOverALineTooLongToHold(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	at := time.Date(2026, 9, 27, 12, 0, 0, 0, time.Local)
	long := `{"at":"` + at.UTC().Format(time.RFC3339) + `","account":"work","window":"5h","utilization":0.15,"pad":"` + strings.Repeat("x", 4096) + `"}` + "\n"
	writeDay(t, dir, "readings-2026-09-27.jsonl", []byte(linesOf(t, workRead(at, 0.1))+long+linesOf(t, workRead(at, 0.2))))
	// A compressed file cut short, read up to the damage.
	damaged := append(gzipOf(t, linesOf(t, workRead(at.AddDate(0, 0, -1), 0.05))), gzipOf(t, linesOf(t, workRead(at.AddDate(0, 0, -1), 0.06)))[:5]...)
	writeDay(t, dir, "readings-2026-09-26.jsonl.gz", damaged)

	got := slices.Collect(readings.Between(readings.Files(dir, logs.For("cli")), at.AddDate(0, 0, -1), at.Add(time.Hour)))
	want := []readings.Reading{workRead(at.AddDate(0, 0, -1), 0.05), workRead(at, 0.1), workRead(at, 0.2)}
	if !slices.EqualFunc(got, want, sameReading) {
		t.Errorf("Between() = %+v, want %+v: the lines either side of the one too long to hold, and those before the damage", got, want)
	}
	if !log.Has("level=WARN", `msg="readings history read short"`, "file=readings-2026-09-26.jsonl.gz", "component=cli") {
		t.Errorf("log reads\n%s\nwant the damaged file warned of, as the readings history", log)
	}
}

// sameReading reports whether a and b are the same reading.
func sameReading(a, b readings.Reading) bool {
	return a.At.Equal(b.At) && a.Account == b.Account && a.Window() == b.Window() && a.Source == b.Source
}

// linesOf returns read as the history's lines.
func linesOf(t *testing.T, read ...readings.Reading) string {
	t.Helper()
	var lines strings.Builder
	for _, r := range read {
		line, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		lines.Write(line)
		lines.WriteByte('\n')
	}
	return lines.String()
}

// gzipOf returns text compressed, as a gzip member.
func gzipOf(t *testing.T, text string) []byte {
	t.Helper()
	var data bytes.Buffer
	w := gzip.NewWriter(&data)
	if _, err := w.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

// writeDay writes the file with the given name in dir, holding data.
func writeDay(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
