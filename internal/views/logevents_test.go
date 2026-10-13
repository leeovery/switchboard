package views_test

import (
	"iter"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// span is a span of time a reader of the router's events was asked for.
type span struct {
	from, to time.Time
}

// countedEvents reads the router's events through events, noting each span
// it's asked for, as the Log's Events' cost is held to.
type countedEvents struct {
	events views.Events
	asked  *[]span
}

func (c countedEvents) Between(from, to time.Time) iter.Seq[events.Line] {
	*c.asked = append(*c.asked, span{from: from, to: to})
	return c.events.Between(from, to)
}

// eventsRead is the router's events of the moment, as the Log's Events
// read them from the first day it shows: its files read through files, and
// the document taken in at a look.
func eventsRead(hm viewstest.EventMoment, files views.Events) views.EventsRead {
	return views.ReadEvents(files, hm.From, hm.Now).Look(liveAt(hm.Live, hm.Now), hm.Run)
}

// eventsOf is the Log's Events of the moment, the events read as eventsRead
// reads them, the moment's lines read as the Log's Requests read them.
func eventsOf(hm viewstest.EventMoment, read views.EventsRead) views.EventsTable {
	return views.LogEvents(views.LogEventSources{
		From: hm.From, Events: read, Read: views.ReadLog(ledgerHolding(hm.Now, hm.Lines...), hm.Lines),
		Names: status.NameOfKey, Prices: ledger.Pricing, Now: hm.Now,
	})
}

// dirOf is the directory the moment's lines of the session with the given
// id last name: "" where none does.
func dirOf(hm viewstest.EventMoment, session string) string {
	var dir string
	for _, l := range hm.Lines {
		if l.Session == session && l.Dir != "" {
			dir = l.Dir
		}
	}
	return dir
}

// wanted reports whether the event with the given key is among the
// moment's events wanted, whose facts it names: in a time zone other than
// the moment's, the first day the Log shows may begin before them.
func wanted(hm viewstest.EventMoment, key viewstest.Key) bool {
	return slices.ContainsFunc(hm.Events, func(l events.Line) bool { return viewstest.KeyOf(l) == key })
}

// moveSaid is what a row's move came to, as a moment wants it.
func moveSaid(m *views.EventMove) viewstest.Move {
	if m == nil {
		return viewstest.Move{}
	}
	said := viewstest.Move{Request: m.Request, Written: m.Written}
	if m.Cost != nil {
		said.Cost = *m.Cost
	}
	return said
}

// At each hard moment, the Log's Events are a row of each event read that
// happened from the first day they show, oldest first, each read once, as
// EventsSince merges the document's and the files' from the same start;
// each row says what events says of its event, told over the same events
// before it, those before the first day among them; its session's directory
// is as its lines last name it; a limit's, a cap's or a refusal's moved count
// is as its last version counts; and a move's row says what its move came
// to, as its request's line and its session's story settle it.
func TestTheLogsEventsAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.EventMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			files := filedIn(t, hm)
			table := eventsOf(hm, eventsRead(hm, files))

			telling := views.NewTelling(status.NameOfKey)
			var want []views.EventRow
			var keys []views.EventKey
			for _, e := range views.EventsSince(views.EventsFrom(hm.From), liveAt(hm.Live, hm.Now), hm.Run, files) {
				row := telling.Row(events.Held{Event: e.Event, Run: e.Key.Run}, hm.Now)
				if views.Today(hm.From).Holds(e.Event.At) {
					want, keys = append(want, row), append(keys, e.Key)
				}
			}
			if len(table.Rows) != len(want) {
				t.Fatalf("the Log's Events have %d rows, want %d: %v", len(table.Rows), len(want), keys)
			}
			for i, got := range table.Rows {
				w, key := want[i], viewstest.Key{Run: keys[i].Run.UnixNano(), ID: keys[i].ID}
				if got.Key != keys[i] || !got.At.Equal(w.At) || got.At.Location() != hm.Now.Location() || got.Session != w.Session {
					t.Errorf("row %d is event %v at %s of %q, want event %v at %s of %q, in now's time zone", i, got.Key, got.At, got.Session, keys[i], w.At, w.Session)
				}
				if got.Line.Kind != w.Kind || got.Line.Account.String() != w.Account.String() || got.Line.What.String() != w.What.String() {
					t.Errorf("event %d's row says %q, %q, %q; events says %q, %q, %q", got.Key.ID,
						got.Line.Kind, got.Line.Account, got.Line.What, w.Kind, w.Account, w.What)
				}
				if dir := dirOf(hm, got.Session); got.Dir != dir {
					t.Errorf("event %d's session's directory is %q, want %q, as its lines last name it", got.Key.ID, got.Dir, dir)
				}
				if wanted(hm, key) && got.Moved != hm.Facts[key].Moved {
					t.Errorf("event %d counts %d sessions moved, want %d", got.Key.ID, got.Moved, hm.Facts[key].Moved)
				}
				if said := moveSaid(got.Move); said != hm.Moves[key] {
					t.Errorf("event %d's move came to %+v, want %+v", got.Key.ID, said, hm.Moves[key])
				}
			}
		})
	}
}

