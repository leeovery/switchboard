package watch

import (
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// graphsTab is what the watch holds of Graphs as a look builds it: its
// lanes, which its cards say what Runway says of each account by, and WHEN
// YOU WORK's page. Of the lanes: Runway's lanes over its live view of
// each window a burn-down section shown charts, by the window, each holding
// what it judged of the past till what that's judged from changes, as
// Runway's do; what the frame shows of them, as the last look built them;
// how a look builds them, as lanesLooking has it. WHEN YOU WORK's page, as
// the last look on Graphs built it, held off the tab, where nothing draws
// it. POOL's stack, held look to look, as it holds what it judged of the
// past till what that's judged from changes, and while POOL hides, for when
// it shows again; and how a look builds it, as poolLooking has it. The
// pool's room over each window's span GLANCE draws, by the window, held so
// too; and how a look builds it, as spanLooking has it. And the moment of
// the message they were built at, as each is built once.
type graphsTab struct {
	lanes    map[views.RunwayWindow]views.Lanes
	shown    map[views.RunwayWindow]dashboard.WindowLanes
	look     func(views.Lanes, views.LanesFrom) views.Lanes
	hours    *views.HoursPage
	pool     views.PoolStack
	poolLook func(views.PoolStack, views.PoolStackFrom) views.PoolStack
	spans    map[views.RunwayWindow]views.PoolSpan
	spanLook func(views.PoolSpan, views.PoolRoomFrom) views.PoolSpan
	builtAt  *status.HeldMoment
}

// graphsBuilt is m with Graphs' lanes built afresh at the moment of the
// message taken in, while it shows with no session's page over it, once a
// moment: of each window it reads, as graphsWindows has them, none where
// neither a burn-down section nor GLANCE shows, over Runway's live view of
// it, as graphsTimeline has it, as views.Lanes builds them from the last
// look's, of the readings history's live tail and the readings the watch read itself, and the
// ledger's days they're judged by, as Runway's are; POOL's stack, while it
// shows, of the same readings, as graphsPool builds it; the pool's room over
// each window's span, while GLANCE shows, of them too, as graphsSpans builds
// it; and WHEN YOU WORK's page, as graphsHours has it.
func (m Model) graphsBuilt() Model {
	t := m.graphsTab
	if m.view != dashboard.Graphs || m.session != nil || t.builtAt != nil && t.builtAt == m.taking.moment {
		return m
	}
	now := m.now()
	mo, live, r := m.moment(now), liveSpan(now), m.runwayTab
	read, look := r.readingsOf(live, live), lanesLooking(t.look)
	lanes := make(map[views.RunwayWindow]views.Lanes)
	shown := make(map[views.RunwayWindow]dashboard.WindowLanes)
	for _, w := range m.graphsWindows() {
		tl := m.graphsTimeline(w, mo, now)
		built := look(t.lanes[w], views.LanesFrom{
			Moment: mo, Readings: read, Days: m.runwayDays(tl, now), Kept: r.kept, Seen: r.seen.from(), Timeline: tl, Window: w,
		})
		lanes[w], shown[w] = built, dashboard.WindowLanes{Window: w, Timeline: tl, Lanes: &built}
	}
	if m.graphs.Has(dashboard.GraphsPool) {
		t.pool = m.graphsPool(t, mo, read)
	}
	if m.graphs.Has(dashboard.GraphsGlance) {
		t.spans = m.graphsSpans(t, mo, read)
	}
	t.lanes, t.shown, t.hours, t.builtAt = lanes, shown, m.graphsHours(), m.taking.moment
	m.graphsTab = t
	return m
}

// graphsPool is POOL's stack at the moment mo, built from t's last, as its
// poolLooking builds it, of the readings given, the live tail's and those the
// watch read itself, read through when the live tail's last read was asked,
// across the columns POOL takes on the terminal, as
// dashboard.GraphsPoolColumns has them.
func (m Model) graphsPool(t graphsTab, mo status.Moment, read []readings.Reading) views.PoolStack {
	return poolLooking(t.poolLook)(t.pool, views.PoolStackFrom{
		Moment: mo, Readings: read, ReadThrough: m.runwayTab.live.through, Columns: dashboard.GraphsPoolColumns(m.size.Width),
	})
}

// graphsSpans is the pool's room over each window's span GLANCE draws at the
// moment mo, by the window, each built from t's last, as its spanLooking
// builds it, of the readings given, read through when the live tail's last
// read was asked, across GLANCE's sparklines' columns, as
// dashboard.GraphsGlanceColumns has them.
func (m Model) graphsSpans(t graphsTab, mo status.Moment, read []readings.Reading) map[views.RunwayWindow]views.PoolSpan {
	look := spanLooking(t.spanLook)
	spans := make(map[views.RunwayWindow]views.PoolSpan, len(views.RunwayWindows()))
	for _, w := range views.RunwayWindows() {
		spans[w] = look(t.spans[w], views.PoolRoomFrom{
			Moment: mo, Readings: read, ReadThrough: m.runwayTab.live.through, Key: w.KeyIn(mo.Policy()), Columns: dashboard.GraphsGlanceColumns,
		})
	}
	return spans
}

// graphsShows reports whether Graphs shows the section, with no session's
// page over it.
func (m Model) graphsShows(s dashboard.GraphsSection) bool {
	return m.view == dashboard.Graphs && m.session == nil && m.graphs.Has(s)
}

// graphsReadsHistory reports whether Graphs draws of the readings history's
// live tail as it shows: its lanes, as graphsWindows has them, POOL, or
// GLANCE's pool's room.
func (m Model) graphsReadsHistory() bool {
	return len(m.graphsWindows()) > 0 || m.graphsShows(dashboard.GraphsPool) || m.graphsShows(dashboard.GraphsGlance)
}

// graphsTimeline is the timeline of Runway's live view of the window w at
// now, as Runway would draw it on the terminal drawn at, its account column
// as Runway shows it, its labels sized for the accounts of the moment mo:
// so what's said under each lane is what Runway says, as its stretches are
// placed in its columns.
func (m Model) graphsTimeline(w views.RunwayWindow, mo status.Moment, now time.Time) views.Timeline {
	return m.runwayTab.shown.LiveOf(w).Labelled(mo).TimelineAt(now, m.cfg.WeekStarts, m.size.Width)
}

// graphsWindows are the windows whose lanes Graphs reads, a burn-down
// section's shown each and GLANCE's, as dashboard.GraphsWindows has them,
// while it shows with no session's page over it: none else.
func (m Model) graphsWindows() []views.RunwayWindow {
	if m.view != dashboard.Graphs || m.session != nil {
		return nil
	}
	return dashboard.GraphsWindows(m.graphs)
}

// lanesBuilt is m with the lanes of the tab shown built at the moment of the
// message taken in, as runwayBuilt builds Runway's and graphsBuilt Graphs'.
func (m Model) lanesBuilt() Model {
	return m.runwayBuilt().graphsBuilt()
}

// lanesShown reports whether the tab shown draws of what's judged by the
// readings history as lanes are, with no session's page over it: Runway, or
// Graphs with a burn-down section, POOL or GLANCE shown, as
// graphsReadsHistory has it.
func (m Model) lanesShown() bool {
	return m.view == dashboard.Runway && m.session == nil || m.graphsReadsHistory()
}

// lanesTimelines are the timelines of the lanes the tab shown builds at now,
// as lanesShown has it: Runway's, as it shows it, or Graphs', Runway's live
// view of each window it reads, as graphsWindows has them, across no
// columns, as what lanes want of the ledger is the same at any width; none
// where none shows.
func (m Model) lanesTimelines(now time.Time) []views.Timeline {
	if m.view == dashboard.Runway && m.session == nil {
		return []views.Timeline{m.runwayTimeline(now)}
	}
	var timelines []views.Timeline
	for _, w := range m.graphsWindows() {
		timelines = append(timelines, views.LiveTimeline(w, now, 0))
	}
	return timelines
}

// lanesLooking is how lanes are built at a look: by look, where it's given,
// as a test counts the builds through it; else as views.Lanes.Look builds
// them.
func lanesLooking(look func(views.Lanes, views.LanesFrom) views.Lanes) func(views.Lanes, views.LanesFrom) views.Lanes {
	if look == nil {
		return views.Lanes.Look
	}
	return look
}

// poolLooking is how POOL's stack is built at a look: by look, where it's
// given, as a test counts the builds through it; else as views.PoolStack.Look
// builds it.
func poolLooking(look func(views.PoolStack, views.PoolStackFrom) views.PoolStack) func(views.PoolStack, views.PoolStackFrom) views.PoolStack {
	if look == nil {
		return views.PoolStack.Look
	}
	return look
}

// spanLooking is how the pool's room over a window's span is built at a
// look: by look, where it's given, as a test counts the builds through it;
// else as views.PoolSpan.Look builds it.
func spanLooking(look func(views.PoolSpan, views.PoolRoomFrom) views.PoolSpan) func(views.PoolSpan, views.PoolRoomFrom) views.PoolSpan {
	if look == nil {
		return views.PoolSpan.Look
	}
	return look
}
