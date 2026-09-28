package testguard

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// watched are the directories in the real home that a test reaching past its
// isolation would change: switchboard's config, and its state, logs and all.
var watched = []string{
	filepath.Join(".config", "switchboard"),
	filepath.Join(".local", "state", "switchboard"),
}

// snapshot is what the watched directories hold, by path from the home.
type snapshot map[string]entry

// entry is what's at a path: its type, size and modification time.
type entry struct {
	kind    fs.FileMode
	size    int64
	modTime int64
}

// take notes what each watched directory in home holds, the directory and
// everything under it, following symlinks: a config kept elsewhere, such as
// with dotfiles, and linked in is the real one all the same. One that isn't
// there holds nothing, and so does an unknown home.
func take(home string) snapshot {
	if home == "" {
		return nil
	}
	s := make(snapshot)
	for _, dir := range watched {
		root, err := filepath.EvalSymlinks(filepath.Join(home, dir))
		if err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			info, err := os.Stat(path)
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			s[filepath.ToSlash(filepath.Join(dir, rel))] = entry{kind: info.Mode().Type(), size: info.Size(), modTime: info.ModTime().UnixNano()}
			return nil
		})
	}
	return s
}

// changes lists what differs from before to after, a line a path, in order.
func changes(before, after snapshot) []string {
	paths := make(map[string]bool)
	for path := range before {
		paths[path] = true
	}
	for path := range after {
		paths[path] = true
	}
	var lines []string
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		was, wasThere := before[path]
		is, isThere := after[path]
		switch {
		case !wasThere:
			lines = append(lines, "the real ~/"+path+" was created")
		case !isThere:
			lines = append(lines, "the real ~/"+path+" was removed")
		case was != is:
			lines = append(lines, "the real ~/"+path+" was modified")
		}
	}
	return lines
}
