package watch

import (
	"errors"
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
		{name: "the view kept", kept: dashboard.Accounts, want: dashboard.Accounts},
		{name: "none kept: the first", kept: "", want: dashboard.Accounts},
		{name: "one there isn't, as from a later switchboard: the first", kept: "runway", want: dashboard.Accounts},
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
	h.model.views = []dashboard.View{dashboard.Accounts, "sessions", "runway"}
	h.start()

	if !strings.HasPrefix(h.footer(), "tab views · ") {
		t.Errorf("the footer reads %q, want tab first, there being views to move between", h.footer())
	}
	for _, step := range []struct {
		key  string
		want dashboard.View
	}{
		{key: "tab", want: "sessions"},
		{key: "tab", want: "runway"},
		{key: "tab", want: dashboard.Accounts},
		{key: "shift+tab", want: "runway"},
	} {
		h.press(step.key)
		if h.model.view != step.want || prefs.kept.View != string(step.want) {
			t.Errorf("%s showed %q and kept %q, want %q kept", step.key, h.model.view, prefs.kept.View, step.want)
		}
	}
	if !strings.Contains(h.view(), " runway ") {
		t.Errorf("the screen is\n%s\nwant runway's tab", h.view())
	}
}

func TestWithOneViewTabMovesNowhere(t *testing.T) {
	h := newHarness(t, calm())
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
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

	if !strings.HasPrefix(h.footer(), "w window: auto · ") {
		t.Errorf("the footer reads %q, want w saying the cards feature each its own, as auto has it", h.footer())
	}
	for _, step := range []struct {
		want   dashboard.Feature
		footer string
	}{
		{want: "5h", footer: "w window: 5h · "},
		{want: "7d", footer: "w window: week · "},
		{want: "7d_oi", footer: "w window: Fable wk · "},
		{want: dashboard.Auto, footer: "w window: auto · "},
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

	if !strings.HasPrefix(h.footer(), "w window: auto · ") {
		t.Errorf("the footer reads %q, want auto, as the cards show Fable's week, unused", h.footer())
	}
	if help := h.model.helpKeys(); help[0].Does != "cycle the window every card features, now auto" {
		t.Errorf("the help says w does %q, want it to say auto", help[0].Does)
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

	if !strings.HasPrefix(h.footer(), "w window: week · ") {
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
