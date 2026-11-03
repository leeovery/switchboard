package export_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/export"
)

// The installs the tests' Macs' folders are of.
const (
	installOne = "0123456789abcdef"
	installTwo = "fedcba9876543210"
)

// macsFolder writes, at folder, a Mac's folder of the install with the given
// id, whose export.json is of the given version, holding nothing else.
func macsFolder(t *testing.T, folder, id string, version int) string {
	t.Helper()
	manifest := `{"version":` + strconv.Itoa(version) + `,"install":"` + id + `","exported_at":"2026-10-09T14:42:07Z"}` + "\n"
	writeFile(t, filepath.Join(folder, "export.json"), []byte(manifest), exportNow)
	return folder
}

// stage stages the folders given in importDir, failing t where it can't.
func stage(t *testing.T, importDir string, folders ...string) {
	t.Helper()
	if err := export.Stage(importDir, folders); err != nil {
		t.Fatalf("Stage(%s, %q) error = %v", importDir, folders, err)
	}
}

// filesUnder returns the paths of the files under root, from root, oldest
// name first.
func filesUnder(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		paths = append(paths, rel)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestFoldersTakesTheOneMacsFolderDirIs(t *testing.T) {
	dir := macsFolder(t, t.TempDir(), installOne, export.Version)
	macsFolder(t, filepath.Join(dir, "mac-"+installTwo), installTwo, export.Version)

	if got, err := export.Folders(dir); err != nil || !slices.Equal(got, []string{dir}) {
		t.Errorf("Folders() = %q, %v; want %q alone", got, err, dir)
	}
}

func TestFoldersTakesEachMacsFolderInDirByNamePassingOverTheRest(t *testing.T) {
	dir := t.TempDir()
	two := macsFolder(t, filepath.Join(dir, "mac-"+installTwo), installTwo, export.Version)
	one := macsFolder(t, filepath.Join(dir, "mac-"+installOne), installOne, export.Version)
	older := macsFolder(t, filepath.Join(dir, "renamed"), installOne, export.Version-1)
	writeFile(t, filepath.Join(dir, "notes", "readme.txt"), []byte("no export.json here\n"), exportNow)
	writeFile(t, filepath.Join(dir, "broken", "export.json"), []byte("not json\n"), exportNow)
	writeFile(t, filepath.Join(dir, "export.json.txt"), []byte("a file\n"), exportNow)
	macsFolder(t, filepath.Join(one, "nested"), installTwo, export.Version)

	if got, err := export.Folders(dir); err != nil || !slices.Equal(got, []string{one, two, older}) {
		t.Errorf("Folders() = %q, %v; want %q", got, err, []string{one, two, older})
	}
}

func TestFoldersRefusesAFolderANewerSwitchboardExported(t *testing.T) {
	tests := []struct {
		name string
		// folder is where the newer folder is in dir.
		folder string
	}{
		{name: "the folder given"},
		{name: "a folder among others", folder: "mac-" + installTwo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			macsFolder(t, filepath.Join(dir, "mac-"+installOne), installOne, export.Version)
			newer := macsFolder(t, filepath.Join(dir, tt.folder), installTwo, export.Version+1)

			got, err := export.Folders(dir)
			if err == nil || !strings.Contains(err.Error(), newer) || !strings.Contains(err.Error(), "a newer switchboard exported it") {
				t.Errorf("Folders() = %q, %v; want it refused, naming %s and saying a newer switchboard exported it", got, err, newer)
			}
		})
	}
}

func TestFoldersFailsWhereDirHoldsNoMacsFolder(t *testing.T) {
	empty := t.TempDir()
	unrelated := t.TempDir()
	writeFile(t, filepath.Join(unrelated, "photos", "a.jpg"), []byte("jpeg\n"), exportNow)
	tests := []struct {
		name, dir, want string
	}{
		{name: "an empty folder", dir: empty, want: "there's nothing to import in " + empty + ": it holds no Mac's folder"},
		{name: "a folder of others", dir: unrelated, want: "there's nothing to import in " + unrelated + ": it holds no Mac's folder"},
		{name: "a folder that isn't there", dir: filepath.Join(empty, "gone"), want: "can't import from " + filepath.Join(empty, "gone") + ": "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := export.Folders(tt.dir); err == nil || !strings.HasPrefix(err.Error(), tt.want) {
				t.Errorf("Folders() = %q, %v; want it to fail, saying %q", got, err, tt.want)
			}
		})
	}
}

