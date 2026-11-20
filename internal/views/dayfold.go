package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// DayFold is a local day's sessions folded of its lines, what each spent by
// the local hour its requests arrived in, and what the requests of none spent
// so, as todaysSessions folds today's: what Accounts' WORTH and Projects'
// WORTH BY chart the day by the hour of, as worthCharts has it. Its hours are
// known, as hoursOn has it, of the tail's fold of its day, whatever it holds,
// as the tail reads every line of it; and of a day's read, where the ledger
// keeps the day's lines, as linesKept has it, a quiet day's too, as no hours
// are known of a day whose lines are no longer kept. The zero DayFold is of
// no day.
type DayFold struct {
	date  string
	held  daySessions
	known bool
}

// ReadDayFold is the fold of the local day with the given date in the pool
// read p, as DayFold has it, of the day's lines in the pool as ReadDay reads
// them whole, read once at now: its hours known where the ledger keeps the
// day's lines at now, as linesKept has it of where linesFrom says they
// begin, so a day whose lines are no longer kept is charted by the day, as
// its sessions are told as no longer kept; none of them read where it
// doesn't.
func ReadDayFold(l Ledger, date string, now time.Time, p PoolRead) DayFold {
	f := DayFold{date: date, held: hourlySessions(), known: linesKept(date, linesFrom(l, now))}
	if f.known {
		f.held = f.held.with(ReadDay(l, date, LogFilter{}, p).records)
	}
	return f
}

// TodayFold is the fold of d's day the ledger's tail feeds, today's, or the
// day it turned from till the tail's next read, as DayFold has it, its hours
// known.
func (d SessionDays) TodayFold() DayFold {
	return DayFold{date: d.today.Date, held: d.held, known: true}
}

// HoursFold is the fold the period's day is charted by the hours of at now,
// weeks starting on the day given, as worthCharts charts a day's hours, of
// read, the fold read of a day gone, as ReadDayFold reads it, and tail, the
// tail's, as TodayFold has it: of a period gone, read where it's of that day,
// its hours known, else tail where it is, as the day the tail turned from is
// till its next read; at now, tail alone, as today's is folded live and
// never stood for by a read of it; the zero DayFold where neither is, as of
// a period of more than a day, or of a day whose lines are no longer kept,
// which is charted by the day. It's the one choice of a day's hours,
// Accounts' and Projects' alike.
func (p Period) HoursFold(now time.Time, starts time.Weekday, read, tail DayFold) DayFold {
	folds := []DayFold{tail}
	if p.spanAt(dayfile.CivilOf(now), starts).gone {
		folds = []DayFold{read, tail}
	}
	for _, f := range folds {
		if p.hourly(f.hoursOn(), now, starts) {
			return f
		}
	}
	return DayFold{}
}

// hoursOn is the date of the day whose hours the fold knows: its day's, ""
// where they aren't known.
func (f DayFold) hoursOn() string {
	if !f.known {
		return ""
	}
	return f.date
}

// hourlySessions is a fold of a day's sessions of no line yet, what each
// spent kept by the hour, and what requests of no session spent, no story
// told of its requests, as a day is charted by the hour.
func hourlySessions() daySessions {
	d := newDaySessions(false)
	d.hourly, d.loose = true, accountHours{}
	return d
}
