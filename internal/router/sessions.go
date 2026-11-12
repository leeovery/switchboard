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
	// Dir is the directory the session's last request to name one was
	// started in, as the request ledger gives it, or "" while none has.
	Dir string `json:"dir,omitempty"`
	// Pool is the id of the pool the session's last request was routed
	// within: "" in a config from before pools, or as the state file kept it
	// from a router that didn't know pools, which counts as the first pool,
	// as a request naming none does.
	Pool string `json:"pool,omitempty"`
	// Reason says why the session went to its account.
	Reason string `json:"reason"`
	// AssignedAt is when the session came to its account, which a choice
	// that keeps it there leaves as it was: its time on the account, as the
	// hold curve weighs its answers, runs from then.
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

// dirAfter is the directory the assignment names once req is noted on it:
// req's, where it names one, else the one it named.
func (a assignment) dirAfter(req Request) string {
	return cmp.Or(req.Dir, a.Dir)
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
// assignment, with the same pin, directory, pool and reason.
func (a assignment) usedAgain(b assignment) bool {
	return a.same(b) && a.Pin == b.Pin && a.Dir == b.Dir && a.Pool == b.Pool && a.Reason == b.Reason
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
		Dir:        e.Dir,
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
	// pins are each pool's pin.
	pins pins
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

// situation is what's known as req's account is chosen at now, f being what's
// remembered of its session and accounts what's known of the accounts: the
// request in the pool the view has it in, under that pool's pin, as the
// view's pools have it, pinned as its session's own pin has it in that pool,
// as pinIn says.
func (f found) situation(req Request, now time.Time, accounts view) situation {
	on := situation{req: req, now: now, current: f.current, assigned: f.assigned, pin: f.pins[accounts.pool], accounts: accounts}
	on.req.Pool = accounts.pool
	on.req.Pin, on.pinnedAt, on.unpooled = f.pinIn(req.Pin, accounts)
	return on
}

// pinIn returns the session's own pin as pin does, for a request that carries
// launched, in the pool accounts is a view of, and the account of the pin it
// passes over as it's outside that pool, "" where it passes over none. A pin
// outside the pool is no pin for the request, nor does it pass over the one
// the session was launched with, which stands where it's in the pool.
func (f found) pinIn(launched string, accounts view) (pin string, since time.Time, unpooled string) {
	pin, since = f.pin(launched)
	switch {
	case pin == "" || accounts.pooled(pin):
		return pin, since, ""
	case f.given && accounts.pooled(launched):
		return launched, time.Time{}, pin
	}
	return "", time.Time{}, pin
}

// sessions remembers the account each session's requests of each model go
// to, the pins sessions are given while they run, and each pool's pin, all
// of which the state file keeps. It's safe for concurrent use.
type sessions struct {
	now func() time.Time
	// changed hears of each change, for the state file to keep, and usedAgain
	// of each that's only an assignment used again, which it keeps less
	// often, both with s.mu held: they mustn't block, nor call s.
	changed, usedAgain func()
	// bound reports whether a model's thinking is bound to the account that
	// produced it, which keeps a session that yielded its pin where it went
	// once its cache is cold: no model's, until the router says.
	bound func(model string) bool

	mu          sync.Mutex
	assignments map[key]assignment
	// before holds, by session and model, the assignment as it stood before
	// the request last noted on it first noted it, which forget puts back:
	// none for a session that had none.
	before map[key]assignment
	// unstarted holds the sessions and models assigned afresh that are yet to
	// be told of as started, as none of their requests has been answered
	// with success: never one the state file kept, told of before.
	unstarted map[key]bool
	// own holds the pins sessions were given while they ran, by session id.
	own  map[string]ownPin
	pins pins
}

func newSessions(now func() time.Time, changed, usedAgain func()) *sessions {
	return &sessions{
		now:         now,
		changed:     changed,
		usedAgain:   usedAgain,
		bound:       func(string) bool { return false },
		assignments: make(map[key]assignment),
		before:      make(map[key]assignment),
		unstarted:   make(map[key]bool),
		own:         make(map[string]ownPin),
	}
}

// lookup returns what's remembered of the session and model k name.
func (s *sessions) lookup(k key) found {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, assigned := s.assignments[k]
	own, given := s.own[k.session]
	return found{current: current, assigned: assigned, own: own, given: given, pins: s.pins}
}

// remember notes that req went where d says at now, carrying its pin as its
// session's own, its pool, and its directory, should it name one, unless the
// assignment of its session and model has changed since req's choice found
// it as was, zero for none: another request of the session moved it
// meanwhile, and that newer assignment stands. It returns the assignment it
// found, and reports whether it noted the request.
func (s *sessions) remember(req Request, was assignment, d decision, now time.Time) (assignment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := req.key()
	found, had := s.assignments[k]
	if !found.same(was) {
		return found, false
	}
	if !had || found.by != req.ID {
		s.noteBefore(k, found, had)
	}
	if !had {
		s.unstarted[k] = true
	}
	a := found
	if a.Account != d.account {
		a.Account, a.AssignedAt = d.account, now
	}
	if !d.sticky {
		a.Reason = d.reason
	}
	a.Pin, a.Pool, a.LastSeen, a.by = req.Pin, req.Pool, now, req.ID
	a.Dir = a.dirAfter(req)
	s.assignments[k] = a.inUTC()
	if a.usedAgain(found) {
		s.usedAgain()
	} else {
		s.changed()
	}
	return found, true
}

// noteBefore notes the assignment of the session and model k name as it
// stands before a request first notes it, which forget puts back: found,
// else none, when had is unset. s.mu must be held.
func (s *sessions) noteBefore(k key, found assignment, had bool) {
	if had {
		s.before[k] = found
		return
	}
	delete(s.before, k)
}

// forget puts the assignment of req's session and model back as it stood
// before req first noted it, forgetting it when there was none, while req is
// the last request noted on it: one noted since stands, whether it made the
// assignment, moved it or stayed on it. It returns the assignment req left,
// and the one it put back, none when it's forgotten, and reports whether it
// put it back.
func (s *sessions) forget(req Request) (left, back assignment, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := req.key()
	left, ok = s.assignments[k]
	if !ok || left.by != req.ID {
		return assignment{}, assignment{}, false
	}
	back, had := s.before[k]
	delete(s.before, k)
	if had {
		s.assignments[k] = back
	} else {
		delete(s.assignments, k)
		delete(s.unstarted, k)
	}
	s.changed()
	return left, back, true
}

// started notes that a request of the session and model k name was answered
// with success, and returns their assignment, reporting true while it's yet
// to be told of as started, which it is from then on.
func (s *sessions) started(k key) (assignment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.assignments[k]
	if !ok || !s.unstarted[k] {
		return assignment{}, false
	}
	delete(s.unstarted, k)
	return a, true
}

// onAccount returns how long, at now, the session and model k name have been
// on the account with the given id, as their assignment says: none where
// they have no assignment there.
func (s *sessions) onAccount(k key, id string, now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.assignments[k]
	if !ok || a.Account != id {
		return 0
	}
	return now.Sub(a.AssignedAt)
}

// pinsNow returns each pool's pin.
func (s *sessions) pinsNow() pins {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pins
}

// setPin sets the pin of the pool rd reads, as with says, and with force,
// clears the own pin of every session in that pool at once, as clearOwn
// does. It returns how many sessions had one.
func (s *sessions) setPin(rd poolRead, pin status.Pin, force bool) (cleared int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pins = s.pins.with(rd.pool, pin)
	if force {
		cleared = s.clearOwn(rd.routes)
	}
	s.changed()
	return cleared
}

// unpin clears the pin of the pool rd reads, as without says, and with
// force, the own pin of every session in that pool, as clearOwn does. It
// returns the pool's pin as it was, and how many sessions had a pin of their
// own.
func (s *sessions) unpin(rd poolRead, force bool) (was status.Pin, cleared int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pins, was = s.pins.without(rd.pool)
	if force {
		cleared = s.clearOwn(rd.routes)
	}
	if !was.IsZero() || cleared > 0 {
		s.changed()
	}
	return was, cleared
}

// fit fits each pool's pin to the pools as t has them, as fitted says, and
// returns the pins as it leaves them, and each it dropped whole.
func (s *sessions) fit(t *poolTable) (pins, []gonePin) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next, gone, changed := s.pins.fitted(t)
	if changed {
		s.pins = next
		s.changed()
	}
	return s.pins, gone
}

// poolOf returns the pool the session with the given id was last routed
// within, as remembered of the request it saw last, reporting false for a
// session never seen.
func (s *sessions) poolOf(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	last, seen := s.lastUsed(id)
	return last.Pool, seen
}

// pinSession gives the session with the given id its own pin to account from
// now on, passing over the one it was launched with, or, where account is "",
// clears its own pin, the one it was launched with included. It returns the
// account the session's own pin named before, "" for none, and reports
// false, and does nothing, for a session never seen.
func (s *sessions) pinSession(id, account string) (was string, seen bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	last, seen := s.lastUsed(id)
	if !seen {
		return "", false
	}
	was, _ = s.pinOf(id, last)
	s.own[id] = ownPin{Account: account, Since: s.now().UTC()}
	s.changed()
	return was, true
}

// session reports what the router says of the session with the given id,
// but for the status of its account, and false for a session never seen, or
// one whose pool in isn't, as remembered of the request it saw last.
func (s *sessions) session(id string, in func(pool string) bool) (status.Session, bool) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, ok := s.entries()[id]
	if !ok || !in(entries[0].Pool) {
		return status.Session{}, false
	}
	return s.report(id, entries, now), true
}

