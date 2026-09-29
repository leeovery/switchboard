package testguard

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// switchboard's directories in the real home.
var (
	configDir = filepath.Join(".config", "switchboard")
	stateDir  = filepath.Join(".local", "state", "switchboard")
)

// realHome watches switchboard's directories in the real home, each as
// closely as a live switchboard running alongside the tests allows. Nothing
// live writes the config, so any change there is a test's. A live router
// writes its state as it runs, its logs and state.json among it, so only the
// state directory appearing is a test's; and the OS sandbox denies a test any
// write there anyway.
type realHome struct {
	// dir is the real home, "" when there's none to watch.
	dir string
	// config is what the config directory held as the tests began.
	config snapshot
	// hadState is whether the state directory was there as they began.
	hadState bool
}

func watchHome(dir string) realHome {
	if dir == "" {
		return realHome{}
	}
	return realHome{dir: dir, config: take(dir, configDir), hadState: exists(filepath.Join(dir, stateDir))}
}

// changes lists what's changed in the real home that a test would have
// changed, a line each.
func (h realHome) changes() []string {
	if h.dir == "" {
		return nil
	}
	lines := diff(h.config, take(h.dir, configDir))
	if !h.hadState && exists(filepath.Join(h.dir, stateDir)) {
		lines = append(lines, "the real ~/"+filepath.ToSlash(stateDir)+" appeared")
	}
	return lines
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// snapshot is what a directory holds, by path from the home.
type snapshot map[string]entry

// entry is what's at a path: its type, size and modification time.
type entry struct {
	kind    fs.FileMode
	size    int64
	modTime int64
}

// take notes what the directory dir in home holds, the directory and
// everything under it, following symlinks: a config kept elsewhere, such as
// with dotfiles, and linked in is the real one all the same. One that isn't
// there holds nothing.
func take(home, dir string) snapshot {
	s := make(snapshot)
	root, err := filepath.EvalSymlinks(filepath.Join(home, dir))
	if err != nil {
		return s
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
	return s
}

// diff lists what differs from before to after, a line a path, in order.
func diff(before, after snapshot) []string {
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
