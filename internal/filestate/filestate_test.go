package filestate_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/filestate"
)

// A file stands the same while it's the same file, unchanged: written
// again, or another file moved over it, it has moved on; a path that leads
// nowhere hasn't, and two that lead nowhere stand alike.
func TestAFileStandsTheSameWhileItsTheSameFileUnchanged(t *testing.T) {
	dir := t.TempDir()
	path, other := filepath.Join(dir, "config.toml"), filepath.Join(dir, "saved.toml")
	write(t, path, time.Unix(1_700_000_000, 0))
	was := filestate.Of(path)

	write(t, other, time.Unix(1_700_000_000, 0))
	replaced := func(t *testing.T) {
		if err := os.Rename(other, path); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name          string
		change        func(t *testing.T)
		same, movedTo bool
	}{
		{name: "unchanged", change: func(*testing.T) {}, same: true},
		{name: "written again", change: func(t *testing.T) { write(t, path, time.Unix(1_700_000_060, 0)) }, movedTo: true},
		{name: "another file moved over it, as an editor saves", change: replaced, movedTo: true},
		{name: "gone", change: func(t *testing.T) { remove(t, path) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.change(t)
			now := filestate.Of(path)
			if got := now.Same(was); got != tt.same {
				t.Errorf("Same is %v, want %v", got, tt.same)
			}
			if got := was.MovedTo(now); got != tt.movedTo {
				t.Errorf("MovedTo is %v, want %v", got, tt.movedTo)
			}
			was = now
		})
	}
	if gone := filestate.Of(path); gone.There() || !gone.Same(filestate.State{}) {
		t.Errorf("a path leading nowhere is there %v, or stands otherwise than none; want none", gone.There())
	}
}

// Info is how the file stood as its info was read, as Of has it.
func TestInfoStandsAsOfDoes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binary")
	write(t, path, time.Unix(1_700_000_000, 0))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !filestate.Info(info).Same(filestate.Of(path)) || filestate.Info(nil).There() {
		t.Error("Info stands otherwise than Of, or nil is there; want alike, and none")
	}
}

// write writes the file at path, modified at modified.
func write(t *testing.T, path string, modified time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte("[[accounts]]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}

// remove removes the file at path.
func remove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
