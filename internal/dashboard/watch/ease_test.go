package watch

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

func TestEaseOutCubic(t *testing.T) {
	tests := []struct {
		t, want float64
	}{
		{t: 0, want: 0},
		{t: 0.25, want: 0.578125},
		{t: 0.5, want: 0.875},
		{t: 1, want: 1},
	}
	for _, tt := range tests {
		if got := easeOutCubic(tt.t); got != tt.want {
			t.Errorf("easeOutCubic(%v) = %v, want %v", tt.t, got, tt.want)
		}
	}
}

func TestProgress(t *testing.T) {
	tests := []struct {
		name    string
		elapsed time.Duration
		want    float64
	}{
		{name: "before the start", elapsed: -time.Millisecond, want: 0},
		{name: "at the start", elapsed: 0, want: 0},
		{name: "half way", elapsed: 300 * time.Millisecond, want: 0.5},
		{name: "at the end", elapsed: 600 * time.Millisecond, want: 1},
		{name: "after the end", elapsed: time.Second, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := progress(tt.elapsed, 600*time.Millisecond); got != tt.want {
				t.Errorf("progress(%v, 600ms) = %v, want %v", tt.elapsed, got, tt.want)
			}
		})
	}
}

func TestLerp(t *testing.T) {
	tests := []struct {
		t, want float64
	}{
		{t: 0, want: 0.1},
		{t: 0.5, want: 0.2},
		{t: 1, want: 0.3},
	}
	for _, tt := range tests {
		if got := lerp(0.1, 0.3, tt.t); got != tt.want {
			t.Errorf("lerp(0.1, 0.3, %v) = %v, want %v", tt.t, got, tt.want)
		}
	}
}

func TestBarsRiseFromNothingOnTheFirstRead(t *testing.T) {
	h := easingHarness(t, sessionAndWeek(0.5, 0.25))

	tests := []struct {
		name          string
		after         time.Duration
		session, week float64
	}{
		{name: "at the start", after: 0, session: 0, week: 0},
		{name: "half way through", after: easeFor / 2, session: 0.4375, week: 0.21875},
		{name: "at the end", after: easeFor, session: 0.5, week: 0.25},
		{name: "after the end", after: 2 * easeFor, session: 0.5, week: 0.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h.clock.now = start.Add(tt.after)
			checkScreen(t, h, tt.session, tt.week)
			if view := h.view(); !strings.Contains(view, "  25%  → 44%") {
				t.Errorf("the screen is\n%s\nwant the room left as read, the bars alone easing", view)
			}
		})
	}
}

func TestBarsEaseFromWhereTheyStand(t *testing.T) {
	h := easingHarness(t, sessionAndWeek(0.5, 0.25))
	h.settle()

	h.clock.now = at(13, 20, 0)
	h.read(sessionAndWeek(0.75, 0.25))
	checkScreen(t, h, 0.5, 0.25)
	h.clock.now = at(13, 20, 0).Add(easeFor / 2)
	checkScreen(t, h, 0.71875, 0.25)

	// A read arriving mid-way sets off again from the bars as drawn.
	h.read(sessionAndWeek(0.25, 0.25))
	checkScreen(t, h, 0.71875, 0.25)
	h.clock.now = h.clock.now.Add(easeFor)
	checkScreen(t, h, 0.25, 0.25)
}

func TestFramesRunOnlyWhileBarsMove(t *testing.T) {
	h := newHarness(t, sessionAndWeek(0.5, 0.25))
	h.start()

	frames := 0
	for tm, ok := h.pendingFrame(); ok; tm, ok = h.pendingFrame() {
		if tm.delay != frameEvery {
			t.Errorf("frame %d came %v after the last, want %v", frames+1, tm.delay, frameEvery)
		}
		if frames > 0 && !tm.due.Add(-frameEvery).Before(start.Add(easeFor)) {
			t.Errorf("frame %d came after the frame that finished the easing", frames+1)
		}
		h.fire(tm)
		frames++
	}
	if want := int(easeFor / frameEvery); frames < want-3 || frames > want+2 {
		t.Errorf("the easing took %d frames, want about 30 a second over %v", frames, easeFor)
	}

	h.read(sessionAndWeek(0.5, 0.25))
	if _, ok := h.pendingFrame(); ok {
		t.Error("a read that moved no bar started frames")
	}

	h.read(sessionAndWeek(0.75, 0.25))
	h.read(sessionAndWeek(0.25, 0.25))
	pending := 0
	for _, tm := range h.timers {
		if _, ok := tm.msg.(frameMsg); ok && !tm.fired {
			pending++
		}
	}
	if pending != 1 {
		t.Errorf("two reads that moved bars left %d frames pending, want one run of frames", pending)
	}
}

// tenIDs are the ids of ten accounts, too many for USAGE alone on a
// terminal 30 rows tall: ops and studio are scrolled out of view.
var tenIDs = []string{"work", "personal", "side", "client", "spare", "lab", "team", "extra", "ops", "studio"}

