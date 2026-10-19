package ledger_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// cantBuildHead is what the digesting warns of a day whose head can't be
// built, or written.
const cantBuildHead = "can't build the request ledger's head of a day"

// logdayPath is the path of the head of the local day days after the
// summaries' day the ledger in dir holds.
func logdayPath(dir string, days int) string {
	return filepath.Join(dir, "logday-"+dayOf(days)+".json.gz")
}

// dayHead returns the head the world's ledger holds of the local day days
// after the summaries' day, of the sides the tests' Digester gives, failing
// the test where it holds none that reads.
func (w *lookupWorld) dayHead(days int) ledger.DayHead {
	w.t.Helper()
	head, err := ledger.ReadDayHead(w.dir, dayOf(days), sideBody)
	if err != nil {
		w.t.Fatalf("the head of %s: %v", dayOf(days), err)
	}
	return head
}

// headed reports whether the world's ledger holds a head of the local day
// days after the summaries' day.
func (w *lookupWorld) headed(days int) bool {
	w.t.Helper()
	_, err := os.Stat(logdayPath(w.dir, days))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		w.t.Fatal(err)
	}
	return err == nil
}

// follower returns a follower of the world's ledger, by its clock.
func (w *lookupWorld) follower() *ledger.Follower {
	return followerOf(w.state, &testClock{at: w.at}, caps)
}

// requestsOn returns the requests of the lines f gives of the local day with
// the given date, as Day gives them, in its order.
func requestsOn(f *ledger.Follower, date string) []string {
	var requests []string
	for _, h := range f.Day(date) {
		requests = append(requests, h.Request)
	}
	return requests
}

// headsIn returns every head the ledger in dir holds, by its day's date, of
// the sides the tests' Digester gives, failing the test where one doesn't
// read.
func headsIn(t *testing.T, dir string) map[string]ledger.DayHead {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "logday-*.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	heads := make(map[string]ledger.DayHead, len(paths))
	for _, path := range paths {
		date := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "logday-"), ".json.gz")
		if heads[date], err = ledger.ReadDayHead(dir, date, sideBody); err != nil {
			t.Fatalf("the head of %s: %v", date, err)
		}
	}
	return heads
}

// gunzippedFile is what the gzipped file at path holds, inflated.
func gunzippedFile(t *testing.T, path string) []byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	r, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return plain
}

// removeHeads removes every head the ledger in dir holds, and every rows
// file.
func removeHeads(tb testing.TB, dir string) {
	tb.Helper()
	for _, pattern := range []string{"logday-*", "rows-*"} {
		held, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			tb.Fatal(err)
		}
		for _, path := range held {
			if err := os.Remove(path); err != nil {
				tb.Fatal(err)
			}
		}
	}
}

