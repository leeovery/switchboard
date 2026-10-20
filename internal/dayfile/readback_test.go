package dayfile

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// upTo returns what takes each value it's handed into got, until got holds
// n.
func upTo[T any](got *[]T, n int) func(T) bool {
	return func(v T) bool {
		*got = append(*got, v)
		return len(*got) < n
	}
}

// backTo returns what ReadBack hands on of f's day with the given date, as
// decode makes it, up to the n-th, and what it returns.
func backTo[T any](f *Files, date string, decode func([]byte) (T, bool), n int) (got []T, lines, unread int, ok bool, err error) {
	lines, unread, ok, err = ReadBack(f, date, decode, upTo(&got, n))
	return got, lines, unread, ok, err
}

// forwardTo returns what ReadDay hands on of f's day with the given date, as
// decode makes it, up to the n-th, and what it returns.
func forwardTo[T any](f *Files, date string, decode func([]byte) (T, bool), n int) (got []T, lines, unread int, err error) {
	lines, unread, err = ReadDay(f, date, decode, upTo(&got, n))
	return got, lines, unread, err
}

// turnedAbout returns values, the last first.
func turnedAbout[T any](values []T) []T {
	turned := slices.Clone(values)
	slices.Reverse(turned)
	return turned
}

// jsonLine returns a line of JSON numbered n, length bytes long, or as short
// as it can be where that's longer.
func jsonLine(n, length int) string {
	head := `{"n":` + strconv.Itoa(n) + `,"pad":"`
	return head + strings.Repeat("x", max(length-len(head)-2, 0)) + `"}`
}

// torn returns the first half of line, as a write cut short leaves it.
func torn(line string) string {
	return line[:len(line)/2]
}

// lineKinds are the kinds of line a day's plain file holds, each as it's
// written, its line ending included: numbered n, of length bytes as a rule,
// in files read with LineMax most.
var lineKinds = []func(n, length, most int) string{
	// JSON, as a line written whole is.
	func(n, length, _ int) string { return jsonLine(n, length) + "\n" },
	// JSON ending in a carriage return and a line feed, the return dropped.
	func(n, length, _ int) string { return jsonLine(n, length) + "\r\n" },
	// JSON ending in two carriage returns and a line feed, one return kept.
	func(n, length, _ int) string { return jsonLine(n, length) + "\r\r\n" },
	// Blank.
	func(int, int, int) string { return "\n" },
	// Blank but for a carriage return.
	func(int, int, int) string { return "\r\n" },
	// Not JSON.
	func(int, int, int) string { return "not JSON\n" },
	// Cut short, as a crash leaves a line, and ended by the next write.
	func(n, length, _ int) string { return torn(jsonLine(n, length)) + "\n" },
	// As long as is held, its line ending included.
	func(n, _, most int) string { return jsonLine(n, most-1) + "\n" },
	// As long as is held, ending in a carriage return and a line feed.
	func(n, _, most int) string { return jsonLine(n, most-2) + "\r\n" },
	// A byte too long to hold.
	func(n, _, most int) string { return jsonLine(n, most) + "\n" },
	// A byte too long to hold, by its carriage return.
	func(n, _, most int) string { return jsonLine(n, most-1) + "\r\n" },
	// Far too long to hold.
	func(n, length, most int) string { return jsonLine(n, most+3*length) + "\n" },
}

// endings are the ways a day's plain file ends, each its last line as it's
// written: numbered n, of length bytes as a rule, in files read with LineMax
// most.
var endings = []func(n, length, most int) string{
	// In a line ending.
	func(int, int, int) string { return "" },
	// In a line of JSON without its ending.
	func(n, length, _ int) string { return jsonLine(n, length) },
	// In a line cut short, as one still being written.
	func(n, length, _ int) string { return torn(jsonLine(n, length)) },
	// In a carriage return alone.
	func(int, int, int) string { return "\r" },
	// In a line of JSON and a carriage return.
	func(n, length, _ int) string { return jsonLine(n, length) + "\r" },
	// In a line as long as is held without its ending.
	func(n, _, most int) string { return jsonLine(n, most-1) },
	// In a line a byte too long to hold without its ending.
	func(n, _, most int) string { return jsonLine(n, most) },
}

