package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
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
	// Move is what a move's move came to, as its session's story settles it:
	// nil for any other event, and for a move no line read tells of.
	Move *EventMove
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
	// Read holds the ledger's lines of the days the events are of, and every
	// line of their sessions, as ReadLog reads them: a move's cost, and a
	// session's directory, are its session's story's.
	Read LogRead
	// Filter is which events it shows.
	Filter EventFilter
	// Names name a window by its key, as the moment's NameOf does: nil names
	// it as status.NameOfKey does.
	Names func(key string) string
	// Prices are the price table a move's cost is priced by, at today's
	// prices, as the Log's Requests price it.
	Prices ledger.Table
	Now    time.Time
}

// EventsTable is the Log's Events over the days it spans.
type EventsTable struct {
	// Rows are the events of the days it spans the filter shows, oldest
	// first, by when each happened, then by run and id.
	Rows []LogEvent
	// Today counts today's events, those the filter hides among them.
	Today EventCount
	// told are every event of the days it spans, those the filter hides
	// among them, and names names a window by its key, as Details reads
	// them.
	told  []toldEvent
	names func(key string) string
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
	stories := src.Read.stories(src.Prices, today)
	telling := NewTelling(src.Names)
	table := EventsTable{names: src.Names}
	for _, e := range src.Events.Events() {
		line, fact := telling.tell(e.Event, src.Now)
		if !first.Holds(e.Event.At) {
			continue
		}
		row := eventRow(e, line, fact, stories[e.Event.Session], src.Now)
		table.told = append(table.told, toldEvent{event: e.Event, fact: fact, row: row})
		shown := src.Filter.Keeps(row)
		if shown {
			table.Rows = append(table.Rows, row)
		}
		if today.Holds(e.Event.At) {
			table.Today.add(row, shown)
		}
	}
	return table
}

// eventRow is the row of the event e, told as line, its facts fact, its
// session's story s, nil where it's of none, or none read; its time in now's
// time zone.
func eventRow(e Event, line EventLine, fact eventFact, s *story, now time.Time) LogEvent {
	row := LogEvent{
		Key: e.Key, At: e.Event.At.In(now.Location()), Kind: e.Event.Kind, Well: wellAgain(e.Event),
		Session: e.Event.Session, Accounts: namedAccounts(e.Event), Line: line, Moved: fact.moved,
	}
	if s != nil {
		row.Dir = s.dir
	}
	if e.Event.Kind == status.EventMoved {
		row.Move = eventMove(moveOf(s, e.Event))
	}
	return row
}