// The Log's Events mark each event as LATELY does, from its kind, and of
// the router's health whether it's well again, at each hard moment, every
// event LATELY tells of among the document's.
func TestTheLogsEventsMarkEachEventAsLATELYDoes(t *testing.T) {
	compared := 0
	for _, hm := range viewstest.EventMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			m := liveAt(hm.Live, hm.Now)
			marked := make(map[views.EventKey]views.LogEvent)
			for _, row := range eventsOf(hm, views.ReadEvents(nil, hm.From, hm.Now).Look(m, hm.Run)).Rows {
				marked[row.Key] = row
			}
			for _, s := range views.Lately(m, len(hm.Live)) {
				row, ok := marked[views.EventKey{Run: hm.Run, ID: s.ID}]
				if !ok {
					continue
				}
				compared++
				if row.Kind != s.Kind || row.Well != s.Well {
					t.Errorf("event %d is marked as %q, well %v, but LATELY marks it as %q, well %v", s.ID, row.Kind, row.Well, s.Kind, s.Well)
				}
			}
		})
	}
	if compared == 0 {
		t.Error("no event is marked by both")
	}
}

// The Log's Events read the router's files once a span, at their first
// look, from FactsReach before the first day they show to the end of today;
// each look after takes in the document alone; and reading back reads the
// days back alone, once.
func TestTheLogsEventsReadTheFilesOnceASpan(t *testing.T) {
	hm := pressureTheDayBefore(t)
	var asked []span
	files := countedEvents{events: filedIn(t, hm), asked: &asked}
	read := views.ReadEvents(files, hm.From, hm.Now)
	for range 5 {
		read = read.Look(liveAt(hm.Live, hm.Now), hm.Run)
		eventsOf(hm, read)
	}
	today := views.Today(hm.From).Start
	want := []span{{from: today.Add(-views.FactsReach), to: today.AddDate(0, 0, 1)}}
	if !slices.Equal(asked, want) {
		t.Errorf("five looks read the files over %v, want %v, once", asked, want)
	}

	yesterday := today.AddDate(0, 0, -1)
	read = read.Back(files, yesterday).Back(files, yesterday).Back(files, today)
	want = append(want, span{from: yesterday.Add(-views.FactsReach), to: today.Add(-views.FactsReach)})
	if !slices.Equal(asked, want) {
		t.Errorf("reading back a day, and to it again, read the files over %v, want %v", asked, want)
	}
	if read.Since() != yesterday.Add(-views.FactsReach) {
		t.Errorf("read back a day, the events begin at %s, want %s", read.Since(), yesterday.Add(-views.FactsReach))
	}
}

// pressureTheDayBefore is the hard moment of a move after midnight whose
// pressure was told the day before.
func pressureTheDayBefore(t *testing.T) viewstest.EventMoment {
	t.Helper()
	for _, hm := range viewstest.EventMoments() {
		if hm.Name == "a move after midnight passing an account over, its pressure told the day before" {
			return hm
		}
	}
	t.Fatal("no moment of a move whose pressure was told the day before")
	return viewstest.EventMoment{}
}

