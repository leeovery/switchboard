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

// switchboard's directories in a home, the directory its LaunchAgent goes in,
// and Claude Code's config directory; the directory its themes go in, in its
// config directory; the directory its token files go in, and its
// preferences file, in its state directory; and the directory its skill goes
// in, in Claude Code's config directory.
var (
	configDir       = filepath.Join(".config", "switchboard")
	stateDir        = filepath.Join(".local", "state", "switchboard")
	binDir          = filepath.Join(".local", "share", "switchboard", "bin")
	launchAgentsDir = filepath.Join("Library", "LaunchAgents")
	claudeDir       = ".claude"
	themesDir       = "themes"
	tokensDir       = "tokens"
	prefsFile       = "prefs.json"
	skillDir        = filepath.Join("skills", "switchboard")
)

// claudeLink is the name of switchboard's claude link, which a directory on
// PATH holds ahead of the real claude.
const claudeLink = "claude"

// watcher watches something of the real system's, as closely as a live
// switchboard running alongside the tests allows, and says how the tests
// changed it, a line each.
type watcher interface {
	changes() []string
}

// watchers are what testguard watches of the real system.
type watchers []watcher

// changes are how the tests changed what's watched, a line each, and once:
// the themes directory is the config directory's too, unless it's a link.
func (ws watchers) changes() []string {
	var lines []string
	for _, w := range ws {
		for _, line := range w.changes() {
			if !slices.Contains(lines, line) {
				lines = append(lines, line)
			}
		}
	}
	return lines
}