// A finished day's head is built of exactly the lines its day gives, as
// Follower.Day gives them, in its order: those its file holds, oldest first
// by their arrival, whenever each was written, and one a change of time zone
// filed under the day after, which the day after's leaves out. It's of the
// day's bounds, the lines its summary counts, and as many as the day gave,
// and so are its rows; and a follower reads its side and rows as they were
// built, where it stands: not those of the day the day after's digest tells
// of a line of, as no day head stands whose day isn't alone. A day of no
// lines has none, nor rows.
func TestADaysHeadIsBuiltOfExactlyTheLinesItsDayGives(t *testing.T) {
	w := newLookupWorld(t)
	w.file(-3, askedBy("1", "one", on(-3, 9, 0)))
	w.file(0, askedBy("3", "one", on(0, 9, 0)), askedBy("2", "two", on(0, 8, 0)))
	w.file(1, askedBy("4", "two", on(0, 23, 0)), askedBy("5", "one", on(1, 9, 0)))
	w.summarise()

	w.digestEnded()

	f := w.follower()
	tests := []struct {
		days       int
		summarised int
		requests   []string
		stands     bool
	}{
		{days: -3, summarised: 1, requests: []string{"1"}, stands: true},
		{days: 0, summarised: 2, requests: []string{"2", "3", "4"}},
		{days: 1, summarised: 2, requests: []string{"5"}, stands: true},
	}
	for _, tt := range tests {
		t.Run(dayOf(tt.days), func(t *testing.T) {
			date := dayOf(tt.days)
			start, end, _ := dayfile.Day(date)
			want := ledger.DayHead{Body: sideBody, Day: date, Start: start, End: end, SummaryLines: tt.summarised, Lines: len(tt.requests), Side: sideOf(t, tt.requests...)}
			head := w.dayHead(tt.days)
			wantRows := builtRead(chunksOf(tt.requests...))
			if rows := rowsRead(t, f, date, head.Rows); !reflect.DeepEqual(rows, wantRows) {
				t.Errorf("the rows of %s are %+v, want %+v", date, rows, wantRows)
			}
			if head.Rows = (ledger.DayRows{}); !reflect.DeepEqual(head, want) {
				t.Errorf("the head of %s is\n%+v\nwant\n%+v", date, head, want)
			}
			if given := requestsOn(f, date); !slices.Equal(given, tt.requests) {
				t.Errorf("Day(%s) gives the lines of %q, want %q, as the head was built of", date, given, tt.requests)
			}
			if side, ok := f.DaySide(date, sideBody); ok != tt.stands || ok && !bytes.Equal(side, want.Side) {
				t.Errorf("DaySide(%s) = %s, %t; want %s, as it was built, read %t", date, side, ok, want.Side, tt.stands)
			}
			if rows, ok := dayRows(t, f, date); ok != tt.stands || ok && !reflect.DeepEqual(rows, wantRows) {
				t.Errorf("DayRows(%s) = %+v, %t; want %+v, as they were built, read %t", date, rows, ok, wantRows, tt.stands)
			}
		})
	}
	for _, days := range []int{-2, -1, 2} {
		if w.headed(days) || w.rowed(days) {
			t.Errorf("the ledger holds a head of %s %t, its rows %t, a day of no lines; want neither", dayOf(days), w.headed(days), w.rowed(days))
		}
	}
}

// A day's side, and its rows, are read only where its head stands, as it was
// built of the day as the ledger keeps it now, of the version asked for, and
// no line of the days either side arrived on it, as their digests tell: none
// where the day's lines are read instead.
func TestADaysSideAndRowsAreReadOnlyWhereItsHeadStands(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, w *lookupWorld)
		body   int
		stands bool
	}{
		{name: "as it was built", change: func(*testing.T, *lookupWorld) {}, body: sideBody, stands: true},
		{name: "none held", change: func(t *testing.T, w *lookupWorld) { removeFile(t, w.dir, filepath.Base(logdayPath(w.dir, 0))) }, body: sideBody},
		{name: "asked of another version", change: func(*testing.T, *lookupWorld) {}, body: sideBody + 1},
		{name: "built in another time zone", change: func(t *testing.T, w *lookupWorld) {
			head := w.dayHead(0)
			head.Start, head.End = head.Start.Add(3*time.Hour), head.End.Add(3*time.Hour)
			if err := ledger.WriteDayHead(w.dir, head); err != nil {
				t.Fatal(err)
			}
		}, body: sideBody},
		{name: "a line filed under its day since, its summary not made again", change: func(_ *testing.T, w *lookupWorld) {
			w.file(0, askedBy("late", "two", on(0, 12, 0)))
		}, body: sideBody},
		{name: "a line filed under its day since, its summary made again", change: func(_ *testing.T, w *lookupWorld) {
			w.file(0, askedBy("late", "two", on(0, 12, 0)))
			w.summarise()
		}, body: sideBody},
		{name: "its lines pruned", change: func(t *testing.T, w *lookupWorld) { removeFile(t, w.dir, "requests-"+dayOf(0)+".jsonl") }, body: sideBody},
		{name: "a line arriving on it filed since under the day after, that day digested again", change: func(_ *testing.T, w *lookupWorld) {
			w.file(1, askedBy("x", "four", on(0, 23, 0)))
			w.summarise()
			w.digestEnded()
		}, body: sideBody},
		{name: "a line arriving on it filed since under the day before, that day digested", change: func(_ *testing.T, w *lookupWorld) {
			w.file(-1, askedBy("x", "four", on(0, 0, 30)))
			w.summarise()
			w.digestEnded()
		}, body: sideBody},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newLookupWorld(t)
			sharedDays(w)
			w.digestEnded()
			tt.change(t, w)

			f := w.follower()
			side, ok := f.DaySide(dayOf(0), tt.body)
			rows, rowsOK := f.DayRows(dayOf(0), tt.body)

			if want := sideOf(t, "4", "5"); ok != tt.stands || ok && !bytes.Equal(side, want) {
				t.Errorf("DaySide(%s, %d) = %s, %t; want %s read %t", dayOf(0), tt.body, side, ok, want, tt.stands)
			}
			if want := builtRead(chunksOf("4", "5")); rowsOK != tt.stands || rowsOK && !reflect.DeepEqual(rowsRead(t, f, dayOf(0), rows), want) {
				t.Errorf("DayRows(%s, %d) = %+v, %t; want %+v read %t", dayOf(0), tt.body, rows, rowsOK, want, tt.stands)
			}
		})
	}
}

