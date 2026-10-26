package watch

import (
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// graphsTab is what the watch holds of Graphs' lanes, which its cards say
// what Runway says of each account by: Runway's lanes over its live view of
// each window a burn-down section shown charts, by the window, each holding
// what it judged of the past till what that's judged from changes, as
// Runway's do; what the frame shows of them, as the last look built them;
// how a look builds them, as lanesLooking has it; WHEN YOU WORK's page, as
// the last look built it; and the moment of the message they were built at,
// as each is built once.
type graphsTab struct {
	lanes   map[views.RunwayWindow]views.Lanes
	shown   map[views.RunwayWindow]dashboard.WindowLanes
	look    func(views.Lanes, views.LanesFrom) views.Lanes
	hours   *views.HoursPage
	builtAt *status.HeldMoment
}

// graphsBuilt is m with Graphs' lanes built afresh at the moment of the
// message taken in, while it shows with no session's page over it, once a
// moment: of each window it reads, as graphsWindows has them, none where no
// burn-down section shows, over Runway's live view of it, as graphsTimeline
// has it, as views.Lanes builds them from the last look's, of the readings
// history's live tail and the readings the watch read itself, and the
// ledger's days they're judged by, as Runway's are; and WHEN YOU WORK's
// page, as graphsHours has it.
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
	t.lanes, t.shown, t.hours, t.builtAt = lanes, shown, m.graphsHours(), m.taking.moment
	m.graphsTab = t
	return m
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
// section's shown each, as dashboard.GraphsWindows has them, while it shows
// with no session's page over it: none else.
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

// lanesShown reports whether the tab shown draws of lanes judged by the
// readings history, with no session's page over it: Runway, or Graphs with
// a burn-down section shown, as graphsWindows has them.
func (m Model) lanesShown() bool {
	return m.view == dashboard.Runway && m.session == nil || len(m.graphsWindows()) > 0
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
