package views

import (
	"encoding/json"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// The pooled nine days' sessions, routed as pooledNine routes them.
var pooledNineIDs = []string{"a", "b", "c", "d", "e", "f"}

// pooledNine routes the line l of the pooled nine days' sessions, on the
// day given, in its session's pool: a in Acme's pool; b in Default's, the
// first's; c in Default's for its first five days, then in Acme's, as one
// resumed once its directory's fence changes; d in none, as before pools,
// so the first's; e in Default's but for the day after the nine, in Acme's;
// and f in none for its first five days, then in Acme's, so Default's by
// naming none, as one from before pools resumed in Acme's.
func pooledNine(l *ledger.Line, day int) {
	switch id := l.Session; {
	case id == "a", id == "c" && day >= 5, id == "e" && day == 9, id == "f" && day >= 5:
		l.Pool = "q7mt"
	case id == "d", id == "f":
		l.Pool = ""
	default:
		l.Pool = "b2n8"
	}
}

// apartNine routes the line l of the pooled nine days as pooledNine does,
// but with the work in Default's of c, e and f, the sessions that work in
// both, made under another id: so each of them is in Acme's alone.
func apartNine(l *ledger.Line, day int) {
	if pooledNine(l, day); l.Pool != "q7mt" && slices.Contains([]string{"c", "e", "f"}, l.Session) {
		l.Session += "-default"
	}
}

// The reads of the pooled nine days' pools, Acme's and Default's, the first
// pool's, no account placed in either, as nothing of a session goes by its
// account's pool.
var (
	acmeNine    = PoolRead{ID: "q7mt", First: "b2n8", PoolOf: func(string) (string, bool) { return "", false }}
	defaultNine = PoolRead{ID: "b2n8", First: "b2n8", PoolOf: func(string) (string, bool) { return "", false }}
)

// runningOf are those of the nine days' sessions running with the given
// ids, as the router lists them in a pool.
func (n nineDays) runningOf(ids []string) []status.Session {
	return slices.DeleteFunc(slices.Clone(n.running), func(s status.Session) bool { return !slices.Contains(ids, s.ID) })
}

// listedOf are the ids of the sessions the List lists, in its order.
func listedOf(list SessionList) []string {
	ids := make([]string, len(list.Sessions))
	for i, s := range list.Sessions {
		ids[i] = s.Session
	}
	return ids
}

// A pool's List over digests, and each session's page, opened as the watch
// opens it and held by the store beside the List, are those read raw of
// the pool's lines alone, the sessions of other pools none of it: a, Acme's
// alone; b and d, Default's, d naming none; c, Default's then Acme's; e,
// Default's till the day after the nine, Acme's then; and f, naming none,
// then Acme's; each page of a session with work in the pool showing its
// points. Every pool's are too.
func TestAPoolsSessionsOverDigestsAreThoseOfItsLinesReadRaw(t *testing.T) {
	n := newRoutedNineDays(t, pooledNineIDs, pooledNine)
	tests := []struct {
		name            string
		read            PoolRead
		listed, pointed []string
	}{
		{name: "Acme's", read: acmeNine, listed: []string{"a", "c", "e", "f"}, pointed: []string{"a", "c", "e", "f"}},
		{name: "Default's", read: defaultNine, listed: []string{"b", "d"}, pointed: []string{"b", "c", "d", "e", "f"}},
		{name: "every pool's", read: PoolRead{}, listed: pooledNineIDs, pointed: pooledNineIDs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			based, raw := tt.read.Ledger(n.follower(DigestVersion)), tt.read.Ledger(n.follower(DigestVersion+1))
			running := n.runningOf(tt.listed)
			list := func(l Ledger) SessionList {
				return ListSessions(SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: n.now})
			}

			got, want := list(based), list(raw)

			if listed := listedOf(want); !reflect.DeepEqual(got, want) || !slices.Equal(slices.Sorted(slices.Values(listed)), tt.listed) {
				t.Errorf("over digests, the List is\n%+v\nwant, as read raw, of %v\n%+v", got, tt.listed, want)
			}
			for _, id := range pooledNineIDs {
				got, want := n.opened(based, id, Stream{}), n.opened(raw, id, Stream{})
				if !reflect.DeepEqual(got, want) || slices.Contains(tt.pointed, id) != pointed(want.page) {
					t.Errorf("over digests, %s's page shows\n%+v\nwant, as read raw, its points where it's the pool's\n%+v", id, got, want)
				}
				if got, want := n.stored(based, id, true, Stream{}), n.stored(raw, id, true, Stream{}); !reflect.DeepEqual(got, want) {
					t.Errorf("over digests, %s's page held beside the List shows\n%+v\nwant, as read raw\n%+v", id, got, want)
				}
			}
		})
	}
}

