package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Find returns where the claude command is: on PATH, as lookPath searches it,
// else at the first of installPaths that holds a program. run starts the
// claude it finds, and probes claim that one's version, so switchboard finds
// claude this way alone. PATH comes first, as the user's shell has it; the
// install paths stand in for a PATH as short as a LaunchAgent's.
func Find(lookPath func(file string) (string, error), installPaths []string) (string, error) {
	if path, err := lookPath(Command); err == nil {
		return path, nil
	}
	for _, path := range installPaths {
		if isProgram(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("can't find claude: it isn't on PATH, nor at %s", strings.Join(installPaths, ", "))
}

// isProgram reports whether path holds a file that can be run.
func isProgram(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
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
