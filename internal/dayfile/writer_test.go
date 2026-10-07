package dayfile

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// noted is what the tests note to a Writer: a line's text, filed under the
// local day of when it was noted.
type noted struct {
	at   time.Time
	text string
}

// linesNoted puts n as the files' lines.
func linesNoted(n noted) Lines {
	lines := make(Lines)
	lines.Add(n.at, []byte(n.text))
	return lines
}

// testWriter returns a Writer of f, on now's clock, whose queue holds queue
// of what's noted, keeping a day's files two weeks, and logging what's noted
// as readings.
func testWriter(f *Files, now func() time.Time, queue int) *Writer[noted] {
	return NewWriter(f, linesNoted, WriterOptions{Queue: queue, Keep: twoWeeks, Now: now, Items: "readings"})
}

// running has w write what's noted to it, and returns what stops it, once it
// has written everything noted before.
func running[T any](t *testing.T, w *Writer[T]) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	return func() {
		cancel()
		<-done
	}
}

// testClock is a clock that reads now, which a test moves as it goes.
type testClock struct {
	now time.Time
}

func (c *testClock) read() time.Time {
	return c.now
}

func TestAWriterWritesWhatsNotedToTheFilesOfItsDays(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := readingsHistory(t.TempDir())
		w := testWriter(f, at(start), 4)
		yesterday := start.Local().AddDate(0, 0, -1)
		holds := func(when string, date string, want string) {
			t.Helper()
			if got := heldIn(t, f, plainFile(date)); got != want {
				t.Errorf("%s, the plain file of %s holds\n%s\nwant what was noted of its day, in the order noted\n%s", when, date, got, want)
			}
		}
		stop := running(t, w)

		w.Note(noted{at: yesterday, text: "one"})
		w.Note(noted{at: start, text: "two"})
		synctest.Wait()
		holds("as it runs", dateOf(yesterday), linesOf("one"))
		holds("as it runs", dateOf(start), linesOf("two"))
		w.Note(noted{at: start, text: "three"})
		stop()
		holds("once it has stopped", dateOf(start), linesOf("two", "three"))
	})
}

func TestTheFilesDirectoryIsMadePrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	w := testWriter(readingsHistory(dir), at(start), 1)

	running(t, w)()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != fs.ModeDir|0o700 {
		t.Errorf("the files' directory is %v, want %v", info.Mode(), fs.ModeDir|0o700)
	}
}

func TestAWriteThatFailsIsLoggedOnceUntilOneSucceeds(t *testing.T) {
	log := logstest.Capture(t)
	f := readingsHistory(t.TempDir())
	w := testWriter(f, at(start), 1)
	one := noted{at: start, text: "one"}
	today := f.path(plainFile(dateOf(start)))
	// Today's file can't be opened to append while a directory stands at its
	// path.
	block := func() {
		if err := os.Mkdir(today, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	unblock := func() {
		if err := os.Remove(today); err != nil {
			t.Fatal(err)
		}
	}

	block()
	w.write(one)
	w.write(one)
	unblock()
	w.write(one)
	if err := os.Remove(today); err != nil {
		t.Fatal(err)
	}
	block()
	w.write(one)
	if got := len(linesWith(log, "can't write the readings history")); got != 2 {
		t.Errorf("log reads\n%s\nwant the failure noted twice, once before the write that succeeded and once after", log)
	}
	if got := len(linesWith(log, "writing the readings history again")); got != 1 {
		t.Errorf("log reads\n%s\nwant the files noted as written again once", log)
	}
}

func TestFallingBehindDropsWhatsNotedRatherThanWait(t *testing.T) {
	log := logstest.Capture(t)
	const queue = 4
	w := testWriter(readingsHistory(t.TempDir()), at(start), queue)
	one := noted{at: start, text: "one"}
	fill := func() {
		for range queue + 2 {
			w.Note(one)
		}
	}

	fill()
	if got := len(linesWith(log, "fell behind")); got != 1 {
		t.Errorf("log reads\n%s\nwant what was dropped noted once", log)
	}
	w.write(<-w.queue)
	fill()
	if got := len(linesWith(log, "fell behind")); got != 2 {
		t.Errorf("log reads\n%s\nwant what was dropped noted again, once a write has caught up", log)
	}
}

func TestTheFilesArePrunedOnTheFirstWriteOfANewDay(t *testing.T) {
	clock := &testClock{now: start}
	f := readingsHistory(t.TempDir())
	w := testWriter(f, clock.read, 1)
	w.round()
	// Its day ended 13 days before start's, so it goes on the day after.
	old := dateOf(start.Local().AddDate(0, 0, -14))
	writeDay(t, f, plainFile(old), linesOf("old"))

	w.write(noted{at: clock.now, text: "one"})
	if !holdsDay(f, old) {
		t.Fatal("the day's file went on a day it's kept")
	}
	clock.now = start.Add(24 * time.Hour)
	w.write(noted{at: clock.now, text: "two"})
	if holdsDay(f, old) {
		t.Error("the day's file stayed on the day after, want it pruned with that day's first write")
	}
}

func TestTheFilesArePrunedOnANewDayWithNothingToWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := readingsHistory(t.TempDir())
		began := time.Now()
		w := testWriter(f, func() time.Time { return start.Add(time.Since(began)) }, 1)
		stop := running(t, w)
		defer stop()
		synctest.Wait()
		// A day long past keeping, whose file comes once the files have been
		// pruned for start's day, so it goes at the next prune.
		old := dateOf(start.Local().AddDate(0, 0, -30))
		writeDay(t, f, plainFile(old), linesOf("old"))
		y, m, d := start.Local().Date()
		tomorrow := time.Date(y, m, d+1, 0, 0, 0, 0, time.Local)

		time.Sleep(tomorrow.Sub(start) - time.Minute)
		synctest.Wait()
		if !holdsDay(f, old) {
			t.Fatal("the day's file went before the day after start's, want it kept until the files are next pruned")
		}
		// The writer looks every pruneLook, and the clocks may change by an
		// hour on the way.
		time.Sleep(time.Minute + 2*pruneLook)
		synctest.Wait()
		if holdsDay(f, old) {
			t.Error("the day's file stayed on the day after start's, want it pruned though nothing was written")
		}
	})
}

