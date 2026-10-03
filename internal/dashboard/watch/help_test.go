package watch

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// helpOpen reports whether the help is drawn over the screen.
func (h *harness) helpOpen() bool {
	return strings.Contains(h.view(), "? Keys") && strings.Contains(h.view(), "esc close")
}

func TestQuestionMarkOpensTheHelpAndQuestionMarkOrEscCloseIt(t *testing.T) {
	for _, closing := range []tea.KeyPressMsg{{Code: '?', Text: "?"}, {Code: tea.KeyEscape}} {
		t.Run(closing.String(), func(t *testing.T) {
			h := newHarness(t, calm())
			h.start()
			if h.helpOpen() {
				t.Fatal("the help is open before ? is pressed")
			}
			h.press("?")
			if !h.helpOpen() {
				t.Fatalf("after ?, the screen is\n%s\nwant the help over it", h.view())
			}
			h.update(closing)
			if h.helpOpen() {
				t.Errorf("after %s, the screen is\n%s\nwant the help closed", closing, h.view())
			}
		})
	}
}

func TestTheHelpTakesEveryKeyButThoseThatQuit(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.press("?")
	for _, key := range []string{"w", "1", "a", "r", "t", "j", "tab"} {
		h.deliver(h.press(key)...)
	}
	if !h.helpOpen() || h.model.featured != dashboard.Auto || len(h.source.orders) > 0 || h.model.fetching || h.model.picker.open {
		t.Errorf("with the help open, keys acted: featuring %q, orders %q, reading %v, picker open %v; want them all taken by the help", h.model.featured, h.source.orders, h.model.fetching, h.model.picker.open)
	}
	for _, key := range []string{"q", "ctrl+c"} {
		if msgs := h.press(key); !slices.ContainsFunc(msgs, func(m tea.Msg) bool { _, ok := m.(tea.QuitMsg); return ok }) {
			t.Errorf("with the help open, %s sent %v, want it to quit", key, msgs)
		}
	}
}

func TestTheHelpListsEveryKeyThatWorksAndTheFooterTheMostUsed(t *testing.T) {
	tests := []struct {
		name string
		h    func(t *testing.T) *harness
		// wantHelp are the keys the help lists, and wantFooter those the
		// footer does.
		wantHelp, wantFooter []string
	}{
		{
			name:       "the router of three",
			h:          func(t *testing.T) *harness { return routedHarness(t, routerDocument(three()...)) },
			wantHelp:   []string{"tab", "w", "←→", "space", "s", "1-3", "a", "m", "r", "?", "q"},
			wantFooter: []string{"tab", "w", "←→", "space", "1-3", "a", "m", "?", "q"},
		},
		{
			name:       "probing twelve, which scroll",
			h:          func(t *testing.T) *harness { return newHarness(t, twelve()) },
			wantHelp:   []string{"tab", "w", "←→", "space", "s", "j k", "r", "?", "q"},
			wantFooter: []string{"tab", "w", "←→", "space", "?", "q"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.h(t)
			h.start()
			if got := listedKeys(h.model.helpKeys()); !slices.Equal(got, tt.wantHelp) {
				t.Errorf("the help lists %q, want %q", got, tt.wantHelp)
			}
			if got := listedKeys(h.model.keys()); !slices.Equal(got, tt.wantFooter) {
				t.Errorf("the footer lists %q, want %q", got, tt.wantFooter)
			}
		})
	}
}

func TestTheHelpListsTheThemePickerInColour(t *testing.T) {
	h := newHarness(t, calm())
	h.model.cfg.Themes = &fakeThemes{}
	h.start()
	if got := listedKeys(h.model.helpKeys()); !slices.Contains(got, "t") || slices.Contains(listedKeys(h.model.keys()), "t") {
		t.Errorf("in colour, the help lists %q and the footer %q, want t in the help alone", got, listedKeys(h.model.keys()))
	}
}

func TestTheHelpSaysWhatEachKeyDoes(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.start()
	h.press("?")
	for _, want := range []string{
		"w       cycle the window every card features, now auto",
		"←→      move the focus; ↑↓ between rows, over a flipped card's sessions first",
		"space   flip the card with the focus to its sessions, or back",
		"s       flip every card, or back",
		"1-3     toggle the account in that place in the pin",
		"a       route automatically again",
		"m       move running sessions to the pinned accounts",
		"r       refresh",
		"?       these keys, and the key to the glyphs",
		"q       quit",
		"▆▆      room left",
		"●       session, lit while busy",
	} {
		if !strings.Contains(h.view(), want) {
			t.Errorf("the screen is\n%s\nwant the help to say %q", h.view(), want)
		}
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
