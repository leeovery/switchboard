package ledger_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
)

// eachOnPage has a read of sessions rest each on its page.
func eachOnPage(string) ledger.Rest { return ledger.RestOnPage }

// pageAsOf returns the sessions with the given ids as of the local day days
// after the summaries' day, each resting on its page, as the follower's
// lookup finds them, failing the test where the router's writer's lookup
// finds them otherwise.
func (w *lookupWorld) pageAsOf(days int, ids ...string) map[string]ledger.SessionFrom {
	w.t.Helper()
	date := dayOf(days)
	read := followerOf(w.state, &testClock{at: w.at}, caps).SessionsAsOf(ids, date, eachOnPage, nil)
	if written := w.writer.SessionsAsOf(ids, date, w.at, eachOnPage); !reflect.DeepEqual(read, written) {
		w.t.Errorf("as of %s, resting on their pages, the follower finds %q as\n%+v\nand the router's writer as\n%+v; want them alike", date, ids, read, written)
	}
	return read
}

// onPage is a session as the lookup finds it resting on its page, as the
// tests tell it: the date of the state it rests on, "" for none; the dates
// of its chain of parts, as chained gives them; and the requests of its
// lines, in order.
type onPage struct {
	base     string
	chain    []string
	requests []string
}

// pagesOf tells each of sessions as onPage has it, by its id, failing the
// test where one rests on a state its digest's day doesn't tell, or on a
// chain of parts that doesn't lead to its state, as chained says.
func pagesOf(t testing.TB, sessions map[string]ledger.SessionFrom) map[string]onPage {
	t.Helper()
	pages := make(map[string]onPage, len(sessions))
	for id, f := range foundOf(t, sessions) {
		from := sessions[id]
		page := onPage{chain: chained(t, id, from), requests: f.requests}
		if from.Base != nil {
			page.base = from.Base.Date
		}
		pages[id] = page
	}
	return pages
}

// chained returns the dates of the chain of parts from gives, oldest first,
// failing the test where it doesn't lead to its base's state: each part the
// session's own of its day, as stateAt tells it, the first folded on none,
// each after it on the day of the one before, the last of the base's day,
// and their requests, one after another, its base's state's.
func chained(t testing.TB, id string, from ledger.SessionFrom) []string {
	t.Helper()
	var dates, requests []string
	on := ""
	for _, p := range from.Parts {
		var part testPart
		if err := json.Unmarshal(p.Part, &part); err != nil {
			t.Fatal(err)
		}
		if part.Of != stateAt(id, p.Date) || part.Base != on {
			t.Errorf("session %s's part of %s is %+v; want its own of the day, folded on %q", id, p.Date, part, on)
		}
		dates, requests, on = append(dates, p.Date), append(requests, part.Requests...), p.Date
	}
	if from.Base == nil {
		if len(from.Parts) > 0 {
			t.Errorf("session %s rests on none, of a chain of parts of %q; want none", id, dates)
		}
		return dates
	}
	if base := baseOf(t, from.Base); on != from.Base.Date || !slices.Equal(requests, base.Requests) {
		t.Errorf("session %s's chain of parts ends on %q, of %q; want it ending on its base's day, %q, of its state's %q", id, on, requests, from.Base.Date, base.Requests)
	}
	return dates
}

// isPaged fails t where got, the sessions a lookup as of the local day days
// after the summaries' day found resting on their pages, aren't want.
func isPaged(t *testing.T, days int, got, want map[string]onPage) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("as of %s, the sessions found resting on their pages are\n%+v\nwant\n%+v", dayOf(days), got, want)
	}
}

// datesOn returns the dates of the local days the given numbers of days
// after the summaries' day.
func datesOn(days ...int) []string {
	dates := make([]string, len(days))
	for i, d := range days {
		dates[i] = dayOf(d)
	}
	return dates
}

