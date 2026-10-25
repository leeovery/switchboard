package watch

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/views"
)

// While Graphs shows, each window's lanes are built over Runway's live view
// of it, across the columns Runway's lanes take on the terminal, once a
// message and never at a frame, and the frame shows them; Runway's own are
// built only while Runway shows, and neither while another tab does.
func TestGraphsBuildsEachWindowsLanesOnceAMessageWhileItShows(t *testing.T) {
	r := runwayHarness(t, dashboard.Graphs, nil)
	shown := r.model.frame(r.model.now()).Graphs.Lanes
	for _, w := range views.RunwayWindows() {
		want := views.LiveTimeline(w, start, 146)
		if r.graphsBuilds[w] == 0 || r.graphsFrom[w].Timeline != want || r.graphsFrom[w].Window != w {
			t.Errorf("once started, Graphs built the %q lanes %d times, over %+v; want them built over Runway's live view, %+v", w, r.graphsBuilds[w], r.graphsFrom[w].Timeline, want)
		}
		if got := shown[w]; got.Lanes == nil || got.Window != w || got.Timeline != want || len(got.Lanes.Accounts) != 3 {
			t.Errorf("the frame shows the %q lanes as %+v, want each account's, over %+v", w, got, want)
		}
	}
	if r.builds != 0 {
		t.Errorf("with Graphs shown, Runway's lanes were built %d times, want none", r.builds)
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
		{name: "a read landing a document", msg: func(*runwayed) tea.Msg { return fetchedMsg{doc: busier} }, want: 1},
		{name: "a read of the history landing", msg: func(r *runwayed) tea.Msg { return runwayReadMsg{span: r.model.runwayTab.live.span} }, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runwayHarness(t, dashboard.Graphs, nil)
			built := maps.Clone(r.graphsBuilds)
			r.update(tt.msg(r))
			for _, w := range views.RunwayWindows() {
				if got := r.graphsBuilds[w] - built[w]; got != tt.want {
					t.Errorf("%s built the %q lanes %d times, want %d", tt.name, w, got, tt.want)
				}
			}
		})
	}

	r.update(fetchedMsg{doc: busier})
	if lanes := r.model.graphsTab.shown[views.RunwayFiveHour].Lanes; lanes == nil || lanes.Accounts[0].Use.Used != 0.6 {
		t.Errorf("a read landing work's 5-hour window at 60%% left Graphs' 5-hour lanes %+v, want them built of it", lanes)
	}

	for _, view := range []dashboard.View{dashboard.Runway, dashboard.History} {
		other := runwayHarness(t, view, nil)
		other.update(nudgeMsg{})
		if len(other.graphsBuilds) != 0 || other.model.graphsTab.shown != nil {
			t.Errorf("with %s shown, Graphs' lanes were built %v, shown %v; want none", view, other.graphsBuilds, other.model.graphsTab.shown)
		}
	}
}

// While Graphs shows, the readings history's live tail is read as it opens,
// and again at each look for what it gained, and Graphs' lanes are judged by
// what it holds; and live, by the ledger's days where they reach back before
// the first day the readings history holds, a week before each timeline's
// first day to its last, as Runway's are.
func TestGraphsLanesAreJudgedByTheReadingsHistoryAndTheLedgersDaysAsRunways(t *testing.T) {
	inSpan := filedOn("work", "five_hour", start.Add(-time.Hour), 0.3)
	r := runwayHarness(t, dashboard.Graphs, nil, inSpan)
	if len(r.filed.starts) != 1 || r.filed.tails == 0 {
		t.Fatalf("as Graphs opens, %d tails were made and read %d times, want one, read", len(r.filed.starts), r.filed.tails)
	}
	held := r.model.runwayTab.live.held
	for _, w := range views.RunwayWindows() {
		if !slices.Equal(held, []readings.Reading{inSpan}) || !slices.Equal(r.graphsFrom[w].Readings, held) {
			t.Errorf("the tail holds %v, the %q lanes built of %v; want the reading in its span, both", held, w, r.graphsFrom[w].Readings)
		}
		if r.graphsFrom[w].Days != nil {
			t.Errorf("live, the readings history holding every day, the %q lanes were built of the days %v, want none", w, r.graphsFrom[w].Days)
		}
	}
	read := r.filed.tails
	r.lookAt(start.Add(interval))
	if len(r.filed.starts) != 1 || r.filed.tails != read+1 {
		t.Errorf("at a look, %d tails were made and read %d times in all, want the one read again, %d times", len(r.filed.starts), r.filed.tails, read+1)
	}

	kept := dayfile.DayStart(start, -3)
	keptLater := runwayHarness(t, dashboard.Graphs, func(r *runwayed, _ *Config) { r.filed.first = kept })
	firsts := map[views.RunwayWindow]time.Time{views.RunwayWeek: civilDay(-8), views.RunwayFiveHour: civilDay(-7)}
	for w, first := range firsts {
		from := keptLater.graphsFrom[w]
		if got, want := summaryDates(from.Days), datesFrom(first, civilDay(0)); !from.Kept.Equal(kept) || !slices.Equal(got, want) {
			t.Errorf("live, the readings history holding days from %s, the %q lanes were built of the days %q, kept from %s; want %q, kept from its first day", kept, w, got, from.Kept, want)
		}
	}
	yearFrom := views.YearFrom(views.YearAsk{}, start, time.Sunday)
	if !slices.EqualFunc(keptLater.ledger.reads, []time.Time{yearFrom}, time.Time.Equal) {
		t.Errorf("the ledger was read from %v, want once, from the 12 months' first day, %s, as History reads it", keptLater.ledger.reads, yearFrom)
	}
}

// Graphs, waiting on a read of the readings history LoadingAfter or longer,
// says loading… on its first section's heading till the read lands; a read
// landing sooner says nothing of it.
func TestGraphsSaysItsLoadingWhileItsReadOfTheReadingsHistoryTakesAWhile(t *testing.T) {
	tests := []struct {
		name string
		late bool
	}{
		{name: "its read landing late", late: true},
		{name: "its read landing soon"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.model.view = dashboard.Graphs
			unread := holdingBack[runwayReadMsg](h, h.init()...)
			loading, ok := h.pending(loadingMsg{})
			if len(unread) == 0 || !ok {
				t.Fatalf("the readings history's read held back %d, its loading armed %t; want the read held, its loading armed", len(unread), ok)
			}
			if got := h.row(4); strings.Contains(got, loadingSaid) {
				t.Errorf("its read just asked for, the heading reads %q, want nothing of loading", got)
			}
			if !tt.late {
				h.deliver(unread...)
			}
			h.fire(loading)
			if got, want := h.row(4), tt.late; strings.HasPrefix(got, "WEEK   "+loadingSaid) != want {
				t.Errorf("%s on, the heading reads %q; want it saying loading… after WEEK %t", LoadingAfter, got, want)
			}
			h.deliver(unread...)
			if got := h.row(4); strings.Contains(got, loadingSaid) {
				t.Errorf("its read landed, the heading reads %q, want nothing of loading", got)
			}
		})
	}
}