// tenWith is the router's document of ten accounts, each using a quarter of
// its session and half its week, those with the ids given changed as change
// says; the pin keeps work and studio out of the pool, so their changes move
// nothing on screen but their own.
func tenWith(change func(*status.Account), ids ...string) status.Document {
	var accounts []status.Account
	for _, a := range tenIDs {
		accounts = append(accounts, account(a, a, session(0.25, 3*time.Hour), week(0.5)))
	}
	for _, id := range ids {
		change(&accounts[slices.Index(tenIDs, id)])
	}
	doc := routerDocument(accounts...)
	doc.Pin = status.Pin{Accounts: slices.DeleteFunc(slices.Clone(tenIDs), func(a string) bool { return a == "work" || a == "studio" })}
	return doc.WorkedOut(policy)
}

// tenOnAShortScreen is a model of a source whose router answers with doc,
// of tenWith's ten accounts, showing USAGE alone on a terminal 30 rows tall,
// started and settled, studio scrolled out of view.
func tenOnAShortScreen(t *testing.T, doc status.Document) *harness {
	t.Helper()
	h := routedHarness(t, doc)
	h.model.sections = dashboard.Sections{}.Toggled(dashboard.SectionRouting).Toggled(dashboard.SectionComing).Toggled(dashboard.SectionLately)
	h.update(tea.WindowSizeMsg{Width: 160, Height: 30})
	h.start()
	h.settle()
	if view := h.view(); strings.Contains(view, "10 studio") {
		t.Fatalf("the screen is\n%s\nwant USAGE too short for studio, scrolled out of view", view)
	}
	return h
}

func TestFramesRunOnlyWhileBarsOnScreenMove(t *testing.T) {
	using := func(a *status.Account) { a.Windows[0].Utilization = 0.75 }
	h := tenOnAShortScreen(t, tenWith(using))
	for _, read := range []struct {
		name   string
		moved  []string
		bar    string
		frames bool
	}{
		{name: "a read moving a bar scrolled out of view", moved: []string{"studio"}, bar: "studio"},
		{name: "a read moving a bar on screen", moved: []string{"studio", "work"}, bar: "work", frames: true},
	} {
		h.startRouter(tenWith(using, read.moved...))
		seen := h.tickUntilAsked()
		if _, ok := h.model.ease.eased(h.model.doc, seen)[dashboard.Ref{Account: read.bar, Window: "5h"}]; !ok {
			t.Fatalf("%s: %s's session bar isn't easing, want the read to have moved it", read.name, read.bar)
		}
		if _, framing := h.pendingFrame(); framing != read.frames {
			t.Errorf("%s: a frame is armed: %t, want %t", read.name, framing, read.frames)
		}
	}
}

func TestFramesRunOnWhereTheClockReadsALittleBehindTheirTimers(t *testing.T) {
	h := easingHarness(t, sessionAndWeek(0.5, 0.25))

	const lag = 30 * time.Microsecond
	for frames := 0; ; frames++ {
		tm, ok := h.pendingFrame()
		if !ok {
			break
		}
		if frames > 100 {
			t.Fatal("frames ran on past 100, want them to stop once the bars have eased")
		}
		h.fireBehind(tm, lag)
	}
	if h.clock.now.Before(start.Add(easeFor)) {
		t.Errorf("frames stopped at %s, before the bars had eased, at %s", h.clock.now.Format(time.StampMicro), start.Add(easeFor).Format(time.StampMicro))
	}
	checkScreen(t, h, 0.5, 0.25)
}

func TestEasingLeavesTheDocumentAlone(t *testing.T) {
	doc := sessionAndWeek(0.5, 0.25)
	h := newHarness(t, doc)
	h.start()

	h.clock.now = start.Add(easeFor / 2)
	h.model.View()
	if got := doc.Accounts[0].Windows[0].Utilization; got != 0.5 {
		t.Errorf("drawing the bars part way changed the document's session to %v, want 0.5", got)
	}
}

// easingHarness is a model of a source that reads doc, showing Sessions,
// whose panels draw the bars as they ease, started.
func easingHarness(t *testing.T, doc status.Document) *harness {
	t.Helper()
	h := newHarness(t, doc)
	h.model.view = dashboard.Sessions
	h.start()
	return h
}

// sessionAndWeek is an account using session of its session and week of its
// week: fractions a float64 holds exactly, eased or not, so frames compare
// exactly.
func sessionAndWeek(sessionUsed, weekUsed float64) status.Document {
	return document(account("work", "Work", session(sessionUsed, 3*time.Hour), week(weekUsed)))
}

// checkScreen checks that the screen draws the document read, its bars alone
// eased, filled to session of its session, and week of its week.
func checkScreen(t *testing.T, h *harness, session, week float64) {
	t.Helper()
	now := h.model.now()
	f := h.model.live(now)
	f.Eased = map[dashboard.Ref]float64{{Account: "work", Window: "5h"}: session, {Account: "work", Window: "7d"}: week}
	if got, frame := h.model.View().Content, strings.Join(f.Draw(h.model.doc, now), "\n"); got != frame {
		t.Errorf("at %s the screen is\n%s\nwant\n%s", h.clock.now.Format(time.StampMilli), got, frame)
	}
}
