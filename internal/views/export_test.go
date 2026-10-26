package views

import (
	"slices"
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
	rec := lineRecordOf(&l)
	r := lineRow(&rec.rowRecord, storyOf(pricedRequestsOf(session, prices, today.Date), prices, today))
	r.Worth = rec.worth(prices, today.Date)
	return r
}

// Retold are the ids of the requests whose rows r's last change told again,
// in order: none where it told none again.
func Retold(r LogRead) []string {
	if len(r.changes) == 0 {
		return nil
	}
	var ids []string
	for _, l := range r.changes[len(r.changes)-1].retold {
		ids = append(ids, l.flight.request)
	}
	slices.Sort(ids)
	return ids
}

// LinedDetails are the details of the request whose row is given, its line l
// come.
func LinedDetails(row RequestRow, l ledger.Line) RequestDetails {
	rec := lineRecordOf(&l)
	d, _ := LogLine{Row: row, record: &rec}.Details()
	return d
}

// CountingPrices is r counting in priced each line it prices from then on,
// by its request's id and its arrival, as it prices it.
func CountingPrices(r LogRead, priced map[string]int) LogRead {
	prices := r.prices
	r.price = func(rec *lineRecord, date string) Worth {
		priced[rec.request+" "+rec.arrived().String()]++
		return rec.worth(prices, date)
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

// countingRoom judges the room an account had left in a window at a moment
// gone as RoomThen does, counting in judged each time it does.
func countingRoom(judged *int) roomJudge {
	return func(m status.Moment, a status.AccountNow, read status.Readings, told []status.Span, key string, t time.Time) (float64, bool) {
		*judged++
		return m.RoomThen(a, read, told, key, t)
	}
}

// notingRoom judges the room an account had left in a window at a moment
// gone as RoomThen does, noting in at each moment it judges.
func notingRoom(at *[]time.Time) roomJudge {
	return func(m status.Moment, a status.AccountNow, read status.Readings, told []status.Span, key string, t time.Time) (float64, bool) {
		*at = append(*at, t)
		return m.RoomThen(a, read, told, key, t)
	}
}

// CountingPoolStack is the pool's stack not yet looked at that counts in
// judged each time a look judges the room an account had left in a window
// at a moment gone, as RoomThen judges it.
func CountingPoolStack(judged *int) PoolStack {
	return PoolStack{then: countingRoom(judged)}
}

// NotingPoolStack is the pool's stack not yet looked at that notes in at each
// moment a look judges the room an account had left in a window then, as
// RoomThen judges it.
func NotingPoolStack(at *[]time.Time) PoolStack {
	return PoolStack{then: notingRoom(at)}
}

// CountingPoolSpan is the pool's room over a window's span not yet looked at
// that counts in judged each time a look judges the room an account had left
// in a window at a moment gone, as RoomThen judges it.
func CountingPoolSpan(judged *int) PoolSpan {
	return PoolSpan{then: countingRoom(judged)}
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

// Lines are t's lines, the oldest first, as Line gives each.
func (t LogTable) Lines() []LogLine {
	lines := make([]LogLine, 0, t.Len())
	for _, l := range t.All() {
		lines = append(lines, l)
	}
	return lines
}

// NewSinceHeld is how many of the rows t shows were of no request among
// the rows all holds, every one shown, on the days from all's first: the
// rows come since, as the Log counted them against the table it held as it
// paused, before it held a mark in its place.
func NewSinceHeld(t, all LogTable) int {
	n := 0
	for _, l := range t.All() {
		if !l.IsRow() || !all.lines.first.Holds(l.day.Start) {
			continue
		}
		held := false
		for _, o := range all.All() {
			held = held || o.IsRow() && o.Row.Request == l.Row.Request && runSpanOf(o.Row).meets(runSpanOf(l.Row))
		}
		if !held {
			n++
		}
	}
	return n
}
