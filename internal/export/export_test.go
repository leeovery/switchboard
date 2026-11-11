package export_test

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// exportNow is when the tests' exports run, by their clock.
var exportNow = time.Date(2026, 10, 9, 14, 42, 7, 500, time.UTC)

// The times the tests' files were last modified.
var (
	compressedAt = time.Date(2026, 10, 3, 1, 0, 1, 750_000_123, time.UTC)
	appendedAt   = time.Date(2026, 10, 8, 23, 59, 0, 456, time.UTC)
	summarisedAt = time.Date(2026, 10, 4, 1, 0, 1, 0, time.UTC)
)

// kind is one kind of file of lines a day: the directory it's in, in the
// state directory and in a Mac's folder alike, and the prefix its files'
// names begin with.
type kind struct {
	name, dir, prefix string
}

// kinds are every kind of file of lines a day an export exports.
var kinds = []kind{
	{name: "the request ledger", dir: "ledger", prefix: "requests"},
	{name: "the router's events", dir: "ledger", prefix: "events"},
	{name: "the readings history", dir: "history", prefix: "readings"},
}

// plain is where the plain file of the day with the given date is, of the
// kind k, in root, a state directory or a Mac's folder.
func (k kind) plain(root, date string) string {
	return filepath.Join(root, k.dir, k.prefix+"-"+date+".jsonl")
}

// compressed is where the compressed file of the day with the given date is,
// of the kind k, in root.
func (k kind) compressed(root, date string) string {
	return k.plain(root, date) + ".gz"
}

// files are the files of the kind k in root, as the package that keeps them
// gives them.
func (k kind) files(root string) *dayfile.Files {
	logger := slog.New(slog.DiscardHandler)
	switch k.prefix {
	case "events":
		return events.Files(ledger.Dir(root), logger)
	case "readings":
		return readings.Files(readings.Dir(root), logger)
	}
	return ledger.Files(ledger.Dir(root), logger)
}

// requests are the request ledger's files, the kind most tests export.
var requests = kinds[0]

// summary is where the summary of the day with the given date is in root.
func summary(root, date string) string {
	return filepath.Join(root, "ledger", "day-"+date+".json")
}

// exporter exports the history in the state directory stateDir, by the
// tests' clock.
func exporter(stateDir string) export.Exporter {
	return export.Exporter{StateDir: stateDir, Now: func() time.Time { return exportNow }, Logger: slog.New(slog.DiscardHandler)}
}

// exportInto exports the history in stateDir into dir, failing t where it
// can't, and returns what it did.
func exportInto(t *testing.T, stateDir, dir string) export.Result {
	t.Helper()
	r, err := exporter(stateDir).Export(dir)
	if err != nil {
		t.Fatalf("Export(%s) error = %v", dir, err)
	}
	return r
}

// wantResult fails t where got isn't an export into the folder of stateDir's
// install in dir that wrote the days given and left those given as they were.
func wantResult(t *testing.T, got export.Result, stateDir, dir string, written, asTheyWere int) {
	t.Helper()
	want := export.Result{Folder: folderOf(t, stateDir, dir), Written: written, AsTheyWere: asTheyWere}
	if got.Folder != want.Folder || got.Written != want.Written || got.AsTheyWere != want.AsTheyWere || len(got.Unread) > 0 {
		t.Errorf("Export() = %+v, want %+v", got, want)
	}
}

// folderOf is the folder of stateDir's install in dir.
func folderOf(t *testing.T, stateDir, dir string) string {
	t.Helper()
	id, err := export.InstallID(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	return export.Folder(dir, id)
}

// writeFile writes data to the file at path, making its directory, last
// modified at modified.
func writeFile(t *testing.T, path string, data []byte, modified time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Time{}, modified); err != nil {
		t.Fatal(err)
	}
}

// appendFile appends data to the file at path, last modified at modified
// after.
func appendFile(t *testing.T, path string, data []byte, modified time.Time) {
	t.Helper()
	held, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, append(held, data...), modified)
}

// gzipOf is text compressed, as a gzip member.
func gzipOf(t *testing.T, text string) []byte {
	t.Helper()
	var member bytes.Buffer
	w := gzip.NewWriter(&member)
	if _, err := w.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return member.Bytes()
}

