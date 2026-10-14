package views

import (
	"slices"
	"time"
)

// LineKey is a line of the Log's table as each table built afresh knows it,
// so a line chosen stays chosen, and a line seen isn't new: a request's row
// by its request's id and the time it ran over, which its line filling it in
// keeps, as the router's ids start again each run; a day's rule by the days
// it falls between; and the oldest day's line by its day.
type LineKey struct {
	id   string
	kind lineKind
	ran  runSpan
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
	return LineKey{id: l.Row.Request, ran: runSpanOf(l.Row)}
}

// is reports whether k and o are the same line's.
func (k LineKey) is(o LineKey) bool {
	return k.id == o.id && k.kind == o.kind && (k.kind != rowLine || k.ran.meets(o.ran))
}

// Find is the place among t's lines of the line whose identity is k: false
// where t shows none, as where its filter hides it.
func (t LogTable) Find(k LineKey) (int, bool) {
	i := slices.IndexFunc(t.Lines, func(l LogLine) bool { return l.Key().is(k) })
	return i, i >= 0
}

// At is the place of the first request t shows at or after at: of its rows
// whose requests arrived then or later, or, before their lines came, were
// sent, the one that came first, the first placed of those that came
// together. It's false where none came then or later. A row sits where it
// was sent, so one may sit after one that arrived later.
func (t LogTable) At(at time.Time) (int, bool) {
	found := -1
	for i, l := range t.Lines {
		if l.IsRow() && !l.Row.At.Before(at) && (found < 0 || l.Row.At.Before(t.Lines[found].Row.At)) {
			found = i
		}
	}
	return found, found >= 0
}

// NewSince is how many of the requests' rows t shows were of no request
// among then's, shown or hidden, on the days then spanned: those come since
// the table was then, as t's filter shows them. A row filling in as its
// line comes isn't new, nor is one of a day before then's first, read back
// since as the table's top was reached.
func (t LogTable) NewSince(then LogTable) int {
	n := 0
	for _, l := range t.Lines {
		if l.IsRow() && then.first.Holds(l.day.Start) && !then.held(l.Key()) {
			n++
		}
	}
	return n
}

// held reports whether a request's row with the identity k is among t's
// rows, shown or hidden.
func (t LogTable) held(k LineKey) bool {
	return slices.ContainsFunc(t.ran[k.id], func(s runSpan) bool { return s.meets(k.ran) })
}

// IsRow reports whether the line is a request's row: the one kind of line
// the Log chooses.
func (l LogLine) IsRow() bool {
	return l.Rule == nil && l.Oldest == nil
}