// A session resting on its page rests on the newest state a digest of a day
// naming it holds that stands, as one resting on its state does, where its
// chain of parts reads, the parts given oldest first, from its first day's,
// folded on none, to its base's own; one no digest holds rests on none, of
// no parts.
func TestASessionsPageRestsOnTheNewestStateWhoseChainOfPartsReads(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digest(-3, "one", "two")
	w.digest(-2, "two")
	w.digest(0, "one", "two")
	w.digest(1, "one")

	got := pagesOf(t, w.pageAsOf(2, "one", "two", "three"))

	isPaged(t, 2, got, map[string]onPage{
		"one":   {base: dayOf(1), chain: datesOn(-3, 0, 1), requests: []string{}},
		"two":   {base: dayOf(0), chain: datesOn(-3, -2, 0), requests: []string{}},
		"three": {requests: []string{"7"}},
	})
	for id, s := range w.asOf(2, "one", "two", "three") {
		want := ""
		if got[id].base != "" {
			want = stateAt(id, got[id].base)
		}
		if s.base != want {
			t.Errorf("session %s rests on the state of %q, its page on %q; want them alike", id, s.base, want)
		}
	}
}

// oddDays files the lines of session one the page's tests of its links
// share, on the 2nd, the 4th and the 6th, and none between, and digests each
// day: so its state as of the 6th stands, folded on the 4th's, folded on the
// 2nd's, folded on none, whatever becomes of the 4th's, as the day before
// the 6th holds no lines.
func oddDays(w *lookupWorld) {
	w.file(-3, askedBy("1", "one", on(-3, 9, 0)))
	w.file(-1, askedBy("2", "one", on(-1, 9, 0)))
	w.file(1, askedBy("3", "one", on(1, 9, 0)))
	w.summarise()
	for _, days := range []int{-3, -1, 1} {
		w.digest(days, "one")
	}
}

// partsWritten returns the parts of the sessions the digest of the local day
// days after the summaries' day points at, by their ids, each as it reads,
// and the date each was folded on.
func (w *lookupWorld) partsWritten(days int) (map[string]json.RawMessage, map[string]string) {
	w.t.Helper()
	parts, on := make(map[string]json.RawMessage), make(map[string]string)
	for id, e := range w.digestOf(days).Sessions {
		part, err := ledger.ReadPart(w.dir, dayOf(days), *e.Part)
		if err != nil {
			w.t.Fatal(err)
		}
		parts[id], on[id] = part, e.Part.On
	}
	return parts, on
}

// rewritten writes the digest of the local day days after the summaries' day
// again, as change changes it and the dates its parts were folded on, its
// parts as they read.
func (w *lookupWorld) rewritten(days int, change func(d *ledger.Digest, on map[string]string)) {
	w.t.Helper()
	digest := w.digestOf(days)
	parts, on := w.partsWritten(days)
	change(&digest, on)
	if _, err := ledger.WriteDigestWithParts(w.dir, digest, parts, on); err != nil {
		w.t.Fatal(err)
	}
}

