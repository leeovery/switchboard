// Package childenv is the environment switchboard starts the programs it
// runs for itself in, claude --version, osascript and launchctl: only what
// they need to run, so none of the tokens in switchboard's own reaches them.
package childenv

import (
	"path/filepath"
	"slices"
	"strings"
)

// kept are the variables such a program is given: where to find programs,
// the home directory, the user's own temporary directory rather than the
// /tmp everyone shares, and the language to speak.
var kept = []string{"PATH", "HOME", "TMPDIR", "LANG"}

// Minimal returns those of kept that getenv finds set, as exec.Cmd's Env
// takes them. It's never nil: a nil Env hands on the whole environment.
func Minimal(getenv func(string) string) []string {
	env := make([]string, 0, len(kept))
	for _, name := range kept {
		if value := getenv(name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	return env
}

// Beside returns env, an environment as Minimal gives it, with the
// directories of the program at path first on its PATH: the one it's named
// in, then the one its links lead to, when that's another. A script there,
// such as npm's claude, run by env node, finds the interpreter installed
// beside it, as it does on the user's own PATH, where a LaunchAgent's holds
// the system's directories alone.
func Beside(env []string, path string) []string {
	dirs := []string{filepath.Dir(path)}
	if resolved, err := filepath.EvalSymlinks(path); err == nil && filepath.Dir(resolved) != dirs[0] {
		dirs = append(dirs, filepath.Dir(resolved))
	}
	env = slices.Clone(env)
	for i, variable := range env {
		if value, ok := strings.CutPrefix(variable, "PATH="); ok {
			env[i] = pathOf(append(dirs, value))
			return env
		}
	}
	return append(env, pathOf(dirs))
}

// pathOf is PATH, set to the directories dirs lists, in order.
func pathOf(dirs []string) string {
	return "PATH=" + strings.Join(dirs, string(filepath.ListSeparator))
}
