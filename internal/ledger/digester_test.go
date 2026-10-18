package ledger_test

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// folding is the Digester the digester's tests digest with: each session's
// state folded as foldedFrom folds it, but failing as a Digester fails where
// its base doesn't read, as it may be asked on the digester's goroutine;
// first doing what during does, where it's set, failing where that fails;
// and noting each day it's handed, in the order handed, as handedDay tells
// it.
type folding struct {
	mu     sync.Mutex
	during func(ctx context.Context, day ledger.DigestDay) error
	handed []handedDay
}

// handedDay is a day as the tests' Digester was handed it: when, by the
// clock; its date; and each session's, by its id, as handedSession tells it.
type handedDay struct {
	at       time.Time
	date     string
	sessions map[string]handedSession
}

// handedSession is a session as the tests' Digester was handed it: the date
// of the state it rests on, "" for none, and the requests of its lines.
type handedSession struct {
	base     string
	requests []string
}

// errFolding is the error the tests' Digester fails with where a test has
// it fail.
var errFolding = errors.New("can't fold the day")

// Version is the version of the states the tests' Digester gives.
func (f *folding) Version() int {
	return digestBody
}

// Digest notes day as handed, does what during does, where it's set, and
// gives each of its sessions' states.
func (f *folding) Digest(ctx context.Context, day ledger.DigestDay) (map[string]json.RawMessage, error) {
	f.mu.Lock()
	during := f.during
	f.handed = append(f.handed, handedOf(day))
	f.mu.Unlock()
	if during != nil {
		if err := during(ctx, day); err != nil {
			return nil, err
		}
	}
	states := make(map[string]json.RawMessage, len(day.Sessions))
	for id, s := range day.Sessions {
		var base testState
		if s.Base != nil {
			if err := json.Unmarshal(s.Base.State, &base); err != nil {
				return nil, err
			}
		}
		state, err := json.Marshal(testState{Of: stateAt(id, day.Date), Requests: append(base.Requests, requestsOf(s.Lines)...)})
		if err != nil {
			return nil, err
		}
		states[id] = state
	}
	return states, nil
}

// handedOf is day as handedDay tells it, handed now.
func handedOf(day ledger.DigestDay) handedDay {
	handed := handedDay{at: time.Now(), date: day.Date, sessions: make(map[string]handedSession, len(day.Sessions))}
	for id, s := range day.Sessions {
		var base string
		if s.Base != nil {
			base = s.Base.Date
		}
		handed.sessions[id] = handedSession{base: base, requests: requestsOf(s.Lines)}
	}
	return handed
}

// doing has the tests' Digester do what during does, as it's handed each
// day from then on.
func (f *folding) doing(during func(ctx context.Context, day ledger.DigestDay) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.during = during
}

// days returns the days the tests' Digester was handed, in the order handed,
// letting go of them.
func (f *folding) days() []handedDay {
	f.mu.Lock()
	defer f.mu.Unlock()
	handed := f.handed
	f.handed = nil
	return handed
}

// datesOf returns the dates of the days handed, in the order handed.
func datesOf(handed []handedDay) []string {
	dates := make([]string, len(handed))
	for i, h := range handed {
		dates[i] = h.date
	}
	return dates
}

// failingOn returns what fails the day with the given date, as the tests'
// Digester is to, and none of the others.
func failingOn(date string) func(context.Context, ledger.DigestDay) error {
	return func(_ context.Context, day ledger.DigestDay) error {
		if day.Date == date {
			return errFolding
		}
		return nil
	}
}

// opened returns the world's ledger, opened afresh to digest its days with
// fold.
func (w *lookupWorld) opened(fold *folding) *ledger.Ledger {
	return ledger.Open(w.dir, 400*24*time.Hour, func() time.Time { return w.at }, historyIn(w.history), caps, logs.For("router"), ledger.Digesting(fold))
}

