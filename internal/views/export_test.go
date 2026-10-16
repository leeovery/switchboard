package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// TakenFacts are an event's facts as eventFacts work them out, for the
// package's tests outside it, which the shared hard moments' package, itself
// importing views, keeps out of the package.
type TakenFacts struct {
	RunsOut time.Time
	Moved   int
	To      string
	Windows []string
}

// TakeFacts works out the facts of each of the events, taken in turn, oldest
// first, their windows named as name names them.
func TakeFacts(name func(key string) string, events []status.Event) []TakenFacts {
	facts := eventFacts{name: name}
	taken := make([]TakenFacts, len(events))
	for i, e := range events {
		f := facts.take(e)
		taken[i] = TakenFacts{RunsOut: f.runsOut, Moved: f.moved, To: f.to, Windows: f.windows}
	}
	return taken
}

// RowOfLine is the row of the request l, of a session whose lines, oldest
// first, are those given, its story told from them, at now's prices.
func RowOfLine(l ledger.Line, session []ledger.Line, prices ledger.Table, now time.Time) RequestRow {
	today := Today(now)
	r := lineRow(&l, storyOf(pricedRequestsOf(session, prices, today.Date), prices, today))
	r.Worth = summed(prices, &l, today.Date)
	return r
}

// LinedDetails are the details of the request whose row is given, its line l
// come.
func LinedDetails(row RequestRow, l ledger.Line) RequestDetails {
	d := RequestDetails{Row: row}
	d.lined(&l)
	return d
}

// CountingPrices is r counting in priced each line it prices from then on,
// by its request's id and its arrival, as it prices it.
func CountingPrices(r LogRead, priced map[string]int) LogRead {
	prices := r.prices
	r.price = func(l *ledger.Line, date string) Worth {
		priced[l.Request+" "+l.At.String()]++
		return summed(prices, l, date)
	}
	return r
}

// CountingLanes are lanes not yet looked at that count in judged each time
// a look judges how an account stood at a moment gone, as Then judges it.
func CountingLanes(judged *int) Lanes {
	return Lanes{then: func(m status.Moment, a status.AccountNow, read status.Readings, reach status.Reach, told []status.Span, t time.Time) status.Was {
		*judged++
		return m.Then(a, read, reach, told, t)
	}}
}

// CountingPoolRoom is the pool's room not yet looked at that counts in judged
// each time a look judges the room an account had left in a window at a
// moment gone, as RoomThen judges it.
func CountingPoolRoom(judged *int) PoolRoom {
	return PoolRoom{then: func(m status.Moment, a status.AccountNow, read status.Readings, told []status.Span, key string, t time.Time) (float64, bool) {
		*judged++
		return m.RoomThen(a, read, told, key, t)
	}}
}

// NotingPoolRoom is the pool's room not yet looked at that notes in at each
// moment a look judges the room an account had left in a window then, as
// RoomThen judges it.
func NotingPoolRoom(at *[]time.Time) PoolRoom {
	return PoolRoom{then: func(m status.Moment, a status.AccountNow, read status.Readings, told []status.Span, key string, t time.Time) (float64, bool) {
		*at = append(*at, t)
		return m.RoomThen(a, read, told, key, t)
	}}
}

// RowOfTold is the row of the request t, as the stream told of it, at now.
func RowOfTold(t Told, now time.Time) RequestRow {
	return toldRow(t, now)
}

// From is when the period begins at now, weeks starting on the day given: at
// the start of its first local day.
func (p AccountsPeriod) From(now time.Time, starts time.Weekday) time.Time {
	start, _, _ := dayfile.Day(p.first(dayfile.CivilOf(now), starts).Format(time.DateOnly))
	return start
}

// WhyRunning is why the model of the session with the given id is on its
// account at the moment m, as SESSIONS says it, as the List gives where that
// model goes and why, in sm.
func WhyRunning(m status.Moment, session string, sm SessionModel) status.Words {
	return whyRunning(m, session, status.Assignment{Model: sm.Model, Account: sm.Account, Reason: sm.Reason, AssignedAt: sm.Since})
}
