package ledger

import (
	"cmp"
	"encoding/json"
	"maps"
	"os"
	"reflect"
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
	for name, lookOf := range looks() {
		for _, tt := range tests {
			t.Run(name+", "+tt.name, func(t *testing.T) {
				d := digestDays(t)
				if _, err := d.writeDigest(*cmp.Or(tt.older, &older), nil); err != nil {
					t.Fatal(err)
				}
				writeNewer(t, d, tt.newer, tt.stored)
				look := &countingLook{daysLook: lookOf(d), lines: keeping(withDayBeforeRested(tt.kept))}
				s := &sessionAsked{naming: []string{rested, digested}}

				newSessionsLookup(d, look).rest("work", s, RestOnState)

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

// looks are the looks at the ledger's days d a lookup's tests look through,
// by whose they are: the router's writer's, and a reader's, of the sessions'
// states of statesVersion.
func looks() map[string]func(d days) daysLook {
	return map[string]func(d days) daysLook{
		"the writer's": func(d days) daysLook { return writerLook{days: d, body: statesVersion} },
		"a reader's": func(d days) daysLook {
			return followerLook{f: &Follower{days: d, body: statesVersion, heads: make(map[string]heldHead)}}
		},
	}
}

// pagedDigests writes the digests of the page's lookup tests: older, of
// rested, its session work folded on none; and the digest of digested, its
// session folded on rested's state; each with work's part beside it.
func pagedDigests(t *testing.T, d days, older Digest) {
	t.Helper()
	newer := Digest{Head: headOf(digested, 40), Sessions: map[string]DigestEntry{"work": {RestsOn: map[string]int{rested: 12, digested: 40}, State: json.RawMessage(`{}`)}}}
	for _, written := range []struct {
		digest Digest
		part   builtPart
	}{
		{digest: older, part: builtPart{data: json.RawMessage(`["` + rested + `"]`)}},
		{digest: newer, part: builtPart{on: rested, data: json.RawMessage(`["` + digested + `"]`)}},
	} {
		if _, err := d.writeDigest(written.digest, map[string]builtPart{"work": written.part}); err != nil {
			t.Fatal(err)
		}
	}
}

// olderPaged is the older digest of the page's lookup tests, of rested, its
// session work folded on none, of the version of the states given.
func olderPaged(body int) Digest {
	head := headOf(rested, 12)
	head.Body = body
	return Digest{Head: head, Sessions: map[string]DigestEntry{"work": {RestsOn: map[string]int{rested: 12}, State: json.RawMessage(`{}`)}}}
}

// partsDated returns the dates of parts, in order.
func partsDated(parts []SessionPart) []string {
	dates := make([]string, len(parts))
	for i, p := range parts {
		dates[i] = p.Date
	}
	return dates
}

// A lookup resting a session on its page, the router's writer's and a
// reader's alike, follows the chain of parts behind a state that stands
// through digests whose heads stand alone, decoding the entries of no link
// whose head says its digest doesn't stand, as one of another version: the
// state that chain leads through rests on none.
func TestAPagesWalkDecodesNoLinkWhoseHeadDoesntStand(t *testing.T) {
	tests := []struct {
		name    string
		older   Digest
		decoded []string
		base    string
		parts   []string
	}{
		{name: "each standing", older: olderPaged(statesVersion), decoded: []string{digested, rested}, base: digested, parts: []string{rested, digested}},
		{name: "the older of another version of the states", older: olderPaged(statesVersion + 1), decoded: []string{digested}, base: ""},
	}
	for name, lookOf := range looks() {
		for _, tt := range tests {
			t.Run(name+", "+tt.name, func(t *testing.T) {
				d := digestDays(t)
				pagedDigests(t, d, tt.older)
				look := &countingLook{daysLook: lookOf(d), lines: keeping(withDayBeforeRested(nil))}
				s := &sessionAsked{naming: []string{rested, digested}}

				newSessionsLookup(d, look).rest("work", s, RestOnPage)

				base := ""
				if s.base != nil {
					base = s.base.Date
				}
				if !slices.Equal(look.decoded, tt.decoded) || base != tt.base || !slices.Equal(partsDated(s.parts), tt.parts) {
					t.Errorf("rest decoded the digests of %q, resting the session on %q, of the parts of %q; want those of %q decoded, resting it on %q, of the parts of %q",
						look.decoded, base, partsDated(s.parts), tt.decoded, tt.base, tt.parts)
				}
			})
		}
	}
}

// A lookup reads a session's part of a day once, whichever of the session's
// rests lead through it: a rest after its parts files are gone rests on the
// parts it read.
func TestALookupReadsASessionsPartOnce(t *testing.T) {
	d := digestDays(t)
	pagedDigests(t, d, olderPaged(statesVersion))
	l := newSessionsLookup(d, &countingLook{daysLook: looks()["a reader's"](d), lines: keeping(withDayBeforeRested(nil))})
	first := &sessionAsked{naming: []string{rested, digested}}
	l.rest("work", first, RestOnPage)
	for _, date := range []string{rested, digested} {
		if err := os.Remove(partsFile(d.files.Dir, date)); err != nil {
			t.Fatal(err)
		}
	}

	again := &sessionAsked{naming: []string{rested, digested}}
	l.rest("work", again, RestOnPage)

	if again.base == nil || again.base.Date != digested || !reflect.DeepEqual(again.parts, first.parts) || len(first.parts) != 2 {
		t.Errorf("rested again, its parts gone, the session rests on %+v, of the parts %q; want it on %s's state, of the parts first read, %q",
			again.base, partsDated(again.parts), digested, partsDated(first.parts))
	}
}

// A lookup resting a session on its state, as the List's and the store's
// other sessions are rested, reads none of its parts, and gives none.
func TestALookupRestingASessionOnItsStateReadsNoPart(t *testing.T) {
	d := digestDays(t)
	pagedDigests(t, d, olderPaged(statesVersion))
	l := newSessionsLookup(d, &countingLook{daysLook: looks()["a reader's"](d), lines: keeping(withDayBeforeRested(nil))})
	s := &sessionAsked{naming: []string{rested, digested}}

	l.rest("work", s, RestOnState)

	if s.base == nil || s.base.Date != digested || len(l.parts) > 0 || s.parts != nil {
		t.Errorf("the session rests on %+v, of the parts %q, the lookup having read %d; want it on %s's state, none read", s.base, partsDated(s.parts), len(l.parts), digested)
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
