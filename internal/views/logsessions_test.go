package views_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
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
// request of the session waits for its answer, asking, throttled or refused;
// started, while its first request came within the busy minute; nothing,
// while it's busy; idle; or ended. The frame's two sessions answering read
// differently, the one started within the minute, the other not.
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
			activity: views.Activity{Doing: views.Refused, Status: 403, Since: sent, LastSeen: sent}, started: now.Add(-time.Hour), want: views.LogAsking,
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
// sessions counted as the Log's Requests count them, each line of today's,
// and each request the stream told of whose line hasn't come, toward the
// session it counts toward; and the events of each, as counted.
func panelOf(lines []ledger.Line, running []status.Session, unlisted bool, stream views.Stream, events map[string]int, at time.Time) []views.LogSession {
	today := views.Today(at)
	var todays []ledger.Line
	for _, l := range lines {
		if today.Holds(l.At) && !l.At.After(at) {
			todays = append(todays, l)
		}
	}
	read := views.ReadLogHeld(ledgerHolding(at, lines...), heldOf(todays...), ledger.Pricing, at)
	counted := views.LogRows(views.LogSources{From: at, Now: at, Read: read, Stream: stream}).Count(today.Date)
	return views.LogSessions(views.LogSessionSources{
		Read: read, Lines: counted.Sessions, Events: events, Running: running, Unlisted: unlisted, Stream: stream, Now: at,
	})
}

// At each moment the views have to tell what a session is doing at, the
// panel holds each of today's sessions as the List does: its directory, the
// account it's on and what it's doing; and says it asking exactly where it
// waits for an answer, as Activity.Waiting says, the one rule SESSIONS says
// ↑ ask by.
func TestThePanelsSessionsAreTheListsAtEachHardMoment(t *testing.T) {
	compared := 0
	waited := make(map[views.Doing]bool)
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
				if doing, _ := s.Doing(hm.Now); (doing == views.LogAsking) != l.Activity.Waiting() {
					t.Errorf("the panel says %s is %s, but it's %s, waiting = %t", s.ID[:4], logDoingNames[doing], said(l.Activity), l.Activity.Waiting())
				}
				if l.Activity.Waiting() {
					waited[l.Activity.Doing] = true
				}
			}
		})
	}
	if compared < len(viewstest.SessionMoments()) {
		t.Errorf("the panel and the List are compared at %d sessions, want at least one a moment", compared)
	}
	if want := map[views.Doing]bool{views.Asking: true, views.Throttled: true, views.Refused: true}; !maps.Equal(waited, want) {
		t.Errorf("the moments hold sessions waiting as %v, want asking, throttled and refused among them", waited)
	}
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

// A quota check, which Claude Code sends as it resumes a session under an id
// it never uses again, counts toward no session, its line come or in flight:
// the panel never lists that id, nor counts a check among a session's lines.
func TestTheQuotaChecksSessionIsNoneThePanelLists(t *testing.T) {
	const throwaway = "99999999-0000-4000-8000-000000000009"
	r0 := withID("r0", line(mover, opus, "work", "new", october(7, 12, 0), hourUsage))
	checking := func(session string) ledger.Line {
		l := withID("c0", line(session, haiku, "work", "sticky", october(7, 12, 1), ""))
		l.Kind = ledger.KindCheck
		return l
	}
	inFlight := views.Told{Request: "c1", Session: throwaway, Model: haiku, Account: "work", Check: true, Sent: october(7, 13, 11), Heard: october(7, 13, 11)}
	tests := []struct {
		name   string
		lines  []ledger.Line
		stream views.Stream
	}{
		{name: "its line come", lines: []ledger.Line{r0, checking(throwaway)}},
		{name: "in flight, as the stream tells it", lines: []ledger.Line{r0}, stream: streamOf(inFlight)},
		{name: "its line naming a session of today's", lines: []ledger.Line{r0, checking(mover)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			panel := panelOf(tt.lines, nil, true, tt.stream, nil, now)
			if len(panel) != 1 || panel[0].ID != mover || panel[0].Lines != 1 {
				var listed []string
				for _, s := range panel {
					listed = append(listed, fmt.Sprintf("%s, %d lines", s.ID[:8], s.Lines))
				}
				t.Errorf("the panel lists %q, want %s alone, its line alone", listed, mover[:8])
			}
		})
	}
}

