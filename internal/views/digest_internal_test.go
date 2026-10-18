package views

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
// its state of the last day before that named it, as the ledger's files hold
// them, are the sessions read whole: each state restored onto each later
// day, and told on with the session's lines since, as heldAsOf tells them, is
// the session read whole on that day, sealed at the same horizon, flights
// aside, as a restored session holds none; and each session as a Follower of
// the files gives it, resting on its newest state, is the session read whole
// through the same Follower. Of the generated worlds, read with tables other
// than the one the states are built with.
func TestChainedDigestsAreTheSessionsReadWhole(t *testing.T) {
	prices := digestPrices(t)
	var restored, followed, chained int
	for _, w := range chainWorlds {
		for seed := range uint64(chainSeeds) {
			c := newDigestChain(t, fmt.Sprintf("%s %d", w.name, seed), seed, w.lines(seed))
			c.digestDayByDay(func(day LedgerDay) { c.checkFollowed(day.Start.Add(12*time.Hour), prices) })
			chained = max(chained, c.checkChained())
			restored += c.checkRestored(prices)
			followed += c.followed
		}
	}
	if restored == 0 || followed == 0 || chained < 4 {
		t.Errorf("%d states restored onto a later day, %d sessions followed resting on one, %d states the longest chain: want some of each, and a chain of four",
			restored, followed, chained)
	}
}

