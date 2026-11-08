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

// Places are where switchboard's config is: Dir, the config directory, as
// Dir gives it; and File, the config file read, and Themes, the themes
// directory, as ThemesDir gives it, each "" where it's in Dir, as it is but
// where the environment, or the command line, puts it elsewhere.
type Places struct {
	Dir, File, Themes string
}

// PlacesOf returns where switchboard's config is, file being the config file
// read.
func PlacesOf(getenv func(string) string, homeDir func() (string, error), file string) (Places, error) {
	dir, err := Dir(getenv, homeDir)
	if err != nil {
		return Places{}, err
	}
	themes, err := ThemesDir(getenv, homeDir)
	if err != nil {
		return Places{}, err
	}
	places := Places{Dir: dir}
	if !holds(dir, file) {
		places.File = file
	}
	if !holds(dir, themes) {
		places.Themes = themes
	}
	return places, nil
}

// Removal is what RemoveAll, or RemoveFile, did.
type Removal int

const (
	// NotThere is a directory, or a file, that wasn't there.
	NotThere Removal = iota
	// Removed is a directory removed, with everything in it, or a file.
	Removed
	// KeptFor is a directory removed but for the folder kept in it, and the
	// directories on the way there.
	KeptFor
	// Kept is a directory that's the folder kept itself, left whole.
	Kept
)

// RemoveAll removes dir, with everything in it but keep, a folder that's to
// stay wherever it is, as the export's does, and the directories on the way
// to it from dir. dir goes whole where keep is "" or isn't in it, or isn't
// there to keep, and stays whole where it's keep. Where keep is there in
// dir, or the way to it can't be read, dir is never removed whole. keep is found in dir
// however either is named, by what each is, not by its path. A dir that's a
// link is removed alone, never what it leads to. It goes on past what it
// can't remove, and fails, saying why.
func RemoveAll(dir, keep string) (Removal, error) {
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return NotThere, nil
	case err != nil:
		return NotThere, fmt.Errorf("look at %s: %w", dir, err)
	case info.Mode()&fs.ModeSymlink == 0 && sameFile(dir, keep):
		return Kept, nil
	case info.Mode()&fs.ModeSymlink == 0 && holds(dir, keep):
		kept, err := removeAllBut(dir, keep)
		if kept || err != nil || !missing(keep) {
			return KeptFor, err
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return Removed, fmt.Errorf("remove %s: %w", dir, err)
	}
	return Removed, nil
}

// RemoveFile removes the file at path, and reports whether it was there. A
// directory there is left alone, and RemoveFile fails, saying so.
func RemoveFile(path string) (Removal, error) {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return NotThere, nil
	case err != nil:
		return NotThere, fmt.Errorf("look at %s: %w", path, err)
	case info.IsDir():
		return NotThere, fmt.Errorf("%s is a directory, not a config file, so it's left alone", path)
	}
	if err := os.Remove(path); err != nil {
		return NotThere, fmt.Errorf("remove %s: %w", path, err)
	}
	return Removed, nil
}

// removeAllBut removes everything in dir but keep, and the directories on
// the way to it, which dir holds, and reports whether it came to keep, and
// kept it.
func removeAllBut(dir, keep string) (bool, error) {
	if sameFile(dir, keep) {
		return true, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", dir, err)
	}
	kept := false
	var errs []error
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() && holds(path, keep) {
			in, err := removeAllBut(path, keep)
			kept = kept || in
			errs = append(errs, err)
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	return kept, errors.Join(errs...)
}

// holds reports whether path is dir, or in it: as written, or else as what
// each is, path's links followed, so a path named through a link, or in
// another case on a filesystem that matches names in any, is found. "" is in
// nothing.
func holds(dir, path string) bool {
	if path == "" {
		return false
	}
	if rel, err := filepath.Rel(dir, path); err == nil && filepath.IsLocal(rel) {
		return true
	}
	at, err := os.Stat(dir)
	if err != nil {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = filepath.Clean(path)
	}
	for p := resolved; ; p = filepath.Dir(p) {
		if info, err := os.Stat(p); err == nil && os.SameFile(at, info) {
			return true
		}
		if filepath.Dir(p) == p {
			return false
		}
	}
}

// missing reports whether there's nothing at path.
func missing(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, fs.ErrNotExist)
}

// sameFile reports whether a and b are the one file, links followed.
func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	return err == nil && os.SameFile(ai, bi)
}
