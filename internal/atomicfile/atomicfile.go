// Package atomicfile writes files whole or not at all: each is written beside
// where it goes, synced, then renamed into place, so no reader finds half of
// one, and a crash can't cost the one it replaces. One found corrupt is set
// aside beside where it was, for someone to look at.
package atomicfile

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
	// relative is set once a relative link is followed, which leaves path
	// uncleaned, to clean once it's followed to its end.
	relative := false
	reached := func() string {
		if relative {
			return resolved(path)
		}
		return path
	}
	for range maxLinks {
		info, err := os.Lstat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return reached(), nil
		case err != nil:
			return "", err
		case info.Mode()&fs.ModeSymlink == 0:
			return reached(), nil
		}
		dest, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(dest) {
			// A relative link leads on from the directory it's in, as the system
			// reads it: its text after that directory, uncleaned, as cleaning
			// would take a ".." in it back through a linked directory before
			// it, where the system goes back from where that link leads.
			dir, _ := split(path)
			dest = dir + string(filepath.Separator) + dest
			relative = true
		}
		path = dest
	}
	return "", fmt.Errorf("%s: too many links", named)
}

// resolved is path with the links in its directory followed as the system
// follows them, a ".." after one going back from where it leads, and cleaned:
// path as it is where its directory can't be followed, as where it isn't
// there.
func resolved(path string) string {
	dir, base := split(path)
	followed, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return path
	}
	return filepath.Join(followed, base)
}

// split splits path at its last separator, into its directory and its name,
// as text, without cleaning either, as filepath.Split would its directory:
// "." for a path with no directory.
func split(path string) (dir, base string) {
	i := strings.LastIndexByte(path, filepath.Separator)
	switch {
	case i < 0:
		return ".", path
	case i == 0:
		return string(filepath.Separator), path[1:]
	}
	return path[:i], path[i+1:]
}

// Write writes data to a file at path, with the permissions perm, in place of
// any file there: whole, or not at all.
func Write(path string, data []byte, perm fs.FileMode) error {
	return WriteFrom(path, perm, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

// WriteFrom writes what write writes, to the writer it's given, to a file at
// path, with the permissions perm, in place of any file there: whole, or not
// at all, as Write does, and not at all where write fails.
func WriteFrom(path string, perm fs.FileMode, write func(w io.Writer) error) (err error) {
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
	buffered := bufio.NewWriter(tmp)
	if err = write(buffered); err != nil {
		return err
	}
	if err = buffered.Flush(); err != nil {
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
