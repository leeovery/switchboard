package ledger_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// tick is when the summaries' day's compressed file was last modified, to
// the second, as a file system that keeps whole seconds keeps it, as HFS+
// does.
var tick = on(1, 0, 30)

// compressedLines lays the summaries' day's two requests out in dir, in its
// compressed file, last modified at tick.
func compressedLines(t *testing.T, dir string) {
	t.Helper()
	compressed := filepath.Join(dir, "requests-"+date+".jsonl.gz")
	writeFile(t, dir, filepath.Base(compressed), gzipped(t, jsonOf(t, asked("1", on(0, 9, 0)))+"\n"+jsonOf(t, asked("2", on(0, 10, 0)))+"\n"))
	if err := os.Chtimes(compressed, time.Time{}, tick); err != nil {
		t.Fatal(err)
	}
}

// plainLines lays the summaries' day's two requests out in dir, in its plain
// file.
func plainLines(t *testing.T, dir string) {
	t.Helper()
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
}

// heldVersionOne is the summary a release of version 1 made of the
// summaries' day, of its two requests, and its session's highest use.
var heldVersionOne = versionOne(date, 2, 2, `,"highest":{"5h":0.9}`)

// markVersionOne writes heldVersionOne as the ledger in dir holds it, marked
// with the day's files as they stand, and stamped, where the day has a
// compressed file, as that release stamped it: when the file was last
// modified.
func markVersionOne(t *testing.T, dir string) {
	t.Helper()
	sizes := dayBytes(t, dir, date)
	marked := strings.Replace(heldVersionOne, `"lines":2,`, fmt.Sprintf(`"lines":2,"bytes":{"plain":%d,"compressed":%d},`, sizes.Plain, sizes.Compressed), 1)
	writeFile(t, dir, "day-"+date+".json", []byte(marked+"\n"))
	if info, err := os.Stat(filepath.Join(dir, "requests-"+date+".jsonl.gz")); err == nil {
		if err := os.Chtimes(summaryFile(dir, date), time.Time{}, info.ModTime()); err != nil {
			t.Fatal(err)
		}
	}
}

// wholeSeconds has the file at path last modified at the second it was, as
// a file system that keeps whole seconds keeps it.
func wholeSeconds(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Time{}, info.ModTime().Truncate(time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestADaySummarisedUnderVersionOneIsSummarisedAgainOnceWhileItsLinesAreKept(t *testing.T) {
	tests := []struct {
		name string
		// lay lays the day's lines out in dir.
		lay func(t *testing.T, dir string)
		// unread are the patterns of the names of the files a round that
		// finds the day's summary stands reads none of.
		unread []string
	}{
		{name: "its lines plain", lay: plainLines, unread: []string{"requests-*"}},
		{name: "its lines compressed, on a file system that keeps whole seconds", lay: compressedLines, unread: []string{"requests-*", "day-*"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			dir := t.TempDir()
			tt.lay(t, dir)
			markVersionOne(t, dir)
			l := ledger.Open(dir, 400*24*time.Hour, func() time.Time { return on(3, 10, 0) }, noReadings, caps, logs.For("router"))

			// The readings history holds none of the day's readings any longer,
			// pruned by a keep shorter than the ledger's.
			l.SummariseEnded(on(3, 10, 0))
			if got, want := heldSummary(t, dir, date), summaryOfTwo(windowsRead{highest: `{"5h":0.9}`}); got != want {
				t.Fatalf("the day's summary is\n%s\nwant it summarised again from its lines, as version 2, knowing what the one before did, "+
					"its windows' rises, resets and minutes never read, rather than none\n%s", got, want)
			}
			wholeSeconds(t, summaryFile(dir, date))
			for _, pattern := range tt.unread {
				unopenable(t, dir, pattern)
			}
			l.SummariseEnded(on(3, 11, 0))
			if n, summarised := opened(log), strings.Count(log.String(), `msg="summarised a day of the request ledger"`); n != 0 || summarised != 1 {
				t.Errorf("log reads\n%s\nthe round after opened %d of the day's files, and the day was summarised %d times; want none opened, "+
					"and the day summarised once: its summary is of version 2 now", log, n, summarised)
			}
		})
	}
}

func TestAReaderReadsASummaryOfVersionOneAsItIsOnceItsLinesAreGone(t *testing.T) {
	tests := []struct {
		name string
		// lay lays the day's lines out in dir, where they're kept.
		lay  func(t *testing.T, dir string)
		want string
	}{
		{name: "its lines kept, summarised from them", lay: compressedLines, want: summaryOf(date, 2, 2, windowsRead{highest: `{"5h":0.9}`})},
		{name: "its lines gone, as it is", lay: func(*testing.T, string) {}, want: heldVersionOne},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, dir, _ := stateDirs(t)
			tt.lay(t, dir)
			markVersionOne(t, dir)
			held := heldSummary(t, dir, date)

			days := readerAt(state, on(3, 10, 0)).Days(on(0, 0, 0))
			if got := summariesJSON(t, days[:1]); got[0] != tt.want {
				t.Errorf("the day is\n%s\nwant\n%s", got[0], tt.want)
			}
			if got := heldSummary(t, dir, date); got != held {
				t.Errorf("the ledger holds the day's summary as\n%s\nwant it as it was\n%s", got, held)
			}
		})
	}
}

func TestASummaryOfVersionOneWhoseLinesAreGoneHoldsNoneOfWhatItNeverRead(t *testing.T) {
	state, dir, _ := stateDirs(t)
	markVersionOne(t, dir)

	got := readerAt(state, on(3, 10, 0)).Days(on(0, 0, 0))[0]
	if len(got.Accounts) != 1 {
		t.Fatalf("the day is %+v, want work's day in it", got)
	}
	if a := got.Accounts[0]; a.SessionIDs != nil || a.Rise != nil || a.Resets != nil || a.MinutesAtCap != nil || a.MinutesAtLimit != nil {
		t.Errorf("work's day is %+v, want its session ids, rises, resets and minutes never read, not none", a)
	}
	if got.Version != 1 {
		t.Errorf("the day's summary is of version %d, want 1, as it was written", got.Version)
	}
}
