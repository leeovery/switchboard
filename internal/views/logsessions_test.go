package views_test

import (
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

// logDoingNames name what the panel's doing column says, for the tests to
// say it.
var logDoingNames = map[views.LogDoing]string{
	views.LogBusy: "nothing", views.LogAsking: "asking", views.LogStarted: "started", views.LogIdle: "idle", views.LogEnded: "ended",
}

// The panel's doing column says the first that holds: asking, while a
// request of the session asks, or is throttled, to ask again; started, while
// its first request came within the busy minute; nothing, while it's busy;
// idle; or ended. The frame's two sessions answering read differently, the
// one started within the minute, the other not.
func TestThePanelSaysWhatASessionIsDoing(t *testing.T) {
	sent := now.Add(-7 * time.Second)
	tests := []struct {
		name      string
		activity  views.Activity
		started   time.Time
		want      views.LogDoing
		wantSince time.Time
	}{
		{name: "asking", activity: views.Activity{Doing: views.Asking, Since: sent, LastSeen: sent}, started: now.Add(-time.Hour), want: views.LogAsking},
		{
			name:     "asking its first request, started within the minute",
			activity: views.Activity{Doing: views.Asking, Since: sent, LastSeen: sent}, started: sent, want: views.LogAsking,
		},
		{
			name:     "throttled, to ask again",
			activity: views.Activity{Doing: views.Throttled, Status: 429, Since: sent, LastSeen: sent}, started: now.Add(-time.Hour), want: views.LogAsking,
		},
		{
			name:     "answering, its first request come within the minute, as the frame's c61b is",
			activity: views.Activity{Doing: views.Answering, Chars: 5100, Since: sent, LastSeen: now}, started: now.Add(-56 * time.Second),
			want: views.LogStarted, wantSince: now.Add(-56 * time.Second),
		},
		{
			name:     "answering, started before the minute, as the frame's 7f3a is",
			activity: views.Activity{Doing: views.Answering, Chars: 1900, Since: sent, LastSeen: now}, started: now.Add(-2 * time.Hour), want: views.LogBusy,
		},
		{
			name:     "refused, before it goes out again elsewhere",
			activity: views.Activity{Doing: views.Refused, Status: 403, Since: sent, LastSeen: sent}, started: now.Add(-time.Hour), want: views.LogBusy,
		},
		{
			name:     "idle, at work within the minute",
			activity: views.Activity{Since: now.Add(-30 * time.Second), LastSeen: now.Add(-30 * time.Second)}, started: now.Add(-time.Hour), want: views.LogBusy,
		},
		{
			name:     "idle, at work within the minute, its first request come a minute ago",
			activity: views.Activity{Since: now.Add(-50 * time.Second), LastSeen: now.Add(-50 * time.Second)}, started: now.Add(-time.Minute), want: views.LogBusy,
		},
		{
			name:     "idle 9m",
			activity: views.Activity{Since: now.Add(-9 * time.Minute), LastSeen: now.Add(-9 * time.Minute)}, started: now.Add(-time.Hour),
			want: views.LogIdle, wantSince: now.Add(-9 * time.Minute),
		},
		{
			name:     "ended at 11:21",
			activity: views.Activity{Doing: views.Ended, Since: local(11, 21, 0), LastSeen: local(11, 21, 0)}, started: local(9, 0, 0),
			want: views.LogEnded, wantSince: local(11, 21, 0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := views.LogSession{Activity: tt.activity, Started: tt.started}
			if got, since := s.Doing(now); got != tt.want || !since.Equal(tt.wantSince) {
				t.Errorf("Doing(now) of %s, started %v = %s since %v, want %s since %v",
					said(tt.activity), tt.started, logDoingNames[got], since, logDoingNames[tt.want], tt.wantSince)
			}
		})
	}
}

// panelOf is the panel's sessions of the ledger's lines that arrived by now,
// the router listing those given, the stream telling what it did: today's
// sessions counted as the Log counts them, each line of today's of a session,
// and each request the stream told of whose line hasn't come; and the events
// of each, as counted.
func panelOf(lines []ledger.Line, running []status.Session, unlisted bool, stream views.Stream, events map[string]int, at time.Time) []views.LogSession {
	today := views.Today(at)
	var todays []ledger.Line
	counted := make(map[string]int)
	lined := make(map[string]bool)
	for _, l := range lines {
		if today.Holds(l.At) && !l.At.After(at) {
			todays = append(todays, l)
			counted[l.Session]++
			lined[l.Request] = true
		}
	}
	for id, t := range stream.Requests {
		if !lined[id] && today.Holds(t.Sent) {
			counted[t.Session]++
		}
	}
	return views.LogSessions(views.LogSessionSources{
		Read: views.ReadLog(ledgerHolding(at, lines...), todays), Lines: counted, Events: events,
		Running: running, Unlisted: unlisted, Stream: stream, Prices: ledger.Pricing, Now: at,
	})
}

// At each moment the views have to tell what a session is doing at, the
// panel holds each of today's sessions as the List does: its directory, the
// account it's on and what it's doing, so its asking is the List's, and
// SESSIONS', where SESSIONS lists it: asking where the busiest of its rows
// asks.
func TestThePanelsSessionsAreTheListsAtEachHardMoment(t *testing.T) {
	compared := 0
	for _, hm := range viewstest.SessionMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			panel := panelOf(hm.Lines, hm.Sessions, hm.Unlisted, hm.Stream, nil, hm.Now)
			list := views.ListSessions(views.SessionSources{
				Running: hm.Sessions, Unlisted: hm.Unlisted, Stream: hm.Stream, Ledger: ledgerHolding(hm.Now, hm.Lines...), Prices: ledger.Pricing, Now: hm.Now,
			})
			listed := make(map[string]views.ListedSession)
			for _, s := range list.Sessions {
				listed[s.Session] = s
			}
			rows := views.RunningOf(hm.Doc.At(hm.Now, claude.Policy), hm.Sessions, hm.Stream)
			for _, s := range panel {
				l, ok := listed[s.ID]
				if !ok {
					t.Errorf("the panel holds %s, the List doesn't", s.ID[:4])
					continue
				}
				compared++
				if said(s.Activity) != said(l.Activity) || s.Account != l.Account || s.Dir != l.Dir || !s.Started.Equal(l.Started) {
					t.Errorf("the panel holds %s %s on %q in %q, started %v; the List %s on %q in %q, started %v",
						s.ID[:4], said(s.Activity), s.Account, s.Dir, s.Started, said(l.Activity), l.Account, l.Dir, l.Started)
				}
				doing, _ := s.Doing(hm.Now)
				if asks, ok := sessionsAsks(rows, s.ID); ok && (doing == views.LogAsking) != asks {
					t.Errorf("the panel says %s is %s, but SESSIONS has it asking = %t", s.ID[:4], logDoingNames[doing], asks)
				}
			}
		})
	}
	if compared < len(viewstest.SessionMoments()) {
		t.Errorf("the panel and the List are compared at %d sessions, want at least one a moment", compared)
	}
}

