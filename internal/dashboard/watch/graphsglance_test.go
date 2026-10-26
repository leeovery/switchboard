package watch

import (
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// glanceKeys are the keys of the windows GLANCE draws, in the policy of the
// harness's moment, in Runway's order.
func glanceKeys(r *runwayed) []string {
	policy := r.model.moment(r.model.now()).Policy()
	var keys []string
	for _, w := range views.RunwayWindows() {
		keys = append(keys, w.KeyIn(policy))
	}
	return keys
}

// spansBuilt are how many times the pool's room over each of GLANCE's
// windows' spans has been built, by the window's key.
func spansBuilt(r *runwayed) map[string]int {
	return maps.Clone(r.spanBuilds)
}

// While Graphs shows GLANCE, the pool's room over each window's span is built
// once a message and never at a frame, of the readings history's live tail,
// read through when its last read was asked, across GLANCE's sparklines'
// columns, each from the last, as it holds what it judged; the frame shows
// them. With GLANCE hidden none is built, the last kept, and shown again
// each is built from that; on another tab, none is built.
func TestGraphsBuildsGlancesPoolRoomOnceAMessageWhileItShows(t *testing.T) {
	read := filedOn("work", "7d", start.Add(-2*time.Hour), 0.3)
	r := runwayHarness(t, dashboard.Graphs, graphsShowing("glance"), read)
	live, shown := r.model.runwayTab.live, r.model.frame(r.model.now()).Graphs.Spans
	if r.filed.tails == 0 || live.through.IsZero() {
		t.Fatalf("GLANCE alone shown, the live tail was read %d times, through %s; want it read", r.filed.tails, live.through)
	}
	for i, key := range glanceKeys(r) {
		from := r.spanFrom[key]
		if r.spanBuilds[key] == 0 || from.Columns != dashboard.GraphsGlanceColumns || !slices.Equal(from.Readings, []readings.Reading{read}) || !from.ReadThrough.Equal(live.through) {
			t.Errorf("once started, the pool's room in %s was built %d times, across %d columns, of %v read through %s; want it built across %d, of the live tail's %v, read through %s",
				key, r.spanBuilds[key], from.Columns, from.Readings, from.ReadThrough, dashboard.GraphsGlanceColumns, read, live.through)
		}
		if w := views.RunwayWindows()[i]; !reflect.DeepEqual(shown[w], r.spanBuilt[key]) || len(shown[w].Columns) != dashboard.GraphsGlanceColumns {
			t.Errorf("the frame shows the pool's room in %s as %+v, want the %d columns built, %+v", key, shown[w], dashboard.GraphsGlanceColumns, r.spanBuilt[key])
		}
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
			r := runwayHarness(t, dashboard.Graphs, graphsShowing("glance"))
			built, last := spansBuilt(r), maps.Clone(r.spanBuilt)
			r.update(tt.msg(r))
			for _, key := range glanceKeys(r) {
				if got := r.spanBuilds[key] - built[key]; got != tt.want {
					t.Errorf("%s built the pool's room in %s %d times, want %d", tt.name, key, got, tt.want)
				}
				if tt.want > 0 && !reflect.DeepEqual(r.spanGiven[key], last[key]) {
					t.Errorf("%s built the pool's room in %s from %+v, want from the last built, %+v", tt.name, key, r.spanGiven[key], last[key])
				}
			}
		})
	}

	toggled := runwayHarness(t, dashboard.Graphs, graphsShowing("week", "glance"), read)
	held := maps.Clone(toggled.model.graphsTab.spans)
	toggled.keyed("g")
	built := spansBuilt(toggled)
	toggled.lookAt(start.Add(interval))
	if !maps.Equal(toggled.spanBuilds, built) || !reflect.DeepEqual(toggled.model.graphsTab.spans, held) {
		t.Errorf("GLANCE hidden, a look built the pool's room %v times, holding %+v; want none built, the last kept, %v times", toggled.spanBuilds, toggled.model.graphsTab.spans, built)
	}
	toggled.keyed("g")
	for i, key := range glanceKeys(toggled) {
		if toggled.spanBuilds[key] != built[key]+1 || !reflect.DeepEqual(toggled.spanGiven[key], held[views.RunwayWindows()[i]]) {
			t.Errorf("GLANCE shown again, the pool's room in %s was built %d times, from %+v; want once, from the one kept", key, toggled.spanBuilds[key]-built[key], toggled.spanGiven[key])
		}
	}

	for _, view := range []dashboard.View{dashboard.Overview, dashboard.Runway} {
		other := runwayHarness(t, view, graphsShowing("glance"))
		other.update(nudgeMsg{})
		if len(other.spanBuilds) != 0 {
			t.Errorf("with %s shown, the pool's room was built %v times, want none", view, other.spanBuilds)
		}
	}
}

// Under a session's page opened over Graphs, GLANCE alone shown, nothing
// reads the live tail, and neither the pool's room nor any lanes are built at
// a look; closed, Graphs builds the pool's room again from what it held.
func TestGraphsUnderASessionsPageBuildsNoGlance(t *testing.T) {
	r := runwayHarness(t, dashboard.Graphs, graphsShowing("glance"), filedOn("work", "7d", start.Add(-2*time.Hour), 0.3))
	r.source.sessions = []status.Session{sessionOn(idD28C, opus, "work", 0)}
	r.tickUntilAsked()
	r.openPage(idD28C)
	held, built, lanes, tails := maps.Clone(r.spanBuilt), spansBuilt(r), maps.Clone(r.graphsBuilds), r.filed.tails
	r.lookAt(r.clock.now.Add(interval))
	if r.filed.tails != tails || !maps.Equal(r.spanBuilds, built) || !maps.Equal(r.graphsBuilds, lanes) {
		t.Errorf("under a session's page, a look read the live tail %d times, built the pool's room %v times and the lanes %v; want none, as %v and %v",
			r.filed.tails-tails, r.spanBuilds, r.graphsBuilds, built, lanes)
	}
	r.update(escKey)
	if r.model.session != nil {
		t.Fatal("esc pressed, the session's page is still open")
	}
	for _, key := range glanceKeys(r) {
		if r.spanBuilds[key] != built[key]+1 || !reflect.DeepEqual(r.spanGiven[key], held[key]) {
			t.Errorf("the page closed, the pool's room in %s was built %d times, from %+v; want once, from the one held, %+v", key, r.spanBuilds[key]-built[key], r.spanGiven[key], held[key])
		}
	}
}
