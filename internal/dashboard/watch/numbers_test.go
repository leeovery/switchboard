package watch

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// The numbers the tests watch brighten, by the number each is.
var (
	workSessionUsed      = dashboard.Number{Kind: dashboard.NumberUsed, Account: "work", Of: "5h"}
	workSessionAllowance = dashboard.Number{Kind: dashboard.NumberAllowance, Account: "work", Of: "5h"}
	poolSessionRoom      = dashboard.Number{Kind: dashboard.NumberRoom, Of: "5h"}
	workSessions         = dashboard.Number{Kind: dashboard.NumberSessions, Account: "work"}
	workRate             = dashboard.Number{Kind: dashboard.NumberRate, Account: "work"}
	poolOpen             = dashboard.Number{Kind: dashboard.NumberStates, Of: "open"}
	routedSessions       = dashboard.Number{Kind: dashboard.NumberRouted}
	d28cRate             = dashboard.Number{Kind: dashboard.NumberRate, Account: "work", Of: idD28C}
)

// brightened are the numbers brightening at now, by the number each is.
func (h *harness) brightened() map[dashboard.Number]float64 {
	return h.model.numbers.brightened(h.clock.now)
}

// drawnNumber is the number given as the page draws it at now: "" where it
// draws none.
func (h *harness) drawnNumber(n dashboard.Number) string {
	return h.model.numbersDrawn(h.clock.now)[n]
}

// routedThree is the router's document of three, three sessions routed in
// the last hour, changed as change says, with what's worked out of it, as
// the source gives it.
func routedThree(change func(*status.Document)) status.Document {
	doc := routerDocument(three()...)
	doc.Sessions = 3
	change(&doc)
	return doc.WorkedOut(policy)
}

func TestTheFirstLookBrightensNoNumber(t *testing.T) {
	h := routedHarness(t, routedThree(func(*status.Document) {}))
	h.start()

	if got := h.brightened(); len(got) > 0 {
		t.Errorf("the first look brightened %v, want none: there's nothing before it to change from", got)
	}
}

func TestNumbersReadElsewhereThanTheLastLookStartAfresh(t *testing.T) {
	started := start.Add(-time.Hour).UTC()
	moved := routedThree(func(d *status.Document) { d.Accounts[0].Windows[0].Utilization = 0.3 })
	readTheRouter := func(h *harness) {
		h.startRouter(moved)
		h.tickUntilAsked()
	}
	tests := []struct {
		name      string
		before    func(h *harness)
		to        func(h *harness)
		brightens bool
	}{
		{name: "the same router", to: readTheRouter, brightens: true},
		{
			name: "another router, as one restarted",
			to: func(h *harness) {
				h.source.health = router.Health{PID: 8, StartedAt: started}
				readTheRouter(h)
			},
		},
		{
			name: "probing after the router",
			to: func(h *harness) {
				h.stopRouter()
				h.source.doc = probedWithoutTheRouter()
				h.source.doc.Accounts[0].Windows[0].Utilization = 0.3
				h.deliver(h.press("R")...)
			},
		},
		{name: "the router after probing", before: (*harness).stopRouter, to: readTheRouter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routedThree(func(*status.Document) {}))
			h.source.health = router.Health{PID: 7, StartedAt: started}
			if tt.before != nil {
				tt.before(h)
			}
			h.start()
			was := h.drawnNumber(workSessionUsed)

			tt.to(h)
			if is := h.drawnNumber(workSessionUsed); was == "" || is == "" || is == was {
				t.Fatalf("work's session is drawn %q, where it was drawn %q, want it drawn, and changed", is, was)
			}
			got := h.brightened()
			if tt.brightens && got[workSessionUsed] != 1 {
				t.Errorf("brightened %v, want work's session at 1", got)
			}
			if !tt.brightens && len(got) > 0 {
				t.Errorf("brightened %v, want none: the numbers start afresh", got)
			}
		})
	}
}