// A state the Digests can't restore, as one spoiled, is folded past: the day
// after it the session is handed again read whole, so every later day's
// digest holds its state, the session read whole, and no day stays due.
func TestADigestPastASpoiledStateHoldsTheSessionReadWhole(t *testing.T) {
	prices := digestPrices(t)
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
// the order filed; its state directory; the ledger, by the clock at; and how
// many sessions a Follower gave resting on a state.
type digestChain struct {
	t        *testing.T
	name     string
	seed     uint64
	lines    []ledger.Line
	state    string
	at       time.Time
	ledger   *ledger.Ledger
	digests  *recordedDigests
	followed int
}

// recordedDigests are the views' Digests, each day handed them recorded, by
// its date, as last handed, with the states they gave of it, and how many
// times they were handed one; but of the day with the date spoiled, where
// it's set, they give spoiledState of each session, recording none.
type recordedDigests struct {
	Digests
	handed  map[string]ledger.DigestDay
	states  map[string]map[string]json.RawMessage
	calls   int
	spoiled string
}

// spoiledState is a state as one edited by hand might be: JSON, but of
// another day than its digest's, so it won't restore.
const spoiledState = `{"d":"2000-01-01"}`

// Digest gives what the views' Digests give of day, recording both, or
// spoiled states, of the day spoiled.
func (r *recordedDigests) Digest(ctx context.Context, day ledger.DigestDay) (map[string]json.RawMessage, error) {
	r.calls++
	r.handed[day.Date] = day
	states, err := r.Digests.Digest(ctx, day)
	if day.Date == r.spoiled {
		for id := range states {
			states[id] = json.RawMessage(spoiledState)
		}
		return states, err
	}
	if r.states[day.Date] == nil {
		r.states[day.Date] = make(map[string]json.RawMessage)
	}
	maps.Copy(r.states[day.Date], states)
	return states, err
}

// quiet is a logger of nothing.
var quiet = slog.New(slog.DiscardHandler)

// newDigestChain writes lines as the ledger files them, each under the day
// it arrived on, in a state directory of the test's own, and opens the
// ledger there, digesting with recordedDigests.
func newDigestChain(t *testing.T, name string, seed uint64, lines []ledger.Line) *digestChain {
	t.Helper()
	c := &digestChain{t: t, name: name, seed: seed, lines: lines, state: t.TempDir(),
		digests: &recordedDigests{handed: make(map[string]ledger.DigestDay), states: make(map[string]map[string]json.RawMessage)}}
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
	c.ledger = ledger.Open(ledger.Dir(c.state), 400*24*time.Hour, func() time.Time { return c.at }, ledger.HistoryIn(readings.Files(readings.Dir(c.state), quiet)),
		ledger.CapsOf(nil, nil), quiet, ledger.Digesting(c.digests))
	return c
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

// digestDayByDay has the ledger, at half past one of each day, summarise
// and digest the days that have ended, as the router's round and its
// digester do, so each day's digest is built once, the day after it, on the
// one before it; then doing on each day what then does.
func (c *digestChain) digestDayByDay(then func(day LedgerDay)) {
	c.t.Helper()
	for _, day := range c.days() {
		c.at = day.Start.Add(90 * time.Minute)
		c.ledger.SummariseEnded(c.at)
		c.ledger.DigestEnded(c.t.Context(), c.at)
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

// checkChained checks the ledger handed the Digests each session its day
// names resting on the state they gave of it on the last day before that
// named it, its bytes as given, or on none where no day before named it;
// and that they gave a state of every session handed them. It returns how
// many states the longest chain of them holds.
func (c *digestChain) checkChained() int {
	c.t.Helper()
	last := make(map[string]string)
	chains := make(map[string]int)
	for _, date := range slices.Sorted(maps.Keys(c.digests.handed)) {
		day, states := c.digests.handed[date], c.digests.states[date]
		for _, id := range slices.Sorted(maps.Keys(day.Sessions)) {
			from, was := day.Sessions[id], last[id]
			switch {
			case was == "" && from.Base != nil:
				c.t.Errorf("%s, session %s on %s: handed resting on the state of %s, want none, as no day before named it", c.name, id, date, from.Base.Date)
			case was != "" && (from.Base == nil || from.Base.Date != was || !bytes.Equal(from.Base.State, c.digests.states[was][id])):
				c.t.Errorf("%s, session %s on %s: handed resting on %+v, want the state given of it on %s", c.name, id, date, from.Base, was)
			}
			if _, given := states[id]; !given {
				c.t.Errorf("%s, session %s on %s: no state given", c.name, id, date)
			}
			last[id] = date
			chains[id]++
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
					got, err := heldAsOf(ledger.SessionFrom{Base: base, Lines: c.filed(id, date, on.Date)}, p.read, on, horizon)
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
// through the day of the state it rests on.
func (c *digestChain) checkFollowed(now time.Time, prices []pricedAs) {
	c.t.Helper()
	ids := sessionsIn(c.lines)
	follower := ledger.NewFollower(c.state, func() time.Time { return now }, ledger.CapsOf(nil, nil), DigestVersion, quiet)
	based := follower.SessionsOf(ids, func(string) bool { return false }, nil)
	whole := follower.SessionsOf(ids, eachWhole, nil)
	day, horizon := Today(now), horizonAt(now, nil)
	for _, id := range slices.Sorted(maps.Keys(whole)) {
		from, through := based[id], ""
		if from.Base != nil {
			through = from.Base.Date
			c.followed++
		}
		for _, p := range prices {
			got, err := heldAsOf(from, p.read, day, horizon)
			if err != nil {
				c.t.Fatalf("%s, session %s, followed at %v: %v", c.name, id, now, err)
			}
			c.same(fmt.Sprintf("session %s followed at %v, resting on the state of %q, %s", id, now, through, p.name), got,
				readOf(whole[id].Lines, nil, p.read, day, horizon, false), through)
		}
	}
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
// state at an earlier day's end, of that and its lines since.
func TestASessionAsOfADayIsHeldAsReadWhole(t *testing.T) {
	lines := generatedSession(2)
	day, end := digestDay()
	on := Today(dayfile.DayStart(day.Start, 2))
	horizon := instantOf(on.Start.Add(9 * time.Hour))
	state, err := stateAsOf(ledger.SessionFrom{Lines: arrivedBefore(lines, end)}, day)
	if err != nil {
		t.Fatal(err)
	}
	since := slices.DeleteFunc(slices.Clone(lines), func(l ledger.Line) bool { return l.At.Before(end) })
	if len(since) == 0 {
		t.Fatalf("session 2 has no request after %v, to tell on", end)
	}
	twice := slices.Insert(slices.Clone(since), 1, since[0])
	tests := []struct {
		name  string
		from  ledger.SessionFrom
		whole []ledger.Line
	}{
		{name: "resting on no base", from: ledger.SessionFrom{Lines: lines}, whole: lines},
		{name: "resting on its state at an earlier day's end", from: ledger.SessionFrom{Base: baseOf(day.Date, state), Lines: since}, whole: lines},
		{
			name: "resting on its state, a line since filed twice", from: ledger.SessionFrom{Base: baseOf(day.Date, state), Lines: twice},
			whole: append(arrivedBefore(lines, end), twice...),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := heldAsOf(tt.from, changingPrices(), on, horizon)
			if err != nil {
				t.Fatal(err)
			}
			want := readOf(tt.whole, nil, changingPrices(), on, horizon, false)
			if !sameHeld(got, want, day.Date) {
				q := queryOf(lines)
				t.Errorf("holds\n%s\nwant, as read whole\n%s", heldSaid(got, q), heldSaid(want, q))
			}
		})
	}
}

// A session resting on a base is held only behind a horizon at its base's
// own, the end of its base's day, or after it, where it's held as read
// whole: behind an earlier one, the base holds sealed what a whole read
// holds open, so that's an error, for its caller to read it whole.
func TestASessionAsOfADayIsHeldOnABaseOnlyFromTheBasesEnd(t *testing.T) {
	lines := generatedSession(2)
	day, end := digestDay()
	state, err := stateAsOf(ledger.SessionFrom{Lines: arrivedBefore(lines, end)}, day)
	if err != nil {
		t.Fatal(err)
	}
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
			got, err := heldAsOf(from, ledger.Pricing, on, horizon)
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
	state, err := stateAsOf(ledger.SessionFrom{Lines: arrivedBefore(generatedSession(2), end)}, day)
	if err != nil {
		t.Fatal(err)
	}
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
			if s, err := heldAsOf(from, ledger.Pricing, tt.on, endOfDay(tt.on)); err == nil {
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
			states, err := Digests{}.Digest(t.Context(), handed)
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
	if states, err := (Digests{}).Digest(ctx, handed); err == nil || states != nil {
		t.Errorf("gave %d states, %v; want none, and the context's error", len(states), err)
	}
}
