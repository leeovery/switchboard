package ledger_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
)

// Each session's part of a day is written beside the day's digest, its entry
// pointing at it there, naming the day of the state the session was handed
// resting on, none where it rested on none: so a session's parts chain from
// its first day's, folded whole, through the states each was folded on. The
// digest's head records its parts file's size.
func TestEachSessionsPartIsWrittenOnTheStateItWasFoldedOn(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)

	w.digestEnded()

	part := func(id string, days int, base string, requests ...string) partIn {
		return partIn{on: base, part: testPart{Of: stateAt(id, dayOf(days)), Base: base, Requests: requests}}
	}
	want := map[int]map[string]partIn{
		-3: {"one": part("one", -3, "", "1"), "two": part("two", -3, "", "2")},
		-2: {"two": part("two", -2, dayOf(-3), "3")},
		0:  {"one": part("one", 0, dayOf(-3), "4"), "two": part("two", 0, dayOf(-2), "5")},
		1:  {"one": part("one", 1, dayOf(0), "6"), "three": part("three", 1, "", "7")},
	}
	for days, parts := range want {
		if got := w.partsIn(days); !reflect.DeepEqual(got, parts) {
			t.Errorf("the parts of %s are\n%+v\nwant\n%+v", dayOf(days), got, parts)
		}
		if size := fileInfo(t, partsPath(w.dir, days)).Size(); w.digestOf(days).Head.PartsSize != size {
			t.Errorf("the digest of %s records a parts file of %d bytes, want %d, its size", dayOf(days), w.digestOf(days).Head.PartsSize, size)
		}
	}
}

// A day whose parts file isn't as long as its digest's head records, as one
// gone, or written again by a build that stopped before its digest, has the
// parts its digest points at read as none, and is digested again at the
// next pass, as a day of no digest is, each session folded on the state
// before it: its parts then read as they were first written, and the pass
// after digests nothing.
func TestADayWhosePartsFileIsntAsItsHeadRecordsIsDigestedAgain(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, w *lookupWorld)
	}{
		{name: "its parts file gone", change: func(t *testing.T, w *lookupWorld) {
			removeFile(t, w.dir, filepath.Base(partsPath(w.dir, 0)))
		}},
		{name: "its parts file written again, of other parts, by a build that stopped before its digest", change: func(t *testing.T, w *lookupWorld) {
			if err := ledger.WriteParts(w.dir, dayOf(0), map[string]json.RawMessage{"two": json.RawMessage(`{"requests":["5","late"]}`)}); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newLookupWorld(t)
			sharedDays(w)
			w.digestEnded()
			w.fold.days()
			parts, digest := w.partsIn(0), w.digestOf(0)
			tt.change(t, w)

			for id, e := range digest.Sessions {
				if part, err := ledger.ReadPart(w.dir, dayOf(0), *e.Part); err == nil {
					t.Errorf("the part of %s the digest of %s points at reads %s; want it read as none", id, dayOf(0), part)
				}
			}
			w.digestEnded()

			handed := w.fold.days()
			wantHanded := map[string]handedSession{"one": {base: dayOf(-3), requests: []string{"4"}}, "two": {base: dayOf(-2), requests: []string{"5"}}}
			if len(handed) != 1 || handed[0].date != dayOf(0) || !reflect.DeepEqual(handed[0].sessions, wantHanded) {
				t.Fatalf("the pass handed %+v, want %s alone, of %+v", handed, dayOf(0), wantHanded)
			}
			if got := w.partsIn(0); !reflect.DeepEqual(got, parts) {
				t.Errorf("the parts of %s are\n%+v\nwant\n%+v, as first written", dayOf(0), got, parts)
			}
			w.digestEnded()
			if again := w.fold.days(); len(again) > 0 {
				t.Errorf("the pass after digested %q again, want none", datesOf(again))
			}
		})
	}
}

// A part damaged in its parts file, the file as long, doesn't read, but
// leaves its day not due, as the digesting looks at the file's size alone,
// for a ledger opened afresh too. Once its day is digested again, for a late
// line of another session, its session, whose state there stands still, is
// handed read whole, its part then folded whole.
func TestAPartDamagedItsFileAsLongIsFoldedWholeOnceItsDayIsDigestedAgain(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	w.fold.days()
	ref := w.digestOf(0).Sessions["one"].Part
	damageAt(t, partsPath(w.dir, 0), ref.Offset+ref.Length/2)

	w.digestEnded()
	fresh := &folding{}
	w.opened(fresh).DigestEnded(t.Context(), w.at)

	if part, err := ledger.ReadPart(w.dir, dayOf(0), *ref); err == nil {
		t.Errorf("one's part of %s, damaged, reads %s; want it read as none", dayOf(0), part)
	}
	if handed := slices.Concat(w.fold.days(), fresh.days()); len(handed) > 0 {
		t.Errorf("with one's part damaged, %q were digested again; want none, the parts file as long as its head records", datesOf(handed))
	}
	w.lateLine()
	w.digestEnded()
	handed := w.fold.days()
	if dates, want := datesOf(handed), []string{dayOf(-2), dayOf(0), dayOf(0)}; !slices.Equal(dates, want) {
		t.Fatalf("the days handed are %q, want %q: the late line's, then its day after, of two, and again, of one read whole", dates, want)
	}
	if got, want := handed[2].sessions, map[string]handedSession{"one": {requests: []string{"1", "4"}}}; !reflect.DeepEqual(got, want) {
		t.Errorf("%s was handed again %+v, want %+v, one read whole", dayOf(0), got, want)
	}
	if got, want := w.partsIn(0)["one"], (partIn{part: testPart{Of: stateAt("one", dayOf(0)), Requests: []string{"1", "4"}}}); !reflect.DeepEqual(got, want) {
		t.Errorf("one's part of %s is %+v, want %+v, folded whole", dayOf(0), got, want)
	}
}

// damageAt flips the bits of the byte of the file at path at the given
// offset, leaving it as long.
func damageAt(t *testing.T, path string, offset int64) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[offset] ^= 0xff
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// A ledger whose Digester gives a session an empty part writes it none: its
// entry points at none, and a day none of whose sessions has one has no
// parts file, its digest's head recording none.
func TestASessionGivenAnEmptyPartHasNone(t *testing.T) {
	w := newLookupWorld(t)
	w.file(0, askedBy("1", "one", on(0, 9, 0)))
	w.summarise()

	w.opened(partless{&folding{}}).DigestEnded(t.Context(), w.at)

	digest := w.digestOf(0)
	if one, held := digest.Sessions["one"]; !held || one.Part != nil || digest.Head.PartsSize != 0 {
		t.Errorf("the digest of %s is %+v; want one's entry pointing at no part, its head recording no parts file", dayOf(0), digest)
	}
	if _, err := os.Stat(partsPath(w.dir, 0)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the parts file of %s is looked at: %v; want none", dayOf(0), err)
	}
}

// partless is a Digester giving each session's state as the one it wraps
// gives it, and an empty part.
type partless struct {
	ledger.Digester
}

// Digest gives each session of day the state the wrapped Digester gives it,
// and an empty part.
func (p partless) Digest(ctx context.Context, day ledger.DigestDay) (map[string]ledger.Folded, error) {
	folded, err := p.Digester.Digest(ctx, day)
	for id, f := range folded {
		folded[id] = ledger.Folded{State: f.State, Part: json.RawMessage{}}
	}
	return folded, err
}
