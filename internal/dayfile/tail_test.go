package dayfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// tailed is what a reader of a day's lines holds as it reads them with a
// Tail: the lines taken since it last read them afresh, and how many times it
// has.
type tailed struct {
	tail   *Tail[string]
	held   []string
	afresh int
}

// tailOf returns a reader of the lines of f's files of the local day with the
// given date that are JSON, as a line written whole is, read with a Tail.
func tailOf(f *Files, date string) *tailed {
	return &tailed{tail: NewTail(f, date, asJSONLine)}
}

// asJSONLine is a line as the text it holds, where it's JSON.
func asJSONLine(line []byte) (string, bool) {
	return string(line), json.Valid(line)
}

// read reads the lines the day's files gained, as Read does.
func (r *tailed) read() bool {
	return r.tail.Read(func() {
		r.held = nil
		r.afresh++
	}, func(line string) { r.held = append(r.held, line) })
}

// lines returns the lines held, and the line cut short after them, where
// there's one that's JSON.
func (r *tailed) lines() []string {
	if cut, ok := r.tail.Cut(); ok {
		return append(slices.Clone(r.held), cut)
	}
	return r.held
}

// wholeDay returns the lines of f's files of the local day with the given
// date that are JSON, as ReadDay reads them whole, and how many lines they
// hold, and how many of those aren't JSON.
func wholeDay(f *Files, date string) (read []string, lines, unread int) {
	lines, unread, _ = ReadDay(f, date, asJSONLine, func(line string) bool {
		read = append(read, line)
		return true
	})
	return read, lines, unread
}

