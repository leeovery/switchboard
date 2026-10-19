package watch

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// The page's part: the long session's page open across midnight, its
// newest turn opened before it and closed at the end, the store fed for it
// at every read of the ledger's tail. The page's session is read once, as
// it opens, resting on its state at the end of the day before, and its
// turn's lines once, as it opens, never again; a read of sessions reads no
// day behind the digests' states the sessions it gives rest on; a day
// before today is read again only as the day turns; and the page holds its
// session as the store holds it, no more of its requests open than the
// store's rule keeps open, the lines of no request but its turn open's,
// while one is, and its points the answers of today and yesterday alone.
// Just after midnight, a few minutes on and at the end, the page and its
// turn open are those of a page read whole at the same moment.
func TestThePageHoldsOnlyItsTurns(t *testing.T) {
	midnight := dayfile.DayStart(guardStart, 1)
	begins, ends := midnight.Add(-time.Minute), midnight.Add(guardLateFor+time.Minute)
	closes := ends.Add(-30 * time.Second)
	g := &pageGuard{t: t, checks: []time.Time{midnight.Add(30 * time.Second), midnight.Add(guardLateFor), closes.Add(-2 * lookEvery)}}
	s := newSoak(t, soakSetup{start: begins, days: guardDays, pace: guardPace, guard: g})
	g.id = s.w.longest()
	s.show("session")
	if m := s.h.model; m.session == nil || m.session.shown.Loading || g.wholes != 1 || g.rested != 1 {
		t.Fatalf("the long session's page is open %t, its session read %d times, %d of them resting on its state at the end of the day before; want it "+
			"open, read once, resting on it", m.session != nil, g.wholes, g.rested)
	}
	s.schedule = append(s.schedule,
		soakAction{at: midnight.Add(-30 * time.Second), do: func() { s.keyed(tea.KeyPressMsg{Code: tea.KeyEnter}) }},
		soakAction{at: closes, do: func() { s.keyed(tea.KeyPressMsg{Code: tea.KeyEscape}) }},
	)
	s.runUntil(ends)
	g.judged()
	m := s.h.model
	switch {
	case m.session == nil:
		t.Fatal("the page closed, want it open to the end")
	case len(g.checks) > 0:
		t.Errorf("no read of the tail landed after %v to compare the page at", g.checks)
	case g.wholes != 1 || g.turns != 1 || g.fed == 0 || g.late == 0 || !g.opened || g.answers == 0:
		t.Errorf("the page's session was read %d times, a turn's lines %d, the store fed %d times, the day before read again %d, a turn compared open %t, "+
			"its points holding answers at %d reads; want the session read once, a turn's lines once, the store fed at each read of the tail, the day before "+
			"read again as the day turned, a turn compared, answers held", g.wholes, g.turns, g.fed, g.late, g.opened, g.answers)
	}
	if turn := reflect.ValueOf(m.session.held).FieldByName("turn"); m.session.shown.Open != 0 || !turn.IsNil() {
		t.Errorf("its turn closed, the page has turn %d open, holding its lines %t; want none open, none held", m.session.shown.Open, !turn.IsNil())
	}
}

// pageGuard holds the page to what it reads and holds, read by read: s is
// the soak; id the page's session's; unjudged the reads of the ledger's
// lines since a read last landed; want what the read of the tail under way
// wants of the store; checks the moments still to compare the page with
// one read whole at, the earliest first; wholes how many times the page's
// session was read alone, turns how many times a turn's lines were, rested
// how many of the reads of it alone gave it resting on its state at the end
// of the day before then, fed how many reads of the tail fed the store, late
// how many times the day before was read again; opened whether the page was
// compared with a turn open; and answers at how many reads its points held
// answers.
type pageGuard struct {
	t        *testing.T
	s        *soak
	id       string
	unjudged []ledgerRead
	want     views.SessionsWanted
	checks   []time.Time
	wholes   int
	turns    int
	rested   int
	fed      int
	late     int
	opened   bool
	answers  int
}

func (g *pageGuard) reads(s *soak, l views.Ledger) views.Ledger {
	g.s = s
	return &countingLedger{Ledger: l, read: g.counted}
}

// counted counts a read of the ledger's lines as it's made, and holds it to
// judge once the read it's made for lands; of a read of the tail, what it
// wants of the store; and of a read of the page's session alone, whether it
// gave it resting on its state at the end of the day before then.
func (g *pageGuard) counted(r ledgerRead) {
	m := g.s.h.model
	switch r.method {
	case "Today":
		if f := m.heldFold(m.now()); f != nil {
			g.want = f.want
		}
	case "SessionsOf", "Sessions":
		if slices.Equal(r.asked, []string{g.id}) {
			g.wholes++
			if r.rests[g.id] == dayfile.DateOf(dayfile.DayStart(m.now(), -1)) {
				g.rested++
			}
		}
	case "Session":
		g.turns++
	}
	g.unjudged = append(g.unjudged, r)
}

func (g *pageGuard) took(msg tea.Msg, _ Model) {
	if msg, ok := msg.(tailedMsg); ok {
		g.tailed(msg)
	}
}

