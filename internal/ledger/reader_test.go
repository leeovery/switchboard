package ledger_test

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

// summaryOfOne is the summary of the day with the given date of one request
// asked makes, and of the windows' highest use, highest, as its JSON gives
// it, where it's given.
func summaryOfOne(date, highest string) string {
	return `{"version":1,"day":"` + date + `","accounts":[{"account":"work","models":[{"model":"claude-opus-5-5","upstream":1,"unsent":0,"checks":0,` +
		`"counts":0,"sessions":1,"usage":{"input_tokens":10,"output_tokens":20}}],"sessions":1,"moved_on":0,"moved_off":0` + highest + `}]}`
}

// summariesJSON returns each of summaries as JSON.
func summariesJSON(t *testing.T, summaries []ledger.Summary) []string {
	t.Helper()
	var all []string
	for _, s := range summaries {
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, string(data))
	}
	return all
}

func TestDaysAreReadFromTheirSummariesOrSummarisedFromTheirLines(t *testing.T) {
	state, dir, history := stateDirs(t)
	// The 3rd's lines are gone, its summary kept for good.
	keptAlone := `{"version":1,"day":"2026-10-03","accounts":[{"account":"side","models":[{"model":"claude-opus-5-5","upstream":7,"unsent":0,` +
		`"checks":0,"counts":0,"sessions":2}],"sessions":2,"moved_on":0,"moved_off":0}]}`
	writeFile(t, dir, "day-2026-10-03.json", []byte(keptAlone+"\n"))
	// The 5th's summary, as written, before a line was filed under the day.
	summarised := summaryOfOne(date, "")
	writeFile(t, dir, "day-2026-10-05.json", []byte(summarised+"\n"))
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 23, 59)))
	// The 6th, not yet summarised, as when the router was stopped as it ended,
	// and today.
	holdLines(t, dir, "2026-10-06", asked("3", on(1, 10, 0)))
	holdLines(t, dir, "2026-10-07", asked("4", on(2, 9, 0)))
	writeFile(t, history, "readings-2026-10-06.jsonl", []byte(readingJSON(t, workRead(on(1, 10, 0), "5h", 0.2, on(1, 14, 0), quota.StatusAllowed))+"\n"))
	writeFile(t, history, "readings-2026-10-07.jsonl", []byte(readingJSON(t, workRead(on(2, 9, 0), "5h", 0.35, on(2, 13, 0), quota.StatusAllowed))+"\n"))

	got := summariesJSON(t, readerAt(state, on(2, 12, 0)).Days(on(-2, 15, 0)))
	want := []string{keptAlone, summarised, summaryOfOne("2026-10-06", `,"highest":{"5h":0.2}`), summaryOfOne("2026-10-07", `,"highest":{"5h":0.35}`)}
	if !slices.Equal(got, want) {
		t.Errorf("Days() =\n%s\nwant\n%s: a day's summary as it's held, never summarised again, the days without one summarised as they're read, "+
			"a day of nothing left out", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, date := range []string{"2026-10-04", "2026-10-06", "2026-10-07"} {
		if held := heldSummary(t, dir, date); held != "" {
			t.Errorf("the ledger holds a summary of %s, as\n%s\nwant none written by reading it", date, held)
		}
	}
}

func TestADayWhoseSummaryCantBeReadIsSummarisedFromItsLines(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, _ := stateDirs(t)
	writeFile(t, dir, "day-2026-10-05.json", []byte(`{"version":1,"day":`))
	holdLines(t, dir, date, asked("1", on(0, 9, 0)))

	got := summariesJSON(t, readerAt(state, on(2, 12, 0)).Days(on(0, 0, 0)))
	if want := []string{summaryOfOne(date, "")}; !slices.Equal(got, want) {
		t.Errorf("Days() = %q, want %q", got, want)
	}
	if !log.Has("level=WARN", `msg="can't read the request ledger's summary of a day; summarising it from its lines"`, "day="+date) {
		t.Errorf("log reads\n%s\nwant the summary that can't be read warned of", log)
	}
}

func TestADayNoneOfWhoseLinesReadIsLeftOut(t *testing.T) {
	state, dir, _ := stateDirs(t)
	writeFile(t, dir, "requests-2026-10-05.jsonl", []byte(`{"at":"2026-10-05T09:00:00Z","requ`))

	if got := readerAt(state, on(1, 12, 0)).Days(on(0, 0, 0)); len(got) > 0 {
		t.Errorf("Days() = %+v, want none: the day's only line is torn", got)
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
