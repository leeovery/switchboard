package watch

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayed is a harness of Runway: its readings history, its ledger, and
// how many times it built Runway's lanes, and what it built them from last;
// and of Graphs, by the window, how many times it built Graphs' lanes, and
// what it built them from last.
type runwayed struct {
	*harness
	filed        *filedReadings
	ledger       *daysLedger
	builds       int
	from         views.LanesFrom
	graphsBuilds map[views.RunwayWindow]int
	graphsFrom   map[views.RunwayWindow]views.LanesFrom
}

// runwayHarness is the router of three, on the tab given, 160 cells wide and
// 37 tall, its readings history filed holding the readings given, its ledger
// holding the days from 2024 on, as historyHarness's does, each of no
// requests but as the test puts them there, set up as setup has it, where
// it's given, Runway kept on the 5-hour, which > steps on from to the week;
// it counts Runway's lanes and Graphs' as they're built. It's started.
func runwayHarness(t *testing.T, view dashboard.View, setup func(*runwayed, *Config), held ...readings.Reading) *runwayed {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
	r := &runwayed{
		harness: h, filed: &filedReadings{held: held},
		graphsBuilds: make(map[views.RunwayWindow]int), graphsFrom: make(map[views.RunwayWindow]views.LanesFrom),
	}
	r.ledger = &daysLedger{Empty: h.emptyLedger(), clock: h.clock, first: "2024-06-03", days: map[string]ledger.Summary{}}
	cfg := h.config(Size{Width: 160, Height: 37})
	cfg.Readings, cfg.Ledger, cfg.View = r.filed, r.ledger, view
	cfg.RunwayShown = dashboard.RunwayShown{Window: views.RunwayFiveHour}
	if setup != nil {
		setup(r, &cfg)
	}
	h.model = New(t.Context(), cfg)
	h.model.runwayTab.look, h.model.graphsTab.look = r.counted, r.countedForGraphs
	h.start()
	return r
}

// counted builds Runway's lanes as views.Lanes.Look does, counting each
// build, and holding what it was built from.
func (r *runwayed) counted(l views.Lanes, from views.LanesFrom) views.Lanes {
	r.builds++
	r.from = from
	return l.Look(from)
}

// countedForGraphs builds Graphs' lanes as views.Lanes.Look does, counting
// each build of each window's, and holding what it was built from.
func (r *runwayed) countedForGraphs(l views.Lanes, from views.LanesFrom) views.Lanes {
	r.graphsBuilds[from.Window]++
	r.graphsFrom[from.Window] = from
	return l.Look(from)
}

// weeksFrom has weeks start on the day given.
func weeksFrom(day time.Weekday) func(*runwayed, *Config) {
	return func(_ *runwayed, cfg *Config) { cfg.WeekStarts = day }
}

// civilDay is the day the clock starts on, moved by the days given, as
// dayfile.Civil gives one.
func civilDay(days int) time.Time {
	return time.Date(start.Year(), start.Month(), start.Day()+days, 0, 0, 0, 0, time.UTC)
}

// < and > show the other window's live view, which the preferences keep; ←
// and → step back and on, which they never keep.
func TestRunwaysWindowIsKeptAsItChangesAndAStepBackNever(t *testing.T) {
	r := runwayHarness(t, dashboard.Runway, nil)
	prefs := &fakePrefs{}
	r.model.cfg.Prefs = prefs
	steps := []struct {
		key     string
		window  views.RunwayWindow
		back    bool
		updates int
	}{
		{key: ">", window: views.RunwayWeek, updates: 1},
		{key: "left", window: views.RunwayWeek, back: true, updates: 1},
		{key: "right", window: views.RunwayWeek, updates: 1},
		{key: "left", window: views.RunwayWeek, back: true, updates: 1},
		{key: "<", window: views.RunwayFiveHour, updates: 2},
		{key: "<", window: views.RunwayWeek, updates: 3},
	}
	for _, step := range steps {
		r.keyed(step.key)
		shown := r.model.runwayTab.shown
		if shown.Window != step.window || shown.Back.IsZero() == step.back || prefs.kept.Runway != shown.Kept() || prefs.updates != step.updates {
			t.Fatalf("after %s, Runway shows %+v, the preferences keep %+v, updated %d times; want %q stepped back %t, kept, %d times",
				step.key, shown, prefs.kept.Runway, prefs.updates, step.window, step.back, step.updates)
		}
	}
	if prefs.kept.Runway != (theme.RunwayPrefs{}) {
		t.Errorf("the preferences keep %+v, want the week", prefs.kept.Runway)
	}
}

