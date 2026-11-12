package watch

import (
	"io/fs"
	"iter"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// countedFiles are the router's files of events, read as an events.Reader
// reads them, a file a local day: every day's file from the day before a
// read's from to the day after its to, events.ChangesFor days on, or now
// on its clock where that's sooner, as an event's versions are filed on the
// days after it; counting each day's file read by its date.
type countedFiles struct {
	lines []events.Line
	clock *fakeClock
	reads map[string]int
}

// filesOf are the router's files holding the lines given, none read yet,
// read at the time the clock it's given tells.
func filesOf(lines ...events.Line) *countedFiles {
	return &countedFiles{lines: lines, reads: make(map[string]int)}
}

func (c *countedFiles) Between(from, to time.Time) iter.Seq[events.Line] {
	last := to.AddDate(0, 0, events.ChangesFor)
	if c.clock.now.Before(last) {
		last = c.clock.now
	}
	for _, date := range dayfile.Dates(from, last) {
		c.reads[date]++
	}
	return func(yield func(events.Line) bool) {
		for _, l := range c.lines {
			if !l.At.Before(from) && l.At.Before(to) && !yield(l) {
				return
			}
		}
	}
}

func (c *countedFiles) FirstDay() (time.Time, error) {
	if len(c.lines) == 0 {
		return time.Time{}, fs.ErrNotExist
	}
	first := slices.MinFunc(c.lines, func(a, b events.Line) int { return a.At.Compare(b.At) })
	return dayfile.DayStart(first.At.Local(), 0), nil
}

// readOnce reports whether each day's file read was read once alone.
func (c *countedFiles) readOnce() bool {
	for _, n := range c.reads {
		if n != 1 {
			return false
		}
	}
	return true
}

// datesRead are the dates of the days' files read, the oldest first.
func (c *countedFiles) datesRead() []string {
	return slices.Sorted(maps.Keys(c.reads))
}

// limitAt is an account's 5-hour limit with the id given, at the time given.
func limitAt(id int, at time.Time, account string) status.Event {
	return status.Event{ID: id, At: at, Kind: status.EventLimit, Account: account, Windows: []string{"5h"}}
}

// latelySaid are the lines Runway's LATELY tells, in full.
func latelySaid(m Model) []string {
	var said []string
	for _, s := range m.runwayTab.shown.Lately {
		said = append(said, s.Full.String())
	}
	return said
}

// runwayOfRun is Runway shown of the router's document giving the events
// given, the router's run having begun at run, its files of events those
// given, read at the harness's time.
func runwayOfRun(t *testing.T, run time.Time, files *countedFiles, given ...status.Event) *runwayed {
	t.Helper()
	doc := routerDocument(three()...)
	doc.Events = given
	return runwayHarnessOf(t, doc, dashboard.Runway, func(r *runwayed, cfg *Config) {
		r.source.health = router.Health{OK: true, PID: 1, StartedAt: run}
		files.clock, cfg.Events = r.clock, files
	})
}

// Runway's LATELY tells the events the router filed over the week, beyond
// those its document gives, as a router keeps its newest 50 and forgets
// them as it restarts; and a router restarted, its ids from 1 again, those
// of the run before are kept.
func TestRunwaysLATELYReachesBackPastTheDocumentAndAcrossARestart(t *testing.T) {
	before, run := past(-5*day), past(-time.Hour)
	filed, given := limitAt(1, past(-4*day), "side"), limitAt(1, past(-30*time.Minute), "work")
	r := runwayOfRun(t, run, filesOf(events.Line{Event: filed, Run: before}, events.Line{Event: given, Run: run}), given)
	want := []string{"work reached its 5-hour limit", "side reached its 5-hour limit"}
	if got := latelySaid(r.model); !slices.Equal(got, want) {
		t.Errorf("LATELY tells %q, want %q: the document's, and the week's the router filed before it started", got, want)
	}

	restarted := limitAt(1, past(time.Minute), "personal")
	doc := routerDocument(three()...)
	doc.Events = []status.Event{restarted}
	r.source.health = router.Health{OK: true, PID: 2, StartedAt: past(30 * time.Second)}
	r.clock.now = past(2 * time.Minute)
	r.startRouter(doc)
	r.typed("R")
	want = append([]string{"personal reached its 5-hour limit"}, want...)
	if got := latelySaid(r.model); !slices.Equal(got, want) {
		t.Errorf("the router restarted, LATELY tells %q, want %q: the new run's, and every one the run before told", got, want)
	}
}

// Till the read of the router's files lands, LATELY tells the events the
// document gives; as it lands, it fills back.
func TestRunwaysLATELYTellsTheDocumentsEventsTillTheFilesAreRead(t *testing.T) {
	before, run := past(-5*day), past(-time.Hour)
	filed, given := limitAt(1, past(-4*day), "side"), limitAt(1, past(-30*time.Minute), "work")
	files := filesOf(events.Line{Event: filed, Run: before}, events.Line{Event: given, Run: run})
	doc := routerDocument(three()...)
	doc.Events = []status.Event{given}
	h := routedHarness(t, doc)
	h.source.health = router.Health{OK: true, PID: 1, StartedAt: run}
	cfg := h.config(Size{Width: 173, Height: 53})
	files.clock, cfg.Events, cfg.View = h.clock, files, dashboard.Runway
	h.model = New(t.Context(), cfg)
	read := holdingBack[latelyReadMsg](h, h.init()...)
	if got, want := latelySaid(h.model), []string{"work reached its 5-hour limit"}; len(read) != 1 || !slices.Equal(got, want) {
		t.Fatalf("its read of the files under way (%d), LATELY tells %q, want %q, the document's", len(read), got, want)
	}
	h.deliver(read...)
	if got, want := latelySaid(h.model), []string{"work reached its 5-hour limit", "side reached its 5-hour limit"}; !slices.Equal(got, want) {
		t.Errorf("its read landed, LATELY tells %q, want %q, the files' among them", got, want)
	}
}

// LATELY reads the files of the week it tells once, as an events.Reader
// reads them, from the day before the week's to the day after today's: not
// again as the document's read afresh, nor as midnight passes, as the day
// that closed was read as it was today's; and tells none of the events
// before the week.
func TestRunwaysLATELYReadsItsWeekOnceAndTellsNoneBeforeIt(t *testing.T) {
	run := past(-time.Hour)
	files := filesOf(events.Line{Event: limitAt(1, past(-8*day), "work"), Run: past(-9 * day)})
	r := runwayOfRun(t, run, files)
	want := dayfile.Dates(views.RunwayLatelyFrom(past(0)), past(0))
	midnight := dayfile.DayStart(past(0).Local(), 1)
	for _, at := range []time.Time{past(time.Minute), past(2 * time.Minute), midnight.Add(time.Minute), midnight.Add(time.Hour)} {
		r.clock.now = at
		r.typed("R")
	}
	if got := files.datesRead(); !slices.Equal(got, want) || !files.readOnce() {
		t.Errorf("across refreshes and a midnight, LATELY read the days' files %v, %v; want %v, each once", got, files.reads, want)
	}
	if len(r.model.runwayTab.shown.Lately) != 0 {
		t.Errorf("LATELY tells %q, want nothing from before its week", latelySaid(r.model))
	}
}

// LATELY and the Log hold the router's events in one read: where Runway
// shows first, the Log, shown after, reads none of its days' files again.
func TestRunwayAndTheLogReadEachDayOnce(t *testing.T) {
	run := past(-time.Hour)
	files := filesOf(events.Line{Event: limitAt(1, past(-30*time.Minute), "work"), Run: run})
	r := runwayOfRun(t, run, files)
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -time.Minute))
	onTheLog(r.harness, state)
	r.typed("R")
	if got, want := files.datesRead(), dayfile.Dates(views.RunwayLatelyFrom(past(0)), past(0)); !slices.Equal(got, want) || !files.readOnce() {
		t.Errorf("Runway then the Log read the days' files %v, %v; want %v, each once", got, files.reads, want)
	}
	if rows := r.model.log.page.Events.Rows; len(rows) != 1 || rows[0].Key != (views.EventKey{Run: run, ID: 1}) {
		t.Errorf("the Log's Events show %v, want work's limit, of the events held", rows)
	}
}

