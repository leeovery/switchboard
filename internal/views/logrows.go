package views

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// LogSources are what the Log's Requests are built from.
type LogSources struct {
	// From is when the table starts: it spans the local days from the one
	// From falls on to Now's.
	From time.Time
	// Lines are the ledger's lines of those days, every kind, in any order,
	// as the days' files hold them.
	Lines []ledger.Line
	// Sessions are the lines of the sessions of Lines, whatever day each is
	// of, by each session's id, oldest first, as Ledger.Sessions reads them:
	// what each session's story is told from, which says which of its
	// requests is its first, the moves they made and what each cost. A
	// session they don't hold is told from its lines among Lines alone.
	Sessions map[string][]ledger.Line
	// Stream is what the request stream told: each request in flight, and
	// each ended or lost, by its id. The zero Stream told of none.
	Stream Stream
	// Totals are the figures of the days, as TotalsByDay gives them: a day's
	// rule says those of the day that ended.
	Totals []DayTotals
	// ShowChecks and ShowCounts show Claude Code's quota checks and its
	// counts of tokens, each hidden until it's shown.
	ShowChecks, ShowCounts bool
	// Prices are the price table worth is priced by, at today's prices.
	Prices ledger.Table
	Now    time.Time
}

// hides reports whether the table hides the rows of what's asked of the
// kind given.
func (src LogSources) hides(kind RowKind) bool {
	return kind == RowCheck && !src.ShowChecks || kind == RowCount && !src.ShowCounts
}

// LogTable is the Log's Requests over the days it spans.
type LogTable struct {
	// Lines are the table's lines, the oldest first: the row of each request
	// shown, each request once, by its id; and before the first row shown of
	// each day after the first with one, that day's rule.
	Lines  []LogLine
	counts map[string]LogCount
}

// LogLine is a line of the Log's table: a request's row, or, where Rule is
// set, the rule where one day's rows end and the next's begin.
type LogLine struct {
	Row  RequestRow
	Rule *DayRule
}

// DayRule is where one day's rows end and the next's begin, a night's quiet
// unfolded: it falls before the next day's first row shown, so it appears as
// that row does, and one stands across the days between with none.
type DayRule struct {
	// Ended is the day that ended, the last before Began with a row shown,
	// and Began the day that began.
	Ended, Began LedgerDay
	// Totals are Ended's figures, as LogSources' Totals give them: none but
	// its date where they don't.
	Totals DayTotals
}

// LogCount is what a day's rows count: those shown, which the table holds;
// and those hidden, by what they asked, Claude Code's quota checks and its
// counts of tokens. Together they're each of the day's requests, once.
type LogCount struct {
	Shown, Checks, Counts int
}

// Hidden is how many of the day's rows are hidden.
func (c LogCount) Hidden() int {
	return c.Checks + c.Counts
}

// add counts a row of what's asked of the kind given, shown or hidden.
func (c *LogCount) add(kind RowKind, shown bool) {
	switch {
	case shown:
		c.Shown++
	case kind == RowCheck:
		c.Checks++
	default:
		c.Counts++
	}
}

// Count is what the rows of the local day with the given date count, from
// the table's own rows: none where it holds no row of it.
func (t LogTable) Count(date string) LogCount {
	return t.counts[date]
}

// placedRow is a request's row as the table places it: on the local day its
// request arrived on, or, before its line comes, was sent on; and, within
// it, where it was sent, where the stream told of that, else where it
// arrived.
type placedRow struct {
	row   RequestRow
	day   LedgerDay
	place time.Time
}