func TestAWriterMakesItsRoundAsItStartsAndEveryHourAfterBeforePruning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := readingsHistory(t.TempDir())
		// A day long past keeping, which the first round's pruning removes.
		old := dateOf(start.Local().AddDate(0, 0, -30))
		writeDay(t, f, plainFile(old), linesOf("old"))
		began := time.Now()
		var rounds []time.Time
		var held []bool
		w := NewWriter(f, linesNoted, WriterOptions{Queue: 1, Keep: twoWeeks, Now: func() time.Time { return start.Add(time.Since(began)) }, Items: "readings",
			Round: func(now time.Time) {
				rounds = append(rounds, now)
				held = append(held, holdsDay(f, old))
			}})
		stop := running(t, w)

		time.Sleep(2*pruneLook + time.Minute)
		synctest.Wait()
		stop()
		if want := []time.Time{start, start.Add(pruneLook), start.Add(2 * pruneLook)}; !slices.EqualFunc(rounds, want, time.Time.Equal) {
			t.Errorf("rounds were made at %v, want %v: as the writer started, and every %v after", rounds, want, pruneLook)
		}
		if want := []bool{true, false, false}; !slices.Equal(held, want) {
			t.Errorf("the day past keeping was there for each round = %v, want %v: the first round made before the pruning it went at", held, want)
		}
	})
}

func TestTheFirstWriteOfANewDayMakesTheRoundBeforeItPrunes(t *testing.T) {
	clock := &testClock{now: start}
	f := readingsHistory(t.TempDir())
	// A day whose files go once they're pruned a day on.
	old := dateOf(start.Local().AddDate(0, 0, -14))
	writeDay(t, f, plainFile(old), linesOf("old"))
	var rounds []time.Time
	var held []bool
	w := NewWriter(f, linesNoted, WriterOptions{Queue: 1, Keep: twoWeeks, Now: clock.read, Items: "readings", Round: func(now time.Time) {
		rounds = append(rounds, now)
		held = append(held, holdsDay(f, old))
	}})
	w.round()

	// A day on, the first write comes before the hourly round, as after a
	// sleep, which holds that back.
	clock.now = start.Add(24 * time.Hour)
	w.write(noted{at: clock.now, text: "one"})
	if want := []time.Time{start, clock.now}; !slices.EqualFunc(rounds, want, time.Time.Equal) || !slices.Equal(held, []bool{true, true}) {
		t.Errorf("rounds were made at %v, the day past keeping there for each = %v; want %v, the second as the write came, before the prune", rounds, held, want)
	}
	if holdsDay(f, old) {
		t.Error("the day past keeping stayed, want it pruned with the day's first write")
	}
}

