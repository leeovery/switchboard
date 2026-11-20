package views_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// framesDay is the time given on the day of September 2026 given, the month
// the signed-off frames of stepping back step through.
func framesDay(day, hour, minute int) time.Time {
	return time.Date(2026, 9, day, hour, minute, 0, 0, time.Local)
}

// pastLatelyRun is when the router that told pastLatelyLines started.
var pastLatelyRun = framesDay(20, 9, 0)

// pastLatelyLines are the router's files of events over the week to the
// frames' clock, Thursday 1 October, oldest first: work at its cap more
// than a week before Tuesday's end; open again on Monday; primed, under
// pressure, held as its subscription lapsed and side started on Tuesday; a
// session started on it on Wednesday; its hold lifted, and its cap, on
// Thursday.
func pastLatelyLines() []events.Line {
	lapsed := "subscription lapsed"
	told := []status.Event{
		{At: framesDay(22, 20, 0), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}},
		{At: framesDay(28, 21, 44), Kind: status.EventRoom, Account: "work", Windows: []string{"5h"}},
		{At: framesDay(29, 12, 10), Kind: status.EventPrimed, Account: "work", Windows: []string{"5h"}},
		{At: framesDay(29, 13, 22), Kind: status.EventPressure, Account: "work", Windows: []string{"5h"}, Until: framesDay(29, 13, 50)},
		{At: framesDay(29, 14, 31), Kind: status.EventHeld, Account: "work", Reason: lapsed, Status: 403},
		{At: framesDay(29, 15, 0), Kind: status.EventStarted, Account: "side", Session: "d28c0b1e", Reason: status.ReasonNew},
		{At: framesDay(30, 9, 12), Kind: status.EventStarted, Account: "work", Session: "c61b2f8d", Reason: status.ReasonNew},
		{At: time.Date(2026, 10, 1, 8, 24, 0, 0, time.Local), Kind: status.EventRoom, Account: "work", Reason: lapsed},
		{At: time.Date(2026, 10, 1, 10, 0, 0, 0, time.Local), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}},
	}
	lines := make([]events.Line, len(told))
	for i, e := range told {
		e.ID = i + 1
		lines[i] = events.Line{Event: e, Run: pastLatelyRun}
	}
	return lines
}

// pastLatelyDocument is the router's document at the frames' clock, giving
// every one of pastLatelyLines' events, the newest first.
func pastLatelyDocument() status.Document {
	doc := status.Document{Source: status.SourceRouter}
	for _, l := range slices.Backward(pastLatelyLines()) {
		doc.Events = append(doc.Events, l.Event)
	}
	return doc
}

// fullsOf are the sentences given, each in full.
func fullsOf(told []views.Sentence) []string {
	var full []string
	for _, s := range told {
		full = append(full, s.Full.String())
	}
	return full
}

// Detail's LATELY of a day gone, one and two days back from the frames'
// clock, tells the account's events of the week to the end of that day
// alone, in the words it tells them in at now: a hold lifted after the day
// is told as it stood, unlifted; a time is said beside now, as a pressure's
// run-out on Tuesday is "Tue 13:50"; the events of every account are told
// as at now too. At now it tells every one the document gives, its hold
// lifted.
func TestDetailsLATELYOfAPastPeriodTellsTheEventsUpToTheEndOfItsLastDay(t *testing.T) {
	m := pastLatelyDocument().At(framesNow, claude.Policy())
	tuesday := []string{
		"came under pressure: out ~Tue 13:50 at this pace",
		"primed: its 5-hour window started",
		"open again: its 5-hour window reset",
	}
	heldTuesday := slices.Concat([]string{"held (subscription lapsed)"}, tuesday)
	tests := []struct {
		name   string
		back   int
		id     string
		lately []string
	}{
		{name: "two days back, Tuesday", back: 2, id: "work", lately: heldTuesday},
		{name: "a day back, yesterday", back: 1, id: "work", lately: slices.Concat([]string{"c61b started here, the best"}, heldTuesday)},
		{name: "two days back, every account's", back: 2, lately: []string{
			"d28c started on side, the best",
			"work held (subscription lapsed)",
			"work came under pressure: its 5-hour runs out ~Tue 13:50",
			"work primed: its 5-hour window started",
			"work open again: its 5-hour window reset",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pressed(t, views.LengthToday, strings.Repeat("<", tt.back), framesNow, time.Monday, time.Time{})
			until, gone := p.Until(framesNow, time.Monday)
			if !gone {
				t.Fatalf("%d back from today is now, want it gone", tt.back)
			}
			held := views.ReadLatelyTo(filedLines(pastLatelyLines()), until)
			got := fullsOf(views.AccountLatelyTo(m, held, tt.id, until, 12, views.SessionDirs{}))
			if !slices.Equal(got, tt.lately) {
				t.Errorf("LATELY to %v tells %q, want %q", until, got, tt.lately)
			}
		})
	}
	atNow := []string{
		"reached its cap",
		"open again: hold lifted (subscription lapsed)",
		"c61b started here, the best",
		"held (subscription lapsed), till Thu 08:24",
	}
	if got := fullsOf(views.AccountLately(m, "work", 4, views.SessionDirs{})); !slices.Equal(got, atNow) {
		t.Errorf("LATELY at now tells %q, want %q", got, atNow)
	}
}

