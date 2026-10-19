package ledger

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
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
// session folded on rested's state; each with work's part beside it, and
// its times, as pagedPart has them. It returns them as written.
func pagedDigests(t *testing.T, d days, older Digest) []Digest {
	t.Helper()
	newer := Digest{Head: headOf(digested, 40), Sessions: map[string]DigestEntry{"work": {RestsOn: map[string]int{rested: 12, digested: 40}, State: json.RawMessage(`{}`)}}}
	var digests []Digest
	for _, written := range []struct {
		digest Digest
		on     string
	}{
		{digest: older},
		{digest: newer, on: rested},
	} {
		date := written.digest.Head.Day
		digest, err := d.writeDigest(written.digest, builtOf(t, map[string]givenPart{"work": pagedPart(written.on, date)}))
		if err != nil {
			t.Fatal(err)
		}
		digests = append(digests, digest)
	}
	return digests
}

// pagedPart is work's part of the local day with the given date, folded on
// the state of the day with the date on, as pagedDigests writes it: its
// part, and its times, each naming the day.
func pagedPart(on, date string) givenPart {
	return givenPart{on: on, part: `["` + date + `"]`, times: `["times of ` + date + `"]`}
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

// A lookup reads a session's part of a day, and its times, once, whichever
// of the session's rests lead through it: a rest after its parts files are
// gone rests on the parts it read, their times among them.
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

	timed := !slices.ContainsFunc(first.parts, func(p SessionPart) bool { return p.Times == nil })
	if again.base == nil || again.base.Date != digested || !reflect.DeepEqual(again.parts, first.parts) || len(first.parts) != 2 || !timed {
		t.Errorf("rested again, its parts gone, the session rests on %+v, of the parts %q; want it on %s's state, of the parts first read, %q, each with its times",
			again.base, partsDated(again.parts), digested, partsDated(first.parts))
	}
}

// memberAt is a member of a day's parts file as a lookup read it: the day's
// date, and where the member lies.
type memberAt struct {
	date string
	m    Member
}

// readsOf has l note each member it reads of a parts file, in the order
// read, giving what it notes.
func readsOf(l *sessionsLookup) *[]memberAt {
	var read []memberAt
	l.reading = func(date string, m Member) { read = append(read, memberAt{date: date, m: m}) }
	return &read
}

// A lookup resting a session to fold, as the router's writer rests each
// session of a day it builds, reads each part of the chain behind its state
// once, and none of their times, its parts given without them; resting it
// on its page, as a reader of its page does, it reads each part and its
// times once, both given, as they were written, but for their
// insignificant spaces.
func TestALookupRestingToFoldReadsNoPartsTimes(t *testing.T) {
	for name, lookOf := range looks() {
		for _, on := range []Rest{RestToFold, RestOnPage} {
			t.Run(name+", "+on.String(), func(t *testing.T) {
				d := digestDays(t)
				written := pagedDigests(t, d, olderPaged(statesVersion))
				l := newSessionsLookup(d, &countingLook{daysLook: lookOf(d), lines: keeping(withDayBeforeRested(nil))})
				read := readsOf(l)
				s := &sessionAsked{naming: []string{rested, digested}}

				l.rest("work", s, on)

				var parts []SessionPart
				var reads []memberAt
				for _, digest := range slices.Backward(written) {
					date, ref := digest.Head.Day, digest.Sessions["work"].Part
					given, p := pagedPart(ref.On, date), SessionPart{Date: date}
					p.Part, reads = compacted(t, given.part), append(reads, memberAt{date: date, m: ref.Member})
					if on == RestOnPage {
						p.Times, reads = compacted(t, given.times), append(reads, memberAt{date: date, m: *ref.Times})
					}
					parts = append([]SessionPart{p}, parts...)
				}
				if s.base == nil || s.base.Date != digested || !reflect.DeepEqual(s.parts, parts) || !slices.Equal(*read, reads) {
					t.Errorf("the session rests on %+v, of the parts\n%s\nthe members read %+v; want it on %s's state, of the parts\n%s\nthe members read %+v",
						s.base, partsSaid(s.parts), *read, digested, partsSaid(parts), reads)
				}
			})
		}
	}
}

// partsSaid is what parts hold, each its date, its part and its times.
func partsSaid(parts []SessionPart) string {
	var said strings.Builder
	for _, p := range parts {
		fmt.Fprintf(&said, "%s: %s, times %s\n", p.Date, p.Part, p.Times)
	}
	return said.String()
}

