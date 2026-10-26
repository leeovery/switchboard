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
	"github.com/leeovery/switchboard/internal/status"
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
			t.Errorf("once started, Graphs built the %s lanes %d times, over %+v; want them built over Runway's live view, %+v", windowName(w), r.graphsBuilds[w], r.graphsFrom[w].Timeline, want)
		}
		if got := shown[w]; got.Lanes == nil || got.Window != w || got.Timeline != want || len(got.Lanes.Accounts) != 3 {
			t.Errorf("the frame shows the %s lanes as %+v, want each account's, over %+v", windowName(w), got, want)
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
					t.Errorf("%s built the %s lanes %d times, want %d", tt.name, windowName(w), got, tt.want)
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

// Graphs reads each window's lanes over Runway's own live view of it at the
// terminal's width: across the columns Runway's lanes take, its account
// column collapsed or not as Runway shows it, sized for the accounts' labels,
// long ones among them; and live, whatever Runway was stepped back to before
// Graphs showed.
func TestGraphsLanesAreOverRunwaysOwnLiveViewOfEachWindow(t *testing.T) {
	long := three()
	long[0].Label, long[2].Label = "someone@example.com", "a-much-longer-team-account"
	collapsed := func(_ *runwayed, cfg *Config) { cfg.RunwayShown.Collapsed = true }
	tests := []struct {
		name  string
		doc   status.Document
		setup func(*runwayed, *Config)
		// back are the keys Runway is stepped back by before tab shows Graphs.
		back string
		// moved is set where Runway's lanes take other columns than those of
		// the short labels, expanded, 146, so the case tells a timeline sized
		// otherwise from Runway's.
		moved bool
	}{
		{name: "short labels, expanded", doc: routerDocument(three()...)},
		{name: "collapsed", doc: routerDocument(three()...), setup: collapsed, moved: true},
		{name: "long labels", doc: routerDocument(long...), moved: true},
		{name: "long labels, collapsed", doc: routerDocument(long...), setup: collapsed, moved: true},
		{name: "after Runway stepped back a day", doc: routerDocument(long...), back: "left left", moved: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := runwayHarnessOf(t, tt.doc, dashboard.Graphs, tt.setup)
			if tt.back != "" {
				g = runwayHarnessOf(t, tt.doc, dashboard.Runway, tt.setup)
				g.keyed(tt.back)
				g.deliver(g.update(tabKey)...)
			}
			for _, w := range views.RunwayWindows() {
				runway := runwayHarnessOf(t, tt.doc, dashboard.Runway, func(r *runwayed, cfg *Config) {
					if tt.setup != nil {
						tt.setup(r, cfg)
					}
					cfg.RunwayShown.Window = w
				})
				got, want := g.graphsFrom[w].Timeline, runway.from.Timeline
				if got != want {
					t.Errorf("Graphs' %s lanes are over %+v, want Runway's own live view of it, %+v", windowName(w), got, want)
				}
				if moved := want.Columns != 146; moved != tt.moved {
					t.Errorf("Runway's %s lanes take %d columns, the case meaning them moved from 146 %t", windowName(w), want.Columns, tt.moved)
				}
			}
		})
	}
}