// linesOf is lines, each ended.
func linesOf(lines ...string) string {
	var text strings.Builder
	for _, line := range lines {
		text.WriteString(line + "\n")
	}
	return text.String()
}

// linesIn are the lines files hold of the day with the given date, as
// dayfile reads them back.
func linesIn(t *testing.T, files *dayfile.Files, date string) []string {
	t.Helper()
	var lines []string
	_, _, err := dayfile.ReadDay(files, date, func(line []byte) (string, bool) { return string(line), true }, func(line string) bool {
		lines = append(lines, line)
		return true
	})
	if err != nil {
		t.Fatalf("ReadDay(%s, %s) error = %v", files.Dir, date, err)
	}
	return lines
}

// look returns a look at the file at path, failing t where it can't.
func look(t *testing.T, path string) fs.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// wantCopy fails t where the file at path doesn't hold data, the user's
// alone, last modified at modified.
func wantCopy(t *testing.T, path string, data []byte, modified time.Time) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Errorf("%s holds %q (%v), want %q", path, got, err, data)
	}
	if info := look(t, path); !info.ModTime().Equal(modified) || info.Mode().Perm() != 0o600 {
		t.Errorf("%s is modified at %v, %v; want %v, 0600", path, info.ModTime(), info.Mode().Perm(), modified)
	}
}

// wantUnwritten fails t where the file at path isn't the file was looked at.
func wantUnwritten(t *testing.T, path string, was fs.FileInfo) {
	t.Helper()
	if now := look(t, path); !dayfile.SameAs(was, now) {
		t.Errorf("%s was written again, want it left as it was", path)
	}
}

const (
	day1 = "2026-10-02"
	day2 = "2026-10-08"
)

func TestACompressedDayIsCopiedByteForByteWithItsTimeThenPassedOver(t *testing.T) {
	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			state, dir := t.TempDir(), t.TempDir()
			data := gzipOf(t, linesOf(`{"n":1}`, `{"n":2}`))
			writeFile(t, k.compressed(state, day1), data, compressedAt)

			wantResult(t, exportInto(t, state, dir), state, dir, 1, 0)
			copied := k.compressed(folderOf(t, state, dir), day1)
			wantCopy(t, copied, data, compressedAt)
			was := look(t, copied)

			wantResult(t, exportInto(t, state, dir), state, dir, 0, 1)
			wantUnwritten(t, copied, was)
		})
	}
}

func TestACompressedDayIsCopiedAgainOnceItChanges(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		modified time.Time
	}{
		{name: "another size", data: gzipOf(t, linesOf(`{"n":1}`, `{"n":2}`, `{"n":3}`)), modified: compressedAt},
		{name: "another time", data: gzipOf(t, linesOf(`{"n":1}`, `{"n":2}`)), modified: compressedAt.Add(3 * time.Second)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, dir := t.TempDir(), t.TempDir()
			writeFile(t, requests.compressed(state, day1), gzipOf(t, linesOf(`{"n":1}`, `{"n":2}`)), compressedAt)
			exportInto(t, state, dir)
			writeFile(t, requests.compressed(state, day1), tt.data, tt.modified)

			wantResult(t, exportInto(t, state, dir), state, dir, 1, 0)
			wantCopy(t, requests.compressed(folderOf(t, state, dir), day1), tt.data, tt.modified)
		})
	}
}

