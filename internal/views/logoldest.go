package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// OldestDay is the oldest day the Log reads back to, as OldestKept gives it,
// and how long the ledger keeps its lines, dayfile.Forever for good. The
// zero OldestDay is none known, and the Log reads back without end.
type OldestDay struct {
	Day  LedgerDay
	Keep time.Duration
}

// OldestKept is the oldest day the Log reads back to at now, the ledger
// keeping its lines as long as keep says, dayfile.Forever for good, its
// first day of lines first's: the later of first's day and the oldest day
// keep spares, keep's days before today, part of a day counted whole, as
// dayfile keeps them.
func OldestKept(first time.Time, keep time.Duration, now time.Time) OldestDay {
	oldest := OldestDay{Day: Today(first), Keep: keep}
	if keep == dayfile.Forever {
		return oldest
	}
	const day = 24 * time.Hour
	days := keep / day
	if keep%day > 0 {
		days++
	}
	if kept := dayfile.DayStart(now.Local(), -int(days)); kept.After(oldest.Day.Start) {
		oldest.Day = Today(kept)
	}
	return oldest
}

// DayBefore is the day the Log reads back to next, the first its table
// spans being first: the one before first, false where first is the oldest
// day, or before it, so the day before the oldest is never read.
func (o OldestDay) DayBefore(first LedgerDay) (LedgerDay, bool) {
	if !first.Start.After(o.Day.Start) {
		return LedgerDay{}, false
	}
	return Today(dayfile.DayStart(first.Start, -1)), true
}

// reached reports whether a table spanning the days from first's reaches
// the oldest day: never where it's none known.
func (o OldestDay) reached(first LedgerDay) bool {
	return o.Day.Date != "" && !first.Start.After(o.Day.Start)
}
