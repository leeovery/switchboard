package views

import (
	"encoding/json"
	"fmt"
	"maps"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// digestWorlds are the generated worlds whose sessions are stored at the end
// of each day they ran and restored onto each later day: those the stored
// request's tests decode, a page's among other sessions', some of whose
// lines have no id; a session whose requests never say their class, as a
// Claude Code from before they did sends them; and one whose requests start
// saying it on 8 October, its second day.
var digestWorlds = append(slices.Clip(storedWorlds), []struct {
	name  string
	lines func(seed uint64) []ledger.Line
}{
	{name: "unclassed", lines: func(seed uint64) []ledger.Line {
		return unclassedWhere(generatedSession(seed), func(*ledger.Line) bool { return true })
	}},
	{name: "classed from its second day", lines: func(seed uint64) []ledger.Line {
		second := Today(foldDays[1]).Start
		return unclassedWhere(generatedSession(seed), func(l *ledger.Line) bool { return l.At.Before(second) })
	}},
}...)

// unclassedWhere is lines, none of those unsaid reports true of saying its
// class or its subagent, as a Claude Code from before requests said them
// sends them.
func unclassedWhere(lines []ledger.Line, unsaid func(l *ledger.Line) bool) []ledger.Line {
	for i := range lines {
		if unsaid(&lines[i]) {
			lines[i].Class, lines[i].AgentID = "", ""
		}
	}
	return lines
}

// digestSeeds is how many seeds of each world the round trip runs.
const digestSeeds = 30

// pricedAs are the price tables a round trip's states are built with, and
// those they're restored and read whole with, by a name for the moment they
// make.
type pricedAs struct {
	name        string
	built, read ledger.Table
}

// digestPrices are the tables the round trip runs with: a dated change of
// Claude Opus 5.5's writes to the cache from the worlds' second day, as
// changingPrices has it, built and read with; and the built-in table, read
// with the config's prices of those writes in place of its own, as an
// override prices every day alike.
func digestPrices(t *testing.T) []pricedAs {
	t.Helper()
	overridden, passed := ledger.Pricing.With(config.Prices{Models: map[string]config.ModelPrices{foldOpus: {CacheWrite5m: new(7.5), CacheWrite1h: new(12.0)}}})
	if len(passed) > 0 {
		t.Fatalf("the config's prices pass over %q", passed)
	}
	return []pricedAs{
		{name: "a dated change", built: changingPrices(), read: changingPrices()},
		{name: "a config override", built: ledger.Pricing, read: overridden},
	}
}

// A session stored as it stands at the end of each day it ran, through the
// bytes a digest holds, then restored onto each later day and told on with
// its requests since, as the store tells them, is the session read whole on
// that day, sealed at the same horizon on it: flights aside, as a restored
// session holds none. Of generated worlds whose sessions run over several
// days, built with one price table and read with another, meeting each of
// the hard moments tripMoments names.
func TestASessionRestoredFromItsStateIsTheSessionReadWhole(t *testing.T) {
	met := make(map[string]bool)
	prices := digestPrices(t)
	for _, w := range digestWorlds {
		for seed := range uint64(digestSeeds) {
			lines := w.lines(seed)
			for _, id := range sessionsIn(lines) {
				ofSession := slices.DeleteFunc(slices.Clone(lines), func(l ledger.Line) bool { return l.Session != id })
				requests := heldRequestsOf(ofSession)
				for _, p := range prices {
					r := &restoring{
						t: t, name: fmt.Sprintf("%s %d, session %s, %s", w.name, seed, id, p.name), seed: seed, prices: p,
						lines: ofSession, requests: requests, met: met,
					}
					r.roundTrips()
				}
			}
		}
	}
	moments := slices.Collect(maps.Keys(tripMoments))
	for _, p := range prices {
		moments = append(moments, costedOtherwise(p.name))
	}
	for _, moment := range slices.Sorted(slices.Values(moments)) {
		if !met[moment] {
			t.Errorf("no round trip meets %s", moment)
		}
	}
}

// sessionsIn are the ids of the sessions of lines' requests, in order.
func sessionsIn(lines []ledger.Line) []string {
	var ids []string
	for i := range lines {
		if isRequest(&lines[i]) {
			ids = append(ids, lines[i].Session)
		}
	}
	return slices.Compact(slices.Sorted(slices.Values(ids)))
}

// restoring is a generated session as the round trip stores and restores
// it: named by its world, seed, id and tables; its lines, as filed, and its
// requests, decoded once, unpriced, in order; and the hard moments the round
// trips met.
type restoring struct {
	t        *testing.T
	name     string
	seed     uint64
	prices   pricedAs
	lines    []ledger.Line
	requests []heldRequest
	met      map[string]bool
}

// dayState is a session's state at the end of the day it's of: the session
// read whole on the day, sealed at its end, and the state a digest's bytes
// of it give back.
type dayState struct {
	day    LedgerDay
	held   *heldSession
	stored storedSession
}

// roundTrips stores the session at the end of each day it ran, and holds it,
// restored onto each later day through the third after its last, to the
// session read whole on that day, sealed at a moment on it the seed gives.
func (r *restoring) roundTrips() {
	r.t.Helper()
	rng := rand.New(rand.NewPCG(r.seed, 12))
	var states []dayState
	for _, day := range r.days() {
		end := dayfile.DayStart(day.Start, 1)
		horizon := instantOf(day.Start.Add(time.Duration(rng.Int64N(int64(end.Sub(day.Start))))))
		whole := r.readWhole(end, r.prices.read, day, horizon)
		for _, from := range states {
			r.roundTrip(from, day, horizon, whole)
		}
		if len(r.arrived(day.Start, end)) > 0 {
			states = append(states, r.stateAt(day))
		}
	}
}

// days are the days from the one the session's first request arrived on
// through the third after the one its last did.
func (r *restoring) days() []LedgerDay {
	first, last := r.requests[0].arrived(), r.requests[len(r.requests)-1].arrived()
	var days []LedgerDay
	for at := dayfile.DayStart(first.Local(), 0); !at.After(dayfile.DayStart(last.Local(), 3)); at = dayfile.DayStart(at, 1) {
		days = append(days, Today(at))
	}
	return days
}

// arrived are the session's requests that arrived from from until until, in
// order, afresh.
func (r *restoring) arrived(from, until time.Time) []heldRequest {
	return slices.DeleteFunc(slices.Clone(r.requests), func(q heldRequest) bool { return q.arrived().Before(from) || !q.arrived().Before(until) })
}

// readWhole is the session read whole on the day given, as readOf reads it:
// of its requests that arrived before until, each once, as distinct has
// them, priced by prices on the day, sealed behind the horizon given.
func (r *restoring) readWhole(until time.Time, prices ledger.Table, day LedgerDay, horizon instant) *heldSession {
	return heldSessionOf(pricedAt(distinct(r.arrived(time.Time{}, until)), prices, day.Date), nil, prices, day, horizon, false)
}

// stateAt is the session's state at the end of the day given, read whole
// with the table it's built with, stored, written as a digest's bytes and
// read back.
func (r *restoring) stateAt(day LedgerDay) dayState {
	r.t.Helper()
	end := dayfile.DayStart(day.Start, 1)
	held := r.readWhole(end, r.prices.built, day, instantOf(end))
	st, err := held.stored()
	if err != nil {
		r.t.Fatalf("%s, stored at the end of %s: %v", r.name, day.Date, err)
	}
	data, err := json.Marshal(st)
	if err != nil {
		r.t.Fatalf("%s, stored at the end of %s, written: %v", r.name, day.Date, err)
	}
	var back storedSession
	if err := json.Unmarshal(data, &back); err != nil {
		r.t.Fatalf("%s, stored at the end of %s, read back from %s: %v", r.name, day.Date, data, err)
	}
	return dayState{day: day, held: held, stored: back}
}

// roundTrip restores the session's state from onto the day given, tells it
// on with its requests since, each once, as distinct has them, priced on the
// day, sealing it at the horizon given, and holds it to whole, the session
// read whole on the day, noting the hard moments it meets.
func (r *restoring) roundTrip(from dayState, day LedgerDay, horizon instant, whole *heldSession) {
	r.t.Helper()
	got, err := from.stored.restored(r.prices.read, day)
	if err != nil {
		r.t.Fatalf("%s, stored at the end of %s, restored on %s: %v", r.name, from.day.Date, day.Date, err)
	}
	if since := r.arrived(dayfile.DayStart(from.day.Start, 1), dayfile.DayStart(day.Start, 1)); len(since) > 0 {
		got = got.with(pricedAt(distinct(since), r.prices.read, day.Date))
	}
	got = got.sealedAt(horizon)
	if !sameHeld(got, whole, from.day.Date) {
		q := queryOf(r.lines)
		r.t.Errorf("%s, stored at the end of %s, restored on %s, holds\n%s\nwant, as read whole\n%s", r.name, from.day.Date, day.Date, heldSaid(got, q), heldSaid(whole, q))
	}
	r.note(trip{r: r, from: from, on: day, whole: whole})
}

// sameHeld reports whether two sessions the store holds are the same, but
// for the flights of their requests that arrived on the day with the given
// date or before it, those a session restored from its state at that day's
// end holds none of.
func sameHeld(a, b *heldSession, through string) bool {
	x, y := *a, *b
	x.flights, y.flights = flightsAfter(a.flights, through), flightsAfter(b.flights, through)
	return reflect.DeepEqual(x, y)
}

// flightsAfter are those of flights of the days after the one with the given
// date, afresh.
func flightsAfter(flights map[string]heldFlights, date string) map[string]heldFlights {
	after := maps.Clone(flights)
	maps.DeleteFunc(after, func(d string, _ heldFlights) bool { return d <= date })
	return after
}

// heldSaid is what a session the store holds tells, asked q: its horizon,
// its counts, its open requests, and its sealed story and its story of them
// all, as toldOfHeld has them.
func heldSaid(s *heldSession, q foldQuery) string {
	return fmt.Sprintf("horizon %v, counts %v, open %v,\nsealed %+v,\ntold %+v", s.horizon.time(), s.counts, flightsOf(s.open), toldOfHeld(s.sealed, q), toldOfHeld(s.story, q))
}

// trip is a round trip of a session: its state from, restored on the day
// on, and the session read whole there.
type trip struct {
	r     *restoring
	from  dayState
	on    LedgerDay
	whole *heldSession
}

// next is the day after the one the trip's state is of.
func (tr trip) next() LedgerDay {
	return Today(dayfile.DayStart(tr.from.day.Start, 1))
}

// onDay are the session's requests that arrived on the day given, as
// restoring's arrived has them.
func (tr trip) onDay(day LedgerDay) []heldRequest {
	return tr.r.arrived(day.Start, dayfile.DayStart(day.Start, 1))
}

// stateMoves are the moves of the story the trip's state was built of.
func (tr trip) stateMoves() []move {
	return tr.from.held.sealed.moves
}

// wholeMove is the move of the session read whole made by the request that
// made m, nil where it holds none.
func (tr trip) wholeMove(m move) *move {
	i := slices.IndexFunc(tr.whole.story.moves, func(w move) bool { return w.by == m.by })
	if i < 0 {
		return nil
	}
	return &tr.whole.story.moves[i]
}

// The hard moments of a round trip that the reads over digests meet too, of
// a session resting on its state of a day D, read on a day T.
const (
	settledAfter = "a move on D settled by a request after midnight"
	flightAcross = "a request in flight across midnight, arrived on D and ended on D+1"
	agentAcross  = "a subagent's thread first seen on D continuing on D+1"
	idlessStored = "a request without an id, stored"
	idlessLater  = "a request without an id, told on after D"
)

// tripMoments are the hard moments a round trip meets, each with what tells
// a trip met it, of its state of a day D restored on a day T.
var tripMoments = map[string]func(tr trip) bool{
	settledAfter: func(tr trip) bool {
		return slices.ContainsFunc(tr.stateMoves(), func(m move) bool { w := tr.wholeMove(m); return !m.settled && w != nil && w.settled })
	},
	"a move whose rewriting request found the cache cold": func(tr trip) bool {
		return slices.ContainsFunc(tr.stateMoves(), func(m move) bool { return m.settled && m.rewrote == nil && m.written > 0 })
	},
	flightAcross: func(tr trip) bool {
		end := tr.next().Start
		return slices.ContainsFunc(tr.onDay(tr.from.day), func(q heldRequest) bool { return !q.ended().Before(end) })
	},
	agentAcross: func(tr trip) bool {
		before := tr.r.arrived(time.Time{}, tr.from.day.Start)
		return slices.ContainsFunc(tr.onDay(tr.from.day), func(q heldRequest) bool {
			continues := func(o heldRequest) bool { return o.agentID == q.agentID }
			return q.agentID.Value() != "" && !slices.ContainsFunc(before, continues) && slices.ContainsFunc(tr.onDay(tr.next()), continues)
		})
	},
	"a session whose requests never say their class": func(tr trip) bool { return tr.whole.story.begun && !tr.whole.story.classed },
	"a session whose requests start saying their class on D+1": func(tr trip) bool {
		return !tr.from.held.sealed.classed && slices.ContainsFunc(tr.onDay(tr.next()), func(q heldRequest) bool { return q.class.Value() != "" })
	},
	idlessStored: func(tr trip) bool {
		return slices.ContainsFunc(tr.r.arrived(time.Time{}, tr.next().Start), func(q heldRequest) bool { return q.request == "" })
	},
	idlessLater: func(tr trip) bool {
		return slices.ContainsFunc(tr.r.arrived(tr.next().Start, dayfile.DayStart(tr.on.Start, 1)), func(q heldRequest) bool { return q.request == "" })
	},
	"a session idle for days between D and T": func(tr trip) bool {
		return !tr.on.Start.Before(dayfile.DayStart(tr.from.day.Start, 3)) && len(tr.r.arrived(tr.next().Start, tr.on.Start)) == 0
	},
}

// costedOtherwise is the moment of a round trip with the tables named that
// restores a move whose cost on its day T is otherwise than it was as built
// on its state's day D.
func costedOtherwise(prices string) string {
	return prices + ": a move costing otherwise on T than as built on D"
}

// note notes the hard moments tr meets.
func (r *restoring) note(tr trip) {
	for moment, meets := range tripMoments {
		r.met[moment] = r.met[moment] || meets(tr)
	}
	otherwise := costedOtherwise(r.prices.name)
	r.met[otherwise] = r.met[otherwise] || slices.ContainsFunc(tr.stateMoves(), func(m move) bool {
		w := tr.wholeMove(m)
		return m.cost != nil && w != nil && w.cost != nil && *w.cost != *m.cost
	})
}

// digestDay is the day the tests of a single state store it on, and the
// start of the day after it.
func digestDay() (day LedgerDay, end time.Time) {
	day = Today(foldDays[0])
	return day, dayfile.DayStart(day.Start, 1)
}

// arrivedBefore are those of lines that arrived before until.
func arrivedBefore(lines []ledger.Line, until time.Time) []ledger.Line {
	return slices.DeleteFunc(slices.Clone(lines), func(l ledger.Line) bool { return !l.At.Before(until) })
}

// A session's state is stored only as it stands at the end of its day:
// sealed at the start of the day after its own, none of its requests open,
// its page tallied or not, what the page carries stored where it is. Any
// other is refused, as its caller's fault.
func TestASessionsStateIsStoredOnlyAsItStandsAtItsDaysEnd(t *testing.T) {
	lines := generatedSession(2)
	day, end := digestDay()
	if len(arrivedBefore(lines, end)) == len(lines) {
		t.Fatalf("session 2 has no request after %v, to hold open", end)
	}
	tests := []struct {
		name            string
		through, sealed time.Time
		paged           bool
		refused         bool
	}{
		{name: "sealed at its day's end", through: end, sealed: end},
		{name: "sealed before its day's end", through: end, sealed: end.Add(-time.Nanosecond), refused: true},
		{name: "sealed after its day's end", through: end, sealed: end.Add(time.Nanosecond), refused: true},
		{name: "a request of the next day held open", through: end.Add(24 * time.Hour), sealed: end, refused: true},
		{name: "its page tallied", through: end, sealed: end, paged: true},
		{name: "its page tallied, sealed before its day's end", through: end, sealed: end.Add(-time.Nanosecond), paged: true, refused: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			held := readOf(arrivedBefore(lines, tt.through), nil, ledger.Pricing, day, instantOf(tt.sealed), tt.paged)
			st, err := held.stored()
			if (err != nil) != tt.refused {
				t.Errorf("stored with error %v, want it refused %t", err, tt.refused)
			}
			if paged := st.Page != nil; err == nil && paged != tt.paged {
				t.Errorf("stored holding what its page carries %t, want %t", paged, tt.paged)
			}
		})
	}
}

