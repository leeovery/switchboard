package views

// DayFold is a local day's sessions folded of its lines, what each spent by
// the local hour its requests arrived in, and what the requests of none spent
// so, as todaysSessions folds today's: what Accounts' WORTH and Projects'
// WORTH BY chart the day by the hour of, as worthCharts has it. Its hours are
// known, as hoursOn has it, of the tail's fold of its day, whatever it holds,
// as the tail reads every line of it; and of a day's read, where it found a
// line of the day, as it finds none of one whose lines are no longer kept.
// The zero DayFold is of no day.
type DayFold struct {
	date  string
	held  daySessions
	known bool
}

// ReadDayFold is the fold of the local day with the given date in the pool
// read p, as DayFold has it, of the day's lines in the pool as ReadDay reads
// them whole, read once: its hours known where it found a line of the day,
// so a day whose lines are no longer kept is charted by the day.
func ReadDayFold(l Ledger, date string, p PoolRead) DayFold {
	day := ReadDay(l, date, LogFilter{}, p)
	return DayFold{date: date, held: hourlySessions().with(day.records), known: len(day.records) > 0}
}

// TodayFold is the fold of d's day the ledger's tail feeds, today's, or the
// day it turned from till the tail's next read, as DayFold has it, its hours
// known.
func (d SessionDays) TodayFold() DayFold {
	return DayFold{date: d.today.Date, held: d.held, known: true}
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