// A move after midnight passing an account over says when that account
// runs out, as its pressure event the day before said, though the Log shows
// the day of the move alone: its events are read from FactsReach before.
func TestAMoveAfterMidnightSaysWhenThePressureToldTheDayBeforeRunsOut(t *testing.T) {
	hm := pressureTheDayBefore(t)
	rows := eventsOf(hm, eventsRead(hm, filedIn(t, hm))).Rows
	if len(rows) != 1 || rows[0].Kind != status.EventMoved {
		t.Fatalf("the Log's Events of the day after the pressure are %+v, want the move alone", rows)
	}
	if got, want := rows[0].Line.What.String(), "personal came under pressure, its 5-hour to run out at 01:10"; got != want {
		t.Errorf("the move says %q, want %q", got, want)
	}
}

// The document's events are kept as it gives newer ones, and as a router
// restarted gives its own, each once, by run and id, as it last stands; and
// one that happened before the read begins is passed over.
func TestTheDocumentsEventsAreKeptAsItGivesNewer(t *testing.T) {
	first, second := october(7, 8, 0), october(7, 12, 0)
	room := func(id int, at time.Time, account string) status.Event {
		return status.Event{ID: id, At: at, Kind: status.EventRoom, Account: account}
	}
	read := views.ReadEvents(nil, october(7, 0, 0), october(7, 9, 0)).
		Look(liveAt([]status.Event{room(2, october(7, 9, 0), "side"), room(1, october(6, 18, 0), "work")}, october(7, 9, 0)), first).
		Look(liveAt([]status.Event{room(3, october(7, 10, 0), "personal"), room(2, october(7, 9, 0), "work")}, october(7, 10, 0)), first).
		Look(liveAt([]status.Event{room(1, october(7, 12, 5), "side")}, october(7, 12, 5)), second)
	var got []string
	for _, e := range read.Events() {
		got = append(got, e.Key.Run.Format("15:04")+"#"+string(rune('0'+e.Key.ID))+" "+e.Event.Account)
	}
	want := []string{"08:00#2 work", "08:00#3 personal", "12:00#1 side"}
	if !slices.Equal(got, want) {
		t.Errorf("the events kept are %q, want %q", got, want)
	}
}

// A move event is of the move of its session and model, from and to the
// accounts it names, whose request arrived last at or before it: never a
// like move after it, nor one of another model; one naming no model, from a
// router from before events named it, is of its session's of any model.
func TestAMoveEventIsOfTheMoveWhoseRequestArrivedLastBeforeIt(t *testing.T) {
	const off = "moved: work is at its reserve"
	main := func(id, model, account, reason string, at time.Time) ledger.Line {
		return withID(id, of(ledger.ClassMain, line(mover, model, account, reason, at, hourUsage)))
	}
	lines := []ledger.Line{
		main("o0", opus, "work", "new", october(7, 10, 0)),
		main("h0", haiku, "work", "new", october(7, 10, 0).Add(time.Second)),
		movedFrom("work", main("o1", opus, "side", off, october(7, 10, 30))),
		movedFrom("work", main("h1", haiku, "side", off, october(7, 10, 30).Add(time.Second))),
		movedFrom("side", main("o2", opus, "work", "rescored after 1h idle", october(7, 11, 0))),
		movedFrom("work", main("o3", opus, "side", off, october(7, 11, 20))),
	}
	tests := []struct {
		name  string
		event status.Event
		want  string
	}{
		{name: "the first like move, a moment after its request", event: moving(opus, "work", "side", october(7, 10, 30).Add(200*time.Millisecond)), want: "o1"},
		{name: "the second like move", event: moving(opus, "work", "side", october(7, 11, 20).Add(200*time.Millisecond)), want: "o3"},
		{name: "a move as its request arrived, to the millisecond", event: moving(opus, "side", "work", october(7, 11, 0)), want: "o2"},
		{name: "another model's like move", event: moving(haiku, "work", "side", october(7, 10, 30).Add(1200*time.Millisecond)), want: "h1"},
		{name: "a like move told after another model's", event: moving(opus, "work", "side", october(7, 10, 30).Add(1300*time.Millisecond)), want: "o1"},
		{name: "a move naming no model", event: moving("", "work", "side", october(7, 10, 30).Add(1200*time.Millisecond)), want: "h1"},
		{name: "a move before any like move's request", event: moving(opus, "work", "side", october(7, 10, 29))},
		{name: "a move between accounts no request moved it between", event: moving(opus, "side", "personal", october(7, 11, 30))},
		{name: "a move of a session no line tells of", event: status.Event{ID: 1, At: october(7, 11, 30), Kind: status.EventMoved, Session: older, Model: opus, From: "work", To: "side"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hm := viewstest.EventMoment{Live: []status.Event{tt.event}, Run: october(7, 8, 0), Now: october(7, 12, 0), From: october(7, 0, 0), Lines: lines}
			rows := eventsOf(hm, eventsRead(hm, nil)).Rows
			if len(rows) != 1 {
				t.Fatalf("the Log's Events are %+v, want the move alone", rows)
			}
			if got := moveSaid(rows[0].Move).Request; got != tt.want {
				t.Errorf("the move at %s is of request %q, want %q", tt.event.At.Format("15:04:05.000"), got, tt.want)
			}
		})
	}
}

