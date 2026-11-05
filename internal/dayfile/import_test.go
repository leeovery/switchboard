package dayfile

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// stagedOn is when the tests' staged files were last modified, as the Mac
// they came from left them.
var stagedOn = time.Date(2026, 9, 27, 9, 30, 0, 0, time.UTC)

// stage writes data as the file of the local day with the given date staged
// to import, last modified at stagedOn, in a directory of its own, and
// returns the day staged.
func stage(t *testing.T, date string, data []byte) stagedDay {
	t.Helper()
	path := filepath.Join(t.TempDir(), compressedFile(date).name("staged"))
	put(t, path, data)
	if err := os.Chtimes(path, time.Time{}, stagedOn); err != nil {
		t.Fatal(err)
	}
	day, _, _ := Day(date)
	return stagedDay{date: date, day: day, path: path}
}

// taking takes s into f at now, the files kept as long as keep says,
// failing the test where it fails.
func taking(t *testing.T, f *Files, s stagedDay, now time.Time, keep time.Duration) Imported {
	t.Helper()
	imported, err := f.take(s, now, keep)
	if err != nil {
		t.Fatalf("take(%s) error = %v", s.date, err)
	}
	return imported
}

// fileState is a file as it's written, and when it was last modified.
type fileState struct {
	data     string
	modified int64
}

