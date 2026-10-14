package dashboard

import "slices"

// LogColumn is one of the Log's Requests' columns.
type LogColumn int

// The Log's Requests' columns, in the table's order: at, which always
// shows; those shown at first, to out; the extras f shows, worth first, in
// the panel's order; then stopped and routing, which take the room past 160
// columns.
const (
	LogAt LogColumn = iota
	LogSession
	LogModel
	LogOn
	LogStatus
	LogFirst
	LogTook
	LogRead
	LogWritten
	LogOut
	LogWorth
	LogInput
	LogAttempts
	LogSize
	LogMessages
	LogTools
	LogThinking
	LogAgent
	LogRequestID
	LogStopped
	LogRouting
)

// alignment is how a column's words sit in its cells.
type alignment int

const (
	// alignLeft starts them at the column's start.
	alignLeft alignment = iota
	// alignRight ends them at its end, as a number is.
	alignRight
)

// logColumnFacts are each column's name, as its head, the f panel and the
// preferences have it; its width, the most stopped and routing take until
// the room past 160 columns is shared; and its alignment.
var logColumnFacts = [...]struct {
	name    string
	width   int
	aligned alignment
}{
	LogAt:        {"at", 8, alignLeft},
	LogSession:   {"session", 17, alignLeft},
	LogModel:     {"model", 9, alignLeft},
	LogOn:        {"on", 8, alignLeft},
	LogStatus:    {"status", 6, alignLeft},
	LogFirst:     {"first", 5, alignRight},
	LogTook:      {"took", 6, alignRight},
	LogRead:      {"read", 6, alignRight},
	LogWritten:   {"written", 7, alignRight},
	LogOut:       {"out", 5, alignRight},
	LogWorth:     {"worth", 6, alignRight},
	LogInput:     {"input", 6, alignRight},
	LogAttempts:  {"attempts", 8, alignRight},
	LogSize:      {"size", 6, alignRight},
	LogMessages:  {"messages", 8, alignRight},
	LogTools:     {"tools", 5, alignRight},
	LogThinking:  {"thinking", 8, alignRight},
	LogAgent:     {"agent", 16, alignLeft},
	LogRequestID: {"request id", 10, alignLeft},
	LogStopped:   {"stopped", 27, alignLeft},
	LogRouting:   {"routing", 30, alignLeft},
}

// EveryLogColumn are the Log's Requests' columns, in the table's order.
func EveryLogColumn() []LogColumn {
	columns := make([]LogColumn, len(logColumnFacts))
	for i := range columns {
		columns[i] = LogColumn(i)
	}
	return columns
}

// Name is the column's name, as its head, the f panel and the preferences
// have it, as "request id".
func (c LogColumn) Name() string {
	return logColumnFacts[c].name
}

// extra reports whether the column is one f shows that's off at first:
// worth, and those after it before stopped.
func (c LogColumn) extra() bool {
	return LogWorth <= c && c <= LogRequestID
}

// least is the fewest cells the column is cut to before the table narrows
// further: routing's routingLeast, and every other's width, stopped's as at
// 160 columns.
func (c LogColumn) least() int {
	if c == LogRouting {
		return routingLeast
	}
	return logColumnFacts[c].width
}

// placed is the column placed from x, width cells wide.
func (c LogColumn) placed(x, width int) logPlace {
	return logPlace{column: c, x: x, width: width, aligned: logColumnFacts[c].aligned}
}

// The Requests' spacing, as log-requests-160 draws it: the column at starts
// at; the cells between two columns; the fewest cells routing is cut to, as
// with worth shown at 160 columns, from 136; and how far short of the last
// column routing ends once it's whole, as at 160 columns, at 157.
const (
	logLeft      = 2
	logColumnGap = 2
	routingLeast = 24
	routingShort = 2
)

// requestsSpread is where a table of requests' rows lies across the
// terminal: on a wide one, from column left, its routing, once whole, ending
// short cells short of the last column; on a phone's, its columns before
// stopped at the places given, and stopped, or routing alone, from tail to
// the last column.
type requestsSpread struct {
	left, short int
	phone       []logPlace
	tail        int
}

var (
	// logSpread is the Log's Requests', as log-requests-160 and
	// log-requests-phone-52 draw them: on a phone, at; the session's id
	// alone; on; and took, ending at 33; stopped from 37.
	logSpread = requestsSpread{
		left: logLeft, short: routingShort,
		phone: []logPlace{LogAt.placed(2, 8), LogSession.placed(11, 4), LogOn.placed(20, 8), LogTook.placed(28, 6)},
		tail:  37,
	}
	// turnSpread is a session's turn opened's, its requests' rows without
	// their session, as session-turn-opened-160 draws them: from 5, routing
	// whole from 112 to 141 at 160 columns; and on a phone, as the Log's less
	// the session (call 57), at 5, on 14, took ending at 27, stopped from
	// 31.
	turnSpread = requestsSpread{
		left: 5, short: 18,
		phone: []logPlace{LogAt.placed(5, 8), LogOn.placed(14, 8), LogTook.placed(22, 6)},
		tail:  31,
	}
)

// logLayout is the Log's Requests laid out across the terminal: the columns
// shown, in the table's order; whether routing folds into stopped, which
// then says the row's routing where it has words; and whether it's a
// phone's, its session the id alone and its stopped as PhoneStopped says it.
type logLayout struct {
	places        []logPlace
	folded, phone bool
}

