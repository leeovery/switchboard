package views

import (
	"slices"
	"time"
)

// LineKey is a line of the Log's table as each table built afresh knows it,
// so a line chosen stays chosen, and a line seen isn't new: a request's row
// by its request's id and the time it ran over, which its line filling it in
// keeps, as the router's ids start again each run; a day's rule by the days
// it falls between; and the oldest day's line by its day. A row's key holds
// where it sits within its day too, as placedRow has it, which its line
// filling it in keeps, so a table finds it without reading every row.
type LineKey struct {
	id    string
	kind  lineKind
	ran   runSpan
	place time.Time
}

// lineKind is what a line of the table is.
type lineKind int

const (
	rowLine lineKind = iota
	ruleLine
	oldestLine
)

// runSpan is the time a request ran over, as its row tells it: from when it
// arrived, or, before its line comes, was sent, to its end, or, in flight,
// to when its row was built.
type runSpan struct {
	from, to time.Time
}

// runSpanOf is the time the request of the row r ran over.
func runSpanOf(r RequestRow) runSpan {
	s := runSpan{from: r.At, to: r.At}
	if r.Took != nil {
		s.to = r.At.Add(*r.Took)
	}
	return s
}

// meets reports whether s and o share a moment, as a request's row in flight
// and its row once its line comes do.
func (s runSpan) meets(o runSpan) bool {
	return !s.from.After(o.to) && !o.from.After(s.to)
}

// Key is the line's identity.
func (l LogLine) Key() LineKey {
	switch {
	case l.Rule != nil:
		return LineKey{id: l.Rule.Ended.Date + " " + l.Rule.Began.Date, kind: ruleLine}
	case l.Oldest != nil:
		return LineKey{id: l.Oldest.Day.Date, kind: oldestLine}
	}
	return LineKey{id: l.Row.Request, ran: runSpanOf(l.Row), place: l.place}
}

// is reports whether k and o are the same line's.
func (k LineKey) is(o LineKey) bool {
	return k.id == o.id && k.kind == o.kind && (k.kind != rowLine || k.ran.meets(o.ran))
}

// Find is the place among t's lines of the line whose identity is k: false
// where t shows none, as where its filter hides it. A row is looked for
// where it sits, on the day it was sent or arrived on.
func (t LogTable) Find(k LineKey) (int, bool) {
	at := 0
	if t.oldest != nil {
		if (LogLine{Oldest: t.oldest}).Key().is(k) {
			return 0, true
		}
		at = 1
	}
	days := []string{Today(k.place).Date, Today(k.place.Add(-arrivesWithin)).Date}
	for _, s := range t.sections {
		switch {
		case k.kind == ruleLine && s.rule != nil && (LogLine{Rule: s.rule}).Key().is(k):
			return at + s.start, true
		case k.kind != rowLine || !slices.Contains(days, s.day.Date):
			continue
		}
		if o, ok := s.find(k); ok {
			return at + s.start + o, true
		}
	}
	return 0, false
}

// find is the place among the section's own lines of the row whose identity
// is k, as Find has it.
func (s tableSection) find(k LineKey) (int, bool) {
	from := s.rows.search(func(l *loggedLine) bool { return !l.place.Before(k.place) })
	for i, l := range s.rows.from(from) {
		if !l.place.Equal(k.place) {
			break
		}
		if s.lined(i).Key().is(k) {
			return s.placeOf(i), true
		}
	}
	for j, p := range s.told {
		if (LogLine{Row: p.row, place: p.place}).Key().is(k) {
			o := p.before + j
			if s.rule != nil {
				o++
			}
			return o, true
		}
	}
	return 0, false
}

// At is the place of the first request t shows at or after at: of its rows
// whose requests arrived then or later, or, before their lines came, were
// sent, the one that came first, the first placed of those that came
// together. It's false where none came then or later. A row sits where it
// was sent, so one may sit after one that arrived later.
func (t LogTable) At(at time.Time) (int, bool) {
	found, first := -1, time.Time{}
	for i, l := range t.All() {
		if l.IsRow() && !l.Row.At.Before(at) && (found < 0 || l.Row.At.Before(first)) {
			found, first = i, l.Row.At
		}
	}
	return found, found >= 0
}

// LogMark is where the Log's Requests stood as they paused, for the rows
// come since to be counted: the first day they spanned; how many lines the
// read they were built of had taken in; and the keys of the rows the stream
// told of then, shown or hidden. It holds no row of the table.
type LogMark struct {
	first LedgerDay
	seq   int
	told  []LineKey
}

// Mark is where t stands, as LogMark has it.
func (t LogTable) Mark() LogMark {
	return LogMark{first: t.lines.first, seq: t.lines.read.seq, told: t.told}
}

// NewSince is how many of the requests' rows t shows were of no request
// among those of the table as it stood at the mark m, shown or hidden, on
// the days it spanned: those come since, as t's filter shows them. A row
// filling in as its line comes isn't new, nor is one of a day before m's
// first, read back since as the table's top was reached. The rows of lines
// it counts are those of the lines taken in since m, each day's found as
// they were taken in, so it reads none it held then.
func (t LogTable) NewSince(m LogMark) int {
	n := 0
	for _, d := range t.lines.read.days {
		if d.day.Date < m.first.Date || d.day.Date < t.lines.first.Date {
			continue
		}
		from := d.lines.search(func(l *loggedLine) bool { return l.seq > m.seq })
		for _, l := range d.lines.from(from) {
			k := LineKey{id: l.row.Request, ran: runSpanOf(l.row)}
			if t.lines.filter.hiding(&l.row) == notHidden && !m.toldOf(k) {
				n++
			}
		}
	}
	for _, s := range t.sections {
		for _, p := range s.told {
			if m.first.Holds(p.day.Start) && !m.toldOf(LineKey{id: p.row.Request, ran: runSpanOf(p.row)}) {
				n++
			}
		}
	}
	return n
}

// toldOf reports whether a row the stream told of at the mark m had the
// identity k, shown or hidden.
func (m LogMark) toldOf(k LineKey) bool {
	return slices.ContainsFunc(m.told, func(o LineKey) bool { return o.is(k) })
}

// IsRow reports whether the line is a request's row: the one kind of line
// the Log chooses.
func (l LogLine) IsRow() bool {
	return l.Rule == nil && l.Oldest == nil
}

// FilledFrom is the latest day t could start from and still fill a view
// rows rows tall, its lines from that day to the last at least as many,
// the rule before its first row and the oldest day's line gone as it would
// start there: false where none fills it, as where t holds fewer lines.
func (t LogTable) FilledFrom(rows int) (LedgerDay, bool) {
	n := 0
	for _, s := range slices.Backward(t.sections) {
		n += s.n
		if s.rule != nil && n-1 >= rows || s.rule == nil && n >= rows {
			return s.day, true
		}
	}
	return LedgerDay{}, false
}
