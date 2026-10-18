package ledger_test

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// digestBody is the version of the sessions' states the lookup's tests rest
// sessions on, as their builder would give it.
const digestBody = 4

// dayOf is the date of the local day days after the summaries' day.
func dayOf(days int) string {
	return on(days, 12, 0).Format(time.DateOnly)
}

// lookupWorld is a ledger the lookup's tests look sessions up in, as of a
// day: the lines each test files under its days, summarised, and digested,
// as the router's round and digester would, by a clock stopped at at, two
// days after the last day the tests file lines under.
type lookupWorld struct {
	t                   *testing.T
	state, dir, history string
	at                  time.Time
	writer              *ledger.Ledger
}

// newLookupWorld returns a world of the test's own, holding no lines yet.
func newLookupWorld(t *testing.T) *lookupWorld {
	t.Helper()
	state, dir, history := stateDirs(t)
	at := on(3, 10, 0)
	writer := ledger.Open(dir, 400*24*time.Hour, func() time.Time { return at }, historyIn(history), caps, logs.For("router"))
	return &lookupWorld{t: t, state: state, dir: dir, history: history, at: at, writer: writer}
}

// file files lines under the local day days after the summaries' day,
// whenever each arrived.
func (w *lookupWorld) file(days int, lines ...ledger.Line) {
	w.t.Helper()
	holdLines(w.t, w.dir, dayOf(days), lines...)
}

// summarise summarises the days that have ended and have no summary that
// stands, as the router's round does.
func (w *lookupWorld) summarise() {
	summariseAt(w.dir, w.history, w.at)
}

// digest writes the digest of the local day days after the summaries' day,
// as the router's digester would: its head of the day as the ledger keeps
// it, and an entry of each of the sessions with the given ids, as the
// writer's lookup finds it as of the day's end, of the state stateOf gives,
// resting on what the lookup says a state folded from it rests on.
func (w *lookupWorld) digest(days int, ids ...string) {
	w.t.Helper()
	date := dayOf(days)
	lines, known := w.writer.Kept(date)
	if !known {
		w.t.Fatalf("the digest of %s: the ledger can't tell the lines it keeps of the day", date)
	}
	start, end, _ := dayfile.Day(date)
	digest := ledger.Digest{
		Head:     ledger.DigestHead{Body: digestBody, Day: date, Start: start, End: end, SummaryLines: lines, Lines: lines},
		Sessions: make(map[string]ledger.DigestEntry),
	}
	sessions := w.writer.SessionsAsOf(ids, dayOf(days+1), digestBody, w.at)
	for _, id := range ids {
		from, ok := sessions[id]
		if !ok || from.RestsOn() == nil {
			w.t.Fatalf("the digest of %s: session %s is found %t, resting on %v; want it found, resting on days it can tell", date, id, ok, from.RestsOn())
		}
		digest.Sessions[id] = ledger.DigestEntry{RestsOn: from.RestsOn(), State: stateOf(id, date)}
	}
	if err := ledger.WriteDigest(w.dir, digest); err != nil {
		w.t.Fatal(err)
	}
}

// stateOf is the state the lookup's tests fold the session with the given
// id into as of the end of the local day with the given date: the two,
// told, as stateAt tells them.
func stateOf(id, date string) json.RawMessage {
	return json.RawMessage(strconv.Quote(stateAt(id, date)))
}

// stateAt tells the session with the given id as of the end of the local day
// with the given date.
func stateAt(id, date string) string {
	return id + "@" + date
}

// asOf returns the sessions with the given ids as of the local day days after
// the summaries' day, as the follower's lookup finds them, as found tells
// them, failing the test where the router's writer's lookup finds them
// otherwise.
func (w *lookupWorld) asOf(days int, ids ...string) map[string]found {
	w.t.Helper()
	date := dayOf(days)
	read := followerOf(w.state, &testClock{at: w.at}, caps).SessionsAsOf(ids, date, digestBody, nil)
	if written := w.writer.SessionsAsOf(ids, date, digestBody, w.at); !reflect.DeepEqual(read, written) {
		w.t.Errorf("as of %s, the follower finds %q as\n%+v\nand the router's writer as\n%+v; want them alike", date, ids, read, written)
	}
	return foundOf(w.t, read)
}

// found is a session as the lookup finds it, as the tests tell it: the state
// it rests on, as stateAt tells it, "" for none; the requests of its lines,
// in order; and what a state folded from it rests on.
type found struct {
	base     string
	requests []string
	restsOn  map[string]int
}