// backShape is a shape of the day's plain files the property tests generate.
type backShape struct {
	name string
	// most is the files' LineMax, and longest the longest of most lines held.
	most, longest int
	// before is how much of a file comes before the line too long to hold
	// planted across the edge of the stretch its last lines fill, at least.
	before int
}

// backShapes are the shapes of the plain files the property tests generate:
// of lines as long as a request's; and of lines longer than a stretch, those
// as long as can be held among them, held across a stretch's edge.
var backShapes = []backShape{
	{name: "lines as long as a request's", most: 4096, longest: 1500, before: backStretch},
	{name: "lines longer than a stretch", most: 80 << 10, longest: 72 << 10, before: 2 * backStretch},
}

// backSeeds are how many seeds the property tests generate files of: one
// for every ending of every shape.
var backSeeds = uint64(len(backShapes) * len(endings))

// backShapeOf returns the shape of the file backFile generates of the seed
// given: each in turn.
func backShapeOf(seed uint64) backShape {
	return backShapes[seed%uint64(len(backShapes))]
}

// backDay writes the plain file of the day with the given date, as backFile
// generates it of the seed given, to files read with the LineMax its shape
// has, and returns them.
func backDay(t *testing.T, date string, seed uint64) *Files {
	t.Helper()
	f := requestLedger(t.TempDir())
	f.LineMax = backShapeOf(seed).most
	writeDay(t, f, plainFile(date), backFile(seed))
	return f
}

// backFile returns a day's plain file as the seed given generates it, of the
// shape it has: every kind of line once, among lines of JSON mostly, now and
// then another kind, in an order of their own; then a line too long to hold,
// across the edge of the stretch the lines after it fill, read back, or,
// where an ending as long as can be held fills one alone, the next; then
// lines and an ending as the seed has them.
func backFile(seed uint64) string {
	shape := backShapeOf(seed)
	rng := rand.New(rand.NewPCG(seed, 47))
	n := 0
	line := func(kind func(n, length, most int) string) string {
		n++
		return kind(n, lineLength(rng, shape.longest), shape.most)
	}
	var head []string
	for _, kind := range lineKinds {
		head = append(head, line(kind))
	}
	for size := 0; size < shape.before; size += len(head[len(head)-1]) {
		head = append(head, line(anyKind(rng)))
	}
	rng.Shuffle(len(head), func(i, j int) { head[i], head[j] = head[j], head[i] })
	ending := line(endings[seed%uint64(len(endings))])
	var tail []string
	size := len(ending)
	for next := line(anyKind(rng)); size+len(next) < backStretch-1; next = line(anyKind(rng)) {
		tail, size = append(tail, next), size+len(next)
	}
	planted := line(func(n, _, most int) string { return jsonLine(n, max(most, backStretch-size)+rng.IntN(most)) + "\n" })
	return strings.Join(head, "") + planted + strings.Join(tail, "") + ending
}

// anyKind returns a kind of line: JSON three times in four, and any other
// kind now and then.
func anyKind(rng *rand.Rand) func(n, length, most int) string {
	if rng.IntN(4) > 0 {
		return lineKinds[0]
	}
	return lineKinds[rng.IntN(len(lineKinds))]
}

// lineLength returns how long a line runs, as a rule, up to longest: most as
// long as a request's, now and then one longer.
func lineLength(rng *rand.Rand, longest int) int {
	if rng.IntN(4) > 0 {
		return rng.IntN(min(longest, 1500))
	}
	return rng.IntN(longest)
}

// asBraced is a line as the text it holds, where it's between braces, as a
// line of JSON written whole is, and a blank line, one cut short, or one
// ending in a carriage return kept, isn't: as cheap to tell as can be, for
// reads again and again.
func asBraced(line []byte) (string, bool) {
	return string(line), len(line) > 1 && line[0] == '{' && line[len(line)-1] == '}'
}

