package capture

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/views"
)

// toldEvent is an event as the Log's Events tells it: its time, its kind's
// word, the account it befell and what happened.
type toldEvent struct {
	at, kind, account, what string
}

// toldBetween is what the world's events reader gives from from up to to, as
// the Log's Events tells each, at now.
func toldBetween(now, from, to time.Time) []toldEvent {
	r := &records{events: routerEvents(now)}
	_, _, happened := r.readers(func() time.Time { return now })
	telling := views.NewTelling(claude.WindowName)
	var told []toldEvent
	for e := range happened.Between(from, to) {
		line := telling.Line(e.Event, now)
		told = append(told, toldEvent{at: e.At.In(now.Location()).Format(time.TimeOnly), kind: line.Kind, account: line.Account.String(), what: line.What.String()})
	}
	return told
}

func TestTheRoutersEventsAreTheLogsAsItTellsThem(t *testing.T) {
	now := moment(time.Local)
	today := dayfile.DayStart(now, 0)
	told := toldBetween(now, clockAt(now, -1, 16, 55, 0), now)
	// The Log's Events (log-events-160), yesterday's from 16:55 and today's
	// 20, but db8a's move under pressure, which the frame says ran out at
	// 15:10: no pressure event is told today to say when, and the frame's
	// counts hold none.
	want := []toldEvent{
		{at: "16:55:20", kind: "started", account: "personal"},
		{at: "17:10:00", kind: "open", account: "work", what: "its 5-hour window reset"},
		{at: "17:20:15", kind: "pin", account: "personal", what: "new sessions go there, set from the command line"},
		{at: "18:02:40", kind: "auto", what: "new sessions back from personal to the router's choice, set from the dashboard"},
		{at: "19:12:40", kind: "limit", account: "personal", what: "its 5-hour window, till 23:58: its session moves to side"},
		{at: "19:12:52", kind: "moved", account: "personal → side", what: "personal reached its limit"},
		{at: "23:47:30", kind: "started", account: "side"},
		{at: "23:58:40", kind: "open", account: "personal", what: "its 5-hour window reset"},
		{at: "07:05:12", kind: "primed", account: "work", what: "its 5-hour window started, resets at 12:05"},
		{at: "07:58:30", kind: "started", account: "work"},
		{at: "08:31:02", kind: "started", account: "side"},
		{at: "08:47:00", kind: "primed", account: "side", what: "its 5-hour window started, resets at 13:47"},
		{at: "09:12:40", kind: "started", account: "work"},
		{at: "09:40:05", kind: "started", account: "work"},
		{at: "10:02:18", kind: "started", account: "personal"},
		{at: "11:30:02", kind: "primed", account: "personal", what: "its 5-hour window started, resets at 16:30"},
		{at: "11:55:31", kind: "started", account: "personal"},
		{at: "12:02:10", kind: "started", account: "work"},
		{at: "12:10:00", kind: "primed", account: "work", what: "its 5-hour window started, resets at 17:10"},
		{at: "12:47:12", kind: "started", account: "personal"},
		{at: "13:22:40", kind: "moved", account: "personal → work", what: "personal came under pressure"},
		{at: "13:50:00", kind: "primed", account: "side", what: "its 5-hour window started, resets at 18:50"},
		{at: "14:12:30", kind: "started", account: "work"},
		{at: "14:31:03", kind: "cap", account: "work", what: "95% of its 5-hour window: its 3 sessions move to side as their next requests come"},
		{at: "14:31:13", kind: "moved", account: "work → side", what: "work reached its cap"},
		{at: "14:31:20", kind: "moved", account: "work → side", what: "work reached its cap"},
		{at: "14:31:40", kind: "moved", account: "work → side", what: "work reached its cap"},
		{at: "14:41:11", kind: "started", account: "side"},
	}
	if len(told) != len(want) {
		t.Fatalf("the events reader gives %d events from yesterday's 16:55, want %d: %+v", len(told), len(want), told)
	}
	for i := range want {
		if told[i] != want[i] {
			t.Errorf("event %d is told %+v, want %+v", i+1, told[i], want[i])
		}
	}

	kinds := make(map[string]int)
	for _, e := range toldBetween(now, today, now) {
		kinds[e.kind]++
	}
	wantKinds := map[string]int{"started": 10, "moved": 4, "cap": 1, "primed": 5}
	if len(kinds) != len(wantKinds) {
		t.Errorf("today's events by kind = %v, want %v", kinds, wantKinds)
	}
	for kind, n := range wantKinds {
		if kinds[kind] != n {
			t.Errorf("today's %s events = %d, want %d, as the Log's panel counts them", kind, kinds[kind], n)
		}
	}
}
