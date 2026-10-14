package views_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
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
			rows := views.RunningOf(m, hm.Sessions, hm.Stream, listHeldOf(hm.Lines, hm.Sessions, hm.Now))
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
			counts := views.RunningCounts(rows)
			for _, a := range hm.Doc.Accounts {
				want := 0
				for _, doing := range hm.Want {
					if _, ok := doing.On[a.ID]; ok {
						want++
					}
				}
				if got := counts[a.ID]; got != want {
					t.Errorf("RunningCounts[%s] = %d, want %d, as many as run there", a.ID, got, want)
				}
			}
		})
	}
}

// At each moment the views have to tell what a session is doing at, its page
// says what the List says it's doing: one rule, the stream told and the
// router gone or probing alike.
func TestASessionsPageDoesAsTheListSaysAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.SessionMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			list := views.ListSessions(views.SessionSources{
				Running: hm.Sessions, Unlisted: hm.Unlisted, Stream: hm.Stream,
				Ledger: fakeLedger{lines: hm.Lines, now: hm.Now}, Now: hm.Now,
			})
			for _, s := range list.Sessions {
				if got := momentsPage(t, hm, s.Session).Activity; said(got) != said(s.Activity) {
					t.Errorf("%s's page says %s, the List %s", s.Session[:4], said(got), said(s.Activity))
				}
			}
			if len(list.Sessions) != len(hm.Want) {
				t.Errorf("the List holds %d sessions, want %d", len(list.Sessions), len(hm.Want))
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

// A refused session works, its request to be sent again elsewhere, as the
// Switchboard's call keeps its id bold and its cord lit while it shows ✕.
func TestASessionWorksWhileItAsksAnswersIsThrottledOrRefused(t *testing.T) {
	works := map[views.Doing]bool{views.Asking: true, views.Answering: true, views.Throttled: true, views.Refused: true}
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

// With no router to list them, a session is idle till the IdleHour after its
// last request, since its answer ended, as the router would list it still,
// and ended after that; with a router listing the sessions running, one it
// doesn't list has ended, however lately its last request came.
func TestWithNoRouterASessionIsIdleTillAnHourAfterItsLastRequest(t *testing.T) {
	tests := []struct {
		name     string
		ago      time.Duration
		unlisted bool
		want     views.Doing
	}{
		{name: "an hour after, to the second", ago: time.Hour, unlisted: true, want: views.Idle},
		{name: "an hour and a second after", ago: time.Hour + time.Second, unlisted: true, want: views.Ended},
		{name: "a minute after, a router listing the sessions running", ago: time.Minute, want: views.Ended},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			last := ledger.Line{At: now.Add(-tt.ago), Kind: ledger.KindMessage, Session: early, Model: opus, Account: "work", Status: 200, Attempts: 1, TotalMS: 3000}
			list := views.ListSessions(views.SessionSources{Unlisted: tt.unlisted, Ledger: fakeLedger{lines: []ledger.Line{last}, now: now}, Now: now})
			if len(list.Sessions) != 1 {
				t.Fatalf("the List holds %d sessions, want the one", len(list.Sessions))
			}
			want := views.Activity{Doing: tt.want, Since: last.At, LastSeen: last.At}
			if tt.want == views.Idle {
				want.Since = last.At.Add(3 * time.Second)
			}
			if got := list.Sessions[0].Activity; said(got) != said(want) {
				t.Errorf("it's %s, want %s", said(got), said(want))
			}
		})
	}
}

// With a router listing the sessions running, one it doesn't list does as
// the live stream tells, where that has seen it at work within the IdleHour,
// as a new one, its first line read before the router's listing at the next
// look; and it has ended where the live stream hasn't.
func TestASessionTheRouterDoesntListDoesAsTheLiveStreamTellsTillItHasntSeenItForAnHour(t *testing.T) {
	sent, answered := now.Add(-time.Minute), now.Add(-30*time.Second)
	asked := views.Told{Request: "r1", Session: early, Model: opus, Account: "work", Sent: sent, Heard: sent}
	done := asked
	done.Ended, done.Heard, done.Status = answered, answered, 200
	tests := []struct {
		name string
		// before is how long before a minute ago its request came.
		before time.Duration
		told   []views.Told
		live   bool
		want   views.Activity
	}{
		{name: "asking, a request of it in flight", told: []views.Told{asked}, live: true, want: views.Activity{Doing: views.Asking, Since: sent, LastSeen: sent}},
		{
			name: "idle since the stream last told of it, after its line's end", told: []views.Told{done}, live: true,
			want: views.Activity{Since: answered, LastSeen: answered},
		},
		{name: "ended, the stream not live", told: []views.Told{done}, want: views.Activity{Doing: views.Ended, Since: sent, LastSeen: sent}},
		{
			name: "ended, the live stream last telling of it over an hour ago", before: time.Hour, live: true,
			told: []views.Told{{Request: "r1", Session: early, Model: opus, Account: "work", Sent: sent.Add(-time.Hour), Ended: sent.Add(-time.Hour), Heard: sent.Add(-time.Hour)}},
			want: views.Activity{Doing: views.Ended, Since: sent.Add(-time.Hour), LastSeen: sent.Add(-time.Hour)},
		},
		{name: "ended, the live stream telling of none of it", live: true, want: views.Activity{Doing: views.Ended, Since: sent, LastSeen: sent}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			last := ledger.Line{At: sent.Add(-tt.before), Kind: ledger.KindMessage, Session: early, Model: opus, Account: "work", Status: 200, Attempts: 1, TotalMS: 3000}
			stream := views.Stream{Live: tt.live, Requests: make(map[string]views.Told)}
			for _, r := range tt.told {
				stream.Requests[r.Request] = r
			}
			another := []status.Session{{ID: asking, Assignments: []status.Assignment{{Model: opus, Account: "side", LastSeen: now}}}}
			list := views.ListSessions(views.SessionSources{Running: another, Stream: stream, Ledger: fakeLedger{lines: []ledger.Line{last}, now: now}, Now: now})
			i := slices.IndexFunc(list.Sessions, func(s views.ListedSession) bool { return s.Session == early })
			if i < 0 {
				t.Fatalf("the List holds %+v, want %s among them", list.Sessions, early[:4])
			}
			if got := list.Sessions[i].Activity; said(got) != said(tt.want) {
				t.Errorf("it's %s, want %s", said(got), said(tt.want))
			}
		})
	}
}
