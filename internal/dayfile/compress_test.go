package dayfile

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// earlierDate is the day of the compressed file an earlier build's compress
// wrote, in testdata: of {"n":1} and {"n":2}, compressed; then {"n":3} and a
// line cut short, compressed after them, the clock set back to the day; then
// {"n":5}, compressed after a line ending, again so.
const earlierDate = "2026-09-25"

// earlierLines are the lines the earlier build's compressed file gives.
const earlierLines = "{\"n\":1}\n{\"n\":2}\n{\"n\":3}\n{\"n\":\n{\"n\":5}\n"

// earlierFile returns the compressed file an earlier build's compress wrote,
// as testdata holds it.
func earlierFile(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", compressedFile(earlierDate).name("requests")))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestACompressedFileAnEarlierBuildWroteReadsAsItDid(t *testing.T) {
	earlier := earlierFile(t)
	f := requestLedger(t.TempDir())
	put(t, f.path(compressedFile(earlierDate)), earlier)
	if got := heldIn(t, f, compressedFile(earlierDate)); got != earlierLines || membersIn(t, earlier) != 3 {
		t.Fatalf("the earlier build's file gives %q in %d members, want %q in 3", got, membersIn(t, earlier), earlierLines)
	}
	if read, unread := readJSON(f, earlierDate); !slices.Equal(read, []string{`{"n":1}`, `{"n":2}`, `{"n":3}`, `{"n":5}`}) || unread != 1 {
		t.Errorf("read %q, %d unread; want the lines it was given, but the one cut short", read, unread)
	}

	put(t, f.path(plainFile(earlierDate)), []byte(linesOf(`{"n":6}`)))
	if err := f.compress(earlierDate); err != nil {
		t.Fatalf("compress() = %v", err)
	}
	if got, want := heldIn(t, f, compressedFile(earlierDate)), earlierLines+linesOf(`{"n":6}`); got != want {
		t.Errorf("compressed again, it gives %q, want %q", got, want)
	}
	if got := written(t, f, compressedFile(earlierDate)); !bytes.HasPrefix(got, earlier) {
		t.Errorf("compressed again, its members are rewritten, want them kept as they were, the new one after them")
	}
	if read, unread := readJSON(f, earlierDate); !slices.Equal(read, []string{`{"n":1}`, `{"n":2}`, `{"n":3}`, `{"n":5}`, `{"n":6}`}) || unread != 1 {
		t.Errorf("read %q, %d unread; want the lines it was given, but the one cut short", read, unread)
	}
}

func TestCompressingAsAnEarlierBuildDidGivesTheLinesItsFileGives(t *testing.T) {
	f := requestLedger(t.TempDir())
	for _, plain := range []string{linesOf(`{"n":1}`, `{"n":2}`), linesOf(`{"n":3}`) + `{"n":`, linesOf(`{"n":5}`)} {
		put(t, f.path(plainFile(earlierDate)), []byte(plain))
		if err := f.compress(earlierDate); err != nil {
			t.Fatalf("compress() = %v", err)
		}
	}
	got := written(t, f, compressedFile(earlierDate))
	if lines := heldIn(t, f, compressedFile(earlierDate)); lines != earlierLines || membersIn(t, got) != membersIn(t, earlierFile(t)) {
		t.Errorf("compressed in turn, the file gives %q in %d members, want %q in %d, as the earlier build's does",
			lines, membersIn(t, got), earlierLines, membersIn(t, earlierFile(t)))
	}
}

