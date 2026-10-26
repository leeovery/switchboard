package watch

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// threeKeys are the keys the Overview's key line lists of three accounts,
// ↑↓ and p among them, and oneKeys those it lists of one, which has none
// other to focus or pin to: each after tab views and the toggles.
const (
	toggles   = "tab views · u usage · r routing · s sessions · c coming up · l lately · b by: window · "
	threeKeys = toggles + "↑↓ focus · p pin · ? help · q quit"
	oneKeys   = toggles + "? help · q quit"
	// unreadKeys are those listed before anything is read, ↑↓ and p doing
	// nothing till there are accounts to focus and pin to.
	unreadKeys = oneKeys
)

func TestLooksAtTheRoutersDocumentEveryFiveSeconds(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	if got, want := h.source.asked, []status.Read{{Refresh: interval, Probe: true}}; !slices.Equal(got, want) {
		t.Fatalf("starting asked for %+v, want %+v: the router refreshing what it hasn't read in an interval", got, want)
	}
	for i := range 3 {
		want := start.Add(time.Duration(i+1)*lookEvery + tickSlack)
		if got := h.tickUntilAsked(); !got.Equal(want) {
			t.Errorf("look %d at %s, want %s", i+1, got.Format(time.StampMilli), want.Format(time.StampMilli))
		}
	}
	want := []status.Read{{Refresh: interval, Probe: true}, {}, {}, {}}
	if !slices.Equal(h.source.asked, want) {
		t.Errorf("asked for %+v, want %+v: a look at the router's document every five seconds, which never probes", h.source.asked, want)
	}
	if h.source.reads != 0 {
		t.Errorf("probed %d times, want never while the router answers", h.source.reads)
	}
	if got, want := h.keyLine(), threeKeys; got != want {
		t.Errorf("the key line reads %q, want %q", got, want)
	}
}

func TestEveryReadOfTheRouterListsItsSessions(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{
		{ID: "d28c5e17", Assignments: []status.Assignment{{Model: "claude-opus-5-5", Account: "work", LastSeen: start.Add(-50 * time.Second).UTC()}}},
		{ID: "7f3a0c94", Assignments: []status.Assignment{{Model: "claude-haiku-4-5", Account: "work", LastSeen: start.Add(-9 * time.Minute).UTC()}}},
	}
	h.start()

	if h.source.listed != 1 || len(h.model.sessions) != 2 {
		t.Fatalf("listed the sessions %d times, holding %d, want once with the first read, holding both", h.source.listed, len(h.model.sessions))
	}
	h.tickUntilAsked()
	if h.source.listed != 2 {
		t.Errorf("listed the sessions %d times, want again with a look at the router's document", h.source.listed)
	}
	h.model.sections = dashboard.SectionsNamed([]string{"sessions"})
	h.redraw()
	if !strings.Contains(h.view(), "d28c") || !strings.Contains(h.view(), "7f3a") {
		t.Errorf("the screen is\n%s\nwant SESSIONS listing d28c and 7f3a", h.view())
	}
}

func TestProbingListsNoSessions(t *testing.T) {
	h := newHarness(t, calm())
	h.start()

	if h.source.listed != 0 || h.model.sessions != nil {
		t.Errorf("listed the sessions %d times, holding %v, want never while probing", h.source.listed, h.model.sessions)
	}
}

func TestSessionsTheRouterDoesntListAreHeldAsNone(t *testing.T) {
	work := account("work", "Work", session(0.25, 3*time.Hour), week(0.5))
	work.Sessions = 2
	h := routedHarness(t, routerDocument(work, three()[1]))
	h.source.sessions = []status.Session{{ID: "d28c5e17", Assignments: []status.Assignment{{Account: "work", LastSeen: start.UTC()}}}}
	h.source.sessionsErr = errors.New("connection reset")
	h.start()

	if h.model.sessions != nil {
		t.Errorf("holding the sessions %v, want none listed", h.model.sessions)
	}
}

func TestHasTheRouterRefreshOnceAnInterval(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	var refreshed []time.Time
	for h.clock.now.Before(at(14, 12, 30)) {
		asked := len(h.source.asked)
		h.fire(h.lastTick())
		for _, r := range h.source.asked[asked:] {
			if r.Refresh > 0 {
				refreshed = append(refreshed, h.clock.now)
			}
		}
	}
	if len(refreshed) != 2 {
		t.Fatalf("the router refreshed at %v, want twice more in the hour", refreshed)
	}
	for i, due := range []time.Time{at(13, 42, 0), refreshed[0].Add(interval)} {
		if got := refreshed[i]; got.Before(due) || got.After(due.Add(lookEvery+tickSlack)) {
			t.Errorf("refresh %d at %s, want the first tick at or after %s", i+1, got.Format(time.StampMilli), due.Format(time.StampMilli))
		}
	}
}

