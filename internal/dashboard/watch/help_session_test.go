package watch

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// Help over a session's page lists its own keys, not its tab's, j and k,
// PgUp and PgDn and the wheel among them wherever they scroll something: its
// turns, or with a turn open, that turn's requests, which they scroll in its
// turns' place.
func TestHelpOverASessionsPageListsItsKeysNotItsTabs(t *testing.T) {
	running := []string{"tab views", "esc back", "↑↓ turn", "enter its requests", "w models", "p pin", "R refresh", "? help", "q quit"}
	opened := []string{"tab views", "esc close", "↑↓ request", "enter open", "w models", "p pin", "R refresh", "? help", "q quit"}
	scrolled := func(keys []string) []string {
		return slices.Concat(keys[:5], []string{"j k scroll", "PgUp PgDn page", "wheel scroll"}, keys[5:])
	}
	turns := func(s *dashboard.SessionShown, n int) {
		s.Page.Turns = make([]views.Turn, n)
		for i := range s.Page.Turns {
			s.Page.Turns[i].Turn = n - i
		}
		s.Turn = n
	}
	tests := []struct {
		name string
		page func(*dashboard.SessionShown)
		// over is the tab the page is open over, where it isn't the
		// Overview.
		over dashboard.View
		want []string
	}{
		{name: "running", want: running},
		{name: "running, over the Log, whose own keys don't work on it", over: dashboard.Log, want: running},
		{name: "ended, with no p", page: func(s *dashboard.SessionShown) { s.Page.Activity.Doing = views.Ended },
			want: []string{"tab views", "esc back", "↑↓ turn", "enter its requests", "w models", "R refresh", "? help", "q quit"}},
		{name: "a turn open", page: func(s *dashboard.SessionShown) { s.Open = 1 }, want: opened},
		{name: "its turns scrolling", page: func(s *dashboard.SessionShown) { turns(s, 60) }, want: scrolled(running)},
		{name: "a turn open, its requests scrolling where its turns don't",
			page: func(s *dashboard.SessionShown) { s.Open, s.Requests = 1, make([]views.LogLine, 40) }, want: scrolled(opened)},
		{name: "a turn open, its requests all in view where its turns scroll",
			page: func(s *dashboard.SessionShown) { turns(s, 60); s.Open, s.Requests = 60, make([]views.LogLine, 2) }, want: opened},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Opened from the Overview, whose toggles don't work on it.
			h := sessionPaged(t, "")
			if tt.over != "" {
				h.model.view = tt.over
			}
			if tt.page != nil {
				tt.page(&h.model.session.shown)
			}
			var got []string
			for _, k := range helpOver(h.model) {
				got = append(got, strings.TrimSpace(k.Key+" "+k.Does))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("help lists %q, want %q", got, tt.want)
			}
		})
	}
}

// ? on a session's page opens its help, explaining its STORY first,
// whatever tab it's open over.
func TestQuestionMarkOnASessionsPageOpensItsHelp(t *testing.T) {
	for _, over := range []dashboard.View{dashboard.Overview, dashboard.Log} {
		t.Run(string(over), func(t *testing.T) {
			h := sessionPaged(t, "")
			h.model.view = over
			h.keyed("?")
			if screen := h.view(); !h.inHelp() || !strings.Contains(screen, "─ HELP · SESSION ") || !strings.Contains(screen, "│  STORY ") {
				t.Errorf("after ? on a session's page over %s, the screen is\n%s\nwant HELP · SESSION over it, explaining its STORY", over, screen)
			}
		})
	}
}