// a collapses Runway's account column and expands it again, keeping each,
// whatever the view or step shows, its lanes built afresh over the columns
// its labels give up and given back: 155 at 160 collapsed, 146 expanded.
func TestACollapsesRunwaysAccountColumnAndKeepsIt(t *testing.T) {
	r := runwayHarness(t, dashboard.Runway, nil)
	prefs := &fakePrefs{}
	r.model.cfg.Prefs = prefs
	steps := []struct {
		keys      string
		collapsed bool
		back      bool
		columns   int
		updates   int
	}{
		{keys: "a", collapsed: true, columns: 155, updates: 1},
		{keys: "left", collapsed: true, back: true, columns: 155, updates: 1},
		{keys: "a", back: true, columns: 146, updates: 2},
		{keys: "right a", collapsed: true, columns: 155, updates: 3},
	}
	for _, step := range steps {
		r.keyed(step.keys)
		shown := r.model.runwayTab.shown
		if shown.Collapsed != step.collapsed || shown.Back.IsZero() == step.back || prefs.kept.Runway != shown.Kept() || prefs.updates != step.updates {
			t.Fatalf("after %s, Runway shows %+v, the preferences keep %+v, updated %d times; want collapsed %t, stepped back %t, kept, %d times",
				step.keys, shown, prefs.kept.Runway, prefs.updates, step.collapsed, step.back, step.updates)
		}
		if got := r.from.Timeline.Columns; got != step.columns {
			t.Errorf("after %s, Runway's lanes are built over %d columns, want %d", step.keys, got, step.columns)
		}
	}
	if want := (theme.RunwayPrefs{Window: "5-hour", Collapsed: true}); prefs.kept.Runway != want {
		t.Errorf("the preferences keep %+v, want %+v", prefs.kept.Runway, want)
	}
}

// Runway opens with its account column as the preferences kept it: expanded,
// as with nothing kept, or collapsed.
func TestRunwayOpensWithItsAccountColumnAsKept(t *testing.T) {
	for _, collapsed := range []bool{false, true} {
		r := runwayHarness(t, dashboard.Runway, func(_ *runwayed, cfg *Config) {
			cfg.RunwayShown = dashboard.RunwayKept(theme.RunwayPrefs{Collapsed: collapsed})
		})
		columns := map[bool]int{false: 146, true: 155}[collapsed]
		if shown := r.model.runwayTab.shown; shown.Collapsed != collapsed || r.from.Timeline.Columns != columns {
			t.Errorf("kept collapsed %t, Runway opens collapsed %t over %d columns, want %d", collapsed, shown.Collapsed, r.from.Timeline.Columns, columns)
		}
	}
}

// With nothing kept, Runway opens on the week's live view.
func TestRunwayOpensOnTheWeek(t *testing.T) {
	r := runwayHarness(t, dashboard.Runway, func(_ *runwayed, cfg *Config) { cfg.RunwayShown = dashboard.RunwayKept(theme.RunwayPrefs{}) })
	if shown := r.model.runwayTab.shown; shown.Window != views.RunwayWeek || !shown.Back.IsZero() {
		t.Errorf("Runway opens on %+v, want the week's live view", shown)
	}
	if r.from.Timeline != views.LiveTimeline(views.RunwayWeek, start, 146) {
		t.Errorf("its lanes are built over %+v, want the week's live view", r.from.Timeline)
	}
}

func TestRunwaysKeysAreTakenOnRunwayAlone(t *testing.T) {
	for _, view := range []dashboard.View{dashboard.Overview, dashboard.Accounts, dashboard.History, dashboard.Sessions, dashboard.Log} {
		t.Run(view.Title(), func(t *testing.T) {
			r := runwayHarness(t, view, nil)
			r.keyed("> left a")
			if shown := r.model.runwayTab.shown; shown.Window != views.RunwayFiveHour || !shown.Back.IsZero() || shown.Collapsed {
				t.Errorf(">, ← and a on %s leave Runway showing %+v, want its live 5-hour view, expanded, as it was", view.Title(), shown)
			}
		})
	}
}

