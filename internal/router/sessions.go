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

// export is the assignment as the control API gives it, for model.
func (a assignment) export(model string) Assignment {
	return Assignment{
		Model:      model,
		Account:    a.Account,
		Pinned:     a.Pin != "" && a.Pin == a.Account,
		Reason:     a.Reason,
		AssignedAt: a.AssignedAt,
		LastSeen:   a.LastSeen,
	}
}

// sessions remembers the account each session's requests of each model go
// to, and the global pin, and keeps them in the state file once it has one.
// It's safe for concurrent use.
type sessions struct {
	now func() time.Time

	mu sync.Mutex
	// file is the state file, or nil to keep nothing.
	file        *stateFile
	assignments map[key]assignment
	pin         status.Pin
	// unsaved is set while the state file lacks a change.
	unsaved bool
	// changed signals a change to keep.
	changed chan struct{}
}

func newSessions(now func() time.Time) *sessions {
	return &sessions{now: now, assignments: make(map[key]assignment), changed: make(chan struct{}, 1)}
}

// lookup returns the assignment k names, if there is one, and the global pin.
func (s *sessions) lookup(k key) (assignment, bool, status.Pin) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.assignments[k]
	return a, ok, s.pin
}

// remember notes that a request of the session and model k names went where
// d says at now, carrying pin as the session's own. It returns the account
// they went to before, or "" for a new session.
func (s *sessions) remember(k key, pin string, d decision, now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.assignments[k]
	before := a.Account
	if !ok || a.Account != d.account {
		a.Account, a.AssignedAt = d.account, now
	}
	if !d.sticky {
		a.Reason = d.reason
	}
	a.Pin, a.LastSeen = pin, now
	s.assignments[k] = a
	s.change()
	return before
}

// globalPin returns the global pin, zero when there's none.
func (s *sessions) globalPin() status.Pin {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pin
}

// setPin sets the global pin.
func (s *sessions) setPin(pin status.Pin) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pin = pin
	s.change()
}

// unpin clears the global pin, and returns it as it was.
func (s *sessions) unpin() status.Pin {
	s.mu.Lock()
	defer s.mu.Unlock()
	was := s.pin
	if was != (status.Pin{}) {
		s.pin = status.Pin{}
		s.change()
	}
	return was
}

// of returns the assignments of the session with the given id, the one used
// last first.
func (s *sessions) of(id string) []Assignment {
	s.mu.Lock()
	defer s.mu.Unlock()
	var found []Assignment
	for k, a := range s.assignments {
		if k.session == id {
			found = append(found, a.export(k.model))
		}
	}
	slices.SortFunc(found, func(a, b Assignment) int {
		return cmp.Or(b.LastSeen.Compare(a.LastSeen), cmp.Compare(a.Model, b.Model))
	})
	return found
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

// prune forgets the assignments gone unused for forgetAfter at now.
func (s *sessions) prune(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	held := len(s.assignments)
	maps.DeleteFunc(s.assignments, func(_ key, a assignment) bool { return a.forgotten(now) })
	if forgotten := held - len(s.assignments); forgotten > 0 {
		logger.Debug("forgot sessions unused for a week", "assignments", forgotten)
		s.change()
	}
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

// load takes in the state file at path, and keeps the sessions in it from
// then on. Assignments gone unused for forgetAfter are forgotten, and neither
// an assignment nor the pin is kept for an account that sendable says
// requests can't go out on.
func (s *sessions) load(path string, sendable func(id string) bool) {
	file := &stateFile{path: path, write: writeAtomic}
	now := s.now()
	saved := file.read(now)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.file = file
	dropped := false
	for _, a := range saved.Sessions {
		if a.Session == "" || a.forgotten(now) || !sendable(a.Account) {
			dropped = true
			continue
		}
		s.assignments[key{session: a.Session, model: a.Model}] = a.assignment
	}
	switch pin := saved.Pin; {
	case pin.Account == "":
	case sendable(pin.Account):
		s.pin = pin
	default:
		logger.Warn("pin dropped: nothing can go out on its account", "account", pin.Account)
		dropped = true
	}
	if dropped {
		s.change()
	}
	logger.Info("loaded state", "path", path, "assignments", len(s.assignments), "pin", s.pin.Account)
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
	saved := savedState{Version: stateVersion, Pin: s.pin, Sessions: make([]savedAssignment, 0, len(s.assignments))}
	for k, a := range s.assignments {
		saved.Sessions = append(saved.Sessions, savedAssignment{Session: k.session, Model: k.model, assignment: a})
	}
	slices.SortFunc(saved.Sessions, func(a, b savedAssignment) int {
		return cmp.Or(cmp.Compare(a.Session, b.Session), cmp.Compare(a.Model, b.Model))
	})
	return saved
}
