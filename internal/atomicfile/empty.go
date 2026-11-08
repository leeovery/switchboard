package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// RemoveIfEmpty removes the directory at dir where nothing's in it, and
// reports whether it did: one that isn't there, or holds anything, and
// anything there that isn't a directory, is left as it is.
func RemoveIfEmpty(dir string) (bool, error) {
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("look at %s: %w", dir, err)
	case !info.IsDir():
		return false, nil
	}
	err = os.Remove(dir)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTEMPTY), errors.Is(err, syscall.EEXIST):
		return false, nil
	}
	return false, fmt.Errorf("remove %s: %w", dir, err)
}