// ← steps back as far as the day summaries reach, the ledger's first day, a
// day at a time in the 5-hour view and a week at a time in the week's, to
// the week holding it; today and this week always, as the readings history's
// live tail draws them, even where the ledger doesn't say where it began.
func TestLeftStepsBackNoFurtherThanTheLedgersFirstDay(t *testing.T) {
	beginning := func(first string) func(*runwayed, *Config) {
		return func(r *runwayed, _ *Config) { r.ledger.first = first }
	}
	noBeginning := func(r *runwayed, cfg *Config) { cfg.Ledger = r.emptyLedger() }
	tests := []struct {
		name   string
		window string
		setup  func(*runwayed, *Config)
		want   []time.Time
	}{
		{name: "the 5-hour view", setup: beginning("2026-09-26"), want: []time.Time{civilDay(0), civilDay(-1), civilDay(-2), civilDay(-2)}},
		{name: "the week view", window: ">", setup: beginning("2026-09-26"), want: []time.Time{civilDay(-1), civilDay(-8), civilDay(-8)}},
		{name: "the week view, the first day a week's first", window: ">", setup: beginning("2026-09-20"), want: []time.Time{civilDay(-1), civilDay(-8), civilDay(-8)}},
		{name: "the week view, the first day a week's last", window: ">", setup: beginning("2026-09-19"), want: []time.Time{civilDay(-1), civilDay(-8), civilDay(-15), civilDay(-15)}},
		{name: "a ledger that doesn't say where it began", setup: noBeginning, want: []time.Time{civilDay(0), civilDay(0)}},
		{name: "its week view", window: ">", setup: noBeginning, want: []time.Time{civilDay(-1), civilDay(-1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runwayHarness(t, dashboard.Runway, tt.setup)
			r.keyed(tt.window)
			for i, want := range tt.want {
				r.keyed("left")
				if got := r.model.runwayTab.shown.Back; !got.Equal(want) {
					t.Fatalf("after ← %d times, Runway shows %s, want %s", i+1, got.Format(time.DateOnly), want.Format(time.DateOnly))
				}
			}
		})
	}
}

// Runway's lanes are built once at each message but a frame, which takes
// nothing in: over the live view of the window shown, across the lanes'
// columns; and a read landing a document builds them as it lands, of that
// document, for the numbers its look compares to be drawn of them.
func TestRunwaysLanesAreBuiltOnceAMessageAndNeverAtAFrame(t *testing.T) {
	r := runwayHarness(t, dashboard.Runway, nil)
	if shown := r.model.runwayTab.shown; r.builds == 0 || shown.Lanes == nil || r.from.Timeline != views.LiveTimeline(views.RunwayFiveHour, start, 146) {
		t.Fatalf("once started, Runway built lanes %d times, shows %v, over %+v; want them built over the live 5-hour view across 146 columns", r.builds, shown.Lanes, r.from.Timeline)
	}
	busier := routerDocument(three()...)
	busier.Accounts[0].Windows[0].Utilization = 0.6
	tests := []struct {
		name string
		msg  func(r *runwayed) tea.Msg
		want int
	}{
		{name: "a frame", msg: func(r *runwayed) tea.Msg { return frameMsg{gen: r.model.frames} }},
		{name: "a message taken in", msg: func(*runwayed) tea.Msg { return nudgeMsg{} }, want: 1},
		{name: "a key", msg: func(*runwayed) tea.Msg { return tea.KeyPressMsg{Code: '>', Text: ">"} }, want: 1},
		{name: "a read landing a document", msg: func(*runwayed) tea.Msg { return fetchedMsg{doc: busier} }, want: 1},
		{name: "a read of the history landing", msg: func(r *runwayed) tea.Msg { return runwayReadMsg{span: r.model.runwayTab.live.span} }, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runwayHarness(t, dashboard.Runway, nil)
			built := r.builds
			r.update(tt.msg(r))
			if got := r.builds - built; got != tt.want {
				t.Errorf("%s built Runway's lanes %d times, want %d", tt.name, got, tt.want)
			}
		})
	}

	r.update(fetchedMsg{doc: busier})
	if lanes := r.model.runwayTab.shown.Lanes; lanes == nil || lanes.Accounts[0].Use.Used != 0.6 {
		t.Errorf("a read landing work's 5-hour window at 60%% left Runway's lanes %+v, want them built of it", lanes)
	}
}

// The readings history's live tail is made once, read as Runway opens, and
// read again at each look for what it gained, never at a frame; while
// another tab shows, it's never read.
func TestTheReadingsHistorysTailIsReadAtEachLookWhileRunwayShows(t *testing.T) {
	inSpan := filedOn("work", "five_hour", start.Add(-time.Hour), 0.3)
	tooOld := filedOn("work", "five_hour", start.AddDate(0, 0, -20), 0.1)
	r := runwayHarness(t, dashboard.Runway, nil, tooOld, inSpan)
	if len(r.filed.starts) != 1 || r.filed.tails == 0 {
		t.Fatalf("as Runway opens, %d tails were made and read %d times, want one, read", len(r.filed.starts), r.filed.tails)
	}
	if held := r.model.runwayTab.live.held; !slices.Equal(held, []readings.Reading{inSpan}) || !slices.Equal(r.from.Readings, held) {
		t.Errorf("the tail holds %v, the lanes built of %v, want the reading in its span alone", held, r.from.Readings)
	}

	read := r.filed.tails
	r.deliver(r.update(frameMsg{gen: r.model.frames})...)
	if r.filed.tails != read {
		t.Errorf("a frame read the tail, %d reads in all, want %d", r.filed.tails, read)
	}
	r.lookAt(start.Add(interval))
	if len(r.filed.starts) != 1 || r.filed.tails != read+1 {
		t.Errorf("at a look, %d tails were made and read %d times in all, want the one read again, %d times", len(r.filed.starts), r.filed.tails, read+1)
	}

	other := runwayHarness(t, dashboard.History, nil, inSpan)
	other.lookAt(start.Add(interval))
	if len(other.filed.starts) != 0 || other.filed.tails != 0 {
		t.Errorf("with History shown, %d tails were made and read %d times, want none", len(other.filed.starts), other.filed.tails)
	}
}

// The live tail spans from the day a week before today: the live views,
// today, each day of the week before and this week are read of it alone,
// today's readings never twice. A day or a week before it is read once, a
// tail of its own from its first day, and held: stepping back to it again
// reads nothing.
func TestADayOrAWeekSteppedBackToIsReadOnceAndHeld(t *testing.T) {
	weekBack, sunday, friday := dayfile.DayStart(start, -7), dayfile.DayStart(start, -8), dayfile.DayStart(start, -10)
	tests := []struct {
		keys   string
		starts []time.Time
		tails  int
	}{
		{keys: "left left left left left left left left", starts: []time.Time{weekBack}},
		{keys: "left", starts: []time.Time{weekBack, sunday}, tails: 1},
		{keys: "right left", starts: []time.Time{weekBack, sunday}},
		{keys: "> left", starts: []time.Time{weekBack, sunday}},
		{keys: "left", starts: []time.Time{weekBack, sunday, friday}, tails: 1},
		{keys: "right right left left", starts: []time.Time{weekBack, sunday, friday}},
	}
	r := runwayHarness(t, dashboard.Runway, weeksFrom(time.Friday))
	for _, tt := range tests {
		tails := r.filed.tails
		r.keyed(tt.keys)
		if !slices.EqualFunc(r.filed.starts, tt.starts, time.Time.Equal) || r.filed.tails != tails+tt.tails {
			t.Fatalf("after %s, showing %+v, the tails made start %v and were read %d times more, want %v, read %d times more",
				tt.keys, r.model.runwayTab.shown, r.filed.starts, r.filed.tails-tails, tt.starts, tt.tails)
		}
	}
}

