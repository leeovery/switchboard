// Package shell is switchboard in the user's shell: a file of switchboard's
// own for each shell it knows, which puts its bin directory first on PATH and
// loads its completions; the one line setup adds to the end of the shell's
// startup file, which sources that file; and asking the user's login shell
// which claude it runs. The startup file is written once, and everything
// switchboard does in the shell after that lives in its own file, which setup
// writes and the router brings up to date.
package shell

import (
	"path/filepath"
	"slices"
)

// Shell is a shell switchboard knows, by its program's name.
type Shell string

// The shells switchboard writes a file for, and offers its line to.
const (
	Zsh  Shell = "zsh"
	Bash Shell = "bash"
	Fish Shell = "fish"
)

// Shells are every shell switchboard knows.
var Shells = []Shell{Zsh, Bash, Fish}

// Of returns the shell at path, as $SHELL names it, by its base name, and
// whether it's one switchboard knows.
func Of(path string) (Shell, bool) {
	if path == "" {
		return "", false
	}
	sh := Shell(filepath.Base(path))
	return sh, slices.Contains(Shells, sh)
}

// InitFile returns where sh's file of switchboard's is in dir, switchboard's
// shell directory: init.<shell>.
func InitFile(dir string, sh Shell) string {
	return filepath.Join(dir, "init."+string(sh))
}

// CompletionFile returns where bash's completion script is in dir,
// switchboard's shell directory, beside bash's file, which sources it:
// completion.bash.
func CompletionFile(dir string) string {
	return filepath.Join(dir, "completion.bash")
}