// A line filed late under a day whose head is built has the day's head and
// rows built again, of the line, once the day is summarised again with it,
// and none other: until then its lines can't be told, so it isn't, and
// neither its side nor its rows are read.
func TestALateLineHasItsDaysHeadBuiltAgain(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	w.fold.sides()
	w.file(-2, askedBy("late", "two", on(-2, 12, 0)))

	w.digestEnded()

	if built := requestsOfSides(w.fold.sides()); len(built) > 0 {
		t.Errorf("with %s not summarised again, sides of %q were built, want none", dayOf(-2), built)
	}
	f := w.follower()
	if _, ok := f.DaySide(dayOf(-2), sideBody); ok {
		t.Errorf("DaySide(%s) is read, a line filed under it since its head was built; want it unread", dayOf(-2))
	}
	if rows, ok := f.DayRows(dayOf(-2), sideBody); ok {
		t.Errorf("DayRows(%s) = %+v, read, a line filed under it since its head was built; want them unread", dayOf(-2), rows)
	}
	w.summarise()
	w.digestEnded()
	if built, want := requestsOfSides(w.fold.sides()), [][]string{{"3", "late"}}; !reflect.DeepEqual(built, want) {
		t.Errorf("once %s is summarised again, sides of %q were built, want %q: its own alone", dayOf(-2), built, want)
	}
	if head := w.dayHead(-2); head.SummaryLines != 2 || head.Lines != 2 || !bytes.Equal(head.Side, sideOf(t, "3", "late")) {
		t.Errorf("the head of %s is %+v; want it of the two lines its summary counts, the late one among them", dayOf(-2), head)
	}
	if rows, ok := dayRows(t, f, dayOf(-2)); !ok || !reflect.DeepEqual(rows, builtRead(chunksOf("3", "late"))) {
		t.Errorf("DayRows(%s) = %+v, %t; want them of 3 and the late line, read", dayOf(-2), rows, ok)
	}
}

// A day's head built in another time zone is built again in this one, by
// the router started again in it, its bounds the day's here, of the lines
// its day gives here: a line that arrived late in the evening there falls on
// the day after here, whose side isn't read, as the digest of the day its
// file is of tells of it.
func TestADaysHeadBuiltInAnotherTimeZoneIsBuiltAgain(t *testing.T) {
	w := newLookupWorld(t)
	w.file(0, askedBy("night", "one", on(0, 22, 30)))
	sharedDays(w)
	w.digestEnded()
	w.fold.sides()
	zone := time.Local
	_, offset := on(0, 12, 0).Zone()
	time.Local = time.FixedZone("elsewhere", offset+3*60*60)
	t.Cleanup(func() { time.Local = zone })
	f := w.follower()
	if side, ok := f.DaySide(dayOf(0), sideBody); ok {
		t.Errorf("DaySide(%s) = %s in another time zone than its head was built in; want it unread", dayOf(0), side)
	}

	// A process's time zone is the one it starts in, so the router digests
	// in another only once it starts again there.
	w.opened(w.fold).DigestEnded(t.Context(), w.at)

	if built := w.fold.sides(); len(built) != 4 {
		t.Errorf("%d sides were built in another time zone, want 4, of every day of lines", len(built))
	}
	for days, want := range map[int][]string{-3: {"1", "2"}, -2: {"3"}, 0: {"4", "5"}, 1: {"night", "6", "7"}} {
		date := dayOf(days)
		start, end, _ := dayfile.Day(date)
		if head := w.dayHead(days); !head.Start.Equal(start) || !head.End.Equal(end) || !bytes.Equal(head.Side, sideOf(t, want...)) {
			t.Errorf("the head of %s is %+v; want it from %v to %v, of %q", date, head, start, end, want)
		}
		if given := requestsOn(f, date); !slices.Equal(given, want) {
			t.Errorf("Day(%s) gives the lines of %q, want %q, as the head was built of", date, given, want)
		}
		if side, ok := f.DaySide(date, sideBody); ok != (days != 1) || ok && !bytes.Equal(side, sideOf(t, want...)) {
			t.Errorf("DaySide(%s) = %s, %t; want %s, as it was built again, read %t", date, side, ok, sideOf(t, want...), days != 1)
		}
	}
}

