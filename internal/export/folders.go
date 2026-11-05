package export

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// leftoverAge is how long ago a file staged in a Mac's folder must last have
// been modified for an export to take it for one a write cut short left, as
// a router stopping mid-export leaves one, rather than one another export,
// as one run by hand beside the router's, is writing still.
const leftoverAge = time.Hour

// ErrNoFolder is the error of an export by an Exporter with NoParents into a
// folder that isn't there and can't be made, as one on a disk that isn't
// mounted, or one gone as it exports, as with its disk ejected.
var ErrNoFolder = errors.New("it isn't there")

// makeDir makes dir, the folder exported into, the user's alone, where it
// isn't there, in the folder it goes in, which must be there: failing with
// ErrNoFolder where that isn't there either, or where dir can't be made in
// it, saying why, as /Volumes/<disk> of a disk not mounted can't be, in a
// folder only the system can write. A dir there already is never made, as a
// disk's mount point in such a folder is there.
func makeDir(dir string) error {
	err := os.Mkdir(dir, 0o700)
	switch {
	case err == nil, errors.Is(err, fs.ErrExist):
		return nil
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%w, nor the folder it goes in", ErrNoFolder)
	}
	info, look := os.Stat(dir)
	switch {
	case look == nil && info.IsDir():
		return nil
	case errors.Is(look, fs.ErrNotExist):
		return fmt.Errorf("%w, and can't be made: %w", ErrNoFolder, err)
	}
	return err
}

// making returns what makes a folder an export writes into in the folder at
// root, the user's alone, where it isn't there: with every folder it goes
// in; but with NoParents, a level at a time below root, which must be there,
// as makeBelow makes it, so none is made once root has gone.
func (e Exporter) making(root string) func(dir string) error {
	if !e.NoParents {
		return makeAll
	}
	return func(dir string) error { return makeBelow(root, dir) }
}

// makeAll makes the folder at dir, the user's alone, with every folder it
// goes in, where they aren't there.
func makeAll(dir string) error {
	return os.MkdirAll(dir, 0o700)
}

// makeBelow makes dir, a folder in the folder at root, a level at a time,
// each in the one before, as makeIn makes it: none where root isn't there.
func makeBelow(root, dir string) error {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return err
	}
	for part := range strings.SplitSeq(rel, string(filepath.Separator)) {
		if part == "." {
			continue
		}
		root = filepath.Join(root, part)
		if err := makeIn(root); err != nil {
			return err
		}
	}
	return nil
}

// makeIn makes the folder at path, the user's alone, where it isn't there,
// in the folder it goes in, failing with ErrNoFolder where that isn't there.
func makeIn(path string) error {
	err := os.Mkdir(path, 0o700)
	switch {
	case err == nil, errors.Is(err, fs.ErrExist):
		return nil
	case errors.Is(err, fs.ErrNotExist):
		return ErrNoFolder
	}
	return err
}

// removeLeftovers removes the files staged in the Mac's folder at folder, and
// in the folders in it an export writes into, that were last modified
// leftoverAge or more before now, as leftover says: those writes cut short
// left, as an export a router stopping cut short leaves them, which sync as
// any file does. One that can't be removed is warned of; a folder that can't
// be listed is passed over, as the export's writes into it say why, where
// they fail.
func (e Exporter) removeLeftovers(folder string) {
	for _, dir := range []string{folder, ledger.Dir(folder), readings.Dir(folder)} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !e.leftover(entry) {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				e.Logger.Warn("can't remove a file an export cut short left", "path", path, "error", err)
			}
		}
	}
}

// leftover reports whether entry is a file staged beside one an export
// writes, as atomicfile names it, last modified leftoverAge or more before
// now.
func (e Exporter) leftover(entry fs.DirEntry) bool {
	if _, staged := atomicfile.LeftoverOf(entry.Name()); !staged || !entry.Type().IsRegular() {
		return false
	}
	info, err := entry.Info()
	return err == nil && e.Now().Sub(info.ModTime()) >= leftoverAge
}