// sessionsAsks reports whether SESSIONS has the session with the given id
// asking, as its busiest row says: one asking, or throttled to ask again,
// and none answering, which is busier. It reports false where SESSIONS lists
// it on no account.
func sessionsAsks(rows []views.Running, id string) (asks, listed bool) {
	answers := false
	for _, r := range rows {
		if r.Session != id {
			continue
		}
		listed = true
		asks = asks || r.Activity.Doing == views.Asking || r.Activity.Doing == views.Throttled
		answers = answers || r.Activity.Doing == views.Answering
	}
	return asks && !answers, listed
}

// The panel lists today's sessions the Log counts lines or events of, with
// those counts as given, and no other: those that haven't ended, seen last
// first, then those ended, the latest ended first.
func TestThePanelListsTodaysSessionsInItsOrder(t *testing.T) {
	lines := []ledger.Line{
		in("~/Code/web", line(early, opus, "work", "new", local(9, 0, 0), smallUsage)),
		in("~/Code/web", line(early, opus, "work", "sticky", local(11, 0, 0), smallUsage)),
		in("~/Code/api", line(rescored, opus, "side", "new", local(10, 0, 0), smallUsage)),
		in("~/Code/api", line(rescored, opus, "side", "sticky", local(12, 0, 0), smallUsage)),
		in("~/Code/switchboard", line(older, opus, "work", "new", local(12, 30, 0), smallUsage)),
		in("~/Code/mobile", line(mover, opus, "side", "new", local(13, 0, 0), smallUsage)),
	}
	running := []status.Session{
		{ID: older, Assignments: []status.Assignment{seatOn(opus, "opus", "work", status.ReasonSticky, time.Hour, 5*time.Minute, "")}},
		{ID: mover, Assignments: []status.Assignment{seatOn(opus, "opus", "side", status.ReasonNew, time.Hour, time.Minute, "")}},
		// quiet's own pin was set today, and it has sent nothing since.
		{ID: quiet, Assignments: []status.Assignment{seatOn(opus, "opus", "side", status.ReasonPinned, time.Hour, 2*time.Minute, "")}},
		{ID: asking, Assignments: []status.Assignment{seatOn(opus, "opus", "side", status.ReasonNew, time.Hour, 3*time.Minute, "")}},
	}
	events := map[string]int{quiet: 2, older: 1, rescored: 0}

	panel := panelOf(lines, running, false, views.Stream{}, events, now)
	var got []string
	counts := make(map[string][2]int)
	for _, s := range panel {
		got = append(got, s.ID)
		counts[s.ID] = [2]int{s.Lines, s.Events}
	}
	if want := []string{mover, quiet, older, rescored, early}; !slices.Equal(got, want) {
		t.Errorf("the panel lists %q, want %q: those running, seen last first, one with events alone among them, then those ended, the latest ended first",
			got, want)
	}
	wantCounts := map[string][2]int{mover: {1, 0}, quiet: {0, 2}, older: {1, 1}, rescored: {2, 0}, early: {2, 0}}
	if !maps.Equal(counts, wantCounts) {
		t.Errorf("the panel counts %v lines and events, want %v, as the Log counts them", counts, wantCounts)
	}
}
