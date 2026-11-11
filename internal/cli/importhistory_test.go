package cli_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/router"
)

// importRefused is what import says as it's refused while another import
// runs.
const importRefused = "Error: an import is running already\n"

// importDeps are deps whose state directory's path is short enough to hold
// the router's control socket, and that directory.
func importDeps(t *testing.T) (cli.Deps, string) {
	t.Helper()
	deps := testDeps(map[string]string{"XDG_STATE_HOME": shortTempDir(t)}, t.TempDir())
	markMoved(t, stateDir(t, deps))
	return deps, stateDir(t, deps)
}

// importFolder writes, at folder, a Mac's folder holding its export.json
// and the request ledger's lines of a day, and returns it.
func importFolder(t *testing.T, folder string) string {
	t.Helper()
	writeStateFile(t, folder, "export.json", []byte(`{"version":2,"install":"0123456789abcdef","exported_at":"2026-10-09T14:42:07Z","ids":true}`+"\n"))
	writeStateFile(t, ledger.Dir(folder), "requests-2026-10-02.jsonl.gz", []byte("the 2nd's lines\n"))
	return folder
}

// staged returns the paths of the files staged to import in state, from its
// import directory.
func staged(state string) ([]string, error) {
	dir := router.ImportDir(state)
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		paths = append(paths, rel)
		return err
	})
	return paths, err
}

// answeringAsStaged answers POST /import with imported, once it has sent
// what's staged in state, as staged finds it, on the channel it returns.
func answeringAsStaged(t *testing.T, state string, imported router.Imported) (http.HandlerFunc, <-chan []string) {
	t.Helper()
	asStaged := make(chan []string, 1)
	return func(w http.ResponseWriter, r *http.Request) {
		paths, err := staged(state)
		if err != nil {
			t.Error(err)
		}
		asStaged <- paths
		answering(imported)(w, r)
	}, asStaged
}

// wantStaged fails t where what was staged in state as the router was asked
// to import, as asStaged has it, isn't want.
func wantStaged(t *testing.T, state string, asStaged <-chan []string, want ...string) {
	t.Helper()
	select {
	case got := <-asStaged:
		if !slices.Equal(got, want) {
			t.Errorf("as the router was asked to import, %s held %q, want %q", router.ImportDir(state), got, want)
		}
	default:
		t.Error("the router wasn't asked to import")
	}
}

// imports is a router's import, as a fake router serves it.
type imports struct {
	// running is what GET /import answers.
	running bool
	// post answers POST /import.
	post http.HandlerFunc
	// asked counts the GET /import asked, and posted the POST /import.
	asked, posted atomic.Int64
}

// serve serves the imports on the control socket of the router whose state
// directory is state.
func (i *imports) serve(t *testing.T, state string) {
	t.Helper()
	serveRouter(t, router.SocketPath(state), new(atomic.Int64), map[string]http.HandlerFunc{
		"GET /import": func(w http.ResponseWriter, _ *http.Request) {
			i.asked.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]bool{"running": i.running})
		},
		"POST /import": func(w http.ResponseWriter, r *http.Request) {
			i.posted.Add(1)
			i.post(w, r)
		},
	})
}

// answering answers POST /import with imported.
func answering(imported router.Imported) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(imported)
	}
}

// refusing refuses POST /import with the status given, saying why.
func refusing(status int, why string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": why})
	}
}

