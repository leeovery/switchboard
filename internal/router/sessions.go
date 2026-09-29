package router

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

const (
	// forgetAfter is how long a session's assignment lasts unused before
	// it's forgotten.
	forgetAfter = 7 * 24 * time.Hour
	// pruneEvery is how often assignments unused for forgetAfter are
	// forgotten.
	pruneEvery = time.Hour
	// saveAfter is how soon after a change the state file is written, so a
	// burst of changes makes one write.
	saveAfter = time.Second
)

// key is what a session's assignment is remembered by: its id and a model,
// as a prompt cache is a model's.
type key struct {
	session string
	model   string
}

// assignment is the account a session's requests of one model go to.
type assignment struct {
	Account string `json:"account"`
	// Pin is the account the session's own pin named when the session was
	// last routed, or "" when it named none.
	Pin string `json:"pin,omitempty"`
	// Reason says why the session went to its account.
	Reason     string    `json:"reason"`
	AssignedAt time.Time `json:"assigned_at"`
	LastSeen   time.Time `json:"last_seen"`
}

// warm reports whether the session's prompt cache on its account is warm at
// now: it was last used no longer than cacheLife before.
func (a assignment) warm(now time.Time) bool {
	return now.Sub(a.LastSeen) <= cacheLife
}

// forgotten reports whether the assignment has gone unused for forgetAfter
// at now.
func (a assignment) forgotten(now time.Time) bool {
	return now.Sub(a.LastSeen) >= forgetAfter
}

// same reports whether a and b are the same assignment: to the same account,
// made at the same time, however it has been used since.
func (a assignment) same(b assignment) bool {
	return a.Account == b.Account && a.AssignedAt.Equal(b.AssignedAt)
}

// inUTC is the assignment with its times in UTC.
func (a assignment) inUTC() assignment {
	a.AssignedAt, a.LastSeen = a.AssignedAt.UTC(), a.LastSeen.UTC()
	return a
}

// entry is a session's assignment for one model.
type entry struct {
	model string
	assignment
}

// export is the assignment as the control API gives it.
func (e entry) export() status.Assignment {
	return status.Assignment{
		Model:      e.model,
		Account:    e.Account,
		Pinned:     e.Pin != "" && e.Pin == e.Account,
		Reason:     e.Reason,
		AssignedAt: e.AssignedAt,
		LastSeen:   e.LastSeen,
	}
}

// ownPin is a pin a session was given while it ran, which passes over the one
// it was launched with from then on: the session's requests go to Account
// while it has room, or, where Account is "", the pin was cleared, and the
// session has none.
type ownPin struct {
	Account string    `json:"account"`
	Since   time.Time `json:"since"`
}

// found is what's remembered of a session as a request of one of its models
// is chosen for.
type found struct {
	// current is the session's assignment for the model, when assigned is set.
	current  assignment
	assigned bool
	// own is the pin the session was given while it ran, when given is set.
	own   ownPin
	given bool
	// global is the global pin, zero when there's none.
	global status.Pin
}

// pin returns the session's own pin for a request that carries launched, the
// pin the session was launched with, and when the session was given it: the
// pin given while it runs, which passes over launched, else launched, given as
// the session started, which zero stands for.
func (f found) pin(launched string) (string, time.Time) {
	if f.given {
		return f.own.Account, f.own.Since
	}
	return launched, time.Time{}
}

// sessions remembers the account each session's requests of each model go
// to, the pins sessions are given while they run, and the global pin, and
// keeps them in the state file once it has one, with what the accounts know
// of their tokens. It's safe for concurrent use.
type sessions struct {
	now func() time.Time

	mu sync.Mutex
	// file is the state file, or nil to keep nothing.
	file *stateFile
	// accounts are those whose tokens the state file keeps what's known of,
	// once it's loaded.
	accounts    accounts
	assignments map[key]assignment
	// own holds the pins sessions were given while they ran, by session id.
	own map[string]ownPin
	pin status.Pin
	// unsaved is set while the state file lacks a change.
	unsaved bool
	// changed signals a change to keep.
	changed chan struct{}
}

