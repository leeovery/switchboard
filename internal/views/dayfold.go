package views

import "time"

// DayFold is a local day's sessions folded of its lines, what each spent by
// the local hour its requests arrived in, and what the requests of none spent
// so, as todaysSessions folds today's: what Accounts' WORTH and Projects'
// WORTH BY chart the day by the hour of, as worthCharts has it. Its hours are
// known, as hoursOn has it, of the tail's fold of its day, whatever it holds,
// as the tail reads every line of it; and of a day's read, where the ledger
// keeps the day's lines, as OldestDay's kept has it, a quiet day's too, as
// no hours are known of a day whose lines are no longer kept. It's the
// tail's where TodayFold gives it. The zero DayFold is of no day.
type DayFold struct {
	date   string
	held   daySessions
	known  bool
	tailed bool
}

// ReadDayFold is the fold of the local day with the given date in the pool
// read p, as DayFold has it, of the day's lines in the pool as ReadDay reads
// them whole, read once at now: its hours known where the ledger keeps the
// day's lines at now, keeping them as long as keep says, as [ledger] keep
// gives it, as OldestDay's kept has it of the oldest day Begun's Oldest
// gives, so a day whose lines are no longer kept is charted by the day, as
// its sessions are told as no longer kept; none of them read where it
// doesn't.
func ReadDayFold(l Ledger, date string, keep time.Duration, now time.Time, p PoolRead) DayFold {
	f := DayFold{date: date, held: hourlySessions(), known: linesBegunIn(l, now).Oldest(keep, now).kept(date)}
	if f.known {
		f.held = f.held.with(ReadDay(l, date, LogFilter{}, p).records)
	}
	return f
}

// TodayFold is the fold of d's day the ledger's tail feeds, today's, or the
// day it turned from till the tail's next read, as DayFold has it, its hours
// known.
func (d SessionDays) TodayFold() DayFold {
	return DayFold{date: d.today.Date, held: d.held, known: true, tailed: true}
}

// Tailed reports whether the fold is the ledger's tail's, as TodayFold gives
// it: so of the fold HoursFold picks, whether the day is charted by the
// tail's hours, and follows the tail's reads, or by a read of it, which
// doesn't.
func (f DayFold) Tailed() bool {
	return f.tailed
}

// HoursFold is the fold the period's day, as Day has it, is charted by the
// hours of at now, weeks starting on the day given, as worthCharts charts a
// day's hours, of tail, the tail's fold, as TodayFold has it, and read, the
// fold read of a day gone, as ReadDayFold reads it: tail wherever it's of
// that day, as each takes the day's sessions of it, so a day's hours and its
// sessions are of one fold: today's at now, never stood for by a read of
// it; and the day the tail turned from till its next read, a pick the watch
// waits past rather than charting, as that fold misses the day's lines
// filed since the tail was last read; else of a day gone, read where it's
// of that day, its hours known; the zero DayFold where neither is, as of a
// period of more than a day, or of a day whose lines are no longer kept,
// which is charted by the day. It's the one choice of a day's hours,
// Accounts' and Projects' alike.
func (p Period) HoursFold(now time.Time, starts time.Weekday, read, tail DayFold) DayFold {
	date, gone, ok := p.Day(now, starts)
	switch {
	case !ok:
		return DayFold{}
	case tail.hoursOn() == date:
		return tail
	case gone && read.hoursOn() == date:
		return read
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
