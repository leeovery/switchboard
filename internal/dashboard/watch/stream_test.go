package watch

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// tabKey is tab, as the terminal sends it, and toSessions the tabs that show
// Sessions from the Overview.
var (
	tabKey     = tea.KeyPressMsg{Code: tea.KeyTab}
	toSessions = []tea.KeyPressMsg{tabKey, tabKey, tabKey}
)

// streamingHarness is a model of the router of three, d28c busy on work and
// 7f3a idle there, in a terminal 160 columns wide and 40 tall, started on
// the Overview, ROUTING hidden there, so the stream opens once Sessions
// shows.
func streamingHarness(t *testing.T) *harness {
	t.Helper()
	return streamingOn(t, dashboard.Sections{}.Toggled(dashboard.SectionRouting))
}

// streamingOn is streamingHarness, the Overview showing the sections given.
func streamingOn(t *testing.T, sections dashboard.Sections) *harness {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
	h.model.sections = sections
	h.source.sessions = []status.Session{
		sessionOn(idD28C, opus, "work", 10*time.Second),
		sessionOn(id7F3A, "claude-haiku-4-5", "work", 9*time.Minute),
	}
	h.update(tea.WindowSizeMsg{Width: 160, Height: 40})
	h.start()
	return h
}

// opened counts the times the request stream was opened.
func (h *harness) opened() int {
	return len(h.source.streams)
}

// closed reports whether the request stream opened n-th, from 1, was closed:
// its context ended.
func (h *harness) closed(n int) bool {
	return h.source.contexts[n-1].Err() != nil
}

// pendingRejoin is the timer armed to ask for the stream again and not yet
// fired, if any.
func (h *harness) pendingRejoin() (*timer, bool) {
	for _, tm := range slices.Backward(h.timers) {
		if _, ok := tm.msg.(rejoinMsg); ok && !tm.fired {
			return tm, true
		}
	}
	return nil, false
}

func TestTheStreamIsReadWhileSessionsTheLogOrTheOverviewsROUTINGShows(t *testing.T) {
	h := streamingOn(t, dashboard.Sections{})

	steps := []struct {
		key string
		// opened is how many times the stream has been opened since the
		// start, and open whether the last is open.
		opened int
		open   bool
	}{
		{key: "", opened: 1, open: true},
		{key: "tab", opened: 1},
		{key: "tab", opened: 1},
		{key: "tab", opened: 2, open: true},
		{key: "tab", opened: 2},
		{key: "tab", opened: 2},
		{key: "tab", opened: 3, open: true},
		{key: "tab", opened: 3, open: true},
		{key: "shift+tab", opened: 3, open: true},
	}
	for _, step := range steps {
		switch step.key {
		case "":
		case "shift+tab":
			h.deliver(h.update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})...)
		case "tab":
			h.keys(tabKey)
		}
		open := h.opened() > 0 && !h.closed(h.opened())
		if h.opened() != step.opened || open != step.open {
			t.Errorf("after %q, showing %s, the stream was opened %d times, the last open %v; want %d, open %v",
				step.key, h.model.view, h.opened(), open, step.opened, step.open)
		}
	}
}

func TestTheOverviewReadsTheStreamWhileROUTINGOrSESSIONSShows(t *testing.T) {
	none := dashboard.Sections{}.Toggled(dashboard.SectionRouting)
	tests := []struct {
		name     string
		sections dashboard.Sections
		want     bool
	}{
		{name: "ROUTING, as at first", sections: dashboard.Sections{}, want: true},
		{name: "SESSIONS alone", sections: none.Toggled(dashboard.SectionSessions), want: true},
		{name: "neither", sections: none},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.model.sections = tt.sections
			h.start()
			if open := h.opened() > 0 && !h.closed(h.opened()); open != tt.want {
				t.Errorf("showing %v, the stream is open: %v, want %v", tt.sections.Names(), open, tt.want)
			}
		})
	}
}

func TestTheStreamIsntReadWithoutTheRouter(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	h.start()
	h.keys(toSessions...)

	if h.opened() > 0 {
		t.Errorf("probing, the stream was opened %d times, want none", h.opened())
	}
}

