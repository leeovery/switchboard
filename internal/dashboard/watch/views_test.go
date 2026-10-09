package watch

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestOpensOnTheViewKept(t *testing.T) {
	tests := []struct {
		name string
		kept dashboard.View
		want dashboard.View
	}{
		{name: "the view kept", kept: dashboard.Runway, want: dashboard.Runway},
		{name: "Sessions kept", kept: dashboard.Sessions, want: dashboard.Sessions},
		{name: "none kept: the first", kept: "", want: dashboard.Accounts},
		{name: "one there isn't, as from a later switchboard: the first", kept: "a-later-view", want: dashboard.Accounts},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(t.Context(), Config{View: tt.kept})
			if m.view != tt.want {
				t.Errorf("opened on %q, want %q", m.view, tt.want)
			}
		})
	}
}

func TestTabShowsTheNextViewAndKeepsIt(t *testing.T) {
	h := newHarness(t, calm())
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()

	if !strings.HasPrefix(h.footer(), "tab views · ") {
		t.Errorf("the footer reads %q, want tab first, there being views to move between", h.footer())
	}
	for _, step := range []struct {
		key  string
		want dashboard.View
	}{
		{key: "tab", want: dashboard.Sessions},
		{key: "tab", want: dashboard.Runway},
		{key: "tab", want: dashboard.Accounts},
		{key: "shift+tab", want: dashboard.Runway},
	} {
		h.press(step.key)
		if h.model.view != step.want || prefs.kept.View != string(step.want) {
			t.Errorf("%s showed %q and kept %q, want %q kept", step.key, h.model.view, prefs.kept.View, step.want)
		}
	}
	if !strings.Contains(h.view(), " Runway ") {
		t.Errorf("the screen is\n%s\nwant Runway's tab", h.view())
	}
}

func TestTabTurnsRoundTheViewsBuilt(t *testing.T) {
	h := newHarness(t, calm())
	h.start()

	if got, want := h.model.views, []dashboard.View{dashboard.Accounts, dashboard.Sessions, dashboard.Runway}; !slices.Equal(got, want) {
		t.Fatalf("the views are %q, want %q", got, want)
	}
	for _, step := range []struct {
		key  string
		want dashboard.View
		// wantShows is what the screen shows of the view.
		wantShows string
	}{
		{key: "tab", want: dashboard.Sessions, wantShows: "┌─ 1 · WORK ─"},
		{key: "tab", want: dashboard.Runway, wantShows: "accounts with room"},
		{key: "tab", want: dashboard.Accounts, wantShows: "╭─ 1 Work"},
		{key: "shift+tab", want: dashboard.Runway, wantShows: "accounts with room"},
		{key: "shift+tab", want: dashboard.Sessions, wantShows: "┌─ 1 · WORK ─"},
		{key: "shift+tab", want: dashboard.Accounts, wantShows: "╭─ 1 Work"},
	} {
		h.press(step.key)
		if h.model.view != step.want || !strings.Contains(h.view(), step.wantShows) {
			t.Errorf("%s showed %q, the screen\n%s\nwant %q, showing %q", step.key, h.model.view, h.view(), step.want, step.wantShows)
		}
	}
	if !strings.Contains(h.view(), " Accounts   Sessions   Runway   tab ⇥") {
		t.Errorf("the screen is\n%s\nwant the views built as tabs, and tab's hint", h.view())
	}
}

func TestAViewTabTurnsToShowsFromItsTop(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	h.press("j")
	h.press("j")

	h.press("shift+tab")
	if h.model.scroll != 0 || !strings.Contains(h.view(), " 1 Account A") {
		t.Errorf("turned to Runway, scrolled %d, the screen\n%s\nwant it from its first lane", h.model.scroll, h.view())
	}
	if h.press("j"); h.model.scroll != 1 || strings.Contains(h.view(), " 1 Account A") {
		t.Errorf("j scrolled Runway to %d, the screen\n%s\nwant its lanes a row down", h.model.scroll, h.view())
	}
	if help := h.model.helpKeys(); !slices.Contains(help, dashboard.Key{Key: "j k", Does: "scroll the lanes; PgUp and PgDn a page, or the wheel"}) {
		t.Errorf("the help lists %+v, want j and k scrolling the lanes", help)
	}
}