// Each live view, today and this week is read of the live tail, whatever the
// clocks did: the week's live view, from 24 hours before now, starts before
// yesterday's midnight in the hour after the clocks go forward, and the live
// tail starts no later.
func TestEachLiveViewTodayAndThisWeekAreReadOfTheLiveTail(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatalf("load Europe/London: %v", err)
	}
	nows := []time.Time{
		start,
		time.Date(2026, time.March, 30, 0, 30, 0, 0, london),
		time.Date(2026, time.March, 30, 0, 50, 0, 0, london),
		time.Date(2026, time.March, 30, 1, 5, 0, 0, london),
		time.Date(2026, time.October, 26, 0, 30, 0, 0, london),
	}
	for _, now := range nows {
		for _, starts := range []time.Weekday{time.Sunday, time.Monday} {
			live := liveSpan(now)
			timelines := map[string]views.Timeline{
				"the live 5-hour": views.LiveTimeline(views.RunwayFiveHour, now, 136),
				"the live week":   views.LiveTimeline(views.RunwayWeek, now, 136),
				"today":           views.Stepped(views.RunwayFiveHour, civilOf(now), now, starts, 136),
				"this week":       views.Stepped(views.RunwayWeek, civilOf(now), now, starts, 136),
			}
			for name, tl := range timelines {
				if s := spanOf(tl, live); !s.is(live) {
					t.Errorf("at %s, weeks starting %s, %s from %s is read of a span of its own from %s, want the live tail's, from %s",
						now.Format(time.RFC3339), starts, name, tl.Start.Format(time.RFC3339), s.start.Format(time.RFC3339), live.start.Format(time.RFC3339))
				}
			}
		}
	}
}

// The live tail reaches both a week back from now, where the pool's week
// chart starts, and the start of the day a week before now's, where the live
// views do, starting at the latest start of a day that reaches both, whatever
// the clocks did: in the hour after midnight for a week after they go
// forward, a week back from now falls on the day before the one a week
// before now's, and the tail starts there; for a week after they go back, it
// can fall on the day after, and the tail starts at the day a week before.
func TestTheLiveTailReachesAWeekBackFromNowWhateverTheClocksDid(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load America/New_York: %v", err)
	}
	nows := []time.Time{
		time.Date(2026, time.March, 9, 0, 30, 0, 0, newYork),
		time.Date(2026, time.March, 14, 0, 59, 0, 0, newYork),
		time.Date(2026, time.March, 9, 1, 0, 0, 0, newYork),
		time.Date(2026, time.March, 15, 0, 30, 0, 0, newYork),
		time.Date(2026, time.November, 2, 23, 30, 0, 0, newYork),
		time.Date(2026, time.November, 5, 12, 0, 0, 0, newYork),
		start,
	}
	for _, now := range nows {
		live := liveSpan(now)
		weekBack, dayBefore := now.Add(-7*quota.Day), dayfile.DayStart(now, -7)
		if live.start.After(weekBack) || live.start.After(dayBefore) {
			t.Errorf("at %s, the live tail starts at %s, want it to reach a week back, %s, and the day a week before, %s",
				now.Format(time.RFC3339), live.start.Format(time.RFC3339), weekBack.Format(time.RFC3339), dayBefore.Format(time.RFC3339))
		}
		if next := dayfile.DayStart(live.start, 1); !next.After(weekBack) && !next.After(dayBefore) {
			t.Errorf("at %s, the live tail starts at %s, want it a day later, %s, which reaches a week back, %s, and the day a week before, %s",
				now.Format(time.RFC3339), live.start.Format(time.RFC3339), next.Format(time.RFC3339), weekBack.Format(time.RFC3339), dayBefore.Format(time.RFC3339))
		}
	}
}

// As the day turns, the live tail is made afresh over the span from the day
// a week before the new today, and holds what it read till its read lands,
// so the lanes never go blank as it's read.
func TestTheLiveTailIsMadeAfreshAsTheDayTurns(t *testing.T) {
	inSpan := filedOn("work", "5h", start.Add(-time.Hour), 0.3)
	r := runwayHarness(t, dashboard.Runway, nil, inSpan)
	tomorrow := dayfile.DayStart(start, 1).Add(time.Minute)

	pending := r.lastTick()
	r.clock.now, pending.fired = tomorrow, true
	msgs := r.update(pending.msg)
	if tail := r.model.runwayTab.live; !tail.span.is(liveSpan(tomorrow)) || !slices.Equal(tail.held, []readings.Reading{inSpan}) {
		t.Fatalf("as the day turned, the live tail spans %+v holding %v, want it made afresh over %+v, holding what it read till its read lands", tail.span, tail.held, liveSpan(tomorrow))
	}
	r.deliver(msgs...)
	if len(r.filed.starts) != 2 || !r.filed.starts[1].Equal(dayfile.DayStart(start, -6)) {
		t.Errorf("as the day turned, the tails made start %v, want a second from the day a week before tomorrow", r.filed.starts)
	}
}