func newSessions(now func() time.Time) *sessions {
	return &sessions{
		now:         now,
		assignments: make(map[key]assignment),
		own:         make(map[string]ownPin),
		changed:     make(chan struct{}, 1),
	}
}

// lookup returns what's remembered of the session and model k name.
func (s *sessions) lookup(k key) found {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, assigned := s.assignments[k]
	own, given := s.own[k.session]
	return found{current: current, assigned: assigned, own: own, given: given, global: s.pin}
}

// remember notes that a request of the session and model k names went where
// d says at now, carrying pin as the session's own, unless the assignment of
// k has changed since the request's choice found it as was, zero for none:
// another request of the session moved it meanwhile, and that newer
// assignment stands. It returns the assignment it found, and reports whether
// it noted the request.
func (s *sessions) remember(k key, was assignment, pin string, d decision, now time.Time) (assignment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := s.assignments[k]
	if !found.same(was) {
		return found, false
	}
	a := found
	if a.Account != d.account {
		a.Account, a.AssignedAt = d.account, now
	}
	if !d.sticky {
		a.Reason = d.reason
	}
	a.Pin, a.LastSeen = pin, now
	s.assignments[k] = a.inUTC()
	s.change()
	return found, true
}

// globalPin returns the global pin, zero when there's none.
func (s *sessions) globalPin() status.Pin {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pin
}

// setPin sets the global pin, and with force, clears every session's own pin
// at once, as clearOwn does. It returns how many sessions had one.
func (s *sessions) setPin(pin status.Pin, force bool) (cleared int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pin = pin
	if force {
		cleared = s.clearOwn()
	}
	s.change()
	return cleared
}

// unpin clears the global pin, and with force, every session's own pin, as
// clearOwn does. It returns the global pin as it was, and how many sessions
// had a pin of their own.
func (s *sessions) unpin(force bool) (was status.Pin, cleared int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	was, s.pin = s.pin, status.Pin{}
	if force {
		cleared = s.clearOwn()
	}
	if was != (status.Pin{}) || cleared > 0 {
		s.change()
	}
	return was, cleared
}

// pinSession gives the session with the given id its own pin to account from
// now on, passing over the one it was launched with, or, where account is "",
// clears its own pin, the one it was launched with included. It reports
// false, and does nothing, for a session never seen.
func (s *sessions) pinSession(id, account string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.seen(id) {
		return false
	}
	s.own[id] = ownPin{Account: account, Since: s.now().UTC()}
	s.change()
	return true
}

// session reports what the router says of the session with the given id,
// but for the status of its account, and false for a session never seen.
func (s *sessions) session(id string) (status.Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, ok := s.entries()[id]
	if !ok {
		return status.Session{}, false
	}
	return s.report(id, entries), true
}

// running reports the sessions routed in the last hour at now, each as
// session does, the one seen last first.
func (s *sessions) running(now time.Time) []status.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	listed := []status.Session{}
	for id, entries := range s.entries() {
		if entries[0].warm(now) {
			listed = append(listed, s.report(id, entries))
		}
	}
	slices.SortFunc(listed, func(a, b status.Session) int {
		return cmp.Or(b.Assignments[0].LastSeen.Compare(a.Assignments[0].LastSeen), cmp.Compare(a.ID, b.ID))
	})
	return listed
}

// active counts the sessions whose caches are warm at now: by account, and in
// all, where a session whose models went to two accounts counts once.
func (s *sessions) active(now time.Time) (byAccount map[string]int, all int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	on := make(map[string]map[string]bool)
	anywhere := make(map[string]bool)
	for k, a := range s.assignments {
		if !a.warm(now) {
			continue
		}
		if on[a.Account] == nil {
			on[a.Account] = make(map[string]bool)
		}
		on[a.Account][k.session] = true
		anywhere[k.session] = true
	}
	byAccount = make(map[string]int, len(on))
	for account, sessions := range on {
		byAccount[account] = len(sessions)
	}
	return byAccount, len(anywhere)
}