// LogRows builds the Log's Requests from src: a row of each request, once,
// by its id: from its line, where it has come; else from what the stream
// told of it, in flight, ended or lost. A request the router answers itself
// is never on the stream, so its row comes with its line.
//
// A row sits on the day its request arrived on, as the ledger files its
// line, so a request in flight as a day ends sits above the next day's rule;
// before its line comes, on the day it was sent, the one time the stream
// tells. Within its day it sits where it was sent, where the stream told
// that, so a row stays where it sat in flight as its line fills it in,
// though the line's arrival is a moment earlier; else where it arrived. Rows
// placed alike keep the order of Lines, then of the stream's alone.
func LogRows(src LogSources) LogTable {
	today, first := Today(src.Now), Today(src.From)
	stories := storiesOf(src, today)
	rows := make([]placedRow, 0, len(src.Lines)+len(src.Stream.Requests))
	lined := make(map[string]bool, len(src.Lines))
	for i := range src.Lines {
		l := &src.Lines[i]
		lined[l.Request] = true
		place := l.At
		if t, told := src.Stream.Requests[l.Request]; told && !t.Sent.IsZero() {
			place = t.Sent
		}
		rows = append(rows, placedRow{row: lineRow(l, stories[l.Session], src.Prices, today), day: Today(l.At), place: place})
	}
	for _, t := range toldAlone(src.Stream, lined) {
		rows = append(rows, placedRow{row: toldRow(t, src.Now), day: Today(t.Sent), place: t.Sent})
	}
	rows = slices.DeleteFunc(rows, func(p placedRow) bool { return p.day.Start.Before(first.Start) })
	slices.SortStableFunc(rows, func(a, b placedRow) int {
		return cmp.Or(a.day.Start.Compare(b.day.Start), a.place.Compare(b.place))
	})
	return tableOf(rows, src)
}

// toldAlone are the requests the stream told of whose lines aren't among
// those lined, by their ids, but those it told no sending of, which have no
// place, by when they were sent, then their ids.
func toldAlone(s Stream, lined map[string]bool) []Told {
	var alone []Told
	for _, id := range slices.Sorted(maps.Keys(s.Requests)) {
		if t := s.Requests[id]; !lined[id] && !t.Sent.IsZero() {
			alone = append(alone, t)
		}
	}
	slices.SortStableFunc(alone, func(a, b Told) int { return a.Sent.Compare(b.Sent) })
	return alone
}

// storiesOf are the stories of the sessions of src's requests, by their
// ids, as src's Sessions tell them, else as their lines among src's Lines
// do, oldest first, priced at today's prices.
func storiesOf(src LogSources, today LedgerDay) map[string]*story {
	own := make(map[string][]ledger.Line)
	for _, l := range src.Lines {
		if _, held := src.Sessions[l.Session]; !held && isRequest(&l) {
			own[l.Session] = append(own[l.Session], l)
		}
	}
	stories := make(map[string]*story)
	for _, l := range src.Lines {
		if _, told := stories[l.Session]; told || !isRequest(&l) {
			continue
		}
		lines, held := src.Sessions[l.Session]
		if !held {
			lines = own[l.Session]
			slices.SortStableFunc(lines, func(a, b ledger.Line) int {
				return cmp.Or(a.At.Compare(b.At), strings.Compare(a.Request, b.Request))
			})
		}
		stories[l.Session] = storyOf(requestsOf(lines, src.Prices, today.Date), src.Prices, today)
	}
	return stories
}

// tableOf is the table of rows, placed in order, src's kinds hidden as it
// says, each day's rule before its first row shown, with the figures of the
// day that ended, and each day's counts.
func tableOf(rows []placedRow, src LogSources) LogTable {
	totals := make(map[string]DayTotals, len(src.Totals))
	for _, t := range src.Totals {
		totals[t.Date] = t
	}
	table := LogTable{counts: make(map[string]LogCount)}
	var last *LedgerDay
	for _, p := range rows {
		shown := !src.hides(p.row.Kind)
		c := table.counts[p.day.Date]
		c.add(p.row.Kind, shown)
		table.counts[p.day.Date] = c
		if !shown {
			continue
		}
		if last != nil && last.Date != p.day.Date {
			ended := totals[last.Date]
			ended.Date = last.Date
			table.Lines = append(table.Lines, LogLine{Rule: &DayRule{Ended: *last, Began: p.day, Totals: ended}})
		}
		table.Lines = append(table.Lines, LogLine{Row: p.row})
		last = &p.day
	}
	return table
}
