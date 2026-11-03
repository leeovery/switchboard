package export_test

import (
	"os"
	"slices"
	"testing"
	"time"
)

// keptCoarsely sets the time of the file at path to what a folder whose file
// system keeps times to the second, as FAT's 2, keeps of it, failing t where
// it can't.
func keptCoarsely(t *testing.T, path string, to time.Duration) {
	t.Helper()
	if err := os.Chtimes(path, time.Time{}, look(t, path).ModTime().Truncate(to)); err != nil {
		t.Fatal(err)
	}
}

func TestACopyInAFolderKeepingCoarserTimesIsPassedOver(t *testing.T) {
	for _, to := range []time.Duration{time.Second, 2 * time.Second} {
		t.Run(to.String(), func(t *testing.T) {
			state, dir := t.TempDir(), t.TempDir()
			data := gzipOf(t, linesOf(`{"n":1}`))
			writeFile(t, requests.compressed(state, day1), data, compressedAt)
			writeFile(t, summary(state, day1), []byte("{}\n"), summarisedAt.Add(1999*time.Millisecond))
			exportInto(t, state, dir)
			folder := folderOf(t, state, dir)
			for _, path := range []string{requests.compressed(folder, day1), summary(folder, day1)} {
				keptCoarsely(t, path, to)
			}
			was := look(t, requests.compressed(folder, day1))

			wantResult(t, exportInto(t, state, dir), state, dir, 0, 1)
			wantUnwritten(t, requests.compressed(folder, day1), was)
		})
	}
}

func TestACopyInAFolderKeepingCoarserTimesIsCopiedAgainOnceItChanges(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	writeFile(t, summary(state, day1), []byte(`{"n":1}`+"\n"), summarisedAt)
	exportInto(t, state, dir)
	folder := folderOf(t, state, dir)
	keptCoarsely(t, summary(folder, day1), 2*time.Second)

	// Summarised again, as long, a second for each version later.
	writeFile(t, summary(state, day1), []byte(`{"n":2}`+"\n"), summarisedAt.Add(3*time.Second))
	wantResult(t, exportInto(t, state, dir), state, dir, 1, 0)
	wantCopy(t, summary(folder, day1), []byte(`{"n":2}`+"\n"), summarisedAt.Add(3*time.Second))
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
