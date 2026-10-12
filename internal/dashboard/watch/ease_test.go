package watch

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
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
			h.redraw()
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
	h.redraw()
	checkScreen(t, h, 0.71875, 0.25)

	// A read arriving mid-way sets off again from the bars as drawn.
	h.read(sessionAndWeek(0.25, 0.25))
	checkScreen(t, h, 0.71875, 0.25)
	h.clock.now = h.clock.now.Add(easeFor)
	h.redraw()
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
		if _, ok := h.model.ease.eased(h.model.frame(seen), h.model.doc, seen)[dashboard.Ref{Account: read.bar, Window: "5h"}]; !ok {
			t.Fatalf("%s: %s's session bar isn't easing, want the read to have moved it", read.name, read.bar)
		}
		if _, framing := h.pendingFrame(); framing != read.frames {
			t.Errorf("%s: a frame is armed: %t, want %t", read.name, framing, read.frames)
		}
	}
}

// USAGE draws a window of an account without a usable token as an empty
// bar, whatever was read of it before: a read that empties it moves nothing
// on screen, so it eases nothing and starts no frames.
func TestAReadLeavingABarEmptyStartsNoFrames(t *testing.T) {
	using := func(a *status.Account) { a.Windows[0].Utilization = 0.75 }
	h := tenOnAShortScreen(t, tenWith(using))
	tokenless := func(a *status.Account) {
		using(a)
		a.TokenSet = false
	}
	h.startRouter(tenWith(tokenless, "work"))
	seen := h.tickUntilAsked()
	if got := h.model.ease.eased(h.model.frame(seen), h.model.doc, seen); len(got) > 0 {
		t.Errorf("the read eased %v, want no bar easing, work's left empty", got)
	}
	if _, framing := h.pendingFrame(); framing {
		t.Error("a read leaving work's bars empty started frames")
	}
}

// Sessions' panels draw a lapsed window as not started, with no bar: a read
// that changes its use, here as it lapses, moves nothing on screen, so it
// eases nothing and starts no frames.
func TestOnSessionsAReadChangingALapsedWindowsUseStartsNoFrames(t *testing.T) {
	h := easingHarness(t, sessionAndWeek(0.5, 0.25))
	h.settle()

	work := account("work", "Work", quota.Window{Key: "5h", Label: "Session"}, week(0.25))
	work.Lapsed = []string{"5h"}
	h.read(document(work))
	if !strings.Contains(h.view(), "not started") {
		t.Fatalf("the screen is\n%s\nwant work's session not started", h.view())
	}
	now := h.model.now()
	if got := h.model.ease.eased(h.model.frame(now), h.model.doc, now); len(got) > 0 {
		t.Errorf("the read eased %v, want no bar easing, work's session drawn without one", got)
	}
	if _, framing := h.pendingFrame(); framing {
		t.Error("a read changing the use of work's lapsed session started frames")
	}
}

// drawnAfresh is the screen as View draws it of the model as it stands,
// keeping nothing Update drew.
func (h *harness) drawnAfresh() string {
	m := h.model
	m.shown = shown{}
	return m.View().Content
}

// Where anything may settle, Update draws the screen once, to know what
// moves on it, and View shows that drawing rather than drawing it again: a
// frame among them, so a frame is drawn once. Where a frame is armed to come
// sooner, Update draws nothing, and View draws the screen.
func TestTheScreenUpdateDrawsIsTheScreenDrawnAfresh(t *testing.T) {
	using := func(a *status.Account) { a.Windows[0].Utilization = 0.75 }
	h, _ := themedHarness(t, theme.One("nord"))
	ten := tenWith(using)
	h.source.router = &ten
	h.model.sections = dashboard.Sections{}.Toggled(dashboard.SectionRouting).Toggled(dashboard.SectionComing).Toggled(dashboard.SectionLately)
	h.update(tea.WindowSizeMsg{Width: 160, Height: 30})
	h.start()
	h.answer(darkBackground)
	h.settle()
	if view := h.view(); strings.Contains(view, "10 studio") {
		t.Fatalf("the screen is\n%s\nwant USAGE too short for studio, scrolled out of view", view)
	}
	read := func(moved ...string) func() {
		return func() {
			h.startRouter(tenWith(using, moved...))
			h.tickUntilAsked()
		}
	}
	steps := []struct {
		name  string
		take  func()
		drawn bool
	}{
		{name: "a read moving a bar out of view", take: read("studio"), drawn: true},
		{name: "a key opening the theme picker", take: func() {
			if h.typed("t"); !h.model.picker.open {
				t.Fatal("t opened no picker")
			}
		}, drawn: true},
		{name: "a key in the picker", take: func() { h.typed("down") }, drawn: true},
		{name: "a read moving a bar on screen", take: read("studio", "work"), drawn: true},
		{name: "a frame", take: func() {
			tm, _ := h.pendingFrame()
			h.fire(tm)
		}, drawn: true},
		{name: "a key in the picker, a frame armed", take: func() { h.typed("down") }},
	}
	for _, step := range steps {
		step.take()
		if h.model.shown.drawn != step.drawn {
			t.Errorf("%s: Update drew the screen %t, want %t", step.name, h.model.shown.drawn, step.drawn)
		}
		if got, want := h.model.View().Content, h.drawnAfresh(); got != want {
			t.Errorf("%s: the screen is\n%s\nwant it as drawn afresh\n%s", step.name, ansi.Strip(got), ansi.Strip(want))
		}
	}
}

