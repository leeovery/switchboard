package dayfile

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/switchboard/internal/atomicfile"
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

// gzipCutShort is a compressed file of the lines given, its last bytes lost, as
// a write cut short loses them: its lines read whole, then it fails.
func gzipCutShort(t *testing.T, lines string) []byte {
	whole := gzipOf(t, lines)
	return whole[:len(whole)-6]
}

// gzipFlushed is a gzip member of kept, then lost, flushed between, so kept
// inflates whatever comes after it; and where lost's compressed data lies in
// it, from from to to.
func gzipFlushed(t *testing.T, kept, lost string) (member []byte, from, to int) {
	t.Helper()
	var data bytes.Buffer
	w := gzip.NewWriter(&data)
	if _, err := io.WriteString(w, kept); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	from = data.Len()
	if _, err := io.WriteString(w, lost); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes(), from, data.Len() - 8
}

// gzipCutInside is a compressed file of kept, then lost, cut short inside
// lost's compressed data, as a write cut short there leaves it: kept reads
// whole, then it fails.
func gzipCutInside(t *testing.T, kept, lost string) []byte {
	member, from, to := gzipFlushed(t, kept, lost)
	return member[:(from+to)/2]
}

// gzipBrokenAt is a gzip member of kept, then lost, whose compressed data is
// broken where lost's starts, as a disk's fault breaks it: kept reads whole,
// then it fails, lost and anything after it lost.
func gzipBrokenAt(t *testing.T, kept, lost string) []byte {
	member, from, _ := gzipFlushed(t, kept, lost)
	// A block of the type no block has.
	member[from] = 0xff
	return member
}

// lostLine is a long line, which damage inside its compressed data loses all
// but a start of, no whole JSON.
var lostLine = linesOf(`{"a":"old","lost":"` + strings.Repeat("0123456789abcdef", 64) + `"}`)

// staged returns the files staged beside the file at path and left there.
func staged(t *testing.T, path string) []string {
	t.Helper()
	found, err := atomicfile.Leftovers(path)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// linkFailing links no file, failing as linking does, as why says.
func linkFailing(why syscall.Errno) func(oldname, newname string) error {
	return func(oldname, newname string) error {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: why}
	}
}

func TestADamagedCompressedFileIsRewrittenUpToItsDamageAndSetAside(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"cut short", gzipCutShort(t, linesOf(`{"a":"old"}`, `{"a":"old","b":"old"}`)+`{"a":"old","cut`), linesOf(`{"a":"new"}`, `{"a":"new","b":"new"}`)},
		{"cut short after a last line without its ending, whole JSON", gzipCutShort(t, linesOf(`{"a":"old"}`)+`{"a":"old","b":1}`), linesOf(`{"a":"new"}`, `{"a":"new","b":1}`)},
		{"cut short inside its compressed data", gzipCutInside(t, linesOf(`{"a":"old"}`, `{"a":"old","b":1}`), lostLine+linesOf(`{"a":"old","c":1}`)), linesOf(`{"a":"new"}`, `{"a":"new","b":1}`)},
		{"broken in a member before its last", slices.Concat(gzipOf(t, linesOf(`{"a":"old"}`)), gzipBrokenAt(t, linesOf(`{"a":"old","b":1}`), lostLine), gzipOf(t, linesOf(`{"a":"old","c":1}`))), linesOf(`{"a":"new"}`, `{"a":"new","b":1}`)},
		{"not gzip at all", []byte("not gzip, nor ever was\n"), ""},
		{"empty", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())
			path := f.path(compressedFile(rewriteDate))
			put(t, path, tc.data)

			if _, err := f.Rewrite(rewriteDate, toNew); err != nil {
				t.Fatalf("Rewrite() = %v", err)
			}
			if got := heldIn(t, f, compressedFile(rewriteDate)); got != tc.want {
				t.Errorf("the compressed file gives %q, want the lines before the damage, rewritten, %q", got, tc.want)
			}
			aside, err := os.ReadFile(path + ".damaged")
			if err != nil || !bytes.Equal(aside, tc.data) {
				t.Errorf("the damaged file set aside holds %q, %v, want it as it was, %q", aside, err, tc.data)
			}
			if dates, err := f.All(); err != nil || !slices.Equal(dates, []string{rewriteDate}) {
				t.Errorf("the files list the days %q, %v, want the one day, the damaged file set aside unlisted", dates, err)
			}
			if left := staged(t, path); len(left) > 0 {
				t.Errorf("files staged beside it were left behind: %q", left)
			}
		})
	}
}

// A compress stopped once it had written the compressed file, before it
// removed the plain one, then damage that lost the end of the lines it added:
// the repair keeps each member's lines a member of their own, so the
// compress after finds the lines of the plain file it holds, and holds each
// line once.
func TestADamagedFileACompressStoppedPartWayLeftHoldsEachLineOnce(t *testing.T) {
	f := requestLedger(t.TempDir())
	added := linesOf(`{"a":"old","n":1}`)
	put(t, f.path(compressedFile(rewriteDate)), slices.Concat(gzipOf(t, linesOf(`{"a":"old","n":0}`)), gzipCutInside(t, added, lostLine)))
	put(t, f.path(plainFile(rewriteDate)), []byte(added+lostLine))

	if _, err := f.Rewrite(rewriteDate, toNew); err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	if got, want := heldIn(t, f, compressedFile(rewriteDate)), string(toNew([]byte(linesOf(`{"a":"old","n":0}`)+added+lostLine))); got != want {
		t.Errorf("the compressed file gives\n%s\nwant each line once\n%s", got, want)
	}
}

