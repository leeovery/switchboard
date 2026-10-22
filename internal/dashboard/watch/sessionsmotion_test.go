package watch

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/theme"
)

// settledOnSessions is sessionsOnHarness, its bars eased, no frame armed.
func settledOnSessions(t *testing.T) *harness {
	t.Helper()
	h, _ := sessionsOnHarness(t)
	h.settle()
	return h
}

// sinceStart is how long past when the clock started it now reads, for the
// stream to tell of what happens now.
func (h *harness) sinceStart() time.Duration {
	return h.clock.now.Sub(start)
}

// frameDue is when the frame armed is due, failing the test where none is.
func (h *harness) frameDue(why string) time.Time {
	h.t.Helper()
	tm, ok := h.pendingFrame()
	if !ok {
		h.t.Fatalf("%s, no frame is armed, want one", why)
	}
	return tm.due
}

// noFrame fails the test where a frame is armed.
func (h *harness) noFrame(why string) {
	h.t.Helper()
	if tm, ok := h.pendingFrame(); ok {
		h.t.Errorf("%s, a frame is armed for %s, want none", why, tm.due.Format(time.StampMilli))
	}
}

// While a pulse runs along a cord Sessions draws, a frame is drawn every
// frameEvery, until the pulse is home; and none while it runs on the Log,
// which reads the stream, or under a session's page.
func TestAPulseAlongACordDrawsFramesOnSessionsAlone(t *testing.T) {
	h := settledOnSessions(t)
	sent := h.sinceStart()
	h.hear(told(router.StreamSent, "r1", "work", sent))
	if due := h.frameDue("d28c's request sent"); !due.Equal(h.clock.now.Add(frameEvery)) {
		t.Fatalf("d28c's request sent, a frame is armed for %s, want one frameEvery on, %s", due.Format(time.StampMilli), h.clock.now.Add(frameEvery).Format(time.StampMilli))
	}
	h.framesUntil(past(sent + pulseFor - frameEvery))
	h.frameDue("its pulse yet to reach the jack")
	h.framesUntil(past(sent + pulseFor + frameEvery))
	h.noFrame("its pulse home, d28c asking")

	others := []struct {
		name string
		on   func(h *harness)
	}{
		{name: "on the Log", on: func(h *harness) { h.model.view = dashboard.Log }},
		{name: "under d28c's page", on: func(h *harness) { h.openPage(idD28C) }},
	}
	for _, tt := range others {
		t.Run(tt.name, func(t *testing.T) {
			h := settledOnSessions(t)
			tt.on(h)
			h.settle()
			if !h.listening() {
				t.Fatal("the stream isn't read")
			}
			h.hear(told(router.StreamSent, "r1", "work", h.sinceStart()))
			h.noFrame("d28c's request sent")
		})
	}
}

// While an answer streams back, a frame is drawn a step at a time, as the
// clock passes each whole shimmerStep, just past it, and not frameEvery.
func TestAShimmerDrawsAFrameAStepAtATime(t *testing.T) {
	h := settledOnSessions(t)
	sent := h.sinceStart()
	h.clock.now = past(sent + pulseFor)
	h.hear(told(router.StreamSent, "r1", "work", sent), told(router.StreamFirst, "r1", "work", sent+pulseFor))

	for step := range 3 {
		now := h.clock.now
		want := now.Add(untilStep(now, shimmerStep) + stepSlack)
		due := h.frameDue("d28c's answer streaming")
		if !due.Equal(want) {
			t.Fatalf("frame %d: at %s, a frame is armed for %s, want %s, just past the clock's next step", step, now.Format(time.StampMilli), due.Format(time.StampMilli), want.Format(time.StampMilli))
		}
		tm, _ := h.pendingFrame()
		h.fire(tm)
	}
}

// A cord a move no limit forced lets go hangs loose, drawing frames as it
// fades over looseFor, and none once it's gone: in a look that can't fade
// it, gone halfway.
func TestALooseCordDrawsFramesWhileItFades(t *testing.T) {
	tests := []struct {
		name     string
		coloured bool
		gone     time.Duration
	}{
		{name: "in a theme", coloured: true, gone: looseFor},
		{name: "without colour", gone: looseFor / 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := settledOnSessions(t)
			if tt.coloured {
				h.model.cfg.Themes = &fakeThemes{}
				h.answer(darkBackground)
				h.model.showing, _ = theme.Builtin("nord")
			}
			moved := h.sinceStart()
			h.hear(pinnedToSide(moved))

			h.framesUntil(past(moved + tt.gone - frameEvery))
			h.frameDue("d28c's cord hanging from work, fading")
			h.framesUntil(past(moved + tt.gone + frameEvery))
			h.noFrame("its cord gone")
		})
	}
}

// No frame is drawn under help, though a pulse runs along a cord behind it;
// help closed, the frames go on.
func TestNoFrameIsDrawnUnderHelp(t *testing.T) {
	h := settledOnSessions(t)
	h.keyed("?")
	if h.model.open != helpOpen {
		t.Fatal("? didn't open help")
	}
	h.hear(told(router.StreamSent, "r1", "work", h.sinceStart()))
	h.noFrame("help open, d28c's request sent")

	h.keyed("esc")
	h.frameDue("help closed, d28c's pulse running")
}

