package export

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
)

// Why a folder holding what an export writes isn't taken as a Mac's folder,
// as Folders tells it.
var (
	// ErrUnread is a folder whose export.json doesn't read, as a damaged
	// one doesn't.
	ErrUnread = errors.New("its export.json doesn't read")
	// ErrNoManifest is a folder holding an accounts record or days without
	// an export.json, as an export cut short leaves one, or a sync that
	// hasn't carried it whole.
	ErrNoManifest = errors.New("it holds an export without its export.json, as an export cut short or a sync not done leaves one: import it again once it's exported whole")
)

// Folders returns the Macs' folders to import from dir: dir itself, where
// it's one, else each folder in it that's one, by name, a Mac's folder being
// one holding an export.json that reads, as ReadManifest reads it, this
// Mac's own among them. A folder holding an export.json that doesn't read,
// or what an export writes without one, is passed over, and given to passed
// with why, ErrUnread or ErrNoManifest, so neither loses a Mac's history
// unsaid. It fails where one was exported by a newer switchboard than this,
// where there's none, and where dir can't be read.
func Folders(dir string, passed func(folder string, why error)) ([]string, error) {
	switch is, err := macsFolder(dir, passed); {
	case err != nil:
		return nil, err
	case is:
		return []string{dir}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("can't import from %s: %w", dir, err)
	}
	var folders []string
	for _, e := range entries {
		folder := filepath.Join(dir, e.Name())
		is, err := macsFolder(folder, passed)
		if err != nil {
			return nil, err
		}
		if is {
			folders = append(folders, folder)
		}
	}
	if len(folders) == 0 {
		return nil, fmt.Errorf("there's nothing to import in %s: it holds no Mac's folder, one with the export.json export writes", dir)
	}
	return folders, nil
}

// macsFolder reports whether folder is a Mac's folder, holding an
// export.json that reads, giving it to passed where it holds one that
// doesn't, or none beside what an export writes, as holdsExport says, and
// failing where it was exported by a newer switchboard, whose files this
// one may not read.
func macsFolder(folder string, passed func(folder string, why error)) (bool, error) {
	m, err := ReadManifest(folder)
	switch {
	case err != nil:
		if why := whyPassed(folder); why != nil {
			passed(folder, why)
		}
		return false, nil
	case m.Version > Version:
		return false, fmt.Errorf("can't import from %s: a newer switchboard exported it, its export.json of version %d, and this one reads version %d: upgrade switchboard to import it", folder, m.Version, Version)
	}
	return true, nil
}

// whyPassed is why folder, holding no export.json that reads, is passed
// over, where that's to be said: ErrUnread where it holds one, ErrNoManifest
// where it holds what an export writes, as holdsExport says; nil otherwise,
// as for a file, or a folder of anything else.
func whyPassed(folder string) error {
	if _, err := os.Stat(filepath.Join(folder, manifestFile)); err == nil {
		return ErrUnread
	}
	if holdsExport(folder) {
		return ErrNoManifest
	}
	return nil
}

// holdsExport reports whether folder holds what an export writes but
// export.json, as stagedFiles finds it: the accounts record, or a day's
// file. One whose files can't be listed holds none.
func holdsExport(folder string) bool {
	paths, err := stagedFiles(folder)
	return err == nil && slices.ContainsFunc(paths, func(path string) bool {
		_, err := os.Lstat(path)
		return err == nil && filepath.Base(path) != manifestFile
	})
}

// Stage stages the Macs' folders given in importDir, for the router to take
// in: first removing what's there, as an import cut short leaves it; then
// copying each folder's files into a folder of its own, named 1, 2, … in the
// order given, laid out as the folder lays them out, as stageFolder copies
// them. Where one can't be staged, it removes what it staged, so a copy as
// big as the export, made on a disk it may have filled, never stays.
func Stage(importDir string, folders []string) error {
	if err := os.RemoveAll(importDir); err != nil {
		return fmt.Errorf("remove what an import cut short left in %s: %w", importDir, err)
	}
	for i, folder := range folders {
		if err := stageFolder(folder, filepath.Join(importDir, strconv.Itoa(i+1))); err != nil {
			staged := fmt.Errorf("stage %s to import: %w", folder, err)
			if err := os.RemoveAll(importDir); err != nil {
				return errors.Join(staged, fmt.Errorf("remove what was staged in %s: %w", importDir, err))
			}
			return staged
		}
	}
	return nil
}

// stageFolder copies the files of the Mac's folder at folder into dst, which
// it makes, the user's alone, each where the folder holds it, as copyFile
// copies it, with its modification time: the files stagedFiles finds, and
// nothing else the folder holds.
func stageFolder(folder, dst string) error {
	if err := makeAll(dst); err != nil {
		return err
	}
	paths, err := stagedFiles(folder)
	if err != nil {
		return err
	}
	for _, path := range paths {
		rel, err := filepath.Rel(folder, path)
		if err != nil {
			return err
		}
		if _, err := copyFile(path, filepath.Join(dst, rel), makeAll); err != nil {
			return err
		}
	}
	return nil
}

// stagedFiles returns where the files to stage of the Mac's folder at folder
// are: each day's compressed file of each kind of line, as Files' Compressed
// finds them, each day's summary, as ledger.SummaryDates finds them, the
// accounts record, which copyFile passes over where there's none, and
// export.json, for the router to tell whose folder it is and how it was
// exported; passing over anything else, as a temporary file an export cut
// short left.
func stagedFiles(folder string) ([]string, error) {
	var paths []string
	for _, files := range linesIn(folder, slog.New(slog.DiscardHandler)) {
		dates, err := files.Compressed()
		if err != nil {
			return nil, fmt.Errorf("read the %s: %w", files.Name, err)
		}
		for _, date := range dates {
			paths = append(paths, files.CompressedPath(date))
		}
	}
	dir := ledger.Dir(folder)
	dates, err := ledger.SummaryDates(dir)
	if err != nil {
		return nil, fmt.Errorf("read the request ledger's summaries: %w", err)
	}
	for _, date := range dates {
		paths = append(paths, ledger.SummaryFile(dir, date))
	}
	return append(paths, identity.NewFile(folder).Path(), filepath.Join(folder, manifestFile)), nil
}
