package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/forecast"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
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
// assignments, the pins they were given while they ran and each pool's pin, so
// a restart doesn't scatter sessions and cost their caches; what it knows of
// the accounts' tokens, so a token replaced before a restart still counts as
// its account's after; each account's last readings, so a restart needs no
// probe, which, being a request, could start a window, and how its probes
// went, so a restart neither probes it sooner than they allow nor forgets why
// it can't be read; the hold curve, so a restart doesn't take it back to the
// default; the last few pools each account joined, so a restart takes up each
// reading of the history in the pool its account was in then; and what's been
// told of each account's token's year, so a restart tells nothing twice.
type stateFile struct {
	// path is where the file is: "" until it's loaded, and nothing is kept.
	path string
	// write writes data to a file at path whole, or not at all.
	write   func(path string, data []byte) error
	now     func() time.Time
	changes *changes
	// sessions, accounts, state and told are what the file keeps what's
	// known of.
	sessions *sessions
	accounts accounts
	state    *state
	told     *yearsTold
	// heirs say which account the tokens of each account the router was
	// started without count as, as the config it was started from makes them.
	heirs heirs
	// emit tells the events loading the file calls for: each pool's pin it
	// drops whole, as Unpinned, and each hold never told of, as ReadHeld.
	emit func(Event)
	// refusalIn reads why the API refused a read off the text of its
	// failure, as a file from before kept it alone, as the provider's
	// RefusedReadIn does.
	refusalIn func(text string) quota.ReadRefusal
}

func newStateFile(now func() time.Time, changes *changes, sessions *sessions, accounts accounts, state *state, told *yearsTold, heirs heirs, refusalIn func(string) quota.ReadRefusal) *stateFile {
	return &stateFile{
		write: writeState, now: now, changes: changes, sessions: sessions, accounts: accounts, state: state, told: told, heirs: heirs,
		emit: func(Event) {}, refusalIn: refusalIn,
	}
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
	// TokenYears are what's been told of each account's token's year, by the
	// account's id.
	TokenYears map[string]toldYear `json:"token_years,omitempty"`
}

// savedSessions is what the state file holds of the sessions.
type savedSessions struct {
	// Pin is the one pin of a config from before pools, which a state file
	// from before pools kept whatever the config, and Pins each pool's pin,
	// by the pool's id.
	Pin      status.Pin        `json:"pin,omitzero"`
	Pins     pins              `json:"pins,omitempty"`
	Sessions []savedAssignment `json:"sessions"`
	// SessionPins are the pins sessions were given while they ran, by the
	// session's id, each in the pool its session was routed in last of those
	// it has one in, the one pin a router that kept a session's assignments
	// whatever its pool reads; and SessionPinsApart, those of the sessions'
	// other pools.
	SessionPins      map[string]savedPin `json:"session_pins,omitempty"`
	SessionPinsApart []savedPinApart     `json:"session_pins_apart,omitempty"`
}

// everyPin returns every pin saved, by the id of its pool: "" for the one
// pin of a config from before pools.
func (s savedSessions) everyPin() pins {
	every := maps.Clone(s.Pins)
	if every == nil {
		every = make(pins)
	}
	if !s.Pin.IsZero() {
		every[""] = s.Pin
	}
	return every
}

// savedAssignment is an assignment as the state file holds it, with the
// session and model it's for.
type savedAssignment struct {
	Session string `json:"session"`
	Model   string `json:"model"`
	assignment
}

// savedPin is a session's own pin as the state file holds it, in the pool
// with the id Pool gives: none in a config from before pools, nor as a
// router that kept a session's one pin whatever its pool wrote it, which is
// taken in in the pool the session was routed in last.
type savedPin struct {
	ownPin
	Pool string `json:"pool,omitempty"`
}

