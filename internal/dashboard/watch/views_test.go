package watch

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestOpensOnTheViewKept(t *testing.T) {
	tests := []struct {
		name string
		kept dashboard.View
		want dashboard.View
	}{
		{name: "the tab kept", kept: dashboard.Runway, want: dashboard.Runway},
		{name: "the Log kept", kept: dashboard.Log, want: dashboard.Log},
		{name: "none kept: the Overview", kept: "", want: dashboard.Overview},
		{name: "one there isn't, as from a later switchboard: the Overview", kept: "a-later-view", want: dashboard.Overview},
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

func TestOpensWithTheOverviewsSectionsAndGroupingAsConfigured(t *testing.T) {
	sections := dashboard.SectionsNamed([]string{"usage", "graphs"})
	m := New(t.Context(), Config{Sections: sections, By: dashboard.ByAccount})
	if m.sections != sections || m.by != dashboard.ByAccount {
		t.Errorf("opened showing %v, by %v; want %v, by account", m.sections.Names(), m.by, sections.Names())
	}
}

func TestTabShowsTheNextTabAndKeepsIt(t *testing.T) {
	h := newHarness(t, calm())
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()

	if !strings.HasPrefix(h.keyLine(), "tab views · ") {
		t.Errorf("the key line reads %q, want tab first", h.keyLine())
	}
	for _, step := range []struct {
		key  string
		want dashboard.View
	}{
		{key: "tab", want: dashboard.Accounts},
		{key: "tab", want: dashboard.History},
		{key: "shift+tab", want: dashboard.Accounts},
		{key: "shift+tab", want: dashboard.Overview},
		{key: "shift+tab", want: dashboard.Log},
	} {
		h.press(step.key)
		if h.model.view != step.want || prefs.kept.View != string(step.want) {
			t.Errorf("%s showed %q and kept %q, want %q kept", step.key, h.model.view, prefs.kept.View, step.want)
		}
	}
}

func TestTabTurnsRoundTheSixTabs(t *testing.T) {
	h := newHarness(t, calm())
	h.start()

	if !strings.Contains(h.view(), " Overview   Accounts   History   Sessions   Runway   Log   tab ⇥") {
		t.Errorf("the screen is\n%s\nwant the six tabs, and tab's hint", h.view())
	}
	overview := "USAGE"
	for _, step := range []struct {
		key  string
		want dashboard.View
		// wantShows is what the screen shows of the tab: "" for one not yet
		// built, which shows none of the Overview's sections.
		wantShows string
	}{
		{key: "tab", want: dashboard.Accounts},
		{key: "tab", want: dashboard.History},
		{key: "tab", want: dashboard.Sessions, wantShows: "┌─ 1 · WORK ─"},
		{key: "tab", want: dashboard.Runway, wantShows: "accounts with room"},
		{key: "tab", want: dashboard.Log},
		{key: "tab", want: dashboard.Overview, wantShows: overview},
		{key: "shift+tab", want: dashboard.Log},
		{key: "shift+tab", want: dashboard.Runway, wantShows: "accounts with room"},
	} {
		h.press(step.key)
		shown := h.view()
		if h.model.view != step.want || !strings.Contains(shown, step.wantShows) || step.wantShows == "" && strings.Contains(shown, overview) {
			t.Errorf("%s showed %q, the screen\n%s\nwant %q, showing %q", step.key, h.model.view, shown, step.want, step.wantShows)
		}
	}
}

func TestAViewTabTurnsToShowsFromItsTop(t *testing.T) {
	h := runwayHarness(t, runwaySize)
	h.press("j")
	h.press("j")

	h.press("tab")
	h.press("shift+tab")
	if h.model.scroll != 0 || !strings.Contains(h.view(), " 1 Account A") {
		t.Errorf("turned to Runway, scrolled %d, the screen\n%s\nwant it from its first lane", h.model.scroll, h.view())
	}
	if h.press("j"); h.model.scroll != 1 || strings.Contains(h.view(), " 1 Account A") {
		t.Errorf("j scrolled Runway to %d, the screen\n%s\nwant its lanes a row down", h.model.scroll, h.view())
	}
	if help := h.model.helpKeys(); !slices.Contains(help, dashboard.Key{Key: "j k", Does: "scroll"}) {
		t.Errorf("the help lists %+v, want j and k scrolling the lanes", help)
	}
}

func TestWSwitchesRunwayBetweenTheDayAndTheWeek(t *testing.T) {
	h := newHarness(t, calm())
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()
	h.press("shift+tab")
	h.press("shift+tab")
	prefs.updates = 0

	for _, step := range []struct {
		want       dashboard.Span
		keys       string
		does       string
		wantShows  string
		wantHidden string
	}{
		{want: dashboard.Day, keys: "tab views · w window: day · ", does: "window: day", wantShows: "accounts with room", wantHidden: "weeks with room"},
		{want: dashboard.Week, keys: "tab views · w window: week · ", does: "window: week", wantShows: "weeks with room", wantHidden: "accounts with room"},
		{want: dashboard.Day, keys: "tab views · w window: day · ", does: "window: day", wantShows: "accounts with room", wantHidden: "weeks with room"},
	} {
		if h.model.span != step.want || !strings.HasPrefix(h.keyLine(), step.keys) {
			t.Errorf("Runway shows the %s, the key line reading %q, want the %s, it starting %q", h.model.span.Name(), h.keyLine(), step.want.Name(), step.keys)
		}
		if help := h.model.helpKeys(); !slices.Contains(help, dashboard.Key{Key: "w", Does: step.does}) {
			t.Errorf("the help lists %+v, want w as %q", help, step.does)
		}
		if !strings.Contains(h.view(), step.wantShows) || strings.Contains(h.view(), step.wantHidden) {
			t.Errorf("the screen is\n%s\nwant %q, and no %q", h.view(), step.wantShows, step.wantHidden)
		}
		h.press("w")
	}
	if prefs.updates > 0 {
		t.Errorf("w in Runway kept the preferences %d times, want nothing kept", prefs.updates)
	}
	h.press("tab")
	h.press("tab")
	prefs.updates = 0
	if h.press("w"); h.model.span != dashboard.Week || prefs.updates > 0 {
		t.Errorf("w on the Overview left Runway showing the %s, keeping the preferences %d times; want it left on the week, nothing kept", h.model.span.Name(), prefs.updates)
	}
}

func TestAViewThatCantBeKeptIsShownAllTheSame(t *testing.T) {
	log := logstest.Capture(t)
	h := newHarness(t, calm())
	h.model.cfg.Prefs = &fakePrefs{err: errors.New("keep the preferences: read-only file system")}
	h.start()

	h.press("tab")
	if h.model.view != dashboard.Accounts {
		t.Errorf("showing %q, want accounts all the same", h.model.view)
	}
	if want := []string{"level=WARN", `msg="couldn't keep the view shown"`, "view=accounts"}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
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
