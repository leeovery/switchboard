package dashboard

import (
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// eventsPart is what one of the Events' columns holds.
type eventsPart int

const (
	eventsAt eventsPart = iota
	eventsSession
	eventsKind
	eventsAccount
	eventsWhat
)

// eventHeads are the Events' columns' heads, by what each holds.
var eventHeads = [...]string{
	eventsAt: "at", eventsSession: "session", eventsKind: "event", eventsAccount: "account", eventsWhat: "what happened",
}

// eventColumn is one of the Events' columns as it's laid out: what it
// holds, where it starts, and how many cells it takes, none for the last,
// which runs to the last column.
type eventColumn struct {
	part     eventsPart
	x, width int
}

// The Events' columns as the design lays them out, and on a phone, the
// session leaving, as the phone keeps one over the directory.
var (
	eventColumns = []eventColumn{
		{eventsAt, 2, 8}, {eventsSession, 12, 17}, {eventsKind, 31, 8}, {eventsAccount, 41, 16}, {eventsWhat, 59, 0},
	}
	phoneEventColumns = []eventColumn{
		{eventsAt, 2, 8}, {eventsKind, 11, 8}, {eventsAccount, 20, 8}, {eventsWhat, 30, 0},
	}
)

// eventColumns are the Events' columns as the frame lays them out.
func (f Frame) eventColumns() []eventColumn {
	if f.phone() {
		return phoneEventColumns
	}
	return eventColumns
}

// movedApart is the cells between why a session moved and what writing its
// context again cost, as "work reached its cap   182k cached again, $1.82".
const movedApart = 3

// eventLines are the Events' lines, the oldest first, as the table holds
// them; then, where it shows none of today's, the words saying so.
func (p LogPage) eventLines() []logLine {
	lines := make([]logLine, len(p.Events.Lines), len(p.Events.Lines)+1)
	for i, l := range p.Events.Lines {
		switch {
		case l.Row != nil:
			lines[i] = logLine{key: LogKey{event: l.Row.Key}, at: l.Row.At, event: l.Row}
		case l.Rule != nil:
			lines[i] = logLine{key: LogKey{other: ruleKey + l.Rule.Ended.Date + " " + l.Rule.Began.Date}, rule: l.Rule}
		default:
			lines[i] = logLine{key: LogKey{other: oldestKey}, oldest: l.Oldest}
		}
	}
	if p.Events.Today.Shown == 0 {
		lines = append(lines, logLine{key: LogKey{other: emptyKey}, empty: noEventsToday})
	}
	return lines
}

// eventsHeads draws the Events' heads along row y, in text.subtle, each at
// its column's start.
func (f Frame) eventsHeads(c *canvas, y int) {
	for _, col := range f.eventColumns() {
		c.line(col.x, y, line{{eventHeads[col.part], dimInk}})
	}
}

// logEvent draws an event's row along a row, of the moment m at now, as
// eventRow draws it.
func (f Frame) logEvent(m status.Moment, now time.Time) func(c *canvas, y int, l logLine) {
	return func(c *canvas, y int, l logLine) {
		f.eventRow(c, y, *l.event, m, now)
	}
}

// eventRow draws the event e's row along row y, of the moment m at now:
// its kind's mark, bold, at column 0; then in its columns, when it happened,
// to the second; its session, as Requests' names one, blank for an
// account's event; its kind's word, in text.secondary; the account it
// befell, white, a move's two as moveNames cuts them; and what happened, in
// text.muted, cut as whatHappened has it.
func (f Frame) eventRow(c *canvas, y int, e views.LogEvent, m status.Moment, now time.Time) {
	mark := kindMark(e.Kind)
	if e.Well {
		mark.ink = positiveInk
	}
	mark.ink.bold = true
	c.line(0, y, line{mark})
	for _, col := range f.eventColumns() {
		width := col.width
		if width == 0 {
			width = f.Width - col.x
		}
		c.line(col.x, y, eventCell(e, col.part, width, m, now))
	}
}

// eventCell is what the event e's row says in the column holding part,
// width cells wide, of the moment m at now.
func eventCell(e views.LogEvent, part eventsPart, width int, m status.Moment, now time.Time) line {
	switch part {
	case eventsAt:
		return line{{prose.ToTheSecond(now, e.At), mutedInk}}
	case eventsSession:
		if prose.Clean(e.Session) == "" {
			return nil
		}
		return sessionCell(e.Session, e.Dir, false).fit(width)
	case eventsKind:
		return line{{e.Line.Kind, secondaryInk}}.fit(width)
	case eventsAccount:
		return accountCell(e, width, m)
	}
	return whatHappened(e, width)
}

// accountCell is the account the event e befell, in width cells, each
// named as the moment m names it, white, the words between them muted: a
// move's two, from and to, as moveNames cuts them; else as the event names
// them, cut with an ellipsis.
func accountCell(e views.LogEvent, width int, m status.Moment) line {
	var names []string
	var l line
	for _, p := range e.Line.Account {
		if p.Kind != status.PartAccount {
			l = append(l, span{p.Text, mutedInk})
			continue
		}
		name := named(m, p.Text)
		names = append(names, name)
		l = append(l, span{name, accountInk})
	}
	if e.Kind == status.EventMoved && len(names) == 2 {
		return moveNames(names[0], names[1], width, accountInk)
	}
	return l.fit(width)
}

// whatHappened is what happened of the event e, in width cells, in
// text.muted, the accounts it names muted too, as the account column
// carries the white: a window's use bold in its level's colour; the
// sessions its moves counted brightening as they rise, the number they are;
// and where a move wrote its session's context again, three cells on, how
// much and what it cost, in accent.attention, kept whole as what comes
// before it is cut.
func whatHappened(e views.LogEvent, width int) line {
	var said line
	for _, p := range e.Line.What {
		part := span{p.Text, mutedInk}
		switch p.Kind {
		case status.PartUse:
			part.ink = usedInk(e.Use, ink{token: theme.VizRamp1, bold: true})
		case status.PartFigure:
			part = part.numbered(Number{Kind: NumberMoved, Of: eventKeyed(e.Key)})
		}
		said = append(said, part)
	}
	cached := movedCost(e.Move)
	if cached == "" {
		return costKept(said, false, width)
	}
	return costKept(append(said, spaces(movedApart), span{cached, warningInk}), true, width)
}

// movedCost is what a move came to, where it wrote its session's context
// again: the tokens it wrote, as the Log counts them, and what that cost,
// exactly, "182k cached again, $1.82"; else nothing.
func movedCost(m *views.EventMove) string {
	if m == nil || m.Written == 0 || m.Cost == nil {
		return ""
	}
	return prose.Tokens(m.Written) + " cached again, " + views.Worth{Amount: m.Cost}.Cents()
}

// eventKeyed names an event by its key, as a number on its row is of it:
// its run's start, to the nanosecond, and its id.
func eventKeyed(k views.EventKey) string {
	return strconv.FormatInt(k.Run.UnixNano(), 10) + " " + strconv.Itoa(k.ID)
}