// filesIn returns each file f's directory holds, by its name, as it is.
func filesIn(t *testing.T, f *Files) map[string]fileState {
	t.Helper()
	entries, err := os.ReadDir(f.Dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	files := make(map[string]fileState)
	for _, e := range entries {
		path := filepath.Join(f.Dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = fileState{data: string(data), modified: info.ModTime().UnixNano()}
	}
	return files
}

// lay writes the files of the local day with the given date: its compressed
// file of members, a gzip member each, where there are any, and its plain
// file of plain, where it holds anything.
func lay(t *testing.T, f *Files, date string, members []string, plain string) {
	t.Helper()
	if len(members) > 0 {
		writeDay(t, f, compressedFile(date), members...)
	}
	if plain != "" {
		writeDay(t, f, plainFile(date), plain)
	}
}

// exportOf returns the file of the local day with the given date as export
// writes it into a folder: the day's compressed file as it's written, where
// it's the day's only file; else the lines ReadDay reads of it that are
// JSON, compressed.
func exportOf(t *testing.T, f *Files, date string) []byte {
	t.Helper()
	if _, err := os.Stat(f.path(plainFile(date))); errors.Is(err, fs.ErrNotExist) {
		return written(t, f, compressedFile(date))
	}
	read, _, _ := wholeDay(f, date)
	return gzipOf(t, linesOf(read...))
}

// paddedLines returns numbered lines, each marked with mark, and each some
// two kilobytes long, to as many bytes as most, at least: a merge holds a hash
// of each line, as long whatever the line's length.
func paddedLines(mark string, most int) string {
	pad := strings.Repeat("x", 2000)
	var lines strings.Builder
	for n := 0; lines.Len() < most; n++ {
		fmt.Fprintf(&lines, `{"mark":%q,"n":%d,"pad":%q}`+"\n", mark, n, pad)
	}
	return lines.String()
}

func TestADayPastKeepingIsLeftOutOfAnImport(t *testing.T) {
	for _, tt := range keptInZones {
		t.Run(tt.name+", "+tt.zone, func(t *testing.T) {
			loc := zone(t, tt.zone)
			day, _, _ := dayIn(tt.date, loc)
			_, ends, _ := dayIn(tt.until, loc)
			for _, at := range []struct {
				now  time.Time
				want Taken
			}{{now: ends.Add(-time.Nanosecond), want: Added}, {now: ends, want: PastKeep}} {
				f := readingsHistory(t.TempDir())
				s := stage(t, tt.date, gzipOf(t, linesOf(`{"n":1}`)))
				s.day = day

				got := taking(t, f, s, at.now, tt.keep)
				if got != (Imported{Taken: at.want}) || holdsDay(f, tt.date) != (at.want == Added) {
					t.Errorf("taken in at %v, the day of %s kept %v, until %s, = %+v, the files holding it = %v; want %v, held only where it's taken in",
						at.now, tt.date, tt.keep, tt.until, got, holdsDay(f, tt.date), at.want)
				}
			}
		})
	}
}

func TestADayTheFilesHoldPastKeepingIsLeftAsItIs(t *testing.T) {
	// A day past keeping a prune spares, as the request ledger's without a
	// summary that stands.
	date := DateOf(start.Local().AddDate(0, 0, -30))
	f := requestLedger(t.TempDir())
	writeDay(t, f, compressedFile(date), linesOf(`{"n":1}`))
	before := filesIn(t, f)

	got := taking(t, f, stage(t, date, gzipOf(t, linesOf(`{"n":1}`, `{"n":2}`))), start, twoWeeks)
	if got != (Imported{Taken: PastKeep}) {
		t.Errorf("take(%s) = %+v, want it left out, past keeping", date, got)
	}
	if after := filesIn(t, f); !maps.Equal(after, before) {
		t.Errorf("the files changed, want the day past keeping left as it is")
	}
}

func TestADayKeptForeverIsNeverPastKeeping(t *testing.T) {
	for _, date := range []string{DateOf(start.Local().AddDate(0, 0, -401)), "0001-01-01"} {
		t.Run(date, func(t *testing.T) {
			f := readingsHistory(t.TempDir())

			got := taking(t, f, stage(t, date, gzipOf(t, linesOf(`{"n":1}`))), start, Forever)
			if got != (Imported{Taken: Added}) || heldIn(t, f, compressedFile(date)) != linesOf(`{"n":1}`) {
				t.Errorf("take(%s) kept forever = %+v, want the day added", date, got)
			}
		})
	}
}

func TestADayTheFilesDontHoldIsCopiedInOnceItsDoneWithElseAppendedToItsPlainFile(t *testing.T) {
	lines := linesOf(`{"n":1}`, `{"n":2}`)
	for _, tt := range doneInZones {
		t.Run(tt.name+", "+tt.zone, func(t *testing.T) {
			loc := zone(t, tt.zone)
			day, _, _ := dayIn(tt.date, loc)
			done, _, _ := dayIn(tt.done, loc)
			for _, at := range []struct {
				now time.Time
				// in is the day's file that holds the lines once they're taken
				// in, and gone the one that isn't there.
				in, gone dayFile
			}{
				{now: done.Add(-time.Nanosecond), in: plainFile(tt.date), gone: compressedFile(tt.date)},
				{now: done, in: compressedFile(tt.date), gone: plainFile(tt.date)},
			} {
				f := readingsHistory(t.TempDir())
				s := stage(t, tt.date, gzipOf(t, lines))
				s.day = day

				got := taking(t, f, s, at.now, twoWeeks)
				if got != (Imported{Taken: Added}) {
					t.Errorf("taken in at %v, the day of %s = %+v, want it added", at.now, tt.date, got)
				}
				if held := heldIn(t, f, at.in); held != lines {
					t.Errorf("taken in at %v, %s holds %q, want the day's lines %q", at.now, at.in.name(f.Prefix), held, lines)
				}
				if _, err := os.Stat(f.path(at.gone)); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("taken in at %v, %s is there (%v), want it not: the day done with from the start of %s", at.now, at.gone.name(f.Prefix), err, tt.done)
				}
			}
		})
	}
}

func TestADayCopiedInIsCopiedAsItIsWithItsModificationTime(t *testing.T) {
	const date = "2026-09-25"
	// The first member ends in a line cut short, and the second holds a line
	// that isn't JSON: the file reads to its end all the same.
	staged := gzipOf(t, linesOf(`{"n":1}`)+`{"n":`, linesOf("not json", `{"n":2}`))
	f := requestLedger(t.TempDir())

	taking(t, f, stage(t, date, staged), start, twoWeeks)
	files := filesIn(t, f)
	name := compressedFile(date).name(f.Prefix)
	if got := files[name]; got.data != string(staged) || got.modified != stagedOn.UnixNano() || len(files) != 1 {
		t.Errorf("the files are %v, want %s alone, the staged file byte for byte, modified at %v", slices.Sorted(maps.Keys(files)), name, stagedOn)
	}
	info, err := os.Stat(f.path(compressedFile(date)))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != 0o600 {
		t.Errorf("the day's compressed file is %v, want %v", info.Mode(), fs.FileMode(0o600))
	}
}

func TestADayTheFilesHoldGainsTheStagedLinesItDoesntHold(t *testing.T) {
	const date = "2026-09-25"
	one, two, three, four := `{"n":1}`, `{"n":2}`, `{"n":3}`, `{"n":4}`
	tests := []struct {
		name string
		// members are the day's compressed file's, none where it has none,
		// plain its plain file's lines, none where it has none, and staged
		// the lines staged.
		members []string
		plain   string
		staged  []string
		// in is the file the lines are added to, want the lines the day reads
		// after, as ReadDay reads them, and added how many were added.
		in    dayFile
		want  []string
		added int
	}{
		{
			name:  "appended to its plain file",
			plain: linesOf(one, three), staged: []string{one, two, three, four},
			in: plainFile(date), want: []string{one, three, two, four}, added: 2,
		},
		{
			name:    "a member of their own after its compressed file's",
			members: []string{linesOf(one), linesOf(three)}, staged: []string{one, two, three, four},
			in: compressedFile(date), want: []string{one, three, two, four}, added: 2,
		},
		{
			name:    "appended to its plain file beside its compressed one, as after the clock was set back to it",
			members: []string{linesOf(one)}, plain: linesOf(three), staged: []string{one, two, three, four},
			in: plainFile(date), want: []string{one, three, two, four}, added: 2,
		},
		{
			name:  "after a line cut short its plain file ends in",
			plain: linesOf(one) + `{"n":`, staged: []string{one, two},
			in: plainFile(date), want: []string{one, two}, added: 1,
		},
		{
			name:    "after a line cut short its compressed file ends in",
			members: []string{linesOf(one) + `{"n":`}, staged: []string{one, two},
			in: compressedFile(date), want: []string{one, two}, added: 1,
		},
		{
			name:  "once, of a line staged twice",
			plain: linesOf(one), staged: []string{one, two, two},
			in: plainFile(date), want: []string{one, two}, added: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())
			lay(t, f, date, tt.members, tt.plain)
			before := filesIn(t, f)

			got := taking(t, f, stage(t, date, gzipOf(t, linesOf(tt.staged...))), start, twoWeeks)
			if want := (Imported{Taken: Merged, Lines: tt.added}); got != want {
				t.Errorf("take(%s) = %+v, want %+v", date, got, want)
			}
			if read, _, _ := wholeDay(f, date); !slices.Equal(read, tt.want) {
				t.Errorf("the day reads %q, want %q: each line once", read, tt.want)
			}
			after, in := filesIn(t, f), tt.in.name(f.Prefix)
			if !slices.Equal(slices.Sorted(maps.Keys(after)), slices.Sorted(maps.Keys(before))) {
				t.Errorf("the files are %v, want those there before, %v", slices.Sorted(maps.Keys(after)), slices.Sorted(maps.Keys(before)))
			}
			for name, was := range before {
				if name != in && after[name] != was {
					t.Errorf("%s changed, want it as it was, the lines added to %s", name, in)
				}
			}
			if !strings.HasPrefix(after[in].data, before[in].data) {
				t.Errorf("%s doesn't begin as it was, want the lines added after what it held", in)
			}
			if tt.in.compressed && membersIn(t, []byte(after[in].data)) != len(tt.members)+1 {
				t.Errorf("%s holds %d members, want one more than its %d", in, membersIn(t, []byte(after[in].data)), len(tt.members))
			}
		})
	}
}

