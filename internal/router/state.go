package router

import (
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// refusedFor is how long an account has no room for the requests the
	// upstream refused on it, whatever its windows say.
	refusedFor = 10 * time.Minute
	// limitedFor is how long an account under a limit has no room for the
	// requests it holds back, when the upstream doesn't say.
	limitedFor = 5 * time.Minute
)

// usage is what the router knows of one account's usage.
type usage struct {
	// windows holds the latest reading of each window, by key.
	windows map[string]quota.Window
	// updated is when a reading last came in.
	updated time.Time
	// probed is when a probe of the account last ended, whether it read
	// anything or not.
	probed time.Time
	// probeErr says why the last probe read nothing, until a reading comes in.
	probeErr string
	// failures are the windows the last probe expected and couldn't read,
	// each until it's read.
	failures []quota.Failure
	// refused is the upstream's last refusal of the account's token, which
	// holds back every request.
	refused refusal
	// forbidden holds, by model family, the upstream's last refusal of a
	// request of the family on the account, its token standing, which holds
	// back the family's requests alone.
	forbidden map[string]refusal
	// limited is the limit the account last reached.
	limited limit
}

// refusal is the upstream refusing requests on an account, answering with
// status, at a time: the account has no room for them for refusedFor after.
type refusal struct {
	at     time.Time
	status int
}

// inForce reports whether the refusal is in force at now.
func (r refusal) inForce(now time.Time) bool {
	return !r.at.IsZero() && now.Sub(r.at) < refusedFor
}

// report is the refusal as the status document gives it, holding back the
// requests of family, or every request when that's "".
func (r refusal) report(family string) status.Refusal {
	return status.Refusal{Until: r.at.Add(refusedFor), Status: r.status, Family: family}
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
// which windows count: it's in force, and it was reached in a window that
// counts the request, or in none named.
func (l limit) holds(now time.Time, applies func(key string) bool) bool {
	return l.inForce(now) && (len(l.windows) == 0 || slices.ContainsFunc(l.windows, applies))
}

// inForce reports whether the limit is in force at now: it hasn't lifted.
func (l limit) inForce(now time.Time) bool {
	return now.Before(l.until)
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
		s.usage[a.ID] = &usage{windows: make(map[string]quota.Window), forbidden: make(map[string]refusal)}
	}
	return s
}

// record takes in a reading of an account's windows. Each window is merged
// with the reading of its key before it, and the account's other windows
// stand.
func (s *state) record(id string, windows []quota.Window) {
	at := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage[id].take(windows, at)
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
	u.take(probed.Windows, at)
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

// refuse notes that the upstream refused the account's token, answering with
// status: the account has no room for refusedFor.
func (s *state) refuse(id string, status int) {
	r := refusal{at: s.now().UTC(), status: status}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage[id].refused = r
}

// forbid notes that the upstream refused the account a request of a model of
// family, answering with status, though not its token: the account has no
// room for the family's requests for refusedFor.
func (s *state) forbid(id, family string, status int) {
	r := refusal{at: s.now().UTC(), status: status}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage[id].forbidden[family] = r
}

// limit notes that the account reached its limit, in the windows named, if
// any, until until, or limitedFor from now when that isn't to come, and
// returns the news of it. Reached while the account's last limit is in force,
// it's that limit reached again, which now holds as this answer says, the
// upstream's latest word: one that doesn't say until when extends it. A
// reading showing it lifted lifts it sooner.
func (s *state) limit(id string, windows []string, until time.Time) LimitReached {
	now := s.now().UTC()
	if !until.After(now) {
		until = now.Add(limitedFor)
	}
	reached := limit{windows: slices.Clone(windows), until: until.UTC()}
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.usage[id]
	again := u.limited.inForce(now)
	u.limited = reached
	return LimitReached{Account: id, Windows: slices.Clone(windows), Until: reached.until, Again: again}
}

// due reports whether an account's usage wants probing at now, before a
// choice: nothing has been read of it for staleAfter, as olderThan says.
func (s *state) due(id string, now time.Time) bool {
	return s.olderThan(staleAfter)(id, now)
}

// olderThan returns what reports whether an account's usage wants probing at
// now: nothing has been read of it for longer than age, and no probe of it
// has ended in the last reprobeAfter, so an account whose probes fail isn't
// probed at every ask.
func (s *state) olderThan(age time.Duration) func(id string, now time.Time) bool {
	return func(id string, now time.Time) bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		u := s.usage[id]
		return now.Sub(u.updated) > age && now.Sub(u.probed) >= reprobeAfter
	}
}

// dueAgain reports whether an account whose usage leaves it no room wants
// probing again at now, in case a window has reset unseen: nothing has been
// read of it, nor has a probe of it ended, in the last reprobeAfter.
func (s *state) dueAgain(id string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.usage[id]
	return now.Sub(u.updated) >= reprobeAfter && now.Sub(u.probed) >= reprobeAfter
}