// Where the Log read the router's events first, LATELY reads back its week
// before them, as an events.Reader reads back: its files from the day
// before the week's on to today's and the day after, as an event's versions
// are filed on the days after it, so those the Log read are read again, the
// rest once.
func TestLatelyReadsBackTheWeekBeforeTheLogsDays(t *testing.T) {
	run := past(-time.Hour)
	filed := limitAt(1, past(-3*day), "side")
	files := filesOf(events.Line{Event: filed, Run: past(-4 * day)})
	h := routedHarness(t, routerDocument(three()...))
	h.source.health = router.Health{OK: true, PID: 1, StartedAt: run}
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -time.Minute))
	onTheLog(h, state)
	files.clock, h.model.cfg.Events = h.clock, files
	h.start()
	logged := files.datesRead()
	h.model.view = dashboard.Runway
	h.typed("R")
	if !slices.Equal(logged, dayfile.Dates(views.EventsFrom(past(0)), past(0))) {
		t.Errorf("the Log read the days' files %v, want those of its day", logged)
	}
	for date, n := range files.reads {
		if want := 1 + btoi(slices.Contains(logged, date)); n != want {
			t.Errorf("the Log then Runway read %s's file %d times, want %d: %v, the Log's %v", date, n, want, files.reads, logged)
		}
	}
	if got, want := files.datesRead(), dayfile.Dates(views.RunwayLatelyFrom(past(0)), past(0)); !slices.Equal(got, want) {
		t.Errorf("the Log then Runway read the days' files %v, want %v, LATELY's week", got, want)
	}
	if got, want := latelySaid(h.model), []string{"side reached its 5-hour limit"}; !slices.Equal(got, want) {
		t.Errorf("LATELY tells %q, want %q, read back past the Log's days", got, want)
	}
}

