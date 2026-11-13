package ledger_test

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
)

// apartDays files the lines of three sessions over four days, the first
// pool known as first, as once the move to pools is done: one in Acme's
// alone; two in the first's, then in Acme's, then in both, as one resumed
// in Acme's once its directory's fence changes, then back; and three
// naming none, as before pools, then the first's, then Acme's.
func apartDays(t *testing.T) *lookupWorld {
	t.Helper()
	w := newLookupWorld(t)
	w.writer = w.opened(w.fold, ledger.FirstPool(func() string { return first }))
	w.file(-3, inPool(askedBy("1", "one", on(-3, 9, 0)), acme), inPool(askedBy("2", "two", on(-3, 10, 0)), first), askedBy("3", "three", on(-3, 11, 0)))
	w.file(-2, inPool(askedBy("4", "two", on(-2, 9, 0)), acme), inPool(askedBy("5", "three", on(-2, 10, 0)), first))
	w.file(0, inPool(askedBy("6", "one", on(0, 9, 0)), acme), inPool(askedBy("7", "two", on(0, 10, 0)), first),
		inPool(askedBy("8", "two", on(0, 11, 0)), acme), inPool(askedBy("9", "three", on(0, 12, 0)), acme))
	w.summarise()
	w.digestEnded()
	return w
}

// apartState is a state apart as a test reads it of a digest's entry: the
// pools of its lines, the requests its state holds, the date its part was
// folded on, "" for none, and the requests of the lines its part was folded
// of.
type apartState struct {
	pools, requests []string
	on              string
	part            []string
}

// apartIn returns the states apart each session's entry holds in the digest
// of the local day days after the summaries' day, as apartState reads them,
// by its id, failing the test where a part doesn't read: none of an entry
// that holds none.
func (w *lookupWorld) apartIn(days int) map[string][]apartState {
	w.t.Helper()
	apart := make(map[string][]apartState)
	for id, e := range w.digestOf(days).Sessions {
		for _, s := range e.Apart {
			var state testState
			var part testPart
			if err := json.Unmarshal(s.State, &state); err != nil {
				w.t.Fatal(err)
			}
			if s.Part == nil {
				w.t.Fatalf("the state of %s apart in %v of %s points at no part", id, s.Pools, dayOf(days))
			}
			data, err := ledger.ReadPart(w.dir, dayOf(days), *s.Part)
			if err == nil {
				err = json.Unmarshal(data, &part)
			}
			if err != nil {
				w.t.Fatalf("the part of %s apart in %v of %s: %v", id, s.Pools, dayOf(days), err)
			}
			apart[id] = append(apart[id], apartState{pools: s.Pools, requests: state.Requests, on: s.Part.On, part: part.Requests})
		}
	}
	return apart
}