func TestAMemberAddedAfterACompressedFilesIsReadAndCompressedAfterOnce(t *testing.T) {
	f := requestLedger(t.TempDir())
	writeDay(t, f, compressedFile(earlierDate), linesOf(`{"n":1}`))
	taking(t, f, stage(t, earlierDate, gzipOf(t, linesOf(`{"n":1}`, `{"n":2}`))), start, twoWeeks)
	if read, lines, unread := wholeDay(f, earlierDate); !slices.Equal(read, []string{`{"n":1}`, `{"n":2}`}) || lines != 2 || unread != 0 {
		t.Errorf("the day reads %q, %d lines, %d unread; want its line and the one taken in, each once", read, lines, unread)
	}

	// Lines come to the day after, as after the clock was set back to it, a
	// compress stopping once it had written the compressed file, a line
	// appended after, and the next compress finishing it.
	appendingPlain(t, f, linesOf(`{"n":3}`))
	addingPlain(t, f)
	appendingPlain(t, f, linesOf(`{"n":4}`))
	compressingDay(t, f)
	if got, want := heldIn(t, f, compressedFile(earlierDate)), linesOf(`{"n":1}`, `{"n":2}`, `{"n":3}`, `{"n":4}`); got != want {
		t.Errorf("compressed after, the day's file holds %q, want %q: every line once", got, want)
	}
}

func TestTakingADayInAgainAddsNothing(t *testing.T) {
	const date = "2026-09-25"
	day, _, _ := Day(date)
	// The day isn't done with until the start of the third day after it.
	notDone := DayStart(day, 2)
	staged := linesOf(`{"n":1}`, `{"n":2}`)
	tests := []struct {
		name string
		// members and plain are the day's files' lines, as lay lays them, and
		// now when it's taken in.
		members []string
		plain   string
		now     time.Time
	}{
		{name: "a day copied in", now: start},
		{name: "a day appended to its plain file", now: notDone},
		{name: "a day of a plain file", plain: linesOf(`{"n":3}`), now: start},
		{name: "a day of a compressed file", members: []string{linesOf(`{"n":3}`)}, now: start},
		{name: "a day of both files", members: []string{linesOf(`{"n":3}`)}, plain: linesOf(`{"n":4}`), now: start},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())
			lay(t, f, date, tt.members, tt.plain)
			s := stage(t, date, gzipOf(t, staged))
			taking(t, f, s, tt.now, twoWeeks)
			before := filesIn(t, f)

			if got := taking(t, f, s, tt.now, twoWeeks); got != (Imported{Taken: Merged}) {
				t.Errorf("taken in again, the day = %+v, want it merged, adding nothing", got)
			}
			if after := filesIn(t, f); !maps.Equal(after, before) {
				t.Errorf("taken in again, the files changed, want them as they were")
			}
		})
	}
}

