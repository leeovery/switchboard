package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/readings"
)

// RunwayReadings are the readings Runway's past is judged by, as
// LanesFrom takes them: the readings history's, filed, as a tail reads them,
// oldest first; and those the watch read of the documents it read itself
// while the router didn't answer, or as it probes, seen, which the history
// holds none of, oldest first; all of them oldest first, those read at one
// time the history's first. With none seen, they're the history's as given.
func RunwayReadings(filed, seen []readings.Reading) []readings.Reading {
	if len(seen) == 0 {
		return filed
	}
	joined := slices.Concat(filed, seen)
	slices.SortStableFunc(joined, byReadingTime)
	return joined
}