// Graphs builds the lanes of the windows its burn-down sections shown chart
// alone, WEEK's the week's and 5-HOUR's the 5-hour's, and reads the readings
// history only while one shows: with neither, nothing is built or read, at
// a look as at the start.
func TestGraphsBuildsAndReadsTheLanesOfTheBurnDownSectionsShownAlone(t *testing.T) {
	tests := []struct {
		name   string
		hidden []dashboard.GraphsSection
		want   []views.RunwayWindow
	}{
		{name: "both shown", want: []views.RunwayWindow{views.RunwayWeek, views.RunwayFiveHour}},
		{name: "WEEK hidden", hidden: []dashboard.GraphsSection{dashboard.GraphsWeek}, want: []views.RunwayWindow{views.RunwayFiveHour}},
		{name: "5-HOUR hidden", hidden: []dashboard.GraphsSection{dashboard.GraphsFiveHour}, want: []views.RunwayWindow{views.RunwayWeek}},
		{name: "both hidden", hidden: []dashboard.GraphsSection{dashboard.GraphsWeek, dashboard.GraphsFiveHour}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runwayHarness(t, dashboard.Graphs, func(_ *runwayed, cfg *Config) {
				for _, s := range tt.hidden {
					cfg.Graphs = cfg.Graphs.Toggled(s)
				}
			}, filedOn("work", "five_hour", start.Add(-time.Hour), 0.3))
			r.lookAt(start.Add(interval))
			built := slices.Sorted(maps.Keys(r.graphsBuilds))
			shown := slices.Sorted(maps.Keys(r.model.graphsTab.shown))
			want := slices.Sorted(slices.Values(tt.want))
			if !slices.Equal(built, want) || !slices.Equal(shown, want) {
				t.Errorf("Graphs built the lanes of %v, shows those of %v, want %v alone", windowNames(built), windowNames(shown), windowNames(want))
			}
			if read := r.filed.tails > 0; read != (len(tt.want) > 0) {
				t.Errorf("the readings history's tail was made %d times and read %d, want it read %t", len(r.filed.starts), r.filed.tails, len(tt.want) > 0)
			}
		})
	}
}

// A burn-down section toggled while Graphs shows has the lanes of the
// windows shown after it built, and those alone shown, none kept of a window
// no longer shown.
func TestGraphsBuildsTheLanesOfTheBurnDownSectionsShownAsTheyreToggled(t *testing.T) {
	r := runwayHarness(t, dashboard.Graphs, nil)
	steps := []struct {
		key  string
		want []views.RunwayWindow
	}{
		{key: "f", want: []views.RunwayWindow{views.RunwayWeek}},
		{key: "w"},
		{key: "f", want: []views.RunwayWindow{views.RunwayFiveHour}},
		{key: "w", want: []views.RunwayWindow{views.RunwayWeek, views.RunwayFiveHour}},
	}
	for _, step := range steps {
		was := maps.Clone(r.graphsBuilds)
		r.keyed(step.key)
		var built []views.RunwayWindow
		for _, w := range views.RunwayWindows() {
			if r.graphsBuilds[w] > was[w] {
				built = append(built, w)
			}
		}
		shown := slices.Sorted(maps.Keys(r.model.graphsTab.shown))
		want := slices.Sorted(slices.Values(step.want))
		if !slices.Equal(slices.Sorted(slices.Values(built)), want) || !slices.Equal(shown, want) {
			t.Fatalf("%s pressed, Graphs built the lanes of %v, shows those of %v, want %v alone", step.key, windowNames(built), windowNames(shown), windowNames(want))
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
			t.Errorf("the tail holds %v, the %s lanes built of %v; want the reading in its span, both", held, windowName(w), r.graphsFrom[w].Readings)
		}
		if r.graphsFrom[w].Days != nil {
			t.Errorf("live, the readings history holding every day, the %s lanes were built of the days %v, want none", windowName(w), r.graphsFrom[w].Days)
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
			t.Errorf("live, the readings history holding days from %s, the %s lanes were built of the days %q, kept from %s; want %q, kept from its first day", kept, windowName(w), got, from.Kept, want)
		}
	}
	yearFrom := views.YearFrom(views.YearAsk{}, start, time.Sunday)
	if !slices.EqualFunc(keptLater.ledger.reads, []time.Time{yearFrom}, time.Time.Equal) {
		t.Errorf("the ledger was read from %v, want once, from the 12 months' first day, %s, as History reads it", keptLater.ledger.reads, yearFrom)
	}
}

// Graphs, waiting on a read of the readings history LoadingAfter or longer,
// as its burn-downs or POOL read it, says loading… on its first section's
// heading till the read lands; a read landing sooner says nothing of it.
func TestGraphsSaysItsLoadingWhileItsReadOfTheReadingsHistoryTakesAWhile(t *testing.T) {
	tests := []struct {
		name  string
		shown dashboard.GraphsSections
		first string
		late  bool
	}{
		{name: "its read landing late", first: "WEEK", late: true},
		{name: "its read landing soon", first: "WEEK"},
		{name: "POOL alone, its read landing late", shown: dashboard.GraphsSectionsNamed([]string{"pool"}), first: "POOL", late: true},
		{name: "POOL alone, its read landing soon", shown: dashboard.GraphsSectionsNamed([]string{"pool"}), first: "POOL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.model.view, h.model.graphs = dashboard.Graphs, tt.shown
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
			if got, want := h.row(4), tt.late; strings.HasPrefix(got, tt.first+"   "+loadingSaid) != want {
				t.Errorf("%s on, the heading reads %q; want it saying loading… after %s %t", LoadingAfter, got, tt.first, want)
			}
			h.deliver(unread...)
			if got := h.row(4); strings.Contains(got, loadingSaid) {
				t.Errorf("its read landed, the heading reads %q, want nothing of loading", got)
			}
		})
	}
}