// A session's page doesn't rest on a state a link of whose chain of parts
// doesn't hold, though the state stands: one whose digest is gone, or of
// another version, or holds no entry of the session, or whose part is gone,
// or damaged, its file as long; or one folded, as its part says, on a state
// of a day not naming the session, or of its own day. It rests on the newest
// older state whose chain holds, its lines those after it; on none where
// none's does, every line of it read.
func TestAPageRestsOnAnOlderStateWhereALinkOfItsChainDoesntHold(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, w *lookupWorld)
		want   onPage
	}{
		{name: "the digest of a day mid-chain gone", change: func(t *testing.T, w *lookupWorld) {
			removeFile(t, w.dir, filepath.Base(digestPath(w.dir, -1)))
		}, want: onPage{base: dayOf(-3), chain: datesOn(-3), requests: []string{"2", "3"}}},
		{name: "the digest of a day mid-chain of another version", change: func(t *testing.T, w *lookupWorld) {
			w.rewritten(-1, func(d *ledger.Digest, _ map[string]string) { d.Head.Body++ })
		}, want: onPage{base: dayOf(-3), chain: datesOn(-3), requests: []string{"2", "3"}}},
		{name: "the digest of a day mid-chain counting a line that arrived after it", change: func(t *testing.T, w *lookupWorld) {
			w.rewritten(-1, func(d *ledger.Digest, _ map[string]string) { d.Head.Late = 1 })
		}, want: onPage{base: dayOf(-3), chain: datesOn(-3), requests: []string{"2", "3"}}},
		{name: "the entry mid-chain resting on a day's lines as they no longer are", change: func(t *testing.T, w *lookupWorld) {
			w.rewritten(-1, func(d *ledger.Digest, _ map[string]string) { d.Sessions["one"].RestsOn[dayOf(-3)]++ })
		}, want: onPage{base: dayOf(-3), chain: datesOn(-3), requests: []string{"2", "3"}}},
		{name: "the digest of a day mid-chain holding no entry of the session", change: func(t *testing.T, w *lookupWorld) {
			w.rewritten(-1, func(d *ledger.Digest, _ map[string]string) { delete(d.Sessions, "one") })
		}, want: onPage{base: dayOf(-3), chain: datesOn(-3), requests: []string{"2", "3"}}},
		{name: "the parts file of a day mid-chain gone", change: func(t *testing.T, w *lookupWorld) {
			removeFile(t, w.dir, filepath.Base(partsPath(w.dir, -1)))
		}, want: onPage{base: dayOf(-3), chain: datesOn(-3), requests: []string{"2", "3"}}},
		{name: "the part of a day mid-chain damaged, its file as long", change: func(t *testing.T, w *lookupWorld) {
			ref := w.digestOf(-1).Sessions["one"].Part
			damageAt(t, partsPath(w.dir, -1), ref.Offset+ref.Length/2)
		}, want: onPage{base: dayOf(-3), chain: datesOn(-3), requests: []string{"2", "3"}}},
		{name: "the part of the first day damaged, its file as long", change: func(t *testing.T, w *lookupWorld) {
			ref := w.digestOf(-3).Sessions["one"].Part
			damageAt(t, partsPath(w.dir, -3), ref.Offset+ref.Length/2)
		}, want: onPage{requests: []string{"1", "2", "3"}}},
		{name: "the newest part folded on a day not naming the session", change: func(t *testing.T, w *lookupWorld) {
			w.rewritten(1, func(_ *ledger.Digest, on map[string]string) { on["one"] = dayOf(0) })
		}, want: onPage{base: dayOf(-1), chain: datesOn(-3, -1), requests: []string{"3"}}},
		{name: "the newest part folded on its own day", change: func(t *testing.T, w *lookupWorld) {
			w.rewritten(1, func(_ *ledger.Digest, on map[string]string) { on["one"] = dayOf(1) })
		}, want: onPage{base: dayOf(-1), chain: datesOn(-3, -1), requests: []string{"3"}}},
		{name: "a part mid-chain folded on a later day", change: func(t *testing.T, w *lookupWorld) {
			w.rewritten(-1, func(_ *ledger.Digest, on map[string]string) { on["one"] = dayOf(1) })
		}, want: onPage{base: dayOf(-3), chain: datesOn(-3), requests: []string{"2", "3"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newLookupWorld(t)
			oddDays(w)
			tt.change(t, w)

			got := pagesOf(t, w.pageAsOf(2, "one"))

			isPaged(t, 2, got, map[string]onPage{"one": tt.want})
			if s := w.asOf(2, "one")["one"]; s.base != stateAt("one", dayOf(1)) {
				t.Errorf("session one rests on the state of %q, want it on its state as of %s, which stands still", s.base, dayOf(1))
			}
		})
	}
}

// A session's page rests on a state folded on one older than the last day
// naming the session before it, as one whose entry that day's digest left
// out: its chain passes that day by.
func TestAPagesChainPassesByADayWhoseDigestLeftItsSessionOut(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digest(-3, "one", "two")
	w.digest(-2, "two")
	w.digest(0, "two")
	w.digest(1, "one")

	got := pagesOf(t, w.pageAsOf(2, "one"))

	isPaged(t, 2, got, map[string]onPage{"one": {base: dayOf(1), chain: datesOn(-3, 1), requests: []string{}}})
}