func TestStageLaysOutEachFolderNumberedInOrderAsTheRouterReadsIt(t *testing.T) {
	dir := t.TempDir()
	one := macsFolder(t, filepath.Join(dir, "mac-"+installOne), installOne, export.Version)
	two := macsFolder(t, filepath.Join(dir, "mac-"+installTwo), installTwo, export.Version)
	files := map[string]struct {
		data     []byte
		modified time.Time
	}{
		requests.compressed(one, day1): {gzipOf(t, linesOf(`{"n":1}`)), compressedAt},
		kinds[1].compressed(one, day1): {gzipOf(t, linesOf(`{"n":2}`)), appendedAt},
		summary(one, day1):             {[]byte("{\"version\":6}\n"), summarisedAt},
		kinds[2].compressed(one, day2): {gzipOf(t, linesOf(`{"n":3}`)), compressedAt},
		requests.compressed(two, day2): {gzipOf(t, linesOf(`{"n":4}`)), appendedAt},
		summary(two, "2025-10-02"):     {[]byte("{\"version\":5}\n"), summarisedAt},
	}
	for path, f := range files {
		writeFile(t, path, f.data, f.modified)
	}
	importDir := filepath.Join(t.TempDir(), "state", "import")

	stage(t, importDir, one, two)

	staged := map[string]string{one: filepath.Join(importDir, "1"), two: filepath.Join(importDir, "2")}
	var want []string
	for path, f := range files {
		folder := one
		if strings.HasPrefix(path, two) {
			folder = two
		}
		rel, err := filepath.Rel(folder, path)
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, filepath.Join(filepath.Base(staged[folder]), rel))
		wantCopy(t, filepath.Join(staged[folder], rel), f.data, f.modified)
	}
	slices.Sort(want)
	if got := filesUnder(t, importDir); !slices.Equal(got, want) {
		t.Errorf("%s holds %q, want %q", importDir, got, want)
	}
	for _, d := range []string{importDir, staged[one], filepath.Join(staged[one], "ledger"), filepath.Join(staged[one], "history"), staged[two]} {
		if info := look(t, d); !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Errorf("%s is %v, want a folder, 0700", d, info.Mode())
		}
	}
}

func TestStageStagesAFolderOfNothingButItsExportJSONAsAFolderOfNothing(t *testing.T) {
	one := macsFolder(t, t.TempDir(), installOne, export.Version)
	importDir := filepath.Join(t.TempDir(), "import")

	stage(t, importDir, one)

	if info := look(t, filepath.Join(importDir, "1")); !info.IsDir() {
		t.Errorf("%s/1 is %v, want a folder", importDir, info.Mode())
	}
	if got := filesUnder(t, importDir); len(got) != 0 {
		t.Errorf("%s holds %q, want nothing", importDir, got)
	}
}