// basePools are the pools of the lines the state s rests on was folded
// from, as its entry's state in the pool read names them, none where it
// rests on none.
func basePools(s ledger.SessionFrom) []string {
	if s.Base == nil {
		return nil
	}
	return s.Base.Pools
}

// Over digests, a pool's read rests each session on its state in the pool,
// one of the pool's work alone, at the day before's end, the session's lines
// after it the pool's: Acme's, a on its own, all Acme's; c, Default's for
// five days, then Acme's, and f, naming none for five, then Acme's, on their
// states apart in Acme's; e, none of whose lines before today were Acme's,
// on none, today's lines alone being Acme's; and b and d, of none of Acme's
// work, not given. Default's rests b on its own, d and f on their states of
// their lines naming none, and c and e on their states of their Default
// work, none of their lines today being Default's; a, all Acme's, not
// given.
func TestAPoolsReadRestsEachSessionOnItsStateInThePool(t *testing.T) {
	n := newRoutedNineDays(t, pooledNineIDs, pooledNine)
	yesterday := yesterdayOf(n.today)
	type rested struct {
		pools []string
		today bool
	}
	tests := []struct {
		name string
		read PoolRead
		pool string
		want map[string]rested
	}{
		{name: "Acme's", read: acmeNine, pool: "q7mt", want: map[string]rested{
			"a": {pools: []string{"q7mt"}, today: true}, "c": {pools: []string{"q7mt"}, today: true},
			"e": {today: true}, "f": {pools: []string{"q7mt"}, today: true},
		}},
		{name: "Default's", read: defaultNine, pool: "b2n8", want: map[string]rested{
			"b": {pools: []string{"b2n8"}, today: true}, "c": {pools: []string{"b2n8"}}, "d": {pools: []string{""}, today: true},
			"e": {pools: []string{"b2n8"}}, "f": {pools: []string{""}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.read.Ledger(n.follower(DigestVersion)).SessionsOf(pooledNineIDs, nil, nil)

			if ids, want := slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(tt.want)); !slices.Equal(ids, want) {
				t.Fatalf("the read gave %v, want %v", ids, want)
			}
			for id, want := range tt.want {
				s := got[id]
				if pools := basePools(s); !slices.Equal(pools, want.pools) || want.pools != nil && s.Base.Date != yesterday {
					t.Errorf("%s rests on %+v, want its state in the pool of the pools %q at the end of %s", id, s.Base, want.pools, yesterday)
				}
				if today := len(s.Lines) > 0; today != want.today || today && dayfile.DateOf(s.Lines[0].At) != n.today.Date {
					t.Errorf("%s's lines after its state are %v, want today's alone: %t", id, s.Lines, want.today)
				}
				if i := slices.IndexFunc(s.Lines, func(l ledger.Line) bool { return !tt.read.keeps(l.Pool) }); i >= 0 {
					t.Errorf("%s's lines hold %+v, want the pool's alone", id, s.Lines[i])
				}
			}
		})
	}
}

// A digest's entry of a session whose lines were routed in more than one
// pool holds its state in each apart, in the order of their pools, from the
// day it first ran in a second: c's, Default's then Acme's, and f's, naming
// none, then Acme's, as the day before's digest holds them; its own state
// before then, as each other session's always, its alone, in the form it had
// before pools, as their entries' files hold them.
func TestASplitSessionsEntryHoldsItsStateInEachPool(t *testing.T) {
	n := newRoutedNineDays(t, pooledNineIDs, pooledNine)
	dates := dayfile.Span(n.first.Start, n.today.Start)
	apartOn := func(date string) map[string][][]string {
		var digest struct {
			Sessions map[string]struct {
				Apart []struct {
					Pools []string `json:"pools"`
				} `json:"apart"`
			} `json:"sessions"`
		}
		data, err := os.ReadFile(n.chain.digestPath(date))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(gunzipped(t, data), &digest); err != nil {
			t.Fatal(err)
		}
		apart := make(map[string][][]string)
		for id, e := range digest.Sessions {
			for _, s := range e.Apart {
				apart[id] = append(apart[id], s.Pools)
			}
		}
		return apart
	}

	if got := apartOn(dates[4]); len(got) != 0 {
		t.Errorf("the digest of %s, before any session ran in a second pool, holds states apart of %v, want none", dates[4], got)
	}
	for _, date := range []string{dates[5], dates[8]} {
		if got, want := apartOn(date), map[string][][]string{"c": {{"b2n8"}, {"q7mt"}}, "f": {nil, {"q7mt"}}}; !reflect.DeepEqual(got, want) {
			t.Errorf("the digest of %s holds the states apart of %v, want c's in Default's and Acme's, and f's of its lines naming none and in Acme's, those of none stored as none", date, got)
		}
	}
}