// running reports the sessions routed in the last hour at now whose pool, as
// remembered of the request each saw last, in is, each as session does, the
// one seen last first.
func (s *sessions) running(now time.Time, in func(pool string) bool) []status.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	listed := []status.Session{}
	for id, entries := range s.entries() {
		if entries[0].warm(now) && in(entries[0].Pool) {
			listed = append(listed, s.report(id, entries, now))
		}
	}
	slices.SortFunc(listed, status.SeenLastFirst)
	return listed
}

// active counts the sessions whose caches are warm at now, of their models'
// requests routed in a pool in is, as remembered of the last of each: by
// account, and in all, where a session whose models went to two accounts
// counts once.
func (s *sessions) active(now time.Time, in func(pool string) bool) (byAccount map[string]int, all int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	on := make(map[string]map[string]bool)
	anywhere := make(map[string]bool)
	for k, a := range s.assignments {
		if !a.warm(now) || !in(a.Pool) {
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
	maps.DeleteFunc(s.before, func(k key, _ assignment) bool {
		_, assigned := s.assignments[k]
		return !assigned
	})
	maps.DeleteFunc(s.unstarted, func(k key, _ bool) bool {
		_, assigned := s.assignments[k]
		return !assigned
	})
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
		slices.SortFunc(entries, byUse)
	}
	return bySession
}

// lastUsed returns the assignment the session with the given id used last,
// the first of its entries, reporting false for a session never seen. s.mu
// must be held.
func (s *sessions) lastUsed(id string) (assignment, bool) {
	var last entry
	seen := false
	for k, a := range s.assignments {
		if e := (entry{model: k.model, assignment: a}); k.session == id && (!seen || byUse(e, last) < 0) {
			last, seen = e, true
		}
	}
	return last.assignment, seen
}

// dir is the directory the session with the given id runs in: that of its
// model used last to name one, as the dashboard reads it off the session's
// listing; "" where none does, as for a session never seen.
func (s *sessions) dir(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var last entry
	for k, a := range s.assignments {
		if e := (entry{model: k.model, assignment: a}); k.session == id && a.Dir != "" && (last.Dir == "" || byUse(e, last) < 0) {
			last = e
		}
	}
	return last.Dir
}

// byUse orders a session's entries the one used last first, then by model.
func byUse(a, b entry) int {
	return cmp.Or(b.LastSeen.Compare(a.LastSeen), cmp.Compare(a.model, b.model))
}

// report is what the router says at now of the session with the given id,
// whose assignments are entries, the one used last first, but for the status
// of its account. s.mu must be held.
func (s *sessions) report(id string, entries []entry, now time.Time) status.Session {
	pin, pinnedAt := s.pinOf(id, entries[0].assignment)
	assignments := make([]status.Assignment, len(entries))
	for i, e := range entries {
		assignments[i] = e.export()
		if pin != "" {
			assignments[i].PinnedAt = pinnedAt
		}
		assignments[i].Yielded = e.yielded(pin, pinnedAt, now, s.bound(e.model))
	}
	return status.Session{ID: id, Pin: pin, Assignments: assignments}
}

// yielded reports whether, at now, the session's own pin to the account pin
// names, given at pinnedAt, zero for the one it was launched with, has
// yielded, the session going elsewhere for want of room there, and the
// session stays where it went, as a choice for it would keep it there: its
// cache is warm, or its model's thinking is bound to the account, as bound
// says. A pin given since the session was last routed sends it on.
func (e entry) yielded(pin string, pinnedAt, now time.Time, bound bool) bool {
	return pin != "" && e.Pin == pin && e.Account != pin && e.LastSeen.After(pinnedAt) && (e.warm(now) || bound)
}

// pinOf returns the own pin of the session with the given id, whose last-used
// assignment is last: the one it was given while it ran, and when, else the
// one its requests last carried, given as it started, which zero stands for.
// s.mu must be held.
func (s *sessions) pinOf(id string, last assignment) (string, time.Time) {
	if own, given := s.own[id]; given {
		return own.Account, own.Since
	}
	return last.Pin, time.Time{}
}

// clearOwn clears the own pin of every session that has one in a pool in
// says, as remembered of the request it saw last, the one it was launched
// with included, which its requests go on carrying and are passed over from
// then on, and returns how many sessions had one. A session first seen
// afterwards has the pin it's launched with. s.mu must be held.
func (s *sessions) clearOwn(in func(pool string) bool) int {
	cleared := ownPin{Since: s.now().UTC()}
	n := 0
	for id, entries := range s.entries() {
		if pin, _ := s.pinOf(id, entries[0].assignment); pin != "" && in(entries[0].Pool) {
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

// saved is what the state file keeps of the sessions, in a steady order:
// each pool's pin, the one of a config from before pools as such, the
// sessions' assignments, and the pins they were given while they ran.
func (s *sessions) saved() savedSessions {
	s.mu.Lock()
	defer s.mu.Unlock()
	pooled := maps.Clone(s.pins)
	delete(pooled, "")
	saved := savedSessions{
		Pin:         s.pins[""],
		Pins:        pooled,
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
// starts at now, the pools as t has them, but for what can no longer be
// used: assignments gone unused for forgetAfter, any assignment, session's
// own pin or pool's pin to an account no longer configured, of the accounts
// given, each pool's pin fitted to the pools, as recallPins says. One to an
// account without a usable token stands, as its token file may only have
// been caught as it's rewritten: while the account has none, choices pass it
// over. It reports whether it left anything out, or fitted any pin, and
// returns each pool's pin the fit dropped whole.
func (s *sessions) recall(saved savedSessions, accounts accounts, now time.Time, t *poolTable) (bool, []gonePin) {
	s.mu.Lock()
	defer s.mu.Unlock()
	assignments := s.recallAssignments(saved.Sessions, accounts, now)
	ownPins := s.recallOwnPins(saved.SessionPins, accounts)
	pins, gone := s.recallPins(saved, accounts, t)
	return assignments || ownPins || pins, gone
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

// recallPins takes in each pool's pin saved, but for the accounts each names
// no longer configured, the pin going with the last of them, as recalledPin
// says, fitted to the pools as t has them, as fitted says: the one pin of a
// state file from before pools settled as its home pool's, each pin left
// naming its own pool's accounts alone, and the pin of a pool t doesn't have
// gone. It reports whether that changed any, and returns each pool's pin
// dropped whole, by recalledPin or the fit, as the live fit tells them: none
// of a config from before pools, whose one pin going with its last account
// was never told. s.mu must be held.
func (s *sessions) recallPins(saved savedSessions, accounts accounts, t *poolTable) (changed bool, gone []gonePin) {
	everyPin := saved.everyPin()
	kept := make(pins)
	for _, pool := range slices.Sorted(maps.Keys(everyPin)) {
		pin, left := recalledPin(everyPin[pool], accounts)
		changed = changed || left
		switch {
		case !pin.IsZero():
			kept[pool] = pin
		case pool != "":
			gone = append(gone, gonePin{pool: pool, pin: everyPin[pool]})
		}
	}
	fitted, refit, refitted := kept.fitted(t)
	s.pins = fitted
	return changed || refitted, append(gone, refit...)
}

// recalledPin returns the pin saved but for its accounts no longer
// configured, of the accounts given, which it logs, the rest in the order
// configured, and reports whether it left any out.
func recalledPin(saved status.Pin, accounts accounts) (status.Pin, bool) {
	kept := accounts.only(saved.Accounts).configured().IDs()
	left := except(saved.Accounts, kept)
	for _, id := range left {
		logger.Warn("dropped from the pin: the account is no longer configured", "account", id)
	}
	saved.Accounts = kept
	return saved, len(left) > 0
}
