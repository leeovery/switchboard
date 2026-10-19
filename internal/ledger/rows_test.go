package ledger_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// rowsPath is the path of the rows file of the local day days after the
// summaries' day the ledger in dir holds.
func rowsPath(dir string, days int) string {
	return filepath.Join(dir, "rows-"+dayOf(days)+".gz")
}

// rowed reports whether the world's ledger holds a rows file of the local
// day days after the summaries' day.
func (w *lookupWorld) rowed(days int) bool {
	w.t.Helper()
	_, err := os.Stat(rowsPath(w.dir, days))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		w.t.Fatal(err)
	}
	return err == nil
}

// chunkRead is a chunk of a day's rows as the tests read it: its rows, as
// their builder gave them, how many they are, and its meta.
type chunkRead struct {
	rows  string
	count int
	meta  string
}

// builtRead is chunks, as their builder gives them, as chunkRead has each:
// none of none.
func builtRead(chunks []ledger.BuiltChunk) []chunkRead {
	var read []chunkRead
	for _, c := range chunks {
		read = append(read, chunkRead{rows: string(c.Rows), count: c.Count, meta: string(c.Meta)})
	}
	return read
}

// rowsRead returns each chunk of rows, the rows of the local day with the
// given date, as f reads it, as RowChunk reads each, as chunkRead has it:
// none of none, failing the test where one doesn't read.
func rowsRead(t *testing.T, f *ledger.Follower, date string, rows ledger.DayRows) []chunkRead {
	t.Helper()
	var read []chunkRead
	for i, c := range rows.Chunks {
		data, err := f.RowChunk(date, c)
		if err != nil {
			t.Fatalf("chunk %d of the rows of %s: %v", i, date, err)
		}
		read = append(read, chunkRead{rows: string(data), count: c.Count, meta: string(c.Meta)})
	}
	return read
}

// dayRows returns the rows f gives of the local day with the given date, as
// DayRows gives them, of the sides and rows the tests' Digester gives, each
// chunk read as rowsRead reads it: reporting false where DayRows gives none.
func dayRows(t *testing.T, f *ledger.Follower, date string) ([]chunkRead, bool) {
	t.Helper()
	rows, ok := f.DayRows(date, sideBody)
	if !ok {
		return nil, false
	}
	return rowsRead(t, f, date, rows), true
}

// A finished day's rows are written beside its head, of the lines its day
// gives, in its order, in the chunks their builder cut them into: each a
// gzip member of its own, one after another, so the file reads whole as
// their rows; the file the user's alone, the head recording its size, and
// where each chunk lies in it, how many rows it holds, and its meta, as
// built. A follower reads each chunk as it was built.
func TestADaysRowsAreWrittenBesideItsHeadAChunkAMember(t *testing.T) {
	w := newLookupWorld(t)
	w.file(0, askedBy("3", "one", on(0, 11, 0)), askedBy("1", "one", on(0, 9, 0)), askedBy("2", "two", on(0, 10, 0)),
		askedBy("4", "two", on(0, 12, 0)), askedBy("5", "one", on(0, 13, 0)))
	w.summarise()

	w.digestEnded()

	head := w.dayHead(0)
	file := fileInfo(t, rowsPath(w.dir, 0))
	if head.Rows.Size != file.Size() || file.Mode().Perm() != 0o600 {
		t.Errorf("the rows file of %s is %d bytes, %v; want it as long as its head records, %d, the user's alone, 0600", dayOf(0), file.Size(), file.Mode().Perm(), head.Rows.Size)
	}
	var at int64
	for i, c := range head.Rows.Chunks {
		if c.Offset != at || c.Length <= 0 {
			t.Errorf("chunk %d lies %d bytes at %d; want it where the one before it ends, %d", i, c.Length, c.Offset, at)
		}
		at = c.Offset + c.Length
	}
	if at != head.Rows.Size {
		t.Errorf("the chunks end at %d of a rows file of %d bytes; want them to fill it", at, head.Rows.Size)
	}
	if got, want := string(gunzippedFile(t, rowsPath(w.dir, 0))), "1\n2\n3\n4\n5\n"; got != want {
		t.Errorf("the rows file of %s reads whole as %q, want %q, its chunks' rows one after another", dayOf(0), got, want)
	}
	if got, ok := dayRows(t, w.follower(), dayOf(0)); !ok || !reflect.DeepEqual(got, builtRead(chunksOf("1", "2", "3", "4", "5"))) {
		t.Errorf("DayRows(%s) = %+v, %t; want %+v, as built, read", dayOf(0), got, ok, builtRead(chunksOf("1", "2", "3", "4", "5")))
	}
}

