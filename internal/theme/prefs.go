package theme

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

// prefsName is the preferences file's name in the state directory.
const prefsName = "prefs.json"

// Prefs are the dashboard's preferences, which it keeps as they change, and
// the user never needs to write: the themes it's drawn in, the view it
// shows, the window its cards feature, and the style they draw it in.
type Prefs struct {
	Choice
	// View names the view shown, such as "accounts", for a watch to open on:
	// "" for the default.
	View string `json:"view,omitempty"`
	// Featured is the key of the window every card features, such as "7d":
	// "" for each card's own, as auto chooses it.
	Featured string `json:"featured,omitempty"`
	// Chart names the style every card draws its chart in, such as
	// "hourglass": "" for the default, a burn-down.
	Chart string `json:"chart,omitempty"`
}

// PrefsFile is the preferences file, prefs.json in the state directory,
// which the dashboard writes whole as it's used: never the config file,
// which is the user's, nor in the config directory, often a link into the
// user's dotfiles.
type PrefsFile struct {
	path string
	// now is when a corrupt file is set aside.
	now func() time.Time
}

// NewPrefsFile is the preferences file in the state directory given, a
// corrupt one set aside at the time now gives.
func NewPrefsFile(stateDir string, now func() time.Time) PrefsFile {
	return PrefsFile{path: filepath.Join(stateDir, prefsName), now: now}
}

// Read reads the preferences. Without the file, they're the defaults, and
// so, with a warning, are those of a file that can't be read, or is corrupt:
// that one is set aside, as prefs.json.corrupt-<unix time>, for someone to
// look at.
func (f PrefsFile) Read() Prefs {
	data, err := os.ReadFile(f.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Prefs{}
	case err != nil:
		logger.Warn("can't read the preferences file; the defaults stand", "path", f.path, "error", err)
		return Prefs{}
	}
	var p Prefs
	if err := json.Unmarshal(data, &p); err != nil {
		f.setAside(err)
		return Prefs{}
	}
	return p
}

// setAside renames the corrupt preferences file to
// <path>.corrupt-<now in Unix seconds>.
func (f PrefsFile) setAside(corruption error) {
	aside := fmt.Sprintf("%s.corrupt-%d", f.path, f.now().Unix())
	if err := os.Rename(f.path, aside); err != nil {
		logger.Warn("preferences file corrupt, and can't be set aside; the defaults stand", "path", f.path, "error", corruption, "rename_error", err)
		return
	}
	logger.Warn("preferences file corrupt; set aside, the defaults stand", "path", f.path, "aside", aside, "error", corruption)
}

// Update changes the preferences as change says, and writes them whole: read
// afresh, so what another dashboard kept since stands, then written beside
// the file, and renamed over it, or over where it leads, should it be a link.
func (f PrefsFile) Update(change func(*Prefs)) error {
	p := f.Read()
	change(&p)
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the preferences: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return fmt.Errorf("keep the preferences: %w", err)
	}
	target, err := atomicfile.Target(f.path)
	if err != nil {
		return fmt.Errorf("keep the preferences: %w", err)
	}
	if err := atomicfile.Write(target, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("keep the preferences: %w", err)
	}
	return nil
}