// watchReal notes, before the tests begin, what the real system holds of
// switchboard's: its config, its themes and its state, by default in home,
// and wherever SWITCHBOARD_CONFIG, SWITCHBOARD_THEMES_DIR, XDG_CONFIG_HOME
// and XDG_STATE_HOME put them, as getenv reads them; its files among the
// LaunchAgents in home; its skill in Claude Code's config directory, by
// default in home, and wherever CLAUDE_CONFIG_DIR puts it; its bin
// directory, which holds its claude link, by default in home, and wherever
// XDG_DATA_HOME puts it; and what's named claude in each directory on PATH.
// Nothing live writes the config, a theme, a LaunchAgent, the skill, a token
// file or a claude link as it runs, nor the preferences file, but as a
// dashboard's user picks a theme, so any change to one is a test's. A live
// router writes the rest of its state as it runs, its logs, state.json and
// readings history among it, so of that, only a state directory appearing is
// a test's; and the OS sandbox denies a test any write there anyway.
func watchReal(home string, getenv func(string) string) watchers {
	var ws watchers
	for _, p := range slices.Concat(configPlaces(home, getenv), themesPlaces(home, getenv)) {
		ws = append(ws, watchContents(p, nil, followed))
	}
	for _, p := range statePlaces(home, getenv) {
		if !exists(p.path) {
			ws = append(ws, absence{p})
			continue
		}
		ws = append(ws, watchContents(p.in(tokensDir), nil, followed), watchContents(p.in(prefsFile), nil, followed))
	}
	if home != "" {
		ws = append(ws, watchContents(inHome(home, launchAgentsDir), mentionsSwitchboard, followed))
	}
	for _, p := range skillPlaces(home, getenv) {
		ws = append(ws, watchContents(p, nil, followed))
	}
	for _, p := range slices.Concat(binPlaces(home, getenv), claudeLinkPlaces(getenv)) {
		ws = append(ws, watchContents(p, nil, asItIs))
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

// themesPlaces are where the dashboard's themes are: by default in home's
// config directory, and where SWITCHBOARD_THEMES_DIR and XDG_CONFIG_HOME say.
// A relative SWITCHBOARD_THEMES_DIR names none testguard can know, as it's
// relative to wherever the dashboard runs.
func themesPlaces(home string, getenv func(string) string) []place {
	var places []place
	if home != "" {
		places = append(places, inHome(home, filepath.Join(configDir, themesDir)))
	}
	if dir := getenv("SWITCHBOARD_THEMES_DIR"); filepath.IsAbs(dir) {
		places = append(places, named(dir))
	}
	if dir := getenv("XDG_CONFIG_HOME"); filepath.IsAbs(dir) {
		places = append(places, named(filepath.Join(dir, "switchboard", themesDir)))
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

// skillPlaces are where switchboard's skill is, in Claude Code's config
// directory: by default in home, and where CLAUDE_CONFIG_DIR says. A relative
// one names none testguard can know, as it's relative to wherever Claude Code
// runs.
func skillPlaces(home string, getenv func(string) string) []place {
	var places []place
	if home != "" {
		places = append(places, inHome(home, filepath.Join(claudeDir, skillDir)))
	}
	if dir := getenv("CLAUDE_CONFIG_DIR"); filepath.IsAbs(dir) {
		places = append(places, named(filepath.Join(dir, skillDir)))
	}
	return places
}

// binPlaces are where switchboard's bin directory is, which holds its claude
// link: by default in home, and where XDG_DATA_HOME says.
func binPlaces(home string, getenv func(string) string) []place {
	var places []place
	if home != "" {
		places = append(places, inHome(home, binDir))
	}
	if dir := getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		places = append(places, named(filepath.Join(dir, "switchboard", "bin")))
	}
	return places
}

// claudeLinkPlaces are where a claude on PATH would be found, as getenv reads
// PATH: in each of its directories, once, however many times PATH names it. A
// relative directory names none testguard can know, as it's relative to
// wherever the program looking in it runs.
func claudeLinkPlaces(getenv func(string) string) []place {
	var places []place
	for _, dir := range filepath.SplitList(getenv("PATH")) {
		if !filepath.IsAbs(dir) {
			continue
		}
		p := inDir(dir, claudeLink)
		if !slices.ContainsFunc(places, func(q place) bool { return q.path == p.path }) {
			places = append(places, p)
		}
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

// inDir is the place named name in dir, named as the environment names dir:
// dir resolved, but not name, which may itself be a link.
func inDir(dir, name string) place {
	return place{path: filepath.Join(resolve(dir), name), shown: filepath.ToSlash(filepath.Join(dir, name))}
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

// in is the place named name in p, named from p.
func (p place) in(name string) place {
	return place{path: resolve(filepath.Join(p.path, name)), shown: path.Join(p.shown, filepath.ToSlash(name))}
}

// contents watches what's at a place, and everything under it, that keep
// keeps, or all of it when keep is nil, each noted as note notes it.
type contents struct {
	place
	keep   func(name string) bool
	note   func(path string) (entry, bool)
	before snapshot
}

func watchContents(p place, keep func(name string) bool, note func(path string) (entry, bool)) contents {
	c := contents{place: p, keep: keep, note: note}
	c.before = c.take()
	return c
}

func (c contents) changes() []string {
	return diff(c.before, c.take())
}

// take notes what the place holds. One that isn't there holds nothing.
func (c contents) take() snapshot {
	s := make(snapshot)
	_ = filepath.WalkDir(c.path, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		e, ok := c.note(p)
		if !ok {
			return nil
		}
		rel, _ := filepath.Rel(c.path, p)
		if shown := path.Join(c.shown, filepath.ToSlash(rel)); c.keep == nil || c.keep(shown) {
			s[shown] = e
		}
		return nil
	})
	return s
}

// followed notes what's at path, links followed: a config kept elsewhere,
// such as with dotfiles, and linked in is the real one all the same.
func followed(path string) (entry, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return entry{}, false
	}
	return entry{kind: info.Mode().Type(), size: info.Size(), modTime: info.ModTime().UnixNano()}, true
}

// asItIs notes what's at path itself, a link as a link, with where it leads,
// so one made, or made to lead elsewhere, even nowhere, is seen.
func asItIs(path string) (entry, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return entry{}, false
	}
	target, _ := os.Readlink(path)
	return entry{kind: info.Mode().Type(), size: info.Size(), modTime: info.ModTime().UnixNano(), target: target}, true
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

// entry is what's at a path: its type, size and modification time, and where
// it leads, when it's a link taken as one.
type entry struct {
	kind    fs.FileMode
	size    int64
	modTime int64
	target  string
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
