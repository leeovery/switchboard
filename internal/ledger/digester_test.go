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
// its base doesn't read, as it may be asked on the digester's goroutine, and
// its part as partFrom gives it; first doing what during does, where it's
// set, failing where that fails; giving none of the session refused, where
// it's handed resting on a base, as a Digester gives none of one whose base
// it can't restore; and noting each day it's handed, in the order handed, as
// handedDay tells it. Each day's side it builds as sideOf gives it, and its
// rows as chunksOf gives them, doing what taking does with each line's
// request it's handed, where it's set, and what building does as it builds
// them, failing where that fails; and noting each side built, in the order
// built, as builtSide tells it.
type folding struct {
	mu       sync.Mutex
	during   func(ctx context.Context, day ledger.DigestDay) error
	taking   func(request string)
	building func(requests []string) error
	refused  string
	handed   []handedDay
	built    []builtSide
}

// sideBody is the version of the days' sides the tests' Digester gives.
const sideBody = 5

// builtSide is a day's side as the tests' Digester built it: when, by the
// clock, and the requests of the lines its builder was handed, in the order
// handed.
type builtSide struct {
	at       time.Time
	requests []string
}

// testSide is a day's side as the tests' Digester builds it: the requests
// of the day's lines, in the order its builder was handed them.
type testSide struct {
	Requests []string `json:"requests"`
}

// sideOf is the side the tests' Digester builds of the lines of the
// requests given, in their order, as the day's head keeps it.
func sideOf(t testing.TB, requests ...string) json.RawMessage {
	t.Helper()
	side, err := json.Marshal(testSide{Requests: requests})
	if err != nil {
		t.Fatal(err)
	}
	return side
}

// rowsAChunk is how many rows the tests' Digester cuts a day's rows into a
// chunk at.
const rowsAChunk = 2

// chunksOf is the rows the tests' Digester builds of the lines of the
// requests given, in their order, as the day's rows file keeps them: each
// row a line of its request's, rowsAChunk of them a chunk, each chunk's meta
// naming its first and last request: none of none.
func chunksOf(requests ...string) []ledger.BuiltChunk {
	var chunks []ledger.BuiltChunk
	for rows := range slices.Chunk(requests, rowsAChunk) {
		chunks = append(chunks, ledger.BuiltChunk{
			Rows:  []byte(strings.Join(rows, "\n") + "\n"),
			Count: len(rows),
			Meta:  json.RawMessage(`{"first":"` + rows[0] + `","last":"` + rows[len(rows)-1] + `"}`),
		})
	}
	return chunks
}

// RowsVersion is the version of the sides and rows the tests' Digester
// gives.
func (f *folding) RowsVersion() int {
	return sideBody
}

// Day gives a builder of a day's side and rows by the tests' Digester, as
// foldingSide has it, handed no line yet.
func (f *folding) Day() ledger.DayBuilder {
	return &foldingSide{f: f}
}

// foldingSide is a builder of a day's side and rows by the tests' Digester,
// f: the requests of the lines it's handed, in the order handed.
type foldingSide struct {
	f        *folding
	requests []string
}

// Take notes l's request among those handed, and does with it what taking
// does, where it's set.
func (b *foldingSide) Take(l ledger.Line) {
	b.requests = append(b.requests, l.Request)
	b.f.mu.Lock()
	taking := b.f.taking
	b.f.mu.Unlock()
	if taking != nil {
		taking(l.Request)
	}
}

// Built has the tests' Digester build the side and rows of the requests
// handed, as side does.
func (b *foldingSide) Built() (ledger.DayBuilt, error) {
	return b.f.side(b.requests)
}

// side notes the side of the requests given built now, does what building
// does, where it's set, and gives the side, as a testSide of them, and the
// rows, as chunksOf has them.
func (f *folding) side(requests []string) (ledger.DayBuilt, error) {
	f.mu.Lock()
	building := f.building
	f.built = append(f.built, builtSide{at: time.Now(), requests: requests})
	f.mu.Unlock()
	if building != nil {
		if err := building(requests); err != nil {
			return ledger.DayBuilt{}, err
		}
	}
	side, err := json.Marshal(testSide{Requests: requests})
	return ledger.DayBuilt{Side: side, Chunks: chunksOf(requests...)}, err
}

