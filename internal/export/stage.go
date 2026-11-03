package export

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// Folders returns the Macs' folders to import from dir: dir itself, where
// it's one, else each folder in it that's one, by name, a Mac's folder being
// one holding an export.json that reads, as ReadManifest reads it, this
// Mac's own among them. It fails where one was exported by a newer
// switchboard than this, where there's none, and where dir can't be read.
func Folders(dir string) ([]string, error) {
	switch is, err := macsFolder(dir); {
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
		is, err := macsFolder(folder)
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
// export.json that reads, failing where it was exported by a newer
// switchboard, whose files this one may not read.
func macsFolder(folder string) (bool, error) {
	m, err := ReadManifest(folder)
	switch {
	case err != nil:
		return false, nil
	case m.Version > Version:
		return false, fmt.Errorf("can't import from %s: a newer switchboard exported it, its export.json of version %d, and this one reads version %d: upgrade switchboard to import it", folder, m.Version, Version)
	}
	return true, nil
}

// Stage stages the Macs' folders given in importDir, for the router to take
// in: first removing what's there, as an import cut short leaves it; then
// copying each folder's files into a folder of its own, named 1, 2, … in the
// order given, laid out as the folder lays them out, as stageFolder copies
// them.
func Stage(importDir string, folders []string) error {
	if err := os.RemoveAll(importDir); err != nil {
		return fmt.Errorf("remove what an import cut short left in %s: %w", importDir, err)
	}
	for i, folder := range folders {
		if err := stageFolder(folder, filepath.Join(importDir, strconv.Itoa(i+1))); err != nil {
			return fmt.Errorf("stage %s to import: %w", folder, err)
		}
	}
	return nil
}

// stageFolder copies the files of the Mac's folder at folder into dst, which
// it makes, the user's alone, each where the folder holds it, as copyFile
// copies it, with its modification time: the files stagedFiles finds, and
// nothing else the folder holds.
func stageFolder(folder, dst string) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
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
		if _, err := copyFile(path, filepath.Join(dst, rel)); err != nil {
			return err
		}
	}
	return nil
}

// stagedFiles returns where the files to stage of the Mac's folder at folder
// are: each day's compressed file of each kind of line, as compressedIn
// finds them, and each day's summary, passing over anything else, as a
// temporary file an export cut short left.
func stagedFiles(folder string) ([]string, error) {
	var paths []string
	for _, files := range linesIn(folder, slog.New(slog.DiscardHandler)) {
		compressed, err := compressedIn(files)
		if err != nil {
			return nil, fmt.Errorf("read the %s: %w", files.Name, err)
		}
		paths = append(paths, compressed...)
	}
	dir := ledger.Dir(folder)
	dates, err := ledger.SummaryDates(dir)
	if err != nil {
		return nil, fmt.Errorf("read the request ledger's summaries: %w", err)
	}
	for _, date := range dates {
		paths = append(paths, ledger.SummaryFile(dir, date))
	}
	return paths, nil
}

// compressedIn returns where the compressed files of the days files holds
// are, as export writes them, passing over anything else, a day's plain file
// among them: none where their directory isn't there.
func compressedIn(files *dayfile.Files) ([]string, error) {
	entries, err := files.List()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		date, ok := files.DateOf(e.Name())
		if path := files.CompressedPath(date); ok && e.Type().IsRegular() && e.Name() == filepath.Base(path) {
			paths = append(paths, path)
		}
	}
	return paths, nil
}