// Stepped back, the lanes are judged by the ledger's days, from a week
// before the first shown to the last, as History's reads hold them, the
// readings history's live tail joining them; and live, by none, but where
// they reach back before the first day the readings history holds, as its
// tail last said, which a day before is drawn from its summary.
func TestTheLanesAreJudgedByTheLedgersDaysSteppedBackOrBeforeTheReadingsKeep(t *testing.T) {
	r := runwayHarness(t, dashboard.Runway, nil)
	if r.from.Days != nil || !r.from.Kept.IsZero() || len(r.ledger.reads) != 0 {
		t.Fatalf("live, the lanes were built of the days %v, kept from %s, the ledger read %d times; want none, none and none", r.from.Days, r.from.Kept, len(r.ledger.reads))
	}

	r.keyed("left left")
	if got, want := summaryDates(r.from.Days), datesFrom(civilDay(-8), civilDay(-1)); !slices.Equal(got, want) {
		t.Errorf("stepped back to yesterday, the lanes were built of the days %q, want %q", got, want)
	}
	yearFrom := views.YearFrom(views.YearAsk{}, start, time.Sunday)
	if !slices.EqualFunc(r.ledger.reads, []time.Time{yearFrom}, time.Time.Equal) {
		t.Errorf("stepped back, the ledger was read from %v, want once, from the 12 months' first day, %s, as History reads it", r.ledger.reads, yearFrom)
	}
	r.lookAt(start.Add(interval))
	if len(r.ledger.reads) != 2 || !r.ledger.reads[1].Equal(dayfile.DayStart(start.Local(), 0)) {
		t.Errorf("at a look stepped back, the ledger was read from %v, want today alone read again", r.ledger.reads)
	}

	kept := dayfile.DayStart(start, -3)
	keptLater := runwayHarness(t, dashboard.Runway, func(r *runwayed, _ *Config) { r.filed.first = kept })
	if got, want := summaryDates(keptLater.from.Days), datesFrom(civilDay(-7), civilDay(0)); !keptLater.from.Kept.Equal(kept) || !slices.Equal(got, want) {
		t.Errorf("live, the readings history holding days from %s, the lanes were built of the days %q, kept from %s; want %q, kept from its first day", kept, got, keptLater.from.Kept, want)
	}
}

// flakyKept is a readings history's files that fail to say where they
// begin the first fails times they're asked, as of too many files open,
// then say it as the files they hold do; asked counts the times.
type flakyKept struct {
	*filedReadings
	fails, asked int
}

func (f *flakyKept) FirstDay() (time.Time, error) {
	f.asked++
	if f.asked <= f.fails {
		return time.Time{}, errors.New("too many open files")
	}
	return f.filedReadings.FirstDay()
}

// A failure to say where the readings history begins is passing: each look
// after it asks again, till the history says, the failures logged the once;
// then no look asks again till the live span moves.
func TestAReadingsHistoryThatFailsToSayWhereItBeginsIsAskedAgainTillItSays(t *testing.T) {
	log := logstest.Capture(t)
	kept := dayfile.DayStart(start, -3)
	var flaky *flakyKept
	r := runwayHarness(t, dashboard.Runway, func(r *runwayed, cfg *Config) {
		r.filed.first = kept
		flaky = &flakyKept{filedReadings: r.filed, fails: 2}
		cfg.Readings = flaky
	})
	if !r.model.runwayTab.kept.IsZero() {
		t.Fatalf("the readings history failed to say where it begins, but it's taken as %s", r.model.runwayTab.kept)
	}

	for i := range 3 {
		r.lookAt(start.Add(time.Duration(i+1) * interval))
	}
	if !r.model.runwayTab.kept.Equal(kept) || flaky.asked != 3 {
		t.Errorf("three looks on, the history is taken to begin %s, asked %d times; want %s, asked till it said, 3 times", r.model.runwayTab.kept, flaky.asked, kept)
	}
	if got := strings.Count(log.String(), "couldn't read where the readings history begins"); got != 1 {
		t.Errorf("the log reads\n%s\nwant one warning that the history couldn't say where it begins, of two failures", log)
	}
}