func TestImportSaysWhatItTookIn(t *testing.T) {
	tests := []struct {
		name     string
		imported router.Imported
		want     string
	}{
		{
			name:     "days added, merged and past keeping",
			imported: router.Imported{Folders: 1, Days: router.ImportedDays{Added: 27, Merged: 3, PastKeep: 12}, Lines: 1204, Summaries: 27},
			want:     "imported 1 folder: 27 days added, 3 merged, 12 past keeping left out, 27 days' summaries",
		},
		{
			name:     "one day of each, from several folders",
			imported: router.Imported{Folders: 3, Days: router.ImportedDays{Added: 1, Merged: 1, PastKeep: 1}, Lines: 4, Summaries: 1},
			want:     "imported 3 folders: 1 day added, 1 merged, 1 past keeping left out, 1 day's summary",
		},
		{
			name:     "days added, their summaries copied in with them",
			imported: router.Imported{Folders: 1, Days: router.ImportedDays{Added: 1240}, Summaries: 1240},
			want:     "imported 1 folder: 1,240 days added, 1,240 days' summaries",
		},
		{
			name:     "days added, and more days' summaries, every one counted",
			imported: router.Imported{Folders: 1, Days: router.ImportedDays{Added: 200}, Summaries: 500},
			want:     "imported 1 folder: 200 days added, 500 days' summaries",
		},
		{
			name:     "days added alone, no summary copied in",
			imported: router.Imported{Folders: 1, Days: router.ImportedDays{Added: 1240}},
			want:     "imported 1 folder: 1,240 days added",
		},
		{
			name:     "days merged, and summaries of days past keeping",
			imported: router.Imported{Folders: 1, Days: router.ImportedDays{Merged: 3, PastKeep: 12}, Lines: 41, Summaries: 12},
			want:     "imported 1 folder: 3 days merged, 12 past keeping left out, 12 days' summaries",
		},
		{
			name:     "days merged alone",
			imported: router.Imported{Folders: 2, Days: router.ImportedDays{Merged: 3}, Lines: 41},
			want:     "imported 2 folders: 3 days merged",
		},
		{
			name:     "days merged and past keeping",
			imported: router.Imported{Folders: 1, Days: router.ImportedDays{Merged: 3, PastKeep: 12}, Lines: 41},
			want:     "imported 1 folder: 3 days merged, 12 past keeping left out",
		},
		{
			name:     "days past keeping alone, the Mac holding their summaries",
			imported: router.Imported{Folders: 1, Days: router.ImportedDays{PastKeep: 12}},
			want:     "imported 1 folder: 12 days past keeping left out",
		},
		{
			name:     "summaries of days past keeping",
			imported: router.Imported{Folders: 1, Days: router.ImportedDays{PastKeep: 12}, Summaries: 12},
			want:     "imported 1 folder: 12 days' summaries, 12 days' lines past keeping left out",
		},
		{
			name:     "a summary of a day past keeping",
			imported: router.Imported{Folders: 1, Days: router.ImportedDays{PastKeep: 1}, Summaries: 1},
			want:     "imported 1 folder: 1 day's summary, 1 day's lines past keeping left out",
		},
		{
			name:     "summaries of days of no lines",
			imported: router.Imported{Folders: 1, Summaries: 4},
			want:     "imported 1 folder: 4 days' summaries",
		},
		{
			name:     "an import repeated",
			imported: router.Imported{Folders: 1},
			want:     "imported 1 folder: nothing new",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, state := importDeps(t)
			(&imports{post: answering(tt.imported)}).serve(t, state)

			if got, want := run(t, deps, "import", importFolder(t, t.TempDir())), (result{stdout: tt.want + "\n"}); got != want {
				t.Errorf("switchboard import = %+v, want %+v", got, want)
			}
		})
	}
}

func TestImportStagesTheFoldersBeforeTheRouterIsAskedToTakeThemIn(t *testing.T) {
	deps, state := importDeps(t)
	dir := t.TempDir()
	importFolder(t, filepath.Join(dir, "mac-0123456789abcdef"))
	importFolder(t, filepath.Join(dir, "mac-fedcba9876543210"))
	post, asStaged := answeringAsStaged(t, state, router.Imported{Folders: 2, Days: router.ImportedDays{Added: 1}})
	(&imports{post: post}).serve(t, state)

	if got, want := run(t, deps, "import", dir), (result{stdout: "imported 2 folders: 1 day added\n"}); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
	wantStaged(t, state, asStaged, filepath.Join("1", "export.json"), filepath.Join("1", "ledger", "requests-2026-10-02.jsonl.gz"), filepath.Join("2", "export.json"), filepath.Join("2", "ledger", "requests-2026-10-02.jsonl.gz"))
}

