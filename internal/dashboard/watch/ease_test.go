package watch

import (
	"testing"
	"time"

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
	h := newHarness(t, sessionAndWeek(0.5, 0.25))
	h.start()

	tests := []struct {
		name          string
		after         time.Duration
		session, week float64
	}{
		{name: "at the start", after: 0, session: 0, week: 0},
		{name: "half way through", after: 300 * time.Millisecond, session: 0.4375, week: 0.21875},
		{name: "at the end", after: easeFor, session: 0.5, week: 0.25},
		{name: "after the end", after: time.Second, session: 0.5, week: 0.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h.clock.now = start.Add(tt.after)
			checkScreen(t, h, sessionAndWeek(tt.session, tt.week))
		})
	}
}

func TestBarsEaseFromWhereTheyStand(t *testing.T) {
	h := newHarness(t, sessionAndWeek(0.5, 0.25))
	h.start()
	h.settle()

	h.clock.now = at(13, 20, 0)
	h.read(sessionAndWeek(0.75, 0.25))
	checkScreen(t, h, sessionAndWeek(0.5, 0.25))
	h.clock.now = at(13, 20, 0).Add(300 * time.Millisecond)
	checkScreen(t, h, sessionAndWeek(0.71875, 0.25))

	// A read arriving mid-way sets off again from the bars as drawn.
	h.read(sessionAndWeek(0.25, 0.25))
	checkScreen(t, h, sessionAndWeek(0.71875, 0.25))
	h.clock.now = h.clock.now.Add(easeFor)
	checkScreen(t, h, sessionAndWeek(0.25, 0.25))
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
	if frames < 15 || frames > 20 {
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

func TestEasingLeavesTheDocumentAlone(t *testing.T) {
	doc := sessionAndWeek(0.5, 0.25)
	h := newHarness(t, doc)
	h.start()

	h.clock.now = start.Add(300 * time.Millisecond)
	h.model.View()
	if got := doc.Accounts[0].Windows[0].Utilization; got != 0.5 {
		t.Errorf("drawing the bars part way changed the document's session to %v, want 0.5", got)
	}
}

// sessionAndWeek is an account using session of its session and week of its
// week: fractions a float64 holds exactly, eased or not, so frames compare
// exactly.
func sessionAndWeek(sessionUsed, weekUsed float64) status.Document {
	return document(account("work", "Work", session(sessionUsed, 3*time.Hour), week(weekUsed)))
}

// checkScreen checks that the screen draws want.
func checkScreen(t *testing.T, h *harness, want status.Document) {
	t.Helper()
	frame := "\n" + dashboard.Render(want, h.clock.now, dashboard.Options{Width: 150, Height: 49, Color: true, Footer: h.model.footer(h.clock.now)})
	if got := h.model.View().Content; got != frame {
		t.Errorf("at %s the screen is\n%s\nwant\n%s", h.clock.now.Format(time.StampMilli), got, frame)
	}
}
