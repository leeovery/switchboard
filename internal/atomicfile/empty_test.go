package atomicfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

func TestRemoveIfEmpty(t *testing.T) {
	tests := []struct {
		name string
		// make makes what's at path.
		make        func(t *testing.T, path string)
		wantRemoved bool
		wantThere   bool
	}{
		{name: "not there", make: func(*testing.T, string) {}},
		{name: "empty", make: mkdir, wantRemoved: true},
		{
			name: "holding a file",
			make: func(t *testing.T, path string) {
				mkdir(t, path)
				write(t, filepath.Join(path, "kept"))
			},
			wantThere: true,
		},
		{name: "a file", make: write, wantThere: true},
		{
			name: "a link to an empty directory",
			make: func(t *testing.T, path string) {
				target := filepath.Join(filepath.Dir(path), "target")
				mkdir(t, target)
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			},
			wantThere: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "dir")
			tt.make(t, path)

			removed, err := atomicfile.RemoveIfEmpty(path)
			if err != nil || removed != tt.wantRemoved {
				t.Errorf("RemoveIfEmpty() = %v, %v, want %v", removed, err, tt.wantRemoved)
			}
			if _, err := os.Lstat(path); (err == nil) != tt.wantThere {
				t.Errorf("what was at the path: %v, want it there: %v", err, tt.wantThere)
			}
		})
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