// digestEnded has the world's ledger digest each finished day due, as
// DigestEnded does.
func (w *lookupWorld) digestEnded() {
	w.writer.DigestEnded(w.t.Context(), w.at)
}

// digestOf returns the digest the world's ledger holds of the local day days
// after the summaries' day, failing the test where it holds none that reads.
func (w *lookupWorld) digestOf(days int) ledger.Digest {
	w.t.Helper()
	digest, err := ledger.ReadDigest(w.dir, dayOf(days), digestBody)
	if err != nil {
		w.t.Fatalf("the digest of %s: %v", dayOf(days), err)
	}
	return digest
}

// requestsIn returns the requests each session's state holds in the digest
// the world's ledger holds of the local day days after the summaries' day,
// by its id.
func (w *lookupWorld) requestsIn(days int) map[string][]string {
	w.t.Helper()
	requests := make(map[string][]string)
	for id, e := range w.digestOf(days).Sessions {
		var state testState
		if err := json.Unmarshal(e.State, &state); err != nil {
			w.t.Fatal(err)
		}
		requests[id] = state.Requests
	}
	return requests
}

// digested reports whether the world's ledger holds a digest of the local day
// days after the summaries' day.
func (w *lookupWorld) digested(days int) bool {
	w.t.Helper()
	_, err := os.Stat(filepath.Join(w.dir, "digest-"+dayOf(days)+".json.gz"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		w.t.Fatal(err)
	}
	return err == nil
}

// lateLine files a line of session two under the 3rd, after the days are
// digested, as one in flight past the hour its day was given, and has the
// day summarised again with it.
func (w *lookupWorld) lateLine() {
	w.file(-2, askedBy("late", "two", on(-2, 12, 0)))
	w.summarise()
}

// The days the ledger holds lines of are digested oldest first, the day of
// none passed over, each session handed on the state of the day before it
// ran, which the day before's digest holds, with the lines of its own day:
// so the states chain, each holding every request of its session up to its
// day.
func TestFinishedDaysAreDigestedOldestFirstEachOnTheDayBefore(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)

	w.digestEnded()

	handed := w.fold.days()
	if dates, want := datesOf(handed), []string{dayOf(-3), dayOf(-2), dayOf(0), dayOf(1)}; !slices.Equal(dates, want) {
		t.Fatalf("the days handed are %q, want %q, oldest first, but the day of none", dates, want)
	}
	wantHanded := []map[string]handedSession{
		{"one": {requests: []string{"1"}}, "two": {requests: []string{"2"}}},
		{"two": {base: dayOf(-3), requests: []string{"3"}}},
		{"one": {base: dayOf(-3), requests: []string{"4"}}, "two": {base: dayOf(-2), requests: []string{"5"}}},
		{"one": {base: dayOf(0), requests: []string{"6"}}, "three": {requests: []string{"7"}}},
	}
	for i, want := range wantHanded {
		if !reflect.DeepEqual(handed[i].sessions, want) {
			t.Errorf("%s was handed\n%+v\nwant\n%+v", handed[i].date, handed[i].sessions, want)
		}
	}
	if got, want := w.requestsIn(1), map[string][]string{"one": {"1", "4", "6"}, "three": {"7"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("the digest of %s holds %v, want each session's every request up to it", dayOf(1), got)
	}
}

// A digest is written of the day as its files hold it: its bounds, its
// summary's lines, as many lines as its files hold, and how many arrived
// before it began, and after it ended; and each session's entry resting on
// each day naming it, with the lines its summary counts.
func TestADigestIsWrittenOfTheDayAsItsFilesHoldIt(t *testing.T) {
	w := newLookupWorld(t)
	w.file(-3, askedBy("1", "one", on(-3, 9, 0)))
	w.file(0, askedBy("early", "one", on(-1, 23, 0)), askedBy("2", "one", on(0, 9, 0)), askedBy("late", "two", on(1, 0, 30)))
	w.summarise()

	w.digestEnded()

	start, end, _ := dayfile.Day(dayOf(0))
	want := ledger.Digest{
		Head: ledger.DigestHead{Body: digestBody, Day: dayOf(0), Start: start, End: end, SummaryLines: 3, Lines: 3, Early: 1, Late: 1},
		Sessions: map[string]ledger.DigestEntry{
			"one": {RestsOn: map[string]int{dayOf(-3): 1, dayOf(0): 3}, State: json.RawMessage(`{"of":"one@` + dayOf(0) + `","requests":["1","early","2"]}`)},
			"two": {RestsOn: map[string]int{dayOf(0): 3}, State: json.RawMessage(`{"of":"two@` + dayOf(0) + `","requests":["late"]}`)},
		},
	}
	if got := w.digestOf(0); !reflect.DeepEqual(got, want) {
		t.Errorf("the digest of %s is\n%+v\nwant\n%+v", dayOf(0), got, want)
	}
}

// A line filed late under a day already digested, the day summarised again
// with it, has that day digested again, and in the same pass the days after
// it whose entries rest on it, but not the others: those before it, and those
// after it of sessions that never ran on it.
func TestALateLineHasItsDayAndThoseRestingOnItDigestedAgain(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	w.fold.days()
	w.lateLine()

	w.digestEnded()

	if dates, want := datesOf(w.fold.days()), []string{dayOf(-2), dayOf(0)}; !slices.Equal(dates, want) {
		t.Errorf("the days digested again are %q, want %q: the day the line was filed under, and the day after it resting on it", dates, want)
	}
	if got, want := w.requestsIn(0)["two"], []string{"2", "3", "late", "5"}; !slices.Equal(got, want) {
		t.Errorf("two's state as of %s holds %q, want %q, the late line among them", dayOf(0), got, want)
	}
}

// A day digested again for one of its sessions has another, resting on the
// day's own digest still, keep its state there, not handed to be folded
// again.
func TestASessionStandingOnItsDaysOwnDigestKeepsItsState(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	w.fold.days()
	was := w.digestOf(0).Sessions["one"]
	w.lateLine()

	w.digestEnded()

	handed := w.fold.days()
	if i := slices.IndexFunc(handed, func(h handedDay) bool { return h.date == dayOf(0) }); i < 0 || !slices.Equal(slices.Sorted(maps.Keys(handed[i].sessions)), []string{"two"}) {
		t.Errorf("%s was handed %+v, want two's session alone", dayOf(0), handed)
	}
	if got := w.digestOf(0).Sessions["one"]; !reflect.DeepEqual(got, was) {
		t.Errorf("one's entry in the digest of %s is %+v, want it kept as it was, %+v", dayOf(0), got, was)
	}
}

// A day one of whose lines arrived on another, as a change of time zone files
// it, whose digest never stands, is digested once, and never again while
// nothing changes, nor by a ledger opened afresh.
func TestADayThatNeverStandsIsDigestedOnce(t *testing.T) {
	w := newLookupWorld(t)
	w.file(0, askedBy("early", "one", on(-1, 23, 0)), askedBy("1", "one", on(0, 9, 0)))
	w.summarise()

	for range 3 {
		w.digestEnded()
	}
	fresh := &folding{}
	w.opened(fresh).DigestEnded(t.Context(), w.at)

	if dates, want := datesOf(w.fold.days()), []string{dayOf(0)}; !slices.Equal(dates, want) {
		t.Errorf("the days digested over three passes are %q, want %q, once", dates, want)
	}
	if handed := fresh.days(); len(handed) > 0 {
		t.Errorf("a ledger opened afresh digested %q again, want none", datesOf(handed))
	}
	if early := w.digestOf(0).Head.Early; early != 1 {
		t.Errorf("the digest of %s holds %d lines that arrived before it began, want 1", dayOf(0), early)
	}
}

// A pass that finds nothing changed since the last reads no digest: with
// every digest unreadable, none is digested again, nor warned of. A ledger
// opened afresh, holding nothing of the last, reads them, and digests them
// again.
func TestAPassFindingNothingChangedReadsNoDigest(t *testing.T) {
	log := logstest.Capture(t)
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	w.fold.days()
	unopenable(t, w.dir, "digest-*")

	w.digestEnded()

	if handed := w.fold.days(); len(handed) > 0 {
		t.Errorf("a pass finding nothing changed digested %q again, want none", datesOf(handed))
	}
	if log.Has("level=WARN") {
		t.Errorf("log reads\n%s\nwant nothing warned of", log)
	}
	fresh := &folding{}
	w.opened(fresh).DigestEnded(t.Context(), w.at)
	if dates, want := datesOf(fresh.days()), []string{dayOf(-3), dayOf(-2), dayOf(0), dayOf(1)}; !slices.Equal(dates, want) {
		t.Errorf("a ledger opened afresh digested %q, want %q, every digest unreadable", dates, want)
	}
}

// What the digesting warns: that the digest of a day can't be built, or
// written; and that the days after one can't be digested, as its lines can't
// be told against its summary.
const (
	cantDigest = "can't digest a day of the request ledger"
	cantTell   = "can't digest the request ledger's days after one whose lines can't be told against its summary"
)

// warnedOf returns how many of the lines log holds warn msg of the local day
// with the given date.
func warnedOf(log *logstest.Log, msg, date string) int {
	return len(slices.DeleteFunc(log.Lines(), func(line string) bool {
		return !strings.Contains(line, "level=WARN") || !strings.Contains(line, `msg="`+msg+`"`) || !strings.Contains(line, "day="+date)
	}))
}

// A day whose Digester fails is tried again at each pass, warned of once
// until it changes, then again, and digested once its Digester can.
func TestADayThatCantBeDigestedIsWarnedOfOnceUntilItChanges(t *testing.T) {
	log := logstest.Capture(t)
	w := newLookupWorld(t)
	sharedDays(w)
	w.fold.doing(failingOn(dayOf(0)))

	for range 3 {
		w.digestEnded()
	}

	tried := slices.DeleteFunc(datesOf(w.fold.days()), func(date string) bool { return date != dayOf(0) })
	if len(tried) != 3 || w.digested(0) {
		t.Errorf("over three passes %s was tried %d times, digested %t; want three times, none written", dayOf(0), len(tried), w.digested(0))
	}
	if warned := warnedOf(log, cantDigest, dayOf(0)); warned != 1 {
		t.Errorf("log reads\n%s\nwant %s warned of once, not %d times", log, dayOf(0), warned)
	}
	w.file(0, askedBy("8", "one", on(0, 12, 0)))
	w.summarise()
	w.digestEnded()
	if warned := warnedOf(log, cantDigest, dayOf(0)); warned != 2 {
		t.Errorf("log reads\n%s\nwant %s warned of again once it changed, twice in all, not %d times", log, dayOf(0), warned)
	}
	w.fold.doing(nil)
	w.digestEnded()
	if !w.digested(0) || warnedOf(log, cantDigest, dayOf(0)) != 2 {
		t.Errorf("once it can be, %s is digested %t, warned of %d times; want it digested, warned of no more", dayOf(0), w.digested(0), warnedOf(log, cantDigest, dayOf(0)))
	}
}

// A digest that can't be written is warned of, and written at a later pass
// once it can be.
func TestADigestThatCantBeWrittenIsWrittenOnceItCanBe(t *testing.T) {
	log := logstest.Capture(t)
	w := newLookupWorld(t)
	sharedDays(w)
	// A digest can't be written over a directory standing at its path.
	blocked := filepath.Join(w.dir, "digest-"+dayOf(0)+".json.gz")
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}

	w.digestEnded()
	w.digestEnded()

	if warned := warnedOf(log, cantDigest, dayOf(0)); warned != 1 {
		t.Errorf("log reads\n%s\nwant %s warned of once over two passes, not %d times", log, dayOf(0), warned)
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	w.digestEnded()
	if got, want := w.requestsIn(0), map[string][]string{"one": {"1", "4"}, "two": {"2", "3", "5"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("once it can be written, the digest of %s holds %v, want %v", dayOf(0), got, want)
	}
}

// A day a line is filed under as it's digested has nothing written of it,
// and is digested at the next pass once it's summarised again, the line
// among it.
func TestADayGainingALineAsItsDigestedHasNoneWritten(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	added := false
	w.fold.doing(func(_ context.Context, day ledger.DigestDay) error {
		if day.Date == dayOf(0) && !added {
			added = true
			w.file(0, askedBy("added", "two", on(0, 11, 0)))
		}
		return nil
	})

	w.digestEnded()

	if w.digested(0) {
		t.Errorf("a digest of %s was written, as a line was filed under it while it was digested; want none", dayOf(0))
	}
	w.summarise()
	w.digestEnded()
	if got, want := w.requestsIn(0)["two"], []string{"2", "3", "5", "added"}; !slices.Equal(got, want) {
		t.Errorf("two's state as of %s holds %q, want %q, the line added among them", dayOf(0), got, want)
	}
}

// A day whose lines go as it's digested, as to a prune, has nothing written
// of it, and isn't digested again.
func TestADayLosingItsLinesAsItsDigestedHasNoneWritten(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.fold.doing(func(_ context.Context, day ledger.DigestDay) error {
		if day.Date == dayOf(0) {
			removeFile(t, w.dir, "requests-"+dayOf(0)+".jsonl")
		}
		return nil
	})

	w.digestEnded()
	w.digestEnded()

	if w.digested(0) {
		t.Errorf("a digest of %s was written, as its lines went while it was digested; want none", dayOf(0))
	}
	if tried := slices.DeleteFunc(datesOf(w.fold.days()), func(date string) bool { return date != dayOf(0) }); len(tried) != 1 {
		t.Errorf("%s was digested %d times over two passes, want once, its lines gone after", dayOf(0), len(tried))
	}
}

// The digest of a day whose lines are pruned is deleted, as is a stray one
// of a day never written; those of the days either side are kept as they
// were.
func TestTheDigestOfADayWithoutLinesIsDeleted(t *testing.T) {
	w := newLookupWorld(t)
	w.file(-3, askedBy("1", "one", on(-3, 9, 0)))
	w.file(-2, askedBy("2", "two", on(-2, 9, 0)))
	w.file(0, askedBy("3", "one", on(0, 9, 0)))
	w.summarise()
	w.digestEnded()
	stray, _, _ := dayfile.Day(dayOf(-1))
	if err := ledger.WriteDigest(w.dir, ledger.Digest{Head: ledger.DigestHead{Body: digestBody, Day: dayOf(-1), Start: stray, End: stray.AddDate(0, 0, 1)}}); err != nil {
		t.Fatal(err)
	}
	kept := map[int]os.FileInfo{-3: digestInfo(t, w.dir, -3), 0: digestInfo(t, w.dir, 0)}
	removeFile(t, w.dir, "requests-"+dayOf(-2)+".jsonl")

	w.digestEnded()

	for _, days := range []int{-2, -1} {
		if w.digested(days) {
			t.Errorf("the digest of %s, a day without lines, is kept; want it deleted", dayOf(days))
		}
	}
	for days, was := range kept {
		if !dayfile.SameAs(was, digestInfo(t, w.dir, days)) {
			t.Errorf("the digest of %s changed; want it kept as it was", dayOf(days))
		}
	}
}

// digestInfo is a look at the digest of the local day days after the
// summaries' day the ledger in dir holds, failing the test where it holds
// none.
func digestInfo(t *testing.T, dir string, days int) os.FileInfo {
	t.Helper()
	info, err := os.Stat(filepath.Join(dir, "digest-"+dayOf(days)+".json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// A ledger opened without a Digester starts no goroutine to digest its days
// as it runs, as one opened with one does, and writes no digest, as it runs
// or as it's asked to digest its days.
func TestALedgerWithoutADigesterWritesNoDigest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := runWorld(t)
		l := ledger.Open(w.dir, 400*24*time.Hour, func() time.Time { return w.at }, historyIn(w.history), caps, logs.For("router"))

		without := digestingWhileRunning(t, l)
		l.DigestEnded(t.Context(), w.at)

		if without != 0 {
			t.Errorf("the ledger ran %d goroutines digesting its days, want none", without)
		}
		if digests, err := filepath.Glob(filepath.Join(w.dir, "digest-*")); err != nil || len(digests) > 0 {
			t.Errorf("the ledger holds digests %q, %v; want none", digests, err)
		}
		if with := digestingWhileRunning(t, w.writer); with != 1 {
			t.Errorf("a ledger opened with a Digester ran %d goroutines digesting its days, want one", with)
		}
	})
}

// digestingWhileRunning runs l, in a synctest bubble, until every goroutine
// it starts waits, and returns how many goroutines were then digesting its
// days, as their stacks tell it.
func digestingWhileRunning(t *testing.T, l *ledger.Ledger) int {
	t.Helper()
	stop := running(t, l)
	defer stop()
	synctest.Wait()
	stacks := make([]byte, 1<<16)
	for {
		n := runtime.Stack(stacks, true)
		if n < len(stacks) {
			return strings.Count(string(stacks[:n]), "ledger.(*digesting).run(")
		}
		stacks = make([]byte, 2*len(stacks))
	}
}

// A day whose lines can't be told against its summary, as one filed under it
// since it was summarised, stops the digesting at it: the days before it are
// digested, those after it left as they are, even one whose digest is gone,
// and that's warned of once until it changes, naming it. Once it's
// summarised again, the days after it are digested.
func TestADayThatCantBeToldStopsDigestingAfterIt(t *testing.T) {
	log := logstest.Capture(t)
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	w.fold.days()
	w.file(-2, askedBy("late", "two", on(-2, 12, 0)))
	for _, days := range []int{-3, 1} {
		removeFile(t, w.dir, "digest-"+dayOf(days)+".json.gz")
	}

	w.digestEnded()
	w.digestEnded()

	if dates, want := datesOf(w.fold.days()), []string{dayOf(-3)}; !slices.Equal(dates, want) || w.digested(1) {
		t.Errorf("with %s untold, %q were digested, %s's digest written %t; want %q alone, the day before it", dayOf(-2), dates, dayOf(1), w.digested(1), want)
	}
	if warned := warnedOf(log, cantTell, dayOf(-2)); warned != 1 {
		t.Errorf("log reads\n%s\nwant %s warned of once over two passes, not %d times", log, dayOf(-2), warned)
	}
	w.file(-2, askedBy("later", "two", on(-2, 13, 0)))
	w.digestEnded()
	if warned := warnedOf(log, cantTell, dayOf(-2)); warned != 2 {
		t.Errorf("log reads\n%s\nwant %s warned of again once it changed, twice in all, not %d times", log, dayOf(-2), warned)
	}
	w.summarise()
	w.digestEnded()
	if dates, want := datesOf(w.fold.days()), []string{dayOf(-2), dayOf(0), dayOf(1)}; !slices.Equal(dates, want) {
		t.Errorf("once %s is summarised again, %q were digested, want %q: it, the day resting on it, and the day whose digest is gone", dayOf(-2), dates, want)
	}
}

// A ledger whose directory holds digests, but no day, has them deleted.
func TestTheDigestsOfALedgerHoldingNoDayAreDeleted(t *testing.T) {
	w := newLookupWorld(t)
	stray, _, _ := dayfile.Day(dayOf(-1))
	if err := ledger.WriteDigest(w.dir, ledger.Digest{Head: ledger.DigestHead{Body: digestBody, Day: dayOf(-1), Start: stray, End: stray.AddDate(0, 0, 1)}}); err != nil {
		t.Fatal(err)
	}

	w.digestEnded()

	if w.digested(-1) {
		t.Errorf("the digest of %s is kept, the ledger holding no day; want it deleted", dayOf(-1))
	}
}

// Digesting the days as they change, pass by pass, leaves the digests a
// ledger opened afresh writes of them, from none: each day's due judged
// again where a change may have made it so, and none passed over.
func TestDigestingAsDaysChangeWritesWhatALedgerAfreshDoes(t *testing.T) {
	tests := []struct {
		name   string
		change func(w *lookupWorld)
	}{
		{name: "a late line of a session under a day it never ran on", change: func(w *lookupWorld) {
			w.file(-2, askedBy("late", "one", on(-2, 12, 0)))
			w.summarise()
		}},
		{name: "a late line under a day of none", change: func(w *lookupWorld) {
			w.file(-1, askedBy("late", "two", on(-1, 12, 0)))
			w.summarise()
		}},
		{name: "a new session under an old day", change: func(w *lookupWorld) {
			w.file(-3, askedBy("late", "four", on(-3, 12, 0)))
			w.summarise()
		}},
		{name: "the first day pruned", change: func(w *lookupWorld) {
			removeFile(w.t, w.dir, "requests-"+dayOf(-3)+".jsonl")
			removeFile(w.t, w.dir, filepath.Base(summaryFile(w.dir, dayOf(-3))))
		}},
		{name: "a digest deleted by hand", change: func(w *lookupWorld) {
			removeFile(w.t, w.dir, "digest-"+dayOf(0)+".json.gz")
		}},
		{name: "every day compressed and summarised again", change: func(w *lookupWorld) {
			compressAll(w.t, w.dir)
			w.summarise()
		}},
		{name: "a day naming a session its digest holds no entry of", change: func(w *lookupWorld) {
			digest := w.digestOf(0)
			delete(digest.Sessions, "one")
			if err := ledger.WriteDigest(w.dir, digest); err != nil {
				w.t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newLookupWorld(t)
			sharedDays(w)
			w.digestEnded()
			tt.change(w)

			w.digestEnded()
			w.digestEnded()

			digested := digestsRead(t, w.dir)
			removeDigests(t, w.dir)
			w.opened(&folding{}).DigestEnded(t.Context(), w.at)
			if afresh := digestsRead(t, w.dir); !reflect.DeepEqual(digested, afresh) {
				t.Errorf("digested pass by pass, the digests are\n%+v\nwant\n%+v\nas a ledger opened afresh writes them", digested, afresh)
			}
		})
	}
}

// digestsRead returns every digest the ledger in dir holds, by its day's date,
// failing the test where one doesn't read.
func digestsRead(t *testing.T, dir string) map[string]ledger.Digest {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "digest-*.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	digests := make(map[string]ledger.Digest, len(paths))
	for _, path := range paths {
		date := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "digest-"), ".json.gz")
		if digests[date], err = ledger.ReadDigest(dir, date, digestBody); err != nil {
			t.Fatalf("the digest of %s: %v", date, err)
		}
	}
	return digests
}

// removeDigests removes every digest the ledger in dir holds.
func removeDigests(tb testing.TB, dir string) {
	tb.Helper()
	digests, err := filepath.Glob(filepath.Join(dir, "digest-*"))
	if err != nil {
		tb.Fatal(err)
	}
	for _, path := range digests {
		if err := os.Remove(path); err != nil {
			tb.Fatal(err)
		}
	}
}

// compressAll compresses each of the ledger's plain files in dir, as a
// prune does, so the ledger's Run finds none to compress.
func compressAll(t *testing.T, dir string) {
	t.Helper()
	plain, err := filepath.Glob(filepath.Join(dir, "requests-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range plain {
		compressFile(t, dir, filepath.Base(path))
	}
}

// runWorld returns a world whose ledger's Run digests sharedDays' days,
// their files compressed, as Run's prune would have them, and summarised.
func runWorld(t *testing.T) *lookupWorld {
	t.Helper()
	w := newLookupWorld(t)
	sharedDays(w)
	compressAll(t, w.dir)
	w.summarise()
	return w
}

// Run digests the finished days as it starts, pausing after each build as
// long as it took, so the next begins twice that after the last.
func TestRunPausesAfterEachBuildAsLongAsItTook(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const took = time.Minute
		w := runWorld(t)
		w.fold.doing(func(context.Context, ledger.DigestDay) error {
			time.Sleep(took)
			return nil
		})

		stop := running(t, w.writer)
		synctest.Sleep(time.Hour / 2)
		stop()

		handed := w.fold.days()
		if dates, want := datesOf(handed), []string{dayOf(-3), dayOf(-2), dayOf(0), dayOf(1)}; !slices.Equal(dates, want) {
			t.Fatalf("Run digested %q, want %q", dates, want)
		}
		for i := 1; i < len(handed); i++ {
			if gap := handed[i].at.Sub(handed[i-1].at); gap != 2*took {
				t.Errorf("%s was begun %v after %s, want %v: its build, and as long again", handed[i].date, gap, handed[i-1].date, 2*took)
			}
		}
	})
}