// savedPinApart is a session's own pin in a pool other than the one
// SessionPins holds its pin in, with the session's id.
type savedPinApart struct {
	Session string `json:"session"`
	savedPin
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
	// Hold is the hold curve. A file without it, as from before it was kept,
	// or with one of a shape it doesn't know, holds the curve of no answers,
	// as forecast.Hold reads it.
	Hold forecast.Hold `json:"hold,omitzero"`
	// PoolsJoined are the pools each account joined latest, joinedKept at
	// most, by the account's id, each with when it joined, oldest first. A
	// file without them, as from before they were kept, records none, and
	// each account joins the pool it's in as the router starts.
	PoolsJoined map[string]poolsJoined `json:"pools_joined,omitempty"`
}

// read counts the accounts whose usage has been read, but those whose
// probes alone are kept.
func (s savedUsage) read() int {
	read := 0
	for _, r := range s.Readings {
		if len(r.Windows) > 0 || r.Extra.Given() {
			read++
		}
	}
	return read
}

// savedReading is an account's windows as last read, and when, and when each
// window was, by its key, the keys of the windows its whole readings stopped
// reporting, in order, and its extra usage; and how its probes went: when one
// last ended, why the last read nothing, how many in a row did, and, where
// that's a read refused, why the API refused it, when the refusal began, and
// whether the hold it started has been told of. A file from before kept none
// of the probes, which reads as none ended, and no window retired; one from
// before kept no refusal, as refusal reads it; and one from before the hold
// was told kept a hold untold. A router from before reads a file kept with
// them, ignoring them.
type savedReading struct {
	ReadAt        time.Time            `json:"read_at,omitzero"`
	Windows       []quota.Window       `json:"windows,omitempty"`
	WindowsReadAt map[string]time.Time `json:"windows_read_at,omitempty"`
	WindowsAbsent []string             `json:"windows_absent,omitempty"`
	Extra         quota.ExtraUsage     `json:"extra_usage,omitzero"`
	Probed        time.Time            `json:"probed,omitzero"`
	ProbeError    string               `json:"probe_error,omitempty"`
	RefusedRead   quota.ReadRefusal    `json:"refused_read,omitempty"`
	Misses        int                  `json:"misses,omitempty"`
	RefusedSince  time.Time            `json:"refused_since,omitzero"`
	HeldTold      bool                 `json:"held_told,omitzero"`
}

// readRefusals has each reading the file kept say why the API refused its
// account's last read, as its refusal reads it, by readIn where a file from
// before kept the refusal's text alone, and reports whether any reads
// otherwise than it was kept, so the file is written again as this router
// keeps it.
func (s savedUsage) readRefusals(readIn func(text string) quota.ReadRefusal) (changed bool) {
	for id, r := range s.Readings {
		if refused := r.refusal(readIn); refused != r.RefusedRead {
			r.RefusedRead = refused
			s.Readings[id] = r
			changed = true
		}
	}
	return changed
}

// refusal is why the API refused the account's last read, as the file kept
// it: its RefusedRead, where that's a refusal, as status.RefusedRead judges
// it. A file from before kept none: one that kept a refusal's start, as a
// router that judged a read refused by its error's text alone wrote it, has
// the refusal readIn reads off that text; one from before the hold, which
// kept no start, has none, as its router held nothing, till the account's
// next read judges it. A refusal this router doesn't know, as a later one
// may have kept, is none.
func (r savedReading) refusal(readIn func(text string) quota.ReadRefusal) quota.ReadRefusal {
	switch {
	case status.RefusedRead(r.RefusedRead) != 0:
		return r.RefusedRead
	case r.RefusedRead == "" && !r.RefusedSince.IsZero():
		return readIn(r.ProbeError)
	}
	return ""
}

// KeptHolds is what the router's state file keeps of what holds each account
// back from every request, which a router started again holds it back by
// while the account holds the token it was found of, as recall takes them
// in: why the API refused its last read, and the hash of the token it held
// then; and the hash of a token a probe found another subscription's. A
// launch with the router down passes over an account so held, as Of has it.
// The zero KeptHolds keeps none.
type KeptHolds struct {
	readings map[string]savedReading
	tokens   map[string]savedTokens
}

