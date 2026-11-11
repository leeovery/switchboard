package dayfile

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

// rewriteDate is the day the rewrite tests' files are of.
const rewriteDate = "2026-11-02"

// toNew rewrites each "old" in a line to "new", as a move of ids does.
func toNew(line []byte) []byte {
	return bytes.ReplaceAll(line, []byte("old"), []byte("new"))
}

func TestRewritingAPlainFileRewritesEachLineInPlace(t *testing.T) {
	f := requestLedger(t.TempDir())
	put(t, f.path(plainFile(rewriteDate)), []byte(linesOf(`{"a":"old"}`, `{"a":"other"}`, `{"a":"old","b":"old"}`)))

	r, err := f.Rewrite(rewriteDate, toNew)
	if err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	want := linesOf(`{"a":"new"}`, `{"a":"other"}`, `{"a":"new","b":"new"}`)
	if got := heldIn(t, f, plainFile(rewriteDate)); got != want {
		t.Errorf("the plain file holds %q, want %q", got, want)
	}
	if !r.Changed || r.Before.PlainSize != int64(len(linesOf(`{"a":"old"}`, `{"a":"other"}`, `{"a":"old","b":"old"}`))) || r.After.PlainSize != int64(len(want)) {
		t.Errorf("Rewrite() = %+v, want it changed, its sizes before and after", r)
	}
}

func TestRewritingACompressedFileWritesOneMemberOfTheSameLines(t *testing.T) {
	f := requestLedger(t.TempDir())
	put(t, f.path(compressedFile(rewriteDate)), gzipOf(t, linesOf(`{"a":"old"}`), linesOf(`{"a":"other"}`), `{"cut":`))

	if _, err := f.Rewrite(rewriteDate, toNew); err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	if got, want := heldIn(t, f, compressedFile(rewriteDate)), linesOf(`{"a":"new"}`, `{"a":"other"}`)+`{"cut":`; got != want {
		t.Errorf("the compressed file gives %q, want %q", got, want)
	}
	if n := membersIn(t, written(t, f, compressedFile(rewriteDate))); n != 1 {
		t.Errorf("it's written in %d members, want 1", n)
	}
}

func TestRewritingADayWhoseLinesDontChangeLeavesItsFilesAsTheyAre(t *testing.T) {
	f := requestLedger(t.TempDir())
	put(t, f.path(plainFile(rewriteDate)), []byte(linesOf(`{"a":"new"}`)))
	was := statOf(t, f.path(plainFile(rewriteDate)))

	r, err := f.Rewrite(rewriteDate, toNew)
	if err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	if r.Changed || r.Before != r.After {
		t.Errorf("Rewrite() = %+v, want nothing changed", r)
	}
	if now := statOf(t, f.path(plainFile(rewriteDate))); !os.SameFile(was, now) {
		t.Error("the plain file was written again, want it left as it was")
	}
	if leftovers(t, f) != 0 {
		t.Error("a file written beside it was left behind")
	}
}

func TestADamagedCompressedFileIsLeftAsItIs(t *testing.T) {
	whole := gzipOf(t, linesOf(`{"a":"old"}`, `{"a":"old","b":"old"}`))
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"cut short", whole[:len(whole)-6]},
		{"not gzip at all", []byte("not gzip, nor ever was\n")},
		{"empty", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())
			put(t, f.path(compressedFile(rewriteDate)), tc.data)
			was := statOf(t, f.path(compressedFile(rewriteDate)))

			r, err := f.Rewrite(rewriteDate, toNew)
			if err != nil || r != (Rewritten{Damaged: true}) {
				t.Errorf("Rewrite() = %+v, %v, want it left as damaged, and no failure", r, err)
			}
			if now := statOf(t, f.path(compressedFile(rewriteDate))); !os.SameFile(was, now) || now.Size() != was.Size() {
				t.Error("the damaged file was written again, want it left as it was")
			}
			if leftovers(t, f) != 0 {
				t.Error("a file written beside it was left behind")
			}
		})
	}
}

func TestADamagedCompressedFilesPlainFileIsRewrittenAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"cut short", func() []byte { whole := gzipOf(t, linesOf(`{"a":"old"}`)); return whole[:len(whole)-6] }()},
		{"not gzip at all", []byte("not gzip, nor ever was\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())
			put(t, f.path(compressedFile(rewriteDate)), tc.data)
			put(t, f.path(plainFile(rewriteDate)), []byte(linesOf(`{"a":"old","late":1}`)))

			r, err := f.Rewrite(rewriteDate, toNew)
			if err != nil || !r.Changed {
				t.Fatalf("Rewrite() = %+v, %v, want the plain file rewritten", r, err)
			}
			if got, want := heldIn(t, f, plainFile(rewriteDate)), linesOf(`{"a":"new","late":1}`); got != want {
				t.Errorf("the plain file holds %q, want %q", got, want)
			}
			if got := written(t, f, compressedFile(rewriteDate)); !bytes.Equal(got, tc.data) {
				t.Errorf("the damaged file holds %q, want it left as it was, %q", got, tc.data)
			}
		})
	}
}

func TestReadingADayWhoseCompressedFileIsntGzipFailsAsDamaged(t *testing.T) {
	f := requestLedger(t.TempDir())
	put(t, f.path(compressedFile(rewriteDate)), []byte("not gzip, nor ever was\n"))
	_, _, err := ReadDay(f, rewriteDate, func(line []byte) ([]byte, bool) { return line, true }, func([]byte) bool { return true })
	if !errors.Is(err, ErrDamaged) || !strings.Contains(err.Error(), "gzip: invalid header") {
		t.Errorf("ReadDay() = %v, want it failing as damaged, saying why as gzip does", err)
	}
}

