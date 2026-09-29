package testguard

import (
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// switchboard's directories in a home, and the directory its LaunchAgent goes
// in.
var (
	configDir       = filepath.Join(".config", "switchboard")
	stateDir        = filepath.Join(".local", "state", "switchboard")
	launchAgentsDir = filepath.Join("Library", "LaunchAgents")
)

// watcher watches something of the real system's, as closely as a live
// switchboard running alongside the tests allows, and says how the tests
// changed it, a line each.
type watcher interface {
	changes() []string
}

// watchers are what testguard watches of the real system.
type watchers []watcher

func (ws watchers) changes() []string {
	var lines []string
	for _, w := range ws {
		lines = append(lines, w.changes()...)
	}
	return lines
}

// watchReal notes, before the tests begin, what the real system holds of
// switchboard's: its config and its state, by default in home, and wherever
// SWITCHBOARD_CONFIG, XDG_CONFIG_HOME and XDG_STATE_HOME put them, as getenv
// reads them; and its files among the LaunchAgents in home. Nothing live
// writes the config or a LaunchAgent, so any change to either is a test's. A
// live router writes its state as it runs, its logs and state.json among it,
// so only a state directory appearing is a test's; and the OS sandbox denies
// a test any write there anyway.
func watchReal(home string, getenv func(string) string) watchers {
	var ws watchers
	for _, p := range configPlaces(home, getenv) {
		ws = append(ws, watchContents(p, nil))
	}
	for _, p := range statePlaces(home, getenv) {
		if !exists(p.path) {
			ws = append(ws, absence{p})
		}
	}
	if home != "" {
		ws = append(ws, watchContents(inHome(home, launchAgentsDir), mentionsSwitchboard))
	}
	return ws
}

// configPlaces are where switchboard's config is: by default in home, and
// where SWITCHBOARD_CONFIG and XDG_CONFIG_HOME say. switchboard ignores a
// relative XDG directory, and a relative config file names none testguard can
// know, as it's relative to wherever switchboard runs.
func configPlaces(home string, getenv func(string) string) []place {
	var places []place
	if home != "" {
		places = append(places, inHome(home, configDir))
	}
	if file := getenv("SWITCHBOARD_CONFIG"); filepath.IsAbs(file) {
		places = append(places, named(file))
	}
	if dir := getenv("XDG_CONFIG_HOME"); filepath.IsAbs(dir) {
		places = append(places, named(filepath.Join(dir, "switchboard")))
	}
	return places
}

// stateDirs are where switchboard's state is, as statePlaces finds it, a live
// router's control socket among it.
func stateDirs(home string, getenv func(string) string) []string {
	var dirs []string
	for _, p := range statePlaces(home, getenv) {
		dirs = append(dirs, p.path)
	}
	return dirs
}

// statePlaces are where switchboard's state is: by default in home, and where
// XDG_STATE_HOME says.
func statePlaces(home string, getenv func(string) string) []place {
	var places []place
	if home != "" {
		places = append(places, inHome(home, stateDir))
	}
	if dir := getenv("XDG_STATE_HOME"); filepath.IsAbs(dir) {
		places = append(places, named(filepath.Join(dir, "switchboard")))
	}
	return places
}

// place is somewhere of the real system's: where it is, its symlinks
// resolved as the tests began, so a link changed since changes nothing, and
// how a report names it.
type place struct {
	path  string
	shown string
}

// inHome is the place dir is in home, named from the home, as in
// ~/.config/switchboard.
func inHome(home, dir string) place {
	return place{path: resolve(filepath.Join(home, dir)), shown: "~/" + filepath.ToSlash(dir)}
}

// named is the place at where, named as the environment names it.
func named(where string) place {
	return place{path: resolve(where), shown: filepath.ToSlash(where)}
}

// contents watches what's at a place, and everything under it, that keep
// keeps, or all of it when keep is nil.
type contents struct {
	place
	keep   func(name string) bool
	before snapshot
}

func watchContents(p place, keep func(name string) bool) contents {
	c := contents{place: p, keep: keep}
	c.before = c.take()
	return c
}

func (c contents) changes() []string {
	return diff(c.before, c.take())
}

// take notes what the place holds, following symlinks: a config kept
// elsewhere, such as with dotfiles, and linked in is the real one all the
// same. One that isn't there holds nothing.
func (c contents) take() snapshot {
	s := make(snapshot)
	_ = filepath.WalkDir(c.path, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := os.Stat(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(c.path, p)
		if shown := path.Join(c.shown, filepath.ToSlash(rel)); c.keep == nil || c.keep(shown) {
			s[shown] = entry{kind: info.Mode().Type(), size: info.Size(), modTime: info.ModTime().UnixNano()}
		}
		return nil
	})
	return s
}

// mentionsSwitchboard reports whether a file's name mentions switchboard, in
// any case: only such LaunchAgents are switchboard's, where other programs
// add and change their own as they please.
func mentionsSwitchboard(name string) bool {
	return strings.Contains(strings.ToLower(path.Base(name)), "switchboard")
}

// absence watches a place that wasn't there as the tests began: a state
// directory, which a live router writes in only once it's there, so its
// appearing is a test's.
type absence struct {
	place
}

func (a absence) changes() []string {
	if !exists(a.path) {
		return nil
	}
	return []string{"the real " + a.shown + " appeared"}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// snapshot is what a place holds, by how a report names each path.
type snapshot map[string]entry

// entry is what's at a path: its type, size and modification time.
type entry struct {
	kind    fs.FileMode
	size    int64
	modTime int64
}

// diff lists what differs from before to after, a line a path, in order.
func diff(before, after snapshot) []string {
	paths := make(map[string]bool)
	for p := range before {
		paths[p] = true
	}
	for p := range after {
		paths[p] = true
	}
	var lines []string
	for _, p := range slices.Sorted(maps.Keys(paths)) {
		was, wasThere := before[p]
		is, isThere := after[p]
		switch {
		case !wasThere:
			lines = append(lines, "the real "+p+" was created")
		case !isThere:
			lines = append(lines, "the real "+p+" was removed")
		case was != is:
			lines = append(lines, "the real "+p+" was modified")
		}
	}
	return lines
}