// foundOf tells each of sessions as found has it, by its id, failing the
// test where one rests on a state its digest's day doesn't tell.
func foundOf(t *testing.T, sessions map[string]ledger.SessionFrom) map[string]found {
	t.Helper()
	all := make(map[string]found, len(sessions))
	for id, s := range sessions {
		f := found{requests: requestsOf(s.Lines), restsOn: s.RestsOn()}
		if s.Base != nil {
			if err := json.Unmarshal(s.Base.State, &f.base); err != nil {
				t.Fatal(err)
			}
			if f.base != stateAt(id, s.Base.Date) {
				t.Errorf("session %s rests on the state %q, as of %s; want the state of its own as of that day", id, f.base, s.Base.Date)
			}
		}
		all[id] = f
	}
	return all
}

// isFound fails t where got, the sessions a lookup as of the local day days
// after the summaries' day found, aren't want.
func isFound(t *testing.T, days int, got, want map[string]found) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("as of %s, the sessions found are\n%+v\nwant\n%+v", dayOf(days), got, want)
	}
}

// sharedDays files the lines of sessions one and two the lookup's tests
// share, a day of none between them, and three's first, and summarises them:
// one on the 2nd, the 5th and the 6th; two on the 2nd, the 3rd and the 5th;
// three on the 6th; none on the 4th.
func sharedDays(w *lookupWorld) {
	w.file(-3, askedBy("1", "one", on(-3, 9, 0)), askedBy("2", "two", on(-3, 10, 0)))
	w.file(-2, askedBy("3", "two", on(-2, 9, 0)))
	w.file(0, askedBy("4", "one", on(0, 9, 0)), askedBy("5", "two", on(0, 10, 0)))
	w.file(1, askedBy("6", "one", on(1, 9, 0)), askedBy("7", "three", on(1, 10, 0)))
	w.summarise()
}

// A session rests on the newest state a digest of a day naming it holds that
// stands, a day of no lines before it, and its lines are those of the days
// naming it after that one; one no digest holds rests on none, its lines
// every one; and one no day names isn't found. What a state folded from each
// rests on is every day naming it, with the lines its summary counts.
func TestASessionRestsOnTheNewestStateADayNamingItHolds(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digest(-3, "one", "two")
	w.digest(-2, "two")
	w.digest(0, "one", "two")

	got := w.asOf(2, "one", "two", "three", "four")

	isFound(t, 2, got, map[string]found{
		"one":   {base: stateAt("one", dayOf(0)), requests: []string{"6"}, restsOn: map[string]int{dayOf(-3): 2, dayOf(0): 2, dayOf(1): 2}},
		"two":   {base: stateAt("two", dayOf(0)), requests: []string{}, restsOn: map[string]int{dayOf(-3): 2, dayOf(-2): 1, dayOf(0): 2}},
		"three": {requests: []string{"7"}, restsOn: map[string]int{dayOf(1): 2}},
	})
}

// Where the newest digest of a day naming a session doesn't stand, as its
// day was summarised again since, of a line filed late, the session rests on
// an older one that does, its lines those of the days after it.
func TestASessionRestsOnAnOlderStateWhereTheNewestDoesntStand(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digest(-3, "one", "two")
	w.digest(-2, "two")
	w.digest(0, "one", "two")
	w.file(0, askedBy("8", "two", on(0, 11, 0)))
	w.summarise()

	got := w.asOf(2, "one", "two")

	isFound(t, 2, got, map[string]found{
		"one": {base: stateAt("one", dayOf(-3)), requests: []string{"4", "6"}, restsOn: map[string]int{dayOf(-3): 2, dayOf(0): 3, dayOf(1): 2}},
		"two": {base: stateAt("two", dayOf(-2)), requests: []string{"5", "8"}, restsOn: map[string]int{dayOf(-3): 2, dayOf(-2): 1, dayOf(0): 3}},
	})
}

// A state resting on a day summarised again since, of more lines, doesn't
// stand, though its own day's digest does: the session rests on none whose
// state rests on that day, its lines every one, the late line among them.
func TestAStateRestingOnADaySummarisedAgainDoesntStand(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digest(-3, "one", "two")
	w.digest(-2, "two")
	w.digest(0, "one", "two")
	w.digest(1, "one")
	w.file(-3, askedBy("9", "one", on(-3, 11, 0)))
	w.summarise()

	got := w.asOf(2, "one")

	isFound(t, 2, got, map[string]found{
		"one": {requests: []string{"1", "9", "4", "6"}, restsOn: map[string]int{dayOf(-3): 3, dayOf(0): 2, dayOf(1): 2}},
	})
}

