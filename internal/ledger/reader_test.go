package ledger_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

// stateDirs returns a state directory of the test's own, and the ledger's
// and the readings history's directories in it, both made.
func stateDirs(t *testing.T) (state, ledgerDir, historyDir string) {
	t.Helper()
	state = t.TempDir()
	ledgerDir, historyDir = ledger.Dir(state), readings.Dir(state)
	for _, dir := range []string{ledgerDir, historyDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return state, ledgerDir, historyDir
}

// readerAt returns a reader of the ledger in state, by a clock stopped at at.
func readerAt(state string, at time.Time) *ledger.Reader {
	return ledger.NewReader(state, func() time.Time { return at }, logs.For("cli"))
}

// jsonOf returns line as the ledger writes it.
func jsonOf(t *testing.T, line ledger.Line) string {
	t.Helper()
	data, err := json.Marshal(line)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// writeFile writes data to the file with the given name in dir.
func writeFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
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

// heldJSON returns the JSON of each line held, as the reader read it.
func heldJSON(held []ledger.Held) []string {
	var lines []string
	for _, h := range held {
		lines = append(lines, string(h.JSON))
	}
	return lines
}

func TestTheLinesOfARangeOfDaysAreReadBackOldestFirst(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, _ := stateDirs(t)
	before, first, second := jsonOf(t, asked("1", on(-1, 23, 0))), jsonOf(t, asked("2", on(0, 10, 0))), jsonOf(t, asked("3", on(0, 23, 59)))
	// Yesterday's lines as their requests ended: the fourth arrived first, and
	// ended after the fifth. The fifth holds a field a later release added.
	fourth, fifth := jsonOf(t, asked("4", on(1, 14, 0))), strings.TrimSuffix(jsonOf(t, asked("5", on(1, 14, 1))), "}")+`,"later":true}`
	sixth, future := jsonOf(t, asked("6", on(2, 9, 0))), jsonOf(t, asked("7", on(2, 13, 0)))
	writeFile(t, dir, "requests-2026-10-04.jsonl.gz", gzipped(t, before+"\n"))
	// The day the range starts on, compressed, its first line before it.
	writeFile(t, dir, "requests-2026-10-05.jsonl.gz", gzipped(t, jsonOf(t, asked("0", on(0, 9, 0)))+"\n"+first+"\n", second+"\n"))
	writeFile(t, dir, "requests-2026-10-06.jsonl", []byte(fifth+"\nnot a line\n"+fourth+"\n"))
	// Today's, the last line torn as a crash leaves it, and one of a time to
	// come, as after the clock was set back.
	writeFile(t, dir, "requests-2026-10-07.jsonl", []byte(sixth+"\n"+future+"\n"+`{"at":"2026-10-07T11:00:00Z","requ`))

	got := slices.Collect(readerAt(state, on(2, 12, 0)).Lines(on(0, 9, 30)))
	if want := []string{first, second, fourth, fifth, sixth}; !slices.Equal(heldJSON(got), want) {
		t.Errorf("Lines() =\n%s\nwant\n%s: those from 09:30 on the 5th until now, oldest first, as the ledger holds them", strings.Join(heldJSON(got), "\n"), strings.Join(want, "\n"))
	}
	if len(got) == 5 && (got[2].Request != "4" || !got[2].At.Equal(on(1, 14, 0)) || got[2].Model != opus) {
		t.Errorf("the third line read as %+v, want request 4's", got[2].Line)
	}
	if !log.Has("level=WARN", `msg="request ledger lines unread"`, "lines=2", "component=cli") {
		t.Errorf("log reads\n%s\nwant the two lines that don't read noted", log)
	}
}

func TestADamagedCompressedFileIsReadUpToTheDamage(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, _ := stateDirs(t)
	first, second := jsonOf(t, asked("1", on(0, 10, 0))), jsonOf(t, asked("2", on(0, 11, 0)))
	writeFile(t, dir, "requests-2026-10-05.jsonl.gz", append(gzipped(t, first+"\n"), gzipped(t, second+"\n")[:5]...))
	writeFile(t, dir, "requests-2026-10-06.jsonl", []byte(jsonOf(t, asked("3", on(1, 9, 0)))+"\n"))

	got := heldJSON(slices.Collect(readerAt(state, on(1, 12, 0)).Lines(on(0, 0, 0))))
	if want := []string{first, jsonOf(t, asked("3", on(1, 9, 0)))}; !slices.Equal(got, want) {
		t.Errorf("Lines() = %q, want %q: the lines before the damage, and the days after it", got, want)
	}
	if !log.Has("level=WARN", `msg="request ledger read short"`, "file=requests-2026-10-05.jsonl.gz") {
		t.Errorf("log reads\n%s\nwant the damaged file warned of", log)
	}
}

func TestReadingLinesStopsWhenAsked(t *testing.T) {
	state, dir, _ := stateDirs(t)
	writeFile(t, dir, "requests-2026-10-05.jsonl", []byte(jsonOf(t, asked("1", on(0, 10, 0)))+"\n"+jsonOf(t, asked("2", on(0, 11, 0)))+"\n"))

	var got []string
	for h := range readerAt(state, on(0, 12, 0)).Lines(on(0, 0, 0)) {
		got = append(got, h.Request)
		break
	}
	if !slices.Equal(got, []string{"1"}) {
		t.Errorf("read %q before stopping, want the first line alone", got)
	}
}

func TestLinesAreReadBackOldestFirstWhicheverDaysFileTheyreIn(t *testing.T) {
	state, dir, _ := stateDirs(t)
	// After a move east, a line of the 5th's evening is filed under the 6th,
	// and after one back west, one of the 6th's first hour under the 5th.
	first, second, third, fourth := asked("1", on(0, 22, 0)), asked("2", on(0, 23, 0)), asked("3", on(1, 0, 30)), asked("4", on(1, 9, 0))
	holdLines(t, dir, date, first, third)
	holdLines(t, dir, "2026-10-06", second, fourth)

	got := heldJSON(slices.Collect(readerAt(state, on(1, 12, 0)).Lines(on(0, 0, 0))))
	if want := []string{jsonOf(t, first), jsonOf(t, second), jsonOf(t, third), jsonOf(t, fourth)}; !slices.Equal(got, want) {
		t.Errorf("Lines() =\n%s\nwant\n%s: oldest first, whichever day's file each is in", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// noRequests is the summary of the day with the given date of no requests,
// made from as many lines as lines says.
func noRequests(date string, lines int) string {
	return fmt.Sprintf(`{"version":1,"day":"%s","lines":%d}`, date, lines)
}

// summariesJSON returns each of summaries as JSON, but for the sizes of its
// day's files a summary is marked with.
func summariesJSON(t *testing.T, summaries []ledger.Summary) []string {
	t.Helper()
	var all []string
	for _, s := range summaries {
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, unmarked(string(data)))
	}
	return all
}

func TestDaysAreEveryDayOfTheRangeTodaysLast(t *testing.T) {
	state, dir, _ := stateDirs(t)
	holdLines(t, dir, "2026-10-04", asked("1", on(-1, 9, 0)))
	// A day whose only line was torn as it was written.
	writeFile(t, dir, "requests-2026-10-06.jsonl", []byte(`{"at":"2026-10-06T09:00:00Z","requ`))

	got := summariesJSON(t, readerAt(state, on(2, 12, 0)).Days(on(-1, 15, 0)))
	want := []string{summaryOf("2026-10-04", 1, 1, ""), noRequests(date, 0), noRequests("2026-10-06", 1), noRequests("2026-10-07", 0)}
	if !slices.Equal(got, want) {
		t.Errorf("Days() =\n%s\nwant\n%s: every day from the 4th's to today's, a day of no requests a summary of no accounts", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestDaysStartAtTheFirstTheLedgerHolds(t *testing.T) {
	today := on(2, 12, 0)
	tests := []struct {
		name string
		// lay lays the ledger out in dir.
		lay  func(t *testing.T, dir string)
		from time.Time
		// want are the dates of the days given.
		want []string
	}{
		{
			name: "its first file of lines's, from a day before",
			lay:  func(t *testing.T, dir string) { holdLines(t, dir, "2026-10-06", asked("1", on(1, 9, 0))) },
			from: on(-30, 0, 0),
			want: []string{"2026-10-06", "2026-10-07"},
		},
		{
			name: "its first summary's, the lines of its day pruned",
			lay: func(t *testing.T, dir string) {
				writeFile(t, dir, "day-2026-10-04.json", []byte(summaryOf("2026-10-04", 1, 1, "")+"\n"))
				holdLines(t, dir, "2026-10-06", asked("1", on(1, 9, 0)))
			},
			from: on(-30, 0, 0),
			want: []string{"2026-10-04", date, "2026-10-06", "2026-10-07"},
		},
		{
			name: "the day asked for, after the ledger's first",
			lay:  func(t *testing.T, dir string) { holdLines(t, dir, "2026-10-04", asked("1", on(-1, 9, 0))) },
			from: on(1, 15, 0),
			want: []string{"2026-10-06", "2026-10-07"},
		},
		{
			name: "today alone, the ledger holding no day",
			lay:  func(t *testing.T, dir string) { writeFile(t, dir, "notes.txt", nil) },
			from: on(-30, 0, 0),
			want: []string{"2026-10-07"},
		},
		{
			name: "today alone, the ledger holding none before the day after tomorrow, as a clock once set ahead names one",
			lay:  func(t *testing.T, dir string) { holdLines(t, dir, "2026-10-09", asked("1", on(4, 9, 0))) },
			from: on(-30, 0, 0),
			want: []string{"2026-10-07"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, dir, _ := stateDirs(t)
			tt.lay(t, dir)

			var got []string
			for _, s := range readerAt(state, today).Days(tt.from) {
				got = append(got, s.Day)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Days() are of %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDaysAreReadFromTheirSummariesWhileTheyStand(t *testing.T) {
	state, dir, history := stateDirs(t)
	// The 3rd's lines are gone, its summary kept for good.
	keptAlone := `{"version":1,"day":"2026-10-03","lines":7,"accounts":[{"account":"side","models":[{"model":"claude-opus-5-5","upstream":7,"no_usage":0,` +
		`"unsent":0,"checks":0,"counts":0,"sessions":2}],"sessions":2,"moved_on":0,"moved_off":0}]}`
	writeFile(t, dir, "day-2026-10-03.json", []byte(keptAlone+"\n"))
	// The 4th's summary, made from the line it holds, and one more since lost
	// to damage, with the highest use the readings history gave as it was
	// summarised, since pruned.
	standing := summaryOf("2026-10-04", 2, 2, `,"highest":{"5h":0.9}`)
	writeFile(t, dir, "day-2026-10-04.json", []byte(standing+"\n"))
	holdLines(t, dir, "2026-10-04", asked("1", on(-1, 9, 0)))
	// The 5th's summary, written before a line came to be filed under the day.
	written := summaryOf(date, 1, 1, "") + "\n"
	writeFile(t, dir, "day-2026-10-05.json", []byte(written))
	holdLines(t, dir, date, asked("2", on(0, 9, 0)), asked("3", on(0, 23, 59)))
	// The 6th, not yet summarised, as when the router was stopped as it ended,
	// and today.
	holdLines(t, dir, "2026-10-06", asked("4", on(1, 10, 0)))
	holdLines(t, dir, "2026-10-07", asked("5", on(2, 9, 0)))
	writeFile(t, history, "readings-2026-10-06.jsonl", []byte(readingJSON(t, workRead(on(1, 10, 0), "5h", 0.2, on(1, 14, 0), quota.StatusAllowed))+"\n"))
	writeFile(t, history, "readings-2026-10-07.jsonl", []byte(readingJSON(t, workRead(on(2, 9, 0), "5h", 0.35, on(2, 13, 0), quota.StatusAllowed))+"\n"))

	got := summariesJSON(t, readerAt(state, on(2, 12, 0)).Days(on(-2, 15, 0)))
	want := []string{keptAlone, standing, summaryOf(date, 2, 2, ""), summaryOf("2026-10-06", 1, 1, `,"highest":{"5h":0.2}`),
		summaryOf("2026-10-07", 1, 1, `,"highest":{"5h":0.35}`)}
	if !slices.Equal(got, want) {
		t.Errorf("Days() =\n%s\nwant\n%s: a day's summary as it's held while its lines are no more than it was made from, or pruned, and the others "+
			"summarised from their lines as they're read", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if held := heldSummary(t, dir, date); held != written {
		t.Errorf("the ledger holds the 5th's summary as\n%s\nwant it as it was\n%s", held, written)
	}
	for _, date := range []string{"2026-10-06", "2026-10-07"} {
		if held := heldSummary(t, dir, date); held != "" {
			t.Errorf("the ledger holds a summary of %s, as\n%s\nwant none written by reading it", date, held)
		}
	}
}

func TestALedgerThatCantBeListedIsReadFromTheStartAskedFor(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, _ := stateDirs(t)
	holdLines(t, dir, "2026-10-06", asked("1", on(1, 9, 0)))
	// Its files can be opened by name, but not listed.
	if err := os.Chmod(dir, 0o300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	var got []string
	for _, s := range readerAt(state, on(2, 12, 0)).Days(on(0, 0, 0)) {
		got = append(got, s.Day)
	}
	if want := []string{date, "2026-10-06", "2026-10-07"}; !slices.Equal(got, want) {
		t.Errorf("Days() are of %q, want %q: every day asked for, as the first the ledger holds can't be told", got, want)
	}
	if !log.Has("level=WARN", `msg="can't read the request ledger"`, "dir="+dir) {
		t.Errorf("log reads\n%s\nwant the ledger that can't be listed warned of", log)
	}
}

func TestDaysOpenNoFileOfADayWhoseLinesHaventChangedSinceItsSummary(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, _ := stateDirs(t)
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
	writeAt(t, dir, on(1, 10, 0), noReadings)
	unopenable(t, dir, "requests-*")

	got := summariesJSON(t, readerAt(state, on(1, 12, 0)).Days(on(0, 0, 0)))
	if want := []string{summaryOf(date, 2, 2, ""), noRequests("2026-10-06", 0)}; !slices.Equal(got, want) || opened(log) != 0 {
		t.Errorf("log reads\n%s\nDays() =\n%s\nwant\n%s: the 5th's summary as it's held, its file unopened, as its lines haven't changed since",
			log, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestADayWhoseSummaryCantBeReadAsItsOwnIsSummarisedFromItsLines(t *testing.T) {
	tests := []struct {
		name string
		held string
	}{
		{name: "cut short", held: `{"version":1,"day":`},
		{name: "of another day", held: summaryOf("2026-10-04", 1, 1, "")},
		{name: "of no day", held: `{"version":1,"lines":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			state, dir, _ := stateDirs(t)
			writeFile(t, dir, "day-2026-10-05.json", []byte(tt.held))
			holdLines(t, dir, date, asked("1", on(0, 9, 0)))

			got := summariesJSON(t, readerAt(state, on(0, 12, 0)).Days(on(0, 0, 0)))
			if want := []string{summaryOf(date, 1, 1, "")}; !slices.Equal(got, want) {
				t.Errorf("Days() = %q, want %q", got, want)
			}
			if !log.Has("level=WARN", `msg="can't read the request ledger's summary of a day; summarising it from its lines"`, "day="+date) {
				t.Errorf("log reads\n%s\nwant the summary that can't be read as the day's warned of", log)
			}
		})
	}
}

func TestADayIsSummarisedWithItsReadingsInTheOrderTheyWereRead(t *testing.T) {
	state, dir, history := stateDirs(t)
	holdLines(t, dir, date, asked("1", on(0, 9, 0)))
	// A change of time zone files a reading under the date beside its own:
	// the 3rd's file holds the last of work's session before the day, and the
	// 6th's the first of the two its week was read rejected at that day.
	resets, weekResets := on(0, 3, 0), on(2, 10, 0)
	for name, read := range map[string]readings.Reading{
		"readings-2026-10-03.jsonl": workRead(on(-1, 23, 0), "5h", 0.8, resets, quota.StatusAllowed),
		"readings-2026-10-04.jsonl": workRead(on(-1, 22, 0), "5h", 0.3, resets, quota.StatusAllowed),
		"readings-2026-10-05.jsonl": workRead(on(0, 15, 0), "7d", 1, weekResets, quota.StatusRejected),
		"readings-2026-10-06.jsonl": workRead(on(0, 14, 0), "7d", 1, weekResets, quota.StatusRejected),
	} {
		writeFile(t, history, name, []byte(readingJSON(t, read)+"\n"))
	}

	got := summariesJSON(t, readerAt(state, on(0, 18, 0)).Days(on(0, 0, 0)))
	limit := `{"window":"7d","at":"` + on(0, 14, 0).UTC().Format(time.RFC3339) + `","resets_at":"` + weekResets.UTC().Format(time.RFC3339) + `"}`
	if want := []string{summaryOf(date, 1, 1, `,"highest":{"5h":0.8,"7d":1},"limits":[`+limit+`]`)}; !slices.Equal(got, want) {
		t.Errorf("Days() =\n%s\nwant\n%s: the use work's session began the day at, as its last reading before it read, and its week's limit as first read",
			strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// readingJSON returns r as the readings history writes it.
func readingJSON(t *testing.T, r readings.Reading) string {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
