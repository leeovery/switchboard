package views

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// chainSeeds is how many seeds of each world the chained digests run.
const chainSeeds = 8

// chainWorlds are the generated worlds the chained digests run: those the
// stored request's tests decode, and a session running every day for a
// week, so a state rests on a chain of several before it.
var chainWorlds = append(slices.Clip(storedWorlds), struct {
	name  string
	lines func(seed uint64) []ledger.Line
}{name: "a week of days", lines: everyDay})

// everyDay is a generated session's lines, as generatedSession makes them,
// and again a day later, and so on four times more, as dayAfterDay has them.
func everyDay(seed uint64) []ledger.Line {
	return dayAfterDay(generatedSession(seed), 5)
}

// dayAfterDay are lines, and again a day later, as movedOn moves them, and
// so on, on as many days in all as given, each request of each day's its
// own, in the order the ledger files them.
func dayAfterDay(lines []ledger.Line, days int) []ledger.Line {
	all := slices.Clone(lines)
	for day := 1; day < days; day++ {
		for _, l := range lines {
			l = movedOn(l, day)
			l.Request = fmt.Sprintf("%s-%d", l.Request, day)
			all = append(all, l)
		}
	}
	slices.SortStableFunc(all, func(a, b ledger.Line) int { return ended(&a).Compare(ended(&b)) })
	return all
}

// Digests built by a real ledger day by day, each session's state chained on
// its state of the last day before that named it, its page's part on that
// state's, as the ledger's files hold them, are the sessions read whole:
// each state restored onto each later day, and told on with the session's
// lines since, as heldAsOf tells them, is the session read whole on that
// day, sealed at the same horizon, flights aside, as a restored session
// holds none; each state's page, restored from it and its chain of parts,
// is the page folded whole at its day's end; and each session as a Follower
// of the files gives it, resting on its newest state, or on its page, its
// points of every account's answers of the days read after it, is the
// session read whole through the same Follower, its page tallied alike. A
// page resting on a state is refused, by the Digests or held paged, only as
// its points can't take the days since in, and only of the sessions the
// worlds build so, as takesSplits names them. Of the generated worlds, read
// with tables other than the one the states are built with.
func TestChainedDigestsAreTheSessionsReadWhole(t *testing.T) {
	prices := digestPrices(t, changingPrices())
	var restored, paged, followed, pagesFollowed, based, chained int
	var refused, pagesRefused []string
	for _, w := range chainWorlds {
		for seed := range uint64(chainSeeds) {
			c := newDigestChain(t, fmt.Sprintf("%s %d", w.name, seed), seed, w.lines(seed))
			c.digestDayByDay(func(day LedgerDay) { c.checkFollowed(day.Start.Add(12*time.Hour), prices) })
			chained = max(chained, c.checkChained())
			restored += c.checkRestored(prices)
			paged += c.checkPaged()
			followed += c.followed
			pagesFollowed += c.pagesFollowed
			based += c.based
			refused = append(refused, c.refused...)
			pagesRefused = append(pagesRefused, c.pagesRefused...)
		}
	}
	if restored == 0 || paged == 0 || followed == 0 || pagesFollowed == 0 || based == 0 || chained < 4 {
		t.Errorf("%d states restored onto a later day, %d pages restored from their chains, %d sessions followed resting on a state, %d on a page, "+
			"%d states folded on another, %d states the longest chain: want some of each, and a chain of four",
			restored, paged, followed, pagesFollowed, based, chained)
	}
	for what, sessions := range map[string][]string{"the Digests gave no state of": refused, "a Follower's held paged refused": pagesRefused} {
		if got := slices.Compact(slices.Sorted(slices.Values(sessions))); !slices.Equal(got, takesSplits) {
			t.Errorf("%s %q resting on a state, want the sessions the worlds build a split their points can't take, %q", what, got, takesSplits)
		}
	}
}

// takesSplits are the sessions of the chained worlds, by world and id, whose
// pages' points can't take a day in on the state of a day before it, as
// takes can't, a model first reading a window only some models share:
// pagedRun dates that two hours after its session's first request, of the
// chain's seeds 2 and 6; of 6, on the day after its first, while it runs; of
// 2, after its every request.
var takesSplits = []string{"pagedRun 6, session s"}

