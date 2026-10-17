package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// LogEvent is an event's row in the Log's Events, as LogEvents builds it. It
// holds no raw event: what it says is what Telling tells, as events prints
// it, and its facts are typed, each printed form left to its one owner.
type LogEvent struct {
	// Key tells it apart from every other event, by its run and id.
	Key EventKey
	// At is when it happened, in now's time zone.
	At time.Time
	// Kind is its kind as the router names it, as status.EventLimit, which
	// its mark is drawn for, as LATELY's Sentence has it; Well is set, of the
	// router's health, where it's well again, which colours that mark apart.
	Kind string
	Well bool
	// Session is the whole id of the session it's of, one that started or
	// moved, or whose own pin was set or cleared: "" for an account's event.
	// Dir is the directory the session was started in, as its lines last
	// name it: "" where none read does.
	Session, Dir string
	// Accounts are the accounts it names, by id, each once, as the filter
	// keeps and counts it: a move's, from and to; the global pin's, set or
	// cleared, those it names; any other's, the one it befell; none of a
	// restart's or the router's health.
	Accounts []string
	// Line is what it says, as Telling tells it, its facts worked out over
	// the events before it, and as events prints it.
	Line EventLine
	// Moved counts the sessions a limit, a cap or a refusal moved, as its
	// Line says them: it rises in place as each of its moves counts.
	Moved int
	// Use is how much of its window a cap's account had used, as its Line's
	// status.PartUse says it, which colours it by its level: none of any
	// other event.
	Use float64
	// Move is what a move's move came to, as its session's story settles it:
	// nil for any other event, and for a move no line read tells of.
	Move *EventMove
}

// MoveSaid is what the event's move came to, as the Log's Events say it
// after what happened, as RoutingSaid's Said says a request's: once the move
// wrote its session's context again, the tokens it wrote, as the Log counts
// tokens, and what that cost, as its CostSaid says it, "182k cached again,
// $1.82", or "182k cached again, unpriced", as RowCost. None before, or of
// any other event.
func (e LogEvent) MoveSaid() RowWords {
	m := e.Move
	if m == nil || m.Written == 0 || m.CostSaid() == "" {
		return nil
	}
	return RowWords{{Text: prose.Tokens(m.Written) + " cached again, " + m.CostSaid(), Kind: RowCost}}
}

// LogEventSources are what the Log's Events are built from.
type LogEventSources struct {
	// From is when the table starts: it shows the events of the local days
	// from the one From falls on to Now's.
	From time.Time
	// Events are the router's events read, as EventsRead holds them,
	// reaching back to EventsFrom of From's day, so the facts of the first
	// shown are worked out over those told before them.
	Events EventsRead
	// Held is the store, which tells a move's cost, and a session's
	// directory, as its session's story, as the Log's Requests and the List
	// tell them.
	Held HeldSessions
	// Filter is which events it shows.
	Filter EventFilter
	// Names name a window by its key, as the moment's NameOf does: nil names
	// it as status.NameOfKey does.
	Names func(key string) string
	// Totals are the figures of the days it spans, as LogSources' are, which
	// its day rules say; and Oldest the oldest day the ledger keeps, as
	// LogSources' is, which its oldest day's line says where it reaches it.
	Totals []DayTotals
	Oldest OldestDay
	// Filed is the oldest day it reads back to, as OldestKept gives it of
	// the first day the router filed events on: the days before show none.
	// The zero OldestDay, none known, reads none back.
	Filed OldestDay
	Now   time.Time
}

// EventsTable is the Log's Events over the days it spans.
type EventsTable struct {
	// Rows are the events of the days it spans the filter shows, oldest
	// first, by when each happened, then by run and id.
	Rows []LogEvent
	// Lines are its lines, the oldest first, as the Requests' are: each of
	// its rows; before the first of each day after the first with one, that
	// day's rule; and, where it reaches the oldest day, that day's line atop
	// them all.
	Lines []EventsLine
	// Today counts today's events, those the filter hides among them.
	Today EventCount
	// told are every event of the days it spans, those the filter hides
	// among them, and names names a window by its key, as Details reads
	// them.
	told  []toldEvent
	names func(key string) string
	// filed is the oldest day it reads back to, and first the first day it
	// spans.
	filed OldestDay
	first LedgerDay
}