// A session's chain of parts ends at a part folded on none, as one read
// whole folds it, though states before it stand: their parts aren't read,
// so one of them damaged leaves the page on its newest state.
func TestAPagesChainEndsAtAPartFoldedOnNone(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digest(-3, "one", "two")
	w.digest(-2, "two")
	w.digestResting(0, wholeOf("one"), "one", "two")
	w.digest(1, "one")
	ref := w.digestOf(-3).Sessions["one"].Part
	damageAt(t, partsPath(w.dir, -3), ref.Offset+ref.Length/2)

	got := pagesOf(t, w.pageAsOf(2, "one", "two"))

	isPaged(t, 2, got, map[string]onPage{
		"one": {base: dayOf(1), chain: datesOn(0, 1), requests: []string{}},
		"two": {base: dayOf(0), chain: datesOn(-3, -2, 0), requests: []string{}},
	})
}

// A session's page whose base falls, as a line read after it arrived before
// the base's day ended, rests on none, of no parts, its lines every one.
func TestAPageWhoseBaseFallsRestsOnNoParts(t *testing.T) {
	w := newLookupWorld(t)
	w.file(-3, askedBy("1", "one", on(-3, 9, 0)))
	w.file(-2, askedBy("2", "one", on(-2, 9, 0)))
	w.file(-1, askedBy("3", "one", on(-1, 9, 0)), askedBy("early", "one", on(-2, 22, 0)))
	w.summarise()
	w.digest(-3, "one")
	w.digest(-2, "one")

	got := pagesOf(t, w.pageAsOf(2, "one"))

	isPaged(t, 2, got, map[string]onPage{"one": {requests: []string{"1", "2", "early", "3"}}})
}

// upsetParts changes the parts and digests of the ledger since its days were
// digested, as r draws it: a day's digest gone, a day's parts file gone, and
// a session's part damaged, its file as long.
func (g *generatedLedger) upsetParts() {
	if days := g.parted(); len(days) > 0 && g.r.IntN(3) == 0 {
		removeFile(g.w.t, g.w.dir, filepath.Base(digestPath(g.w.dir, days[g.r.IntN(len(days))])))
	}
	if days := g.parted(); len(days) > 0 && g.r.IntN(3) == 0 {
		removeFile(g.w.t, g.w.dir, filepath.Base(partsPath(g.w.dir, days[g.r.IntN(len(days))])))
	}
	if days := g.parted(); len(days) > 0 && g.r.IntN(2) == 0 {
		d := days[g.r.IntN(len(days))]
		sessions := g.w.digestOf(d).Sessions
		ref := sessions[slices.Sorted(maps.Keys(sessions))[g.r.IntN(len(sessions))]].Part
		damageAt(g.w.t, partsPath(g.w.dir, d), ref.Offset+ref.Length/2)
	}
}

// parted returns the days the world's ledger holds a digest and a parts file
// of, by their offsets from the summaries' day, oldest first.
func (g *generatedLedger) parted() []int {
	var days []int
	for d := -4; d <= 1; d++ {
		if _, err := os.Stat(partsPath(g.w.dir, d)); err == nil && g.w.digested(d) {
			days = append(days, d)
		}
	}
	return days
}

// pageBase returns the date of the state the session with the given id, as
// of the day two after the summaries', ought to rest on resting on its page,
// and the dates of its chain of parts, oldest first, as the world's own
// files tell them, not as the lookup's walk does: the newest state no newer
// than base, the one it rests on resting on its state, that stands and
// whose chain of parts holds, each link standing, as stands has it, and its
// part reading, each folded on the day its part's Base names; none where
// none's does.
func (g *generatedLedger) pageBase(id, base string, stands map[string]map[string]bool) (string, []string) {
	for d := 1; d >= -4; d-- {
		if date := dayOf(d); date <= base && stands[date][id] {
			if chain, ok := g.chainOf(id, date, stands); ok {
				return date, chain
			}
		}
	}
	return "", nil
}