// A day whose rows file isn't as long as its head records, as one gone, or
// written again by a build that stopped before its head, of other lines, has
// its rows read as none, but its side as it was built, as the side stands
// with the head alone; and its head and rows are built again at the next
// pass, its rows then read as they were first written, and the pass after
// builds nothing.
func TestADaysRowsNotAsItsHeadRecordsAreBuiltAgain(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, w *lookupWorld)
	}{
		{name: "its rows file gone", change: func(t *testing.T, w *lookupWorld) {
			removeFile(t, w.dir, filepath.Base(rowsPath(w.dir, 0)))
		}},
		{name: "its rows file written again, of other rows, by a build that stopped before its head", change: func(t *testing.T, w *lookupWorld) {
			if err := ledger.WriteRows(w.dir, dayOf(0), chunksOf("early", "4", "5")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newLookupWorld(t)
			sharedDays(w)
			w.digestEnded()
			w.fold.sides()
			tt.change(t, w)

			f := w.follower()
			if side, ok := f.DaySide(dayOf(0), sideBody); !ok || !bytes.Equal(side, sideOf(t, "4", "5")) {
				t.Errorf("DaySide(%s) = %s, %t; want it read as built, its rows aside", dayOf(0), side, ok)
			}
			if rows, ok := f.DayRows(dayOf(0), sideBody); ok {
				t.Errorf("DayRows(%s) = %+v, read; want them unread", dayOf(0), rows)
			}
			w.digestEnded()

			if built, want := requestsOfSides(w.fold.sides()), [][]string{{"4", "5"}}; !reflect.DeepEqual(built, want) {
				t.Fatalf("the pass built sides of %q, want %q: its own alone", built, want)
			}
			if rows, ok := dayRows(t, f, dayOf(0)); !ok || !reflect.DeepEqual(rows, builtRead(chunksOf("4", "5"))) {
				t.Errorf("DayRows(%s) = %+v, %t; want them read as first written", dayOf(0), rows, ok)
			}
			w.digestEnded()
			if built := requestsOfSides(w.fold.sides()); len(built) > 0 {
				t.Errorf("the pass after built sides of %q again, want none", built)
			}
		})
	}
}

