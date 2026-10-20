package views

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// filedLedger is a session's lines as the ledger files them, each as its
// request ends, as a read at now finds them: those of the requests that
// arrived and ended by then, but for the last short of them, which end at
// now, as a read part-way through filing requests ending together finds.
// It holds no day's rows.
type filedLedger struct {
	noRows
	lines []ledger.Line
	now   time.Time
	short int
}

// noRows reads a day's rows as a Follower of a ledger that holds no day's
// head does: none, whatever the day, so each is read raw.
type noRows struct{}

func (noRows) DayRows(string, int) (ledger.DayRows, bool) {
	return ledger.DayRows{}, false
}

func (noRows) RowChunk(date string, _ ledger.RowsChunk) ([]byte, error) {
	return nil, fmt.Errorf("a chunk of the rows of %s: %w", date, fs.ErrNotExist)
}

// filed are the lines l holds by its now, as filed.
func (l filedLedger) filed() []ledger.Line {
	filed := slices.DeleteFunc(slices.Clone(l.lines), func(line ledger.Line) bool { return line.At.After(l.now) || ended(&line).After(l.now) })
	return filed[:len(filed)-l.short]
}

// endingWith is how many of lines, as filed, after the one at i end as it
// does.
func endingWith(lines []ledger.Line, i int) int {
	n := 0
	for j := i + 1; j < len(lines) && ended(&lines[j]).Equal(ended(&lines[i])); j++ {
		n++
	}
	return n
}