// Where Update draws nothing, View draws the live page Update built the
// sections of, at the moment it took the message in, so they're built once a
// message: the clock moving since moves nothing on screen till a message
// comes, as Bubble Tea draws after each.
func TestViewDrawsThePageAsTheLastMessageLeftIt(t *testing.T) {
	h := newHarness(t, calm())
	h.start()
	h.settle()
	h.redraw()
	if h.model.shown.drawn {
		t.Fatal("Update drew the screen with nothing settling, want View to draw the page it built")
	}
	was := h.view()
	h.clock.now = h.clock.now.Add(time.Minute)
	if got := h.view(); got != was {
		t.Errorf("a minute on, no message since, the screen is\n%s\nwant it as the last message left it\n%s", got, was)
	}
	h.redraw()
	if got := h.view(); got == was {
		t.Errorf("a message a minute on, the screen is\n%s\nwant it drawn then", got)
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

func TestThePoolsBarsEaseAsTheAccountsDo(t *testing.T) {
	both := func(used float64) status.Document {
		return document(
			account("work", "Work", session(used, 3*time.Hour), week(0.5)),
			account("personal", "Personal", session(used, 4*time.Hour), week(0.5)),
		).WorkedOut(policy)
	}
	h := newHarness(t, both(0.5))
	h.start()
	h.settle()

	h.clock.now = h.clock.now.Add(time.Minute)
	h.read(both(0.75))
	h.clock.now = h.clock.now.Add(easeFor / 2)
	h.redraw()

	pool := dashboard.Ref{Window: "5h"}
	from, to := both(0.5).Pool.Windows[0].Used, both(0.75).Pool.Windows[0].Used
	now := h.model.now()
	f := h.model.live(now)
	if got, want := f.Eased[pool], lerp(from, to, easeOutCubic(0.5)); got != want {
		t.Fatalf("half way, the pool's 5-hour bar fills %v, want %v, eased from %v to %v", got, want, from, to)
	}
	delete(f.Eased, pool)
	if got, asRead := h.view(), ansi.Strip(strings.Join(f.Draw(h.model.doc, now), "\n")); got == asRead {
		t.Errorf("half way, the screen is\n%s\nwant the pool's bar eased, not filled as read", got)
	}
}

// The Overview draws a session read before its reset empty once it resets,
// where Sessions' panels draw it as last read: each view's bar eases from
// the bar that view drew, whichever view showed as the read landed.
func TestAcrossAResetEachViewEasesItsBarFromTheBarItDrew(t *testing.T) {
	before := document(account("work", "Work", session(0.75, 10*time.Minute), week(0.5)))
	after := document(account("work", "Work", session(0.25, 20*time.Minute+5*time.Hour), week(0.5)))
	bar := dashboard.Ref{Account: "work", Window: "5h"}
	tests := []struct {
		name            string
		readOn, shownOn dashboard.View
		from            float64
	}{
		{name: "the Overview", readOn: dashboard.Overview, shownOn: dashboard.Overview, from: 0},
		{name: "Sessions", readOn: dashboard.Sessions, shownOn: dashboard.Sessions, from: 0.75},
		{name: "the Overview, shown after Sessions as the read landed", readOn: dashboard.Sessions, shownOn: dashboard.Overview, from: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, before)
			h.model.view = tt.readOn
			h.start()
			h.settle()
			h.clock.now = start.Add(20 * time.Minute)
			h.redraw()
			h.read(after)
			h.model.view = tt.shownOn
			landed := h.model.now()
			if got, ok := h.model.live(landed).Eased[bar]; !ok || got != tt.from {
				t.Errorf("as the read lands, work's session bar fills %v, easing %t, want %v, as the view drew it", got, ok, tt.from)
			}
			h.clock.now = landed.Add(easeFor / 2)
			if got, want := h.model.live(h.clock.now).Eased[bar], lerp(tt.from, 0.25, easeOutCubic(0.5)); got != want {
				t.Errorf("half way, work's session bar fills %v, want %v, eased from %v to 0.25", got, want, tt.from)
			}
		})
	}
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