// A digest's entry of a session whose lines were routed in more than one
// pool holds its state in each apart, in the order of their pools, as the
// first pool, known, tells them: each folded from that pool's lines alone,
// on its state in the pool before them, its part chained to that state's;
// on the state of its entry of one pool before, that state being the
// pool's; and on none, in a pool new to it. Its own state holds every
// line's, as ever. One whose lines were all one pool's, though some named
// none and the rest the first pool, holds its own alone, in the form it had
// before pools. The builder is handed the sessions of each pool once a day,
// In telling the pool's lines; and built again, as a late line has a day's,
// each state apart is folded afresh on the day's.
func TestADigestsEntryOfASessionSplitBetweenPoolsHoldsItsStateInEach(t *testing.T) {
	w := apartDays(t)

	want := map[int]map[string][]apartState{
		-3: {},
		-2: {"two": {
			{pools: []string{first}, requests: []string{"2"}, on: dayOf(-3), part: []string{}},
			{pools: []string{acme}, requests: []string{"4"}, part: []string{"4"}},
		}},
		0: {
			"two": {
				{pools: []string{first}, requests: []string{"2", "7"}, on: dayOf(-2), part: []string{"7"}},
				{pools: []string{acme}, requests: []string{"4", "8"}, on: dayOf(-2), part: []string{"8"}},
			},
			"three": {
				{pools: []string{"", first}, requests: []string{"3", "5"}, on: dayOf(-2), part: []string{}},
				{pools: []string{acme}, requests: []string{"9"}, part: []string{"9"}},
			},
		},
	}
	for days, apart := range want {
		if got := w.apartIn(days); !reflect.DeepEqual(got, apart) {
			t.Errorf("the entries of %s hold the states apart\n%+v\nwant\n%+v", dayOf(days), got, apart)
		}
	}
	if got, want := w.requestsIn(0), map[string][]string{"one": {"1", "6"}, "two": {"2", "4", "7", "8"}, "three": {"3", "5", "9"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("the entries of %s hold their own states of the requests %v, want every request of each, %v", dayOf(0), got, want)
	}
	inPools := make(map[string][]string)
	for _, h := range w.fold.days() {
		if h.in != nil && h.date == dayOf(0) {
			pool := first
			if h.in(acme) {
				pool = acme
			}
			inPools[pool] = slices.Sorted(maps.Keys(h.sessions))
		}
	}
	if want := map[string][]string{first: {"three", "two"}, acme: {"three", "two"}}; !reflect.DeepEqual(inPools, want) {
		t.Errorf("the builder was handed the sessions of %s in each pool as %v, want two and three in each, once", dayOf(0), inPools)
	}

	w.file(-2, inPool(askedBy("late", "two", on(-2, 12, 0)), acme))
	w.summarise()
	w.digestEnded()

	again := w.apartIn(0)["two"]
	if want := []apartState{
		{pools: []string{first}, requests: []string{"2", "7"}, on: dayOf(-2), part: []string{"7"}},
		{pools: []string{acme}, requests: []string{"4", "late", "8"}, on: dayOf(-2), part: []string{"8"}},
	}; !reflect.DeepEqual(again, want) {
		t.Errorf("built again of a late line in Acme's, two's entry of %s holds the states apart\n%+v\nwant\n%+v", dayOf(0), again, want)
	}
}

// inPoolOf is what reports whether a request was routed in the pool with
// the given id, as RecordedIn has it of the first pool first, as a reader
// in the pool reads.
func inPoolOf(pool, first string) ledger.RoutedIn {
	return func(recorded string) bool { return ledger.RecordedIn(recorded, pool, first) }
}

// restedIn is a session as a read of it in a pool gave it, as a test reads
// it: the date of the state it rests on, "" for none, the pools of its
// lines, the requests the state holds, the dates of the parts of its chain,
// oldest first, and the requests of the lines each was folded of, and the
// requests of its lines after it.
type restedIn struct {
	base         string
	pools        []string
	state        []string
	chain        []string
	parts, lines [][]string
}

// restedOf is each of sessions, as restedIn reads it, by its id, failing
// the test where a state or a part doesn't read.
func restedOf(t *testing.T, sessions map[string]ledger.SessionFrom) map[string]restedIn {
	t.Helper()
	rested := make(map[string]restedIn, len(sessions))
	for id, s := range sessions {
		r := restedIn{base: s.BaseDate(), lines: [][]string{requestsOf(s.Lines)}}
		if s.Base != nil {
			var state testState
			if err := json.Unmarshal(s.Base.State, &state); err != nil {
				t.Fatal(err)
			}
			r.pools, r.state = s.Base.Pools, state.Requests
		}
		for _, p := range s.Parts {
			var part testPart
			if err := json.Unmarshal(p.Part, &part); err != nil {
				t.Fatal(err)
			}
			r.chain, r.parts = append(r.chain, p.Date), append(r.parts, part.Requests)
		}
		rested[id] = r
	}
	return rested
}

// A reader in a pool rests each session on its state in the pool, resting
// on its page on that state's chain of parts, through its states in the
// pool before it, its lines after it every pool's: Acme's two and three on
// their states in Acme's, and one, all Acme's, on its own; Default's, the
// first pool's, two on its state there, its chain through its state of one
// pool before it split, and three on its state of its lines naming none
// and the first pool's, one alone of none of Default's work not given. A
// reader whose first pool tells lines naming none apart from the first's,
// as the builder didn't, finds no state of its pool's alone in three's
// entries since they were the first pool's too, and rests it on its last of
// none's alone, its lines after it read. Every pool's reader rests each on
// its own state, as ever.
func TestAReaderInAPoolRestsEachSessionOnItsStateThere(t *testing.T) {
	w := apartDays(t)
	at := func(days int) string { return dayOf(days) }
	tests := []struct {
		name string
		in   ledger.RoutedIn
		want map[string]restedIn
	}{
		{name: "Acme's", in: inPoolOf(acme, first), want: map[string]restedIn{
			"one":   {base: at(0), pools: []string{acme}, state: []string{"1", "6"}, chain: []string{at(-3), at(0)}, parts: [][]string{{"1"}, {"6"}}, lines: [][]string{{}}},
			"two":   {base: at(0), pools: []string{acme}, state: []string{"4", "8"}, chain: []string{at(-2), at(0)}, parts: [][]string{{"4"}, {"8"}}, lines: [][]string{{}}},
			"three": {base: at(0), pools: []string{acme}, state: []string{"9"}, chain: []string{at(0)}, parts: [][]string{{"9"}}, lines: [][]string{{}}},
		}},
		{name: "Default's", in: inPoolOf(first, first), want: map[string]restedIn{
			"two":   {base: at(0), pools: []string{first}, state: []string{"2", "7"}, chain: []string{at(-3), at(-2), at(0)}, parts: [][]string{{"2"}, {}, {"7"}}, lines: [][]string{{}}},
			"three": {base: at(0), pools: []string{"", first}, state: []string{"3", "5"}, chain: []string{at(-3), at(-2), at(0)}, parts: [][]string{{"3"}, {"5"}, {}}, lines: [][]string{{}}},
		}},
		{name: "none's, before the first pool is known", in: inPoolOf("", ""), want: map[string]restedIn{
			"three": {base: at(-3), pools: []string{""}, state: []string{"3"}, chain: []string{at(-3)}, parts: [][]string{{"3"}}, lines: [][]string{{"5", "9"}}},
		}},
		{name: "every pool's", want: map[string]restedIn{
			"one":   {base: at(0), pools: []string{acme}, state: []string{"1", "6"}, chain: []string{at(-3), at(0)}, parts: [][]string{{"1"}, {"6"}}, lines: [][]string{{}}},
			"two":   {base: at(0), pools: []string{first, acme}, state: []string{"2", "4", "7", "8"}, chain: []string{at(-3), at(-2), at(0)}, parts: [][]string{{"2"}, {"4"}, {"7", "8"}}, lines: [][]string{{}}},
			"three": {base: at(0), pools: []string{"", first, acme}, state: []string{"3", "5", "9"}, chain: []string{at(-3), at(-2), at(0)}, parts: [][]string{{"3"}, {"5"}, {"9"}}, lines: [][]string{{}}},
		}},
	}
	onPage := func(string) ledger.Rest { return ledger.RestOnPage }
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := restedOf(t, w.follower().SessionsOfIn(tt.in, []string{"one", "two", "three"}, onPage, nil))

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("the sessions read in the pool rest as\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}
