package router

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

const (
	// stateFileName is the state file's name in the state directory.
	stateFileName = "state.json"
	// stateVersion is the version of the state file's format.
	stateVersion = 1
)

// stateFile is where the router keeps the sessions' assignments, the pins
// they were given while they ran and the global pin, so a restart doesn't
// scatter sessions and cost their caches, and what it knows of the accounts'
// tokens, so a token replaced before a restart still counts as its account's
// after.
type stateFile struct {
	path string
	// write writes data to a file at path whole, or not at all.
	write func(path string, data []byte) error
}

// savedState is what the state file holds.
type savedState struct {
	Version  int               `json:"version"`
	Pin      status.Pin        `json:"pin,omitzero"`
	Sessions []savedAssignment `json:"sessions"`
	// SessionPins are the pins sessions were given while they ran, by the
	// session's id.
	SessionPins map[string]ownPin `json:"session_pins,omitempty"`
	// Tokens are what's kept of each account's tokens, by the account's id.
	Tokens map[string]savedTokens `json:"tokens,omitempty"`
}

// savedAssignment is an assignment as the state file holds it, with the
// session and model it's for.
type savedAssignment struct {
	Session string `json:"session"`
	Model   string `json:"model"`
	assignment
}

// read returns what the state file holds. A file that isn't there holds
// nothing, and so, with a warning, does one that can't be read, or is
// corrupt: that one is set aside, at now, for someone to look at.
func (f *stateFile) read(now time.Time) savedState {
	data, err := os.ReadFile(f.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return savedState{}
	case err != nil:
		logger.Warn("can't read the state file; starting empty", "path", f.path, "error", err)
		return savedState{}
	}
	h, err := parseState(data)
	if err != nil {
		f.setAside(now, err)
		return savedState{}
	}
	return h
}

func parseState(data []byte) (savedState, error) {
	var h savedState
	if err := json.Unmarshal(data, &h); err != nil {
		return savedState{}, err
	}
	if h.Version != stateVersion {
		return savedState{}, fmt.Errorf("version %d, where %d is known", h.Version, stateVersion)
	}
	return h, nil
}

// setAside renames the corrupt state file to <path>.corrupt-<now in Unix
// seconds>.
func (f *stateFile) setAside(now time.Time, corruption error) {
	aside := fmt.Sprintf("%s.corrupt-%d", f.path, now.Unix())
	if err := os.Rename(f.path, aside); err != nil {
		logger.Warn("state file corrupt, and can't be set aside; starting empty", "path", f.path, "error", corruption, "rename_error", err)
		return
	}
	logger.Warn("state file corrupt; set aside, starting empty", "path", f.path, "aside", aside, "error", corruption)
}

// save writes h to the state file.
func (f *stateFile) save(h savedState) error {
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return f.write(f.path, append(data, '\n'))
}

// writeAtomic writes data to a file at path that only its owner can read or
// write, whole or not at all: it's written beside it, then renamed over it, so
// no reader finds half a file. It isn't synced: the file is written as often
// as every second, and an operating system's crash that cost it would cost no
// more than a corrupt file, which is set aside.
func writeAtomic(path string, data []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
