package watch

import (
	"reflect"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/views"
)

// graphsShowing has Graphs show the sections named alone.
func graphsShowing(names ...string) func(*runwayed, *Config) {
	return func(_ *runwayed, cfg *Config) { cfg.Graphs = dashboard.GraphsSectionsNamed(names) }
}

// While Graphs shows POOL, its stack is built once a message and never at a
// frame, of the readings history's live tail, read through when its last
// read was asked, across the terminal's columns, each from the last, as it
// holds what it judged; the frame shows it. With POOL hidden none is built,
// the last kept, and shown again it's built from that; on another tab, none
// is built.
func TestGraphsBuildsPoolsStackOnceAMessageWhileItShows(t *testing.T) {
	read := filedOn("work", "7d", start.Add(-2*time.Hour), 0.3)
	r := runwayHarness(t, dashboard.Graphs, graphsShowing("week", "pool"), read)
	from, live := r.poolFrom, r.model.runwayTab.live
	if r.poolBuilds == 0 || from.Columns != 160 || !slices.Equal(from.Readings, []readings.Reading{read}) || !from.ReadThrough.Equal(live.through) || live.through.IsZero() {
		t.Fatalf("once started, POOL's stack was built %d times, across %d columns, of %v read through %s; want it built across 160, of the live tail's %v, read through %s",
			r.poolBuilds, from.Columns, from.Readings, from.ReadThrough, read, live.through)
	}
	if shown := r.model.frame(r.model.now()).Graphs.Pool; shown.Timeline.Columns != 160 || len(shown.Accounts) != 3 {
		t.Errorf("the frame shows POOL's stack across %d columns, of %d accounts; want 160, of the three", shown.Timeline.Columns, len(shown.Accounts))
	}

	tests := []struct {
		name string
		msg  func(r *runwayed) tea.Msg
		want int
	}{
		{name: "a frame", msg: func(r *runwayed) tea.Msg { return frameMsg{gen: r.model.frames} }},
		{name: "a message taken in", msg: func(*runwayed) tea.Msg { return nudgeMsg{} }, want: 1},
		{name: "a read landing a document", msg: func(*runwayed) tea.Msg { return fetchedMsg{doc: routerDocument(three()...)} }, want: 1},
		{name: "a read of the history landing", msg: func(r *runwayed) tea.Msg { return runwayReadMsg{span: r.model.runwayTab.live.span} }, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runwayHarness(t, dashboard.Graphs, graphsShowing("pool"))
			built, last := r.poolBuilds, r.poolBuilt
			r.update(tt.msg(r))
			if got := r.poolBuilds - built; got != tt.want {
				t.Errorf("%s built POOL's stack %d times, want %d", tt.name, got, tt.want)
			}
			if tt.want > 0 && !reflect.DeepEqual(r.poolGiven, last) {
				t.Errorf("%s built POOL's stack from %+v, want from the last built, %+v", tt.name, r.poolGiven, last)
			}
		})
	}

	held := r.poolBuilt
	r.keyed("o")
	built := r.poolBuilds
	r.lookAt(start.Add(interval))
	if r.poolBuilds != built || !reflect.DeepEqual(r.model.graphsTab.pool, held) {
		t.Errorf("POOL hidden, a look built its stack %d times, holding %+v; want none built, the last kept", r.poolBuilds-built, r.model.graphsTab.pool)
	}
	r.keyed("o")
	if r.poolBuilds != built+1 || !reflect.DeepEqual(r.poolGiven, held) {
		t.Errorf("POOL shown again, its stack was built %d times, from %+v; want once, from the one kept", r.poolBuilds-built, r.poolGiven)
	}

	for _, view := range []dashboard.View{dashboard.Overview, dashboard.Runway} {
		other := runwayHarness(t, view, graphsShowing("pool"))
		other.update(nudgeMsg{})
		if other.poolBuilds != 0 {
			t.Errorf("with %s shown, POOL's stack was built %d times, want none", view, other.poolBuilds)
		}
	}
}

// POOL alone shown, Graphs has the readings history's live tail read as it
// opens and again at each look, as the lanes have it read; with neither
// POOL nor a burn-down section shown, nothing reads it.
func TestGraphsReadsTheReadingsHistoryForPoolAlone(t *testing.T) {
	read := filedOn("work", "7d", start.Add(-2*time.Hour), 0.3)
	r := runwayHarness(t, dashboard.Graphs, graphsShowing("pool"), read)
	if len(r.filed.starts) != 1 || r.filed.tails == 0 || !slices.Equal(r.poolFrom.Readings, []readings.Reading{read}) {
		t.Fatalf("POOL alone shown, %d tails were made, read %d times, its stack built of %v; want one made and read, of %v", len(r.filed.starts), r.filed.tails, r.poolFrom.Readings, read)
	}
	tails := r.filed.tails
	r.lookAt(start.Add(interval))
	if r.filed.tails != tails+1 {
		t.Errorf("at a look, the live tail was read %d more times, want once", r.filed.tails-tails)
	}

	none := runwayHarness(t, dashboard.Graphs, graphsShowing("glance"), read)
	none.lookAt(start.Add(interval))
	if none.filed.tails != 0 || none.poolBuilds != 0 {
		t.Errorf("with GLANCE alone shown, the live tail was read %d times, POOL's stack built %d; want neither", none.filed.tails, none.poolBuilds)
	}
}

// POOL's stack is built across the terminal's columns as it resizes, as
// the page lays POOL out across its width.
func TestGraphsBuildsPoolsStackAcrossTheTerminalsColumns(t *testing.T) {
	r := runwayHarness(t, dashboard.Graphs, graphsShowing("pool"))
	for _, width := range []int{173, 73, 52} {
		r.update(tea.WindowSizeMsg{Width: width, Height: 40})
		if got, want := r.poolFrom.Columns, dashboard.GraphsPoolColumns(width); got != want || got != width {
			t.Errorf("at %d columns, POOL's stack was built across %d, want %d", width, got, want)
		}
		if shown := r.model.frame(r.model.now()).Graphs.Pool; shown.Timeline != views.LiveTimeline(views.RunwayWeek, r.model.now(), width) {
			t.Errorf("at %d columns, the frame shows POOL's stack over %+v, want Runway's week's live timeline across them", width, shown.Timeline)
		}
	}
}