func TestTakingInADaysOwnExportAddsNothing(t *testing.T) {
	const date = "2026-09-25"
	tests := []struct {
		name string
		// members and plain are the day's files' lines, as lay lays them.
		members []string
		plain   string
	}{
		{name: "a day of a plain file, its last line cut short", plain: linesOf(`{"n":1}`, `{"n":2}`) + `{"n":3`},
		{name: "a day of a compressed file", members: []string{linesOf(`{"n":1}`, `{"n":`), linesOf("not json", `{"n":2}`)}},
		{name: "a day of both files", members: []string{linesOf(`{"n":1}`)}, plain: linesOf(`{"n":2}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())
			lay(t, f, date, tt.members, tt.plain)
			s := stage(t, date, exportOf(t, f, date))
			before := filesIn(t, f)

			if got := taking(t, f, s, start, twoWeeks); got != (Imported{Taken: Merged}) {
				t.Errorf("its own export taken in, the day = %+v, want it merged, adding nothing", got)
			}
			if after := filesIn(t, f); !maps.Equal(after, before) {
				t.Errorf("its own export taken in, the files changed, want them as they were")
			}
		})
	}
}

func TestAStagedLineThatIsntJSONIsntAdded(t *testing.T) {
	const date = "2026-09-25"
	day, _, _ := Day(date)
	// Lines that aren't JSON: one that never was, an empty one, one cut short
	// to a bare start of a line, then ended, as a compressed file holds one,
	// and one cut short at the file's end.
	staged := linesOf(`{"n":1}`, "not json", "", `{"n":`, `{"n":2}`) + `{"n":3`
	tests := []struct {
		name string
		// members are the day's compressed file's, none where it has none, and
		// now when it's taken in; in is the file the lines are added to, and
		// want what it holds after.
		members []string
		now     time.Time
		in      dayFile
		want    string
	}{
		{name: "appended to a plain file", now: DayStart(day, 2), in: plainFile(date), want: linesOf(`{"n":1}`, `{"n":2}`)},
		{name: "added as a member", members: []string{linesOf(`{"n":0}`)}, now: start, in: compressedFile(date), want: linesOf(`{"n":0}`, `{"n":1}`, `{"n":2}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())
			lay(t, f, date, tt.members, "")

			taking(t, f, stage(t, date, gzipOf(t, staged)), tt.now, twoWeeks)
			if got := heldIn(t, f, tt.in); got != tt.want {
				t.Errorf("%s holds %q, want %q: the staged lines that are JSON alone", tt.in.name(f.Prefix), got, tt.want)
			}
		})
	}
}

func TestADamagedStagedFileIsTakenInAsFarAsItReadsAndWarnedOf(t *testing.T) {
	const date = "2026-09-25"
	day, _, _ := Day(date)
	// A member whole, then one cut short.
	staged := append(gzipOf(t, linesOf(`{"n":1}`, `{"n":2}`)), gzipOf(t, linesOf(`{"n":3}`))[:5]...)
	tests := []struct {
		name string
		// members and plain are the day's files' lines, as lay lays them, and
		// now when it's taken in; want the lines the day reads after.
		members []string
		plain   string
		now     time.Time
		want    []string
	}{
		{name: "a day the files don't hold", now: DayStart(day, 2), want: []string{`{"n":1}`, `{"n":2}`}},
		{name: "a day the files don't hold, done with", now: start, want: []string{`{"n":1}`, `{"n":2}`}},
		{name: "a day of a plain file", plain: linesOf(`{"n":0}`), now: start, want: []string{`{"n":0}`, `{"n":1}`, `{"n":2}`}},
		{name: "a day of a compressed file", members: []string{linesOf(`{"n":0}`)}, now: start, want: []string{`{"n":0}`, `{"n":1}`, `{"n":2}`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			f := requestLedger(t.TempDir())
			lay(t, f, date, tt.members, tt.plain)
			s := stage(t, date, staged)

			taking(t, f, s, tt.now, twoWeeks)
			if read, _, _ := wholeDay(f, date); !slices.Equal(read, tt.want) {
				t.Errorf("the day reads %q, want %q: the staged lines before the damage taken in", read, tt.want)
			}
			if !log.Has("level=WARN", `msg="request ledger staged to import read short"`, "file="+s.path) || len(linesWith(log, "read short")) != 1 {
				t.Errorf("log reads\n%s\nwant the staged file warned of once, as reading short", log)
			}
		})
	}
}

func TestADamagedFileOfADayDoneWithIsTakenInAsItsLinesAndWholeLater(t *testing.T) {
	first, rest := linesOf(`{"n":1}`, `{"n":2}`), linesOf(`{"n":3}`)
	whole := gzipOf(t, first, rest)
	// Cut short partway through its second member, as a file half synced is.
	damaged := whole[:len(gzipOf(t, first))+5]
	f := requestLedger(t.TempDir())

	if got := taking(t, f, stage(t, earlierDate, damaged), start, twoWeeks); got != (Imported{Taken: Added}) {
		t.Errorf("the damaged file taken in, the day = %+v, want it added", got)
	}
	if got := heldIn(t, f, plainFile(earlierDate)); got != first || holdsCompressed(f, earlierDate) {
		t.Errorf("the damaged file taken in, the day's plain file holds %q, a compressed file there = %v; want the lines it read, %q, and no compressed file",
			got, holdsCompressed(f, earlierDate), first)
	}
	if got := taking(t, f, stage(t, earlierDate, whole), start, twoWeeks); got != (Imported{Taken: Merged, Lines: 1}) {
		t.Errorf("the whole file taken in after, the day = %+v, want it merged, gaining the line the damage left out", got)
	}
	compressingDay(t, f)
	if got := heldIn(t, f, compressedFile(earlierDate)); got != first+rest {
		t.Errorf("compressed after, the day's file holds %q, want %q: every line once", got, first+rest)
	}
}

func TestADayWhoseOwnCompressedFileIsDamagedIsPassedOverAndWarnedOf(t *testing.T) {
	log := logstest.Capture(t)
	f := requestLedger(t.TempDir())
	whole := gzipOf(t, linesOf(`{"n":1}`, `{"n":2}`))
	put(t, f.path(compressedFile(earlierDate)), whole[:len(whole)-4])
	was := filesIn(t, f)

	if got := taking(t, f, stage(t, earlierDate, gzipOf(t, linesOf(`{"n":1}`, `{"n":3}`))), start, twoWeeks); got != (Imported{}) {
		t.Errorf("take(%s) = %+v, want nothing taken in: no member can be added after the damage", earlierDate, got)
	}
	if got := filesIn(t, f); !maps.Equal(got, was) {
		t.Errorf("the files are %v, want them as they were, %v", got, was)
	}
	if !log.Has("level=WARN", `msg="can't import into a day of the request ledger whose own compressed file doesn't read; it's passed over"`, "day="+earlierDate) {
		t.Errorf("log reads\n%s\nwant the day passed over warned of, by its date", log)
	}
}

// holdsCompressed reports whether f holds a compressed file of the local day
// with the given date.
func holdsCompressed(f *Files, date string) bool {
	_, err := os.Stat(f.path(compressedFile(date)))
	return err == nil
}

func TestAStagedFileOfNoLinesTakesNothingIn(t *testing.T) {
	const date = "2026-09-25"
	day, _, _ := Day(date)
	tests := []struct {
		name string
		// data is the staged file, and now when it's taken in.
		data []byte
		now  time.Time
	}{
		{name: "an empty file, the day done with", data: []byte{}, now: start},
		{name: "an empty file, the day not yet done with", data: []byte{}, now: DayStart(day, 2)},
		{name: "a member of no lines, the day done with", data: gzipOf(t, ""), now: start},
		{name: "a member of no lines, the day not yet done with", data: gzipOf(t, ""), now: DayStart(day, 2)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := requestLedger(t.TempDir())

			if got := taking(t, f, stage(t, date, tt.data), tt.now, twoWeeks); got != (Imported{}) || holdsDay(f, date) {
				t.Errorf("take(%s) = %+v, the files holding the day = %v; want nothing taken in", date, got, holdsDay(f, date))
			}
		})
	}
}

