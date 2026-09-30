package router

import (
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/prime"
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

// moment is a point in the order of the requests the router sends and the
// readings it takes in, as the router counts them: of two, the greater came
// later, for certain, where a wall clock can be set back between them. Zero
// comes before them all, as the readings the state file kept do.
type moment uint64

// usage is what the router knows of one account's usage.
type usage struct {
	// windows holds the latest reading of each window, by key, and taken the
	// moment each reading held was taken in.
	windows map[string]quota.Window
	taken   map[string]moment
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
	// reserved are the keys of the windows last found to have reached the
	// account's reserve.
	reserved []string
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
// reached, if any, when the account is to have room again, and the moment it
// was set. It holds back the requests those windows count, or every request
// when they're none, whatever the account's windows read: a rejection they
// don't show, or that a reading from before it outweighs, holds as well as
// one they do.
type limit struct {
	windows []string
	until   time.Time
	set     moment
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

// liftedBy reports whether windows, read at a time off the answer to a request
// sent at sent, show the limit lifted: the request was sent after the limit
// was set, as the answer to one sent before may show room there was before
// it, and each window that reached it read again, with room. A limit
// reached in no window named can't be seen to lift in the windows: it lifts
// in time, or once a request sent after it is answered with success, as
// admits says.
func (l limit) liftedBy(windows []quota.Window, at time.Time, sent moment) bool {
	if len(l.windows) == 0 || sent <= l.set {
		return false
	}
	for _, key := range l.windows {
		i := slices.IndexFunc(windows, func(w quota.Window) bool { return w.Key == key })
		if i < 0 || !score.Available(windows[i:i+1], 0, func(string) bool { return true }, at) {
			return false
		}
	}
	return true
}

// admits reports whether a request sent at sent, answered with success on
// the account, shows the limit lifted: the limit was reached in no window
// named, which holds back every request, and set before the request was sent.
func (l limit) admits(sent moment) bool {
	return len(l.windows) == 0 && sent > l.set
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
	// changed hears of each change to what the state file keeps of the
	// accounts' usage, and readOff of each that's only a reading off the
	// answer to a request, which it keeps less often, both with s.mu held:
	// they mustn't block, nor call s.
	changed, readOff func()
	// moments counts the moments marked.
	moments atomic.Uint64

	mu    sync.Mutex
	usage map[string]*usage
	// seen holds, by window key, the model families whose requests a window
	// has been reported on.
	seen map[string]map[string]bool
}

func newState(accounts accounts, policy score.Policy, family func(string) string, now func() time.Time, changed, readOff func()) *state {
	s := &state{
		accounts: accounts,
		policy:   policy,
		family:   family,
		now:      now,
		changed:  changed,
		readOff:  readOff,
		usage:    make(map[string]*usage, len(accounts)),
		seen:     make(map[string]map[string]bool),
	}
	for _, a := range accounts {
		s.usage[a.ID] = &usage{windows: make(map[string]quota.Window), taken: make(map[string]moment), forbidden: make(map[string]refusal)}
	}
	return s
}

// mark returns a new moment, after every moment marked before it: the one a
// request goes upstream at, which the reading its answer gives is judged by.
func (s *state) mark() moment {
	return moment(s.moments.Add(1))
}

// record takes in a reading of an account's windows, off the answer to a
// request sent at sent. Each window is merged with the reading of its key
// before it, and the account's other windows stand.
func (s *state) record(id string, windows []quota.Window, sent moment) {
	at := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.usage[id].take(windows, at, sent, s.mark()) {
		s.readOff()
	}
}

// admitted notes that the upstream answered a request on the account, sent
// at sent, with success, which lifts the account's limit as admits says.
func (s *state) admitted(id string, sent moment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage[id].admitted(sent)
}

// recordProbe takes in what probing an account, from sent on, found: its
// usage and which models reported each window, and whether a request of it
// was answered with success, or why it read nothing.
func (s *state) recordProbe(id string, probed quota.Probe, err error, sent moment) {
	at := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.usage[id]
	u.probed = at
	if err != nil {
		u.probeErr = err.Error()
		return
	}
	if probed.Admitted {
		u.admitted(sent)
	}
	u.failures = slices.Clone(probed.Failures)
	news := u.take(probed.Windows, at, sent, s.mark())
	for key, models := range probed.Models {
		for _, model := range models {
			news = s.see(key, model) || news
		}
	}
	if news {
		s.changed()
	}
}

// learn notes that the response to a request for model reported windows:
// each counts requests of the model's family.
func (s *state) learn(model string, windows []quota.Window) {
	s.mu.Lock()
	defer s.mu.Unlock()
	news := false
	for _, w := range windows {
		news = s.see(w.Key, model) || news
	}
	if news {
		s.changed()
	}
}

// see notes that the window named key counts requests of model's family, and
// reports whether that's news. A model that isn't known says nothing. s.mu
// must be held.
func (s *state) see(key, model string) bool {
	if model == "" {
		return false
	}
	return s.seeFamily(key, s.family(model))
}

// seeFamily notes that the window named key counts requests of family, and
// reports whether that's news. s.mu must be held.
func (s *state) seeFamily(key, family string) bool {
	if s.seen[key][family] {
		return false
	}
	if s.seen[key] == nil {
		s.seen[key] = make(map[string]bool)
	}
	s.seen[key][family] = true
	return true
}

// refuse notes that the upstream refused the account's token, answering with
// status: the account has no room for refusedFor.
func (s *state) refuse(id string, status int) {
	r := refusal{at: s.now().UTC(), status: status}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage[id].refused = r
}

// tokenReplaced notes that the account with the given id goes out on another
// token from now on: the upstream's refusal of the one before no longer holds
// it back.
func (s *state) tokenReplaced(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage[id].refused = refusal{}
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
// upstream's latest word: one that doesn't say until when extends it. The
// answer to a request sent after it, showing it lifted, lifts it sooner.
func (s *state) limit(id string, windows []string, until time.Time) LimitReached {
	now := s.now().UTC()
	if !until.After(now) {
		until = now.Add(limitedFor)
	}
	reached := limit{windows: slices.Clone(windows), until: until.UTC(), set: s.mark()}
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.usage[id]
	again := u.limited.inForce(now)
	u.limited = reached
	return LimitReached{Account: id, Windows: slices.Clone(windows), Until: reached.until, Again: again}
}

// unread reports whether nothing has been read of an account, which wants
// probing as the router starts: one read before, whose reading the state file
// kept, wants none.
func (s *state) unread(id string, _ time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage[id].updated.IsZero()
}

// due reports whether an account's usage wants probing at now, before a
// choice: nothing has been read of it for staleAfter, as olderThan says.
func (s *state) due(id string, now time.Time) bool {
	return s.olderThan(staleAfter)(id, now)
}

// olderThan returns what reports whether an account's usage wants probing at
// now: nothing has been read of it for longer than age, and it can be
// probed, as probeable says.
func (s *state) olderThan(age time.Duration) func(id string, now time.Time) bool {
	return func(id string, now time.Time) bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		u := s.usage[id]
		return now.Sub(u.updated) > age && s.probeable(u, now)
	}
}

// dueAgain reports whether an account whose usage leaves it no room wants
// probing again at now, in case a window has reset unseen: nothing has been
// read of it in the last reprobeAfter, and it can be probed, as probeable
// says.
func (s *state) dueAgain(id string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.usage[id]
	return now.Sub(u.updated) >= reprobeAfter && s.probeable(u, now)
}

// probeable reports whether the account whose usage is u can be probed at
// now: no probe of it has ended in the last reprobeAfter, so an account whose
// probes fail isn't probed at every ask, and none of its windows has lapsed,
// as a probe is a request, and would start it. An account that's spent, as
// spent says, is the exception: it can take no request anyway, so a probe
// that starts its window costs nothing, and a probe is how a limit lifted
// before its reset, as by a reset made by hand, is seen. s.mu must be held.
func (s *state) probeable(u *usage, now time.Time) bool {
	return now.Sub(u.probed) >= reprobeAfter && (u.spent(s.policy, now) || len(s.policy.Lapsed(u.latest(), now)) == 0)
}

// nextPrime returns when the account with the given id is next to be primed,
// at now or after, as primeAt says. It reports false when the schedule can't
// say, and while the account's token is refused, or it's spent, as spent
// says: a prime couldn't start its window.
func (s *state) nextPrime(id string, schedule prime.Schedule, now time.Time) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.usage[id]
	if freed, ok := u.freed(s.policy, now); !ok || freed.After(now) {
		return time.Time{}, false
	}
	return u.primeAt(id, schedule, s.policy, now)
}

// nextLook returns when the primer is next to look at whether the account
// with the given id is due a prime: when it's next to be primed, as nextPrime
// says, or, while its token is refused or it's spent, when it could next be,
// once neither holds it back, as freed says, so it's primed on time once it
// can start a window. It reports false when neither can say.
func (s *state) nextLook(id string, schedule prime.Schedule, now time.Time) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.usage[id]
	freed, ok := u.freed(s.policy, now)
	if !ok {
		return time.Time{}, false
	}
	return u.primeAt(id, schedule, s.policy, freed.In(now.Location()))
}

// primeAt returns when the account with the given id is next to be primed, at
// from or after: when schedule says of its windows as last read, but not
// before reprobeAfter has passed since a probe of it last ended, or
// reprimeAfter when that one failed as a prime, as primeFailed says. It
// reports false when the schedule can't say.
func (u *usage) primeAt(id string, schedule prime.Schedule, policy score.Policy, from time.Time) (time.Time, bool) {
	at, ok := schedule.Next(id, u.latest(), from)
	if !ok {
		return time.Time{}, false
	}
	retry := reprobeAfter
	if u.primeFailed(policy) {
		retry = reprimeAfter
	}
	return later(at, u.probed.Add(retry)), true
}

// freed returns when, at now or after, nothing holds back a prime of the
// account: its token's refusal has ended, and it's spent no longer, as spent
// says, a limit holding back its every request having ended, and each spent
// window every model shares having reset. It reports false when one of those
// windows' reset isn't known.
func (u *usage) freed(policy score.Policy, now time.Time) (time.Time, bool) {
	at := now
	if u.refused.inForce(now) {
		at = later(at, u.refused.at.Add(refusedFor))
	}
	if u.limited.holds(now, policy.IsShared) {
		at = later(at, u.limited.until)
	}
	windows := u.current(policy, now)
	for i, w := range windows {
		switch {
		case score.Available(windows[i:i+1], 0, policy.IsShared, now):
		case w.ResetsAt.IsZero():
			return time.Time{}, false
		default:
			at = later(at, w.ResetsAt)
		}
	}
	return at, true
}

// later returns the later of two times.
func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// primeFailed reports whether the last probe of the account with the given id
// failed as a prime, as its usage's primeFailed says.
func (s *state) primeFailed(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage[id].primeFailed(s.policy)
}

// primeFailed reports whether the account's last probe failed as a prime: it
// read nothing, or the window a request starts, as policy names it, wasn't
// running as it ended, by what has been read, so the prime didn't start it.
func (u *usage) primeFailed(policy score.Policy) bool {
	windows := u.latest()
	return u.probeErr != "" || len(windows) == 0 || len(policy.Lapsed(windows, u.probed)) > 0
}

// view returns what a choice of account for a request of model knows at now:
// every account with a token, as it stands, with its reserve, which windows
// count the request, which accounts have no room for it whatever their windows
// read, and which of those refused it lately. It notes in the log how the
// accounts' reserves hold them back, as noteReserves says.
func (s *state) view(model string, now time.Time) view {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noteReserves(now)
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
		candidates = append(candidates, score.Candidate{ID: a.ID, Windows: u.current(s.policy, now), Reserve: a.Reserve})
		if u.barred(now, family, applies) {
			barred = append(barred, a.ID)
		}
		if u.refuses(now, family) {
			refused = append(refused, a.ID)
		}
	}
	return view{policy: s.policy, now: now, candidates: candidates, applies: applies, barred: barred, refused: refused}
}

