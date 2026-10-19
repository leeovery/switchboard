package watch

import (
	"encoding/json"
	"encoding/json/jsontext"
	"iter"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// The soak's guard: a soak of the dashboard over a world far quieter than
// the owner's, run across midnight, holding what each reader reads and
// holds to what it shows, exactly, at every read of the ledger. The heap
// isn't held to a number: the full soak, behind -soak, reports it.

// guardPace is the guarded world's: 600 requests a day, each answer's
// progress told every five seconds, each session listed for three minutes
// after it was last seen, so most of the day before's are let go of as the
// day turns. Across the guard's run, one request is in flight as it starts,
// one goes out at midnight, arriving moments before it, its line filed on
// the day before after the day turned, and the long session's first request
// of the new day moves it.
var guardPace = soakPace{rate: 600, step: 5 * time.Second, listed: 3 * time.Minute}

// guardDays are the days the guarded world has run as the guard starts, at
// guardStart, two minutes before midnight; guardEnd is when it ends, once
// the day before is no longer read again.
const guardDays = 2

var (
	guardStart = soakAt(23, 58)
	guardEnd   = guardStart.Add(8*time.Minute + 30*time.Second)
)

// The rule the store keeps its sessions' requests open by, as the guard
// holds it to it from outside: for openFor after each arrived, or from
// arrivesWithin before the earliest the stream tells of went upstream,
// where that's within pinnedFor. lateFor is how long into a day the
// readers read the day before again, for the lines of the requests that
// ended across midnight.
const (
	guardOpenFor   = time.Hour
	guardArrives   = time.Minute
	guardPinnedFor = 6 * time.Hour
	guardLateFor   = 5 * time.Minute
)

// soakGuard holds a soak to what it reads and holds: the ledger it reads
// through, made of the world's as the soak starts; and what it's told of
// each message the model takes in, with the model as it was before.
type soakGuard interface {
	reads(s *soak, l views.Ledger) views.Ledger
	took(msg tea.Msg, was Model)
}

// countingLedger is a ledger that tells of each read of its lines as it's
// made, for the guard to hold the readers to what they read. It hands each
// read of sessions a take of its own, so the ledger decodes every line of
// the days it reads whole, with lineIn, never as production's reads without
// one do, keptLineIn's way, the sessions' own alone: a fault of that decoding
// alone passes the guard.
type countingLedger struct {
	views.Ledger
	read func(ledgerRead)
}

// ledgerRead is a read of a ledger's lines: its method, and the ids it asked
// for, or the date of the day; and, of a read of sessions, the sessions of
// the lines it read of each day, by its date, and the date of the digest
// whose state each session it gave rests on, by its id, "" of one resting
// on none.
type ledgerRead struct {
	method string
	asked  []string
	days   map[string]map[string]bool
	rests  map[string]string
}

// sessionsRead is a read of sessions by the method named, of the ids asked
// for, as it begins: none of its days read yet.
func sessionsRead(method string, asked []string) ledgerRead {
	return ledgerRead{method: method, asked: asked, days: make(map[string]map[string]bool), rests: make(map[string]string)}
}

// taking is take, where it's given, each line handed it noted among the
// read's days first.
func (r ledgerRead) taking(take func(string, ledger.Line)) func(string, ledger.Line) {
	return func(date string, l ledger.Line) {
		if r.days[date] == nil {
			r.days[date] = make(map[string]bool)
		}
		r.days[date][l.Session] = true
		if take != nil {
			take(date, l)
		}
	}
}

// gave is the read once it gave sessions, each noted with what it rests on.
func (r ledgerRead) gave(sessions map[string]ledger.SessionFrom) ledgerRead {
	for id, from := range sessions {
		r.rests[id] = ""
		if from.Base != nil {
			r.rests[id] = from.Base.Date
		}
	}
	return r
}

// behindBases are the dates of the days the read read whose lines no session
// it gave needed, oldest first: each it gave with a line there rests on the
// state of that day, or of a later one, which holds the line already. A day
// with no line of a session it gave, as a near day, read for every
// session's lines, can be, is none of them.
func (r ledgerRead) behindBases() []string {
	var behind []string
	for date, sessions := range r.days {
		given, needed := false, false
		for id := range sessions {
			base, ok := r.rests[id]
			given = given || ok
			needed = needed || ok && base < date
		}
		if given && !needed {
			behind = append(behind, date)
		}
	}
	slices.Sort(behind)
	return behind
}

func (c *countingLedger) Today(mark ledger.Mark) ([]ledger.Held, ledger.Mark, bool) {
	c.read(ledgerRead{method: "Today"})
	return c.Ledger.Today(mark)
}

func (c *countingLedger) Day(date string) []ledger.Held {
	c.read(ledgerRead{method: "Day", asked: []string{date}})
	return c.Ledger.Day(date)
}

func (c *countingLedger) Session(id string) iter.Seq[ledger.Held] {
	c.read(ledgerRead{method: "Session", asked: []string{id}})
	return c.Ledger.Session(id)
}

// A read of sessions read a day behind the states the sessions it gave rest
// on only where each it gave with a line there rests on the state of that
// day or a later one: not where one rests on none, or on an earlier day's,
// nor where it gave none with a line there.
func TestAReadOfSessionsIsBehindItsBasesWhereNoSessionItGaveNeedsTheDay(t *testing.T) {
	const day = "2026-10-08"
	tests := []struct {
		name   string
		lines  []string
		rests  map[string]string
		behind bool
	}{
		{name: "a session resting on none", lines: []string{"a"}, rests: map[string]string{"a": ""}},
		{name: "a session resting on an earlier day's state", lines: []string{"a"}, rests: map[string]string{"a": "2026-10-07"}},
		{name: "a session resting on the day's own state", lines: []string{"a"}, rests: map[string]string{"a": day}, behind: true},
		{name: "a session resting on a later day's state", lines: []string{"a"}, rests: map[string]string{"a": "2026-10-09"}, behind: true},
		{name: "one resting on a later day's, another on none", lines: []string{"a", "b"}, rests: map[string]string{"a": "2026-10-09", "b": ""}},
		{name: "one resting on a later day's, another not given", lines: []string{"a", "c"}, rests: map[string]string{"a": "2026-10-09"}, behind: true},
		{name: "none given, as a near day's other sessions", lines: []string{"c"}, rests: map[string]string{"a": "2026-10-09"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := sessionsRead("Sessions", nil)
			take := r.taking(nil)
			for _, id := range tt.lines {
				take(day, ledger.Line{Session: id})
			}
			maps.Copy(r.rests, tt.rests)
			if got := len(r.behindBases()) > 0; got != tt.behind {
				t.Errorf("reads %s behind its bases %t, want %t", day, got, tt.behind)
			}
		})
	}
}

func (c *countingLedger) Sessions(ids []string, rest func(string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	r := sessionsRead("Sessions", ids)
	sessions := c.Ledger.Sessions(ids, rest, r.taking(take))
	c.read(r.gave(sessions))
	return sessions
}

func (c *countingLedger) SessionsOf(ids []string, rest func(string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	r := sessionsRead("SessionsOf", ids)
	sessions := c.Ledger.SessionsOf(ids, rest, r.taking(take))
	c.read(r.gave(sessions))
	return sessions
}

// The store's part: Sessions' List open across midnight, the store fed at
// every read of the ledger's tail, each read is held to what it gained.
// Today hands the tail each line filed today once, every one by the time
// it's filed; a session's lines are read whole only of one the store
// doesn't hold, every one of today's once, by the List's first read; a day
// before today is read again only as the day turns, for the lines of the
// requests that ended across midnight; a read of sessions reads no day
// behind the digests' states the sessions it gives rest on, and gives some
// resting on one; and a feed folds only what it gained and the requests the
// store held open, none open longer than the store's rule keeps it. As the
// run starts, once the day before is no longer read again, and at the end,
// the store holds today's sessions and those running alone, having let go
// by the end of those that stopped running before midnight; and as the day
// turns, just after, and at the end, the List of the store as the reads fed
// it is the List of one read whole.
func TestTheStoreHoldsOnlyWhatTheListShows(t *testing.T) {
	midnight := dayfile.DayStart(guardStart, 1)
	last := guardEnd.Add(-2 * lookEvery)
	g := &storeGuard{t: t, ids: make(map[int]string), held: []time.Time{guardStart, midnight.Add(guardLateFor), last},
		checks: []time.Time{midnight, midnight.Add(90 * time.Second), last}}
	s := newSoak(t, soakSetup{start: guardStart, days: guardDays, pace: guardPace, guard: g})
	s.show("sessions")
	if !s.h.model.sessionsTab.listed {
		t.Fatal("Sessions shows no List held, its first read landed")
	}
	s.runUntil(guardEnd)
	g.judged(s.h.model)
	crossed := func(r soakRequest) bool { return r.arrived().Before(midnight) && r.done.After(midnight) }
	if !slices.ContainsFunc(s.w.filedFrom(guardStart), crossed) {
		t.Fatal("the world filed no request that arrived before midnight and ended after it, want one, its line read late")
	}
	if len(g.checks) > 0 || len(g.held) > 0 {
		t.Errorf("no read of the tail landed after %v to compare the store at, nor after %v to see what it holds", g.checks, g.held)
	}
	if g.listed != 1 || g.fed == 0 || g.late == 0 {
		t.Errorf("today's sessions were read whole %d times, the store fed %d and the day before read again %d; "+
			"want them read once, by the List's first read, the store fed at each read of the tail, and the day before read again as the day turned",
			g.listed, g.fed, g.late)
	}
	if day := s.h.model.held.Holding().Day; day != dayfile.DateOf(midnight) {
		t.Errorf("the store was last fed on %s, want the day after midnight, %s", day, dayfile.DateOf(midnight))
	}
	if !g.letGo {
		t.Error("the store let go of no session by the end, want those that stopped running before midnight let go")
	}
	if g.rested == 0 {
		t.Error("no read of sessions gave one resting on a digest's state, want the long session on its state at the end of the day before")
	}
}

// storeGuard holds the store to what the List shows, read by read: s is the
// soak; unjudged are the reads of the ledger's lines since a read of the
// List or the tail last landed; today the ids of today's lines the tail was
// handed, as each request's id knows its line; want what the read under
// way wants of the store; checks the moments still to compare the store
// with one read whole at, and held those still to see what it holds at,
// each the earliest first, holding the sessions it held then; listed how
// many times every one of today's sessions was read whole, fed how many
// reads of the tail fed the store, late how many times the day before was
// read again, rested how many sessions its reads of sessions gave resting on
// a digest's state, and letGo whether the store let go of a session it held
// at the moment before; and ids the world's sessions' ids, by their numbers.
type storeGuard struct {
	t        *testing.T
	s        *soak
	unjudged []ledgerRead
	today    map[string]bool
	want     views.SessionsWanted
	checks   []time.Time
	held     []time.Time
	holding  []string
	listed   int
	fed      int
	late     int
	rested   int
	letGo    bool
	ids      map[int]string
}

func (g *storeGuard) reads(s *soak, l views.Ledger) views.Ledger {
	g.s = s
	return &countingLedger{Ledger: l, read: g.read}
}

// read holds a read of the ledger's lines, as it's made, to judge once the
// read it's made for lands; and what the read under way wants of the store,
// for a store read whole to want the same: as the List wants it, of its
// first read, and as the tail's fold has it, of a read of the tail.
func (g *storeGuard) read(r ledgerRead) {
	m := g.s.h.model
	now := m.now()
	switch r.method {
	case "Sessions":
		g.want = views.ListWanted(m.sessions, m.traffic.stream(now))
	case "Today":
		if f := m.heldFold(now); f != nil {
			g.want = f.want
		}
	}
	g.unjudged = append(g.unjudged, r)
}

func (g *storeGuard) took(msg tea.Msg, was Model) {
	switch msg := msg.(type) {
	case listReadMsg:
		g.judged(was)
	case tailedMsg:
		g.tailed(msg, was)
	}
}

// tailed judges a read of the tail that landed, the model as it was before:
// what it read, as judged and todays have it; and, where it fed the store,
// what the feed folded and the store holds open, as fedFrom has them, then
// what it holds and its List, at the moments due.
func (g *storeGuard) tailed(msg tailedMsg, was Model) {
	g.t.Helper()
	g.judged(was)
	g.todays(msg, was.tail)
	if msg.held == nil {
		return
	}
	g.fed++
	now := g.s.h.model.now()
	g.fedFrom(was.held.Holding(), now)
	if len(g.held) > 0 && !now.Before(g.held[0]) {
		g.holds(now)
		g.held = g.held[1:]
	}
	if len(g.checks) > 0 && !now.Before(g.checks[0]) {
		g.sameAsOneRead(now)
		g.checks = g.checks[1:]
	}
}

// judged judges the reads of the ledger's lines since a read last landed,
// as the one that landed now made them, the model as it was before: a day
// before today read only as the day turns; no session read whole that the
// store holds; and every one of today's sessions read whole, by the List's
// first read alone. None are left to judge.
func (g *storeGuard) judged(was Model) {
	g.t.Helper()
	now := g.s.h.model.now()
	held := was.held.Holding().Open
	for _, r := range g.unjudged {
		switch r.method {
		case "Day":
			g.late++
			if r.asked[0] != dayfile.DateOf(dayfile.DayStart(now, -1)) || now.Sub(dayfile.DayStart(now, 0)) >= guardLateFor {
				g.t.Errorf("at %s the day %s was read again, want only the day before today, in the first %s of the day", now.Format(time.DateTime), r.asked[0],
					guardLateFor)
			}
		case "Sessions":
			g.listed++
			fallthrough
		case "SessionsOf", "Session":
			for _, id := range r.asked {
				if _, ok := held[id]; ok {
					g.t.Errorf("at %s %s read %s whole, which the store holds", now.Format(time.DateTime), r.method, id)
				}
			}
			g.restedOn(r, now)
		}
	}
	g.unjudged = nil
}

// restedOn judges a read of sessions, r, at now, as readNoneBehind does; and
// counts the sessions it gave resting on a state.
func (g *storeGuard) restedOn(r ledgerRead, now time.Time) {
	g.t.Helper()
	readNoneBehind(g.t, r, now)
	for _, base := range r.rests {
		if base != "" {
			g.rested++
		}
	}
}

// readNoneBehind fails the test where the read of sessions r, at now, read a
// day behind the states the sessions it gave there rest on, as behindBases
// has them.
func readNoneBehind(t *testing.T, r ledgerRead, now time.Time) {
	t.Helper()
	if behind := r.behindBases(); len(behind) > 0 {
		t.Errorf("at %s %s read the lines of %v, each a day the state every session of it given rests on holds, %v", now.Format(time.DateTime), r.method, behind, r.rests)
	}
}

// todays judges the lines today's gained, as a read of the tail read them,
// the tail as it was before: each filed today, and handed the tail once, the
// tail having read as many of today's as the world has filed. A tail of
// another day, or none, has read none of today's.
func (g *storeGuard) todays(msg tailedMsg, was ledgerTail) {
	g.t.Helper()
	if g.today == nil || was.day.Date != msg.day.Date {
		g.today = make(map[string]bool)
	}
	for _, h := range msg.lines.gained {
		if g.today[h.Request] || dayfile.DateOf(h.At) != msg.day.Date {
			g.t.Errorf("on %s the tail was handed %s, filed %s, again or of another day", msg.day.Date, h.Request, h.At.Format(time.DateTime))
		}
		g.today[h.Request] = true
	}
	if filed := g.s.w.filedOn[msg.day.Date]; len(g.today) != filed {
		g.t.Errorf("on %s the tail has read %d of today's lines, want the %d the world has filed", msg.day.Date, len(g.today), filed)
	}
}

// fedFrom judges what the store's last feed did at now, the store as it was
// held before it: it folded none but the requests it gained and those it
// held open; it holds open no more of a session's requests than the world
// filed at or past the horizon the rule gives it; and it holds the keys of
// no more requests than the world filed of its sessions since yesterday
// began.
func (g *storeGuard) fedFrom(was views.Holding, now time.Time) {
	g.t.Helper()
	is := g.s.h.model.held.Holding()
	open := 0
	for _, n := range was.Open {
		open += n
	}
	if is.Folded > is.Gained+open {
		g.t.Errorf("at %s the store folded %d requests, more than the %d it gained and the %d it held open", now.Format(time.DateTime), is.Folded, is.Gained, open)
	}
	yesterday := dayfile.DayStart(now, -1)
	recent := 0
	for _, r := range g.s.w.filedFrom(yesterday) {
		if _, held := is.Open[g.idOf(r.g)]; held {
			recent++
		}
	}
	if is.Keys > recent {
		g.t.Errorf("at %s the store holds the keys of %d requests, more than the %d the world filed of its sessions since %s", now.Format(time.DateTime), is.Keys, recent,
			yesterday.Format(time.DateTime))
	}
	horizons := make(map[string]time.Time, len(is.Open))
	earliest := now
	for id := range is.Open {
		horizons[id] = g.horizonOf(id, now)
		if horizons[id].Before(earliest) {
			earliest = horizons[id]
		}
	}
	past := make(map[string]int)
	for _, r := range g.s.w.filedFrom(earliest) {
		if id := g.idOf(r.g); !r.arrived().Before(horizons[id]) {
			past[id]++
		}
	}
	for id, n := range is.Open {
		if n > past[id] {
			g.t.Errorf("at %s the store holds %d of %s's requests open, more than the %d the world filed at or past %s", now.Format(time.DateTime), n, id, past[id],
				horizons[id].Format(time.DateTime))
		}
	}
}

// horizonOf is the horizon the rule gives the session with the given id at
// now, its pins as the feed was given them: the earliest within
// guardPinnedFor holds it back.
func (g *storeGuard) horizonOf(id string, now time.Time) time.Time {
	horizon := now.Add(-guardOpenFor)
	for _, pin := range g.want.Pins[id] {
		if now.Sub(pin) <= guardPinnedFor && pin.Add(-guardArrives).Before(horizon) {
			horizon = pin.Add(-guardArrives)
		}
	}
	return horizon
}

// idOf is the id of the world's gth session.
func (g *storeGuard) idOf(n int) string {
	id, ok := g.ids[n]
	if !ok {
		id = busySession(n)
		g.ids[n] = id
	}
	return id
}

// holds judges what the store holds at now: today's sessions, those with a
// line filed today, and those running alone, none of what it decodes once,
// as heldRaw has it; and notes whether it let go of one it held when it was
// last seen.
func (g *storeGuard) holds(now time.Time) {
	g.t.Helper()
	heldRaw(g.t, g.s.h.model, now)
	want := make(map[string]bool)
	for _, r := range g.s.w.filedFrom(dayfile.DayStart(now, 0)) {
		want[g.idOf(r.g)] = true
	}
	for _, id := range g.want.Running {
		want[id] = true
	}
	got := slices.Sorted(maps.Keys(g.s.h.model.held.Holding().Open))
	if wanted := slices.Sorted(maps.Keys(want)); !slices.Equal(got, wanted) {
		g.t.Errorf("at %s the store holds %v, want today's sessions and those running, %v", now.Format(time.DateTime), got, wanted)
	}
	g.letGo = g.letGo || slices.ContainsFunc(g.holding, func(id string) bool { return !slices.Contains(got, id) })
	g.holding = got
}

// rawHeld are what v holds, by any path, that the readers keep decoded once
// in its place, each by its type: a line's raw JSON, its usage among it; a
// map of a line's limits, but a page's tally's map of its agents' prompts,
// of the same type; and a line itself, but for the open turn's, as its page
// holds them, read as it opened.
func rawHeld(v any) []string {
	var held []string
	prompts := make(map[uintptr]bool)
	walked(v, func(v reflect.Value) bool {
		switch t := v.Type(); {
		case t.PkgPath() == viewsPkg && t.Name() == "pageTally":
			if agents := v.FieldByName("agents").Pointer(); agents != 0 {
				prompts[agents] = true
			}
		case t == limitsType && v.Pointer() != 0 && prompts[v.Pointer()]:
		case slices.Contains(rawTypes, t):
			held = append(held, t.String())
			return false
		case t.PkgPath() == viewsPkg && t.Name() == "turnLines":
			return false
		}
		return true
	})
	return held
}

// limitsType is a line's limits' type, and rawTypes the types of what
// rawHeld finds.
var (
	limitsType = reflect.TypeOf(ledger.Reply{}.Limits)
	rawTypes   = []reflect.Type{reflect.TypeFor[json.RawMessage](), reflect.TypeFor[jsontext.Value](), limitsType, reflect.TypeFor[ledger.Line]()}
)

// heldRaw judges what the readers of m hold at now, as rawHeld finds it:
// none in the store, the Log or the page open.
func heldRaw(t *testing.T, m Model, now time.Time) {
	t.Helper()
	held := map[string]any{"the store": m.held, "the Log": m.log}
	if m.session != nil {
		held["the page"] = m.session.held
	}
	for name, v := range held {
		if raw := rawHeld(v); len(raw) > 0 {
			t.Errorf("at %s %s holds %d of %v, want what it holds decoded once in their place", now.Format(time.DateTime), name, len(raw), slices.Compact(slices.Sorted(slices.Values(raw))))
		}
	}
}

// sameAsOneRead judges the List of the store at now the List of a store
// read whole at now, wanting what the store's last feed wanted.
func (g *storeGuard) sameAsOneRead(now time.Time) {
	g.t.Helper()
	m := g.s.h.model
	whole, _ := views.HoldSessions(m.cfg.Prices).Fed(g.s.follower, views.Taken{}, g.want, now)
	live := m.listLive(now, m.traffic.stream(now))
	if got, want := m.held.List(live), whole.List(live); !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		g.t.Errorf("at %s the List of the store fed read by read is\n%s\nwant the List of one read whole\n%s", now.Format(time.DateTime), gotJSON, wantJSON)
	}
}