// At each hard moment the Log's rows have to get right, the panel lists the
// sessions today's lines count toward, each with as many lines, as the day's
// sessions read off its lines do, by the one counted rule; and the day's
// summary names them alike.
func TestThePanelTheDaysSessionsAndItsSummaryNameTheSameSessionsAtEachHardMoment(t *testing.T) {
	checked := 0
	for _, hm := range viewstest.LogMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			today := views.Today(hm.Now)
			var todays []ledger.Line
			for _, l := range hm.Lines {
				if views.Today(l.At).Date != today.Date {
					continue
				}
				todays = append(todays, l)
				if l.Kind == ledger.KindCheck {
					checked++
				}
			}
			src := logSourcesOf(hm)
			src.Stream = views.Stream{}
			counted := views.LogRows(src).Count(today.Date)
			panel := make(map[string]int)
			for _, s := range views.LogSessions(views.LogSessionSources{Read: src.Read, Lines: counted.Sessions, Unlisted: true, Now: hm.Now}) {
				panel[s.ID] = s.Lines
			}
			days := make(map[string]int)
			for _, s := range views.DaySessions(heldOf(todays...)) {
				days[s.ID] = s.Lines
			}
			if !maps.Equal(panel, days) {
				t.Errorf("the panel lists %v, by their lines today, but the day's sessions are %v", panel, days)
			}
			summary, err := ledger.Summarise(today.Date, slices.Values(todays), readings.Empty{}.Between, nil, ledger.Caps{}, hm.Now)
			if err != nil {
				t.Fatal(err)
			}
			var named []string
			for _, a := range summary.Accounts {
				named = append(named, a.SessionIDs...)
			}
			slices.Sort(named)
			if listed := slices.Sorted(maps.Keys(panel)); !slices.Equal(listed, slices.Compact(named)) {
				t.Errorf("the panel lists %q, but the day's summary names %q", listed, named)
			}
		})
	}
	if checked == 0 {
		t.Error("no hard moment holds a quota check of today's")
	}
}

// A session the filter ticks joins the panel's sessions, ticked, though it
// has no lines nor events today, as one ticked before midnight, or of
// another day the find picked: as the List has it from its story where the
// Log's read holds its lines; else as the find found it, ended at its last
// request, its account not known; else its id alone. It's listed after those
// that haven't ended, and once, as one of today's or ticked twice; and one
// the find found that's not ticked isn't.
func TestASessionTickedThatTodayCountsNoneOfJoinsThePanel(t *testing.T) {
	last := local(15, 0, 0).AddDate(0, 0, -2)
	theirs := []ledger.Line{
		in("~/Code/api", line(older, opus, "side", "new", last.Add(-time.Hour), smallUsage)),
		in("~/Code/api", line(older, opus, "side", "sticky", last, smallUsage)),
	}
	todays := []ledger.Line{in("~/Code/web", line(early, opus, "work", "new", local(13, 0, 0), smallUsage))}
	l := ledgerHolding(now, slices.Concat(theirs, todays)...)
	unread := views.ReadLogHeld(l, heldOf(todays...), ledger.Pricing, now)
	read := unread.Back(l, heldOf(theirs...), now)
	found := []views.FoundSession{{ID: older, Dir: "~/Code/api", Day: views.Today(last), Ended: true, Last: last}}
	tests := []struct {
		name   string
		read   views.LogRead
		ticked []string
		found  []views.FoundSession
		want   []string
		// ended is whether older is listed ended at its last request.
		ended bool
	}{
		{name: "its lines read", read: read, ticked: []string{older}, want: []string{early + " ~/Code/web work", older + " ~/Code/api side"}, ended: true},
		{
			name: "its lines read and found", read: read, ticked: []string{older}, found: found,
			want: []string{early + " ~/Code/web work", older + " ~/Code/api side"}, ended: true,
		},
		{name: "found alone", read: unread, ticked: []string{older}, found: found, want: []string{early + " ~/Code/web work", older + " ~/Code/api "}, ended: true},
		{name: "neither read nor found", read: unread, ticked: []string{older}, want: []string{early + " ~/Code/web work", older + "  "}},
		{name: "ticked twice", read: read, ticked: []string{older, early, older}, want: []string{early + " ~/Code/web work", older + " ~/Code/api side"}, ended: true},
		{name: "one of today's", read: unread, ticked: []string{early}, found: []views.FoundSession{{ID: early, Day: views.Today(now)}}, want: []string{early + " ~/Code/web work"}},
		{name: "found, not ticked", read: read, found: found, want: []string{early + " ~/Code/web work"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			panel := views.LogSessions(views.LogSessionSources{
				Read: tt.read, Lines: map[string]int{early: 1}, Ticked: tt.ticked, Found: tt.found, Unlisted: true, Now: now,
			})
			var got []string
			for _, s := range panel {
				got = append(got, s.ID+" "+s.Dir+" "+s.Account)
				if s.ID != older {
					continue
				}
				if doing, since := s.Doing(now); tt.ended && (doing != views.LogEnded || !since.Equal(last)) || s.Lines != 0 {
					t.Errorf("the session ticked is %s since %v, %d lines today; want none today, ended at its last request, %v, where it's known",
						logDoingNames[doing], since, s.Lines, last)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("the panel lists %q, want %q", got, tt.want)
			}
		})
	}
}
