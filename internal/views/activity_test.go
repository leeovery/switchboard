package views_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// doingNames name what's done, for the tests to say it.
var doingNames = map[views.Doing]string{
	views.Idle: "idle", views.Asking: "asking", views.Answering: "answering",
	views.Throttled: "throttled", views.Refused: "refused", views.Ended: "ended",
}

// said says what a is doing, its times in UTC, for the tests to compare.
func said(a views.Activity) string {
	return fmt.Sprintf("%s, %d characters, status %d, since %s, seen %s",
		doingNames[a.Doing], a.Chars, a.Status, a.Since.UTC().Format(time.TimeOnly), a.LastSeen.UTC().Format(time.TimeOnly))
}

// saidOn says what each activity is doing, by its account.
func saidOn(on map[string]views.Activity) map[string]string {
	says := make(map[string]string, len(on))
	for account, a := range on {
		says[account] = said(a)
	}
	return says
}

func TestWhatEachSessionIsDoingAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.SessionMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			list := views.ListSessions(views.SessionSources{
				Running: hm.Sessions, Unlisted: hm.Unlisted, Stream: hm.Stream,
				Ledger: fakeLedger{lines: hm.Lines, now: hm.Now}, Now: hm.Now,
			})
			whole := make(map[string]views.Activity)
			for _, s := range list.Sessions {
				whole[s.Session] = s.Activity
			}
			m := hm.Doc.At(hm.Now, claude.Policy)
			rows := views.RunningOf(m, hm.Sessions, hm.Stream)
			on := make(map[string]map[string]views.Activity)
			for _, r := range rows {
				if on[r.Session] == nil {
					on[r.Session] = make(map[string]views.Activity)
				}
				on[r.Session][r.Account] = r.Activity
			}
			for _, id := range slices.Sorted(maps.Keys(hm.Want)) {
				want := hm.Want[id]
				if got, ok := whole[id]; !ok || said(got) != said(want.Whole) {
					t.Errorf("%s, listed %t, is %s, want %s", id[:4], ok, said(got), said(want.Whole))
				}
				if got, want := saidOn(on[id]), saidOn(want.On); !maps.Equal(got, want) {
					t.Errorf("%s runs %v, want %v", id[:4], got, want)
				}
			}
			for id := range on {
				if _, ok := hm.Want[id]; !ok {
					t.Errorf("%s runs, want it not to", id[:4])
				}
			}
		})
	}
}

// A session as a whole does what the busiest of its models does: answering,
// asking, throttled, refused, then idle, as it's seen as late as any of them,
// and idle since the latest of them.
func TestASessionDoesWhatItsBusiestModelDoes(t *testing.T) {
	asked := now.Add(-5 * time.Second)
	doing := map[views.Doing]views.Told{
		views.Asking:    {Sent: asked},
		views.Answering: {Sent: asked, First: asked.Add(time.Second), Chars: 400},
		views.Throttled: {Sent: asked, Verdict: views.VerdictThrottled, Status: 429},
		views.Refused:   {Sent: asked, Verdict: views.VerdictRefused, Status: 403},
		views.Idle:      {Sent: asked, Ended: asked.Add(2 * time.Second)},
	}
	busiestFirst := []views.Doing{views.Answering, views.Asking, views.Throttled, views.Refused, views.Idle}
	for i, busier := range busiestFirst {
		for _, other := range busiestFirst[i+1:] {
			t.Run(doingNames[busier]+" over "+doingNames[other], func(t *testing.T) {
				opus, haiku := doing[busier], doing[other]
				opus.Request, opus.Session, opus.Model, opus.Account, opus.Heard = "r1", asking, "claude-opus-5-5", "work", asked
				haiku.Request, haiku.Session, haiku.Model, haiku.Account, haiku.Heard = "r2", asking, "claude-haiku-4-5", "side", now
				session := status.Session{ID: asking, Assignments: []status.Assignment{
					seatOn("claude-haiku-4-5", "haiku", "side", status.ReasonNew, time.Hour, time.Minute, ""),
					seatOn("claude-opus-5-5", "opus", "work", status.ReasonNew, time.Hour, time.Minute, ""),
				}}
				list := views.ListSessions(views.SessionSources{
					Running: []status.Session{session}, Ledger: fakeLedger{now: now}, Now: now,
					Stream: views.Stream{Live: true, Requests: map[string]views.Told{"r1": opus, "r2": haiku}},
				})
				got := list.Sessions[0].Activity
				if got.Doing != busier || !got.LastSeen.Equal(now) {
					t.Errorf("the session is %s, want %s, seen as late as either", said(got), doingNames[busier])
				}
			})
		}
	}
}

func TestASessionIsBusyWithARequestInFlightOrSeenInTheLastMinute(t *testing.T) {
	tests := []struct {
		doing views.Doing
		seen  time.Duration
		want  bool
	}{
		{doing: views.Asking, seen: time.Hour, want: true},
		{doing: views.Answering, seen: time.Hour, want: true},
		{doing: views.Throttled, seen: time.Hour, want: true},
		{doing: views.Refused, seen: time.Hour, want: true},
		{doing: views.Idle, seen: 59 * time.Second, want: true},
		{doing: views.Idle, seen: time.Minute},
		{doing: views.Ended, seen: time.Hour},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s, seen %s ago", doingNames[tt.doing], tt.seen), func(t *testing.T) {
			if got := (views.Activity{Doing: tt.doing, LastSeen: now.Add(-tt.seen)}).Busy(now); got != tt.want {
				t.Errorf("Busy = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestASessionWorksWhileItAsksAnswersOrIsThrottled(t *testing.T) {
	works := map[views.Doing]bool{views.Asking: true, views.Answering: true, views.Throttled: true}
	for doing, name := range doingNames {
		t.Run(name, func(t *testing.T) {
			if got := (views.Activity{Doing: doing, LastSeen: now}).Works(); got != works[doing] {
				t.Errorf("Works = %t, want %t, seen now or not", got, works[doing])
			}
		})
	}
}

func TestAnAnswersTokensSoFarAreItsCharactersAtFourAToken(t *testing.T) {
	if got := (views.Activity{Doing: views.Answering, Chars: 4936}).Tokens(); got != 1234 {
		t.Errorf("Tokens = %d, want 1,234", got)
	}
}
