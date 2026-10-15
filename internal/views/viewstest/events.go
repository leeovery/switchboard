// Package viewstest builds what views read of the router's events and the
// request ledger, as the router files and gives them, at the moments views
// have got them wrong at, or could, for each view's facts to be checked
// against what's wanted of them at each.
package viewstest

import (
	"fmt"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// EventMoment is the router's events at a moment: those its files hold, and
// those the router running gives in its status document, read at Now from
// Since; and what's wanted of them then.
type EventMoment struct {
	Name string
	// Filed are the versions of the events the router's files hold, in the
	// order filed, each with when it was: a version is filed under the
	// local day it's written on.
	Filed []Filing
	// Live are the events the router running gives in its status document,
	// the newest first, and Run when it started: none, and zero, where none
	// runs.
	Live []status.Event
	Run  time.Time
	// Now is when the events are read, and Since when those read begin.
	Now, Since time.Time
	// From is the first day the Log's Events show, at its start: Since, or,
	// where Since reaches back before it for the facts of its first events,
	// the day after.
	From time.Time
	// Events are the events wanted of those read: each once, as it last
	// stands, oldest first, by when it happened, then by run and id.
	Events []events.Line
	// Facts are what's wanted told of Events beyond their own fields,
	// worked out over them, by their keys: an event not named has none.
	Facts map[Key]Facts
	// Lines are the ledger's lines of the sessions the events are of, every
	// day's, oldest first, each filed under the local day its request
	// arrived on: none where no line tells of them.
	Lines []ledger.Line
	// Moves are what each move event's move came to, by the event's key, as
	// the Log's Events and its request's routing say it: a move not named is
	// one no line tells of.
	Moves map[Key]Move
	// Details are what's wanted of the details cards of the events named, by
	// their keys: an event not named isn't held to any.
	Details map[Key]Details
}

// Details are what's wanted of an event's details card: its headline; WHAT
// HAPPENED's items, "name: value" each; the id of the request it was known
// from; `<ACCOUNT>` THEN's, as "account: work", each window's use, as
// "5-hour: 97%", "resets: 17:10" and "sessions: 3"; and WHAT FOLLOWED's,
// each move it counted, as "d28c: to side $2.00", "new sessions: till
// 17:10" and "in all: $7.00", or a move's own, as "request: d1", "cached
// again: 1000000" and "the move: $8.00". Times are as the card shows them
// beside the event's.
type Details struct {
	Headline  string
	Happened  []string
	KnownFrom string
	Then      []string
	Followed  []string
}

// Move is what a move event's move came to: the request that made it, and,
// where a request wrote the session's context again on the account it went
// to, the tokens it wrote and what that cost; none, and 0, where none has,
// or the cache would have run out anyway.
type Move struct {
	Request string
	Written int
	Cost    ledger.Picodollars
}

// Filing is a version of an event as the router filed it, and when.
type Filing struct {
	Line  events.Line
	Filed time.Time
}

// Key tells an event apart from every other, as the router's files do: the
// run of the router that told it, to the nanosecond, and its id.
type Key struct {
	Run int64
	ID  int
}

// KeyOf is the key of the event line is a version of.
func KeyOf(line events.Line) Key {
	return Key{Run: line.Run.UnixNano(), ID: line.ID}
}

// Facts are what's told of an event beyond its own fields: when the account
// its reason passed over under pressure runs out, as that account's latest
// pressure event before it said; the sessions a limit, a cap or a refusal
// moved, and the account they went to, where they all went to one; and the
// windows it names, as status.NameOfKey names them in a sentence.
type Facts struct {
	RunsOut time.Time
	Moved   int
	To      string
	Windows []string
}

// The accounts the moments' events befall, and the window each is named by.
const (
	personal, side, work = "personal", "side", "work"
	fiveHour, week       = "5h", "7d"
)

// fiveHourNamed and bothNamed are a 5-hour window's names, and the 5-hour
// and week's, as status.NameOfKey names them in a sentence.
var (
	fiveHourNamed = []string{"5-hour"}
	bothNamed     = []string{"5-hour", "week"}
)

// EventMoments are the moments views have got the router's events wrong at,
// or could. Those of the events alone are in London's time, whose clocks go
// back an hour at 02:00 on Sunday 25 October 2026; those whose sessions'
// requests the ledger's lines tell of are in time.Local, as the ledger files
// its lines under local days: a move whose choice passed an account over,
// its pressure told the day before; two like moves of one session in a day;
// a cap moving one session twice, the second's request in flight; a side
// request's move, settled on its conversation's next request; a move
// after its cache ran out; a cap whose moves count after it's told, and one
// with all three counted; a model family refused alone; pins, global and a
// session's own; an event-free today, filled from yesterday; and the
// router's health turning, and a restart falling due.
func EventMoments() []EventMoment {
	london := zone("Europe/London")
	on := func(day, hour, minute int) time.Time { return time.Date(2026, 10, day, hour, minute, 0, 0, london) }
	return []EventMoment{
		restarted(on),
		joined(on),
		overlapping(on),
		eventFree(on),
		acrossMidnight(on),
		filedAgainToday(on),
		fallingBack(london),
		PressureTheDayBefore(),
		movedTwiceAlike(),
		capMovingOneTwice(),
		sideMoveSettled(),
		unpricedMove(),
		coldMove(),
		capCounting(),
		capDetails(),
		familyRefused(),
		pins(),
		filledFromYesterday(),
		healthTurning(),
	}
}

// restarted is a router restarted at noon on 7 October, the limit work
// reached under its first run holding across it, reached again under the
// second; the second run's move passing personal over under pressure, which
// the first run's pressure event says when it runs out. The second run's
// last event isn't filed yet.
func restarted(on func(day, hour, minute int) time.Time) EventMoment {
	first, second := on(6, 9, 0), on(7, 12, 0)
	pressure := status.Event{ID: 1, At: on(7, 11, 40), Kind: status.EventPressure, Account: personal, Windows: []string{fiveHour}, Until: on(7, 15, 10), Since: on(7, 11, 10)}
	limit := limitOf(2, on(7, 11, 50), on(7, 16, 0), fiveHour)
	reached := limitOf(1, on(7, 12, 0).Add(30*time.Second), on(7, 16, 0), fiveHour)
	passing := status.Event{ID: 3, At: on(7, 12, 10), Kind: status.EventMoved, Session: "s6", From: personal, To: side, Reason: status.PassingOver(status.Rescored(15*time.Hour), personal)}
	firstMoves := []status.Event{forcedBy(limit, 3, "s1", on(7, 11, 52)), forcedBy(limit, 4, "s2", on(7, 11, 55))}
	secondMove := forcedBy(reached, 2, "s5", on(7, 12, 5))
	return EventMoment{
		Name: "a router restarted, a limit holding across it",
		Filed: []Filing{
			filed(first, pressure, pressure.At),
			filed(first, limit, limit.At),
			filed(first, firstMoves[0], firstMoves[0].At), filed(first, counting(limit, 1, side), firstMoves[0].At),
			filed(first, firstMoves[1], firstMoves[1].At), filed(first, counting(limit, 2, side), firstMoves[1].At),
			filed(second, reached, reached.At),
			filed(second, secondMove, secondMove.At), filed(second, counting(reached, 1, side), secondMove.At),
		},
		Live: []status.Event{passing, secondMove, counting(reached, 1, side)},
		Run:  second,
		Now:  on(7, 12, 15), Since: on(7, 0, 0), From: on(7, 0, 0),
		Events: []events.Line{
			{Event: pressure, Run: first}, {Event: counting(limit, 2, side), Run: first},
			{Event: firstMoves[0], Run: first}, {Event: firstMoves[1], Run: first},
			{Event: counting(reached, 1, side), Run: second}, {Event: secondMove, Run: second}, {Event: passing, Run: second},
		},
		Facts: map[Key]Facts{
			{Run: first.UnixNano(), ID: 1}:  {Windows: fiveHourNamed},
			{Run: first.UnixNano(), ID: 2}:  {Moved: 2, To: side, Windows: fiveHourNamed},
			{Run: second.UnixNano(), ID: 1}: {Moved: 1, To: side, Windows: fiveHourNamed},
			{Run: second.UnixNano(), ID: 3}: {RunsOut: on(7, 15, 10)},
		},
	}
}

// joined is a limit work reached in its 5-hour window, then in its week,
// joining its event, filed again as each of its moves counts; the document
// holding it as it stands since its last move, which isn't filed yet.
func joined(on func(day, hour, minute int) time.Time) EventMoment {
	run := on(7, 8, 0)
	limit := limitOf(1, on(7, 13, 0), on(7, 18, 0), fiveHour)
	both := limit
	both.Windows, both.Until = []string{fiveHour, week}, on(9, 21, 0)
	moves := []status.Event{forcedBy(limit, 2, "s1", on(7, 13, 2)), forcedBy(limit, 3, "s2", on(7, 13, 20))}
	return EventMoment{
		Name: "a limit joined, and filed again",
		Filed: []Filing{
			filed(run, limit, limit.At),
			filed(run, moves[0], moves[0].At), filed(run, counting(limit, 1, side), moves[0].At),
			filed(run, counting(both, 1, side), on(7, 13, 10)),
		},
		Live: []status.Event{moves[1], moves[0], counting(both, 2, side)},
		Run:  run,
		Now:  on(7, 13, 30), Since: on(7, 0, 0), From: on(7, 0, 0),
		Events: []events.Line{{Event: counting(both, 2, side), Run: run}, {Event: moves[0], Run: run}, {Event: moves[1], Run: run}},
		Facts:  map[Key]Facts{{Run: run.UnixNano(), ID: 1}: {Moved: 2, To: side, Windows: bothNamed}},
	}
}

// overlapping is sixty sessions started under one run: the document holding
// the newest 50, the files every one but the last two, not filed yet.
func overlapping(on func(day, hour, minute int) time.Time) EventMoment {
	run := on(7, 6, 0)
	m := EventMoment{Name: "the document's newest 50 overlapping the files", Run: run, Now: on(7, 9, 0), Since: on(7, 0, 0), From: on(7, 0, 0)}
	const started, filedOf, livesOf = 60, 58, 50
	for id := 1; id <= started; id++ {
		e := status.Event{ID: id, At: on(7, 7, id), Kind: status.EventStarted, Account: work, Session: fmt.Sprintf("s%d", id), Reason: status.ReasonNew}
		if id <= filedOf {
			m.Filed = append(m.Filed, filed(run, e, e.At))
		}
		if id > started-livesOf {
			m.Live = append([]status.Event{e}, m.Live...)
		}
		m.Events = append(m.Events, events.Line{Event: e, Run: run})
	}
	return m
}

// eventFree is a router that's told of nothing today: side primed yesterday,
// a session started, work open again, and a limit reached then that lifts
// today.
func eventFree(on func(day, hour, minute int) time.Time) EventMoment {
	run := on(6, 9, 0)
	told := []status.Event{
		{ID: 1, At: on(6, 9, 30), Kind: status.EventPrimed, Account: side, Windows: []string{fiveHour}, Until: on(6, 14, 30)},
		{ID: 2, At: on(6, 10, 0), Kind: status.EventStarted, Account: work, Session: "s1", Reason: status.ReasonNew},
		{ID: 3, At: on(6, 18, 0), Kind: status.EventRoom, Account: work, Windows: []string{fiveHour}},
		limitOf(4, on(6, 22, 0), on(7, 2, 0), fiveHour),
	}
	m := EventMoment{Name: "an event-free today", Run: run, Now: on(7, 9, 0), Since: on(7, 0, 0), From: on(7, 0, 0)}
	for _, e := range told {
		m.Filed, m.Live = append(m.Filed, filed(run, e, e.At)), append([]status.Event{e}, m.Live...)
	}
	return m
}

// midnightLimit is the limit work reached at 23:50 on 6 October, and its
// moves either side of midnight, as acrossMidnight and filedAgainToday read
// them: the limit filed again as each counts, its last version under the
// 7th.
func midnightLimit(on func(day, hour, minute int) time.Time) (limit status.Event, moves []status.Event, versions []Filing) {
	run := on(6, 9, 0)
	limit = limitOf(2, on(6, 23, 50), on(7, 3, 50), fiveHour)
	moves = []status.Event{forcedBy(limit, 3, "s1", on(6, 23, 51)), forcedBy(limit, 5, "s2", on(7, 0, 10))}
	versions = []Filing{
		filed(run, limit, limit.At),
		filed(run, moves[0], moves[0].At), filed(run, counting(limit, 1, side), moves[0].At),
		filed(run, moves[1], moves[1].At), filed(run, counting(limit, 2, side), moves[1].At),
	}
	return limit, moves, versions
}

// acrossMidnight is midnightLimit's limit and moves read from the day before,
// with personal under pressure before midnight, to run out after it, and a
// session moved off personal after midnight, its choice passing personal
// over.
func acrossMidnight(on func(day, hour, minute int) time.Time) EventMoment {
	run := on(6, 9, 0)
	limit, moves, versions := midnightLimit(on)
	pressure := status.Event{ID: 1, At: on(6, 23, 40), Kind: status.EventPressure, Account: personal, Windows: []string{fiveHour}, Until: on(7, 0, 30)}
	passing := status.Event{ID: 4, At: on(7, 0, 5), Kind: status.EventMoved, Session: "s3", From: personal, To: side, Reason: status.PassingOver(status.Rescored(time.Hour+10*time.Minute), personal)}
	return EventMoment{
		Name:  "a day's boundary between two events of one limit, read from the day before",
		Filed: append([]Filing{filed(run, pressure, pressure.At)}, append(versions, filed(run, passing, passing.At))...),
		Live:  []status.Event{moves[1], passing, moves[0], counting(limit, 2, side), pressure},
		Run:   run,
		Now:   on(7, 0, 20), Since: on(6, 0, 0), From: on(6, 0, 0),
		Events: []events.Line{
			{Event: pressure, Run: run}, {Event: counting(limit, 2, side), Run: run}, {Event: moves[0], Run: run},
			{Event: passing, Run: run}, {Event: moves[1], Run: run},
		},
		Facts: map[Key]Facts{
			{Run: run.UnixNano(), ID: 1}: {Windows: fiveHourNamed},
			{Run: run.UnixNano(), ID: 2}: {Moved: 2, To: side, Windows: fiveHourNamed},
			{Run: run.UnixNano(), ID: 4}: {RunsOut: on(7, 0, 30)},
		},
	}
}

// filedAgainToday is midnightLimit's limit and moves read from today, by a
// reader whose router isn't running: the limit's last version is filed under
// today, but it happened yesterday.
func filedAgainToday(on func(day, hour, minute int) time.Time) EventMoment {
	_, moves, versions := midnightLimit(on)
	return EventMoment{
		Name:   "a day's boundary between two events of one limit, read from the day after",
		Filed:  versions,
		Now:    on(7, 0, 20),
		Since:  on(7, 0, 0),
		From:   on(7, 0, 0),
		Events: []events.Line{{Event: moves[1], Run: on(6, 9, 0)}},
	}
}

// fallingBack is two events at 01:30 on 25 October, as London's clocks go
// back an hour at 02:00: personal under pressure at the first, to run out at
// 01:40 by the clocks gone back, and a session started at the second,
// passing personal over.
func fallingBack(london *time.Location) EventMoment {
	utc := func(hour, minute int) time.Time {
		return time.Date(2026, 10, 25, hour, minute, 0, 0, time.UTC).In(london)
	}
	run := time.Date(2026, 10, 24, 20, 0, 0, 0, london)
	pressure := status.Event{ID: 1, At: utc(0, 30), Kind: status.EventPressure, Account: personal, Windows: []string{fiveHour}, Until: utc(1, 40)}
	passing := status.Event{ID: 2, At: utc(1, 30), Kind: status.EventStarted, Account: side, Session: "s1", Reason: status.PassingOver(status.ReasonNew, personal)}
	return EventMoment{
		Name:   "two events at 01:30 as the clocks go back",
		Filed:  []Filing{filed(run, pressure, pressure.At)},
		Live:   []status.Event{passing, pressure},
		Run:    run,
		Now:    utc(2, 30),
		Since:  time.Date(2026, 10, 25, 0, 0, 0, 0, london),
		From:   time.Date(2026, 10, 25, 0, 0, 0, 0, london),
		Events: []events.Line{{Event: pressure, Run: run}, {Event: passing, Run: run}},
		Facts: map[Key]Facts{
			{Run: run.UnixNano(), ID: 1}: {Windows: fiveHourNamed},
			{Run: run.UnixNano(), ID: 2}: {RunsOut: utc(1, 40)},
		},
	}
}

// limitOf is the limit work reached at at, with the given id, in the windows
// with the given keys, lifting at until, as it's first told, moving none yet.
func limitOf(id int, at, until time.Time, windows ...string) status.Event {
	return status.Event{ID: id, At: at, Kind: status.EventLimit, Account: work, Windows: windows, Until: until, Limit: 4}
}

// counting is the event e, a limit, a cap or a refusal, once it has moved
// count sessions, to the account given.
func counting(e status.Event, count int, to string) status.Event {
	e.Count, e.To = count, to
	return e
}

// forcedBy is a move of the session with the given id off the account e, a
// limit, held back, to side, at at, with the given id, counted in e's line.
func forcedBy(e status.Event, id int, session string, at time.Time) status.Event {
	return status.Event{ID: id, At: at, Kind: status.EventMoved, Session: session, From: e.Account, To: side, Reason: "moved: work hit its limit", Limit: e.ID, ForcedBy: e.ID}
}

// filed is the event e, told by the router whose run started at run, as it
// was filed at at.
func filed(run time.Time, e status.Event, at time.Time) Filing {
	return Filing{Line: events.Line{Event: e, Run: run}, Filed: at}
}

// zone is the time zone with the given name, as the system's zone files
// give it: the moments can't be built without it.
func zone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(fmt.Sprintf("viewstest: %v", err))
	}
	return loc
}
