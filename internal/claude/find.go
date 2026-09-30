package claude

import (
	"cmp"
	"debug/buildinfo"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Find returns where the real claude is: the first program named claude in
// the directories pathList names, as PATH does, else the first of
// installPaths that's a program. Any that is switchboard is passed over:
// switchboard's own executable, as executable gives it, links followed,
// which is its claude link, and any other build of switchboard, as the Go
// build info each holds says. Starting one would start switchboard, which
// would find claude again, and so on forever. run starts the claude Find
// finds, and probes claim that one's version, so switchboard finds claude
// this way alone. PATH comes first, as the user's shell has it; the install
// paths stand in for a PATH as short as a LaunchAgent's.
func Find(pathList string, installPaths []string, executable func() (string, error)) (string, error) {
	return FindAfter("", pathList, installPaths, executable)
}

// FindAfter returns where the real claude is, as Find does, but looking only
// past after among the places Find looks: "" looks at them all. A switchboard
// started again in the place of the claude it started, at after, as by a
// wrapper named claude that runs switchboard, finds the claude past that one,
// and fails when there's none, rather than start it again. When after isn't
// among those places, as when such a wrapper changes PATH, FindAfter looks at
// them all.
func FindAfter(after, pathList string, installPaths []string, executable func() (string, error)) (string, error) {
	self, err := describeSelf(executable)
	if err != nil {
		return "", fmt.Errorf("can't find claude: can't tell it from switchboard's own link: %w", err)
	}
	paths := candidates(pathList, installPaths)
	// passed is what's to blame when there's no claude: the first switchboard
	// passed over, or the claude that led back to switchboard.
	var passed string
	if i := slices.Index(paths, after); i >= 0 {
		paths, passed = paths[i+1:], after+", which leads back to switchboard"
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || !isProgram(info) {
			continue
		}
		switchboard := self.passedOver(path, info)
		if switchboard == "" {
			return path, nil
		}
		passed = cmp.Or(passed, switchboard)
	}
	what := "claude"
	if passed != "" {
		what += " past " + passed
	}
	return "", fmt.Errorf("can't find %s: it isn't on PATH, nor at %s", what, strings.Join(installPaths, ", "))
}

// thisSwitchboard is this switchboard, as Find tells it from claude.
type thisSwitchboard struct {
	// info describes its executable, links followed.
	info os.FileInfo
	// main is the path of the main package it was built from, as its Go build
	// info gives it: "" when that can't be read.
	main string
}

// describeSelf describes the switchboard whose executable executable names.
func describeSelf(executable func() (string, error)) (thisSwitchboard, error) {
	path, err := executable()
	if err != nil {
		return thisSwitchboard{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return thisSwitchboard{}, err
	}
	return thisSwitchboard{info: info, main: mainPackage(path)}, nil
}

// passedOver says what the program at path, which info describes, is when
// it's switchboard, which Find passes over: its own executable, or another
// build of it. It's "" for any other program.
func (s thisSwitchboard) passedOver(path string, info os.FileInfo) string {
	switch {
	case os.SameFile(info, s.info):
		return "switchboard's own link"
	case s.main != "" && mainPackage(path) == s.main:
		return "another switchboard, at " + path
	}
	return ""
}

// mainPackage is the path of the main package the program at path was built
// from, as the Go build info it holds gives it, read without running it: ""
// for a program that isn't Go's, such as a script.
func mainPackage(path string) string {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return ""
	}
	return info.Path
}

// candidates lists where Find looks for claude, in order: in each directory
// pathList names, then at installPaths. Each path is listed once, as when an
// install path's directory is on PATH too, so FindAfter, looking past one,
// never comes to it again.
func candidates(pathList string, installPaths []string) []string {
	var paths []string
	for _, path := range slices.Concat(onPath(pathList), installPaths) {
		if !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	return paths
}

// ThroughSwitchboard reports whether the claude a shell runs from PATH, the
// first program named claude in the directories pathList names, is
// switchboard's own executable, as executable gives it, links followed: its
// claude link, which every claude started from PATH goes through. With no
// claude on PATH, none does. It fails when executable does, as there's no
// telling then.
func ThroughSwitchboard(pathList string, executable func() (string, error)) (bool, error) {
	self, err := describeSelf(executable)
	if err != nil {
		return false, err
	}
	for _, path := range onPath(pathList) {
		if info, err := os.Stat(path); err == nil && isProgram(info) {
			return os.SameFile(info, self.info), nil
		}
	}
	return false, nil
}

// onPath lists where claude would be in each directory pathList names, in
// order.
func onPath(pathList string) []string {
	var paths []string
	for _, dir := range filepath.SplitList(pathList) {
		// A relative directory, "" among them, means wherever switchboard
		// happens to be started: exec.LookPath refuses a program found there
		// too.
		if filepath.IsAbs(dir) {
			paths = append(paths, filepath.Join(dir, Command))
		}
	}
	return paths
}

// isProgram reports whether info describes a file that can be run.
func isProgram(info os.FileInfo) bool {
	return info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// InstallPaths lists where Claude Code's installers put the CLI, for the home
// directory given.
func InstallPaths(home string) []string {
	paths := []string{
		filepath.Join(home, ".local", "bin", "claude"),
		"/opt/homebrew/bin/claude",
		"/usr/local/bin/claude",
		filepath.Join(home, ".claude", "local", "claude"),
	}
	// Without a home directory, the home-relative paths would resolve against
	// the working directory.
	return slices.DeleteFunc(paths, func(path string) bool { return !filepath.IsAbs(path) })
}