// A session's state is restored only onto a later day than its own, of a
// date that's one: on its own day, or an earlier one, it's refused.
func TestASessionsStateIsRestoredOnlyOntoALaterDay(t *testing.T) {
	day, end := digestDay()
	tests := []struct {
		name    string
		date    string
		on      time.Time
		refused bool
	}{
		{name: "the day after", date: day.Date, on: end},
		{name: "days after", date: day.Date, on: end.AddDate(0, 0, 3)},
		{name: "its own day", date: day.Date, on: day.Start, refused: true},
		{name: "the day before", date: day.Date, on: day.Start.Add(-time.Hour), refused: true},
		{name: "a date that isn't one", date: "7 October", on: end, refused: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := storedSession{Date: tt.date}
			if _, err := st.restored(ledger.Pricing, Today(tt.on)); (err != nil) != tt.refused {
				t.Errorf("restored on %s with error %v, want it refused %t", Today(tt.on).Date, err, tt.refused)
			}
		})
	}
}

// A restored session holds none of the flights of its requests, so a line
// of its state's day given again, which it can't know, has it read whole:
// on the day after, as the line is behind its horizon; on a later one,
// unless it's its state's day read again whole, as many requests as it
// holds of that day, which it places, taking none in, as the store places
// a day read again before the one before its own. Read whole, it's told of
// its lines, so never wrong.
func TestARestoredSessionGivenALineOfItsDayAgainIsReadWholeOrPlacedByItsCount(t *testing.T) {
	lines := generatedSession(2)
	day, end := digestDay()
	ofDay := pricedAt(heldRequestsOf(arrivedBefore(lines, end)), ledger.Pricing, day.Date)
	if len(ofDay) < 2 {
		t.Fatalf("session 2 has %d requests on %s, want two at least", len(ofDay), day.Date)
	}
	st, err := readOf(arrivedBefore(lines, end), nil, ledger.Pricing, day, instantOf(end), false).stored()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		after  int
		whole  bool
		short  int
		placed bool
	}{
		{name: "the day after, in its tail", after: 1},
		{name: "the day after, the day read again whole", after: 1, whole: true},
		{name: "days after, in its tail", after: 2},
		{name: "days after, the day read again whole", after: 2, whole: true, placed: true},
		{name: "days after, the day read again whole, a request short", after: 2, whole: true, short: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			on := Today(dayfile.DayStart(day.Start, tt.after))
			s, err := st.restored(ledger.Pricing, on)
			if err != nil {
				t.Fatal(err)
			}
			added, placed := s.gained(ofDay[tt.short:], tt.whole)
			if placed != tt.placed || len(added) > 0 {
				t.Errorf("restored on %s, given %d of its %d requests of %s again: %d taken in, placed %t; want none taken in, placed %t",
					on.Date, len(ofDay)-tt.short, len(ofDay), day.Date, len(added), placed, tt.placed)
			}
		})
	}
}