func TestTheStreamTellsOfRequestsAsTheyHappen(t *testing.T) {
	h := streamingHarness(t)
	h.deliver(h.press("s")...)

	h.hear(told(router.StreamSent, "r1", "work", 0))
	if !strings.Contains(h.view(), "d28c  opus    now   ↑ ask") {
		t.Errorf("as d28c's request goes out, the screen is\n%s\nwant it asking", h.view())
	}
	h.hear(told(router.StreamFirst, "r1", "work", time.Second), alter(told(router.StreamProgress, "r1", "work", 2*time.Second), func(e *router.StreamEvent) { e.Chars = 4936 }))
	if !strings.Contains(h.view(), "d28c  opus    now   ↓ ~1.2k") {
		t.Errorf("as its answer streams, the screen is\n%s\nwant its tokens so far", h.view())
	}
	h.hear(alter(told(router.StreamDone, "r1", "work", 3*time.Second), func(e *router.StreamEvent) {
		e.Status, e.Tokens = 200, &quota.Tokens{Output: 1180}
	}))
	if !strings.Contains(h.view(), "d28c  opus    now   idle") {
		t.Errorf("as its answer ends, the screen is\n%s\nwant it idle", h.view())
	}
	if !h.listening() {
		t.Error("having told of the request, the model no longer listens to the stream")
	}
}

func TestTheQuotaCheckIsntDrawnAsARequest(t *testing.T) {
	h := streamingHarness(t)
	h.deliver(h.press("s")...)

	h.hear(alter(told(router.StreamSent, "r1", "work", 0), func(e *router.StreamEvent) { e.Check = true }))
	if strings.Contains(h.view(), "↑ ask") {
		t.Errorf("as the quota check goes out, the screen is\n%s\nwant nothing of it", h.view())
	}
}

func TestTheStreamIsAskedForAgainOnceItEnds(t *testing.T) {
	h := streamingHarness(t)
	h.deliver(h.press("s")...)
	h.hear(alter(told(router.StreamInFlight, "r1", "work", 0), func(e *router.StreamEvent) { e.SentAt = past(-5 * time.Second) }))

	h.endStream()
	rejoin, ok := h.pendingRejoin()
	if !ok || rejoin.delay != rejoinAfter {
		t.Fatalf("once the stream ends, the rejoin armed is %+v, want one in %v", rejoin, rejoinAfter)
	}
	if calls := h.model.frame(h.clock.now).Traffic.Calls; len(calls) != 1 {
		t.Errorf("until the stream opens again, the frame draws seats doing %+v, want what it told kept", calls)
	}
	h.fire(rejoin)
	if h.opened() != 2 {
		t.Fatalf("the rejoin opened the stream %d times in all, want twice", h.opened())
	}
	if calls := h.model.frame(h.clock.now).Traffic.Calls; len(calls) > 0 {
		t.Errorf("opened again, the frame draws seats doing %+v, want what the stream told before forgotten, till it tells it afresh", calls)
	}
	h.hear(alter(told(router.StreamInFlight, "r1", "work", time.Second), func(e *router.StreamEvent) { e.SentAt = past(-5 * time.Second) }))
	if !strings.Contains(h.view(), "↑ ask") {
		t.Errorf("told afresh, the screen is\n%s\nwant d28c asking", h.view())
	}
}

// A stream the watch closes tells nothing more of what each seat is doing, so
// none is drawn doing anything until it opens again and tells afresh; the
// requests it told of are kept, for its first look to settle.
func TestOnceTheWatchClosesTheStreamNoSeatIsDrawnDoingAnything(t *testing.T) {
	tests := []struct {
		name  string
		close func(h *harness)
	}{
		{name: "the router stops answering, for half an hour", close: func(h *harness) {
			h.stopRouter()
			h.tickUntilAsked()
			h.tickUntil(h.clock.now.Add(30 * time.Minute))
		}},
		{name: "ROUTING hidden, then shown again, the stream yet to open", close: func(h *harness) {
			h.deliver(h.press("r")...)
			h.press("r")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := streamingOn(t, dashboard.Sections{})
			h.hear(told(router.StreamSent, "r1", "work", 0))
			tt.close(h)

			if !h.closed(1) {
				t.Fatal("the stream opened first is open still, want it closed")
			}
			f := h.model.frame(h.clock.now)
			if len(f.Traffic.Calls) > 0 {
				t.Errorf("the stream closed, the frame draws seats doing %+v, want none doing anything till it opens again", f.Traffic.Calls)
			}
			if r := f.Stream.Requests["r1"]; !r.InFlight() {
				t.Errorf("the stream closed, r1 is %+v, want it kept in flight, for the stream's first look to settle", r)
			}
		})
	}
}