// A state the Digests can't restore, as one spoiled, is folded past: the day
// after it the session is handed again read whole, so every later day's
// digest holds its state, the session read whole, and no day stays due.
func TestADigestPastASpoiledStateHoldsTheSessionReadWhole(t *testing.T) {
	prices := digestPrices(t, changingPrices())
	var lines []ledger.Line
	for day := range 5 {
		lines = append(lines, ledger.Line{
			At: time.Date(2026, 10, 7+day, 12, 0, 0, 0, time.Local).UTC(), Request: fmt.Sprintf("r%d", day), Kind: ledger.KindMessage, Session: "s",
			Model: foldOpus, Account: "work", Reason: "sticky", Status: 200, Attempts: 1, TotalMS: 3000, Class: ledger.ClassMain, Usage: json.RawMessage(foldUsages[2]),
		})
	}
	c := newDigestChain(t, "a request a day", 0, lines)
	spoiled := dayfile.DateOf(lines[2].At)
	c.digests.spoiled = spoiled
	c.digestDayByDay(func(LedgerDay) {})
	for _, l := range lines[3:] {
		if date := dayfile.DateOf(l.At); c.digests.states[date]["s"] == nil {
			t.Errorf("the digest of %s holds no state of s, after the state of %s spoiled", date, spoiled)
		}
	}
	if after := dayfile.DateOf(lines[3].At); c.digests.handed[after].Sessions["s"].Base != nil {
		t.Errorf("the digest of %s, the day after the spoiled state's, was handed s last resting on %+v, want it read whole", after, c.digests.handed[after].Sessions["s"].Base)
	}
	c.checkRestored(prices)
	c.checkFollowed(c.at, prices)
	if c.followed == 0 {
		t.Errorf("no session followed at %v rests on a state", c.at)
	}
	calls := c.digests.calls
	c.ledger.DigestEnded(t.Context(), c.at)
	if c.digests.calls != calls {
		t.Errorf("a pass after every day's digest was built handed the Digests %d more days, want none due", c.digests.calls-calls)
	}
}

// digestChain is a generated world written as a real ledger, digested day by
// day, as the router's ledger digests it, by the views' Digests, as
// recordedDigests records them: named by its world and seed; its lines, in
// the order filed; its state directory; the ledger, by the clock at; how
// many sessions a Follower gave resting on a state, and on a page, and each
// of those on a page refused held paged, as samePaged has them; and how many
// states the Digests folded on another, and each session they gave no state
// of resting on one, as checkChained has them: each session by its world
// and id.
type digestChain struct {
	t             *testing.T
	name          string
	seed          uint64
	lines         []ledger.Line
	state         string
	at            time.Time
	ledger        *ledger.Ledger
	digests       *recordedDigests
	followed      int
	pagesFollowed int
	pagesRefused  []string
	based         int
	refused       []string
}

// recordedDigests are the views' Digests, each day handed a builder of them
// recorded, by its date: as last handed; the states and parts they gave of
// it, and the parts' times, by session, each with the base it was folded on,
// nil for none; the sessions handed that they gave none of, as refusal has
// each; and how many times a builder was handed one. What they give of a
// day is changed as alter changes it, where it's set, before it's recorded;
// but of the day with the date spoiled, where it's set, they give
// spoiledState of each session, recording none.
type recordedDigests struct {
	Digests
	handed  map[string]ledger.DigestDay
	states  map[string]map[string]json.RawMessage
	parts   map[string]map[string]json.RawMessage
	times   map[string]map[string]json.RawMessage
	bases   map[string]map[string]*ledger.SessionBase
	refused map[string]map[string]refusal
	calls   int
	alter   func(date string, folded map[string]ledger.Folded)
	spoiled string
}

// refusal is a session handed a builder that gave none of it: the date of
// the state it was handed resting on, "" for none; and, of one resting on a
// state, whether its page's points take in the answers the builder took of
// the days since, as takesSince has it, or why its state won't restore.
type refusal struct {
	base      string
	takes     bool
	restoring error
}

// newRecordedDigests are the views' Digests, recording none yet.
func newRecordedDigests() *recordedDigests {
	return &recordedDigests{
		handed: make(map[string]ledger.DigestDay), states: make(map[string]map[string]json.RawMessage), parts: make(map[string]map[string]json.RawMessage),
		times: make(map[string]map[string]json.RawMessage), bases: make(map[string]map[string]*ledger.SessionBase), refused: make(map[string]map[string]refusal),
	}
}

// spoiledState is a state as one edited by hand might be: JSON, but of
// another day than its digest's, so it won't restore.
const spoiledState = `{"d":"2000-01-01"}`

// Builder gives a builder of a day's digest as the views' Digests give one,
// each day it's handed recorded, as recordedDay has it.
func (r *recordedDigests) Builder() ledger.DigestBuilder {
	return recordedDay{DigestBuilder: r.Digests.Builder(), r: r}
}

// recordedDay is a builder of a day's digest by the views' Digests, whose
// day handed is recorded by r.
type recordedDay struct {
	ledger.DigestBuilder
	r *recordedDigests
}

