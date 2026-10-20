package watch

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/views"
)

// findGuardDays are the days the find's guarded world has run as it starts,
// in the evening: every one before today's head built, and its side
// standing.
const findGuardDays = 4

// The find's part: the Log's find opened in the evening, walked back through
// every past day the world ran, reads each day's side where it stands and
// no day's lines whose side does: here every past day's, so it reads none
// of their lines.
func TestTheFindReadsNoDayRawWhoseSideStands(t *testing.T) {
	start := soakAt(21, 0)
	g := readingHeads("DaySide")
	s := newSoak(t, soakSetup{start: start, days: findGuardDays, pace: guardPace, guard: g})
	s.show("log")
	s.typed("/")
	s.runUntil(start.Add(time.Minute))
	if found := s.h.model.log.page.Finding.Found; !found.Done() {
		t.Fatalf("the find still has %s to read a minute after it opened, want it done", found.Next)
	}
	read := slices.Sorted(maps.Keys(g.stood))
	for back := findGuardDays - 1; back > 0; back-- {
		if past := dayfile.DateOf(dayfile.DayStart(start, -back)); !g.stood[past] {
			t.Errorf("the find read the sides of %v, want every past day's among them, %s's", read, past)
		}
	}
	if len(g.raw) > 0 {
		t.Errorf("the find read the lines of %v, want none, every past day's side standing", g.raw)
	}
}

// headsGuard holds a reader to what it reads of the days' heads: the dates
// of the days whose reads by the method named, of a day's side or its rows,
// stood as it made them, and of the days whose lines it read, in turn.
type headsGuard struct {
	method string
	stood  map[string]bool
	raw    []string
}

// readingHeads is a guard of the reads by the method named, none made yet.
func readingHeads(method string) *headsGuard {
	return &headsGuard{method: method, stood: make(map[string]bool)}
}

func (g *headsGuard) reads(_ *soak, l views.Ledger) views.Ledger {
	return &countingLedger{Ledger: l, read: g.read}
}

// read notes a read by g's method that stood, or of a day's lines.
func (g *headsGuard) read(r ledgerRead) {
	switch r.method {
	case g.method:
		g.stood[r.asked[0]] = g.stood[r.asked[0]] || r.stood
	case "Day":
		g.raw = append(g.raw, r.asked[0])
	}
}

func (g *headsGuard) took(tea.Msg, Model) {}

// pickGuardDays are the days the find's pick's guarded world has run as it
// starts, in the evening, every past day's rows standing; and pickBack how
// many days back the session picked last ran: the day after it is beside
// it, its summary naming none of its lines but its neighbour's naming some,
// and none of those of the days between that and today name any.
const (
	pickGuardDays = 6
	pickBack      = 4
)

// The find's pick's part: the Log's find opened in the evening, enter on a
// session that last ran pickBack days back lands the Log on its newest line
// in one read of the Log's own. It reads the rows of no day the summaries
// rule out, as theirs and their neighbours' name none of its lines; no
// chunk of a day it has no line on, whose metas each rule it out, as the
// day beside its newest has, whose rows it reads; and no day's lines raw
// whose rows stand. Landed, it spans from the day the same history read
// whole spans from, its Requests are those of one read whole, and no reader
// holds what it decodes once, the find, closed, holding none of the days'
// summaries, as sameLogAsReadWhole has it.
func TestTheFindsPickReadsOnlyWhatMayHoldItsSession(t *testing.T) {
	start := soakAt(21, 0)
	g := &pickGuard{}
	s := newSoak(t, soakSetup{start: start, days: pickGuardDays, pace: guardPace, guard: g})
	s.show("log")
	n := busySeat(busyDays-1-pickBack, busyStarting)
	id := busySession(n)
	s.typed("/")
	s.typed(strings.Split(id[:8], "")...)
	if found := s.h.model.log.page.Finding.Found; len(found.Sessions) != 1 || found.Sessions[0].ID != id {
		t.Fatalf("the find found %+v for %s, want %s alone", found.Sessions, id[:8], id)
	}

	g.picking = true
	s.keyed(enter)
	m, now := s.h.model, s.h.model.now()
	newest, beside := dayfile.DateOf(dayfile.DayStart(start, -pickBack)), dayfile.DateOf(dayfile.DayStart(start, 1-pickBack))
	row, _, chosen := m.log.page.Chosen()
	if g.landed != 1 || !chosen || !m.log.page.Paused() || row.Session != id || m.log.find.landing != nil || m.log.from.Date != newest {
		t.Errorf("the pick landed in %d reads, paused %t at %t a line of %s, spanning from %s; want it landed in one, paused at the line of %s, from %s, its newest day",
			g.landed, m.log.page.Paused(), chosen, row.Session, m.log.from.Date, id, newest)
	}
	ran := daysRan(s.w, n)
	var rows []string
	for _, r := range g.rows {
		date := r.asked[0]
		rows = append(rows, date)
		bounds, _ := dayfile.DayBounds(date)
		if !slices.ContainsFunc(bounds.FileDates(), func(d string) bool { return ran[d] }) {
			t.Errorf("the pick read the rows of %s, whose summary and its neighbours' name none of %s's lines", date, id)
		}
		if !r.stood {
			t.Errorf("the rows of %s don't stand, want every past day's standing in the evening", date)
		}
	}
	if want := []string{beside, newest}; !slices.Equal(rows, want) {
		t.Errorf("the pick read the rows of %v, want %v alone: the day beside its newest, then its newest", rows, want)
	}
	for _, date := range g.chunks {
		if !ran[date] {
			t.Errorf("the pick read a chunk of %s, which holds none of %s's lines, its meta ruling it out", date, id)
		}
	}
	if len(g.raw) > 0 {
		t.Errorf("the pick read the lines of %v raw, want none, every past day's rows standing", g.raw)
	}
	sameLogAsReadWhole(t, m, pickedWhole(t, start, id, func(*soak) {}), s.follower, now)
}

