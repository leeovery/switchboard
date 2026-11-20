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
// week length's on its last week's Sunday; at now it doesn't end.
func TestAPastPeriodEndsAtTheEndOfItsLastDay(t *testing.T) {
	tests := []struct {
		name   string
		length views.Length
		keys   string
		until  time.Time
		gone   bool
	}{
		{name: "today, now", length: views.LengthToday},
		{name: "yesterday", length: views.LengthToday, keys: "<", until: framesDay(30, 0, 0).AddDate(0, 0, 1), gone: true},
		{name: "last week, to its Sunday", length: views.LengthThisWeek, keys: "<", until: framesDay(28, 0, 0), gone: true},
		{name: "30 days back", length: views.LengthThirtyDays, keys: "<", until: time.Date(2026, 9, 2, 0, 0, 0, 0, time.Local), gone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pressed(t, tt.length, tt.keys, framesNow, time.Monday, time.Time{})
			until, gone := p.Until(framesNow, time.Monday)
			if gone != tt.gone || !until.Equal(tt.until) {
				t.Errorf("it ends at %v, gone %v; want %v, %v", until, gone, tt.until, tt.gone)
			}
		})
	}
}