// The head of a day whose lines are pruned is deleted, with its rows file,
// its digest and its parts file, as are a stray head and rows file of a day
// never written; those of the days either side are kept as they were.
func TestTheHeadOfADayWithoutLinesIsDeleted(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	for _, days := range []int{-1, 2} {
		start, end, _ := dayfile.Day(dayOf(days))
		if err := ledger.WriteDayHead(w.dir, ledger.DayHead{Body: sideBody, Day: dayOf(days), Start: start, End: end}); err != nil {
			t.Fatal(err)
		}
		if err := ledger.WriteRows(w.dir, dayOf(days), chunksOf("stray")); err != nil {
			t.Fatal(err)
		}
	}
	kept := map[string]os.FileInfo{}
	for _, days := range []int{-3, 0, 1} {
		for _, path := range []string{logdayPath(w.dir, days), rowsPath(w.dir, days)} {
			kept[path] = fileInfo(t, path)
		}
	}
	removeFile(t, w.dir, "requests-"+dayOf(-2)+".jsonl")

	w.digestEnded()

	for _, days := range []int{-2, -1, 2} {
		for _, path := range []string{logdayPath(w.dir, days), rowsPath(w.dir, days), digestPath(w.dir, days), partsPath(w.dir, days)} {
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("%s, of a day without lines, is kept, looked at: %v; want it deleted", filepath.Base(path), err)
			}
		}
	}
	for path, was := range kept {
		if !dayfile.SameAs(was, fileInfo(t, path)) {
			t.Errorf("%s changed; want it kept as it was", filepath.Base(path))
		}
	}
}

// A ledger whose directory holds heads and rows files, but no day, has them
// deleted: a rows file of a day of no head among them.
func TestTheHeadsOfALedgerHoldingNoDayAreDeleted(t *testing.T) {
	w := newLookupWorld(t)
	start, end, _ := dayfile.Day(dayOf(-1))
	if err := ledger.WriteDayHead(w.dir, ledger.DayHead{Body: sideBody, Day: dayOf(-1), Start: start, End: end}); err != nil {
		t.Fatal(err)
	}
	for _, days := range []int{-1, -2} {
		if err := ledger.WriteRows(w.dir, dayOf(days), chunksOf("stray")); err != nil {
			t.Fatal(err)
		}
	}

	w.digestEnded()

	if w.headed(-1) || w.rowed(-1) || w.rowed(-2) {
		t.Errorf("the head of %s is kept %t, its rows %t, and the rows of %s %t, the ledger holding no day; want each deleted",
			dayOf(-1), w.headed(-1), w.rowed(-1), dayOf(-2), w.rowed(-2))
	}
}

// Run builds the finished days' heads as it starts, after their digests,
// newest first, as none rests on another: yesterday's first.
func TestRunBuildsTheDaysHeadsNewestFirstAfterTheirDigests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const took = time.Minute
		w := runWorld(t)
		w.fold.doing(func(context.Context, ledger.DigestDay) error {
			time.Sleep(took)
			return nil
		})

		stop := running(t, w.writer)
		synctest.Sleep(time.Hour / 2)
		stop()

		handed, built := w.fold.days(), w.fold.sides()
		if got, want := requestsOfSides(built), [][]string{{"6", "7"}, {"4", "5"}, {"3"}, {"1", "2"}}; !reflect.DeepEqual(got, want) {
			t.Fatalf("Run built sides of %q, want %q, newest first", got, want)
		}
		if last := handed[len(handed)-1]; !built[0].at.After(last.at) {
			t.Errorf("the first head was built at %v, the last digest begun at %v; want the heads after the digests", built[0].at, last.at)
		}
	})
}

