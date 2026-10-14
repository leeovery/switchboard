package capture

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// resumed is the auto world as session-resumed-160 draws it: f41a's lines of
// yesterday and today among the sessions' world's, and the router listing it,
// on personal, an answer streaming to it, in 9c2e's place, so it lists six
// sessions, as the status line counts them.
func (a accountWorlds) resumed() accountWorld {
	return a.kept("resumed", func() accountWorld {
		w := a.auto()
		w.routed, w.lines = routedResumed, resumedLines
		return w
	})
}

// resumedLines are the sessions' world's lines, and f41a's, oldest first.
func resumedLines(now time.Time) []ledger.Line {
	lines := slices.Concat(sessionsLines(now), resumedPlan(now).lines())
	slices.SortStableFunc(lines, func(a, b ledger.Line) int { return a.At.Compare(b.At) })
	return lines
}