func TestRewritingADayOfNoFilesDoesNothing(t *testing.T) {
	f := requestLedger(t.TempDir())
	if r, err := f.Rewrite(rewriteDate, toNew); err != nil || r != (Rewritten{}) {
		t.Errorf("Rewrite() = %+v, %v, want nothing, and no failure", r, err)
	}
}

func TestLinesAppendedWhileADayIsRewrittenAreRewrittenAndKept(t *testing.T) {
	for _, tc := range []struct {
		name, appended, want string
	}{
		{"under the old id", linesOf(`{"a":"old","late":1}`), linesOf(`{"a":"new","late":1}`)},
		{"under the new id", linesOf(`{"a":"new","late":2}`), linesOf(`{"a":"new","late":2}`)},
		{"cut short", `{"a":"old","cut`, `{"a":"new","cut`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())
			put(t, f.path(plainFile(rewriteDate)), []byte(linesOf(`{"a":"old"}`)))
			f.rewriting = func(dayFile) {
				f.rewriting = nil
				if err := appendLines(f.path(plainFile(rewriteDate)), []byte(tc.appended)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.Rewrite(rewriteDate, toNew); err != nil {
				t.Fatalf("Rewrite() = %v", err)
			}
			if got, want := heldIn(t, f, plainFile(rewriteDate)), linesOf(`{"a":"new"}`)+tc.want; got != want {
				t.Errorf("the plain file holds %q, want %q", got, want)
			}
		})
	}
}

func TestALineHalfWrittenAsADayIsReadIsLeftForTheHold(t *testing.T) {
	f := requestLedger(t.TempDir())
	put(t, f.path(plainFile(rewriteDate)), []byte(linesOf(`{"a":"old"}`)+`{"a":"ol`))
	f.rewriting = func(dayFile) {
		f.rewriting = nil
		finish(t, f.path(plainFile(rewriteDate)), linesOf(`d"}`))
	}
	if _, err := f.Rewrite(rewriteDate, toNew); err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	if got, want := heldIn(t, f, plainFile(rewriteDate)), linesOf(`{"a":"new"}`, `{"a":"new"}`); got != want {
		t.Errorf("the plain file holds %q, want the line finished meanwhile rewritten whole, %q", got, want)
	}
}

func TestADayCompressedWhileItsRewrittenIsRewrittenAfresh(t *testing.T) {
	f := requestLedger(t.TempDir())
	put(t, f.path(plainFile(rewriteDate)), []byte(linesOf(`{"a":"old"}`)))
	f.rewriting = func(dayFile) {
		f.rewriting = nil
		if err := f.compress(rewriteDate); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Rewrite(rewriteDate, toNew); err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	if _, err := os.Stat(f.path(plainFile(rewriteDate))); err == nil {
		t.Error("the plain file the compress removed came back")
	}
	if got, want := heldIn(t, f, compressedFile(rewriteDate)), linesOf(`{"a":"new"}`); got != want {
		t.Errorf("the compressed file gives %q, want %q", got, want)
	}
}

func TestADayWithBothFilesIsCompressedThenRewritten(t *testing.T) {
	f := requestLedger(t.TempDir())
	put(t, f.path(compressedFile(rewriteDate)), gzipOf(t, linesOf(`{"a":"old","n":1}`)))
	put(t, f.path(plainFile(rewriteDate)), []byte(linesOf(`{"a":"old","n":1}`, `{"a":"old","n":2}`)))

	if _, err := f.Rewrite(rewriteDate, toNew); err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	if _, err := os.Stat(f.path(plainFile(rewriteDate))); err == nil {
		t.Error("the plain file is still there, want it compressed")
	}
	if got, want := heldIn(t, f, compressedFile(rewriteDate)), linesOf(`{"a":"new","n":1}`, `{"a":"new","n":2}`); got != want {
		t.Errorf("the compressed file gives %q, want each line once, rewritten, %q", got, want)
	}
}

func TestADayThatKeepsChangingFailsToBeRewritten(t *testing.T) {
	f := requestLedger(t.TempDir())
	put(t, f.path(plainFile(rewriteDate)), []byte(linesOf(`{"a":"old"}`)))
	f.rewriting = func(dayFile) {
		data := written(t, f, plainFile(rewriteDate))
		if err := os.Remove(f.path(plainFile(rewriteDate))); err != nil {
			t.Fatal(err)
		}
		put(t, f.path(plainFile(rewriteDate)), data)
	}
	if _, err := f.Rewrite(rewriteDate, toNew); err == nil || !strings.Contains(err.Error(), "kept changing") {
		t.Errorf("Rewrite() = %v, want it to fail as its file kept being replaced", err)
	}
	if got, want := heldIn(t, f, plainFile(rewriteDate)), linesOf(`{"a":"old"}`); got != want {
		t.Errorf("the plain file holds %q, want it left as it was, %q", got, want)
	}
}

// finish writes rest at the end of the file at path, as a write still going
// as the file was read finishes the line it was writing.
func finish(t *testing.T, path, rest string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(rest); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

// statOf looks at the file at path.
func statOf(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// leftovers counts the files of f's directory that aren't a day's.
func leftovers(t *testing.T, f *Files) int {
	t.Helper()
	entries, err := os.ReadDir(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if _, ok := f.DateOf(e.Name()); !ok {
			n++
		}
	}
	return n
}