// A session named only by a count of tokens, which is none of its requests,
// restored from its state onto a later day, its page tallied or not, is the
// session read whole there: holding none open, as a read of no request
// holds none, where a session of requests, every one sealed, holds them
// empty.
func TestASessionOfNoRequestRestoredFromItsStateIsTheSessionReadWhole(t *testing.T) {
	day, end := digestDay()
	counted := generatedSession(2)[0]
	counted.Kind = ledger.KindCount
	if !day.Holds(counted.At) {
		t.Fatalf("the count arrived at %v, want it on %s", counted.At, day.Date)
	}
	lines := []ledger.Line{counted}
	f, err := foldedAsOf(counted.Session, ledger.SessionFrom{Lines: lines}, nil, day)
	if err != nil {
		t.Fatal(err)
	}
	from := ledger.SessionFrom{Base: baseOf(day.Date, f.State), Parts: []ledger.SessionPart{{Date: day.Date, Part: f.Part, Times: f.Times}}}
	on, horizon := Today(end), instantOf(end.Add(9*time.Hour))
	for _, paged := range []bool{false, true} {
		t.Run(fmt.Sprintf("its page tallied %t", paged), func(t *testing.T) {
			got, err := heldAsOf(counted.Session, from, nil, ledger.Pricing, on, horizon, paged)
			if err != nil {
				t.Fatal(err)
			}
			want := readOf(lines, nil, ledger.Pricing, on, horizon, paged)
			if !sameHeld(unpaged(got, true), unpaged(want, true), day.Date) || paged && !samePage(got.page, want.page) {
				t.Errorf("restored, it holds open %#v, its page\n%s\nwant, as read whole, %#v, its page\n%s", got.open, pageSaid(got.page), want.open, pageSaid(want.page))
			}
		})
	}
}

