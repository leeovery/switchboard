package events_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// now is the time by the clock in tests.
var now = time.Date(2026, 10, 1, 13, 12, 0, 0, time.Local)

// keep is how long the tests' files are kept from the end of their day,
// where a test doesn't say: as long as [ledger] keep keeps them unless it's
// set.
const keep = 400 * 24 * time.Hour

// runs are the times two routers started, as their events' runs.
var (
	firstRun  = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	secondRun = time.Date(2026, 9, 11, 7, 30, 0, 0, time.UTC)
)

// september returns the time on the given day of September 2026, at
// hour:minute local time.
func september(day, hour, minute int) time.Time {
	return time.Date(2026, 9, day, hour, minute, 0, 0, time.Local)
}

// dateOf is the date of the local day t falls on, as the files' names give
// it.
func dateOf(t time.Time) string {
	return t.Local().Format(time.DateOnly)
}

// limit returns a version of the limit event of run with the given id, that
// happened at at, having moved count sessions.
func limit(run time.Time, id int, at time.Time, count int) events.Line {
	return events.Line{ID: id, At: at, Kind: "limit", Account: "work", Windows: []string{"five_hour"}, Until: at.Add(3 * time.Hour), Count: count, Limit: 1, Run: run}
}

// jsonOf returns line as it's filed.
func jsonOf(t *testing.T, line events.Line) string {
	t.Helper()
	data, err := json.Marshal(line)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// jsonsOf returns lines as they're filed.
func jsonsOf(t *testing.T, lines []events.Line) []string {
	t.Helper()
	jsons := make([]string, len(lines))
	for i, line := range lines {
		jsons[i] = jsonOf(t, line)
	}
	return jsons
}

// linesOf returns texts as lines, a line each.
func linesOf(texts ...string) string {
	var lines strings.Builder
	for _, text := range texts {
		lines.WriteString(text + "\n")
	}
	return lines.String()
}

// gzipped returns texts compressed, a gzip member each, one after another.
func gzipped(t *testing.T, texts ...string) []byte {
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

// writeFile writes data to the file with the given name in dir, making dir.
func writeFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// write has a writer of the events' files in dir, kept for keep, by a clock
// stopped at at, note lines, and returns once it has written every one it
// can, as a router started at at and stopped at once has it.
func write(t *testing.T, dir string, at time.Time, keep time.Duration, lines ...events.Line) {
	t.Helper()
	w := events.Open(dir, keep, func() time.Time { return at }, logs.For("router"))
	for _, line := range lines {
		w.Note(line)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	w.Run(ctx)
}

// fileLines returns the lines of the file with the given name in dir.
func fileLines(t *testing.T, dir, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func TestALineIsAnEventAsTheStatusDocumentGivesItWithItsRun(t *testing.T) {
	line := limit(firstRun, 7, september(10, 9, 0), 2)
	got := jsonOf(t, line)
	event, err := json.Marshal(line.Event)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.TrimSuffix(string(event), "}") + `,"run":"2026-09-01T08:00:00Z"}`; got != want {
		t.Errorf("line is\n%s\nwant\n%s", got, want)
	}

	read, ok := events.In([]byte(got))
	if !ok || jsonOf(t, read) != got {
		t.Errorf("In(%s) = %s, %v; want the line back", got, jsonOf(t, read), ok)
	}
}

func TestALineIsFiledAsItWasReadALaterReleasesFieldsIncluded(t *testing.T) {
	made := limit(firstRun, 7, september(10, 9, 0), 2)
	if got, err := made.Filed(); err != nil || string(got) != jsonOf(t, made) {
		t.Errorf("a line made, not read, is filed as\n%s (%v)\nwant\n%s", got, err, jsonOf(t, made))
	}

	filed := `{"a_later_field":{"kept":true},` + strings.TrimPrefix(jsonOf(t, made), "{")
	read, ok := events.In([]byte(filed))
	if got, err := read.Filed(); !ok || err != nil || string(got) != filed {
		t.Errorf("In(%s) is filed as\n%s (%v, %v)\nwant it as it was read", filed, got, ok, err)
	}
}

func TestALineThatDoesntReadAsAnEventIsPassedOver(t *testing.T) {
	whole := jsonOf(t, limit(firstRun, 7, september(10, 9, 0), 2))
	without := func(field string) string {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(whole), &fields); err != nil {
			t.Fatal(err)
		}
		delete(fields, field)
		data, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	tests := []struct {
		name string
		line string
	}{
		{name: "not JSON", line: "a line that isn't JSON"},
		{name: "cut short", line: whole[:len(whole)/2]},
		{name: "no run", line: without("run")},
		{name: "no id", line: without("id")},
		{name: "no at", line: without("at")},
		{name: "no kind", line: without("kind")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := events.In([]byte(tt.line)); ok {
				t.Errorf("In(%s) read it as an event", tt.line)
			}
		})
	}
}

func TestALineWithFieldsItDoesntKnowReads(t *testing.T) {
	line := limit(firstRun, 7, september(10, 9, 0), 2)
	later := `{"a_later_field":{"of":"a later release"},` + strings.TrimPrefix(jsonOf(t, line), "{")

	read, ok := events.In([]byte(later))
	if !ok || jsonOf(t, read) != jsonOf(t, line) {
		t.Errorf("In(%s) = %s, %v; want the line, the field it doesn't know passed over", later, jsonOf(t, read), ok)
	}
}

func TestALineIsFiledInTheLedgersDirectoryUnderTheDayItsWrittenOn(t *testing.T) {
	dir := ledger.Dir(t.TempDir())
	// A version of an event of three days before.
	line := limit(firstRun, 7, now.AddDate(0, 0, -3), 4)

	write(t, dir, now, keep, line)

	name := "events-" + dateOf(now) + ".jsonl"
	if got := fileLines(t, dir, name); !slices.Equal(got, []string{jsonOf(t, line)}) {
		t.Errorf("%s holds %q, want the line", name, got)
	}
	info, err := os.Stat(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("%s is %v, want 0600", name, info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(dir, "events-"+dateOf(line.At)+".jsonl")); err == nil {
		t.Errorf("a file of its event's day was written, want the line under the day it's written on alone")
	}
}

func TestALineShapedLikeATokenIsHidden(t *testing.T) {
	dir := ledger.Dir(t.TempDir())
	line := limit(firstRun, 7, now, 1)
	line.Reason = "upstream said sk-ant-oat01-" + strings.Repeat("x", 40)

	write(t, dir, now, keep, line)

	got := strings.Join(fileLines(t, dir, "events-"+dateOf(now)+".jsonl"), "\n")
	if strings.Contains(got, "sk-ant-") || !strings.Contains(got, "upstream said [redacted]") {
		t.Error("the file holds the token, want it hidden")
	}
}

func TestTheWritersRoundsKeepTheEventsFilesAndLeaveEverythingElseAlone(t *testing.T) {
	dir := ledger.Dir(t.TempDir())
	old, done := dateOf(now.AddDate(0, 0, -30)), dateOf(now.AddDate(0, 0, -3))
	others := map[string]string{
		"requests-" + old + ".jsonl":  linesOf(`{"request":"an old one"}`),
		"requests-" + done + ".jsonl": linesOf(`{"request":"one of three days ago"}`),
		"day-" + old + ".json":        `{"day":"` + old + `"}` + "\n",
		"notes.txt":                   "the user's own\n",
	}
	for name, data := range others {
		writeFile(t, dir, name, []byte(data))
	}
	writeFile(t, dir, "events-"+old+".jsonl", []byte(linesOf(jsonOf(t, limit(firstRun, 1, now.AddDate(0, 0, -30), 1)))))
	writeFile(t, dir, "events-"+done+".jsonl", []byte(linesOf(jsonOf(t, limit(firstRun, 2, now.AddDate(0, 0, -3), 1)))))

	write(t, dir, now, 8*24*time.Hour)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{"day-" + old + ".json", "events-" + done + ".jsonl.gz", "notes.txt", "requests-" + old + ".jsonl", "requests-" + done + ".jsonl"}
	if slices.Sort(want); !slices.Equal(names, want) {
		t.Errorf("the ledger's directory holds %q, want %q: the old day's events removed, the day done with compressed, everything else as it was", names, want)
	}
	for name, data := range others {
		if got := strings.Join(fileLines(t, dir, name), "\n") + "\n"; got != data {
			t.Errorf("%s holds %q, want %q, as it was", name, got, data)
		}
	}
}

func TestTheWriterQueues1024LinesAndDropsThoseBeyond(t *testing.T) {
	log := logstest.Capture(t)
	dir := ledger.Dir(t.TempDir())
	lines := make([]events.Line, 1030)
	for i := range lines {
		lines[i] = limit(firstRun, i+1, now, 1)
	}

	write(t, dir, now, keep, lines...)

	if got := fileLines(t, dir, "events-"+dateOf(now)+".jsonl"); len(got) != 1024 {
		t.Errorf("the file holds %d lines, want the 1,024 queued", len(got))
	}
	if dropped := slices.DeleteFunc(log.Lines(), func(line string) bool { return !strings.Contains(line, "fell behind") }); len(dropped) != 1 ||
		!strings.Contains(dropped[0], `msg="router's events fell behind; events dropped from it"`) {
		t.Errorf("log reads\n%s\nwant the lines dropped warned of once", log)
	}
}

func TestALineThatCantBeWrittenIsLoggedOnce(t *testing.T) {
	log := logstest.Capture(t)
	dir := ledger.Dir(t.TempDir())
	// A time past the year 9999 can't be put as JSON.
	unwritable := now.AddDate(8000, 0, 0)

	write(t, dir, now, keep, limit(firstRun, 1, unwritable, 1), limit(firstRun, 2, unwritable, 1), limit(firstRun, 3, now, 1))

	if got := fileLines(t, dir, "events-"+dateOf(now)+".jsonl"); !slices.Equal(got, []string{jsonOf(t, limit(firstRun, 3, now, 1))}) {
		t.Errorf("the file holds %q, want the line that could be written, alone", got)
	}
	if n := strings.Count(log.String(), `msg="router's events can't hold a line; it goes unwritten"`); n != 1 || !log.Has("level=WARN", "event=1") {
		t.Errorf("log reads\n%s\nwant the first line that couldn't be written warned of, once", log)
	}
}

func TestAWriteThatFailsIsLoggedOnce(t *testing.T) {
	log := logstest.Capture(t)
	// A file where the directory goes, so nothing can be written in it.
	dir := filepath.Join(t.TempDir(), "ledger")
	if err := os.WriteFile(dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	write(t, dir, now, keep, limit(firstRun, 1, now, 1), limit(firstRun, 2, now, 1), limit(firstRun, 3, now, 1))

	if failed := slices.DeleteFunc(log.Lines(), func(line string) bool { return !strings.Contains(line, "can't write the router's events") }); len(failed) != 1 {
		t.Errorf("log reads\n%s\nwant the failing write warned of once", log)
	}
}