// Run builds the heads due as it starts, pausing after each build as long as
// it took, so the next begins twice that after the last.
func TestRunPausesAfterEachHeadBuiltAsLongAsItTook(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const took = time.Minute
		w := runWorld(t)
		w.digestEnded()
		w.fold.sides()
		removeHeads(t, w.dir)
		w.fold.buildingSides(func([]string) error {
			time.Sleep(took)
			return nil
		})

		stop := running(t, w.writer)
		synctest.Sleep(time.Hour / 2)
		stop()

		built := w.fold.sides()
		if len(built) != 4 {
			t.Fatalf("Run built %d sides, want 4, of every day of lines", len(built))
		}
		for i := 1; i < len(built); i++ {
			if gap := built[i].at.Sub(built[i-1].at); gap != 2*took {
				t.Errorf("the side of %q was begun %v after that of %q, want %v: its build, and as long again", built[i].requests, gap, built[i-1].requests, 2*took)
			}
		}
	})
}

// A head built as Run's context ends is written of none, nor its rows, and no
// other is begun: Run returns once its builder is done.
func TestRunStopsBuildingHeadsAsItsContextEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := runWorld(t)
		w.digestEnded()
		w.fold.sides()
		removeHeads(t, w.dir)
		w.fold.buildingSides(func([]string) error {
			time.Sleep(time.Minute)
			return nil
		})
		began := time.Now()
		stop := running(t, w.writer)
		synctest.Sleep(time.Minute / 2)

		stop()

		if ran, built := time.Since(began), len(w.fold.sides()); ran != time.Minute || built != 1 {
			t.Errorf("Run returned after %v, having built %d sides; want it returned once its builder was done, at a minute, having built one", ran, built)
		}
		if heads, rows := headsIn(t, w.dir), filesMatching(t, w.dir, "rows-*"); len(heads)+len(rows) > 0 {
			t.Errorf("the ledger holds heads of %q, and rows files %q, want none written", slices.Sorted(maps.Keys(heads)), slices.Sorted(maps.Keys(rows)))
		}
	})
}

// A day's lines are read for its head only until the context ends: its
// builder is handed none after, its head is written of none, and no day's
// after it is built.
func TestAHeadsReadStopsAsItsContextEnds(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	removeHeads(t, w.dir)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var taken []string
	w.fold.takingSides(func(request string) {
		taken = append(taken, request)
		if request == "4" {
			cancel()
		}
	})

	w.writer.DigestEnded(ctx, w.at)

	if want := []string{"6", "7", "4"}; !slices.Equal(taken, want) {
		t.Errorf("the builders were handed the lines of %q, want %q: yesterday's, then the day before's until the context ended", taken, want)
	}
	if !w.headed(1) || w.headed(0) || w.headed(-2) || w.headed(-3) {
		t.Errorf("heads are held of %s %t, %s %t, %s %t, %s %t; want yesterday's alone", dayOf(1), w.headed(1), dayOf(0), w.headed(0),
			dayOf(-2), w.headed(-2), dayOf(-3), w.headed(-3))
	}
}

// A head that can't be written is warned of once over passes, and written
// at a later pass once it can be.
func TestAHeadThatCantBeWrittenIsWarnedOfOnceAndWrittenOnceItCanBe(t *testing.T) {
	log := logstest.Capture(t)
	w := newLookupWorld(t)
	sharedDays(w)
	// A head can't be written over a directory standing at its path.
	blocked := logdayPath(w.dir, 0)
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}

	w.digestEnded()
	w.digestEnded()

	if warned := warnedOf(log, cantBuildHead, dayOf(0)); warned != 1 {
		t.Errorf("log reads\n%s\nwant %s warned of once over two passes, not %d times", log, dayOf(0), warned)
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	w.digestEnded()
	if head := w.dayHead(0); !bytes.Equal(head.Side, sideOf(t, "4", "5")) {
		t.Errorf("once it can be written, the head of %s is %+v, want its side of 4 and 5", dayOf(0), head)
	}
}

