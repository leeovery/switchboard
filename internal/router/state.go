package router

import (
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// refusedFor is how long an account whose token the upstream refused has
	// no room, whatever its windows say.
	refusedFor = 10 * time.Minute
	// limitedFor is how long an account under a limit has no room for the
	// requests it holds back, when the upstream doesn't say.
	limitedFor = 5 * time.Minute
)

// origin is what a reading of an account's usage came from.
type origin string

const (
	fromResponse origin = "response"
	fromProbe    origin = "probe"
)

// reading is a window as it was last read, and when.
type reading struct {
	quota.Window
	at time.Time
}

// usage is what the router knows of one account's usage.
type usage struct {
	// windows holds the latest reading of each window, by key.
	windows map[string]reading
	// updated is when a reading last came in, and from is what it came from.
	updated time.Time
	from    origin
	// probed is when a probe of the account last ended, whether it read
	// anything or not.
	probed time.Time
	// probeErr says why the last probe read nothing, until a reading comes in.
	probeErr string
	// failures are the windows the last probe expected and couldn't read,
	// each until it's read.
	failures []quota.Failure
	// refused is when the upstream last refused the account's token.
	refused time.Time
	// limited is the limit the account last reached.
	limited limit
}

// limit is a limit an account reached: the windows the upstream named as
// reached, if any, and when the account is to have room again. It holds back
// the requests those windows count, or every request when they're none,
// whatever the account's windows read: a rejection they don't show, or that
// a reading from before it outweighs, holds as well as one they do.
type limit struct {
	windows []string
	until   time.Time
}

// holds reports whether the limit holds back, at now, a request applies says
// which windows count: it hasn't lifted, and it was reached in a window that
// counts the request, or in none named.
func (l limit) holds(now time.Time, applies func(key string) bool) bool {
	return now.Before(l.until) && (len(l.windows) == 0 || slices.ContainsFunc(l.windows, applies))
}

// liftedBy reports whether windows, read at a time, show the limit lifted:
// each window that reached it read again, with room. A limit reached in no
// window named can't be seen to lift, and lifts only in time.
func (l limit) liftedBy(windows []quota.Window, at time.Time) bool {
	if len(l.windows) == 0 {
		return false
	}
	for _, key := range l.windows {
		i := slices.IndexFunc(windows, func(w quota.Window) bool { return w.Key == key })
		if i < 0 || !score.Available(windows[i:i+1], func(string) bool { return true }, at) {
			return false
		}
	}
	return true
}

// state is what the router knows of every account's usage, learnt from the
// responses it forwards and from probes, and of which requests each window
// counts. It's safe for concurrent use.
type state struct {
	accounts accounts
	policy   score.Policy
	// family names the family of models a model belongs to.
	family func(model string) string
	now    func() time.Time

	mu    sync.Mutex
	usage map[string]*usage
	// seen holds, by window key, the model families whose requests a window
	// has been reported on.
	seen map[string]map[string]bool
}

func newState(accounts accounts, policy score.Policy, family func(string) string, now func() time.Time) *state {
	s := &state{
		accounts: accounts,
		policy:   policy,
		family:   family,
		now:      now,
		usage:    make(map[string]*usage, len(accounts)),
		seen:     make(map[string]map[string]bool),
	}
	for _, a := range accounts {
		s.usage[a.ID] = &usage{windows: make(map[string]reading)}
	}
	return s
}

// record takes in a reading of an account's windows. Each window is merged
// with the reading of its key before it, and the account's other windows
// stand.
func (s *state) record(id string, windows []quota.Window, from origin) {
	at := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage[id].take(windows, from, at)
}

// recordProbe takes in what probing an account found: its usage and which
// models reported each window, or why it read nothing.
func (s *state) recordProbe(id string, probed quota.Probe, err error) {
	at := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.usage[id]
	u.probed = at
	if err != nil {
		u.probeErr = err.Error()
		return
	}
	u.failures = slices.Clone(probed.Failures)
	u.take(probed.Windows, fromProbe, at)
	for key, models := range probed.Models {
		for _, model := range models {
			s.see(key, model)
		}
	}
}