func TestHasTheRouterRefreshSoonerWhileAnAccountCantBeRead(t *testing.T) {
	h := routedHarness(t, routerDocument(account("work", "Work", session(0.25, 3*time.Hour), week(0.5)), unreadable("side", "Side")))
	h.start()

	var refreshed []time.Time
	for h.clock.now.Before(at(13, 19, 0)) {
		asked := len(h.source.asked)
		h.fire(h.lastTick())
		if slices.ContainsFunc(h.source.asked[asked:], func(r status.Read) bool { return r.Refresh > 0 }) {
			refreshed = append(refreshed, h.clock.now)
		}
	}
	for i, want := range []time.Time{at(13, 14, 0), at(13, 18, 0)} {
		if i >= len(refreshed) || refreshed[i].Before(want) || refreshed[i].After(want.Add(lookEvery+tickSlack)) {
			t.Errorf("the router refreshed at %v, want refresh %d at the look after %s, backing off", refreshed, i+1, want.Format(time.Kitchen))
		}
	}
}

func TestHasTheRouterRefreshOnceAWindowOnScreenResets(t *testing.T) {
	// The router's document shows work's week resetting at 13:20, and goes on
	// showing it after, as it would were work idle.
	h := routedHarness(t, routerDocument(
		account("work", "Work", session(0.25, 3*time.Hour), window("7d", "Week", 0.5, 8*time.Minute)),
		account("side", "Side", session(0.4, 2*time.Hour), week(0.6)),
	))
	h.start()

	refreshed := h.refreshesUntil(at(13, 25, 0))
	due := at(13, 21, 0)
	if len(refreshed) != 1 || refreshed[0].Before(due) || refreshed[0].After(due.Add(lookEvery+tickSlack)) {
		t.Errorf("the router refreshed what it hadn't read in the last minute at %v, want once, at the first look from %s, a minute after work's week reset",
			refreshed, due.Format(time.Kitchen))
	}
}

func TestShowsASessionThatHasLapsedEmptyRatherThanRefreshingIt(t *testing.T) {
	// Work's session resets at 13:20, and nothing is read of work since, so
	// from then on the router's document shows it lapsed.
	side := account("side", "Side", session(0.4, 2*time.Hour), week(0.6))
	h := routedHarness(t, routerDocument(account("work", "Work", session(0.25, 8*time.Minute), week(0.5)), side))
	h.start()
	h.tickUntil(at(13, 20, 0))
	work := account("work", "Work", quota.Window{Key: "5h", Label: "Session"}, week(0.5))
	work.Lapsed = []string{"5h"}
	h.startRouter(routerDocument(work, side))

	if refreshed := h.refreshesUntil(at(13, 25, 0)); len(refreshed) > 0 {
		t.Errorf("the router refreshed what it hadn't read in the last minute at %v, want never: it doesn't probe an account whose session has lapsed", refreshed)
	}
	if !strings.Contains(h.view(), "not started") {
		t.Errorf("the screen is\n%s\nwant work's session empty, not started", h.view())
	}
}

func TestAWindowResettingOnAnAccountWhoseSessionHasLapsedHasTheRouterRefreshNothing(t *testing.T) {
	// Work's session has lapsed, and its week resets at 13:20; side's session
	// runs, and its week resets at 13:22.
	work := account("work", "Work", quota.Window{Key: "5h", Label: "Session"}, window("7d", "Week", 0.5, 8*time.Minute))
	work.Lapsed = []string{"5h"}
	h := routedHarness(t, routerDocument(work, account("side", "Side", session(0.4, 2*time.Hour), window("7d", "Week", 0.6, 10*time.Minute))))
	h.start()

	refreshed := h.refreshesUntil(at(13, 25, 0))
	due := at(13, 23, 0)
	if len(refreshed) != 1 || refreshed[0].Before(due) || refreshed[0].After(due.Add(lookEvery+tickSlack)) {
		t.Errorf("the router refreshed what it hadn't read in the last minute at %v, want once, at the first look from %s, a minute after side's week reset: the router won't probe work",
			refreshed, due.Format(time.Kitchen))
	}
}

