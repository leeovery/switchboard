package watch

import (
	"maps"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// rowsGuardDays are the days the Log's day back's guarded world has run as
// it starts, in the evening: every one before today's head built, its rows
// standing.
const rowsGuardDays = 4

// The Log's day back's part: the Log opened in the evening, paged back
// through every past day the world ran, reads each day from its rows where
// they stand and no day's lines whose rows do: here every past day's, so it
// reads none of their lines. Read so, its Requests are those of one read
// whole, every day's lines read raw, as sameLogAsOneRead has it.
func TestTheLogsDayBackReadsNoDayRawWhoseRowsStand(t *testing.T) {
	start := soakAt(21, 0)
	g := readingHeads("DayRows")
	s := newSoak(t, soakSetup{start: start, days: rowsGuardDays, pace: guardPace, guard: g})
	s.show("log")
	oldest := dayfile.DateOf(dayfile.DayStart(start, 1-rowsGuardDays))
	for range 3 * rowsGuardDays {
		if s.h.model.log.from.Date == oldest {
			break
		}
		s.pagedBack()
	}
	if from := s.h.model.log.from.Date; from != oldest {
		t.Fatalf("paged back, the Log spans from %s, want the first day the world ran, %s", from, oldest)
	}
	read := slices.Sorted(maps.Keys(g.stood))
	for back := rowsGuardDays - 1; back > 0; back-- {
		if past := dayfile.DateOf(dayfile.DayStart(start, -back)); !g.stood[past] {
			t.Errorf("the Log read back the rows of %v, want every past day's among them, %s's", read, past)
		}
	}
	if len(g.raw) > 0 {
		t.Errorf("the Log read back the lines of %v, want none, every past day's rows standing", g.raw)
	}
	sameLogAsOneRead(t, s.h.model, s.follower, s.h.model.now())
}