// Digest gives what the views' builder gives of day, recording both, or
// spoiled states, of the day spoiled.
func (b recordedDay) Digest(ctx context.Context, day ledger.DigestDay) (map[string]ledger.Folded, error) {
	r := b.r
	r.calls++
	r.handed[day.Date] = day
	folded, err := b.DigestBuilder.Digest(ctx, day)
	if r.alter != nil {
		r.alter(day.Date, folded)
	}
	if day.Date == r.spoiled {
		for id, f := range folded {
			f.State = json.RawMessage(spoiledState)
			folded[id] = f
		}
		return folded, err
	}
	for id, from := range day.Sessions {
		f, given := folded[id]
		if !given {
			recorded(r.refused, day.Date)[id] = b.refusalOf(id, from, day)
			continue
		}
		recorded(r.states, day.Date)[id], recorded(r.parts, day.Date)[id], recorded(r.bases, day.Date)[id] = f.State, f.Part, from.Base
		recorded(r.times, day.Date)[id] = f.Times
	}
	return folded, err
}

// refusalOf is the session with the given id that from gives, which the
// builder gave none of on day, as refusal has it, of the answers the builder
// took.
func (b recordedDay) refusalOf(id string, from ledger.SessionFrom, day ledger.DigestDay) refusal {
	r := refusal{base: from.BaseDate()}
	if from.Base != nil {
		r.takes, r.restoring = takesSince(id, from, b.DigestBuilder.(*dayDigest).answers, LedgerDay{Start: day.Start, Date: day.Date})
	}
	return r
}

// recorded is what byDate records of the day with the given date, by
// session, made where it records none yet.
func recorded[T any](byDate map[string]map[string]T, date string) map[string]T {
	if byDate[date] == nil {
		byDate[date] = make(map[string]T)
	}
	return byDate[date]
}

// quiet is a logger of nothing.
var quiet = slog.New(slog.DiscardHandler)

