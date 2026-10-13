package views_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
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
			events := make([]status.Event, len(hm.Events))
			for i, line := range hm.Events {
				events[i] = line.Event
			}
			for i, got := range views.TakeFacts(status.NameOfKey, events) {
				line := hm.Events[i]
				if want := hm.Facts[viewstest.KeyOf(line)]; !sameFacts(got, want) {
					t.Errorf("the %s event %d has facts %+v, want %+v", line.Kind, line.ID, got, want)
				}
			}
		})
	}
}

// sameFacts reports whether got are the facts want, their run-outs the same
// instant.
func sameFacts(got views.TakenFacts, want viewstest.Facts) bool {
	return got.RunsOut.Equal(want.RunsOut) && got.Moved == want.Moved && got.To == want.To && slices.Equal(got.Windows, want.Windows)
}