// learn notes that the response to a request for model reported windows:
// each counts requests of the model's family.
func (s *state) learn(model string, windows []quota.Window) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range windows {
		s.see(w.Key, model)
	}
}

// see notes that the window named key counts requests of model's family. A
// model that isn't known says nothing.
func (s *state) see(key, model string) {
	if model == "" {
		return
	}
	if s.seen[key] == nil {
		s.seen[key] = make(map[string]bool)
	}
	s.seen[key][s.family(model)] = true
}

// refuse notes that the upstream refused the account's token: the account has
// no room for refusedFor.
func (s *state) refuse(id string) {
	at := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage[id].refused = at
}

// limit notes that the account reached its limit, in the windows named, if
// any, and returns until when it holds: until, or limitedFor from now when
// that isn't to come. A reading showing it lifted lifts it sooner.
func (s *state) limit(id string, windows []string, until time.Time) time.Time {
	now := s.now().UTC()
	if !until.After(now) {
		until = now.Add(limitedFor)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage[id].limited = limit{windows: slices.Clone(windows), until: until}
	return until
}

// due reports whether an account's usage wants probing at now, before a
// choice: nothing has been read of it for staleAfter, as olderThan says.
func (s *state) due(id string, now time.Time) bool {
	return s.olderThan(staleAfter)(id, now)
}

// olderThan returns what reports whether an account's usage wants probing at
// now: nothing has been read of it for longer than age, and no probe of it
// has ended in the last retryAfter, so an account whose probes fail isn't
// probed at every ask.
func (s *state) olderThan(age time.Duration) func(id string, now time.Time) bool {
	return func(id string, now time.Time) bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		u := s.usage[id]
		return now.Sub(u.updated) > age && now.Sub(u.probed) >= retryAfter
	}
}

// dueAgain reports whether an account whose usage leaves it no room wants
// probing again at now, in case a window has reset unseen: nothing has been
// read of it, nor has a probe of it ended, in the last retryAfter.
func (s *state) dueAgain(id string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.usage[id]
	return now.Sub(u.updated) >= retryAfter && now.Sub(u.probed) >= retryAfter
}

// view returns what a choice of account for a request of model knows at now:
// every account with a token, as last read, which windows count the request,
// and which accounts have no room for it whatever their windows read.
func (s *state) view(model string, now time.Time) view {
	s.mu.Lock()
	defer s.mu.Unlock()
	applies := s.counting(model)
	var (
		candidates []score.Candidate
		barred     []string
	)
	for _, a := range s.accounts {
		if !a.hasToken {
			continue
		}
		u := s.usage[a.ID]
		candidates = append(candidates, score.Candidate{ID: a.ID, Windows: u.latest()})
		if u.barred(now, applies) {
			barred = append(barred, a.ID)
		}
	}
	return view{policy: s.policy, now: now, candidates: candidates, applies: applies, barred: barred}
}

// barred reports whether the account has no room at now, whatever its windows
// read, for a request applies says which windows count: its token was refused
// too lately, or a limit it reached holds the request back.
func (u *usage) barred(now time.Time, applies func(key string) bool) bool {
	return u.refusedLately(now) || u.limited.holds(now, applies)
}

// refusedLately reports whether the account's token was refused too lately,
// at now, for anything to go out on it.
func (u *usage) refusedLately(now time.Time) bool {
	return !u.refused.IsZero() && now.Sub(u.refused) < refusedFor
}

// counting returns which windows count a request of model, by key: every
// window the policy shares between all models, and any other that has been
// seen on the model's family, or on none yet.
func (s *state) counting(model string) func(key string) bool {
	family := s.family(model)
	others := make(map[string]bool)
	for key, families := range s.seen {
		if !s.policy.IsShared(key) && !families[family] {
			others[key] = true
		}
	}
	return func(key string) bool { return !others[key] }
}