func TestAPlainDayIsCompressedThenPassedOverUntilALineIsAppended(t *testing.T) {
	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			state, dir := t.TempDir(), t.TempDir()
			writeFile(t, k.plain(state, day2), []byte(linesOf(`{"n":1}`, `{"n":2}`)), appendedAt)

			wantResult(t, exportInto(t, state, dir), state, dir, 1, 0)
			folder := folderOf(t, state, dir)
			copied := k.compressed(folder, day2)
			if got, want := linesIn(t, k.files(folder), day2), []string{`{"n":1}`, `{"n":2}`}; !slices.Equal(got, want) {
				t.Errorf("the folder's copy holds %q, want %q", got, want)
			}
			if info := look(t, copied); !info.ModTime().Equal(appendedAt) || info.Mode().Perm() != 0o600 {
				t.Errorf("the folder's copy is modified at %v, %v; want the plain file's time, %v, 0600", info.ModTime(), info.Mode().Perm(), appendedAt)
			}
			if _, err := os.Stat(k.plain(folder, day2)); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("the folder holds the day's plain file (%v), want its compressed file alone", err)
			}
			was := look(t, copied)

			wantResult(t, exportInto(t, state, dir), state, dir, 0, 1)
			wantUnwritten(t, copied, was)

			appended := appendedAt.Add(time.Minute)
			appendFile(t, k.plain(state, day2), []byte(linesOf(`{"n":3}`)), appended)
			wantResult(t, exportInto(t, state, dir), state, dir, 1, 0)
			if got, want := linesIn(t, k.files(folder), day2), []string{`{"n":1}`, `{"n":2}`, `{"n":3}`}; !slices.Equal(got, want) {
				t.Errorf("once a line is appended, the folder's copy holds %q, want %q", got, want)
			}
			if info := look(t, copied); !info.ModTime().Equal(appended) {
				t.Errorf("once a line is appended, the folder's copy is modified at %v, want %v", info.ModTime(), appended)
			}
		})
	}
}

func TestADayWithBothFilesIsCompressedWithEachLineOnce(t *testing.T) {
	tests := []struct {
		name              string
		compressed, plain string
		want              []string
	}{
		{
			name:       "a compress stopped before it removed the plain file",
			compressed: linesOf(`{"n":1}`, `{"n":2}`),
			plain:      linesOf(`{"n":1}`, `{"n":2}`),
			want:       []string{`{"n":1}`, `{"n":2}`},
		},
		{
			name:       "lines appended after the day was compressed, as the clock set back appends them",
			compressed: linesOf(`{"n":1}`, `{"n":2}`),
			plain:      linesOf(`{"n":3}`),
			want:       []string{`{"n":1}`, `{"n":2}`, `{"n":3}`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, dir := t.TempDir(), t.TempDir()
			writeFile(t, requests.compressed(state, day1), gzipOf(t, tt.compressed), compressedAt)
			writeFile(t, requests.plain(state, day1), []byte(tt.plain), appendedAt)

			wantResult(t, exportInto(t, state, dir), state, dir, 1, 0)
			folder := folderOf(t, state, dir)
			if got := linesIn(t, requests.files(folder), day1); !slices.Equal(got, tt.want) {
				t.Errorf("the folder's copy holds %q, want %q", got, tt.want)
			}
			if info := look(t, requests.compressed(folder, day1)); !info.ModTime().Equal(appendedAt) {
				t.Errorf("the folder's copy is modified at %v, want the plain file's time, %v", info.ModTime(), appendedAt)
			}
		})
	}
}

func TestALineCutShortAtThePlainFilesEndIsLeftForTheNextExport(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	writeFile(t, requests.plain(state, day2), []byte(linesOf(`{"n":1}`, `{"n":2}`)+`{"n":`), appendedAt)

	exportInto(t, state, dir)
	folder := folderOf(t, state, dir)
	if got, want := linesIn(t, requests.files(folder), day2), []string{`{"n":1}`, `{"n":2}`}; !slices.Equal(got, want) {
		t.Errorf("the folder's copy holds %q, want %q, the line cut short left out", got, want)
	}

	appendFile(t, requests.plain(state, day2), []byte("3}\n"), appendedAt.Add(time.Second))
	exportInto(t, state, dir)
	if got, want := linesIn(t, requests.files(folder), day2), []string{`{"n":1}`, `{"n":2}`, `{"n":3}`}; !slices.Equal(got, want) {
		t.Errorf("once the line is whole, the folder's copy holds %q, want %q", got, want)
	}
}

func TestADaysSummaryIsCopiedWithItsStampThenPassedOver(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	data := []byte(`{"version":1,"day":"2026-10-02"}` + "\n")
	writeFile(t, summary(state, day1), data, summarisedAt)

	wantResult(t, exportInto(t, state, dir), state, dir, 1, 0)
	copied := summary(folderOf(t, state, dir), day1)
	wantCopy(t, copied, data, summarisedAt)
	was := look(t, copied)

	wantResult(t, exportInto(t, state, dir), state, dir, 0, 1)
	wantUnwritten(t, copied, was)

	stamped := summarisedAt.Add(time.Hour)
	writeFile(t, summary(state, day1), data, stamped)
	wantResult(t, exportInto(t, state, dir), state, dir, 1, 0)
	wantCopy(t, copied, data, stamped)
}

