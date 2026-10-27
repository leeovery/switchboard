package setuptoken

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The files of the shim's directory: the script, and the address it hands
// back.
const (
	shimScript  = "browser"
	shimAddress = "address"
)

// shim is the browser setup-token is given: a script, in a private directory
// of its own, that hands back the sign-in address it's run with, as its one
// argument, by writing it to a file there, which Make looks for. It writes it
// whole, under another name, then renames it, so Make never reads it half
// written, and it never waits on Make, as it would writing to a pipe.
type shim struct {
	dir string
}

// newShim makes the shim, in a directory of its own in the temporary
// directory, that only this user can read.
func newShim() (shim, error) {
	dir, err := os.MkdirTemp("", "switchboard-setup-token-")
	if err != nil {
		return shim{}, err
	}
	s := shim{dir: dir}
	script := fmt.Sprintf("#!/bin/sh\ncd %s || exit 1\nprintf '%%s\\n' \"$1\" > %s.$$ && /bin/mv -f %s.$$ %s\n",
		shellQuote(dir), shimAddress, shimAddress, shimAddress)
	if err := os.WriteFile(s.browser(), []byte(script), 0o700); err != nil {
		s.remove()
		return shim{}, err
	}
	return s, nil
}

// browser is the path of the shim's script, which setup-token runs as its
// browser.
func (s shim) browser() string {
	return filepath.Join(s.dir, shimScript)
}

// address returns the sign-in address the shim was handed, false while it
// hasn't been.
func (s shim) address() (string, bool) {
	data, err := os.ReadFile(filepath.Join(s.dir, shimAddress))
	address := strings.TrimSpace(string(data))
	return address, err == nil && address != ""
}

// remove removes the shim's directory, and all in it.
func (s shim) remove() {
	if err := os.RemoveAll(s.dir); err != nil {
		logger.Warn("couldn't remove setup-token's browser", "dir", s.dir, "error", err)
	}
}

// shellQuote quotes s for sh, whatever it holds.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
