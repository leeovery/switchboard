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
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
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
	h.keys(toSessions...)

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
	h.keys(toSessions...)

	h.hear(alter(told(router.StreamSent, "r1", "work", 0), func(e *router.StreamEvent) { e.Check = true }))
	if strings.Contains(h.view(), "↑ ask") {
		t.Errorf("as the quota check goes out, the screen is\n%s\nwant nothing of it", h.view())
	}
	h.settle()
	h.hear(told(router.StreamSent, "r2", "work", 0), alter(told(router.StreamThrottled, "r2", "work", 0), func(e *router.StreamEvent) { e.Status = 429 }))
	if !strings.Contains(h.view(), "d28c  opus    now  … 429") {
		t.Errorf("throttled, the screen is\n%s\nwant it dim, saying so", h.view())
	}
	if _, ok := h.pendingFrame(); ok {
		t.Error("throttled, a frame is armed, want none: no pulse runs along its cord")
	}
}

func TestTheStreamIsAskedForAgainOnceItEnds(t *testing.T) {
	h := streamingHarness(t)
	h.keys(toSessions...)
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
	h.keys(toSessions...)

	if _, ok := h.pendingRejoin(); ok || !h.model.stream.absent {
		t.Error("a router from before the stream has it asked for again, want the watch to go without it")
	}
	if row := "d28c  opus    now          ●━"; !strings.Contains(h.view(), row) {
		t.Errorf("without the stream, the screen is\n%s\nwant d28c's call as the look read it, %q, nothing travelling", h.view(), row)
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
	h.keys(toSessions...)
	h.hear(told(router.StreamSent, "r1", "work", 0), told(router.StreamFirst, "r1", "work", time.Second))
	h.endStream()

	// It restarts: d28c's request went with the router it was on.
	h.source.health = router.Health{OK: true, PID: 200, StartedAt: start.Add(time.Minute).UTC()}
	h.tickUntilAsked()
	h.tickUntil(past(3 * time.Minute))
	if strings.Contains(h.view(), "d28c  opus    now") {
		t.Errorf("minutes after d28c was last seen, the screen is\n%s\nwant d28c idle, its request that went with the last router no longer in flight", h.view())
	}
}

func TestWhatTheStreamSawOfASessionOutlastsItsAnswer(t *testing.T) {
	h := streamingHarness(t)
	h.keys(toSessions...)
	h.hear(told(router.StreamSent, "r1", "work", 0), told(router.StreamFirst, "r1", "work", time.Second))
	h.tickUntil(past(90 * time.Second))
	h.hear(alter(told(router.StreamDone, "r1", "work", 90*time.Second), func(e *router.StreamEvent) { e.Status = 200 }))

	h.tickUntil(past(90*time.Second + answeredFor + time.Second))
	if want := "  d28c  opus    now "; !strings.Contains(h.view(), want) {
		t.Errorf("once its 90-second answer is done with, the screen is\n%s\nwant d28c seen now, as the stream saw it as its answer ended, %q", h.view(), want)
	}
}

func TestTheCordsDrawFramesOnSessionsSwitchboardAlone(t *testing.T) {
	alone := func(t *testing.T) *harness {
		h := routedHarness(t, routerDocument(three()[0]))
		h.source.sessions = []status.Session{sessionOn(idD28C, opus, "work", 10*time.Second)}
		h.update(tea.WindowSizeMsg{Width: 160, Height: 40})
		h.start()
		return h
	}
	narrow := func(t *testing.T) *harness {
		h := streamingHarness(t)
		h.update(tea.WindowSizeMsg{Width: 89, Height: 40})
		return h
	}
	tests := []struct {
		name    string
		harness func(*testing.T) *harness
		keys    []tea.KeyPressMsg
		want    bool
	}{
		{name: "Sessions' switchboard, its cord shimmering", harness: streamingHarness, keys: toSessions, want: true},
		{name: "the Overview", harness: func(t *testing.T) *harness { return streamingOn(t, dashboard.Sections{}) }},
		{name: "Sessions' plain list of one account", harness: alone, keys: toSessions},
		{name: "Sessions' plain list, under 90 columns", harness: narrow, keys: toSessions},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.harness(t)
			h.keys(tt.keys...)
			h.settle()
			h.hear(told(router.StreamSent, "r1", "work", 0), told(router.StreamFirst, "r1", "work", 0))
			h.framesUntil(h.clock.now.Add(brightFor + frameEvery))
			if _, framing := h.pendingFrame(); framing != tt.want {
				t.Errorf("as d28c's answer streams, work's rate brightened by its request faded back, a frame is armed: %v, want %v", framing, tt.want)
			}
		})
	}
}

