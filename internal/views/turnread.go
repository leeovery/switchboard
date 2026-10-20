package views

import (
	"iter"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// readLine is a line of a session as a turn's read gives it: its flight and
// its turn facts, as mine reads them; and of a day read raw, the line,
// decoded into its record only once the turn takes it, or of a day read
// from its rows, its record, restored.
type readLine struct {
	flight
	facts turnFacts
	line  ledger.Line
	rec   *lineRecord
}

// rawLine is the line l, read raw.
func rawLine(l ledger.Line) readLine {
	return readLine{flight: flightOf(&l), facts: turnFactsOf(&l), line: l}
}

// restoredLine is the line r is the record of, restored from its day's rows.
func restoredLine(r *lineRecord) readLine {
	return readLine{flight: r.flight, facts: r.turnFacts(), rec: r}
}

// record is l's record: as it was restored, or decoded of its line.
func (l *readLine) record() lineRecord {
	if l.rec != nil {
		return *l.rec
	}
	return lineRecordOf(&l.line)
}

// linesOfTurn gives the lines of h's session that ReadTurn reads of the turn
// t, through l, at w's now, newest first, as l's Session gives them: of
// every day raw, where turnSpans gives no span; else of the turn's own days
// and the horizon's, as turnDaysOf has them.
func (w worked) linesOfTurn(l LedgerSession, t *turnTally) iter.Seq[readLine] {
	spans := w.turnSpans(t)
	if spans == nil {
		return turnDays{}.read(l.Session(w.h.id))
	}
	days := turnDaysOf(l, w.h.id, spans[0], spans[1])
	return days.read(l.Session(w.h.id, days.rawSpans()...))
}

// turnDays are the days a turn's lines are read of: the turn's span and the
// horizon's, as turnSpans has them; the days of the turn's span read from
// their rows, the newest first; and the records of the session their rows
// hold, newest first, as Session gives a session's lines.
type turnDays struct {
	turn, horizon ledger.Span
	rowDays       []dayfile.Bounds
	restored      []lineRecord
}

// turnDaysOf are the days the lines of a turn of the session with the given
// id are read of, through l, its span turn and the horizon's given: each day
// of the turn's before the one the horizon falls on from its rows, where
// they stand, as rowsOfTurn reads them; the rest raw, the horizon's days,
// today's among them, and those whose rows don't stand.
func turnDaysOf(l LedgerRows, id string, turn, horizon ledger.Span) turnDays {
	d := turnDays{turn: turn, horizon: horizon}
	before := dayfile.DateOf(horizon.From)
	for _, date := range slices.Backward(dayfile.Span(turn.From, turn.To)) {
		if date >= before {
			continue
		}
		if records, ok := rowsOfTurn(l, id, date, turn); ok {
			day, _ := dayfile.DayBounds(date)
			d.rowDays, d.restored = append(d.rowDays, day), append(d.restored, records...)
		}
	}
	return d
}

// rowsOfTurn are the records of the session with the given id the rows of
// the local day with the given date hold, newest first, read of the chunks
// whose meta says they may hold a line of the turn whose span is given, as
// meets has it, and no other: reporting false, for the day to be read raw,
// where its rows don't stand, as l's DayRows says, a chunk's meta won't
// read, or a chunk read won't restore, as restoredChunks has it.
func rowsOfTurn(l LedgerRows, id, date string, turn ledger.Span) ([]lineRecord, bool) {
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
		if meta.meets(id, turn) {
			meeting = append(meeting, c)
		}
	}
	records, ok := restoredChunks(l, date, meeting, nil)
	if !ok {
		return nil, false
	}
	records = slices.DeleteFunc(records, func(r lineRecord) bool { return r.session != id })
	slices.Reverse(records)
	return records, true
}

// rawSpans are the spans the lines of d's days not read from their rows are
// read raw within, as Session reads them: the turn's, cut where each day
// read from its rows falls, so none meets one, and the horizon's.
func (d turnDays) rawSpans() []ledger.Span {
	var spans []ledger.Span
	from := d.turn.From
	for _, day := range slices.Backward(d.rowDays) {
		if from.Before(day.Start) {
			// A span holds the instant it runs to, so this one ends an
			// instant before the day starts.
			spans = append(spans, ledger.Span{From: from, To: day.Start.Add(-time.Nanosecond)})
		}
		from = day.End
	}
	if !from.After(d.turn.To) {
		spans = append(spans, ledger.Span{From: from, To: d.turn.To})
	}
	return append(spans, d.horizon)
}

// read gives the lines of d's days, newest first, as Session gives them:
// those raw gives, read within d's raw spans, as rawSpans has them, but of
// the days read from their rows, whose records it gives in their place. So
// each line is given once, by the day it arrived on, whatever day's file
// holds it: one of a day read from its rows that raw gives, as the file of
// a day beside it holds it, is passed over.
func (d turnDays) read(raw iter.Seq[ledger.Held]) iter.Seq[readLine] {
	return func(yield func(readLine) bool) {
		restored := d.restored
		// restoredAfter gives those restored that arrived after at, reporting
		// whether yield wanted more.
		restoredAfter := func(at instant) bool {
			for ; len(restored) > 0 && restored[0].at.compare(at) > 0; restored = restored[1:] {
				if !yield(restoredLine(&restored[0])) {
					return false
				}
			}
			return true
		}
		for held := range raw {
			if d.fromRows(held.At) {
				continue
			}
			if !restoredAfter(instantOf(held.At)) || !yield(rawLine(held.Line)) {
				return
			}
		}
		restoredAfter(unsealed)
	}
}

// fromRows reports whether a line that arrived at at is of a day d reads
// from its rows.
func (d turnDays) fromRows(at time.Time) bool {
	for _, day := range d.rowDays {
		if day.Holds(at) {
			return true
		}
	}
	return false
}