func TestStagePassesOverWhatTheStateDirectoryDoesntName(t *testing.T) {
	one := macsFolder(t, t.TempDir(), installOne, export.Version)
	writeFile(t, requests.compressed(one, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
	for _, path := range []string{
		filepath.Join(one, "ledger", ".requests-"+day1+".jsonl.gz.2786451"),
		filepath.Join(one, "ledger", ".day-"+day1+".json.913"),
		filepath.Join(one, "history", ".readings-"+day2+".jsonl.gz.55"),
		requests.plain(one, day2),
		filepath.Join(one, "ledger", "digest-"+day1+".json.gz"),
		filepath.Join(one, "ledger", "requests-"+day1+".jsonl.gz.bak"),
		filepath.Join(one, "history", "readings-2026-1-1.jsonl.gz"),
		filepath.Join(one, "ledger", "notes.txt"),
		filepath.Join(one, "notes.txt"),
		filepath.Join(one, "tokens", "work"),
		filepath.Join(one, "ledger", "requests-"+day2+".jsonl.gz", "inside"),
	} {
		writeFile(t, path, []byte("not the history's\n"), exportNow)
	}
	importDir := filepath.Join(t.TempDir(), "import")

	stage(t, importDir, one)

	if got, want := filesUnder(t, importDir), []string{filepath.Join("1", "ledger", "requests-"+day1+".jsonl.gz")}; !slices.Equal(got, want) {
		t.Errorf("%s holds %q, want %q alone", importDir, got, want)
	}
}

func TestStageRemovesWhatAnImportCutShortLeftFirst(t *testing.T) {
	one := macsFolder(t, t.TempDir(), installOne, export.Version)
	writeFile(t, requests.compressed(one, day1), gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
	importDir := filepath.Join(t.TempDir(), "import")
	for _, left := range []string{
		filepath.Join(importDir, "1", "ledger", "requests-"+day2+".jsonl.gz"),
		filepath.Join(importDir, "2", "history", "readings-"+day2+".jsonl.gz"),
		filepath.Join(importDir, "stray"),
	} {
		writeFile(t, left, []byte("left by an import cut short\n"), exportNow)
	}

	stage(t, importDir, one)

	if got, want := filesUnder(t, importDir), []string{filepath.Join("1", "ledger", "requests-"+day1+".jsonl.gz")}; !slices.Equal(got, want) {
		t.Errorf("%s holds %q, want %q alone", importDir, got, want)
	}
}

func TestStageFailsWhereAFolderCantBeRead(t *testing.T) {
	one := macsFolder(t, t.TempDir(), installOne, export.Version)
	path := requests.compressed(one, day1)
	writeFile(t, path, gzipOf(t, linesOf(`{"n":1}`)), compressedAt)
	unreadable(t, path)

	if err := export.Stage(filepath.Join(t.TempDir(), "import"), []string{one}); err == nil || !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), one) {
		t.Errorf("Stage() error = %v, want it to fail, naming %s, as its file can't be read", err, one)
	}
}

func TestAnExportStagedIsByteAndTimeTheExportsOwn(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	for _, k := range kinds {
		writeFile(t, k.compressed(state, day1), append(gzipOf(t, linesOf(`{"n":1}`)), gzipOf(t, linesOf(`{"n":2}`))...), compressedAt)
		writeFile(t, k.plain(state, day2), []byte(linesOf(`{"n":3}`, `{"n":4}`)), appendedAt)
	}
	writeFile(t, summary(state, day1), []byte("{\"version\":6}\n"), summarisedAt)
	exportInto(t, state, dir)
	importDir := filepath.Join(t.TempDir(), "import")

	folders, err := export.Folders(dir)
	if err != nil {
		t.Fatal(err)
	}
	stage(t, importDir, folders...)

	folder := folderOf(t, state, dir)
	exported := slices.DeleteFunc(filesUnder(t, folder), func(path string) bool { return path == "export.json" })
	if got := filesUnder(t, filepath.Join(importDir, "1")); len(exported) != 7 || !slices.Equal(got, exported) {
		t.Fatalf("staged %q, want the export's %q, seven files", got, exported)
	}
	for _, rel := range exported {
		src, dst := filepath.Join(folder, rel), filepath.Join(importDir, "1", rel)
		want, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(dst); err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s holds %q (%v), want the export's %q", dst, got, err, want)
		}
		if got, want := look(t, dst).ModTime(), look(t, src).ModTime(); !got.Equal(want) {
			t.Errorf("%s is modified at %v, want the export's %v", dst, got, want)
		}
	}
}

func TestASecondImportsLockIsRefusedWhileOneHoldsIt(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	unlock, err := export.LockImport(state)
	if err != nil {
		t.Fatalf("LockImport() error = %v", err)
	}

	if again, err := export.LockImport(state); !errors.Is(err, export.ErrImportRunning) || err.Error() != "an import is running already" {
		if again != nil {
			again()
		}
		t.Errorf("LockImport() while one holds it: %v, want ErrImportRunning", err)
	}
	unlock()
	again, err := export.LockImport(state)
	if err != nil {
		t.Fatalf("LockImport() once the lock's released: %v, want it taken", err)
	}
	again()
	if info := look(t, filepath.Join(state, "import.lock")); info.Mode().Perm() != 0o600 {
		t.Errorf("import.lock is %v, want it left there, 0600", info.Mode())
	}
}