func TestImportTakesARelativeFolderFromTheWorkingDirectory(t *testing.T) {
	deps, state := importDeps(t)
	wd, err := deps.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	importFolder(t, filepath.Join(wd, "backups", "mac-0123456789abcdef"))
	post, asStaged := answeringAsStaged(t, state, router.Imported{Folders: 1})
	(&imports{post: post}).serve(t, state)

	if got, want := run(t, deps, "import", "backups"), (result{stdout: "imported 1 folder: nothing new\n"}); got != want {
		t.Errorf("switchboard import backups = %+v, want %+v", got, want)
	}
	wantStaged(t, state, asStaged, filepath.Join("1", "export.json"), filepath.Join("1", "ledger", "requests-2026-10-02.jsonl.gz"))
}

func TestImportWithoutTheRouterStagesNothing(t *testing.T) {
	deps, state := importDeps(t)

	if got, want := run(t, deps, "import", importFolder(t, t.TempDir())), (result{stderr: routerDown, code: 1}); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
	if _, err := os.Stat(router.ImportDir(state)); !os.IsNotExist(err) {
		t.Errorf("%s: %v, want nothing staged for a router that isn't there", router.ImportDir(state), err)
	}
}

func TestImportAsTheRouterClosesUnanswered(t *testing.T) {
	deps, state := importDeps(t)
	(&imports{post: func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	}}).serve(t, state)

	got := run(t, deps, "import", importFolder(t, t.TempDir()))
	want := result{stderr: "Error: the router closed before it answered, so the import may have stopped partway: what it took in stays, and importing again takes in the rest\n", code: 1}
	if got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
}

func TestImportTheRouterRefusesSaysWhy(t *testing.T) {
	tests := []struct {
		status int
		why    string
	}{
		{status: http.StatusConflict, why: "an import is running already"},
		{status: http.StatusBadRequest, why: "there's nothing staged to import: /state/import holds no folder to take in"},
		{status: http.StatusServiceUnavailable, why: "the router stopped before the import was done: what it took in stays, and importing again takes in the rest"},
		{status: http.StatusInternalServerError, why: "can't take in the request ledger of 2026-10-02: gzip: invalid header"},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			deps, state := importDeps(t)
			(&imports{post: refusing(tt.status, tt.why)}).serve(t, state)

			if got, want := run(t, deps, "import", importFolder(t, t.TempDir())), (result{stderr: "Error: " + tt.why + "\n", code: 1}); got != want {
				t.Errorf("switchboard import = %+v, want %+v", got, want)
			}
		})
	}
}

// leftStaged writes a file into state's import directory, as an import the
// router still runs reads, and returns its path.
func leftStaged(t *testing.T, state string) string {
	t.Helper()
	dir := filepath.Join(router.ImportDir(state), "1", "ledger")
	writeStateFile(t, dir, "requests-2026-09-30.jsonl.gz", []byte("being taken in\n"))
	return filepath.Join(dir, "requests-2026-09-30.jsonl.gz")
}

// wantLeftStaged fails t where the file at path, left staged, isn't there as
// it was, or the router was asked to import.
func wantLeftStaged(t *testing.T, path string, i *imports) {
	t.Helper()
	if data, err := os.ReadFile(path); err != nil || string(data) != "being taken in\n" {
		t.Errorf("%s holds %q (%v), want it left as it was", path, data, err)
	}
	if n := i.posted.Load(); n != 0 {
		t.Errorf("the router was asked to import %d times, want none", n)
	}
}

func TestImportIsRefusedWhileAnotherHoldsTheLockTouchingNothing(t *testing.T) {
	deps, state := importDeps(t)
	unlock, err := export.LockImport(state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unlock)
	left := leftStaged(t, state)
	i := &imports{post: answering(router.Imported{Folders: 1})}
	i.serve(t, state)

	if got, want := run(t, deps, "import", importFolder(t, t.TempDir())), (result{stderr: importRefused, code: 1}); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
	wantLeftStaged(t, left, i)
	if n := i.asked.Load(); n != 0 {
		t.Errorf("the router was asked whether it's importing %d times, want none: the lock refuses first", n)
	}
}

func TestImportHoldsTheLockWhileTheRouterTakesTheFoldersIn(t *testing.T) {
	deps, state := importDeps(t)
	locked := make(chan error, 1)
	(&imports{post: func(w http.ResponseWriter, r *http.Request) {
		unlock, err := export.LockImport(state)
		if err == nil {
			unlock()
		}
		locked <- err
		answering(router.Imported{Folders: 1})(w, r)
	}}).serve(t, state)

	if got := run(t, deps, "import", importFolder(t, t.TempDir())); got.code != 0 {
		t.Fatalf("switchboard import = %+v, want it to import", got)
	}
	if err := <-locked; !errors.Is(err, export.ErrImportRunning) {
		t.Errorf("LockImport() as the router was asked to import: %v, want ErrImportRunning: the lock held until it answers", err)
	}
}

