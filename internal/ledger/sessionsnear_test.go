package ledger_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// nearWorld is a lookup world whose sessions a follower reads as of the near
// days, as the List does, at a moment its clock is set to: the sessions each
// day's lines name, by the day's offset from the summaries' day.
type nearWorld struct {
	*lookupWorld
	named map[int][]string
}

// newNearWorld returns a world of the test's own, holding no lines yet.
func newNearWorld(t testing.TB) *nearWorld {
	t.Helper()
	return &nearWorld{lookupWorld: newLookupWorld(t), named: make(map[int][]string)}
}

// file files lines under the local day days after the summaries' day,
// whenever each arrived, noting the sessions they name.
func (w *nearWorld) file(days int, lines ...ledger.Line) {
	w.t.Helper()
	w.lookupWorld.file(days, lines...)
	for _, l := range lines {
		w.named[days] = append(w.named[days], l.CountedSession())
	}
}

// by sets the world's clock to at, and has the router's round summarise, and
// its digester digest, what it would have by then: each day that ended an
// hour or more before, oldest first, digested of the sessions its lines
// name.
func (w *nearWorld) by(at time.Time) {
	w.t.Helper()
	w.at = at
	w.summarise()
	for _, days := range slices.Sorted(maps.Keys(w.named)) {
		if _, end, _ := dayfile.Day(dayOf(days)); !at.Before(end.Add(time.Hour)) {
			w.digest(days, slices.Compact(slices.Sorted(slices.Values(w.named[days])))...)
		}
	}
}

// follower is a follower of the world's ledger by its clock, its sessions
// resting on the states its digests hold.
func (w *nearWorld) follower() *ledger.Follower {
	return followerOf(w.state, &testClock{at: w.at}, caps)
}

// takenDays is what take was handed: the requests of the lines of each day,
// by its date, each with the times it was handed.
type takenDays map[string]map[string]int

// take notes l, a line of the day with the given date, handed take.
func (d takenDays) take(date string, l ledger.Line) {
	if d[date] == nil {
		d[date] = make(map[string]int)
	}
	d[date][l.Request]++
}

// dates are the dates of the days whose lines take was handed, oldest first.
func (d takenDays) dates() []string {
	return slices.Sorted(maps.Keys(d))
}

// on is what take was handed of the days with the given dates alone.
func (d takenDays) on(dates []string) takenDays {
	on := takenDays{}
	for _, date := range dates {
		if requests, ok := d[date]; ok {
			on[date] = requests
		}
	}
	return on
}

// nearDays files the lines the tests of a follower's sessions share, the
// summaries' day yesterday and the day after it today: one on each of the
// three days before today, and today; two two days before, and today; three
// today alone; four the day before yesterday alone, asked for; and five two
// days before alone, never asked for.
func nearDays(w *nearWorld) {
	w.file(-2, askedBy("1", "one", on(-2, 9, 0)), askedBy("5", "two", on(-2, 10, 0)), askedBy("9", "five", on(-2, 11, 0)))
	w.file(-1, askedBy("2", "one", on(-1, 9, 0)), askedBy("8", "four", on(-1, 10, 0)))
	w.file(0, askedBy("3", "one", on(0, 9, 0)))
	w.file(1, askedBy("7", "three", on(1, 0, 20)), askedBy("4", "one", on(1, 0, 30)), askedBy("6", "two", on(1, 0, 40)))
}

// isRead fails t where got, the sessions a follower's read at at found,
// aren't want.
func isRead(t *testing.T, at time.Time, got, want map[string]found) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("at %s, the sessions read are\n%+v\nwant\n%+v", at.Format(time.DateTime), got, want)
	}
}