// prune forgets the assignments gone unused for forgetAfter at now, with the
// pins of the sessions it forgets, and the accounts' former tokens that no
// longer count.
func (s *sessions) prune(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	held := len(s.assignments)
	maps.DeleteFunc(s.assignments, func(_ key, a assignment) bool { return a.forgotten(now) })
	if forgotten := held - len(s.assignments); forgotten > 0 {
		logger.Debug("forgot sessions unused for a week", "assignments", forgotten)
		maps.DeleteFunc(s.own, func(id string, _ ownPin) bool { return !s.seen(id) })
		s.change()
	}
	if s.accounts.forget(now) {
		logger.Debug("forgot tokens replaced a week ago")
		s.change()
	}
}

// tokensChanged notes that what the accounts know of their tokens has
// changed, for the state file to keep.
func (s *sessions) tokensChanged() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.change()
}

// change notes a change the state file lacks, for keep to save. s.mu must be
// held.
func (s *sessions) change() {
	s.unsaved = true
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// entries returns every session's assignments, by the session's id, the one
// used last first. s.mu must be held.
func (s *sessions) entries() map[string][]entry {
	bySession := make(map[string][]entry)
	for k, a := range s.assignments {
		bySession[k.session] = append(bySession[k.session], entry{model: k.model, assignment: a})
	}
	for _, entries := range bySession {
		slices.SortFunc(entries, func(a, b entry) int {
			return cmp.Or(b.LastSeen.Compare(a.LastSeen), cmp.Compare(a.model, b.model))
		})
	}
	return bySession
}

// report is what the router says of the session with the given id, whose
// assignments are entries, the one used last first, but for the status of its
// account. s.mu must be held.
func (s *sessions) report(id string, entries []entry) status.Session {
	assignments := make([]status.Assignment, len(entries))
	for i, e := range entries {
		assignments[i] = e.export()
	}
	return status.Session{ID: id, Pin: s.pinOf(id, entries[0].assignment), Assignments: assignments}
}

// pinOf returns the own pin of the session with the given id, whose last-used
// assignment is last: the one it was given while it ran, else the one its
// requests last carried. s.mu must be held.
func (s *sessions) pinOf(id string, last assignment) string {
	if own, given := s.own[id]; given {
		return own.Account
	}
	return last.Pin
}

// clearOwn clears the own pin of every session that has one, the one it was
// launched with included, which its requests go on carrying and are passed
// over from then on, and returns how many sessions had one. A session first
// seen afterwards has the pin it's launched with. s.mu must be held.
func (s *sessions) clearOwn() int {
	cleared := ownPin{Since: s.now().UTC()}
	n := 0
	for id, entries := range s.entries() {
		if s.pinOf(id, entries[0].assignment) != "" {
			s.own[id] = cleared
			n++
		}
	}
	return n
}

// seen reports whether the session with the given id has an assignment. s.mu
// must be held.
func (s *sessions) seen(id string) bool {
	for k := range s.assignments {
		if k.session == id {
			return true
		}
	}
	return false
}

// load takes in the state file at path, and keeps in it from then on the
// sessions, their own pins, the global pin, and what accounts know of their
// tokens. Assignments gone unused for forgetAfter are forgotten, and none of
// an assignment, a session's own pin and the global pin is kept for an
// account requests can't go out on.
func (s *sessions) load(path string, accounts accounts) {
	file := &stateFile{path: path, write: writeAtomic}
	now := s.now()
	saved := file.read(now)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.file, s.accounts = file, accounts
	changed := accounts.recall(saved.Tokens, now)
	changed = s.loadAssignments(saved.Sessions, now) || changed
	changed = s.loadOwnPins(saved.SessionPins) || changed
	changed = s.loadPin(saved.Pin) || changed
	if changed {
		s.change()
	}
	logger.Info("loaded state", "path", path, "assignments", len(s.assignments), "pin", s.pin.Account)
}

// loadAssignments takes in the assignments saved, but those gone unused for
// forgetAfter at now, and those of accounts requests can't go out on, and
// reports whether it left any out. s.mu must be held.
func (s *sessions) loadAssignments(saved []savedAssignment, now time.Time) (dropped bool) {
	for _, a := range saved {
		if a.Session == "" || a.forgotten(now) || !s.accounts.canSend(a.Account) {
			dropped = true
			continue
		}
		s.assignments[key{session: a.Session, model: a.Model}] = a.inUTC()
	}
	return dropped
}

// loadOwnPins takes in the sessions' own pins saved, but those of sessions
// without an assignment, and those to accounts requests can't go out on, and
// reports whether it left any out. s.mu must be held.
func (s *sessions) loadOwnPins(saved map[string]ownPin) (dropped bool) {
	for id, own := range saved {
		switch {
		case !s.seen(id):
			dropped = true
		case own.Account != "" && !s.accounts.canSend(own.Account):
			logger.Warn("session's pin dropped: nothing can go out on its account", "session", status.ShortID(id), "account", own.Account)
			dropped = true
		default:
			s.own[id] = own
		}
	}
	return dropped
}

// loadPin takes in the global pin saved, unless it's to an account requests
// can't go out on, and reports whether it left it out. s.mu must be held.
func (s *sessions) loadPin(saved status.Pin) (dropped bool) {
	switch {
	case saved.Account == "":
	case s.accounts.canSend(saved.Account):
		s.pin = saved
	default:
		logger.Warn("pin dropped: nothing can go out on its account", "account", saved.Account)
		dropped = true
	}
	return dropped
}

// keep writes the state file saveAfter after a change, so a burst of changes
// makes one write; forgets the assignments gone unused for forgetAfter every
// pruneEvery; and writes the file once more as ctx ends.
func (s *sessions) keep(ctx context.Context) {
	prune := time.NewTicker(pruneEvery)
	defer prune.Stop()
	for {
		select {
		case <-s.changed:
			select {
			case <-time.After(saveAfter):
			case <-ctx.Done():
			}
			s.save()
		case <-prune.C:
			s.prune(s.now())
		case <-ctx.Done():
			s.save()
			return
		}
	}
}

// save writes the state file, when it lacks a change. A write that fails
// leaves the change for the next.
func (s *sessions) save() {
	s.mu.Lock()
	if s.file == nil || !s.unsaved {
		s.mu.Unlock()
		return
	}
	file, snapshot := s.file, s.snapshot()
	s.unsaved = false
	s.mu.Unlock()
	if err := file.save(snapshot); err != nil {
		logger.Warn("can't save the state file", "path", file.path, "error", err)
		s.mu.Lock()
		s.unsaved = true
		s.mu.Unlock()
	}
}

// snapshot is what the state file is to hold, in a steady order. s.mu must be
// held.
func (s *sessions) snapshot() savedState {
	saved := savedState{
		Version:     stateVersion,
		Pin:         s.pin,
		Sessions:    make([]savedAssignment, 0, len(s.assignments)),
		SessionPins: maps.Clone(s.own),
		Tokens:      s.accounts.kept(),
	}
	for k, a := range s.assignments {
		saved.Sessions = append(saved.Sessions, savedAssignment{Session: k.session, Model: k.model, assignment: a})
	}
	slices.SortFunc(saved.Sessions, func(a, b savedAssignment) int {
		return cmp.Or(cmp.Compare(a.Session, b.Session), cmp.Compare(a.Model, b.Model))
	})
	return saved
}