// A look at today stepped back to that reads today's summary afresh builds
// the lanes once as it lands, of it, its numbers drawn before and after.
func TestALookReadingTodaysSummaryBuildsTheLanesOfIt(t *testing.T) {
	today := dayfile.DateOf(start)
	r := runwayHarness(t, dashboard.Runway, nil)
	r.ledger.days[today] = usedAt(today, 0.3)
	r.keyed("left")
	if use := r.model.runwayTab.shown.Lanes.Accounts[0].Use; use.Used != 0.3 || !use.Highest {
		t.Fatalf("today stepped back to, work's use under its name is %+v, want today's highest, 30%%", use)
	}

	r.ledger.days[today] = usedAt(today, 0.5)
	pending := r.lastTick()
	r.clock.now, pending.fired = start.Add(interval), true
	fetched := r.pick(r.update(pending.msg), isFetched)
	since := r.pick(r.update(fetched), isSince)
	built := r.builds
	r.update(since)
	if use := r.model.runwayTab.shown.Lanes.Accounts[0].Use; r.builds-built != 1 || use.Used != 0.5 {
		t.Errorf("today's summary read afresh built the lanes %d times, work's use under its name %+v; want once, today's highest, 50%%", r.builds-built, use)
	}
}

// Probing, and while the router doesn't answer, the documents the watch
// reads join the readings history the lanes are judged by: of each account
// read since it was last joined, its windows as read; never the router's own,
// which it files in the history itself. The lanes know they begin at the
// first look joined, as nothing the watch read tells what came before it.
func TestTheDocumentsTheWatchProbesJoinTheReadingsTheLanesAreJudgedBy(t *testing.T) {
	r := runwayHarness(t, dashboard.Runway, func(r *runwayed, _ *Config) { r.stopRouter() })
	readAt := func(doc status.Document, at time.Time, ids ...string) status.Document {
		for i, a := range doc.Accounts {
			if slices.Contains(ids, a.ID) {
				doc.Accounts[i].FetchedAt = at.UTC()
			}
		}
		return doc
	}
	var seen []readings.Reading
	read := func(doc status.Document) {
		mo := doc.At(start, policy)
		for _, a := range doc.Accounts {
			if a.FetchedAt.After(lastReadOf(seen, a.ID)) {
				seen = append(seen, mo.ReadingsOf(a.ID)...)
			}
		}
	}
	read(probedWithoutTheRouter())
	if !slices.Equal(r.from.Readings, views.RunwayReadings(nil, seen)) {
		t.Fatalf("probed as it opened, the lanes were judged by %+v, want the three's windows as read", r.from.Readings)
	}
	first := slices.MinFunc(seen, func(a, b readings.Reading) int { return a.At.Compare(b.At) }).At
	if !r.from.Seen.Equal(first) {
		t.Errorf("probed as it opened, the lanes knew the watch's own readings begin at %s, want %s, its first look", r.from.Seen, first)
	}

	steps := []struct {
		name   string
		doc    status.Document
		router bool
	}{
		{name: "the same read again", doc: probedWithoutTheRouter()},
		{name: "work read anew", doc: readAt(probedWithoutTheRouter(), start.Add(interval), "work")},
		{name: "the router answering, every account read anew", doc: readAt(routerDocument(three()...), start.Add(2*interval), "work", "personal", "side"), router: true},
	}
	for i, step := range steps {
		if step.router {
			r.startRouter(step.doc)
		} else {
			r.source.doc = step.doc
			read(step.doc)
		}
		r.deliver(r.press("R")...)
		if want := views.RunwayReadings(nil, seen); !slices.Equal(r.from.Readings, want) {
			t.Errorf("after %d looks, %s, the lanes were judged by %d readings, want %d: %+v", i+1, step.name, len(r.from.Readings), len(want), r.from.Readings)
		}
		if !r.from.Seen.Equal(first) {
			t.Errorf("after %d looks, %s, the lanes knew the watch's own readings begin at %s, want %s, its first look", i+1, step.name, r.from.Seen, first)
		}
	}
}

// The readings the watch read itself are let go as the live tail is made
// afresh to start after them, as the day turns: no timeline it draws reaches
// back to them, and the lanes know of none.
func TestTheReadingsTheWatchReadItselfAreLetGoOnceTheLiveTailStartsAfterThem(t *testing.T) {
	r := runwayHarness(t, dashboard.Runway, func(r *runwayed, _ *Config) { r.stopRouter() })
	if len(r.from.Readings) == 0 {
		t.Fatal("probed as it opened, the lanes were judged by no reading, want the three's windows as read")
	}
	r.lookAt(start.AddDate(0, 0, 14))
	if len(r.from.Readings) == 0 {
		t.Fatalf("on the Monday after next, the live tail carrying in from the day they were read, the lanes were judged by no reading, want those read as Runway opened")
	}
	r.lookAt(start.AddDate(0, 0, 15))
	if len(r.from.Readings) > 0 || !r.from.Seen.IsZero() {
		t.Errorf("on the Tuesday after next, the live tail carrying in from after them, the lanes were judged by %+v, the watch's own from %s, want none", r.from.Readings, r.from.Seen)
	}
}