// Today's sessions, and those asked for, rest each on the newest state the
// digest of a day before the near days holds of it, their lines those of
// the days naming them after it, and the near days', every one that arrived
// by now: yesterday's files read raw, every session's, till its digest
// stands, an hour after it ends; then only for a session it names resting on
// an older state, as none here does. What a state folded from them rests on
// can't be told: today's lines go on growing.
func TestSessionsRestOnTheDigestsOfTheDaysBeforeTheNearDays(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		want map[string]found
		read []string
	}{
		{
			name: "at 23:59, yesterday digested",
			at:   on(0, 23, 59),
			want: map[string]found{
				"one":  {base: stateAt("one", dayOf(-1)), requests: []string{"3"}},
				"four": {base: stateAt("four", dayOf(-1)), requests: []string{}},
			},
			read: []string{dayOf(0)},
		},
		{
			name: "at 00:01, yesterday not yet summarised, read raw",
			at:   on(1, 0, 1),
			want: map[string]found{"four": {base: stateAt("four", dayOf(-1)), requests: []string{}}},
			read: []string{dayOf(0)},
		},
		{
			name: "at 00:59, yesterday not yet summarised, read raw",
			at:   on(1, 0, 59),
			want: map[string]found{
				"one":   {base: stateAt("one", dayOf(-1)), requests: []string{"3", "4"}},
				"two":   {base: stateAt("two", dayOf(-2)), requests: []string{"6"}},
				"three": {requests: []string{"7"}},
				"four":  {base: stateAt("four", dayOf(-1)), requests: []string{}},
			},
			read: []string{dayOf(0), dayOf(1)},
		},
		{
			name: "at 01:30, yesterday digested",
			at:   on(1, 1, 30),
			want: map[string]found{
				"one":   {base: stateAt("one", dayOf(0)), requests: []string{"4"}},
				"two":   {base: stateAt("two", dayOf(-2)), requests: []string{"6"}},
				"three": {requests: []string{"7"}},
				"four":  {base: stateAt("four", dayOf(-1)), requests: []string{}},
			},
			read: []string{dayOf(1)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newNearWorld(t)
			nearDays(w)
			w.by(tt.at)
			taken := takenDays{}

			got := foundOf(t, w.follower().Sessions([]string{"four"}, nil, taken.take))

			isRead(t, tt.at, got, tt.want)
			if dates := taken.dates(); !slices.Equal(dates, tt.read) {
				t.Errorf("at %s, take was handed the lines of %q, want those of %q alone", tt.at.Format(time.DateTime), dates, tt.read)
			}
		})
	}
}

// A line of today's files that arrived before the day a session's base rests
// on ended, filed under today's date as after a change of time zone, leaves
// the session on none, its lines every one, by the lookup's own rule for the
// lines after a base.
func TestANearDaysLineArrivingBeforeItsSessionsBaseEndedLeavesItOnNone(t *testing.T) {
	w := newNearWorld(t)
	nearDays(w)
	w.file(1, askedBy("early", "one", on(0, 23, 0)))
	w.by(on(1, 1, 30))

	got := foundOf(t, w.follower().Sessions(nil, nil, nil))

	isRead(t, w.at, got, map[string]found{
		"one":   {requests: []string{"1", "2", "3", "early", "4"}},
		"two":   {base: stateAt("two", dayOf(-2)), requests: []string{"6"}},
		"three": {requests: []string{"7"}},
	})
}

// A session whole reports true of is read whole, resting on none, its lines
// every one, though a digest holds a state of it that stands; the others
// rest on theirs.
func TestASessionAskedWholeRestsOnNone(t *testing.T) {
	w := newNearWorld(t)
	nearDays(w)
	w.by(on(1, 1, 30))

	got := foundOf(t, w.follower().Sessions([]string{"four"}, func(id string) bool { return id == "one" }, nil))

	isRead(t, w.at, got, map[string]found{
		"one":   {requests: []string{"1", "2", "3", "4"}},
		"two":   {base: stateAt("two", dayOf(-2)), requests: []string{"6"}},
		"three": {requests: []string{"7"}},
		"four":  {base: stateAt("four", dayOf(-1)), requests: []string{}},
	})
}

// SessionsOf gives the sessions asked for alone, as Sessions gives them: the
// near days' files read for them, take handed every line of those days, of
// whichever session.
func TestSessionsOfGivesThoseAskedForAloneOnTheirDigests(t *testing.T) {
	w := newNearWorld(t)
	nearDays(w)
	w.by(on(1, 1, 30))
	taken := takenDays{}

	got := foundOf(t, w.follower().SessionsOf([]string{"one", "four"}, nil, taken.take))

	isRead(t, w.at, got, map[string]found{
		"one":  {base: stateAt("one", dayOf(0)), requests: []string{"4"}},
		"four": {base: stateAt("four", dayOf(-1)), requests: []string{}},
	})
	if want := (takenDays{dayOf(1): {"4": 1, "6": 1, "7": 1}}); !reflect.DeepEqual(taken, want) {
		t.Errorf("SessionsOf() handed take %v, want %v: every line of today's files, once", taken, want)
	}
}

