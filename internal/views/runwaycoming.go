package views

import "github.com/leeovery/switchboard/internal/status"

// RunwayComing is what Runway's COMING UP lists of what's coming up at the
// moment m, in its order: each account back from every request, never from a
// model's own window, which its lane doesn't draw; and the run-outs and
// resets of the windows every model shares. A prime, which no lane draws, it
// leaves out.
func RunwayComing(m status.Moment) []status.Upcoming {
	var coming []status.Upcoming
	for _, u := range m.ComingUp {
		if runwayLists(m, u) {
			coming = append(coming, u)
		}
	}
	return coming
}

// runwayLists reports whether Runway's COMING UP lists u, coming up at the
// moment m, as RunwayComing has it.
func runwayLists(m status.Moment, u status.Upcoming) bool {
	switch u.Kind {
	case status.UpcomingBack:
		return u.Window == ""
	case status.UpcomingRunsOut, status.UpcomingReset:
		return m.Policy().IsShared(u.Window)
	}
	return false
}