// newDigestChain writes lines as the ledger files them, each under the day
// it arrived on, in a state directory of the test's own, and opens the
// ledger there, digesting with recordedDigests.
func newDigestChain(t *testing.T, name string, seed uint64, lines []ledger.Line) *digestChain {
	t.Helper()
	c := &digestChain{t: t, name: name, seed: seed, lines: lines, state: t.TempDir(), digests: newRecordedDigests()}
	for _, dir := range []string{ledger.Dir(c.state), readings.Dir(c.state)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	filed := make(map[string][]ledger.Line)
	for _, l := range lines {
		date := dayfile.DateOf(l.At)
		filed[date] = append(filed[date], l)
	}
	for date, lines := range filed {
		c.file(date, lines...)
	}
	c.ledger = c.opened()
	return c
}

// opened is the ledger in the chain's state directory, opened as the router
// opens it, by the chain's clock, digesting with its recordedDigests.
func (c *digestChain) opened() *ledger.Ledger {
	return ledger.Open(ledger.Dir(c.state), 400*24*time.Hour, func() time.Time { return c.at }, ledger.HistoryIn(readings.Files(readings.Dir(c.state), quiet)),
		ledger.CapsOf(nil, nil), quiet, ledger.Digesting(c.digests))
}

// cloned is a chain of c's lines, as the test given holds it, in a state
// directory of its own holding a copy of c's files, its ledger opened
// there afresh, its clock where c's is: so each of several tests upsets
// the same days, built once.
func (c *digestChain) cloned(t *testing.T) *digestChain {
	t.Helper()
	clone := &digestChain{t: t, name: c.name, seed: c.seed, lines: c.lines, state: t.TempDir(), at: c.at, digests: newRecordedDigests()}
	if err := os.CopyFS(clone.state, os.DirFS(c.state)); err != nil {
		t.Fatal(err)
	}
	clone.ledger = clone.opened()
	return clone
}

// file files lines, in their order, under the local day with the given
// date, after those filed there before, whenever each arrived.
func (c *digestChain) file(date string, lines ...ledger.Line) {
	c.t.Helper()
	var data []byte
	for _, l := range lines {
		line, err := json.Marshal(l)
		if err != nil {
			c.t.Fatal(err)
		}
		data = append(append(data, line...), '\n')
	}
	f, err := os.OpenFile(filepath.Join(ledger.Dir(c.state), "requests-"+date+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		c.t.Fatal(err)
	}
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		c.t.Fatal(err)
	}
}

// digestPath is where the ledger keeps the digest of the local day with the
// given date, as it names one.
func (c *digestChain) digestPath(date string) string {
	return filepath.Join(ledger.Dir(c.state), "digest-"+date+".json.gz")
}

// gunzipped is data inflated.
func gunzipped(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return plain
}

// gzipped is data deflated, as the ledger writes a digest.
func gzipped(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, err := w.Write(data)
	if closeErr := w.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// days are the days from the one after the first line arrived on through
// the third after the one the last did.
func (c *digestChain) days() []LedgerDay {
	first := slices.MinFunc(c.lines, func(a, b ledger.Line) int { return a.At.Compare(b.At) }).At
	last := slices.MaxFunc(c.lines, func(a, b ledger.Line) int { return a.At.Compare(b.At) }).At
	var days []LedgerDay
	for at := dayfile.DayStart(first.Local(), 1); !at.After(dayfile.DayStart(last.Local(), 3)); at = dayfile.DayStart(at, 1) {
		days = append(days, Today(at))
	}
	return days
}

// by sets the world's clock to at, and has the router's round summarise,
// and its digester digest, the days that have ended by then, as they would
// have.
func (c *digestChain) by(at time.Time) {
	c.at = at
	c.ledger.SummariseEnded(at)
	c.ledger.DigestEnded(c.t.Context(), at)
}

// digestDayByDay has the ledger, at half past one of each day, summarise
// and digest the days that have ended, as the router's round and its
// digester do, so each day's digest is built once, the day after it, on the
// one before it; then doing on each day what then does.
func (c *digestChain) digestDayByDay(then func(day LedgerDay)) {
	c.t.Helper()
	for _, day := range c.days() {
		c.by(day.Start.Add(90 * time.Minute))
		yesterday := dayfile.DateOf(dayfile.DayStart(day.Start, -1))
		if _, built := c.digests.handed[yesterday]; !built && c.filedOn(yesterday) {
			c.t.Fatalf("%s: %s, its lines filed, isn't digested by %v", c.name, yesterday, c.at)
		}
		then(day)
	}
}

// filedOn reports whether a line is filed on the day with the given date.
func (c *digestChain) filedOn(date string) bool {
	return slices.ContainsFunc(c.lines, func(l ledger.Line) bool { return dayfile.DateOf(l.At) == date })
}

// checkChained checks the Digests gave each state of a session folded on the
// state they gave of it on the last day before that named it, its bytes as
// given, or on none where no day before named it; or, where they gave none
// of the session handed resting on that state, only as its page's points
// couldn't take the days since in, as takesSince has it, on none, the
// session handed again read whole; and that they gave a state of every
// session handed them, each with a part. It counts the states folded on
// another, and names each session given none resting on a state by its
// world and id, as refused has them. It returns how many states the
// longest chain of them holds.
func (c *digestChain) checkChained() int {
	c.t.Helper()
	last := make(map[string]string)
	chains := make(map[string]int)
	for _, date := range slices.Sorted(maps.Keys(c.digests.states)) {
		for _, id := range slices.Sorted(maps.Keys(c.digests.states[date])) {
			base, was := c.digests.bases[date][id], last[id]
			refused, again := c.digests.refused[date][id]
			switch {
			case again && (refused.base != was || base != nil):
				c.t.Errorf("%s, session %s on %s: given no state resting on the state of %q, then folded on %+v; want it handed on the state of %q, then read whole",
					c.name, id, date, refused.base, base, was)
			case again && (refused.takes || refused.restoring != nil):
				c.t.Errorf("%s, session %s on %s: given no state resting on the state of %s, its points taking the answers since %t, restored with error %v; "+
					"want it given none only as they can't", c.name, id, date, refused.base, refused.takes, refused.restoring)
			case !again && was == "" && base != nil:
				c.t.Errorf("%s, session %s on %s: folded on the state of %s, want none, as no day before named it", c.name, id, date, base.Date)
			case !again && was != "" && (base == nil || base.Date != was || !bytes.Equal(base.State, c.digests.states[was][id])):
				c.t.Errorf("%s, session %s on %s: folded on %+v, want the state given of it on %s", c.name, id, date, base, was)
			case c.digests.parts[date][id] == nil:
				c.t.Errorf("%s, session %s on %s: no part given", c.name, id, date)
			}
			if base != nil {
				c.based++
			}
			if again && refused.base != "" {
				c.refused = append(c.refused, fmt.Sprintf("%s, session %s", c.name, id))
			}
			last[id] = date
			chains[id]++
		}
	}
	for _, date := range slices.Sorted(maps.Keys(c.digests.refused)) {
		for _, id := range slices.Sorted(maps.Keys(c.digests.refused[date])) {
			if _, given := c.digests.states[date][id]; !given {
				c.t.Errorf("%s, session %s on %s: no state given", c.name, id, date)
			}
		}
	}
	return slices.Max(slices.Concat([]int{0}, slices.Collect(maps.Values(chains))))
}

// checkRestored checks each state the Digests gave, restored onto each later
// day and told on with the session's lines filed since, as heldAsOf has it,
// read with each of the tables given, is the session read whole on that
// day, as readOf reads it, sealed at the same horizon, a moment on the day
// the seed gives: flights aside, as sameHeld has them. It returns how many
// it restored.
func (c *digestChain) checkRestored(prices []pricedAs) int {
	c.t.Helper()
	rng := rand.New(rand.NewPCG(c.seed, 13))
	restored := 0
	for _, on := range c.days() {
		horizon := instantOf(on.Start.Add(time.Duration(rng.Int64N(int64(dayfile.DayStart(on.Start, 1).Sub(on.Start))))))
		for _, date := range slices.Sorted(maps.Keys(c.digests.states)) {
			if date >= on.Date {
				continue
			}
			for _, id := range slices.Sorted(maps.Keys(c.digests.states[date])) {
				base := baseOf(date, c.digests.states[date][id])
				for _, p := range prices {
					got, err := heldAsOf(id, ledger.SessionFrom{Base: base, Lines: c.filed(id, date, on.Date)}, nil, p.read, on, horizon, false)
					if err != nil {
						c.t.Fatalf("%s, session %s, its state of %s held on %s: %v", c.name, id, date, on.Date, err)
					}
					c.same(fmt.Sprintf("session %s, its state of %s restored on %s, %s", id, date, on.Date, p.name), got,
						readOf(c.filed(id, "", on.Date), nil, p.read, on, horizon, false), date)
					restored++
				}
			}
		}
	}
	return restored
}

// checkPaged checks the page each state the Digests gave a part with holds,
// restored with its chain of parts, as they gave each, from the part of a
// state folded on none, onto the day after its own, as restoredFrom has it,
// is the page folded whole at its day's end, of every line of the session's
// by then, and every account's answers of the days through it. It returns
// how many it restored.
func (c *digestChain) checkPaged() int {
	c.t.Helper()
	restored := 0
	for _, date := range slices.Sorted(maps.Keys(c.digests.states)) {
		day := dayOf(date)
		end := endOfDay(day)
		for _, id := range slices.Sorted(maps.Keys(c.digests.states[date])) {
			if c.digests.parts[date][id] == nil {
				continue
			}
			from := ledger.SessionFrom{Base: baseOf(date, c.digests.states[date][id]), Parts: c.chainOf(date, id)}
			got, err := restoredFrom(from, ledger.Pricing, Today(end.time()), pageWhole)
			if err != nil {
				c.t.Fatalf("%s, session %s, its page of %s restored with %d parts: %v", c.name, id, date, len(from.Parts), err)
			}
			want := readOf(arrivedBefore(c.filed(id, "", date), end.time()), c.answers(id, date), ledger.Pricing, day, end, true)
			if !samePage(got.page, want.page) {
				c.t.Errorf("%s, session %s, its page of %s restored with %d parts, is\n%s\nwant, as folded whole\n%s",
					c.name, id, date, len(from.Parts), pageSaid(got.page), pageSaid(want.page))
			}
			restored++
		}
	}
	return restored
}

// chainOf is the chain of parts of the state the Digests gave of the session
// with the given id on the day with the given date, oldest first: from the
// part of a state folded on none, then that of each folded on the one
// before, each with its times, as they gave each.
func (c *digestChain) chainOf(date, id string) []ledger.SessionPart {
	var chain []ledger.SessionPart
	for date != "" {
		chain = append(chain, ledger.SessionPart{Date: date, Part: c.digests.parts[date][id], Times: c.digests.times[date][id]})
		date = ledger.SessionFrom{Base: c.digests.bases[date][id]}.BaseDate()
	}
	slices.Reverse(chain)
	return chain
}

// answers are every account's answers of the world's lines filed on the day
// with the given date and before, by date, as answeredOf has each of the
// page of the session with the given id.
func (c *digestChain) answers(id, through string) map[string][]answered {
	byDate := linesByDate(c.lines)
	maps.DeleteFunc(byDate, func(date string, _ []ledger.Line) bool { return date > through })
	return answersOf(byDate, id)
}

// baseOf is a base of the state given, as the digest of the local day with
// the given date holds it.
func baseOf(date string, state json.RawMessage) *ledger.SessionBase {
	return &ledger.SessionBase{Date: date, State: state}
}

// filed are the lines of the session with the given id filed on the days
// after the one with the date from through the one with the date through,
// in the order filed.
func (c *digestChain) filed(id, from, through string) []ledger.Line {
	return slices.DeleteFunc(slices.Clone(c.lines), func(l ledger.Line) bool {
		date := dayfile.DateOf(l.At)
		return l.Session != id || date <= from || date > through
	})
}

// checkFollowed checks each session of the world as a Follower of the
// ledger's files gives it at now, resting on the newest state that stands
// for it, held as heldAsOf holds it on the day now falls on, priced as each
// of the tables given reads, sealed at the horizon of now, is the session
// the same Follower gives read whole, as readOf reads it: flights aside,
// through the day of the state it rests on. And as it gives it resting on
// its page, as RestOnPage has it, held paged, its points of every account's
// answers the read took, is the session read whole, its page of every
// account's answers that read took, as samePaged has them: priced as the
// first of the tables reads, as a page holds nothing priced.
func (c *digestChain) checkFollowed(now time.Time, prices []pricedAs) {
	c.t.Helper()
	ids := sessionsIn(c.lines)
	follower := ledger.NewFollower(c.state, func() time.Time { return now }, ledger.CapsOf(nil, nil), DigestVersion, quiet)
	based := follower.SessionsOf(ids, nil, nil)
	pagedAnswers, wholeAnswers := make(takenAnswers), make(takenAnswers)
	paged := follower.SessionsOf(ids, onPage, pagedAnswers.take)
	whole := follower.SessionsOf(ids, eachWhole, wholeAnswers.take)
	day, horizon := Today(now), horizonAt(now, nil)
	for _, id := range slices.Sorted(maps.Keys(whole)) {
		from, through := based[id], ""
		if from.Base != nil {
			through = from.Base.Date
			c.followed++
		}
		for _, p := range prices {
			got, err := heldAsOf(id, from, nil, p.read, day, horizon, false)
			if err != nil {
				c.t.Fatalf("%s, session %s, followed at %v: %v", c.name, id, now, err)
			}
			c.same(fmt.Sprintf("session %s followed at %v, resting on the state of %q, %s", id, now, through, p.name), got,
				readOf(whole[id].Lines, nil, p.read, day, horizon, false), through)
		}
		p := prices[0]
		want := readOf(whole[id].Lines, wholeAnswers.of(id, ""), p.read, day, horizon, true)
		c.samePaged(id, fmt.Sprintf("session %s followed at %v, resting on its page, %s", id, now, p.name), paged[id],
			pagedAnswers, p.read, day, horizon, want)
	}
}

// samePaged checks from, the session with the given id resting on its page
// as what names it says, held paged on the day given behind horizon, as
// heldAsOf holds it, its points of the answers taken, is want, the session
// read whole, its page tallied: flights aside, through the day of the state
// it rests on, as sameHeld has them, and its page as samePage has it. Where
// it rests on a state, but heldAsOf refuses it, it's refused only as its
// points can't take the days since in, as takesSince has it, and named
// among those refused.
func (c *digestChain) samePaged(id, what string, from ledger.SessionFrom, answers takenAnswers, prices ledger.Table, day LedgerDay, horizon instant, want *heldSession) {
	c.t.Helper()
	through := from.BaseDate()
	got, err := heldAsOf(id, from, answers, prices, day, horizon, true)
	switch {
	case err != nil && from.Base == nil:
		c.t.Fatalf("%s, %s, read whole: %v", c.name, what, err)
	case err != nil:
		if takes, restoring := takesSince(id, from, answers, day); takes || restoring != nil {
			c.t.Fatalf("%s, %s, on the state of %s and %d parts, refused: %v; its points take the answers since %t, restored with error %v; "+
				"want it refused only as they can't", c.name, what, through, len(from.Parts), err, takes, restoring)
		}
		c.pagesRefused = append(c.pagesRefused, fmt.Sprintf("%s, session %s", c.name, id))
		return
	case !sameHeld(unpaged(got, true), unpaged(want, true), through) || !samePage(got.page, want.page):
		q := queryOf(c.lines)
		c.t.Errorf("%s, %s, on the state of %q, holds\n%s\nits page\n%s\nwant, as read whole\n%s\nits page\n%s",
			c.name, what, through, heldSaid(got, q), pageSaid(got.page), heldSaid(want, q), pageSaid(want.page))
	}
	if from.Base != nil {
		c.pagesFollowed++
	}
}

// takesSince reports whether the page from rests on, the page of the session
// with the given id, its state restored onto the day d to fold, as
// restoredFrom has it, whose points need none of its chain's times, takes in
// the answers taken of each day after its state's, as tookDays has them; or
// why it won't restore.
func takesSince(id string, from ledger.SessionFrom, answers takenAnswers, d LedgerDay) (bool, error) {
	s, err := restoredFrom(from, ledger.Pricing, d, pageToFold)
	if err != nil {
		return false, err
	}
	_, takes := s.page.points.tookDays(answers.of(id, from.BaseDate()), d)
	return takes, nil
}

// same checks got, a session held as what names it says, is want, the
// session read whole, as sameHeld has them, flights through the day with the
// given date aside.
func (c *digestChain) same(what string, got, want *heldSession, through string) {
	c.t.Helper()
	if !sameHeld(got, want, through) {
		q := queryOf(c.lines)
		c.t.Errorf("%s, %s, holds\n%s\nwant, as read whole\n%s", c.name, what, heldSaid(got, q), heldSaid(want, q))
	}
}

// A session as of a day is held as the store would hold it had it read
// every line of it whole: resting on no base, of its lines; resting on its
// state at an earlier day's end, of that and its lines since. Its page
// tallied, it's the page read whole too: resting on its state, restored with
// its part of that day, its points taking in the answers of the days since.
func TestASessionAsOfADayIsHeldAsReadWhole(t *testing.T) {
	lines := generatedSession(2)
	day, end := digestDay()
	on := Today(dayfile.DayStart(day.Start, 2))
	horizon := instantOf(on.Start.Add(9 * time.Hour))
	id, byDate := lines[0].Session, linesByDate(lines)
	answers, taken := answersOf(byDate, id), answersTaken(byDate)
	f, err := foldedAsOf(id, ledger.SessionFrom{Lines: arrivedBefore(lines, end)}, answersTaken(datesThrough(byDate, day.Date)), day)
	if err != nil {
		t.Fatal(err)
	}
	since := slices.DeleteFunc(slices.Clone(lines), func(l ledger.Line) bool { return l.At.Before(end) })
	if len(since) == 0 {
		t.Fatalf("session 2 has no request after %v, to tell on", end)
	}
	twice := slices.Insert(slices.Clone(since), 1, since[0])
	base, chain := baseOf(day.Date, f.State), []ledger.SessionPart{{Date: day.Date, Part: f.Part, Times: f.Times}}
	tests := []struct {
		name  string
		from  ledger.SessionFrom
		whole []ledger.Line
	}{
		{name: "resting on no base", from: ledger.SessionFrom{Lines: lines}, whole: lines},
		{name: "resting on its state at an earlier day's end", from: ledger.SessionFrom{Base: base, Parts: chain, Lines: since}, whole: lines},
		{
			name: "resting on its state, a line since filed twice", from: ledger.SessionFrom{Base: base, Parts: chain, Lines: twice},
			whole: append(arrivedBefore(lines, end), twice...),
		},
	}
	for _, tt := range tests {
		for _, paged := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s, its page tallied %t", tt.name, paged), func(t *testing.T) {
				var given map[string][]answered
				if paged {
					given = answers
				}
				got, err := heldAsOf(id, tt.from, taken, changingPrices(), on, horizon, paged)
				if err != nil {
					t.Fatal(err)
				}
				want := readOf(tt.whole, given, changingPrices(), on, horizon, paged)
				if !sameHeld(unpaged(got, true), unpaged(want, true), day.Date) || paged && !samePage(got.page, want.page) {
					q := queryOf(lines)
					t.Errorf("holds\n%s\nits page %s\nwant, as read whole\n%s\nits page %s", heldSaid(got, q), pageSaid(got.page), heldSaid(want, q), pageSaid(want.page))
				}
			})
		}
	}
}