func TestWSwitchesRunwayBetweenTheDayAndTheWeek(t *testing.T) {
	h := newHarness(t, calm())
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()
	h.press("shift+tab")
	prefs.updates = 0

	for _, step := range []struct {
		want       dashboard.Span
		footer     string
		help       string
		wantShows  string
		wantHidden string
	}{
		{want: dashboard.Day, footer: "tab views · w window: day · ", help: "switch between the day and the week, now the day", wantShows: "accounts with room", wantHidden: "weeks with room"},
		{want: dashboard.Week, footer: "tab views · w window: week · ", help: "switch between the day and the week, now the week", wantShows: "weeks with room", wantHidden: "accounts with room"},
		{want: dashboard.Day, footer: "tab views · w window: day · ", help: "switch between the day and the week, now the day", wantShows: "accounts with room", wantHidden: "weeks with room"},
	} {
		if h.model.span != step.want || !strings.HasPrefix(h.footer(), step.footer) {
			t.Errorf("Runway shows the %s, the footer reading %q, want the %s, it starting %q", h.model.span.Name(), h.footer(), step.want.Name(), step.footer)
		}
		if help := h.model.helpKeys(); !slices.Contains(help, dashboard.Key{Key: "w", Does: step.help}) {
			t.Errorf("the help lists %+v, want w to %q", help, step.help)
		}
		if !strings.Contains(h.view(), step.wantShows) || strings.Contains(h.view(), step.wantHidden) {
			t.Errorf("the screen is\n%s\nwant %q, and no %q", h.view(), step.wantShows, step.wantHidden)
		}
		h.press("w")
	}
	if h.model.featured != dashboard.Auto || prefs.updates > 0 {
		t.Errorf("w in Runway featured %q, keeping the preferences %d times, want the cards' window left as it was, and nothing kept", h.model.featured, prefs.updates)
	}
	h.press("tab")
	if h.press("w"); h.model.featured != "5h" || h.model.span != dashboard.Week {
		t.Errorf("w in Accounts featured %q, Runway showing the %s, want the cards featuring the 5-hour window, and Runway left on the week", h.model.featured, h.model.span.Name())
	}
}

func TestWithOneViewTabMovesNowhere(t *testing.T) {
	h := newHarness(t, calm())
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.model.views = []dashboard.View{dashboard.Accounts}
	h.start()

	h.press("tab")
	h.press("shift+tab")
	if h.model.view != dashboard.Accounts || prefs.updates > 0 {
		t.Errorf("showing %q, and kept the view %d times; want Accounts, and nothing kept", h.model.view, prefs.updates)
	}
	if strings.Contains(h.footer(), "tab") || strings.Contains(h.view(), "tab ⇥") {
		t.Errorf("the screen is\n%s\nwant no word of tab, there being no view to move to", h.view())
	}
}

func TestAViewThatCantBeKeptIsShownAllTheSame(t *testing.T) {
	log := logstest.Capture(t)
	h := newHarness(t, calm())
	h.model.cfg.Prefs = &fakePrefs{err: errors.New("keep the preferences: read-only file system")}
	h.start()

	h.press("tab")
	if h.model.view != dashboard.Sessions {
		t.Errorf("showing %q, want sessions all the same", h.model.view)
	}
	if want := []string{"level=WARN", `msg="couldn't keep the view shown"`, "view=sessions"}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
}

func TestTheKeysThatSendSessionsDoNothingWithOneAccount(t *testing.T) {
	doc := routerDocument(three()[0])
	for _, key := range []string{"1", "a", "m"} {
		t.Run(key, func(t *testing.T) {
			h := routedHarness(t, doc)
			h.start()

			h.deliver(h.press(key)...)
			if len(h.source.orders) > 0 || h.model.note != "" {
				t.Errorf("orders = %q, note %q, want neither: there's no other account", h.source.orders, h.model.note)
			}
		})
	}
}

