package cli_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// exportDeps are deps whose state directory holds two days: the 2nd's
// request ledger's lines and summary, and the 8th's readings, but no config,
// which export needs none of, nor a router.
func exportDeps(t *testing.T) cli.Deps {
	t.Helper()
	deps := testDeps(nil, t.TempDir())
	state := stateDir(t, deps)
	writeStateFile(t, ledger.Dir(state), "requests-2026-10-02.jsonl", []byte(`{"n":1}`+"\n"))
	writeStateFile(t, ledger.Dir(state), "day-2026-10-02.json", []byte("{}\n"))
	writeStateFile(t, readings.Dir(state), "readings-2026-10-08.jsonl", []byte(`{"n":2}`+"\n"))
	return deps
}

// macsFolder is the folder of the install commands run with deps export as,
// in dir.
func macsFolder(t *testing.T, deps cli.Deps, dir string) string {
	t.Helper()
	id, err := export.InstallID(stateDir(t, deps))
	if err != nil {
		t.Fatal(err)
	}
	return export.Folder(dir, id)
}

func TestExportSaysHowManyDaysItWroteAndLeftAsTheyWere(t *testing.T) {
	deps := exportDeps(t)
	dir := t.TempDir()
	folder := macsFolder(t, deps, dir)

	if got, want := run(t, deps, "export", dir), (result{stdout: "exported 2 days to " + folder + ": 2 written, 0 as they were\n"}); got != want {
		t.Errorf("switchboard export = %+v, want %+v", got, want)
	}
	if got, want := run(t, deps, "export", dir), (result{stdout: "exported 2 days to " + folder + ": 0 written, 2 as they were\n"}); got != want {
		t.Errorf("switchboard export again = %+v, want %+v", got, want)
	}
	if _, err := os.Stat(filepath.Join(folder, "ledger", "requests-2026-10-02.jsonl.gz")); err != nil {
		t.Errorf("the folder's copy of the 2nd's lines: %v, want it there", err)
	}
}

func TestExportCountsADayAsOne(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	writeStateFile(t, ledger.Dir(stateDir(t, deps)), "day-2026-10-02.json", []byte("{}\n"))
	dir := t.TempDir()

	if got, want := run(t, deps, "export", dir), (result{stdout: "exported 1 day to " + macsFolder(t, deps, dir) + ": 1 written, 0 as they were\n"}); got != want {
		t.Errorf("switchboard export = %+v, want %+v", got, want)
	}
}

func TestExportTakesARelativeFolderFromTheWorkingDirectory(t *testing.T) {
	deps := exportDeps(t)
	wd, err := deps.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	folder := macsFolder(t, deps, filepath.Join(wd, "backups"))

	if got, want := run(t, deps, "export", "backups"), (result{stdout: "exported 2 days to " + folder + ": 2 written, 0 as they were\n"}); got != want {
		t.Errorf("switchboard export backups = %+v, want %+v", got, want)
	}
	if _, err := os.Stat(filepath.Join(folder, "export.json")); err != nil {
		t.Errorf("the folder's export.json: %v, want it there", err)
	}
}

func TestExportSaysWhichDaysItCouldntReadThenFails(t *testing.T) {
	deps := exportDeps(t)
	ledgerDir := ledger.Dir(stateDir(t, deps))
	writeStateFile(t, ledgerDir, "events-2026-10-02.jsonl.gz", []byte("not gzip, nor ever was\n"))
	writeStateFile(t, ledgerDir, "events-2026-10-02.jsonl", []byte(`{"n":3}`+"\n"))
	dir := t.TempDir()

	got := run(t, deps, "export", dir)
	want := result{
		stdout: "exported 2 days to " + macsFolder(t, deps, dir) + ": 2 written, 0 as they were\n",
		stderr: "switchboard: couldn't export the router's events of 2026-10-02: read events-2026-10-02.jsonl.gz: gzip: invalid header\n",
		code:   1,
	}
	if got != want {
		t.Errorf("switchboard export = %+v, want %+v", got, want)
	}
}

func TestExportIntoAFileFails(t *testing.T) {
	deps := exportDeps(t)
	dir := filepath.Join(t.TempDir(), "backups")
	if err := os.WriteFile(dir, []byte("a file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got, want := run(t, deps, "export", dir), (result{stderr: "Error: can't export into " + dir + ": it's a file, not a folder\n", code: 1}); got != want {
		t.Errorf("switchboard export into a file = %+v, want %+v", got, want)
	}
}

func TestExportLogsAFileAWriteCutShortLeftThatItCouldntRemove(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	writeStateFile(t, ledger.Dir(stateDir(t, deps)), "day-2026-10-02.json", []byte("{}\n"))
	dir := t.TempDir()
	history := readings.Dir(macsFolder(t, deps, dir))
	writeStateFile(t, history, ".readings-2026-10-08.jsonl.gz.789", []byte("staged\n"))
	if err := os.Chmod(history, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(history, 0o700) })
	// The export runs two hours after the file was left, by its clock.
	left := time.Now()
	deps.Now = func() time.Time { return left.Add(2 * time.Hour) }

	if got := run(t, deps, "export", dir); got.code != 0 {
		t.Fatalf("switchboard export = %+v, want it exported", got)
	}
	path := filepath.Join(history, ".readings-2026-10-08.jsonl.gz.789")
	if log := readLog(t, deps, "cli.log"); !hasLine(log, "level=WARN", `msg="can't remove a file an export cut short left"`, "path="+path, "permission denied") {
		t.Errorf("cli.log reads\n%s\nwant the file that couldn't be removed warned of", log)
	}
}

func TestExportNeedsOneFolder(t *testing.T) {
	deps := exportDeps(t)
	for _, args := range [][]string{{"export"}, {"export", ""}, {"export", t.TempDir(), t.TempDir()}} {
		if got := run(t, deps, args...); got.code != 1 || got.stderr != "Error: give the folder to export into\n" {
			t.Errorf("switchboard %q = %+v, want it refused, saying why", args, got)
		}
	}
}