// Yesterday's lines are handed to take while it's a near day, whatever
// sessions it names; once its digest stands, only where a session read
// reads it: one it names asked for whole, or resting on an older state. A
// raw read hands take yesterday's lines all the same.
func TestYesterdaysLinesAreTakenWhileItsNearOrASessionReadReadsIt(t *testing.T) {
	tests := []struct {
		name  string
		at    time.Time
		ids   []string
		whole []string
		read  []string
	}{
		{name: "at 00:59, yesterday near, naming none asked for", at: on(1, 0, 59), ids: []string{"four"}, read: []string{dayOf(0), dayOf(1)}},
		{name: "at 01:30, yesterday digested, naming none asked for", at: on(1, 1, 30), ids: []string{"four"}, read: []string{dayOf(1)}},
		{name: "at 01:30, yesterday digested, naming one asked for resting on it", at: on(1, 1, 30), ids: []string{"one"}, read: []string{dayOf(1)}},
		{
			name: "at 01:30, yesterday digested, naming one asked for whole", at: on(1, 1, 30), ids: []string{"one"}, whole: []string{"one"},
			read: []string{dayOf(-2), dayOf(-1), dayOf(0), dayOf(1)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newNearWorld(t)
			nearDays(w)
			w.by(tt.at)
			taken := takenDays{}

			w.follower().SessionsOf(tt.ids, func(id string) bool { return slices.Contains(tt.whole, id) }, taken.take)

			if dates := taken.dates(); !slices.Equal(dates, tt.read) {
				t.Errorf("SessionsOf(%q), %q whole, handed take the lines of %q, want those of %q", tt.ids, tt.whole, dates, tt.read)
			}
		})
	}
}

// A follower holds the head of a day's digest while its file looks as it did
// when it was read, opening it no more, and reads it afresh once it's
// written again: a digest of yesterday's of another version leaves it a near
// day, read raw.
func TestAFollowerHoldsADigestsHeadWhileItsFileLooksAsItDid(t *testing.T) {
	w := newNearWorld(t)
	nearDays(w)
	w.by(on(1, 1, 30))
	follower := w.follower()
	read := func() []string {
		taken := takenDays{}
		follower.SessionsOf([]string{"four"}, eachWhole, taken.take)
		return taken.dates()
	}
	digested := []string{dayOf(-1), dayOf(1)}
	if dates := read(); !slices.Equal(dates, digested) {
		t.Fatalf("SessionsOf() handed take the lines of %q, want those of %q: four's day, and today's, yesterday's digest standing", dates, digested)
	}
	unopenable(t, w.dir, "digest-*")

	if dates := read(); !slices.Equal(dates, digested) {
		t.Errorf("read again, its digests open to no one, SessionsOf() handed take the lines of %q, want those of %q: their heads held", dates, digested)
	}
	if err := ledger.WriteDigest(w.dir, ledger.Digest{Head: ledger.DigestHead{Body: digestBody + 1, Day: dayOf(0)}}); err != nil {
		t.Fatal(err)
	}
	if dates, want := read(), []string{dayOf(-1), dayOf(0), dayOf(1)}; !slices.Equal(dates, want) {
		t.Errorf("yesterday's digest written again, of another version, SessionsOf() handed take the lines of %q, want those of %q: yesterday read raw", dates, want)
	}
}

// A follower of another version of the sessions' states than the digests
// hold rests no session on one, reading yesterday's files raw too, and each
// session's every day.
func TestAFollowerOfAnotherVersionRestsNoSessionOnADigest(t *testing.T) {
	w := newNearWorld(t)
	nearDays(w)
	w.by(on(1, 1, 30))
	taken := takenDays{}
	other := ledger.NewFollower(w.state, (&testClock{at: w.at}).now, caps, digestBody+1, logs.For("cli"))

	got := foundOf(t, other.Sessions([]string{"four"}, nil, taken.take))

	isRead(t, w.at, got, map[string]found{
		"one":   {requests: []string{"1", "2", "3", "4"}},
		"two":   {requests: []string{"5", "6"}},
		"three": {requests: []string{"7"}},
		"four":  {requests: []string{"8"}},
	})
	if dates, want := taken.dates(), []string{dayOf(-2), dayOf(-1), dayOf(0), dayOf(1)}; !slices.Equal(dates, want) {
		t.Errorf("take was handed the lines of %q, want those of %q: yesterday's and each session's every day", dates, want)
	}
}