// Today stepped back to stays itself as the clock passes midnight, as it's
// kept by its day: it's yesterday's then, with no now, and drawn of the live
// tail made afresh, as the day turned, no tail of its own; this week stays
// itself alike, passing the week's last midnight to be a week gone, drawn of
// the live tail made afresh too, as it reaches back a week.
func TestADayOrWeekSteppedBackToStaysItselfAcrossMidnight(t *testing.T) {
	tests := []struct {
		name   string
		window string
		before time.Time
		first  time.Time
		last   time.Time
	}{
		{name: "today", before: at(23, 59, 30), first: civilDay(0), last: civilDay(0)},
		{name: "this week", window: ">", before: at(23, 59, 30).AddDate(0, 0, 5), first: civilDay(-1), last: civilDay(5)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runwayHarness(t, dashboard.Runway, nil)
			r.lookAt(tt.before)
			r.keyed(tt.window + " left")
			back := r.model.runwayTab.shown.Back
			made := len(r.filed.starts)

			midnight := dayfile.DayStart(tt.before, 1).Add(30 * time.Second)
			r.lookAt(midnight)
			first, last, named := r.from.Timeline.Named()
			if shown := r.model.runwayTab.shown; !shown.Back.Equal(back) || !named || !first.Equal(tt.first) || !last.Equal(tt.last) || !r.from.Timeline.Now.IsZero() {
				t.Errorf("past midnight, Runway shows %s, its lanes over %s to %s, now %s; want %s kept, over %s to %s, with no now",
					shown.Back.Format(time.DateOnly), first.Format(time.DateOnly), last.Format(time.DateOnly), r.from.Timeline.Now, back.Format(time.DateOnly), tt.first.Format(time.DateOnly), tt.last.Format(time.DateOnly))
			}
			renewed := r.model.runwayTab.live.span.is(liveSpan(midnight))
			if _, own := r.model.runwayTab.holds(spanOf(r.from.Timeline, liveSpan(midnight))); len(r.filed.starts) != made+1 || !renewed || own {
				t.Errorf("past midnight, %d tails were made, the live one renewed %t, one of the day or week's own held %t; want one made, the live one renewed, none of its own",
					len(r.filed.starts)-made, renewed, own)
			}
		})
	}
}

// A click does on Runway what its key does: on < or > by the window, it
// steps the window; on the key line's < > window, it steps it on, and on ←→,
// it steps on, as → does. Nothing else on Runway takes a click, its labels
// and lanes among it, as nothing on it is chosen.
func TestAClickOnRunwayDoesWhatItsKeyDoes(t *testing.T) {
	tests := []struct {
		name  string
		keys  string
		click func(dashboard.Target) bool
		want  dashboard.RunwayShown
	}{
		{name: "< by the window", click: onStep(-1), want: dashboard.RunwayShown{Window: views.RunwayWeek}},
		{name: "> by the window", keys: ">", click: onStep(1), want: dashboard.RunwayShown{Window: views.RunwayFiveHour}},
		{name: "the key line's < > window", click: onKey("< >"), want: dashboard.RunwayShown{Window: views.RunwayWeek}},
		{name: "the key line's ←→ day, stepped back", keys: "left left", click: onKey("←→"), want: dashboard.RunwayShown{Window: views.RunwayFiveHour, Back: civilDay(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runwayHarness(t, dashboard.Runway, nil)
			r.lookAt(start.Add(interval))
			r.keyed(tt.keys)
			r.clickWhere(tt.click)
			if got := r.model.runwayTab.shown; got.Window != tt.want.Window || !got.Back.Equal(tt.want.Back) {
				t.Errorf("the click left Runway showing %q back to %s, want %q back to %s", got.Window, got.Back, tt.want.Window, tt.want.Back)
			}
		})
	}

	r := runwayHarness(t, dashboard.Runway, nil)
	r.keyed("left")
	was, targets := r.model.runwayTab.shown, r.targets()
	for y := range r.model.size.Height {
		for _, x := range []int{0, 2, 12, 22, 24, 30, 80, 159} {
			if _, on := targetAt(targets, x, y); on {
				continue
			}
			if r.click(x, y) || r.model.view != dashboard.Runway || r.model.runwayTab.shown.Window != was.Window || !r.model.runwayTab.shown.Back.Equal(was.Back) || r.model.open != nothingOpen {
				t.Fatalf("a click at %d,%d, on no target, acted: showing %q, Runway %+v, open %v", x, y, r.model.view, r.model.runwayTab.shown, r.model.open)
			}
		}
	}
}

// pick is the message of those given that is, failing the test where none
// is.
func (r *runwayed) pick(msgs []tea.Msg, is func(tea.Msg) bool) tea.Msg {
	r.t.Helper()
	i := slices.IndexFunc(msgs, is)
	if i < 0 {
		r.t.Fatalf("no such message among %#v", msgs)
	}
	return msgs[i]
}

// isFetched and isSince report whether a message is a read of the source
// landing, and of the ledger's days since.
func isFetched(msg tea.Msg) bool { _, ok := msg.(fetchedMsg); return ok }
func isSince(msg tea.Msg) bool   { _, ok := msg.(sinceMsg); return ok }

// usedAt is a day of no requests, work's 5-hour window used at most as given.
func usedAt(date string, used float64) ledger.Summary {
	return ledger.Summary{Day: date, Accounts: []ledger.AccountDay{{Account: "work", Highest: map[string]float64{"5h": used}}}}
}

// summaryDates are the dates of the summaries given.
func summaryDates(days []ledger.Summary) []string {
	var dates []string
	for _, d := range days {
		dates = append(dates, d.Day)
	}
	return dates
}

// datesFrom are the dates of the days from first to last, each as
// dayfile.Civil gives one.
func datesFrom(first, last time.Time) []string {
	var dates []string
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		dates = append(dates, d.Format(time.DateOnly))
	}
	return dates
}

