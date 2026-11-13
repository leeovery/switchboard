package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// OpenAgain is what LATELY says befell an account with room again, its short
// form whole: the fewest words Runway's LATELY says of an account, as its
// least is reckoned by.
const OpenAgain = "open again"

// runwayKinds are the kinds of events Runway's LATELY tells of: what befell
// an account, a cap, a limit, a refusal, coming under pressure, and room
// again.
var runwayKinds = []string{status.EventCap, status.EventLimit, status.EventRefused, status.EventPressure, status.EventRoom}

// RunwayLatelyFrom is when the events Runway's LATELY tells at now begin: a
// week before now, as long as the live week's span.
func RunwayLatelyFrom(now time.Time) time.Time {
	return now.Add(-weekSpan)
}

// RunwayLately are the router's events held, oldest first, as EventsRead's
// Events gives them, as Runway's LATELY tells them at the Now of the moment
// m, the newest first: those of the kinds runwayKinds names, from
// RunwayLatelyFrom on, each told in full and short as Lately tells it, the
// sessions a cap, a limit or a refusal moved folded into its line, counted.
// A session's start, or its move alone, never shows.
func RunwayLately(m status.Moment, held []Event) []Sentence {
	since := RunwayLatelyFrom(m.Now)
	befell := side{
		tells: func(e status.Event) bool { return slices.Contains(runwayKinds, e.Kind) && !e.At.Before(since) },
		tell:  overview.tell,
	}
	newest := slices.Clone(held)
	slices.Reverse(newest)
	return toldFrom(m, newest, len(newest), befell, SessionDirs{})
}
