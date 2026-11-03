package export_test

import (
	"os"
	"slices"
	"testing"
	"time"
)

// stampedAt is when the tests' summaries are stamped, to the nanosecond, as
// their compressed files' times stamp them.
var stampedAt = time.Date(2026, 10, 4, 1, 0, 1, 987_654_321, time.UTC)

// keptCoarsely sets the time of the file at path to what a folder whose file
// system keeps times to the precision given, by truncating them, keeps of
// it; none keeps it to the nanosecond.
func keptCoarsely(t *testing.T, path string, precision time.Duration) {
	t.Helper()
	if err := os.Chtimes(path, time.Time{}, look(t, path).ModTime().Truncate(precision)); err != nil {
		t.Fatal(err)
	}
}

// folders are the folders' file systems the tests keep copies on, by how
// finely each keeps their times.
var folders = []struct {
	name      string
	precision time.Duration
}{
	{name: "a folder keeping nanoseconds"},
	{name: "a folder keeping 10 milliseconds, as exFAT", precision: 10 * time.Millisecond},
	{name: "a folder keeping whole seconds", precision: time.Second},
}

func TestAnUnchangedCopyIsPassedOverHoweverFinelyItsFolderKeepsTimes(t *testing.T) {
	for _, f := range folders {
		t.Run(f.name, func(t *testing.T) {
			state, dir := t.TempDir(), t.TempDir()
			writeFile(t, requests.compressed(state, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
			writeFile(t, summary(state, day1), []byte(`{"version":6}`+"\n"), stampedAt)
			exportInto(t, state, dir)
			folder := folderOf(t, state, dir)
			copies := []string{requests.compressed(folder, day1), summary(folder, day1)}
			var was []os.FileInfo
			for _, path := range copies {
				keptCoarsely(t, path, f.precision)
				was = append(was, look(t, path))
			}

			wantResult(t, exportInto(t, state, dir), state, dir, 0, 1)
			for i, path := range copies {
				wantUnwritten(t, path, was[i])
			}
		})
	}
}

func TestASummaryOfTheNextVersionIsCopiedAgainHoweverFinelyItsFolderKeepsTimes(t *testing.T) {
	for _, f := range folders {
		t.Run(f.name, func(t *testing.T) {
			state, dir := t.TempDir(), t.TempDir()
			writeFile(t, summary(state, day1), []byte(`{"version":6}`+"\n"), stampedAt)
			exportInto(t, state, dir)
			folder := folderOf(t, state, dir)
			keptCoarsely(t, summary(folder, day1), f.precision)

			// Summarised again in the next version, as long, its stamp a second
			// later.
			next := []byte(`{"version":7}` + "\n")
			writeFile(t, summary(state, day1), next, stampedAt.Add(time.Second))
			wantResult(t, exportInto(t, state, dir), state, dir, 1, 0)
			wantCopy(t, summary(folder, day1), next, stampedAt.Add(time.Second))
		})
	}
}

func TestOnAFATFolderAnUnchangedCopyOfAnOddSecondIsWrittenAgainAtEachExport(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	// FAT keeps 01:00:01.98 as 01:00:00, which no truncation of it to a
	// precision the folder's time shows gives.
	writeFile(t, summary(state, day1), []byte(`{"version":6}`+"\n"), stampedAt)
	exportInto(t, state, dir)
	keptCoarsely(t, summary(folderOf(t, state, dir), day1), 2*time.Second)

	wantResult(t, exportInto(t, state, dir), state, dir, 1, 0)
}

func TestASnapshotInAFolderKeepingCoarserTimesIsPassedOverUntilThePlainFileChanges(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	writeFile(t, requests.plain(state, day2), []byte(linesOf(`{"n":1}`)), appendedAt)
	exportInto(t, state, dir)
	folder := folderOf(t, state, dir)
	copied := requests.compressed(folder, day2)
	keptCoarsely(t, copied, 2*time.Second)
	was := look(t, copied)

	wantResult(t, exportInto(t, state, dir), state, dir, 0, 1)
	wantUnwritten(t, copied, was)

	// A line appended within the same tick of the folder's clock.
	appendFile(t, requests.plain(state, day2), []byte(linesOf(`{"n":2}`)), appendedAt.Add(time.Nanosecond))
	wantResult(t, exportInto(t, state, dir), state, dir, 1, 0)
	if got, want := linesIn(t, requests.files(folder), day2), []string{`{"n":1}`, `{"n":2}`}; !slices.Equal(got, want) {
		t.Errorf("once a line is appended, the folder's copy holds %q, want %q", got, want)
	}
}

func TestASnapshotIsTakenAgainWhereThePlainFileChangesButNotItsSize(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	writeFile(t, requests.plain(state, day2), []byte(linesOf(`{"n":1}`)), appendedAt)
	exportInto(t, state, dir)

	writeFile(t, requests.plain(state, day2), []byte(linesOf(`{"n":2}`)), appendedAt.Add(time.Nanosecond))
	wantResult(t, exportInto(t, state, dir), state, dir, 1, 0)
	if got, want := linesIn(t, requests.files(folderOf(t, state, dir)), day2), []string{`{"n":2}`}; !slices.Equal(got, want) {
		t.Errorf("the folder's copy holds %q, want %q", got, want)
	}
}