func TestADayImportedAfterACompressStoppedPartwayHoldsEachLineOnceCompressed(t *testing.T) {
	f := requestLedger(t.TempDir())
	writeDay(t, f, compressedFile(earlierDate), linesOf(`{"n":1}`))
	appendingPlain(t, f, linesOf(`{"n":2}`))
	// The compress stops once it has written the compressed file, which then
	// ends with the plain file's lines, before it removes the plain one.
	addingPlain(t, f)

	got := taking(t, f, stage(t, earlierDate, gzipOf(t, linesOf(`{"n":1}`, `{"n":2}`, `{"n":3}`))), start, twoWeeks)
	if got != (Imported{Taken: Merged, Lines: 1}) {
		t.Errorf("take(%s) = %+v, want it merged, gaining the one line it didn't hold", earlierDate, got)
	}
	compressingDay(t, f)
	if got, want := heldIn(t, f, compressedFile(earlierDate)), linesOf(`{"n":1}`, `{"n":2}`, `{"n":3}`); got != want {
		t.Errorf("compressed after, the day's file holds %q, want %q: every line once", got, want)
	}
	if read, _, _ := wholeDay(f, earlierDate); !slices.Equal(read, []string{`{"n":1}`, `{"n":2}`, `{"n":3}`}) {
		t.Errorf("compressed after, the day reads %q, want every line once", read)
	}
}

