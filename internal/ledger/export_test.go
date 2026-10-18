package ledger

import (
	"maps"
	"slices"
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
