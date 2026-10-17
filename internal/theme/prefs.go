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
// the user never needs to write: the themes it's drawn in, the tab it
// shows, the Overview's sections shown, how its windows are grouped and the
// chart GRAPHS draws, whether the live views show each model's own window,
// what Accounts, History's views, Sessions and Runway show, and the Log's
// columns.
type Prefs struct {
	Choice
	// Tab names the tab shown, one of overview, accounts, history, sessions,
	// runway and log, for a watch to open on: "" for the default, the
	// Overview, as one it doesn't name reads. Milestone 5 kept its card view
	// under "view", as "accounts", which isn't today's Accounts: a file of
	// then is read as having no tab, its "view" kept as a key these don't
	// know.
	Tab string `json:"tab,omitempty"`
	// Sections name the Overview's sections shown, of usage, graphs,
	// routing, sessions, coming and lately: nil, as a file without them
	// reads, for those shown at first, and none, kept as [], for none.
	Sections []string `json:"sections,omitzero"`
	// By names how the Overview's windows are grouped, window or account, as
	// b last left it: "" for the config's [usage] by.
	By string `json:"by,omitempty"`
	// Graphs names the chart GRAPHS draws, as v last left it: burn-down, or
	// "" for the burn-up, as a name the dashboard doesn't know reads.
	// Milestone 5 kept its cards' chart style under "chart", which isn't
	// this: a file of then keeps it as a key these don't know.
	Graphs string `json:"graphs,omitempty"`
	// Models is whether the live views show each model's own window, as
	// Fable's week, as w last left it: unset, as a file without it reads,
	// hides them.
	Models bool `json:"models,omitempty"`
	// Accounts is what Accounts shows, as its keys last left it.
	Accounts AccountsPrefs `json:"accounts,omitzero"`
	// History is what History's views show, as their keys last left them.
	History HistoryPrefs `json:"history,omitzero"`
	// Sessions is what Sessions shows, as its keys last left it.
	Sessions SessionsPrefs `json:"sessions,omitzero"`
	// Runway is what Runway shows, as its keys last left it.
	Runway RunwayPrefs `json:"runway,omitzero"`
	// Log is what the Log shows, as its f panel last left it.
	Log LogPrefs `json:"log,omitzero"`
}

// LogPrefs are what the Log shows: Columns, the names of the columns its
// Requests show, at aside, which always shows: nil, as a file without them
// reads, for those shown at first, and none, kept as [], for at alone. A
// name the dashboard doesn't know, as a later build's column, shows nothing.
type LogPrefs struct {
	Columns []string `json:"columns,omitzero"`
}

// AccountsPrefs are what Accounts shows, each by its name: the view shown,
// compare or detail; and the period both count over, today, "" for this
// week, 30-days or 12-weeks. A name the dashboard doesn't know reads as the
// default. What's chosen in a view is the run's alone.
type AccountsPrefs struct {
	View   string `json:"view,omitempty"`
	Period string `json:"period,omitempty"`
}

// SessionsPrefs are what Sessions shows, by its name: the view shown,
// switchboard or list. A name the dashboard doesn't know reads as the
// default, the Switchboard. What's chosen in a view is the run's alone.
type SessionsPrefs struct {
	View string `json:"view,omitempty"`
}

// RunwayPrefs are what Runway shows, by its name: the window its view is
// named for, 5-hour, or "" for the week. A name the dashboard doesn't know,
// as "week", which earlier builds kept the week by, reads as the week. A day
// or week stepped back to is the run's alone.
type RunwayPrefs struct {
	Window string `json:"window,omitempty"`
}

// HistoryPrefs are what History's views show, each by its name: the view
// shown, of year, weeks, days and tokens; what the Year shows, all for all
// use, a model version by its name, as Opus 5.5, or an account by its id;
// Weeks' shape, table or columns; and Tokens' period and split. A name the
// dashboard doesn't know reads as the default, as does a version or an
// account no longer there to show.
type HistoryPrefs struct {
	View   string      `json:"view,omitempty"`
	Year   string      `json:"year,omitempty"`
	Weeks  string      `json:"weeks,omitempty"`
	Tokens TokensPrefs `json:"tokens,omitzero"`
}

// TokensPrefs are how History's Tokens shows its periods: Period, day, week
// or month; and Split, account, model, account+model or none.
type TokensPrefs struct {
	Period string `json:"period,omitempty"`
	Split  string `json:"split,omitempty"`
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
