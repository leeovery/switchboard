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
	for _, key := range []string{"w", "1", "a", "m", "r", "R", "t", "j", "g", "p", "tab", " ", "s", "q"} {
		if quits(h.press(key)) {
			t.Errorf("with help open, %q quit, want it taken by help", key)
		}
	}
	if !h.inHelp() || h.model.featured != dashboard.Auto || h.model.chart != dashboard.Burndown || h.model.view != dashboard.Overview ||
		len(h.source.orders) > 0 || h.model.fetching || h.model.picker.open || h.model.flipped != nil {
		t.Errorf("with help open, keys acted: featuring %q, drawing %q, showing %q, orders %q, reading %v, picker open %v, flipped %v; want them all taken by help",
			h.model.featured, h.model.chart, h.model.view, h.source.orders, h.model.fetching, h.model.picker.open, h.model.flipped)
	}
	if !quits(h.press("ctrl+c")) {
		t.Error("with help open, ctrl+c didn't quit, want it to")
	}
}

func TestHelpListsEveryKeyThatWorksAndTheKeyLineTheMostUsed(t *testing.T) {
	pinned := routerDocument(three()[0])
	pinned.Pin = status.Pin{Accounts: []string{"work"}}
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
			wantHelp: []string{"tab", "w", "←→", "space", "s", "g", "p", "R", "?", "q"},
			wantLine: []string{"tab", "w", "←→", "space", "g", "p", "?", "q"},
		},
		{
			name:     "probing twelve, which scroll",
			h:        func(t *testing.T) *harness { return newHarness(t, twelve()) },
			wantHelp: []string{"tab", "w", "←→", "space", "s", "g", "j k", "PgUp PgDn", "wheel", "p", "R", "?", "q"},
			wantLine: []string{"tab", "w", "←→", "space", "g", "p", "?", "q"},
		},
		{
			name:     "one account, with nowhere else to pin",
			h:        func(t *testing.T) *harness { return routedHarness(t, routerDocument(three()[0])) },
			wantHelp: []string{"tab", "w", "←→", "space", "s", "g", "R", "?", "q"},
			wantLine: []string{"tab", "w", "←→", "space", "g", "?", "q"},
		},
		{
			name:     "one account, pinned from the command line",
			h:        func(t *testing.T) *harness { return routedHarness(t, pinned) },
			wantHelp: []string{"tab", "w", "←→", "space", "s", "g", "p", "R", "?", "q"},
			wantLine: []string{"tab", "w", "←→", "space", "g", "p", "?", "q"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.h(t)
			h.start()
			if got := listedKeys(helpOver(h.model)); !slices.Equal(got, tt.wantHelp) {
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
	if got := listedKeys(helpOver(h.model)); !slices.Contains(got, "t") || slices.Contains(h.listed(), "t") {
		t.Errorf("in colour, help lists %q and the key line %q, want t in help alone", got, h.listed())
	}
}

func TestADocumentLandingUnderHelpMovesNothingInIt(t *testing.T) {
	h := newHarness(t, calm())
	h.start()
	h.press("?")
	before := h.view()

	h.source.doc = twelve()
	h.tickUntilRead()
	if len(h.model.doc.Accounts) != len(twelve().Accounts) {
		t.Fatalf("the watch has %d accounts after the read, want the twelve it read", len(h.model.doc.Accounts))
	}
	if got := h.view(); got != before {
		t.Errorf("once a document that scrolls lands under help, the screen is\n%s\nwant it as help opened\n%s", got, before)
	}
}

// helpOver is the keys help lists, opened over the page as m stands.
func helpOver(m Model) []dashboard.Key {
	return m.openHelp().help.keys
}

// listedKeys are the keys of keys, in order.
func listedKeys(keys []dashboard.Key) []string {
	listed := make([]string, len(keys))
	for i, k := range keys {
		listed[i] = k.Key
	}
	return listed
}