// A chunk damaged in its day's rows file, the file as long, as by a byte
// flipped, or two chunks of one length swapped, each still a gzip member
// whole, doesn't read, by its CRC, while the day's other chunks, and its
// side, do. Its day isn't built again, by the digesting, or by a ledger
// opened afresh, as the rows file is looked at, never read: its reader reads
// the day's lines instead, until it's built again for another reason.
func TestAChunkDamagedDoesntReadButLeavesItsDayNotDue(t *testing.T) {
	requests := []string{"a1", "a2", "a3", "a4", "a5"}
	tests := []struct {
		name   string
		damage func(t *testing.T, path string, rows ledger.DayRows)
		// damaged are the chunks the damage leaves unreadable.
		damaged map[int]bool
	}{
		{name: "a byte of it flipped", damage: func(t *testing.T, path string, rows ledger.DayRows) {
			c := rows.Chunks[1]
			damageAt(t, path, c.Offset+c.Length/2)
		}, damaged: map[int]bool{1: true}},
		{name: "two of one length swapped", damage: func(t *testing.T, path string, rows ledger.DayRows) {
			first, second := rows.Chunks[0], rows.Chunks[1]
			if first.Length != second.Length {
				t.Fatalf("the first two chunks are %d and %d bytes; want them of one length, to be swapped", first.Length, second.Length)
			}
			file, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			swapped := bytes.Clone(file)
			copy(swapped[first.Offset:], file[second.Offset:second.Offset+second.Length])
			copy(swapped[second.Offset:], file[first.Offset:first.Offset+first.Length])
			if err := os.WriteFile(path, swapped, 0o600); err != nil {
				t.Fatal(err)
			}
		}, damaged: map[int]bool{0: true, 1: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newLookupWorld(t)
			var lines []ledger.Line
			for i, request := range requests {
				lines = append(lines, askedBy(request, "one", on(0, 9+i, 0)))
			}
			w.file(0, lines...)
			w.summarise()
			w.digestEnded()
			w.fold.sides()
			rows := w.dayHead(0).Rows
			tt.damage(t, rowsPath(w.dir, 0), rows)

			f := w.follower()
			given, ok := f.DayRows(dayOf(0), sideBody)
			if !ok {
				t.Fatalf("DayRows(%s) is unread; want it read, its rows file as long as its head records", dayOf(0))
			}
			want := builtRead(chunksOf(requests...))
			for i, c := range given.Chunks {
				data, err := f.RowChunk(dayOf(0), c)
				switch {
				case tt.damaged[i] && (err == nil || errors.Is(err, fs.ErrNotExist)):
					t.Errorf("chunk %d, damaged, reads %q, %v; want it failing as damaged", i, data, err)
				case !tt.damaged[i] && (err != nil || string(data) != want[i].rows):
					t.Errorf("chunk %d reads %q, %v; want %q, as built", i, data, err, want[i].rows)
				}
			}
			if side, ok := f.DaySide(dayOf(0), sideBody); !ok || !bytes.Equal(side, sideOf(t, requests...)) {
				t.Errorf("DaySide(%s) = %s, %t; want it read as built", dayOf(0), side, ok)
			}
			w.digestEnded()
			fresh := &folding{}
			w.opened(fresh).DigestEnded(t.Context(), w.at)
			if built := append(requestsOfSides(w.fold.sides()), requestsOfSides(fresh.sides())...); len(built) > 0 {
				t.Errorf("with a chunk damaged, sides of %q were built again; want none, the rows file as long as its head records", built)
			}
		})
	}
}

// A day whose rows can't be written has no head written either, as a head
// is written only once the rows it points at are: that's warned of once,
// and both are written at a later pass once they can be.
func TestAHeadIsWrittenOnlyOnceItsRowsAre(t *testing.T) {
	log := logstest.Capture(t)
	w := newLookupWorld(t)
	sharedDays(w)
	// The rows can't be written over a directory standing at their path.
	blocked := rowsPath(w.dir, 0)
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}

	w.digestEnded()
	w.digestEnded()

	if w.headed(0) {
		t.Errorf("a head of %s was written, its rows not; want none, as a head is written only once its rows are", dayOf(0))
	}
	if warned := warnedOf(log, cantBuildHead, dayOf(0)); warned != 1 {
		t.Errorf("log reads\n%s\nwant %s warned of once over two passes, not %d times", log, dayOf(0), warned)
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	w.digestEnded()
	if rows, ok := dayRows(t, w.follower(), dayOf(0)); !ok || !reflect.DeepEqual(rows, builtRead(chunksOf("4", "5"))) {
		t.Errorf("once they can be written, DayRows(%s) = %+v, %t; want them of 4 and 5, read", dayOf(0), rows, ok)
	}
}