func TestWCyclesTheWindowEveryCardFeaturesAndKeepsIt(t *testing.T) {
	doc := document(
		account("work", "Work", session(0.25, 3*time.Hour), week(0.5), fableWeek(0.1)),
		account("side", "Side", session(0.4, 2*time.Hour), week(0.6), fableWeek(0)),
	)
	h := newHarness(t, doc)
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()

	if !strings.HasPrefix(h.footer(), "tab views · w window: auto · ") {
		t.Errorf("the footer reads %q, want w saying the cards feature each its own, as auto has it", h.footer())
	}
	for _, step := range []struct {
		want   dashboard.Feature
		footer string
	}{
		{want: "5h", footer: "tab views · w window: 5h · "},
		{want: "7d", footer: "tab views · w window: week · "},
		{want: "7d_oi", footer: "tab views · w window: Fable wk · "},
		{want: dashboard.Auto, footer: "tab views · w window: auto · "},
	} {
		h.press("w")
		if h.model.featured != step.want || prefs.kept.Featured != string(step.want) {
			t.Errorf("w featured %q and kept %q, want %q kept", h.model.featured, prefs.kept.Featured, step.want)
		}
		if !strings.HasPrefix(h.footer(), step.footer) {
			t.Errorf("the footer reads %q, want it to start %q", h.footer(), step.footer)
		}
	}
}

func TestWPassesOverAWindowNoAccountUses(t *testing.T) {
	h := newHarness(t, document(account("work", "Work", session(0.25, 3*time.Hour), week(0.5), fableWeek(0))))
	h.start()

	for _, want := range []dashboard.Feature{"5h", "7d", dashboard.Auto} {
		if h.press("w"); h.model.featured != want {
			t.Errorf("w featured %q, want %q: Fable's week hidden, unused", h.model.featured, want)
		}
	}
}

func TestAWindowKeptButNoLongerInUseIsNamedAsTheCardsShowItAuto(t *testing.T) {
	h := newHarness(t, document(account("work", "Work", session(0.25, 3*time.Hour), week(0.5), fableWeek(0))))
	h.model.featured = "7d_oi"
	h.start()

	if !strings.HasPrefix(h.footer(), "tab views · w window: auto · ") {
		t.Errorf("the footer reads %q, want auto, as the cards show Fable's week, unused", h.footer())
	}
	if help := h.model.helpKeys(); help[1].Key != "w" || help[1].Does != "cycle the window every card features, now auto" {
		t.Errorf("the help says %s does %q, want w to say auto", help[1].Key, help[1].Does)
	}
	if h.press("w"); h.model.featured != "5h" {
		t.Errorf("w featured %q, want 5h, moving on from auto", h.model.featured)
	}
}

func TestOpensFeaturingTheWindowKept(t *testing.T) {
	h := unsizedHarness(t, calm(), Size{Width: 160, Height: 40})
	h.model = New(t.Context(), Config{
		Source: h.source, Notifier: h.notifier, Notifications: notifications, Now: h.clock.Now, After: h.arm,
		Interval: interval, Policy: policy, Size: Size{Width: 160, Height: 40}, Featured: "7d",
		Ledger: fakeLedger{}, Readings: fakeReadings{}, Events: fakeEvents{},
	})
	h.start()

	if !strings.HasPrefix(h.footer(), "tab views · w window: week · ") {
		t.Errorf("the footer reads %q, want the week the preferences kept", h.footer())
	}
	if !strings.Contains(h.view(), "WEEK  7-day window") {
		t.Errorf("the screen is\n%s\nwant the card featuring its week", h.view())
	}
}

func TestWDoesNothingTillTheDocumentIsRead(t *testing.T) {
	h := newHarness(t, calm())
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.init()

	h.press("w")
	if h.model.featured != dashboard.Auto || prefs.updates > 0 {
		t.Errorf("featuring %q, kept %d times; want auto, nothing kept, there being nothing to feature", h.model.featured, prefs.updates)
	}
}

