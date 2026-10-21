package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
)

// latestFitting are where the latest of count runs, each cells wide and gap
// apart, lie in room cells, as flex lays them: as many as fit, the earliest
// giving way first, the latest last.
func latestFitting(room, count, cells, gap int) []flex.Span {
	runs := make([]flex.Part, count)
	for i := range runs {
		runs[i] = flex.Part{Least: cells, Natural: cells, Leaves: i + 1, Gap: gap}
	}
	spans, _ := flex.Lay(room, runs...)
	return slices.DeleteFunc(spans, func(s flex.Span) bool { return s.Left })
}