func TestADamagedFileIsSetAsideAsACopyOnAFileSystemWithoutLinks(t *testing.T) {
	for _, why := range []syscall.Errno{syscall.ENOTSUP, syscall.EPERM} {
		t.Run(why.Error(), func(t *testing.T) {
			f := requestLedger(t.TempDir())
			f.link = linkFailing(why)
			path := f.path(compressedFile(rewriteDate))
			damaged := gzipCutShort(t, linesOf(`{"a":"old"}`))
			put(t, path, damaged)

			if _, err := f.Rewrite(rewriteDate, toNew); err != nil {
				t.Fatalf("Rewrite() = %v", err)
			}
			if got, want := heldIn(t, f, compressedFile(rewriteDate)), linesOf(`{"a":"new"}`); got != want {
				t.Errorf("the compressed file gives %q, want %q", got, want)
			}
			if aside, err := os.ReadFile(path + ".damaged"); err != nil || !bytes.Equal(aside, damaged) {
				t.Errorf("the damaged file set aside holds %q, %v, want it as it was", aside, err)
			}
		})
	}
}

// An I/O error repairing a damaged file fails the rewrite, leaving the file as
// it was and nothing beside it, so the rewrite run again repairs it.
func TestAnIOErrorRepairingADamagedFileFailsTheRewriteForItToBeRunAgain(t *testing.T) {
	f := requestLedger(t.TempDir())
	f.link = linkFailing(syscall.EIO)
	path := f.path(compressedFile(rewriteDate))
	damaged := gzipCutShort(t, linesOf(`{"a":"old"}`))
	put(t, path, damaged)

	if _, err := f.Rewrite(rewriteDate, toNew); !errors.Is(err, syscall.EIO) {
		t.Fatalf("Rewrite() = %v, want it failing as the disk did", err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, damaged) {
		t.Errorf("the damaged file holds %q, %v, want it left as it was", got, err)
	}
	if _, err := os.Lstat(path + ".damaged"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the damaged file set aside: %v, want none", err)
	}
	if left := staged(t, path); len(left) > 0 {
		t.Errorf("files staged beside it were left behind: %q", left)
	}

	f.link = nil
	if _, err := f.Rewrite(rewriteDate, toNew); err != nil {
		t.Fatalf("Rewrite() again = %v", err)
	}
	if got, want := heldIn(t, f, compressedFile(rewriteDate)), linesOf(`{"a":"new"}`); got != want {
		t.Errorf("run again, the compressed file gives %q, want %q", got, want)
	}
	if aside, err := os.ReadFile(path + ".damaged"); err != nil || !bytes.Equal(aside, damaged) {
		t.Errorf("run again, the damaged file set aside holds %q, %v, want it as it was", aside, err)
	}
}

func TestADamagedCompressedFileBesideAPlainOneIsRewrittenWithItWhole(t *testing.T) {
	f := requestLedger(t.TempDir())
	damaged := gzipCutShort(t, linesOf(`{"a":"old"}`))
	put(t, f.path(compressedFile(rewriteDate)), damaged)
	put(t, f.path(plainFile(rewriteDate)), []byte(linesOf(`{"a":"old","late":1}`)))

	r, err := f.Rewrite(rewriteDate, toNew)
	if err != nil || !r.Changed {
		t.Fatalf("Rewrite() = %+v, %v, want the day rewritten", r, err)
	}
	if got, want := heldIn(t, f, compressedFile(rewriteDate)), linesOf(`{"a":"new"}`, `{"a":"new","late":1}`); got != want {
		t.Errorf("the compressed file gives %q, want %q", got, want)
	}
	if _, err := os.Stat(f.path(plainFile(rewriteDate))); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the plain file: %v, want it compressed with the rest", err)
	}
	if aside, err := os.ReadFile(f.path(compressedFile(rewriteDate)) + ".damaged"); err != nil || !bytes.Equal(aside, damaged) {
		t.Errorf("the damaged file set aside holds %q, %v, want it as it was", aside, err)
	}
}

func TestADamagedFileSetAsideKeepsOneSetAsideBefore(t *testing.T) {
	f := requestLedger(t.TempDir())
	path := f.path(compressedFile(rewriteDate))
	put(t, path+".damaged", []byte("set aside before"))
	put(t, path, []byte("not gzip, nor ever was\n"))

	if _, err := f.Rewrite(rewriteDate, toNew); err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	for aside, want := range map[string]string{path + ".damaged": "set aside before", path + ".damaged-2": "not gzip, nor ever was\n"} {
		if got, err := os.ReadFile(aside); err != nil || string(got) != want {
			t.Errorf("%s holds %q, %v, want %q", filepath.Base(aside), got, err, want)
		}
	}
}

func TestOnlyACompressedFilesOwnDamageCountsAsDamage(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"not gzip", gzip.ErrHeader, true},
		{"failing its checksum", gzip.ErrChecksum, true},
		{"not inflating", flate.CorruptInputError(12), true},
		{"cut short", io.ErrUnexpectedEOF, true},
		{"empty", io.EOF, true},
		{"the disk failing", &fs.PathError{Op: "read", Path: "requests-2026-11-02.jsonl.gz", Err: syscall.EIO}, false},
		{"anything else", errors.New("bad file descriptor"), false},
	} {
		if got := errors.Is(damaged(tc.err), ErrDamaged); got != tc.want {
			t.Errorf("%s: damaged = %t, want %t", tc.name, got, tc.want)
		}
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