// A past period's LATELY is read from the router's files over the week to
// the end of its last day, and the FactsReach before it its facts are
// worked out over: no further back, and nothing after.
func TestDetailsLATELYOfAPastPeriodReadsTheWeekToItsEnd(t *testing.T) {
	until := framesDay(30, 0, 0)
	var asked []span
	views.ReadLatelyTo(countedEvents{events: filedLines(pastLatelyLines()), asked: &asked}, until)
	want := []span{{from: until.Add(-7*24*time.Hour - views.FactsReach), to: until}}
	if !slices.Equal(asked, want) {
		t.Errorf("the read asked the files for %v, want %v", asked, want)
	}
}

// A past period's last day ends at the end of the local day it ends on, a
// week length's on its last week's last day, weeks starting on any day;
// at now it doesn't end.
func TestAPastPeriodEndsAtTheEndOfItsLastDay(t *testing.T) {
	tests := []struct {
		name   string
		length views.Length
		keys   string
		until  time.Time
		gone   bool
		starts time.Weekday
	}{
		{name: "today, now", length: views.LengthToday},
		{name: "yesterday", length: views.LengthToday, keys: "<", until: framesDay(30, 0, 0).AddDate(0, 0, 1), gone: true},
		{name: "last week, to its Sunday", length: views.LengthThisWeek, keys: "<", until: framesDay(28, 0, 0), gone: true, starts: time.Monday},
		{name: "last week from Sunday, to its Saturday", length: views.LengthThisWeek, keys: "<", until: framesDay(27, 0, 0), gone: true, starts: time.Sunday},
		{name: "30 days back", length: views.LengthThirtyDays, keys: "<", until: time.Date(2026, 9, 2, 0, 0, 0, 0, time.Local), gone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pressed(t, tt.length, tt.keys, framesNow, tt.starts, time.Time{})
			until, gone := p.Until(framesNow, tt.starts)
			if gone != tt.gone || !until.Equal(tt.until) {
				t.Errorf("it ends at %v, gone %v; want %v, %v", until, gone, tt.until, tt.gone)
			}
		})
	}
}

// refiledLines are the router's files of events as it files each event
// again as it changes, every version in the order filed: on Tuesday, work
// held as its subscription lapsed, personal at its cap at 23:00 and a
// session moved off it to side at 23:30, and work at its cap at 23:50; on
// Wednesday, two sessions moved off work to side, and one off personal to
// work, each counted in its cap; on Thursday, work's hold lifted, in its
// run.
func refiledLines() filedLines {
	lapsed, thursday := "subscription lapsed", time.Date(2026, 10, 1, 8, 24, 0, 0, time.Local)
	held := status.Event{ID: 1, At: framesDay(29, 14, 31), Kind: status.EventHeld, Account: "work", Reason: lapsed, Status: 403}
	personal := status.Event{ID: 2, At: framesDay(29, 23, 0), Kind: status.EventCap, Account: "personal", Windows: []string{"5h"}}
	work := status.Event{ID: 4, At: framesDay(29, 23, 50), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}}
	move := func(id int, at time.Time, session, from, to string, forcedBy int) status.Event {
		return status.Event{ID: id, At: at, Kind: status.EventMoved, Session: session, From: from, To: to, ForcedBy: forcedBy}
	}
	counted := func(e status.Event, count int, to string) status.Event {
		e.Count, e.To = count, to
		return e
	}
	lifted := held
	lifted.Until = thursday
	told := []status.Event{
		held, personal,
		move(3, framesDay(29, 23, 30), "d28c0b1e", "personal", "side", 2), counted(personal, 1, "side"),
		work,
		move(5, framesDay(30, 0, 10), "7f3a5c90", "work", "side", 4), counted(work, 1, "side"),
		move(6, framesDay(30, 0, 20), "db8a71e3", "work", "side", 4), counted(work, 2, "side"),
		move(7, framesDay(30, 0, 30), "9e21d4a8", "personal", "work", 2), counted(personal, 2, ""),
		{ID: 8, At: thursday, Kind: status.EventRoom, Account: "work", Reason: lapsed}, lifted,
	}
	lines := make(filedLines, len(told))
	for i, e := range told {
		lines[i] = events.Line{Event: e, Run: pastLatelyRun}
	}
	return lines
}

