package theme

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// prefsKeys are the keys the file keeps Prefs under, as their fields' tags
// name them: any other it holds, as a newer build's preferences, is kept as
// it is.
var prefsKeys = func() []string {
	var keys []string
	for _, field := range reflect.VisibleFields(reflect.TypeFor[Prefs]()) {
		if key, _, _ := strings.Cut(field.Tag.Get("json"), ","); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}()

// Read reads the preferences. Without the file, they're the defaults, and
// so, with a warning, are those of a file that can't be read, or is corrupt:
// that one is set aside, as prefs.json.corrupt-<unix time>, for someone to
// look at.
func (f PrefsFile) Read() Prefs {
	p, _ := f.read()
	return p
}

// read reads the preferences as Read does, and the keys of the file they
// don't know, as a newer build's preferences, as they're written there.
func (f PrefsFile) read() (Prefs, map[string]json.RawMessage) {
	data, err := os.ReadFile(f.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Prefs{}, nil
	case err != nil:
		logger.Warn("can't read the preferences file; the defaults stand", "path", f.path, "error", err)
		return Prefs{}, nil
	}
	var p Prefs
	var others map[string]json.RawMessage
	err = json.Unmarshal(data, &p)
	if err == nil {
		err = json.Unmarshal(data, &others)
	}
	if err != nil {
		f.setAside(err)
		return Prefs{}, nil
	}
	for _, key := range prefsKeys {
		delete(others, key)
	}
	return p, others
}

// setAside sets the corrupt preferences file aside, as moveAside does.
func (f PrefsFile) setAside(corruption error) {
	aside, err := f.moveAside()
	if err != nil {
		logger.Warn("preferences file corrupt, and can't be set aside; the defaults stand", "path", f.path, "error", corruption, "rename_error", err)
		return
	}
	logger.Warn("preferences file corrupt; set aside, the defaults stand", "path", f.path, "aside", aside, "error", corruption)
}

// moveAside renames the preferences file, or where it leads, should it be a
// link, which is kept, to <file>.corrupt-<now in Unix seconds>, and returns
// where it went.
func (f PrefsFile) moveAside() (string, error) {
	file, err := atomicfile.Target(f.path)
	if err != nil {
		return "", err
	}
	aside := fmt.Sprintf("%s.corrupt-%d", file, f.now().Unix())
	return aside, os.Rename(file, aside)
}

// Update changes the preferences as change says, and writes them whole: read
// afresh, so what another dashboard kept since stands, as do the keys a newer
// build keeps that this one doesn't know, then written beside the file, and
// renamed over it, or over where it leads, should it be a link.
func (f PrefsFile) Update(change func(*Prefs)) error {
	p, others := f.read()
	change(&p)
	data, err := encode(p, others)
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

// encode is the preferences as the file keeps them, beside others, the keys
// of the file they don't know, as they were.
func encode(p Prefs, others map[string]json.RawMessage) ([]byte, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &others); err != nil {
		return nil, err
	}
	return json.MarshalIndent(others, "", "  ")
}
