//go:build darwin

package router

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

func TestAFileOrFolderLockedInTheFinderIsSaidToBeLocked(t *testing.T) {
	tests := []struct {
		name string
		// locks makes what the case locks, of the folder exported into, dir,
		// and the Mac's folder in it, folder, exported into by x, and returns
		// it.
		locks func(t *testing.T, x *exports, dir, folder string) string
	}{
		{name: "the folder exported into", locks: func(_ *testing.T, _ *exports, dir, _ string) string { return dir }},
		{
			name: "the Mac's folder in it",
			locks: func(t *testing.T, _ *exports, _, folder string) string {
				if err := os.Mkdir(folder, 0o700); err != nil {
					t.Fatal(err)
				}
				return folder
			},
		},
		{
			name: "a file in that, which an export renames its new one onto",
			locks: func(t *testing.T, x *exports, _, folder string) string {
				x.exportNow(t.Context())
				return filepath.Join(folder, "export.json")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			state, dir := t.TempDir(), t.TempDir()
			id, err := export.InstallID(state)
			if err != nil {
				t.Fatal(err)
			}
			cfg := testConfig(at(start), &stubProber{}, testTokens.Read)
			cfg.StateDir, cfg.Export = state, config.Export{Dir: dir}
			x := newExports(cfg, tended, noRestart)
			locked := tt.locks(t, x, dir, export.Folder(dir, id))
			lock(t, locked)

			x.exportNow(t.Context())

			if got, want := x.report().Error, locked+" is locked: unlock it in the Finder, in Get Info"; got != want {
				t.Errorf("report().Error = %q, want %q", got, want)
			}
		})
	}
}

// lock locks the file or folder at path, as the Finder's Get Info does, until
// the test ends, skipping the test where it can't.
func lock(t *testing.T, path string) {
	t.Helper()
	if err := unix.Chflags(path, unix.UF_IMMUTABLE); err != nil {
		t.Skipf("can't lock %s: %v", path, err)
	}
	t.Cleanup(func() { _ = unix.Chflags(path, 0) })
}