func TestAShimmerAloneIsDrawnAStepAtATime(t *testing.T) {
	h := streamingHarness(t)
	h.keys(toSessions...)
	h.settle()
	h.hear(told(router.StreamSent, "r1", "work", -2*time.Second), told(router.StreamFirst, "r1", "work", -time.Second))

	for range 3 {
		frames := h.pendingFrames()
		var delays []time.Duration
		for _, tm := range frames {
			delays = append(delays, tm.delay)
		}
		if want := untilStep(h.clock.now, shimmerStep) + stepSlack; !slices.Equal(delays, []time.Duration{want}) {
			t.Fatalf("d28c's answer streaming alone, frames are armed %v on; want one, %v on, just past when its shimmer next steps", delays, want)
		}
		h.fire(frames[0])
	}
}

func TestAMoveToldAsTheSessionsAreListedRePatchesTillTheyreListedWhereItWent(t *testing.T) {
	h := streamingHarness(t)
	h.keys(toSessions...)
	read := h.press("R")
	h.clock.now = past(time.Second)
	h.hear(
		told(router.StreamSent, "r1", "work", 0),
		alter(told(router.StreamLimited, "r1", "work", 300*time.Millisecond), func(e *router.StreamEvent) { e.Status = 429 }),
		alter(told(router.StreamMoved, "r1", "side", 300*time.Millisecond), func(e *router.StreamEvent) {
			e.From, e.To, e.Reason = "work", "side", "moved: work hit its limit"
		}),
		alter(told(router.StreamSent, "r1", "side", 300*time.Millisecond), func(e *router.StreamEvent) { e.Attempt = 2 }),
	)

	h.clock.now = past(2 * time.Second)
	h.deliver(read...)
	if placeholder := "  d28c  ↪ moved to Side"; !strings.Contains(h.view(), placeholder) {
		t.Errorf("the read under way as d28c moved landing, the screen is\n%s\nwant it re-patching still, %q, the sessions listed before it", h.view(), placeholder)
	}
	h.tickUntilAsked()
	if !strings.Contains(h.view(), "↪ moved to") {
		t.Errorf("the router listing d28c on work still, the screen is\n%s\nwant it re-patching still", h.view())
	}
	h.source.sessions[0] = sessionOn(idD28C, opus, "side", 0)
	h.tickUntilAsked()
	if strings.Contains(h.view(), "↪ moved to") {
		t.Errorf("the router listing d28c on side, where it went, the screen is\n%s\nwant its re-patch done", h.view())
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

func TestAMoveRePatchesThroughARejoinAndAViewSwitch(t *testing.T) {
	h := streamingHarness(t)
	h.keys(toSessions...)
	h.hear(movedToSide(0)...)
	h.clock.now = past(pulseFor + 100*time.Millisecond)
	h.redraw()
	const placeholder = "  d28c  ↪ moved to Side"
	if !strings.Contains(h.view(), placeholder) {
		t.Fatalf("d28c moved to side, the screen is\n%s\nwant it re-patched, %q", h.view(), placeholder)
	}

	h.endStream()
	rejoin, _ := h.pendingRejoin()
	h.fire(rejoin)
	if h.opened() != 2 || !strings.Contains(h.view(), placeholder) {
		t.Errorf("the stream opened again, %d times in all, the screen is\n%s\nwant d28c re-patched still, the router's sessions not listed since", h.opened(), h.view())
	}
	h.keys(tabKey, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if !strings.Contains(h.view(), placeholder) {
		t.Errorf("Runway shown, then Sessions again, the screen is\n%s\nwant d28c re-patched still", h.view())
	}
}

// pinnedToSide is what the request stream tells of d28c's request r1, d past
// when the clock starts, as d28c's own pin moves it to side, where the
// request goes out.
func pinnedToSide(d time.Duration) []router.StreamEvent {
	return []router.StreamEvent{
		alter(told(router.StreamMoved, "r1", "side", d), func(e *router.StreamEvent) { e.From, e.To, e.Reason = "work", "side", "pinned" }),
		told(router.StreamSent, "r1", "side", d),
	}
}

// twoOnWork is a model of the router of three, d28c and 7f3a, each on opus,
// on work, in a terminal 160 columns wide and 40 tall, showing Sessions.
func twoOnWork(t *testing.T) *harness {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{
		sessionOn(idD28C, opus, "work", 10*time.Second),
		sessionOn(id7F3A, opus, "work", 15*time.Second),
	}
	h.update(tea.WindowSizeMsg{Width: 160, Height: 40})
	h.start()
	h.keys(toSessions...)
	return h
}

func TestAMovesRePatchLastsTillAListingShowsItWhereItWent(t *testing.T) {
	h := twoOnWork(t)

	// The look asks for the sessions as the router's clock, behind the
	// watch's, has d28c move a moment later: the listing has it on work.
	read := h.press("R")
	h.hear(pinnedToSide(-100 * time.Millisecond)...)
	h.deliver(read...)
	placeholder, other := rowStarting(h.view(), "  d28c  ↪ moved to Side"), rowStarting(h.view(), "  7f3a  opus")
	if placeholder < 0 || placeholder > other {
		t.Errorf("a listing with d28c on work still landing, the screen is\n%s\nwant it re-patched still, its placeholder on its row, over 7f3a's", h.view())
	}

	h.source.sessions[0] = sessionOn(idD28C, opus, "side", 0)
	h.deliver(h.press("R")...)
	if strings.Contains(h.view(), "↪ moved to") || !strings.Contains(h.view(), "╌○") {
		t.Errorf("the router listing d28c on side, the screen is\n%s\nwant its re-patch done, its cord hanging loose from a jack of work's", h.view())
	}
}

// rowStarting is the first of the screen's rows that starts as given,
// counting from 0: -1 where none does.
func rowStarting(screen, start string) int {
	return slices.IndexFunc(strings.Split(screen, "\n"), func(row string) bool { return strings.HasPrefix(row, start) })
}

func TestAListingThatShowsAMoveDoneEndsItsRePatch(t *testing.T) {
	h := twoOnWork(t)

	// The router lists d28c on side as its clock, ahead of the watch's, has
	// it move a moment after the watch asks.
	h.source.sessions[0] = sessionOn(idD28C, opus, "side", 0)
	read := h.press("R")
	h.hear(pinnedToSide(100 * time.Millisecond)...)
	h.clock.now = past(200 * time.Millisecond)
	h.deliver(read...)
	if strings.Contains(h.view(), "↪ moved to") || !strings.Contains(h.view(), "╌○") {
		t.Errorf("a listing with d28c on side landing, the screen is\n%s\nwant its re-patch done, its cord hanging loose from a jack of work's", h.view())
	}
}

func TestARePatchedCallKeepsItsRowAtTheNextLook(t *testing.T) {
	onSide := func(id string, assigned time.Duration) status.Session {
		s := sessionOn(id, opus, "side", 5*time.Second)
		s.Assignments[0].AssignedAt = past(assigned)
		return s
	}
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{sessionOn(idD28C, opus, "work", 10*time.Second), onSide(id7F3A, -time.Minute), onSide(id9E21, -time.Hour)}
	h.update(tea.WindowSizeMsg{Width: 160, Height: 40})
	h.start()
	h.keys(toSessions...)
	h.hear(
		told(router.StreamSent, "r1", "work", 0),
		alter(told(router.StreamLimited, "r1", "work", 0), func(e *router.StreamEvent) { e.Status = 429 }),
		alter(told(router.StreamMoved, "r1", "side", 0), func(e *router.StreamEvent) { e.From, e.To, e.Reason = "work", "side", "moved: work hit its limit" }),
		alter(told(router.StreamSent, "r1", "side", 0), func(e *router.StreamEvent) { e.Attempt = 2 }),
	)
	h.tickUntil(past(pulseFor + time.Second))

	want := []string{"7f3a", "9e21", "d28c"}
	if got := callsRun(h.view(), "7f3a", "9e21", "d28c"); !slices.Equal(got, want) {
		t.Fatalf("d28c re-patched to side, side's calls run %q, want %q, d28c joining its foot", got, want)
	}
	h.source.sessions[0] = onSide(idD28C, 0)
	h.tickUntilAsked()
	if got := callsRun(h.view(), "7f3a", "9e21", "d28c"); !slices.Equal(got, want) {
		t.Errorf("the router listing d28c on side, put there last, side's calls run %q, want %q, each on its row", got, want)
	}
}

// callsRun are the ids given of the calls of opus on the Sessions screen, as
// its rows run, top to bottom.
func callsRun(screen string, ids ...string) []string {
	var run []string
	for row := range strings.SplitSeq(screen, "\n") {
		for _, id := range ids {
			if strings.HasPrefix(row, "  "+id+"  opus") {
				run = append(run, id)
			}
		}
	}
	return run
}