// answersOf returns the answers byDate holds, by the date of the day whose
// files hold them, each answered as answeredOf has it, those of the session
// with the given id its own: as a read's takenAnswers give them of a page.
func answersOf(byDate map[string][]ledger.Line, id string) map[string][]answered {
	answers := make(map[string][]answered, len(byDate))
	for date, lines := range byDate {
		for i := range lines {
			if isAnswer(&lines[i]) {
				answers[date] = append(answers[date], answeredOf(&lines[i], id))
			}
		}
	}
	return answers
}

// answersTaken returns the answers a read of the lines byDate holds takes,
// by the date of the day whose files hold them, each line taken as
// takenAnswers takes it, in the order byDate holds them.
func answersTaken(byDate map[string][]ledger.Line) takenAnswers {
	taken := make(takenAnswers)
	for date, lines := range byDate {
		for _, l := range lines {
			taken.take(date, l)
		}
	}
	return taken
}

// linesByDate are lines by the date of the day each arrived on, as the
// ledger files them, in their order.
func linesByDate(lines []ledger.Line) map[string][]ledger.Line {
	byDate := make(map[string][]ledger.Line)
	for _, l := range lines {
		date := dayfile.DateOf(l.At)
		byDate[date] = append(byDate[date], l)
	}
	return byDate
}