// Sessions reads as a Follower of a ledger that holds no digest does: the
// sessions with the given ids, and today's, each read whole.
func (l filedLedger) Sessions(ids []string, rest func(string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	today := Today(l.now)
	wanted := slices.Clone(ids)
	for _, line := range l.filed() {
		if isRequest(&line) && today.Holds(line.At) {
			wanted = append(wanted, line.Session)
		}
	}
	return l.SessionsOf(wanted, rest, take)
}

// SessionsOf reads as a Follower of a ledger that holds no digest does: the
// sessions with the given ids, each read whole, handing take, where it's
// given, every line filed, by the date of the day it's filed on.
func (l filedLedger) SessionsOf(ids []string, _ func(string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	read := make(map[string][]ledger.Line)
	for _, line := range l.filed() {
		if take != nil {
			take(dayfile.DateOf(line.At), line)
		}
		if slices.Contains(ids, line.Session) {
			read[line.Session] = append(read[line.Session], line)
		}
	}
	return ledger.SessionsWhole(read)
}

// unseen is the id of a request the stream never told of, as one whose
// router went away as it ran: it pins none of its session's later requests.
const unseen = "unseen"

// generatedRun is a generated session as a run of feeds is given it: as
// generatedSession makes it, but for two in five, one of whose requests runs
// two hours: in one, untold by the stream, so its line lands behind its
// session's horizon; in the other, told of, so it pins the horizon before
// it till its line comes.
func generatedRun(seed uint64) []ledger.Line {
	lines := generatedSession(seed)
	if seed%5 < 2 {
		r := rand.New(rand.NewPCG(seed, 3))
		i := r.IntN(len(lines))
		lines[i].TotalMS = (2 * time.Hour).Milliseconds()
		if seed%5 == 0 {
			lines[i].Request = unseen
		}
		slices.SortStableFunc(lines, func(a, b ledger.Line) int { return ended(&a).Compare(ended(&b)) })
	}
	return lines
}

// feeding is a run of feeds of the store over a generated session, as the
// watch feeds it a read of the ledger's tail at each look: the session's
// lines, as filed; the store; the requests it has been given, by their keys,
// and those of them it read whole; when it was last fed, and how many lines
// ending then its read had yet to find; the session's horizon as the rule
// has it; and the hard moments the runs met.
type feeding struct {
	t       *testing.T
	seed    uint64
	lines   []ledger.Line
	prices  ledger.Table
	held    HeldSessions
	given   map[lineKey]ledger.Line
	read    map[lineKey]bool
	fed     time.Time
	short   int
	horizon instant
	met     map[string]bool
}

// look feeds the store a read of the ledger's tail at now, as Fed takes it,
// short of the last short lines filed ending at now: what the ledger filed
// since the last look, today's, and each earlier day
// that gained a line read again whole, as the tail reads a late day; and,
// where again says, every one of today's again, as a read afresh gives
// them. Its reads whole are of a ledger lag after now, as a read whole may
// find lines the next look's gives again. It wants the session, pinned by
// its requests in flight, as the stream tells of them. A line it gains
// behind the horizon has the session read whole; and the store is held to
// what it was given, as check has it.
func (f *feeding) look(now time.Time, lag time.Duration, again bool, short int) {
	f.t.Helper()
	today := Today(now)
	was := filedLedger{lines: f.lines, now: f.fed, short: f.short}.filed()
	fresh := func(l ledger.Line) bool {
		return !slices.ContainsFunc(was, func(w ledger.Line) bool { return w.Request == l.Request && w.At.Equal(l.At) })
	}
	var gained []ledger.Held
	days := make(map[string][]ledger.Held)
	for _, line := range (filedLedger{lines: f.lines, now: now, short: short}).filed() {
		switch date := dayfile.DateOf(line.At); {
		case date < today.Date:
			days[date] = append(days[date], ledger.Held{Line: line})
		case again || fresh(line):
			gained = append(gained, ledger.Held{Line: line})
		}
	}
	maps.DeleteFunc(days, func(_ string, lines []ledger.Held) bool {
		return !slices.ContainsFunc(lines, func(h ledger.Held) bool { return fresh(h.Line) })
	})
	f.note(now, gained, days)
	before, sealed := f.held.sessions["s"], f.sealedOf(f.held.sessions["s"])
	behind, ruled := f.behind(gained, days, func(s *heldSession) instant { return s.horizon }), f.behind(gained, days, func(*heldSession) instant { return f.horizon })
	l := filedLedger{lines: f.lines, now: now.Add(lag)}
	if lag == 0 {
		l.short = short
	}
	held, gains := f.held.Fed(l, Took(gained, days), SessionsWanted{Page: "s", Pins: f.pins(now)}, now)
	switch whole := gains["s"].whole; {
	case behind && !whole:
		f.t.Fatalf("session %d fed at %v took a line behind its horizon in, want it read whole", f.seed, now)
	case whole && before != nil && !ruled && !now.Before(f.fed) && !f.gainsLong(now, days):
		f.t.Fatalf("session %d fed at %v was read whole, want it held as it was, its lines all placed", f.seed, now)
	case whole:
		f.horizon = f.ruled(now)
	default:
		f.horizon = later(f.horizon, f.ruled(now))
	}
	if h := held.sessions["s"].horizon; h.compare(f.horizon) > 0 {
		f.t.Fatalf("session %d fed at %v sealed to %v, want no further than the rule's horizon, %v", f.seed, now, h.time(), f.horizon.time())
	}
	f.held, f.fed, f.short = held, now, short
	if gains["s"].whole {
		f.given, f.read = make(map[lineKey]ledger.Line), make(map[lineKey]bool)
		for _, line := range l.filed() {
			f.see(line, true)
		}
	}
	for _, h := range gained {
		f.see(h.Line, false)
	}
	for _, lines := range days {
		for _, h := range lines {
			f.see(h.Line, false)
		}
	}
	f.check(now, gains["s"], before, sealed)
}

// behind reports whether a look gains a line the store doesn't hold that
// arrived behind its session's horizon, as horizon gives it of the session
// held, today's or a day's read again.
func (f *feeding) behind(gained []ledger.Held, days map[string][]ledger.Held, horizon func(*heldSession) instant) bool {
	s, held := f.held.sessions["s"]
	if !held {
		return false
	}
	for _, lines := range slices.Concat([][]ledger.Held{gained}, slices.Collect(maps.Values(days))) {
		for _, h := range lines {
			_, given := f.given[keyOfLine(&h.Line)]
			if isRequest(&h.Line) && !given && instantOf(h.At).compare(horizon(s)) < 0 {
				return true
			}
		}
	}
	return false
}

// gainsLong reports whether a look at now reads again whole a day before
// yesterday, whose keys the store no longer holds, with a line on it the
// store doesn't hold: a count it can't match, so it reads the session whole.
func (f *feeding) gainsLong(now time.Time, days map[string][]ledger.Held) bool {
	since := dayfile.DateOf(dayfile.DayStart(now.Local(), -1))
	for date, lines := range days {
		for _, h := range lines {
			if _, given := f.given[keyOfLine(&h.Line)]; date < since && isRequest(&h.Line) && !given {
				return true
			}
		}
	}
	return false
}

// ruled is the session's horizon at now as the rule has it, before the
// horizon it last had: openFor before now, or a moment before the earliest
// of its pins no more than pinnedFor before now, arrivesWithin, where
// that's earlier.
func (f *feeding) ruled(now time.Time) instant {
	horizon := now.Add(-openFor)
	for _, pin := range f.pins(now)["s"] {
		if now.Sub(pin) <= pinnedFor && pin.Add(-arrivesWithin).Before(horizon) {
			horizon = pin.Add(-arrivesWithin)
		}
	}
	return instantOf(horizon)
}

// pins are the session's pins at now: when each of its requests in flight
// then arrived, but for the one the stream never told of.
func (f *feeding) pins(now time.Time) map[string][]time.Time {
	pins := make(map[string][]time.Time)
	for _, line := range f.lines {
		if line.Request != unseen && !line.At.After(now) && ended(&line).After(now) {
			pins["s"] = append(pins["s"], line.At)
		}
	}
	return pins
}

// note notes the hard moments a look at now meets, given what it gains: a
// line given again, as a read afresh gives it; one read whole before the
// look's read gives it; a day before today read again whole; the clock set
// back; a line behind the horizon; and the day turned on a move whose cost
// the new day's prices change.
func (f *feeding) note(now time.Time, gained []ledger.Held, days map[string][]ledger.Held) {
	for _, h := range gained {
		k := keyOfLine(&h.Line)
		if _, given := f.given[k]; given && !f.read[k] {
			f.met["a line given again"] = true
		}
		if f.read[k] {
			f.met["a read whole overlapping the next look's"] = true
		}
		_, given := f.given[k]
		if !given && f.laterTied(&h.Line) {
			f.met["requests ending together given in separate feeds, the later first"] = true
		}
		switch {
		case given:
		case h.Request == unseen:
			f.met["a line behind the horizon"] = true
		case now.Sub(h.At) > openFor:
			f.met["a line behind openFor, pinned"] = true
		}
	}
	if len(days) > 0 {
		f.met["a day read again whole"] = true
	}
	for date := range days {
		if s, held := f.held.sessions["s"]; held && date < dayfile.DateOf(dayfile.DayStart(now.Local(), -1)) && s.counts[date] > 0 {
			f.met["a day before yesterday read again whole"] = true
		}
	}
	if f.gainsLong(now, days) {
		f.met["a day before yesterday gaining a line"] = true
	}
	if !f.fed.IsZero() && now.Before(f.fed) {
		f.met["the clock set back"] = true
	}
	s, held := f.held.sessions["s"]
	if !held || f.fed.IsZero() || Today(now).Date <= Today(f.fed).Date {
		return
	}
	rolled := s.story.on(Today(now))
	for i, m := range s.story.moves {
		if m.cost != nil && rolled.moves[i].cost != nil && *m.cost != *rolled.moves[i].cost {
			f.met["midnight, a move's cost priced again"] = true
		}
	}
}

// laterTied reports whether the store was given a request that arrived and
// ended as the line l's did, coming after it in the one order, by its id.
func (f *feeding) laterTied(l *ledger.Line) bool {
	for _, g := range f.given {
		if g.At.Equal(l.At) && ended(&g).Equal(ended(l)) && g.Request > l.Request {
			return true
		}
	}
	return false
}

// keyOfLine is the line l's key, as its flight's key has it.
func keyOfLine(l *ledger.Line) lineKey {
	k, _ := flightOf(l).key()
	return k
}

// see notes the line l as given the store, read whole where read says, a
// request of the session's alone.
func (f *feeding) see(l ledger.Line, read bool) {
	if !isRequest(&l) {
		return
	}
	k := keyOfLine(&l)
	if _, given := f.given[k]; !given {
		f.given[k], f.read[k] = l, read
	}
}

// sealedOf is how many of the requests the store was given its session s
// holds sealed: none where it holds none.
func (f *feeding) sealedOf(s *heldSession) int {
	if s == nil {
		return 0
	}
	return len(f.given) - len(s.open)
}

// check holds the store, fed at now, to the story of every request it was
// given, read at once, in the one order, at the day now falls on: its
// sealed story that of those behind its horizon, those at or past it held
// open. And it holds what the feed gained to what it changed of the session
// as it stood before, of whose requests sealed were sealed: the requests it
// sealed, and all that it tells otherwise of the requests it held, as
// toldOfLine tells it.
func (f *feeding) check(now time.Time, gain SessionGain, before *heldSession, sealed int) {
	f.t.Helper()
	day := Today(now)
	all := pricedAt(heldRequestsOf(slices.Collect(maps.Values(f.given))), f.prices, day.Date)
	want := storyOf(all, f.prices, day)
	s := f.held.sessions["s"]
	if !sameStory(s.story, want) {
		q := queryOf(f.lines)
		f.t.Fatalf("session %d fed at %v tells\n%+v\nwant, as read at once\n%+v", f.seed, now, toldOfHeld(s.story, q), toldOfHeld(want, q))
	}
	i, _ := slices.BinarySearchFunc(all, s.horizon, func(r heldRequest, h instant) int { return r.at.compare(h) })
	if !sameStory(s.sealed, storyOf(all[:i], f.prices, day)) || !slices.Equal(flightsOf(s.open), flightsOf(all[i:])) {
		f.t.Fatalf("session %d fed at %v seals %d of its %d requests, holds %d open; want those behind its horizon sealed, the rest open",
			f.seed, now, len(all)-len(s.open), len(all), len(s.open))
	}
	if gain.whole || before == nil {
		return
	}
	if got, want := flightsOf(gain.sealed), flightsOf(all[sealed:i]); !slices.Equal(got, want) {
		f.t.Fatalf("session %d fed at %v gained %d sealed, want the %d sealed by the feed", f.seed, now, len(got), len(want))
	}
	retold := make(map[flight]bool)
	for _, fl := range gain.retold {
		retold[fl] = true
	}
	for _, line := range f.lines {
		r := heldRequestOf(&line)
		if _, held := f.given[keyOfLine(&line)]; !held || retold[r.flight] {
			continue
		}
		if was, is := toldOfLine(before.story, &line), toldOfLine(s.story, &line); was != is {
			f.t.Fatalf("session %d fed at %v tells of %s\n%s\nwhere it told\n%s\nbut its gain names it not retold", f.seed, now, line.Request, is, was)
		}
	}
}

// flightsOf are the flights of requests.
func flightsOf(requests []heldRequest) []flight {
	flights := make([]flight, len(requests))
	for i, r := range requests {
		flights[i] = r.flight
	}
	return flights
}

// toldOfLine is what the story s tells of the line l, as a row of it reads
// it: whether it's the session's first request, the move it made, all of
// it, and whether it wrote the session's context again on the account a
// move took it to.
func toldOfLine(s *story, l *ledger.Line) string {
	first, made, rewrote := s.toldOf(l.Request, l.At)
	if made == nil {
		return fmt.Sprintf("first %t, no move, rewrote %t", first, rewrote)
	}
	cost := "none"
	if made.cost != nil {
		cost = fmt.Sprint(*made.cost)
	}
	by := ""
	if made.rewrote != nil {
		by = flightSaid(made.rewrote.flight)
	}
	return fmt.Sprintf("first %t, moved from %s to %s for %q, settled %t, writing %d, costing %s, rewritten by %q, rewrote %t",
		first, made.from, made.to, made.reason, made.settled, made.written, cost, by, rewrote)
}

// The store, fed a read of the ledger's tail at each look, tells after each
// the story of every request it was given, read at once, in the one order,
// at the day the look falls on, sealing those behind its horizon, and says
// what changed: of generated sessions whose requests end out of the order
// they came in, come to the millisecond together, move with their caches
// warm and cold, of five minutes and of an hour, as subagents, side
// requests, compactions and lines without a class, across midnight, a
// model's prices changing on the new day; a line landing behind the
// horizon, read whole; lines given again, a day read again whole, a read
// whole overlapping the next look's, a look part-way through filing
// requests ending together, and the clock set back.
func TestTheStoreFedLookByLookTellsWhatOneReadAtOnceTells(t *testing.T) {
	prices := changingPrices()
	met := make(map[string]bool)
	for seed := range uint64(150) {
		f := &feeding{
			t: t, seed: seed, lines: generatedRun(seed), prices: prices, held: HoldSessions(prices),
			given: make(map[lineKey]ledger.Line), read: make(map[lineKey]bool), met: met,
		}
		r := rand.New(rand.NewPCG(seed, 4))
		at := f.lines[0].At.Add(-time.Minute)
		f.look(at, 0, false, 0)
		looked := len(f.lines)
		if seed%4 == 3 {
			looked /= 2
		}
		for i := 0; i < looked; i += 1 + r.IntN(4) {
			if short, tied := endingWith(f.lines, i), ended(&f.lines[i]); short > 0 && !tied.Before(at) {
				f.look(tied, 0, false, short)
				at = tied
			}
			now := later(instantOf(at), instantOf(ended(&f.lines[i]))).time().Add(time.Duration(r.IntN(5000)) * time.Millisecond)
			if r.IntN(25) == 0 {
				now = at.Add(-time.Duration(1+r.IntN(20)) * time.Minute)
			}
			f.look(now, time.Duration(r.IntN(3))*10*time.Second, r.IntN(10) == 0, 0)
			at = now
		}
		last := ended(&f.lines[len(f.lines)-1])
		if looked < len(f.lines) {
			f.look(last.AddDate(0, 0, 2), 0, false, 0)
			continue
		}
		f.look(last.Add(3*time.Hour), 0, false, 0)
	}
	for _, moment := range []string{
		"a line given again", "a read whole overlapping the next look's", "a day read again whole", "a day before yesterday read again whole",
		"a day before yesterday gaining a line",
		"the clock set back",
		"a line behind the horizon", "a line behind openFor, pinned", "midnight, a move's cost priced again",
		"requests ending together given in separate feeds, the later first",
	} {
		if !met[moment] {
			t.Errorf("no generated run meets %s", moment)
		}
	}
}

// A store whose day was set in another zone than the clock's now, as the
// zone changed since it was last fed, holds none of what it held: its next
// feed reads its sessions whole, as when the clock was set back. Fed again
// in the zone it was fed in, it reads none.
func TestTheStoreFedInAnotherZoneIsReadWhole(t *testing.T) {
	lines := generatedSession(1)
	at := ended(&lines[len(lines)-1]).Add(time.Minute)
	l := filedLedger{lines: lines, now: at}
	want := SessionsWanted{Page: "s"}
	held, _ := HoldSessions(ledger.Pricing).Fed(l, Taken{}, want, at)
	_, offset := at.Zone()
	tests := []struct {
		name string
		zone *time.Location
		read bool
	}{
		{name: "fed in the clock's zone", zone: time.Local},
		{name: "fed in another", zone: time.FixedZone("elsewhere", offset+3*60*60), read: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := held
			h.fed.day.Start = dayfile.DayStart(h.fed.at.In(tt.zone), 0)
			fed, _ := h.Fed(l, Taken{}, want, at.Add(time.Minute))
			if got := fed.Holding().Read > 0; got != tt.read {
				t.Errorf("the next feed read %d requests whole, want them read whole %t", fed.Holding().Read, tt.read)
			}
		})
	}
}

// A line of a request the store holds, of the same id and arriving at the
// same moment, that says otherwise of it, as a hand-edited ledger may hold,
// has its session read whole, so the store keeps the line a read whole
// keeps, the first it reads, whichever it was fed first: one filed before
// the line it holds, or after it. The same line given again is passed over,
// nothing read. Of a request it holds sealed, and one it holds open.
func TestALineSayingOtherwiseOfARequestHeldHasItsSessionReadWhole(t *testing.T) {
	lines := generatedSession(7)
	now := ended(&lines[len(lines)-1]).Add(time.Minute)
	changes := map[string]func(*ledger.Line){
		"the same":             func(*ledger.Line) {},
		"ending later":         func(l *ledger.Line) { l.TotalMS += 1000 },
		"spending otherwise":   func(l *ledger.Line) { l.Usage = json.RawMessage(`{"input_tokens":7,"output_tokens":3}`) },
		"on another account":   func(l *ledger.Line) { l.Account = "spare" },
		"tried on another":     func(l *ledger.Line) { l.Tried = []ledger.Tried{{Account: "spare", Why: "limited"}} },
		"of another class":     func(l *ledger.Line) { l.Class = "elsewhere" },
		"in another directory": func(l *ledger.Line) { l.Dir = "/code/elsewhere" },
		"for another reason":   func(l *ledger.Line) { l.Reason = "elsewhere" },
		"moved from another":   func(l *ledger.Line) { l.From = "spare" },
		"of another agent":     func(l *ledger.Line) { l.AgentID = "a9" },
		"served elsewhere":     func(l *ledger.Line) { l.Shape.InferenceGeo = "eu" },
	}
	for _, held := range []struct {
		name string
		i    int
	}{{name: "sealed", i: 0}, {name: "open", i: len(lines) - 1}} {
		for name, change := range changes {
			for _, before := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s, %s, filed before it %t", held.name, name, before), func(t *testing.T) {
					other := lines[held.i]
					change(&other)
					at := held.i + 1
					if before {
						at = held.i
					}
					l := filedLedger{lines: slices.Insert(slices.Clone(lines), at, other), now: now}
					want := SessionsWanted{Page: "s"}
					store, _ := HoldSessions(ledger.Pricing).Fed(filedLedger{lines: lines, now: now}, Taken{}, want, now)
					fed, _ := store.Fed(l, Took([]ledger.Held{{Line: other}}, nil), want, now)
					whole, _ := HoldSessions(ledger.Pricing).Fed(l, Taken{}, want, now)
					if !sameStory(fed.sessions["s"].story, whole.sessions["s"].story) {
						t.Errorf("fed the line, the store tells\n%+v\nwant it as read whole\n%+v", fed.sessions["s"].story, whole.sessions["s"].story)
					}
					if read, same := fed.Holding().Read > 0, name == "the same"; read == same {
						t.Errorf("fed the line, the store read %d requests whole, want them read whole %t", fed.Holding().Read, !same)
					}
				})
			}
		}
	}
}