func TestADayIsCountedOnceWhateverFilesItHas(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	for _, k := range kinds {
		writeFile(t, k.compressed(state, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
	}
	writeFile(t, summary(state, day1), []byte("{}\n"), summarisedAt)
	writeFile(t, requests.plain(state, day2), []byte(linesOf(`{"n":2}`)), appendedAt)
	exportInto(t, state, dir)

	// Of day1, only its summary changes; day2 is as it was.
	writeFile(t, summary(state, day1), []byte("{}\n"), summarisedAt.Add(time.Hour))
	wantResult(t, exportInto(t, state, dir), state, dir, 1, 1)
}

func TestNothingInTheFolderIsRemoved(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	data := gzipOf(t, linesOf(`{"n":1}`))
	writeFile(t, requests.compressed(state, day1), data, compressedAt)
	writeFile(t, summary(state, day1), []byte("{}\n"), summarisedAt)
	writeFile(t, requests.plain(state, day2), []byte(linesOf(`{"n":2}`)), appendedAt)
	exportInto(t, state, dir)
	folder := folderOf(t, state, dir)
	writeFile(t, filepath.Join(folder, "ledger", "notes.txt"), []byte("mine\n"), appendedAt)

	// day1 goes past keeping, its lines pruned.
	if err := os.Remove(requests.compressed(state, day1)); err != nil {
		t.Fatal(err)
	}
	wantResult(t, exportInto(t, state, dir), state, dir, 0, 2)

	wantCopy(t, requests.compressed(folder, day1), data, compressedAt)
	for _, path := range []string{summary(folder, day1), requests.compressed(folder, day2), filepath.Join(folder, "ledger", "notes.txt")} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s: %v, want it kept", path, err)
		}
	}
}

func TestTheExportedFilesReadBackAsTheStateDirectorysLines(t *testing.T) {
	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			state, dir := t.TempDir(), t.TempDir()
			writeFile(t, k.compressed(state, day1), append(gzipOf(t, linesOf(`{"n":1}`)), gzipOf(t, linesOf(`{"n":2}`))...), compressedAt)
			writeFile(t, k.plain(state, day2), []byte(linesOf(`{"n":3}`, `{"n":4}`)), appendedAt)

			exportInto(t, state, dir)

			folder := folderOf(t, state, dir)
			for _, date := range []string{day1, day2} {
				if got, want := linesIn(t, k.files(folder), date), linesIn(t, k.files(state), date); !slices.Equal(got, want) {
					t.Errorf("of %s, the folder's copy reads back as %q, want the state directory's %q", date, got, want)
				}
			}
		})
	}
}

func TestTwoExportsAtOnceIntoOneFolderLeaveItWhole(t *testing.T) {
	state := t.TempDir()
	for _, k := range kinds {
		writeFile(t, k.compressed(state, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
		writeFile(t, k.plain(state, day2), []byte(linesOf(`{"n":2}`, `{"n":3}`)), appendedAt)
	}
	writeFile(t, summary(state, day1), []byte(`{"date":"`+day1+`"}`+"\n"), summarisedAt)

	// Each time, two exports, as the router's and one run by hand, race
	// into a folder of their own.
	for range 10 {
		dir := t.TempDir()
		errs := make(chan error, 2)
		var exporting sync.WaitGroup
		for range 2 {
			exporting.Go(func() {
				_, err := exporter(state).Export(dir)
				errs <- err
			})
		}
		exporting.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("Export() beside another: %v", err)
			}
		}

		folder := folderOf(t, state, dir)
		for _, k := range kinds {
			for _, date := range []string{day1, day2} {
				if got, want := linesIn(t, k.files(folder), date), linesIn(t, k.files(state), date); !slices.Equal(got, want) {
					t.Errorf("of %s, %s's copy reads back as %q, want the state directory's %q", date, k.name, got, want)
				}
			}
		}
		if got, err := os.ReadFile(summary(folder, day1)); err != nil || string(got) != `{"date":"`+day1+`"}`+"\n" {
			t.Errorf("the summary's copy reads %q (%v), want the state directory's", got, err)
		}
		if _, err := export.ReadManifest(folder); err != nil {
			t.Errorf("ReadManifest() = %v, want export.json whole", err)
		}
		wantNothingHidden(t, folder)
	}
}