func TestMergingABigDayNeverHoldsItWhole(t *testing.T) {
	// size is how long the day's lines and the staged lines each run to:
	// lines all different, as a day's are.
	const size = 8 << 20
	held, staged := paddedLines("held", size), paddedLines("staged", size)
	f := requestLedger(t.TempDir())
	writeDay(t, f, compressedFile(earlierDate), held)
	s := stage(t, earlierDate, gzipOf(t, staged))

	allocated := allocatedBy(t, func() { taking(t, f, s, start, twoWeeks) })
	if allocated > size/2 {
		t.Errorf("merging allocated %d MiB, want no more than %d: neither the day's lines nor the staged ones held whole", allocated>>20, (size/2)>>20)
	}
	if got, want := sumOf(t, f, compressedFile(earlierDate)), sha256.Sum256([]byte(held+staged)); got != want {
		t.Errorf("the day's compressed file gives other lines than its own and the staged ones, each once")
	}
}

func TestWorkHandedToAWriterRunsOnItsGoroutineBetweenItsWrites(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := readingsHistory(t.TempDir())
		w := testWriter(f, at(start), 4)
		stop := running(t, w)
		defer stop()
		today := plainFile(DateOf(start))
		var during []byte

		err := w.Do(t.Context(), func() {
			w.Note(noted{at: start, text: "noted"})
			// Time passes once every other goroutine waits: Run's would write
			// what's noted first, were it free to.
			time.Sleep(time.Minute)
			during, _ = os.ReadFile(f.path(today))
		})
		if err != nil {
			t.Fatalf("Do() = %v", err)
		}
		synctest.Wait()
		if len(during) > 0 {
			t.Errorf("as the work ran, the files gained %q, want nothing written until it was done, on Run's goroutine", during)
		}
		if got := heldIn(t, f, today); got != linesOf("noted") {
			t.Errorf("once the work was done, the files hold %q, want what was noted as it ran", got)
		}
	})
}

func TestALineNotedAsADayIsImportedIsNeitherLostNorRunIntoIt(t *testing.T) {
	const notes, imported = 200, 2000
	f := readingsHistory(t.TempDir())
	w := testWriter(f, at(start), notes)
	date := DateOf(start)
	var want, staged []string
	for n := range imported {
		staged = append(staged, fmt.Sprintf(`{"staged":%d,"pad":%q}`, n, strings.Repeat("x", 100)))
	}
	s := stage(t, date, gzipOf(t, linesOf(staged...)))
	stop := running(t, w)
	var noting sync.WaitGroup

	noting.Go(func() {
		for n := range notes {
			text := fmt.Sprintf(`{"noted":%d}`, n)
			want = append(want, text)
			w.Note(noted{at: start, text: text})
		}
	})
	if _, err := w.Import(t.Context(), date, s.path); err != nil {
		t.Fatalf("Import(%s) = %v", date, err)
	}
	noting.Wait()
	stop()
	want = append(want, staged...)
	read, _, unread := wholeDay(f, date)
	slices.Sort(read)
	slices.Sort(want)
	if !slices.Equal(read, want) || unread != 0 {
		t.Errorf("the day reads %d lines, %d unread; want the %d noted and the %d imported, each once, whole", len(read), unread, notes, imported)
	}
}