func TestANumberALookChangesBrightensFadingBackOverASecond(t *testing.T) {
	workUsing := func(used float64) func(*status.Document) {
		return func(d *status.Document) { d.Accounts[0].Windows[0].Utilization = used }
	}
	tests := []struct {
		name   string
		by     dashboard.By
		change func(*status.Document)
		number dashboard.Number
	}{
		{name: "a window's percentage", change: workUsing(0.3), number: workSessionUsed},
		{name: "the allowance its words lead with", change: workUsing(0.3), number: workSessionAllowance},
		{name: "the pool's room", change: workUsing(0.45), number: poolSessionRoom},
		{name: "ROUTING's sessions on an account", change: func(d *status.Document) { d.Accounts[0].Sessions = 2 }, number: workSessions},
		{name: "the status line's sessions", change: func(d *status.Document) { d.Sessions = 4 }, number: routedSessions},
		{
			name: "by account, the pool's states counted", by: dashboard.ByAccount,
			change: func(d *status.Document) { d.Accounts[0].Limit = limitedWork().Accounts[0].Limit }, number: poolOpen,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routedThree(func(*status.Document) {}))
			h.model.by = tt.by
			h.start()
			was := h.drawnNumber(tt.number)
			h.startRouter(routedThree(tt.change))
			seen := h.tickUntilAsked()
			if is := h.drawnNumber(tt.number); was == "" || is == "" || is == was {
				t.Fatalf("the look drew %q, where it drew %q before, want the number drawn, and changed", is, was)
			}

			for _, at := range []struct {
				after time.Duration
				want  float64
			}{
				{after: 0, want: 1},
				{after: brightFor / 2, want: 0.875},
			} {
				if got := h.model.numbers.brightened(seen.Add(at.after))[tt.number]; got != at.want {
					t.Errorf("%s after the look, it's %v bright, want %v", at.after, got, at.want)
				}
			}
			if got, ok := h.model.numbers.brightened(seen.Add(brightFor))[tt.number]; ok {
				t.Errorf("a second after the look, it's %v bright, want it faded back", got)
			}
			if got := h.model.frame(seen).Brightened[tt.number]; got != 1 {
				t.Errorf("the frame drawn as the look sees it has it %v bright, want 1", got)
			}
		})
	}
}

// sessionsStreaming is a model streaming the router's requests, SESSIONS
// shown, on a terminal tall enough to draw it.
func sessionsStreaming(t *testing.T) *harness {
	t.Helper()
	h := streamingOn(t, dashboard.Sections{}.Toggled(dashboard.SectionSessions))
	h.update(tea.WindowSizeMsg{Width: 160, Height: 80})
	return h
}

func TestANumberTheStreamChangesBrightens(t *testing.T) {
	routing := func(t *testing.T) *harness { return streamingOn(t, dashboard.Sections{}) }
	sent := told(router.StreamSent, "r1", "work", 0)
	moved := alter(told(router.StreamMoved, "r1", "side", 0), func(e *router.StreamEvent) { e.From, e.To, e.Reason = "work", "side", "pinned" })
	tests := []struct {
		name    string
		harness func(t *testing.T) *harness
		told    router.StreamEvent
		numbers []dashboard.Number
	}{
		{name: "ROUTING's rate, as a request goes out", harness: routing, told: sent, numbers: []dashboard.Number{workRate}},
		{name: "SESSIONS' rate, as its session's request goes out, and its account's", harness: sessionsStreaming, told: sent, numbers: []dashboard.Number{d28cRate, workRate}},
		{
			name: "ROUTING's sessions, as a session moves", harness: routing, told: moved,
			numbers: []dashboard.Number{workSessions, {Kind: dashboard.NumberSessions, Account: "side"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.harness(t)
			h.hear(tt.told)
			for _, n := range tt.numbers {
				if got := h.brightened()[n]; got != 1 {
					t.Errorf("%+v, %q, is %v bright, want 1", n, h.drawnNumber(n), got)
				}
			}
		})
	}
}

// What the stream tells as the watch joins it, the requests in flight then,
// is part of the join's first look, as today's lines read for the traces
// are: it brightens nothing, a request sent after it brightening as any.
func TestWhatAJoinTellsBrightensNothing(t *testing.T) {
	inFlight := func(request, account string) router.StreamEvent {
		return alter(told(router.StreamInFlight, request, account, -10*time.Second), func(e *router.StreamEvent) { e.SentAt = e.At })
	}
	joining := []router.StreamEvent{inFlight("r1", "work"), inFlight("r2", "side")}
	sent := told(router.StreamSent, "r3", "work", 0)
	sideRate := dashboard.Number{Kind: dashboard.NumberRate, Account: "side"}
	tests := []struct {
		name     string
		rejoined bool
		heard    [][]router.StreamEvent
		want     []dashboard.Number
	}{
		{name: "the requests in flight as it opens", heard: [][]router.StreamEvent{joining}},
		{name: "the requests in flight as it's joined again", rejoined: true, heard: [][]router.StreamEvent{joining}},
		{name: "a request sent after them", heard: [][]router.StreamEvent{joining, {sent}}, want: []dashboard.Number{workRate}},
		{name: "a request sent behind them, as they're told", heard: [][]router.StreamEvent{append(slices.Clone(joining), sent)}, want: []dashboard.Number{workRate}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := streamingOn(t, dashboard.Sections{})
			h.settle()
			if tt.rejoined {
				h.endStream()
				rejoin, ok := h.pendingRejoin()
				if !ok {
					t.Fatal("once the stream ended, no rejoin was armed")
				}
				h.fire(rejoin)
			}
			for _, events := range tt.heard {
				h.hear(events...)
			}
			if got := h.drawnNumber(sideRate); got != "1/min" {
				t.Fatalf("side's rate reads %q, want 1/min, the request in flight there counted", got)
			}
			if got := slices.Collect(maps.Keys(h.brightened())); !sameNumbers(got, tt.want) {
				t.Errorf("brightened %+v, want %+v", got, tt.want)
			}
		})
	}
}

