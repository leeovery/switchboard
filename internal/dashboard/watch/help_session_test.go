package watch

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

func TestHelpOverASessionsPageListsItsKeysNotItsTabs(t *testing.T) {
	tests := []struct {
		name string
		page func(*dashboard.SessionShown)
		// over is the tab the page is open over, where it isn't the
		// Overview.
		over dashboard.View
		want []string
	}{
		{name: "running", want: []string{"tab views", "esc back", "↑↓ turn", "enter its requests", "p pin", "R refresh", "? help", "q quit"}},
		{name: "running, over the Log, whose own keys don't work on it", over: dashboard.Log,
			want: []string{"tab views", "esc back", "↑↓ turn", "enter its requests", "p pin", "R refresh", "? help", "q quit"}},
		{name: "ended, with no p", page: func(s *dashboard.SessionShown) { s.Page.Activity.Doing = views.Ended },
			want: []string{"tab views", "esc back", "↑↓ turn", "enter its requests", "R refresh", "? help", "q quit"}},
		{name: "a turn open", page: func(s *dashboard.SessionShown) { s.Open = 1 },
			want: []string{"tab views", "esc close", "↑↓ request", "enter open", "p pin", "R refresh", "? help", "q quit"}},
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
				if !slices.Contains([]string{"j k", "PgUp PgDn", "wheel"}, k.Key) {
					got = append(got, strings.TrimSpace(k.Key+" "+k.Does))
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("help lists %q, want %q, but for the scroll keys, which follow its turns", got, tt.want)
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
