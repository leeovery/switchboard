package watch

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// streamingHarness is a model of the router of three, d28c busy on work and
// 7f3a idle there, in a terminal 160 columns wide and 40 tall, started on
// the Accounts view.
func streamingHarness(t *testing.T) *harness {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
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

func TestTheStreamIsReadWhileSessionsShowsOrACardIsFlipped(t *testing.T) {
	h := streamingHarness(t)

	steps := []struct {
		key string
		// opened is how many times the stream has been opened since the
		// start, and open whether the last is open.
		opened int
		open   bool
	}{
		{key: "", opened: 0},
		{key: "tab", opened: 1, open: true},
		{key: "tab", opened: 1},
		{key: "tab", opened: 1},
		{key: " ", opened: 2, open: true},
		{key: " ", opened: 2},
		{key: "s", opened: 3, open: true},
		{key: "shift+tab", opened: 3},
	}
	for _, step := range steps {
		switch step.key {
		case "":
		case "shift+tab":
			h.deliver(h.update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})...)
		case " ":
			h.keys(spaceKey)
		case "tab":
			h.deliver(h.update(tea.KeyPressMsg{Code: tea.KeyTab})...)
		default:
			h.deliver(h.press(step.key)...)
		}
		open := h.opened() > 0 && !h.closed(h.opened())
		if h.opened() != step.opened || open != step.open {
			t.Errorf("after %q, showing %s, flipped %v, the stream was opened %d times, the last open %v; want %d, open %v",
				step.key, h.model.view, h.model.flipped, h.opened(), open, step.opened, step.open)
		}
	}
}

func TestTheStreamIsntReadWithoutTheRouter(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	h.start()
	h.deliver(h.update(tea.KeyPressMsg{Code: tea.KeyTab})...)

	if h.opened() > 0 {
		t.Errorf("probing, the stream was opened %d times, want none", h.opened())
	}
}

func TestTheStreamTellsOfRequestsAsTheyHappen(t *testing.T) {
	h := streamingHarness(t)
	h.deliver(h.update(tea.KeyPressMsg{Code: tea.KeyTab})...)

	h.hear(told(router.StreamSent, "r1", "work", 0))
	if !strings.Contains(h.view(), "d28c  opus    now  ↑ ask") {
		t.Errorf("as d28c's request goes out, the screen is\n%s\nwant it asking", h.view())
	}
	if _, ok := h.pendingFrame(); !ok {
		t.Error("as d28c's request goes out, no frame is armed to draw its pulse")
	}
	h.hear(told(router.StreamFirst, "r1", "work", time.Second), alter(told(router.StreamProgress, "r1", "work", 2*time.Second), func(e *router.StreamEvent) { e.Chars = 4936 }))
	if !strings.Contains(h.view(), "d28c  opus    now  ↓ ~1.2k") {
		t.Errorf("as its answer streams, the screen is\n%s\nwant its tokens so far", h.view())
	}
	h.hear(alter(told(router.StreamDone, "r1", "work", 3*time.Second), func(e *router.StreamEvent) {
		e.Status, e.Tokens = 200, &quota.Tokens{Output: 1180}
	}))
	if !strings.Contains(h.view(), "d28c  opus    now  ↓ 1.2k") {
		t.Errorf("as its answer ends, the screen is\n%s\nwant its tokens, exact", h.view())
	}
	if !h.listening() {
		t.Error("having told of the request, the model no longer listens to the stream")
	}
}

func TestThrottlingAndTheQuotaCheckArentDrawnAsRequests(t *testing.T) {
	h := streamingHarness(t)
	h.deliver(h.update(tea.KeyPressMsg{Code: tea.KeyTab})...)

	h.hear(alter(told(router.StreamSent, "r1", "work", 0), func(e *router.StreamEvent) { e.Check = true }))
	if strings.Contains(h.view(), "↑ ask") {
		t.Errorf("as the quota check goes out, the screen is\n%s\nwant nothing of it", h.view())
	}
	h.settle()
	h.hear(told(router.StreamSent, "r2", "work", 0), alter(told(router.StreamThrottled, "r2", "work", 0), func(e *router.StreamEvent) { e.Status = 429 }))
	if !strings.Contains(h.view(), "d28c  opus    now  … 429") {
		t.Errorf("throttled, the screen is\n%s\nwant it dim, saying so", h.view())
	}
	if h.model.traffic.moving(h.clock.now) {
		t.Error("throttled, its cord moves, want no pulse")
	}
}

