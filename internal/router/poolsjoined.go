package router

import (
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
)

// joinedKept is how many of the pools an account joined its record keeps,
// the latest: enough for every move the days of the readings history seed
// takes up can hold. It's a count, not a time, as those days are the two
// newest with a file, which a quiet day or a router stopped for days can
// leave of any age.
const joinedKept = 4

// poolJoined is a pool an account joined, by the pool's id, "" being the one
// pool there is in a config from before pools, and when it joined it.
type poolJoined struct {
	Pool  string    `json:"pool"`
	Since time.Time `json:"since"`
}

// poolsJoined are the pools an account has been in, each as it joined it,
// oldest first, as the state file keeps them, so a router started again
// takes up each reading of the history in the pool its account was in then.
type poolsJoined []poolJoined

// at returns the id of the pool the account was in at at: the one it joined
// last by then, else, before it joined any, as far as is known, the first.
// It reports false where none is recorded.
func (js poolsJoined) at(at time.Time) (string, bool) {
	if len(js) == 0 {
		return "", false
	}
	i := len(js) - 1
	for i > 0 && at.Before(js[i].Since) {
		i--
	}
	return js[i].Pool, true
}

// joining returns js with the account in the pool with the given id from at
// on: as it is where it's already there, as config.SamePool compares them.
func (js poolsJoined) joining(pool string, at time.Time) poolsJoined {
	if len(js) > 0 && config.SamePool(js[len(js)-1].Pool, pool) {
		return js
	}
	return append(js, poolJoined{Pool: pool, Since: at})
}

// pruned returns the last joinedKept of js, the pools the account joined
// latest.
func (js poolsJoined) pruned() poolsJoined {
	return js[max(len(js)-joinedKept, 0):]
}

// joinPools has the record of the pools each account has been in, as the
// pools are grouped, take in at at each account in another pool than it was
// in last, or never recorded in one, as joining it then, and forget what
// it no longer needs, as pruned says. An account the pools put in none, as
// every account of a closed table, joins none. It reports whether the record
// changed. s.mu must be held.
func (s *state) joinPools(at time.Time) (changed bool) {
	for _, a := range s.accounts {
		was := s.joined[a.ID]
		js := was
		if pool, in := s.grouped.in(a.ID); in {
			js = js.joining(pool, at)
		}
		if kept := js.pruned(); len(js) != len(was) || len(kept) != len(js) {
			s.joined[a.ID], changed = kept, true
		}
	}
	return changed
}

// recallJoined takes in the record of the pools each account has been in, as
// the state file kept it, as the router starts, but for accounts no longer
// configured, each in the order its pools were joined, and has it take in the
// pools as they stand, at now, as joinPools does, so an account moved while
// the router was down joined its pool as the router starts. It reports
// whether the record taken in differs from the one kept, as one kept out of
// order does. s.mu must be held.
func (s *state) recallJoined(kept map[string]poolsJoined, now time.Time) (changed bool) {
	bySince := func(a, b poolJoined) int { return a.Since.Compare(b.Since) }
	for id, js := range kept {
		if _, configured := s.usage[id]; !configured {
			changed = true
			continue
		}
		if !slices.IsSortedFunc(js, bySince) {
			changed = true
		}
		s.joined[id] = slices.SortedStableFunc(slices.Values(js), bySince)
	}
	return s.joinPools(now) || changed
}

// savedJoined is the record of the pools each account has been in, by the
// account's id, as the state file keeps it.
func (s *state) savedJoined() map[string]poolsJoined {
	saved := maps.Clone(s.joined)
	for id, js := range saved {
		saved[id] = slices.Clone(js)
	}
	return saved
}

// inAt returns what gives the id of the pool each account was in at at, as
// seed takes up the history, of the pools as grouped: the one the record of
// the pools it has been in gives then, as it stands now, as standing says; or,
// where nothing's recorded, the one it's in now. It reports false where the
// account was in none that stands. s.mu must be held as what it returns is
// called.
func (s *state) inAt(at time.Time) placing {
	return func(id string) (string, bool) {
		pool, recorded := s.joined[id].at(at)
		if !recorded {
			return s.grouped.in(id)
		}
		return s.grouped.standing(pool)
	}
}

// joinedAt returns when each account joined a pool after its first, the
// moments a pool's accounts changed. s.mu must be held.
func (s *state) joinedAt() []time.Time {
	var at []time.Time
	for _, js := range s.joined {
		for _, j := range js[min(1, len(js)):] {
			at = append(at, j.Since)
		}
	}
	return at
}