// A store given another's sessions keeps its own prices whatever the
// other's are; takes those of its sessions it doesn't hold; is as last fed,
// for the days a read of the ledger's tail reads again for it, by one rule,
// whether it holds a session or not: as the other, where it was never fed;
// else, of the two that hold a session, the earlier fed; of two that hold
// none, the later; and as last fed, for a clock set back, at the later fed
// of the two, whichever. It counts the more feeds of the two.
func TestTheStoreGivenAnothersSessionsKeepsItsPricesAndIsAsLastFedByOneRule(t *testing.T) {
	early, late := time.Date(2026, 10, 6, 23, 0, 0, 0, time.Local), time.Date(2026, 10, 7, 9, 0, 0, 0, time.Local)
	own, others := ledger.Pricing, changingPrices()
	holding := func(prices ledger.Table, at time.Time, feeds int, ids ...string) HeldSessions {
		h := HeldSessions{prices: prices, fed: fedOn(at), unread: fedOn(at), feeds: feeds, sessions: make(map[string]*heldSession)}
		for _, id := range ids {
			h.sessions[id] = readOf(nil, nil, prices, Today(at), instantOf(at), false)
		}
		return h
	}
	tests := []struct {
		name     string
		h, found HeldSessions
		unread   time.Time
		held     []string
		feeds    int
	}{
		{name: "never fed, given a read", h: HoldSessions(own), found: holding(others, late, 1, "a"), unread: late, held: []string{"a"}, feeds: 1},
		{name: "fed, holding none, given nothing", h: holding(own, late, 3), found: HeldSessions{}, unread: late, feeds: 3},
		{name: "fed, holding none, given a later read holding none", h: holding(own, early, 3), found: holding(others, late, 1), unread: late, feeds: 3},
		{name: "fed, holding none, given an earlier read holding none", h: holding(own, late, 3), found: holding(others, early, 1), unread: late, feeds: 3},
		{name: "fed long ago, holding none, given a later read", h: holding(own, early, 3), found: holding(others, late, 1, "a"), unread: late, held: []string{"a"}, feeds: 3},
		{name: "fed, holding, given a later read holding none", h: holding(own, late, 3, "a"), found: holding(others, early, 1), unread: late, held: []string{"a"}, feeds: 3},
		{name: "fed, holding, given an earlier read", h: holding(own, late, 3, "a"), found: holding(others, early, 1, "a", "b"), unread: early, held: []string{"a", "b"}, feeds: 3},
		{name: "fed, holding, given a later read", h: holding(own, early, 3, "a"), found: holding(others, late, 4, "b"), unread: early, held: []string{"a", "b"}, feeds: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.h.With(tt.found)
			if !reflect.DeepEqual(got.prices, own) {
				t.Errorf("the store is priced as %s, want its own prices, as of %s", got.prices.AsOf, own.AsOf)
			}
			if got.unread != fedOn(tt.unread) {
				t.Errorf("the store reads again as last fed at %v, on %s; want %v, on %s", got.unread.at, got.unread.day.Date, tt.unread, Today(tt.unread).Date)
			}
			if got.fed != fedOn(late) {
				t.Errorf("the store is as last fed at %v, on %s; want the later of the two, %v, on %s", got.fed.at, got.fed.day.Date, late, Today(late).Date)
			}
			if ids := slices.Sorted(maps.Keys(got.sessions)); !slices.Equal(ids, tt.held) {
				t.Errorf("the store holds %v, want %v", ids, tt.held)
			}
			if tt.h.sessions["a"] != nil && got.sessions["a"] != tt.h.sessions["a"] {
				t.Error("the store took in place of a session it held the other's, want its own kept")
			}
			if got.feeds != tt.feeds {
				t.Errorf("the store counts %d feeds, want %d", got.feeds, tt.feeds)
			}
		})
	}
}
