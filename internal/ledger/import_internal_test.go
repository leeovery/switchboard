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
// its summary, in a directory of the test's own, and returns it as l, holding
// nothing of it, looks at it.
func staged(t *testing.T, l *Ledger) LookedDay {
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
	looked, err := l.Look([]StagedDay{day})
	if err != nil || !looked[0].Unseen {
		t.Fatalf("Look() = %+v, %v, want the day unseen", looked, err)
	}
	return looked[0]
}

// holding reports whether l holds the day's lines, and its summary.
func holding(l *Ledger, day LookedDay) (lines, summary bool) {
	_, err := l.days.files.Stat(day.Date)
	_, summaryErr := os.Stat(SummaryFile(l.days.files.Dir, day.Date))
	return err == nil, summaryErr == nil
}

// inRound starts taking day in as a round holds l's summaries, as the
// readings history's summarising of the days before its prune does, and
// returns once the day's lines are in, with what Import fails with once it
// returns, what cuts it short, and what ends the round, as the test's end
// does too.
func inRound(t *testing.T, l *Ledger, day LookedDay) (imported <-chan error, cancel, endRound func()) {
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
	day := staged(t, l)
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
	day := staged(t, l)
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
	day := staged(t, l)
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

func TestADaysSummaryIsUnseenOnlyWhereTheLedgerHoldsNothingOfIt(t *testing.T) {
	date := dayfile.DateOf(importedOn.AddDate(0, 0, -10))
	tests := []struct {
		name string
		// held are the ledger's own files, by their names.
		held map[string]string
		want bool
	}{
		{name: "a day it never saw", want: true},
		{name: "a day of lines alone", held: map[string]string{"requests-" + date + ".jsonl": `{"request":"own"}` + "\n"}},
		{name: "a day of its summary alone", held: map[string]string{"day-" + date + ".json": `{"version":6,"day":"` + date + `","lines":0}` + "\n"}},
		{name: "a day of a summary that doesn't read as its own", held: map[string]string{"day-" + date + ".json": "{"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logs.For("router")
			d := days{files: Files(t.TempDir(), logger), logger: logger}
			for name, data := range tt.held {
				if err := os.WriteFile(filepath.Join(d.files.Dir, name), []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			got, err := d.unseen(date)
			if err != nil || got != tt.want {
				t.Errorf("unseen(%s) = %v, %v, want %v", date, got, err, tt.want)
			}
		})
	}
}

func TestADayStagedWithoutItsSummaryIsNeverUnseen(t *testing.T) {
	l := runningLedger(t)
	date := dayfile.DateOf(importedOn.AddDate(0, 0, -10))

	looked, err := l.Look([]StagedDay{{Date: date, Lines: filepath.Join(t.TempDir(), "requests-"+date+".jsonl.gz")}})
	if err != nil || looked[0].Unseen {
		t.Errorf("Look() of a day staged without its summary = %+v, %v, want it never unseen, as there's none to copy in", looked, err)
	}
}