// moving is mover's model moving from one account to another at at, as the
// router tells of it.
func moving(model, from, to string, at time.Time) status.Event {
	return status.Event{ID: 1, At: at, Kind: status.EventMoved, Session: mover, Model: model, From: from, To: to, Reason: "moved: work is at its reserve"}
}

// A move's cost is the same wherever it's said, as its session's story
// settles it, at each hard moment: the Log's Events row of its event, the
// row of the request that made it in the Log's Requests, and the List's
// move that brought its session to the account it's on, where that's the
// one.
func TestAMovesCostIsTheSameInItsEventsRowItsRequestsRowAndTheList(t *testing.T) {
	compared := make(map[string]int)
	for _, hm := range viewstest.EventMoments() {
		if len(hm.Lines) == 0 {
			continue
		}
		t.Run(hm.Name, func(t *testing.T) {
			rows := eventsOf(hm, eventsRead(hm, filedIn(t, hm))).Rows
			requests := views.LogRows(views.LogSources{
				From: hm.Lines[0].At, Now: hm.Now, Read: views.ReadLog(ledgerHolding(hm.Now, hm.Lines...), hm.Lines), Prices: ledger.Pricing,
			})
			list := views.ListSessions(views.SessionSources{Unlisted: true, Ledger: ledgerHolding(hm.Now, hm.Lines...), Prices: ledger.Pricing, Now: hm.Now})
			for _, row := range rows {
				if row.Move == nil {
					continue
				}
				for _, l := range requests.Lines {
					if l.Rule == nil && l.Row.Request == row.Move.Request {
						compared["request"]++
						if !sameCost(l.Row.Routing.Cost, row.Move.Cost) {
							t.Errorf("event %d's move cost %s, but its request %s's routing says %s", row.Key.ID, costSaid(row.Move.Cost), l.Row.Request, costSaid(l.Row.Routing.Cost))
						}
					}
				}
				for _, s := range list.Sessions {
					if s.Session == row.Session && s.Moved != nil && s.Moved.At.Equal(row.Move.At) {
						compared["list"]++
						if !sameCost(s.Moved.Cost, row.Move.Cost) {
							t.Errorf("event %d's move cost %s, but the List's says %s", row.Key.ID, costSaid(row.Move.Cost), costSaid(s.Moved.Cost))
						}
					}
				}
			}
		})
	}
	for _, where := range []string{"request", "list"} {
		if compared[where] == 0 {
			t.Errorf("no move's cost is held to its %s's", where)
		}
	}
}

// sameCost reports whether two costs are the same: both none, or the same
// amount.
func sameCost(a, b *ledger.Picodollars) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// costSaid says a cost, "none" for none.
func costSaid(c *ledger.Picodollars) string {
	if c == nil {
		return "none"
	}
	return c.Cents()
}