func TestAWriterImportsByItsOwnKeepAndClock(t *testing.T) {
	lines := linesOf(`{"n":1}`)
	tests := []struct {
		name string
		// back is how many days before the clock's the day is, and compressed
		// whether its lines are in its compressed file after, where it's added.
		back       int
		want       Taken
		compressed bool
	}{
		{name: "today's, appended", back: 0, want: Added},
		{name: "a day done with, copied in", back: 3, want: Added, compressed: true},
		{name: "a day past keeping, left out", back: 30, want: PastKeep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := readingsHistory(t.TempDir())
			w := testWriter(f, at(start), 1)
			date := DateOf(start.Local().AddDate(0, 0, -tt.back))
			s := stage(t, date, gzipOf(t, lines))
			stop := running(t, w)
			defer stop()

			got, err := w.Import(t.Context(), date, s.path)
			if err != nil || got != (Imported{Taken: tt.want}) {
				t.Fatalf("Import(%s) = %+v, %v; want %v", date, got, err, tt.want)
			}
			if held := holdsDay(f, date); held != (tt.want == Added) {
				t.Errorf("the files hold the day of %s = %v, want them to only where it's added", date, held)
			}
			if in := (dayFile{date: date, compressed: tt.compressed}); tt.want == Added && heldIn(t, f, in) != lines {
				t.Errorf("%s holds %q, want the day's lines", in.name(f.Prefix), heldIn(t, f, in))
			}
		})
	}
}

func TestWhatFollowsADayTakenInIsDoneWithItAsOne(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := readingsHistory(t.TempDir())
		w := testWriter(f, at(start), 1)
		stop := running(t, w)
		defer stop()
		date := DateOf(start)
		lines := linesOf(`{"n":1}`)
		s := stage(t, date, gzipOf(t, lines))
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		var held []byte
		var given Imported
		finished := make(chan struct{})

		_, err := w.ImportThen(ctx, date, s.path, func(took Imported) error {
			given = took
			held, _ = os.ReadFile(f.path(plainFile(date)))
			time.Sleep(2 * time.Minute)
			close(finished)
			return nil
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("ImportThen() = %v, want its context's end, which came first", err)
		}
		select {
		case <-finished:
		case <-time.After(time.Hour):
			t.Fatal("what follows the day taken in never finished, want it run to its end with the day")
		}
		if string(held) != lines || given != (Imported{Taken: Added}) {
			t.Errorf("as what follows ran, the day held %q, and it was given %+v; want the lines taken in, %q, and the day as added", held, given, lines)
		}
	})
}

func TestWhatFollowsADayTakenInFailsTheImport(t *testing.T) {
	f := readingsHistory(t.TempDir())
	w := testWriter(f, at(start), 1)
	stop := running(t, w)
	defer stop()
	date := DateOf(start)
	s := stage(t, date, gzipOf(t, linesOf(`{"n":1}`)))
	failed := errors.New("what follows failed")

	if _, err := w.ImportThen(t.Context(), date, s.path, func(Imported) error { return failed }); !errors.Is(err, failed) {
		t.Errorf("ImportThen() = %v, want what followed's error, %v", err, failed)
	}
}

func TestWhatFollowsADayIsntDoneWhereTheDayCantBeTakenIn(t *testing.T) {
	f := readingsHistory(t.TempDir())
	w := testWriter(f, at(start), 1)
	stop := running(t, w)
	defer stop()
	followed := false

	_, err := w.ImportThen(t.Context(), DateOf(start), filepath.Join(t.TempDir(), "gone.jsonl.gz"), func(Imported) error {
		followed = true
		return nil
	})
	if err == nil || followed {
		t.Errorf("ImportThen() of a file that isn't there = %v, what follows done = %v; want it to fail, doing none of it", err, followed)
	}
}

func TestImportingADayThatIsntADateFails(t *testing.T) {
	f := readingsHistory(t.TempDir())
	w := testWriter(f, at(start), 1)
	s := stage(t, DateOf(start), gzipOf(t, linesOf(`{"n":1}`)))

	if got, err := w.Import(t.Context(), "soon", s.path); err == nil {
		t.Errorf(`Import("soon") = %+v, want it to fail, as no date`, got)
	}
}

func TestRoundNowMakesTheWritersRoundAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var rounds []time.Time
		w := NewWriter(readingsHistory(t.TempDir()), linesNoted, WriterOptions{Queue: 1, Keep: twoWeeks, Now: at(start), Items: "readings",
			Round: func(now time.Time) { rounds = append(rounds, now) }})
		stop := running(t, w)
		defer stop()

		if err := w.RoundNow(t.Context()); err != nil {
			t.Fatalf("RoundNow() = %v", err)
		}
		if want := []time.Time{start, start}; !slices.EqualFunc(rounds, want, time.Time.Equal) {
			t.Errorf("rounds were made at %v, want %v: as the writer started, and again at once", rounds, want)
		}
	})
}