func TestReadBackReadingWholeHandsWhatReadDayHandsTurnedAbout(t *testing.T) {
	const date = "2026-09-25"
	decodes := []struct {
		name   string
		decode func(line []byte) (string, bool)
	}{
		{name: "every line, as its text", decode: asText},
		{name: "lines of JSON alone", decode: asJSONLine},
	}
	for seed := range backSeeds {
		t.Run(fmt.Sprintf("seed %d, %s", seed, backShapeOf(seed).name), func(t *testing.T) {
			f := backDay(t, date, seed)
			for _, d := range decodes {
				t.Run(d.name, func(t *testing.T) {
					forward, lines, unread, err := forwardTo(f, date, d.decode, math.MaxInt)
					if err != nil {
						t.Fatalf("seed %d: ReadDay() failed: %v", seed, err)
					}

					back, backLines, backUnread, ok, err := backTo(f, date, d.decode, math.MaxInt)
					if !slices.Equal(back, turnedAbout(forward)) || backLines != lines || backUnread != unread || !ok || err != nil {
						t.Errorf("seed %d: ReadBack() handed on %d lines, = %d, %d, %v, %v; want ReadDay's %d turned about, %d, %d, true, nil",
							seed, len(back), backLines, backUnread, ok, err, len(forward), lines, unread)
					}
				})
			}
		})
	}
}

func TestReadBackToAnyLineReadsWhatReadDayReadsOfTheLastLines(t *testing.T) {
	const date = "2026-09-25"
	for seed := range backSeeds {
		t.Run(fmt.Sprintf("seed %d, %s", seed, backShapeOf(seed).name), func(t *testing.T) {
			f := backDay(t, date, seed)
			forward, lines, unread, err := forwardTo(f, date, asBraced, math.MaxInt)
			if err != nil {
				t.Fatalf("seed %d: ReadDay() failed: %v", seed, err)
			}

			for k := 1; k <= len(forward); k++ {
				back, backLines, backUnread, _, _ := backTo(f, date, asBraced, k)
				_, forwardLines, forwardUnread, _ := forwardTo(f, date, asBraced, len(forward)-k+1)
				// Read forward to the line, and back to it, every line is read, that
				// one twice.
				wantLines, wantUnread := lines-forwardLines+1, unread-forwardUnread
				if !slices.Equal(back, turnedAbout(forward[len(forward)-k:])) || backLines != wantLines || backUnread != wantUnread {
					t.Fatalf("seed %d: ReadBack() to the %d-th line handed on = %d, %d; want ReadDay's last %d turned about, %d, %d: the file's, less those read forward before them",
						seed, k, backLines, backUnread, k, wantLines, wantUnread)
				}
			}
		})
	}
}

func TestReadBackReadsAFileAsFarAsItWasLongAsItOpenedIt(t *testing.T) {
	const date = "2026-09-25"
	long := strings.Repeat("x", 40)
	text := "one\r\ntwo\n" + long + "\nthree"
	tests := []struct {
		name string
		// size is how long the file was as it was opened.
		size int
	}{
		{name: "holding nothing", size: 0},
		{name: "ending in a line ending", size: strings.Index(text, "two")},
		{name: "ending partway through a line", size: strings.Index(text, "two") + 2},
		{name: "ending between a carriage return and its line feed", size: strings.Index(text, "\n")},
		{name: "ending partway through a line too long to hold, cut short enough to hold", size: strings.Index(text, long) + 10},
		{name: "ending partway through a line too long to hold", size: strings.Index(text, long) + 20},
		{name: "ending in a line too long to hold, without its ending", size: strings.Index(text, long) + len(long)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())
			f.LineMax = 16
			writeDay(t, f, plainFile(date), text[:tt.size])
			want, lines, unread, _ := forwardTo(f, date, asText, math.MaxInt)

			var got []string
			handed, long := f.readBack(plainFile(date), strings.NewReader(text), int64(tt.size), func(line []byte) bool {
				got = append(got, string(line))
				return true
			})
			if !slices.Equal(got, turnedAbout(want)) || handed+long != lines || long != unread {
				t.Errorf("read back %q, %d handed on, %d too long; want %q, ReadDay's of the file as it was turned about, %d read, %d unread",
					got, handed, long, turnedAbout(want), lines, unread)
			}
		})
	}
}

func TestReadBackNeverHandsALineAppendedAsItReads(t *testing.T) {
	const date = "2026-09-25"
	f := requestLedger(t.TempDir())
	// Lines enough for more than one stretch, the last cut short, as one still
	// being written.
	writeDay(t, f, plainFile(date), numbered(0, 8000)+`{"n":8000`)
	want, wantLines, wantUnread, _ := forwardTo(f, date, asText, math.MaxInt)

	var got []string
	lines, unread, ok, err := ReadBack(f, date, asText, func(line string) bool {
		if len(got) == 0 {
			// The line cut short written whole, and lines after it.
			appendTo(t, f, plainFile(date), "}\n"+numbered(8001, 8100))
		}
		got = append(got, line)
		return true
	})
	if !slices.Equal(got, turnedAbout(want)) || lines != wantLines || unread != wantUnread || !ok || err != nil {
		t.Errorf("read back %d lines, the first %q, %d read, %d unread, %v, %v; want %d, the line cut short first, as ReadDay read the file before, %d read, %d unread",
			len(got), got[:min(len(got), 1)], lines, unread, ok, err, len(want), wantLines, wantUnread)
	}
}

