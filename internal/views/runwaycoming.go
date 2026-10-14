package views

import (
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// RunwayComing is what Runway's COMING UP lists in the view of the window w,
// of what's coming up at the moment m, in its order: each account back from
// every request, never from a model's own window, which its lane doesn't
// draw; and the run-outs and resets of the windows every model shares that
// the view draws, the 5-hour view's those of a day or shorter, the week
// view's all of them. A prime, which no lane draws, it leaves out.
func RunwayComing(m status.Moment, w RunwayWindow) []status.Upcoming {
	var coming []status.Upcoming
	for _, u := range m.ComingUp {
		if runwayLists(m, u, w) {
			coming = append(coming, u)
		}
	}
	return coming
}

// runwayLists reports whether Runway's COMING UP lists u, coming up at the
// moment m, in the view of the window w, as RunwayComing has it.
func runwayLists(m status.Moment, u status.Upcoming, w RunwayWindow) bool {
	switch u.Kind {
	case status.UpcomingBack:
		return u.Window == ""
	case status.UpcomingRunsOut, status.UpcomingReset:
		return m.Policy().IsShared(u.Window) && (w == RunwayWeek || !quota.MultiDay(u.Window))
	}
	return false
}
