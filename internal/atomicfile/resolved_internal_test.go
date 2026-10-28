package atomicfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvedFailsWithWhyADirectoryCantBeFollowed(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root searches any directory")
	}
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	_, err := resolved(filepath.Join(locked, "gone", "file"))
	if !errors.Is(err, fs.ErrPermission) || strings.Contains(err.Error(), "isn't there") {
		t.Errorf("resolved() error = %v, want why the directory can't be searched", err)
	}
}
