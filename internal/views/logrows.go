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
	// Totals are the figures of the days, as TotalsOn gives them: a day's
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
	// hidden, by their ids; and awaited the days the lines are filed on of
	// its rows, shown or hidden, of a request whose end the stream told, its
	// line yet to come.
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
	// line is the line the row is of, nil before it comes; day the local
	// day the row sits on; and place where it sits within it, as placedRow
	// has it.
	line  *ledger.Line
	day   LedgerDay
	place time.Time
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

// Awaited are the dates of the days before the day given that the line is
// filed on, as Told.FiledOn has it, of a row the table holds, shown or
// hidden, of a request whose end the stream told, its line yet to come, each
// once: a line is filed under the day its request arrived on, so a later
// day's lines never hold the line of a request across midnight.
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
// arrived. Filed is the day its line is filed on, as the ledger files it,
// or before it comes, as Told.FiledOn has it.
type placedRow struct {
	row        RequestRow
	line       *ledger.Line
	day, filed LedgerDay
	place      time.Time
}

// LogRows builds the Log's Requests from src: a row of each request, once:
// from its line, where it has come, as FiledAs knows it; else from what the
// stream told of it, in flight, ended or lost, as LogLinesOf places and
// tables the lines and Rows takes the stream's rows in among them. A
// request the router answers itself is never on the stream, so its row
// comes with its line.
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
	return LogLinesOf(LogLines{}, src).Rows(src)
}

// LogLines are the Log's Requests of the lines read alone, through a
// filter, each placed once, as LogLinesOf places it: what Rows takes the
// stream's own rows in among, so what the stream tells builds nothing of
// the lines afresh, and the read growing places none of them again.
type LogLines struct {
	table  LogTable
	places map[heldLine]time.Time
}

// LogLinesOf are the Log's Requests of src's lines alone, as LogRows places
// and tables them, each line placed as was placed it, where it did, so a
// row stays where it sat as the read grows, and as the stream lets its
// request go: else where it was sent, where src's stream told that, as
// sentOf has it, else where it arrived.
func LogLinesOf(was LogLines, src LogSources) LogLines {
	first := Today(src.From)
	places := make(map[heldLine]time.Time, len(src.Read.lines))
	rows := make([]placedRow, 0, len(src.Read.lines))
	for _, h := range src.Read.lines {
		key := heldLine{request: h.line.Request, at: h.line.At.UnixNano()}
		place, placed := was.places[key]
		if !placed {
			place = h.line.At
			if sent, told := src.Stream.sentOf(&h.line); told {
				place = sent
			}
		}
		places[key] = place
		if !h.day.Start.Before(first.Start) {
			rows = append(rows, placedRow{row: h.row, line: &h.line, day: h.day, filed: h.day, place: place})
		}
	}
	slices.SortStableFunc(rows, func(a, b placedRow) int { return a.compare(b.day, b.place) })
	return LogLines{table: tableOf(rows, first, src), places: places}
}

// Rows are the Log's Requests of src, as LogRows has them: the lines' as l
// holds them, and in among them, the rows of the requests src's stream told
// of whose lines src's Read doesn't hold, as toldAlone has them, each after
// the lines' placed alike, as taken in builds them.
func (l LogLines) Rows(src LogSources) LogTable {
	var told []placedRow
	for _, t := range toldAlone(src.Stream, src.Read) {
		filed, _ := t.FiledOn()
		p := placedRow{row: toldRow(t, src.Now), day: Today(t.Sent), filed: filed, place: t.Sent}
		if !p.day.Start.Before(l.table.first.Start) {
			told = append(told, p)
		}
	}
	if len(told) == 0 {
		return l.table
	}
	return l.table.taken(told, src)
}

// compare orders the row p against a row placed at place on the local day
// given: by day, then within it by place.
func (p placedRow) compare(day LedgerDay, place time.Time) int {
	return cmp.Or(p.day.Start.Compare(day.Start), p.place.Compare(place))
}

// taken is the table t with the rows given, oldest first, taken in among
// its own, as tableOf would table them all, its own first where placed
// alike: those src's filter hides left out, each counted and held; each
// day's rule before its first row shown, made again as the rows taken in
// cross days. What t holds it leaves as it was.
func (t LogTable) taken(rows []placedRow, src LogSources) LogTable {
	in := LogTable{counts: maps.Clone(t.counts), ran: maps.Clone(t.ran), awaited: slices.Clone(t.awaited), first: t.first}
	in.Lines = make([]LogLine, 0, len(t.Lines)+2*len(rows))
	rules := dayRulesOf(src.Totals)
	var last *LedgerDay
	show := func(l LogLine) {
		if last != nil && last.Date != l.day.Date {
			in.Lines = append(in.Lines, LogLine{Rule: rules.between(*last, l.day)})
		}
		in.Lines = append(in.Lines, l)
		last = &l.day
	}
	take := func(p placedRow) {
		if in.counted(p, src.Filter) {
			show(LogLine{Row: p.row, line: p.line, day: p.day, place: p.place})
		}
	}
	i := 0
	for _, l := range t.Lines {
		switch {
		case l.Oldest != nil:
			in.Lines = append(in.Lines, l)
			continue
		case l.Rule != nil:
			continue
		}
		for ; i < len(rows) && rows[i].compare(l.day, l.place) < 0; i++ {
			take(rows[i])
		}
		show(l)
	}
	for _, p := range rows[i:] {
		take(p)
	}
	return in
}

// counted holds and counts the row p among the table's, as tableOf does,
// reporting whether filter shows it.
func (t *LogTable) counted(p placedRow, filter LogFilter) bool {
	t.hold(p)
	hid := filter.hiding(p)
	c := t.counts[p.day.Date]
	c.add(p, hid)
	t.counts[p.day.Date] = c
	return hid == notHidden
}

// hold notes the row p among those the table holds, shown or hidden: the
// time its request ran over; and, where the stream told its end, its line yet
// to come, the day its line is filed on, as awaiting its line.
func (t *LogTable) hold(p placedRow) {
	t.ran[p.row.Request] = append(slices.Clip(t.ran[p.row.Request]), runSpanOf(p.row))
	if p.row.Stage == RowEnded && !slices.ContainsFunc(t.awaited, func(d LedgerDay) bool { return d.Date == p.filed.Date }) {
		t.awaited = append(t.awaited, p.filed)
	}
}

// toldAlone are the requests the stream told of that it knows, as known has
// them, whose lines read doesn't hold, as holdsLineOf has it, but those it
// told no sending of, which have no place, by when they were sent, then
// their ids.
func toldAlone(s Stream, read LogRead) []Told {
	var alone []Told
	for t := range s.known() {
		if !t.Sent.IsZero() && !read.holdsLineOf(t) {
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
		if !table.counted(p, src.Filter) {
			continue
		}
		if last != nil && last.Date != p.day.Date {
			table.Lines = append(table.Lines, LogLine{Rule: rules.between(*last, p.day)})
		}
		table.Lines = append(table.Lines, LogLine{Row: p.row, line: p.line, day: p.day, place: p.place})
		last = &p.day
	}
	return table
}
