package ledger

import (
	"encoding/json"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

// SetLooked has a round call looked with the date of each day it summarises
// once it has looked at the day's files, before it reads them, for tests of
// what comes between the two.
func (l *Ledger) SetLooked(looked func(date string)) {
	l.looked = looked
}

// WriteDigest writes digest as the ledger in dir holds the digest of its
// day, its file alone, as one written by hand would be: its head, and its
// entries' parts, as given, the day's parts file left as it is, for tests of
// what passes over it.
func WriteDigest(dir string, digest Digest) error {
	data, err := gzippedJSON(storedOf(digest))
	if err != nil {
		return err
	}
	return atomicfile.Write(digestFile(dir, digest.Head.Day), data, 0o600)
}

// WriteParts writes the parts given, by their sessions' ids, as the ledger in
// dir holds the parts file of the local day with the given date, as a build
// writes it before its digest, the digest left as it is, for tests of a
// build that stopped between the two.
func WriteParts(dir, date string, parts map[string]json.RawMessage) error {
	built := make(map[string]builtPart, len(parts))
	for id, part := range parts {
		built[id] = builtPart{data: part}
	}
	file, _, err := partsFileOf(built)
	if err != nil {
		return err
	}
	return days{files: filesIn(dir, nil)}.writeParts(date, file)
}

// ReadDigest returns the digest the ledger in dir holds of the local day with
// the given date, its sessions' states of the version body, for tests of
// what's written.
func ReadDigest(dir, date string, body int) (Digest, error) {
	return days{files: filesIn(dir, nil)}.digest(date, body)
}

// ReadPart returns the part of a session ref points at in the parts file the
// ledger in dir holds of the local day with the given date, as readPart
// reads it, for tests of what's written.
func ReadPart(dir, date string, ref PartRef) (json.RawMessage, error) {
	return days{files: filesIn(dir, nil)}.readPart(date, ref)
}

// SessionsAsOf returns the sessions with the given ids as of the start of
// the local day with the given date, as the follower's lookup finds them,
// handing take each line read, where it's given, for tests of the lookup.
func (f *Follower) SessionsAsOf(ids []string, date string, take func(date string, l Line)) map[string]SessionFrom {
	read := newSessionsRead(f.now(), take)
	return f.lookup().asOf(f.datesBefore(date, read.now), nil, wanting(make(map[string]bool), ids), nil, read)
}

// SessionsAsOf returns the sessions with the given ids as of the start of
// the local day with the given date, at now, as the router's digester's
// lookup finds them, through the look its passes take at the days, held
// from one to the next, their states of the version its Digester gives, for
// tests of the lookup. The ledger must be opened Digesting.
func (l *Ledger) SessionsAsOf(ids []string, date string, now time.Time) map[string]SessionFrom {
	l.digesting.passing.Lock()
	defer l.digesting.passing.Unlock()
	first, err := l.days.first()
	view := l.digesting.view()
	return newSessionsLookup(l.days, view).asOf(l.days.datesBefore(date, first, err, now), nil, wanting(make(map[string]bool), ids), nil, newSessionsRead(now, nil))
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
// date, as the router's digester's look gives them to the lookup, for tests
// of its contract. The ledger must be opened Digesting.
func (l *Ledger) Kept(date string) (int, bool) {
	l.digesting.passing.Lock()
	defer l.digesting.passing.Unlock()
	return l.digesting.view().kept(date)
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
