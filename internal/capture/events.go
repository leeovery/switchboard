package capture

import (
	"slices"
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
// off work at its cap, one off personal at its limit, and one off work
// refused.
const (
	reasonCapped  = status.ReasonMovedOff + "work " + status.WhyReserve
	reasonLimited = status.ReasonMovedOff + "personal " + status.WhyLimit
	reasonRefused = status.ReasonMovedOff + "work " + status.WhyRefused
)

// rescoredUnderPressure is the router's reason, as its log has it, for
// choosing afresh the account of a session that idled as long as idle says,
// passing the account given over under pressure.
func rescoredUnderPressure(idle, account string) string {
	return status.PassingOver(status.Rescored(idle), account)
}

// todaysLimit is the identity of a limit a world's account reached today:
// the run's second, personal's of yesterday its first.
const todaysLimit = personalLimit + 1

// eventsKept is how many of its events the router's status document gives:
// the newest, as the router keeps them.
const eventsKept = 50

// routerEvents are the auto world's router's events, of the day now falls on
// and of the evening before, a line each as it last stands, as events -f
// reads them.
func routerEvents(now time.Time) []events.Line {
	told := autoEvents(now)
	run := routerRun(now)
	lines := make([]events.Line, len(told))
	for i, e := range told {
		lines[i] = events.Line{Event: e, Run: run}
	}
	return lines
}

// autoEvents are the auto world's router's events, numbered as it numbers
// them: as happenings' auto has them.
func autoEvents(now time.Time) []status.Event {
	return ofTheRun(happenings{now}.auto())
}

// ofTheRun are the events given as the router that started at routerRun
// numbers them: in the order they happened, each with an id one more than
// the one's before, from 1; and each move forced counted in the newest cap,
// limit or refusal of the account it left, no newer than it, and in a limit,
// as that limit's.
func ofTheRun(happened []status.Event) []status.Event {
	told := slices.Clone(happened)
	slices.SortStableFunc(told, func(a, b status.Event) int { return a.At.Compare(b.At) })
	for i := range told {
		told[i].ID = i + 1
		if told[i].ForcedBy == forced {
			told[i] = countedIn(told[i], told[:i])
		}
	}
	return told
}

// countedIn is the move counted in the newest cap, limit or refusal of the
// account it left among those told before it, and in a limit, as that
// limit's: in none where there's none.
func countedIn(move status.Event, before []status.Event) status.Event {
	move.ForcedBy = 0
	for _, by := range slices.Backward(before) {
		if !forces(by.Kind) || by.Account != move.From {
			continue
		}
		move.ForcedBy = by.ID
		if by.Kind == status.EventLimit {
			move.Limit = by.ID
		}
		break
	}
	return move
}

// forces reports whether an event of the kind given moves the sessions on its
// account: a cap's, a limit's or a refusal's.
func forces(kind string) bool {
	return kind == status.EventCap || kind == status.EventLimit || kind == status.EventRefused
}

// toldAt are the router's events its status document gives at at, of those
// told, which are oldest first: those that happened by then, the newest
// first, as many as it keeps.
func toldAt(told []status.Event, at time.Time) []status.Event {
	var given []status.Event
	for _, e := range slices.Backward(told) {
		if len(given) == eventsKept {
			break
		}
		if !e.At.After(at) {
			given = append(given, e)
		}
	}
	return given
}

// happenings are the router's events as the frames' worlds have them happen,
// on the day now falls on and the evening before, each world's built of what
// they share: unnumbered, a move forced not yet counted in what forced it, as
// ofTheRun numbers and counts them.
type happenings struct {
	now time.Time
}

// at is a time of the day days after now's.
func (h happenings) at(days, hour, minute, second int) time.Time {
	return clockAt(h.now, days, hour, minute, second)
}

// today is a time of now's day.
func (h happenings) today(hour, minute, second int) time.Time {
	return h.at(0, hour, minute, second)
}

// started is the session's start on the account, its first request, of opus,
// having come at the time given, and why it went there.
func (h happenings) started(at time.Time, session, account, reason string) status.Event {
	return status.Event{Kind: status.EventStarted, At: at.Add(startedAfter), Account: account, Session: session, Model: opus, Reason: reason}
}

// primed is a prime starting the account's 5-hour window at the time given,
// to reset at resets.
func (h happenings) primed(at time.Time, account string, resets time.Time) status.Event {
	return status.Event{Kind: status.EventPrimed, At: at, Account: account, Windows: []string{fiveHourKey}, Until: resets}
}

// moved is the session's requests of opus moving from one account to another
// at the time given, and why.
func (h happenings) moved(at time.Time, session, from, to, reason string) status.Event {
	return status.Event{Kind: status.EventMoved, At: at, Session: session, Model: opus, From: from, To: to, Reason: reason}
}

// forcedOff is the session's move, as moved says, forced by the newest cap,
// limit or refusal of the account it left.
func (h happenings) forcedOff(at time.Time, session, from, to, reason string) status.Event {
	e := h.moved(at, session, from, to, reason)
	e.ForcedBy = forced
	return e
}

// pinned is the global pin set from the dashboard at the time given, new
// sessions going to the accounts given.
func (h happenings) pinned(at time.Time, accounts ...string) status.Event {
	return status.Event{Kind: status.EventPin, At: at, Account: accounts[0], Accounts: accounts, By: "dashboard"}
}

// yesterday are the evening before's events, from 8a2c's start to personal's
// window reset (log2.YESTERDAY): personal's limit at 19:12:40, the run's
// first, moving 8a2c to side.
func (h happenings) yesterday() []status.Event {
	return []status.Event{
		h.started(h.at(-1, 16, 55, 20), id8A2C, "personal", status.ReasonNew),
		{Kind: status.EventRoom, At: h.at(-1, 17, 10, 0), Account: "work", Windows: []string{fiveHourKey}},
		{Kind: status.EventPin, At: h.at(-1, 17, 20, 15), Account: "personal", Accounts: []string{"personal"}, By: "cli"},
		{Kind: status.EventAuto, At: h.at(-1, 18, 2, 40), Accounts: []string{"personal"}, By: "dashboard"},
		{Kind: status.EventLimit, At: h.at(-1, 19, 12, 40), Account: "personal", Windows: []string{fiveHourKey}, Until: h.at(-1, 23, 58, 0), Count: 1,
			To: "side", Limit: personalLimit},
		h.forcedOff(h.at(-1, 19, 12, 52), id8A2C, "personal", "side", reasonLimited),
		h.started(h.at(-1, 23, 47, 30), id5E7D, "side", status.ReasonNew),
		{Kind: status.EventRoom, At: h.at(-1, 23, 58, 40), Account: "personal", Windows: []string{fiveHourKey}},
	}
}

// morning are today's events from 07:05:12 to side's prime at 13:50 (log2.
// TODAY): four primes, personal's as given, seven sessions started, and db8a
// moved from personal to work at 13:22:40, chosen afresh after it idled since
// yesterday's 16:07:19, personal under pressure.
func (h happenings) morning(personalPrimed status.Event) []status.Event {
	return []status.Event{
		h.primed(h.today(7, 5, 12), "work", h.today(12, 5, 0)),
		h.started(h.today(7, 58, 30), id2B6D, "work", status.ReasonNew),
		h.started(h.today(8, 31, 2), idA8F1, "side", status.ReasonNew),
		h.primed(h.today(8, 47, 0), "side", h.today(13, 47, 0)),
		h.started(h.today(9, 12, 40), idE0C4, "work", status.ReasonNew),
		h.started(h.today(9, 40, 5), id3D7A, "work", status.ReasonNew),
		h.started(h.today(10, 2, 18), id5B19, "personal", status.ReasonNew),
		personalPrimed,
		h.started(h.today(11, 55, 31), id9C2E, "personal", status.ReasonNew),
		h.started(h.today(12, 2, 10), idD28C, "work", status.ReasonNew),
		h.primed(h.today(12, 10, 0), "work", h.today(17, 10, 0)),
		h.started(h.today(12, 47, 12), id41E0, "personal", status.ReasonNew),
		h.moved(h.today(13, 22, 40), idDB8A, "personal", "work", rescoredUnderPressure("21h 15m", "personal")),
		h.primed(h.today(13, 50, 0), "side", h.today(18, 50, 0)),
	}
}

// personalPrimed is personal's 5-hour window primed at 11:30:02, to reset at
// 16:30, as the auto world reads it.
func (h happenings) personalPrimed() status.Event {
	return h.primed(h.today(11, 30, 2), "personal", h.today(16, 30, 0))
}

// personalPrimedToItsLimit is personal's 5-hour window primed at 10:54, to
// reset at 15:54, as a world where it reached its limit reads it.
func (h happenings) personalPrimedToItsLimit() status.Event {
	return h.primed(h.today(10, 54, 0), "personal", h.today(15, 54, 0))
}

// personalsLimit is personal reaching its 5-hour limit at 14:12, till 15:54,
// and the two sessions on it, 41e0 and 9c2e, moved to side then, as routing's
// world of a limit lists them.
func (h happenings) personalsLimit() []status.Event {
	at := h.today(14, 12, 0)
	return []status.Event{
		{Kind: status.EventLimit, At: at, Account: "personal", Windows: []string{fiveHourKey}, Until: h.today(15, 54, 0), Count: 2, To: "side",
			Limit: todaysLimit},
		h.forcedOff(at, id41E0, "personal", "side", reasonLimited),
		h.forcedOff(at, id9C2E, "personal", "side", reasonLimited),
	}
}

// worksCap is work reaching its cap at 14:31:03, 95% of its 5-hour window,
// and the three sessions it moved to side, d28c, 7f3a and db8a, as their next
// requests came.
func (h happenings) worksCap() []status.Event {
	return []status.Event{
		{Kind: status.EventCap, At: h.today(14, 31, 3), Account: "work", Windows: []string{fiveHourKey}, Until: h.today(17, 10, 0),
			Reserve: workReserve, Count: 3, To: "side"},
		h.forcedOff(h.today(14, 31, 13), idD28C, "work", "side", reasonCapped),
		h.forcedOff(h.today(14, 31, 20), id7F3A, "work", "side", reasonCapped),
		h.forcedOff(h.today(14, 31, 40), idDB8A, "work", "side", reasonCapped),
	}
}

// auto are the auto world's events (log2.YESTERDAY and log2.TODAY):
// yesterday's; the morning's; 7f3a started on work at 14:12:30; work's cap
// and its three moves; and c61b started on side at 14:41:11, the router's
// best.
func (h happenings) auto() []status.Event {
	return slices.Concat(
		h.yesterday(), h.morning(h.personalPrimed()),
		[]status.Event{h.started(h.today(14, 12, 30), id7F3A, "work", status.ReasonNew)},
		h.worksCap(),
		[]status.Event{h.started(h.today(14, 41, 11), idC61B, "side", status.ReasonNew)},
	)
}

// pinnedToPersonal are the pinned world's events: the auto world's to side's
// prime at 13:50; 7f3a started on work at 13:55:30, before the pin, as work's
// cap finds it there; new sessions pinned to personal at 14:02; work's cap
// and its three moves to side, as the frame has them, where a router would
// have sent them to personal, the pin's; and c61b started on personal, as
// pinned.
func (h happenings) pinnedToPersonal() []status.Event {
	return slices.Concat(
		h.yesterday(), h.morning(h.personalPrimed()),
		[]status.Event{h.started(h.today(13, 55, 30), id7F3A, "work", status.ReasonNew), h.pinned(h.today(14, 2, 0), "personal")},
		h.worksCap(),
		[]status.Event{h.started(h.today(14, 41, 11), idC61B, "personal", status.ReasonGlobalPin)},
	)
}

// atPersonalsLimit are routing's world of a limit's events: the auto world's,
// but personal primed at 10:54, and its limit at 14:12 moving 41e0 and 9c2e
// to side.
func (h happenings) atPersonalsLimit() []status.Event {
	return slices.Concat(
		h.yesterday(), h.morning(h.personalPrimedToItsLimit()), h.personalsLimit(),
		[]status.Event{h.started(h.today(14, 12, 30), id7F3A, "work", status.ReasonNew)},
		h.worksCap(),
		[]status.Event{h.started(h.today(14, 41, 11), idC61B, "side", status.ReasonNew)},
	)
}

// badHour are the bad hour's events: the auto world's to side's prime at
// 13:50, personal primed at 10:54; new sessions pinned to personal and side
// at 13:40; 7f3a started on side at 13:55:30, as pinned, before personal's
// limit, as the frame's LATELY tells nothing between that limit and work's
// refusal; personal's limit at 14:12 moving 41e0 and 9c2e to side; work's
// token refused at 14:20, as refusal says, moving d28c and db8a to side; and
// c61b started on side, as pinned. Work reached its cap at 14:15, which no
// event tells, as the frame tells none.
func (h happenings) badHour(refusal status.Refusal) []status.Event {
	refused := h.today(14, 20, 0)
	return slices.Concat(
		h.yesterday(), h.morning(h.personalPrimedToItsLimit()),
		[]status.Event{
			h.pinned(h.today(13, 40, 0), "personal", "side"),
			h.started(h.today(13, 55, 30), id7F3A, "side", status.ReasonGlobalPin),
		},
		h.personalsLimit(),
		[]status.Event{
			{Kind: status.EventRefused, At: refused, Account: "work", Until: refusal.Until, Status: refusal.Status, Count: 2, To: "side"},
			h.forcedOff(refused, idD28C, "work", "side", reasonRefused),
			h.forcedOff(h.today(14, 20, 41), idDB8A, "work", "side", reasonRefused),
			h.started(h.today(14, 41, 11), idC61B, "side", status.ReasonGlobalPin),
		},
	)
}