// chainOf returns the dates of the chain of parts behind the state of the
// session with the given id as of the end of the local day with the given
// date, oldest first, as pageBase follows it, reporting false where it
// doesn't hold.
func (g *generatedLedger) chainOf(id, date string, stands map[string]map[string]bool) ([]string, bool) {
	var chain []string
	for date != "" {
		if !stands[date][id] {
			return nil, false
		}
		digest, err := ledger.ReadDigest(g.w.dir, date, digestBody)
		if err != nil || digest.Sessions[id].Part == nil {
			return nil, false
		}
		data, err := ledger.ReadPart(g.w.dir, date, *digest.Sessions[id].Part)
		if err != nil {
			return nil, false
		}
		var part testPart
		if err := json.Unmarshal(data, &part); err != nil {
			g.w.t.Fatal(err)
		}
		if part.Base >= date {
			return nil, false
		}
		chain, date = append(chain, date), part.Base
	}
	slices.Reverse(chain)
	return chain, true
}

// standing returns, of each local day from four before the summaries' day to
// the one after it, by its date, the sessions whose state its digest holds
// stands, by their ids, as a lookup as of the day after rests each on it.
func (g *generatedLedger) standing() map[string]map[string]bool {
	stands := make(map[string]map[string]bool)
	for d := -4; d <= 1; d++ {
		date := dayOf(d)
		stands[date] = make(map[string]bool)
		for id, from := range followerOf(g.w.state, &testClock{at: g.w.at}, caps).SessionsAsOf(generatedSessions, dayOf(d+1), nil, nil) {
			stands[date][id] = from.Base != nil && from.Base.Date == date
		}
	}
	return stands
}

// A lookup over a generated ledger, its days digested with their parts as
// the router's digester would, then its parts and digests changed since,
// and on every other seed its lines too, rests each session on its page as
// a raw read of its lines has it: its base's state folded from exactly
// those filed under the base's day and before, its chain of parts leading
// to that state, and its lines those filed after; on the newest state that
// stands whose chain holds, as the world's own files tell it, never newer
// than the state it rests on resting on its state; the router's writer's
// lookup and a reader's alike. Lines are filed beside their days more
// rarely than the lookup's own property has them, so more states stand to
// be passed over for their chains.
func TestALookupRestsAPageAsARawReadHasItOverAGeneratedLedger(t *testing.T) {
	for seed := range uint64(40) {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			g := newGeneratedLedger(t, seed)
			g.shifts = 24
			g.generate()
			g.digestEach()
			if seed%2 == 1 {
				g.upset()
			}
			g.upsetParts()

			pages := g.w.pageAsOf(2, generatedSessions...)
			states := followerOf(g.w.state, &testClock{at: g.w.at}, caps).SessionsAsOf(generatedSessions, dayOf(2), nil, nil)
			stands := g.standing()

			for _, id := range generatedSessions {
				raw := requestsOf(followerOf(g.w.state, &testClock{at: g.w.at}, caps).SessionsOf([]string{id}, eachWhole, nil)[id].Lines)
				from, base := pages[id], ""
				if from.Base != nil {
					base = from.Base.Date
				}
				var before, after []string
				for _, request := range raw {
					if g.filed[request] <= base {
						before = append(before, request)
					} else {
						after = append(after, request)
					}
				}
				if folded, lines := baseOf(t, from.Base).Requests, requestsOf(from.Lines); !slices.Equal(folded, before) || !slices.Equal(lines, after) {
					t.Errorf("seed %d: session %s's page rests on its state as of %q, folded from %q, its lines %q; want it folded from %q, its lines %q, as a raw read of %q has them",
						seed, id, base, folded, lines, before, after, raw)
				}
				chain := chained(t, id, from)
				stateBase := ""
				if s := states[id]; s.Base != nil {
					stateBase = s.Base.Date
				}
				if wantBase, wantChain := g.pageBase(id, stateBase, stands); base != wantBase || !slices.Equal(chain, wantChain) {
					t.Errorf("seed %d: session %s's page rests on its state as of %q, of the chain %q; want it on %q's, of the chain %q, its state resting on %q's",
						seed, id, base, chain, wantBase, wantChain, stateBase)
				}
			}
		})
	}
}
