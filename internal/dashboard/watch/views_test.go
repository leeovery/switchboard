package watch

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

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
		{name: "the view kept", kept: dashboard.Runway, want: dashboard.Runway},
		{name: "none kept: the first", kept: "", want: dashboard.Accounts},
		{name: "one there isn't, as from a later switchboard: the first", kept: "sessions", want: dashboard.Accounts},
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
	h.model.views = []dashboard.View{dashboard.Accounts, "sessions", dashboard.Runway}
	h.start()

	if !strings.HasPrefix(h.footer(), "tab views · ") {
		t.Errorf("the footer reads %q, want tab first, there being views to move between", h.footer())
	}
	for _, step := range []struct {
		key  string
		want dashboard.View
	}{
		{key: "tab", want: "sessions"},
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

	if got, want := h.model.views, []dashboard.View{dashboard.Accounts, dashboard.Runway}; !slices.Equal(got, want) {
		t.Fatalf("the views are %q, want %q", got, want)
	}
	for _, step := range []struct {
		key  string
		want dashboard.View
		// wantShows is what the screen shows of the view.
		wantShows string
	}{
		{key: "tab", want: dashboard.Runway, wantShows: "accounts with room"},
		{key: "tab", want: dashboard.Accounts, wantShows: "╭─ 1 Work"},
		{key: "shift+tab", want: dashboard.Runway, wantShows: "accounts with room"},
		{key: "shift+tab", want: dashboard.Accounts, wantShows: "╭─ 1 Work"},
	} {
		h.press(step.key)
		if h.model.view != step.want || !strings.Contains(h.view(), step.wantShows) {
			t.Errorf("%s showed %q, the screen\n%s\nwant %q, showing %q", step.key, h.model.view, h.view(), step.want, step.wantShows)
		}
	}
	if !strings.Contains(h.view(), " Accounts   Runway   tab ⇥") {
		t.Errorf("the screen is\n%s\nwant the views built as tabs, and tab's hint", h.view())
	}
}

func TestAViewTabTurnsToShowsFromItsTop(t *testing.T) {
	h := newHarness(t, twelve())
	h.start()
	h.press("j")
	h.press("j")

	h.press("tab")
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
	h.press("tab")
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
	h.model.views = []dashboard.View{dashboard.Accounts, "sessions"}
	h.start()

	h.press("tab")
	if h.model.view != "sessions" {
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