func TestHasTheRouterRefreshAtOnceForAWindowThatResetBeforeTheWatchBegan(t *testing.T) {
	h := routedHarness(t, routerDocument(account("work", "Work", session(0.25, 3*time.Hour), window("7d", "Week", 0.5, -12*time.Minute))))
	h.start()

	h.tickUntilAsked()
	h.tickUntilAsked()
	want := []status.Read{{Refresh: interval, Probe: true}, {Refresh: status.FreshFor}, {}}
	if !slices.Equal(h.source.asked, want) {
		t.Errorf("asked for %+v, want %+v: the first look has the router refresh what it hasn't read in the last minute, once", h.source.asked, want)
	}
}

func TestKeepsTheRoutersDocumentWhileItDoesntAnswerALook(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	h.stopRouter()
	h.tickUntilAsked() // 13:12:05.
	looks := h.source.looks()
	h.tickUntil(at(13, 13, 5).Add(tickSlack))
	if h.source.reads != 0 {
		t.Fatalf("while the router doesn't answer a look, probed %d times, want never: the router may be away for a moment", h.source.reads)
	}
	if got := h.source.looks() - looks; got < 10 || got > 12 {
		t.Errorf("looked at the router's document %d times in the minute after it stopped answering, want every 5 seconds or so", got)
	}
	if !h.model.routed() {
		t.Error("the document on screen isn't the router's, want its last one kept")
	}
	if h.model.answering() {
		t.Error("the router doesn't answer a look, yet the watch has it answering")
	}
	if got := strings.Split(h.view(), "\n")[2]; !strings.HasPrefix(got, "■ the router isn't answering  ·  since 13:12:05  ·  auto  ·  ") {
		t.Errorf("the status line reads %q, want it to say since when the router hasn't answered, the rest as its last document had them", got)
	}

	h.startRouter(routerDocument(three()...))
	h.tickUntilAsked()
	if !h.model.answering() {
		t.Error("once the router answers a look again, the watch has it not answering, want it answering")
	}
	if got := strings.Split(h.view(), "\n")[2]; !strings.HasPrefix(got, "● healthy  ·  auto  ·  ") {
		t.Errorf("the status line reads %q, want the router healthy again", got)
	}
}

func TestAReadThatFailsWhileTheRoutersDocumentShowsSaysTheRouterIsntAnswering(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.stopRouter()
	h.source.err = errors.New("connection refused")
	h.deliver(h.press("R")...)

	if !h.model.routed() || h.model.answering() {
		t.Errorf("the document on screen is the router's: %v, the router answering: %v; want its last one kept, the router not answering", h.model.routed(), h.model.answering())
	}
	if got := strings.Split(h.view(), "\n")[2]; !strings.HasPrefix(got, "■ the router isn't answering  ·  since 13:12:00  ·  auto  ·  ") {
		t.Errorf("the status line reads %q, want it to say since when the router hasn't answered, the rest as its last document had them", got)
	}

	h.source.err = nil
	h.startRouter(routerDocument(three()...))
	h.tickUntilAsked()
	if got := strings.Split(h.view(), "\n")[2]; !h.model.answering() || !strings.HasPrefix(got, "● healthy  ·  auto  ·  ") {
		t.Errorf("once the router answers again, the status line reads %q, want the router healthy", got)
	}
}

func TestAFirstReadThatFindsNoRouterSaysTheRouterIsntAnsweringTillAReadLands(t *testing.T) {
	tests := []struct {
		name string
		// lands has a read land: probed, or the router's.
		lands func(h *harness)
		// wantStatus is how the status line then starts.
		wantStatus string
	}{
		{name: "probed", lands: func(h *harness) { h.tickUntilRead() }, wantStatus: "○ probed · router not running"},
		{
			name:       "the router's",
			lands:      func(h *harness) { h.startRouter(routerDocument(three()...)); h.tickUntilAsked() },
			wantStatus: "● healthy  ·  auto  ·  ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, probedWithoutTheRouter())
			h.source.err = status.ErrNoRouter
			h.start()
			if got, want := strings.TrimSpace(strings.Split(h.view(), "\n")[2]), "■ the router isn't answering  ·  since 13:12:00"; got != want {
				t.Errorf("the status line reads %q, want %q: nothing read, that alone", got, want)
			}

			h.source.err = nil
			tt.lands(h)
			if got := strings.Split(h.view(), "\n")[2]; !strings.HasPrefix(got, tt.wantStatus) || !h.model.lost.IsZero() {
				t.Errorf("once a read lands, the status line reads %q, the router lost at %v; want it to start %q, and the router lost no more", got, h.model.lost, tt.wantStatus)
			}
		})
	}
}