// take takes in windows read at a time, each merged with the reading of its
// key before it, and lifts the account's limit when they show it lifted.
// Windows that are all stale leave the account as it was.
func (u *usage) take(windows []quota.Window, from origin, at time.Time) {
	var read []quota.Window
	for _, w := range windows {
		kept, current := u.windows[w.Key].merge(w, at)
		if !current {
			continue
		}
		u.windows[w.Key] = kept
		u.failures = slices.DeleteFunc(u.failures, func(f quota.Failure) bool { return f.Window == w.Key })
		read = append(read, w)
	}
	if len(read) == 0 {
		return
	}
	u.updated, u.from, u.probeErr = at, from, ""
	if u.limited.liftedBy(read, at) {
		u.limited = limit{}
	}
}

// merge returns what to keep of a window, given r and a reading w of it at a
// time, and reports whether w is current. A later reset is a new window,
// however little used. The same reset is the same window, whose use only
// rises, so the higher reading stands, as of w's time: a slow response
// reporting it late can't pull it back. An earlier reset is a window that's
// gone, and w is stale. Without a reset to go by, the newest reading stands.
func (r reading) merge(w quota.Window, at time.Time) (reading, bool) {
	switch {
	case w.ResetsAt.IsZero() || w.ResetsAt.After(r.ResetsAt):
		return reading{Window: w, at: at}, true
	case w.ResetsAt.Before(r.ResetsAt):
		return r, false
	case w.Utilization < r.Utilization:
		return reading{Window: r.Window, at: at}, true
	default:
		return reading{Window: w, at: at}, true
	}
}

// document reports every account's usage as the router knows it, in the
// order configured, with the best account to use next: never one with no
// room for any request, whatever its windows read.
func (s *state) document() status.Document {
	now := s.now()
	accounts, open := s.statuses(now)
	return status.Document{
		GeneratedAt: now.UTC(),
		Source:      status.SourceRouter,
		Best:        status.Best(s.policy, open, now),
		Accounts:    accounts,
	}
}

// statuses returns every account's status at now, and, of those, the ones
// the best can be: all but those barred from the requests of every model.
func (s *state) statuses(now time.Time) (all, open []status.Account) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all = make([]status.Account, len(s.accounts))
	for i, a := range s.accounts {
		u := s.usage[a.ID]
		all[i] = u.status(a, now)
		if !u.barred(now, s.policy.IsShared) {
			open = append(open, all[i])
		}
	}
	return all, open
}

// standings returns how every account with a token stands at now, in the
// order configured.
func (s *state) standings(now time.Time) standings {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all standings
	for _, a := range s.accounts {
		if a.hasToken {
			all = append(all, s.usage[a.ID].standing(a, s.policy, now))
		}
	}
	return all
}

// standing is how the account stands at now. Its quota leaves it no room for
// a request of any model while a limit holds such requests back, or while a
// window every model shares is spent, as last read, and hasn't reset since.
func (u *usage) standing(a account, policy score.Policy, now time.Time) standing {
	limited := u.limited.holds(now, policy.IsShared)
	st := u.status(a, now)
	return standing{
		Account: st,
		quota:   !limited && score.Available(st.Windows, policy.IsShared, now),
		known:   limited || len(st.Windows) > 0,
		refused: u.refusedLately(now),
	}
}

// status is the account's usage as last read, or why there's none, and the
// limit that holds on it at now, if one does.
func (u *usage) status(a account, now time.Time) status.Account {
	st := status.Account{ID: a.ID, Label: a.Label, TokenSet: a.hasToken}
	if !a.hasToken {
		st.Error = status.TokenMissing(a.Account)
		return st
	}
	st.FetchedAt = u.updated
	st.Windows = u.latest()
	st.Failures = slices.Clone(u.failures)
	st.Error = u.probeErr
	if now.Before(u.limited.until) {
		st.Limit = status.Limit{Windows: slices.Clone(u.limited.windows), Until: u.limited.until}
	}
	return st
}

// latest returns the latest reading of each window, in quota.Sort's order, or
// nil when there's none.
func (u *usage) latest() []quota.Window {
	if len(u.windows) == 0 {
		return nil
	}
	windows := make([]quota.Window, 0, len(u.windows))
	for _, r := range u.windows {
		windows = append(windows, r.Window)
	}
	quota.Sort(windows)
	return windows
}
