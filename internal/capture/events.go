package capture

import (
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/status"
)

// The frames' sessions beyond milestone 5's samples and those yesterday's
// days name, each a Claude Code session's id, which the frames cut to its
// first four characters: those that ended today or yesterday, and f41a,
// resumed this morning.
const (
	id9C2E = "9c2e47b1-0d5a-4e83-b6f2-8a1c3e9d0f57"
	id2B6D = "2b6d8e03-a4c7-4f19-9d2b-6e05c1a8f374"
	idA8F1 = "a8f1c5d9-3b72-4e06-8f4a-d17b2e9c6a05"
	idE0C4 = "e0c4a917-6f2b-4d58-a3e1-0b9d7c4f2e68"
	id3D7A = "3d7a2c6e-8b14-4f90-b7d3-5e2a9f0c1b46"
	id5B19 = "5b19f0a4-c3d8-4e27-9a6b-1f8e4d2c7a03"
	idB9C4 = "b9c4d2a7-5e18-4f63-a0b9-e7c3f1d8256a"
	idF41A = "f41a8c3e-2b97-4d05-9e6a-0f4d1b7c8e29"
)

// clockAt is when the clocks in now's time zone first read the time of day
// given on the day days after now's, as clockOn says.
func clockAt(now time.Time, days, hour, minute, second int) time.Time {
	y, m, d := now.Date()
	return clockOn(time.Date(y, m, d+days, 0, 0, 0, 0, time.UTC), hour, minute, second, now.Location())
}

// workReserve is the share of each of work's windows the router leaves
// unused: its cap is 95% of them.
const workReserve = 0.05

// startedAfter is how long after a session's first request arrived the router
// tells it started, as its first answer of success comes: in the same second.
const startedAfter = 700 * time.Millisecond

// routerRun is when the router whose events the frames tell started:
// yesterday at 16:50, so its events' ids rise from 1 there.
func routerRun(now time.Time) time.Time {
	return clockAt(now, -1, 16, 50, 0)
}

// The router's reasons the frames' moves give, as its log has them: a move
// off work at its cap, and one off personal at its limit.
const (
	reasonCapped  = status.ReasonMovedOff + "work " + status.WhyReserve
	reasonLimited = status.ReasonMovedOff + "personal " + status.WhyLimit
)

// rescoredUnderPressure is the router's reason, as its log has it, for
// choosing afresh the account of a session that idled as long as idle says,
// passing the account given over under pressure.
func rescoredUnderPressure(idle, account string) string {
	return status.Rescored(idle) + ", " + account + " under pressure"
}

// The identities of the frames' limits and caps, as the router's events
// number them: personal's limit, yesterday, and work's cap, today.
const (
	personalLimitEvent = 5
	workCapEvent       = 24
)

// routerEvents are the router's events of the day now falls on and of the
// evening before, a line each as it last stands, as events -f reads them:
// yesterday's last, from 8a2c's start to personal's window reset; and today's
// 20 from 07:05:12, five primes, ten sessions started, the last c61b at
// 14:41:11, db8a moved from personal to work at 13:22:40, chosen afresh after
// it idled since yesterday's 16:07:19, personal under pressure, and work's cap
// at 14:31:03 and its three moves (log2.YESTERDAY and log2.TODAY).
func routerEvents(now time.Time) []events.Line {
	at := func(days, hour, minute, second int) time.Time { return clockAt(now, days, hour, minute, second) }
	run := routerRun(now)
	five := []string{fiveHourKey}
	started := func(days, hour, minute, second int, session, account string) status.Event {
		return status.Event{Kind: status.EventStarted, At: at(days, hour, minute, second).Add(startedAfter), Account: account, Session: session,
			Model: opus, Reason: status.ReasonNew}
	}
	primed := func(hour, minute, second int, account string, resets time.Time) status.Event {
		return status.Event{Kind: status.EventPrimed, At: at(0, hour, minute, second), Account: account, Windows: five, Until: resets}
	}
	moved := func(at time.Time, session, from, to, reason string, forcedBy int) status.Event {
		e := status.Event{Kind: status.EventMoved, At: at, Session: session, Model: opus, From: from, To: to, Reason: reason, ForcedBy: forcedBy}
		if forcedBy == personalLimitEvent {
			e.Limit = forcedBy
		}
		return e
	}
	happened := []status.Event{
		started(-1, 16, 55, 20, id8A2C, "personal"),
		{Kind: status.EventRoom, At: at(-1, 17, 10, 0), Account: "work", Windows: five},
		{Kind: status.EventPin, At: at(-1, 17, 20, 15), Account: "personal", Accounts: []string{"personal"}, By: "cli"},
		{Kind: status.EventAuto, At: at(-1, 18, 2, 40), Accounts: []string{"personal"}, By: "dashboard"},
		{Kind: status.EventLimit, At: at(-1, 19, 12, 40), Account: "personal", Windows: five, Until: at(-1, 23, 58, 0), Count: 1, To: "side",
			Limit: personalLimit},
		moved(at(-1, 19, 12, 52), id8A2C, "personal", "side", reasonLimited, personalLimitEvent),
		started(-1, 23, 47, 30, id5E7D, "side"),
		{Kind: status.EventRoom, At: at(-1, 23, 58, 40), Account: "personal", Windows: five},
		primed(7, 5, 12, "work", at(0, 12, 5, 0)),
		started(0, 7, 58, 30, id2B6D, "work"),
		started(0, 8, 31, 2, idA8F1, "side"),
		primed(8, 47, 0, "side", at(0, 13, 47, 0)),
		started(0, 9, 12, 40, idE0C4, "work"),
		started(0, 9, 40, 5, id3D7A, "work"),
		started(0, 10, 2, 18, id5B19, "personal"),
		primed(11, 30, 2, "personal", at(0, 16, 30, 0)),
		started(0, 11, 55, 31, id9C2E, "personal"),
		started(0, 12, 2, 10, idD28C, "work"),
		primed(12, 10, 0, "work", at(0, 17, 10, 0)),
		started(0, 12, 47, 12, id41E0, "personal"),
		moved(at(0, 13, 22, 40), idDB8A, "personal", "work", rescoredUnderPressure("21h 15m", "personal"), 0),
		primed(13, 50, 0, "side", at(0, 18, 50, 0)),
		started(0, 14, 12, 30, id7F3A, "work"),
		{Kind: status.EventCap, At: at(0, 14, 31, 3), Account: "work", Windows: five, Until: at(0, 17, 10, 0), Reserve: workReserve, Count: 3, To: "side"},
		moved(at(0, 14, 31, 13), idD28C, "work", "side", reasonCapped, workCapEvent),
		moved(at(0, 14, 31, 20), id7F3A, "work", "side", reasonCapped, workCapEvent),
		moved(at(0, 14, 31, 40), idDB8A, "work", "side", reasonCapped, workCapEvent),
		started(0, 14, 41, 11, idC61B, "side"),
	}
	lines := make([]events.Line, len(happened))
	for i, e := range happened {
		e.ID = i + 1
		lines[i] = events.Line{Event: e, Run: run}
	}
	return lines
}
