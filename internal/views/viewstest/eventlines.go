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

// pressureTheDayBefore is a move off work at its cap 20 minutes after
// midnight, its choice passing personal over, whose pressure was told at
// 22:50 the day before, to run out at 01:10: the Log's Events show the day
// of the move alone, read from five hours before it.
func pressureTheDayBefore() EventMoment {
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
	return m
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
	return m
}

// capWorld is work reaching its cap at 14:31:03 on 7 October, 95% of its
// 5-hour window, resetting at 17:10, with d28c, 7f3a and db8a on it, each
// reading the window as it rose, 7f3a's request at 14:30:51 the first to
// read it at its cap; then each moved to side as its next request came, the
// cap filed again as each counts. The lines and versions are those by the
// time given.
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
		onWork("d0", D28C, status.ReasonNew, at(13, 40, 0), 0.8),
		onWork("b0", sDB8A, status.ReasonNew, at(13, 50, 0), 0.85),
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
	}
}

// capDetails is capWorld read at 14:42:07, each of its three moves counted.
func capDetails() EventMoment {
	now := localOn(7, 14, 42).Add(7 * time.Second)
	run, cap, moves, lines, versions := capWorld(now)
	last := counting(cap, len(moves), side)
	return EventMoment{
		Name:  "a cap moving three sessions, each counted",
		Filed: versions,
		Live:  []status.Event{moves[2], moves[1], moves[0], last},
		Run:   run, Now: now, Since: localOn(7, 0, 0), From: localOn(7, 0, 0),
		Events: []events.Line{{Event: last, Run: run}, {Event: moves[0], Run: run}, {Event: moves[1], Run: run}, {Event: moves[2], Run: run}},
		Facts:  map[Key]Facts{keyOf(run, cap): {Moved: 3, To: side, Windows: fiveHourNamed}},
		Lines:  lines,
		Moves: map[Key]Move{
			keyOf(run, moves[0]): {Request: "d2", Written: 250_000, Cost: writeCost(250_000)},
			keyOf(run, moves[1]): {Request: "f2", Written: 500_000, Cost: writeCost(500_000)},
			keyOf(run, moves[2]): {Request: "b2", Written: 125_000, Cost: writeCost(125_000)},
		},
	}
}

// familyRefused is work refusing c61b's opus requests alone, with a 403,
// till 13:40: its opus moves to side as the request refused goes out again
// there, its haiku staying on work.
func familyRefused() EventMoment {
	run := localOn(7, 8, 0)
	c0 := asked("c0", C61B, work, status.ReasonNew, localOn(7, 12, 0))
	h0 := classed("auxiliary", requested("h0", C61B, Haiku, work, status.ReasonNew, localOn(7, 12, 1)))
	c1 := writing(500_000, tried(status.WhyRefused, movedOff(work, asked("c1", C61B, side, refusedOff, localOn(7, 12, 40))), work))
	h1 := classed("auxiliary", requested("h1", C61B, Haiku, work, status.ReasonSticky, localOn(7, 12, 41)))
	refusal := status.Event{ID: 1, At: c1.At.Add(100 * time.Millisecond), Kind: status.EventRefused, Account: work, Status: 403, Family: "opus", Until: localOn(7, 13, 40)}
	move := counted(movedAfter(2, c1, work, refusedOff), refusal)
	m := toldAll("a model family refused alone, its sessions' opus moving and their haiku staying", run, localOn(7, 0, 0), localOn(7, 12, 45),
		counting(refusal, 1, side), move)
	m.Lines = []ledger.Line{c0, h0, c1, h1}
	m.Facts = map[Key]Facts{keyOf(run, refusal): {Moved: 1, To: side}}
	m.Moves = map[Key]Move{keyOf(run, move): {Request: "c1", Written: 500_000, Cost: writeCost(500_000)}}
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
	return m
}

// filledFromYesterday is a router that's told of nothing today, the Log's
// Events showing yesterday's: side primed, a session started, work open
// again, and a limit reached that lifts today.
func filledFromYesterday() EventMoment {
	run := localOn(6, 9, 0)
	primed := status.Event{ID: 1, At: localOn(6, 9, 30), Kind: status.EventPrimed, Account: side, Windows: []string{fiveHour}, Until: localOn(6, 14, 30)}
	started := status.Event{ID: 2, At: localOn(6, 10, 0), Kind: status.EventStarted, Account: work, Session: C61B, Reason: status.ReasonNew}
	open := status.Event{ID: 3, At: localOn(6, 18, 0), Kind: status.EventRoom, Account: work, Windows: []string{fiveHour}}
	limit := limitOf(4, localOn(6, 22, 0), localOn(7, 2, 0), fiveHour)
	m := toldAll("an event-free today, filled from yesterday", run, localOn(6, 0, 0), localOn(7, 9, 0), primed, started, open, limit)
	m.Facts = map[Key]Facts{keyOf(run, primed): {Windows: fiveHourNamed}, keyOf(run, open): {Windows: fiveHourNamed}, keyOf(run, limit): {Windows: fiveHourNamed}}
	m.Lines = []ledger.Line{asked("c0", C61B, work, status.ReasonNew, localOn(6, 10, 0))}
	return m
}

// healthTurning is the router turning unhealthy, then well again, and a
// restart falling due as its config changed.
func healthTurning() EventMoment {
	return toldAll("the router unhealthy, then well again, and a restart falling due", localOn(7, 8, 0), localOn(7, 0, 0), localOn(7, 11, 30),
		status.Event{ID: 1, At: localOn(7, 10, 0), Kind: status.EventHealth, Reason: "most requests failing"},
		status.Event{ID: 2, At: localOn(7, 10, 5), Kind: status.EventHealth},
		status.Event{ID: 3, At: localOn(7, 11, 0), Kind: status.EventRestart, Reason: status.RestartForConfig},
	)
}