// appendTo appends text to the file of f, making it where it isn't there.
func appendTo(t *testing.T, f *Files, file dayFile, text string) {
	t.Helper()
	out, err := os.OpenFile(f.path(file), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := out.WriteString(text); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// replace replaces the file of f with another holding text, as a restore
// does.
func replace(t *testing.T, f *Files, file dayFile, text string) {
	t.Helper()
	other := filepath.Join(t.TempDir(), "other")
	if err := os.WriteFile(other, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(other, f.path(file)); err != nil {
		t.Fatal(err)
	}
}

func TestATailGivesWhatAWholeReadGivesAsTheDayGrowsAndChanges(t *testing.T) {
	const date = "2026-09-25"
	plain := plainFile(date)
	steps := []struct {
		name string
		do   func(t *testing.T, f *Files)
		// afresh is set where the step has the day's files read afresh.
		afresh bool
	}{
		{name: "the first lines written", do: func(t *testing.T, f *Files) { appendTo(t, f, plain, linesOf(`{"n":1}`, `{"n":2}`)) }, afresh: true},
		{name: "nothing written", do: func(*testing.T, *Files) {}},
		{name: "a line half written", do: func(t *testing.T, f *Files) { appendTo(t, f, plain, `{"n":3`) }},
		{name: "its end written, and a line after it", do: func(t *testing.T, f *Files) { appendTo(t, f, plain, "}\n"+linesOf(`{"n":4}`)) }},
		{
			name: "a line torn, as a crash leaves one, and ended as the next are appended",
			do: func(t *testing.T, f *Files) {
				appendTo(t, f, plain, `{"n":`)
				if err := f.Append(Lines{date: []byte(linesOf(`{"n":6}`))}); err != nil {
					t.Fatal(err)
				}
			},
		},
		{name: "a line written all but its line ending", do: func(t *testing.T, f *Files) { appendTo(t, f, plain, `{"n":7}`) }},
		{name: "its line ending written", do: func(t *testing.T, f *Files) { appendTo(t, f, plain, "\n") }},
		{name: "compressed", do: compressing(date), afresh: true},
		{name: "written again, as after the clock was set back to it", do: func(t *testing.T, f *Files) { appendTo(t, f, plain, linesOf(`{"n":8}`)) },
			afresh: true},
		{name: "written again beside its compressed file", do: func(t *testing.T, f *Files) { appendTo(t, f, plain, linesOf(`{"n":9}`)) }, afresh: true},
		{name: "compressed again", do: compressing(date), afresh: true},
		{name: "pruned", do: func(t *testing.T, f *Files) { removeFile(t, f, compressedFile(date)) }, afresh: true},
		{name: "nothing written, its files gone", do: func(*testing.T, *Files) {}},
		{name: "written once more", do: func(t *testing.T, f *Files) { appendTo(t, f, plain, linesOf(`{"n":10}`, `{"n":11}`)) }, afresh: true},
		{name: "replaced, by a file longer still", do: func(t *testing.T, f *Files) { replace(t, f, plain, linesOf(`{"n":12}`, `{"n":13}`, `{"n":14}`)) },
			afresh: true},
		{name: "shrunk", do: func(t *testing.T, f *Files) { shrink(t, f, plain, int64(len(linesOf(`{"n":12}`)))) }, afresh: true},
		{name: "a line written after", do: func(t *testing.T, f *Files) { appendTo(t, f, plain, linesOf(`{"n":15}`)) }},
	}
	f := requestLedger(t.TempDir())
	tail := tailOf(f, date)
	for _, step := range steps {
		step.do(t, f)
		before := tail.afresh

		if !tail.read() {
			t.Fatalf("%s: Read() = false, want true", step.name)
		}
		want, wantLines, wantUnread := wholeDay(f, date)
		if got := tail.lines(); !slices.Equal(got, want) {
			t.Errorf("%s: the tail holds %q, want %q, as a whole read gives", step.name, got, want)
		}
		if lines, unread := tail.tail.Lines(); lines != wantLines || unread != wantUnread {
			t.Errorf("%s: Lines() = %d, %d unread; want %d, %d unread, as a whole read counts them", step.name, lines, unread, wantLines, wantUnread)
		}
		if afresh := tail.afresh > before; afresh != step.afresh {
			t.Errorf("%s: read afresh %v, want %v", step.name, afresh, step.afresh)
		}
	}
}

// compressing returns what compresses the files of f of the local day with
// the given date, as a prune does once its day ended two days before.
func compressing(date string) func(t *testing.T, f *Files) {
	return func(t *testing.T, f *Files) {
		t.Helper()
		if err := f.compress(date); err != nil {
			t.Fatal(err)
		}
	}
}

// removeFile removes the file of f, as a prune does.
func removeFile(t *testing.T, f *Files, file dayFile) {
	t.Helper()
	if err := os.Remove(f.path(file)); err != nil {
		t.Fatal(err)
	}
}

// shrink cuts the file of f to its first size bytes.
func shrink(t *testing.T, f *Files, file dayFile, size int64) {
	t.Helper()
	if err := os.Truncate(f.path(file), size); err != nil {
		t.Fatal(err)
	}
}

func TestATailReadsOnlyWhatThePlainFileGained(t *testing.T) {
	const date = "2026-09-25"
	f := requestLedger(t.TempDir())
	appendTo(t, f, plainFile(date), linesOf(`{"n":1}`, `{"n":2}`))
	tail := tailOf(f, date)
	tail.read()
	// The lines read rewritten in place, as no writer does, and a line
	// appended after them: a tail never reads them again to see.
	if err := os.WriteFile(f.path(plainFile(date)), []byte(linesOf(`{"n":8}`, `{"n":9}`)+linesOf(`{"n":3}`)), 0o600); err != nil {
		t.Fatal(err)
	}

	tail.read()
	if want := []string{`{"n":1}`, `{"n":2}`, `{"n":3}`}; !slices.Equal(tail.lines(), want) || tail.afresh != 1 {
		t.Errorf("the tail holds %q, read afresh %d times; want %q, read afresh once, as first read: the line appended alone read since",
			tail.lines(), tail.afresh, want)
	}
}

func TestATailOpensNoFileOfADayThatHasntChanged(t *testing.T) {
	log := logstest.Capture(t)
	const date = "2026-09-25"
	f := requestLedger(t.TempDir())
	writeDay(t, f, compressedFile(date), linesOf(`{"n":1}`))
	writeDay(t, f, plainFile(date), linesOf(`{"n":2}`))
	tail := tailOf(f, date)
	tail.read()
	for _, file := range []dayFile{plainFile(date), compressedFile(date)} {
		if err := os.Chmod(f.path(file), 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(f.path(file), 0o600) })
	}

	if !tail.read() || tail.afresh != 1 || log.Has("level=WARN") {
		t.Errorf("log reads\n%s\nread afresh %d times; want the day's files unopened, read once, as first read", log, tail.afresh)
	}
}

func TestATailOfAFileThatCantBeOpenedIsWarnedOfOnceAndReadAfreshOnceItCanBe(t *testing.T) {
	log := logstest.Capture(t)
	const date = "2026-09-25"
	f := requestLedger(t.TempDir())
	writeDay(t, f, compressedFile(date), linesOf(`{"n":1}`))
	writeDay(t, f, plainFile(date), linesOf(`{"n":2}`))
	if err := os.Chmod(f.path(compressedFile(date)), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(f.path(compressedFile(date)), 0o600) })
	tail := tailOf(f, date)

	for range 3 {
		if tail.read() {
			t.Fatal("Read() = true, want false: the compressed file can't be opened")
		}
	}
	if warned := linesWith(log, "file="+compressedFile(date).name(f.Prefix)); len(warned) != 1 || !log.Has("level=WARN", `msg="can't read the request ledger"`) {
		t.Errorf("log reads\n%s\nwant the file that can't be opened warned of once", log)
	}
	if want := []string{`{"n":2}`}; !slices.Equal(tail.lines(), want) {
		t.Errorf("the tail holds %q, want %q: the lines of the file that could be read", tail.lines(), want)
	}

	if err := os.Chmod(f.path(compressedFile(date)), 0o600); err != nil {
		t.Fatal(err)
	}
	if want := []string{`{"n":1}`, `{"n":2}`}; !tail.read() || !slices.Equal(tail.lines(), want) || tail.afresh != 4 {
		t.Errorf("once the file can be opened, the tail holds %q, read afresh %d times; want %q, read afresh at each read", tail.lines(), tail.afresh, want)
	}
}

// errBadSector is the error of a read of a part of a disk that can't be read.
var errBadSector = errors.New("bad sector")

// badSector reads data as a file holding it reads, but for any part of it
// from from up to to, which fails.
type badSector struct {
	data     []byte
	from, to int64
}

func (b badSector) ReadAt(p []byte, off int64) (int, error) {
	if off < b.to && off+int64(len(p)) > b.from {
		return 0, errBadSector
	}
	return bytes.NewReader(b.data).ReadAt(p, off)
}

// numbered returns lines of JSON numbered from from up to to.
func numbered(from, to int) string {
	var texts []string
	for n := from; n < to; n++ {
		texts = append(texts, `{"n":`+strconv.Itoa(n)+`}`)
	}
	return linesOf(texts...)
}

func TestATailThatReadsShortReadsAfreshNextTime(t *testing.T) {
	log := logstest.Capture(t)
	const date = "2026-09-25"
	f := requestLedger(t.TempDir())
	appendTo(t, f, plainFile(date), numbered(0, 10))
	tail := tailOf(f, date)
	tail.read()
	ended := tail.tail.end
	// Lines enough that the end of the last is far past the part that fails.
	appendTo(t, f, plainFile(date), numbered(10, 4000))
	data, err := os.ReadFile(f.path(plainFile(date)))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(f.path(plainFile(date)))
	if err != nil {
		t.Fatal(err)
	}

	// Reading on fails partway through what the file gained, as at a bad
	// sector of a disk.
	failing := badSector{data: data, from: ended + 2*lookBack, to: ended + 2*lookBack + 1}
	if tail.tail.follow(failing, info, func(line string) { tail.held = append(tail.held, line) }) {
		t.Fatal("follow() = true, want false: it read short")
	}
	if !log.Has("level=WARN", `msg="request ledger read short"`, `error="bad sector"`) {
		t.Errorf("log reads\n%s\nwant the read that failed warned of", log)
	}
	tail.read()
	if want, _, _ := wholeDay(f, date); !slices.Equal(tail.lines(), want) || tail.afresh != 2 {
		t.Errorf("the next read holds %d lines, read afresh %d times; want the day's %d, read afresh again, none passed over", len(tail.lines()),
			tail.afresh, len(want))
	}
}

func TestATailFindsTheEndOfALineLongerThanItReadsBackThroughAtATime(t *testing.T) {
	const date = "2026-09-25"
	f := &Files{Dir: t.TempDir(), Prefix: "requests", Name: "request ledger", LineMax: 4 * lookBack, Logger: testLogger}
	long := `{"text":"` + string(slices.Repeat([]byte("x"), 2*lookBack)) + `"}`
	appendTo(t, f, plainFile(date), linesOf(`{"n":1}`)+long)
	tail := tailOf(f, date)

	tail.read()
	if lines, unread := tail.tail.Lines(); !slices.Equal(tail.held, []string{`{"n":1}`}) || !slices.Equal(tail.lines(), []string{`{"n":1}`, long}) ||
		lines != 2 || unread != 0 {
		t.Errorf("the tail holds %d lines, %d of them whole, Lines() = %d, %d unread; want the first whole and the long line cut short after it",
			len(tail.lines()), len(tail.held), lines, unread)
	}
	appendTo(t, f, plainFile(date), "\n")
	tail.read()
	if _, cut := tail.tail.Cut(); !slices.Equal(tail.held, []string{`{"n":1}`, long}) || cut || tail.afresh != 1 {
		t.Errorf("once ended, the tail holds %d lines whole, and one cut short %v; want both whole, read on from the first", len(tail.held), cut)
	}
}
