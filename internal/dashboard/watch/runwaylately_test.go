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
// reads them, a file a local day, counting each day's file read by its
// date, as each read reaches it.
type countedFiles struct {
	lines []events.Line
	reads map[string]int
}

// filesOf are the router's files holding the lines given, none read yet.
func filesOf(lines ...events.Line) *countedFiles {
	return &countedFiles{lines: lines, reads: make(map[string]int)}
}

func (c *countedFiles) Between(from, to time.Time) iter.Seq[events.Line] {
	for day := dayfile.DayStart(from.Local(), 0); day.Before(to); day = dayfile.DayStart(day, 1) {
		c.reads[dayfile.DateOf(day)]++
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

// localDates are the dates of the local days from the one from falls on to
// the one to falls on, the oldest first.
func localDates(from, to time.Time) []string {
	var dates []string
	for day := dayfile.DayStart(from.Local(), 0); !day.After(to); day = dayfile.DayStart(day, 1) {
		dates = append(dates, dayfile.DateOf(day))
	}
	return dates
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
// given.
func runwayOfRun(t *testing.T, run time.Time, files *countedFiles, given ...status.Event) *runwayed {
	t.Helper()
	doc := routerDocument(three()...)
	doc.Events = given
	return runwayHarnessOf(t, doc, dashboard.Runway, func(r *runwayed, cfg *Config) {
		r.source.health = router.Health{OK: true, PID: 1, StartedAt: run}
		cfg.Events = files
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
	cfg.Events, cfg.View = files, dashboard.Runway
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

// LATELY reads each day's file of the week it tells once: not again as the
// document's read afresh, nor as midnight passes, as the day that closed was
// read as it was today's; and none before the week.
func TestRunwaysLATELYReadsEachDayOnceAndNoneBeforeItsWeek(t *testing.T) {
	run := past(-time.Hour)
	files := filesOf(events.Line{Event: limitAt(1, past(-8*day), "work"), Run: past(-9 * day)})
	r := runwayOfRun(t, run, files)
	want := localDates(views.RunwayLatelyFrom(past(0)), past(0))
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
	if got, want := files.datesRead(), localDates(views.RunwayLatelyFrom(past(0)), past(0)); !slices.Equal(got, want) || !files.readOnce() {
		t.Errorf("Runway then the Log read the days' files %v, %v; want %v, each once", got, files.reads, want)
	}
	if rows := r.model.log.page.Events.Rows; len(rows) != 1 || rows[0].Key != (views.EventKey{Run: run, ID: 1}) {
		t.Errorf("the Log's Events show %v, want work's limit, of the events held", rows)
	}
}

// Where the Log read the router's events first, LATELY reads back the days
// of its week before those, each once: but the day the Log's reach back
// for its facts falls in, read for its last hours, as the Log's own read
// back reads it again.
func TestLatelyReadsBackTheWeekBeforeTheLogsDays(t *testing.T) {
	run := past(-time.Hour)
	filed := limitAt(1, past(-3*day), "side")
	files := filesOf(events.Line{Event: filed, Run: past(-4 * day)})
	h := routedHarness(t, routerDocument(three()...))
	h.source.health = router.Health{OK: true, PID: 1, StartedAt: run}
	state := t.TempDir()
	writeLines(t, state, arrived("0a000001", "work", -time.Minute))
	onTheLog(h, state)
	h.model.cfg.Events = files
	h.start()
	logged := files.datesRead()
	h.model.view = dashboard.Runway
	h.typed("R")
	reach := dayfile.DateOf(views.EventsFrom(past(0)))
	for date, n := range files.reads {
		if want := 1 + btoi(date == reach); n != want {
			t.Errorf("the Log then Runway read %s's file %d times, want %d: %v, the Log's %v", date, n, want, files.reads, logged)
		}
	}
	if got, want := files.datesRead(), localDates(views.RunwayLatelyFrom(past(0)), past(0)); !slices.Equal(got, want) {
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
	read := views.ReadEventsFrom(filesOf(events.Line{Event: gone, Run: past(-8 * day)}), views.RunwayLatelyFrom(now), now)
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