// A split session's read in a pool over digests reads no day before its
// state there: c's List, its page opened as the watch opens it, and its page
// held by the store beside the List, in Acme's and in Default's, read the
// lines of today's file alone, as a session of one pool's do, where the same
// reads raw read every day it ran. Each shows what it shows read raw.
func TestASplitSessionsReadInAPoolReadsNoDayBeforeItsStateThere(t *testing.T) {
	n := newRoutedNineDays(t, pooledNineIDs, pooledNine)
	reads := []struct {
		name string
		read func(l Ledger) any
	}{
		{name: "the List", read: func(l Ledger) any {
			return ListSessions(SessionSources{Running: n.running, Ledger: l, Prices: ledger.Pricing, Now: n.now})
		}},
		{name: "its page opened", read: func(l Ledger) any { return n.opened(l, "c", Stream{}) }},
		{name: "its page held beside the List", read: func(l Ledger) any { return n.stored(l, "c", true, Stream{}) }},
	}
	for _, pool := range []PoolRead{acmeNine, defaultNine} {
		for _, r := range reads {
			t.Run(pool.ID+"/"+r.name, func(t *testing.T) {
				based, raw := takingDates(n.follower(DigestVersion)), takingDates(n.follower(DigestVersion+1))

				got, want := r.read(pool.Ledger(based)), r.read(pool.Ledger(raw))

				if dates := based.datesRead(); !slices.Equal(dates, []string{n.today.Date}) {
					t.Errorf("over digests, it read the lines of %v, want those of %s alone", dates, n.today.Date)
				}
				if dates, every := raw.datesRead(), dayfile.Span(n.first.Start, n.today.Start); !slices.Equal(dates, every) {
					t.Errorf("read raw, it read the lines of %v, want those of every day, %v", dates, every)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("over digests, it shows\n%+v\nwant, as read raw\n%+v", got, want)
				}
			})
		}
	}
}

// Default's read of today's sessions over digests passes over c, unread,
// where it isn't asked for: resumed in Acme's on its sixth day, it made no
// request in Default's since, so the read gives none of it, its state there
// unrestored, and the List never lists it. Asked for, c rests on its state
// in Default's, of its Default work of its first five days, none of its
// lines after it, and none of its first days read.
func TestAPoolsReadPassesOverASessionThatLeftItUnlessAsked(t *testing.T) {
	n := newRoutedNineDays(t, pooledNineIDs, pooledNine)
	first := dayfile.Span(n.first.Start, n.today.Start)[0]
	read := func(ids []string) (ledger.SessionFrom, bool, bool) {
		var reached bool
		got := defaultNine.Ledger(n.follower(DigestVersion)).Sessions(ids, nil, func(date string, _ ledger.Line) { reached = reached || date == first })
		c, ok := got["c"]
		return c, ok, reached
	}

	if c, ok, reached := read([]string{"b", "d"}); ok || reached {
		t.Errorf("Default's read of today's sessions gave c as %+v, reading its first day %t; want none of it, unread", c, reached)
	}
	c, ok, reached := read([]string{"b", "c", "d"})
	if !ok || reached || !slices.Equal(basePools(c), []string{"b2n8"}) || c.Base.Date != yesterdayOf(n.today) || len(c.Lines) != 0 {
		t.Errorf("asked for, c is given as %+v, %t, reading its first day %t; want it resting on its state in Default's at the end of %s, none of its lines after it, its first day unread", c, ok, reached, yesterdayOf(n.today))
	}
}

