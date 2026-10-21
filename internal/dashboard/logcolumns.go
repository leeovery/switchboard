package dashboard

import (
	"slices"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// LogColumn is one of the Log's Requests' columns.
type LogColumn int

// The Log's Requests' columns, in the table's order: at, which always
// shows; those shown at first, to out; the extras f shows, worth first, in
// the panel's order; then stopped and routing, which take the room left.
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
// preferences have it; its cells: where they're fixed, them; of on, its
// least; of stopped, the fewest it takes whole; of routing, its natural; and
// of the session, none, as its directory sizes it, as sessionPart has it;
// and its alignment.
var logColumnFacts = [...]struct {
	name    string
	width   int
	aligned alignment
}{
	LogAt:        {"at", 8, alignLeft},
	LogSession:   {"session", 0, alignLeft},
	LogModel:     {"model", 9, alignLeft},
	LogOn:        {"on", labelLeast, alignLeft},
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

// least is the fewest cells the column takes whole, before it gives way:
// routing's routingLeast, and every other's cells.
func (c LogColumn) least() int {
	if c == LogRouting {
		return routingLeast
	}
	return logColumnFacts[c].width
}

// headCells are the cells the column's head takes.
func (c LogColumn) headCells() int {
	return ansi.StringWidth(c.Name())
}

// placed is the column placed from x, width cells wide.
func (c LogColumn) placed(x, width int) logPlace {
	return logPlace{column: c, x: x, width: width, aligned: logColumnFacts[c].aligned}
}

// The Requests' spacing, as log-requests-160 draws it: the column at starts
// at; the cells between two columns; the fewest cells routing is cut to, as
// with worth shown at 160 columns, from 136; how far short of the last
// column routing ends once it's whole, as at 160 columns, at 157; and the
// fewest cells the table's last column, stopped or routing alone, is cut to,
// stopped then saying its words as a phone's do, as new session's.
const (
	logLeft      = 2
	logColumnGap = 2
	routingLeast = 24
	routingShort = 2
	tailLeast    = 11
)

// requestsSpread is where a table of requests' rows lies across the
// terminal: from column left, its routing, once whole, ending short cells
// short of the last column; and, of a session's turn opened on a phone, its
// columns before stopped at the places given, and stopped, or routing alone,
// from tail to the last column.
type requestsSpread struct {
	left, short int
	phone       []logPlace
	tail        int
}

var (
	// logSpread is the Log's Requests', as log-requests-160 draws them.
	logSpread = requestsSpread{left: logLeft, short: routingShort}
	// turnSpread is a session's turn opened's, its requests' rows without
	// their session, as session-turn-opened-160 draws them: from 5, routing
	// whole from 112 to 141 at 160 columns; and on a phone, as the Log's
	// was, less the session (call 57), at 5, on 14, took ending at 27,
	// stopped from 31.
	turnSpread = requestsSpread{
		left: 5, short: 18,
		phone: []logPlace{LogAt.placed(5, 8), LogOn.placed(14, 8), LogTook.placed(22, 6)},
		tail:  31,
	}
)

// logLayout is the Log's Requests laid out across the terminal: the columns
// shown, in the table's order; whether routing folds into stopped, which
// then says the row's routing where it has words; whether stopped says its
// words briefly, as PhoneStopped says them; and whether the session names
// its directory after its id.
type logLayout struct {
	places               []logPlace
	folded, brief, named bool
}

// logPlace is a column as it's laid out: from column x, width cells wide,
// its words aligned in them.
type logPlace struct {
	column   LogColumn
	x, width int
	aligned  alignment
}

// logNaturals are the natural cells of the Requests' columns that name: of
// the session's directory, the widest the rows shown name, as
// views.SessionDirName names it, none where none names one; and of on, the
// widest account they're on, as labelColumnCells counts it.
type logNaturals struct {
	dir, on int
}

// with is n widened to the row r's, its account named as name names it.
func (n logNaturals) with(r views.RequestRow, name func(id string) string) logNaturals {
	n.dir = max(n.dir, ansi.StringWidth(views.SessionDirName(r.Dir)))
	n.on = max(n.on, labelColumnCells(name(r.Account)))
	return n
}

// logColumns lays the Log's Requests out across a terminal width cells wide,
// the columns ticked shown as they fit, those that name at the naturals
// given, as the spread lays them out.
func logColumns(width int, ticked LogTicks, n logNaturals) logLayout {
	return logSpread.laid(width, ticked.shown(), n)
}

// wide lays the columns ticked out across a terminal width cells wide as
// the spread has them, as logColumns does, on at its least.
func (s requestsSpread) wide(width int, ticked LogTicks) logLayout {
	return s.laid(width, ticked.shown(), logNaturals{})
}

// laid lays the columns shown out across width cells from the spread's left,
// those that name at the naturals given, the table running to the last
// column, as flex lays logParts out: each column at its parts' cells
// together, from where its first starts. Where the columns don't all fit,
// they give way as logNarrowing orders them, a turn at a time, until they
// do.
func (s requestsSpread) laid(width int, shown []LogColumn, n logNaturals) logLayout {
	parts := logParts(shown, n, s.short)
	flexed := make([]flex.Part, len(parts))
	for i, p := range parts {
		flexed[i] = p.Part
	}
	spans, _ := flex.Lay(width-s.left, flexed...)
	layout := logLayout{named: n.dir > 0}
	for i, p := range parts {
		span := spans[i]
		if span.Left {
			layout = layout.gaveWay(p)
		}
		if p.role == routingMargin || span.Left && span.Size == 0 {
			continue
		}
		if last := len(layout.places) - 1; last >= 0 && layout.places[last].column == p.column {
			layout.places[last].width += span.Size
			continue
		}
		layout.places = append(layout.places, p.column.placed(s.left+span.At, span.Size))
	}
	return layout
}

// gaveWay is l once the part given gave way: routing folded into stopped;
// stopped saying its words briefly; or the session naming its id alone.
func (l logLayout) gaveWay(p logPart) logLayout {
	switch {
	case p.column == LogRouting && p.role == wholeColumn:
		l.folded = true
	case p.column == LogStopped && p.role == tailPast:
		l.brief = true
	case p.column == LogSession:
		l.named = false
	}
	return l
}

// logRole is what of its column a part of a row of requests is.
type logRole int

const (
	// wholeColumn is the whole of it.
	wholeColumn logRole = iota
	// tailWords are the table's last column's first tailLeast cells, which
	// grow to the last column.
	tailWords
	// tailPast are its cells past them to its least, which give way.
	tailPast
	// routingMargin are the cells routing, whole, ends short of the last
	// column, of no column.
	routingMargin
)

// logPart is a part of a row of requests as flex lays it out, and what of
// its column it is.
type logPart struct {
	column LogColumn
	role   logRole
	flex.Part
}

// logParts are the parts of a row of the columns shown, in the table's
// order, each column two cells after the one before, giving way in turn as
// logNarrowing orders them:
//   - a column of fixed cells, as logColumnFacts has them, at them;
//   - the session, as sessionPart has it, its directory n.dir cells, giving
//     way to its id alone, in as many cells as its head;
//   - on, a column of names, at least labelLeast cells, its natural n.on;
//   - the last column, stopped, or routing alone, as two parts: its
//     tailLeast cells, which take the room left, and those past them to its
//     least, which give way;
//   - and routing beside stopped, from routingLeast to its natural, ending
//     short cells short of the last column once whole, and sharing the room
//     left with stopped, the odd cell its.
func logParts(shown []LogColumn, n logNaturals, short int) []logPart {
	turns := logNarrowing(shown)
	turn := func(c LogColumn) int { return slices.Index(turns, c) + 1 }
	tail := LogStopped
	if !slices.Contains(shown, tail) {
		tail = LogRouting
	}
	var parts []logPart
	for _, c := range shown {
		cells := logColumnFacts[c].width
		switch c {
		case tail:
			past := c.least() - tailLeast
			parts = append(parts,
				logPart{c, tailWords, flex.Part{Least: tailLeast, Natural: tailLeast, Grows: true, Gap: logColumnGap}},
				logPart{c, tailPast, flex.Part{Least: past, Natural: past, Leaves: turn(c)}},
			)
		case LogRouting:
			parts = append(parts,
				logPart{c, wholeColumn, flex.Part{Least: c.least(), Natural: cells, Grows: true, Odd: true, Leaves: turn(c), Gap: logColumnGap}},
				logPart{c, routingMargin, flex.Part{Natural: short, Leaves: turn(c)}},
			)
		case LogSession:
			parts = append(parts, logPart{c, wholeColumn, sessionPart(n.dir, turn(c), max(status.IDShown, c.headCells()))})
		case LogOn:
			parts = append(parts, logPart{c, wholeColumn, flex.Part{Least: cells, Natural: n.on, Gap: logColumnGap}})
		default:
			parts = append(parts, logPart{c, wholeColumn, flex.Part{Least: cells, Natural: cells, Leaves: turn(c), Gap: logColumnGap}})
		}
	}
	return parts
}

// sessionPart is a session's column as flex lays it out, two cells after
// the one before, of a directory dir cells wide, none where it names none:
// its id, and a cell on, its directory, in no fewer cells than its head;
// giving way in the turn given to the cells kept.
func sessionPart(dir, leaves, kept int) flex.Part {
	cells := status.IDShown
	if dir > 0 {
		cells += 1 + dir
	}
	cells = max(cells, LogSession.headCells())
	return flex.Part{Least: cells, Natural: cells, Leaves: leaves, Kept: kept, Gap: logColumnGap}
}

// logNarrowing is the order the columns shown give way in as the table
// narrows, as the design has it: the extras, the last first, so worth
// leaves last of them; then routing, folding into stopped, where both show;
// then first, read, written and out; then the last column, stopped, or
// routing alone, cut to its tailLeast cells; then model and status; and
// last, the session's directory, its id kept.
func logNarrowing(shown []LogColumn) []LogColumn {
	var order []LogColumn
	for _, c := range slices.Backward(shown) {
		if c.extra() {
			order = append(order, c)
		}
	}
	stopped, routing := slices.Contains(shown, LogStopped), slices.Contains(shown, LogRouting)
	if stopped && routing {
		order = append(order, LogRouting)
	}
	order = append(order, LogFirst, LogRead, LogWritten, LogOut)
	switch {
	case stopped:
		order = append(order, LogStopped)
	case routing:
		order = append(order, LogRouting)
	}
	order = append(order, LogModel, LogStatus, LogSession)
	return slices.DeleteFunc(order, func(c LogColumn) bool { return !slices.Contains(shown, c) })
}

// phoned lays the columns ticked out across a phone's width cells as the
// spread has them, as a session's turn opened does on a phone: its places,
// those ticked, and stopped, or routing alone, from its tail to the last
// column, briefly, routing folded into it where both are ticked.
func (s requestsSpread) phoned(width int, ticked LogTicks) logLayout {
	layout := logLayout{brief: true, folded: ticked.Shows(LogStopped) && ticked.Shows(LogRouting)}
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