func TestAStreamThatWontOpenIsAskedForLaterEachTime(t *testing.T) {
	log := logstest.Capture(t)
	h := streamingHarness(t)
	h.source.streamErr = errors.New("ask the router: connection refused")
	h.keys(toSessions...)

	var waits []time.Duration
	for range 7 {
		rejoin, ok := h.pendingRejoin()
		if !ok {
			t.Fatal("no rejoin armed")
		}
		waits = append(waits, rejoin.delay)
		h.fire(rejoin)
	}
	if want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}; !slices.Equal(waits, want) {
		t.Errorf("the stream was asked for again after %v, want %v", waits, want)
	}
	if !log.Has("level=DEBUG", `msg="couldn't open the router's request stream"`) {
		t.Errorf("log reads\n%s\nwant why the stream didn't open", log)
	}
}

func TestARouterFromBeforeTheStreamGoesWithoutIt(t *testing.T) {
	log := logstest.Capture(t)
	h := streamingHarness(t)
	h.source.health = router.Health{OK: true, PID: 100, StartedAt: start.UTC()}
	h.tickUntilAsked()
	h.source.streamErr = fmt.Errorf("%w: the router answered GET /stream with 404 Not Found", router.ErrNoStream)
	h.deliver(h.press("s")...)

	if _, ok := h.pendingRejoin(); ok || !h.model.stream.absent {
		t.Error("a router from before the stream has it asked for again, want the watch to go without it")
	}
	if row := "d28c  opus    now   idle"; !strings.Contains(h.view(), row) {
		t.Errorf("without the stream, the screen is\n%s\nwant d28c as the look read it, %q", h.view(), row)
	}
	if !log.Has("level=INFO", `msg="the router is from before GET /stream: the sessions are drawn as each look reads them"`) {
		t.Errorf("log reads\n%s\nwant it to say the router has no stream", log)
	}

	// The stream would open now, were it asked for again.
	h.source.streamErr = nil
	h.tickUntilAsked()
	if h.opened() > 0 {
		t.Errorf("the same router answering again, the stream was opened %d times, want none", h.opened())
	}
	h.source.health = router.Health{OK: true, PID: 200, StartedAt: start.Add(time.Minute).UTC()}
	h.tickUntilAsked()
	if h.opened() != 1 {
		t.Errorf("another router answering, as one restarted, the stream was opened %d times, want once", h.opened())
	}
}

func TestAnotherRouterHasItsStreamAskedForAtOnce(t *testing.T) {
	h := streamingHarness(t)
	h.source.health = router.Health{OK: true, PID: 100, StartedAt: start.UTC()}
	h.tickUntilAsked()
	h.source.streamErr = errors.New("the router is stopping: ask again once it's back")
	h.keys(toSessions...)
	for range 3 {
		rejoin, _ := h.pendingRejoin()
		h.fire(rejoin)
	}
	rejoin, _ := h.pendingRejoin()

	h.source.streamErr = nil
	h.source.health = router.Health{OK: true, PID: 200, StartedAt: start.Add(time.Minute).UTC()}
	asked := h.tickUntilAsked()
	if !asked.Before(rejoin.due) || h.opened() != 1 {
		t.Errorf("another router answered at %s, the stream opened %d times, want it opened then, before the rejoin due at %s",
			asked.Format(time.TimeOnly), h.opened(), rejoin.due.Format(time.TimeOnly))
	}
}

