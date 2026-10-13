package views

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// LogSources are what the Log's Requests are built from.
type LogSources struct {
	// From is when the table starts: it spans the local days from the one
	// From falls on to Now's.
	From time.Time
	// Read holds the ledger's lines of those days, and their sessions', as
	// ReadLog reads them.
	Read LogRead
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

// LogRead is a read of the ledger for the Log's Requests, as ReadLog reads
// it: the lines of the days the table spans; and every line of their
// sessions, whatever day it's of, by each one's id, oldest first, which each
// session's story is told from. A row's story is never told from the table's
// lines alone, which would make a session that began the day before new
// today, and a move after a night's idle cost its context written again.
type LogRead struct {
	lines    []ledger.Line
	sessions map[string][]ledger.Line
}

// ReadLog reads, through l, the sessions of lines, the ledger's lines of the
// days the Log's Requests span, every kind, in any order, as the days' files
// hold them: every line of each, whatever day it's of, as the ledger's
// Sessions reads them, which says which of its requests is its first, the
// moves they made and what each cost.
func ReadLog(l LedgerSessions, lines []ledger.Line) LogRead {
	return LogRead{lines: lines, sessions: l.Sessions(sessionIDs(lines), nil)}
}

// sessionIDs are the ids of the sessions of the requests among lines, each
// once.
func sessionIDs(lines []ledger.Line) []string {
	ids := make(map[string]bool)
	for i := range lines {
		if isRequest(&lines[i]) {
			ids[lines[i].Session] = true
		}
	}
	return slices.Collect(maps.Keys(ids))
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
// placed alike keep the order of the lines read, then of the stream's alone.
func LogRows(src LogSources) LogTable {
	today, first := Today(src.Now), Today(src.From)
	stories := src.Read.stories(src.Prices, today)
	lines := src.Read.lines
	rows := make([]placedRow, 0, len(lines)+len(src.Stream.Requests))
	lined := make(map[string]bool, len(lines))
	for i := range lines {
		l := &lines[i]
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

// stories are the stories of the sessions read, by their ids, as their lines
// tell them, priced at today's prices.
func (r LogRead) stories(prices ledger.Table, today LedgerDay) map[string]*story {
	stories := make(map[string]*story, len(r.sessions))
	for id, lines := range r.sessions {
		stories[id] = storyOf(requestsOf(lines, prices, today.Date), prices, today)
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