// view returns what a choice of account for a request of model knows at now:
// every account with a token, as last read, which windows count the request,
// which accounts have no room for it whatever their windows read, and which
// of those refused it lately.
func (s *state) view(model string, now time.Time) view {
	s.mu.Lock()
	defer s.mu.Unlock()
	family, applies := s.family(model), s.counting(model)
	var (
		candidates      []score.Candidate
		barred, refused []string
	)
	for _, a := range s.accounts {
		if !a.hasToken() {
			continue
		}
		u := s.usage[a.ID]
		candidates = append(candidates, score.Candidate{ID: a.ID, Windows: u.latest()})
		if u.barred(now, family, applies) {
			barred = append(barred, a.ID)
		}
		if u.refuses(now, family) {
			refused = append(refused, a.ID)
		}
	}
	return view{policy: s.policy, now: now, candidates: candidates, applies: applies, barred: barred, refused: refused}
}

// barred reports whether the account has no room at now, whatever its windows
// read, for a request of a model of family, applies saying which windows count
// it: a refusal in force holds it back, or a limit the account reached does.
func (u *usage) barred(now time.Time, family string, applies func(key string) bool) bool {
	return u.refuses(now, family) || u.limited.holds(now, applies)
}

// refuses reports whether a refusal in force at now holds back a request of a
// model of family: the account's token's, or the family's.
func (u *usage) refuses(now time.Time, family string) bool {
	return u.refused.inForce(now) || u.forbidden[family].inForce(now)
}

// shut reports whether the account has no room at now for a request of any
// model, whatever its windows read: its token's refusal is in force, or a
// limit it reached holds back the requests of every model, which shared says
// the windows of.
func (u *usage) shut(now time.Time, shared func(key string) bool) bool {
	return u.refused.inForce(now) || u.limited.holds(now, shared)
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
// key before it, as mergeLater merges them, and lifts the account's limit
// when the windows merged show it lifted. Windows that are all stale leave
// the account as it was.
func (u *usage) take(windows []quota.Window, at time.Time) {
	var merged []quota.Window
	for _, w := range windows {
		kept, current := mergeLater(u.windows[w.Key], w)
		if !current {
			continue
		}
		u.windows[w.Key] = kept
		u.failures = slices.DeleteFunc(u.failures, func(f quota.Failure) bool { return f.Window == w.Key })
		merged = append(merged, kept)
	}
	if len(merged) == 0 {
		return
	}
	u.updated, u.probeErr = at, ""
	if u.limited.liftedBy(merged, at) {
		u.limited = limit{}
	}
}

// mergeLater returns what to keep of a window, given held, as it was read
// before, and w, a reading of it taken later, and reports whether w is
// current. Unlike quota.MergeMax, which merges readings taken together, it
// can't just keep the higher use: the window may have reset in between, so
// the reset decides. A later reset is a new window, however little used. The
// same reset is the same window, whose use only rises, so a reading as high
// stands, and a lower one is from before held: it leaves held's higher use, so
// a slow response reporting it late can't pull it back, and it can't lift a
// rejection either reading holds. An earlier reset is a window that's gone,
// and w is stale. Without a reset to go by, the newest reading stands.
func mergeLater(held, w quota.Window) (quota.Window, bool) {
	switch {
	case w.ResetsAt.IsZero() || w.ResetsAt.After(held.ResetsAt):
		return w, true
	case w.ResetsAt.Before(held.ResetsAt):
		return held, false
	case w.Utilization < held.Utilization:
		if w.Status == quota.StatusRejected {
			held.Status = w.Status
		}
		return held, true
	default:
		return w, true
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
		if !u.shut(now, s.policy.IsShared) {
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
		if a.hasToken() {
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
		refused: u.refused.inForce(now),
	}
}

// status is the account's usage as last read, or why there's none, and the
// limit and the refusal in force on it at now, if any are.
func (u *usage) status(a account, now time.Time) status.Account {
	st := status.Account{ID: a.ID, Label: a.Label, TokenSet: a.hasToken()}
	if !a.hasToken() {
		st.Error = a.problem
		return st
	}
	st.FetchedAt = u.updated
	st.Windows = u.latest()
	st.Failures = slices.Clone(u.failures)
	st.Error = u.probeErr
	if u.limited.inForce(now) {
		st.Limit = status.Limit{Windows: slices.Clone(u.limited.windows), Until: u.limited.until}
	}
	st.Refused = u.refusedStatus(now)
	return st
}

// refusedStatus is the refusal the status document gives the account at now,
// zero when none is in force: its token's, which holds back every request,
// else the latest of those holding back a family's requests alone.
func (u *usage) refusedStatus(now time.Time) status.Refusal {
	if u.refused.inForce(now) {
		return u.refused.report("")
	}
	var latest status.Refusal
	for _, family := range slices.Sorted(maps.Keys(u.forbidden)) {
		if r := u.forbidden[family]; r.inForce(now) && r.at.Add(refusedFor).After(latest.Until) {
			latest = r.report(family)
		}
	}
	return latest
}

// latest returns the latest reading of each window, in quota.Sort's order, or
// nil when there's none.
func (u *usage) latest() []quota.Window {
	if len(u.windows) == 0 {
		return nil
	}
	windows := slices.Collect(maps.Values(u.windows))
	quota.Sort(windows)
	return windows
}