func TestAFeaturedWindowThatCantBeKeptIsFeaturedAllTheSame(t *testing.T) {
	log := logstest.Capture(t)
	h := newHarness(t, calm())
	h.model.cfg.Prefs = &fakePrefs{err: errors.New("keep the preferences: read-only file system")}
	h.start()

	h.press("w")
	if h.model.featured != "5h" {
		t.Errorf("featuring %q, want the 5-hour window all the same", h.model.featured)
	}
	if want := []string{"level=WARN", `msg="couldn't keep the window the cards feature"`, "window=5h"}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
}

func TestGCyclesTheChartEveryCardDrawsAndKeepsIt(t *testing.T) {
	h := newHarness(t, calm())
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.update(tea.WindowSizeMsg{Width: 160, Height: 50})
	h.start()

	for _, step := range []struct {
		want     dashboard.Chart
		kept     string
		glyphs   string
		describe string
	}{
		{want: dashboard.BurnRate, kept: "burn-rate", glyphs: "▃▅ use per 10 min", describe: "now burn rate"},
		{want: dashboard.Hourglass, kept: "hourglass", glyphs: "▐▌ its recent rate, falling while busy", describe: "now hourglass"},
		{want: dashboard.Burndown, kept: "", glyphs: "⠂⠄⡀ heading", describe: "now burn-down"},
	} {
		h.press("g")
		if h.model.chart != step.want || prefs.kept.Chart != step.kept {
			t.Errorf("g drew %q and kept %q, want %q, kept as %q", h.model.chart, prefs.kept.Chart, step.want, step.kept)
		}
		if !strings.Contains(h.view(), step.glyphs) {
			t.Errorf("the screen is\n%s\nwant the key line to explain the %s's glyphs, %q", h.view(), step.want.Name(), step.glyphs)
		}
		if g := listedHelp(h.model, "g"); !strings.HasSuffix(g, step.describe) {
			t.Errorf("the help says g does %q, want it to end %q", g, step.describe)
		}
	}
}

func TestGWorksInTheAccountsViewAlone(t *testing.T) {
	h := newHarness(t, calm())
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()
	h.press("tab")

	if h.press("g"); h.model.chart != dashboard.Burndown || prefs.kept.Chart != "" {
		t.Errorf("g in Sessions drew %q and kept %q, want the cards' style left as it was, and nothing kept", h.model.chart, prefs.kept.Chart)
	}
	if listedHelp(h.model, "g") != "" || strings.Contains(h.footer(), "g chart") {
		t.Errorf("in Sessions, the help lists g as %q and the footer reads %q, want g in neither", listedHelp(h.model, "g"), h.footer())
	}
}

func TestOpensDrawingTheChartKept(t *testing.T) {
	h := unsizedHarness(t, calm(), Size{Width: 160, Height: 40})
	h.model = New(t.Context(), Config{
		Source: h.source, Notifier: h.notifier, Notifications: notifications, Now: h.clock.Now, After: h.arm,
		Interval: interval, Policy: policy, Size: Size{Width: 160, Height: 40}, Chart: dashboard.Hourglass,
		Ledger: fakeLedger{}, Readings: fakeReadings{}, Events: fakeEvents{},
	})
	h.start()

	if h.model.chart != dashboard.Hourglass || !strings.Contains(h.view(), "its recent rate, falling while busy") {
		t.Errorf("drawing %q, the screen is\n%s\nwant the cards' hourglasses the preferences kept", h.model.chart, h.view())
	}
}

func TestAChartStyleThatCantBeKeptIsDrawnAllTheSame(t *testing.T) {
	log := logstest.Capture(t)
	h := newHarness(t, calm())
	h.model.cfg.Prefs = &fakePrefs{err: errors.New("keep the preferences: read-only file system")}
	h.start()

	if h.press("g"); h.model.chart != dashboard.BurnRate {
		t.Errorf("drawing %q, want burn rate all the same", h.model.chart)
	}
	if want := []string{"level=WARN", `msg="couldn't keep the chart style"`, `chart="burn rate"`}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
}

