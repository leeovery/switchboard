package views

import (
	"slices"

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

// RunwayLately are the events of the document m is a moment of, as Runway's
// LATELY tells them at its Now, the newest first: those of the kinds
// runwayKinds names, from a week before Now, as far back as the live week's
// span, each told in full and short as Lately tells it, the sessions a cap,
// a limit or a refusal moved folded into its line, counted. A session's
// start, or its move alone, never shows.
func RunwayLately(m status.Moment) []Sentence {
	since := m.Now.Add(-weekSpan)
	befell := side{
		tells: func(e status.Event) bool { return slices.Contains(runwayKinds, e.Kind) && !e.At.Before(since) },
		tell:  overview.tell,
	}
	return toldFrom(m, len(m.Events), befell, SessionDirs{})
}
