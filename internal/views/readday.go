package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/ledger"
)

// ReadDay is the LogDay of the local day with the given date in the pool
// read p, as the Log's tables reach back to it, read through the filter
// given, as LogFilter's through has it, its through. Of every pool's read,
// through the zero LogFilter, which keeps every line, it's the day whole:
// its records restored from its rows where they stand and every chunk of
// them restores, as restoredDay has it; else of its lines, as l's Day gives
// them, as of a day with no rows standing, today's among them, and
// yesterday's until its head is built, or one a chunk of whose rows won't
// read. A day's rows are exactly its lines' records, in Day's order, so
// either way it's the same day. Through any other filter, it's none where
// the day's summaries in the pool, as p's Dates gives them, say it may hold
// none the through keeps; else it's read as the day whole is, of those
// chunks of its rows alone whose meta may keep one, of the pool, as a
// pool's read keeps only the chunks that may hold its rows. Of a pool's
// read, it holds the pool's records alone, as p's keeps has them. So it
// gives every record of the day whole in the pool that its through keeps,
// in order, among others of the pool it drops none of, and holds the day's
// date.
func ReadDay(l Ledger, date string, through LogFilter, p PoolRead) LogDay {
	through = through.through()
	day := readThrough(l, date, through, p)
	day.through, day.date = through, date
	return day
}

// readThrough is the LogDay of the local day with the given date in the
// pool read p, read through the filter given, a through, as ReadDay reads
// it, its through left unset: none where the day's summaries in the pool say
// it may hold none through keeps, as LogFilter's mayKeepOn has it; else of
// its rows, as restoredDay has them; else of its lines in the pool, as l's
// Day gives them and p's Held keeps them.
func readThrough(l Ledger, date string, through LogFilter, p PoolRead) LogDay {
	if !through.mayKeepOn(p.Dates(l), date) {
		// A day of none holds none, not nil, as RecordsOf has it of no lines.
		return LogDay{records: []lineRecord{}}
	}
	if day, ok := restoredDay(l, date, through, p); ok {
		return day
	}
	return RecordsOf(p.Held(l.Day(date)))
}

// restoredDay is the LogDay the rows of the local day with the given date
// in the pool read p hold, as l's DayRows points at them, of the chunks that
// may hold a line through and p keep, as chunksThrough chooses them, each
// restored, as restoredChunks restores them, of the pool's records alone, as
// p's keeps has them: reporting false where the rows don't stand, a meta
// read won't, or any chunk won't restore, never part of those chunks.
func restoredDay(l LedgerRows, date string, through LogFilter, p PoolRead) (LogDay, bool) {
	chunks, ok := chunksThrough(l, date, through, p)
	if !ok {
		return LogDay{}, false
	}
	// A day of none holds none, not nil, as RecordsOf has it of no lines.
	records, ok := restoredChunks(l, date, chunks, []lineRecord{})
	if !p.every() {
		records = slices.DeleteFunc(records, func(r lineRecord) bool { return !p.keeps(r.pool) })
	}
	return LogDay{records: records}, ok
}

// chunksThrough are the chunks of the rows of the local day with the given
// date, as l's DayRows points at them, that may hold a line through and the
// pool read p keep, in order, none read: every one where both keep every
// line, reading no meta; else those whose meta may keep one, as
// chunksMeeting chooses them by mayKeep and holdsPool. It reports false
// where the rows don't stand, or a meta read won't.
func chunksThrough(l LedgerRows, date string, through LogFilter, p PoolRead) ([]ledger.RowsChunk, bool) {
	if through.whole() && p.every() {
		rows, ok := l.DayRows(date, RowsVersion)
		return rows.Chunks, ok
	}
	return chunksMeeting(l, date, func(m *chunkMeta) bool { return m.holdsPool(p) && m.mayKeep(through) })
}

// chunksMeeting are the chunks of the rows of the local day with the given
// date, as l's DayRows points at them, whose meta meets says may hold a
// line wanted, in order, none read: reporting false, for the day to be read
// raw, where its rows don't stand, as DayRows says, or a chunk's meta won't
// read. It's the one choice of a day's chunks by their metas, of a turn's
// or a filter's.
func chunksMeeting(l LedgerRows, date string, meets func(m *chunkMeta) bool) ([]ledger.RowsChunk, bool) {
	rows, ok := l.DayRows(date, RowsVersion)
	if !ok {
		return nil, false
	}
	var meeting []ledger.RowsChunk
	for _, c := range rows.Chunks {
		meta, err := chunkMetaOf(c.Meta)
		if err != nil {
			return nil, false
		}
		if meets(&meta) {
			meeting = append(meeting, c)
		}
	}
	return meeting, true
}

// restoredChunks are records with the records of the chunks given appended,
// chunks of the rows of the local day with the given date as l's DayRows
// points at them, each read as its RowChunk reads it, and restored, as
// restoredRows restores it, in order: reporting false where any won't read,
// won't restore, or restores other than as many records as the head counts
// it, so its reader reads the day's lines instead, never part of them. It's
// the one read of a day's chunks, of every one or of some.
func restoredChunks(l LedgerRows, date string, chunks []ledger.RowsChunk, records []lineRecord) ([]lineRecord, bool) {
	// Room grows with the rows read, never as the head counts them, which a
	// head edited by hand may count as many as it likes.
	for _, c := range chunks {
		data, err := l.RowChunk(date, c)
		if err != nil {
			return nil, false
		}
		from := len(records)
		if records, err = restoredRows(records, data); err != nil || len(records)-from != c.Count {
			return nil, false
		}
	}
	return records, true
}