// noteReserves notes in the log, as it finds the accounts at now, each window
// that has reached its account's reserve since they were last looked at,
// which holds the account back from the router's own choices, and each that
// has reset since, letting it go. The router looks at them afresh for every
// choice, so the log tells of a reserve before any choice it holds back, and
// of its letting go before any choice it doesn't. s.mu must be held.
func (s *state) noteReserves(now time.Time) {
	for _, a := range s.accounts {
		if a.hasToken() {
			s.usage[a.ID].noteReserve(a, s.policy, now)
		}
	}
}

// noteReserve notes in the log the account's windows that have reached its
// reserve at now since it was last looked at, as they stand as policy judges,
// and those that have reset since.
func (u *usage) noteReserve(a account, policy score.Policy, now time.Time) {
	reserved := score.AtReserve(u.current(policy, now), a.Reserve, now)
	if held := except(reserved, u.reserved); len(held) > 0 {
		logger.Info("held back by its reserve", "account", a.ID, "windows", strings.Join(held, ","), "reserve", a.Reserve)
	}
	if freed := except(u.reserved, reserved); len(freed) > 0 {
		logger.Info("let go at its reset", "account", a.ID, "windows", strings.Join(freed, ","))
	}
	u.reserved = reserved
}

// except returns the keys that aren't among others, in order.
func except(keys, others []string) []string {
	return slices.DeleteFunc(slices.Clone(keys), func(key string) bool { return slices.Contains(others, key) })
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

// spent reports whether the account's quota leaves it no room at now for a
// request of any model, as policy judges: a limit it reached holds back every
// request, or a window every model shares is spent, as last read, and hasn't
// reset since.
func (u *usage) spent(policy score.Policy, now time.Time) bool {
	windows := u.current(policy, now)
	return u.limited.holds(now, policy.IsShared) || len(windows) > 0 && !score.Available(windows, 0, policy.IsShared, now)
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

// take takes in windows read at a time off the answer to a request sent at
// sent, and taken in at the moment taken, each merged with the reading of its
// key before it, as mergeLater merges them, and lifts the account's limit
// when the windows merged show it lifted. It reports whether it took any:
// windows that are all stale leave the account as it was.
func (u *usage) take(windows []quota.Window, at time.Time, sent, taken moment) bool {
	var merged []quota.Window
	for _, w := range windows {
		kept, outcome := mergeLater(u.windows[w.Key], w, sent > u.taken[w.Key])
		if outcome == stale {
			continue
		}
		if outcome == counted {
			u.windows[w.Key], u.taken[w.Key] = kept, taken
		}
		u.failures = slices.DeleteFunc(u.failures, func(f quota.Failure) bool { return f.Window == w.Key })
		merged = append(merged, kept)
	}
	if len(merged) == 0 {
		return false
	}
	u.updated, u.probeErr = at, ""
	if u.limited.liftedBy(merged, at, sent) {
		u.limited = limit{}
	}
	return true
}

// admitted lifts the account's limit when a request sent at sent, answered
// with success, shows it lifted, as admits says.
func (u *usage) admitted(sent moment) {
	if u.limited.admits(sent) {
		u.limited = limit{}
	}
}

// fate is what becomes of a reading of a window merged with the one held.
type fate int

const (
	// stale is a reading of a window that's gone, which changes nothing.
	stale fate = iota
	// outweighed is a reading older than the one held, which stands as it
	// was.
	outweighed
	// counted is a reading the window now stands as, as far as it goes: what
	// the window is known to be was read no earlier than its request.
	counted
)

// mergeLater returns what to keep of a window, given held, as it was read
// before, and w, a reading of it taken later, and says what becomes of w.
// after says whether w's request was sent after held was taken in: then w
// counts, however it reads, as the upstream reckons use as it takes a request
// in, which it can only have done after held's, so a reset made by hand,
// which drops use but may keep the reset, is seen. The answer to a request
// sent before may have been overtaken, and unlike quota.MergeMax, which
// merges readings taken together, mergeLater can't just keep the higher use:
// the window may have reset in between, so the reset decides. A later reset
// is a new window, however little used. The same reset is the same window,
// whose use only rises, so a reading as high counts, and a lower one is from
// before held: it's outweighed, leaving held's higher use, so a slow response
// reporting it late can't pull it back. Either way, a rejection either
// reading holds stands, as the later may be the older. An earlier reset is a
// window that's gone, and w is stale. Without a reset to go by, the newest
// reading counts.
func mergeLater(held, w quota.Window, after bool) (quota.Window, fate) {
	switch {
	case after || w.ResetsAt.IsZero() || w.ResetsAt.After(held.ResetsAt):
		return w, counted
	case w.ResetsAt.Before(held.ResetsAt):
		return held, stale
	case w.Utilization >= held.Utilization:
		if held.Status == quota.StatusRejected {
			w.Status = held.Status
		}
		return w, counted
	case w.Status == quota.StatusRejected && held.Status != quota.StatusRejected:
		held.Status = w.Status
		return held, counted
	default:
		return held, outweighed
	}
}

// document reports every account's usage as the router knows it, in the
// order configured, with the best account to use next, never one with no
// room for any request, whatever its windows read, and the primary.
func (s *state) document() status.Document {
	now := s.now()
	accounts, open := s.statuses(now)
	return status.Document{
		GeneratedAt: now.UTC(),
		Source:      status.SourceRouter,
		Best:        status.Best(s.policy, open, now),
		Primary:     status.PrimaryOf(accounts),
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
		all[i] = u.status(a, s.policy, now)
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
// window every model shares is spent, or has reached the account's reserve,
// as last read, and hasn't reset since.
func (u *usage) standing(a account, policy score.Policy, now time.Time) standing {
	limited := u.limited.holds(now, policy.IsShared)
	st := u.status(a, policy, now)
	return standing{
		Account: st,
		quota:   !limited && score.Available(st.Windows, a.Reserve, policy.IsShared, now),
		known:   limited || len(st.Windows) > 0,
		refused: u.refused.inForce(now),
	}
}

// status is the account as configured, with its usage as last read, standing
// as it does at now as policy judges, or why there's none, and the limit and
// the refusal in force on it then, if any are.
func (u *usage) status(a account, policy score.Policy, now time.Time) status.Account {
	st := status.Configured(a.Account)
	if !a.hasToken() {
		st.Error = a.problem()
		return st
	}
	st.TokenSet = true
	st.FetchedAt = u.updated
	st.Windows = u.latest()
	st = st.AsOf(policy, now)
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

// current returns the latest reading of each window as it stands at now, as
// policy judges, in quota.Sort's order: one that has lapsed reads empty.
func (u *usage) current(policy score.Policy, now time.Time) []quota.Window {
	return policy.AsOf(u.latest(), now)
}

// saved is what the state file keeps of the accounts' usage: each account's
// windows as last read, and when, and the model families each window has been
// reported on.
func (s *state) saved() savedUsage {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved := savedUsage{Readings: make(map[string]savedReading), WindowFamilies: make(map[string][]string)}
	for id, u := range s.usage {
		if len(u.windows) > 0 {
			saved.Readings[id] = savedReading{ReadAt: u.updated, Windows: u.latest()}
		}
	}
	for key, families := range s.seen {
		saved.WindowFamilies[key] = slices.Sorted(maps.Keys(families))
	}
	return saved
}

// recall takes in what the state file kept of the accounts' usage, as the
// router starts, but for the readings of accounts no longer configured, and
// reports whether it left any out.
func (s *state) recall(saved savedUsage) (dropped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, reading := range saved.Readings {
		u, configured := s.usage[id]
		if !configured {
			dropped = true
			continue
		}
		for _, w := range reading.Windows {
			u.windows[w.Key] = w
		}
		u.updated = reading.ReadAt.UTC()
	}
	for key, families := range saved.WindowFamilies {
		for _, family := range families {
			s.seeFamily(key, family)
		}
	}
	return dropped
}
