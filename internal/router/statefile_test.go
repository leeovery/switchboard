package router

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomic(t *testing.T) {
	tests := []struct {
		name string
		// before is what the file holds before the write, and its mode, or
		// nothing when there's no file.
		before     string
		beforeMode fs.FileMode
	}{
		{name: "creates the file, readable by its owner alone"},
		{name: "replaces a file, and a mode more open than its own", before: `{"version": 1}`, beforeMode: 0o644},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "state.json")
			if tt.before != "" {
				if err := os.WriteFile(path, []byte(tt.before), tt.beforeMode); err != nil {
					t.Fatal(err)
				}
			}

			if err := writeAtomic(path, []byte("written")); err != nil {
				t.Fatalf("writeAtomic() error = %v", err)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != "written" {
				t.Errorf("file holds %q (%v), want what was written", data, err)
			}
			if info, err := os.Stat(path); err != nil || info.Mode() != 0o600 {
				t.Errorf("file's mode is %v (%v), want -rw-------", info.Mode(), err)
			}
			checkAlone(t, dir)
		})
	}
}

func TestAWriteThatFailsLeavesTheFileAsItWas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := writeAtomic(path, []byte("after")); err == nil {
		t.Fatal("writeAtomic() into a directory it can't write = nil, want an error")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "before" {
		t.Errorf("file holds %q (%v), want it as it was", data, err)
	}
	checkAlone(t, dir)
}

// checkAlone checks dir holds the state file and nothing else.
func checkAlone(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Errorf("directory holds %v, want state.json alone", entries)
	}
}