// Graphs with both burn-down sections and POOL hidden waits on no read of
// the readings history, so its first heading, GLANCE's here, says nothing of
// loading while a read of the live tail asked before they were hidden is
// under way, however long it takes.
func TestGraphsWithItsBurnDownsHiddenSaysNothingOfLoading(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.model.view = dashboard.Graphs
	unread := holdingBack[runwayReadMsg](h, h.init()...)
	if len(unread) == 0 || !h.model.runwayTab.readingLive {
		t.Fatalf("the live tail's read held back %d, of the live tail %t; want it held", len(unread), h.model.runwayTab.readingLive)
	}
	for _, key := range []string{"w", "f", "g"} {
		unread = append(unread, holdingBack[runwayReadMsg](h, h.update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})...)...)
	}
	h.clock.now = h.clock.now.Add(2 * LoadingAfter)
	if loading, ok := h.pending(loadingMsg{}); ok {
		h.fire(loading)
	}
	h.deliver(h.update(nudgeMsg{})...)
	if tab := h.model.runwayTab; !tab.reading || !tab.readingLive {
		t.Fatalf("the heading read with the live tail's read under way %t, of the live tail %t; want it under way", tab.reading, tab.readingLive)
	}
	if got := h.row(4); !strings.HasPrefix(got, "GLANCE") || strings.Contains(got, loadingSaid) {
		t.Errorf("WEEK, 5-HOUR and POOL hidden, the live tail's read under way, the heading reads %q, want GLANCE's, saying nothing of loading", got)
	}
	h.deliver(unread...)
}

// Graphs shown while the read of a span Runway was stepped back to is under
// way waits on none of its own, so says nothing of loading, however long that
// read takes.
func TestGraphsSaysNothingOfLoadingWhileRunwaysSteppedBackReadIsUnderWay(t *testing.T) {
	h := runwayHarness(t, dashboard.Runway, func(_ *runwayed, cfg *Config) { cfg.RunwayShown.Window = views.RunwayWeek })
	h.keyed("left")
	back := holdingBack[runwayReadMsg](h.harness, h.update(namedKeys["left"])...)
	if len(back) != 1 || h.model.runwayTab.readingLive {
		t.Fatalf("stepped back to last week, %d reads held back, of the live tail %t; want the week's own read held", len(back), h.model.runwayTab.readingLive)
	}
	more := holdingBack[runwayReadMsg](h.harness, h.update(tabKey)...)
	if h.model.view != dashboard.Graphs || len(more) != 0 {
		t.Fatalf("tab pressed, Graphs shows %t, %d more reads asked; want Graphs shown, waiting on the read under way", h.model.view == dashboard.Graphs, len(more))
	}
	h.clock.now = h.clock.now.Add(2 * LoadingAfter)
	if loading, ok := h.pending(loadingMsg{}); ok {
		h.fire(loading)
	}
	h.deliver(h.update(nudgeMsg{})...)
	if got := h.row(4); strings.Contains(got, loadingSaid) {
		t.Errorf("Runway's read of last week under way, Graphs' heading reads %q, want nothing of loading", got)
	}
	h.deliver(back...)
}

// windowName is the window as Runway's heading names it, as "week".
func windowName(w views.RunwayWindow) string {
	if w == views.RunwayWeek {
		return "week"
	}
	return string(w)
}

// windowNames are the windows given, each as windowName names it.
func windowNames(windows []views.RunwayWindow) []string {
	names := make([]string, len(windows))
	for i, w := range windows {
		names[i] = windowName(w)
	}
	return names
}
