//go:build unix

package theme_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/theme"
)

func TestTheThemesDirectoryIsReadForRegularFilesAlone(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "lake.theme"), file(nil))
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe.theme"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink(t, os.DevNull, filepath.Join(dir, "device.theme"))
	lib := theme.NewLibrary(dir)

	// Reading a FIFO waits for a writer that never comes.
	type loaded struct {
		listed []string
		err    error
	}
	done := make(chan loaded, 1)
	go func() {
		_, err := lib.Load("pipe")
		done <- loaded{listed: listed(lib.List()), err: err}
	}()
	select {
	case got := <-done:
		if want := []string{"amber", "exchange", "lake", "nord", "terminal", "tokyo-night", "tokyo-night-day"}; !slices.Equal(got.listed, want) {
			t.Errorf("List() lists %q, want %q, past the FIFO and the device", got.listed, want)
		}
		if p, ok := errors.AsType[*theme.Problem](got.err); !ok || p.Reason != "not found" {
			t.Errorf("Load(pipe) error = %v, want not found, a FIFO being no theme", got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the themes directory is still being read after 5s: a FIFO in it was read")
	}
}
