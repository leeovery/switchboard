package views

import (
	"cmp"
	"iter"
	"slices"
	"sort"
	"strings"
	"time"
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
	// Read holds the ledger's lines of those days, each row priced, placed
	// and told, as the Log's read grows.
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

// LogTable is the Log's Requests over the days it spans: its lines, the
// oldest first, as Line gives each; the row of each request shown, each
// request once, by its id; before the first row shown of each day after the
// first with one, that day's rule; and, where the table reaches the oldest
// day, the oldest day's line atop them all. Its rows of lines are the read's,
// as LogLines tables them, never copied; the stream's own lie over them,
// each where it sits among them.
type LogTable struct {
	lines LogLines
	// sections are its days with a row shown, the oldest first, each with
	// its rule and its rows; oldest the oldest day's line atop them, where
	// it has one; and n how many lines it has.
	sections []tableSection
	oldest   *OldestDay
	n        int
	// counts are what the lines of each day the stream told of a request on
	// count, the stream's rows among them, by the day's date: of any other
	// day, as the lines' own count it.
	counts map[string]LogCount
	// awaited are the days the lines are filed on of its rows, shown or
	// hidden, of a request whose end the stream told, its line yet to come;
	// and told the keys of every row the stream told of, shown or hidden.
	awaited []LedgerDay
	told    []LineKey
}

// tableSection is a day of the table's with a row shown: where its first
// line falls among the table's, the oldest day's line aside; the day; its
// rule, nil for the first day shown; the rows of its lines shown, in order;
// the stream's rows shown on it, each with the place among those it comes
// before, in order; and how many lines it has, its rule among them.
type tableSection struct {
	start int
	day   LedgerDay
	rule  *DayRule
	rows  chunked[*loggedLine]
	told  []toldPlaced
	n     int
}

// toldPlaced is a row the stream told of, placed on its day, among the
// rows of the day's lines before the one at the place given.
type toldPlaced struct {
	placedRow
	before int
}

// LogLine is a line of the Log's table: a request's row; where Rule is set,
// the rule where one day's rows end and the next's begin; or, where Oldest
// is set, the oldest day's line, which says how long the ledger keeps its
// lines.
type LogLine struct {
	Row    RequestRow
	Rule   *DayRule
	Oldest *OldestDay
	// logged is the Log's line the row is of, and record, of a turn's
	// request, its line's record, each nil before it comes; day the local
	// day the row sits on; and place where it sits within it, as placedRow
	// has it.
	logged *loggedLine
	record *lineRecord
	day    LedgerDay
	place  time.Time
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

// Len is how many lines t has.
func (t LogTable) Len() int {
	return t.n
}

// Line is t's line at the place given among them, the oldest first.
func (t LogTable) Line(i int) LogLine {
	if t.oldest != nil {
		if i == 0 {
			return LogLine{Oldest: t.oldest}
		}
		i--
	}
	k := sort.Search(len(t.sections), func(k int) bool { return t.sections[k].start > i }) - 1
	return t.sections[k].line(i - t.sections[k].start)
}

// All are t's lines, the oldest first, each with its place among them.
func (t LogTable) All() iter.Seq2[int, LogLine] {
	return func(yield func(int, LogLine) bool) {
		at := 0
		if t.oldest != nil {
			if !yield(0, LogLine{Oldest: t.oldest}) {
				return
			}
			at = 1
		}
		for _, s := range t.sections {
			for o := range s.n {
				if !yield(at+s.start+o, s.line(o)) {
					return
				}
			}
		}
	}
}

// line is the section's line at the place o among its own: its rule first,
// where it has one, then its rows, each of the stream's among those of its
// lines before the one it comes before.
func (s tableSection) line(o int) LogLine {
	if s.rule != nil {
		if o == 0 {
			return LogLine{Rule: s.rule}
		}
		o--
	}
	before := 0
	for k, p := range s.told {
		switch at := p.before + k; {
		case at == o:
			return LogLine{Row: p.row, day: p.day, place: p.place}
		case at > o:
			return s.lined(o - before)
		}
		before++
	}
	return s.lined(o - before)
}

// lined is the line of the row of the section's line at the place given
// among its rows of lines.
func (s tableSection) lined(i int) LogLine {
	l := s.rows.at(i)
	return LogLine{Row: l.row, logged: l, day: s.day, place: l.place}
}

// placeOf is the place among the section's own lines of the row of its line
// at the place given among its rows of lines: after its rule, and after the
// stream's rows that come before it.
func (s tableSection) placeOf(i int) int {
	o := i
	if s.rule != nil {
		o++
	}
	for _, p := range s.told {
		if p.before > i {
			break
		}
		o++
	}
	return o
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
	if c, ok := t.counts[date]; ok {
		return c
	}
	if i, ok := t.lines.dayAt(date); ok {
		return t.lines.days[i].count
	}
	return LogCount{}
}

// placedRow is a row the stream told of as the table places it before its
// line comes: on the local day it was sent on, and within it, where it was
// sent. Filed is the day its line is filed on, as Told.FiledOn has it.
type placedRow struct {
	row        RequestRow
	day, filed LedgerDay
	place      time.Time
}

// LogRows builds the Log's Requests from src: a row of each request, once:
// from its line, where it has come, as FiledAs knows it; else from what the
// stream told of it, in flight, ended or lost, as LogLinesOf tables the lines
// and Rows takes the stream's rows in among them. A request the router
// answers itself is never on the stream, so its row comes with its line.
//
// A row sits on the day its request arrived on, as the ledger files its
// line, so a request in flight as a day ends sits above the next day's rule;
// before its line comes, on the day it was sent, the one time the stream
// tells. Within its day it sits where it was sent, where the stream told
// that as its line was taken in, so a row stays where it sat in flight as
// its line fills it in, though the line's arrival is a moment earlier; else
// where it arrived. Rows placed alike keep the order of the lines taken in,
// then of the stream's alone.
//
// Its rows of lines are as src's Read holds them, priced, placed and told as
// their lines were taken in: it tables them, and builds the stream's own, at
// each call, reading and pricing nothing.
func LogRows(src LogSources) LogTable {
	return LogLinesOf(LogLines{}, src).Rows(src)
}

// LogLines are the Log's Requests of the lines read alone, through a
// filter, each tabled once, as LogLinesOf tables it: what Rows takes the
// stream's own rows in among, so what the stream tells builds nothing of
// the lines afresh, and the read growing tables only what it gained. Each
// day holds its rows shown in chunks, as chunked holds them, so tabling a
// line copies a chunk of its day's, never the table.
type LogLines struct {
	// read is the read it tables, filter what it shows of it, and first the
	// first day it spans; days the days it holds lines of, the oldest first;
	// and copied how many chunks its days' rows have had copied, as they
	// were tabled.
	read   LogRead
	filter LogFilter
	first  LedgerDay
	days   []tableDay
	copied int
}

// tableDay is a day of the table's lines: its rows shown, in order, as
// placedOrder orders them, and what its lines count, those the filter hides
// among them.
type tableDay struct {
	day   LedgerDay
	rows  chunked[*loggedLine]
	count LogCount
}

// LogLinesOf are the Log's Requests of src's lines alone, as LogRows tables
// them: was, the lines tabled of a read src's grew from, through the same
// filter, from the same day or one before src's first, taking in what the
// read gained since and what it told again; else tabled afresh.
func LogLinesOf(was LogLines, src LogSources) LogLines {
	first := Today(src.From)
	if !was.followedBy(src.Read, src.Filter, first) {
		return linesOf(src.Read, src.Filter, first)
	}
	return was.caughtUp(src.Read, first)
}

// Filter is the filter l tables its read through: the zero LogFilter of
// lines never tabled.
func (l LogLines) Filter() LogFilter {
	return l.filter
}

// Copied is how many chunks l's days' rows have had copied, as they were
// tabled since l was first tabled afresh: a read that gained k lines copies
// no more than a chunk for each, and one for each it told again.
func (l LogLines) Copied() int {
	return l.copied
}

// followedBy reports whether l takes in what the read r gained, tabling it
// through filter from the day first: l is of a read r grew from by the
// changes r keeps, through the same filter, from first or a day before it.
func (l LogLines) followedBy(r LogRead, filter LogFilter, first LedgerDay) bool {
	switch {
	case l.read.grew == nil || l.read.grew != r.grew || !l.filter.Equal(filter) || first.Date < l.first.Date:
		return false
	case l.read.gen == r.gen:
		return true
	}
	return len(r.changes) > 0 && r.changes[0].gen <= l.read.gen+1
}

// linesOf are the lines of r tabled afresh through filter from the day
// first.
func linesOf(r LogRead, filter LogFilter, first LedgerDay) LogLines {
	l := LogLines{read: r, filter: filter, first: first}
	for _, d := range r.days {
		if d.day.Date < first.Date {
			continue
		}
		day := tableDay{day: d.day}
		var shown []*loggedLine
		for _, line := range d.lines.all() {
			hid := filter.hiding(&line.row)
			day.count.add(&line.row, hid)
			if hid == notHidden {
				shown = append(shown, line)
			}
		}
		slices.SortFunc(shown, placedOrder)
		day.rows = chunkedOf(shown)
		l.days = append(l.days, day)
	}
	return l
}

// caughtUp is l with what the read r gained since l was tabled taken in,
// from the day first on: the days r or first no longer span let go; each
// line r took in since tabled on its day, counted, and where the filter
// shows it, placed among the rows shown, as placedOrder orders them; and
// each line r told again since in place of its row shown.
func (l LogLines) caughtUp(r LogRead, first LedgerDay) LogLines {
	was := l.read
	l.read, l.first = r, first
	l.days = slices.DeleteFunc(slices.Clone(l.days), func(d tableDay) bool {
		_, held := r.dayAt(d.day.Date)
		return !held || d.day.Date < first.Date
	})
	counted := make(map[string]bool)
	for _, d := range r.days {
		if d.day.Date < first.Date {
			continue
		}
		at := d.lines.search(func(line *loggedLine) bool { return line.seq > was.seq })
		for _, line := range d.lines.from(at) {
			l.tabled(d.day, line, counted)
		}
	}
	for _, c := range r.changes {
		if c.gen <= was.gen {
			continue
		}
		for _, line := range c.retold {
			if line.seq <= was.seq {
				l.retold(line)
			}
		}
	}
	return l
}

// tabled has l, whose days are its own, table line, a line of day's: counted
// on its day, the day's count its own where counted doesn't say it's been
// made so, and where the filter shows it, placed among the day's rows shown.
func (l *LogLines) tabled(day LedgerDay, line *loggedLine, counted map[string]bool) {
	i, held := l.dayAt(day.Date)
	if !held {
		l.days = slices.Insert(l.days, i, tableDay{day: day})
	}
	d := &l.days[i]
	if !counted[day.Date] {
		d.count, counted[day.Date] = d.count.cloned(), true
	}
	hid := l.filter.hiding(&line.row)
	d.count.add(&line.row, hid)
	if hid != notHidden {
		return
	}
	copied := d.rows.copied
	d.rows = d.rows.inserted(d.rows.search(placedAfter(line)), line)
	l.copied += d.rows.copied - copied
}

// retold has l, whose days are its own, show line, told again, in place of
// its row as it was told before, where it's shown.
func (l *LogLines) retold(line *loggedLine) {
	i, held := l.dayAt(Today(line.row.At).Date)
	if !held || l.filter.hiding(&line.row) != notHidden {
		return
	}
	d := &l.days[i]
	at := d.rows.search(placedFrom(line))
	if at == d.rows.len() || d.rows.at(at).seq != line.seq {
		return
	}
	copied := d.rows.copied
	d.rows = d.rows.set(at, line)
	l.copied += d.rows.copied - copied
}

// placedOrder orders a day's rows: by where each sits, then the order the
// lines were taken in.
func placedOrder(a, b *loggedLine) int {
	return cmp.Or(a.place.Compare(b.place), cmp.Compare(a.seq, b.seq))
}

// placedAfter reports of a row of a day's whether it's placed after line's,
// as placedOrder has it.
func placedAfter(line *loggedLine) func(*loggedLine) bool {
	return func(o *loggedLine) bool { return placedOrder(o, line) > 0 }
}

// placedFrom reports of a row of a day's whether it's line's or placed after
// it, as placedOrder has it.
func placedFrom(line *loggedLine) func(*loggedLine) bool {
	return func(o *loggedLine) bool { return placedOrder(o, line) >= 0 }
}

// dayAt is the place among l's days of the day with the given date,
// reporting false where l holds none of it, the place it would take.
func (l LogLines) dayAt(date string) (int, bool) {
	return slices.BinarySearchFunc(l.days, date, func(d tableDay, date string) int { return strings.Compare(d.day.Date, date) })
}

// Rows are the Log's Requests of src, as LogRows has them: the lines' as l
// tables them, and in among them, the rows of the requests src's stream told
// of that they show, as toldAlone has them, each after the lines' placed
// alike; each day's rule before its first row shown; and, where they reach
// the oldest day, the oldest day's line atop them. The stream's rows are
// counted and held, shown or hidden, and those shown laid over the lines'
// own, which it leaves as they are.
func (l LogLines) Rows(src LogSources) LogTable {
	t := LogTable{lines: l, counts: make(map[string]LogCount)}
	if oldest := src.Oldest; oldest.reached(l.first) {
		t.oldest, t.n = &oldest, 1
	}
	told := t.toldOn(src)
	rules := dayRulesOf(src.Totals)
	var last *LedgerDay
	at := 0
	for _, date := range l.datesWith(told) {
		s := tableSection{day: dayOf(date), start: at}
		if i, held := l.dayAt(date); held {
			s.day, s.rows = l.days[i].day, l.days[i].rows
		}
		for _, p := range told[date] {
			s.told = append(s.told, toldPlaced{placedRow: p, before: s.rows.search(func(o *loggedLine) bool { return o.place.After(p.place) })})
		}
		s.n = s.rows.len() + len(s.told)
		if s.n == 0 {
			continue
		}
		if last != nil {
			s.rule, s.n = rules.between(*last, s.day), s.n+1
		}
		t.sections, at, last = append(t.sections, s), at+s.n, &s.day
	}
	t.n += at
	return t
}

// toldOn are the rows of the requests src's stream told of that the lines
// don't hold, as toldAlone has them, each placed, counted on its day with
// the day's lines, and held, as hold has it: those shown, by their days'
// dates, each day's in order.
func (t *LogTable) toldOn(src LogSources) map[string][]placedRow {
	shown := make(map[string][]placedRow)
	for _, told := range toldAlone(src.Stream, src.Read, t.lines.first) {
		filed, _ := told.FiledOn()
		p := placedRow{row: toldRow(told, src.Now), day: toldOn(told), filed: filed, place: told.Sent}
		t.hold(p)
		c, counted := t.counts[p.day.Date]
		if !counted {
			c = t.Count(p.day.Date).cloned()
		}
		hid := t.lines.filter.hiding(&p.row)
		c.add(&p.row, hid)
		t.counts[p.day.Date] = c
		if hid == notHidden {
			shown[p.day.Date] = append(shown[p.day.Date], p)
		}
	}
	return shown
}

// datesWith are the dates of l's days and of those the stream's rows told
// are shown on, in order, each once.
func (l LogLines) datesWith(told map[string][]placedRow) []string {
	dates := make([]string, 0, len(l.days)+len(told))
	for _, d := range l.days {
		dates = append(dates, d.day.Date)
	}
	for date := range told {
		dates = append(dates, date)
	}
	slices.Sort(dates)
	return slices.Compact(dates)
}

// hold notes the row p the stream told of among those the table holds,
// shown or hidden: its key; and, where the stream told its end, its line yet
// to come, the day its line is filed on, as awaiting its line.
func (t *LogTable) hold(p placedRow) {
	t.told = append(t.told, LineKey{id: p.row.Request, ran: runSpanOf(p.row), place: p.place})
	if p.row.Stage == RowEnded && !slices.ContainsFunc(t.awaited, func(d LedgerDay) bool { return d.Date == p.filed.Date }) {
		t.awaited = append(t.awaited, p.filed)
	}
}

// ShowsTold reports whether the Log's Requests, over the days from the local
// day first, show the row the stream told of the request t, as Rows takes it
// in among the lines: one it told the sending of, its line not among those r
// holds, as holdsLineOf has it, sent on a day they span, as toldOn places
// it.
func (r LogRead) ShowsTold(t Told, first LedgerDay) bool {
	return !t.Sent.IsZero() && !r.holdsLineOf(t) && !toldOn(t).Start.Before(first.Start)
}

// toldOn is the local day the row the stream told of the request t is
// placed on till its line comes: the day it was sent on.
func toldOn(t Told) LedgerDay {
	return Today(t.Sent)
}

// toldAlone are the requests the stream told of that it knows, as known has
// them, whose rows the Log's Requests over the days from first show, read
// holding none of their lines, as ShowsTold has it, by when they were sent,
// then their ids.
func toldAlone(s Stream, read LogRead, first LedgerDay) []Told {
	var alone []Told
	for t := range s.known() {
		if read.ShowsTold(t, first) {
			alone = append(alone, t)
		}
	}
	slices.SortFunc(alone, func(a, b Told) int { return cmp.Or(a.Sent.Compare(b.Sent), strings.Compare(a.Request, b.Request)) })
	return alone
}