func TestImportIsRefusedWhileTheRouterRunsOneTouchingNothing(t *testing.T) {
	deps, state := importDeps(t)
	left := leftStaged(t, state)
	i := &imports{running: true, post: answering(router.Imported{Folders: 1})}
	i.serve(t, state)

	if got, want := run(t, deps, "import", importFolder(t, t.TempDir())), (result{stderr: importRefused, code: 1}); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
	wantLeftStaged(t, left, i)
	unlock, err := export.LockImport(state)
	if err != nil {
		t.Fatalf("LockImport() after a refused import: %v, want the lock released", err)
	}
	unlock()
}

func TestImportRemovesWhatAnImportCutShortLeftThenStages(t *testing.T) {
	deps, state := importDeps(t)
	leftStaged(t, state)
	post, asStaged := answeringAsStaged(t, state, router.Imported{Folders: 1})
	(&imports{post: post}).serve(t, state)

	if got := run(t, deps, "import", importFolder(t, t.TempDir())); got.code != 0 {
		t.Fatalf("switchboard import = %+v, want it to import", got)
	}
	wantStaged(t, state, asStaged, filepath.Join("1", "export.json"), filepath.Join("1", "ledger", "requests-2026-10-02.jsonl.gz"))
}

func TestImportFromAFolderOfNoMacsFolderFails(t *testing.T) {
	deps, state := importDeps(t)
	i := &imports{post: answering(router.Imported{Folders: 1})}
	i.serve(t, state)
	dir := t.TempDir()

	want := result{stderr: "Error: there's nothing to import in " + dir + ": it holds no Mac's folder, one with the export.json export writes\n", code: 1}
	if got := run(t, deps, "import", dir); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
	if n := i.posted.Load(); n != 0 {
		t.Errorf("the router was asked to import %d times, want none", n)
	}
}

func TestImportSaysWhichFolderItPassedOverAsItsExportJSONDoesntReadThenCarriesOn(t *testing.T) {
	deps, state := importDeps(t)
	dir := t.TempDir()
	importFolder(t, filepath.Join(dir, "mac-0123456789abcdef"))
	broken := filepath.Join(dir, "mac-fedcba9876543210")
	writeStateFile(t, broken, "export.json", []byte(`{"version":1,"install":"fedc`))
	post, asStaged := answeringAsStaged(t, state, router.Imported{Folders: 1, Days: router.ImportedDays{Added: 1}})
	(&imports{post: post}).serve(t, state)

	want := result{stdout: "imported 1 folder: 1 day added\n", stderr: "switchboard: passed over " + broken + ": its export.json doesn't read\n"}
	if got := run(t, deps, "import", dir); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
	wantStaged(t, state, asStaged, filepath.Join("1", "export.json"), filepath.Join("1", "ledger", "requests-2026-10-02.jsonl.gz"))
}

// Another Mac's folder holding its days without its export.json, as its
// export cut short leaves it, is passed over, said by its path and why.
func TestImportSaysWhichFolderItPassedOverAsItHoldsAnExportWithoutItsExportJSONThenCarriesOn(t *testing.T) {
	deps, state := importDeps(t)
	dir := t.TempDir()
	importFolder(t, filepath.Join(dir, "mac-0123456789abcdef"))
	cut := filepath.Join(dir, "mac-fedcba9876543210")
	writeStateFile(t, ledger.Dir(cut), "requests-2026-10-03.jsonl.gz", []byte("the 3rd's lines\n"))
	post, _ := answeringAsStaged(t, state, router.Imported{Folders: 1, Days: router.ImportedDays{Added: 1}})
	(&imports{post: post}).serve(t, state)

	want := result{stdout: "imported 1 folder: 1 day added\n",
		stderr: "switchboard: passed over " + cut + ": it holds an export without its export.json, as an export cut short or a sync not done leaves one: import it again once it's exported whole\n"}
	if got := run(t, deps, "import", dir); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
}

