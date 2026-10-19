package ledger

import (
	"cmp"
	"encoding/json"
	"maps"
	"os"
	"slices"
	"testing"
	"time"
)

// countingLook is a look at the ledger's days as the look it holds gives the
// digests' heads and the digests, noting the date of each digest read whole,
// its entries decoded, and keeping the lines of each day lines gives.
type countingLook struct {
	daysLook
	lines   map[string]keptDay
	decoded []string
}

// digest gives the digest the look held gives, noting its date as decoded.
func (l *countingLook) digest(date string) (Digest, bool) {
	l.decoded = append(l.decoded, date)
	return l.daysLook.digest(date)
}

// kept gives the lines the ledger keeps of the local day with the given
// date, as lines gives them.
func (l *countingLook) kept(date string) (int, bool) {
	return l.lines[date].lines, l.lines[date].known
}

// A lookup, the router's writer's and a reader's alike, decodes the entries
// of no digest whose head, as it holds it, says the digest doesn't stand: of
// another version of the envelope or of the sessions' states, as every one is
// after a release raising either; of another day's bounds; or against a
// summary of its day whose lines changed since. It rests the session on the
// newest digest naming it that does stand, decoding that one's alone.
func TestALookupDecodesNoDigestWhoseHeadDoesntStand(t *testing.T) {
	entry := func(restsOn map[string]int) map[string]DigestEntry {
		return map[string]DigestEntry{"work": {RestsOn: restsOn, State: json.RawMessage(`{}`)}}
	}
	older := Digest{Head: headOf(rested, 12), Sessions: entry(map[string]int{rested: 12})}
	newer := Digest{Head: headOf(digested, 40), Sessions: entry(map[string]int{rested: 12, digested: 40})}
	shifted := newer
	shifted.Head.Start, shifted.Head.End = newer.Head.Start.Add(-time.Hour), newer.Head.End.Add(-time.Hour)
	otherStates, olderStates := newer, older
	otherStates.Head.Body, olderStates.Head.Body = statesVersion+1, statesVersion+1
	envelope := storedOf(newer)
	envelope.Version = digestVersion + 1
	looks := map[string]func(d days) daysLook{
		"the writer's": func(d days) daysLook { return writerLook{days: d, body: statesVersion} },
		"a reader's": func(d days) daysLook {
			return followerLook{f: &Follower{days: d, body: statesVersion, heads: make(map[string]heldHead)}}
		},
	}
	tests := []struct {
		name string
		// older is the digest of the older day naming the session, nil for
		// one that stands; and newer the newer day's, written as storedOf
		// keeps it, nil where stored is written in its place.
		older  *Digest
		newer  *Digest
		stored *storedDigest
		kept   map[string]int
		// decoded are the dates of the digests read whole, newest first, and
		// base the date of the one the session rests on, "" for none.
		decoded []string
		base    string
	}{
		{name: "both standing", newer: &newer, decoded: []string{digested}, base: digested},
		{name: "the newer of another version of the states", newer: &otherStates, decoded: []string{rested}, base: rested},
		{name: "the newer of another version of the envelope", stored: &envelope, decoded: []string{rested}, base: rested},
		{name: "the newer of another day's bounds", newer: &shifted, decoded: []string{rested}, base: rested},
		{name: "the newer day's summary made again since, of more lines", newer: &newer, kept: map[string]int{digested: 41}, decoded: []string{rested}, base: rested},
		{name: "both of another version of the states", older: &olderStates, newer: &otherStates, base: ""},
		{name: "neither standing, each summary made again since", newer: &newer, kept: map[string]int{rested: 13, digested: 41}, base: ""},
	}
	for name, lookOf := range looks {
		for _, tt := range tests {
			t.Run(name+", "+tt.name, func(t *testing.T) {
				d := digestDays(t)
				if _, err := d.writeDigest(*cmp.Or(tt.older, &older), nil); err != nil {
					t.Fatal(err)
				}
				writeNewer(t, d, tt.newer, tt.stored)
				look := &countingLook{daysLook: lookOf(d), lines: keeping(withDayBeforeRested(tt.kept))}
				s := &sessionAsked{naming: []string{rested, digested}}

				newSessionsLookup(d, look).rest("work", s)

				base := ""
				if s.base != nil {
					base = s.base.Date
				}
				if !slices.Equal(look.decoded, tt.decoded) || base != tt.base {
					t.Errorf("rest decoded the digests of %q, resting the session on %q; want those of %q decoded, resting it on %q",
						look.decoded, base, tt.decoded, tt.base)
				}
			})
		}
	}
}

// withDayBeforeRested is kept, the lines the ledger keeps of the days it
// names, with none of the day before rested, so the older digest stands
// alone.
func withDayBeforeRested(kept map[string]int) map[string]int {
	over := map[string]int{"2026-10-01": 0}
	maps.Copy(over, kept)
	return over
}

// writeNewer writes the newer digest of a lookup's tests: digest, as
// writeDigest keeps it, or stored, gzipped, where digest is nil.
func writeNewer(t *testing.T, d days, digest *Digest, stored *storedDigest) {
	t.Helper()
	if digest != nil {
		if _, err := d.writeDigest(*digest, nil); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := gzippedJSON(*stored)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(digestFile(d.files.Dir, stored.Day), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