// btoi is 1 where b is set, else 0.
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// The events held let go of those neither the Log nor Runway's LATELY
// wants: as the Log lets go of the days its tables no longer show, those
// before them, but none of LATELY's week where it wants them; and as
// LATELY's week moves on, the day before it, a day at a time.
func TestTheEventsHeldKeepWhatTheLogAndLATELYWant(t *testing.T) {
	now := past(0)
	later := now.Add(day)
	cut := dayfile.DayStart(views.RunwayLatelyFrom(later).Local(), 0)
	gone := limitAt(1, cut.Add(-time.Minute), "side")
	files := filesOf(events.Line{Event: gone, Run: past(-8 * day)})
	files.clock = &fakeClock{now: now}
	read := views.ReadEventsFrom(files, views.RunwayLatelyFrom(now), now)
	logged := logTab{opened: true, from: views.Today(now)}
	tests := []struct {
		name   string
		log    logTab
		lately bool
		at     time.Time
		since  time.Time
		held   int
	}{
		{name: "the Log's alone", log: logged, at: now, since: views.EventsFrom(now)},
		{name: "the Log's and LATELY's", log: logged, lately: true, at: now, since: views.RunwayLatelyFrom(now), held: 1},
		{name: "LATELY's, a day on", lately: true, at: later, since: cut},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{log: tt.log, events: heldEvents{lately: tt.lately}.took(&read, nil)}.eventsLetGo(tt.at)
			if got := m.events.read; !got.Since().Equal(tt.since) || len(got.Events()) != tt.held {
				t.Errorf("the events held reach back to %v, holding %d; want %v, holding %d", got.Since(), len(got.Events()), tt.since, tt.held)
			}
		})
	}
}