// A part's times damaged in place, its parts file as long, break the chain
// of each state leading through it for a lookup resting a session on its
// page, which rests it on the state before; but not for one resting it to
// fold, which never reads them.
func TestAPartsTimesDamagedBreakAPagesChainButNotAFolds(t *testing.T) {
	for name, lookOf := range looks() {
		d := digestDays(t)
		written := pagedDigests(t, d, olderPaged(statesVersion))
		damaged(t, partsFile(d.files.Dir, digested), *written[1].Sessions["work"].Part.Times)
		for on, chain := range map[Rest][]string{RestToFold: {rested, digested}, RestOnPage: {rested}} {
			l := newSessionsLookup(d, &countingLook{daysLook: lookOf(d), lines: keeping(withDayBeforeRested(nil))})
			s := &sessionAsked{naming: []string{rested, digested}}

			l.rest("work", s, on)

			want := chain[len(chain)-1]
			if s.base == nil || s.base.Date != want || !slices.Equal(partsDated(s.parts), chain) {
				t.Errorf("%s, resting %s, the session rests on %+v, of the parts %q; want it on %s's state, of the parts %q", name, on, s.base, partsDated(s.parts), want, chain)
			}
		}
	}
}

// A lookup resting a session on its page judges each of its states as a link
// of a chain once, though the chain of every one leads down to a first part
// that's damaged: a long chain broken at its root costs a judgement a state,
// not one for each state below each.
func TestAPagesWalkJudgesEachLinkOnce(t *testing.T) {
	const length = 40
	d := digestDays(t)
	first, _, _ := dayfile.Day(rested)
	kept := map[string]keptDay{dayfile.DateOf(dayfile.DayStart(first, -1)): {known: true}}
	var naming []string
	var root Digest
	for i := range length {
		date := dayfile.DateOf(dayfile.DayStart(first, i))
		naming, kept[date] = append(naming, date), keptDay{lines: 1, known: true}
		restsOn := make(map[string]int, len(naming))
		for _, on := range naming {
			restsOn[on] = 1
		}
		on := ""
		if i > 0 {
			on = naming[i-1]
		}
		written, err := d.writeDigest(Digest{Head: headOf(date, 1), Sessions: map[string]DigestEntry{"work": {RestsOn: restsOn, State: json.RawMessage(`{}`)}}},
			builtOf(t, map[string]givenPart{"work": pagedPart(on, date)}))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			root = written
		}
	}
	damaged(t, partsFile(d.files.Dir, rested), root.Sessions["work"].Part.Member)
	l := newSessionsLookup(d, &countingLook{daysLook: looks()["a reader's"](d), lines: kept})
	judged := make(map[string]int)
	l.judged = func(date string) { judged[date]++ }
	s := &sessionAsked{naming: naming}

	l.rest("work", s, RestOnPage)

	once := !slices.ContainsFunc(naming, func(date string) bool { return judged[date] != 1 })
	if s.base != nil || !once || len(judged) != length {
		t.Errorf("over a chain of %d states broken at its root, the session rests on %+v, the states judged as links %v; want it on none, each of the %d judged once",
			length, s.base, judged, length)
	}
}

// damaged flips the bits of a byte in the middle of the member m of the file
// at path, leaving it as long.
func damaged(t *testing.T, path string, m Member) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[m.Offset+m.Length/2] ^= 0xff
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// A lookup resting a session on its state, as the List's and the store's
// other sessions are rested, reads none of its parts, and gives none.
func TestALookupRestingASessionOnItsStateReadsNoPart(t *testing.T) {
	d := digestDays(t)
	pagedDigests(t, d, olderPaged(statesVersion))
	l := newSessionsLookup(d, &countingLook{daysLook: looks()["a reader's"](d), lines: keeping(withDayBeforeRested(nil))})
	read := readsOf(l)
	s := &sessionAsked{naming: []string{rested, digested}}

	l.rest("work", s, RestOnState)

	if s.base == nil || s.base.Date != digested || len(*read) > 0 || s.parts != nil {
		t.Errorf("the session rests on %+v, of the parts %q, the lookup having read %+v; want it on %s's state, none read", s.base, partsDated(s.parts), *read, digested)
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