func TestTheStreamIsAskedForAgainOnceItEnds(t *testing.T) {
	h := streamingHarness(t)
	h.deliver(h.update(tea.KeyPressMsg{Code: tea.KeyTab})...)
	h.hear(alter(told(router.StreamInFlight, "r1", "work", 0), func(e *router.StreamEvent) { e.SentAt = past(-5 * time.Second) }))

	h.endStream()
	rejoin, ok := h.pendingRejoin()
	if !ok || rejoin.delay != rejoinAfter {
		t.Fatalf("once the stream ends, the rejoin armed is %+v, want one in %v", rejoin, rejoinAfter)
	}
	if !strings.Contains(h.view(), "↑ ask") {
		t.Errorf("until the stream opens again, the screen is\n%s\nwant what it told kept", h.view())
	}
	h.fire(rejoin)
	if h.opened() != 2 {
		t.Fatalf("the rejoin opened the stream %d times in all, want twice", h.opened())
	}
	if strings.Contains(h.view(), "↑ ask") {
		t.Errorf("opened again, the screen is\n%s\nwant what the stream told before forgotten, till it tells it afresh", h.view())
	}
	h.hear(alter(told(router.StreamInFlight, "r1", "work", time.Second), func(e *router.StreamEvent) { e.SentAt = past(-5 * time.Second) }))
	if !strings.Contains(h.view(), "↑ ask") {
		t.Errorf("told afresh, the screen is\n%s\nwant d28c asking", h.view())
	}
}

func TestAStreamThatWontOpenIsAskedForLaterEachTime(t *testing.T) {
	log := logstest.Capture(t)
	h := streamingHarness(t)
	h.source.streamErr = errors.New("ask the router: connection refused")
	h.deliver(h.update(tea.KeyPressMsg{Code: tea.KeyTab})...)

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
	h.deliver(h.update(tea.KeyPressMsg{Code: tea.KeyTab})...)

	if _, ok := h.pendingRejoin(); ok || !h.model.stream.absent {
		t.Error("a router from before the stream has it asked for again, want the watch to go without it")
	}
	if row := "d28c  opus    now          ●━"; !strings.Contains(h.view(), row) {
		t.Errorf("without the stream, the screen is\n%s\nwant d28c's call as the look read it, %q, nothing travelling", h.view(), row)
	}
	if !log.Has("level=INFO", `msg="the router is from before GET /stream: the cords and the cards' backs draw the sessions as each look reads them"`) {
		t.Errorf("log reads\n%s\nwant it to say the router has no stream", log)
	}

	h.tickUntilAsked()
	if h.opened() > 0 {
		t.Errorf("the same router answering again, the stream was opened %d times, want none", h.opened())
	}
	h.source.streamErr = nil
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
	h.deliver(h.update(tea.KeyPressMsg{Code: tea.KeyTab})...)
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

func TestTheCardsBacksSayWhatTheStreamTells(t *testing.T) {
	h := gridHarness(t)
	h.keys(spaceKey)
	h.hear(
		alter(told(router.StreamInFlight, "r1", "work", 0), func(e *router.StreamEvent) {
			e.SentAt, e.FirstAt, e.Chars = past(-time.Minute), past(-50*time.Second), 4936
		}),
		alter(told(router.StreamInFlight, "r2", "work", 0), func(e *router.StreamEvent) {
			e.Session, e.Model, e.SentAt = id7F3A, "claude-haiku-4-5", past(-38*time.Second)
		}),
	)

	for _, want := range []string{"● d28c  opus    streaming  ↓ ~1.2k", "● 7f3a  haiku   waiting    38s"} {
		if !strings.Contains(h.view(), want) {
			t.Errorf("the screen is\n%s\nwant %q", h.view(), want)
		}
	}
	h.fire(h.lastTick())
	if want := "● 7f3a  haiku   waiting    39s"; !strings.Contains(h.view(), want) {
		t.Errorf("a second on, the screen is\n%s\nwant the wait counting on, %q", h.view(), want)
	}
}