// stateOf is the state of the session of lines at the end of the day d, as
// a day's digest keeps it, as foldedAsOf folds it, of its lines alone.
func stateOf(t *testing.T, lines []ledger.Line, d LedgerDay) json.RawMessage {
	t.Helper()
	f, err := foldedAsOf(lines[0].Session, ledger.SessionFrom{Lines: lines}, nil, d)
	if err != nil {
		t.Fatal(err)
	}
	return f.State
}

// A session resting on a base is held only behind a horizon at its base's
// own, the end of its base's day, or after it, where it's held as read
// whole: behind an earlier one, the base holds sealed what a whole read
// holds open, so that's an error, for its caller to read it whole.
func TestASessionAsOfADayIsHeldOnABaseOnlyFromTheBasesEnd(t *testing.T) {
	lines := generatedSession(2)
	day, end := digestDay()
	state := stateOf(t, arrivedBefore(lines, end), day)
	from := ledger.SessionFrom{Base: baseOf(day.Date, state), Lines: slices.DeleteFunc(slices.Clone(lines), func(l ledger.Line) bool { return l.At.Before(end) })}
	on := Today(end)
	tests := []struct {
		name    string
		after   time.Duration
		refused bool
	}{
		{name: "a minute before its end", after: -time.Minute, refused: true},
		{name: "half an hour before its end", after: -30 * time.Minute, refused: true},
		{name: "an hour before its end", after: -time.Hour, refused: true},
		{name: "six hours before its end", after: -6 * time.Hour, refused: true},
		{name: "at its end", after: 0},
		{name: "a minute after its end", after: time.Minute},
		{name: "six hours after its end", after: 6 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			horizon := instantOf(end.Add(tt.after))
			got, err := heldAsOf("s", from, nil, ledger.Pricing, on, horizon, false)
			switch {
			case tt.refused:
				if err == nil {
					t.Errorf("held behind %v, want an error", horizon.time())
				}
			case err != nil:
				t.Fatal(err)
			default:
				if want := readOf(lines, nil, ledger.Pricing, on, horizon, false); !sameHeld(got, want, day.Date) {
					q := queryOf(lines)
					t.Errorf("holds\n%s\nwant, as read whole\n%s", heldSaid(got, q), heldSaid(want, q))
				}
			}
		})
	}
}

