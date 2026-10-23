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
// preferences have it; its cells: where they're fixed, them, the session's
// its directory whole; of on, none, its names' as headedNames has them; of
// stopped, the fewest it takes whole; and of routing, its natural; and its
// alignment.
var logColumnFacts = [...]struct {
	name    string
	width   int
	aligned alignment
}{
	LogAt:        {"at", 8, alignLeft},
	LogSession:   {"session", sessionColumnCells, alignLeft},
	LogModel:     {"model", 9, alignLeft},
	LogOn:        {"on", 0, alignLeft},
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
// short of the last column.
type requestsSpread struct {
	left, short int
}

var (
	// logSpread is the Log's Requests', as log-requests-160 draws them.
	logSpread = requestsSpread{left: logLeft, short: routingShort}
	// turnSpread is a session's turn opened's, its requests' rows without
	// their session, as session-turn-opened-160 draws them: from 5, routing
	// whole from 112 to 141 at 160 columns.
	turnSpread = requestsSpread{left: 5, short: 18}
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

// logColumns lays the Log's Requests out across a terminal width cells wide,
// the columns ticked shown as they fit, on's natural the cells given, as the
// spread lays them out.
func logColumns(width int, ticked LogTicks, on int) logLayout {
	return logSpread.laid(width, ticked.shown(), on)
}

// laid lays the columns shown out across width cells from the spread's left,
// on's natural the cells given, the table running to the last column, in two
// passes of flex, as decided and secondPass have them: the first decides
// what shows, and the second lays out only that. Each column takes its
// parts' cells together, from where its first starts: stopped's, or routing
// alone's, two where its cells past its tailLeast stay.
func (s requestsSpread) laid(width int, shown []LogColumn, on int) logLayout {
	layout, kept := s.decided(width, shown, on)
	spans, _ := flex.Lay(width-s.left, flexed(kept)...)
	for i, p := range kept {
		if p.role == routingMargin {
			continue
		}
		if last := len(layout.places) - 1; last >= 0 && layout.places[last].column == p.column {
			layout.places[last].width += spans[i].Size
			continue
		}
		layout.places = append(layout.places, p.column.placed(s.left+spans[i].At, spans[i].Size))
	}
	return layout
}

// decided is the first pass: logParts laid across width cells from the
// spread's left, giving way as logNarrowing orders them, a turn at a time,
// until they fit, on holding its natural until its turn. It returns the
// layout as what gave way leaves it, with no columns placed yet, and the
// parts that show, each as secondPass has it.
func (s requestsSpread) decided(width int, shown []LogColumn, on int) (logLayout, []logPart) {
	parts := logParts(shown, on, s.short)
	spans, _ := flex.Lay(width-s.left, flexed(parts)...)
	layout := logLayout{named: true}
	var kept []logPart
	for i, p := range parts {
		if spans[i].Left {
			layout = layout.gaveWay(p)
		}
		if k, ok := p.secondPass(spans[i]); ok {
			kept = append(kept, k)
		}
	}
	return layout, kept
}

// secondPass is the part as the second pass lays it, of where the first
// left it, as flex's Decided has it, so on is free from its least to its
// natural, and takes what's past the leasts before any other. It reports
// false where the part is gone.
func (p logPart) secondPass(span flex.Span) (logPart, bool) {
	part, shown := p.Decided(span)
	p.Part = part
	return p, shown
}

// flexed are the parts as flex has them.
func flexed(parts []logPart) []flex.Part {
	flexed := make([]flex.Part, len(parts))
	for i, p := range parts {
		flexed[i] = p.Part
	}
	return flexed
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
//   - a column of fixed cells, as logColumnFacts has them, at them, giving
//     way to what it keeps, as keeps has it;
//   - on, a column of names under its head, as headedNames has it of its
//     natural on, holding it until its turn;
//   - the last column, stopped, or routing alone, as two parts: its
//     tailLeast cells, which take the room left, and those past them to its
//     least, which give way;
//   - and routing beside stopped, from routingLeast to its natural, ending
//     short cells short of the last column once whole, and sharing the room
//     left with stopped, the odd cell its.
func logParts(shown []LogColumn, on, short int) []logPart {
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
		case LogOn:
			named := headedNames(on, c.Name())
			named.Narrows, named.Gap = turn(c), logColumnGap
			parts = append(parts, logPart{c, wholeColumn, named})
		default:
			parts = append(parts, logPart{c, wholeColumn, flex.Part{Least: cells, Natural: cells, Leaves: turn(c), Kept: c.keeps(), Gap: logColumnGap}})
		}
	}
	return parts
}

// sessionColumnCells are the cells a session's column takes, its directory
// whole: its id, and a cell on, its directory, views.SessionDirCells at the
// most, as views.SessionDirName names it.
const sessionColumnCells = status.IDShown + 1 + views.SessionDirCells

// keeps are the cells the column keeps where it gives way: the session its
// id alone, in no fewer cells than its head; any other none.
func (c LogColumn) keeps() int {
	if c == LogSession {
		return max(status.IDShown, c.headCells())
	}
	return 0
}

// logNarrowing is the order the columns shown give way in as the table
// narrows, as the design has it: the extras, the last first, so worth
// leaves last of them; then routing, folding into stopped, where both show;
// then first, read and written; on, toward its least; out; the last column,
// stopped, or routing alone, cut to its tailLeast cells; model and status;
// and last, the session's directory, its id kept.
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
	order = append(order, LogFirst, LogRead, LogWritten, LogOn, LogOut)
	switch {
	case stopped:
		order = append(order, LogStopped)
	case routing:
		order = append(order, LogRouting)
	}
	order = append(order, LogModel, LogStatus, LogSession)
	return slices.DeleteFunc(order, func(c LogColumn) bool { return !slices.Contains(shown, c) })
}