// A day whose side can't be built is tried again at each pass, warned of
// once until it changes, then again, and its head built once its builder
// can.
func TestADayWhoseSideCantBeBuiltIsWarnedOfOnceUntilItChanges(t *testing.T) {
	log := logstest.Capture(t)
	w := newLookupWorld(t)
	sharedDays(w)
	w.fold.buildingSides(func(requests []string) error {
		if slices.Contains(requests, "4") {
			return errFolding
		}
		return nil
	})

	for range 3 {
		w.digestEnded()
	}

	tried := slices.DeleteFunc(requestsOfSides(w.fold.sides()), func(requests []string) bool { return !slices.Contains(requests, "4") })
	if len(tried) != 3 || w.headed(0) {
		t.Errorf("over three passes %s's side was built %d times, its head written %t; want three times, none written", dayOf(0), len(tried), w.headed(0))
	}
	if warned := warnedOf(log, cantBuildHead, dayOf(0)); warned != 1 {
		t.Errorf("log reads\n%s\nwant %s warned of once, not %d times", log, dayOf(0), warned)
	}
	w.file(0, askedBy("8", "one", on(0, 12, 0)))
	w.summarise()
	w.digestEnded()
	if warned := warnedOf(log, cantBuildHead, dayOf(0)); warned != 2 {
		t.Errorf("log reads\n%s\nwant %s warned of again once it changed, twice in all, not %d times", log, dayOf(0), warned)
	}
	w.fold.buildingSides(nil)
	w.digestEnded()
	if !w.headed(0) || warnedOf(log, cantBuildHead, dayOf(0)) != 2 {
		t.Errorf("once it can be, %s's head is written %t, warned of %d times; want it written, warned of no more", dayOf(0), w.headed(0), warnedOf(log, cantBuildHead, dayOf(0)))
	}
}