// A session whose base won't decode, or restore onto the day it's held on,
// isn't held: that's an error, for its caller to read it whole.
func TestASessionAsOfADayIsntHeldOnABaseThatWontRestore(t *testing.T) {
	day, end := digestDay()
	state := stateOf(t, arrivedBefore(generatedSession(2), end), day)
	next := Today(end)
	tests := []struct {
		name  string
		date  string
		state string
		on    LedgerDay
	}{
		{name: "a state that isn't JSON", date: day.Date, state: `{"d":`, on: next},
		{name: "a state of none", date: day.Date, state: `null`, on: next},
		{name: "a state of another day than its digest's", date: next.Date, state: string(state), on: Today(dayfile.DayStart(day.Start, 2))},
		{name: "held on its own day", date: day.Date, state: string(state), on: day},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from := ledger.SessionFrom{Base: baseOf(tt.date, json.RawMessage(tt.state))}
			if s, err := heldAsOf("s", from, nil, ledger.Pricing, tt.on, endOfDay(tt.on), false); err == nil {
				t.Errorf("held on %s as %+v, want an error", tt.on.Date, s)
			}
		})
	}
}

// The Digests give a state of each session handed them but those they can't
// hold as of the day's end: one whose base won't decode, or restore, or one
// with a request arrived after the day, as a change of time zone files one.
// Those are left out, for their readers to read whole, and no error is
// given: the others' states are kept.
func TestDigestsLeaveOutASessionTheyCantHoldAsOfTheDaysEnd(t *testing.T) {
	day, end := digestDay()
	lines := generatedSession(2)
	ofDay := arrivedBefore(lines, end)
	after := slices.DeleteFunc(slices.Clone(lines), func(l ledger.Line) bool { return l.At.Before(end) })
	if len(after) == 0 {
		t.Fatalf("session 2 has no request after %v", end)
	}
	tests := []struct {
		name string
		from ledger.SessionFrom
	}{
		{name: "a base that isn't JSON", from: ledger.SessionFrom{Base: baseOf("2026-10-06", json.RawMessage(`{"d":`))}},
		{name: "a base of another day than its digest's", from: ledger.SessionFrom{Base: baseOf("2026-10-05", json.RawMessage(`{"d":"2026-10-06"}`))}},
		{name: "a request arrived after the day", from: ledger.SessionFrom{Lines: append(slices.Clone(ofDay), after[0])}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handed := ledger.DigestDay{Date: day.Date, Start: day.Start, End: end, Sessions: map[string]ledger.SessionFrom{"kept": {Lines: ofDay}, "left": tt.from}}
			states, err := Digests{}.Builder().Digest(t.Context(), handed)
			if _, kept := states["kept"]; err != nil || !kept || len(states) != 1 {
				t.Errorf("gave states of %v, %v; want one of kept alone, and no error", slices.Sorted(maps.Keys(states)), err)
			}
		})
	}
}

// The Digests give no state once their context has ended, but its error.
func TestDigestsFailOnceTheirContextEnds(t *testing.T) {
	day, end := digestDay()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	handed := ledger.DigestDay{Date: day.Date, Start: day.Start, End: end, Sessions: map[string]ledger.SessionFrom{"s": {Lines: arrivedBefore(generatedSession(2), end)}}}
	if states, err := (Digests{}).Builder().Digest(ctx, handed); err == nil || states != nil {
		t.Errorf("gave %d states, %v; want none, and the context's error", len(states), err)
	}
}
