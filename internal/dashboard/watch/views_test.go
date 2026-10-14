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
		if h.model.view != step.want || prefs.kept.Tab != string(step.want) {
			t.Errorf("%s showed %q and kept %q, want %q kept", step.key, h.model.view, prefs.kept.Tab, step.want)
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
		{key: "tab", want: dashboard.Sessions},
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
	if help := helpOver(h.model); !slices.Contains(help, dashboard.Key{Key: "j k", Does: "scroll"}) {
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
		if help := helpOver(h.model); !slices.Contains(help, dashboard.Key{Key: "w", Does: step.does}) {
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

func TestASectionsKeyShowsOrHidesItAndKeepsTheSectionsShown(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()

	for _, step := range []struct {
		key        string
		want       []string
		wantShown  string
		wantHidden string
	}{
		{key: "g", want: []string{"usage", "graphs", "routing", "coming", "lately"}, wantShown: "GRAPHS"},
		{key: "u", want: []string{"graphs", "routing", "coming", "lately"}, wantHidden: "USAGE"},
		{key: "s", want: []string{"graphs", "routing", "sessions", "coming", "lately"}, wantShown: "SESSIONS"},
		{key: "c", want: []string{"graphs", "routing", "sessions", "lately"}, wantHidden: "COMING UP"},
		{key: "u", want: []string{"usage", "graphs", "routing", "sessions", "lately"}, wantShown: "USAGE"},
	} {
		h.typed(step.key)
		if got := h.model.sections.Names(); !slices.Equal(got, step.want) || !slices.Equal(prefs.kept.Sections, step.want) {
			t.Errorf("after %s, showing %q, kept %q; want %q shown and kept", step.key, got, prefs.kept.Sections, step.want)
		}
		if shown := h.view(); step.wantShown != "" && !strings.Contains(shown, step.wantShown) || step.wantHidden != "" && strings.Contains(shown, step.wantHidden) {
			t.Errorf("after %s, the screen is\n%s\nwant %q on it, and no %q", step.key, shown, step.wantShown, step.wantHidden)
		}
	}
}

func TestHidingROUTINGAndSESSIONSClosesTheStreamAndShowingEitherOpensIt(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	for _, step := range []struct {
		key string
		// opened is how many times the stream has been opened since the
		// start, and open whether the last is open.
		opened int
		open   bool
	}{
		{key: "s", opened: 1, open: true},
		{key: "r", opened: 1, open: true},
		{key: "s", opened: 1},
		{key: "r", opened: 2, open: true},
	} {
		h.typed(step.key)
		if open := h.opened() > 0 && !h.closed(h.opened()); h.opened() != step.opened || open != step.open {
			t.Errorf("after %s, showing %q, the stream was opened %d times, the last open %v; want %d, open %v",
				step.key, h.model.sections.Names(), h.opened(), open, step.opened, step.open)
		}
	}
}

func TestEverySectionHiddenIsKeptAsNone(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()

	h.typed("u", "r", "c", "l")
	if prefs.kept.Sections == nil || len(prefs.kept.Sections) > 0 {
		t.Errorf("every section hidden, the preferences keep %#v, want none, kept apart from the defaults", prefs.kept.Sections)
	}
}

func TestBSwitchesHowTheWindowsAreGroupedAndKeepsIt(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()

	for _, want := range []struct {
		by   dashboard.By
		kept string
	}{{by: dashboard.ByAccount, kept: "account"}, {by: dashboard.ByWindow, kept: "window"}} {
		h.typed("b")
		if h.model.by != want.by || prefs.kept.By != want.kept || !strings.Contains(h.keyLine(), "b by: "+want.kept) {
			t.Errorf("after b, grouped by %v, kept %q, the key line %q; want by %s, kept, and said", h.model.by, prefs.kept.By, h.keyLine(), want.kept)
		}
	}
	if prefs.kept.Sections != nil || prefs.kept.Tab != "" {
		t.Errorf("b kept the sections %q and the tab %q, want neither touched", prefs.kept.Sections, prefs.kept.Tab)
	}
}

func TestTheSectionsOrGroupingThatCantBeKeptAreShownAllTheSame(t *testing.T) {
	log := logstest.Capture(t)
	h := routedHarness(t, routerDocument(three()...))
	h.model.cfg.Prefs = &fakePrefs{err: errors.New("keep the preferences: read-only file system")}
	h.start()

	h.typed("g", "b")
	if !h.model.sections.Has(dashboard.SectionGraphs) || h.model.by != dashboard.ByAccount {
		t.Errorf("showing %q, by %v; want GRAPHS shown and by account all the same", h.model.sections.Names(), h.model.by)
	}
	for _, want := range [][]string{
		{"level=WARN", `msg="couldn't keep the sections shown"`, "sections=usage,graphs,routing,coming,lately"},
		{"level=WARN", `msg="couldn't keep how the windows are grouped"`, "by=account"},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
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
