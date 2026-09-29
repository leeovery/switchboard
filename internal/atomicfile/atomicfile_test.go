package atomicfile_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

func TestWrite(t *testing.T) {
	tests := []struct {
		name string
		// before is what the file holds before it's written, when it's there.
		before *string
		perm   fs.FileMode
	}{
		{name: "a new file", perm: 0o600},
		{name: "in place of a file", before: new("old"), perm: 0o600},
		{name: "in place of a file, with other permissions", before: new("old"), perm: 0o644},
		{name: "with permissions a umask would take away", perm: 0o666},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "file")
			if tt.before != nil {
				if err := os.WriteFile(path, []byte(*tt.before), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			if err := atomicfile.Write(path, []byte("new\n"), tt.perm); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != "new\n" {
				t.Errorf("the file holds %q (%v), want %q", data, err, "new\n")
			}
			if info, err := os.Stat(path); err != nil || info.Mode() != tt.perm {
				t.Errorf("the file's mode = %v (%v), want %v", info.Mode(), err, tt.perm)
			}
			if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
				t.Errorf("the directory holds %v (%v), want the file alone", entries, err)
			}
		})
	}
}

func TestWriteLeavesNothingBehindWhenItFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	// A directory, with something in it, where the file goes.
	if err := os.MkdirAll(filepath.Join(path, "kept"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := atomicfile.Write(path, []byte("new\n"), 0o600); err == nil {
		t.Fatal("Write() succeeded, want it to fail to replace a directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "file" || !entries[0].IsDir() {
		t.Errorf("the directory holds %v (%v), want what was there alone", entries, err)
	}
}
