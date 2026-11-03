package export_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/export"
)

// unreadable makes the file at path one that can't be read, failing t where
// it can't.
func unreadable(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
}

// wantPassedOver fails t where an export of state, its 8th's plain file
// beside the 2nd's file that can't be read, doesn't pass the 2nd's over,
// naming it as want, why matching why where it's given, and export the rest,
// export.json among it.
func wantPassedOver(t *testing.T, state, want string, why error) {
	t.Helper()
	writeFile(t, requests.plain(state, day2), []byte(linesOf(`{"n":2}`)), appendedAt)

	r, err := exporter(state).Export(t.TempDir())
	if err != nil {
		t.Fatalf("Export() error = %v, want the 2nd passed over", err)
	}
	if len(r.Unread) != 1 || r.Unread[0].What != want || r.Unread[0].Date != day1 || r.Unread[0].Err == nil || why != nil && !errors.Is(r.Unread[0], why) {
		t.Errorf("Export() passed over %+v, want %s, saying why", r.Unread, want)
	}
	if r.Written != 1 || r.AsTheyWere != 0 {
		t.Errorf("Export() = %+v, want the 8th written", r)
	}
	if _, err := os.Stat(requests.compressed(r.Folder, day2)); err != nil {
		t.Errorf("the 8th's copy: %v, want it exported", err)
	}
	if _, err := export.ReadManifest(r.Folder); err != nil {
		t.Errorf("ReadManifest() error = %v, want export.json written", err)
	}
}

func TestACompressedDayThatCantBeReadIsPassedOverAndTheRestExported(t *testing.T) {
	state := t.TempDir()
	writeFile(t, requests.compressed(state, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
	unreadable(t, requests.compressed(state, day1))

	wantPassedOver(t, state, "the request ledger of "+day1, fs.ErrPermission)
}

func TestADayWhoseCompressedFilesHeaderIsDamagedIsPassedOverAndTheRestExported(t *testing.T) {
	state := t.TempDir()
	writeFile(t, kinds[1].compressed(state, day1), []byte("not gzip"), compressedAt)
	writeFile(t, kinds[1].plain(state, day1), []byte(linesOf(`{"n":1}`)), appendedAt)

	wantPassedOver(t, state, "the router's events of "+day1, nil)
}

func TestASummaryThatCantBeReadIsPassedOverAndTheRestExported(t *testing.T) {
	state := t.TempDir()
	writeFile(t, summary(state, day1), []byte("{}\n"), summarisedAt)
	unreadable(t, summary(state, day1))

	wantPassedOver(t, state, "the request ledger's summary of "+day1, fs.ErrPermission)
}

func TestADaysErrorSaysWhichAndWhy(t *testing.T) {
	e := export.DayError{What: "the request ledger of 2026-10-30", Date: "2026-10-30", Err: fs.ErrPermission}
	if got, want := e.Error(), "couldn't export the request ledger of 2026-10-30: permission denied"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestADayGoneBetweenItsLookAndItsReadIsPassedOver(t *testing.T) {
	tests := []struct {
		name string
		// files are the day's files, those of the path given with the data
		// given.
		files map[string][]byte
	}{
		{name: "a compressed day, copied", files: map[string][]byte{requests.compressed("", day1): gzipOf(t, linesOf(`{"n":1}`))}},
		{name: "a plain day, snapshotted", files: map[string][]byte{requests.plain("", day1): []byte(linesOf(`{"n":1}`))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, dir := t.TempDir(), t.TempDir()
			for path, data := range tt.files {
				writeFile(t, filepath.Join(state, path), data, compressedAt)
			}
			pruned := export.WithLooked(exporter(state), func(string) {
				for path := range tt.files {
					if err := os.Remove(filepath.Join(state, path)); err != nil {
						t.Error(err)
					}
				}
			})

			r, err := pruned.Export(dir)
			if err != nil || r.Written != 0 || len(r.Unread) > 0 {
				t.Fatalf("Export() = %+v, %v; want the day passed over", r, err)
			}
			if entries, err := os.ReadDir(filepath.Join(r.Folder, "ledger")); err == nil && len(entries) > 0 {
				t.Errorf("the folder's ledger holds %v, want nothing of the day", entries)
			}
		})
	}
}

func TestAWriteThatFailsStopsTheExportLeavingNothingHalfWritten(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	writeFile(t, requests.compressed(state, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
	writeFile(t, requests.plain(state, day2), []byte(linesOf(`{"n":2}`)), appendedAt)
	folder := folderOf(t, state, dir)
	// Where the day's copy goes, a folder that can't be renamed over.
	if err := os.MkdirAll(filepath.Join(requests.compressed(folder, day1), "in-the-way"), 0o700); err != nil {
		t.Fatal(err)
	}

	if r, err := exporter(state).Export(dir); err == nil {
		t.Fatalf("Export() = %+v, want it to fail", r)
	}
	entries, err := os.ReadDir(filepath.Join(folder, "ledger"))
	if err != nil || len(entries) != 1 {
		t.Errorf("the folder's ledger holds %v (%v), want the folder in the way alone: no file half written", entries, err)
	}
	if _, err := os.Stat(filepath.Join(folder, "export.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("export.json: %v, want none written", err)
	}
}
