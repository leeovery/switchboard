// Package atomicfile writes files whole or not at all: each is written beside
// where it goes, synced, then renamed into place, so no reader finds half of
// one, and a crash can't cost the one it replaces.
package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// maxLinks is how many links Target follows, as a loop of them leads
// nowhere.
const maxLinks = 40

// Target returns where writing to path leads: path itself, or where the links
// it names lead, which needn't be there yet. Write renames a file into place,
// which would replace a link at path, so a file kept elsewhere, and linked to,
// is written at its target.
func Target(path string) (string, error) {
	named := path
	for range maxLinks {
		info, err := os.Lstat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return path, nil
		case err != nil:
			return "", err
		case info.Mode()&fs.ModeSymlink == 0:
			return path, nil
		}
		dest, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(dest) {
			// A relative link leads on from the directory it's in, as the system
			// finds it, following any link to that directory, where joining the
			// two as text would take a ".." in the link back through it.
			dir, err := filepath.EvalSymlinks(filepath.Dir(path))
			if err != nil {
				return "", err
			}
			dest = filepath.Join(dir, dest)
		}
		path = dest
	}
	return "", fmt.Errorf("%s: too many links", named)
}

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
