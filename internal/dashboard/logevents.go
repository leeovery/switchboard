package dashboard

import (
	"slices"
	"strconv"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/switchboard/internal/dashboard/flex"
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
// holds, where it starts, and how many cells it takes.
type eventColumn struct {
	part     eventsPart
	x, width int
}

// The Events' event column's cells; the fewest cells what happened is cut
// to before the session leaves; and the fewest it's cut to after, as a
// phone's 52 columns leave it.
const (
	eventsKindCells = 8
	eventsWhatLeast = 40
	eventsWhatBrief = 20
)

// eventsWhatPast is the part of a row of events past its columns' own: what
// happened's cells past eventsWhatBrief, to its least.
const eventsWhatPast = eventsWhat + 1

// The turns the Events' parts give way in as the row narrows: the session,
// whole; then what happened, to eventsWhatBrief; then the account, toward
// its least.
const (
	sessionLeaves = iota + 1
	whatShortened
	accountNarrows
)

// eventsAccountCells are the Events' account column's cells, of the moment
// m: the longest line a move can make, from one account to another, the two
// widest names, as labelsCells has them, and the arrow between; the widest
// alone where it knows one. Each counts recordedNatural of its cells, as an
// event may name an account no longer configured. A pin naming several is
// cut to them.
func eventsAccountCells(m status.Moment) int {
	cells := labelsCells(m)
	switch len(cells) {
	case 0:
		return 0
	case 1:
		return recordedNatural(cells[0])
	}
	return recordedNatural(cells[0]) + ansi.StringWidth(moveArrow) + recordedNatural(cells[1])
}

// eventsParts are the Events' parts across a row, by what each holds, each
// column two cells after the one before, as the Requests' are: at and the
// session, as Requests' are, the session leaving whole; event; account, a
// column of names a record may hold, as recordedNames has it of the account
// cells given, holding them until its turn; and what happened, as two
// parts, its first eventsWhatBrief cells, which take the room left, and
// those past them to eventsWhatLeast.
func eventsParts(account int) []flex.Part {
	at, session := logColumnFacts[LogAt].width, logColumnFacts[LogSession].width
	named := recordedNames(account)
	named.Narrows, named.Gap = accountNarrows, logColumnGap
	return []flex.Part{
		eventsAt:       {Least: at, Natural: at},
		eventsSession:  {Least: session, Natural: session, Leaves: sessionLeaves, Gap: logColumnGap},
		eventsKind:     {Least: eventsKindCells, Natural: eventsKindCells, Gap: logColumnGap},
		eventsAccount:  named,
		eventsWhat:     {Least: eventsWhatBrief, Natural: eventsWhatBrief, Grows: true, Gap: logColumnGap},
		eventsWhatPast: {Least: eventsWhatLeast - eventsWhatBrief, Natural: eventsWhatLeast - eventsWhatBrief, Leaves: whatShortened},
	}
}

// eventsLaid are the Events' columns laid out across a terminal width cells
// wide, from logLeft, as flex lays eventsParts out, the account at the cells
// given, as eventsAccountCells has them, what happened its two parts
// together.
func eventsLaid(width, account int) []eventColumn {
	spans, _ := flex.Lay(width-logLeft, eventsParts(account)...)
	spans[eventsWhat].Size += spans[eventsWhatPast].Size
	var columns []eventColumn
	for part, s := range spans[:eventsWhatPast] {
		if !s.Left {
			columns = append(columns, eventColumn{part: eventsPart(part), x: logLeft + s.At, width: s.Size})
		}
	}
	return columns
}

// movedApart is the cells between why a session moved and what writing its
// context again cost, as "work reached its cap   182k cached again, $1.82".
const movedApart = 3

// eventLines are the Events' lines, the oldest first, as the table holds
// them; then, where it shows none of today's, the words saying so.
func (p LogPage) eventLines() madeLines {
	lines := make(madeLines, len(p.Events.Lines), len(p.Events.Lines)+1)
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
	if p.eventsEmpty() {
		lines = append(lines, logLine{key: LogKey{other: emptyKey}, empty: noEventsToday})
	}
	return lines
}

// eventsEmpty reports whether the Events show none of today's events, so
// they say so at their foot.
func (p LogPage) eventsEmpty() bool {
	return p.Events.Today.Shown == 0
}

// eventsHeads draws the heads of the Events' columns given along row y, in
// text.subtle, each at its column's start.
func eventsHeads(c *canvas, y int, columns []eventColumn) {
	for _, col := range columns {
		c.line(col.x, y, line{{eventHeads[col.part], dimInk}})
	}
}

// logEvent draws an event's row along a row, in the columns given, of the
// moment m at now, as eventRow draws it.
func logEvent(m status.Moment, now time.Time, columns []eventColumn) func(c *canvas, y int, l logLine) {
	return func(c *canvas, y int, l logLine) {
		eventRow(c, y, *l.event, columns, m, now)
	}
}

// eventRow draws the event e's row along row y, in the columns given, of
// the moment m at now: its mark, as eventMark gives it, bold, at column 0;
// then in its columns, when it happened, to the second; its session, as
// Requests' names one, blank for an account's event; its kind's word, in
// text.secondary; the account it befell, white, a move's two as moveNames
// cuts them; and what happened, in text.muted, cut as whatHappened has it.
func eventRow(c *canvas, y int, e views.LogEvent, columns []eventColumn, m status.Moment, now time.Time) {
	mark := eventMark(e.Kind, e.Well)
	mark.ink.bold = true
	c.line(0, y, line{mark})
	for _, col := range columns {
		c.line(col.x, y, eventCell(e, col.part, col.width, m, now))
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
		return sessionCell(e.Session, e.Dir, true).fit(width)
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
// them, each cut as asLabelled cuts it, then all cut to the cells given.
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
		l = append(l, span{asLabelled(name), accountInk})
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
// and three cells on, what a move came to, as its MoveSaid says, kept whole
// as what comes before it is cut.
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
	cached := e.MoveSaid()
	if len(cached) == 0 {
		return costKept(said, false, width)
	}
	return costKept(slices.Concat(said, line{spaces(movedApart)}, rowLine(cached)), true, width)
}

// eventKeyed names an event by its key, as a number on its row is of it:
// its run's start, to the nanosecond, and its id.
func eventKeyed(k views.EventKey) string {
	return strconv.FormatInt(k.Run.UnixNano(), 10) + " " + strconv.Itoa(k.ID)
}
