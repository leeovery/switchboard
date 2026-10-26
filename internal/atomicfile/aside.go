package atomicfile

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	// asideMark is what a file set aside is named with, after the name it
	// had, before when it was set aside.
	asideMark = ".corrupt-"
	// keptAside is how many of the files set aside of each name are kept: the
	// newest.
	keptAside = 3
)

// SetAside renames the file at path, found corrupt, to <path>.corrupt-<now in
// Unix seconds>, for someone to look at, and returns where it went: "" where
// it couldn't go. It keeps that, whatever times the others' names give, as a
// clock once set ahead names them after now, and of those set aside of the
// file before, the newest, by those times, to keptAside in all, removing the
// older; and returns where the file went with the error of any it couldn't
// remove.
func SetAside(path string, now time.Time) (string, error) {
	aside := fmt.Sprintf("%s%s%d", path, asideMark, now.Unix())
	if err := os.Rename(path, aside); err != nil {
		return "", err
	}
	return aside, removeOlderAside(path, aside)
}

// setAside is a file set aside, by its name, and when it was set aside, as
// its name gives it in Unix seconds.
type setAside struct {
	name string
	at   int64
}

// removeOlderAside removes the files set aside of the file at path but kept,
// the one just set aside, and the newest of the others, to keptAside in all,
// leaving anything else in its directory alone.
func removeOlderAside(path, kept string) error {
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	prefix := filepath.Base(path) + asideMark
	var aside []setAside
	for _, e := range entries {
		if at, ok := asideAt(e, prefix); ok && e.Name() != filepath.Base(kept) {
			aside = append(aside, setAside{name: e.Name(), at: at})
		}
	}
	slices.SortFunc(aside, func(a, b setAside) int { return cmp.Compare(b.at, a.at) })
	var errs []error
	for _, older := range aside[min(keptAside-1, len(aside)):] {
		if err := os.Remove(filepath.Join(dir, older.name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// asideAt returns when the entry, a file set aside, as its name begins with
// prefix, was set aside, as its name gives it, reporting false for anything
// else.
func asideAt(e fs.DirEntry, prefix string) (int64, bool) {
	stamp, ok := strings.CutPrefix(e.Name(), prefix)
	if !ok || e.IsDir() {
		return 0, false
	}
	at, err := strconv.ParseInt(stamp, 10, 64)
	return at, err == nil
}