// Reached reports whether the table reaches the oldest day it reads back
// to, as its sources' Filed says, so it wants no day before.
func (t EventsTable) Reached() bool {
	_, more := t.filed.DayBefore(t.first)
	return !more
}

// EventsLine is a line of the Log's Events: an event's row, one of the
// table's Rows; where Rule is set, the rule where one day's rows end and the
// next's begin; or, where Oldest is set, the oldest day's line.
type EventsLine struct {
	Row    *LogEvent
	Rule   *DayRule
	Oldest *OldestDay
}

// toldEvent is an event of the table's: as the router gave it, its facts,
// and its row.
type toldEvent struct {
	event status.Event
	fact  eventFact
	row   LogEvent
}

// LogEvents builds the Log's Events from src: a row of each event that
// happened from the local day From falls on that the filter shows, each
// told as Telling tells it, over every event read, those before that day
// among them; and today's count, every event of the day the filter shows or
// hides. A move's row says what its move came to as its request's row says
// it, both its session's story's.
func LogEvents(src LogEventSources) EventsTable {
	first, today := Today(src.From), Today(src.Now)
	telling, held := NewTelling(src.Names), src.Held.at(src.Now)
	table := EventsTable{names: src.Names, filed: src.Filed, first: first}
	for _, e := range src.Events.Events() {
		line, fact := telling.tell(e.Event, src.Now)
		if !first.Holds(e.Event.At) {
			continue
		}
		row := eventRow(e, line, fact, held.storyOf(e.Event.Session), src.Now)
		table.told = append(table.told, toldEvent{event: e.Event, fact: fact, row: row})
		shown := src.Filter.Keeps(row)
		if shown {
			table.Rows = append(table.Rows, row)
		}
		if today.Holds(e.Event.At) {
			table.Today.add(row, shown)
		}
	}
	table.Lines = eventsLines(table.Rows, first, src)
	return table
}

// eventsLines are the lines of the Events' rows, from the day first, as
// EventsTable's Lines are: the day each sits on its local day, as the
// ledger files the lines of its moment, its rules saying the figures src's
// Totals give, as the Requests' do.
func eventsLines(rows []LogEvent, first LedgerDay, src LogEventSources) []EventsLine {
	var lines []EventsLine
	if oldest := src.Oldest; oldest.reached(first) {
		lines = append(lines, EventsLine{Oldest: &oldest})
	}
	rules := dayRulesOf(src.Totals)
	var last *LedgerDay
	for i := range rows {
		day := Today(rows[i].At)
		if last != nil && last.Date != day.Date {
			lines = append(lines, EventsLine{Rule: rules.between(*last, day)})
		}
		lines = append(lines, EventsLine{Row: &rows[i]})
		last = &day
	}
	return lines
}

// eventRow is the row of the event e, told as line, its facts fact, its
// session's story s, one of no requests where it's of none, or none read;
// its time in now's time zone.
func eventRow(e Event, line EventLine, fact eventFact, s *story, now time.Time) LogEvent {
	row := LogEvent{
		Key: e.Key, At: e.Event.At.In(now.Location()), Kind: e.Event.Kind, Well: wellAgain(e.Event),
		Session: e.Event.Session, Accounts: namedAccounts(e.Event), Line: line, Moved: fact.moved, Dir: s.dir,
	}
	switch e.Event.Kind {
	case status.EventMoved:
		row.Move = eventMove(moveOf(s, e.Event))
	case status.EventCap:
		row.Use = capUse(e.Event)
	}
	return row
}

// FilledFrom is the latest day t could start from and still fill a view
// rows rows tall, as LogTable's FilledFrom has it of its lines: of a day's
// first row, its lines from it on at least as many.
func (t EventsTable) FilledFrom(rows int) (LedgerDay, bool) {
	for i, l := range slices.Backward(t.Lines) {
		first := i == 0 || t.Lines[i-1].Row == nil
		if l.Row != nil && first && len(t.Lines)-i >= rows {
			return Today(l.Row.At), true
		}
	}
	return LedgerDay{}, false
}
