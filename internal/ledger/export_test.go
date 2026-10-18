package ledger

import (
	"maps"
	"slices"
	"time"
)

// SetLooked has a round call looked with the date of each day it summarises
// once it has looked at the day's files, before it reads them, for tests of
// what comes between the two.
func (l *Ledger) SetLooked(looked func(date string)) {
	l.looked = looked
}

// WriteDigest writes digest as the ledger in dir holds the digest of its
// day, for tests of what passes over it.
func WriteDigest(dir string, digest Digest) error {
	return days{files: filesIn(dir, nil)}.writeDigest(digest)
}

// SessionsAsOf returns the sessions with the given ids as of the start of
// the local day with the given date, as the follower's lookup finds them,
// handing take each line read, where it's given, for tests of the lookup.
func (f *Follower) SessionsAsOf(ids []string, date string, take func(date string, l Line)) map[string]SessionFrom {
	return f.sessionsAsOf(ids, date, take)
}

// SessionsAsOf returns the sessions with the given ids as of the start of
// the local day with the given date, at now, their states of the version
// body, as the router's writer's lookup finds them, for tests of the lookup.
func (l *Ledger) SessionsAsOf(ids []string, date string, body int, now time.Time) map[string]SessionFrom {
	return l.sessionsAsOf(ids, date, body, now)
}

// RestsOn returns what a state folded from s rests on, as the lookup found
// it, for tests of the lookup.
func (s SessionFrom) RestsOn() map[string]int {
	return s.restsOn
}

// Kept gives the lines the ledger keeps of the local day with the given
// date, as the follower's look gives them to the lookup, for tests of its
// contract.
func (f *Follower) Kept(date string) (int, bool) {
	return followerLook{f: f}.kept(date)
}

// Kept gives the lines the ledger keeps of the local day with the given
// date, as the router's writer's look gives them to the lookup, whatever
// version of the states it reads, for tests of its contract.
func (l *Ledger) Kept(date string) (int, bool) {
	return newWriterLook(l.days, 0).kept(date)
}

// SetHistory has the reader list and read the readings history as history
// gives it, for tests of how it's asked.
func (r *Reader) SetHistory(history History) {
	r.days.history = history
}

// SetHistory has the follower list the readings history as history gives
// it, for tests of how it's asked.
func (f *Follower) SetHistory(history History) {
	f.days.history = history
}

// Followed returns the dates of the days the follower has looked at the
// files of, sorted, for tests of how far a read reaches.
func (f *Follower) Followed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Sorted(maps.Keys(f.followed))
}

// TodayHeld returns the requests of today's lines the follower holds, given
// and pending, for tests of what it lets go of.
func (f *Follower) TodayHeld() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var held []string
	for _, line := range slices.Concat(f.today.given, f.today.pending) {
		held = append(held, line.Request)
	}
	return held
}
