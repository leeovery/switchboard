package watch

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

// graphsHarness is a harness of the router of three on Graphs, at the size
// given, its preferences kept as prefs has them.
func graphsHarness(t *testing.T, width, height int) (*harness, *fakePrefs) {
	t.Helper()
	h := routedHarness(t, routerDocument(three()...))
	prefs := &fakePrefs{}
	h.model.cfg.Prefs, h.model.view = prefs, dashboard.Graphs
	h.update(tea.WindowSizeMsg{Width: width, Height: height})
	h.start()
	return h, prefs
}

// On Graphs, w, f, o, g and h each show their section where it's hidden and
// hide it where it shows, and the sections shown are kept by name, every one
// hidden kept as none; the Overview's sections, its chart and each model's
// own window are left as they were.
func TestGraphsKeysShowOrHideItsSectionsAndKeepThem(t *testing.T) {
	h, prefs := graphsHarness(t, 160, 37)
	for _, step := range []struct {
		key        string
		want       []string
		wantShown  string
		wantHidden string
	}{
		{key: "o", want: []string{"week", "5-hour", "pool"}, wantShown: "POOL"},
		{key: "w", want: []string{"5-hour", "pool"}, wantHidden: "WEEK"},
		{key: "h", want: []string{"5-hour", "pool", "when"}, wantShown: "WHEN YOU WORK"},
		{key: "g", want: []string{"5-hour", "pool", "glance", "when"}, wantShown: "GLANCE"},
		{key: "f", want: []string{"pool", "glance", "when"}, wantHidden: "5-HOUR"},
		{key: "o", want: []string{"glance", "when"}, wantHidden: "POOL"},
		{key: "g", want: []string{"when"}, wantHidden: "GLANCE"},
		{key: "h", want: []string{}, wantShown: "every section is hidden:"},
	} {
		h.typed(step.key)
		if got := h.model.graphs.Names(); !slices.Equal(got, step.want) || prefs.kept.GraphsTab.Sections == nil || !slices.Equal(prefs.kept.GraphsTab.Sections, step.want) {
			t.Errorf("after %s, showing %q, kept %#v; want %q shown and kept", step.key, got, prefs.kept.GraphsTab.Sections, step.want)
		}
		if shown := h.view(); step.wantShown != "" && !strings.Contains(shown, step.wantShown) || step.wantHidden != "" && strings.Contains(shown, step.wantHidden+" ") {
			t.Errorf("after %s, the screen is\n%s\nwant %q on it, and no %q", step.key, shown, step.wantShown, step.wantHidden)
		}
	}
	if h.model.sections != (dashboard.Sections{}) || h.model.models || h.model.burn != dashboard.BurnUp || prefs.kept.Sections != nil || prefs.kept.Models {
		t.Errorf("Graphs' keys left the Overview showing %q, models %t, its chart %v, kept %#v, models %t; want them as they were",
			h.model.sections.Names(), h.model.models, h.model.burn, prefs.kept.Sections, prefs.kept.Models)
	}
}

// Graphs' keys act on Graphs alone: on the Overview, g and w are its own,
// and on the Log, f opens its panel.
func TestGraphsKeysLeaveItsSectionsAsTheyWereOnAnotherTab(t *testing.T) {
	for _, view := range []dashboard.View{dashboard.Overview, dashboard.Runway, dashboard.Log} {
		t.Run(string(view), func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.model.view = view
			h.start()
			h.typed("o", "h")
			if h.model.graphs != (dashboard.GraphsSections{}) {
				t.Errorf("on %s, o and h left Graphs showing %q, want them as they were", view, h.model.graphs.Names())
			}
		})
	}
}

func TestTheWatchOpensGraphsShowingTheSectionsThePreferencesKept(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	cfg := h.config(Size{Width: 160, Height: 37})
	cfg.View, cfg.Graphs = dashboard.Graphs, dashboard.GraphsSectionsNamed([]string{"glance"})
	h.model = New(t.Context(), cfg)
	h.start()
	if got := h.model.frame(h.clock.now).Graphs.Sections.Names(); !slices.Equal(got, []string{"glance"}) {
		t.Errorf("the watch opens Graphs showing %q, want glance, as kept", got)
	}
	if shown := h.view(); !strings.Contains(shown, "GLANCE ") || strings.Contains(shown, "WEEK ") {
		t.Errorf("the screen is\n%s\nwant GLANCE's heading alone", shown)
	}
}