// A state resting on a day whose lines are pruned since doesn't stand, its
// summary standing all the same, as a read of the session's lines no longer
// reads that day's; the state folded afresh rests on the day as keeping
// none, and stands.
func TestAStateRestingOnADayPrunedSinceStandsOnceFoldedAfresh(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digest(-3, "one", "two")
	w.digest(-2, "two")
	w.digest(0, "one", "two")
	w.digest(1, "one")
	removeFile(t, w.dir, "requests-"+dayOf(-3)+".jsonl")
	pruned := map[string]int{dayOf(-3): 0, dayOf(0): 2, dayOf(1): 2}

	isFound(t, 2, w.asOf(2, "one"), map[string]found{"one": {requests: []string{"4", "6"}, restsOn: pruned}})

	w.digest(1, "one")

	isFound(t, 2, w.asOf(2, "one"), map[string]found{"one": {base: stateAt("one", dayOf(1)), requests: []string{}, restsOn: pruned}})
}

// A line of a session filed late under a day it never ran on, as after a
// clock set back, has the day's summary name it: a state that doesn't rest
// on that day doesn't stand, though all it rests on does, and the session
// rests on the newest that does, before the day, its lines those after.
func TestAStateNotRestingOnEachDayNamingItsSessionDoesntStand(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digest(-3, "one", "two")
	w.digest(-2, "two")
	w.digest(0, "one", "two")
	w.digest(1, "one")
	w.file(-2, askedBy("9", "one", on(-2, 11, 0)))
	w.summarise()

	got := w.asOf(2, "one")

	isFound(t, 2, got, map[string]found{
		"one": {base: stateAt("one", dayOf(-3)), requests: []string{"9", "4", "6"}, restsOn: map[string]int{dayOf(-3): 2, dayOf(-2): 2, dayOf(0): 2, dayOf(1): 2}},
	})
}

// The lines of a day's files are read with it, those that arrived before it
// began, as after a change of time zone, among them, in the order they
// arrived.
func TestALineArrivingBeforeItsDayIsReadWithIt(t *testing.T) {
	w := newLookupWorld(t)
	w.file(-3, askedBy("1", "one", on(-3, 9, 0)))
	w.file(0, askedBy("2", "one", on(0, 9, 0)), askedBy("early", "one", on(-1, 22, 0)))
	w.summarise()
	w.digest(-3, "one")

	got := w.asOf(2, "one")

	isFound(t, 2, got, map[string]found{
		"one": {base: stateAt("one", dayOf(-3)), requests: []string{"early", "2"}, restsOn: map[string]int{dayOf(-3): 1, dayOf(0): 2}},
	})
}

// A session one of whose lines after its base arrived before the base's day
// ended, filed under the day after it, as after a change of time zone, rests
// on none, its lines every one: a state sealed at the day's end can't take
// it in.
func TestALineAfterABaseArrivingBeforeItsDayEndedLeavesItsSessionOnNone(t *testing.T) {
	w := newLookupWorld(t)
	w.file(-3, askedBy("1", "one", on(-3, 9, 0)))
	w.file(-2, askedBy("2", "one", on(-2, 9, 0)))
	w.file(-1, askedBy("3", "one", on(-1, 9, 0)), askedBy("early", "one", on(-2, 22, 0)))
	w.summarise()
	w.digest(-3, "one")
	w.digest(-2, "one")

	got := w.asOf(2, "one")

	isFound(t, 2, got, map[string]found{
		"one": {requests: []string{"1", "2", "early", "3"}, restsOn: map[string]int{dayOf(-3): 1, dayOf(-2): 1, dayOf(-1): 2}},
	})
}

// A session a day naming it can't be told of, its summary not standing as
// lines were filed under it since, rests on none, its lines every one, and
// what a state folded from it rests on can't be told.
func TestASessionADayNamingCantBeToldOfRestsOnNone(t *testing.T) {
	w := newLookupWorld(t)
	w.file(-3, askedBy("1", "one", on(-3, 9, 0)))
	w.file(0, askedBy("2", "one", on(0, 9, 0)))
	w.summarise()
	w.digest(-3, "one")
	w.file(0, askedBy("3", "one", on(0, 10, 0)))

	got := w.asOf(2, "one")

	isFound(t, 2, got, map[string]found{"one": {requests: []string{"1", "2", "3"}}})
}

// A session one of whose days after its base can't be read rests on none,
// its lines those that can be, which is warned of, and what a state folded
// from it rests on can't be told.
func TestASessionADayAfterItsBaseCantBeReadOfRestsOnNone(t *testing.T) {
	log := logstest.Capture(t)
	w := newLookupWorld(t)
	w.file(-3, askedBy("1", "one", on(-3, 9, 0)))
	w.file(0, askedBy("2", "one", on(0, 9, 0)))
	w.summarise()
	w.digest(-3, "one")
	unopenable(t, w.dir, "requests-"+dayOf(0)+".jsonl")

	got := w.asOf(2, "one")

	isFound(t, 2, got, map[string]found{"one": {requests: []string{"1"}}})
	if want := []string{"level=WARN", `msg="can't read the request ledger"`, "day=" + dayOf(0)}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant the day that couldn't be read warned of: a line with %q", log, want)
	}
}