// logPlace is a column as it's laid out: from column x, width cells wide,
// its words aligned in them.
type logPlace struct {
	column   LogColumn
	x, width int
	aligned  alignment
}

// logColumns lays the Log's Requests out across a terminal width cells wide,
// the columns ticked shown as they fit. The table runs to its last column,
// the width's last, not a margin short of it.
//
// From 100 columns, each sits two cells after the one before, at its width,
// but stopped and routing, which tailed lays out. Where they don't all fit,
// the table narrows as logNarrowing orders, a step at a time, until they
// do: an extra leaves where routing would be cut under its least; then,
// with the columns shown at first, routing is cut to the room left under
// 158 columns, to its least at 152; under that it folds into stopped, and
// first, read, written and out leave, each where stopped, run to the last
// column, would have fewer than its 27 cells. On a phone, as Phone tells
// one, the columns are at a phone's places, those ticked, and stopped, or
// routing alone, runs from 37 to the last column, as logSpread has them.
func logColumns(width int, ticked LogTicks) logLayout {
	if Phone(width) {
		return logSpread.phoned(width, ticked)
	}
	return logSpread.wide(width, ticked)
}

// wide lays the columns ticked out across a terminal width cells wide as
// the spread has them, from its left, narrowing as logColumns says.
func (s requestsSpread) wide(width int, ticked LogTicks) logLayout {
	shown, folded := ticked.shown(), false
	layout, fits := s.laid(width, shown, folded)
	for _, leaving := range logNarrowing(shown) {
		if fits {
			break
		}
		folded = folded || leaving == LogRouting
		shown = slices.DeleteFunc(shown, func(c LogColumn) bool { return c == leaving })
		layout, fits = s.laid(width, shown, folded)
	}
	return layout
}

// logNarrowing is the order the columns shown leave in as the table
// narrows: the extras, the last first, so worth leaves last of them; then
// routing, folding into stopped, where both show; then first, read,
// written and out. From 100 columns, model, status and the session's
// directory, which the design has leave next, always fit.
func logNarrowing(shown []LogColumn) []LogColumn {
	var order []LogColumn
	for _, c := range slices.Backward(shown) {
		if c.extra() {
			order = append(order, c)
		}
	}
	if slices.Contains(shown, LogStopped) && slices.Contains(shown, LogRouting) {
		order = append(order, LogRouting)
	}
	for _, c := range []LogColumn{LogFirst, LogRead, LogWritten, LogOut} {
		if slices.Contains(shown, c) {
			order = append(order, c)
		}
	}
	return order
}

// laid lays the columns shown out across width cells, routing folded into
// stopped where folded says: each before stopped at its width, two cells
// after the one before, from the spread's left; then stopped and routing,
// as tailed lays them out. It reports whether they fit.
func (s requestsSpread) laid(width int, shown []LogColumn, folded bool) (logLayout, bool) {
	layout := logLayout{folded: folded}
	x := s.left
	var tail []LogColumn
	for _, c := range shown {
		if c == LogStopped || c == LogRouting {
			tail = append(tail, c)
			continue
		}
		layout.places = append(layout.places, c.placed(x, logColumnFacts[c].width))
		x += logColumnFacts[c].width + logColumnGap
	}
	places, fits := s.tailed(width, x, tail)
	layout.places = append(layout.places, places...)
	return layout, fits
}

// tailed lays the tail of a table width cells wide out from x, stopped and
// routing, those of them shown, and reports whether each has its least.
// Both shown sit side by side: stopped at its width and routing at its,
// where that leaves routing the spread's short cells short of the last
// column; with less room, routing cut to the room to the last column; with
// more, the two sharing what's past that evenly, the odd cell routing's.
// One alone, as stopped is with routing folded into it, runs to the last
// column. With neither, it reports whether the columns before them fit.
func (s requestsSpread) tailed(width, x int, tail []LogColumn) ([]logPlace, bool) {
	switch len(tail) {
	case 0:
		return nil, x-logColumnGap <= width
	case 1:
		return []logPlace{tail[0].placed(x, width-x)}, width-x >= tail[0].least()
	}
	stopped := logColumnFacts[LogStopped].width
	room := width - x - stopped - logColumnGap
	past := max(room-logColumnFacts[LogRouting].width-s.short, 0)
	routing := min(room, logColumnFacts[LogRouting].width) + past - past/2
	stopped += past / 2
	return []logPlace{LogStopped.placed(x, stopped), LogRouting.placed(x+stopped+logColumnGap, routing)}, routing >= routingLeast
}

// phoned lays the columns ticked out across a phone's width cells as the
// spread has them, as logColumns does under 100 columns.
func (s requestsSpread) phoned(width int, ticked LogTicks) logLayout {
	layout := logLayout{phone: true, folded: ticked.Shows(LogStopped) && ticked.Shows(LogRouting)}
	for _, p := range s.phone {
		if ticked.Shows(p.column) {
			layout.places = append(layout.places, p)
		}
	}
	tail := LogStopped
	if !ticked.Shows(tail) {
		tail = LogRouting
	}
	if ticked.Shows(tail) && width > s.tail {
		layout.places = append(layout.places, tail.placed(s.tail, width-s.tail))
	}
	return layout
}