func TestAFirstReadThatFailsOtherwiseSaysWhyTillAReadFindsTheRouterGone(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	h.source.err = errors.New("the keychain is locked")
	h.start()

	want := "! couldn't read usage: the keychain is locked  ·  next 13:14"
	if got := strings.TrimSpace(strings.Split(h.view(), "\n")[2]); got != want || !h.model.lost.IsZero() {
		t.Errorf("the status line reads %q, the router lost at %v; want %q, why and when it reads next, the router not lost", got, h.model.lost, want)
	}
	asked := h.tickUntilAsked() // A question after the router, which finds it gone.
	want = "■ the router isn't answering  ·  since " + asked.Format("15:04:05")
	if got := strings.TrimSpace(strings.Split(h.view(), "\n")[2]); got != want {
		t.Errorf("once a question finds the router gone, the status line reads %q; want %q, what the latest read found", got, want)
	}

	h.source.err = nil
	h.startRouter(routerDocument(three()...))
	h.tickUntilAsked()
	if got := strings.Split(h.view(), "\n")[2]; !h.model.answering() || !strings.HasPrefix(got, "● healthy  ·  auto  ·  ") {
		t.Errorf("once the router answers, the status line reads %q, want the router healthy", got)
	}
}

func TestAFirstReadSaysTheLatestReadsNewsTheRouterGoneSinceFirstFoundOrAProbesOwnReason(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	h.source.err = errors.New("the keychain is locked")
	h.start()
	asked := h.tickUntilAsked() // A question after the router, which finds it gone.
	if got, want := strings.TrimSpace(strings.Split(h.view(), "\n")[2]), "■ the router isn't answering  ·  since "+asked.Format("15:04:05"); got != want {
		t.Fatalf("once a question finds the router gone, the status line reads %q; want %q", got, want)
	}

	h.tickUntilRead() // A full read, whose probe fails for its own reason.
	want := "! couldn't read usage: the keychain is locked  ·  next "
	if got := strings.TrimSpace(strings.Split(h.view(), "\n")[2]); !strings.HasPrefix(got, want) {
		t.Errorf("once a probe fails for its own reason, the status line reads %q; want it to start %q", got, want)
	}

	again := h.tickUntilAsked()
	if again.Format("15:04:05") == asked.Format("15:04:05") {
		t.Fatalf("the second question after the router came at %v, the first's second; want a later one, to tell them apart", again)
	}
	if got, want := strings.TrimSpace(strings.Split(h.view(), "\n")[2]), "■ the router isn't answering  ·  since "+asked.Format("15:04:05"); got != want {
		t.Errorf("once a question finds the router gone again, the status line reads %q; want %q, since it was first found gone", got, want)
	}
}

func TestAProbeThatFailsUnderAProbedDocumentSaysWhyThoughTheRouterIsFoundGone(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	h.start()
	h.source.err = errors.New("the keychain is locked")
	h.tickUntilRead()
	h.tickUntilAsked() // A question after the router, which finds it gone.

	want := "! couldn't read usage: the keychain is locked  ·  next "
	if got := strings.TrimSpace(strings.Split(h.view(), "\n")[2]); !strings.HasPrefix(got, want) {
		t.Errorf("once a question finds the router gone, the status line reads %q; want it to start %q, as it said the router gone already", got, want)
	}
}

func TestProbingAsAskedAReadThatFailsSaysWhyAndNothingOfTheRouter(t *testing.T) {
	h := newHarness(t, calm())
	h.model.cfg.ProbeOnly = true
	h.source.probing = true
	h.source.err = errors.New("connection refused")
	h.start()
	h.tickUntilAsked() // A question after the router, which a source probing as asked never reads.

	want := "! couldn't read usage: connection refused  ·  next 13:14"
	if got := strings.TrimSpace(strings.Split(h.view(), "\n")[2]); got != want || !h.model.lost.IsZero() {
		t.Errorf("the status line reads %q, the router lost at %v; want %q, why and when it reads next, with no router to lose", got, h.model.lost, want)
	}

	h.source.err = nil
	h.tickUntilRead()
	if got := strings.TrimSpace(strings.Split(h.view(), "\n")[2]); got != "○ probed, as asked" {
		t.Errorf("once a read lands, the status line reads %q, want it probed, as asked", got)
	}
}