func TestAnHourglassFallsFrameByFrameWhileItsAccountIsBusy(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{sessionOn(idD28C, "claude-opus-5-5", "work", 10*time.Second)}
	h.start()
	h.settle()

	h.press("g")
	if _, ok := h.pendingFrame(); ok {
		t.Fatal("drawing burn rates, a frame is armed, want none, nothing moving")
	}
	h.press("g")
	if _, ok := h.pendingFrame(); !ok {
		t.Fatal("drawing hourglasses, work busy, no frame is armed, want frames as its sand falls")
	}
	before, falling := h.view(), h.clock.now
	for h.view() == before {
		tm, ok := h.pendingFrame()
		if !ok || h.clock.now.After(falling.Add(time.Second)) {
			t.Fatalf("a second on, the screen is as it was\n%s\nwant work's stream fallen a grain, frame by frame", before)
		}
		h.fire(tm)
	}

	tm, _ := h.pendingFrame()
	tm.due = start.Add(2 * time.Minute)
	h.fire(tm)
	if _, ok := h.pendingFrame(); ok || h.model.framing {
		t.Error("work idle a minute and more, the frames go on, want them to stop, its stream still")
	}
}

func TestSandFallsAGrainAFrameWhereTheClockReadsALittleBehindTheirTimers(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{sessionOn(idD28C, "claude-opus-5-5", "work", 10*time.Second)}
	h.start()
	h.settle()
	h.press("g")
	h.press("g")

	for frame := range 6 {
		tm, ok := h.pendingFrame()
		if !ok {
			t.Fatalf("after %d frames, none is armed, want work's sand falling on", frame)
		}
		before := h.view()
		h.fireBehind(tm, 30*time.Microsecond)
		if h.view() == before {
			t.Fatalf("frame %d, due %s, drew the screen as it was\n%s\nwant work's stream fallen a grain", frame+1, tm.due.Format(time.StampMicro), before)
		}
	}
}

func TestFallingSandIsDrawnAGrainAtATimeAndWhatMovesSmoothlyAtOnce(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{sessionOn(idD28C, "claude-opus-5-5", "work", 10*time.Second)}
	h.start()
	h.settle()
	h.clock.now = start.Add(2*time.Second + 40*time.Millisecond)
	h.press("g")
	h.press("g")

	if frames := h.pendingFrames(); len(frames) != 1 || frames[0].delay != dashboard.FallStep-40*time.Millisecond+stepSlack {
		t.Fatalf("with work's sand falling alone, %d frames are armed, the next %v on; want one, %v on, just past when its stream next falls a grain", len(frames), frames[0].delay, dashboard.FallStep-40*time.Millisecond+stepSlack)
	}

	moved := routerDocument(three()...)
	moved.Accounts[1].Windows[0].Utilization = 0.3
	h.startRouter(moved)
	h.deliver(h.press("r")...)
	if frames := h.pendingFrames(); len(frames) != 2 || frames[0].delay != frameEvery {
		t.Fatalf("as personal's bar starts to ease, %d frames are armed, the soonest after %v; want one at once, %v on, beside the sand's", len(frames), frames[0].delay, frameEvery)
	}

	h.framesUntil(h.clock.now.Add(easeFor + time.Second))
	if frames := h.pendingFrames(); len(frames) != 1 || frames[0].delay > dashboard.FallStep+stepSlack {
		t.Errorf("the bar eased, %d frames are armed, want one, at the sand's pace again, the frame armed before the bar moved dropped", len(frames))
	}
}

func TestAnHourglassFallsAsLongAsTheStreamSaysItsAccountIsBusy(t *testing.T) {
	g, right := tea.KeyPressMsg{Code: 'g', Text: "g"}, tea.KeyPressMsg{Code: tea.KeyRight}
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{sessionOn(idD28C, opus, "work", 9*time.Minute)}
	h.update(tea.WindowSizeMsg{Width: 160, Height: 40})
	h.start()
	h.settle()
	h.keys(g, g, right, right, spaceKey)
	if _, ok := h.pendingFrame(); ok || h.opened() != 1 {
		t.Fatalf("personal's card flipped, the stream opened %d times, a frame armed %v; want it open, and none armed, d28c idle on work as listed", h.opened(), ok)
	}

	h.hear(told(router.StreamSent, "r1", "work", 0))
	if _, ok := h.pendingFrame(); !ok {
		t.Fatal("as d28c's request goes out on work, no frame is armed, want work's sand falling")
	}
	h.hear(alter(told(router.StreamDone, "r1", "work", 3*time.Second), func(e *router.StreamEvent) { e.Status = 200 }))
	h.framesUntil(past(62 * time.Second))
	if !h.model.framing {
		t.Error("a minute after the stream last told of d28c, work's sand stopped, want it falling till a minute has passed")
	}
	h.framesUntil(past(70 * time.Second))
	if h.model.framing || len(h.pendingFrames()) > 0 {
		t.Error("over a minute since the stream told of d28c, frames still run, want work's sand still, its account idle")
	}
}