// boardOfTwelve is a model of the router of twelve, a session on each, d28c
// on a, the first, and 7f3a on l, the last, on Sessions' Switchboard in a
// terminal 160 columns wide and 18 tall, where it takes step 5, scrolling an
// account at a time, each with its calls, at its top.
func boardOfTwelve(t *testing.T) *harness {
	t.Helper()
	accounts := twelve().Accounts
	h := routedHarness(t, routerDocument(accounts...))
	for i, a := range accounts {
		id := fmt.Sprintf("%04x0000-5e55-4000-8000-000000000000", i+1)
		switch a.ID {
		case "a":
			id = idD28C
		case "l":
			id = id7F3A
		}
		h.source.sessions = append(h.source.sessions, sessionOn(id, opus, a.ID, time.Minute))
	}
	h.update(tea.WindowSizeMsg{Width: 160, Height: 18})
	h.start()
	h.keys(toSessions...)
	h.settle()
	return h
}

// most is as far as the page on screen scrolls.
func (h *harness) most() int {
	f, doc, at := h.model.screen(h.clock.now)
	return f.Scrolling(doc, at).Most
}

// A cord scrolled out of view draws no frame while a pulse runs along it;
// scrolled into view, it does.
func TestACordScrolledOutOfViewDrawsNoFrames(t *testing.T) {
	h := boardOfTwelve(t)
	if h.most() == 0 {
		t.Fatal("the Switchboard of twelve at 160 by 18 doesn't scroll")
	}
	h.hear(alter(told(router.StreamSent, "r1", "l", h.sinceStart()), func(e *router.StreamEvent) { e.Session = id7F3A }))
	h.noFrame("7f3a's request sent, l's line scrolled out of view")

	h.keys(tea.KeyPressMsg{Code: tea.KeyPgDown}, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if h.model.scroll != h.most() {
		t.Fatalf("PgDn twice scrolled to %d, want %d, l's line in view", h.model.scroll, h.most())
	}
	h.frameDue("scrolled to l's line, 7f3a's pulse running")
}

// ↑ with nothing chosen chooses the last row, and the view scrolls as far as
// it goes to show it: on the Switchboard at step 5, and on a long List.
func TestUpFromNoneScrollsToTheLastRow(t *testing.T) {
	tests := []struct {
		name string
		on   func(t *testing.T) *harness
		keys string
		want dashboard.Plug
	}{
		{name: "the Switchboard at step 5", on: boardOfTwelve, keys: "up", want: dashboard.Plug{Session: id7F3A, Account: "l"}},
		{name: "a long List", on: func(t *testing.T) *harness {
			h := streamingHarness(t)
			h.withLines(endedToday(30))
			h.update(tea.WindowSizeMsg{Width: 160, Height: 20})
			h.keys(toSessions...)
			return h
		}, keys: "] up"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.on(t)
			if h.model.scroll != 0 {
				t.Fatalf("scrolled to %d at first, want the top", h.model.scroll)
			}
			h.keyed(tt.keys)
			chosen := h.model.sessionsTab.shown.Chosen
			if chosen == (dashboard.Plug{}) || tt.want != (dashboard.Plug{}) && chosen != tt.want {
				t.Fatalf("%s chose %+v, want the last row, %+v", tt.keys, chosen, tt.want)
			}
			if most := h.most(); most == 0 || h.model.scroll != most {
				t.Errorf("%s scrolled to %d, want %d, as far as it goes", tt.keys, h.model.scroll, most)
			}
		})
	}
}

// A click on the key line's `[ ] view` shows the next sub-tab, as ] does;
// and on its `enter its page`, the page of the session chosen, as enter
// does, or, with none chosen, says so, as enter does.
func TestAClickOnSessionsKeysActsAsTheKey(t *testing.T) {
	h := settledOnSessions(t)
	onKey := func(key string) func(dashboard.Target) bool {
		return func(t dashboard.Target) bool { return t.On == dashboard.OnKey && t.Key == key }
	}

	h.clickWhere(onKey("[ ]"))
	if shown := h.model.sessionsTab.shown.Shown; shown != dashboard.SessionsList {
		t.Errorf("[ ] view clicked shows sub-tab %d, want the List, %d", shown, dashboard.SessionsList)
	}
	h.keyed("[")

	h.clickWhere(onKey("enter"))
	if said := h.model.noted(h.clock.now); h.model.session != nil || said != noSessionChosen {
		t.Errorf("enter clicked with nothing chosen opened a page: %t, noted %q; want none, noted %q", h.model.session != nil, said, noSessionChosen)
	}
	h.clock.now = h.clock.now.Add(noteFor)
	h.redraw()
	h.keyed("down")
	chosen := h.model.sessionsTab.shown.Chosen.Session
	h.clickWhere(onKey("enter"))
	if h.model.session == nil || h.model.session.id != chosen {
		t.Errorf("enter clicked opened %+v, want the page of %s, the session chosen", h.model.session, chosen)
	}
}