// KeptHold is what holds an account back from every request, as the state
// file keeps it of the token the account holds: why the API refused its
// last read, none where it didn't; and whether the token is another
// subscription's. The zero KeptHold holds nothing.
type KeptHold struct {
	RefusedRead quota.ReadRefusal
	Foreign     bool
}

// KeptHoldsIn reads what the state file in stateDir keeps of what holds each
// account back, as KeptHolds has it, a read refused as the router reads it
// as it loads the file, readIn reading one a file from before kept by its
// text alone, as the provider's RefusedReadIn does; and leaves the file as it
// is: none where there's no file. It fails where the file can't be read, or
// isn't one this router reads, as when it's corrupt, which the router sets
// aside as it starts.
func KeptHoldsIn(stateDir string, readIn func(text string) quota.ReadRefusal) (KeptHolds, error) {
	path := filepath.Join(stateDir, stateFileName)
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return KeptHolds{}, nil
	case err != nil:
		return KeptHolds{}, err
	}
	saved, err := parseState(data)
	if err != nil {
		return KeptHolds{}, fmt.Errorf("read the state file %s: %w", path, err)
	}
	saved.readRefusals(readIn)
	return KeptHolds{readings: saved.Readings, tokens: saved.Tokens}, nil
}

// Of is what holds the account with the given id back, which a router
// started again, the account's file holding token, would hold it back by: a
// read refused, or the token another subscription's, each as the file keeps
// it, where it keeps it of token; nothing it keeps of another, as the router
// starts afresh on a token replaced since.
func (k KeptHolds) Of(id string, token tokens.Token) KeptHold {
	held, kept := hash(token.Reveal()), k.tokens[id]
	var hold KeptHold
	if sameToken(kept.SHA256, held) {
		hold.RefusedRead = k.readings[id].RefusedRead
	}
	hold.Foreign = sameToken(kept.Foreign, held)
	return hold
}

// load takes in the state file at path, as the router starts, the pools as
// they stand then, and keeps in it from then on what the router knows that
// should outlast it, telling the state each pool's pin it kept, as recallPins
// says, and emit each the pools left no longer standing, and each hold it
// kept that was never told of, as untold has them. What it held that can no
// longer be used is left out, and the file is to be written again without
// it.
func (f *stateFile) load(path string) {
	f.path = path
	now := f.now()
	saved := f.read(now)
	refusals := saved.readRefusals(f.refusalIn)
	tokens := f.accounts.recall(saved.Tokens, f.heirs, now)
	sessions, unpinned := f.sessions.recall(saved.savedSessions, f.accounts, now, f.state.pools.current())
	f.state.recallPins(f.sessions.pinsNow())
	for _, gone := range unpinned {
		f.emit(gone.unpinned())
	}
	usage := f.state.recall(saved.savedUsage, f.accounts.holding(saved.Tokens))
	untold := f.state.untold()
	for _, held := range untold {
		f.emit(held)
	}
	told := f.told.recall(saved.TokenYears, f.accounts.includes)
	if refusals || tokens || sessions || usage || told || len(untold) > 0 {
		f.changes.note()
	}
	held := f.snapshot()
	logger.Info("loaded state", "path", path, "assignments", len(held.Sessions), "pin", held.everyPin().said(), "readings", held.read())
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

// setAside sets the corrupt state file aside, at now, as atomicfile.SetAside
// does.
func (f *stateFile) setAside(now time.Time, corruption error) {
	aside, err := atomicfile.SetAside(f.path, now)
	if aside == "" {
		logger.Warn("state file corrupt, and can't be set aside; starting empty", "path", f.path, "error", corruption, "rename_error", err)
		return
	}
	logger.Warn("state file corrupt; set aside, starting empty", "path", f.path, "aside", aside, "error", corruption)
	if err != nil {
		logger.Warn("can't remove the older state files set aside", "path", f.path, "error", err)
	}
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
		TokenYears:    f.told.saved(),
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
