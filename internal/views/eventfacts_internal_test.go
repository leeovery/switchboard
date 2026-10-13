package views

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// Each teller says the facts it's given of an event, never its own: given
// facts an event's fields don't hold, the Log's Events and LATELY each say
// those.
func TestBothTellersSayTheFactsTheyreGivenRatherThanWorkThemOut(t *testing.T) {
	now := time.Date(2026, 10, 7, 14, 0, 0, 0, time.UTC)
	given := eventFact{runsOut: now.Add(70 * time.Minute), moved: 7, to: "elsewhere", windows: []string{"odd"}}
	passing := status.PassingOver(status.Rescored("15h"), "personal")
	tests := []struct {
		event          status.Event
		logged, lately string
	}{
		{event: status.Event{Kind: status.EventMoved, Session: "db8a71e3", From: "personal", To: "side", Reason: passing},
			logged: "personal came under pressure, its 5-hour to run out at 15:10",
			lately: "db8a moved personal → side: personal came under pressure, its 5-hour to run out at 15:10"},
		{event: status.Event{Kind: status.EventStarted, Account: "side", Session: "c61b2f8d", Reason: status.PassingOver(status.ReasonNew, "personal")},
			lately: "c61b started on side: personal came under pressure, its 5-hour to run out at 15:10"},
		{event: status.Event{Kind: status.EventPressure, Account: "personal", Until: now.Add(2 * time.Hour)},
			logged: "its odd to run out at 16:00", lately: "personal came under pressure: its odd runs out ~16:00"},
		{event: status.Event{Kind: status.EventCap, Account: "work"},
			logged: "its odd window: its 7 sessions move to elsewhere as their next requests come", lately: "work reached its cap: 7 sessions moved to elsewhere"},
		{event: status.Event{Kind: status.EventLimit, Account: "work"},
			logged: "its odd window: its 7 sessions move to elsewhere", lately: "work reached its odd limit: 7 sessions moved to elsewhere"},
		{event: status.Event{Kind: status.EventRefused, Account: "work"},
			logged: "its token refused: its 7 sessions move to elsewhere", lately: "work refused: 7 sessions moved to elsewhere"},
		{event: status.Event{Kind: status.EventPrimed, Account: "side"},
			logged: "its odd window started", lately: "side primed: its odd window started"},
		{event: status.Event{Kind: status.EventRoom, Account: "work"},
			logged: "its odd window reset", lately: "work open again: its odd window reset"},
	}
	for _, tt := range tests {
		t.Run(tt.event.Kind, func(t *testing.T) {
			tt.event.At = now
			if tt.logged != "" {
				if _, what := tellers[tt.event.Kind](tt.event, given, now); what.String() != tt.logged {
					t.Errorf("the Log says %q, want %q", what, tt.logged)
				}
			}
			if full, _ := latelyTellers[tt.event.Kind](latelyTeller{now: now}, tt.event, given); full.String() != tt.lately {
				t.Errorf("LATELY says %q, want %q", full, tt.lately)
			}
		})
	}
}