// Runway's LATELY is told afresh only as the events held change, the day
// turns, as its words say a time beside it, or a line it told passes out of
// its week: told again with none of those changed, it's as it was told.
func TestRunwaysLATELYIsToldAfreshAsWhatItTellsOfChanges(t *testing.T) {
	now := past(0)
	edge := events.Line{Event: limitAt(1, views.RunwayLatelyFrom(now).Add(2*time.Minute), "side"), Run: past(-7 * day)}
	recent := events.Line{Event: limitAt(2, now.Add(-day), "work"), Run: past(-7 * day)}
	tests := []struct {
		name    string
		held    []events.Line
		changed bool
		at      time.Time
		afresh  bool
	}{
		{name: "nothing it tells of changed", held: []events.Line{edge, recent}, at: now.Add(time.Minute)},
		{name: "the events held changed", held: []events.Line{edge, recent}, changed: true, at: now.Add(time.Minute), afresh: true},
		{name: "its oldest line passed out of its week", held: []events.Line{edge, recent}, at: now.Add(3 * time.Minute), afresh: true},
		{name: "the day turned", held: []events.Line{recent}, at: dayfile.DayStart(now.Local(), 1).Add(time.Minute), afresh: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := filesOf(tt.held...)
			files.clock = &fakeClock{now: now}
			read := views.ReadEventsFrom(files, views.RunwayLatelyFrom(now), now)
			m := Model{events: heldEvents{lately: true}.took(&read, nil)}
			m.runwayTab.lately = m.latelyTold(status.Moment{Now: now})
			if tt.changed {
				m.events.changes++
			}
			was, told := m.runwayTab.lately.sentences, m.latelyTold(status.Moment{Now: tt.at}).sentences
			if afresh := &was[0] != &told[0]; afresh != tt.afresh {
				t.Errorf("LATELY told afresh %t, want %t", afresh, tt.afresh)
			}
		})
	}
}

// latelyCut is where the events held for Runway's LATELY begin at now: the
// start of the day its week begins on.
func latelyCut(now time.Time) time.Time {
	return dayfile.DayStart(views.RunwayLatelyFrom(now).Local(), 0)
}

// heldSince reports whether the events m holds begin at since, and hold
// none from before it.
func heldSince(m Model, since time.Time) bool {
	read := m.events.read
	return read.Since().Equal(since) && !slices.ContainsFunc(read.Events(), func(e views.Event) bool { return e.Event.At.Before(since) })
}

// A dashboard left on the Overview, once Runway's LATELY has read the week,
// lets go of the events held as each look takes in more, day after day:
// it holds no more than LATELY's week.
func TestTheEventsHeldOffRunwayHoldNoMoreThanTheWeek(t *testing.T) {
	run := past(-time.Hour)
	r := runwayOfRun(t, run, filesOf(events.Line{Event: limitAt(1, past(-6*day), "side"), Run: past(-7 * day)}))
	r.model.view = dashboard.Overview
	for d := 1; d <= 10; d++ {
		r.clock.now = past(time.Duration(d) * day)
		doc := routerDocument(three()...)
		doc.Events = []status.Event{limitAt(d, r.clock.now.Add(-time.Minute), "work")}
		r.startRouter(doc)
		r.typed("R")
	}
	if now := r.clock.now; !heldSince(r.model, latelyCut(now)) {
		t.Errorf("ten days on the Overview, the events held begin at %v, holding %d; want none before %v", r.model.events.read.Since(), len(r.model.events.read.Events()), latelyCut(now))
	}
}

// A read of the files under way as midnight passes and the events held let
// go of the day before LATELY's week claims none of it as it lands.
func TestARefileUnderWayAcrossMidnightClaimsNoDayLetGo(t *testing.T) {
	run := past(-time.Hour)
	r := runwayOfRun(t, run, filesOf(events.Line{Event: limitAt(1, latelyCut(past(0)).Add(time.Hour), "side"), Run: past(-7 * day)}))
	r.source.health = router.Health{OK: true, PID: 2, StartedAt: past(time.Minute)}
	r.clock.now = past(2 * time.Minute)
	r.startRouter(routerDocument(three()...))
	read := holdingBack[latelyReadMsg](r.harness, r.press("R")...)
	if len(read) != 1 {
		t.Fatalf("the router restarted, LATELY read the files %d times, want once", len(read))
	}
	r.clock.now = dayfile.DayStart(past(0).Local(), 1).Add(time.Minute)
	if more := holdingBack[latelyReadMsg](r.harness, r.press("R")...); len(more) != 0 {
		t.Fatalf("a read under way, LATELY read the files %d more times, want none", len(more))
	}
	r.deliver(read...)
	if cut := latelyCut(r.clock.now); !heldSince(r.model, cut) {
		t.Errorf("the read landed past midnight, the events held begin at %v, holding %d; want none before %v", r.model.events.read.Since(), len(r.model.events.read.Events()), cut)
	}
}
