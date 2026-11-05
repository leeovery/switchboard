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
	"slices"
	"strings"
	"time"
)

// maxLinks is how many links Target follows, as a loop of them leads
// nowhere.
const maxLinks = 40

// Target returns where writing to path leads: path itself, or where the links
// it names lead, which needn't be there yet. Write renames a file into place,
// which would replace a link at path, so a file kept elsewhere, and linked to,
// is written at its target, cleaned as the system follows the way there,
// whatever links it goes through. It fails where the way there goes back, by
// "..", out of a directory that isn't there, which the system can't follow,
// so where it leads can't be known.
func Target(path string) (string, error) {
	named := path
	// followed is set once a link is followed, which leaves path uncleaned,
	// to clean once it's followed to its end.
	followed := false
	for range maxLinks {
		info, err := os.Lstat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return reached(named, path, followed)
		case err != nil:
			return "", err
		case info.Mode()&fs.ModeSymlink == 0:
			return reached(named, path, followed)
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
		}
		path, followed = dest, true
	}
	return "", fmt.Errorf("%s: too many links", named)
}

// reached is path, reached from named: cleaned, as resolved cleans it, once
// a link has been followed, else as it is. It fails where resolved does.
func reached(named, path string, followed bool) (string, error) {
	clean, err := resolved(path)
	switch {
	case err != nil:
		return "", fmt.Errorf("%s: %w", named, err)
	case followed:
		return clean, nil
	}
	return path, nil
}

// resolved is path with the links in its directory followed as the system
// follows them, a ".." after one going back from where it leads, and cleaned:
// of a directory that isn't there yet, the longest part of it that is, and
// the rest after it. It's path as it is where no part can be followed. It
// fails where a ".." comes after a directory that isn't there, as the system
// would go no further, and where a directory can't be followed for another
// reason, as one that can't be searched.
func resolved(path string) (string, error) {
	dir, rest := split(path)
	for {
		followed, err := filepath.EvalSymlinks(dir)
		if err == nil {
			parts := strings.Split(rest, string(filepath.Separator))
			if slices.Contains(parts[1:], "..") {
				return "", fmt.Errorf("leads back out of %s, which isn't there", filepath.Join(followed, parts[0]))
			}
			return filepath.Join(followed, rest), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent, name := split(dir)
		if parent == dir {
			return path, nil
		}
		dir, rest = parent, name+string(filepath.Separator)+rest
	}
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
func WriteFrom(path string, perm fs.FileMode, write func(w io.Writer) error) error {
	return WriteChecked(path, perm, write, func() error { return nil })
}

// WriteChecked writes as WriteFrom does, but calls check once the file is
// written whole beside path, synced and closed, just before it's renamed
// into place: where check fails, nothing is written, and its error is
// returned. A caller that must not replace a file changed since it read it
// checks there, leaving the change as little time as can be.
func WriteChecked(path string, perm fs.FileMode, write func(w io.Writer) error, check func() error) error {
	return writeBeside(path, perm, write, func(string) error { return check() })
}

// WriteModified writes as WriteFrom does, the file last modified at modified
// before it's renamed into place, as a copy that keeps its original's time
// is: so none is found whole but with another time.
func WriteModified(path string, perm fs.FileMode, modified time.Time, write func(w io.Writer) error) error {
	return writeBeside(path, perm, write, func(tmp string) error { return os.Chtimes(tmp, time.Time{}, modified) })
}

// StageModified writes what write writes to a hidden file beside path, as
// WriteModified would, last modified at modified, synced and closed, and
// returns where, for the caller to rename into place or remove: nothing is
// left beside path where it fails.
func StageModified(path string, perm fs.FileMode, modified time.Time, write func(w io.Writer) error) (string, error) {
	return stage(path, perm, write, func(tmp string) error { return os.Chtimes(tmp, time.Time{}, modified) })
}

// Leftovers are the files beside path that writes to it left there, cut
// short, or staged and neither renamed into place nor removed: those named
// as a file written beside it is.
func Leftovers(path string) ([]string, error) {
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var found []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), besidePrefix(path)) {
			found = append(found, filepath.Join(dir, e.Name()))
		}
	}
	return found, nil
}

// LeftoverOf returns the name of the file that one with the given name was
// written beside, as Write and its kin write beside it, where it's named as
// such a file is: hidden, that file's name, a dot, then the digits that set
// it apart. It reports false for any other name, as one a sync service keeps
// beside a file, such as iCloud Drive's .<name>.icloud for one it hasn't
// downloaded, which is never one of switchboard's. A file so named is one
// being written, or one a write cut short left.
func LeftoverOf(name string) (string, bool) {
	rest, hidden := strings.CutPrefix(name, ".")
	i := strings.LastIndexByte(rest, '.')
	if !hidden || i <= 0 || i == len(rest)-1 || strings.Trim(rest[i+1:], "0123456789") != "" {
		return "", false
	}
	return rest[:i], true
}

// besidePrefix is the name of a file written beside path, before its random
// part: hidden, path's own name, then a dot.
func besidePrefix(path string) string {
	return "." + filepath.Base(path) + "."
}

// writeBeside writes what write writes to a file beside path, with the
// permissions perm, then, once it's synced and closed, calls before with its
// path, and renames it into place: where before fails, nothing is written,
// and its error is returned.
func writeBeside(path string, perm fs.FileMode, write func(w io.Writer) error, before func(tmp string) error) error {
	tmp, err := stage(path, perm, write, before)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// stage writes what write writes to a hidden file beside path, with the
// permissions perm, then, once it's synced and closed, calls before with its
// path, and returns it: where anything fails, before among them, the file is
// removed.
func stage(path string, perm fs.FileMode, write func(w io.Writer) error, before func(tmp string) error) (_ string, err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), besidePrefix(path)+"*")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if err = tmp.Chmod(perm); err != nil {
		return "", err
	}
	buffered := bufio.NewWriter(tmp)
	if err = write(buffered); err != nil {
		return "", err
	}
	if err = buffered.Flush(); err != nil {
		return "", err
	}
	if err = tmp.Sync(); err != nil {
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	if err = before(tmp.Name()); err != nil {
		return "", err
	}
	return tmp.Name(), nil
}