// Over nine days, each session resting on yesterday's digest, a read of
// today's sessions opens today's files alone: none of the days before.
func TestSessionsRestingOnYesterdaysDigestOpenTodaysFilesAlone(t *testing.T) {
	log := logstest.Capture(t)
	w := newNearWorld(t)
	for days := -8; days <= 0; days++ {
		w.file(days, askedBy(fmt.Sprintf("long%d", days), "long", on(days, 9, 0)), askedBy(fmt.Sprintf("busy%d", days), "busy", on(days, 10, 0)))
	}
	w.file(1, askedBy("long", "long", on(1, 0, 30)), askedBy("busy", "busy", on(1, 0, 40)))
	w.by(on(1, 1, 30))
	for days := -8; days <= 0; days++ {
		unopenable(t, w.dir, "requests-"+dayOf(days)+"*")
	}
	taken := takenDays{}

	got := foundOf(t, w.follower().Sessions(nil, nil, taken.take))

	isRead(t, w.at, got, map[string]found{
		"long": {base: stateAt("long", dayOf(0)), requests: []string{"long"}},
		"busy": {base: stateAt("busy", dayOf(0)), requests: []string{"busy"}},
	})
	if n, dates := opened(log), taken.dates(); n != 0 || !slices.Equal(dates, []string{dayOf(1)}) {
		t.Errorf("log reads\n%s\nthe read opened %d files of the days before today, and handed take the lines of %q; want none opened, and today's alone",
			log, n, dates)
	}
}

// The moments a generated ledger's sessions are read at: about the turn of
// the day after its last day's, and the next day's morning.
var turnMoments = []time.Time{on(1, 23, 59), on(2, 0, 1), on(2, 0, 59), on(2, 1, 30), on(3, 10, 0)}

// Over a generated ledger, its days digested as the router's digester would,
// then changed since, a follower's sessions read each whole are what a
// follower reading no digest gives, take handed, of each day the whole read
// reads, the lines that raw read hands it: the raw read reads yesterday's
// too where its digest stands and it names none of them; and read on
// their digests, each is what that raw read gives it: its base's state
// folded from exactly those filed under the base's day and before, and its
// lines those filed after, none of which arrived before the base's day
// ended.
func TestAFollowersSessionsAreWhatARawReadGivesOverAGeneratedLedger(t *testing.T) {
	for seed := range uint64(24) {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			g := newGeneratedLedger(t, seed)
			g.generate()
			g.digestEach()
			g.upset()
			for _, at := range turnMoments {
				clock := &testClock{at: at}
				rawTaken, wholeTaken := takenDays{}, takenDays{}
				raw := ledger.NewFollower(g.w.state, clock.now, caps, digestBody+1, logs.For("cli")).Sessions(generatedSessions, eachWhole, rawTaken.take)
				follower := followerOf(g.w.state, clock, caps)

				whole := follower.Sessions(generatedSessions, eachWhole, wholeTaken.take)
				if rawOn := rawTaken.on(wholeTaken.dates()); !reflect.DeepEqual(whole, raw) || !reflect.DeepEqual(wholeTaken, rawOn) {
					t.Errorf("seed %d, at %s: read whole, the sessions are\n%+v\nand take was handed %v; want them as a follower reading no digest gives them\n%+v\nhanded %v of those days",
						seed, at.Format(time.DateTime), whole, wholeTaken, raw, rawOn)
				}
				g.restOnARawRead(at, follower.Sessions(generatedSessions, nil, nil), raw)
			}
		})
	}
}

// restOnARawRead fails the test where a session of sessions, read at at
// resting on its digests, isn't as raw, the sessions read whole, gives it:
// its base's state folded from exactly those of its requests filed under the
// base's day and before, its lines those filed after, none of which arrived
// before the base's day ended.
func (g *generatedLedger) restOnARawRead(at time.Time, sessions, raw map[string]ledger.SessionFrom) {
	t := g.w.t
	t.Helper()
	when := fmt.Sprintf("seed %d, at %s", g.seed, at.Format(time.DateTime))
	if got, want := slices.Sorted(maps.Keys(sessions)), slices.Sorted(maps.Keys(raw)); !slices.Equal(got, want) {
		t.Errorf("%s: the sessions read are %q; want %q, as a raw read gives them", when, got, want)
	}
	for id, whole := range raw {
		from, base := sessions[id], ""
		if from.Base != nil {
			base = from.Base.Date
		}
		var before, after []string
		for _, request := range requestsOf(whole.Lines) {
			if g.filed[request] <= base {
				before = append(before, request)
			} else {
				after = append(after, request)
			}
		}
		if folded, lines := baseOf(t, from.Base).Requests, requestsOf(from.Lines); !slices.Equal(folded, before) || !slices.Equal(lines, after) {
			t.Errorf("%s: session %s rests on its state as of %q, folded from %q, its lines %q; want it folded from %q, its lines %q, as a raw read of %q has them",
				when, id, base, folded, lines, before, after, requestsOf(whole.Lines))
		}
		if _, ended, _ := dayfile.Day(base); from.Base != nil && slices.ContainsFunc(from.Lines, func(l ledger.Line) bool { return l.At.Before(ended) }) {
			t.Errorf("%s: session %s rests on its state as of %s, its lines %q among them one that arrived before that day ended; want none",
				when, id, base, requestsOf(from.Lines))
		}
	}
}