// A split session over digests, after a late line of its filed under a day
// past in either of its pools, the days from it digested again, is in each
// pool as read raw: c's List and its page, opened and held by the store, in
// Acme's and in Default's, of a line in Acme's and of one in Default's filed
// under its seventh day, Acme's then.
func TestASplitSessionAfterALateLineInEitherPoolIsItsLinesReadRaw(t *testing.T) {
	for _, late := range []string{"q7mt", "b2n8"} {
		t.Run(late, func(t *testing.T) {
			n := newRoutedNineDays(t, pooledNineIDs, pooledNine)
			date := dayfile.Span(n.first.Start, n.today.Start)[6]
			lines := n.chain.lines
			l := lines[slices.IndexFunc(lines, func(l ledger.Line) bool { return l.Session == "c" && dayfile.DateOf(l.At) == date })]
			l.Request, l.At, l.Pool = "late-"+late, l.At.Add(time.Minute), late
			n.chain.file(date, l)
			n.chain.ledger.SummariseEnded(n.now)
			n.chain.ledger.DigestEnded(t.Context(), n.now)
			if handed := n.chain.digests.handed[date].Sessions["c"]; handed.Base == nil {
				t.Fatalf("the digest of %s wasn't built again of c resting on its state, handed %+v", date, handed)
			}

			for _, pool := range []PoolRead{acmeNine, defaultNine} {
				based, raw := pool.Ledger(n.follower(DigestVersion)), pool.Ledger(n.follower(DigestVersion+1))
				if got, want := ListSessions(SessionSources{Running: n.running, Ledger: based, Prices: ledger.Pricing, Now: n.now}),
					ListSessions(SessionSources{Running: n.running, Ledger: raw, Prices: ledger.Pricing, Now: n.now}); !reflect.DeepEqual(got, want) {
					t.Errorf("in %s, over digests, the List is\n%+v\nwant, as read raw\n%+v", pool.ID, got, want)
				}
				if got, want := n.opened(based, "c", Stream{}), n.opened(raw, "c", Stream{}); !reflect.DeepEqual(got, want) {
					t.Errorf("in %s, over digests, c's page shows\n%+v\nwant, as read raw\n%+v", pool.ID, got, want)
				}
				if got, want := n.stored(based, "c", true, Stream{}), n.stored(raw, "c", true, Stream{}); !reflect.DeepEqual(got, want) {
					t.Errorf("in %s, over digests, c's page held beside the List shows\n%+v\nwant, as read raw\n%+v", pool.ID, got, want)
				}
			}
		})
	}
}

// A session's page in a pool counts as its own its requests there alone,
// its others as another session's, as every request on an account climbs
// its windows, of whichever pool: c's, e's and f's in Acme's, over digests
// or read raw, and as the store holds it, are the pages of their Acme work
// alone, points and all, of a ledger whose c, e and f made their requests in
// Default's under other ids, as apartNine routes them.
func TestAPoolsPageTakesTheSessionsRequestsInThePoolAloneAsItsOwn(t *testing.T) {
	n, apart := newRoutedNineDays(t, pooledNineIDs, pooledNine), newRoutedNineDays(t, pooledNineIDs, apartNine)

	for _, id := range []string{"c", "e", "f"} {
		want := apart.opened(acmeNine.Ledger(apart.follower(DigestVersion+1)), id, Stream{})
		if !pointed(want.page) {
			t.Fatalf("%s's page of its Acme work alone shows no points\n%+v", id, want)
		}
		for _, read := range []struct {
			name string
			page shown
		}{
			{"over digests", n.opened(acmeNine.Ledger(n.follower(DigestVersion)), id, Stream{})},
			{"read raw", n.opened(acmeNine.Ledger(n.follower(DigestVersion+1)), id, Stream{})},
			{"held by the store", n.stored(acmeNine.Ledger(n.follower(DigestVersion)), id, true, Stream{})},
		} {
			if !reflect.DeepEqual(read.page, want) {
				t.Errorf("%s, %s's page in Acme's shows\n%+v\nwant that of its Acme work alone, its Default work another session's\n%+v", read.name, id, read.page, want)
			}
		}
	}
}