// lastReadOf is when the last of the readings given of the account with the
// given id was read: zero where none is.
func lastReadOf(read []readings.Reading, id string) time.Time {
	var last time.Time
	for _, r := range read {
		if r.Account == id && r.At.After(last) {
			last = r.At
		}
	}
	return last
}

// A look on Runway brightens each number it reads that it changed, as on
// every page: the use under the name of the account whose 5-hour window rose,
// and the pool's room; never another's that stood still.
func TestALookOnRunwayBrightensTheNumbersItChanged(t *testing.T) {
	r := runwayHarness(t, dashboard.Runway, nil)
	work := dashboard.Number{Kind: dashboard.NumberUsed, Account: "work", Of: "5h"}
	side := dashboard.Number{Kind: dashboard.NumberUsed, Account: "side", Of: "5h"}
	room := dashboard.Number{Kind: dashboard.NumberRoom, Of: "5h"}
	if r.drawnNumber(work) == "" || r.drawnNumber(side) == "" || r.drawnNumber(room) == "" {
		t.Fatalf("Runway draws work's use %q, side's %q and the pool's room %q, want each", r.drawnNumber(work), r.drawnNumber(side), r.drawnNumber(room))
	}
	doc := routerDocument(three()...)
	doc.Accounts[0].Windows[0].Utilization = 0.8
	r.startRouter(doc)
	r.deliver(r.press("R")...)
	if got := r.brightened(); got[work] != 1 || got[room] != 1 || got[side] != 0 {
		t.Errorf("a look that read work's 5-hour window at 80%% brightened %v, want work's use and the pool's room at 1, side's not", got)
	}
}

// A read of the ledger's days landing on today stepped back to draws today's
// highest afresh, and brightens it as it changed, as a number a look reads
// does: a use stepped back to is numbered as a live one is, and only
// today's changes.
func TestASinceReadOnRunwayBrightensTodaysHighestAsItChanges(t *testing.T) {
	today := dayfile.DateOf(start)
	r := runwayHarness(t, dashboard.Runway, nil)
	r.ledger.days[today] = usedAt(today, 0.3)
	r.keyed("left")

	r.ledger.days[today] = usedAt(today, 0.5)
	pending := r.lastTick()
	r.clock.now, pending.fired = start.Add(interval), true
	fetched := r.pick(r.update(pending.msg), isFetched)
	r.update(r.pick(r.update(fetched), isSince))
	if use := r.model.runwayTab.shown.Lanes.Accounts[0].Use; use.Used != 0.5 {
		t.Fatalf("today's summary read afresh left work's use under its name %+v, want today's highest, 50%%", use)
	}
	work := dashboard.Number{Kind: dashboard.NumberUsed, Account: "work", Of: "5h"}
	if got := r.brightened(); len(got) != 1 || got[work] != 1 {
		t.Errorf("a read of today's summary stepped back to brightened %v, want work's use alone, at 1", got)
	}
}

// Where Runway's lanes don't all fit, at the last step of its compaction, j
// and k scroll them an account at a time, PgUp and PgDn a page of them, and a
// turn of the wheel over them an account, never a wheel's rows: no further
// than the last lane, nor above the first.
func TestRunwaysLanesScrollAnAccountAtATimeByTheKeysAndTheWheel(t *testing.T) {
	h := runwayKeysHarness(t, twelve(), 160, "")
	h.deliver(h.update(scrolledSize)...)
	s := h.model.scrolling()
	if s.Most == 0 || s.Page == 0 {
		t.Fatalf("Runway's twelve lanes scroll %+v at 160x30, want some out of view", s)
	}
	x, y := 10, h.overTheView()
	steps := []struct {
		name   string
		scroll func()
		want   int
	}{
		{name: "j, an account down", scroll: func() { h.keyed("j") }, want: 1},
		{name: "k, an account up", scroll: func() { h.keyed("k") }, want: 0},
		{name: "k at the first, nowhere", scroll: func() { h.keyed("k") }, want: 0},
		{name: "PgDn, a page, no further than the last lane", scroll: func() { h.keys(tea.KeyPressMsg{Code: tea.KeyPgDown}) }, want: min(s.Page, s.Most)},
		{name: "j at the last lane, nowhere", scroll: func() { h.keyed("j") }, want: min(s.Page, s.Most)},
		{name: "PgUp, a page", scroll: func() { h.keys(tea.KeyPressMsg{Code: tea.KeyPgUp}) }, want: 0},
		{name: "a turn of the wheel down, an account", scroll: func() { h.wheel(tea.MouseWheelDown, x, y) }, want: 1},
		{name: "a turn up", scroll: func() { h.wheel(tea.MouseWheelUp, x, y) }, want: 0},
	}
	for _, step := range steps {
		step.scroll()
		if h.model.scroll != step.want {
			t.Errorf("%s: Runway's lanes scrolled %d accounts, want %d", step.name, h.model.scroll, step.want)
		}
	}
	h.keyed("j")
	if view := h.view(); strings.Contains(view, "Account A") || !strings.Contains(view, "Account B") {
		t.Errorf("scrolled an account, Runway reads\n%s\nwant Account A's lane out of view, Account B's first", view)
	}
}
