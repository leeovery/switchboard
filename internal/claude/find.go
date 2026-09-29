package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Find returns where the real claude is: the first program named claude in
// the directories pathList names, as PATH does, else the first of
// installPaths that's a program. Any that is switchboard's own executable,
// as executable gives it, links followed, is passed over: it's switchboard's
// claude link, and starting it would start switchboard, which would find it
// again, and so on forever. run starts the claude Find finds, and probes
// claim that one's version, so switchboard finds claude this way alone. PATH
// comes first, as the user's shell has it; the install paths stand in for a
// PATH as short as a LaunchAgent's.
func Find(pathList string, installPaths []string, executable func() (string, error)) (string, error) {
	self, err := statExecutable(executable)
	if err != nil {
		return "", fmt.Errorf("can't find claude: can't tell it from switchboard's own link: %w", err)
	}
	what := "claude"
	for _, path := range slices.Concat(onPath(pathList), installPaths) {
		info, err := os.Stat(path)
		if err != nil || !isProgram(info) {
			continue
		}
		if !os.SameFile(info, self) {
			return path, nil
		}
		what = "claude past switchboard's own link"
	}
	return "", fmt.Errorf("can't find %s: it isn't on PATH, nor at %s", what, strings.Join(installPaths, ", "))
}

// ThroughSwitchboard reports whether the claude a shell runs from PATH, the
// first program named claude in the directories pathList names, is
// switchboard's own executable, as executable gives it, links followed: its
// claude link, which every claude started from PATH goes through. With no
// claude on PATH, none does. It fails when executable does, as there's no
// telling then.
func ThroughSwitchboard(pathList string, executable func() (string, error)) (bool, error) {
	self, err := statExecutable(executable)
	if err != nil {
		return false, err
	}
	for _, path := range onPath(pathList) {
		if info, err := os.Stat(path); err == nil && isProgram(info) {
			return os.SameFile(info, self), nil
		}
	}
	return false, nil
}

// statExecutable describes the file executable names, links followed.
func statExecutable(executable func() (string, error)) (os.FileInfo, error) {
	path, err := executable()
	if err != nil {
		return nil, err
	}
	return os.Stat(path)
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
