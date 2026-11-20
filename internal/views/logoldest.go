package views

import (
	"cmp"
	"errors"
	"io/fs"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
)

// OldestDay is the oldest day the Log reads back to, as OldestKept gives it,
// and how long the ledger keeps its lines, dayfile.Forever for good. The
// zero OldestDay is none known, as before the ledger has said where its
// lines begin: the Log reads no day back, nor draws the oldest day's line.
type OldestDay struct {
	Day  LedgerDay
	Keep time.Duration
}

// ledgerKept is how long the ledger keeps a day's lines, keep being [ledger]
// keep as given: dayfile.Forever for good, and none, as a config that
// doesn't say, config.DefaultLedgerKeep, as the router keeps them.
func ledgerKept(keep time.Duration) time.Duration {
	return cmp.Or(keep, config.DefaultLedgerKeep)
}

// OldestKept is the oldest day the Log reads back to at now, the ledger
// keeping its lines as long as keep says, as [ledger] keep gives it:
// dayfile.Forever for good, and none, as a config that doesn't say,
// config.DefaultLedgerKeep, as the router keeps them. Its first day of lines
// is first's, zero where it holds no day's lines yet, as Ledger's
// FirstLinesDay fails with fs.ErrNotExist: the oldest day is the later of
// first's day, today where it's zero, and the oldest day whose lines the
// ledger keeps at now, as dayfile.OldestKept says.
func OldestKept(first time.Time, keep time.Duration, now time.Time) OldestDay {
	keep = ledgerKept(keep)
	if first.IsZero() {
		first = now
	}
	oldest := OldestDay{Day: Today(first), Keep: keep}
	if kept, ok := dayfile.OldestKept(now, keep); ok && kept.After(oldest.Day.Start) {
		oldest.Day = Today(kept)
	}
	return oldest
}

// DayBefore is the day the Log reads back to next, the first its table
// spans being first: the one before first, false where the ledger doesn't
// keep it, as keeps says, so the day before the oldest is never read.
func (o OldestDay) DayBefore(first LedgerDay) (LedgerDay, bool) {
	before := Today(dayfile.DayStart(first.Start, -1))
	if !o.keeps(before) {
		return LedgerDay{}, false
	}
	return before, true
}

// keeps reports whether the ledger keeps the lines of the day d, to read
// back to: the oldest day, or one after it, as kept has it; none where it's
// none known.
func (o OldestDay) keeps(d LedgerDay) bool {
	return o.known() && o.kept(d.Date)
}

// kept reports whether the ledger keeps the lines of the day with the given
// date, and so its sessions and hours: the oldest day, or one after it;
// every day where it's none known, as none can then be told as no longer
// kept. It's the one rule of a day no longer kept, the Log's, Projects' and
// the List's, and of a day's hours.
func (o OldestDay) kept(date string) bool {
	return !o.known() || date >= o.Day.Date
}

// linesBegun is where the ledger's lines began as a read found them, as its
// FirstLinesDay gives it: first, the start of the first day it held lines
// of, zero where it held none; and known, false where it couldn't say.
type linesBegun struct {
	first time.Time
	known bool
}

// linesBegunIn is where the ledger l's lines begin at now, as linesBegun has
// it.
func linesBegunIn(l Ledger, now time.Time) linesBegun {
	first, err := l.FirstLinesDay(now)
	switch {
	case err == nil:
		return linesBegun{first: first, known: true}
	case errors.Is(err, fs.ErrNotExist):
		return linesBegun{known: true}
	}
	return linesBegun{}
}

// oldest is the oldest day whose lines the ledger keeps at now, keeping them
// as long as keep says, as [ledger] keep gives it, as OldestKept has it of
// where they began, the day the Log reads back to: the zero OldestDay, none
// known, where where they began isn't known.
func (b linesBegun) oldest(keep time.Duration, now time.Time) OldestDay {
	if !b.known {
		return OldestDay{}
	}
	return OldestKept(b.first, keep, now)
}

// reached reports whether a table spanning the days from first's reaches
// the oldest day: never where it's none known.
func (o OldestDay) reached(first LedgerDay) bool {
	return o.known() && !first.Start.After(o.Day.Start)
}

// known reports whether o is a day known, not the zero OldestDay.
func (o OldestDay) known() bool {
	return o.Day.Date != ""
}
