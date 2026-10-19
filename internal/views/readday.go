package views

// ReadDay is the LogDay of the local day with the given date, as the Log's
// tables reach back to it: its records restored from its rows where they
// stand and every chunk of them restores, as restoredDay has it; else of its
// lines, as l's Day gives them, as of a day with no rows standing, today's
// among them, and yesterday's until its head is built, or one a chunk of
// whose rows won't read. A day's rows are exactly its lines' records, in
// Day's order, so either way it's the same day.
func ReadDay(l Ledger, date string) LogDay {
	if day, ok := restoredDay(l, date); ok {
		return day
	}
	return RecordsOf(l.Day(date))
}

// restoredDay is the LogDay the rows of the local day with the given date
// hold, as l's DayRows points at them, each chunk read as its RowChunk reads
// it, and restored, as restoredRows restores it, in order: whole, or
// reporting false where they don't stand, or any chunk won't read, won't
// restore, or restores other than as many records as the head counts it,
// never part of a day.
func restoredDay(l Ledger, date string) (LogDay, bool) {
	rows, ok := l.DayRows(date, RowsVersion)
	if !ok {
		return LogDay{}, false
	}
	// Room grows with the rows read, never as the head counts them, which a
	// head edited by hand may count as many as it likes; and a day of none
	// holds none, not nil, as RecordsOf has it of no lines.
	records := []lineRecord{}
	for _, c := range rows.Chunks {
		data, err := l.RowChunk(date, c)
		if err != nil {
			return LogDay{}, false
		}
		from := len(records)
		if records, err = restoredRows(records, data); err != nil || len(records)-from != c.Count {
			return LogDay{}, false
		}
	}
	return LogDay{records: records}, true
}