func TestReadBackReadsNothingOfADayWithACompressedFile(t *testing.T) {
	const date = "2026-09-25"
	tests := []struct {
		name  string
		files []dayFile
	}{
		{name: "alone", files: []dayFile{compressedFile(date)}},
		{name: "beside its plain one, as after the clock was set back to it", files: []dayFile{compressedFile(date), plainFile(date)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())
			for _, file := range tt.files {
				writeDay(t, f, file, linesOf(`{"n":1}`))
			}

			got, lines, unread, ok, err := backTo(f, date, asText, math.MaxInt)
			if ok || err != nil || len(got) > 0 || lines != 0 || unread != 0 {
				t.Errorf("ReadBack() handed on %q, = %d, %d, %v, %v; want none, 0, 0, false, nil: the day read forward", got, lines, unread, ok, err)
			}
		})
	}
}

func TestReadBackReadsNothingOfADayCompressedAsItOpensItsPlainFile(t *testing.T) {
	const date = "2026-09-25"
	f := requestLedger(t.TempDir())
	writeDay(t, f, plainFile(date), linesOf(`{"n":1}`))
	// Another process, a router, compresses the day once its plain file is
	// opened.
	f.opened = func(dayFile) {
		if err := requestLedger(f.Dir).compress(date); err != nil {
			t.Fatal(err)
		}
	}

	got, lines, unread, ok, err := backTo(f, date, asText, math.MaxInt)
	if ok || err != nil || len(got) > 0 || lines != 0 || unread != 0 {
		t.Errorf("ReadBack() handed on %q, = %d, %d, %v, %v; want none, 0, 0, false, nil: the compressed file looked for once the plain file is opened, "+
			"and the day read forward", got, lines, unread, ok, err)
	}
}

func TestReadBackOfADayWithoutFilesFailsAsReadDayDoes(t *testing.T) {
	f := requestLedger(t.TempDir())
	writeDay(t, f, plainFile("2026-09-26"), linesOf(`{"n":1}`))

	got, lines, unread, ok, err := backTo(f, "2026-09-25", asText, math.MaxInt)
	if !errors.Is(err, fs.ErrNotExist) || !ok || len(got) > 0 || lines != 0 || unread != 0 {
		t.Errorf("ReadBack() handed on %q, = %d, %d, %v, %v; want none, 0, 0, true, and an error matching fs.ErrNotExist", got, lines, unread, ok, err)
	}
}

func TestReadBackOfAPlainFileThatCantBeOpenedFailsAsReadDayDoes(t *testing.T) {
	log := logstest.Capture(t)
	const date = "2026-09-25"
	f := requestLedger(t.TempDir())
	writeDay(t, f, plainFile(date), linesOf(`{"n":1}`))
	if err := os.Chmod(f.path(plainFile(date)), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(f.path(plainFile(date)), 0o600) })

	got, lines, unread, ok, err := backTo(f, date, asText, math.MaxInt)
	if !errors.Is(err, fs.ErrPermission) || !ok || len(got) > 0 || lines != 0 || unread != 0 {
		t.Errorf("ReadBack() handed on %q, = %d, %d, %v, %v; want none, 0, 0, true, and an error matching fs.ErrPermission", got, lines, unread, ok, err)
	}
	if log.Has("level=WARN") {
		t.Errorf("log reads\n%s\nwant nothing warned of: the error says why, as ReadDay's does", log)
	}
}