// The page is drawn twice to compare its numbers, for what the stream tells,
// only where that can change one: a request sent, or a seat moved, on a page
// that draws what they change, as StreamNumbered says; never Sessions',
// whose switchboard the stream is busiest on.
func TestTheStreamHasNumbersComparedOnlyWhereThePageDrawsOneItChanges(t *testing.T) {
	sent := told(router.StreamSent, "r1", "work", 0)
	moved := alter(told(router.StreamMoved, "r1", "side", 0), func(e *router.StreamEvent) { e.From, e.To, e.Reason = "work", "side", "pinned" })
	progress := alter(told(router.StreamProgress, "r1", "work", 0), func(e *router.StreamEvent) { e.Chars = 4800 })
	sessionsAlone := dashboard.Sections{}.Toggled(dashboard.SectionRouting).Toggled(dashboard.SectionSessions)
	tests := []struct {
		name     string
		view     dashboard.View
		sections dashboard.Sections
		told     router.StreamEvent
		want     bool
	}{
		{name: "a request sent, ROUTING shown", view: dashboard.Overview, told: sent, want: true},
		{name: "a seat moved, ROUTING shown", view: dashboard.Overview, told: moved, want: true},
		{name: "a request sent, SESSIONS shown alone", view: dashboard.Overview, sections: sessionsAlone, told: sent, want: true},
		{name: "an answer's progress, ROUTING shown", view: dashboard.Overview, told: progress},
		{name: "a request sent, on Sessions", view: dashboard.Sessions, told: sent},
		{name: "a seat moved, on Sessions", view: dashboard.Sessions, told: moved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := streamingOn(t, tt.sections)
			h.model.view = tt.view
			if got := h.model.numbering([]router.StreamEvent{tt.told}); got != tt.want {
				t.Errorf("the numbers are compared: %t, want %t", got, tt.want)
			}
		})
	}
}

// sameNumbers reports whether got and want hold the same numbers, in any
// order.
func sameNumbers(got, want []dashboard.Number) bool {
	return len(got) == len(want) && !slices.ContainsFunc(want, func(n dashboard.Number) bool { return !slices.Contains(got, n) })
}

// An answer's tokens count up as it streams, told every quarter second, as
// the clock's numbers move: they'd hold frames running for as long as
// anything streams, and say nothing ↓ doesn't.
func TestAnAnswersTokensNeverBrighten(t *testing.T) {
	h := sessionsStreaming(t)
	progress := func(chars int) router.StreamEvent {
		return alter(told(router.StreamProgress, "r1", "work", 0), func(e *router.StreamEvent) { e.Chars = chars })
	}
	h.hear(told(router.StreamSent, "r1", "work", 0), told(router.StreamFirst, "r1", "work", 0), progress(4800))
	h.settle()

	h.hear(progress(6000))
	if !strings.Contains(h.view(), "↓ ~1.5k") {
		t.Fatalf("the screen is\n%s\nwant d28c's answer counted on, ↓ ~1.5k", h.view())
	}
	if got := h.brightened(); len(got) > 0 {
		t.Errorf("more of the answer streaming brightened %v, want none", got)
	}
	if _, framing := h.pendingFrame(); framing {
		t.Error("more of the answer streaming armed a frame, want none")
	}
}

