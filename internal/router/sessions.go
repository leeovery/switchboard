package router

import (
	"cmp"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// forgetAfter is how long a session's assignment lasts unused before it's
// forgotten.
const forgetAfter = 7 * 24 * time.Hour

// key is what a session's assignment is remembered by: its id and a model,
// as a prompt cache is a model's.
type key struct {
	session string
	model   string
}

// key returns what the assignment of the request's session, for its model,
// is remembered by.
func (r Request) key() key {
	return key{session: r.Session, model: r.Model}
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
	// by is the id of the request last noted on the assignment, which made
	// it, moved it or stayed on it: none for one the state file kept.
	by string
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

// usedAgain reports whether a is b used again, and nothing more: the same
// assignment, with the same pin and reason.
func (a assignment) usedAgain(b assignment) bool {
	return a.same(b) && a.Pin == b.Pin && a.Reason == b.Reason
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
// to, the pins sessions are given while they run, and the global pin, all of
// which the state file keeps. It's safe for concurrent use.
type sessions struct {
	now func() time.Time
	// changed hears of each change, for the state file to keep, and usedAgain
	// of each that's only an assignment used again, which it keeps less
	// often, both with s.mu held: they mustn't block, nor call s.
	changed, usedAgain func()

	mu          sync.Mutex
	assignments map[key]assignment
	// own holds the pins sessions were given while they ran, by session id.
	own map[string]ownPin
	pin status.Pin
}

func newSessions(now func() time.Time, changed, usedAgain func()) *sessions {
	return &sessions{
		now:         now,
		changed:     changed,
		usedAgain:   usedAgain,
		assignments: make(map[key]assignment),
		own:         make(map[string]ownPin),
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

// remember notes that req went where d says at now, carrying its pin as its
// session's own, unless the assignment of its session and model has changed
// since req's choice found it as was, zero for none: another request of the
// session moved it meanwhile, and that newer assignment stands. It returns
// the assignment it found, and reports whether it noted the request.
func (s *sessions) remember(req Request, was assignment, d decision, now time.Time) (assignment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := req.key()
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
	a.Pin, a.LastSeen, a.by = req.Pin, now, req.ID
	s.assignments[k] = a.inUTC()
	if a.usedAgain(found) {
		s.usedAgain()
	} else {
		s.changed()
	}
	return found, true
}

// forget forgets the assignment of req's session and model while req is the
// last request noted on it, and reports whether it did: one noted since
// stands, whether it made the assignment, moved it or stayed on it.
func (s *sessions) forget(req Request) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := req.key()
	if a, ok := s.assignments[k]; !ok || a.by != req.ID {
		return false
	}
	delete(s.assignments, k)
	s.changed()
	return true
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
	s.changed()
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
	if !was.IsZero() || cleared > 0 {
		s.changed()
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
	s.changed()
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
// pins of the sessions it forgets.
func (s *sessions) prune(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	held := len(s.assignments)
	maps.DeleteFunc(s.assignments, func(_ key, a assignment) bool { return a.forgotten(now) })
	if forgotten := held - len(s.assignments); forgotten > 0 {
		logger.Debug("forgot sessions unused for a week", "assignments", forgotten)
		maps.DeleteFunc(s.own, func(id string, _ ownPin) bool { return !s.seen(id) })
		s.changed()
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

// saved is what the state file keeps of the sessions, in a steady order: the
// global pin, the sessions' assignments, and the pins they were given while
// they ran.
func (s *sessions) saved() savedSessions {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved := savedSessions{
		Pin:         s.pin,
		Sessions:    make([]savedAssignment, 0, len(s.assignments)),
		SessionPins: maps.Clone(s.own),
	}
	for k, a := range s.assignments {
		saved.Sessions = append(saved.Sessions, savedAssignment{Session: k.session, Model: k.model, assignment: a})
	}
	slices.SortFunc(saved.Sessions, func(a, b savedAssignment) int {
		return cmp.Or(cmp.Compare(a.Session, b.Session), cmp.Compare(a.Model, b.Model))
	})
	return saved
}

// recall takes in what the state file kept of the sessions, as the router
// starts at now, but for what can no longer be used: assignments gone unused
// for forgetAfter, and any assignment, session's own pin or global pin to an
// account no longer configured, of the accounts given. One to an account
// without a usable token stands, as its token file may only have been caught
// as it's rewritten: while the account has none, choices pass it over. It
// reports whether it left anything out.
func (s *sessions) recall(saved savedSessions, accounts accounts, now time.Time) (dropped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dropped = s.recallAssignments(saved.Sessions, accounts, now)
	dropped = s.recallOwnPins(saved.SessionPins, accounts) || dropped
	return s.recallPin(saved.Pin, accounts) || dropped
}

// recallAssignments takes in the assignments saved, but those gone unused for
// forgetAfter at now, and those of accounts no longer configured, and reports
// whether it left any out. s.mu must be held.
func (s *sessions) recallAssignments(saved []savedAssignment, accounts accounts, now time.Time) (dropped bool) {
	for _, a := range saved {
		if a.Session == "" || a.forgotten(now) || !accounts.includes(a.Account) {
			dropped = true
			continue
		}
		s.assignments[key{session: a.Session, model: a.Model}] = a.inUTC()
	}
	return dropped
}

// recallOwnPins takes in the sessions' own pins saved, but those of sessions
// without an assignment, and those to accounts no longer configured, and
// reports whether it left any out. s.mu must be held.
func (s *sessions) recallOwnPins(saved map[string]ownPin, accounts accounts) (dropped bool) {
	for id, own := range saved {
		switch {
		case !s.seen(id):
			dropped = true
		case own.Account != "" && !accounts.includes(own.Account):
			logger.Warn("session's pin dropped: its account is no longer configured", "session", status.ShortID(id), "account", own.Account)
			dropped = true
		default:
			s.own[id] = own
		}
	}
	return dropped
}

// recallPin takes in the global pin saved, but for its accounts no longer
// configured, the pin going with the last of them, and reports whether it
// left any out. It names the rest in the order configured. s.mu must be held.
func (s *sessions) recallPin(saved status.Pin, accounts accounts) (dropped bool) {
	kept := accounts.only(saved.Accounts).configured().IDs()
	for _, id := range except(saved.Accounts, kept) {
		logger.Warn("dropped from the pin: the account is no longer configured", "account", id)
		dropped = true
	}
	if saved.Accounts = kept; !saved.IsZero() {
		s.pin = saved
	}
	return dropped
}