// pickedWhole is the model of a soak of the find's guarded world from start,
// each past day read whole, as soakSetup's whole has it, the Log shown, the
// find's pick made of the session with the given id, then as after has it:
// the same history as a guarded soak's, read whole, for sameLogAsReadWhole
// to hold that soak's Log to.
func pickedWhole(t *testing.T, start time.Time, id string, after func(s *soak)) Model {
	t.Helper()
	s := newSoak(t, soakSetup{start: start, days: pickGuardDays, pace: guardPace, whole: true})
	s.show("log")
	s.typed("/")
	s.typed(strings.Split(id[:8], "")...)
	s.keyed(enter)
	after(s)
	return s.h.model
}

// The find's pick's filter let go while the Log is paused on the line it
// landed on, pickBack days back, as the panel lets it go: each day held
// through the filter is read again whole, from its rows, a day a read, the
// newest first. At every read landed, the Requests are those of one read
// whole through the filter they're tabled through, which every day serves,
// so no table is drawn through a filter its days don't serve; once every
// day is read again, they're every line's, as one read whole has them,
// paused still at the line the pick landed on, from the day the same
// history read whole spans from.
func TestLettingThePicksFilterGoWhilePausedReadsBackToOneReadWhole(t *testing.T) {
	start := soakAt(21, 0)
	g := &refilterGuard{t: t}
	s := newSoak(t, soakSetup{start: start, days: pickGuardDays, pace: guardPace, guard: g})
	s.show("log")
	id := busySession(busySeat(busyDays-1-pickBack, busyStarting))
	s.typed("/")
	s.typed(strings.Split(id[:8], "")...)
	s.keyed(enter)
	landed, _, chosen := s.h.model.log.page.Chosen()
	if !chosen || !s.h.model.log.page.Paused() || landed.Session != id {
		t.Fatalf("the pick is paused %t at %t a line of %s, want it paused at the newest line of %s", s.h.model.log.page.Paused(), chosen, landed.Session, id)
	}

	g.s = s
	s.filtered(func(f *views.LogFilter) { f.Sessions = nil })
	var want []string
	for back := 1; back <= pickBack; back++ {
		want = append(want, dayfile.DateOf(dayfile.DayStart(start, -back)))
	}
	if !slices.Equal(g.rows, want) || g.landed != pickBack {
		t.Errorf("the filter let go, the Log read again the rows of %v in %d reads, want %v, a day a read, the newest first", g.rows, g.landed, want)
	}
	m := s.h.model
	if row, _, chosen := m.log.page.Chosen(); !m.log.page.Paused() || !chosen || row.Request != landed.Request {
		t.Errorf("every day read again, the Log is paused %t at %t a line, %s; want it paused at the line the pick landed on, %s", m.log.page.Paused(), chosen,
			row.Request, landed.Request)
	}
	whole := pickedWhole(t, start, id, func(s *soak) { s.filtered(func(f *views.LogFilter) { f.Sessions = nil }) })
	sameLogAsReadWhole(t, m, whole, s.follower, m.now())
}

// refilterGuard holds the Log, once s is set, to what each read of its own
// lands: the dates of the days whose rows it read, in turn, and how many
// reads landed, each judged as servedAsOneRead has it.
type refilterGuard struct {
	t      *testing.T
	s      *soak
	rows   []string
	landed int
}

func (g *refilterGuard) reads(_ *soak, l views.Ledger) views.Ledger {
	return &countingLedger{Ledger: l, read: func(r ledgerRead) {
		if g.s != nil && r.method == "DayRows" {
			g.rows = append(g.rows, r.asked[0])
		}
	}}
}

func (g *refilterGuard) took(msg tea.Msg, _ Model) {
	g.t.Helper()
	if _, ok := msg.(logReadMsg); !ok || g.s == nil {
		return
	}
	g.landed++
	m := g.s.h.model
	servedAsOneRead(g.t, m, g.s.follower, m.now())
}

// daysRan are the dates of the days the world's nth session has a line
// filed on, as its lines arrived.
func daysRan(w *soakWorld, n int) map[string]bool {
	ran := make(map[string]bool)
	for _, r := range w.filedFrom(w.origin) {
		if r.g == n {
			ran[dayfile.DateOf(r.arrived())] = true
		}
	}
	return ran
}

// pickGuard holds the find's pick to what it reads: once picking, the reads
// of the days' rows, in turn; the dates of the chunks of them it read, and
// of the days whose lines it read raw; and how many reads of the Log's own
// landed.
type pickGuard struct {
	picking bool
	rows    []ledgerRead
	chunks  []string
	raw     []string
	landed  int
}

func (g *pickGuard) reads(_ *soak, l views.Ledger) views.Ledger {
	return &countingLedger{Ledger: l, read: g.read}
}

// read notes a read of a day's rows, a chunk of them, or its lines, once
// picking.
func (g *pickGuard) read(r ledgerRead) {
	if !g.picking {
		return
	}
	switch r.method {
	case "DayRows":
		g.rows = append(g.rows, r)
	case "RowChunk":
		g.chunks = append(g.chunks, r.asked[0])
	case "Day":
		g.raw = append(g.raw, r.asked[0])
	}
}

func (g *pickGuard) took(msg tea.Msg, _ Model) {
	if _, ok := msg.(logReadMsg); ok && g.picking {
		g.landed++
	}
}
