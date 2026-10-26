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
		{name: "Graphs kept", kept: "graphs", want: dashboard.Graphs},
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
	sections := dashboard.SectionsNamed([]string{"usage", "sessions"})
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

func TestTabTurnsRoundTheSevenTabs(t *testing.T) {
	h := newHarness(t, calm())
	h.start()

	if !strings.Contains(h.view(), " Overview   Accounts   History   Sessions   Runway   Graphs   Log ") {
		t.Errorf("the screen is\n%s\nwant the seven tabs", h.view())
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
		{key: "tab", want: dashboard.Runway, wantShows: "RUNWAY  ─"},
		{key: "tab", want: dashboard.Graphs},
		{key: "tab", want: dashboard.Log},
		{key: "tab", want: dashboard.Overview, wantShows: overview},
		{key: "shift+tab", want: dashboard.Log},
		{key: "shift+tab", want: dashboard.Graphs},
		{key: "shift+tab", want: dashboard.Runway, wantShows: "RUNWAY  ─"},
	} {
		h.press(step.key)
		shown := h.view()
		if h.model.view != step.want || !strings.Contains(shown, step.wantShows) || step.wantShows == "" && strings.Contains(shown, overview) {
			t.Errorf("%s showed %q, the screen\n%s\nwant %q, showing %q", step.key, h.model.view, shown, step.want, step.wantShows)
		}
	}
}

func TestAViewTabTurnsToShowsFromItsTop(t *testing.T) {
	h := yearHarness(t, yearSize)
	top := h.scrolled()
	h.press("j")
	h.press("j")

	h.press("tab")
	h.press("shift+tab")
	if h.model.scroll != 0 || h.scrolled() != top {
		t.Errorf("turned to History, scrolled %d, the screen\n%s\nwant its Year from its top", h.model.scroll, h.view())
	}
	if h.press("j"); h.model.scroll != 1 || h.scrolled() == top {
		t.Errorf("j scrolled the Year to %d, the screen\n%s\nwant it a row down", h.model.scroll, h.view())
	}
	if help := helpOver(h.model); !slices.Contains(help, dashboard.Key{Key: "j k", Does: "scroll"}) {
		t.Errorf("the help lists %+v, want j and k scrolling the Year", help)
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
		{key: "s", want: []string{"usage", "routing", "sessions", "coming", "lately"}, wantShown: "SESSIONS"},
		{key: "u", want: []string{"routing", "sessions", "coming", "lately"}, wantHidden: "USAGE"},
		{key: "c", want: []string{"routing", "sessions", "lately"}, wantHidden: "COMING UP"},
		{key: "u", want: []string{"usage", "routing", "sessions", "lately"}, wantShown: "USAGE"},
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

// GRAPHS is gone from the Overview, and its keys with it: g and v change
// nothing there, keep nothing, and neither the key line nor help names them.
func TestGAndVDoNothingOnTheOverview(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()
	before, screen := h.model.sections, h.view()

	h.typed("g", "v")
	if h.model.sections != before || prefs.updates > 0 || h.view() != screen {
		t.Errorf("after g and v, showing %q, kept %d times; want %q shown as before, nothing kept and the screen unchanged", h.model.sections.Names(), prefs.updates, before.Names())
	}
	for _, said := range []string{"g graphs", "v chart"} {
		if strings.Contains(h.keyLine(), said) {
			t.Errorf("the key line reads %q, want no %q", h.keyLine(), said)
		}
	}
	if keys := listedKeys(helpOver(h.model)); slices.Contains(keys, "g") || slices.Contains(keys, "v") {
		t.Errorf("help lists %q, want neither g nor v", keys)
	}
}

// w shows each model's own window and hides it again wherever the page lists
// windows as they stand, keeping it: the Overview, Accounts' Detail,
// Sessions' Switchboard and a session's page. Help lists it there, its word
// faint while they're hidden; the key line never does. Elsewhere it does
// nothing.
func TestWShowsAndHidesModelsWindowsWhereThePageListsThemAndKeepsIt(t *testing.T) {
	tests := []struct {
		name  string
		show  func(h *harness)
		works bool
	}{
		{name: "the Overview", show: func(*harness) {}, works: true},
		{name: "Accounts' Detail", show: func(h *harness) {
			h.model.view, h.model.accounts.shown.Shown = dashboard.Accounts, dashboard.AccountsDetail
		}, works: true},
		{name: "Accounts' Compare", show: func(h *harness) { h.model.view = dashboard.Accounts }},
		{name: "Sessions' Switchboard", show: func(h *harness) { h.model.view = dashboard.Sessions }, works: true},
		{name: "Sessions' List", show: func(h *harness) {
			h.model.view, h.model.sessionsTab.shown.Shown = dashboard.Sessions, dashboard.SessionsList
		}},
		{name: "History", show: func(h *harness) { h.model.view = dashboard.History }},
		{name: "Runway", show: func(h *harness) { h.model.view = dashboard.Runway }},
		{name: "the Log", show: func(h *harness) { h.model.view = dashboard.Log }},
		{name: "a session's page", show: func(h *harness) { h.openPage(idD28C) }, works: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := openedOn(t, t.TempDir(), dashboard.Overview)
			prefs := &fakePrefs{}
			h.model.cfg.Prefs = prefs
			tt.show(h)

			wKey := func(hidden bool) dashboard.Key { return dashboard.Key{Key: "w", Does: "models", Hidden: hidden} }
			if helped := slices.Contains(helpOver(h.model), wKey(true)); helped != tt.works {
				t.Errorf("help lists w %t, want %t: %+v", helped, tt.works, helpOver(h.model))
			}
			for _, want := range []bool{tt.works, false} {
				h.typed("w")
				if h.model.models != want || prefs.kept.Models != want || h.model.frame(h.clock.now).Models != want {
					t.Errorf("after w, models shown %t, kept %t, the frame's %t; want %t",
						h.model.models, prefs.kept.Models, h.model.frame(h.clock.now).Models, want)
				}
				if tt.works != (prefs.updates > 0) {
					t.Errorf("w kept the preferences %d times, want kept %t", prefs.updates, tt.works)
				}
				if tt.works && !slices.Contains(helpOver(h.model), wKey(!want)) {
					t.Errorf("after w, help lists %+v, want w with its word faint %t", helpOver(h.model), !want)
				}
				if slices.Contains(h.listed(), "w") {
					t.Errorf("the key line lists %q, want w left to help", h.listed())
				}
			}
		})
	}
}

func TestTheWatchOpensShowingTheModelsWindowsThePreferencesKept(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.model = New(t.Context(), func() Config { c := h.model.cfg; c.Models = true; return c }())
	h.start()
	if !h.model.frame(h.clock.now).Models {
		t.Error("kept shown, the watch opens with each model's own window hidden")
	}
}

func TestTheSectionsOrGroupingThatCantBeKeptAreShownAllTheSame(t *testing.T) {
	log := logstest.Capture(t)
	h := routedHarness(t, routerDocument(three()...))
	h.model.cfg.Prefs = &fakePrefs{err: errors.New("keep the preferences: read-only file system")}
	h.start()

	h.typed("s", "b")
	if !h.model.sections.Has(dashboard.SectionSessions) || h.model.by != dashboard.ByAccount {
		t.Errorf("showing %q, by %v; want SESSIONS shown and by account all the same", h.model.sections.Names(), h.model.by)
	}
	for _, want := range [][]string{
		{"level=WARN", `msg="couldn't keep the sections shown"`, "sections=usage,routing,sessions,coming,lately"},
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