func TestFallsBackToProbingWhenTheRouterStops(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	h.stopRouter()
	h.tickUntil(at(13, 41, 59))
	if h.source.reads != 0 {
		t.Fatalf("before the full read is due, probed %d times, want never", h.source.reads)
	}
	h.tickUntil(at(13, 42, 0).Add(tickSlack))
	if h.source.reads != 1 {
		t.Fatalf("once the full read is due, probed %d times, want once", h.source.reads)
	}
	if got, want := h.nextRead(), "14:12"; got != want {
		t.Errorf("the next read is due at %s, want %s", got, want)
	}
	if got := strings.TrimSpace(strings.Split(h.view(), "\n")[2]); got != "○ probed · router not running" {
		t.Errorf("the status line reads %q, want it to say the accounts are probed, as the router isn't running, rather than since when it hasn't answered", got)
	}
}

func TestReadsTheRouterAgainOnceItAnswers(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.stopRouter()
	h.deliver(h.press("R")...) // Probing from 13:12.

	h.tickUntil(at(13, 13, 30))
	h.startRouter(routerDocument(three()...))
	h.tickUntilAsked() // 13:14.
	if !h.model.routed() {
		t.Error("once the router answers again, the document on screen isn't the router's, want it read")
	}
	if got, want := h.source.asked[len(h.source.asked)-1], (status.Read{Refresh: interval}); got != want {
		t.Errorf("the read that found the router asked for %+v, want %+v: a question after the router, having it refresh", got, want)
	}
	if h.source.reads != 1 {
		t.Errorf("probed %d times, want once, as the router stopped: asking after it probes nothing", h.source.reads)
	}
}

func TestAsksAfterTheRouterOnceAMinuteAtMost(t *testing.T) {
	// The session is back at 13:17, so until then a countdown ticks every
	// second.
	h := newHarness(t, document(account("work", "Work", refused(session(1, 5*time.Minute)), week(0.5))))
	h.start()

	ticks := h.tickUntil(at(13, 16, 30))
	if ticks < 200 {
		t.Fatalf("ticked %d times by 13:16:30, want every second", ticks)
	}
	if got := countReads(h.source.asked, status.Read{Refresh: interval}); got != 4 {
		t.Errorf("asked after the router %d times by 13:16:30, want 4: once at each minute", got)
	}
}

func TestAsksAfterTheRouterWhenAProbeIsDue(t *testing.T) {
	h := newHarness(t, calm())
	h.start()

	h.tickUntil(at(13, 41, 59).Add(tickSlack))
	h.startRouter(routerDocument(three()...))
	h.tickUntilAsked() // 13:42, when the probe is due.
	if got, want := h.source.asked[len(h.source.asked)-1], (status.Read{Refresh: interval, Probe: true}); got != want {
		t.Errorf("the read due at 13:42 asked for %+v, want %+v", got, want)
	}
	if !h.model.routed() || h.source.reads != 1 {
		t.Errorf("after the read due at 13:42, routed = %v and probes = %d, want the router read, and no probe", h.model.routed(), h.source.reads)
	}
}

func TestRefreshKeyReadingTheRouter(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()

	h.deliver(h.press("R")...)
	if got, want := h.source.asked[len(h.source.asked)-1], (status.Read{Refresh: status.FreshFor, Probe: true}); got != want {
		t.Errorf("R asked for %+v, want %+v: the router refreshing what it hasn't read in the last minute", got, want)
	}
}

