package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// stateFileName is the state file's name in the state directory.
	stateFileName = "state.json"
	// stateVersion is the version of the state file's format. It changes only
	// when a router couldn't read what another wrote: a field added, which a
	// file from before lacks and an older router ignores, leaves it as it is.
	stateVersion = 1
	// saveAfter is how soon after a change the state file is written, so a
	// burst of changes makes one write.
	saveAfter = time.Second
	// saveRoutineEvery is how often the state file is written when its only
	// changes are routine, as every request makes: its session's assignment
	// used again, and the reading off its answer. Saved as they come, they
	// would have the file written every second or so.
	saveRoutineEvery = time.Minute
	// pruneEvery is how often what has gone unused for long enough is
	// forgotten.
	pruneEvery = time.Hour
)

// stateFile is where the router keeps what should outlast it: the sessions'
// assignments, the pins they were given while they ran and the global pin, so
// a restart doesn't scatter sessions and cost their caches; what it knows of
// the accounts' tokens, so a token replaced before a restart still counts as
// its account's after; and each account's last readings, so a restart needs
// no probe, which, being a request, could start a window.
type stateFile struct {
	// path is where the file is: "" until it's loaded, and nothing is kept.
	path string
	// write writes data to a file at path whole, or not at all.
	write   func(path string, data []byte) error
	now     func() time.Time
	changes *changes
	// sessions, accounts and state are what the file keeps what's known of.
	sessions *sessions
	accounts accounts
	state    *state
}

func newStateFile(now func() time.Time, changes *changes, sessions *sessions, accounts accounts, state *state) *stateFile {
	return &stateFile{write: writeState, now: now, changes: changes, sessions: sessions, accounts: accounts, state: state}
}

// changes are the changes the state file lacks. Noting one never blocks, and
// it's safe for concurrent use.
type changes struct {
	// unsaved is set while the file lacks a change.
	unsaved atomic.Bool
	// noted signals a change noted since keep last took one.
	noted chan struct{}
}

func newChanges() *changes {
	return &changes{noted: make(chan struct{}, 1)}
}

// note notes a change for the state file to keep.
func (c *changes) note() {
	c.unsaved.Store(true)
	select {
	case c.noted <- struct{}{}:
	default:
	}
}

// routine notes a routine change for the state file to keep, as every request
// makes, which waits for the next write.
func (c *changes) routine() {
	c.unsaved.Store(true)
}

// savedState is what the state file holds.
type savedState struct {
	Version int `json:"version"`
	savedSessions
	// Tokens are what's kept of each account's tokens, by the account's id.
	Tokens map[string]savedTokens `json:"tokens,omitempty"`
	savedUsage
}

// savedSessions is what the state file holds of the sessions.
type savedSessions struct {
	Pin      status.Pin        `json:"pin,omitzero"`
	Sessions []savedAssignment `json:"sessions"`
	// SessionPins are the pins sessions were given while they ran, by the
	// session's id.
	SessionPins map[string]ownPin `json:"session_pins,omitempty"`
}

// savedAssignment is an assignment as the state file holds it, with the
// session and model it's for.
type savedAssignment struct {
	Session string `json:"session"`
	Model   string `json:"model"`
	assignment
}

// savedUsage is what the state file holds of the accounts' usage.
type savedUsage struct {
	// Readings are each account's windows and extra usage as last read, by
	// the account's id.
	Readings map[string]savedReading `json:"readings,omitempty"`
	// WindowFamilies are the model families whose requests each window has
	// been reported on, by the window's key, which says which requests it
	// counts.
	WindowFamilies map[string][]string `json:"window_families,omitempty"`
}

// savedReading is an account's windows as last read, and when, and its extra
// usage.
type savedReading struct {
	ReadAt  time.Time        `json:"read_at"`
	Windows []quota.Window   `json:"windows"`
	Extra   quota.ExtraUsage `json:"extra_usage,omitzero"`
}

// load takes in the state file at path, as the router starts, and keeps in it
// from then on what the router knows that should outlast it. What it held
// that can no longer be used is left out, and the file is to be written again
// without it.
func (f *stateFile) load(path string) {
	f.path = path
	now := f.now()
	saved := f.read(now)
	changed := f.accounts.recall(saved.Tokens, now)
	changed = f.sessions.recall(saved.savedSessions, f.accounts, now) || changed
	changed = f.state.recall(saved.savedUsage) || changed
	if changed {
		f.changes.note()
	}
	held := f.snapshot()
	logger.Info("loaded state", "path", path, "assignments", len(held.Sessions), "pin", strings.Join(held.Pin.Accounts, ","), "readings", len(held.Readings))
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

// keep writes the state file saveAfter after a change, so a burst of changes
// makes one write, and every saveRoutineEvery while its only changes are
// routine; forgets, every pruneEvery, what has gone unused for long enough;
// and writes the file once more as ctx ends.
func (f *stateFile) keep(ctx context.Context) {
	prune := time.NewTicker(pruneEvery)
	defer prune.Stop()
	routine := time.NewTicker(saveRoutineEvery)
	defer routine.Stop()
	for {
		select {
		case <-f.changes.noted:
			select {
			case <-time.After(saveAfter):
			case <-ctx.Done():
			}
			f.save()
		case <-routine.C:
			f.save()
		case <-prune.C:
			f.prune(f.now())
		case <-ctx.Done():
			f.save()
			return
		}
	}
}

// prune forgets, at now, the assignments gone unused for forgetAfter, with the
// pins of the sessions it forgets, and the accounts' former tokens that no
// longer count.
func (f *stateFile) prune(now time.Time) {
	f.sessions.prune(now)
	if f.accounts.forget(now) {
		logger.Debug("forgot tokens replaced a week ago")
		f.changes.note()
	}
}

// save writes the state file, once it's loaded, when it lacks a change. A
// write that fails leaves the change for the next.
func (f *stateFile) save() {
	if f.path == "" || !f.changes.unsaved.Swap(false) {
		return
	}
	if err := f.store(f.snapshot()); err != nil {
		logger.Warn("can't save the state file", "path", f.path, "error", err)
		f.changes.unsaved.Store(true)
	}
}

// snapshot is what the state file is to hold, in a steady order.
func (f *stateFile) snapshot() savedState {
	return savedState{
		Version:       stateVersion,
		savedSessions: f.sessions.saved(),
		Tokens:        f.accounts.kept(),
		savedUsage:    f.state.saved(),
	}
}

// store writes h to the state file.
func (f *stateFile) store(h savedState) error {
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return f.write(f.path, append(data, '\n'))
}

// writeState writes data to a state file at path that only its owner can
// read or write, whole or not at all.
func writeState(path string, data []byte) error {
	return atomicfile.Write(path, data, 0o600)
}
