package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// ExportFolderOf returns the folder the config file at path has the router
// export into, as Export.Folder gives it, read whatever else in the file is
// wrong: "" where it gives none, or there's no file. It fails where the file
// can't be read as TOML, or its export.dir as one, as then where the export
// goes can't be told.
func ExportFolderOf(path string, homeDir func() (string, error)) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read config: %w", err)
	}
	var only struct {
		Export fileExport `toml:"export"`
	}
	if _, err := toml.Decode(string(data), &only); err != nil {
		return "", unparsed(path, err)
	}
	export, err := parseExport(only.Export.Dir)
	if err != nil {
		return "", invalid(path, err)
	}
	return export.Folder(homeDir)
}

// Places returns where switchboard's config is, file being the config file
// read: the config directory, as Dir gives it, then file and the themes
// directory, as ThemesDir gives it, each where the environment, or the
// command line, puts it outside that directory.
func Places(getenv func(string) string, homeDir func() (string, error), file string) ([]string, error) {
	dir, err := Dir(getenv, homeDir)
	if err != nil {
		return nil, err
	}
	themes, err := ThemesDir(getenv, homeDir)
	if err != nil {
		return nil, err
	}
	places := []string{dir}
	for _, place := range []string{file, themes} {
		if !holds(dir, place) {
			places = append(places, place)
		}
	}
	return places, nil
}

// Removal is what RemoveAll did to a directory.
type Removal int

const (
	// NotThere is a directory that wasn't there.
	NotThere Removal = iota
	// Removed is a directory removed, with everything in it.
	Removed
	// KeptFor is a directory removed but for the folder kept in it, and the
	// directories on the way there.
	KeptFor
)

// RemoveAll removes dir, with everything in it but keep, a folder that's to
// stay wherever it is, as the export's does, and the directories on the way
// to it from dir: dir goes whole where keep is "" or isn't in it, and stays
// whole where keep is dir. A file at dir is removed as a directory would
// be. It goes on past what it can't remove, and fails, saying why.
func RemoveAll(dir, keep string) (Removal, error) {
	if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
		return NotThere, nil
	} else if err != nil {
		return NotThere, fmt.Errorf("look at %s: %w", dir, err)
	}
	if !holds(dir, keep) {
		if err := os.RemoveAll(dir); err != nil {
			return Removed, fmt.Errorf("remove %s: %w", dir, err)
		}
		return Removed, nil
	}
	return KeptFor, removeAllBut(dir, keep)
}

// removeAllBut removes everything in dir but keep, and the directories on
// the way to it, which dir holds.
func removeAllBut(dir, keep string) error {
	if filepath.Clean(dir) == filepath.Clean(keep) {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	var errs []error
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() && holds(path, keep) {
			errs = append(errs, removeAllBut(path, keep))
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

// holds reports whether path is dir, or in it, as written: "" is in
// nothing.
func holds(dir, path string) bool {
	if path == "" {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && filepath.IsLocal(rel)
}
