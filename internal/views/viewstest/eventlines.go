package viewstest

import (
	"fmt"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// The sessions the moments' events are of beyond the Log's, and the
// directory each of theirs runs in.
const (
	s7F3A = "7f3a9c21-5d0e-4b8a-a6f1-2c9e8d4b7a30"
	sDB8A = "db8a71e3-0c4f-4e2d-9b1a-6f5c3e8d2a47"
)

var eventDirs = map[string]string{D28C: "~/Code/switchboard", C61B: "~/Code/mobile", s7F3A: "~/Code/api", sDB8A: "~/Code/infra"}

// refusedOff is the reason for a move off work as it refused the request.
const refusedOff = status.ReasonMovedOff + work + " " + status.WhyRefused

// capped is the reason for a move off the account given at its cap.
func capped(account string) string {
	return status.ReasonMovedOff + account + " " + status.WhyReserve
}

// localOn is the local time on the given day of October 2026 at the clock
// given: the moments whose events the ledger's lines tell of are in
// time.Local, as the ledger files its lines under local days.
func localOn(day, hour, minute int) time.Time {
	return time.Date(2026, 10, day, hour, minute, 0, 0, time.Local)
}

// asked is a request of the session's own conversation, of Claude Opus 5.5,
// with the id given, routed to account for reason, arriving at at, in the
// directory its session runs in, as requested has it.
func asked(id, session, account, reason string, at time.Time) ledger.Line {
	l := requested(id, session, Opus, account, reason, at)
	l.Dir = eventDirs[session]
	return l
}

// writing returns l, its answer writing the tokens given to the cache for an
// hour, as a request writing its session's context again does: a million
// worth $8 of Claude Opus 5.5, as rewriteCost is.
func writing(tokens int, l ledger.Line) ledger.Line {
	l.Usage = fmt.Appendf(nil, `{"input_tokens":3,"cache_creation_input_tokens":%d,"cache_creation":{"ephemeral_1h_input_tokens":%d},"output_tokens":1200}`, tokens, tokens)
	return l
}

// writeCost is what writing the tokens given to the cache for an hour costs
// of Claude Opus 5.5, as writing has them: exact for a multiple of 125,000.
func writeCost(tokens int) ledger.Picodollars {
	return rewriteCost * ledger.Picodollars(tokens) / 1_000_000
}

// reading returns l, its answer reading the windows given, each a key and
// its share used, all resetting at resets.
func reading(resets time.Time, l ledger.Line, windows ...any) ledger.Line {
	l.Limits = make(map[string]string)
	for i := 0; i+1 < len(windows); i += 2 {
		key := windows[i].(string)
		l.Limits[key+"-utilization"] = fmt.Sprint(windows[i+1])
		l.Limits[key+"-reset"] = fmt.Sprint(resets.Unix())
		l.Limits[key+"-status"] = "allowed"
	}
	return l
}

// tried returns l, a request sent again after the accounts given couldn't
// serve it, each as why says.
func tried(why string, l ledger.Line, accounts ...string) ledger.Line {
	for _, a := range accounts {
		l.Tried = append(l.Tried, ledger.Tried{Account: a, Why: why})
	}
	l.Attempts = len(l.Tried) + 1
	return l
}

// movedAfter is the move of the session's Claude Opus 5.5 from one account
// to another for reason, with the id given, told a moment after its request
// l arrived, as the router tells of a move once it's routed the request.
func movedAfter(id int, l ledger.Line, from, reason string) status.Event {
	return status.Event{ID: id, At: l.At.Add(300 * time.Millisecond), Kind: status.EventMoved, Session: l.Session, Model: l.Model, From: from, To: l.Account, Reason: reason}
}

// counted returns the move m, counted in the event e, which forced it.
func counted(m, e status.Event) status.Event {
	m.ForcedBy = e.ID
	return m
}

// toldAll is a moment of the events given, told by the router whose run
// began at run, each filed as it happened and the document giving every one,
// each once as it stands, read at now from the day since falls on: the
// events wanted those from since on.
func toldAll(name string, run, since, now time.Time, told ...status.Event) EventMoment {
	m := EventMoment{Name: name, Run: run, Now: now, Since: since, From: since}
	for _, e := range told {
		m.Filed = append(m.Filed, filed(run, e, e.At))
		m.Live = append([]status.Event{e}, m.Live...)
		if !e.At.Before(since) {
			m.Events = append(m.Events, events.Line{Event: e, Run: run})
		}
	}
	return m
}

// keyOf is the key of the event e told by the router whose run began at
// run.
func keyOf(run time.Time, e status.Event) Key {
	return Key{Run: run.UnixNano(), ID: e.ID}
}

// PressureTheDayBefore is a move off work at its cap 20 minutes after
// midnight, its choice passing personal over, whose pressure was told at
// 22:50 the day before, to run out at 01:10: the Log's Events, and the
// events command, show the day of the move alone, read from five hours
// before it, saying why it moved, then the pressure.
func PressureTheDayBefore() EventMoment {
	run := localOn(6, 9, 0)
	pressure := status.Event{ID: 1, At: localOn(6, 22, 50), Kind: status.EventPressure, Account: personal, Windows: []string{fiveHour}, Until: localOn(7, 1, 10)}
	reason := status.PassingOver(capped(work), personal)
	d0 := asked("d0", D28C, work, status.ReasonNew, localOn(7, 0, 5))
	d1 := writing(1_000_000, movedOff(work, asked("d1", D28C, side, reason, localOn(7, 0, 20))))
	move := movedAfter(2, d1, work, reason)
	m := toldAll("a move after midnight passing an account over, its pressure told the day before", run, localOn(6, 19, 0), localOn(7, 0, 30), pressure, move)
	m.From, m.Lines = localOn(7, 0, 0), []ledger.Line{d0, d1}
	m.Facts = map[Key]Facts{keyOf(run, pressure): {Windows: fiveHourNamed}, keyOf(run, move): {RunsOut: localOn(7, 1, 10)}}
	m.Moves = map[Key]Move{keyOf(run, move): {Request: "d1", Written: 1_000_000, Cost: writeCost(1_000_000)}}
	m.Details = map[Key]Details{keyOf(run, move): {
		Headline: "work reached its cap, personal came under pressure, its 5-hour to run out at 01:10",
		Happened: []string{
			"event: work reached its cap, personal came under pressure, its 5-hour to run out at 01:10", "session: d28c switchboard", "model: opus-5-5",
			"from: work", "to: side",
		},
		Followed: []string{"request: d1", "cached again: 1000000", "the move: $8.00"},
	}}
	return m
}

// movedTwiceAlike is c61b moved off work at its cap twice in a day, back
// onto work between, its cache warm each time, its haiku's title moving
// alike a moment after the second, before the router told of it: each move
// event is of the request that made it, not the like move before or after
// it, nor another model's.
func movedTwiceAlike() EventMoment {
	run := localOn(7, 8, 0)
	c0 := asked("c0", C61B, work, status.ReasonNew, localOn(7, 10, 0))
	h0 := classed("auxiliary", requested("h0", C61B, Haiku, work, status.ReasonNew, localOn(7, 10, 1)))
	c1 := writing(500_000, movedOff(work, asked("c1", C61B, side, capped(work), localOn(7, 10, 30))))
	c2 := asked("c2", C61B, side, status.ReasonSticky, localOn(7, 12, 30))
	c3 := writing(250_000, movedOff(side, asked("c3", C61B, work, capped(side), localOn(7, 13, 0))))
	c4 := writing(1_000_000, movedOff(work, asked("c4", C61B, side, capped(work), localOn(7, 13, 50))))
	h1 := classed("auxiliary", movedOff(work, requested("h1", C61B, Haiku, side, capped(work), c4.At.Add(100*time.Millisecond))))
	first, back, again := movedAfter(1, c1, work, capped(work)), movedAfter(2, c3, side, capped(side)), movedAfter(3, c4, work, capped(work))
	title := movedAfter(4, h1, work, capped(work))
	m := toldAll("two moves of one session off work to side in a day, and back between", run, localOn(7, 0, 0), localOn(7, 14, 0), first, back, again, title)
	m.Lines = []ledger.Line{c0, h0, c1, c2, c3, c4, h1}
	m.Moves = map[Key]Move{
		keyOf(run, first): {Request: "c1", Written: 500_000, Cost: writeCost(500_000)},
		keyOf(run, back):  {Request: "c3", Written: 250_000, Cost: writeCost(250_000)},
		keyOf(run, again): {Request: "c4", Written: 1_000_000, Cost: writeCost(1_000_000)},
		keyOf(run, title): {Request: "h1"},
	}
	return m
}

// capMovingOneTwice is work's cap moving c61b off to side, side's cap
// moving it back, and work's cap moving it off again, the request moving it
// in flight, its line yet to come: the cap's second move is of no request
// yet, never the like move before it, so the cap's card counts the first
// move's cost once in all.
func capMovingOneTwice() EventMoment {
	run := localOn(7, 8, 0)
	cap := status.Event{ID: 1, At: localOn(7, 10, 29), Kind: status.EventCap, Account: work, Windows: []string{fiveHour}, Until: localOn(7, 13, 0), Reserve: 0.05}
	c0 := asked("c0", C61B, work, status.ReasonNew, localOn(7, 10, 0))
	c1 := writing(500_000, movedOff(work, asked("c1", C61B, side, capped(work), localOn(7, 10, 30))))
	c2 := writing(250_000, movedOff(side, asked("c2", C61B, work, capped(side), localOn(7, 11, 0))))
	// c3 moves c61b off work again, told at 11:20:00.3, its line yet to come.
	c3 := asked("c3", C61B, side, capped(work), localOn(7, 11, 20))
	first, back, again := counted(movedAfter(2, c1, work, capped(work)), cap), movedAfter(3, c2, side, capped(side)), counted(movedAfter(4, c3, work, capped(work)), cap)
	m := toldAll("work's cap moving one session off it twice, back between, its second move's request in flight", run, localOn(7, 0, 0), localOn(7, 11, 20).Add(5*time.Second),
		counting(cap, 1, side), first, back, again)
	m.Lines = []ledger.Line{c0, c1, c2}
	m.Facts = map[Key]Facts{keyOf(run, cap): {Moved: 1, To: side, Windows: fiveHourNamed}}
	m.Moves = map[Key]Move{
		keyOf(run, first): {Request: "c1", Written: 500_000, Cost: writeCost(500_000)},
		keyOf(run, back):  {Request: "c2", Written: 250_000, Cost: writeCost(250_000)},
	}
	m.Details = map[Key]Details{keyOf(run, cap): {
		Headline: "work reached its cap",
		Happened: []string{"event: work reached its cap", "its cap: 95% of its 5-hour window"},
		Then:     []string{"account: work", "resets: 13:00", "sessions: 1"},
		Followed: []string{"c61b: to side $4.00", "c61b: to side none", "new sessions: till 13:00", "in all: $4.00"},
	}}
	return m
}

// sideMoveSettled is a side request moving d28c off work at its cap, its
// conversation's next request writing its context again on side a minute
// later: the move is the side request's, what it came to the next one's.
func sideMoveSettled() EventMoment {
	run := localOn(7, 8, 0)
	d0 := asked("d0", D28C, work, status.ReasonNew, localOn(7, 14, 0))
	d1 := classed("auxiliary", movedOff(work, asked("d1", D28C, side, capped(work), localOn(7, 14, 30))))
	d2 := writing(1_000_000, asked("d2", D28C, side, status.ReasonSticky, localOn(7, 14, 31)))
	move := movedAfter(1, d1, work, capped(work))
	m := toldAll("a side request's move, settled on its conversation's next request", run, localOn(7, 0, 0), localOn(7, 14, 40), move)
	m.Lines = []ledger.Line{d0, d1, d2}
	m.Moves = map[Key]Move{keyOf(run, move): {Request: "d1", Written: 1_000_000, Cost: writeCost(1_000_000)}}
	m.Details = map[Key]Details{keyOf(run, move): {
		Headline: "work reached its cap",
		Happened: []string{"event: work reached its cap", "session: d28c switchboard", "model: opus-5-5", "from: work", "to: side"},
		Followed: []string{"request: d1", "cached again: 1000000", "the move: $8.00"},
	}}
	return m
}

// unpricedMove is work's cap moving d28c, on a model the price table doesn't
// know, off to side, its cache warm: writing its context again on side is
// unpriced, and said so, never left unsaid, of the move and in all on the
// cap's card.
func unpricedMove() EventMoment {
	run := localOn(7, 8, 0)
	cap := status.Event{ID: 1, At: localOn(7, 14, 29), Kind: status.EventCap, Account: work, Windows: []string{fiveHour}, Until: localOn(7, 17, 0), Reserve: 0.05}
	d0 := unpriced(asked("d0", D28C, work, status.ReasonNew, localOn(7, 14, 0)))
	d1 := unpriced(writing(1_000_000, movedOff(work, asked("d1", D28C, side, capped(work), localOn(7, 14, 30)))))
	move := counted(movedAfter(2, d1, work, capped(work)), cap)
	m := toldAll("a move on a model the price table can't price, counted by work's cap", run, localOn(7, 0, 0), localOn(7, 14, 40), counting(cap, 1, side), move)
	m.Lines = []ledger.Line{d0, d1}
	m.Facts = map[Key]Facts{keyOf(run, cap): {Moved: 1, To: side, Windows: fiveHourNamed}}
	m.Moves = map[Key]Move{keyOf(run, move): {Request: "d1", Written: 1_000_000}}
	m.Details = map[Key]Details{
		keyOf(run, cap): {
			Headline: "work reached its cap",
			Happened: []string{"event: work reached its cap", "its cap: 95% of its 5-hour window"},
			Then:     []string{"account: work", "resets: 17:00", "sessions: 1"},
			Followed: []string{"d28c: to side unpriced", "new sessions: till 17:00", "in all: unpriced"},
		},
		keyOf(run, move): {
			Headline: "work reached its cap",
			Happened: []string{"event: work reached its cap", "session: d28c switchboard", "model: opus-9", "from: work", "to: side"},
			Followed: []string{"request: d1", "cached again: 1000000", "the move: unpriced"},
		},
	}
	return m
}

// unpriced returns l, asking for a model the price table doesn't know.
func unpriced(l ledger.Line) ledger.Line {
	l.Model = "claude-opus-9"
	return l
}

// coldMove is c61b moved off work two hours after its last request, its
// cache run out an hour since: writing its context again cost nothing
// staying wouldn't have.
func coldMove() EventMoment {
	run := localOn(7, 8, 0)
	c0 := asked("c0", C61B, work, status.ReasonNew, localOn(7, 12, 0))
	c1 := writing(1_000_000, movedOff(work, asked("c1", C61B, side, capped(work), localOn(7, 14, 0))))
	move := movedAfter(1, c1, work, capped(work))
	m := toldAll("a move after its cache ran out", run, localOn(7, 0, 0), localOn(7, 14, 10), move)
	m.Lines = []ledger.Line{c0, c1}
	m.Moves = map[Key]Move{keyOf(run, move): {Request: "c1"}}
	m.Details = map[Key]Details{keyOf(run, move): {
		Headline: "work reached its cap",
		Happened: []string{"event: work reached its cap", "session: c61b mobile", "model: opus-5-5", "from: work", "to: side"},
		Followed: []string{"request: c1", "cached again: 0", "the move: none"},
	}}
	return m
}

// capWorld is work reaching its cap at 14:31:03 on 7 October, 95% of its
// 5-hour window, resetting at 17:10, with d28c, 7f3a and db8a on it in the
// hour before, each reading the window as it rose, 7f3a's request at
// 14:30:51 the first to read it at its cap since db8a's read it there
// before the window last reset, and c61b on it before that hour; then each
// of the three moved to side as its next request came, the cap filed again
// as each counts. The lines and versions are those by the time given.
func capWorld(by time.Time) (run time.Time, cap status.Event, moves []status.Event, lines []ledger.Line, versions []Filing) {
	run = localOn(7, 8, 0)
	resets := localOn(7, 17, 10)
	at := func(hour, minute, second int) time.Time {
		return localOn(7, hour, minute).Add(time.Duration(second) * time.Second)
	}
	onWork := func(id, session, reason string, when time.Time, used float64) ledger.Line {
		return reading(resets, asked(id, session, work, reason, when), fiveHour, used, week, 0.34)
	}
	cap = status.Event{ID: 1, At: at(14, 31, 3), Kind: status.EventCap, Account: work, Windows: []string{fiveHour}, Until: resets, Reserve: 0.05}
	all := []ledger.Line{
		reading(localOn(7, 12, 10), asked("b9", sDB8A, work, status.ReasonNew, at(11, 30, 0)), fiveHour, 0.96, week, 0.3),
		asked("c0", C61B, work, status.ReasonNew, at(13, 20, 0)),
		onWork("d0", D28C, status.ReasonNew, at(13, 40, 0), 0.8),
		onWork("b0", sDB8A, status.ReasonSticky, at(13, 50, 0), 0.85),
		onWork("f0", s7F3A, status.ReasonNew, at(14, 12, 30), 0.9),
		onWork("d1", D28C, status.ReasonSticky, at(14, 20, 0), 0.93),
		onWork("f1", s7F3A, status.ReasonSticky, at(14, 30, 51), 0.96),
		onWork("b1", sDB8A, status.ReasonSticky, at(14, 30, 58), 0.97),
	}
	versions = []Filing{filed(run, cap, cap.At)}
	movers := []ledger.Line{
		writing(250_000, movedOff(work, asked("d2", D28C, side, capped(work), at(14, 31, 13)))),
		writing(500_000, movedOff(work, asked("f2", s7F3A, side, capped(work), at(14, 31, 20)))),
		writing(125_000, movedOff(work, asked("b2", sDB8A, side, capped(work), at(14, 31, 40)))),
	}
	for i, l := range movers {
		move := counted(movedAfter(2+i, l, work, capped(work)), cap)
		if move.At.After(by) {
			break
		}
		all, moves = append(all, l), append(moves, move)
		versions = append(versions, filed(run, move, move.At), filed(run, counting(cap, len(moves), side), move.At))
	}
	return run, cap, moves, all, versions
}

// capCounting is capWorld read at 14:31:30, as its second move counts: the
// files hold the cap as its first move counted, the document as its second
// has, and the third session yet to move.
func capCounting() EventMoment {
	now := localOn(7, 14, 31).Add(30 * time.Second)
	run, cap, moves, lines, versions := capWorld(now)
	last := counting(cap, len(moves), side)
	return EventMoment{
		Name:  "a cap whose moves count after it's told, read as its second counts",
		Filed: versions[:len(versions)-2],
		Live:  []status.Event{moves[1], last, moves[0]},
		Run:   run, Now: now, Since: localOn(7, 0, 0), From: localOn(7, 0, 0),
		Events: []events.Line{{Event: last, Run: run}, {Event: moves[0], Run: run}, {Event: moves[1], Run: run}},
		Facts:  map[Key]Facts{keyOf(run, cap): {Moved: 2, To: side, Windows: fiveHourNamed}},
		Lines:  lines,
		Moves: map[Key]Move{
			keyOf(run, moves[0]): {Request: "d2", Written: 250_000, Cost: writeCost(250_000)},
			keyOf(run, moves[1]): {Request: "f2", Written: 500_000, Cost: writeCost(500_000)},
		},
		Details: map[Key]Details{keyOf(run, cap): capSaid("d28c: to side $2.00", "7f3a: to side $4.00", "new sessions: till 17:10", "in all: $6.00")},
	}
}

// capSaid is what's wanted of capWorld's cap's details card, its moves and
// what followed them as given.
func capSaid(followed ...string) Details {
	return Details{
		Headline:  "work reached its cap",
		Happened:  []string{"event: work reached its cap", "its cap: 95% of its 5-hour window", "known from: 7f3a's request at 14:30:51"},
		KnownFrom: "f1",
		Then:      []string{"account: work", "week: 34%", "5-hour: 97%", "resets: 17:10", "sessions: 3"},
		Followed:  followed,
	}
}

// capDetails is capWorld read at 14:42:07, each of its three moves counted,
// and c61b moved off work since by a choice the cap didn't force.
func capDetails() EventMoment {
	now := localOn(7, 14, 42).Add(7 * time.Second)
	run, cap, moves, lines, versions := capWorld(now)
	last := counting(cap, len(moves), side)
	rescored := status.Event{ID: 5, At: localOn(7, 14, 35), Kind: status.EventMoved, Session: C61B, Model: Opus, From: work, To: side, Reason: status.Rescored(time.Hour)}
	return EventMoment{
		Name:  "a cap moving three sessions, each counted, and a move it didn't force",
		Filed: append(versions, filed(run, rescored, rescored.At)),
		Live:  []status.Event{rescored, moves[2], moves[1], moves[0], last},
		Run:   run, Now: now, Since: localOn(7, 0, 0), From: localOn(7, 0, 0),
		Events: []events.Line{
			{Event: last, Run: run}, {Event: moves[0], Run: run}, {Event: moves[1], Run: run}, {Event: moves[2], Run: run}, {Event: rescored, Run: run},
		},
		Facts: map[Key]Facts{keyOf(run, cap): {Moved: 3, To: side, Windows: fiveHourNamed}},
		Lines: lines,
		Moves: map[Key]Move{
			keyOf(run, moves[0]): {Request: "d2", Written: 250_000, Cost: writeCost(250_000)},
			keyOf(run, moves[1]): {Request: "f2", Written: 500_000, Cost: writeCost(500_000)},
			keyOf(run, moves[2]): {Request: "b2", Written: 125_000, Cost: writeCost(125_000)},
		},
		Details: map[Key]Details{
			keyOf(run, cap): capSaid("d28c: to side $2.00", "7f3a: to side $4.00", "db8a: to side $1.00", "new sessions: till 17:10", "in all: $7.00"),
		},
	}
}

// familyRefused is work refusing c61b's opus requests alone, with a 403,
// till 13:40: its opus moves to side as the request refused goes out again
// there, its haiku staying on work, answered there a moment later, before
// the router told of the refusal.
func familyRefused() EventMoment {
	run := localOn(7, 8, 0)
	c0 := asked("c0", C61B, work, status.ReasonNew, localOn(7, 12, 0))
	h0 := classed("auxiliary", requested("h0", C61B, Haiku, work, status.ReasonNew, localOn(7, 12, 1)))
	c1 := writing(500_000, tried(status.WhyRefused, movedOff(work, asked("c1", C61B, side, refusedOff, localOn(7, 12, 40))), work))
	h1 := classed("auxiliary", requested("h1", C61B, Haiku, work, status.ReasonSticky, c1.At.Add(50*time.Millisecond)))
	refusal := status.Event{ID: 1, At: c1.At.Add(100 * time.Millisecond), Kind: status.EventRefused, Account: work, Status: 403, Family: "opus", Until: localOn(7, 13, 40)}
	move := counted(movedAfter(2, c1, work, refusedOff), refusal)
	m := toldAll("a model family refused alone, its sessions' opus moving and their haiku staying", run, localOn(7, 0, 0), localOn(7, 12, 45),
		counting(refusal, 1, side), move)
	m.Lines = []ledger.Line{c0, h0, c1, h1}
	m.Facts = map[Key]Facts{keyOf(run, refusal): {Moved: 1, To: side}}
	m.Moves = map[Key]Move{keyOf(run, move): {Request: "c1", Written: 500_000, Cost: writeCost(500_000)}}
	m.Details = map[Key]Details{keyOf(run, refusal): {
		Headline: "work's opus requests refused",
		Happened: []string{
			"event: work's opus requests refused", "answer: 403", "family: opus", "till: 13:40", "known from: c61b's request at 12:40:00",
		},
		KnownFrom: "c1",
		Then:      []string{"account: work", "sessions: 1"},
		Followed:  []string{"c61b: to side $4.00", "new sessions: opus till 13:40", "in all: $4.00"},
	}}
	return m
}

// pins is the global pin set to three accounts from the command line, a
// session's own pin set from the dashboard, and the global pin cleared.
func pins() EventMoment {
	run := localOn(7, 8, 0)
	global := status.Event{ID: 1, At: localOn(7, 9, 0), Kind: status.EventPin, Account: work, Accounts: []string{work, side, personal}, By: "cli"}
	own := status.Event{ID: 2, At: localOn(7, 9, 30), Kind: status.EventPin, Account: side, Session: D28C, By: "dashboard"}
	cleared := status.Event{ID: 3, At: localOn(7, 10, 0), Kind: status.EventAuto, Accounts: []string{work, side, personal}, By: "dashboard"}
	m := toldAll("the global pin set to three accounts, a session's own pin, and the global pin cleared", run, localOn(7, 0, 0), localOn(7, 10, 30), global, own, cleared)
	m.Lines = []ledger.Line{asked("d0", D28C, side, status.ReasonGlobalPin, localOn(7, 9, 10))}
	m.Details = map[Key]Details{
		keyOf(run, global): {
			Headline: "pin set by hand",
			Happened: []string{"event: pin set by hand", "accounts: work, side, personal", "goes: new sessions", "set from: the command line"},
		},
		keyOf(run, own): {
			Headline: "d28c pinned to side",
			Happened: []string{"event: d28c pinned to side", "session: d28c switchboard", "accounts: side", "set from: the dashboard"},
			Then:     []string{"account: side", "sessions: 1"},
		},
		keyOf(run, cleared): {
			Headline: "new sessions back to the router's choice",
			Happened: []string{"event: new sessions back to the router's choice", "from: work, side, personal", "set from: the dashboard"},
		},
	}
	return m
}

// startedRead is c61b starting on work, its first answer reading work's week
// to reset before its 5-hour: the card lists the week first, but says the
// 5-hour's reset, the shortest window it read, as an event that names no
// window and no end of its own says.
func startedRead() EventMoment {
	run := localOn(7, 8, 0)
	c0 := reading(localOn(7, 14, 0), asked("c0", C61B, work, status.ReasonNew, localOn(7, 10, 0)), fiveHour, 0.2, week, 0.5)
	c0.Limits[week+"-reset"] = fmt.Sprint(localOn(7, 12, 0).Unix())
	started := status.Event{ID: 1, At: localOn(7, 10, 1), Kind: status.EventStarted, Account: work, Session: C61B, Model: Opus, Reason: status.ReasonNew}
	m := toldAll("a session started, its account's week to reset before its 5-hour", run, localOn(7, 0, 0), localOn(7, 11, 0), started)
	m.Lines = []ledger.Line{c0}
	m.Details = map[Key]Details{keyOf(run, started): {
		Headline: "c61b started on work", Happened: []string{"event: c61b started on work", "session: c61b mobile", "model: opus-5-5"},
		Then: []string{"account: work", "week: 50%", "5-hour: 20%", "resets: 14:00", "sessions: 1"},
	}}
	return m
}

// filledFromYesterday is a router that's told of nothing today, the Log's
// Events showing yesterday's: side primed, a session started, work open
// again, and a limit reached that lifts today.
func filledFromYesterday() EventMoment {
	run := localOn(6, 9, 0)
	primed := status.Event{ID: 1, At: localOn(6, 9, 30), Kind: status.EventPrimed, Account: side, Windows: []string{fiveHour}, Until: localOn(6, 14, 30)}
	started := status.Event{ID: 2, At: localOn(6, 10, 0), Kind: status.EventStarted, Account: work, Session: C61B, Model: Opus, Reason: status.ReasonNew}
	open := status.Event{ID: 3, At: localOn(6, 18, 0), Kind: status.EventRoom, Account: work, Windows: []string{fiveHour}}
	limit := limitOf(4, localOn(6, 22, 0), localOn(7, 2, 0), fiveHour)
	m := toldAll("an event-free today, filled from yesterday", run, localOn(6, 0, 0), localOn(7, 9, 0), primed, started, open, limit)
	m.Facts = map[Key]Facts{keyOf(run, primed): {Windows: fiveHourNamed}, keyOf(run, open): {Windows: fiveHourNamed}, keyOf(run, limit): {Windows: fiveHourNamed}}
	m.Lines = []ledger.Line{asked("c0", C61B, work, status.ReasonNew, localOn(6, 10, 0))}
	m.Details = map[Key]Details{
		keyOf(run, primed): {
			Headline: "side primed", Happened: []string{"event: side primed", "window: its 5-hour window"},
			Then: []string{"account: side", "resets: 14:30", "sessions: 0"},
		},
		keyOf(run, started): {
			Headline: "c61b started on work", Happened: []string{"event: c61b started on work", "session: c61b mobile", "model: opus-5-5"},
			Then: []string{"account: work", "sessions: 1"},
		},
		keyOf(run, limit): {
			Headline: "work reached its limit", Happened: []string{"event: work reached its limit", "its limit: its 5-hour window", "till: Wed 02:00"},
			Then:     []string{"account: work", "resets: Wed 02:00", "sessions: 0"},
			Followed: []string{"new sessions: till Wed 02:00"},
		},
	}
	return m
}

// healthTurning is the router turning unhealthy, then well again, and a
// restart falling due as its config changed.
func healthTurning() EventMoment {
	run := localOn(7, 8, 0)
	unhealthy := status.Event{ID: 1, At: localOn(7, 10, 0), Kind: status.EventHealth, Reason: "most requests failing"}
	well := status.Event{ID: 2, At: localOn(7, 10, 5), Kind: status.EventHealth}
	due := status.Event{ID: 3, At: localOn(7, 11, 0), Kind: status.EventRestart, Reason: status.RestartForConfig}
	m := toldAll("the router unhealthy, then well again, and a restart falling due", run, localOn(7, 0, 0), localOn(7, 11, 30), unhealthy, well, due)
	m.Details = map[Key]Details{
		keyOf(run, unhealthy): {Headline: "the router turned unhealthy", Happened: []string{"event: the router turned unhealthy", "why: most requests failing"}},
		keyOf(run, well):      {Headline: "the router is healthy again", Happened: []string{"event: the router is healthy again"}},
		keyOf(run, due):       {Headline: "restart due", Happened: []string{"event: restart due", "why: the config changed"}},
	}
	return m
}
