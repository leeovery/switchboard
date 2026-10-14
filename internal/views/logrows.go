package views

import (
	"cmp"
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
	// Oldest is the oldest day the Log reads back to, as OldestKept gives it:
	// a table reaching it says, at its top, how long the ledger keeps its
	// lines. The zero OldestDay is none known, and draws no such line.
	Oldest OldestDay
	// Read holds the ledger's lines of those days, and their sessions', each
	// row priced and told, as the Log's read grows.
	Read LogRead
	// Stream is what the request stream told: each request in flight, and
	// each ended or lost, by its id. The zero Stream told of none.
	Stream Stream
	// Totals are the figures of the days, as TotalsByDay gives them: a day's
	// rule says those of the day that ended.
	Totals []DayTotals
	// Filter is what the table shows of the days' lines: the zero Filter
	// every one.
	Filter LogFilter
	Now    time.Time
}

// LogTable is the Log's Requests over the days it spans.
type LogTable struct {
	// Lines are the table's lines, the oldest first: the row of each request
	// shown, each request once, by its id; before the first row shown of each
	// day after the first with one, that day's rule; and, where the table
	// reaches the oldest day, the oldest day's line atop them all.
	Lines  []LogLine
	counts map[string]LogCount
	// ran are the times the requests of the table's rows ran over, shown or
	// hidden, by their ids; and awaited the days its rows sit on, shown or
	// hidden, of a request whose end the stream told, its line yet to come.
	ran     map[string][]runSpan
	awaited []LedgerDay
	// first is the first day the table spans.
	first LedgerDay
}