// Graphs' key line is a page of toggles', as the Overview's: wide, tab
// views, then each toggle, its word faint where its section is hidden, then
// p pin, ? help and q quit, each group five cells apart; on a phone, tab
// views, ? keys and toggles and q quit.
func TestGraphsKeyLineIsTheDesigns(t *testing.T) {
	toggles := "tab views     w week  f 5-hour  o pool  g glance  h when     "
	phone := "tab views   ? keys and toggles   q quit"
	tests := []struct {
		name   string
		doc    status.Document
		width  int
		typed  []string
		want   string
		hidden []string
	}{
		{name: "three accounts", doc: routerDocument(three()...), width: 160, want: toggles + "p pin  ? help  q quit", hidden: []string{"pool", "glance", "when"}},
		{name: "WEEK hidden, POOL and WHEN YOU WORK shown", doc: routerDocument(three()...), width: 160, typed: []string{"w", "o", "h"}, want: toggles + "p pin  ? help  q quit", hidden: []string{"week", "glance"}},
		{name: "one account, with none other to pin to", doc: routerDocument(three()[0]), width: 160, want: toggles + "? help  q quit", hidden: []string{"pool", "glance", "when"}},
		{name: "the wide chrome's narrowest", doc: routerDocument(three()...), width: 100, want: toggles + "p pin  ? help  q quit", hidden: []string{"pool", "glance", "when"}},
		{name: "a phone", doc: routerDocument(three()...), width: 52, want: phone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, tt.doc)
			h.model.view = dashboard.Graphs
			h.update(tea.WindowSizeMsg{Width: tt.width, Height: 40})
			h.start()
			h.typed(tt.typed...)

			rows := strings.Split(h.view(), "\n")
			if got := strings.TrimRight(rows[len(rows)-1], " "); got != tt.want {
				t.Errorf("at %d columns, the key line reads\n%q\nwant\n%q", tt.width, got, tt.want)
			}
			var hidden []string
			for _, k := range h.model.live(h.clock.now).KeyLine.Groups[1] {
				if k.Hidden {
					hidden = append(hidden, k.Does)
				}
			}
			if tt.width >= 100 && !slices.Equal(hidden, tt.hidden) {
				t.Errorf("the toggles hidden are %q, want %q", hidden, tt.hidden)
			}
		})
	}
}

// A click on a section's heading, or on its key at the heading's end, hides
// it as its key does, and keeps it so; a click on a toggle of the key line
// shows its section.
func TestAClickOnGraphsHeadingOrAToggleActsAsItsKey(t *testing.T) {
	tests := []struct {
		name string
		// at is the cell clicked, as the screen draws it.
		at   func(h *harness) (x, y int)
		want []string
	}{
		{name: "WEEK's heading", at: func(h *harness) (int, int) { return h.drawnAt("WEEK ") }, want: []string{"5-hour"}},
		{name: "5-HOUR's key", at: func(h *harness) (int, int) {
			_, y := h.drawnAt("5-HOUR ")
			return h.model.size.Width - 1, y
		}, want: []string{"week"}},
		{name: "the key line's o pool", at: func(h *harness) (int, int) {
			x, y := h.drawnAt("o pool")
			return x + ansi.StringWidth("o pool") - 1, y
		}, want: []string{"week", "5-hour", "pool"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, prefs := graphsHarness(t, 160, 37)
			x, y := tt.at(h)
			if h.click(x, y) || !slices.Equal(h.model.graphs.Names(), tt.want) || !slices.Equal(prefs.kept.GraphsTab.Sections, tt.want) {
				t.Errorf("a click at %d,%d left Graphs showing %q, kept %q; want %q", x, y, h.model.graphs.Names(), prefs.kept.GraphsTab.Sections, tt.want)
			}
		})
	}
}

// Graphs' help lists its toggles first, then the keys every page takes, the
// scroll keys always.
func TestGraphsHelpListsItsTogglesThenTheSharedKeys(t *testing.T) {
	h, _ := graphsHarness(t, 160, 37)
	h.typed("o")
	want := []dashboard.Key{
		{Key: "tab", Does: "views"},
		{Key: "w", Does: "week"}, {Key: "f", Does: "5-hour"}, {Key: "o", Does: "pool"},
		{Key: "g", Does: "glance", Hidden: true}, {Key: "h", Does: "when", Hidden: true},
		{Key: "j k", Does: "scroll"}, {Key: "PgUp PgDn", Does: "page"}, {Key: "wheel", Does: "scroll"},
		{Key: "p", Does: "pin"}, {Key: "R", Does: "refresh"}, {Key: "?", Does: "help"}, {Key: "q", Does: "quit"},
	}
	if got := helpOver(h.model); !slices.Equal(got, want) {
		t.Errorf("help lists\n%+v\nwant\n%+v", got, want)
	}
}

// Over Graphs, a session's page keeps w as its own, showing and hiding each
// model's own window, and leaves Graphs' sections as they were.
func TestASessionsPageOverGraphsKeepsWForModels(t *testing.T) {
	h := sessionPaged(t, "")
	h.model.view = dashboard.Graphs
	h.typed("w")
	if !h.model.models || h.model.graphs != (dashboard.GraphsSections{}) {
		t.Errorf("w on a session's page over Graphs left models shown %t, Graphs showing %q; want models shown, Graphs as it was", h.model.models, h.model.graphs.Names())
	}
}
