package theme_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
)

func TestRemoveFilesTakesTheThemesAlone(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "dotfiles")
	write(t, filepath.Join(dir, "lake.theme"), file(nil))
	write(t, filepath.Join(dir, "LOUD.THEME"), "")
	write(t, filepath.Join(root, "kept.theme"), file(nil))
	symlink(t, filepath.Join(root, "kept.theme"), filepath.Join(dir, "linked.theme"))
	write(t, filepath.Join(dir, ".lake.theme.swp"), "")
	write(t, filepath.Join(dir, "starship.toml"), "")

	removed, err := theme.NewLibrary(dir).RemoveFiles()
	if err != nil || removed != 3 {
		t.Errorf("RemoveFiles() = %d, %v; want the 3 theme files removed", removed, err)
	}
	for name, kept := range map[string]bool{"lake.theme": false, "LOUD.THEME": false, "linked.theme": false, ".lake.theme.swp": true, "starship.toml": true} {
		if _, err := os.Lstat(filepath.Join(dir, name)); (err == nil) != kept {
			t.Errorf("%s: %v, want it kept: %v", name, err, kept)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "kept.theme")); err != nil {
		t.Errorf("what the link led to: %v, want it kept", err)
	}
}

func TestRemoveFilesRemovesTheDirectoryOnceEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	write(t, filepath.Join(dir, "lake.theme"), file(nil))

	if removed, err := theme.NewLibrary(dir).RemoveFiles(); err != nil || removed != 1 {
		t.Errorf("RemoveFiles() = %d, %v; want the theme removed", removed, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the themes directory: %v, want it gone", err)
	}
	if removed, err := theme.NewLibrary(dir).RemoveFiles(); err != nil || removed != 0 {
		t.Errorf("RemoveFiles(), again = %d, %v; want none there", removed, err)
	}
}
