package watch

import (
	"errors"
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
