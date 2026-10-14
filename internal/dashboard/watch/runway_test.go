package watch

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayHarness is the router's three accounts, the readings history filed
// holding the readings given, opened on the tab given, 160 cells wide; it's
// started.
func runwayHarness(t *testing.T, view dashboard.View, held ...readings.Reading) (*harness, *filedReadings) {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
	filed := &filedReadings{held: held}
	h.model.cfg.Readings, h.model.view = filed, view
	h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: 37})...)
	h.start()
	return h, filed
}

// < and > show the other window's live view, which the preferences keep; ←
// and → step back and on, which they never keep.
func TestRunwaysWindowIsKeptAsItChangesAndAStepBackNever(t *testing.T) {
	h, _ := runwayHarness(t, dashboard.Runway)
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	steps := []struct {
		key     string
		window  views.RunwayWindow
		back    bool
		updates int
	}{
		{key: ">", window: views.RunwayWeek, updates: 1},
		{key: "left", window: views.RunwayWeek, back: true, updates: 1},
		{key: "right", window: views.RunwayWeek, updates: 1},
		{key: "left", window: views.RunwayWeek, back: true, updates: 1},
		{key: "<", window: views.RunwayFiveHour, updates: 2},
		{key: "<", window: views.RunwayWeek, updates: 3},
	}
	for _, step := range steps {
		h.keyed(step.key)
		shown := h.model.runwayTab.shown
		if shown.Window != step.window || shown.Back.IsZero() == step.back || prefs.kept.Runway != shown.Kept() || prefs.updates != step.updates {
			t.Fatalf("after %s, Runway shows %+v, the preferences keep %+v, updated %d times; want %q stepped back %t, kept, %d times",
				step.key, shown, prefs.kept.Runway, prefs.updates, step.window, step.back, step.updates)
		}
	}
	if prefs.kept.Runway != (theme.RunwayPrefs{Window: "week"}) {
		t.Errorf("the preferences keep %+v, want the week", prefs.kept.Runway)
	}
}

func TestRunwaysKeysAreTakenOnRunwayAlone(t *testing.T) {
	h, _ := runwayHarness(t, dashboard.History)
	h.keyed("> left")
	if shown := h.model.runwayTab.shown; shown.Window != views.RunwayFiveHour || !shown.Back.IsZero() {
		t.Errorf("> and ← on History leave Runway showing %+v, want its live 5-hour view as it was", shown)
	}
}

// Runway's lanes are built at a look, over the timeline of the window shown
// across the lanes' columns, and never at a frame, which takes nothing in.
func TestRunwaysLanesAreBuiltAtALookAndNeverAFrame(t *testing.T) {
	h, _ := runwayHarness(t, dashboard.Runway)
	shown := h.model.runwayTab.shown
	if shown.Lanes == nil || shown.Timeline != views.LiveTimeline(views.RunwayFiveHour, start, 136) {
		t.Fatalf("once started, Runway shows lanes %v over %+v, want them built over the live 5-hour view across 136 columns", shown.Lanes, shown.Timeline)
	}

	h.model.runwayTab.shown.Lanes = nil
	h.deliver(h.update(frameMsg{gen: h.model.frames})...)
	if h.model.runwayTab.shown.Lanes != nil {
		t.Errorf("a frame built Runway's lanes, want them built at a look alone")
	}
	h.lookAt(start.Add(interval))
	if h.model.runwayTab.shown.Lanes == nil {
		t.Errorf("a look built no lanes for Runway")
	}
}

// The readings history's tail is made once, read as Runway opens, and read
// again at each look for what it gained, never at a frame; while another tab
// shows, it's never read.
func TestTheReadingsHistorysTailIsReadAtEachLookWhileRunwayShows(t *testing.T) {
	inSpan := filedOn("work", "five_hour", start.Add(-time.Hour), 0.3)
	tooOld := filedOn("work", "five_hour", start.AddDate(0, 0, -20), 0.1)
	h, filed := runwayHarness(t, dashboard.Runway, tooOld, inSpan)
	if len(filed.starts) != 1 || filed.tails == 0 {
		t.Fatalf("as Runway opens, %d tails were made and read %d times, want one, read", len(filed.starts), filed.tails)
	}
	if held := h.model.runwayTab.tail.held; !slices.Equal(held, []readings.Reading{inSpan}) {
		t.Errorf("the tail holds %v, want the reading in its span alone", held)
	}

	read := filed.tails
	h.deliver(h.update(frameMsg{gen: h.model.frames})...)
	if filed.tails != read {
		t.Errorf("a frame read the tail, %d reads in all, want %d", filed.tails, read)
	}
	h.lookAt(start.Add(interval))
	if len(filed.starts) != 1 || filed.tails != read+1 {
		t.Errorf("at a look, %d tails were made and read %d times in all, want the one read again, %d times", len(filed.starts), filed.tails, read+1)
	}

	other, otherFiled := runwayHarness(t, dashboard.History, inSpan)
	other.lookAt(start.Add(interval))
	if len(otherFiled.starts) != 0 || otherFiled.tails != 0 {
		t.Errorf("with History shown, %d tails were made and read %d times, want none", len(otherFiled.starts), otherFiled.tails)
	}
}

// The live tail spans from the day before today: a step back within it reads
// it still; one before it, a tail of the day stepped back to, read at once;
// and the day turning, a tail from the day before the new today.
func TestTheTailIsMadeAfreshWhereTheSpanRunwayWantsMoves(t *testing.T) {
	h, filed := runwayHarness(t, dashboard.Runway)
	yesterday, twoDaysAgo := dayfile.DayStart(start, -1), dayfile.DayStart(start, -2)

	h.keyed("left left")
	if !slices.Equal(filed.starts, []time.Time{yesterday}) {
		t.Fatalf("stepped back to yesterday, the tails made start %v, want the live one alone, from %s", filed.starts, yesterday)
	}
	read := filed.tails
	h.keyed("left")
	if !slices.Equal(filed.starts, []time.Time{yesterday, twoDaysAgo}) || filed.tails != read+1 {
		t.Fatalf("stepped back two days, the tails made start %v, read %d times, want one from %s made and read, %d times in all", filed.starts, filed.tails, twoDaysAgo, read+1)
	}
	h.keyed("right right right")
	if !slices.Equal(filed.starts, []time.Time{yesterday, twoDaysAgo, yesterday}) {
		t.Fatalf("on again to the live view, the tails made start %v, want the live one again", filed.starts)
	}

	tomorrow := dayfile.DayStart(start, 1)
	h.lookAt(tomorrow.Add(time.Minute))
	if got := filed.starts[len(filed.starts)-1]; !got.Equal(dayfile.DayStart(start, 0)) {
		t.Errorf("as the day turns, the last tail made starts %s, want today's start, the day before the new today", got)
	}
}