func TestWhatsLoggedNamesTheFilesAndWhatTheyHold(t *testing.T) {
	old, done, today := dateOf(start.AddDate(0, 0, -30)), dateOf(start.AddDate(0, 0, -3)), dateOf(start)
	// blockedDir returns a directory that can't be made, nor read, as a file
	// stands in its path.
	blockedDir := func(t *testing.T) string {
		t.Helper()
		file := filepath.Join(t.TempDir(), "state")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(file, "ledger")
	}
	write := func(t *testing.T, path string, data []byte, perm fs.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, data, perm); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name string
		// blocked says the files' directory can't be made, nor read.
		blocked bool
		// trouble brings the files f, written by w, to trouble.
		trouble func(t *testing.T, f *Files, w *Writer[noted])
		want    []string
	}{
		{
			name:    "a directory that can't be made private",
			blocked: true,
			trouble: func(t *testing.T, f *Files, _ *Writer[noted]) { f.makePrivate() },
			want:    []string{"level=WARN", `msg="can't make the request ledger private"`, "dir="},
		},
		{
			name:    "a directory that can't be pruned",
			blocked: true,
			trouble: func(t *testing.T, f *Files, _ *Writer[noted]) { f.prune(start, twoWeeks) },
			want:    []string{"level=WARN", `msg="can't prune the request ledger"`, "dir="},
		},
		{
			name: "a file that can't be pruned",
			trouble: func(t *testing.T, f *Files, _ *Writer[noted]) {
				// A directory, not empty, stands where the file is.
				if err := os.MkdirAll(filepath.Join(f.path(plainFile(old)), "held"), 0o700); err != nil {
					t.Fatal(err)
				}
				f.prune(start, twoWeeks)
			},
			want: []string{"level=WARN", `msg="can't prune the request ledger"`, "file=requests-" + old + ".jsonl"},
		},
		{
			name: "a file that can't be compressed",
			trouble: func(t *testing.T, f *Files, _ *Writer[noted]) {
				writeDay(t, f, plainFile(done), linesOf("done"))
				write(t, f.path(compressedFile(done)), []byte("not gzip"), 0o600)
				f.prune(start, twoWeeks)
			},
			want: []string{"level=WARN", `msg="can't compress the request ledger"`, "file=requests-" + done + ".jsonl"},
		},
		{
			name:    "a directory that can't be read",
			blocked: true,
			trouble: func(t *testing.T, f *Files, _ *Writer[noted]) { f.Newest(start, 2) },
			want:    []string{"level=WARN", `msg="can't read the request ledger"`, "dir="},
		},
		{
			name: "a file that can't be read",
			trouble: func(t *testing.T, f *Files, _ *Writer[noted]) {
				write(t, f.path(plainFile(today)), []byte("one\n"), 0o000)
				readAll(f, today)
			},
			want: []string{"level=WARN", `msg="can't read the request ledger"`, "file=requests-" + today + ".jsonl"},
		},
		{
			name: "a file that reads short",
			trouble: func(t *testing.T, f *Files, _ *Writer[noted]) {
				write(t, f.path(compressedFile(today)), append(gzipOf(t, linesOf("one")), gzipOf(t, linesOf("two"))[:5]...), 0o600)
				readAll(f, today)
			},
			want: []string{"level=WARN", `msg="request ledger read short"`, "file=requests-" + today + ".jsonl.gz"},
		},
		{
			name:    "a write that fails",
			blocked: true,
			trouble: func(t *testing.T, _ *Files, w *Writer[noted]) { w.write(noted{at: start, text: "one"}) },
			want:    []string{"level=WARN", `msg="can't write the request ledger; lines go unwritten until it can"`, "dir="},
		},
		{
			name: "writing again",
			trouble: func(t *testing.T, f *Files, w *Writer[noted]) {
				blocker := f.path(plainFile(today))
				if err := os.Mkdir(blocker, 0o700); err != nil {
					t.Fatal(err)
				}
				w.write(noted{at: start, text: "one"})
				if err := os.Remove(blocker); err != nil {
					t.Fatal(err)
				}
				w.write(noted{at: start, text: "one"})
			},
			want: []string{"level=INFO", `msg="writing the request ledger again"`, "dir="},
		},
		{
			name: "falling behind",
			trouble: func(t *testing.T, _ *Files, w *Writer[noted]) {
				w.Note(noted{at: start, text: "one"})
				w.Note(noted{at: start, text: "two"})
			},
			want: []string{"level=WARN", `msg="request ledger fell behind; lines dropped from it"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			dir := t.TempDir()
			if tt.blocked {
				dir = blockedDir(t)
			}
			f := requestLedger(dir)
			w := NewWriter(f, linesNoted, WriterOptions{Queue: 1, Keep: twoWeeks, Now: at(start), Items: "lines"})

			tt.trouble(t, f, w)
			if !log.Has(tt.want...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, tt.want)
			}
		})
	}
}