// tailed judges a read of the tail that landed: the reads it made, as
// judged has them; what the page holds, as holds has it; and at the moments
// due, the page against one read whole.
func (g *pageGuard) tailed(msg tailedMsg) {
	g.t.Helper()
	g.judged()
	if msg.held == nil || g.s.h.model.session == nil {
		return
	}
	g.fed++
	now := g.s.h.model.now()
	g.holds(now)
	if len(g.checks) > 0 && !now.Before(g.checks[0]) {
		g.sameAsOneRead(now)
		g.checks = g.checks[1:]
	}
}

// judged judges the reads of the ledger's lines since a read last landed: a
// day before today read only as the day turns; and a read of sessions
// reading no day behind the states the sessions it gave rest on, as
// readNoneBehind has it. None are left to judge.
func (g *pageGuard) judged() {
	g.t.Helper()
	now := g.s.h.model.now()
	for _, r := range g.unjudged {
		switch r.method {
		case "Day":
			g.late++
			if r.asked[0] != dayfile.DateOf(dayfile.DayStart(now, -1)) || now.Sub(dayfile.DayStart(now, 0)) >= guardLateFor {
				g.t.Errorf("at %s the day %s was read again, want only the day before today, in the first %s of the day", now.Format(time.DateTime), r.asked[0],
					guardLateFor)
			}
		case "Sessions", "SessionsOf":
			readNoneBehind(g.t, r, now)
		}
	}
	g.unjudged = nil
}

// holds judges what the page holds at now: its session as the store holds
// it, not a copy of its own; no more of its requests open than the world
// filed at or past the horizon the store's rule gives it; the lines of no
// turn while none is open, nor of any request else, by any path; and its
// points' answers of today and yesterday alone, as pageHolds finds them.
func (g *pageGuard) holds(now time.Time) {
	g.t.Helper()
	m := g.s.h.model
	held := reflect.ValueOf(m.session.held)
	store := reflect.ValueOf(m.held).FieldByName("sessions").MapIndex(reflect.ValueOf(g.id))
	if !store.IsValid() || held.FieldByName("record").Pointer() != store.Pointer() {
		g.t.Errorf("at %s the page holds its session apart from the store's", now.Format(time.DateTime))
	}
	horizon := (&storeGuard{want: g.want}).horizonOf(g.id, now)
	past := 0
	for _, r := range g.s.w.filedFrom(horizon) {
		if busySession(r.g) == g.id {
			past++
		}
	}
	if open := held.FieldByName("record").Elem().FieldByName("open").Len(); open > past {
		g.t.Errorf("at %s the page holds %d of its requests open, more than the %d the world filed at or past %s", now.Format(time.DateTime), open, past,
			horizon.Format(time.DateTime))
	}
	if turn := held.FieldByName("turn"); m.session.shown.Open == 0 && !turn.IsNil() {
		g.t.Errorf("at %s the page holds the lines of a turn, none open", now.Format(time.DateTime))
	}
	dates, lines := pageHolds(m.session.held)
	if m.session.shown.Open == 0 && lines > 0 {
		g.t.Errorf("at %s the page holds %d lines of requests, no turn open", now.Format(time.DateTime), lines)
	}
	today, yesterday := views.Today(now).Date, dayfile.DateOf(dayfile.DayStart(now, -1))
	for date, n := range dates {
		if date != today && date != yesterday {
			g.t.Errorf("at %s the page's points hold %d answers of %s, want those of today and yesterday alone", now.Format(time.DateTime), n, date)
		}
	}
	if len(dates) > 0 {
		g.answers++
	}
}

// pageHolds walks what the page h holds, by any path, as daysHeld walks the
// Log: how many answers its points hold, by the date of the day each
// arrived on, and how many lines of requests it holds.
func pageHolds(h views.HeldPage) (map[string]int, int) {
	answers, lines := make(map[string]int), 0
	walked(h, func(v reflect.Value) bool {
		switch t := v.Type(); {
		case t == reflect.TypeFor[ledger.Line]():
			lines++
			return false
		case t.PkgPath() == viewsPkg && t.Name() == "answered":
			answers[dayfile.DateOf(time.Unix(v.FieldByName("flight").FieldByName("at").FieldByName("sec").Int(), 0))]++
		}
		return true
	})
	return answers, lines
}

// sameAsOneRead judges the page at now, and its turn open, those of a page
// read whole at now, its turn open read whole as it opens; and none of the
// readers holding what they decode once, as heldRaw has it.
func (g *pageGuard) sameAsOneRead(now time.Time) {
	g.t.Helper()
	m := g.s.h.model
	heldRaw(g.t, m, now)
	live := m.pageLive(g.id, now)
	live.Stream = live.Stream.Of(g.id)
	whole := views.ReadPage(g.s.follower, g.id, live.Stream, m.cfg.Prices, m.cfg.LedgerKeep, now)
	open := m.session.shown.Open
	if open != 0 {
		whole = whole.Opening(open, live)
		whole = whole.Turned(whole.ReadTurn(g.s.follower, live))
		g.opened = true
	}
	got, _ := m.session.held.Build(live)
	want, _ := whole.Build(live)
	got.Story.Sent, want.Story.Sent = views.SentAt(slices.Collect(got.Story.Sent.All())...), views.SentAt(slices.Collect(want.Story.Sent.All())...)
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		g.t.Errorf("at %s the page fed read by read is\n%s\nwant the page read whole\n%s", now.Format(time.DateTime), gotJSON, wantJSON)
	}
}
