package watch

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

// inHelp reports whether help is open over the page.
func (h *harness) inHelp() bool {
	return h.model.open == helpOpen
}

func TestQuestionMarkOpensHelpAndQuestionMarkOrEscCloseIt(t *testing.T) {
	for _, closing := range []tea.KeyPressMsg{{Code: '?', Text: "?"}, {Code: tea.KeyEscape}} {
		t.Run(closing.String(), func(t *testing.T) {
			h := newHarness(t, calm())
			h.start()
			if h.inHelp() {
				t.Fatal("help is open before ? is pressed")
			}
			h.press("?")
			if !h.inHelp() || !strings.Contains(h.view(), "─ HELP · OVERVIEW ") {
				t.Fatalf("after ?, the screen is\n%s\nwant help over it, titled as the page is named", h.view())
			}
			h.update(closing)
			if h.inHelp() || strings.Contains(h.view(), "HELP · ") {
				t.Errorf("after %s, the screen is\n%s\nwant help closed", closing, h.view())
			}
		})
	}
}

func TestHelpTakesEveryKeyButCtrlC(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.press("?")
	for _, key := range []string{"w", "1", "a", "m", "r", "R", "t", "j", "g", "u", "b", "down", "p", "tab", " ", "s", "q"} {
		if quits(h.press(key)) {
			t.Errorf("with help open, %q quit, want it taken by help", key)
		}
	}
	if !h.inHelp() || h.model.sections != (dashboard.Sections{}) || h.model.by != dashboard.ByWindow || h.model.focus != "work" || h.model.view != dashboard.Overview ||
		len(h.source.orders) > 0 || h.model.fetching || h.model.picker.open {
		t.Errorf("with help open, keys acted: sections %v, by %v, focus %q, showing %q, orders %q, reading %v, picker open %v; want them all taken by help",
			h.model.sections.Names(), h.model.by, h.model.focus, h.model.view, h.source.orders, h.model.fetching, h.model.picker.open)
	}
	if !quits(h.press("ctrl+c")) {
		t.Error("with help open, ctrl+c didn't quit, want it to")
	}
}

func TestHelpListsEveryKeyThatWorksAndTheKeyLineTheMostUsed(t *testing.T) {
	pinned := routerDocument(three()[0])
	pinned.Pin = status.Pin{Accounts: []string{"work"}}
	toggles := []string{"u", "g", "r", "s", "c", "l", "b"}
	scrolling := []string{"j k", "PgUp PgDn", "wheel"}
	tests := []struct {
		name string
		h    func(t *testing.T) *harness
		// wantHelp are the keys help lists, and wantLine those the key
		// line does.
		wantHelp, wantLine []string
	}{
		{
			name:     "the router of three",
			h:        func(t *testing.T) *harness { return routedHarness(t, routerDocument(three()...)) },
			wantHelp: slices.Concat([]string{"tab"}, toggles, []string{"↑↓"}, scrolling, []string{"p", "R", "?", "q"}),
			wantLine: slices.Concat([]string{"tab"}, toggles, []string{"↑↓", "p", "?", "q"}),
		},
		{
			name: "the router of three in Sessions, which doesn't scroll",
			h: func(t *testing.T) *harness {
				h := routedHarness(t, routerDocument(three()...))
				h.model.view = dashboard.Sessions
				return h
			},
			wantHelp: []string{"tab", "p", "R", "?", "q"},
			wantLine: []string{"tab", "p", "?", "q"},
		},
		{
			name: "probing twelve in Runway, which scroll",
			h: func(t *testing.T) *harness {
				h := newHarness(t, twelve())
				h.model.view = dashboard.Runway
				h.update(runwaySize)
				return h
			},
			wantHelp: []string{"tab", "w", "j k", "PgUp PgDn", "wheel", "p", "R", "?", "q"},
			wantLine: []string{"tab", "w", "p", "?", "q"},
		},
		{
			name:     "one account, with nowhere else to pin",
			h:        func(t *testing.T) *harness { return routedHarness(t, routerDocument(three()[0])) },
			wantHelp: slices.Concat([]string{"tab"}, toggles, scrolling, []string{"R", "?", "q"}),
			wantLine: slices.Concat([]string{"tab"}, toggles, []string{"?", "q"}),
		},
		{
			name:     "one account, pinned from the command line",
			h:        func(t *testing.T) *harness { return routedHarness(t, pinned) },
			wantHelp: slices.Concat([]string{"tab"}, toggles, scrolling, []string{"p", "R", "?", "q"}),
			wantLine: slices.Concat([]string{"tab"}, toggles, []string{"p", "?", "q"}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.h(t)
			h.start()
			if got := listedKeys(h.model.helpKeys()); !slices.Equal(got, tt.wantHelp) {
				t.Errorf("help lists %q, want %q", got, tt.wantHelp)
			}
			if got := h.listed(); !slices.Equal(got, tt.wantLine) {
				t.Errorf("the key line lists %q, want %q", got, tt.wantLine)
			}
		})
	}
}

func TestHelpListsTheThemePickerInColour(t *testing.T) {
	h := newHarness(t, calm())
	h.model.cfg.Themes = &fakeThemes{}
	h.start()
	if got := listedKeys(h.model.helpKeys()); !slices.Contains(got, "t") || slices.Contains(h.listed(), "t") {
		t.Errorf("in colour, help lists %q and the key line %q, want t in help alone", got, h.listed())
	}
}

// listedKeys are the keys of keys, in order.
func listedKeys(keys []dashboard.Key) []string {
	listed := make([]string, len(keys))
	for i, k := range keys {
		listed[i] = k.Key
	}
	return listed
}