// Detail's LATELY of a day gone tells each event as it stood at the end of
// the day, however the router filed it again since: a hold lifted after
// it, unlifted; a cap whose moves came after it, moving none, from the
// side of the account capped and of the one they went to; a cap that moved
// a session before it and another after, the one alone, where it went.
func TestDetailsLATELYOfAPastPeriodTellsEventsAsTheyStoodAtItsEnd(t *testing.T) {
	m := status.Document{Source: status.SourceRouter}.At(framesNow, claude.Policy())
	until := framesDay(30, 0, 0)
	held := views.ReadLatelyTo(refiledLines(), until)
	tests := []struct {
		id     string
		lately []string
	}{
		{id: "work", lately: []string{"reached its cap", "held (subscription lapsed)"}},
		{id: "personal", lately: []string{"reached its cap: 1 session moved to side"}},
		{id: "side", lately: []string{"1 session came from personal: personal reached its cap"}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := fullsOf(views.AccountLatelyTo(m, held, tt.id, until, 12, views.SessionDirs{})); !slices.Equal(got, tt.lately) {
				t.Errorf("%s's LATELY to the end of Tuesday tells %q, want %q", tt.id, got, tt.lately)
			}
		})
	}
}

// A pool's Detail's LATELY of a past period counts the moves recorded in
// another pool in the event that forced them, as the router counted them:
// work, moved since from pool a to pool b, reached its cap in a, two of a's
// sessions moving to side; b shows the cap, from work's side and side's,
// with the moves it counted, though it shows none of the moves.
func TestAPoolsPastLATELYCountsTheMovesOfAnotherPool(t *testing.T) {
	m := status.Document{Source: status.SourceRouter}.At(framesNow, claude.Policy())
	capAt := framesDay(29, 14, 31)
	told := []status.Event{
		{ID: 1, At: capAt, Kind: status.EventCap, Account: "work", Windows: []string{"5h"}},
		{ID: 2, At: capAt.Add(time.Minute), Kind: status.EventMoved, Session: "d28c0b1e", From: "work", To: "side", ForcedBy: 1, Pool: "a"},
		{ID: 3, At: capAt.Add(2 * time.Minute), Kind: status.EventMoved, Session: "7f3a5c90", From: "work", To: "side", ForcedBy: 1, Pool: "a"},
		{ID: 1, At: capAt, Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Count: 2, To: "side"},
	}
	lines := make(filedLines, len(told))
	for i, e := range told {
		lines[i] = events.Line{Event: e, Run: pastLatelyRun}
	}
	inB := func(string) (string, bool) { return "b", true }
	pool := views.PoolRead{ID: "b", First: "a", PoolOf: inB}
	until := framesDay(30, 0, 0)
	held := views.ReadLatelyTo(pool.Filed(lines), until)
	tests := []struct {
		id     string
		lately []string
	}{
		{id: "work", lately: []string{"reached its cap: 2 sessions moved to side"}},
		{id: "side", lately: []string{"2 sessions came from work: work reached its cap"}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := fullsOf(views.AccountLatelyTo(m, held, tt.id, until, 12, views.SessionDirs{})); !slices.Equal(got, tt.lately) {
				t.Errorf("pool b's %s's LATELY to the end of Tuesday tells %q, want %q", tt.id, got, tt.lately)
			}
		})
	}
	var kept []int
	for _, e := range held {
		kept = append(kept, e.Key.ID)
	}
	if !slices.Equal(kept, []int{1}) {
		t.Errorf("pool b's LATELY holds the events %v, want the cap alone, its moves pool a's", kept)
	}
}

// Detail's LATELY of a past period tells the week to the end of its last
// day, whatever it's given: an event a week before the end, and one just
// before it, told; one at the end, or after it, and one before the week,
// not.
func TestDetailsLATELYOfAPastPeriodTellsTheWeekToItsEndAlone(t *testing.T) {
	m := status.Document{Source: status.SourceRouter}.At(framesNow, claude.Policy())
	until := framesDay(30, 0, 0)
	week := 7 * 24 * time.Hour
	var held []views.Event
	for i, at := range []time.Time{until.Add(-week - time.Nanosecond), until.Add(-week), until.Add(-time.Nanosecond), until, until.Add(time.Hour)} {
		e := status.Event{ID: i + 1, At: at, Kind: status.EventPrimed, Account: "work", Windows: []string{"5h"}}
		held = append(held, views.Event{Key: views.EventKey{Run: pastLatelyRun, ID: e.ID}, Event: e})
	}
	var told []int
	for _, s := range views.AccountLatelyTo(m, held, "work", until, 12, views.SessionDirs{}) {
		told = append(told, s.ID)
	}
	if want := []int{3, 2}; !slices.Equal(told, want) {
		t.Errorf("LATELY to the end of Tuesday tells the events %v, want %v: from a week before the end, up to it", told, want)
	}
}
