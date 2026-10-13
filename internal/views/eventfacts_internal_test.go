package views

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// At each hard moment, each event's facts are worked out over the run of the
// events read, oldest first, as the Log tells them: when the account a
// reason passed over runs out, from the latest pressure event before it,
// another run's among them; the sessions a limit moved, as its last version
// counts them; and its windows, each named as status.NameOfKey names it.
func TestEachEventsFactsAreWorkedOutOverTheRunOfEventsAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.EventMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			facts := eventFacts{name: status.NameOfKey}
			for _, line := range hm.Events {
				key := viewstest.KeyOf(line)
				if got, want := factsOf(facts.take(line.Event)), hm.Facts[key]; !sameFacts(got, want) {
					t.Errorf("the %s event %d has facts %+v, want %+v", line.Kind, line.ID, got, want)
				}
			}
		})
	}
}

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

// factsOf are an event's facts as viewstest has them.
func factsOf(f eventFact) viewstest.Facts {
	return viewstest.Facts{RunsOut: f.runsOut, Moved: f.moved, To: f.to, Windows: f.windows}
}

// sameFacts reports whether a and b are the same facts, their run-outs the
// same instant.
func sameFacts(a, b viewstest.Facts) bool {
	return a.RunsOut.Equal(b.RunsOut) && a.Moved == b.Moved && a.To == b.To && slices.Equal(a.Windows, b.Windows)
}