func TestTheStreamIsReadWhileTheCardsDrawHourglasses(t *testing.T) {
	g := tea.KeyPressMsg{Code: 'g', Text: "g"}
	h := streamingHarness(t)

	steps := []struct {
		chart  dashboard.Chart
		opened int
		open   bool
	}{
		{chart: dashboard.BurnRate},
		{chart: dashboard.Hourglass, opened: 1, open: true},
		{chart: dashboard.Burndown, opened: 1},
	}
	for _, step := range steps {
		h.keys(g)
		open := h.opened() > 0 && !h.closed(h.opened())
		if h.model.chart != step.chart || h.opened() != step.opened || open != step.open {
			t.Errorf("drawing %q, the stream was opened %d times, the last open %v; want %q, opened %d times, open %v", h.model.chart, h.opened(), open, step.chart, step.opened, step.open)
		}
	}
}

func TestAnHourglassFallsThroughALongAnswer(t *testing.T) {
	g := tea.KeyPressMsg{Code: 'g', Text: "g"}
	h := streamingHarness(t)
	h.keys(g, g)
	h.hear(told(router.StreamSent, "r1", "work", 0), told(router.StreamFirst, "r1", "work", time.Second))

	h.framesUntil(past(3 * time.Minute))
	if frames := h.pendingFrames(); len(frames) == 0 || !frames[0].due.After(past(3*time.Minute)) {
		t.Error("three minutes into d28c's answer, no frame is armed, want work's sand falling while it streams")
	}
}

func TestAnHourglassUnderTheHelpDrawsNoFrames(t *testing.T) {
	g, help := tea.KeyPressMsg{Code: 'g', Text: "g"}, tea.KeyPressMsg{Code: '?', Text: "?"}
	tests := []struct {
		name, busy string
		want       bool
	}{
		{name: "work's, beside it", busy: "work", want: true},
		{name: "personal's, under it", busy: "personal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.source.sessions = []status.Session{sessionOn(idD28C, opus, tt.busy, 10*time.Second)}
			h.update(tea.WindowSizeMsg{Width: 160, Height: 40})
			h.start()
			h.settle()
			h.keys(g, g, help)
			h.framesUntil(h.clock.now.Add(time.Second))
			if _, ok := h.pendingFrame(); ok != tt.want {
				t.Errorf("the help open, a frame is armed: %v, want %v", ok, tt.want)
			}
		})
	}
}

func TestAnHourglassOfAnIdleAccountDrawsNoFrames(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{sessionOn(id7F3A, "claude-haiku-4-5", "work", 9*time.Minute)}
	h.start()
	h.settle()

	h.press("g")
	h.press("g")
	if _, ok := h.pendingFrame(); ok {
		t.Error("drawing hourglasses, no account busy, a frame is armed, want none, every stream still")
	}
}

// listedHelp is what the help says the key does, as m stands: "" where it
// doesn't list it.
func listedHelp(m Model, key string) string {
	for _, k := range m.helpKeys() {
		if k.Key == key {
			return k.Does
		}
	}
	return ""
}

// fakePrefs keeps the preferences each update makes of them, or fails to with
// err, counting the updates.
type fakePrefs struct {
	kept    theme.Prefs
	updates int
	err     error
}

func (p *fakePrefs) Update(change func(*theme.Prefs)) error {
	p.updates++
	if p.err != nil {
		return p.err
	}
	next := p.kept
	change(&next)
	p.kept = next
	return nil
}
