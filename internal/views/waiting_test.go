package views_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/views"
)

// A session waits for its answer while a request of it asks, is throttled,
// to go out again, or is refused, before it goes out again elsewhere; never
// while it answers, is idle, or has ended.
func TestASessionWaitsWhileARequestOfItHasNoAnswer(t *testing.T) {
	waits := map[views.Doing]bool{
		views.Idle: false, views.Asking: true, views.Answering: false,
		views.Throttled: true, views.Refused: true, views.Ended: false,
	}
	for doing, want := range waits {
		t.Run(doingNames[doing], func(t *testing.T) {
			if got := (views.Activity{Doing: doing}).Waiting(); got != want {
				t.Errorf("Activity{%s}.Waiting() = %t, want %t", doingNames[doing], got, want)
			}
		})
	}
	if len(waits) != len(doingNames) {
		t.Errorf("the test judges %d of the %d things a session does", len(waits), len(doingNames))
	}
}