// A folder the router would refuse is passed over, said by its path and
// why, and the rest imported: another Mac's exported before switchboard
// made account ids, and one whose accounts record hasn't synced.
func TestImportPassesOverAFolderTheRouterWouldRefuseThenCarriesOn(t *testing.T) {
	deps, state := importDeps(t)
	dir := t.TempDir()
	importFolder(t, filepath.Join(dir, "mac-0123456789abcdef"))
	before := filepath.Join(dir, "mac-fedcba9876543210")
	writeStateFile(t, before, "export.json", []byte(`{"version":1,"install":"fedcba9876543210","exported_at":"2026-10-09T14:42:07Z"}`+"\n"))
	unsynced := filepath.Join(dir, "mac-1111111111111111")
	writeStateFile(t, unsynced, "export.json", []byte(`{"version":2,"install":"1111111111111111","exported_at":"2026-10-09T14:42:07Z","ids":true,"accounts":true}`+"\n"))
	post, asStaged := answeringAsStaged(t, state, router.Imported{Folders: 1, Days: router.ImportedDays{Added: 1}})
	(&imports{post: post}).serve(t, state)

	want := result{
		stdout: "imported 1 folder: 1 day added\n",
		stderr: "switchboard: passed over " + unsynced + ": its accounts record hasn't synced yet: import it again once it has\n" +
			"switchboard: passed over " + before + ": another Mac exported it before switchboard made account ids, so its accounts can't be told: export it again from that Mac once it runs this version of switchboard, or leave it out\n",
	}
	if got := run(t, deps, "import", dir); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
	wantStaged(t, state, asStaged, filepath.Join("1", "export.json"), filepath.Join("1", "ledger", "requests-2026-10-02.jsonl.gz"))
}

// This Mac's own folder exported before switchboard made account ids, as
// its install id tells, is imported, and another Mac's whose accounts record
// doesn't read is passed over, said by its path and why.
func TestImportTakesThisMacsFolderFromBeforeIDsAndPassesOverOneWhoseRecordDoesntRead(t *testing.T) {
	deps, state := importDeps(t)
	writeStateFile(t, state, "install-id", []byte("0123456789abcdef\n"))
	dir := t.TempDir()
	own := importFolder(t, filepath.Join(dir, "mac-0123456789abcdef"))
	writeStateFile(t, own, "export.json", []byte(`{"version":1,"install":"0123456789abcdef","exported_at":"2026-10-09T14:42:07Z"}`+"\n"))
	damaged := filepath.Join(dir, "mac-fedcba9876543210")
	writeStateFile(t, damaged, "export.json", []byte(`{"version":2,"install":"fedcba9876543210","exported_at":"2026-10-09T14:42:07Z","ids":true,"accounts":true}`+"\n"))
	writeStateFile(t, damaged, filepath.Base(identity.NewFile(damaged).Path()), []byte("not a record\n"))
	post, asStaged := answeringAsStaged(t, state, router.Imported{Folders: 1, Days: router.ImportedDays{Added: 1}})
	(&imports{post: post}).serve(t, state)

	got := run(t, deps, "import", dir)
	if got.code != 0 || got.stdout != "imported 1 folder: 1 day added\n" || !strings.HasPrefix(got.stderr, "switchboard: passed over "+damaged+": its accounts record doesn't read: ") {
		t.Errorf("switchboard import = %+v, want this Mac's folder imported, the other passed over as its record doesn't read", got)
	}
	wantStaged(t, state, asStaged, filepath.Join("1", "export.json"), filepath.Join("1", "ledger", "requests-2026-10-02.jsonl.gz"))
}