// A build that stops between writing a day's rows and its head, as one whose
// head can't be written, leaves the rows file with no head pointing at it,
// so neither the day's side nor its rows are read; once its head can be
// written, the next pass writes the rows again, as they were, and the head
// pointing at them.
func TestABuildStoppedBeforeItsHeadLeavesItsRowsPointedAtByNone(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	// A head can't be written over a directory standing at its path.
	blocked := logdayPath(w.dir, 0)
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}

	w.digestEnded()

	left := filesMatching(t, w.dir, filepath.Base(rowsPath(w.dir, 0)))
	if len(left) != 1 {
		t.Fatalf("the build left %d rows files of %s; want one, its rows written before its head", len(left), dayOf(0))
	}
	f := w.follower()
	if _, ok := f.DaySide(dayOf(0), sideBody); ok {
		t.Errorf("DaySide(%s) is read, its head unwritten; want it unread", dayOf(0))
	}
	if rows, ok := f.DayRows(dayOf(0), sideBody); ok {
		t.Errorf("DayRows(%s) = %+v, read, its head unwritten; want them unread", dayOf(0), rows)
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	w.digestEnded()
	if again := filesMatching(t, w.dir, filepath.Base(rowsPath(w.dir, 0))); !reflect.DeepEqual(again, left) {
		t.Errorf("the rows file of %s written again is\n%x\nwant\n%x\nas the build that stopped left it", dayOf(0), again, left)
	}
	if rows, ok := dayRows(t, f, dayOf(0)); !ok || !reflect.DeepEqual(rows, builtRead(chunksOf("4", "5"))) {
		t.Errorf("DayRows(%s) = %+v, %t; want them of 4 and 5, read", dayOf(0), rows, ok)
	}
}

// A day's rows built again of the same lines are the same file, byte for
// byte, and its head the same head: each chunk lies where it lay.
func TestADaysRowsBuiltAgainAreTheSameFile(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.file(1, askedBy("8", "one", on(1, 11, 0)), askedBy("9", "three", on(1, 12, 0)))
	w.summarise()
	w.digestEnded()
	rows, heads := filesMatching(t, w.dir, "rows-*"), headsIn(t, w.dir)
	removeHeads(t, w.dir)

	w.opened(&folding{}).DigestEnded(t.Context(), w.at)

	if again := filesMatching(t, w.dir, "rows-*"); len(rows) != 4 || !reflect.DeepEqual(again, rows) {
		t.Errorf("built again, the rows files are\n%x\nwant\n%x\nas first built, one a day of lines", again, rows)
	}
	if again := headsIn(t, w.dir); !reflect.DeepEqual(again, heads) {
		t.Errorf("built again, the heads are\n%+v\nwant\n%+v\nas first built", again, heads)
	}
}

// A day whose Digester builds no rows of it has no rows file, any held
// before removed, and its head points at none: its rows read as none,
// standing.
func TestADayBuiltOfNoRowsHasNoRowsFile(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	removeFile(t, w.dir, filepath.Base(logdayPath(w.dir, 0)))

	w.opened(rowless{&folding{}}).DigestEnded(t.Context(), w.at)

	if head := w.dayHead(0); !reflect.DeepEqual(head.Rows, ledger.DayRows{}) {
		t.Errorf("the head of %s points at rows %+v; want none", dayOf(0), head.Rows)
	}
	if w.rowed(0) {
		t.Errorf("the ledger holds a rows file of %s, built of none; want it removed", dayOf(0))
	}
	if rows, ok := w.follower().DayRows(dayOf(0), sideBody); !ok || len(rows.Chunks) > 0 {
		t.Errorf("DayRows(%s) = %+v, %t; want none, read", dayOf(0), rows, ok)
	}
}

// rowless is a Digester giving each day's side as the one it wraps gives it,
// and no rows.
type rowless struct {
	ledger.Digester
}

// Day gives a builder of a day's side as the wrapped Digester gives one, of
// no rows, as rowlessDay has it.
func (r rowless) Day() ledger.DayBuilder {
	return rowlessDay{r.Digester.Day()}
}

// rowlessDay is a builder of a day's side as the one it wraps builds it, of
// no rows.
type rowlessDay struct {
	ledger.DayBuilder
}

// Built gives the side the wrapped builder gives, and no rows.
func (r rowlessDay) Built() (ledger.DayBuilt, error) {
	built, err := r.DayBuilder.Built()
	built.Chunks = nil
	return built, err
}