func TestADamagedFileIsReadBackAsFarAsTheDamage(t *testing.T) {
	log := logstest.Capture(t)
	const date = "2026-09-25"
	f := requestLedger(t.TempDir())
	text := numbered(0, 20000)
	whole := turnedAbout(strings.Split(strings.TrimSuffix(text, "\n"), "\n"))
	// A bad sector a little way into the file, in its second line.
	bad := badSector{data: []byte(text), from: 10, to: 11}

	var got []string
	f.readBack(plainFile(date), bad, int64(len(text)), func(line []byte) bool {
		got = append(got, string(line))
		return true
	})
	if len(got) == 0 || len(got) > len(whole)-2 || !slices.Equal(got, whole[:len(got)]) {
		t.Errorf("read back %d lines; want the file's last, turned about, as far as the damage, and none of the 2 it reaches", len(got))
	}
	if want := []string{"level=WARN", `msg="request ledger read short"`, "file=" + plainFile(date).name(f.Prefix), `error="bad sector"`}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant the damage warned of, a line with %q", log, want)
	}
}

func TestAFileCutShortUnderAReadBackIsWarnedOfAndNoneOfItsLinesCutHandedOn(t *testing.T) {
	log := logstest.Capture(t)
	const date = "2026-09-25"
	f := requestLedger(t.TempDir())
	text := numbered(0, 20000)
	// The file was as long as text as it was opened, and lost its last bytes
	// since, as no writer leaves one.
	cut := strings.NewReader(text[:len(text)-5])

	var got []string
	f.readBack(plainFile(date), cut, int64(len(text)), func(line []byte) bool {
		got = append(got, string(line))
		return true
	})
	if len(got) > 0 {
		t.Errorf("read back %d lines, the first %q; want none: the lines it ended in are gone", len(got), got[0])
	}
	if want := []string{"level=WARN", `msg="request ledger read short"`, "file=" + plainFile(date).name(f.Prefix), `error="unexpected EOF"`}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant the file cut short warned of, a line with %q", log, want)
	}
}

func TestReadBackReadsNoFurtherBackThanTakeWants(t *testing.T) {
	log := logstest.Capture(t)
	const date = "2026-09-25"
	f := requestLedger(t.TempDir())
	text := numbered(0, 20000)
	// A bad sector at the file's start, which a read back that has had enough
	// by then never reaches.
	bad := badSector{data: []byte(text), from: 0, to: 1}

	var got []string
	handed, _ := f.readBack(plainFile(date), bad, int64(len(text)), func(line []byte) bool {
		got = append(got, string(line))
		return false
	})
	if !slices.Equal(got, []string{`{"n":19999}`}) || handed != 1 || log.Has("level=WARN") {
		t.Errorf("read back %q, %d handed on, logging\n%s\nwant the last line alone, and nothing warned of", got, handed, log)
	}
}

func TestADamagedFileReadBackIsWarnedOfOnceUntilItReadsBackWhole(t *testing.T) {
	log := logstest.Capture(t)
	const date = "2026-09-25"
	plain := plainFile(date)
	f := requestLedger(t.TempDir())
	// damage has the plain file open, but fail as it's read, as a directory
	// does, and mend has it read whole.
	damage := func() {
		if err := os.Mkdir(f.path(plain), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	mend := func() {
		removeFile(t, f, plain)
		writeDay(t, f, plain, linesOf(`{"n":1}`))
	}
	read := func() { _, _, _, _ = ReadBack(f, date, asText, every[string]) }

	damage()
	read()
	read()
	read()
	warned := linesWith(log, "file="+plain.name(f.Prefix))
	if len(warned) != 1 || !strings.Contains(warned[0], "level=WARN") || !strings.Contains(warned[0], `msg="request ledger read short"`) {
		t.Fatalf("log reads\n%s\nwant the file warned of once, as first read back, as ReadDay warns of one read short", log)
	}

	mend()
	read()
	removeFile(t, f, plain)
	damage()
	read()
	read()
	if warned := linesWith(log, "file="+plain.name(f.Prefix)); len(warned) != 2 {
		t.Errorf("log reads\n%s\nwant the file warned of again once it's damaged after reading back whole, once", log)
	}
}

func TestReadBackHoldsTheFilesFromPruningAsItReads(t *testing.T) {
	const date = "2026-09-25"
	f := requestLedger(t.TempDir())
	writeDay(t, f, plainFile(date), numbered(0, 3))
	held := func(when string) {
		if f.mu.TryLock() {
			f.mu.Unlock()
			t.Errorf("the files could be pruned %s, want them held as ReadDay holds them", when)
		}
	}
	f.opened = func(dayFile) { held("as the day's plain file was opened") }

	_, _, _, _ = ReadBack(f, date, asText, func(string) bool {
		held("as a line was handed on")
		return true
	})
}