func TestTheKeyLineByWhatTheDashboardReads(t *testing.T) {
	many := make([]status.Account, 12)
	for i := range many {
		many[i] = account(string(rune('a'+i)), "Account", session(0.1, 3*time.Hour))
	}
	tests := []struct {
		name string
		doc  status.Document
		want string
	}{
		{name: "the router of three accounts", doc: routerDocument(three()...), want: threeKeys},
		{name: "the router of one, which has none other to pin", doc: routerDocument(three()[0]), want: oneKeys},
		{name: "the router of twelve", doc: routerDocument(many...), want: threeKeys},
		{name: "a probe of three", doc: probedWithoutTheRouter(), want: threeKeys},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.doc)
			if tt.doc.Source == status.SourceRouter {
				h.startRouter(tt.doc)
			}
			h.start()

			if got := h.keyLine(); got != tt.want {
				t.Errorf("the key line reads %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPostsNothingWhileReadingTheRouter(t *testing.T) {
	full := routerDocument(account("work", "Work", refused(session(1, 30*time.Minute)), week(0.85), fableWeek(0.5)))
	room := routerDocument(account("work", "Work", session(0.02, 5*time.Hour), week(0.91), fableWeek(0.5)))
	h := routedHarness(t, full)
	h.start()

	h.startRouter(room)
	h.tickUntilAsked()
	if len(h.notifier.posted) > 0 {
		t.Errorf("reading the router, posted %q, want nothing: the router posts its own", h.notifier.posted)
	}

	h.stopRouter()
	h.source.doc = document(account("work", "Work", session(0.02, 5*time.Hour), week(0.93), fableWeek(0.5)))
	h.clock.now = at(13, 20, 0)
	h.deliver(h.press("R")...)
	if h.model.routed() || len(h.notifier.posted) > 0 {
		t.Errorf("probing once the router stopped, routed = %v and posted %q, want probing, and nothing yet: nothing has changed since the router's last document", h.model.routed(), h.notifier.posted)
	}
	h.source.doc = document(account("work", "Work", session(0.02, 5*time.Hour), week(0.93), fableWeek(0.95)))
	h.clock.now = at(13, 25, 0)
	h.deliver(h.press("R")...)
	if want := []string{"work · Work: Fable week at 95%"}; !slices.Equal(h.notifier.posted, want) {
		t.Errorf("probing once the router stopped, posted %q, want %q: the dashboard's own", h.notifier.posted, want)
	}
}

func TestPostsNothingProbingAsAskedWhileTheRouterAnswers(t *testing.T) {
	log := logstest.Capture(t)
	h := newHarness(t, document(account("work", "Work", refused(session(1, 30*time.Minute)), week(0.85), fableWeek(0.5))))
	h.source.probing = true
	h.startRouter(routerDocument(three()...))
	h.start()

	h.clock.now = at(13, 20, 0)
	h.read(document(account("work", "Work", session(0.02, 5*time.Hour), week(0.91), fableWeek(0.5))))
	if h.model.routed() || len(h.notifier.posted) > 0 {
		t.Errorf("probing as asked while the router answers, routed = %v and posted %q, want probing, and nothing: the router posts its own", h.model.routed(), h.notifier.posted)
	}
	for _, news := range []string{`news="room again"`, `news="Week at 91%"`} {
		if want := []string{"level=DEBUG", `msg="notification left to the router" component=watch`, "account=work", news}; !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}

	h.stopRouter()
	h.clock.now = at(13, 25, 0)
	h.read(document(account("work", "Work", session(0.02, 5*time.Hour), week(0.91), fableWeek(0.95))))
	if want := []string{"work · Work: Fable week at 95%"}; !slices.Equal(h.notifier.posted, want) {
		t.Errorf("probing as asked once the router stopped, posted %q, want %q: the dashboard's own, of what changed since", h.notifier.posted, want)
	}
}

func TestLogsTheRouterGoingAndComingBack(t *testing.T) {
	log := logstest.Capture(t)
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.tickUntilAsked()
	h.stopRouter()
	h.tickUntilAsked()
	h.deliver(h.press("R")...)
	h.startRouter(routerDocument(three()...))
	h.tickUntil(at(13, 13, 30))
	h.deliver(h.commanded(order{accounts: []string{"work"}})...)

	for _, want := range [][]string{
		{"level=INFO", `msg="reading the router" component=watch`},
		{"level=INFO", `msg="usage read" component=watch`, "source=router", "read=work,personal,side"},
		{"level=DEBUG", `msg="usage read" component=watch`, "source=router"},
		{"level=WARN", `msg="the router stopped answering; showing its last document" component=watch`},
		{"level=WARN", `msg="the router stopped answering; probing directly" component=watch`, `router="not running"`},
		{"level=INFO", `msg="usage read" component=watch`, "source=probe"},
		{"level=INFO", "msg=pinned component=watch", "accounts=work", "move=false"},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
	if n := strings.Count(log.String(), `msg="reading the router"`); n != 2 {
		t.Errorf("log reads\n%s\nwant the router read twice: at the start, and once it came back", log)
	}
}