// An import every folder of which would be refused says why of each, then
// fails, staging nothing.
func TestImportOfNothingButFoldersTheRouterWouldRefuseSaysSoThenFails(t *testing.T) {
	deps, state := importDeps(t)
	i := &imports{post: answering(router.Imported{Folders: 1})}
	i.serve(t, state)
	dir := t.TempDir()
	before := filepath.Join(dir, "mac-fedcba9876543210")
	writeStateFile(t, before, "export.json", []byte(`{"version":1,"install":"fedcba9876543210","exported_at":"2026-10-09T14:42:07Z"}`+"\n"))

	got := run(t, deps, "import", dir)
	if got.code != 1 || !strings.HasPrefix(got.stderr, "switchboard: passed over "+before+": another Mac exported it") ||
		!strings.HasSuffix(got.stderr, "Error: there's nothing to import in "+dir+": each Mac's folder in it was passed over\n") {
		t.Errorf("switchboard import = %+v, want the folder passed over, then nothing imported", got)
	}
	if _, err := os.Stat(router.ImportDir(state)); err == nil {
		t.Errorf("an import of nothing staged %s, want nothing staged", router.ImportDir(state))
	}
}

func TestImportOfNothingButFoldersWhoseExportJSONDoesntReadSaysSoThenFails(t *testing.T) {
	deps, state := importDeps(t)
	i := &imports{post: answering(router.Imported{Folders: 1})}
	i.serve(t, state)
	dir := t.TempDir()
	broken := filepath.Join(dir, "mac-fedcba9876543210")
	writeStateFile(t, broken, "export.json", []byte("not json\n"))

	want := result{
		stderr: "switchboard: passed over " + broken + ": its export.json doesn't read\n" +
			"Error: there's nothing to import in " + dir + ": it holds no Mac's folder, one with the export.json export writes\n",
		code: 1,
	}
	if got := run(t, deps, "import", dir); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
	if n := i.posted.Load(); n != 0 {
		t.Errorf("the router was asked to import %d times, want none", n)
	}
}

func TestImportThatCantStageAFolderLeavesNothingStaged(t *testing.T) {
	deps, state := importDeps(t)
	i := &imports{post: answering(router.Imported{Folders: 2})}
	i.serve(t, state)
	dir := t.TempDir()
	importFolder(t, filepath.Join(dir, "mac-0123456789abcdef"))
	unreadable := filepath.Join(ledger.Dir(importFolder(t, filepath.Join(dir, "mac-fedcba9876543210"))), "requests-2026-10-02.jsonl.gz")
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}

	if got := run(t, deps, "import", dir); got.code != 1 || !strings.Contains(got.stderr, unreadable) {
		t.Errorf("switchboard import = %+v, want it to fail, naming %s", got, unreadable)
	}
	if _, err := os.Stat(router.ImportDir(state)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s: %v, want what was staged removed", router.ImportDir(state), err)
	}
	if n := i.posted.Load(); n != 0 {
		t.Errorf("the router was asked to import %d times, want none", n)
	}
}

func TestImportAgainstARouterFromBeforeImportSaysToRestartIt(t *testing.T) {
	deps, state := importDeps(t)
	serveRouter(t, router.SocketPath(state), new(atomic.Int64), nil)

	want := result{stderr: "Error: the router running is older than this switchboard: restart it with switchboard service restart (or, run by hand, stop it and run switchboard serve again), then import again\n", code: 1}
	if got := run(t, deps, "import", importFolder(t, t.TempDir())); got != want {
		t.Errorf("switchboard import = %+v, want %+v", got, want)
	}
	if _, err := os.Stat(router.ImportDir(state)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s: %v, want nothing staged for a router that can't import", router.ImportDir(state), err)
	}
}

func TestImportNeedsOneFolder(t *testing.T) {
	deps, _ := importDeps(t)
	for _, args := range [][]string{{"import"}, {"import", ""}, {"import", t.TempDir(), t.TempDir()}} {
		if got := run(t, deps, args...); got.code != 1 || got.stderr != "Error: give the folder to import from\n" {
			t.Errorf("switchboard %q = %+v, want it refused, saying why", args, got)
		}
	}
}

func TestImportRefusesAFolderShapedLikeATokenWithoutSayingIt(t *testing.T) {
	deps, _ := importDeps(t)
	const want = "Error: the folder given holds something shaped like a token, which switchboard never reads: give another\n"
	for _, dir := range []string{tokenShaped, filepath.Join(t.TempDir(), tokenShaped, "backups")} {
		if got := run(t, deps, "import", dir); got.code != 1 || got.stderr != want || strings.Contains(got.stdout, "sk-ant-") {
			t.Errorf("switchboard import <a token> = %+v, want it refused, saying %q, the token unsaid", got, want)
		}
	}
}