// buildingSides has the tests' Digester do what building does, as it
// builds each day's side from then on.
func (f *folding) buildingSides(building func(requests []string) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.building = building
}

// takingSides has the tests' Digester do what taking does with the request
// of each line a builder of a day's side is handed, from then on.
func (f *folding) takingSides(taking func(request string)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.taking = taking
}

// sides returns the sides the tests' Digester built, in the order built,
// letting go of them.
func (f *folding) sides() []builtSide {
	f.mu.Lock()
	defer f.mu.Unlock()
	built := f.built
	f.built = nil
	return built
}

// requestsOfSides returns the requests of each of the sides built, in the
// order built.
func requestsOfSides(built []builtSide) [][]string {
	requests := make([][]string, len(built))
	for i, b := range built {
		requests[i] = b.requests
	}
	return requests
}

// handedDay is a day as the tests' Digester was handed it: when, by the
// clock; its date; each session's, by its id, as handedSession tells it; and
// the lines the builder it was handed to was handed before it, as takenDays
// notes them.
type handedDay struct {
	at       time.Time
	date     string
	sessions map[string]handedSession
	taken    takenDays
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

// Builder gives a builder of a day's digest by the tests' Digester, as
// foldingDay has it, handed no line yet.
func (f *folding) Builder() ledger.DigestBuilder {
	return &foldingDay{f: f, taken: takenDays{}}
}

// foldingDay is a builder of a day's digest by the tests' Digester, f: the
// lines it's handed, as takenDays notes them.
type foldingDay struct {
	f     *folding
	taken takenDays
}

// Take notes l, a line of the day with the given date, as taken.
func (b *foldingDay) Take(date string, l ledger.Line) {
	b.taken.take(date, l)
}

// Digest has the tests' Digester digest day, as digest does, noting it handed
// with the lines taken before it.
func (b *foldingDay) Digest(ctx context.Context, day ledger.DigestDay) (map[string]ledger.Folded, error) {
	return b.f.digest(ctx, day, b.taken)
}

// digest notes day as handed, with the lines taken before it, does what
// during does, where it's set, and gives each of its sessions folded, but
// the one refused resting on a base: its state, its part, and its part's
// times, as timesFrom has them.
func (f *folding) digest(ctx context.Context, day ledger.DigestDay, taken takenDays) (map[string]ledger.Folded, error) {
	f.mu.Lock()
	during, refused := f.during, f.refused
	handed := handedOf(day)
	handed.taken = taken
	f.handed = append(f.handed, handed)
	f.mu.Unlock()
	if during != nil {
		if err := during(ctx, day); err != nil {
			return nil, err
		}
	}
	folded := make(map[string]ledger.Folded, len(day.Sessions))
	for id, s := range day.Sessions {
		if id == refused && s.Base != nil {
			continue
		}
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
		part, err := json.Marshal(partFrom(id, day.Date, s))
		if err != nil {
			return nil, err
		}
		times, err := json.Marshal(timesFrom(id, day.Date, s))
		if err != nil {
			return nil, err
		}
		folded[id] = ledger.Folded{State: state, Part: part, Times: times}
	}
	return folded, nil
}

// timesFrom is the times of the part of the session with the given id of
// the local day with the given date, as partFrom has its part, folded from
// from: as its part, but told as its times.
func timesFrom(id, date string, from ledger.SessionFrom) testPart {
	times := partFrom(id, date, from)
	times.Of = "times of " + times.Of
	return times
}

// testPart is a session's part of a day as the digester's tests fold it: Of
// tells the session and the day, as stateAt tells them; Base, the date of
// the state it was handed resting on, "" for none; and Requests, those of
// the lines it was handed, in their order.
type testPart struct {
	Of       string   `json:"of"`
	Base     string   `json:"base,omitempty"`
	Requests []string `json:"requests"`
}

// partFrom is the part of the session with the given id of the local day
// with the given date, folded from from, the session as of the start of the
// day after.
func partFrom(id, date string, from ledger.SessionFrom) testPart {
	return testPart{Of: stateAt(id, date), Base: from.BaseDate(), Requests: requestsOf(from.Lines)}
}

// handedOf is day as handedDay tells it, handed now.
func handedOf(day ledger.DigestDay) handedDay {
	handed := handedDay{at: time.Now(), date: day.Date, sessions: make(map[string]handedSession, len(day.Sessions))}
	for id, s := range day.Sessions {
		handed.sessions[id] = handedSession{base: s.BaseDate(), requests: requestsOf(s.Lines)}
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

// refuse has the tests' Digester give none of the session with the given
// id handed resting on a base, from then on.
func (f *folding) refuse(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refused = id
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
func (w *lookupWorld) opened(fold ledger.Digester) *ledger.Ledger {
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
// each day naming it, with the lines its summary counts, pointing at its
// part, folded on the state it was handed resting on.
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
	if got := unparted(w.digestOf(0)); !reflect.DeepEqual(got, want) {
		t.Errorf("the digest of %s, its parts aside, is\n%+v\nwant\n%+v", dayOf(0), got, want)
	}
	wantParts := map[string]partIn{
		"one": {on: dayOf(-3), part: testPart{Of: stateAt("one", dayOf(0)), Base: dayOf(-3), Requests: []string{"early", "2"}}},
		"two": {part: testPart{Of: stateAt("two", dayOf(0)), Requests: []string{"late"}}},
	}
	if got := w.partsIn(0); !reflect.DeepEqual(got, wantParts) {
		t.Errorf("the parts of %s are\n%+v\nwant\n%+v", dayOf(0), got, wantParts)
	}
}

// unparted is digest without its parts: its head recording no parts file,
// and none of its entries pointing at a part.
func unparted(digest ledger.Digest) ledger.Digest {
	digest.Head.PartsSize = 0
	for id, e := range digest.Sessions {
		e.Part = nil
		digest.Sessions[id] = e
	}
	return digest
}

// partIn is a session's part as a digest points at it: on, the date its
// entry names it folded on; and the part, as the parts file holds it.
type partIn struct {
	on   string
	part testPart
}

// partsIn returns the part of each session the digest of the local day days
// after the summaries' day points at, by its id, failing the test where one
// doesn't read: none of a session it points at none of.
func (w *lookupWorld) partsIn(days int) map[string]partIn {
	w.t.Helper()
	parts := make(map[string]partIn)
	for id, e := range w.digestOf(days).Sessions {
		if e.Part == nil {
			continue
		}
		data, err := ledger.ReadPart(w.dir, dayOf(days), *e.Part)
		if err != nil {
			w.t.Fatalf("the part of %s of %s: %v", id, dayOf(days), err)
		}
		in := partIn{on: e.Part.On}
		if err := json.Unmarshal(data, &in.part); err != nil {
			w.t.Fatal(err)
		}
		parts[id] = in
	}
	return parts
}

// timesIn returns the times of the part of each session the digest of the
// local day days after the summaries' day points at, by its id, failing the
// test where they don't read, or it points at a part of none: none of a
// session it points at no part of.
func (w *lookupWorld) timesIn(days int) map[string]testPart {
	w.t.Helper()
	times := make(map[string]testPart)
	for id, e := range w.digestOf(days).Sessions {
		if e.Part == nil {
			continue
		}
		data, err := ledger.ReadTimes(w.dir, dayOf(days), *e.Part)
		if err != nil {
			w.t.Fatalf("the times of the part of %s of %s: %v", id, dayOf(days), err)
		}
		var t testPart
		if err := json.Unmarshal(data, &t); err != nil {
			w.t.Fatal(err)
		}
		times[id] = t
	}
	return times
}

// partsPath is the path of the parts file of the local day days after the
// summaries' day the ledger in dir holds.
func partsPath(dir string, days int) string {
	return filepath.Join(dir, "parts-"+dayOf(days)+".gz")
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
// day's own digest still, keep its state there, and its part, on the day it
// was folded on, not handed to be folded again: its part's members and its
// times', copied as they were written, by their CRCs.
func TestASessionStandingOnItsDaysOwnDigestKeepsItsStateAndPart(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	w.fold.days()
	was, wasPart, wasTimes := unparted(w.digestOf(0)).Sessions["one"], w.partsIn(0)["one"], w.timesIn(0)["one"]
	wasRef := w.digestOf(0).Sessions["one"].Part
	w.lateLine()

	w.digestEnded()

	handed := w.fold.days()
	if i := slices.IndexFunc(handed, func(h handedDay) bool { return h.date == dayOf(0) }); i < 0 || !slices.Equal(slices.Sorted(maps.Keys(handed[i].sessions)), []string{"two"}) {
		t.Errorf("%s was handed %+v, want two's session alone", dayOf(0), handed)
	}
	if got := unparted(w.digestOf(0)).Sessions["one"]; !reflect.DeepEqual(got, was) {
		t.Errorf("one's entry in the digest of %s is %+v, want it kept as it was, %+v", dayOf(0), got, was)
	}
	if got := w.partsIn(0)["one"]; !reflect.DeepEqual(got, wasPart) {
		t.Errorf("one's part of %s is %+v, want it kept as it was, %+v", dayOf(0), got, wasPart)
	}
	if got := w.timesIn(0)["one"]; !reflect.DeepEqual(got, wasTimes) {
		t.Errorf("the times of one's part of %s are %+v, want them kept as they were, %+v", dayOf(0), got, wasTimes)
	}
	if ref := w.digestOf(0).Sessions["one"].Part; ref.CRC != wasRef.CRC || ref.Times.CRC != wasRef.Times.CRC {
		t.Errorf("one's part of %s is at %+v, its times at %+v; want each of the CRC it was written with, %d and %d", dayOf(0), ref, ref.Times, wasRef.CRC, wasRef.Times.CRC)
	}
}

// A session resting on the day's own digest still, whose part's times there
// are damaged, its parts file as long, isn't kept, its times copied as they
// were: it's folded afresh, read whole.
func TestASessionOnItsDaysOwnDigestWhoseTimesAreDamagedIsFoldedAfresh(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	w.digestEnded()
	w.fold.days()
	times := w.digestOf(0).Sessions["one"].Part.Times
	damageAt(t, partsPath(w.dir, 0), times.Offset+times.Length/2)
	w.lateLine()

	w.digestEnded()

	handed := w.fold.days()
	i := slices.IndexFunc(handed, func(h handedDay) bool { _, one := h.sessions["one"]; return h.date == dayOf(0) && one })
	if i < 0 || handed[i].sessions["one"].base != "" {
		t.Fatalf("%s was handed %+v, want one read whole among it", dayOf(0), handed)
	}
	want := testPart{Of: "times of " + stateAt("one", dayOf(0)), Requests: []string{"1", "4"}}
	if got := w.timesIn(0)["one"]; !reflect.DeepEqual(got, want) {
		t.Errorf("the times of one's part of %s are %+v, want %+v, folded whole", dayOf(0), got, want)
	}
}

// A day's builder is handed each session resting on a state with the chain
// of parts behind it, none of their times, which a fold resting on the state
// needs none of; and the parts files hold each session's part's times beside
// its part, as its builder gave them.
func TestADaysBuilderIsHandedItsSessionsChainsWithoutTheirTimes(t *testing.T) {
	w := newLookupWorld(t)
	sharedDays(w)
	var chains []ledger.SessionPart
	w.fold.doing(func(_ context.Context, day ledger.DigestDay) error {
		for _, s := range day.Sessions {
			chains = append(chains, s.Parts...)
		}
		return nil
	})

	w.digestEnded()

	untimed := !slices.ContainsFunc(chains, func(p ledger.SessionPart) bool { return p.Part == nil || p.Times != nil })
	if len(chains) == 0 || !untimed {
		t.Errorf("the builders were handed the parts %+v; want some, each without its times", chains)
	}
	times := func(id string, days int, base string, requests ...string) testPart {
		return testPart{Of: "times of " + stateAt(id, dayOf(days)), Base: base, Requests: requests}
	}
	want := map[int]map[string]testPart{
		-3: {"one": times("one", -3, "", "1"), "two": times("two", -3, "", "2")},
		-2: {"two": times("two", -2, dayOf(-3), "3")},
		0:  {"one": times("one", 0, dayOf(-3), "4"), "two": times("two", 0, dayOf(-2), "5")},
		1:  {"one": times("one", 1, dayOf(0), "6"), "three": times("three", 1, "", "7")},
	}
	for days, times := range want {
		if got := w.timesIn(days); !reflect.DeepEqual(got, times) {
			t.Errorf("the times of the parts of %s are\n%+v\nwant\n%+v", dayOf(days), got, times)
		}
	}
}

// A day's builder is handed every line the read of the day's sessions reads,
// once, of whichever session, one of none among them, before it's handed the
// sessions. A session read again whole, as one resting on a state its
// builder gives none of, is handed to a builder of its own, handed every
// line of the days that read reads afresh.
func TestADaysBuilderIsHandedEveryLineItsReadReads(t *testing.T) {
	w := newLookupWorld(t)
	w.file(0, askedBy("probe", "", on(0, 8, 0)))
	sharedDays(w)

	w.digestEnded()

	day0 := takenDays{dayOf(0): {"probe": 1, "4": 1, "5": 1}}
	want := []takenDays{{dayOf(-3): {"1": 1, "2": 1}}, {dayOf(-2): {"3": 1}}, day0, {dayOf(1): {"6": 1, "7": 1}}}
	if got := takenOf(w.fold.days()); !reflect.DeepEqual(got, want) {
		t.Errorf("each day's builder, oldest first, was handed\n%v\nwant\n%v", got, want)
	}
	w.fold.refuse("two")
	w.lateLine()

	w.digestEnded()

	handed := w.fold.days()
	if dates, want := datesOf(handed), []string{dayOf(-2), dayOf(-2), dayOf(0), dayOf(0)}; !slices.Equal(dates, want) {
		t.Fatalf("the days handed are %q, want %q: the late line's, then the day after it, each of two, and each again, of two read whole", dates, want)
	}
	through2 := takenDays{dayOf(-3): {"1": 1, "2": 1}, dayOf(-2): {"3": 1, "late": 1}}
	through0 := maps.Clone(through2)
	maps.Copy(through0, day0)
	want = []takenDays{{dayOf(-2): {"3": 1, "late": 1}}, through2, day0, through0}
	if got := takenOf(handed); !reflect.DeepEqual(got, want) {
		t.Errorf("the builders were handed\n%v\nwant\n%v: each day's own lines, then, afresh, every line of two's days through it", got, want)
	}
	if got, want := handed[3].sessions, map[string]handedSession{"two": {requests: []string{"2", "3", "late", "5"}}}; !reflect.DeepEqual(got, want) {
		t.Errorf("%s was handed again %+v, want %+v, two read whole", dayOf(0), got, want)
	}
}

// takenOf returns the lines each of the days handed was taken with, in the
// order handed.
func takenOf(handed []handedDay) []takenDays {
	taken := make([]takenDays, len(handed))
	for i, h := range handed {
		taken[i] = h.taken
	}
	return taken
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

// The digest of a day whose lines are pruned is deleted, with its parts
// file, as is a stray one, or a stray parts file, of a day never written;
// those of the days either side are kept as they were.
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
	for _, days := range []int{-1, 2} {
		if err := ledger.WriteParts(w.dir, dayOf(days), map[string]json.RawMessage{"one": json.RawMessage(`{}`)}); err != nil {
			t.Fatal(err)
		}
	}
	kept := map[string]os.FileInfo{}
	for _, days := range []int{-3, 0} {
		kept[digestPath(w.dir, days)], kept[partsPath(w.dir, days)] = fileInfo(t, digestPath(w.dir, days)), fileInfo(t, partsPath(w.dir, days))
	}
	removeFile(t, w.dir, "requests-"+dayOf(-2)+".jsonl")

	w.digestEnded()

	for _, days := range []int{-2, -1, 2} {
		for _, path := range []string{digestPath(w.dir, days), partsPath(w.dir, days)} {
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("%s, of a day without lines, is kept, looked at: %v; want it deleted", filepath.Base(path), err)
			}
		}
	}
	for path, was := range kept {
		if !dayfile.SameAs(was, fileInfo(t, path)) {
			t.Errorf("%s changed; want it kept as it was", filepath.Base(path))
		}
	}
}

// digestPath is the path of the digest of the local day days after the
// summaries' day the ledger in dir holds.
func digestPath(dir string, days int) string {
	return filepath.Join(dir, "digest-"+dayOf(days)+".json.gz")
}

// fileInfo is a look at the file at path, failing the test where there's
// none.
func fileInfo(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
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
		change func(t *testing.T, w *lookupWorld)
	}{
		{name: "a late line of a session under a day it never ran on", change: func(t *testing.T, w *lookupWorld) {
			w.file(-2, askedBy("late", "one", on(-2, 12, 0)))
			w.summarise()
		}},
		{name: "a late line under a day of none", change: func(t *testing.T, w *lookupWorld) {
			w.file(-1, askedBy("late", "two", on(-1, 12, 0)))
			w.summarise()
		}},
		{name: "a new session under an old day", change: func(t *testing.T, w *lookupWorld) {
			w.file(-3, askedBy("late", "four", on(-3, 12, 0)))
			w.summarise()
		}},
		{name: "the first day pruned", change: func(t *testing.T, w *lookupWorld) {
			removeFile(t, w.dir, "requests-"+dayOf(-3)+".jsonl")
			removeFile(t, w.dir, filepath.Base(summaryFile(w.dir, dayOf(-3))))
		}},
		{name: "a digest deleted by hand", change: func(t *testing.T, w *lookupWorld) {
			removeFile(t, w.dir, "digest-"+dayOf(0)+".json.gz")
		}},
		{name: "every day compressed and summarised again", change: func(t *testing.T, w *lookupWorld) {
			compressAll(t, w.dir)
			w.summarise()
		}},
		{name: "a day naming a session its digest holds no entry of", change: func(t *testing.T, w *lookupWorld) {
			digest := w.digestOf(0)
			delete(digest.Sessions, "one")
			if err := ledger.WriteDigest(w.dir, digest); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "a parts file deleted by hand", change: func(t *testing.T, w *lookupWorld) {
			removeFile(t, w.dir, filepath.Base(partsPath(w.dir, 0)))
		}},
		{name: "a parts file written again by a build that stopped before its digest", change: func(t *testing.T, w *lookupWorld) {
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
			tt.change(t, w)

			w.digestEnded()
			w.digestEnded()

			digested, parts := digestsRead(t, w.dir), partsFilesIn(t, w.dir)
			removeDigests(t, w.dir)
			w.opened(&folding{}).DigestEnded(t.Context(), w.at)
			if afresh := digestsRead(t, w.dir); !reflect.DeepEqual(digested, afresh) {
				t.Errorf("digested pass by pass, the digests are\n%+v\nwant\n%+v\nas a ledger opened afresh writes them", digested, afresh)
			}
			if afresh := partsFilesIn(t, w.dir); !reflect.DeepEqual(parts, afresh) {
				t.Errorf("digested pass by pass, the parts files are\n%q\nwant\n%q\nas a ledger opened afresh writes them", parts, afresh)
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

// partsFilesIn returns what each parts file the ledger in dir holds holds, by
// its name.
func partsFilesIn(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	return filesMatching(t, dir, "parts-*.gz")
}

// filesMatching returns what each file in dir whose name matches pattern
// holds, by its name.
func filesMatching(t *testing.T, dir, pattern string) map[string][]byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string][]byte, len(paths))
	for _, path := range paths {
		if files[filepath.Base(path)], err = os.ReadFile(path); err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// removeDigests removes every digest the ledger in dir holds, and every parts
// file.
func removeDigests(tb testing.TB, dir string) {
	tb.Helper()
	for _, pattern := range []string{"digest-*", "parts-*"} {
		held, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			tb.Fatal(err)
		}
		for _, path := range held {
			if err := os.Remove(path); err != nil {
				tb.Fatal(err)
			}
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

// Run digests the finished days as its first round ends, pausing after each
// build as long as it took, so the next begins twice that after the last.
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