func TestWhatTheClockMovesNeverBrightens(t *testing.T) {
	tests := []struct {
		name          string
		harness       func(t *testing.T) *harness
		number        dashboard.Number
		before, after time.Duration
	}{
		{
			name: "a rate, as its request's minute passes",
			harness: func(t *testing.T) *harness {
				h := streamingOn(t, dashboard.Sections{})
				h.hear(told(router.StreamSent, "r1", "work", 0))
				return h
			},
			number: workRate, before: 30 * time.Second, after: 70 * time.Second,
		},
		{
			name: "a window's percentage, as it resets",
			harness: func(t *testing.T) *harness {
				h := newHarness(t, document(account("work", "Work", session(0.25, 10*time.Second), week(0.5)), three()[1]))
				h.start()
				return h
			},
			number: workSessionUsed, before: 5 * time.Second, after: 2 * time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.harness(t)
			h.tickUntil(past(tt.before))
			was := h.drawnNumber(tt.number)
			for h.clock.now.Before(past(tt.after)) {
				h.fire(h.lastTick())
				if got := h.brightened(); len(got) > 0 {
					t.Fatalf("at %s, %v brightened, want none", h.clock.now.Format(time.TimeOnly), got)
				}
			}
			if is := h.drawnNumber(tt.number); is == was {
				t.Errorf("the clock left it as it was, %q, want it moved on", is)
			}
		})
	}
}

func TestANumberAppearingOrGoingNeverBrightens(t *testing.T) {
	tests := []struct {
		name          string
		before, after func(*status.Document)
		number        func(dashboard.Number) bool
	}{
		{
			name:   "an account joining, its numbers appearing",
			before: func(*status.Document) {},
			after:  func(d *status.Document) { d.Accounts = four().Accounts },
			number: func(n dashboard.Number) bool { return n.Account == "client" },
		},
		{
			name:   "the status line's sessions appearing, where it had none",
			before: func(d *status.Document) { d.Sessions = 0 },
			after:  func(*status.Document) {},
			number: func(n dashboard.Number) bool { return n == routedSessions },
		},
		{
			name:   "a window's percentage going, its account's token gone",
			before: func(*status.Document) {},
			after:  func(d *status.Document) { d.Accounts[0].TokenSet = false },
			number: func(n dashboard.Number) bool { return n == workSessionUsed },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routedThree(tt.before))
			h.start()
			was := h.model.numbersDrawn(h.clock.now)
			h.startRouter(routedThree(tt.after))
			h.tickUntilAsked()
			is := h.model.numbersDrawn(h.clock.now)
			if slices.ContainsFunc(slices.Collect(maps.Keys(was)), tt.number) == slices.ContainsFunc(slices.Collect(maps.Keys(is)), tt.number) {
				t.Fatal("it's drawn as it was, want it appearing or going")
			}
			for n := range h.brightened() {
				if tt.number(n) {
					t.Errorf("%+v brightened, want it not: it wasn't there before and after", n)
				}
			}
		})
	}
}

func TestFramesAreDrawnWhileANumberBrightensOnScreenAlone(t *testing.T) {
	moreSessions := routedThree(func(d *status.Document) { d.Sessions = 4 })
	tests := []struct {
		name   string
		keys   []string
		after  status.Document
		frames bool
	}{
		{name: "a look changing nothing", after: routedThree(func(*status.Document) {})},
		{name: "a look changing the status line's sessions", after: moreSessions, frames: true},
		{name: "a look changing them under help's card", keys: []string{"?"}, after: moreSessions},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routedThree(func(*status.Document) {}))
			h.start()
			h.settle()
			for _, key := range tt.keys {
				h.deliver(h.press(key)...)
			}
			h.startRouter(tt.after)
			seen := h.tickUntilAsked()
			frame, framing := h.pendingFrame()
			if framing != tt.frames {
				t.Fatalf("a frame is armed: %t, want %t", framing, tt.frames)
			}
			if !framing {
				return
			}
			if frame.delay != frameEvery {
				t.Errorf("the next frame is armed %s on, want %s", frame.delay, frameEvery)
			}
			h.framesUntil(seen.Add(brightFor + frameEvery))
			if _, framing := h.pendingFrame(); framing {
				t.Error("a frame is armed once the sessions have faded back, want none")
			}
		})
	}
}