func TestACompressStoppedPartwayIsFinishedByTheNextOnce(t *testing.T) {
	older := linesOf(`{"n":1}`)
	torn := older + `{"n":`
	tests := []struct {
		name string
		// members are those the compressed file holds before, plain the plain
		// file, stopped how the compress stopped, and appended what's
		// appended to the plain file before the next.
		members  []string
		plain    string
		stopped  func(t *testing.T, f *Files)
		appended string
		want     string
	}{
		{
			name:    "once it had written the compressed file",
			plain:   linesOf(`{"n":2}`),
			stopped: addingPlain,
			want:    linesOf(`{"n":2}`),
		},
		{
			name:    "once it had written the compressed file twice",
			members: []string{older},
			plain:   linesOf(`{"n":2}`),
			stopped: func(t *testing.T, f *Files) { addingPlain(t, f); addingPlain(t, f) },
			want:    older + linesOf(`{"n":2}`),
		},
		{
			name:    "before it renamed the compressed file into place",
			members: []string{older},
			plain:   linesOf(`{"n":2}`),
			stopped: leavingTemp,
			want:    older + linesOf(`{"n":2}`),
		},
		{
			name:     "once it had written the compressed file, and a line appended after",
			members:  []string{older},
			plain:    linesOf(`{"n":2}`),
			stopped:  addingPlain,
			appended: linesOf(`{"n":3}`),
			want:     older + linesOf(`{"n":2}`, `{"n":3}`),
		},
		{
			name:     "once it had written the compressed file, and a line appended after its plain file's line cut short",
			members:  []string{older},
			plain:    linesOf(`{"n":2}`) + `{"n":`,
			stopped:  addingPlain,
			appended: linesOf(`{"n":3}`),
			want:     older + linesOf(`{"n":2}`, `{"n":`, `{"n":3}`),
		},
		{
			name:     "once it had written the compressed file after its line cut short, and a line appended after",
			members:  []string{torn},
			plain:    linesOf(`{"n":2}`),
			stopped:  addingPlain,
			appended: linesOf(`{"n":3}`),
			want:     torn + "\n" + linesOf(`{"n":2}`, `{"n":3}`),
		},
		{
			name:     "with nothing compressed before, once it had written the compressed file, and a line appended after",
			plain:    linesOf(`{"n":2}`),
			stopped:  addingPlain,
			appended: linesOf(`{"n":3}`),
			want:     linesOf(`{"n":2}`, `{"n":3}`),
		},
		{
			name:     "after members of their own, once it had written the compressed file, and a line appended after",
			members:  []string{older, linesOf(`{"n":2}`)},
			plain:    linesOf(`{"n":3}`),
			stopped:  addingPlain,
			appended: linesOf(`{"n":4}`),
			want:     older + linesOf(`{"n":2}`, `{"n":3}`, `{"n":4}`),
		},
		{
			name:    "after members of their own, stopped twice, a line appended after each time",
			members: []string{older, linesOf(`{"n":2}`)},
			plain:   linesOf(`{"n":3}`),
			stopped: func(t *testing.T, f *Files) {
				addingPlain(t, f)
				appendingPlain(t, f, linesOf(`{"n":4}`))
				addingPlain(t, f)
			},
			appended: linesOf(`{"n":5}`),
			want:     older + linesOf(`{"n":2}`, `{"n":3}`, `{"n":4}`, `{"n":5}`),
		},
		{
			name:    "stopped three times, a line appended after each time",
			members: []string{older},
			plain:   linesOf(`{"n":2}`),
			stopped: func(t *testing.T, f *Files) {
				for _, line := range []string{`{"n":3}`, `{"n":4}`} {
					addingPlain(t, f)
					appendingPlain(t, f, linesOf(line))
				}
				addingPlain(t, f)
			},
			appended: linesOf(`{"n":5}`),
			want:     older + linesOf(`{"n":2}`, `{"n":3}`, `{"n":4}`, `{"n":5}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())
			if tt.members != nil {
				writeDay(t, f, compressedFile(earlierDate), tt.members...)
			}
			writeDay(t, f, plainFile(earlierDate), tt.plain)
			tt.stopped(t, f)
			appendingPlain(t, f, tt.appended)

			if err := f.compress(earlierDate); err != nil {
				t.Fatalf("compress() = %v", err)
			}
			if got := heldIn(t, f, compressedFile(earlierDate)); got != tt.want {
				t.Errorf("the day's compressed file gives %q, want each of its lines once: %q", got, tt.want)
			}
			if _, err := os.Stat(f.path(plainFile(earlierDate))); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("the day's plain file is there (%v), want it gone", err)
			}
		})
	}
}

// addingPlain adds the plain file of earlierDate to its compressed file, as a
// compress stopped before it removed the plain file does.
func addingPlain(t *testing.T, f *Files) {
	t.Helper()
	if err := f.addPlain(earlierDate); err != nil {
		t.Fatal(err)
	}
}

// appendingPlain appends lines, where there are any, to the plain file of
// earlierDate, as Append does.
func appendingPlain(t *testing.T, f *Files, lines string) {
	t.Helper()
	if lines == "" {
		return
	}
	if err := appendLines(f.path(plainFile(earlierDate)), []byte(lines)); err != nil {
		t.Fatal(err)
	}
}

// leavingTemp leaves what a compress stopped before it renamed the
// compressed file of earlierDate into place leaves: the file it was writing,
// cut short, beside it.
func leavingTemp(t *testing.T, f *Files) {
	t.Helper()
	member := gzipOf(t, linesOf(`{"n":2}`))
	put(t, filepath.Join(f.Dir, "."+compressedFile(earlierDate).name(f.Prefix)+".2961577413"), member[:len(member)/2])
}

func TestCompressingABigDayNeverHoldsItWhole(t *testing.T) {
	// size is how long each of the files runs to: lines all different, as a
	// day's are.
	const size = 4 << 20
	compressed, plain := bigLines("a", size), bigLines("b", size)
	tests := []struct {
		name string
		// members are what the compressed file holds before, and want what it
		// gives after.
		members []string
		want    string
	}{
		{name: "after the compressed file's lines", members: []string{compressed}, want: compressed + plain},
		{name: "not again, where the compressed file ends with them", members: []string{compressed, plain}, want: compressed + plain},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())
			writeDay(t, f, compressedFile(earlierDate), tt.members...)
			writeDay(t, f, plainFile(earlierDate), plain)

			allocated := allocatedBy(t, func() {
				if err := f.compress(earlierDate); err != nil {
					t.Fatalf("compress() = %v", err)
				}
			})
			if allocated > size/2 {
				t.Errorf("compressing allocated %d MiB, want no more than %d: the day's files never held whole", allocated>>20, (size/2)>>20)
			}
			if got, want := sumOf(t, f, compressedFile(earlierDate)), sha256.Sum256([]byte(tt.want)); got != want {
				t.Errorf("the day's compressed file gives other lines than its own and the plain file's, each once")
			}
		})
	}
}

// bigLines returns numbered lines, each marked with mark, to as many bytes
// as most, at least.
func bigLines(mark string, most int) string {
	var lines bytes.Buffer
	for n := 0; lines.Len() < most; n++ {
		fmt.Fprintf(&lines, `{"mark":%q,"n":%d,"request":"req_%08x"}`+"\n", mark, n, n*2654435761)
	}
	return lines.String()
}

// allocatedBy returns how many bytes do allocates, as the runtime counts
// them: those any other goroutine allocates meanwhile too, which no test
// running alone has.
func allocatedBy(t *testing.T, do func()) uint64 {
	t.Helper()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	do()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// sumOf returns the SHA-256 of the lines the file of f gives.
func sumOf(t *testing.T, f *Files, file dayFile) [sha256.Size]byte {
	t.Helper()
	src, err := os.Open(f.path(file))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Close() }()
	members, err := gzip.NewReader(src)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.New()
	if _, err := io.Copy(sum, members); err != nil {
		t.Fatal(err)
	}
	return [sha256.Size]byte(sum.Sum(nil))
}

// put writes data to a file at path, the user's alone.
func put(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// written returns the file of f as it's written.
func written(t *testing.T, f *Files, file dayFile) []byte {
	t.Helper()
	data, err := os.ReadFile(f.path(file))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// membersIn returns how many gzip members data holds.
func membersIn(t *testing.T, data []byte) int {
	t.Helper()
	src := bytes.NewReader(data)
	members, err := gzip.NewReader(src)
	if err != nil {
		t.Fatal(err)
	}
	for n := 1; ; n++ {
		members.Multistream(false)
		if _, err := io.Copy(io.Discard, members); err != nil {
			t.Fatal(err)
		}
		if err := members.Reset(src); errors.Is(err, io.EOF) {
			return n
		} else if err != nil {
			t.Fatal(err)
		}
	}
}