// A session's stored state is the date of its day, its horizon, its story
// and how many of its requests arrived each day, by short keys, every zero
// left out: each request its story holds as a stored request holds it, and
// each move with what settled it; never a worth, a move's cost, nor its
// requests on its day, though it's priced.
func TestASessionsStoredStateIsItsStoryAndCountsNeverAWorth(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	request := func(id string, after time.Duration, model, account, from, class string, usage int) ledger.Line {
		l := ledger.Line{
			At: at.Add(after), Request: id, Kind: ledger.KindMessage, Session: "s", Dir: "/code/app", Model: model, Account: account, Reason: "sticky",
			Status: 200, Attempts: 1, TotalMS: 3000, Class: class, Usage: json.RawMessage(foldUsages[usage]),
		}
		if from != "" {
			l.From, l.Reason, l.Tried = from, "moved: work hit its limit", []ledger.Tried{{Account: from, Why: "hit its limit"}}
		}
		return l
	}
	lines := []ledger.Line{
		request("r1", 0, foldOpus, "work", "", ledger.ClassMain, 2),
		request("r2", 5*time.Minute, foldOpus, "side", "work", ledger.ClassMain, 3),
		request("r3", 6*time.Minute, foldHaiku, "side", "work", "title", 7),
	}
	day := LedgerDay{Start: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC), Date: "2026-10-07"}
	held := readOf(lines, nil, ledger.Pricing, day, instantOf(dayfile.DayStart(day.Start, 1)), false)
	st, err := held.stored()
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(st)
	r2 := `{"r":"r2","a":{"s":1791374700},"e":{"s":1791374703},"d":"/code/app","m":"claude-opus-5-5","ac":"side","rs":"moved: work hit its limit",` +
		`"f":"work","c":"main","u":true,"t":[{"a":"work","w":"hit its limit"}],"sp":{"i":10,"5m":2000,"r":90000,"o":300,"w":2000,"b":2000,"d":true,"c":true}}`
	want := `{"d":"2026-10-07","s":{"b":true,"f":{"r":"r1","a":{"s":1791374400},"e":{"s":1791374403}},` +
		`"l":{"r":"r3","a":{"s":1791374760},"e":{"s":1791374763}},"mn":{"m":"claude-opus-5-5","a":"side","t":{"s":1791374700}},` +
		`"lt":{"m":"claude-haiku-4-5","a":"side","t":{"s":1791374760}},"d":"/code/app",` +
		`"u":{"r":"r3","a":{"s":1791374760},"e":{"s":1791374763},"d":"/code/app","m":"claude-haiku-4-5","ac":"side","rs":"moved: work hit its limit",` +
		`"f":"work","c":"title","u":true,"t":[{"a":"work","w":"hit its limit"}],"sp":{"i":100,"o":10,"d":true,"c":true}},` +
		`"um":` + r2 + `,"c":true,"mv":[{"b":{"r":"r2","a":{"s":1791374700},"e":{"s":1791374703}},"t":[{"a":"work","w":"hit its limit"}],` +
		`"m":"claude-opus-5-5","f":"work","to":"side","r":"moved: work hit its limit","s":true,"w":2000,"rw":` + r2 + `},` +
		`{"b":{"r":"r3","a":{"s":1791374760},"e":{"s":1791374763}},"t":[{"a":"work","w":"hit its limit"}],"m":"claude-haiku-4-5","f":"work","to":"side",` +
		`"r":"moved: work hit its limit"}],"e":{"conversation":{"s":1791374703},"title claude-haiku-4-5":{"s":1791374763}},` +
		`"lv":{"conversation":300000000000}},"c":{"2026-10-07":3}}`
	if err != nil || string(got) != want {
		t.Errorf("stored as\n%s, %v, want\n%s", got, err, want)
	}
}
