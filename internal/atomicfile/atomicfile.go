// Package atomicfile writes files whole or not at all: each is written beside
// where it goes, synced, then renamed into place, so no reader finds half of
// one, and a crash can't cost the one it replaces.
package atomicfile

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Write writes data to a file at path, with the permissions perm, in place of
// any file there: whole, or not at all.
func Write(path string, data []byte, perm fs.FileMode) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if err = tmp.Chmod(perm); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
