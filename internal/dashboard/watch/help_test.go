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
			name: "probing twelve in Runway, its lanes not drawn",
			h: func(t *testing.T) *harness {
				h := newHarness(t, twelve())
				h.model.view = dashboard.Runway
				return h
			},
			wantHelp: []string{"tab", "p", "R", "?", "q"},
			wantLine: []string{"tab", "p", "?", "q"},
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

func TestHelpListsTheScrollingKeysOfThePageAtTheSizeItsDrawnAt(t *testing.T) {
	scrolling := []string{"j k", "PgUp PgDn", "wheel"}
	// History's Year, as the Overview lists its scrolling keys at any size.
	h := yearHarness(t, tea.WindowSizeMsg{Width: 150, Height: 40})
	h.press("?")
	if got := listedKeys(h.model.helpCard().Keys); slices.ContainsFunc(scrolling, func(k string) bool { return slices.Contains(got, k) }) {
		t.Fatalf("help over a page that fits lists %q, want no scrolling keys", got)
	}

	h.update(tea.WindowSizeMsg{Width: 150, Height: 20})
	if f, doc, at := h.model.screen(h.clock.now); f.Scrolling(doc, at).Most == 0 {
		t.Fatal("the page behind help doesn't scroll at 150x20, want it to")
	}
	if got := listedKeys(h.model.helpCard().Keys); !h.inHelp() || !containsInOrder(got, scrolling) {
		t.Errorf("help over a page shrunk till it scrolls lists %q, open %v; want %q, still open", got, h.inHelp(), scrolling)
	}

	h.update(tea.WindowSizeMsg{Width: 150, Height: 50})
	if got := listedKeys(h.model.helpCard().Keys); slices.ContainsFunc(scrolling, func(k string) bool { return slices.Contains(got, k) }) {
		t.Errorf("help over a page grown till it fits again lists %q, want no scrolling keys", got)
	}
}

// containsInOrder reports whether want appear in got, one after another.
func containsInOrder(got, want []string) bool {
	i := slices.Index(got, want[0])
	return i >= 0 && len(got) >= i+len(want) && slices.Equal(got[i:i+len(want)], want)
}

// helpOver is the keys help lists, opened over the page as m stands.
func helpOver(m Model) []dashboard.Key {
	return m.openHelp().helpCard().Keys
}

// listedKeys are the keys of keys, in order.
func listedKeys(keys []dashboard.Key) []string {
	listed := make([]string, len(keys))
	for i, k := range keys {
		listed[i] = k.Key
	}
	return listed
}

// Help on the Log lists j and k, PgUp and PgDn and the wheel, as its tables
// scroll back through the days, however few lines they hold.
func TestTheLogsHelpListsTheKeysThatScrollIt(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	h.model.view = dashboard.Log
	h.start()
	h.typed("?")
	got := listedKeys(h.model.helpCard().Keys)
	for _, want := range []string{"j k", "PgUp PgDn", "wheel"} {
		if !slices.Contains(got, want) {
			t.Errorf("the Log's help lists %q, want %s among them", got, want)
		}
	}
}
