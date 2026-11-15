package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

// Each thing a session does has its words in each form: Said, row 6's and
// the Switchboard's; State, the List's, its in-flight arrows left to the
// row's mark; Short, SESSIONS'; and WaitingSaid, what a session's page
// ends its newest turn's row in while it waits.
func TestWhatASessionIsDoingIsSaidInEachForm(t *testing.T) {
	now := local(14, 42, 7)
	tests := []struct {
		name                            string
		a                               views.Activity
		said, state, short, waitingSaid string
		waiting                         bool
	}{
		{name: "asking", a: views.Activity{Doing: views.Asking, Since: local(14, 42, 0)},
			said: "↑ asking", state: "asking", short: "↑ ask", waiting: true, waitingSaid: "↑ asking"},
		{name: "answering, its tokens counted", a: views.Activity{Doing: views.Answering, Chars: 4936},
			said: "↓ answering", state: "answering", short: "↓ ~1.2k"},
		{name: "answering, nothing counted yet", a: views.Activity{Doing: views.Answering},
			said: "↓ answering", state: "answering", short: "↓"},
		{name: "throttled, to go out again on its account", a: views.Activity{Doing: views.Throttled, Status: 429},
			said: "… 429", state: "… 429", short: "↑ ask", waiting: true, waitingSaid: "↑ asking"},
		{name: "refused at a limit, before it goes out again elsewhere", a: views.Activity{Doing: views.Refused, Status: 429},
			said: "✕ 429", state: "✕ 429", short: "idle", waiting: true, waitingSaid: "↑ asking"},
		{name: "refused by a 403", a: views.Activity{Doing: views.Refused, Status: 403},
			said: "✕ 403", state: "✕ 403", short: "idle", waiting: true, waitingSaid: "↑ asking"},
		{name: "idle under a minute, in seconds", a: views.Activity{Since: now.Add(-34 * time.Second)},
			said: "idle 34s", state: "idle 34s", short: "idle"},
		{name: "idle past an hour, its minutes two digits", a: views.Activity{Since: now.Add(-65 * time.Minute)},
			said: "idle 1h 05m", state: "idle 1h 05m", short: "idle"},
		{name: "idle since a time not known", a: views.Activity{},
			said: "idle", state: "idle", short: "idle"},
		{name: "ended yesterday", a: views.Activity{Doing: views.Ended, Since: local(14, 42, 7).AddDate(0, 0, -1).Add(4*time.Hour + 5*time.Minute)},
			said: "ended yesterday 18:47", state: "ended yesterday 18:47", short: "idle"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Said(now); got != tt.said {
				t.Errorf("Said = %q, want %q", got, tt.said)
			}
			if got := tt.a.State(now); got != tt.state {
				t.Errorf("State = %q, want %q", got, tt.state)
			}
			if got := tt.a.Short(); got != tt.short {
				t.Errorf("Short = %q, want %q", got, tt.short)
			}
			if got := tt.a.Waiting(); got != tt.waiting {
				t.Errorf("Waiting = %t, want %t", got, tt.waiting)
			}
			if got := tt.a.WaitingSaid(); got != tt.waitingSaid {
				t.Errorf("WaitingSaid = %q, want %q", got, tt.waitingSaid)
			}
		})
	}
}

// A session's state under its day's rule says when it ended by the clock
// alone, whatever day, as its rule names the day; else as State says it.
func TestADaysStateSaysWhenItEndedByTheClockAlone(t *testing.T) {
	now := local(14, 42, 7)
	tests := []struct {
		name string
		a    views.Activity
		want string
	}{
		{name: "ended today", a: views.Activity{Doing: views.Ended, Since: local(11, 21, 0)}, want: "ended 11:21"},
		{name: "ended yesterday", a: views.Activity{Doing: views.Ended, Since: local(18, 47, 0).AddDate(0, 0, -1)}, want: "ended 18:47"},
		{name: "ended a fortnight back", a: views.Activity{Doing: views.Ended, Since: local(9, 5, 0).AddDate(0, 0, -14)}, want: "ended 09:05"},
		{name: "idle", a: views.Activity{Since: now.Add(-34 * time.Second)}, want: "idle 34s"},
		{name: "answering", a: views.Activity{Doing: views.Answering}, want: "answering"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.DayState(now); got != tt.want {
				t.Errorf("DayState = %q, want %q", got, tt.want)
			}
		})
	}
}