// A split session's page in a pool, held across feeds of the ledger's tail
// taken through the pool's Took, as the watch holds it, is at each that of
// its work in the pool alone: c's in Acme's, a request of its in Acme's and
// one in Default's filed before each feed, is the page of a ledger whose c
// made its Default requests under another id, as apartNine routes them, its
// points among it.
func TestASplitSessionsPageHeldAcrossItsPoolsFeedsIsItsWorkThere(t *testing.T) {
	type world struct {
		n     nineDays
		apart bool
		held  *heldOver
		tail  *ledgerTail
	}
	worlds := []*world{{n: newRoutedNineDays(t, pooledNineIDs, pooledNine)}, {n: newRoutedNineDays(t, pooledNineIDs, apartNine), apart: true}}
	at := worlds[0].n.now
	clock := func() time.Time { return at }
	for _, w := range worlds {
		l := ledger.NewFollower(w.n.chain.state, clock, ledger.CapsOf(nil, nil), DigestVersion, quiet)
		w.held, w.tail = heldOn(acmeNine.Ledger(l), "c", at), &ledgerTail{l: l, day: w.n.today.Date, read: acmeNine}
	}
	lines := worlds[0].n.chain.lines
	asked := lines[slices.IndexFunc(lines, func(l ledger.Line) bool { return l.Session == "c" })]

	for i, moment := range []time.Time{at.Add(30 * time.Minute), at.Add(3 * time.Hour)} {
		at = moment
		pages := make([]shown, len(worlds))
		for j, w := range worlds {
			r := rand.New(rand.NewPCG(9, uint64(10+i)))
			for _, pool := range []string{"q7mt", "b2n8"} {
				l := asked
				l.Request, l.At, l.TotalMS, l.Pool = fmt.Sprintf("fed-%d-%s", i, pool), moment.Add(-2*time.Minute), 3000, pool
				if w.apart && pool != "q7mt" {
					l.Session += "-default"
				}
				limitedOf(&l, r, time.Time{})
				w.n.chain.file(dayfile.DateOf(l.At), l)
			}
			pages[j] = w.held.fed(w.tail.at(moment), moment)
		}

		if got, want := pages[0], pages[1]; !reflect.DeepEqual(got, want) || !pointed(want.page) {
			t.Errorf("at %v, c's page in Acme's held across feeds shows\n%+v\nwant that of its Acme work alone, its points among it\n%+v", moment, got, want)
		}
	}
}

// A pool's Took takes the requests of the pool's lines alone, today's and
// the days read again, keeping every line read, of every pool, for a page's
// points, those of another pool as of no session, so none is a session's
// own, leaving the lines given as they are: every pool's Took takes every
// request, and keeps each line as it is.
func TestAPoolsTookTakesItsRequestsAloneAndKeepsEveryLine(t *testing.T) {
	held := func(request, pool string) ledger.Held {
		return ledger.Held{Request: request, Session: "s-" + pool, Kind: ledger.KindMessage, Pool: pool, At: foldDays[0]}
	}
	gained := []ledger.Held{held("1", "q7mt"), held("2", "b2n8"), held("3", "")}
	days := map[string][]ledger.Held{"2026-10-07": {held("4", "q7mt"), held("5", "b2n8")}}
	sessionsOf := func(lines []ledger.Held) []string {
		sessions := make([]string, len(lines))
		for i := range lines {
			sessions[i] = lines[i].Session
		}
		return sessions
	}
	tests := []struct {
		name               string
		read               PoolRead
		gained, before     []string
		owned, ownedBefore []string
	}{
		{
			name: "Acme's", read: acmeNine, gained: []string{"s-q7mt"}, before: []string{"s-q7mt"},
			owned: []string{"s-q7mt", "", ""}, ownedBefore: []string{"s-q7mt", ""},
		},
		{
			name: "Default's", read: defaultNine, gained: []string{"s-", "s-b2n8"}, before: []string{"s-b2n8"},
			owned: []string{"", "s-b2n8", "s-"}, ownedBefore: []string{"", "s-b2n8"},
		},
		{
			name: "every pool's", read: PoolRead{}, gained: []string{"s-", "s-b2n8", "s-q7mt"}, before: []string{"s-b2n8", "s-q7mt"},
			owned: []string{"s-q7mt", "s-b2n8", "s-"}, ownedBefore: []string{"s-q7mt", "s-b2n8"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.read.Took(gained, days)

			if sessions := slices.Sorted(maps.Keys(got.gained)); !slices.Equal(sessions, tt.gained) {
				t.Errorf("took today's requests of %v, want %v", sessions, tt.gained)
			}
			if sessions := slices.Sorted(maps.Keys(got.days["2026-10-07"])); !slices.Equal(sessions, tt.before) {
				t.Errorf("took the day before's requests of %v, want %v", sessions, tt.before)
			}
			if owned := sessionsOf(got.read.gained); !slices.Equal(owned, tt.owned) {
				t.Errorf("kept today's lines for points of the sessions %q, want %q", owned, tt.owned)
			}
			if owned := sessionsOf(got.read.days["2026-10-07"]); !slices.Equal(owned, tt.ownedBefore) {
				t.Errorf("kept the day before's lines for points of the sessions %q, want %q", owned, tt.ownedBefore)
			}
			if given := sessionsOf(gained); !slices.Equal(given, []string{"s-q7mt", "s-b2n8", "s-"}) {
				t.Errorf("left the lines given of the sessions %q, want them as they were", given)
			}
		})
	}
}
