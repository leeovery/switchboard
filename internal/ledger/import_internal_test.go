package ledger

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// importedOn is the clock of the tests of taking a staged day in: a Monday,
// 13:12 UTC.
var importedOn = time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)

// runningLedger opens a ledger in a directory of the test's own, keeping a
// day's lines a year, by a clock stopped at importedOn, its days summarised
// with no readings, and runs it until the test ends, returning once its
// first round is done.
func runningLedger(t *testing.T) *Ledger {
	t.Helper()
	l := Open(t.TempDir(), 365*24*time.Hour, func() time.Time { return importedOn }, &asking{}, Caps{}, logs.For("router"))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		l.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	settled(t, l)
	return l
}

// settled returns once the ledger's writer has done all handed to it before.
func settled(t *testing.T, l *Ledger) {
	t.Helper()
	if err := l.writer.Do(t.Context(), func() {}); err != nil {
		t.Fatal(err)
	}
}

// staged stages a day of a line, done with, ten days before importedOn, and
// its summary, in a directory of the test's own, and returns it.
func staged(t *testing.T) StagedDay {
	t.Helper()
	dir := t.TempDir()
	date := dayfile.DateOf(importedOn.AddDate(0, 0, -10))
	var lines bytes.Buffer
	zw := gzip.NewWriter(&lines)
	if _, err := zw.Write([]byte(`{"request":"staged","kind":"message","account":"work"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	day := StagedDay{Date: date, Lines: filepath.Join(dir, "requests-"+date+".jsonl.gz"), Summary: SummaryFile(dir, date)}
	for path, data := range map[string][]byte{day.Lines: lines.Bytes(), day.Summary: []byte(`{"version":6,"day":"` + date + `","lines":1}` + "\n")} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return day
}

// holding reports whether l holds the day's lines, and its summary.
func holding(l *Ledger, day StagedDay) (lines, summary bool) {
	_, err := l.days.files.Stat(day.Date)
	_, summaryErr := os.Stat(SummaryFile(l.days.files.Dir, day.Date))
	return err == nil, summaryErr == nil
}

// inRound starts taking day in as a round holds l's summaries, as the
// readings history's summarising of the days before its prune does, and
// returns once the day's lines are in, with what Import fails with once it
// returns, what cuts it short, and what ends the round, as the test's end
// does too.
func inRound(t *testing.T, l *Ledger, day StagedDay) (imported <-chan error, cancel, endRound func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	l.summarising.Lock()
	endRound = sync.OnceFunc(l.summarising.Unlock)
	t.Cleanup(endRound)
	answered := make(chan error, 1)
	go func() {
		_, err := l.Import(ctx, day)
		answered <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for lines, _ := holding(l, day); !lines; lines, _ = holding(l, day) {
		if time.Now().After(deadline) {
			t.Fatal("gave up waiting for the day's lines to be taken in")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return answered, cancel, endRound
}

func TestADaysSummaryIsCopiedInOnlyOnceARoundUnderWayIsDone(t *testing.T) {
	logstest.Capture(t)
	l := runningLedger(t)
	day := staged(t)
	imported, _, endRound := inRound(t, l, day)

	// A copy that didn't wait on the round would be done well within this.
	time.Sleep(50 * time.Millisecond)
	if _, summary := holding(l, day); summary {
		t.Error("the day's summary was copied in as a round held the summaries, want it copied once the round is done")
	}
	endRound()
	if err := <-imported; err != nil {
		t.Fatalf("Import() = %v", err)
	}
	if _, summary := holding(l, day); !summary {
		t.Error("once the round was done and the day taken in, the ledger holds no summary of it, want the one staged")
	}
}

func TestADayCutShortAsItsTakenInGetsBothItsLinesAndItsSummary(t *testing.T) {
	logstest.Capture(t)
	l := runningLedger(t)
	day := staged(t)
	imported, cancel, endRound := inRound(t, l, day)

	cancel()
	select {
	case err := <-imported:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Import() as its context ended = %v, want the context's end", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Import() didn't return as its context ended, the summaries held")
	}
	endRound()
	settled(t, l)
	if lines, summary := holding(l, day); !lines || !summary {
		t.Errorf("once the import cut short had run its course, the ledger holds the day's lines: %v, its summary: %v; want both", lines, summary)
	}
}

func TestADayCutShortBeforeItsTakenInGetsNeitherItsLinesNorItsSummary(t *testing.T) {
	logstest.Capture(t)
	l := runningLedger(t)
	day := staged(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := l.Import(ctx, day); !errors.Is(err, context.Canceled) {
		t.Errorf("Import() with its context ended = %v, want the context's end", err)
	}
	settled(t, l)
	if lines, summary := holding(l, day); lines || summary {
		t.Errorf("once the import cut short, the ledger holds the day's lines: %v, its summary: %v; want neither", lines, summary)
	}
}

func TestTheLedgerHoldsNoLineOfADayAsItsTakeOrElseItsFilesTell(t *testing.T) {
	date := dayfile.DateOf(importedOn.AddDate(0, 0, -10))
	tests := []struct {
		name string
		took dayfile.Taken
		// held reports whether the ledger holds a file of the day's lines.
		held bool
		want bool
	}{
		{name: "a day added, its lines just taken in", took: dayfile.Added, held: true, want: true},
		{name: "a day merged", took: dayfile.Merged, held: true},
		{name: "a day past keeping, of no file", took: dayfile.PastKeep, want: true},
		{name: "a day past keeping, of a file", took: dayfile.PastKeep, held: true},
		{name: "a day staged of no line, or of its summary alone, of no file", want: true},
		{name: "a day staged of no line, or of its summary alone, of a file", held: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logs.For("router")
			d := days{files: Files(t.TempDir(), logger), logger: logger}
			if tt.held {
				if err := os.WriteFile(filepath.Join(d.files.Dir, "requests-"+date+".jsonl"), []byte(`{"request":"own"}`+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			got, err := d.holdsNoLine(date, dayfile.Imported{Taken: tt.took})
			if err != nil || got != tt.want {
				t.Errorf("holdsNoLine(%s) = %v, %v, want %v", date, got, err, tt.want)
			}
		})
	}
}

func TestASummaryCountsNoLineWhereItsNoneOrOfTheReadingsAlone(t *testing.T) {
	date := dayfile.DateOf(importedOn.AddDate(0, 0, -10))
	tests := []struct {
		name string
		// summary is the ledger's summary of the day, none where it's "".
		summary string
		want    bool
	}{
		{name: "none", want: true},
		{name: "one of the readings alone", summary: `{"version":6,"day":"` + date + `","lines":0}` + "\n", want: true},
		{name: "one counting a line", summary: `{"version":6,"day":"` + date + `","lines":3}` + "\n"},
		{name: "one cut short, which may have counted some", summary: `{"version":6,"day":"` + date + `","li`},
		{name: "one of another day", summary: `{"version":6,"day":"2026-09-17","lines":0}` + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logs.For("router")
			d := days{files: Files(t.TempDir(), logger), logger: logger}
			if tt.summary != "" {
				if err := os.WriteFile(SummaryFile(d.files.Dir, date), []byte(tt.summary), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			got, err := d.countsNoLine(date)
			if err != nil || got != tt.want {
				t.Errorf("countsNoLine(%s) = %v, %v, want %v", date, got, err, tt.want)
			}
		})
	}
}