// A build under way as Run's context ends stops as its Digester gives up,
// writing nothing, and Run returns then, its goroutine gone.
func TestRunStopsABuildAsItsContextEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := runWorld(t)
		w.fold.doing(func(ctx context.Context, _ ledger.DigestDay) error {
			<-ctx.Done()
			return ctx.Err()
		})
		began := time.Now()
		stop := running(t, w.writer)
		synctest.Sleep(time.Minute)

		stop()

		if ran := time.Since(began); ran != time.Minute || w.digested(-3) {
			t.Errorf("Run returned after %v, digesting %s %t; want it returned as its context ended, at a minute, none written", ran, dayOf(-3), w.digested(-3))
		}
	})
}

// A build whose Digester finishes after Run's context ended writes nothing,
// and Run waits for it before it returns.
func TestRunWaitsForABuildAsItsContextEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := runWorld(t)
		w.fold.doing(func(context.Context, ledger.DigestDay) error {
			time.Sleep(time.Hour / 4)
			return nil
		})
		began := time.Now()
		stop := running(t, w.writer)
		synctest.Sleep(time.Minute)

		stop()

		if ran := time.Since(began); ran != time.Hour/4 || w.digested(-3) {
			t.Errorf("Run returned after %v, digesting %s %t; want it to wait for the build, %v, none written", ran, dayOf(-3), w.digested(-3), time.Hour/4)
		}
	})
}

// Kicks while a pass is under way come to one pass after it: a day that
// fails at each is tried twice, at the pass Run makes as it starts and at
// the one its kicks come to.
func TestKicksDuringAPassComeToOnePassAfterIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := runWorld(t)
		release := make(chan struct{})
		fail := failingOn(dayOf(0))
		w.fold.doing(func(ctx context.Context, day ledger.DigestDay) error {
			if day.Date == dayOf(-3) {
				<-release
			}
			return fail(ctx, day)
		})
		stop := running(t, w.writer)
		defer stop()
		synctest.Wait()

		for range 3 {
			w.writer.SummariseEnded(w.at)
		}
		close(release)
		synctest.Wait()

		if tried := slices.DeleteFunc(datesOf(w.fold.days()), func(date string) bool { return date != dayOf(0) }); len(tried) != 2 {
			t.Errorf("%s, failing at each pass, was tried %d times, want twice: as Run started, and once for every kick after", dayOf(0), len(tried))
		}
	})
}