// A lookup of several sessions sharing days reads each day's files once,
// for every session wanting them, a day read again for a session resting on
// none once its base falls, and none of the days before every session's
// base.
func TestALookupReadsEachDayOnceForEverySessionWantingIt(t *testing.T) {
	w := newLookupWorld(t)
	w.file(-3, askedBy("1", "one", on(-3, 9, 0)), askedBy("2", "two", on(-3, 10, 0)))
	w.file(-2, askedBy("3", "two", on(-2, 9, 0)), askedBy("4", "three", on(-2, 10, 0)))
	w.file(-1, askedBy("10", "three", on(-2, 22, 0)))
	w.file(0, askedBy("5", "one", on(0, 9, 0)), askedBy("6", "two", on(0, 10, 0)), askedBy("7", "three", on(0, 11, 0)))
	w.file(1, askedBy("8", "one", on(1, 9, 0)), askedBy("9", "three", on(1, 10, 0)))
	w.summarise()
	w.digest(-3, "one", "two")
	w.digest(-2, "two", "three")
	read := make(map[string]map[string]int)
	take := func(date string, l ledger.Line) {
		if read[date] == nil {
			read[date] = make(map[string]int)
		}
		read[date][l.Request]++
	}

	got := foundOf(t, followerOf(w.state, &testClock{at: w.at}, caps).SessionsAsOf([]string{"one", "two", "three"}, dayOf(2), digestBody, take))

	for date, requests := range read {
		for request, times := range requests {
			if times != 1 {
				t.Errorf("request %s, of %s, was read %d times, want once", request, date, times)
			}
		}
	}
	if dates, want := slices.Sorted(maps.Keys(read)), []string{dayOf(-2), dayOf(-1), dayOf(0), dayOf(1)}; !slices.Equal(dates, want) {
		t.Errorf("the days read are %q, want %q: those after each session's base, and three's before it, its base fallen", dates, want)
	}
	isFound(t, 2, got, map[string]found{
		"one":   {base: stateAt("one", dayOf(-3)), requests: []string{"5", "8"}, restsOn: map[string]int{dayOf(-3): 2, dayOf(0): 3, dayOf(1): 2}},
		"two":   {base: stateAt("two", dayOf(-2)), requests: []string{"6"}, restsOn: map[string]int{dayOf(-3): 2, dayOf(-2): 2, dayOf(0): 3}},
		"three": {requests: []string{"4", "10", "7", "9"}, restsOn: map[string]int{dayOf(-2): 2, dayOf(-1): 1, dayOf(0): 3, dayOf(1): 2}},
	})
}

// What the lookup's looks give of the lines the ledger keeps of a day, the
// follower's and the router's writer's alike: those its summary counts,
// where it stands; none of a day without files, its lines pruned, though its
// summary stands, or never written; and none known of one whose summary
// doesn't stand, as lines were filed under it since, or that has none.
func TestTheLinesKeptOfADayAreThoseItsFilesHoldAsItsSummaryCountsThem(t *testing.T) {
	w := newLookupWorld(t)
	w.file(-3, askedBy("1", "one", on(-3, 9, 0)), askedBy("2", "two", on(-3, 10, 0)))
	w.file(-2, askedBy("3", "one", on(-2, 9, 0)))
	w.file(0, askedBy("4", "one", on(0, 9, 0)))
	w.summarise()
	removeFile(t, w.dir, "requests-"+dayOf(-2)+".jsonl")
	w.file(0, askedBy("5", "one", on(0, 10, 0)))
	w.file(1, askedBy("6", "one", on(1, 9, 0)))
	follower := followerOf(w.state, &testClock{at: w.at}, caps)
	tests := []struct {
		name  string
		days  int
		lines int
		known bool
	}{
		{name: "a day whose summary stands", days: -3, lines: 2, known: true},
		{name: "a day whose lines are pruned, its summary standing", days: -2, lines: 0, known: true},
		{name: "a day of no lines and no summary", days: -1, lines: 0, known: true},
		{name: "a day whose summary doesn't stand, its lines filed since", days: 0, lines: 0, known: false},
		{name: "a day of lines and no summary", days: 1, lines: 0, known: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			date := dayOf(tt.days)
			if lines, known := follower.Kept(date); lines != tt.lines || known != tt.known {
				t.Errorf("the follower's Kept(%s) = %d, %t; want %d, %t", date, lines, known, tt.lines, tt.known)
			}
			if lines, known := w.writer.Kept(date); lines != tt.lines || known != tt.known {
				t.Errorf("the router's writer's Kept(%s) = %d, %t; want %d, %t", date, lines, known, tt.lines, tt.known)
			}
		})
	}
}