// wantNothingHidden fails t where the folder at root holds a hidden file, as
// a write cut short leaves beside where it goes.
func wantNothingHidden(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasPrefix(d.Name(), ".") {
			t.Errorf("%s is left in the folder", path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTheMacsFolderIsMadeTheUsersAlone(t *testing.T) {
	state, dir := t.TempDir(), filepath.Join(t.TempDir(), "backups")
	writeFile(t, requests.compressed(state, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
	writeFile(t, kinds[2].plain(state, day2), []byte(linesOf(`{"n":2}`)), appendedAt)

	exportInto(t, state, dir)

	folder := folderOf(t, state, dir)
	for _, d := range []string{dir, folder, filepath.Join(folder, "ledger"), filepath.Join(folder, "history")} {
		if info := look(t, d); !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Errorf("%s is %v, want a folder, 0700", d, info.Mode())
		}
	}
	if got := look(t, filepath.Join(folder, "export.json")).Mode().Perm(); got != 0o600 {
		t.Errorf("export.json is %v, want 0600", got)
	}
}

func TestExportJSONSaysWhoseFolderItIsAndWhenItWasExported(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	r := exportInto(t, state, dir)
	id, err := export.InstallID(state)
	if err != nil {
		t.Fatal(err)
	}

	got, err := export.ReadManifest(r.Folder)
	if want := exportNow.Truncate(time.Second); err != nil || got.Version != 2 || got.Install != id || !got.ExportedAt.Equal(want) {
		t.Errorf("ReadManifest() = %+v, %v; want version 2, install %s, exported at %v", got, err, id, want)
	}
	data, err := os.ReadFile(filepath.Join(r.Folder, "export.json"))
	if want := `{"version":2,"install":"` + id + `","exported_at":"2026-10-09T14:42:07Z","ids":true}` + "\n"; err != nil || string(data) != want {
		t.Errorf("export.json holds %q (%v), want %q", data, err, want)
	}
}

// An export into a Mac's folder an earlier switchboard exported removes its
// export.json, of an older version, before it writes a day, so an export
// cut short leaves no day naming the ids made beside one that version
// imports; one of this version stays as it was.
func TestAnExportCutShortLeavesNoExportJSONOfAnOlderVersionBesideItsDays(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version int
		wantOld bool
	}{
		{name: "an older version's, removed", version: export.Version - 1},
		{name: "this version's, left", version: export.Version, wantOld: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, dir := t.TempDir(), t.TempDir()
			writeFile(t, requests.compressed(state, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
			writeFile(t, requests.compressed(state, day2), gzipOf(t, linesOf(`{"n":2}`)), compressedAt)
			id, err := export.InstallID(state)
			if err != nil {
				t.Fatal(err)
			}
			folder := folderOf(t, state, dir)
			old := `{"version":` + strconv.Itoa(tc.version) + `,"install":"` + id + `","exported_at":"2026-10-01T09:00:00Z","ids":true}` + "\n"
			writeFile(t, filepath.Join(folder, "export.json"), []byte(old), compressedAt)
			ledgerDir := filepath.Join(folder, "ledger")
			t.Cleanup(func() { _ = os.Chmod(ledgerDir, 0o700) })
			looked := 0
			// The export is cut short at the second day, its folder no
			// longer written to.
			x := export.WithLooked(exporter(state), func(string) {
				if looked++; looked == 2 {
					if err := os.Chmod(ledgerDir, 0o500); err != nil {
						t.Error(err)
					}
				}
			})

			if _, err := x.Export(dir); err == nil {
				t.Fatal("Export() = nil, want it cut short at the second day")
			}
			if _, err := os.Stat(requests.compressed(folder, day1)); err != nil {
				t.Errorf("the first day: %v, want it exported", err)
			}
			data, err := os.ReadFile(filepath.Join(folder, "export.json"))
			if got := err == nil && string(data) == old; got != tc.wantOld || err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("cut short, the folder's export.json holds %q (%v), want the one there before %v", data, err, tc.wantOld)
			}
		})
	}
}

func TestExportJSONIsRewrittenAtEachExport(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	exportInto(t, state, dir)
	later := exportNow.Add(time.Hour).In(time.FixedZone("ahead", 9*60*60))

	r, err := export.Exporter{StateDir: state, Now: func() time.Time { return later }, Logger: slog.New(slog.DiscardHandler)}.Export(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := export.ReadManifest(r.Folder); err != nil || !got.ExportedAt.Equal(later.Truncate(time.Second)) || got.ExportedAt.Location() != time.UTC {
		t.Errorf("ReadManifest() = %+v, %v; want exported at %v, in UTC", got, err, later)
	}
}

func TestAFolderWithoutExportJSONHasNoManifest(t *testing.T) {
	if _, err := export.ReadManifest(t.TempDir()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadManifest() of a folder without one: %v, want fs.ErrNotExist", err)
	}
}

func TestAnExportJSONThatDoesntReadHasNoManifest(t *testing.T) {
	tests := []struct {
		name, data string
	}{
		{name: "not JSON", data: "not json\n"},
		{name: "an install that isn't an id", data: `{"version":1,"install":"work","exported_at":"2026-10-09T14:42:07Z"}`},
		{name: "no install", data: `{"version":1,"exported_at":"2026-10-09T14:42:07Z"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folder := t.TempDir()
			writeFile(t, filepath.Join(folder, "export.json"), []byte(tt.data), exportNow)

			if got, err := export.ReadManifest(folder); err == nil || errors.Is(err, fs.ErrNotExist) {
				t.Errorf("ReadManifest() of %q = %+v, %v; want it to fail, saying why", tt.data, got, err)
			}
		})
	}
}

func TestExportingIntoAFileFails(t *testing.T) {
	state := t.TempDir()
	dir := filepath.Join(t.TempDir(), "backups")
	writeFile(t, dir, []byte("a file\n"), exportNow)

	_, err := exporter(state).Export(dir)
	if err == nil || !strings.Contains(err.Error(), dir+": it's a file") {
		t.Errorf("Export() into a file: %v, want it to fail, saying it's a file", err)
	}
}

func TestExportingIntoAFolderThatCantBeWrittenFails(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err := exporter(state).Export(dir)
	if !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "can't export into "+dir) {
		t.Errorf("Export() into a folder that can't be written: %v, want it to fail, saying so", err)
	}
}

func TestExportingIntoAFolderThatCantBeMadeFails(t *testing.T) {
	state := t.TempDir()
	file := filepath.Join(t.TempDir(), "a-file")
	writeFile(t, file, []byte("a file\n"), exportNow)
	dir := filepath.Join(file, "backups")

	_, err := exporter(state).Export(dir)
	if err == nil || !strings.Contains(err.Error(), "can't export into "+dir) {
		t.Errorf("Export() into a folder that can't be made: %v, want it to fail, saying so", err)
	}
}

func TestAFolderMissingIsMadeWhereTheFolderItGoesInIsThere(t *testing.T) {
	for _, noParents := range []bool{false, true} {
		t.Run(fmt.Sprintf("no parents %v", noParents), func(t *testing.T) {
			state, dir := t.TempDir(), filepath.Join(t.TempDir(), "backups")
			writeFile(t, requests.compressed(state, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
			e := exporter(state)
			e.NoParents = noParents

			r, err := e.Export(dir)
			if err != nil {
				t.Fatalf("Export(%s) error = %v", dir, err)
			}
			wantResult(t, r, state, dir, 1, 0)
			if info := look(t, dir); !info.IsDir() || info.Mode().Perm() != 0o700 {
				t.Errorf("%s is %v, want a folder, 0700", dir, info.Mode())
			}
		})
	}
}

func TestAFolderWhoseFolderIsntThereIsMadeWithItWithoutNoParents(t *testing.T) {
	state, dir := t.TempDir(), filepath.Join(t.TempDir(), "unmounted", "backups")

	r := exportInto(t, state, dir)

	wantResult(t, r, state, dir, 0, 0)
}

func TestAFolderWhoseFolderIsntThereFailsWithNoParentsWritingNothing(t *testing.T) {
	state, parent := t.TempDir(), filepath.Join(t.TempDir(), "unmounted")
	dir := filepath.Join(parent, "backups")
	writeFile(t, requests.compressed(state, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
	e := exporter(state)
	e.NoParents = true

	_, err := e.Export(dir)
	if !errors.Is(err, export.ErrNoFolder) || err.Error() != "can't export into "+dir+": it isn't there, nor the folder it goes in" {
		t.Errorf("Export() into a folder whose folder isn't there: %v, want it to fail, saying the folder isn't there", err)
	}
	if _, err := os.Lstat(parent); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after the export, %s: %v, want it not there", parent, err)
	}
	if _, err := os.Lstat(filepath.Join(state, "install-id")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after the export, the install's id: %v, want none made", err)
	}
}

func TestAFolderGoneAsTheMacsFolderIsMadeIsNamedOnceWithNoParents(t *testing.T) {
	state, root := t.TempDir(), t.TempDir()
	// A link to a disk not mounted is there to make, and gone to make in.
	dir := filepath.Join(root, "Backup")
	if err := os.Symlink(filepath.Join(root, "unmounted"), dir); err != nil {
		t.Fatal(err)
	}
	e := exporter(state)
	e.NoParents = true

	_, err := e.Export(dir)
	if want := "can't export into " + dir + ": it isn't there"; !errors.Is(err, export.ErrNoFolder) || err.Error() != want {
		t.Errorf("Export() into a folder gone as the Mac's folder is made: %v, want %q", err, want)
	}
}

// readOnly makes the folder at dir one that can't be written, as /Volumes,
// which only the system writes, is to the user, until the test ends.
func readOnly(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

func TestAFolderThatCantBeMadeWhereItGoesFailsWithNoParentsSayingItIsntThere(t *testing.T) {
	state, volumes := t.TempDir(), t.TempDir()
	dir := filepath.Join(volumes, "Backup")
	readOnly(t, volumes)
	e := exporter(state)
	e.NoParents = true

	_, err := e.Export(dir)
	want := "can't export into " + dir + ": it isn't there, and can't be made: mkdir " + dir + ": permission denied"
	if !errors.Is(err, export.ErrNoFolder) || err.Error() != want {
		t.Errorf("Export() into a folder that can't be made where it goes: %v, want %q", err, want)
	}
	if _, err := os.Lstat(filepath.Join(state, "install-id")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after the export, the install's id: %v, want none made", err)
	}
}

func TestAFolderThereInOneThatCantBeWrittenIsExportedIntoWithNoParents(t *testing.T) {
	state, volumes := t.TempDir(), t.TempDir()
	dir := filepath.Join(volumes, "Backup")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	readOnly(t, volumes)
	e := exporter(state)
	e.NoParents = true

	r, err := e.Export(dir)
	if err != nil {
		t.Fatalf("Export() into a disk's mount point, as it were: %v", err)
	}
	wantResult(t, r, state, dir, 0, 0)
}

func TestAMacsFolderGoneMidExportIsMadeAgainOnlyWithoutNoParents(t *testing.T) {
	tests := []struct {
		name      string
		noParents bool
		wantErr   error
	}{
		{name: "made again, with every folder it goes in", noParents: false},
		{name: "not made again, with no parents", noParents: true, wantErr: export.ErrNoFolder},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, dir := t.TempDir(), filepath.Join(t.TempDir(), "backups")
			writeFile(t, requests.compressed(state, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
			e := exporter(state)
			e.NoParents = tt.noParents
			// The folder goes as the first day's looked at, as when its disk is
			// ejected.
			ejected := sync.OnceFunc(func() {
				if err := os.RemoveAll(dir); err != nil {
					t.Error(err)
				}
			})

			_, err := export.WithLooked(e, func(string) { ejected() }).Export(dir)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Export() = %v, want %v", err, tt.wantErr)
			}
			// A failure names the folder that went, not the day's file it was
			// writing.
			want := "can't export into " + dir + ": export the request ledger of " + day1 + ": " + folderOf(t, state, dir) + ": it isn't there"
			if err != nil && err.Error() != want {
				t.Errorf("Export() says %q, want %q", err, want)
			}
			if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) != tt.noParents {
				t.Errorf("after the export, %s: %v; want it made again %v", dir, err, !tt.noParents)
			}
		})
	}
}

func TestAnExportRemovesWhatWritesCutShortLeftAnHourAgoOrMore(t *testing.T) {
	files := []struct {
		name string
		// path is the file's, in the Mac's folder, last changed ago before
		// the export.
		path        string
		ago         time.Duration
		wantRemoved bool
	}{
		{name: "export.json, staged two hours before", path: ".export.json.123", ago: 2 * time.Hour, wantRemoved: true},
		{name: "a day's file, staged an hour before", path: "ledger/.requests-2026-10-02.jsonl.gz.456", ago: time.Hour, wantRemoved: true},
		{name: "a readings file, staged a day before", path: "history/.readings-2026-10-08.jsonl.gz.789", ago: 24 * time.Hour, wantRemoved: true},
		{name: "a summary, staged a day before", path: "ledger/.day-2026-10-02.json.4321", ago: 24 * time.Hour, wantRemoved: true},
		{name: "a day's file, staged half an hour before, as another export writes it still", path: "ledger/.requests-2026-10-08.jsonl.gz.111", ago: 30 * time.Minute},
		{name: "a file of the Finder's", path: ".DS_Store", ago: 24 * time.Hour},
		{name: "a file iCloud Drive hasn't downloaded", path: "ledger/.requests-2026-10-01.jsonl.gz.icloud", ago: 24 * time.Hour},
	}
	for _, f := range files {
		t.Run(f.name, func(t *testing.T) {
			state, dir := t.TempDir(), t.TempDir()
			writeFile(t, requests.compressed(state, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
			path := filepath.Join(folderOf(t, state, dir), f.path)
			// Last modified long before, as a write staging a file sets it
			// back to its source's: it last changed as it was written, and the
			// export runs ago after that, by its clock.
			writeFile(t, path, []byte("staged\n"), compressedAt)
			e := exporter(state)
			written := time.Now()
			e.Now = func() time.Time { return written.Add(f.ago) }

			if _, err := e.Export(dir); err != nil {
				t.Fatalf("Export() error = %v", err)
			}

			_, err := os.Lstat(path)
			if removed := errors.Is(err, fs.ErrNotExist); removed != f.wantRemoved {
				t.Errorf("after the export, %v; want it removed %v", err, f.wantRemoved)
			}
		})
	}
}

func TestAFileAnotherExportStagedIsLeftForItToRenameIntoPlace(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	writeFile(t, requests.compressed(state, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
	path := filepath.Join(folderOf(t, state, dir), "ledger", "requests-2026-10-01.jsonl.gz")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// Staged by another export just now, last modified as its source was, a
	// month before, for it to rename into place.
	staged, err := atomicfile.StageModified(path, 0o600, now.AddDate(0, 0, -30), func(w io.Writer) error {
		_, err := w.Write([]byte("staged\n"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	e := exporter(state)
	e.Now = func() time.Time { return now }

	if _, err := e.Export(dir); err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if err := os.Rename(staged, path); err != nil {
		t.Errorf("the other export's rename: %v, want its file left for it", err)
	}
}

func TestALeftoverThatCantBeRemovedIsNamedAndFailsNoExport(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	writeFile(t, requests.compressed(state, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
	path := filepath.Join(readings.Dir(folderOf(t, state, dir)), ".readings-2026-10-08.jsonl.gz.789")
	writeFile(t, path, []byte("staged\n"), compressedAt)
	readOnly(t, filepath.Dir(path))
	e := exporter(state)
	written := time.Now()
	e.Now = func() time.Time { return written.Add(2 * time.Hour) }

	r, err := e.Export(dir)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(r.Unremoved) != 1 || r.Unremoved[0].Path != path || !errors.Is(r.Unremoved[0].Err, fs.ErrPermission) {
		t.Errorf("Export() left %+v unremoved, want %s alone, for want of permission", r.Unremoved, path)
	}
}
