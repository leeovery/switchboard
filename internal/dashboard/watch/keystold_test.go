package watch

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// keysTold are the keys given, each as "key does".
func keysTold(keys []dashboard.Key) []string {
	said := make([]string, len(keys))
	for i, k := range keys {
		said[i] = k.Key + " " + k.Does
	}
	return said
}

// The keys told outside the dashboard are every page's, then each tab's, in
// the title row's order, then a session's page's: each key a page takes in
// any of its states, by its key and what it does alone, once, a key worded
// otherwise in another state after the word it takes in the first.
func TestEveryPagesKeysAreEachPagesAsItsHelpListsThem(t *testing.T) {
	want := map[string][]string{
		"every page": {"tab views", "p pin", "t themes", "R refresh", "? help", "q quit", "j k scroll", "PgUp PgDn page", "wheel scroll"},
		"Overview":   {"u usage", "g graphs", "r routing", "s sessions", "c coming up", "l lately", "w models", "b by", "v chart", "↑↓ focus"},
		"Accounts": {
			"[ ] view", "< > period", "←→ account", "↑↓ account", "↑↓ session", "→ sessions", "← accounts", "enter its page", "esc unfocus", "w models",
		},
		"History": {"[ ] view", "< > period", "v split", "←→ account"},
		"Runway":  {"< > window", "←→ day", "←→ week", "a names"},
		"Log": {
			"[ ] view", "↑↓ request", "↑↓ event", "enter open", "s its session", "end follow", "esc follow", "f what it shows", "/ find",
			"esc clear the filter",
		},
		"a session's page": {"esc back", "esc close", "↑↓ turn", "↑↓ request", "enter its requests", "enter open", "w models"},
	}
	pages := EveryPagesKeys()
	var named []string
	for _, p := range pages {
		named = append(named, p.Page)
		if keys, ok := want[p.Page]; ok && !slices.Equal(keysTold(p.Keys), keys) {
			t.Errorf("%s's keys are told %q, want %q", p.Page, keysTold(p.Keys), keys)
		}
	}
	if want := []string{"every page", "Overview", "Accounts", "History", "Sessions", "Runway", "Log", "a session's page"}; !slices.Equal(named, want) {
		t.Errorf("the pages are told in the order %q, want %q", named, want)
	}
	sessions := pages[slices.IndexFunc(pages, func(p PageKeys) bool { return p.Page == "Sessions" })]
	if got := keysTold(sessions.Keys); len(got) < 3 || !slices.Equal(got[:3], []string{"[ ] view", "↑↓ session", "enter its page"}) {
		t.Errorf("Sessions' keys are told %q, want them to start [ ] view, ↑↓ session, enter its page", got)
	}
}

// Every key a page's help lists, in each state it's shown in, is told among
// every page's keys or the page's own: so what tells of them outside the
// dashboard never leaves one out.
func TestEveryKeyHelpListsIsTold(t *testing.T) {
	tests := []struct {
		name string
		page string
		show func(h *harness)
	}{
		{name: "the Overview", page: "Overview", show: func(*harness) {}},
		{name: "Compare", page: "Accounts", show: func(h *harness) { h.model.view = dashboard.Accounts }},
		{name: "Detail, in its SESSIONS", page: "Accounts", show: func(h *harness) {
			h.model.view = dashboard.Accounts
			h.model.accounts.shown.Shown, h.model.accounts.shown.InSessions = dashboard.AccountsDetail, true
		}},
		{name: "History", page: "History", show: func(h *harness) { h.model.view = dashboard.History }},
		{name: "Sessions", page: "Sessions", show: func(h *harness) { h.model.view = dashboard.Sessions }},
		{name: "Runway's week", page: "Runway", show: func(h *harness) {
			h.model.view, h.model.runwayTab.shown.Window = dashboard.Runway, views.RunwayWeek
		}},
		{name: "the Log's Events", page: "Log", show: func(h *harness) {
			h.model.view, h.model.log.page.Shown = dashboard.Log, dashboard.LogEvents
		}},
	}
	pages := EveryPagesKeys()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			tt.show(h)
			h.start()
			page := pages[slices.IndexFunc(pages, func(p PageKeys) bool { return p.Page == tt.page })]
			known := keysTold(slices.Concat(pages[0].Keys, page.Keys))
			for _, k := range keysTold(helpOver(h.model)) {
				if !slices.Contains(known, k) {
					t.Errorf("help lists %q, which isn't told among %q", k, known)
				}
			}
		})
	}
}