func TestAnotherRouterAnsweringForgetsWhatTheLastOnesStreamTold(t *testing.T) {
	h := streamingHarness(t)
	h.source.health = router.Health{OK: true, PID: 100, StartedAt: start.UTC()}
	h.tickUntilAsked()
	h.deliver(h.press("s")...)
	h.hear(told(router.StreamSent, "r1", "work", 0), told(router.StreamFirst, "r1", "work", time.Second))
	h.endStream()

	// It restarts: d28c's request went with the router it was on.
	h.source.health = router.Health{OK: true, PID: 200, StartedAt: start.Add(time.Minute).UTC()}
	h.tickUntilAsked()
	h.tickUntil(past(3 * time.Minute))
	if view := h.view(); !strings.Contains(view, "d28c  opus") || strings.Contains(view, "d28c  opus    now") {
		t.Errorf("minutes after d28c was last seen, the screen is\n%s\nwant d28c idle, its request that went with the last router no longer in flight", h.view())
	}
}

func TestWhatTheStreamSawOfASessionOutlastsItsAnswer(t *testing.T) {
	h := streamingHarness(t)
	h.deliver(h.press("s")...)
	h.hear(told(router.StreamSent, "r1", "work", 0), told(router.StreamFirst, "r1", "work", time.Second))
	h.tickUntil(past(90 * time.Second))
	h.hear(alter(told(router.StreamDone, "r1", "work", 90*time.Second), func(e *router.StreamEvent) { e.Status = 200 }))

	h.tickUntil(past(90*time.Second + pulseFor + time.Second))
	if want := "d28c  opus    now "; !strings.Contains(h.view(), want) {
		t.Errorf("once its 90-second answer is done with, the screen is\n%s\nwant d28c seen now, as the stream saw it as its answer ended, %q", h.view(), want)
	}
}

// d28c's answer, sent as the router listed it seen and ended four minutes
// on, is let go once a read of the ledger's tail reads its line, its line
// telling of it from then on; and SESSIONS has d28c idle three minutes
// three minutes after it ended, as the stream saw it end, not seven, as the
// router last saw it routed.
func TestASessionsIdleCountsFromItsAnswersEndOnceItsLineTakesOver(t *testing.T) {
	state := t.TempDir()
	h := routedHarness(t, routerDocument(three()...))
	h.model.sections = dashboard.Sections{}.Toggled(dashboard.SectionRouting).Toggled(dashboard.SectionSessions)
	h.source.sessions = []status.Session{sessionOn(idD28C, opus, "work", 0)}
	h.model.cfg.Ledger = ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, views.DigestVersion, logs.For("watch"))
	h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: 40})...)
	h.start()
	h.hear(told(router.StreamSent, "r1", "work", 0), told(router.StreamFirst, "r1", "work", time.Second))
	h.tickUntil(past(4 * time.Minute))
	h.hear(alter(told(router.StreamDone, "r1", "work", 4*time.Minute), func(e *router.StreamEvent) { e.Status = 200 }))
	writeLines(t, state, took(arrived("r1", "work", 0), 4*time.Minute))

	h.tickUntil(past(7 * time.Minute))
	if _, ok := toldOf(h.model.traffic, "r1"); ok {
		t.Error("its line read, r1 is kept, want it let go")
	}
	if want := "d28c  opus     3m   idle"; !strings.Contains(h.view(), want) {
		t.Errorf("three minutes after d28c's answer ended, the screen is\n%s\nwant %q", h.view(), want)
	}
}

// movedToSide is what the request stream tells of d28c's request r1, sent on
// work d past when the clock starts and refused there by work's limit, as
// the router moves d28c to side and sends it again there.
func movedToSide(d time.Duration) []router.StreamEvent {
	return []router.StreamEvent{
		told(router.StreamSent, "r1", "work", d),
		alter(told(router.StreamLimited, "r1", "work", d), func(e *router.StreamEvent) { e.Status = 429 }),
		alter(told(router.StreamMoved, "r1", "side", d), func(e *router.StreamEvent) { e.From, e.To, e.Reason = "work", "side", "moved: work hit its limit" }),
		alter(told(router.StreamSent, "r1", "side", d), func(e *router.StreamEvent) { e.Attempt = 2 }),
	}
}