// LogLine is a line of the Log's table: a request's row; where Rule is set,
// the rule where one day's rows end and the next's begin; or, where Oldest
// is set, the oldest day's line, which says how long the ledger keeps its
// lines.
type LogLine struct {
	Row    RequestRow
	Rule   *DayRule
	Oldest *OldestDay
	// line is the line the row is of, nil before it comes; and day the
	// local day the row sits on.
	line *ledger.Line
	day  LedgerDay
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

// dayRules are the figures of the days a table of the Log's spans, by date,
// as its day rules say them.
type dayRules map[string]DayTotals

// dayRulesOf are the day rules of the days whose figures totals give.
func dayRulesOf(totals []DayTotals) dayRules {
	rules := make(dayRules, len(totals))
	for _, t := range totals {
		rules[t.Date] = t
	}
	return rules
}

// between is the rule where the rows of the day ended end and those of the
// day began begin: ended's figures, none but its date where they're not
// given.
func (r dayRules) between(ended, began LedgerDay) *DayRule {
	totals := r[ended.Date]
	totals.Date = ended.Date
	return &DayRule{Ended: ended, Began: began, Totals: totals}
}

// Awaited are the dates of the days before the day given that the table
// holds a row on, shown or hidden, of a request whose end the stream told,
// its line yet to come, each once: a line is filed under the day its request
// arrived on, so a later day's lines never hold the line of a request across
// midnight.
func (t LogTable) Awaited(before LedgerDay) []string {
	var dates []string
	for _, d := range t.awaited {
		if d.Start.Before(before.Start) {
			dates = append(dates, d.Date)
		}
	}
	return dates
}

// Count is what the lines of the local day with the given date count, from
// the table's own rows, those its filter hides among them: none where it
// holds no row of it.
func (t LogTable) Count(date string) LogCount {
	return t.counts[date]
}

// placedRow is a request's row as the table places it: on the local day its
// request arrived on, or, before its line comes, was sent on; and, within
// it, where it was sent, where the stream told of that, else where it
// arrived. Its session is the one its request counts toward, as
// ledger.Line.CountedSession has it of its line, or Told.countedSession of
// a request told alone: none for a quota check.
type placedRow struct {
	row     RequestRow
	line    *ledger.Line
	session string
	day     LedgerDay
	place   time.Time
}

// LogRows builds the Log's Requests from src: a row of each request, once:
// from its line, where it has come, as filedAs knows it; else from what the
// stream told of it, in flight, ended or lost. A request the router answers
// itself is never on the stream, so its row comes with its line.
//
// A row sits on the day its request arrived on, as the ledger files its
// line, so a request in flight as a day ends sits above the next day's rule;
// before its line comes, on the day it was sent, the one time the stream
// tells. Within its day it sits where it was sent, where the stream told
// that, so a row stays where it sat in flight as its line fills it in,
// though the line's arrival is a moment earlier; else where it arrived. Rows
// placed alike keep the order of the lines read, then of the stream's alone.
//
// Its rows of lines are as src's Read holds them, priced and told as their
// lines were taken in: it places them, and builds the stream's own, at each
// call, reading and pricing nothing.
func LogRows(src LogSources) LogTable {
	first := Today(src.From)
	rows := make([]placedRow, 0, len(src.Read.lines))
	lined := make(map[string]bool)
	for _, h := range src.Read.lines {
		place := h.line.At
		if sent, told := src.Stream.sentOf(&h.line); told {
			lined[h.line.Request], place = true, sent
		}
		rows = append(rows, placedRow{row: h.row, line: &h.line, session: h.line.CountedSession(), day: h.day, place: place})
	}
	for _, t := range toldAlone(src.Stream, lined) {
		rows = append(rows, placedRow{row: toldRow(t, src.Now), session: t.countedSession(), day: Today(t.Sent), place: t.Sent})
	}
	rows = slices.DeleteFunc(rows, func(p placedRow) bool { return p.day.Start.Before(first.Start) })
	slices.SortStableFunc(rows, func(a, b placedRow) int {
		return cmp.Or(a.day.Start.Compare(b.day.Start), a.place.Compare(b.place))
	})
	return tableOf(rows, first, src)
}

// hold notes the row p among those the table holds, shown or hidden: the
// time its request ran over; and, where the stream told its end, its line yet
// to come, the day it sits on, as awaiting its line.
func (t *LogTable) hold(p placedRow) {
	t.ran[p.row.Request] = append(t.ran[p.row.Request], runSpanOf(p.row))
	if p.row.Stage == RowEnded && !slices.ContainsFunc(t.awaited, func(d LedgerDay) bool { return d.Date == p.day.Date }) {
		t.awaited = append(t.awaited, p.day)
	}
}

// toldAlone are the requests the stream told of that it knows, as known has
// them, whose lines aren't among those read, lined holding the ids of those
// whose are, but those it told no sending of, which have no place, by when
// they were sent, then their ids.
func toldAlone(s Stream, lined map[string]bool) []Told {
	var alone []Told
	for t := range s.known() {
		if !lined[t.Request] && !t.Sent.IsZero() {
			alone = append(alone, t)
		}
	}
	slices.SortFunc(alone, func(a, b Told) int { return cmp.Or(a.Sent.Compare(b.Sent), strings.Compare(a.Request, b.Request)) })
	return alone
}

// tableOf is the table of rows, placed in order, from the day first: those
// src's filter hides left out, each day's rule before its first row shown,
// with the figures of the day that ended, and each day's counts; and, where
// first is the oldest day, or before it, the oldest day's line atop them.
func tableOf(rows []placedRow, first LedgerDay, src LogSources) LogTable {
	rules := dayRulesOf(src.Totals)
	table := LogTable{counts: make(map[string]LogCount), ran: make(map[string][]runSpan), first: first}
	if oldest := src.Oldest; oldest.reached(first) {
		table.Lines = append(table.Lines, LogLine{Oldest: &oldest})
	}
	var last *LedgerDay
	for _, p := range rows {
		table.hold(p)
		hid := src.Filter.hiding(p)
		c := table.counts[p.day.Date]
		c.add(p, hid)
		table.counts[p.day.Date] = c
		if hid != notHidden {
			continue
		}
		if last != nil && last.Date != p.day.Date {
			table.Lines = append(table.Lines, LogLine{Rule: rules.between(*last, p.day)})
		}
		table.Lines = append(table.Lines, LogLine{Row: p.row, line: p.line, day: p.day})
		last = &p.day
	}
	return table
}