func TestTendNowMakesTheWritersRoundOnlyWhereItsFilesAreDueTheirPrune(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var now atomic.Pointer[time.Time]
		now.Store(&start)
		var rounds []time.Time
		w := NewWriter(readingsHistory(t.TempDir()), linesNoted, WriterOptions{Queue: 1, Keep: twoWeeks, Now: func() time.Time { return *now.Load() },
			Items: "readings", Round: func(at time.Time) { rounds = append(rounds, at) }})
		stop := running(t, w)
		defer stop()
		nextDay := start.AddDate(0, 0, 1)

		for _, at := range []time.Time{start, nextDay, nextDay} {
			now.Store(&at)
			if err := w.TendNow(t.Context()); err != nil {
				t.Fatalf("TendNow() at %v = %v", at, err)
			}
		}
		if want := []time.Time{start, nextDay}; !slices.EqualFunc(rounds, want, time.Time.Equal) {
			t.Errorf("rounds were made at %v, want %v: as the writer started, and on the next day alone", rounds, want)
		}
	})
}

func TestACallerWaitsForAWriterNotYetRunningAsLongAsItsContextAndNeverForOneStopped(t *testing.T) {
	calls := []struct {
		name string
		call func(ctx context.Context, w *Writer[noted], s stagedDay) error
	}{
		{name: "Do", call: func(ctx context.Context, w *Writer[noted], _ stagedDay) error { return w.Do(ctx, func() {}) }},
		{name: "RoundNow", call: func(ctx context.Context, w *Writer[noted], _ stagedDay) error { return w.RoundNow(ctx) }},
		{name: "Import", call: func(ctx context.Context, w *Writer[noted], s stagedDay) error {
			_, err := w.Import(ctx, s.date, s.path)
			return err
		}},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := readingsHistory(t.TempDir())
				rounds := 0
				w := NewWriter(f, linesNoted, WriterOptions{Queue: 1, Keep: twoWeeks, Now: at(start), Items: "readings", Round: func(time.Time) { rounds++ }})
				s := stage(t, DateOf(start), gzipOf(t, linesOf(`{"n":1}`)))
				calling := func(ctx context.Context, when string, want error, ran int) {
					t.Helper()
					if err := c.call(ctx, w, s); !errors.Is(err, want) {
						t.Errorf("%s, %s() = %v, want %v", when, c.name, err, want)
					}
					if rounds != ran || holdsDay(f, s.date) {
						t.Errorf("%s, %d rounds were made, the files holding the day = %v; want %d, and nothing taken in", when, rounds, holdsDay(f, s.date), ran)
					}
				}
				ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
				defer cancel()

				calling(ctx, "before Run starts", context.DeadlineExceeded, 0)
				running(t, w)()
				// Waiting for the test's own context would never end.
				calling(t.Context(), "once Run has stopped", ErrStopped, 1)
			})
		})
	}
}

func TestAWriterRunAgainTakesUpWorkAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := testWriter(readingsHistory(t.TempDir()), at(start), 1)
		running(t, w)()
		stop := running(t, w)
		defer stop()
		synctest.Wait()

		// A Run that left the stopped Run's end in place would turn away about
		// half of these, as Do then picks between it and the Run at random.
		for range 100 {
			done := false
			if err := w.Do(t.Context(), func() { done = true }); err != nil || !done {
				t.Fatalf("Do() = %v, the work done = %v; want it done by the Run started after the one that stopped", err, done)
			}
		}
	})
}

func TestACallerWhoseContextEndsAsItsWorkRunsIsToldSoAndTheWorkFinishes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := testWriter(readingsHistory(t.TempDir()), at(start), 1)
		stop := running(t, w)
		defer stop()
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		finished := make(chan struct{})

		err := w.Do(ctx, func() {
			time.Sleep(2 * time.Minute)
			close(finished)
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Do() = %v, want its context's end, which came first", err)
		}
		select {
		case <-finished:
		case <-time.After(time.Hour):
			t.Error("the work never finished, want it run to its end once taken up")
		}
	})
}

func TestWorkDoneAsItsCallersContextEndsIsReportedDone(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	done := make(chan struct{})
	close(done)
	// A choice between the two made at random would make the other in a few
	// tries.
	for range 100 {
		if err := doneBy(ctx, done); err != nil {
			t.Fatalf("doneBy() = %v, the work done and the context ended both; want it reported done", err)
		}
	}
}

func TestWorkHandedOnWithAContextEndedIsntDone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := testWriter(readingsHistory(t.TempDir()), at(start), 1)
		stop := running(t, w)
		defer stop()
		synctest.Wait()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		if err := w.Do(ctx, func() { t.Error("the work ran, want it not, its context ended") }); !errors.Is(err, context.Canceled) {
			t.Errorf("Do() = %v, want its context's end", err)
		}
	})
}