// A day whose files, or those of a day beside it, can't all be read for now
// has no head built, as one of the lines read would want lines, and that's
// warned of once; once they can be read, it's built.
func TestADayWhoseLinesCantAllBeReadHasNoHeadBuilt(t *testing.T) {
	log := logstest.Capture(t)
	w := newLookupWorld(t)
	sharedDays(w)
	path := filepath.Join(w.dir, "requests-"+dayOf(-2)+".jsonl")
	unopenable(t, w.dir, filepath.Base(path))

	w.digestEnded()
	w.digestEnded()

	if w.headed(-3) || w.headed(-2) || !w.headed(0) {
		t.Errorf("with %s's file unreadable, heads are held of %s %t, %s %t, %s %t; want none of it nor the day before, the day after none of its lines",
			dayOf(-2), dayOf(-3), w.headed(-3), dayOf(-2), w.headed(-2), dayOf(0), w.headed(0))
	}
	if warned := warnedOf(log, cantBuildHead, dayOf(-2)); warned != 1 {
		t.Errorf("log reads\n%s\nwant %s warned of once over two passes, not %d times", log, dayOf(-2), warned)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	w.digestEnded()
	if !w.headed(-3) || !bytes.Equal(w.dayHead(-2).Side, sideOf(t, "3")) {
		t.Errorf("once %s's file can be read, heads are held of %s %t, %s %t; want both", dayOf(-2), dayOf(-3), w.headed(-3), dayOf(-2), w.headed(-2))
	}
}

// A day a line is filed under as its head is built has no head written of
// it, and is built at the next pass once it's summarised again, the line
// among it.
func TestADayGainingALineAsItsHeadIsBuiltHasNoneWritten(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	removeHeads(t, w.dir)
	added := false
	w.fold.buildingSides(func(requests []string) error {
		if slices.Contains(requests, "4") && !added {
			added = true
			w.file(0, askedBy("added", "two", on(0, 11, 0)))
		}
		return nil
	})

	w.digestEnded()

	if w.headed(0) {
		t.Errorf("a head of %s was written, as a line was filed under it while it was built; want none", dayOf(0))
	}
	w.summarise()
	w.digestEnded()
	if head := w.dayHead(0); !bytes.Equal(head.Side, sideOf(t, "4", "5", "added")) {
		t.Errorf("the head of %s is %+v, want its side of 4, 5 and the line added", dayOf(0), head)
	}
}

// A day's head that doesn't stand as the digest of a day beside it tells of
// a line of it, filed there since it was built, is built again, of the line,
// once that digest is written again: then not at every pass after, as
// building it again can't have it stand; and once by a ledger opened afresh,
// which can't tell what it was built of.
func TestAHeadNotAloneIsBuiltAgainOnceItsNeighboursDigestChanges(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	w.fold.sides()
	w.file(1, askedBy("x", "four", on(0, 23, 0)))
	w.summarise()

	w.digestEnded()

	if built, want := requestsOfSides(w.fold.sides()), [][]string{{"6", "7"}, {"4", "5", "x"}}; !reflect.DeepEqual(built, want) {
		t.Errorf("once %s is digested again, sides of %q were built, want %q: its own, as its summary changed, and %s's, of the line", dayOf(1), built, want, dayOf(0))
	}
	if side, ok := w.follower().DaySide(dayOf(0), sideBody); ok {
		t.Errorf("DaySide(%s) = %s, read, though the digest of %s tells of a line of it; want it unread", dayOf(0), side, dayOf(1))
	}
	w.digestEnded()
	if built := requestsOfSides(w.fold.sides()); len(built) > 0 {
		t.Errorf("a pass after built sides of %q again, want none, nothing changed since", built)
	}
	fresh := &folding{}
	w.opened(fresh).DigestEnded(t.Context(), w.at)
	if built, want := requestsOfSides(fresh.sides()), [][]string{{"4", "5", "x"}}; !reflect.DeepEqual(built, want) {
		t.Errorf("a ledger opened afresh built sides of %q, want %q: the one head that doesn't stand", built, want)
	}
}

// The first pass of a ledger opened afresh judges each day's head without
// reading its side: one whose side is damaged, the fields before it as they
// were written, isn't built again, though a follower reads it not to stand.
func TestTheFirstPassAfterAStartReadsNoSide(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	head := w.dayHead(0)
	fields, _, _ := strings.Cut(string(gunzippedFile(t, logdayPath(w.dir, 0))), `"side":`)
	writeFile(t, w.dir, filepath.Base(logdayPath(w.dir, 0)), gzipped(t, fields+`"side":not a side`))

	fresh := &folding{}
	w.opened(fresh).DigestEnded(t.Context(), w.at)

	if built := requestsOfSides(fresh.sides()); len(built) > 0 {
		t.Errorf("the first pass of a ledger opened afresh built sides of %q, want none: no side read", built)
	}
	if side, ok := w.follower().DaySide(dayOf(0), sideBody); ok {
		t.Errorf("DaySide(%s) = %s, read, of a side damaged after %+v; want it unread", dayOf(0), side, head)
	}
}

// The digesting holds each day's head it builds, or reads afresh, from pass
// to pass, of no side, nor its rows' chunks, which it never judges by: its
// rows file's size kept, which it does. So it holds no day's side, nor the
// meta of its chunks, from pass to pass.
func TestTheDigestingHoldsItsHeadsWithoutTheirSidesOrChunks(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	fresh := w.opened(&folding{})

	fresh.DigestEnded(t.Context(), w.at)

	for _, held := range []struct {
		name   string
		ledger *ledger.Ledger
	}{{name: "as built", ledger: w.writer}, {name: "as read afresh", ledger: fresh}} {
		heads := held.ledger.HeadsHeld()
		if len(heads) != 4 {
			t.Errorf("%s, the digesting holds heads of %q; want those of the 4 days of lines", held.name, slices.Sorted(maps.Keys(heads)))
		}
		for date, head := range heads {
			written, err := ledger.ReadDayHead(w.dir, date, sideBody)
			if err != nil {
				t.Fatal(err)
			}
			if head.Side != nil || head.Rows.Chunks != nil || written.Rows.Size == 0 || head.Rows.Size != written.Rows.Size {
				t.Errorf("%s, the head of %s is held as %+v; want it of no side, nor chunks, its rows file's size, %d, kept", held.name, date, head, written.Rows.Size)
			}
		}
	}
}

// A pass that finds nothing changed since the last reads no head: with every
// head unreadable, none is built again, nor warned of. A ledger opened
// afresh, holding nothing of the last, reads them, and builds them again.
func TestAPassFindingNothingChangedReadsNoHead(t *testing.T) {
	log := logstest.Capture(t)
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	w.fold.sides()
	unopenable(t, w.dir, "logday-*")

	w.digestEnded()

	if built := requestsOfSides(w.fold.sides()); len(built) > 0 {
		t.Errorf("a pass finding nothing changed built sides of %q again, want none", built)
	}
	if log.Has("level=WARN") {
		t.Errorf("log reads\n%s\nwant nothing warned of", log)
	}
	fresh := &folding{}
	w.opened(fresh).DigestEnded(t.Context(), w.at)
	if built := len(fresh.sides()); built != 4 {
		t.Errorf("a ledger opened afresh built %d sides, want 4, every head unreadable", built)
	}
}

// Building the days' heads as they change, pass by pass, leaves the heads and
// rows files a ledger opened afresh builds of them, from none, byte for byte:
// each day's head judged again where a change may have made it due, and none
// passed over.
func TestBuildingHeadsAsDaysChangeWritesWhatALedgerAfreshDoes(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, w *lookupWorld)
	}{
		{name: "a late line under a day of lines", change: func(_ *testing.T, w *lookupWorld) {
			w.file(-2, askedBy("late", "two", on(-2, 12, 0)))
			w.summarise()
		}},
		{name: "a late line under a day of none", change: func(_ *testing.T, w *lookupWorld) {
			w.file(-1, askedBy("late", "two", on(-1, 12, 0)))
			w.summarise()
		}},
		{name: "a late line under the day after the one it arrived on", change: func(_ *testing.T, w *lookupWorld) {
			w.file(1, askedBy("late", "four", on(0, 23, 0)))
			w.summarise()
		}},
		{name: "a late line under the day before the one it arrived on", change: func(_ *testing.T, w *lookupWorld) {
			w.file(-1, askedBy("late", "four", on(0, 0, 30)))
			w.summarise()
		}},
		{name: "the first day pruned", change: func(t *testing.T, w *lookupWorld) {
			removeFile(t, w.dir, "requests-"+dayOf(-3)+".jsonl")
			removeFile(t, w.dir, filepath.Base(summaryFile(w.dir, dayOf(-3))))
		}},
		{name: "a head deleted by hand", change: func(t *testing.T, w *lookupWorld) {
			removeFile(t, w.dir, filepath.Base(logdayPath(w.dir, 0)))
		}},
		{name: "a head damaged", change: func(t *testing.T, w *lookupWorld) {
			writeFile(t, w.dir, filepath.Base(logdayPath(w.dir, 0)), []byte("not a head"))
		}},
		{name: "a head of another version of its side", change: func(t *testing.T, w *lookupWorld) {
			head := w.dayHead(0)
			head.Body = sideBody + 1
			if err := ledger.WriteDayHead(w.dir, head); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "a head built against another summary", change: func(t *testing.T, w *lookupWorld) {
			head := w.dayHead(0)
			head.SummaryLines++
			if err := ledger.WriteDayHead(w.dir, head); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "a head pointing at rows of another size", change: func(t *testing.T, w *lookupWorld) {
			head := w.dayHead(0)
			head.Rows.Size++
			if err := ledger.WriteDayHead(w.dir, head); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "a rows file deleted by hand", change: func(t *testing.T, w *lookupWorld) {
			removeFile(t, w.dir, filepath.Base(rowsPath(w.dir, 0)))
		}},
		{name: "a rows file written again by a build that stopped before its head", change: func(t *testing.T, w *lookupWorld) {
			if err := ledger.WriteRows(w.dir, dayOf(0), chunksOf("4", "5", "late")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "a stray rows file of a day of lines but no head", change: func(t *testing.T, w *lookupWorld) {
			removeFile(t, w.dir, filepath.Base(logdayPath(w.dir, 0)))
			if err := ledger.WriteRows(w.dir, dayOf(0), chunksOf("stray")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "every day compressed and summarised again", change: func(t *testing.T, w *lookupWorld) {
			compressAll(t, w.dir)
			w.summarise()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newLookupWorld(t)
			sharedDays(w)
			w.digestEnded()
			tt.change(t, w)

			w.digestEnded()
			w.digestEnded()

			built, rows := headsIn(t, w.dir), filesMatching(t, w.dir, "rows-*")
			removeHeads(t, w.dir)
			w.opened(&folding{}).DigestEnded(t.Context(), w.at)
			if afresh := headsIn(t, w.dir); !reflect.DeepEqual(built, afresh) {
				t.Errorf("built pass by pass, the heads are\n%+v\nwant\n%+v\nas a ledger opened afresh builds them", built, afresh)
			}
			if afresh := filesMatching(t, w.dir, "rows-*"); !reflect.DeepEqual(rows, afresh) {
				t.Errorf("built pass by pass, the rows files are\n%x\nwant\n%x\nas a ledger opened afresh builds them", rows, afresh)
			}
		})
	}
}
